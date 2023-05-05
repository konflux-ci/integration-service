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

package scenario

import (
	"context"
	"reflect"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/gitops"
	h "github.com/redhat-appstudio/integration-service/helpers"
	"github.com/redhat-appstudio/operator-goodies/reconciler"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Adapter holds the objects needed to reconcile a Release.
type Adapter struct {
	application *applicationapiv1alpha1.Application
	scenario    *v1alpha1.IntegrationTestScenario
	logger      h.IntegrationLogger
	client      client.Client
	context     context.Context
}

// NewAdapter creates and returns an Adapter instance.
func NewAdapter(application *applicationapiv1alpha1.Application, scenario *v1alpha1.IntegrationTestScenario, logger h.IntegrationLogger, client client.Client,
	context context.Context) *Adapter {
	return &Adapter{
		application: application,
		scenario:    scenario,
		logger:      logger,
		client:      client,
		context:     context,
	}
}

// EnsureCreatedScenarioIsValid is an operation that ensures created IntegrationTestScenario is valid
// in case it is, set its owner reference
func (a *Adapter) EnsureCreatedScenarioIsValid() (reconciler.OperationResult, error) {

	// First check if application exists or not
	if a.application == nil {
		a.logger.Info("Application for scenario was not found.")

		patch := client.MergeFrom(a.scenario.DeepCopy())
		SetScenarioIntegrationStatusAsInvalid(a.scenario, "Failed to get application for scenario.")
		err := a.client.Status().Patch(a.context, a.scenario, patch)
		if err != nil {
			a.logger.Error(err, "Failed to update Scenario")
			return reconciler.RequeueWithError(err)
		}
		return reconciler.ContinueProcessing()
	}

	// application exist, always log it
	a.logger = a.logger.WithApp(*a.application)
	// Checks if scenario has ownerReference assigned to it
	if a.scenario.OwnerReferences == nil {
		patch := client.MergeFrom(a.scenario.DeepCopy())
		err := ctrl.SetControllerReference(a.application, a.scenario, a.client.Scheme())
		if err != nil {
			a.logger.Error(err, "Error setting owner reference.")
			return reconciler.RequeueWithError(err)
		}
		err = a.client.Patch(a.context, a.scenario, patch)
		if err != nil {
			a.logger.Error(err, "Failed to update Scenario")
			return reconciler.RequeueWithError(err)
		}

	}
	// Checks if scenario has environment defined
	if reflect.ValueOf(a.scenario.Spec.Environment).IsZero() {
		a.logger.Info("IntegrationTestScenario has no environment defined")
	} else {
		//Same as function getEnvironmentFromIntegrationTestScenario - this could be later changed to call only that function
		ITSEnv := &applicationapiv1alpha1.Environment{}

		err := a.client.Get(a.context, types.NamespacedName{
			Namespace: a.application.Namespace,
			Name:      a.scenario.Spec.Environment.Name,
		}, ITSEnv)

		if err != nil {
			a.logger.Info("Environment doesn't exist in same namespace as IntegrationTestScenario.",
				"environment.Name:", a.scenario.Spec.Environment.Name)
			patch := client.MergeFrom(a.scenario.DeepCopy())
			SetScenarioIntegrationStatusAsInvalid(a.scenario, "Environment "+a.scenario.Spec.Environment.Name+" is located in different namespace than scenario.")
			err = a.client.Status().Patch(a.context, a.scenario, patch)
			if err != nil {
				a.logger.Error(err, "Failed to update Scenario")
				return reconciler.RequeueWithError(err)
			}
			a.logger.LogAuditEvent("IntegrationTestScenario marked as Invalid. Environment "+a.scenario.Spec.Environment.Name+" is located in different namespace than scenario. ",
				a.scenario, h.LogActionUpdate)
			return reconciler.ContinueProcessing()
		}

	}

	if reflect.ValueOf(a.scenario.Status).IsZero() || (meta.IsStatusConditionFalse(a.scenario.Status.Conditions, gitops.IntegrationTestScenarioValid)) {
		patch := client.MergeFrom(a.scenario.DeepCopy())
		SetScenarioIntegrationStatusAsValid(a.scenario, "Integration test scenario is Valid.")
		err := a.client.Status().Patch(a.context, a.scenario, patch)
		if err != nil {
			a.logger.Error(err, "Failed to update Scenario")
			return reconciler.ContinueProcessing()
		}
		a.logger.LogAuditEvent("IntegrationTestScenario marked as Valid", a.scenario, h.LogActionUpdate)
	}

	return reconciler.ContinueProcessing()
}

// SetSnapshotIntegrationStatusAsInvalid sets the AppStudio integration status condition for the Snapshot to invalid.
func SetScenarioIntegrationStatusAsInvalid(scenario *v1alpha1.IntegrationTestScenario, message string) {
	meta.SetStatusCondition(&scenario.Status.Conditions, metav1.Condition{
		Type:    gitops.IntegrationTestScenarioValid,
		Status:  metav1.ConditionFalse,
		Reason:  gitops.AppStudioIntegrationStatusInvalid,
		Message: message,
	})
}

// SetSnapshotIntegrationStatusAsValid sets the AppStudio integration status condition for the Snapshot to valid.
func SetScenarioIntegrationStatusAsValid(scenario *v1alpha1.IntegrationTestScenario, message string) {
	meta.SetStatusCondition(&scenario.Status.Conditions, metav1.Condition{
		Type:    gitops.IntegrationTestScenarioValid,
		Status:  metav1.ConditionTrue,
		Reason:  gitops.AppStudioIntegrationStatusValid,
		Message: message,
	})
}
