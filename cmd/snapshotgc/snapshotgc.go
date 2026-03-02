package main

import (
	"context"
	"flag"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"
	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	"github.com/konflux-ci/integration-service/gitops"
	"github.com/konflux-ci/operator-toolkit/metadata"
	releasev1alpha1 "github.com/konflux-ci/release-service/api/v1alpha1"
	zap2 "go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/selection"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	scheme = runtime.NewScheme()
)

const (
	// Annotation that can be manually added by users to preven the deletion of a snapshot, they can also
	// define the TTL of snapshot by providing time in following format: 10h28m9s
	// in case the format is not correct the snapshot is considered to deletion
	KeepSnapshotAnnotation = "test.appstudio.openshift.io/keep-snapshot"
	// PRStatusAnnotation contains the status of the PR, it is marked as "merged" when the push build pipelinerun is triggered
	PRStatusAnnotation = "test.appstudio.openshift.io/pr-status"
	// PRStatusMerged indicates that the PR has been merged
	PRStatusMerged = "merged"
	// PRGroupCreationAnnotation is the annotation used to indicate whether the group snapshot has been created for the PR snapshot or not, or will be created
	PRGroupCreationAnnotation = "test.appstudio.openshift.io/create-groupsnapshot-status"
)

func init() {
	utilruntime.Must(applicationapiv1alpha1.AddToScheme(scheme))
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(releasev1alpha1.AddToScheme(scheme))
}

// Stores pointers to resources to which the snapshot is associated
type snapshotData struct {
	release releasev1alpha1.Release
}

// Iterates tenant namespaces and garbage-collect their snapshots
func garbageCollectSnapshots(
	cl client.Client,
	logger logr.Logger,
	prSnapshotsToKeep, nonPrSnapshotsToKeep, minSnapShotsToKeepPerComponent int,
) error {
	namespaces, err := getTenantNamespaces(cl, logger)
	if err != nil {
		return err
	}

	logger.V(1).Info("Snapshot garbage collection started...")
	for _, ns := range namespaces {
		logger.V(1).Info("Processing namespace", "namespace", ns.Name)

		// Use local copies per namespace to prevent cumulative decrease across namespaces
		localPrSnapshotsToKeep := prSnapshotsToKeep
		localNonPrSnapshotsToKeep := nonPrSnapshotsToKeep

		snapToData := make(map[string]snapshotData)
		snapToData, err = getSnapshotsForNSReleases(cl, snapToData, ns.Name, logger)
		if err != nil {
			logger.Error(
				err,
				"Failed getting releases associated with snapshots. Skipping namespace",
				"namespace",
				ns.Name,
			)
			continue
		}

		var candidates []applicationapiv1alpha1.Snapshot
		candidates, err = getUnassociatedNSSnapshots(cl, snapToData, ns.Name, logger)
		if err != nil {
			logger.Error(
				err,
				"Failed getting unassociated snapshots. Skipping namespace",
				"namespace", ns.Name,
			)
			continue
		}
		logger.V(1).Info(
			"Found unassociated snapshots not associated with releases",
			"namespace", ns.Name,
			"count of unassociated snapshots", len(candidates),
		)

		candidates, keptPrSnapshots, keptNonPrSnapshots := filterSnapshotsWithKeepSnapshotAnnotation(candidates, logger)
		localPrSnapshotsToKeep -= keptPrSnapshots
		// Both the Snapshots associated with Releases and ones that have been marked with
		// the keep snapshot annotation count against the non-PR limit
		localNonPrSnapshotsToKeep -= keptNonPrSnapshots
		localNonPrSnapshotsToKeep -= len(snapToData)

		candidates = getSnapshotsForRemoval(
			cl, candidates, localPrSnapshotsToKeep, localNonPrSnapshotsToKeep, minSnapShotsToKeepPerComponent, logger,
		)

		logger.V(1).Info(
			"Deleting snapshots",
			"namespace", ns.Name,
			"count of snapshots to delete", len(candidates),
		)
		deleteSnapshots(cl, candidates, logger)
		logger.V(1).Info("Finished processing namespace", "namespace", ns.Name)
	}
	return nil
}

