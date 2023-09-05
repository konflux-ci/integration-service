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
	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// BindingDeploymentStatusConditionType is the condition type to retrieve from the ComponentDeploymentConditions
	// in the SnapshotEnvironmentBinding's status to copy into the Release status
	BindingDeploymentStatusConditionType string = "AllComponentsDeployed"

	// BindingErrorOccurredStatusConditionType is the condition to check for failures within the
	// SnapshotEnvironmentBindingConditions status
	BindingErrorOccurredStatusConditionType string = "ErrorOccurred"
)

// NewSnapshotEnvironmentBinding creates a new SnapshotEnvironmentBinding using the provided info.
func NewSnapshotEnvironmentBinding(bindingName string, namespace string, applicationName string, environmentName string, snapshot *applicationapiv1alpha1.Snapshot, components []applicationapiv1alpha1.Component) *applicationapiv1alpha1.SnapshotEnvironmentBinding {
	bindingComponents := NewBindingComponents(components)

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

// NewBindingComponents gets all components from the Snapshot and formats them to be used in the
// SnapshotEnvironmentBinding as BindingComponents.
func NewBindingComponents(components []applicationapiv1alpha1.Component) *[]applicationapiv1alpha1.BindingComponent {
	bindingComponents := []applicationapiv1alpha1.BindingComponent{}
	for _, component := range components {
		bindingComponents = append(bindingComponents, applicationapiv1alpha1.BindingComponent{
			Name: component.Spec.ComponentName,
		})
	}
	return &bindingComponents
}

func GetBindingConditionStatus(snapshotEnvironmentBinding *applicationapiv1alpha1.SnapshotEnvironmentBinding) *metav1.Condition {
	bindingStatus := meta.FindStatusCondition(snapshotEnvironmentBinding.Status.BindingConditions, BindingErrorOccurredStatusConditionType)
	return bindingStatus
}

func HaveBindingsFailed(snapshotEnvironmentBinding *applicationapiv1alpha1.SnapshotEnvironmentBinding) bool {
	bindingStatus := GetBindingConditionStatus(snapshotEnvironmentBinding)
	if bindingStatus == nil {
		return false
	}
	return bindingStatus.Status == metav1.ConditionTrue
}

func IsBindingDeployed(snapshotEnvironmentBinding *applicationapiv1alpha1.SnapshotEnvironmentBinding) bool {
	bindingStatus := meta.FindStatusCondition(snapshotEnvironmentBinding.Status.ComponentDeploymentConditions, BindingDeploymentStatusConditionType)
	if bindingStatus == nil {
		return false
	}
	return bindingStatus.Status == metav1.ConditionTrue
}

// hasDeploymentSucceeded returns a boolean that is only true if the first passed object
// is a SnapshotEnvironmentBinding with the componentDeployment status anything other than True and
// the second passed object is a SnapshotEnvironmentBinding with the componentDeployment status True.
func hasDeploymentSucceeded(objectOld, objectNew client.Object) bool {
	var oldCondition, newCondition *metav1.Condition

	if oldBinding, ok := objectOld.(*applicationapiv1alpha1.SnapshotEnvironmentBinding); ok {
		oldCondition = meta.FindStatusCondition(oldBinding.Status.ComponentDeploymentConditions, BindingDeploymentStatusConditionType)
	}
	if newBinding, ok := objectNew.(*applicationapiv1alpha1.SnapshotEnvironmentBinding); ok {
		newCondition = meta.FindStatusCondition(newBinding.Status.ComponentDeploymentConditions, BindingDeploymentStatusConditionType)
		if newCondition == nil {
			return false
		}
	}

	return (oldCondition == nil || oldCondition.Status != metav1.ConditionTrue) && newCondition.Status == metav1.ConditionTrue
}

// hasDeploymentFailed returns a boolean that is only true if the first passed object
// is a SnapshotEnvironmentBinding with the BindingConditions status anything other than True and
// the second passed object is a SnapshotEnvironmentBinding with the BindingErrorOccurred status True.
func hasDeploymentFailed(objectOld, objectNew client.Object) bool {
	var oldCondition, newCondition *metav1.Condition
	if oldBinding, ok := objectOld.(*applicationapiv1alpha1.SnapshotEnvironmentBinding); ok {
		oldCondition = meta.FindStatusCondition(oldBinding.Status.BindingConditions, BindingErrorOccurredStatusConditionType)
	}
	if newBinding, ok := objectNew.(*applicationapiv1alpha1.SnapshotEnvironmentBinding); ok {
		newCondition = meta.FindStatusCondition(newBinding.Status.BindingConditions, BindingErrorOccurredStatusConditionType)
		if newCondition == nil {
			return false
		}
	}
	return (oldCondition == nil || oldCondition.Status != metav1.ConditionTrue) && newCondition.Status == metav1.ConditionTrue
}
