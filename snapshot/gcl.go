package snapshot

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/konflux-ci/integration-service/api/v1beta2"
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

// TODO: write unit test
func UpdateGCLEntry(ctx context.Context, adapterClient client.Client, componentGroup *v1beta2.ComponentGroup, newEntry v1beta2.ComponentState) error {
	log := log.FromContext(ctx)
	patch := client.MergeFrom(componentGroup.DeepCopy())

	// TODO: add mutating webhook for ComponentGroups that adds blank GCL item when component is added to components list and removes existing GCL entry when component is removed from components list
	// Find matching GCL entry for ComponentVersion
	replaced := false
	for i, entry := range componentGroup.Status.GlobalCandidateList {
		if entry.Name == newEntry.Name && entry.Version == newEntry.Version {
			// TODO: do not update GCL entry of entry.LastPromotedBTime is newer than newEntry.LastPromotedBuildTime
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
		return fmt.Errorf("Could not find ComponentVersion '%s',%s' in ComponentGroup '%s'", newEntry.Name, newEntry.Version, componentGroup.Name)
	}

	err := adapterClient.Status().Patch(ctx, componentGroup, patch)
	if err != nil {
		log.Error(err, "Failed to updated GCL entry for ComponentVersion", "componentGroup", componentGroup.Name, "component.Name", newEntry.Name, "component.Version", newEntry.Version)
		return err
	}

	log.Info("Updated Global Candidate List entry for the ComponentVersion", "ComponentGroup", componentGroup.Name, "Component.Name", newEntry.Name, "Component.Version", newEntry.Version, "URL", newEntry.URL, "LastPromotedCommit", newEntry.LastPromotedCommit, "LastPromotedImage", newEntry.LastPromotedImage, "LastPromotedBuildTime", newEntry.LastPromotedBuildTime)
	return nil
}
