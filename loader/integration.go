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

package loader

import (
	"context"

	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/api/v1beta1"
	"github.com/redhat-appstudio/integration-service/helpers"
	tektonv1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	"knative.dev/pkg/apis"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GetLatestPipelineRunForSnapshotAndScenario returns the latest Integration PipelineRun for the
// associated Snapshot and IntegrationTestScenario. In the case the List operation fails,
// an error will be returned.
func GetLatestPipelineRunForSnapshotAndScenario(adapterClient client.Client, ctx context.Context, loader ObjectLoader, snapshot *applicationapiv1alpha1.Snapshot, integrationTestScenario *v1beta1.IntegrationTestScenario) (*tektonv1beta1.PipelineRun, error) {
	integrationPipelineRuns, err := loader.GetAllPipelineRunsForSnapshotAndScenario(adapterClient, ctx, snapshot, integrationTestScenario)
	if err != nil {
		return nil, err
	}

	var latestIntegrationPipelineRun = &tektonv1beta1.PipelineRun{}
	latestIntegrationPipelineRun = nil
	for _, pipelineRun := range *integrationPipelineRuns {
		pipelineRun := pipelineRun // G601
		if !pipelineRun.Status.GetCondition(apis.ConditionSucceeded).IsUnknown() {
			if latestIntegrationPipelineRun == nil {
				latestIntegrationPipelineRun = &pipelineRun
			} else {
				if pipelineRun.Status.CompletionTime.Time.After(latestIntegrationPipelineRun.Status.CompletionTime.Time) {
					latestIntegrationPipelineRun = &pipelineRun
				}
			}
		}
	}
	if latestIntegrationPipelineRun != nil {
		return latestIntegrationPipelineRun, nil
	}

	return nil, err
}

// RemoveFinalizerFromAllIntegrationPipelineRunsOfSnapshot fetches all the Integration
// PipelineRuns associated with the given Snapshot and each of the IntegrationTestScenarios.
// After fetching them, it removes the finalizer from the PipelineRun, and returns error if any.
func RemoveFinalizerFromAllIntegrationPipelineRunsOfSnapshot(adapterClient client.Client, logger helpers.IntegrationLogger, ctx context.Context, snapshot applicationapiv1alpha1.Snapshot, finalizer string) error {
	integrationPipelineRuns := &tektonv1beta1.PipelineRunList{}
	opts := []client.ListOption{
		client.InNamespace(snapshot.Namespace),
		client.MatchingLabels{
			"appstudio.openshift.io/snapshot": snapshot.Name,
		},
	}

	err := adapterClient.List(ctx, integrationPipelineRuns, opts...)
	if err != nil {
		return err
	}

	// Remove finalizer from each of the PipelineRuns
	for _, pipelineRun := range integrationPipelineRuns.Items {
		err = helpers.RemoveFinalizerFromPipelineRun(adapterClient, logger, ctx, &pipelineRun, finalizer)
		if err != nil {
			return err
		}
	}

	return nil
}
