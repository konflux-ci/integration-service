/*
Copyright 2024 Red Hat Inc.

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

package helpers

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/redhat-appstudio/integration-service/api/v1beta1"
)

const (

	// IntegrationTestScenarioValid is the condition for marking the AppStudio integration status of the Scenario.
	IntegrationTestScenarioValid = "IntegrationTestScenarioValid"

	// AppStudioIntegrationStatusInvalid is the reason that's set when the AppStudio integration gets into an invalid state.
	AppStudioIntegrationStatusInvalid = "Invalid"

	// AppStudioIntegrationStatusValid is the reason that's set when the AppStudio integration gets into an valid state.
	AppStudioIntegrationStatusValid = "Valid"
)

// SetScenarioIntegrationStatusAsInvalid sets the IntegrationTestScenarioValid status condition for the Scenario to invalid.
func SetScenarioIntegrationStatusAsInvalid(scenario *v1beta1.IntegrationTestScenario, message string) {
	meta.SetStatusCondition(&scenario.Status.Conditions, metav1.Condition{
		Type:    IntegrationTestScenarioValid,
		Status:  metav1.ConditionFalse,
		Reason:  AppStudioIntegrationStatusInvalid,
		Message: message,
	})
}

// SetScenarioIntegrationStatusAsValid sets the IntegrationTestScenarioValid integration status condition for the Scenario to valid.
func SetScenarioIntegrationStatusAsValid(scenario *v1beta1.IntegrationTestScenario, message string) {
	meta.SetStatusCondition(&scenario.Status.Conditions, metav1.Condition{
		Type:    IntegrationTestScenarioValid,
		Status:  metav1.ConditionTrue,
		Reason:  AppStudioIntegrationStatusValid,
		Message: message,
	})
}

// IsScenarioValid sets the IntegrationTestScenarioValid integration status condition for the Scenario to valid.
func IsScenarioValid(scenario *v1beta1.IntegrationTestScenario) bool {
	statusCondition := meta.FindStatusCondition(scenario.Status.Conditions, IntegrationTestScenarioValid)
	return statusCondition.Status != metav1.ConditionFalse
}
