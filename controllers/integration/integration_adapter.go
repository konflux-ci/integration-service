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
	"github.com/redhat-appstudio/integration-service/tekton"
	releasev1alpha1 "github.com/redhat-appstudio/release-service/api/v1alpha1"
	tektonv1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
	"time"
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

	if existingApplicationSnapshot != nil {
		a.logger.Info("Found existing ApplicationSnapshot",
			"Application.Name", a.application.Name,
			"ApplicationSnapshot.Name", existingApplicationSnapshot.Name,
			"ApplicationSnapshot.Spec.Images", existingApplicationSnapshot.Spec.Images)
		return results.ContinueProcessing()
	}

	newApplicationSnapshot, err := a.createApplicationSnapshotForPipelineRun(a.pipelineRun, a.component, a.application)
	if err != nil {
		a.logger.Error(err, "Failed to create ApplicationSnapshot",
			"Application.Name", a.application.Name, "Application.Namespace", a.application.Namespace)
		return results.RequeueOnErrorOrStop(a.updateStatus())
	}

	a.logger.Info("Created new ApplicationSnapshot",
		"Application.Name", a.application.Name,
		"ApplicationSnapshot.Name", newApplicationSnapshot.Name,
		"ApplicationSnapshot.Spec.Images", newApplicationSnapshot.Spec.Images)

	return results.ContinueProcessing()
}

// EnsureAllReleasesExist is an operation that will ensure that all integration Releases associated
// to the ApplicationSnapshot and the Application's ReleaseLinks exist.
// Otherwise, it will create new Releases for each ReleaseLink.
func (a *Adapter) EnsureAllReleasesExist() (results.OperationResult, error) {
	existingApplicationSnapshot, err := a.findMatchingApplicationSnapshot()
	if err != nil {
		return results.RequeueWithError(err)
	} else if existingApplicationSnapshot == nil {
		return results.RequeueAfter(time.Second*5, err)
	}

	releaseLinks, err := a.getAllApplicationReleaseLinks(a.application)
	if err != nil {
		a.logger.Error(err, "Failed to get all ReleaseLinks",
			"Application.Name", a.application.Name,
			"Application.Namespace", a.application.Namespace)
		return results.RequeueOnErrorOrStop(a.updateStatus())
	}

	err = a.createMissingReleasesForReleaseLinks(releaseLinks, existingApplicationSnapshot)
	if err != nil {
		a.logger.Error(err, "Failed to create new Releases for",
			"ApplicationSnapshot.Name", existingApplicationSnapshot.Name,
			"ApplicationSnapshot.Namespace", existingApplicationSnapshot.Namespace)
		return results.RequeueOnErrorOrStop(a.updateStatus())
	}

	return results.ContinueProcessing()
}

// getApplicationSnapshot returns the all ApplicationSnapshots in the Application's namespace nil if it's not found.
// In the case the List operation fails, an error will be returned.
func (a *Adapter) getAllApplicationSnapshots() (*[]releasev1alpha1.ApplicationSnapshot, error) {
	applicationSnapshots := &releasev1alpha1.ApplicationSnapshotList{}
	opts := []client.ListOption{
		client.InNamespace(a.application.Namespace),
		client.MatchingFields{"spec.application": a.application.Name},
	}

	err := a.client.List(a.context, applicationSnapshots, opts...)
	if err != nil {
		return nil, err
	}

	return &applicationSnapshots.Items, nil
}

// compareApplicationSnapshots compares two ApplicationSnapshots and returns boolean true if their images match exactly.
func (a *Adapter) compareApplicationSnapshots(expectedApplicationSnapshot *releasev1alpha1.ApplicationSnapshot, foundApplicationSnapshot *releasev1alpha1.ApplicationSnapshot) bool {
	snapshotsHaveSameNumberOfImages :=
		len(expectedApplicationSnapshot.Spec.Images) == len(foundApplicationSnapshot.Spec.Images)
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
	return snapshotsHaveSameNumberOfImages && allImagesMatch
}