// Gets all tenant namespaces
func getTenantNamespaces(
	cl client.Client, logger logr.Logger) ([]core.Namespace, error) {

	// First get the toolchain-provisioned tenant namespaces
	req, _ := labels.NewRequirement(
		"toolchain.dev.openshift.com/type", selection.In, []string{"tenant"},
	)
	selector := labels.NewSelector().Add(*req)
	toolChainNamespaceList := &core.NamespaceList{}
	err := cl.List(
		context.Background(),
		toolChainNamespaceList,
		&client.ListOptions{LabelSelector: selector},
	)
	if err != nil {
		logger.Error(err, "Failed listing namespaces")
		return nil, err
	}

	// Then get the Konflux user namespaces
	req, _ = labels.NewRequirement(
		"konflux.ci/type", selection.In, []string{"user"},
	)
	selector = labels.NewSelector().Add(*req)
	konfluxUserNamespaceList := &core.NamespaceList{}
	err = cl.List(
		context.Background(),
		konfluxUserNamespaceList,
		&client.ListOptions{LabelSelector: selector},
	)
	if err != nil {
		logger.Error(err, "Failed listing namespaces")
		return nil, err
	}

	namespaces := append(toolChainNamespaceList.Items, konfluxUserNamespaceList.Items...)

	// Finally get the new format Konflux tenant namespaces
	req, _ = labels.NewRequirement(
		"konflux-ci.dev/type", selection.In, []string{"tenant"},
	)
	selector = labels.NewSelector().Add(*req)
	konfluxTenantNamespaceList := &core.NamespaceList{}
	err = cl.List(
		context.Background(),
		konfluxTenantNamespaceList,
		&client.ListOptions{LabelSelector: selector},
	)
	if err != nil {
		logger.Error(err, "Failed listing namespaces")
		return nil, err
	}

	namespaces = append(namespaces, konfluxTenantNamespaceList.Items...)

	return namespaces, nil
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

// Gets all namespace snapshots that aren't associated with a release
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
				"Skipping snapshot as it's associated with release",
				"namespace", snap.Namespace,
				"snapshot.name", snap.Name,
			)
			continue
		}
		unAssociatedSnaps = append(unAssociatedSnaps, snap)
	}

	return unAssociatedSnaps, nil
}

// extractCancelledOrMergedPRSnapshots gets all namespace PR snapshots which were marked as cancelled or its PR is merged, and are not annotated with the keep snapshot annotation
func extractCancelledOrMergedPRSnapshots(snapshots []applicationapiv1alpha1.Snapshot) []string {
	var cancelledOrMergedPRSnapshots []string

	for _, snap := range snapshots {
		if !isNonPrSnapshot(snap) &&
			(gitops.IsSnapshotMarkedAsCanceled(&snap) || metadata.HasAnnotationWithValue(&snap, PRStatusAnnotation, PRStatusMerged)) &&
			!metadata.HasAnnotationWithValue(&snap, "test.appstudio.openshift.io/keep-snapshot", "true") {
			cancelledOrMergedPRSnapshots = append(cancelledOrMergedPRSnapshots, snap.Name)
		}
	}

	return cancelledOrMergedPRSnapshots
}

// Returns true if the snapshot's age is longer than specific (in seconds)
func isLongerThanSpecificTime(snap applicationapiv1alpha1.Snapshot, specificTime int64) bool {
	creationTime := snap.GetCreationTimestamp().Time
	if creationTime.IsZero() {
		return false
	}

	currentTime := metav1.Now().Time
	elapsedTime := currentTime.Sub(creationTime).Seconds()

	return int64(elapsedTime) > specificTime
}

