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

package binding

import (
	"context"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/go-logr/logr"
	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/controllers/results"
	"github.com/redhat-appstudio/integration-service/gitops"
	"github.com/redhat-appstudio/integration-service/helpers"
	"github.com/redhat-appstudio/integration-service/tekton"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Adapter holds the objects needed to reconcile a Release.
type Adapter struct {
	snapshotEnvironmentBinding *applicationapiv1alpha1.SnapshotEnvironmentBinding
	snapshot                   *applicationapiv1alpha1.Snapshot
	application                *applicationapiv1alpha1.Application
	environment                *applicationapiv1alpha1.Environment
	integrationTestScenario    *v1alpha1.IntegrationTestScenario
	logger                     logr.Logger
	client                     client.Client
	context                    context.Context
}

// NewAdapter creates and returns an Adapter instance.
func NewAdapter(snapshotEnvironmentBinding *applicationapiv1alpha1.SnapshotEnvironmentBinding, snapshot *applicationapiv1alpha1.Snapshot, environment *applicationapiv1alpha1.Environment, application *applicationapiv1alpha1.Application, integrationTestScenario *v1alpha1.IntegrationTestScenario, logger logr.Logger, client client.Client,
	context context.Context) *Adapter {
	return &Adapter{
		snapshotEnvironmentBinding: snapshotEnvironmentBinding,
		snapshot:                   snapshot,
		environment:                environment,
		application:                application,
		integrationTestScenario:    integrationTestScenario,
		logger:                     logger,
		client:                     client,
		context:                    context,
	}
}

// EnsureIntegrationTestPipelineForScenarioExists is an operation that will ensure that the Integration test pipeline
// associated with the Snapshot and the SnapshotEnvironmentBinding's IntegrationTestScenarios exist.
func (a *Adapter) EnsureIntegrationTestPipelineForScenarioExists() (results.OperationResult, error) {
	if gitops.HaveHACBSTestsFinished(a.snapshot) {
		a.logger.Info("The Snapshot has finished testing.")
		return results.ContinueProcessing()
	}

	if a.integrationTestScenario != nil {
		integrationPipelineRun, err := helpers.GetLatestPipelineRunForSnapshotAndScenario(a.client, a.context, a.application, a.snapshot, a.integrationTestScenario)
		if err != nil {
			a.logger.Error(err, "Failed to get latest pipelineRun for snapshot and scenario",
				"integrationPipelineRun:", integrationPipelineRun)
			return results.RequeueOnErrorOrStop(err)
		}
		if integrationPipelineRun != nil {
			a.logger.Info("Found existing integrationPipelineRun",
				"IntegrationTestScenario.Name", a.integrationTestScenario.Name,
				"integrationPipelineRun.Name", integrationPipelineRun.Name)
		} else {
			a.logger.Info("Creating new pipelinerun for integrationTestscenario",
				"IntegrationTestScenario.Name", a.integrationTestScenario.Name,
				"app name", a.application.Name,
				"namespace", a.application.Namespace)
			err := a.createIntegrationPipelineRunWithEnvironment(a.application, a.integrationTestScenario, a.snapshot, a.environment)
			if err != nil {
				a.logger.Error(err, "Failed to create pipelineRun for snapshot, environment and scenario")
				return results.RequeueOnErrorOrStop(err)
			}
		}
	}

	return results.ContinueProcessing()
}

// createIntegrationPipelineRunWithEnvironment creates and returns a new integration PipelineRun. The Pipeline information and the parameters to it
// will be extracted from the given integrationScenario. The integration's Snapshot will also be passed to the integration PipelineRun.
func (a *Adapter) createIntegrationPipelineRunWithEnvironment(application *applicationapiv1alpha1.Application, integrationTestScenario *v1alpha1.IntegrationTestScenario, snapshot *applicationapiv1alpha1.Snapshot, environment *applicationapiv1alpha1.Environment) error {
	pipelineRun := tekton.NewIntegrationPipelineRun(snapshot.Name, application.Namespace, *integrationTestScenario).
		WithSnapshot(snapshot).
		WithIntegrationLabels(integrationTestScenario).
		WithEnvironment(environment).
		AsPipelineRun()
	// copy PipelineRun PAC annotations/labels from snapshot to integration test PipelineRuns
	helpers.CopyAnnotationsByPrefix(&snapshot.ObjectMeta, &pipelineRun.ObjectMeta, gitops.PipelinesAsCodePrefix, gitops.PipelinesAsCodePrefix)
	helpers.CopyLabelsByPrefix(&snapshot.ObjectMeta, &pipelineRun.ObjectMeta, gitops.PipelinesAsCodePrefix, gitops.PipelinesAsCodePrefix)
	err := ctrl.SetControllerReference(snapshot, pipelineRun, a.client.Scheme())
	if err != nil {
		return err
	}
	err = a.client.Create(a.context, pipelineRun)
	if err != nil {
		return err
	}

	return nil

}
