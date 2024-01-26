package main

import (
	"context"

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