// Returns true if snapshot is a push snapshot, override snapshot, or if the
// event-type annnotation for the snapshot is not set.  Returns false otherwise
func isNonPrSnapshot(
	snapshot applicationapiv1alpha1.Snapshot,
) bool {
	label, found := snapshot.GetLabels()["pac.test.appstudio.openshift.io/event-type"]
	isOverrideSnapshot := metadata.HasLabelWithValue(&snapshot, "test.appstudio.openshift.io/type", "override")
	if !found || label == "push" || label == "Push" || isOverrideSnapshot {
		return true
	}
	return false
}

// Returns true if snapshots TTL has expired, for TTL snapshot annotation only
// Returns false otherwise
func isPastTTL(
	snapshot applicationapiv1alpha1.Snapshot,
	logger logr.Logger,
) bool {
	currentTime := metav1.Now()
	creationTime := snapshot.GetCreationTimestamp().Time
	if creationTime.IsZero() {
		return false
	}
	label, found := snapshot.GetAnnotations()["test.appstudio.openshift.io/keep-snapshot"]
	ttl, _ := time.ParseDuration(label)
	if ttl.String() == "0s" {
		logger.V(1).Info(
			"Keep-snapshot annotation has invalid value, snapshot will be garbage collected if it is pull-request type.",
			"namespace", snapshot.Namespace,
			"snapshot.name", snapshot.Name,
		)
		return true
	} else {
		// calculate diff between creationTimestamp and currentTime()
		diff := currentTime.Sub(creationTime)
		if diff >= ttl && found {
			return true
		}
	}
	return false
}

// Removes snapshots with the KeepSnapshotAnnotation from the list of candidate
// snapshots.  Returns a new slice without the reserved snapshots plus the
// number of PR and non-pr snapshots that were reserved
// also checks if the TTL format for keep-snapshot has been provided and remove snapshot
// in case it lives past TTL
func filterSnapshotsWithKeepSnapshotAnnotation(
	snapshots []applicationapiv1alpha1.Snapshot,
	logger logr.Logger,
) ([]applicationapiv1alpha1.Snapshot, int, int) {
	nonReservedSnapshots := make([]applicationapiv1alpha1.Snapshot, 0, len(snapshots))
	keptNonPrSnapshots := 0
	keptPrSnapshots := 0
	for _, snap := range snapshots {
		if metadata.HasAnnotationWithValue(&snap, "test.appstudio.openshift.io/keep-snapshot", "true") || (metadata.HasAnnotation(&snap, "test.appstudio.openshift.io/keep-snapshot") && !isPastTTL(snap, logger)) {
			if isNonPrSnapshot(snap) {
				keptNonPrSnapshots++
			} else {
				keptPrSnapshots++
			}
		} else {
			nonReservedSnapshots = append(nonReservedSnapshots, snap)
		}
	}
	return nonReservedSnapshots, keptPrSnapshots, keptNonPrSnapshots
}

