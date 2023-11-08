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

	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/gitops"
	h "github.com/redhat-appstudio/integration-service/helpers"
	"github.com/redhat-appstudio/integration-service/loader"
	"github.com/redhat-appstudio/integration-service/metrics"
	"github.com/redhat-appstudio/integration-service/tekton"
	"github.com/redhat-appstudio/operator-toolkit/controller"
	"github.com/redhat-appstudio/operator-toolkit/metadata"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
}

// NewAdapter creates and returns an Adapter instance.
func NewAdapter(pipelineRun *tektonv1.PipelineRun, component *applicationapiv1alpha1.Component, application *applicationapiv1alpha1.Application, logger h.IntegrationLogger, loader loader.ObjectLoader, client client.Client,
	context context.Context) *Adapter {
	return &Adapter{
		pipelineRun: pipelineRun,
		component:   component,
		application: application,
		logger:      logger,
		loader:      loader,
		client:      client,
		context:     context,
	}
}

// EnsureSnapshotExists is an operation that will ensure that a pipeline Snapshot associated
// to the build PipelineRun being processed exists. Otherwise, it will create a new pipeline Snapshot.
func (a *Adapter) EnsureSnapshotExists() (controller.OperationResult, error) {
	if !h.HasPipelineRunSucceeded(a.pipelineRun) {
		return controller.ContinueProcessing()
	}

	if a.component == nil {
		a.logger.Info("The build pipelineRun does not have any component associated with it, will not create a new Snapshot.")
		return controller.ContinueProcessing()
	}

	isLatest, err := a.isLatestSucceededBuildPipelineRun()
	if err != nil {
		return controller.RequeueWithError(err)
	}
	if !isLatest {
		// not the last started pipeline that succeeded for current snapshot
		// this prevents deploying older pipeline run over new deployment
		a.logger.Info("The pipelineRun is not the latest successful build pipelineRun for the component, skipping creation of a new Snapshot ",
			"component.Name", a.component.Name)
		return controller.ContinueProcessing()
	}

	expectedSnapshot, err := a.prepareSnapshotForPipelineRun(a.pipelineRun, a.component, a.application)
	if err != nil {
		return controller.RequeueWithError(err)
	}

	allSnapshots, err := a.loader.GetAllSnapshots(a.client, a.context, a.application)
	if err != nil {
		return controller.RequeueWithError(err)
	}
	existingSnapshot := gitops.FindMatchingSnapshot(a.application, allSnapshots, expectedSnapshot)

	if existingSnapshot != nil {
		a.logger.Info("Found existing Snapshot",
			"snapshot.Name", existingSnapshot.Name,
			"snapshot.Spec.Components", existingSnapshot.Spec.Components)
		// Annotate the build pipelineRun with the existing Snapshot if it hasn't been already annotated
		if _, found := a.pipelineRun.ObjectMeta.Annotations[tekton.SnapshotNameLabel]; !found {
			a.pipelineRun, err = a.annotateBuildPipelineRunWithSnapshot(a.pipelineRun, existingSnapshot)
			if err != nil {
				a.logger.Error(err, "Failed to update the build pipelineRun with new annotations",
					"pipelineRun.Name", a.pipelineRun.Name)
				return controller.RequeueWithError(err)
			}
		}
		return controller.ContinueProcessing()
	}

	err = a.client.Create(a.context, expectedSnapshot)
	if err != nil {
		a.logger.Error(err, "Failed to create Snapshot")
		return controller.RequeueWithError(err)
	}
	go metrics.RegisterNewSnapshot()

	a.logger.LogAuditEvent("Created new Snapshot", expectedSnapshot, h.LogActionAdd,
		"snapshot.Name", expectedSnapshot.Name,
		"snapshot.Spec.Components", expectedSnapshot.Spec.Components)

	a.pipelineRun, err = a.annotateBuildPipelineRunWithSnapshot(a.pipelineRun, expectedSnapshot)
	if err != nil {
		a.logger.Error(err, "Failed to update the build pipelineRun with new annotations",
			"pipelineRun.Name", a.pipelineRun.Name)
		return controller.RequeueWithError(err)
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

	applicationComponents, err := a.loader.GetAllApplicationComponents(a.client, a.context, application)
	if err != nil {
		return nil, err
	}

	snapshot, err := gitops.PrepareSnapshot(a.client, a.context, application, applicationComponents, component, newContainerImage, componentSource)
	if err != nil {
		return nil, err
	}

	if snapshot.Labels == nil {
		snapshot.Labels = make(map[string]string)
	}
	snapshot.Labels[gitops.SnapshotTypeLabel] = gitops.SnapshotComponentType
	snapshot.Labels[gitops.SnapshotComponentLabel] = a.component.Name
	snapshot.Labels[gitops.BuildPipelineRunNameLabel] = pipelineRun.Name
	if pipelineRun.Status.CompletionTime != nil {
		snapshot.Labels[gitops.BuildPipelineRunFinishTimeLabel] = strconv.FormatInt(pipelineRun.Status.CompletionTime.Time.Unix(), 10)
	} else {
		snapshot.Labels[gitops.BuildPipelineRunFinishTimeLabel] = strconv.FormatInt(time.Now().Unix(), 10)
	}

	// Copy PipelineRun PAC annotations/labels from Build to snapshot.
	// Modify the prefix so the PaC controller won't react to PipelineRuns generated from the snapshot.
	_ = metadata.CopyLabelsWithPrefixReplacement(&pipelineRun.ObjectMeta, &snapshot.ObjectMeta, "pipelinesascode.tekton.dev", gitops.PipelinesAsCodePrefix)
	_ = metadata.CopyAnnotationsWithPrefixReplacement(&pipelineRun.ObjectMeta, &snapshot.ObjectMeta, "pipelinesascode.tekton.dev", gitops.PipelinesAsCodePrefix)

	// Copy build labels and annotations prefixed with build.appstudio from Build to Snapshot.
	_ = metadata.CopyLabelsByPrefix(&pipelineRun.ObjectMeta, &snapshot.ObjectMeta, gitops.BuildPipelineRunPrefix)
	_ = metadata.CopyAnnotationsByPrefix(&pipelineRun.ObjectMeta, &snapshot.ObjectMeta, gitops.BuildPipelineRunPrefix)

	return snapshot, nil
}

// isLatestSucceededBuildPipelineRun return true if pipelineRun is the latest succeded pipelineRun
// for the component. Pipeline start timestamp is used for comparison because we care about
// time when pipeline was created.
func (a *Adapter) isLatestSucceededBuildPipelineRun() (bool, error) {

	pipelineStartTime := a.pipelineRun.CreationTimestamp.Time

	pipelineRuns, err := a.getSucceededBuildPipelineRunsForComponent(a.component)
	if err != nil {
		return false, err
	}
	for _, run := range *pipelineRuns {
		if a.pipelineRun.Name == run.Name {
			// it's the same pipeline
			continue
		}
		timestamp := run.CreationTimestamp.Time
		if pipelineStartTime.Before(timestamp) {
			// pipeline is not the latest
			// 1 second is minimal granularity, if both pipelines started at the same second, we cannot decide
			return false, nil
		}
	}
	return true, nil
}

// getSucceededBuildPipelineRunsForComponent returns all  succeeded PipelineRun for the
// associated component. In the case the List operation fails,
// an error will be returned.
func (a *Adapter) getSucceededBuildPipelineRunsForComponent(component *applicationapiv1alpha1.Component) (*[]tektonv1.PipelineRun, error) {
	var succeededPipelineRuns []tektonv1.PipelineRun

	buildPipelineRuns, err := a.loader.GetAllBuildPipelineRunsForComponent(a.client, a.context, component)
	if err != nil {
		return nil, err
	}

	for _, pipelineRun := range *buildPipelineRuns {
		pipelineRun := pipelineRun // G601
		if h.HasPipelineRunSucceeded(&pipelineRun) {
			succeededPipelineRuns = append(succeededPipelineRuns, pipelineRun)
		}
	}
	return &succeededPipelineRuns, nil
}

func (a *Adapter) annotateBuildPipelineRunWithSnapshot(pipelineRun *tektonv1.PipelineRun, snapshot *applicationapiv1alpha1.Snapshot) (*tektonv1.PipelineRun, error) {
	patch := client.MergeFrom(pipelineRun.DeepCopy())

	_ = metadata.SetAnnotation(&pipelineRun.ObjectMeta, tekton.SnapshotNameLabel, snapshot.Name)

	err := a.client.Patch(a.context, pipelineRun, patch)
	if err != nil {
		return pipelineRun, err
	}
	a.logger.LogAuditEvent("Updated build pipelineRun", pipelineRun, h.LogActionUpdate,
		"snapshot.Name", snapshot.Name)
	return pipelineRun, nil
}