// findMatchingApplicationSnapshot tries to find the expected ApplicationSnapshot with the same set of images.
func (a *Adapter) findMatchingApplicationSnapshot() (*releasev1alpha1.ApplicationSnapshot, error) {
	var allApplicationSnapshots *[]releasev1alpha1.ApplicationSnapshot
	expectedApplicationSnapshot, err := a.prepareApplicationSnapshotForPipelineRun(a.pipelineRun, a.component, a.application)
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
func (a *Adapter) getAllApplicationReleaseLinks(application *hasv1alpha1.Application) (*[]releasev1alpha1.ReleaseLink, error) {
	releaseLinks := &releasev1alpha1.ReleaseLinkList{}
	opts := []client.ListOption{
		client.InNamespace(application.Namespace),
		client.MatchingFields{"spec.application": application.Name},
	}

	err := a.client.List(a.context, releaseLinks, opts...)
	if err != nil {
		return nil, err
	}

	return &releaseLinks.Items, nil
}

// getApplicationSnapshot returns the all ApplicationSnapshots in the Application's namespace nil if it's not found.
// In the case the List operation fails, an error will be returned.
func (a *Adapter) getReleasesWithApplicationSnapshot(applicationSnapshot *releasev1alpha1.ApplicationSnapshot) (*[]releasev1alpha1.Release, error) {
	releases := &releasev1alpha1.ReleaseList{}
	opts := []client.ListOption{
		client.InNamespace(applicationSnapshot.Namespace),
		client.MatchingFields{"spec.applicationSnapshot": applicationSnapshot.Name},
	}

	err := a.client.List(a.context, releases, opts...)
	if err != nil {
		return nil, err
	}

	return &releases.Items, nil
}

// getImagePullSpecFromPipelineRun gets the full image pullspec from the given build PipelineRun,
// In case the Image pullspec can't be can't be composed, an error will be returned.
func (a *Adapter) getImagePullSpecFromPipelineRun(pipelineRun *tektonv1beta1.PipelineRun) (string, error) {
	var err error
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

// prepareApplicationSnapshotForPipelineRun prepares the ApplicationSnapshot for a given PipelineRun,
// component and application. In case the ApplicationSnapshot can't be created, an error will be returned.
func (a *Adapter) prepareApplicationSnapshotForPipelineRun(pipelineRun *tektonv1beta1.PipelineRun,
	component *hasv1alpha1.Component, application *hasv1alpha1.Application) (*releasev1alpha1.ApplicationSnapshot, error) {
	applicationComponents, err := a.getAllApplicationComponents(application)
	if err != nil {
		return nil, fmt.Errorf("failed to get all Application Components for Application %s", a.application.Name)
	}
	var images []releasev1alpha1.Image

	for _, applicationComponent := range *applicationComponents {
		pullSpec := applicationComponent.Status.ContainerImage
		if applicationComponent.Name == component.Name {
			pullSpec, err = a.getImagePullSpecFromPipelineRun(pipelineRun)
			if err != nil {
				return nil, err
			}
		}
		images = append(images, releasev1alpha1.Image{
			Component: applicationComponent.Name,
			PullSpec:  pullSpec,
		})
	}

	applicationSnapshot := &releasev1alpha1.ApplicationSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: application.Name + "-",
			Namespace:    application.Namespace,
		},
		Spec: releasev1alpha1.ApplicationSnapshotSpec{
			Application: application.Name,
			Images:      images,
		},
	}
	return applicationSnapshot, nil
}

// createApplicationSnapshotForPipelineRun creates the ApplicationSnapshot for a given PipelineRun
// In case the ApplicationSnapshot can't be created, an error will be returned.
func (a *Adapter) createApplicationSnapshotForPipelineRun(pipelineRun *tektonv1beta1.PipelineRun,
	component *hasv1alpha1.Component, application *hasv1alpha1.Application) (*releasev1alpha1.ApplicationSnapshot, error) {
	applicationSnapshot, err := a.prepareApplicationSnapshotForPipelineRun(pipelineRun, component, application)
	if err != nil {
		return nil, err
	}

	err = a.client.Create(a.context, applicationSnapshot)
	if err != nil {
		return nil, err
	}

	return applicationSnapshot, nil
}

// createReleaseForReleaseLink creates the Release for a given ReleaseLink.
// In case the Release can't be created, an error will be returned.
func (a *Adapter) createReleaseForReleaseLink(releaseLink *releasev1alpha1.ReleaseLink, applicationSnapshot *releasev1alpha1.ApplicationSnapshot) (*releasev1alpha1.Release, error) {
	newRelease := &releasev1alpha1.Release{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: applicationSnapshot.Name + "-",
			Namespace:    applicationSnapshot.Namespace,
		},
		Spec: releasev1alpha1.ReleaseSpec{
			ApplicationSnapshot: applicationSnapshot.Name,
			ReleaseLink:         releaseLink.Name,
		},
	}
	err := a.client.Create(a.context, newRelease)
	if err != nil {
		return nil, err
	}

	return newRelease, nil
}

// createReleaseForReleaseLink checks if there's existing Releases for a given list of ReleaseLinks and creates new ones
// if they are missing. In case the Releases can't be created, an error will be returned.
func (a *Adapter) createMissingReleasesForReleaseLinks(releaseLinks *[]releasev1alpha1.ReleaseLink, applicationSnapshot *releasev1alpha1.ApplicationSnapshot) error {
	releases, err := a.getReleasesWithApplicationSnapshot(applicationSnapshot)
	if err != nil {
		return err
	}

	for _, releaseLink := range *releaseLinks {
		var existingRelease *releasev1alpha1.Release = nil
		for _, snapshotRelease := range *releases {
			if snapshotRelease.Spec.ReleaseLink == releaseLink.Name {
				existingRelease = &snapshotRelease
			}
		}
		if existingRelease != nil {
			a.logger.Info("Found existing Release",
				"ApplicationSnapshot.Name", applicationSnapshot.Name,
				"ReleaseLink.Name", releaseLink.Name,
				"Release.Name", existingRelease.Name)
		} else {
			newRelease, err := a.createReleaseForReleaseLink(&releaseLink, applicationSnapshot)
			if err != nil {
				return err
			}
			a.logger.Info("Created new Release",
				"Application.Name", a.application.Name,
				"ReleaseLink.Name", releaseLink.Name,
				"Release.Name", newRelease.Name)
		}
	}
	return nil
}

// updateStatus updates the status of the PipelineRun being processed.
func (a *Adapter) updateStatus() error {
	return a.client.Status().Update(a.context, a.pipelineRun)
}
