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

package buildpipeline

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"k8s.io/client-go/util/retry"

	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	"github.com/konflux-ci/integration-service/api/v1beta2"
	"github.com/konflux-ci/integration-service/gitops"
	h "github.com/konflux-ci/integration-service/helpers"
	"github.com/konflux-ci/integration-service/loader"
	intgteststat "github.com/konflux-ci/integration-service/pkg/integrationteststatus"
	"github.com/konflux-ci/integration-service/status"
	"github.com/konflux-ci/integration-service/tekton"
	"github.com/konflux-ci/operator-toolkit/controller"
	"github.com/konflux-ci/operator-toolkit/metadata"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// Adapter holds the objects needed to reconcile a build PipelineRun.
type Adapter struct {
	pipelineRun *tektonv1.PipelineRun
	component   *applicationapiv1alpha1.Component
	application *applicationapiv1alpha1.Application
	loader      loader.ObjectLoader
	logger      h.IntegrationLogger
	client      client.Client
	context     context.Context
	status      status.StatusInterface
}

// NewAdapter creates and returns an Adapter instance.
func NewAdapter(context context.Context, pipelineRun *tektonv1.PipelineRun, component *applicationapiv1alpha1.Component, application *applicationapiv1alpha1.Application,
	logger h.IntegrationLogger, loader loader.ObjectLoader, client client.Client,
) *Adapter {
	return &Adapter{
		pipelineRun: pipelineRun,
		component:   component,
		application: application,
		logger:      logger,
		loader:      loader,
		client:      client,
		context:     context,
		status:      status.NewStatus(logger.Logger, client),
	}
}

// EnsureSnapshotExists is an operation that will ensure that a pipeline Snapshot associated
// to the build PipelineRun being processed exists. Otherwise, it will create a new pipeline Snapshot.
func (a *Adapter) EnsureSnapshotExists() (result controller.OperationResult, err error) {
	// a marker if we should remove finalizer from build PLR
	var canRemoveFinalizer bool

	defer func() {
		updateErr := a.updateBuildPipelineRunWithFinalInfo(canRemoveFinalizer, err)
		if updateErr != nil {
			if errors.IsNotFound(updateErr) {
				result, err = controller.ContinueProcessing()
			} else {
				a.logger.Error(updateErr, "Failed to update build pipelineRun")
				result, err = controller.RequeueWithError(updateErr)
			}
		}
	}()

	if !h.HasPipelineRunSucceeded(a.pipelineRun) {
		a.handleUnsuccessfulPipelineRun(&canRemoveFinalizer)
		return controller.ContinueProcessing()
	}

	if _, found := a.pipelineRun.ObjectMeta.Annotations[tekton.PipelineRunChainsSignedAnnotation]; !found {
		a.logger.Error(err, "Not processing the pipelineRun because it's not yet signed with Chains")
		return controller.ContinueProcessing()
	}

	if _, found := a.pipelineRun.ObjectMeta.Annotations[tekton.SnapshotNameLabel]; found {
		a.logger.Info("The build pipelineRun is already associated with existing Snapshot via annotation",
			"snapshot.Name", a.pipelineRun.ObjectMeta.Annotations[tekton.SnapshotNameLabel])
		canRemoveFinalizer = true
		return controller.ContinueProcessing()
	}

	existingSnapshots, err := a.loader.GetAllSnapshotsForBuildPipelineRun(a.context, a.client, a.pipelineRun)
	if err != nil {
		a.logger.Error(err, "Failed to fetch Snapshots for the build pipelineRun")
		return controller.RequeueWithError(err)
	}
	if len(*existingSnapshots) > 0 {
		result, err = a.ensureBuildPLRSWithSnapshotAnnotation(&canRemoveFinalizer, existingSnapshots)
		return result, err
	}

	expectedSnapshot, err := a.prepareSnapshotForPipelineRun(a.pipelineRun, a.component, a.application)
	if err != nil {
		return a.updatePipelineRunWithCustomizedError(&canRemoveFinalizer, err, a.context, a.pipelineRun, a.client, a.logger)
	}

	err = a.client.Create(a.context, expectedSnapshot)
	if err != nil {
		result, err = a.handleSnapshotCreationFailure(&canRemoveFinalizer, err)
		return result, err
	}

	a.logger.LogAuditEvent("Created new Snapshot", expectedSnapshot, h.LogActionAdd,
		"snapshot.Name", expectedSnapshot.Name,
		"snapshot.Spec.Components", expectedSnapshot.Spec.Components)

	err = a.annotateBuildPipelineRunWithSnapshot(expectedSnapshot)
	if err != nil {
		a.logger.Error(err, "Failed to update the build pipelineRun with new annotations",
			"pipelineRun.Name", a.pipelineRun.Name)
		return controller.RequeueWithError(err)
	}

	canRemoveFinalizer = true
	return controller.ContinueProcessing()
}

