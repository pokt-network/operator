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

package pocketset

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/nukleros/operator-builder-tools/pkg/controller/workload"

	nodesv1alpha1 "github.com/pokt-network/pocket-operator/apis/nodes/v1alpha1"
	"github.com/pokt-network/pocket-operator/apis/nodes/v1alpha1/pocketset/mutate"
)

// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete

// CreateServiceParentNameParentNameValidators creates the Service resource with name parent.name + -validators.
func CreateServiceParentNameParentNameValidators(
	parent *nodesv1alpha1.PocketSet,
	reconciler workload.Reconciler,
	req *workload.Request,
) ([]client.Object, error) {

	var resourceObj = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata": map[string]interface{}{
				"name":      "" + parent.Name + "-validators", //  controlled by field:
				"namespace": parent.Name,                      //  controlled by field:
			},
			"spec": map[string]interface{}{
				"ports": []interface{}{
					map[string]interface{}{
						"port": 8221,
						"name": "pre2p",
					},
					map[string]interface{}{
						"port": 8222,
						"name": "p2p",
					},
				},
				"clusterIP": "None",
				"selector": map[string]interface{}{
					"v1-purpose": "validator",
				},
			},
		},
	}

	return mutate.MutateServiceParentNameParentNameValidators(resourceObj, parent, reconciler, req)
}
