/*
Copyright 2023 Red Hat Inc.

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

	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/api/v1beta2"
	h "github.com/redhat-appstudio/integration-service/helpers"
	"github.com/redhat-appstudio/integration-service/loader"
	"github.com/redhat-appstudio/operator-toolkit/controller"
	"k8s.io/apimachinery/pkg/api/meta"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Adapter holds the objects needed to reconcile a Release.
type Adapter struct {
	application *applicationapiv1alpha1.Application
	scenario    *v1beta2.IntegrationTestScenario
	logger      h.IntegrationLogger
	loader      loader.ObjectLoader
	client      client.Client
	context     context.Context
}

// NewAdapter creates and returns an Adapter instance.
func NewAdapter(application *applicationapiv1alpha1.Application, scenario *v1beta2.IntegrationTestScenario, logger h.IntegrationLogger, loader loader.ObjectLoader, client client.Client,
	context context.Context) *Adapter {
	return &Adapter{
		application: application,
		scenario:    scenario,
		logger:      logger,
		loader:      loader,
		client:      client,
		context:     context,
	}
}

// EnsureCreatedScenarioIsValid is an operation that ensures created IntegrationTestScenario is valid
// in case it is, set its owner reference
func (a *Adapter) EnsureCreatedScenarioIsValid() (controller.OperationResult, error) {
	if a.scenario.DeletionTimestamp != nil {
		return controller.ContinueProcessing()
	}

	// Check if application exists or not
	if a.application == nil {
		a.logger.Info("Application for scenario was not found.")

		patch := client.MergeFrom(a.scenario.DeepCopy())
		h.SetScenarioIntegrationStatusAsInvalid(a.scenario, "Failed to get application for scenario.")
		err := a.client.Status().Patch(a.context, a.scenario, patch)
		if err != nil {
			a.logger.Error(err, "Failed to update Scenario")
			return controller.RequeueWithError(err)
		}
		return controller.ContinueProcessing()
	}

	// application exist, always log it
	a.logger = a.logger.WithApp(*a.application)
	// Checks if scenario has ownerReference assigned to it
	if a.scenario.OwnerReferences == nil {
		patch := client.MergeFrom(a.scenario.DeepCopy())
		err := ctrl.SetControllerReference(a.application, a.scenario, a.client.Scheme())
		if err != nil {
			a.logger.Error(err, "Error setting owner reference.")
			return controller.RequeueWithError(err)
		}
		err = a.client.Patch(a.context, a.scenario, patch)
		if err != nil {
			a.logger.Error(err, "Failed to update Scenario")
			return controller.RequeueWithError(err)
		}
	}

	// If the scenario status IntegrationTestScenarioValid condition is not defined or false, we set it to Valid
	if reflect.ValueOf(a.scenario.Status).IsZero() || !meta.IsStatusConditionTrue(a.scenario.Status.Conditions, h.IntegrationTestScenarioValid) {
		patch := client.MergeFrom(a.scenario.DeepCopy())
		h.SetScenarioIntegrationStatusAsValid(a.scenario, "Integration test scenario is Valid.")
		err := a.client.Status().Patch(a.context, a.scenario, patch)
		if err != nil {
			a.logger.Error(err, "Failed to update Scenario")
			return controller.RequeueWithError(err)
		}
		a.logger.LogAuditEvent("IntegrationTestScenario marked as Valid", a.scenario, h.LogActionUpdate)
	}

	err := h.AddFinalizerToScenario(a.client, a.logger, a.context, a.scenario, h.IntegrationTestScenarioFinalizer)
	if err != nil {
		a.logger.Error(err, "Failed to add finalizer to IntegrationTestScenario")
		return controller.RequeueWithError(err)
	}

	return controller.ContinueProcessing()
}

// EnsureDeletedScenarioResourcesAreCleanedUp is an operation that ensures that all resources related to the
// deleted IntegrationTestScenario are cleaned up.
func (a *Adapter) EnsureDeletedScenarioResourcesAreCleanedUp() (controller.OperationResult, error) {
	if a.scenario.DeletionTimestamp == nil {
		return controller.ContinueProcessing()
	}

	environmentsForScenario, err := a.loader.GetAllEnvironmentsForScenario(a.client, a.context, a.scenario)
	if err != nil {
		a.logger.Error(err, "Failed to find all Environments for IntegrationTestScenario")
		return controller.RequeueWithError(err)
	}
	for _, testEnvironment := range *environmentsForScenario {
		testEnvironment := testEnvironment
		if h.IsEnvironmentEphemeral(&testEnvironment) {
			dtc, err := a.loader.GetDeploymentTargetClaimForEnvironment(a.client, a.context, &testEnvironment)
			if err != nil || dtc == nil {
				a.logger.Error(err, "Failed to find deploymentTargetClaim defined in environment", "environment.Name", &testEnvironment.Name)
				return controller.RequeueWithError(err)
			}

			err = h.CleanUpEphemeralEnvironments(a.client, &a.logger, a.context, &testEnvironment, dtc)
			if err != nil {
				a.logger.Error(err, "Failed to delete the Ephemeral Environment")
				return controller.RequeueWithError(err)
			}
		}
	}
	// Remove the finalizer from the scenario since the cleanup has been handled
	err = h.RemoveFinalizerFromScenario(a.client, a.logger, a.context, a.scenario, h.IntegrationTestScenarioFinalizer)
	if err != nil {
		a.logger.Error(err, "Failed to remove the finalizer from the Scenario")
		return controller.RequeueWithError(err)
	}
	return controller.ContinueProcessing()
}
