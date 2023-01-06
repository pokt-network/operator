/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package nodes

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	"github.com/nukleros/operator-builder-tools/pkg/controller/phases"
	"github.com/nukleros/operator-builder-tools/pkg/controller/predicates"
	"github.com/nukleros/operator-builder-tools/pkg/controller/workload"
	"github.com/nukleros/operator-builder-tools/pkg/resources"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	nodesv1alpha1 "github.com/pokt-network/pocket-operator/apis/nodes/v1alpha1"
	"github.com/pokt-network/pocket-operator/apis/nodes/v1alpha1/pocketvalidator"
	"github.com/pokt-network/pocket-operator/internal/dependencies"
	"github.com/pokt-network/pocket-operator/internal/mutate"
)

// PocketValidatorReconciler reconciles a PocketValidator object.
type PocketValidatorReconciler struct {
	client.Client
	Name         string
	Log          logr.Logger
	Controller   controller.Controller
	Events       record.EventRecorder
	FieldManager string
	Watches      []client.Object
	Phases       *phases.Registry
}

func NewPocketValidatorReconciler(mgr ctrl.Manager) *PocketValidatorReconciler {
	return &PocketValidatorReconciler{
		Name:         "PocketValidator",
		Client:       mgr.GetClient(),
		Events:       mgr.GetEventRecorderFor("PocketValidator-Controller"),
		FieldManager: "PocketValidator-reconciler",
		Log:          ctrl.Log.WithName("controllers").WithName("nodes").WithName("PocketValidator"),
		Watches:      []client.Object{},
		Phases:       &phases.Registry{},
	}
}

// +kubebuilder:rbac:groups=nodes.pokt.network,resources=pocketvalidators,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=nodes.pokt.network,resources=pocketvalidators/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=nodes.pokt.network,resources=pocketsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=nodes.pokt.network,resources=pocketsets/status,verbs=get;update;patch

// Until Webhooks are implemented we need to list and watch namespaces to ensure
// they are available before deploying resources,
// See:
//   - https://github.com/vmware-tanzu-labs/operator-builder/issues/141
//   - https://github.com/vmware-tanzu-labs/operator-builder/issues/162

