/*
Copyright 2022 Red Hat Inc.

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

package snapshot

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	"github.com/konflux-ci/integration-service/api/v1beta2"
	"github.com/konflux-ci/integration-service/gitops"
	"github.com/konflux-ci/integration-service/helpers"
	"github.com/konflux-ci/integration-service/loader"
	"github.com/konflux-ci/integration-service/tekton"
	"github.com/konflux-ci/operator-toolkit/metadata"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	// prevents stack overflow from recursion. Can be adjusted if users have deeply nested ComponentGroups
	MaxDepth = 15
)

// PrepareSnapshotForPipelineRun prepares the Snapshot for a given PipelineRun,
// component and application. In case the Snapshot can't be created, an error will be returned.
func PrepareSnapshotForPipelineRun(ctx context.Context, adapterClient client.Client, pipelineRun *tektonv1.PipelineRun, componentName string, componentGroup *v1beta2.ComponentGroup, loader loader.ObjectLoader) (*applicationapiv1alpha1.Snapshot, error) {
	logger := helpers.IntegrationLogger{Logger: log.FromContext(ctx)}

	newSnapshotComponent, err := GetSnapshotComponentFromBuildPLR(pipelineRun, componentName, logger)
	if err != nil {
		return nil, err
	}

	var comp applicationapiv1alpha1.Component
	if err := adapterClient.Get(ctx, client.ObjectKey{Namespace: componentGroup.Namespace, Name: componentName}, &comp); err == nil {
		gitops.EnrichBuiltComponentSourceGitContext(&newSnapshotComponent.Source, &comp, newSnapshotComponent.Version)
	}

	snapshot, err := PrepareSnapshot(ctx, adapterClient, componentGroup, newSnapshotComponent, loader, logger)
	if err != nil {
		return nil, err
	}

	prefixes := []string{gitops.BuildPipelineRunPrefix, gitops.TestLabelPrefix, gitops.CustomLabelPrefix, gitops.ReleaseLabelPrefix}
	gitops.CopySnapshotLabelsAndAnnotations(&componentGroup.ObjectMeta, snapshot, componentName, &pipelineRun.ObjectMeta, prefixes, false)

	snapshot.Labels[gitops.BuildPipelineRunNameLabel] = pipelineRun.Name
	if pipelineRun.Status.CompletionTime != nil {
		snapshot.Labels[gitops.BuildPipelineRunFinishTimeLabel] = strconv.FormatInt(pipelineRun.Status.CompletionTime.Unix(), 10)
	} else {
		snapshot.Labels[gitops.BuildPipelineRunFinishTimeLabel] = strconv.FormatInt(time.Now().Unix(), 10)
	}

	if gitops.IsSnapshotCreatedByPACMergeQueueEvent(snapshot) {
		pullRequestNumber := gitops.ExtractPullRequestNumberFromMergeQueueSnapshot(snapshot)
		if pullRequestNumber != "" {
			snapshot.Labels[gitops.PipelineAsCodePullRequestAnnotation] = pullRequestNumber
			snapshot.Annotations[gitops.PipelineAsCodePullRequestAnnotation] = pullRequestNumber
		}
	}

	// Set BuildPipelineRunStartTime annotation with millisecond precision and override snapshot name
	timestampMillis := getPipelineRunStartTimeMillis(pipelineRun)
	// Naming once, at the end
	snapshot.Annotations[gitops.BuildPipelineRunStartTime] = strconv.FormatInt(timestampMillis, 10)
	snapshot.Name = gitops.GenerateSnapshotNameWithTimestamp(componentGroup.Name, timestampMillis)

	// Copy all build PipelineRun results as Snapshot annotations so downstream
	// consumers can access them without loading the PLR (which may be pruned).
	gitops.CopyBuildPipelineRunResultsToSnapshot(pipelineRun, snapshot)

	// Set the integration workflow annotation based on the PipelineRun type
	if tekton.IsPLRCreatedByPACPushEvent(pipelineRun) {
		snapshot.Annotations[gitops.IntegrationWorkflowAnnotation] = gitops.IntegrationWorkflowPushValue
	} else {
		snapshot.Annotations[gitops.IntegrationWorkflowAnnotation] = gitops.IntegrationWorkflowPullRequestValue
	}

	return snapshot, nil
}

// CreateSnapshotWithCollisionHandling attempts to create a snapshot, retrying with a random suffix if collision occurs
func CreateSnapshotWithCollisionHandling(ctx context.Context, client client.Client, pipelineRun *tektonv1.PipelineRun, snapshot *applicationapiv1alpha1.Snapshot, componentGroup v1beta2.ComponentGroup, logger helpers.IntegrationLogger) error {
	originalName := snapshot.Name
	maxRetries := 5

	for attempt := 0; attempt < maxRetries; attempt++ {
		err := client.Create(ctx, snapshot)
		if err == nil {
			// Success
			if attempt > 0 {
				logger.Info("Successfully created snapshot after collision retry",
					"originalName", originalName,
					"finalName", snapshot.Name,
					"attempts", attempt+1)
			}
			return nil
		}

		// Check if it's an "already exists" error
		if !k8sErrors.IsAlreadyExists(err) {
			// Not a collision error, return immediately
			return err
		}

		// Collision detected - generate new name with suffix
		if attempt < maxRetries-1 {
			suffix, suffixErr := generateRandomSuffix()
			if suffixErr != nil {
				logger.Error(suffixErr, "Failed to generate random suffix for snapshot name collision")
				return err // Return original collision error
			}

			// if pipelineRun is nil, this returns time.Now()
			timestampMillis := getPipelineRunStartTimeMillis(pipelineRun)

			// Regenerate name with suffix
			snapshot.Name = gitops.GenerateSnapshotNameWithTimestamp(componentGroup.Name, timestampMillis, suffix)
			logger.Info("Snapshot name collision detected, retrying with suffix",
				"originalName", originalName,
				"newName", snapshot.Name,
				"attempt", attempt+1,
				"maxRetries", maxRetries)
		} else {
			// Max retries reached
			logger.Error(err, "Failed to create snapshot after max retries due to collisions",
				"originalName", originalName,
				"attempts", maxRetries)
			return err
		}
	}

	return fmt.Errorf("failed to create snapshot after %d attempts", maxRetries)
}

// PrepareSnapshot prepares the Snapshot for a given componentGroup, components and the updated component (if any).
// In case the Snapshot can't be created, an error will be returned.
func PrepareSnapshot(ctx context.Context, adapterClient client.Client, componentGroup *v1beta2.ComponentGroup, newSnapshotComponent applicationapiv1alpha1.SnapshotComponent, loader loader.ObjectLoader, logger helpers.IntegrationLogger) (*applicationapiv1alpha1.Snapshot, error) {

	// Get nested snapshotComponents
	snapshotComponentsMap, invalidComponents, err := getAllNestedSnapshotComponents(componentGroup, []string{}, 0, loader, ctx, adapterClient, logger)
	if err != nil {
		return nil, fmt.Errorf("error getting nested snapshot components: %+v", err)
	}
	upsertNewComponentImage(snapshotComponentsMap, invalidComponents, newSnapshotComponent, logger)
	snapshotComponents := flattenSnapshotComponentsMap(snapshotComponentsMap)

	if len(snapshotComponents) == 0 {
		return nil, helpers.NewMissingValidComponentError(joinInvalidComponentNamesAndVersions(invalidComponents))
	}
	snapshot := NewSnapshot(componentGroup, &snapshotComponents)

	// expose the source repo URL and SHA in the snapshot as annotation do we don't have to do lookup in integration tests
	if newSnapshotComponent.Source.GitSource != nil {
		if err := metadata.SetAnnotation(snapshot, gitops.SnapshotGitSourceRepoURLAnnotation, newSnapshotComponent.Source.GitSource.URL); err != nil {
			return nil, fmt.Errorf("failed to set annotation %s: %w", gitops.SnapshotGitSourceRepoURLAnnotation, err)
		}
	}

	// Annotate snapshot with warning about invalid components
	if len(invalidComponents) > 0 {
		if err := metadata.SetAnnotation(snapshot, helpers.CreateSnapshotAnnotationName, fmt.Sprintf("Component(s) '%s' is(are) not included in snapshot due to missing valid containerImage or git source", joinInvalidComponentNamesAndVersions(invalidComponents))); err != nil {
			return nil, fmt.Errorf("failed to set annotation %s: %w", gitops.SnapshotGitSourceRepoURLAnnotation, err)
		}
		logger.Info("Some components were invalid", "invalidComponents", invalidComponents)
	}

	err = ctrl.SetControllerReference(componentGroup, snapshot, adapterClient.Scheme())
	if err != nil {
		return nil, err
	}

	return snapshot, nil
}

// PrepareParentSnapshot creates a snapshot from the parent componentGroup of childSnapshot. The snapshot gathers all GCL images with the parent GCL images overwriting the child GCL images if they conflict.
// It then replaces the GCL images with all new images in the child snapshot. This can be one or many depending on the type of snapshot.
func PrepareParentSnapshot(ctx context.Context, adapterClient client.Client, loader loader.ObjectLoader, logger helpers.IntegrationLogger, componentGroup *v1beta2.ComponentGroup, childSnapshot *applicationapiv1alpha1.Snapshot) (*applicationapiv1alpha1.Snapshot, error) {
	// This should basically work the same as prepareSnapshot EXCEPT that it should use the snapshot's components rather than the component for the corresponding child componentGroup

	snapshotComponentsMap, invalidComponents, err := getAllNestedSnapshotComponents(componentGroup, []string{}, 0, loader, ctx, adapterClient, logger)
	if err != nil {
		return nil, fmt.Errorf("error getting nested snapshot components: %+v", err)
	}
	newSnapshotComponents, err := GetAllNewComponentsInSnapshot(childSnapshot)
	if err != nil {
		return nil, fmt.Errorf("error getting new components in snapshot: %+v", err)
	}
	upsertMultipleComponentImages(snapshotComponentsMap, invalidComponents, newSnapshotComponents, logger)
	snapshotComponents := flattenSnapshotComponentsMap(snapshotComponentsMap)

	if len(snapshotComponents) == 0 {
		return nil, helpers.NewMissingValidComponentError(joinInvalidComponentNamesAndVersions(invalidComponents))
	}
	snapshot := NewSnapshot(componentGroup, &snapshotComponents)

	// expose the source repo URL and SHA in the snapshot as annotation so we don't have to do lookup in integration tests
	// Only done on component snapshots
	if len(newSnapshotComponents) == 1 {
		if newSnapshotComponents[0].Source.GitSource != nil {
			// NOTE: should we skip this annotation for override snaphots?
			if err := metadata.SetAnnotation(snapshot, gitops.SnapshotGitSourceRepoURLAnnotation, newSnapshotComponents[0].Source.GitSource.URL); err != nil {
				return nil, fmt.Errorf("failed to set annotation %s: %w", gitops.SnapshotGitSourceRepoURLAnnotation, err)
			}
		}

	}

	// Annotate snapshot with warning about invalid components
	if len(invalidComponents) > 0 {
		if err := metadata.SetAnnotation(snapshot, helpers.CreateSnapshotAnnotationName, fmt.Sprintf("Component(s) '%s' is(are) not included in snapshot due to missing valid containerImage or git source", joinInvalidComponentNamesAndVersions(invalidComponents))); err != nil {
			return nil, fmt.Errorf("failed to set annotation %s: %w", gitops.SnapshotGitSourceRepoURLAnnotation, err)
		}
		logger.Info("Some components were invalid", "invalidComponents", invalidComponents)
	}

	// Annotate the snapshot with child snapshot info
	if err := metadata.SetAnnotation(snapshot, gitops.ChildSnapshotAnnotation, childSnapshot.Name); err != nil {
		return nil, fmt.Errorf("failed to set annotation %s: %w", gitops.ChildSnapshotAnnotation, err)
	}

	err = ctrl.SetControllerReference(componentGroup, snapshot, adapterClient.Scheme())
	if err != nil {
		return nil, err
	}

	return snapshot, nil
}

// getAllNestedSnapshotComponents gets the GCL SnapshotComponents for a ComponentGroup. It recurses into the nested ComponentGroups to get their SnapshotComponents then builds upward from there.
// As a result, if a parent and child ComponentGroup share a ComponentVersion but have different images, the image in the parent will supersede the one in the child.
func getAllNestedSnapshotComponents(componentGroup *v1beta2.ComponentGroup, usedComponentGroups []string, depth int, loader loader.ObjectLoader, ctx context.Context, adapterClient client.Client, logger helpers.IntegrationLogger) (map[string]applicationapiv1alpha1.SnapshotComponent, map[v1beta2.ComponentState]InvalidComponentReason, error) {
	snapshotComponents := make(map[string]applicationapiv1alpha1.SnapshotComponent)
	invalidComponents := make(map[v1beta2.ComponentState]InvalidComponentReason)

	// Cycle detection since validation webhook is at risk of a race condition
	if slices.Contains(usedComponentGroups, componentGroup.Name) {
		usedComponentGroups = append(usedComponentGroups, componentGroup.Name)
		return nil, nil, fmt.Errorf("cycle found in nested componentGroups: %+v", usedComponentGroups)
	}
	usedComponentGroups = append(usedComponentGroups, componentGroup.Name)
	if depth > MaxDepth {
		return nil, nil, fmt.Errorf("nested ComponentGroups exceeded max depth of %d", MaxDepth)
	}

	nestedComponentGroups, err := loader.GetNestedComponentGroupsForComponentGroup(ctx, adapterClient, componentGroup)
	if err != nil {
		return nil, nil, err
	}

	for _, nestedComponentGroup := range nestedComponentGroups {
		valid, invalid, err := getAllNestedSnapshotComponents(&nestedComponentGroup, usedComponentGroups, depth+1, loader, ctx, adapterClient, logger)
		if err != nil {
			return nil, nil, err
		}
		maps.Copy(snapshotComponents, valid)
		maps.Copy(invalidComponents, invalid)
	}

	valid, invalid := GetSnapshotComponentsFromGCL(componentGroup, logger)
	maps.Copy(snapshotComponents, valid) // bottom-up recursion means that parent componentGroup GCL will overwrite duplicates in children
	maps.Copy(invalidComponents, invalid)

	return snapshotComponents, invalidComponents, nil
}

func flattenSnapshotComponentsMap(snapshotComponentsMap map[string]applicationapiv1alpha1.SnapshotComponent) (snapshotComponents []applicationapiv1alpha1.SnapshotComponent) {
	for _, snapshotComponent := range snapshotComponentsMap {
		snapshotComponents = append(snapshotComponents, snapshotComponent)
	}
	return
}

// This prevents race conditions if EnsureGCLAlignedWithSpecComponents runs late
func GetSnapshotComponentsFromGCL(componentGroup *v1beta2.ComponentGroup, logger helpers.IntegrationLogger) (map[string]applicationapiv1alpha1.SnapshotComponent, map[v1beta2.ComponentState]InvalidComponentReason) {
	snapshotComponents := make(map[string]applicationapiv1alpha1.SnapshotComponent)
	invalidComponents := make(map[v1beta2.ComponentState]InvalidComponentReason)

	specComponents := getSpecComponentsAndVersionsMap(componentGroup)
	for _, gclComponent := range componentGroup.Status.GlobalCandidateList {
		name := gclComponent.Name
		version := gclComponent.Version
		image := gclComponent.LastPromotedImage

		// Necessary to prevent race condition if reconciler function has not
		// cleaned up GCL by the time snapshot is created
		specVersions, ok := specComponents[name]
		if !ok || !slices.Contains(specVersions, version) {
			logger.Info("componentVersion was deleted from spec.Components. Will not add to snapshot", "componentGroup", componentGroup.Name, "component.Name", name, "component.Version", version)
			continue
		}

		// If containerImage is empty, we have run into a race condition in
		// which multiple components are being built in close succession.
		// We omit this not-yet-built component from the snapshot rather than
		// including a component that is incomplete.
		if image == "" {
			// skip components that have not been added to GCL yet
			logger.Info("component cannot be added to snapshot for application due to missing containerImage", "component.Name", gclComponent.Name)
			invalidComponents[gclComponent] = InvalidComponentReason{
				ComponentGroup: componentGroup.Name,
				Reason:         "missing containerImage",
			}
			continue
		}
		err := gitops.ValidateImageDigest(image)
		if err != nil {
			logger.Error(err, "component cannot be added to snapshot for ComponentGroup due to invalid digest in containerImage", "component.Name", gclComponent.Name)
			invalidComponents[gclComponent] = InvalidComponentReason{
				ComponentGroup: componentGroup.Name,
				Reason:         "invalid digest in containerImage",
			}
			continue
		}

		// Get ComponentSource for the component which is not built in this pipeline
		componentSource := GetComponentSourceFromGCLComponent(gclComponent)

		snapshotComponents[helpers.GetComponentVersionString(name, version)] = applicationapiv1alpha1.SnapshotComponent{
			Name:           name,
			Version:        version,
			ContainerImage: image,
			Source:         componentSource,
		}
	}
	return snapshotComponents, invalidComponents
}

func getSpecComponentsAndVersionsMap(componentGroup *v1beta2.ComponentGroup) map[string][]string {
	componentVersions := make(map[string][]string)
	for _, component := range componentGroup.Spec.Components {
		if strings.EqualFold(component.Kind, "componentGroup") {
			continue
		}
		componentVersions[component.Name] = append(componentVersions[component.Name], component.ComponentVersion.Name)
	}
	return componentVersions
}

// Adds the updated Component to the list of snapshotComponents that will be added to the snapshot.  If a SnapshotComponent with
// a matching name and version already exists in the snapshotComponents list then it will be replaced with the updated component.
// Otherwise the updated component will be appended to the list.
func upsertNewComponentImage(snapshotComponents map[string]applicationapiv1alpha1.SnapshotComponent, invalidComponents map[v1beta2.ComponentState]InvalidComponentReason, updatedComponent applicationapiv1alpha1.SnapshotComponent, logger helpers.IntegrationLogger) {
	// Before we do anything else, try to validate the digest
	err := gitops.ValidateImageDigest(updatedComponent.ContainerImage)
	if err != nil { // the updated component is invalid
		logger.Error(err, "component cannot be added to snapshot for ComponentGroup due to invalid digest in containerImage", "component.Name", updatedComponent.Name)
		invalidComponents[v1beta2.ComponentState{
			Name:    updatedComponent.Name,
			Version: updatedComponent.Version,
		}] = InvalidComponentReason{
			ComponentGroup: "",
			Reason:         "invalid digest in containerImage",
		}
	} else { // the updated component is valid
		// add to snapshotComponents
		snapshotComponents[helpers.GetComponentVersionString(updatedComponent.Name, updatedComponent.Version)] = updatedComponent
		// remove from invalidComponents if it was there (it may have been added for a missing containerImage earlier)
		maps.DeleteFunc(invalidComponents, func(K v1beta2.ComponentState, V InvalidComponentReason) bool {
			if K.Name == updatedComponent.Name {
				if K.Version == "" || K.Version == updatedComponent.Version {
					return true
				}
			}
			return false
		})
	}
}

// Accepts a dict of SnapshotComponents representing the current GCL. Updates multiple new SnapshotComponents whose ComponentVersion matches existing ComponentVersions. If no matches exist, the new SnapshotComponents are simply added to the dict
func upsertMultipleComponentImages(snapshotComponents map[string]applicationapiv1alpha1.SnapshotComponent, invalidComponents map[v1beta2.ComponentState]InvalidComponentReason, componentsToInsert []applicationapiv1alpha1.SnapshotComponent, logger helpers.IntegrationLogger) {
	for _, component := range componentsToInsert {
		upsertNewComponentImage(snapshotComponents, invalidComponents, component, logger)
	}
}

// NewSnapshot creates a new snapshot based on the supplied ComponentGroup and components
func NewSnapshot(componentGroup *v1beta2.ComponentGroup, snapshotComponents *[]applicationapiv1alpha1.SnapshotComponent) *applicationapiv1alpha1.Snapshot {
	// Use fallback timestamp (current time) - will be overridden in prepareSnapshotForPipelineRun
	// if BuildPipelineRunStartTime is available
	fallbackTimestamp := time.Now().UnixMilli()

	snapshot := &applicationapiv1alpha1.Snapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      gitops.GenerateSnapshotNameWithTimestamp(componentGroup.Name, fallbackTimestamp),
			Namespace: componentGroup.Namespace,
		},
		Spec: applicationapiv1alpha1.SnapshotSpec{
			ComponentGroup: componentGroup.Name,
			Components:     *snapshotComponents,
		},
	}
	return snapshot
}

func GetComponentSourceFromGCLComponent(gclComponent v1beta2.ComponentState) applicationapiv1alpha1.ComponentSource {
	// NOTE: if we need to fall back on data from component CR we can do it in here
	componentSource := applicationapiv1alpha1.ComponentSource{
		ComponentSourceUnion: applicationapiv1alpha1.ComponentSourceUnion{
			GitSource: &applicationapiv1alpha1.GitSource{
				URL:      gclComponent.URL,
				Revision: gclComponent.LastPromotedCommit,
			},
		},
	}
	return componentSource
}

func GetSnapshotComponentFromBuildPLR(pipelineRun *tektonv1.PipelineRun, componentName string, logger helpers.IntegrationLogger) (applicationapiv1alpha1.SnapshotComponent, error) {
	containerImage, err := tekton.GetImagePullSpecFromPipelineRun(pipelineRun)
	if err != nil {
		return applicationapiv1alpha1.SnapshotComponent{}, err
	}
	err = gitops.ValidateImageDigest(containerImage)
	if err != nil {
		logger.Error(err, "component cannot be added to snapshot for ComponentGroup due to invalid digest in containerImage", "component.Name", componentName)
		return applicationapiv1alpha1.SnapshotComponent{}, errors.Join(helpers.NewInvalidImageDigestError(componentName, containerImage), err)
	}
	componentSource, err := tekton.GetComponentSourceFromPipelineRun(pipelineRun)
	if err != nil {
		return applicationapiv1alpha1.SnapshotComponent{}, err
	}
	componentVersion, err := tekton.GetComponentVersionFromPipelineRun(pipelineRun)
	if err != nil {
		return applicationapiv1alpha1.SnapshotComponent{}, err
	}

	return applicationapiv1alpha1.SnapshotComponent{
		Name:           componentName,
		Version:        componentVersion,
		ContainerImage: containerImage,
		Source:         *componentSource,
	}, nil
}

// PrepareTempGroupSnapshot will prepare a temp group snapshot used to check the integration test scenario that should be applied to the group snapshot under that componentGroup
func PrepareTempGroupSnapshot(componentGroup *v1beta2.ComponentGroup, snapshot *applicationapiv1alpha1.Snapshot) *applicationapiv1alpha1.Snapshot {
	tempGroupSnapshot := NewSnapshot(componentGroup, &[]applicationapiv1alpha1.SnapshotComponent{})
	tempGroupSnapshot, _ = gitops.SetAnnotationAndLabelForGroupSnapshot(tempGroupSnapshot, snapshot, []gitops.ComponentSnapshotInfo{})
	return tempGroupSnapshot
}
