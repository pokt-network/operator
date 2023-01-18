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

package pocketvalidator

import (
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/nukleros/operator-builder-tools/pkg/controller/workload"

	nodesv1alpha1 "github.com/pokt-network/pocket-operator/apis/nodes/v1alpha1"
)

// samplePocketValidator is a sample containing all fields
const samplePocketValidator = `apiVersion: nodes.pokt.network/v1alpha1
kind: PocketValidator
metadata:
  name: pocketvalidator-sample
spec:
  #collection:
    #name: "pocketset-sample"
    #namespace: ""
  prometheusScrape: false
  pocketImage: "ghcr.io/pokt-network/pocket-v1:main-dev"
  ports:
    consensus: 8080
    rpc: 50832
  privateKey:
    secretKeyRef:
      name: "v1-validator1"
      key: "private_key"
  postgres:
    user:
      secretKeyRef:
        name: "postgres-credentials"
        key: "username"
    password:
      secretKeyRef:
        name: "postgres-credentials"
        key: "postgres-password"
    host: "postgres-host"
    port: "5432"
    database: "validatordb"
    schema: "v1-validator1"
`

// samplePocketValidatorRequired is a sample containing only required fields
const samplePocketValidatorRequired = `apiVersion: nodes.pokt.network/v1alpha1
kind: PocketValidator
metadata:
  name: pocketvalidator-sample
spec:
  #collection:
    #name: "pocketset-sample"
    #namespace: ""
  pocketImage: "ghcr.io/pokt-network/pocket-v1:main-dev"
  privateKey:
    secretKeyRef:
      name: "v1-validator1"
      key: "private_key"
  postgres:
    user:
      secretKeyRef:
        name: "postgres-credentials"
        key: "username"
    password:
      secretKeyRef:
        name: "postgres-credentials"
        key: "postgres-password"
    host: "postgres-host"
    port: "5432"
`

// Sample returns the sample manifest for this custom resource.
func Sample(requiredOnly bool) string {
	if requiredOnly {
		return samplePocketValidatorRequired
	}

	return samplePocketValidator
}

// Generate returns the child resources that are associated with this workload given
// appropriate structured inputs.
func Generate(
	workloadObj nodesv1alpha1.PocketValidator,
	collectionObj nodesv1alpha1.PocketSet,
	reconciler workload.Reconciler,
	req *workload.Request,
) ([]client.Object, error) {
	resourceObjects := []client.Object{}

	for _, f := range CreateFuncs {
		resources, err := f(&workloadObj, &collectionObj, reconciler, req)

		if err != nil {
			return nil, err
		}

		resourceObjects = append(resourceObjects, resources...)
	}

	return resourceObjects, nil
}

// GenerateForCLI returns the child resources that are associated with this workload given
// appropriate YAML manifest files.
func GenerateForCLI(workloadFile []byte, collectionFile []byte) ([]client.Object, error) {
	var workloadObj nodesv1alpha1.PocketValidator
	if err := yaml.Unmarshal(workloadFile, &workloadObj); err != nil {
		return nil, fmt.Errorf("failed to unmarshal yaml into workload, %w", err)
	}

	if err := workload.Validate(&workloadObj); err != nil {
		return nil, fmt.Errorf("error validating workload yaml, %w", err)
	}

	var collectionObj nodesv1alpha1.PocketSet
	if err := yaml.Unmarshal(collectionFile, &collectionObj); err != nil {
		return nil, fmt.Errorf("failed to unmarshal yaml into collection, %w", err)
	}

	if err := workload.Validate(&collectionObj); err != nil {
		return nil, fmt.Errorf("error validating collection yaml, %w", err)
	}

	return Generate(workloadObj, collectionObj, nil, nil)
}

// CreateFuncs is an array of functions that are called to create the child resources for the controller
// in memory during the reconciliation loop prior to persisting the changes or updates to the Kubernetes
// database.
var CreateFuncs = []func(
	*nodesv1alpha1.PocketValidator,
	*nodesv1alpha1.PocketSet,
	workload.Reconciler,
	*workload.Request,
) ([]client.Object, error){
	CreateStatefulSetCollectionNameParentName,
	CreateServiceCollectionNameParentName,
	CreateConfigMapCollectionNameParentNameConfig,
}

// InitFuncs is an array of functions that are called prior to starting the controller manager.  This is
// necessary in instances which the controller needs to "own" objects which depend on resources to
// pre-exist in the cluster. A common use case for this is the need to own a custom resource.
// If the controller needs to own a custom resource type, the CRD that defines it must
// first exist. In this case, the InitFunc will create the CRD so that the controller
// can own custom resources of that type.  Without the InitFunc the controller will
// crash loop because when it tries to own a non-existent resource type during manager
// setup, it will fail.
var InitFuncs = []func(
	*nodesv1alpha1.PocketValidator,
	*nodesv1alpha1.PocketSet,
	workload.Reconciler,
	*workload.Request,
) ([]client.Object, error){}

func ConvertWorkload(component, collection workload.Workload) (
	*nodesv1alpha1.PocketValidator,
	*nodesv1alpha1.PocketSet,
	error,
) {
	p, ok := component.(*nodesv1alpha1.PocketValidator)
	if !ok {
		return nil, nil, nodesv1alpha1.ErrUnableToConvertPocketValidator
	}

	c, ok := collection.(*nodesv1alpha1.PocketSet)
	if !ok {
		return nil, nil, nodesv1alpha1.ErrUnableToConvertPocketSet
	}

	return p, c, nil
}