func (a *Adapter) EnsurePipelineIsFinalized() (controller.OperationResult, error) {
	if h.HasPipelineRunFinished(a.pipelineRun) || controllerutil.ContainsFinalizer(a.pipelineRun, h.IntegrationPipelineRunFinalizer) {
		return controller.ContinueProcessing()
	}

	err := h.AddFinalizerToPipelineRun(a.context, a.client, a.logger, a.pipelineRun, h.IntegrationPipelineRunFinalizer)
	if err != nil {
		a.logger.Error(err, fmt.Sprintf("Could not add finalizer %s to build pipeline %s", h.IntegrationPipelineRunFinalizer, a.pipelineRun.Name))
		return controller.RequeueWithError(err)
	}

	return controller.ContinueProcessing()
}

// EnsurePRGroupAnnotated is an operation that will ensure that the pr group info
// is added to build pipelineRun metadata label and annotation once it is created,
// then these label and annotation will be copied to component snapshot when it is created
func (a *Adapter) EnsurePRGroupAnnotated() (controller.OperationResult, error) {
	if tekton.IsPLRCreatedByPACPushEvent(a.pipelineRun) {
		a.logger.Info("build pipelineRun is not created by pull/merge request, no need to annotate")
		return controller.ContinueProcessing()
	}

	if metadata.HasLabel(a.pipelineRun, gitops.PRGroupHashLabel) && metadata.HasAnnotation(a.pipelineRun, gitops.PRGroupAnnotation) {
		// If the pipeline failed, attempt to notify all component Snapshots in the group about the failure
		if !h.HasPipelineRunSucceeded(a.pipelineRun) && h.HasPipelineRunFinished(a.pipelineRun) {
			err := a.notifySnapshotsInGroupAboutFailedBuild(a.pipelineRun)
			if err != nil {
				return controller.RequeueWithError(err)
			}
			a.logger.Info("notified all component snapshots in the pr group about the build pipeline failure")
		}
		a.logger.Info("build pipelineRun has had pr group info in metadata, no need to update")
		return controller.ContinueProcessing()
	}

	err := a.addPRGroupToBuildPLRMetadata(a.pipelineRun)
	if err != nil {
		if errors.IsNotFound(err) {
			a.logger.Error(err, "failed to add pr group info to build pipelineRun metadata due to notfound pipelineRun")
			return controller.StopProcessing()
		} else {
			a.logger.Error(err, "failed to add pr group info to build pipelineRun metadata")
			return controller.RequeueWithError(err)
		}

	}

	a.logger.LogAuditEvent("pr group info is updated to build pipelineRun metadata", a.pipelineRun, h.LogActionUpdate,
		"pipelineRun.Name", a.pipelineRun.Name)

	return controller.ContinueProcessing()
}

