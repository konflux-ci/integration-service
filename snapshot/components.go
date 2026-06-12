package snapshot

import (
	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	"github.com/konflux-ci/integration-service/gitops"
	tektonconsts "github.com/konflux-ci/integration-service/tekton/consts"
)

// GetAllnewComponentsInSnapshot returns a slice of SnapshotComponents. Any SnapshotComponents in the Snapshot that did not
// come from the GCL are returned. This is one Component for Component Snapshots, many Components for Group Snapshots, and
// all Components for Override Snapshots
func GetAllNewComponentsInSnapshot(snapshot *applicationapiv1alpha1.Snapshot) []applicationapiv1alpha1.SnapshotComponent {
	if gitops.IsComponentSnapshot(snapshot) {
		component := snapshot.Labels[gitops.SnapshotComponentLabel]
		version := snapshot.Annotations[tektonconsts.PipelineRunComponentVersionAnnotation]
		for _, snapshotComponent := range snapshot.Spec.Components {
			if snapshotComponent.Name == component {
				if version == "" || snapshotComponent.Version == version {
					return []applicationapiv1alpha1.SnapshotComponent{snapshotComponent}
				}
			}
		}
	} else if gitops.IsGroupSnapshot(snapshot) {
		// TODO: fill in code once PR  1549 is merged
		// Parse GroupSnapshotInfoAnnotation
		// Iterate over ComponenntSnapshotInfo
		// Get SnapshotComponents that match each ComponentVersion
	} else if gitops.IsOverrideSnapshot(snapshot) {
		// Since all images will be promoted to the GCL, everything should take priority
		// in parent snapshot creation
		return snapshot.Spec.Components
	}
	return []applicationapiv1alpha1.SnapshotComponent{}
}
