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
	"fmt"

	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	"github.com/konflux-ci/integration-service/gitops"
	"github.com/konflux-ci/integration-service/helpers"
	tektonconsts "github.com/konflux-ci/integration-service/tekton/consts"
)

// GetAllnewComponentsInSnapshot returns a slice of SnapshotComponents. Any SnapshotComponents in the Snapshot that did not
// come from the GCL are returned. This is one Component for Component Snapshots, many Components for Group Snapshots, and
// all Components for Override Snapshots
func GetAllNewComponentsInSnapshot(snapshot *applicationapiv1alpha1.Snapshot) ([]applicationapiv1alpha1.SnapshotComponent, error) {
	snapshotComponents := []applicationapiv1alpha1.SnapshotComponent{}
	if gitops.IsComponentSnapshot(snapshot) {
		component := snapshot.Labels[gitops.SnapshotComponentLabel]
		version := snapshot.Annotations[tektonconsts.PipelineRunComponentVersionAnnotation]
		updatedComponent, err := getComponentFromSnapshotComponents(snapshot, component, version)
		if err != nil {
			return snapshotComponents, err
		}
		snapshotComponents = append(snapshotComponents, updatedComponent)
	} else if gitops.IsGroupSnapshot(snapshot) {
		var componentSnapshotInfos []*gitops.ComponentSnapshotInfo
		var err error
		if componentSnapshotInfoString, ok := snapshot.Annotations[gitops.GroupSnapshotInfoAnnotation]; ok {
			componentSnapshotInfos, err = gitops.UnmarshalJSON([]byte(componentSnapshotInfoString))
			if err != nil {
				return snapshotComponents, fmt.Errorf("failed to unmarshal JSON string: %+v", err)
			}
		}
		for _, info := range componentSnapshotInfos {
			updatedComponent, err := getComponentFromSnapshotComponents(snapshot, info.Component, info.Version)
			if err != nil {
				return snapshotComponents, fmt.Errorf("could not get component snapshot %s/%s referenced in group snapshot %s/%s", info.Namespace, info.Snapshot, info.Namespace, info.Component)
			}
			snapshotComponents = append(snapshotComponents, updatedComponent)
		}
	} else if gitops.IsOverrideSnapshot(snapshot) {
		// Since all images will be promoted to the GCL, everything should take priority
		// in parent snapshot creation
		snapshotComponents = snapshot.Spec.Components
	}
	return snapshotComponents, nil
}

func getComponentFromSnapshotComponents(snapshot *applicationapiv1alpha1.Snapshot, name, version string) (applicationapiv1alpha1.SnapshotComponent, error) {
	for _, snapshotComponent := range snapshot.Spec.Components {
		if snapshotComponent.Name == name {
			if version == "" || snapshotComponent.Version == "" || snapshotComponent.Version == version {
				return snapshotComponent, nil
			}
		}
	}
	return applicationapiv1alpha1.SnapshotComponent{}, fmt.Errorf("the snapshot '%s' does not contain the updated component/version '%s'", snapshot.Name, helpers.GetComponentVersionLogString(name, version))
}