// EnsureIntegrationTestReportedToGitProvider is an operation that will ensure that the integration test status is initialized as pending
// state when build PLR is triggered/retriggered or cancelled/failed when failing to create snapshot
// to prevent PR/MR from being automerged unexpectedly and also show the snapshot creation failure on PR/MR
func (a *Adapter) EnsureIntegrationTestReportedToGitProvider() (controller.OperationResult, error) {
	if tekton.IsPLRCreatedByPACPushEvent(a.pipelineRun) {
		a.logger.Info("build pipelineRun is not created by pull/merge request, no need to set integration test status in git provider")
		return controller.ContinueProcessing()
	}

	if metadata.HasAnnotation(a.pipelineRun, tekton.SnapshotNameLabel) {
		SnapshotCreated := "SnapshotCreated"
		a.logger.Info("snapshot has been created for build pipelineRun, no need to report integration status from build pipelinerun status")
		if !metadata.HasAnnotationWithValue(a.pipelineRun, h.SnapshotCreationReportAnnotation, SnapshotCreated) {
			if err := tekton.AnnotateBuildPipelineRun(a.context, a.pipelineRun, h.SnapshotCreationReportAnnotation, SnapshotCreated, a.client); err != nil {
				a.logger.Error(err, "failed to annotate build pipelineRun")
				return controller.RequeueWithError(err)
			}
		}
		return controller.ContinueProcessing()
	}

	integrationTestStatus := getIntegrationTestStatusFromBuildPLR(a.pipelineRun)

	if integrationTestStatus == intgteststat.IntegrationTestStatus(0) {
		a.logger.Info("integration test has been set correctly or is being processed, no need to set integration test status from build pipelinerun")
		return controller.ContinueProcessing()
	}

	a.logger.Info("try to set integration test status according to the build PLR status")
	tempSnapshot := a.prepareTemporarySnapshot(a.pipelineRun)

	allIntegrationTestScenarios, err := a.loader.GetAllIntegrationTestScenariosForApplication(a.context, a.client, a.application)
	if err != nil {
		a.logger.Error(err, "Failed to get integration test scenarios for the following application",
			"Application.Namespace", a.application.Namespace, "Application.Name", a.application.Name)
		return controller.RequeueWithError(err)
	}

	if allIntegrationTestScenarios != nil {
		// Handle context in integrationTestScenario, but defer handling of 'group' for now
		integrationTestScenarios := gitops.FilterIntegrationTestScenariosWithContext(allIntegrationTestScenarios, tempSnapshot)
		a.logger.Info(
			fmt.Sprintf("Found %d IntegrationTestScenarios for application", len(*integrationTestScenarios)),
			"Application.Name", a.application.Name,
			"IntegrationTestScenarios", len(*integrationTestScenarios))
		if len(*integrationTestScenarios) == 0 {
			a.logger.Info("no need to report integration test status since no integrationTestScenario can be applied to snapshot created for build pipelinerun")
			return controller.ContinueProcessing()
		}

		err := a.ReportIntegrationTestStatusAccordingToBuildPLR(a.pipelineRun, tempSnapshot, integrationTestScenarios, integrationTestStatus, a.component)
		if err != nil {
			a.logger.Error(err, "failed to report snapshot creation status to git provider from build pipelineRun",
				"pipelineRun.Namespace", a.pipelineRun.Namespace, "pipelineRun.Name", a.pipelineRun.Name)
			return controller.RequeueWithError(err)
		}

		if err = tekton.AnnotateBuildPipelineRun(a.context, a.pipelineRun, h.SnapshotCreationReportAnnotation, integrationTestStatus.String(), a.client); err != nil {
			a.logger.Error(err, fmt.Sprintf("failed to write build plr annotation %s", h.SnapshotCreationReportAnnotation))
			return controller.RequeueWithError(fmt.Errorf("failed to write snapshot report status metadata for annotation %s: %w", h.SnapshotCreationReportAnnotation, err))
		}
	}
	return controller.ContinueProcessing()

}

// getImagePullSpecFromPipelineRun gets the full image pullspec from the given build PipelineRun,
// In case the Image pullspec can't be composed, an error will be returned.
func (a *Adapter) getImagePullSpecFromPipelineRun(pipelineRun *tektonv1.PipelineRun) (string, error) {
	outputImage, err := tekton.GetOutputImage(pipelineRun)
	if err != nil {
		return "", err
	}
	imageDigest, err := tekton.GetOutputImageDigest(pipelineRun)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s@%s", strings.Split(outputImage, ":")[0], imageDigest), nil
}

