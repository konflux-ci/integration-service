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

package integration

import (
	"context"
	"fmt"
	"github.com/go-logr/logr"
	hasv1alpha1 "github.com/redhat-appstudio/application-service/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/controllers/results"
	"github.com/redhat-appstudio/integration-service/release"
	"github.com/redhat-appstudio/integration-service/tekton"
	releasev1alpha1 "github.com/redhat-appstudio/release-service/api/v1alpha1"
	tektonv1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Adapter holds the objects needed to reconcile a Release.
type Adapter struct {
	pipelineRun *tektonv1beta1.PipelineRun
	component   *hasv1alpha1.Component
	application *hasv1alpha1.Application
	logger      logr.Logger
	client      client.Client
	context     context.Context
}

// finalizerName is the finalizer name to be added to the Releases
const finalizerName string = "appstudio.redhat.com/integration-finalizer"

// NewAdapter creates and returns an Adapter instance.
func NewAdapter(pipelineRun *tektonv1beta1.PipelineRun, component *hasv1alpha1.Component, application *hasv1alpha1.Application, logger logr.Logger, client client.Client,
	context context.Context) *Adapter {
	return &Adapter{
		pipelineRun: pipelineRun,
		component:   component,
		application: application,
		logger:      logger,
		client:      client,
		context:     context,
	}
}

// EnsureApplicationSnapshotExists is an operation that will ensure that an integration ApplicationSnapshot associated
// to the PipelineRun being processed exists. Otherwise, it will create a new integration ApplicationSnapshot.
func (a *Adapter) EnsureApplicationSnapshotExists() (results.OperationResult, error) {
	existingApplicationSnapshot, err := a.findMatchingApplicationSnapshot()
	if err != nil {
		return results.RequeueWithError(err)
	}

	if existingApplicationSnapshot == nil {
		newApplicationSnapshot, err := a.createApplicationSnapshotForPipelineRun()
		if err != nil {
			a.logger.Error(err, "Failed to create ApplicationSnapshot",
				"Application.Name", a.application.Name, "Application.Namespace", a.application.Namespace)
			return results.RequeueOnErrorOrStop(a.updateStatus())
		}
		err = a.client.Create(a.context, newApplicationSnapshot)
		if err != nil {
			a.logger.Error(err, "Failed to create ApplicationSnapshot",
				"Application.Name", a.application.Name, "Application.Namespace", a.application.Namespace)
			return results.RequeueOnErrorOrStop(a.updateStatus())
		}
		a.logger.Info("Created new ApplicationSnapshot",
			"Application.Name", a.application.Name,
			"ApplicationSnapshot.Name", newApplicationSnapshot.Name,
			"ApplicationSnapshot.Spec.Images", newApplicationSnapshot.Spec.Images)

	} else {
		a.logger.Info("Found existing ApplicationSnapshot",
			"Application.Name", a.application.Name,
			"ApplicationSnapshot.Name", existingApplicationSnapshot.Name,
			"ApplicationSnapshot.Spec.Images", existingApplicationSnapshot.Spec.Images)
	}

	return results.ContinueProcessing()
}

// getApplicationSnapshot returns the all ApplicationSnapshots in the Application's namespace nil if it's not found.
// In the case the List operation fails, an error will be returned.
func (a *Adapter) getAllApplicationSnapshots() (*[]releasev1alpha1.ApplicationSnapshot, error) {
	applicationSnapshots := &releasev1alpha1.ApplicationSnapshotList{}
	opts := []client.ListOption{
		client.InNamespace(a.application.Namespace),
		//client.MatchingFields{"spec.application": a.application.Name},  TODO: Add the application to the spec
	}

	err := a.client.List(a.context, applicationSnapshots, opts...)
	if err != nil {
		return nil, err
	}

	return &applicationSnapshots.Items, nil
}

// compareApplicationSnapshots compares two ApplicationSnapshots and returns boolean true if their images match exactly
func (a *Adapter) compareApplicationSnapshots(expectedApplicationSnapshot *releasev1alpha1.ApplicationSnapshot, foundApplicationSnapshot *releasev1alpha1.ApplicationSnapshot) bool {
	allImagesMatch := true
	for _, image1 := range expectedApplicationSnapshot.Spec.Images {
		foundImage := false
		for _, image2 := range foundApplicationSnapshot.Spec.Images {
			if image2 == image1 {
				foundImage = true
			}
		}
		if !foundImage {
			allImagesMatch = false
		}

	}
	return allImagesMatch
}

// findMatchingApplicationSnapshot tries to find the expected ApplicationSnapshot with the same set of images
func (a *Adapter) findMatchingApplicationSnapshot() (*releasev1alpha1.ApplicationSnapshot, error) {
	var allApplicationSnapshots *[]releasev1alpha1.ApplicationSnapshot
	expectedApplicationSnapshot, err := a.createApplicationSnapshotForPipelineRun()
	if err != nil {
		return nil, err
	}
	allApplicationSnapshots, err = a.getAllApplicationSnapshots()
	if err != nil {
		return nil, err
	}
	for _, foundApplicationSnapshot := range *allApplicationSnapshots {
		if a.compareApplicationSnapshots(expectedApplicationSnapshot, &foundApplicationSnapshot) {
			return &foundApplicationSnapshot, nil
		}
	}
	return nil, nil
}

// getAllApplicationComponents loads from the cluster all Components associated with the given Application.
// If the Application doesn't have any Components or this is not found in the cluster, an error will be returned.
func (a *Adapter) getAllApplicationComponents(application *hasv1alpha1.Application) (*[]hasv1alpha1.Component, error) {
	applicationComponents := &hasv1alpha1.ComponentList{}
	opts := []client.ListOption{
		client.InNamespace(application.Namespace),
		client.MatchingFields{"spec.application": application.Name},
	}

	err := a.client.List(a.context, applicationComponents, opts...)
	if err != nil {
		return nil, err
	}

	return &applicationComponents.Items, nil
}

// getAllApplicationReleaseLinks returns the ReleaseLinks used by the application being processed. If matching
// ReleaseLinks are not found, an error will be returned.
func (a *Adapter) getAllApplicationReleaseLinks(ctx context.Context, application *hasv1alpha1.Application) (*[]releasev1alpha1.ReleaseLink, error) {
	releaseLinks := &releasev1alpha1.ReleaseLinkList{}
	opts := []client.ListOption{
		client.InNamespace(application.Namespace),
		client.MatchingFields{"spec.application": application.Name},
	}

	err := a.client.List(ctx, releaseLinks, opts...)
	if err != nil {
		return nil, err
	}

	return &releaseLinks.Items, nil
}

// createApplicationSnapshotForPipelineRun creates the ApplicationSnapshot for a given PipelineRun
func (a *Adapter) createApplicationSnapshotForPipelineRun() (*releasev1alpha1.ApplicationSnapshot, error) {
	applicationComponents, err := a.getAllApplicationComponents(a.application)
	if err != nil {
		return nil, fmt.Errorf("failed to get all Application Components for Application %s", a.application.Name)
	}
	var images []releasev1alpha1.Image

	for _, applicationComponent := range *applicationComponents {
		pullSpec := applicationComponent.Status.ContainerImage
		if applicationComponent.Name == a.component.Name {
			var err error
			pullSpec, err = tekton.GetOutputImage(a.pipelineRun)
			if err != nil {
				return nil, err
			}
		}
		image := releasev1alpha1.Image{
			Component: applicationComponent.Name,
			PullSpec:  pullSpec,
		}
		images = append(images, image)
	}

	applicationSnapshot, err := release.CreateApplicationSnapshot(a.application, &images)
	if err != nil {
		return nil, fmt.Errorf("failed to create an ApplicationSnapshot for Application %s and pipelineRun %s: %v",
			a.application.Name, a.pipelineRun.Name, err)
	}

	return applicationSnapshot, nil
}

// createReleasesForApplicationSnapshot creates Releases for a given Application and ApplicationSnapshot
// If ReleaseLinks are not found, an error will be returned.
func (a *Adapter) createReleasesForApplicationSnapshot(applicationSnapshot *releasev1alpha1.ApplicationSnapshot) (*[]releasev1alpha1.Release, error) {
	var releases []releasev1alpha1.Release
	releaseLinks, err := a.getAllApplicationReleaseLinks(a.context, a.application)
	if err != nil {
		return nil, fmt.Errorf("failed to get all ReleaseLinks for Application %s", a.application.Name)
	}
	for _, releaseLink := range *releaseLinks {
		appRelease, err := release.CreateRelease(applicationSnapshot, &releaseLink)
		if err != nil {
			return nil, fmt.Errorf("failed to create a Release for Application %s and ReleaseLink %s: %v",
				a.application.Name, releaseLink.Name, err)
		}
		err = a.client.Create(a.context, appRelease)
		if err != nil {
			return nil, fmt.Errorf("failed to create a Release for Application %s and ReleaseLink %s: %v",
				a.application.Name, releaseLink.Name, err)
		}
		releases = append(releases, *appRelease)
	}

	return &releases, nil
}

// updateStatus updates the status of the PipelineRun being processed.
func (a *Adapter) updateStatus() error {
	return a.client.Status().Update(a.context, a.pipelineRun)
}
