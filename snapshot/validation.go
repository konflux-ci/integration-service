package snapshot

import (
	"context"
	"errors"
	"fmt"

	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	"github.com/konflux-ci/integration-service/api/v1beta2"
	"github.com/konflux-ci/integration-service/gitops"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// ValidateOverrideSnapshotComponentsComponentGroup checks that every component in an override
// snapshot is a known (name, version) member of the given ComponentGroup, has a valid image
// digest, and has git source fields defined. All validation failures are collected and returned
// as a single joined error so the caller receives a complete picture in one pass.
func ValidateOverrideSnapshotComponents(ctx context.Context, snapshot *applicationapiv1alpha1.Snapshot, componentGroup *v1beta2.ComponentGroup) error {
	log := log.FromContext(ctx)

	// Build a set of known (name, version) pairs from the ComponentGroup spec in O(n)
	knownComponents := make(map[string]bool, len(componentGroup.Spec.Components))
	for _, component := range componentGroup.Spec.Components {
		knownComponents[getComponentVersionString(component.Name, component.ComponentVersion.Name)] = true
	}

	var errsForSnapshot error
	for _, snapshotComponent := range snapshot.Spec.Components {
		snapshotComponent := snapshotComponent //G601

		key := getComponentVersionString(snapshotComponent.Name, snapshotComponent.Version)
		if _, found := knownComponents[key]; !found {
			errsForSnapshot = errors.Join(errsForSnapshot, fmt.Errorf("snapshotComponent %s defined in snapshot %s doesn't exist in componentGroup %s/%s", getComponentVersionLogString(snapshotComponent.Name, snapshotComponent.Version), snapshot.Name, componentGroup.Namespace, componentGroup.Name))
		}

		if err := gitops.ValidateImageDigest(snapshotComponent.ContainerImage); err != nil {
			log.Error(err, "containerImage in snapshotComponent has invalid digest", "snapshotComponent.Name", snapshotComponent.Name, "snapshotComponent.ContainerImage", snapshotComponent.ContainerImage)
			errsForSnapshot = errors.Join(errsForSnapshot, err)
		}

		if !gitops.HaveGitSource(snapshotComponent) {
			errsForSnapshot = errors.Join(errsForSnapshot, fmt.Errorf("snapshotComponent %s/%s has no git url/revision fields defined", snapshotComponent.Name, snapshotComponent.Version))
		}
	}

	return errsForSnapshot
}