// getComponentSourceFromPipelineRun gets the component Git Source for the Component built in the given build PipelineRun,
// In case the Git Source can't be composed, an error will be returned.
func (a *Adapter) getComponentSourceFromPipelineRun(pipelineRun *tektonv1.PipelineRun) (*applicationapiv1alpha1.ComponentSource, error) {
	componentSourceGitUrl, err := tekton.GetComponentSourceGitUrl(pipelineRun)
	if err != nil {
		return nil, err
	}
	componentSourceGitCommit, err := tekton.GetComponentSourceGitCommit(pipelineRun)
	if err != nil {
		return nil, err
	}
	componentSource := applicationapiv1alpha1.ComponentSource{
		ComponentSourceUnion: applicationapiv1alpha1.ComponentSourceUnion{
			GitSource: &applicationapiv1alpha1.GitSource{
				URL:      componentSourceGitUrl,
				Revision: componentSourceGitCommit,
			},
		},
	}

	return &componentSource, nil
}

// notifySnapshotsInGroupAboutFailedBuild tries to find the latest group Snapshot and notify it about the failed build.
// If the group Snapshot can't be found, find the component Snapshots that would belong to the same group and notify them instead.
func (a *Adapter) notifySnapshotsInGroupAboutFailedBuild(pipelineRun *tektonv1.PipelineRun) error {
	prGroupHash := pipelineRun.Labels[gitops.PRGroupHashLabel]
	prGroupName := pipelineRun.Annotations[gitops.PRGroupAnnotation]
	buildPLRFailureMsg := fmt.Sprintf("build PLR %s failed for component %s so it can't be added to the group Snapshot for PR group %s", pipelineRun.Name, a.component.Name, prGroupName)

	applicationComponents, err := a.loader.GetAllApplicationComponents(a.context, a.client, a.application)
	if err != nil {
		return err
	}

	// Annotate all latest component Snapshots that are part of the PR group
	for _, applicationComponent := range *applicationComponents {
		applicationComponent := applicationComponent // G601
		allComponentSnapshotsInGroup, err := a.loader.GetMatchingComponentSnapshotsForComponentAndPRGroupHash(a.context, a.client, a.pipelineRun.Namespace, applicationComponent.Name, prGroupHash)
		if err != nil {
			a.logger.Error(err, "Failed to fetch Snapshots for component", "component.Name", applicationComponent.Name)
			return err
		}

		if len(*allComponentSnapshotsInGroup) > 0 {
			latestSnapshot := gitops.SortSnapshots(*allComponentSnapshotsInGroup)[0]
			err = gitops.AnnotateSnapshot(a.context, &latestSnapshot, gitops.PRGroupCreationAnnotation,
				buildPLRFailureMsg, a.client)
			if err != nil {
				return err
			}
		}
	}

	// In case there are in-flight build pipelineRuns, we want to also annotate them to make sure that the failure is propagated
	// to future Snapshots in the group
	buildPipelineRuns, err := a.loader.GetPipelineRunsWithPRGroupHash(a.context, a.client, a.pipelineRun.Namespace, prGroupHash)
	if err != nil {
		a.logger.Error(err, fmt.Sprintf("Failed to get build pipelineRuns for given pr group hash %s", prGroupHash))
		return err
	}
	for _, buildPipelineRun := range *buildPipelineRuns {
		buildPipelineRun := buildPipelineRun
		// check if build PLR finished
		if !h.HasPipelineRunFinished(&buildPipelineRun) && buildPipelineRun.Labels[tekton.ComponentNameLabel] != a.component.Name {
			err := tekton.AnnotateBuildPipelineRun(a.context, &buildPipelineRun, gitops.PRGroupCreationAnnotation, buildPLRFailureMsg, a.client)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// prepareSnapshotForPipelineRun prepares the Snapshot for a given PipelineRun,
// component and application. In case the Snapshot can't be created, an error will be returned.
func (a *Adapter) prepareSnapshotForPipelineRun(pipelineRun *tektonv1.PipelineRun, component *applicationapiv1alpha1.Component, application *applicationapiv1alpha1.Application) (*applicationapiv1alpha1.Snapshot, error) {
	newContainerImage, err := a.getImagePullSpecFromPipelineRun(pipelineRun)
	if err != nil {
		return nil, err
	}
	componentSource, err := a.getComponentSourceFromPipelineRun(pipelineRun)
	if err != nil {
		return nil, err
	}

	applicationComponents, err := a.loader.GetAllApplicationComponents(a.context, a.client, application)
	if err != nil {
		return nil, err
	}

	snapshot, err := gitops.PrepareSnapshot(a.context, a.client, application, applicationComponents, component, newContainerImage, componentSource)
	if err != nil {
		return nil, err
	}

	prefixes := []string{gitops.BuildPipelineRunPrefix, gitops.TestLabelPrefix, gitops.CustomLabelPrefix}
	gitops.CopySnapshotLabelsAndAnnotations(application, snapshot, a.component.Name, &pipelineRun.ObjectMeta, prefixes)

	snapshot.Labels[gitops.BuildPipelineRunNameLabel] = pipelineRun.Name
	if pipelineRun.Status.CompletionTime != nil {
		snapshot.Labels[gitops.BuildPipelineRunFinishTimeLabel] = strconv.FormatInt(pipelineRun.Status.CompletionTime.Time.Unix(), 10)
	} else {
		snapshot.Labels[gitops.BuildPipelineRunFinishTimeLabel] = strconv.FormatInt(time.Now().Unix(), 10)
	}

	if pipelineRun.Status.StartTime != nil {
		snapshot.Annotations[gitops.BuildPipelineRunStartTime] = strconv.FormatInt(pipelineRun.Status.StartTime.Time.Unix(), 10)
	}

	return snapshot, nil
}

func (a *Adapter) annotateBuildPipelineRunWithSnapshot(snapshot *applicationapiv1alpha1.Snapshot) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var err error
		a.pipelineRun, err = a.loader.GetPipelineRun(a.context, a.client, a.pipelineRun.Name, a.pipelineRun.Namespace)
		if err != nil {
			return err
		}

		err = tekton.AnnotateBuildPipelineRun(a.context, a.pipelineRun, tekton.SnapshotNameLabel, snapshot.Name, a.client)
		if err == nil {
			a.logger.LogAuditEvent("Updated build pipelineRun", a.pipelineRun, h.LogActionUpdate,
				"snapshot.Name", snapshot.Name)
		}
		return err
	})
}

