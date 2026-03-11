package snapshot

import (
	"context"
	"errors"
	"fmt"
	"slices"

	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	"github.com/konflux-ci/integration-service/api/v1beta2"
	"github.com/konflux-ci/integration-service/helpers"
	"github.com/konflux-ci/integration-service/tekton"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// updateGCLForBuildPLR updates global candidate list for component snapshots
func UpdateGCLForBuildPLR(ctx context.Context, client client.Client, componentGroups *[]v1beta2.ComponentGroup, pipelineRun *tektonv1.PipelineRun, componentName string) error {
	containerImage, err := tekton.GetImagePullSpecFromPipelineRun(pipelineRun)
	if err != nil {
		return nil
	}

	componentSource, err := tekton.GetComponentSourceFromPipelineRun(pipelineRun)
	if err != nil {
		return nil
	}

	componentVersion, err := tekton.GetComponentVersionFromPipelineRun(pipelineRun)
	if err != nil {
		return nil
	}

	buildTime := pipelineRun.Status.StartTime

	entry := v1beta2.ComponentState{
		Name:                  componentName,
		Version:               componentVersion,
		URL:                   componentSource.GitSource.URL,
		LastPromotedCommit:    componentSource.GitSource.Revision,
		LastPromotedImage:     containerImage,
		LastPromotedBuildTime: buildTime,
	}

	for _, componentGroup := range *componentGroups {
		err = errors.Join(err, UpdateGCLEntry(ctx, client, &componentGroup, entry))
	}
	return err
}

func UpdateGCLEntry(ctx context.Context, adapterClient client.Client, componentGroup *v1beta2.ComponentGroup, newEntry v1beta2.ComponentState) error {
	log := log.FromContext(ctx)
	patch := client.MergeFrom(componentGroup.DeepCopy())

	// TODO: add mutating webhook for ComponentGroups that adds blank GCL item when component is added to components list and removes existing GCL entry when component is removed from components list
	// Find matching GCL entry for ComponentVersion
	replaced := false
	for i, entry := range componentGroup.Status.GlobalCandidateList {
		if entry.Name == newEntry.Name && entry.Version == newEntry.Version {
			if entry == newEntry {
				// Adapter may successfully update GCL on one ComponentGroup but have to requeue reconciliation because another failed.
				// This check prevents us from promoting the same image over and over
				log.Info("Refusing to update GCL with identical ComponentState", "name", entry.Name, "version", entry.Version, "url", entry.URL, "lastPromotedCommit", entry.LastPromotedCommit, "lastPromotedImage", entry.LastPromotedImage, "lastPromotedBuildTime", entry.LastPromotedBuildTime)
				return nil
			}
			if newEntry.LastPromotedBuildTime.Before(entry.LastPromotedBuildTime) {
				log.Info("Refusing to update the Global Candidate List for ComponentGroup and ComponentVersion. Existing GCL entry was built after new entry", "ComponentGroup", componentGroup.Name, "Component.Name", entry.Name, "Component.Version", entry.Version, "oldEntry.LastPromotedBuildTime", entry.LastPromotedBuildTime, "newEntry.LastPromotedBuildTime", newEntry.LastPromotedBuildTime)
				return nil
			}
			componentGroup.Status.GlobalCandidateList = slices.Replace(componentGroup.Status.GlobalCandidateList, i, i+1, newEntry)
			replaced = true
			break
		}
	}
	if !replaced {
		return fmt.Errorf("could not find ComponentVersion '%s',%s' in ComponentGroup '%s'", newEntry.Name, newEntry.Version, componentGroup.Name)
	}

	// TODO: support optimistic locking
	err := adapterClient.Status().Patch(ctx, componentGroup, patch)
	if err != nil {
		log.Error(err, "Failed to updated GCL entry for ComponentVersion", "componentGroup", componentGroup.Name, "component.Name", newEntry.Name, "component.Version", newEntry.Version)
		return err
	}

	log.Info("Updated Global Candidate List entry for the ComponentVersion", "ComponentGroup", componentGroup.Name, "Component.Name", newEntry.Name, "Component.Version", newEntry.Version, "URL", newEntry.URL, "LastPromotedCommit", newEntry.LastPromotedCommit, "LastPromotedImage", newEntry.LastPromotedImage, "LastPromotedBuildTime", newEntry.LastPromotedBuildTime)
	return nil
}

func UpdateMultipleGCLEntries(ctx context.Context, adapterClient client.Client, componentGroup *v1beta2.ComponentGroup, componentsToUpdate map[string]v1beta2.ComponentState, logger helpers.IntegrationLogger) error {
	// TODO: add optimistic locking
	patch := client.MergeFrom(componentGroup.DeepCopy())
	for i, entry := range componentGroup.Status.GlobalCandidateList {
		entryKey := getComponentVersionString(entry.Name, entry.Version)
		newEntry, ok := componentsToUpdate[entryKey]
		if !ok {
			// if key doesn't exist then we want to retain the original GCL entry
			continue
		}
		newEntry.LastPromotedBuildTime = entry.LastPromotedBuildTime

		componentGroup.Status.GlobalCandidateList = slices.Replace(componentGroup.Status.GlobalCandidateList, i, i+1, newEntry)
	}

	err := adapterClient.Status().Patch(ctx, componentGroup, patch)
	if err != nil {
		logger.Error(err, "Failed to updated GCL entry for ComponentVersion with components", "componentGroup", componentGroup.Name, "snapshotComponents", componentsToUpdate)
		return err
	}
	return nil
}

// Update GCL for all Components in snapshot. If one component fails to update, abort
func UpdateGCLForOverrideSnapshot(ctx context.Context, adapterClient client.Client, componentGroup *v1beta2.ComponentGroup, snapshot *applicationapiv1alpha1.Snapshot, logger helpers.IntegrationLogger) error {
	componentsToAdd := map[string]v1beta2.ComponentState{}
	for _, component := range snapshot.Spec.Components {
		component := component //G601

		//  TODO: can we delete this? the digests are already validated in EnsureOverrideSnapshotValid() and snapshot components are immutable
		//if err := gitops.ValidateImageDigest(component.ContainerImage); err != nil {
		//	// if image is invalid, do not add image to GCL but also do not return
		//	logger.Error(err, "containerImage cannot be updated to component Global Candidate List due to invalid digest in containerImage", "component.Name", component.Name, "component.ContainerImage", component.ContainerImage)
		//	continue
		//}
		// Create ComponentState object
		key := getComponentVersionString(component.Name, component.Version)
		componentsToAdd[key] = snapshotComponentToComponentState(component)
	}

	err := UpdateMultipleGCLEntries(ctx, adapterClient, componentGroup, componentsToAdd, logger)
	if err == nil {
		logger.Info("Updated Global Candidate List with override snapshot", "componentGroup.Name", componentGroup.Name, "componentGroup.Namespace", componentGroup.Namespace, "snapshot.Name", snapshot.Name)
	}

	return err
}
