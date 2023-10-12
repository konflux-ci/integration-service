/*
Copyright 2023.

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

package statusreport

import (
	"context"
	"fmt"
	"github.com/redhat-appstudio/integration-service/api/v1beta1"
	"github.com/redhat-appstudio/integration-service/gitops"
	"github.com/redhat-appstudio/integration-service/metrics"
	"github.com/redhat-appstudio/operator-toolkit/metadata"
	"os"

	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/helpers"
	"github.com/redhat-appstudio/integration-service/status"

	"github.com/redhat-appstudio/integration-service/loader"
	"github.com/redhat-appstudio/operator-toolkit/controller"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const FeatureFlagStatusReprotingEnabled = "FEATURE_STATUS_REPORTING_ENABLED"

// Adapter holds the objects needed to reconcile a snapshot's test status report.
type Adapter struct {
	snapshot    *applicationapiv1alpha1.Snapshot
	application *applicationapiv1alpha1.Application
	logger      helpers.IntegrationLogger
	loader      loader.ObjectLoader
	client      client.Client
	context     context.Context
	status      status.Status
}

// NewAdapter creates and returns an Adapter instance.
func NewAdapter(snapshot *applicationapiv1alpha1.Snapshot, application *applicationapiv1alpha1.Application, logger helpers.IntegrationLogger, loader loader.ObjectLoader, client client.Client,
	context context.Context) *Adapter {
	return &Adapter{
		snapshot:    snapshot,
		application: application,
		logger:      logger,
		loader:      loader,
		client:      client,
		context:     context,
		status:      status.NewAdapter(logger.Logger, client),
	}
}

// EnsureSnapshotTestStatusReported will ensure that integration test status including env provision and snapshotEnvironmentBinding error is reported to the git provider
// which (indirectly) triggered its execution.
func (a *Adapter) EnsureSnapshotTestStatusReported() (controller.OperationResult, error) {
	if !isFeatureEnabled() || !metadata.HasLabelWithValue(a.snapshot, gitops.PipelineAsCodeEventTypeLabel, gitops.PipelineAsCodePullRequestType) {
		return controller.ContinueProcessing()
	}

	reporters, err := a.status.GetReporters(a.snapshot)
	if err != nil {
		return controller.RequeueWithError(err)
	}

	for _, reporter := range reporters {
		if err := reporter.ReportStatusForSnapshot(a.client, a.context, &a.logger, a.snapshot); err != nil {
			a.logger.Error(err, "failed to report test status to github for snapshot",
				"snapshot.Namespace", a.snapshot.Namespace, "snapshot.Name", a.snapshot.Name)
			return controller.RequeueWithError(err)
		}
	}

	return controller.ContinueProcessing()
}

// isFeatureEnabled returns true when the feature flag FEATURE_STATUS_REPORTING_ENABLED has been defined in env vars
func isFeatureEnabled() bool {
	if _, found := os.LookupEnv(FeatureFlagStatusReprotingEnabled); found {
		return true
	}
	return false
}

// EnsureSnapshotFinishedAllTests is an operation that will ensure that a pipeline Snapshot
// to the PipelineRun being processed finished and passed all tests for all defined required IntegrationTestScenarios.
// If the Snapshot doesn't have the freshest state of components, a composite Snapshot will be created instead
// and the original Snapshot will be marked as Invalid.
func (a *Adapter) EnsureSnapshotFinishedAllTests() (controller.OperationResult, error) {
	// Get all required integrationTestScenarios for the Application and then use the Snapshot status annotation
	// to check if all Integration tests were finished for that Snapshot
	integrationTestScenarios, err := a.loader.GetRequiredIntegrationTestScenariosForApplication(a.client, a.context, a.application)
	if err != nil {
		return controller.RequeueWithError(err)
	}

	testStatuses, err := gitops.NewSnapshotIntegrationTestStatusesFromSnapshot(a.snapshot)
	if err != nil {
		return controller.RequeueWithError(err)
	}

	allIntegrationTestsFinished, allIntegrationTestsPassed := a.determineIfAllIntegrationTestsFinishedAndPassed(integrationTestScenarios, testStatuses)
	if err != nil {
		a.logger.Error(err, "Failed to determine outcomes for Integration Tests",
			"snapshot.Name", a.snapshot.Name)
		return controller.RequeueWithError(err)
	}

	// Skip doing anything if not all Integration tests were finished for all integrationTestScenarios
	if !allIntegrationTestsFinished {
		a.logger.Info("Not all required Integration PipelineRuns finished",
			"snapshot.Name", a.snapshot.Name)
		return controller.ContinueProcessing()
	}

	// Since all integration tests finished, set the Snapshot Integration status as finished, but don't update the resource yet
	gitops.SetSnapshotIntegrationStatusAsFinished(a.snapshot,
		"Snapshot integration status condition is finished since all testing pipelines completed")
	a.logger.LogAuditEvent("Snapshot integration status condition marked as finished, all testing pipelines completed",
		a.snapshot, helpers.LogActionUpdate)

	// If the Snapshot is a component type, check if the global component list changed in the meantime and
	// create a composite snapshot if it did. Does not apply for PAC pull request events.
	if metadata.HasLabelWithValue(a.snapshot, gitops.SnapshotTypeLabel, gitops.SnapshotComponentType) && !gitops.IsSnapshotCreatedByPACPullRequestEvent(a.snapshot) {
		component, err := a.loader.GetComponentFromSnapshot(a.client, a.context, a.snapshot)
		if err != nil {
			a.logger.Error(err, "Failed to get Component for snapshot")
			return controller.RequeueWithError(err)
		}

		compositeSnapshot, err := a.createCompositeSnapshotsIfConflictExists(a.application, component, a.snapshot)
		if err != nil {
			a.logger.Error(err, "Failed to determine if a composite snapshot needs to be created because of a conflict",
				"snapshot.Name", a.snapshot.Name)
			return controller.RequeueWithError(err)
		}

		if compositeSnapshot != nil {
			a.logger.Info("The global component list has changed in the meantime, marking snapshot as Invalid",
				"snapshot.Name", a.snapshot.Name)
			if !gitops.IsSnapshotMarkedAsInvalid(a.snapshot) {
				patch := client.MergeFrom(a.snapshot.DeepCopy())
				gitops.SetSnapshotIntegrationStatusAsInvalid(a.snapshot,
					"The global component list has changed in the meantime, superseding with a composite snapshot")
				err := a.client.Status().Patch(a.context, a.snapshot, patch)
				if err != nil {
					a.logger.Error(err, "Failed to update the status to Invalid for the snapshot",
						"snapshot.Name", a.snapshot.Name)
					return controller.RequeueWithError(err)
				}
				a.logger.LogAuditEvent("Snapshot integration status condition marked as invalid, the global component list has changed in the meantime",
					a.snapshot, helpers.LogActionUpdate)
			}
			return controller.ContinueProcessing()
		}
	}

	// If all Integration Pipeline runs passed, mark the snapshot as succeeded, otherwise mark it as failed
	// This updates the Snapshot resource on the cluster
	if allIntegrationTestsPassed {
		if !gitops.IsSnapshotMarkedAsPassed(a.snapshot) {
			a.snapshot, err = gitops.MarkSnapshotAsPassed(a.client, a.context, a.snapshot, "All Integration Pipeline tests passed")
			if err != nil {
				a.logger.Error(err, "Failed to Update Snapshot AppStudioTestSucceeded status")
				return controller.RequeueWithError(err)
			}
			a.logger.LogAuditEvent("Snapshot integration status condition marked as passed, all Integration PipelineRuns succeeded",
				a.snapshot, helpers.LogActionUpdate)
		}
	} else {
		if !gitops.IsSnapshotMarkedAsFailed(a.snapshot) {
			a.snapshot, err = gitops.MarkSnapshotAsFailed(a.client, a.context, a.snapshot, "Some Integration pipeline tests failed")
			if err != nil {
				a.logger.Error(err, "Failed to Update Snapshot AppStudioTestSucceeded status")
				return controller.RequeueWithError(err)
			}
			a.logger.LogAuditEvent("Snapshot integration status condition marked as failed, some tests within Integration PipelineRuns failed",
				a.snapshot, helpers.LogActionUpdate)
		}
	}

	return controller.ContinueProcessing()
}

// determineIfAllIntegrationTestsFinishedAndPassed checks if all Integration tests finished and passed for the given
// list of integrationTestScenarios.
func (a *Adapter) determineIfAllIntegrationTestsFinishedAndPassed(integrationTestScenarios *[]v1beta1.IntegrationTestScenario, testStatuses *gitops.SnapshotIntegrationTestStatuses) (bool, bool) {
	allIntegrationTestsFinished, allIntegrationTestsPassed := true, true
	for _, integrationTestScenario := range *integrationTestScenarios {
		integrationTestScenario := integrationTestScenario // G601
		testDetails, ok := testStatuses.GetScenarioStatus(integrationTestScenario.Name)
		if !ok || (testDetails.Status != gitops.IntegrationTestStatusTestPassed && testDetails.Status != gitops.IntegrationTestStatusTestFail) {
			allIntegrationTestsFinished = false
		}
		if ok && testDetails.Status != gitops.IntegrationTestStatusTestPassed {
			allIntegrationTestsPassed = false
		}

	}
	return allIntegrationTestsFinished, allIntegrationTestsPassed
}

// prepareCompositeSnapshot prepares the Composite Snapshot for a given application,
// component, containerImage and containerSource. In case the Snapshot can't be created, an error will be returned.
func (a *Adapter) prepareCompositeSnapshot(application *applicationapiv1alpha1.Application, component *applicationapiv1alpha1.Component, newContainerImage string, newComponentSource *applicationapiv1alpha1.ComponentSource) (*applicationapiv1alpha1.Snapshot, error) {
	applicationComponents, err := a.loader.GetAllApplicationComponents(a.client, a.context, application)
	if err != nil {
		return nil, err
	}

	snapshot, err := gitops.PrepareSnapshot(a.client, a.context, application, applicationComponents, component, newContainerImage, newComponentSource)
	if err != nil {
		return nil, err
	}

	if snapshot.Labels == nil {
		snapshot.Labels = map[string]string{}
	}
	snapshot.Labels[gitops.SnapshotTypeLabel] = gitops.SnapshotCompositeType

	return snapshot, nil
}

// createCompositeSnapshotsIfConflictExists checks if the component Snapshot is good to release by checking if any
// of the other components containerImages changed in the meantime. If any of them changed, it creates a new composite snapshot.
func (a *Adapter) createCompositeSnapshotsIfConflictExists(application *applicationapiv1alpha1.Application, component *applicationapiv1alpha1.Component, testedSnapshot *applicationapiv1alpha1.Snapshot) (*applicationapiv1alpha1.Snapshot, error) {
	newContainerImage, err := a.getImagePullSpecFromSnapshotComponent(testedSnapshot, component)
	if err != nil {
		return nil, err
	}

	newComponentSource, err := a.getComponentSourceFromSnapshotComponent(testedSnapshot, component)
	if err != nil {
		return nil, err
	}

	compositeSnapshot, err := a.prepareCompositeSnapshot(application, component, newContainerImage, newComponentSource)
	if err != nil {
		return nil, err
	}

	// Copy PAC annotations/labels from testedSnapshot to compositeSnapshot.
	_ = metadata.CopyLabelsByPrefix(&testedSnapshot.ObjectMeta, &compositeSnapshot.ObjectMeta, gitops.PipelinesAsCodePrefix)
	_ = metadata.CopyAnnotationsByPrefix(&testedSnapshot.ObjectMeta, &compositeSnapshot.ObjectMeta, gitops.PipelinesAsCodePrefix)

	// Create the new composite snapshot if it doesn't exist already
	if !gitops.CompareSnapshots(compositeSnapshot, testedSnapshot) {
		allSnapshots, err := a.loader.GetAllSnapshots(a.client, a.context, application)
		if err != nil {
			return nil, err
		}
		existingCompositeSnapshot := gitops.FindMatchingSnapshot(a.application, allSnapshots, compositeSnapshot)

		if existingCompositeSnapshot != nil {
			a.logger.Info("Found existing composite Snapshot",
				"snapshot.Name", existingCompositeSnapshot.Name,
				"snapshot.Spec.Components", existingCompositeSnapshot.Spec.Components)
			return existingCompositeSnapshot, nil
		} else {
			err = a.client.Create(a.context, compositeSnapshot)
			if err != nil {
				return nil, err
			}
			go metrics.RegisterNewSnapshot()
			a.logger.LogAuditEvent("CompositeSnapshot created", compositeSnapshot, helpers.LogActionAdd,
				"snapshot.Spec.Components", compositeSnapshot.Spec.Components)
			return compositeSnapshot, nil
		}
	}

	return nil, nil
}

// getImagePullSpecFromSnapshotComponent gets the full image pullspec from the given Snapshot Component,
func (a *Adapter) getImagePullSpecFromSnapshotComponent(snapshot *applicationapiv1alpha1.Snapshot, component *applicationapiv1alpha1.Component) (string, error) {
	for _, snapshotComponent := range snapshot.Spec.Components {
		if snapshotComponent.Name == component.Name {
			return snapshotComponent.ContainerImage, nil
		}
	}
	return "", fmt.Errorf("couldn't find the requested component info in the given Snapshot")
}

// getComponentSourceFromSnapshotComponent gets the component source from the given Snapshot for the given Component,
func (a *Adapter) getComponentSourceFromSnapshotComponent(snapshot *applicationapiv1alpha1.Snapshot, component *applicationapiv1alpha1.Component) (*applicationapiv1alpha1.ComponentSource, error) {
	for _, snapshotComponent := range snapshot.Spec.Components {
		if snapshotComponent.Name == component.Name {
			return &snapshotComponent.Source, nil
		}
	}
	return nil, fmt.Errorf("couldn't find the requested component source info in the given Snapshot")
}