// updateBuildPipelineRunWithFinalInfo adds the final pieces of information to the pipelineRun in order to ensure
// that anything that happened during the reconciliation is reflected in the CR
func (a *Adapter) updateBuildPipelineRunWithFinalInfo(canRemoveFinalizer bool, cerr error) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var err error
		a.pipelineRun, err = a.loader.GetPipelineRun(a.context, a.client, a.pipelineRun.Name, a.pipelineRun.Namespace)
		if err != nil {
			return err
		}

		if a.pipelineRun.GetDeletionTimestamp() == nil {
			err = tekton.AnnotateBuildPipelineRunWithCreateSnapshotAnnotation(a.context, a.pipelineRun, a.client, cerr)
			if err != nil {
				a.logger.Error(err, "Failed to annotate build pipelinerun with the snapshot creation status")
				return err
			}
			a.logger.LogAuditEvent("Updated build pipelineRun", a.pipelineRun, h.LogActionUpdate,
				h.CreateSnapshotAnnotationName, a.pipelineRun.Annotations[h.CreateSnapshotAnnotationName])
		} else {
			a.logger.Info("Cannot annotate build pipelinerun annotation with the snapshot creation status since it has been marked as deleted")
		}
		if canRemoveFinalizer {
			err = h.RemoveFinalizerFromPipelineRun(a.context, a.client, a.logger, a.pipelineRun, h.IntegrationPipelineRunFinalizer)
			if err != nil {
				a.logger.Error(err, "Failed to remove finalizer from build pipelinerun")
				return err
			}
		}
		return nil
	})
}

