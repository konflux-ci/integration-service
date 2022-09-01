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
	hasv1alpha1 "github.com/redhat-appstudio/application-service/api/v1alpha1"
	appstudioshared "github.com/redhat-appstudio/managed-gitops/appstudio-shared/apis/appstudio.redhat.com/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"math"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CreateApplicationSnapshotEnvironmentBinding creates a new ApplicationSnapshotEnvironmentBinding using the provided info.
func CreateApplicationSnapshotEnvironmentBinding(bindingName string, namespace string, applicationName string, environmentName string, appSnapshot *appstudioshared.ApplicationSnapshot, components []hasv1alpha1.Component) *appstudioshared.ApplicationSnapshotEnvironmentBinding {
	bindingComponents := CreateBindingComponents(components)

	applicationSnapshotEnvironmentBinding := &appstudioshared.ApplicationSnapshotEnvironmentBinding{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: bindingName + "-",
			Namespace:    namespace,
		},
		Spec: appstudioshared.ApplicationSnapshotEnvironmentBindingSpec{
			Application: applicationName,
			Environment: environmentName,
			Snapshot:    appSnapshot.Name,
			Components:  *bindingComponents,
		},
	}

	return applicationSnapshotEnvironmentBinding
}

// CreateBindingComponents gets all components from the ApplicationSnapshot and formats them to be used in the
// ApplicationSnapshotEnvironmentBinding as BindingComponents.
func CreateBindingComponents(components []hasv1alpha1.Component) *[]appstudioshared.BindingComponent {
	bindingComponents := []appstudioshared.BindingComponent{}
	for _, component := range components {
		bindingComponents = append(bindingComponents, appstudioshared.BindingComponent{
			Name: component.Spec.ComponentName,
			Configuration: appstudioshared.BindingComponentConfiguration{
				Replicas: int(math.Max(1, float64(component.Spec.Replicas))),
			},
		})
	}
	return &bindingComponents
}

// FindExistingApplicationSnapshotEnvironmentBinding attempts to find an ApplicationSnapshotEnvironmentBinding that's
// associated with the provided environment.
func FindExistingApplicationSnapshotEnvironmentBinding(adapterClient client.Client, ctx context.Context, application *hasv1alpha1.Application, environment *appstudioshared.Environment) (*appstudioshared.ApplicationSnapshotEnvironmentBinding, error) {
	applicationSnapshotEnvironmentBindingList := &appstudioshared.ApplicationSnapshotEnvironmentBindingList{}
	opts := []client.ListOption{
		client.InNamespace(application.Namespace),
		client.MatchingFields{"spec.environment": environment.Name},
	}

	err := adapterClient.List(ctx, applicationSnapshotEnvironmentBindingList, opts...)
	if err != nil {
		return nil, err
	}

	for _, binding := range applicationSnapshotEnvironmentBindingList.Items {
		if binding.Spec.Application == application.Name {
			return &binding, nil
		}
	}

	return nil, nil
}