// Keep a certain amount of pr/non-pr snapshots
func getSnapshotsForRemoval(
	cl client.Client,
	snapshots []applicationapiv1alpha1.Snapshot,
	prSnapshotsToKeep int,
	nonPrSnapshotsToKeep int,
	minSnapShotsToKeepPerComponent int,
	logger logr.Logger,
) []applicationapiv1alpha1.Snapshot {
	sort.Slice(snapshots, func(i, j int) bool {
		// sorting in reverse order, so we keep the latest snapshots
		return snapshots[j].CreationTimestamp.Before(&snapshots[i].CreationTimestamp)
	})

	// preservedPerComponent is a map to track if a snapshot is preserved from garbage collection
	preservedPerComponent := make(map[string]bool)
	shortList := []applicationapiv1alpha1.Snapshot{}
	keptPrSnaps := 0
	keptNonPrSnaps := 0

	// Group push snapshots by component
	componentToPushSnapshots := make(map[string][]applicationapiv1alpha1.Snapshot)
	for _, snap := range snapshots {

		if !isNonPrSnapshot(snap) {
			continue
		}

		if !metadata.HasLabelWithValue(&snap, "test.appstudio.openshift.io/type", "component") {
			continue
		}

		componentName := snap.GetLabels()["appstudio.openshift.io/component"]

		if componentName == "" {
			logger.V(1).Info(
				"Skipping snapshot as component label is empty",
				"namespace", snap.Namespace,
				"snapshot.name", snap.Name,
			)
			continue
		}

		componentToPushSnapshots[componentName] = append(componentToPushSnapshots[componentName], snap)

	}

	// For each component, mark the latest N push snapshots as preserved
	for componentName, compSnapshots := range componentToPushSnapshots {
		// sorting in reverse order, so we keep the latest snapshots
		sort.Slice(compSnapshots, func(i, j int) bool {
			return compSnapshots[j].CreationTimestamp.Before(&compSnapshots[i].CreationTimestamp)
		})

		keepCount := minSnapShotsToKeepPerComponent
		if len(compSnapshots) < keepCount {
			keepCount = len(compSnapshots)
		}

		for i := 0; i < keepCount; i++ {
			preservedPerComponent[compSnapshots[i].Name] = true
			logger.V(1).Info(
				"Preserving push snapshot per component minimum",
				"component", componentName,
				"snapshot.name", compSnapshots[i].Name,
			)
		}
	}

	// First extract canceled PR Snapshots since these should be deleted first
	// as they were superseded by newer Snapshots for that same PR
	cancelledOrMergedPRSnapshots := extractCancelledOrMergedPRSnapshots(snapshots)

	for _, snap := range snapshots {
		snap := snap
		if preservedPerComponent[snap.Name] {
			logger.V(1).Info(
				"Skipping snapshot (preserved per component minimum kept count)",
				"namespace", snap.Namespace,
				"snapshot.name", snap.Name,
				"min-snapshots-to-keep-per-component", minSnapShotsToKeepPerComponent,
			)

			// count the snapshot towards the total number of snapshots to keep / global limits
			if isNonPrSnapshot(snap) {
				keptNonPrSnaps++
			} else {
				keptPrSnaps++
			}
			continue
		}

		// override snapshot does not have the event-type label, but we still want to add it to the cleanup list
		if isNonPrSnapshot(snap) {
			if keptNonPrSnaps < nonPrSnapshotsToKeep {
				logger.V(1).Info(
					"Skipping non-PR candidate snapshot",
					"namespace", snap.Namespace,
					"snapshot.name", snap.Name,
					"non-pr-snapshot-kept", keptNonPrSnaps+1,
					"non-pr-snapshots-to-keep", nonPrSnapshotsToKeep,
				)
				keptNonPrSnaps++
			} else {
				logger.V(1).Info(
					"Adding non-PR candidate snapshot",
					"namespace", snap.Namespace,
					"snapshot.name", snap.Name,
					"non-pr-snapshot-kept", keptNonPrSnaps,
					"non-pr-snapshots-to-keep", nonPrSnapshotsToKeep,
				)
				shortList = append(shortList, snap)
			}
		} else {
			if keptPrSnaps < prSnapshotsToKeep && !slices.Contains(cancelledOrMergedPRSnapshots, snap.Name) {
				logger.V(1).Info(
					"Skipping PR candidate snapshot",
					"namespace", snap.Namespace,
					"snapshot.name", snap.Name,
					"pr-snapshot-kept", keptPrSnaps+1,
					"pr-snapshots-to-keep", prSnapshotsToKeep,
				)
				keptPrSnaps++
			} else if !slices.Contains(cancelledOrMergedPRSnapshots, snap.Name) &&
				metadata.HasAnnotation(&snap, PRGroupCreationAnnotation) &&
				strings.Contains(snap.GetAnnotations()[PRGroupCreationAnnotation], "is still running, won't create group snapshot") &&
				!isLongerThanSpecificTime(snap, 1*24*60*60) {
				// when we have reached the number of nonPrSnapshotToKeep
				// we still try to keep the component snapshot if it is created in one day, unmerged and expecting group snapshot creation but waiting for some other inprogress pipelineruns to finish
				// the message "is still running, won't create group snapshot" comes from integration-service when it decides not to create group snapshot yet
				logger.V(1).Info(
					"Skipping PR candidate snapshot as it is expecting group snapshot creation and is created within one day",
					"namespace", snap.Namespace,
					"snapshot.name", snap.Name,
					"pr-snapshot-kept", keptPrSnaps+1,
					"pr-snapshots-to-keep", prSnapshotsToKeep,
				)
				keptPrSnaps++
			} else {
				logger.V(1).Info(
					"Adding PR candidate snapshot",
					"namespace", snap.Namespace,
					"snapshot.name", snap.Name,
					"pr-snapshot-kept", keptPrSnaps,
					"pr-snapshots-to-keep", prSnapshotsToKeep,
				)
				shortList = append(shortList, snap)
			}
		}
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
		snap := snap
		err := cl.Delete(context.Background(), &snap)
		if err != nil {
			logger.Error(err, "Failed to delete snapshot.", "snapshot.name", snap.Name)
		}
	}
}