// checkForSnapshotsCount makes sure that only one snapshot is associated with build pipelinerun and annotate that PLR with snapshot
// informs about fact when PLR is associated with more existing snapshots
func (a *Adapter) ensureBuildPLRSWithSnapshotAnnotation(canRemoveFinalizer *bool, existingSnapshots *[]applicationapiv1alpha1.Snapshot) (result controller.OperationResult, err error) {
	if len(*existingSnapshots) == 1 {
		existingSnapshot := (*existingSnapshots)[0]
		a.logger.Info("There is an existing Snapshot associated with this build pipelineRun, but the pipelineRun is not yet annotated",
			"snapshot.Name", existingSnapshot.Name)
		err := a.annotateBuildPipelineRunWithSnapshot(&existingSnapshot)
		if err != nil {
			a.logger.Error(err, "Failed to update the build pipelineRun with snapshot name",
				"pipelineRun.Name", a.pipelineRun.Name)
			return controller.RequeueWithError(err)
		}
	} else {
		a.logger.Info("The build pipelineRun is already associated with more than one existing Snapshot")
	}
	*canRemoveFinalizer = true
	return controller.ContinueProcessing()
}

// failedOrDeletedPLR checks for pipelinerun state and proceeds according to it,
// failed or in running state > report this into a logger and set canRemoveFinalizer flag to true
func (a *Adapter) handleUnsuccessfulPipelineRun(canRemoveFinalizer *bool) {
	// The build pipelinerun has not finished
	if h.HasPipelineRunFinished(a.pipelineRun) || a.pipelineRun.GetDeletionTimestamp() != nil {
		// The pipeline run has failed
		// OR pipeline has been deleted but it's still in running state (tekton bug/feature?)
		a.logger.Info("Finished processing of unsuccessful build PLR",
			"statusCondition", a.pipelineRun.GetStatusCondition(),
			"deletionTimestamp", a.pipelineRun.GetDeletionTimestamp(),
		)
		*canRemoveFinalizer = true
	}
}

// failedToCreateSnapshot stops reconcilation immediately when snapshot cannot be created
func (a *Adapter) handleSnapshotCreationFailure(canRemoveFinalizer *bool, cerr error) (result controller.OperationResult, err error) {
	a.logger.Error(cerr, "Failed to create Snapshot")
	if errors.IsForbidden(cerr) {
		// we cannot create a snapshot (possibly because the snapshot quota is hit) and we don't want to block resources, user has to retry
		// we still return the error to make build PLR annotated when meeting quota limitation issue
		*canRemoveFinalizer = true
		return controller.RequeueAfter(time.Hour, cerr)
	}
	return controller.RequeueWithError(cerr)
}

// plrWithCustomizedError checks for customized error returned by PipelineRun result,
// updates build PipelineRun annotation with this error and exits
func (a *Adapter) updatePipelineRunWithCustomizedError(canRemoveFinalizer *bool, cerr error, context context.Context, pipelineRun *tektonv1.PipelineRun, client client.Client, logger h.IntegrationLogger) (result controller.OperationResult, err error) {
	// If PipelineRun result returns cusomized error update PLR annotation and exit
	if h.IsMissingInfoInPipelineRunError(cerr) || h.IsInvalidImageDigestError(cerr) || h.IsMissingValidComponentError(cerr) {
		// update the build PLR annotation with the error cusomized Reason and Value
		if annotateErr := tekton.AnnotateBuildPipelineRunWithCreateSnapshotAnnotation(context, pipelineRun, client, cerr); annotateErr != nil {
			logger.Error(annotateErr, "Could not add create snapshot annotation to build pipelineRun", h.CreateSnapshotAnnotationName, pipelineRun)
		}
		logger.Error(cerr, "Build PipelineRun failed with error, should be fixed and re-run manually", "pipelineRun.Name", pipelineRun.Name)
		*canRemoveFinalizer = true
		return controller.StopProcessing()
	}
	return controller.RequeueOnErrorOrContinue(cerr)

}