// +kubebuilder:rbac:groups=core,resources=namespaces,verbs=list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.7.2/pkg/reconcile
func (r *PocketValidatorReconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	req, err := r.NewRequest(ctx, request)
	if err != nil {
		if errors.Is(err, workload.ErrCollectionNotFound) {
			return ctrl.Result{Requeue: true}, nil
		}

		if !apierrs.IsNotFound(err) {
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, nil
	}

	if err := phases.RegisterDeleteHooks(r, req); err != nil {
		return ctrl.Result{}, err
	}

	// execute the phases
	return r.Phases.HandleExecution(r, req)
}

func (r *PocketValidatorReconciler) NewRequest(ctx context.Context, request ctrl.Request) (*workload.Request, error) {
	component := &nodesv1alpha1.PocketValidator{}

	log := r.Log.WithValues(
		"kind", component.GetWorkloadGVK().Kind,
		"name", request.Name,
		"namespace", request.Namespace,
	)

	// get the component from the cluster
	if err := r.Get(ctx, request.NamespacedName, component); err != nil {
		if !apierrs.IsNotFound(err) {
			log.Error(err, "unable to fetch workload")

			return nil, fmt.Errorf("unable to fetch workload, %w", err)
		}

		return nil, err
	}

	// create the workload request
	workloadRequest := &workload.Request{
		Context:  ctx,
		Workload: component,
		Log:      log,
	}

	// store the collection and return any resulting error
	return workloadRequest, r.SetCollection(component, workloadRequest)
}

// SetCollection sets the collection for a particular workload request.
func (r *PocketValidatorReconciler) SetCollection(component *nodesv1alpha1.PocketValidator, req *workload.Request) error {
	collection, err := r.GetCollection(component, req)
	if err != nil || collection == nil {
		return fmt.Errorf("unable to set collection, %w", err)
	}

	req.Collection = collection

	return r.EnqueueRequestOnCollectionChange(req)
}

// GetCollection gets a collection for a component given a list.
func (r *PocketValidatorReconciler) GetCollection(
	component *nodesv1alpha1.PocketValidator,
	req *workload.Request,
) (*nodesv1alpha1.PocketSet, error) {
	var collectionList nodesv1alpha1.PocketSetList

	if err := r.List(req.Context, &collectionList); err != nil {
		return nil, fmt.Errorf("unable to list collection PocketSet, %w", err)
	}

	// determine if we have requested a specific collection
	name, namespace := component.Spec.Collection.Name, component.Spec.Collection.Namespace

	var collectionRef nodesv1alpha1.PocketValidatorCollectionSpec

	hasSpecificCollection := component.Spec.Collection != collectionRef && component.Spec.Collection.Name != ""

	// if a specific collection has not been requested, we ensure only one exists
	if !hasSpecificCollection {
		if len(collectionList.Items) != 1 {
			return nil, fmt.Errorf("expected only 1 PocketSet collection, found %v", len(collectionList.Items))
		}

		return &collectionList.Items[0], nil
	}

	// find the collection that was requested and return it
	for _, collection := range collectionList.Items {
		if collection.Name == name && collection.Namespace == namespace {
			return &collection, nil
		}
	}

	return nil, workload.ErrCollectionNotFound
}

// EnqueueRequestOnCollectionChange enqueues a reconcile request when an associated collection object changes.
func (r *PocketValidatorReconciler) EnqueueRequestOnCollectionChange(req *workload.Request) error {
	if len(r.Watches) > 0 {
		for _, watched := range r.Watches {
			if reflect.DeepEqual(
				req.Collection.GetObjectKind().GroupVersionKind(),
				watched.GetObjectKind().GroupVersionKind(),
			) {
				return nil
			}
		}
	}

	// create a function which maps this specific reconcile request
	mapFn := func(collection client.Object) []reconcile.Request {
		return []reconcile.Request{
			{
				NamespacedName: types.NamespacedName{
					Name:      req.Workload.GetName(),
					Namespace: req.Workload.GetNamespace(),
				},
			},
		}
	}

	// watch the collection and use our map function to enqueue the request
	if err := r.Controller.Watch(
		&source.Kind{Type: req.Collection},
		handler.EnqueueRequestsFromMapFunc(mapFn),
		predicate.Funcs{
			UpdateFunc: func(e event.UpdateEvent) bool {
				if !resources.EqualNamespaceName(e.ObjectNew, req.Collection) {
					return false
				}

				return e.ObjectNew != e.ObjectOld
			},
			CreateFunc: func(e event.CreateEvent) bool {
				return false
			},
			GenericFunc: func(e event.GenericEvent) bool {
				return false
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				return false
			},
		},
	); err != nil {
		return err
	}

	r.Watches = append(r.Watches, req.Collection)

	return nil
}

// GetResources resources runs the methods to properly construct the resources in memory.
func (r *PocketValidatorReconciler) GetResources(req *workload.Request) ([]client.Object, error) {
	component, collection, err := pocketvalidator.ConvertWorkload(req.Workload, req.Collection)
	if err != nil {
		return nil, err
	}

	return pocketvalidator.Generate(*component, *collection, r, req)
}

// GetEventRecorder returns the event recorder for writing kubernetes events.
func (r *PocketValidatorReconciler) GetEventRecorder() record.EventRecorder {
	return r.Events
}

// GetFieldManager returns the name of the field manager for the controller.
func (r *PocketValidatorReconciler) GetFieldManager() string {
	return r.FieldManager
}

// GetLogger returns the logger from the reconciler.
func (r *PocketValidatorReconciler) GetLogger() logr.Logger {
	return r.Log
}

// GetName returns the name of the reconciler.
func (r *PocketValidatorReconciler) GetName() string {
	return r.Name
}

// GetController returns the controller object associated with the reconciler.
func (r *PocketValidatorReconciler) GetController() controller.Controller {
	return r.Controller
}

// GetWatches returns the objects which are current being watched by the reconciler.
func (r *PocketValidatorReconciler) GetWatches() []client.Object {
	return r.Watches
}

// SetWatch appends a watch to the list of currently watched objects.
func (r *PocketValidatorReconciler) SetWatch(watch client.Object) {
	r.Watches = append(r.Watches, watch)
}

// CheckReady will return whether a component is ready.
func (r *PocketValidatorReconciler) CheckReady(req *workload.Request) (bool, error) {
	return dependencies.PocketValidatorCheckReady(r, req)
}

// Mutate will run the mutate function for the workload.
// WARN: this will be deprecated in the future.  See apis/group/version/kind/mutate*
func (r *PocketValidatorReconciler) Mutate(
	req *workload.Request,
	object client.Object,
) ([]client.Object, bool, error) {
	return mutate.PocketValidatorMutate(r, req, object)
}

func (r *PocketValidatorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.InitializePhases()

	baseController, err := ctrl.NewControllerManagedBy(mgr).
		WithEventFilter(predicates.WorkloadPredicates()).
		For(&nodesv1alpha1.PocketValidator{}).
		Build(r)
	if err != nil {
		return fmt.Errorf("unable to setup controller, %w", err)
	}

	r.Controller = baseController

	return nil
}
