/*
Copyright 2022.

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

package gitops

import (
	"context"
	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"math"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CreateSnapshotEnvironmentBinding creates a new SnapshotEnvironmentBinding using the provided info.
func CreateSnapshotEnvironmentBinding(bindingName string, namespace string, applicationName string, environmentName string, snapshot *applicationapiv1alpha1.Snapshot, components []applicationapiv1alpha1.Component) *applicationapiv1alpha1.SnapshotEnvironmentBinding {
	bindingComponents := CreateBindingComponents(components)

	snapshotEnvironmentBinding := &applicationapiv1alpha1.SnapshotEnvironmentBinding{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: bindingName + "-",
			Namespace:    namespace,
		},
		Spec: applicationapiv1alpha1.SnapshotEnvironmentBindingSpec{
			Application: applicationName,
			Environment: environmentName,
			Snapshot:    snapshot.Name,
			Components:  *bindingComponents,
		},
	}

	return snapshotEnvironmentBinding
}

// CreateBindingComponents gets all components from the Snapshot and formats them to be used in the
// SnapshotEnvironmentBinding as BindingComponents.
func CreateBindingComponents(components []applicationapiv1alpha1.Component) *[]applicationapiv1alpha1.BindingComponent {
	bindingComponents := []applicationapiv1alpha1.BindingComponent{}
	for _, component := range components {
		bindingComponents = append(bindingComponents, applicationapiv1alpha1.BindingComponent{
			Name: component.Spec.ComponentName,
			Configuration: applicationapiv1alpha1.BindingComponentConfiguration{
				Replicas: int(math.Max(1, float64(component.Spec.Replicas))),
			},
		})
	}
	return &bindingComponents
}

// FindExistingSnapshotEnvironmentBinding attempts to find a SnapshotEnvironmentBinding that's
// associated with the provided environment.
func FindExistingSnapshotEnvironmentBinding(adapterClient client.Client, ctx context.Context, application *applicationapiv1alpha1.Application, environment *applicationapiv1alpha1.Environment) (*applicationapiv1alpha1.SnapshotEnvironmentBinding, error) {
	snapshotEnvironmentBindingList := &applicationapiv1alpha1.SnapshotEnvironmentBindingList{}
	opts := []client.ListOption{
		client.InNamespace(application.Namespace),
		client.MatchingFields{"spec.environment": environment.Name},
	}

	err := adapterClient.List(ctx, snapshotEnvironmentBindingList, opts...)
	if err != nil {
		return nil, err
	}

	for _, binding := range snapshotEnvironmentBindingList.Items {
		if binding.Spec.Application == application.Name {
			return &binding, nil
		}
	}

	return nil, nil
}