// addPRGroupToBuildPLRMetadata will add pr-group info gotten from souce-branch to annotation
// and also the string in sha format to metadata label
func (a *Adapter) addPRGroupToBuildPLRMetadata(pipelineRun *tektonv1.PipelineRun) error {
	prGroup := tekton.GetPRGroupFromBuildPLR(pipelineRun)
	if prGroup != "" {
		prGroupHash := tekton.GenerateSHA(prGroup)

		return retry.RetryOnConflict(retry.DefaultRetry, func() error {
			var err error
			pipelineRun, err = a.loader.GetPipelineRun(a.context, a.client, pipelineRun.Name, pipelineRun.Namespace)
			if err != nil {
				return err
			}

			patch := client.MergeFrom(pipelineRun.DeepCopy())

			_ = metadata.SetAnnotation(&pipelineRun.ObjectMeta, gitops.PRGroupAnnotation, prGroup)
			_ = metadata.SetLabel(&pipelineRun.ObjectMeta, gitops.PRGroupHashLabel, prGroupHash)

			return a.client.Patch(a.context, pipelineRun, patch)
		})
	}
	a.logger.Info("can't find source branch info in build PLR, not need to update build pipelineRun metadata")
	return nil
}

// prepareTemporarySnapshot will create a temporary snapshot object to copy the labels/annotations from build pipelinerun
// and be used to communicate with git provider
func (a *Adapter) prepareTemporarySnapshot(pipelineRun *tektonv1.PipelineRun) *applicationapiv1alpha1.Snapshot {
	tempSnapshot := &applicationapiv1alpha1.Snapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tempSnapshot",
			Namespace: pipelineRun.Namespace,
		},
	}
	prefixes := []string{gitops.BuildPipelineRunPrefix, gitops.TestLabelPrefix, gitops.CustomLabelPrefix}
	gitops.CopySnapshotLabelsAndAnnotations(a.application, tempSnapshot, a.component.Name, &pipelineRun.ObjectMeta, prefixes)
	return tempSnapshot
}

func (a *Adapter) ReportIntegrationTestStatusAccordingToBuildPLR(pipelineRun *tektonv1.PipelineRun, snapshot *applicationapiv1alpha1.Snapshot, integrationTestScenarios *[]v1beta2.IntegrationTestScenario,
	integrationTestStatus intgteststat.IntegrationTestStatus, component *applicationapiv1alpha1.Component) error {
	reporter := a.status.GetReporter(snapshot)
	if reporter == nil {
		a.logger.Info("No suitable reporter found, skipping report")
		return nil
	}
	a.logger.Info(fmt.Sprintf("Detected reporter: %s", reporter.GetReporterName()))

	if err := reporter.Initialize(a.context, snapshot); err != nil {
		a.logger.Error(err, "Failed to initialize reporter", "reporter", reporter.GetReporterName())
		return fmt.Errorf("failed to initialize reporter: %w", err)
	}
	a.logger.Info("Reporter initialized", "reporter", reporter.GetReporterName())

	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		err := a.iterateIntegrationTestInStatusReport(reporter, pipelineRun, snapshot, integrationTestScenarios, integrationTestStatus, component)
		if err != nil {
			a.logger.Error(err, fmt.Sprintf("failed to report integration test status according to build pipelinerun %s/%s",
				pipelineRun.Namespace, pipelineRun.Name))
			return fmt.Errorf("failed to report integration test status according to build pipelineRun %s/%s: %w",
				pipelineRun.Namespace, pipelineRun.Name, err)
		}

		a.logger.Info("Successfully report integration test status for build pipelineRun",
			"pipelineRun.Namespace", pipelineRun.Namespace,
			"pipelineRun.Name", pipelineRun.Name,
			"build pipelineRun Status", integrationTestStatus.String())

		return err
	})

	if err != nil {
		return fmt.Errorf("issue occurred during generating or updating report status: %w", err)
	}

	a.logger.Info(fmt.Sprintf("Successfully updated the %s annotation", gitops.SnapshotStatusReportAnnotation), "pipelinerun.Name", pipelineRun.Name)

	return nil
}

