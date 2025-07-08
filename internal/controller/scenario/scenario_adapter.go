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

	"github.com/konflux-ci/integration-service/api/v1beta2"
	h "github.com/konflux-ci/integration-service/helpers"
	"github.com/konflux-ci/integration-service/loader"
	"github.com/konflux-ci/operator-toolkit/controller"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Adapter holds the objects needed to reconcile a Release.
type Adapter struct {
	scenario *v1beta2.IntegrationTestScenario
	logger   h.IntegrationLogger
	loader   loader.ObjectLoader
	client   client.Client
	context  context.Context
}

// NewAdapter creates and returns an Adapter instance.
func NewAdapter(context context.Context, scenario *v1beta2.IntegrationTestScenario, logger h.IntegrationLogger, loader loader.ObjectLoader, client client.Client,
) *Adapter {
	return &Adapter{
		scenario: scenario,
		logger:   logger,
		loader:   loader,
		client:   client,
		context:  context,
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
