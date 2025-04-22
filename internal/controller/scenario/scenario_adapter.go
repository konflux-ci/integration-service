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
	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	"github.com/konflux-ci/integration-service/api/v1beta2"
	h "github.com/konflux-ci/integration-service/helpers"
	"github.com/konflux-ci/integration-service/loader"
	"github.com/konflux-ci/operator-toolkit/controller"
	"k8s.io/apimachinery/pkg/api/meta"
	"reflect"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
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
func NewAdapter(context context.Context, application *applicationapiv1alpha1.Application, scenario *v1beta2.IntegrationTestScenario, logger h.IntegrationLogger, loader loader.ObjectLoader, client client.Client,
) *Adapter {
	return &Adapter{
		application: application,
		scenario:    scenario,
		logger:      logger,
		loader:      loader,
		client:      client,
		context:     context,
	}
}

// TODO: Remove after a couple weeks. This is just to add new field to existing scenarios
// Adds ResourceKind field to existing IntegrationTestScenarios
func (a *Adapter) EnsureScenarioContainsResourceKind() (controller.OperationResult, error) {
	a.logger.Info("Adding ResourceKind to ITS if it does not exist", "scenario", a.scenario)
	if a.scenario.Spec.ResolverRef.ResourceKind == "" {
		// set ResourceKind to 'pipeline'
		patch := client.MergeFrom(a.scenario.DeepCopy())
		a.scenario.Spec.ResolverRef.ResourceKind = "pipeline"
		err := a.client.Patch(a.context, a.scenario, patch)
		if err != nil {
			a.logger.Error(err, "Failed to add ResourceKind to Scenario")
			return controller.RequeueWithError(err)
		}
	}

	return controller.ContinueProcessing()
}

// EnsureCreatedScenarioIsValid is an operation that ensures created IntegrationTestScenario is valid
// in case it is, set its owner reference
func (a *Adapter) EnsureCreatedScenarioIsValid() (controller.OperationResult, error) {
	// remove finalizer artifacts, see STONEINTG-815
	if controllerutil.ContainsFinalizer(a.scenario, h.IntegrationTestScenarioFinalizer) {
		err := h.RemoveFinalizerFromScenario(a.context, a.client, a.logger, a.scenario, h.IntegrationTestScenarioFinalizer)
		if err != nil {
			a.logger.Error(err, "Failed to remove finalizer from the Scenario")
			return controller.RequeueWithError(err)
		}
	}

	if a.scenario.DeletionTimestamp != nil {
		return controller.ContinueProcessing()
	}

	// Checks if scenario has ownerReference assigned to it
	if a.scenario.OwnerReferences == nil {
		patch := client.MergeFrom(a.scenario.DeepCopy())
		err := ctrl.SetControllerReference(a.application, a.scenario, a.client.Scheme())
		if err != nil {
			a.logger.Error(err, "Error setting owner reference of Scenario.")
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
		h.SetScenarioIntegrationStatusAsValid(a.scenario, "IntegrationTestScenario is Valid.")
		err := a.client.Status().Patch(a.context, a.scenario, patch)
		if err != nil {
			a.logger.Error(err, "Failed to update Scenario")
			return controller.RequeueWithError(err)
		}
		a.logger.LogAuditEvent("IntegrationTestScenario marked as Valid", a.scenario, h.LogActionUpdate)
	}

	return controller.ContinueProcessing()
}