// iterates integrationTestScenarios to set integration test status in PR/MR
func (a *Adapter) iterateIntegrationTestInStatusReport(reporter status.ReporterInterface,
	buildPLR *tektonv1.PipelineRun,
	snapshot *applicationapiv1alpha1.Snapshot,
	integrationTestScenarios *[]v1beta2.IntegrationTestScenario,
	intgteststatus intgteststat.IntegrationTestStatus,
	component *applicationapiv1alpha1.Component) error {

	details := generateDetails(buildPLR, intgteststatus)
	integrationTestStatusDetail := intgteststat.IntegrationTestStatusDetail{
		Status:  intgteststatus,
		Details: details,
	}
	for _, integrationTestScenario := range *integrationTestScenarios {
		integrationTestScenario := integrationTestScenario //G601
		integrationTestStatusDetail.ScenarioName = integrationTestScenario.Name

		testReport, reportErr := status.GenerateTestReport(a.context, a.client, integrationTestStatusDetail, snapshot, component.Name)
		if reportErr != nil {
			return fmt.Errorf("failed to generate test report: %w", reportErr)
		}
		if reportStatusErr := reporter.ReportStatus(a.context, *testReport); reportStatusErr != nil {
			return fmt.Errorf("failed to report status to git provider: %w", reportStatusErr)
		}
	}
	return nil
}

// getIntegrationTestStatusFromBuildPLR get the build PLR status to decide the integration test status
// reported to PR/MR when build PLR is triggered/retriggered, build PLR fails or snapshot can't be created
func getIntegrationTestStatusFromBuildPLR(plr *tektonv1.PipelineRun) intgteststat.IntegrationTestStatus {
	var integrationTestStatus intgteststat.IntegrationTestStatus
	// when build pipeline is triggered/retriggered, we need to set integration test status to pending
	if !h.HasPipelineRunFinished(plr) && !metadata.HasAnnotationWithValue(plr, h.SnapshotCreationReportAnnotation, intgteststat.BuildPLRInProgress.String()) {
		integrationTestStatus = intgteststat.BuildPLRInProgress
		return integrationTestStatus
	}
	// when build pipeline fails, we need to set integreation test to canceled or failed
	if h.HasPipelineRunFinished(plr) && !h.HasPipelineRunSucceeded(plr) && !metadata.HasAnnotationWithValue(plr, h.SnapshotCreationReportAnnotation, intgteststat.BuildPLRFailed.String()) {
		integrationTestStatus = intgteststat.BuildPLRFailed
		return integrationTestStatus
	}
	// when build pipeline succeeds but snapshot is not created, we need to set integration test to canceled or failed
	if h.HasPipelineRunSucceeded(plr) && metadata.HasAnnotation(plr, h.CreateSnapshotAnnotationName) && !metadata.HasAnnotation(plr, tekton.SnapshotNameLabel) && !metadata.HasAnnotationWithValue(plr, h.SnapshotCreationReportAnnotation, intgteststat.SnapshotCreationFailed.String()) {
		integrationTestStatus = intgteststat.SnapshotCreationFailed
		return integrationTestStatus
	}
	return integrationTestStatus
}

// generateDetails generates details for integrationTestStatusDetail
func generateDetails(buildPLR *tektonv1.PipelineRun, integrationTestStatus intgteststat.IntegrationTestStatus) string {
	details := ""
	if integrationTestStatus == intgteststat.BuildPLRInProgress {
		details = fmt.Sprintf("build pipelinerun %s/%s is still in progress", buildPLR.Namespace, buildPLR.Name)
	}
	if integrationTestStatus == intgteststat.SnapshotCreationFailed {
		if failureReason, ok := buildPLR.Annotations[h.CreateSnapshotAnnotationName]; ok {
			details = fmt.Sprintf("build pipelinerun %s/%s succeeds but snapshot is not created due to error: %s", buildPLR.Namespace, buildPLR.Name, failureReason)
		} else {
			details = fmt.Sprintf("failed to create snapshot but can't find reason from build plr annotation %s", h.CreateSnapshotAnnotationName)
		}
	}
	if integrationTestStatus == intgteststat.BuildPLRFailed {
		details = fmt.Sprintf("build pipelinerun %s/%s failed, so that snapshot is not created. Please fix build pipelinerun failure and try again.", buildPLR.Namespace, buildPLR.Name)
	}
	return details
}
