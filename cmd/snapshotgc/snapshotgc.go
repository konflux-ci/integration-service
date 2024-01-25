package main

import (
	"context"
	"sort"

	"github.com/go-logr/logr"
	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	releasev1alpha1 "github.com/redhat-appstudio/release-service/api/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	scheme = runtime.NewScheme()
)

func init() {
	utilruntime.Must(applicationapiv1alpha1.AddToScheme(scheme))
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(releasev1alpha1.AddToScheme(scheme))
}

// Stores pointers to resources to which the snapshot is associated
type snapshotData struct {
	environmentBinding applicationapiv1alpha1.SnapshotEnvironmentBinding
	release            releasev1alpha1.Release
}

// Gets a map to allow to tell with direct lookup if a snapshot is associated with
// a release resource
func getSnapshotsForNSReleases(
	cl client.Client,
	snapToData map[string]snapshotData,
	namespace string,
	logger logr.Logger,
) (map[string]snapshotData, error) {
	releases := &releasev1alpha1.ReleaseList{}
	err := cl.List(
		context.Background(),
		releases,
		&client.ListOptions{Namespace: namespace},
	)
	if err != nil {
		logger.Error(err, "Failed to list releases")
		return nil, err
	}

	for _, release := range releases.Items {
		data, ok := snapToData[release.Spec.Snapshot]
		if !ok {
			data = snapshotData{}
		}
		data.release = release
		snapToData[release.Spec.Snapshot] = data
	}
	return snapToData, nil
}

// Gets a map to allow to tell with direct lookup if a snapshot is associated with
// a SnapshotEnvironmentBinding resource
func getSnapshotsForNSBindings(
	cl client.Client,
	snapToData map[string]snapshotData,
	namespace string,
	logger logr.Logger,
) (map[string]snapshotData, error) {
	binds := &applicationapiv1alpha1.SnapshotEnvironmentBindingList{}
	err := cl.List(
		context.Background(),
		binds,
		&client.ListOptions{Namespace: namespace},
	)
	if err != nil {
		logger.Error(err, "Failed to list bindings")
		return nil, err
	}

	for _, bind := range binds.Items {
		data, ok := snapToData[bind.Spec.Snapshot]
		if !ok {
			data = snapshotData{}
		}
		data.environmentBinding = bind
		snapToData[bind.Spec.Snapshot] = data
	}
	return snapToData, nil
}

// Gets all namespace snapshots that aren't associated with a release/binding
func getUnassociatedNSSnapshots(
	cl client.Client,
	snapToData map[string]snapshotData,
	namespace string,
	logger logr.Logger,
) ([]applicationapiv1alpha1.Snapshot, error) {
	snaps := &applicationapiv1alpha1.SnapshotList{}
	err := cl.List(
		context.Background(),
		snaps,
		&client.ListOptions{Namespace: namespace},
	)
	if err != nil {
		logger.Error(err, "Failed to list snapshots")
		return nil, err
	}

	var unAssociatedSnaps []applicationapiv1alpha1.Snapshot

	for _, snap := range snaps.Items {
		if _, found := snapToData[snap.Name]; found {
			logger.V(1).Info(
				"Skipping snapshot as it's associated with release/binding",
				"snapshot.name",
				snap.Name,
			)
			continue
		}
		unAssociatedSnaps = append(unAssociatedSnaps, snap)
	}

	return unAssociatedSnaps, nil
}

// Keep a certain amount of pr/non-pr snapshots
func getSnapshotsForRemoval(
	cl client.Client,
	snapshots []applicationapiv1alpha1.Snapshot,
	prSnapshotsToKeep int,
	nonPrSnapshotsToKeep int,
	logger logr.Logger,
) []applicationapiv1alpha1.Snapshot {
	sort.Slice(snapshots, func(i, j int) bool {
		// sorting in reverse order, so we keep the latest snapshots
		return snapshots[j].CreationTimestamp.Before(&snapshots[i].CreationTimestamp)
	})

	shortList := []applicationapiv1alpha1.Snapshot{}
	keptPrSnaps := 0
	keptNonPrSnaps := 0

	for _, snap := range snapshots {
		label, found := snap.GetLabels()["pac.test.appstudio.openshift.io/event-type"]
		if found && label == "pull_request" && keptPrSnaps < prSnapshotsToKeep {
			// if pr, and we did not keep enough PR snapshots -> discard from candidates
			logger.V(1).Info(
				"Skipping PR candidate snapshot",
				"snapshot.name", snap.Name,
				"pr-snapshot-kept", keptPrSnaps+1,
			)
			keptPrSnaps++
			continue
		} else if (!found || label != "pull_request") && keptNonPrSnaps < nonPrSnapshotsToKeep {
			// same for non-pr
			logger.V(1).Info(
				"Skipping non-PR candidate snapshot",
				"snapshot.name", snap.Name,
				"non-pr-snapshot-kept", keptNonPrSnaps+1,
			)
			keptNonPrSnaps++
			continue
		}
		logger.V(1).Info("Adding candidate", "snapshot.name", snap.Name)
		shortList = append(shortList, snap)
	}
	return shortList
}

// Delete snapshots determined to be garbage-collected
func deleteSnapshots(
	cl client.Client,
	snapshots []applicationapiv1alpha1.Snapshot,
	logger logr.Logger,
) {

	for _, snap := range snapshots {
		err := cl.Delete(context.Background(), &snap)
		if err != nil {
			logger.Error(err, "Failed to delete snapshot")
		}
	}
}