func main() {
	var prSnapshotsToKeep, nonPrSnapshotsToKeep, minSnapShotsToKeepPerComponent int
	flag.IntVar(
		&prSnapshotsToKeep,
		"pr-snapshots-to-keep",
		512,
		"Number of PR snapshots to keep after garbage collection",
	)
	flag.IntVar(
		&nonPrSnapshotsToKeep,
		"non-pr-snapshots-to-keep",
		512,
		"Number of non-PR snapshots to keep after garbage collection",
	)
	flag.IntVar(
		&minSnapShotsToKeepPerComponent,
		"min-snapshots-to-keep-per-component",
		5,
		"Number of push snapshots to keep per component",
	)

	opts := zap.Options{
		Development: false,
		TimeEncoder: zapcore.RFC3339TimeEncoder,
		ZapOpts:     []zap2.Option{zap2.WithCaller(true)},
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	logger := zap.New(zap.UseFlagOptions(&opts))

	var err error

	// We want env vars args to take precedence over cli args
	if value, ok := os.LookupEnv("PR_SNAPSHOTS_TO_KEEP"); ok {
		prSnapshotsToKeep, err = strconv.Atoi(value)
		if err != nil {
			logger.Error(err, "Failed parsing env var PR_SNAPSHOTS_TO_KEEP")
			panic(err.Error())
		}
	}
	if value, ok := os.LookupEnv("NON_PR_SNAPSHOTS_TO_KEEP"); ok {
		nonPrSnapshotsToKeep, err = strconv.Atoi(value)
		if err != nil {
			logger.Error(err, "Failed parsing env var NON_PR_SNAPSHOTS_TO_KEEP")
			panic(err.Error())
		}
	}
	if value, ok := os.LookupEnv("MIN_SNAPSHOTS_TO_KEEP_PER_COMPONENT"); ok {
		minSnapShotsToKeepPerComponent, err = strconv.Atoi(value)
		if err != nil {
			logger.Error(err, "Failed parsing env var MIN_SNAPSHOTS_TO_KEEP_PER_COMPONENT")
			panic(err.Error())
		}
	}

	cl, err := client.New(config.GetConfigOrDie(), client.Options{Scheme: scheme})
	if err != nil {
		logger.Error(err, "Snapshots garbage collection failed creating client")
		panic(err.Error())
	}

	err = garbageCollectSnapshots(cl, logger, prSnapshotsToKeep, nonPrSnapshotsToKeep, minSnapShotsToKeepPerComponent)
	if err != nil {
		logger.Error(err, "Snapshots garbage collection failed")
		panic(err.Error())
	}
}
