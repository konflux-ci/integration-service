package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"time"

	gomonkey "github.com/agiledragon/gomonkey/v2"
	"github.com/go-logr/logr"
	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	"github.com/konflux-ci/integration-service/gitops"
	releasev1alpha1 "github.com/konflux-ci/release-service/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/tonglil/buflogr"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("Test garbage collection for snapshots", func() {
	var buf bytes.Buffer
	var logger logr.Logger

	BeforeEach(func() {
		buf.Reset()
		logger = buflogr.NewWithBuffer(&buf)
	})

	Describe("Test getSnapshotsForNSReleases", func() {
		It("Finds no snapshot when there are no releases", func() {

			cl := fake.NewClientBuilder().
				WithScheme(scheme).
				WithLists(
					&releasev1alpha1.ReleaseList{
						Items: []releasev1alpha1.Release{},
					}).Build()
			snapToData := make(map[string]snapshotData)
			output, err := getSnapshotsForNSReleases(cl, snapToData, "ns1", logger)

			Expect(err).ShouldNot(HaveOccurred())
			Expect(output).To(BeEmpty())
		})

		It("Finds a snapshot for a single release", func() {
			rel1 := &releasev1alpha1.Release{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-release",
					Namespace: "ns1",
				},
				Spec: releasev1alpha1.ReleaseSpec{
					Snapshot: "my-snapshot",
				},
			}
			rel2 := &releasev1alpha1.Release{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "another-ns-release",
					Namespace: "another-ns",
				},
				Spec: releasev1alpha1.ReleaseSpec{
					Snapshot: "another-snapshot",
				},
			}

			cl := fake.NewClientBuilder().
				WithScheme(scheme).
				WithLists(
					&releasev1alpha1.ReleaseList{
						Items: []releasev1alpha1.Release{*rel1, *rel2},
					}).Build()
			snapToData := make(map[string]snapshotData)
			output, err := getSnapshotsForNSReleases(cl, snapToData, "ns1", logger)

			Expect(err).ShouldNot(HaveOccurred())
			Expect(output).To(HaveLen(1))
			Expect(output["my-snapshot"].release.Name).To(Equal("my-release"))
		})

		It("Finds multiple snapshots for multiple releases", func() {
			rel1 := &releasev1alpha1.Release{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rel1",
					Namespace: "ns1",
				},
				Spec: releasev1alpha1.ReleaseSpec{
					Snapshot: "snap1",
				},
			}
			rel2 := &releasev1alpha1.Release{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rel2",
					Namespace: "ns1",
				},
				Spec: releasev1alpha1.ReleaseSpec{
					Snapshot: "snap2",
				},
			}

			cl := fake.NewClientBuilder().
				WithScheme(scheme).
				WithLists(
					&releasev1alpha1.ReleaseList{
						Items: []releasev1alpha1.Release{*rel1, *rel2},
					}).Build()
			snapToData := make(map[string]snapshotData)
			output, err := getSnapshotsForNSReleases(cl, snapToData, "ns1", logger)

			Expect(err).ShouldNot(HaveOccurred())
			Expect(output).To(HaveLen(2))
			Expect(output["snap1"].release.Name).To(Equal("rel1"))
			Expect(output["snap2"].release.Name).To(Equal("rel2"))
		})

		It("Returns error when fails API call", func() {
			cl := fake.NewClientBuilder().Build()
			snapToData := make(map[string]snapshotData)
			_, err := getSnapshotsForNSReleases(cl, snapToData, "ns1", logger)

			Expect(err).Should(HaveOccurred())
			Expect(err).To(MatchError(ContainSubstring(
				"no kind is registered for the type v1alpha1.ReleaseList in scheme",
			)))
		})
	})

	Describe("Test getUnassociatedNSSnapshots", func() {
		It("Finds no unassociated snapshots when there are no snapshots", func() {

			cl := fake.NewClientBuilder().
				WithScheme(scheme).
				WithLists(
					&applicationapiv1alpha1.SnapshotList{
						Items: []applicationapiv1alpha1.Snapshot{},
					}).Build()
			snapToData := make(map[string]snapshotData)
			output, err := getUnassociatedNSSnapshots(cl, snapToData, "ns1", logger)

			Expect(err).ShouldNot(HaveOccurred())
			Expect(output).To(BeEmpty())
		})

		It("Finds no unassociated snapshots when all are associated", func() {
			snap1 := &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "associated-snapshot",
					Namespace: "ns1",
				},
			}
			snap2 := &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "another-associated-snapshot",
					Namespace: "ns1",
				},
			}

			cl := fake.NewClientBuilder().
				WithScheme(scheme).
				WithLists(
					&applicationapiv1alpha1.SnapshotList{
						Items: []applicationapiv1alpha1.Snapshot{*snap1, *snap2},
					}).Build()
			snapToData := make(map[string]snapshotData)
			snapToData["associated-snapshot"] = snapshotData{}
			snapToData["another-associated-snapshot"] = snapshotData{}
			output, err := getUnassociatedNSSnapshots(cl, snapToData, "ns1", logger)

			Expect(err).ShouldNot(HaveOccurred())
			Expect(output).To(BeEmpty())
		})

		It("Finds unassociated snapshots when some snapshots are associated", func() {
			snap1 := &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "associated-snapshot",
					Namespace: "ns1",
				},
			}
			snap2 := &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "another-associated-snapshot",
					Namespace: "ns1",
				},
			}
			snap3 := &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "unassociated-snapshot",
					Namespace: "ns1",
				},
			}

			cl := fake.NewClientBuilder().
				WithScheme(scheme).
				WithLists(
					&applicationapiv1alpha1.SnapshotList{
						Items: []applicationapiv1alpha1.Snapshot{
							*snap1, *snap2, *snap3,
						},
					}).Build()
			snapToData := make(map[string]snapshotData)
			snapToData["associated-snapshot"] = snapshotData{}
			snapToData["another-associated-snapshot"] = snapshotData{}
			output, err := getUnassociatedNSSnapshots(cl, snapToData, "ns1", logger)

			Expect(err).ShouldNot(HaveOccurred())
			Expect(output).To(HaveLen(1))
			Expect(output[0].Name).To(Equal("unassociated-snapshot"))
		})

		It("Finds unassociated snapshots when no snapshots are associated", func() {
			snap1 := &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "unassociated-snapshot",
					Namespace: "ns1",
				},
			}
			snap2 := &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "another-unassociated-snapshot",
					Namespace: "ns1",
				},
			}

			cl := fake.NewClientBuilder().
				WithScheme(scheme).
				WithLists(
					&applicationapiv1alpha1.SnapshotList{
						Items: []applicationapiv1alpha1.Snapshot{*snap1, *snap2},
					}).Build()
			snapToData := make(map[string]snapshotData)
			output, err := getUnassociatedNSSnapshots(cl, snapToData, "ns1", logger)

			Expect(err).ShouldNot(HaveOccurred())
			Expect(output).To(HaveLen(2))
		})

		It("Returns error when fails API call", func() {

			cl := fake.NewClientBuilder().Build()
			snapToData := make(map[string]snapshotData)
			_, err := getUnassociatedNSSnapshots(cl, snapToData, "ns1", logger)

			Expect(err).Should(HaveOccurred())
			Expect(err).To(MatchError(ContainSubstring(
				"no kind is registered for the type v1alpha1.SnapshotList in scheme",
			)))
		})
	})

	Describe("Test extractCancelledPRSnapshots", func() {
		cancelledCondition := metav1.Condition{
			Type:   gitops.AppStudioIntegrationStatusCondition,
			Status: metav1.ConditionTrue,
			Reason: gitops.AppStudioIntegrationStatusCanceled,
		}

		It("Returns empty when no snapshots", func() {
			output := extractCancelledPRSnapshots([]applicationapiv1alpha1.Snapshot{})
			Expect(output).To(BeEmpty())
		})

		It("Returns empty when no PR snapshots", func() {
			pushSnap := applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "push-snap",
					Namespace: "ns1",
					Labels:    map[string]string{"pac.test.appstudio.openshift.io/event-type": "push"},
				},
				Status: applicationapiv1alpha1.SnapshotStatus{
					Conditions: []metav1.Condition{cancelledCondition},
				},
			}
			output := extractCancelledPRSnapshots([]applicationapiv1alpha1.Snapshot{pushSnap})
			Expect(output).To(BeEmpty())
		})

		It("Returns empty when PR snapshots are not cancelled", func() {
			prSnap := applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pr-snap",
					Namespace: "ns1",
					Labels:    map[string]string{"pac.test.appstudio.openshift.io/event-type": "pull_request"},
				},
			}
			output := extractCancelledPRSnapshots([]applicationapiv1alpha1.Snapshot{prSnap})
			Expect(output).To(BeEmpty())
		})

		It("Returns names of cancelled PR snapshots without keep-snapshot annotation", func() {
			cancelledPRSnap := applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cancelled-pr-snap",
					Namespace: "ns1",
					Labels:    map[string]string{"pac.test.appstudio.openshift.io/event-type": "pull_request"},
				},
				Status: applicationapiv1alpha1.SnapshotStatus{Conditions: []metav1.Condition{cancelledCondition}},
			}
			output := extractCancelledPRSnapshots([]applicationapiv1alpha1.Snapshot{cancelledPRSnap})
			Expect(output).To(HaveLen(1))
			Expect(output).To(ContainElement("cancelled-pr-snap"))
		})

		It("Excludes cancelled PR snapshots that have keep-snapshot annotation", func() {
			keptCancelledPRSnap := applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "kept-cancelled-pr-snap",
					Namespace:   "ns1",
					Annotations: map[string]string{"test.appstudio.openshift.io/keep-snapshot": "true"},
					Labels:      map[string]string{"pac.test.appstudio.openshift.io/event-type": "pull_request"},
				},
				Status: applicationapiv1alpha1.SnapshotStatus{Conditions: []metav1.Condition{cancelledCondition}},
			}
			output := extractCancelledPRSnapshots([]applicationapiv1alpha1.Snapshot{keptCancelledPRSnap})
			Expect(output).To(BeEmpty())
		})

		It("Returns multiple cancelled PR snapshot names and excludes non-PR and kept", func() {
			cancelledPR1 := applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cancelled-pr-1",
					Namespace: "ns1",
					Labels:    map[string]string{"pac.test.appstudio.openshift.io/event-type": "pull_request"},
				},
				Status: applicationapiv1alpha1.SnapshotStatus{Conditions: []metav1.Condition{cancelledCondition}},
			}
			cancelledPR2 := applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cancelled-pr-2",
					Namespace: "ns1",
					Labels:    map[string]string{"pac.test.appstudio.openshift.io/event-type": "Merge Request"},
				},
				Status: applicationapiv1alpha1.SnapshotStatus{Conditions: []metav1.Condition{cancelledCondition}},
			}
			pushSnap := applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "push-snap",
					Namespace: "ns1",
					Labels:    map[string]string{"pac.test.appstudio.openshift.io/event-type": "push"},
				},
				Status: applicationapiv1alpha1.SnapshotStatus{Conditions: []metav1.Condition{cancelledCondition}},
			}
			output := extractCancelledPRSnapshots([]applicationapiv1alpha1.Snapshot{cancelledPR1, cancelledPR2, pushSnap})
			Expect(output).To(HaveLen(2))
			Expect(output).To(ContainElement("cancelled-pr-1"))
			Expect(output).To(ContainElement("cancelled-pr-2"))
			Expect(output).NotTo(ContainElement("push-snap"))
		})
	})

	Describe("Test getSnapshotsForRemoval", func() {
		It("Handles no snapshots", func() {

			cl := fake.NewClientBuilder().
				WithScheme(scheme).
				WithLists(
					&applicationapiv1alpha1.SnapshotList{
						Items: []applicationapiv1alpha1.Snapshot{},
					}).Build()
			candidates := []applicationapiv1alpha1.Snapshot{}
			output := getSnapshotsForRemoval(cl, candidates, 2, 1, 0, logger)

			Expect(output).To(BeEmpty())
		})

		It("No PR snapshots, some non-PR snapshots. Not enough to GCed", func() {
			currentTime := time.Now()
			newerSnap := &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name: "newer-snapshot",
					Labels: map[string]string{
						"pac.test.appstudio.openshift.io/event-type": "push",
					},
					CreationTimestamp: metav1.NewTime(currentTime.Add(time.Hour * 1)),
				},
			}
			olderSnap := &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "older-snapshot",
					CreationTimestamp: metav1.NewTime(currentTime),
				},
			}

			cl := fake.NewClientBuilder().
				WithScheme(scheme).
				WithLists(
					&applicationapiv1alpha1.SnapshotList{
						Items: []applicationapiv1alpha1.Snapshot{
							*newerSnap, *olderSnap,
						},
					}).Build()
			candidates := []applicationapiv1alpha1.Snapshot{*newerSnap, *olderSnap}
			output := getSnapshotsForRemoval(cl, candidates, 0, 2, 0, logger)

			Expect(output).To(BeEmpty())
		})

		It("Some PR snapshots, no non-PR snapshots. some to be GCed", func() {
			currentTime := time.Now()
			newerSnap := &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name: "newer-snapshot",
					Labels: map[string]string{
						"pac.test.appstudio.openshift.io/event-type": "pull_request",
					},
					CreationTimestamp: metav1.NewTime(currentTime.Add(time.Hour * 1)),
				},
			}
			olderSnap := &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name: "older-snapshot",
					Labels: map[string]string{
						"pac.test.appstudio.openshift.io/event-type": "Merge Request",
					},
					CreationTimestamp: metav1.NewTime(currentTime),
				},
			}

			cl := fake.NewClientBuilder().
				WithScheme(scheme).
				WithLists(
					&applicationapiv1alpha1.SnapshotList{
						Items: []applicationapiv1alpha1.Snapshot{
							*newerSnap, *olderSnap,
						},
					}).Build()
			candidates := []applicationapiv1alpha1.Snapshot{*newerSnap, *olderSnap}
			output := getSnapshotsForRemoval(cl, candidates, 1, 0, 0, logger)

			Expect(output).To(HaveLen(1))
			Expect(output[0].Name).To(Equal("older-snapshot"))
		})

		It("Cancelled PR snapshots are marked for removal even when prSnapshotsToKeep would allow keeping them", func() {
			cancelledCondition := metav1.Condition{
				Type:   gitops.AppStudioIntegrationStatusCondition,
				Status: metav1.ConditionTrue,
				Reason: gitops.AppStudioIntegrationStatusCanceled,
			}
			currentTime := time.Now()

			// Cancelled PR snapshot (superseded) - should be deleted first
			cancelledPRSnap := &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cancelled-pr-snapshot",
					Labels: map[string]string{
						"pac.test.appstudio.openshift.io/event-type": "pull_request",
					},
					CreationTimestamp: metav1.NewTime(currentTime),
				},
				Status: applicationapiv1alpha1.SnapshotStatus{
					Conditions: []metav1.Condition{cancelledCondition},
				},
			}
			// Non-cancelled PR snapshot - should be kept when prSnapshotsToKeep >= 1
			activePRSnap := &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name: "active-pr-snapshot",
					Labels: map[string]string{
						"pac.test.appstudio.openshift.io/event-type": "pull_request",
					},
					CreationTimestamp: metav1.NewTime(currentTime.Add(time.Hour * 1)),
				},
			}

			cl := fake.NewClientBuilder().
				WithScheme(scheme).
				WithLists(
					&applicationapiv1alpha1.SnapshotList{
						Items: []applicationapiv1alpha1.Snapshot{*cancelledPRSnap, *activePRSnap},
					}).Build()
			candidates := []applicationapiv1alpha1.Snapshot{*cancelledPRSnap, *activePRSnap}
			// prSnapshotsToKeep=1: we have room to keep 1 PR snapshot, but cancelled one should still be removed
			output := getSnapshotsForRemoval(cl, candidates, 1, 0, 0, logger)

			Expect(output).To(HaveLen(1))
			Expect(output[0].Name).To(Equal("cancelled-pr-snapshot"))
		})

		It("Cancelled PR snapshot with keep-snapshot annotation is not in removal list", func() {
			cancelledCondition := metav1.Condition{
				Type:   gitops.AppStudioIntegrationStatusCondition,
				Status: metav1.ConditionTrue,
				Reason: gitops.AppStudioIntegrationStatusCanceled,
			}
			currentTime := time.Now()

			// Cancelled PR snapshot but with keep-snapshot - should NOT be in extractCancelledPRSnapshots, so normal logic applies
			keptCancelledPRSnap := &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name: "kept-cancelled-pr-snapshot",
					Labels: map[string]string{
						"pac.test.appstudio.openshift.io/event-type": "pull_request",
					},
					Annotations:       map[string]string{"test.appstudio.openshift.io/keep-snapshot": "true"},
					CreationTimestamp: metav1.NewTime(currentTime),
				},
				Status: applicationapiv1alpha1.SnapshotStatus{
					Conditions: []metav1.Condition{cancelledCondition},
				},
			}

			cl := fake.NewClientBuilder().
				WithScheme(scheme).
				WithLists(
					&applicationapiv1alpha1.SnapshotList{
						Items: []applicationapiv1alpha1.Snapshot{*keptCancelledPRSnap},
					}).Build()
			candidates := []applicationapiv1alpha1.Snapshot{*keptCancelledPRSnap}
			output := getSnapshotsForRemoval(cl, candidates, 1, 0, 0, logger)

			// With keep-snapshot it is not in canceledPRSnapshots, so it is kept (prSnapshotsToKeep=1)
			Expect(output).To(BeEmpty())
		})

		It("Snapshots of both types to be GCed", func() {
			currentTime := time.Now()
			newerPRSnap := &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name: "newer-pr-snapshot",
					Labels: map[string]string{
						"pac.test.appstudio.openshift.io/event-type": "pull_request",
					},
					CreationTimestamp: metav1.NewTime(currentTime.Add(time.Hour * 1)),
				},
			}
			olderPRSnap := &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name: "older-pr-snapshot",
					Labels: map[string]string{
						"pac.test.appstudio.openshift.io/event-type": "Merge_Request",
					},
					CreationTimestamp: metav1.NewTime(currentTime),
				},
			}
			anotherOldPRSnap := &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name: "another-old-pr-snapshot",
					Labels: map[string]string{
						"pac.test.appstudio.openshift.io/event-type": "Note",
					},
					CreationTimestamp: metav1.NewTime(currentTime.Add(time.Minute * 1)),
				},
			}
			newerNonPRSnap := &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name: "newer-non-pr-snapshot",
					Labels: map[string]string{
						"pac.test.appstudio.openshift.io/event-type": "Push",
					},
					CreationTimestamp: metav1.NewTime(currentTime.Add(time.Hour * 11)),
				},
			}
			olderNonPRSnap := &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "older-non-pr-snapshot",
					CreationTimestamp: metav1.NewTime(currentTime.Add(time.Hour * 10)),
				},
			}
			overrideSnap := &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "override-snapshot",
					CreationTimestamp: metav1.NewTime(currentTime),
					Labels: map[string]string{
						"test.appstudio.openshift.io/type": "override",
					},
				},
			}

			cl := fake.NewClientBuilder().
				WithScheme(scheme).
				WithLists(
					&applicationapiv1alpha1.SnapshotList{
						Items: []applicationapiv1alpha1.Snapshot{
							*newerPRSnap, *olderPRSnap, *anotherOldPRSnap,
							*newerNonPRSnap, *olderNonPRSnap, *overrideSnap,
						},
					}).Build()
			candidates := []applicationapiv1alpha1.Snapshot{
				*newerPRSnap, *olderPRSnap, *newerNonPRSnap, *olderNonPRSnap,
				*anotherOldPRSnap, *overrideSnap,
			}
			output := getSnapshotsForRemoval(cl, candidates, 1, 1, 0, logger)

			Expect(output).To(HaveLen(4))
			Expect(output[0].Name).To(Equal("older-non-pr-snapshot"))
			Expect(output[1].Name).To(Equal("another-old-pr-snapshot"))
			Expect(output[2].Name).To(Equal("older-pr-snapshot"))
			Expect(output[3].Name).To(Equal("override-snapshot"))
		})
	})

	Describe("Test deleteSnapshots", func() {
		It("Handles no snapshots to be removed", func() {
			snap := &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name: "snap",
				},
			}
			cl := fake.NewClientBuilder().
				WithScheme(scheme).
				WithLists(
					&applicationapiv1alpha1.SnapshotList{
						Items: []applicationapiv1alpha1.Snapshot{*snap},
					}).Build()
			snapsToDelete := []applicationapiv1alpha1.Snapshot{}
			deleteSnapshots(cl, snapsToDelete, logger)

			snapsRemaining := &applicationapiv1alpha1.SnapshotList{}
			err := cl.List(context.Background(), snapsRemaining)

			Expect(err).ShouldNot(HaveOccurred())
			Expect(snapsRemaining.Items).To(HaveLen(1))
		})

		It("Handles some snapshots to be removed", func() {
			noDel := &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name: "no-del",
				},
			}
			del1 := &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name: "del1",
				},
			}
			del2 := &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name: "del2",
				},
			}
			cl := fake.NewClientBuilder().
				WithScheme(scheme).
				WithLists(
					&applicationapiv1alpha1.SnapshotList{
						Items: []applicationapiv1alpha1.Snapshot{*noDel, *del1, *del2},
					}).Build()
			snapsToDelete := []applicationapiv1alpha1.Snapshot{*del1, *del2}
			deleteSnapshots(cl, snapsToDelete, logger)

			snapsRemaining := &applicationapiv1alpha1.SnapshotList{}
			err := cl.List(context.Background(), snapsRemaining)

			Expect(err).ShouldNot(HaveOccurred())
			Expect(snapsRemaining.Items).To(HaveLen(1))
			Expect(snapsRemaining.Items[0].Name).To(Equal("no-del"))
		})

		It("Handles all snapshots to be removed and continues on failure", func() {
			nonExist := &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name: "non-existing",
				},
			}
			del1 := &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name: "del1",
				},
			}
			del2 := &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name: "del2",
				},
			}
			cl := fake.NewClientBuilder().
				WithScheme(scheme).
				WithLists(
					&applicationapiv1alpha1.SnapshotList{
						Items: []applicationapiv1alpha1.Snapshot{*del1, *del2},
					}).Build()
			snapsToDelete := []applicationapiv1alpha1.Snapshot{*nonExist, *del1, *del2}
			deleteSnapshots(cl, snapsToDelete, logger)

			snapsRemaining := &applicationapiv1alpha1.SnapshotList{}
			err := cl.List(context.Background(), snapsRemaining)

			Expect(err).ShouldNot(HaveOccurred())
			Expect(snapsRemaining.Items).To(BeEmpty())

			logLines := strings.Split(buf.String(), "\n")
			Expect(logLines[len(logLines)-2]).Should(ContainSubstring(
				"ERROR snapshots.appstudio.redhat.com \"non-existing\" not found " +
					"Failed to delete snapshot. snapshot.name non-existing",
			))
		})
	})

	Describe("Test getTenantNamespaces", func() {
		It("Handles no namespaces", func() {
			cl := fake.NewClientBuilder().WithScheme(scheme).Build()

			tenantsNs, err := getTenantNamespaces(cl, logger)

			Expect(err).ShouldNot(HaveOccurred())
			Expect(tenantsNs).To(BeEmpty())
		})

		It("Handles no tenant namespaces", func() {
			ns1 := &core.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "ns1",
					Labels: map[string]string{
						"toolchain.dev.openshift.com/type": "not-a-tenant",
					},
				},
			}
			ns2 := &core.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "ns2",
				},
			}

			cl := fake.NewClientBuilder().
				WithScheme(scheme).
				WithLists(&core.NamespaceList{Items: []core.Namespace{*ns1, *ns2}}).
				Build()

			tenantsNs, err := getTenantNamespaces(cl, logger)

			Expect(err).ShouldNot(HaveOccurred())
			Expect(tenantsNs).To(BeEmpty())
		})

		It("Handles one tenant namespace", func() {
			ns1 := &core.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "ns1",
					Labels: map[string]string{
						"toolchain.dev.openshift.com/type": "not-a-tenant",
					},
				},
			}
			ns2 := &core.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "ns2",
				},
			}
			ns3 := &core.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "ns3",
					Labels: map[string]string{
						"toolchain.dev.openshift.com/type": "tenant",
					},
				},
			}

			cl := fake.NewClientBuilder().
				WithScheme(scheme).
				WithLists(&core.NamespaceList{Items: []core.Namespace{
					*ns1, *ns2, *ns3,
				}}).
				Build()

			tenantsNs, err := getTenantNamespaces(cl, logger)

			Expect(err).ShouldNot(HaveOccurred())
			Expect(tenantsNs).To(HaveLen(1))
			Expect(tenantsNs[0].Name).To(Equal("ns3"))
		})

		It("Handles all tenant namespaces", func() {
			ns1 := &core.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "ns1",
					Labels: map[string]string{
						"toolchain.dev.openshift.com/type": "tenant",
					},
				},
			}
			ns2 := &core.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "ns2",
					Labels: map[string]string{
						"konflux-ci.dev/type": "tenant",
					},
				},
			}
			ns3 := &core.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "ns3",
					Labels: map[string]string{
						"konflux.ci/type": "user",
					},
				},
			}

			cl := fake.NewClientBuilder().
				WithScheme(scheme).
				WithLists(&core.NamespaceList{Items: []core.Namespace{
					*ns1, *ns2, *ns3,
				}}).
				Build()

			tenantsNs, err := getTenantNamespaces(cl, logger)

			Expect(err).ShouldNot(HaveOccurred())
			Expect(tenantsNs).To(HaveLen(3))
		})

		It("Returns error when fails API call", func() {
			cl := fake.NewClientBuilder().WithScheme(runtime.NewScheme()).Build()
			_, err := getTenantNamespaces(cl, logger)
			Expect(err).Should(HaveOccurred())
			Expect(err).To(MatchError(ContainSubstring(
				"no kind is registered for the type v1.NamespaceList in scheme",
			)))
		})
	})

	Describe("Test garbageCollectSnapshots", func() {
		It("Garbage collect snapshots from multiple namespaces", func() {

			ns1 := &core.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "ns1",
					Labels: map[string]string{
						"toolchain.dev.openshift.com/type": "tenant",
					},
				},
			}
			ns2 := &core.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "ns2",
					Labels: map[string]string{
						"toolchain.dev.openshift.com/type": "not-a-tenant",
					},
				},
			}
			ns3 := &core.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "ns3",
				},
			}
			ns4 := &core.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "ns4",
					Labels: map[string]string{
						"konflux-ci.dev/type": "tenant",
					},
				},
			}

			rel1 := &releasev1alpha1.Release{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rel1",
					Namespace: "ns1",
				},
				Spec: releasev1alpha1.ReleaseSpec{
					Snapshot: "keep11",
				},
			}
			rel2 := &releasev1alpha1.Release{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rel2",
					Namespace: "ns1",
				},
				Spec: releasev1alpha1.ReleaseSpec{
					Snapshot: "keep21",
				},
			}

			currentTime := time.Now()
			keep11 := &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "keep11",
					Namespace:         "ns1",
					CreationTimestamp: metav1.NewTime(currentTime),
				},
			}
			keep21 := &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "keep21",
					Namespace:         "ns1",
					CreationTimestamp: metav1.NewTime(currentTime),
				},
			}
			keep31 := &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "keep31",
					Namespace:         "ns1",
					CreationTimestamp: metav1.NewTime(currentTime),
				},
			}
			keep41 := &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "keep41",
					Namespace:         "ns1",
					CreationTimestamp: metav1.NewTime(currentTime),
				},
			}
			newerPRSnap := &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "newer-pr-snapshot",
					Namespace: "ns1",
					Labels: map[string]string{
						"pac.test.appstudio.openshift.io/event-type": "pull_request",
					},
					CreationTimestamp: metav1.NewTime(currentTime.Add(time.Hour * 1)),
				},
			}
			olderPRSnap := &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "older-pr-snapshot",
					Namespace: "ns1",
					Labels: map[string]string{
						"pac.test.appstudio.openshift.io/event-type": "pull_request",
					},
					CreationTimestamp: metav1.NewTime(currentTime),
				},
			}
			newerNonPRSnap := &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "newer-non-pr-snapshot",
					Namespace:         "ns1",
					CreationTimestamp: metav1.NewTime(currentTime.Add(time.Hour * 11)),
				},
			}
			olderNonPRSnap := &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "older-non-pr-snapshot",
					Namespace:         "ns1",
					CreationTimestamp: metav1.NewTime(currentTime.Add(time.Hour * 10)),
				},
			}

			newerPRSnapNs2 := &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "newer-pr-snapshot",
					Namespace: "ns2",
					Labels: map[string]string{
						"pac.test.appstudio.openshift.io/event-type": "pull_request",
					},
					CreationTimestamp: metav1.NewTime(currentTime.Add(time.Hour * 1)),
				},
			}
			olderPRSnapNs2 := &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "older-pr-snapshot",
					Namespace: "ns2",
					Labels: map[string]string{
						"pac.test.appstudio.openshift.io/event-type": "pull_request",
					},
					CreationTimestamp: metav1.NewTime(currentTime),
				},
			}
			newerNonPRSnapNs3 := &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "newer-non-pr-snapshot",
					Namespace:         "ns3",
					CreationTimestamp: metav1.NewTime(currentTime.Add(time.Hour * 11)),
				},
			}
			olderNonPRSnapNs3 := &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "older-non-pr-snapshot",
					Namespace:         "ns3",
					CreationTimestamp: metav1.NewTime(currentTime.Add(time.Hour * 10)),
				},
			}
			newerPRSnapNs4 := &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "newer-pr-snapshot",
					Namespace: "ns4",
					Labels: map[string]string{
						"pac.test.appstudio.openshift.io/event-type": "pull_request",
					},
					CreationTimestamp: metav1.NewTime(currentTime.Add(time.Hour * 1)),
				},
			}
			olderPRSnapNs4 := &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "older-pr-snapshot",
					Namespace: "ns4",
					Labels: map[string]string{
						"pac.test.appstudio.openshift.io/event-type": "pull_request",
					},
					CreationTimestamp: metav1.NewTime(currentTime),
				},
			}

			cl := fake.NewClientBuilder().
				WithScheme(scheme).
				WithLists(
					&core.NamespaceList{
						Items: []core.Namespace{*ns1, *ns2, *ns3, *ns4},
					},
					&releasev1alpha1.ReleaseList{
						Items: []releasev1alpha1.Release{*rel1, *rel2},
					},
					&applicationapiv1alpha1.SnapshotList{
						Items: []applicationapiv1alpha1.Snapshot{
							*newerPRSnap, *olderPRSnap,
							*newerNonPRSnap, *olderNonPRSnap,
							*keep11, *keep21, *keep31, *keep41,
							*newerPRSnapNs2, *olderPRSnapNs2,
							*newerNonPRSnapNs3, *olderNonPRSnapNs3,
							*newerPRSnapNs4, *olderPRSnapNs4,
						},
					},
				).Build()

			var err error

			snapsBefore := &applicationapiv1alpha1.SnapshotList{}
			err = cl.List(context.Background(), snapsBefore, &client.ListOptions{
				Namespace: "ns1",
			})
			Expect(err).ShouldNot(HaveOccurred())
			Expect(snapsBefore.Items).To(HaveLen(8))
			err = cl.List(context.Background(), snapsBefore, &client.ListOptions{
				Namespace: "ns2",
			})
			Expect(err).ShouldNot(HaveOccurred())
			Expect(snapsBefore.Items).To(HaveLen(2))
			err = cl.List(context.Background(), snapsBefore, &client.ListOptions{
				Namespace: "ns3",
			})
			Expect(err).ShouldNot(HaveOccurred())
			Expect(snapsBefore.Items).To(HaveLen(2))
			err = cl.List(context.Background(), snapsBefore, &client.ListOptions{
				Namespace: "ns4",
			})
			Expect(err).ShouldNot(HaveOccurred())
			Expect(snapsBefore.Items).To(HaveLen(2))

			err = garbageCollectSnapshots(cl, logger, 1, 1, 0)
			Expect(err).ShouldNot(HaveOccurred())

			snapsAfter := &applicationapiv1alpha1.SnapshotList{}
			err = cl.List(context.Background(), snapsAfter, &client.ListOptions{
				Namespace: "ns1",
			})
			Expect(err).ShouldNot(HaveOccurred())
			Expect(snapsAfter.Items).To(HaveLen(3))
			err = cl.List(context.Background(), snapsAfter, &client.ListOptions{
				Namespace: "ns2",
			})
			Expect(err).ShouldNot(HaveOccurred())
			Expect(snapsAfter.Items).To(HaveLen(2))
			err = cl.List(context.Background(), snapsAfter, &client.ListOptions{
				Namespace: "ns3",
			})
			Expect(err).ShouldNot(HaveOccurred())
			Expect(snapsAfter.Items).To(HaveLen(2))
			err = cl.List(context.Background(), snapsAfter, &client.ListOptions{
				Namespace: "ns4",
			})
			Expect(err).ShouldNot(HaveOccurred())
			Expect(snapsAfter.Items).To(HaveLen(1))
		})

		It("Fails if cannot list namespaces", func() {
			cl := fake.NewClientBuilder().WithScheme(runtime.NewScheme()).Build()
			err := garbageCollectSnapshots(cl, logger, 1, 1, 0)
			Expect(err).Should(HaveOccurred())
			Expect(err).To(MatchError(ContainSubstring(
				"no kind is registered for the type v1.NamespaceList in scheme",
			)))
		})

		It("Does not fail if getSnapshotsForNSReleases fails for namespace", func() {
			ns1 := &core.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "ns1",
					Labels: map[string]string{
						"toolchain.dev.openshift.com/type": "tenant",
					},
				},
			}
			// using default scheme having namespace but not releases
			cl := fake.NewClientBuilder().WithLists(
				&core.NamespaceList{Items: []core.Namespace{*ns1}},
			).Build()

			err := garbageCollectSnapshots(cl, logger, 1, 1, 0)
			Expect(err).ShouldNot(HaveOccurred())
			logLines := strings.Split(buf.String(), "\n")
			Expect(logLines[len(logLines)-2]).Should(ContainSubstring(
				"Failed getting releases associated with snapshots.",
			))
		})

		It("Does not fail if getUnassociatedNSSnapshots fails for namespace", func() {
			ns1 := &core.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "ns1",
					Labels: map[string]string{
						"toolchain.dev.openshift.com/type": "tenant",
					},
				},
			}

			// Monkey-patching getUnassociatedNSSnapshots so it returns with error
			patches := gomonkey.ApplyFunc(
				getUnassociatedNSSnapshots, func(cl client.Client,
					snapToData map[string]snapshotData,
					namespace string,
					logger logr.Logger) ([]applicationapiv1alpha1.Snapshot, error) {
					return nil, errors.New("")
				})
			defer patches.Reset()
			cl := fake.NewClientBuilder().WithScheme(scheme).WithLists(
				&core.NamespaceList{Items: []core.Namespace{*ns1}},
			).Build()

			err := garbageCollectSnapshots(cl, logger, 1, 1, 0)
			Expect(err).ShouldNot(HaveOccurred())
			logLines := strings.Split(buf.String(), "\n")
			Expect(logLines[len(logLines)-2]).Should(ContainSubstring(
				"Failed getting unassociated snapshots.",
			))
		})
	})

	Describe("Test filterSnapshotsWithKeepSnapshotAnnotation", func() {
		It("Keeps one PR, one non-pr snapshot and discards the other two", func() {
			snapKeepNonPR := applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "associated-snapshot",
					Namespace: "ns1",
					Labels: map[string]string{
						"pac.test.appstudio.openshift.io/event-type": "push",
					},
					Annotations: map[string]string{
						"test.appstudio.openshift.io/keep-snapshot": "true",
					},
				},
			}
			snapDiscardNonPR := applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "associated-snapshot",
					Namespace: "ns1",
					Labels: map[string]string{
						"pac.test.appstudio.openshift.io/type": "push",
					},
				},
			}
			snapKeepPR := applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "associated-snapshot",
					Namespace: "ns1",
					Labels: map[string]string{
						"pac.test.appstudio.openshift.io/event-type": "pull",
					},
					Annotations: map[string]string{
						"test.appstudio.openshift.io/keep-snapshot": "true",
					},
				},
			}
			snapDiscardPR := applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "associated-snapshot",
					Namespace: "ns1",
					Labels: map[string]string{
						"pac.test.appstudio.openshift.io/type": "pull",
					},
				},
			}

			filteredSnapshotList, keptPr, keptNonPr := filterSnapshotsWithKeepSnapshotAnnotation([]applicationapiv1alpha1.Snapshot{snapKeepNonPR, snapDiscardNonPR, snapKeepPR, snapDiscardPR})
			Expect(filteredSnapshotList).Should(ContainElement(snapDiscardNonPR))
			Expect(filteredSnapshotList).Should(ContainElement(snapDiscardPR))
			Expect(keptPr).To(Equal(1))
			Expect(keptNonPr).To(Equal(1))
			Expect(filteredSnapshotList).To(HaveLen(2))

		})
	})

	Describe("Test isNonPrSnapshotFunction", func() {
		It("Returns true if snapshot is override snapshot", func() {
			snap := applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "associated-snapshot",
					Namespace: "ns1",
					Labels: map[string]string{
						"test.appstudio.openshift.io/type": "override",
					},
				},
			}

			nonPr := isNonPrSnapshot(snap)
			Expect(nonPr).To(BeTrue())

		})

		It("Returns true if event-type is 'push'", func() {
			snap := applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "associated-snapshot",
					Namespace: "ns1",
					Labels: map[string]string{
						"pac.test.appstudio.openshift.io/event-type": "push",
					},
				},
			}

			nonPr := isNonPrSnapshot(snap)
			Expect(nonPr).To(BeTrue())
		})

		It("Returns true if event-type is 'Push'", func() {
			snap := applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "associated-snapshot",
					Namespace: "ns1",
					Labels: map[string]string{
						"pac.test.appstudio.openshift.io/event-type": "Push",
					},
				},
			}

			nonPr := isNonPrSnapshot(snap)
			Expect(nonPr).To(BeTrue())
		})

		It("Returns true if event-type label does not exist", func() {
			snap := applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "associated-snapshot",
					Namespace: "ns1",
				},
			}

			nonPr := isNonPrSnapshot(snap)
			Expect(nonPr).To(BeTrue())
		})

		It("Returns false if snapshot is not override and event-typep label is not send to 'push' or 'Push'", func() {
			snap := applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "associated-snapshot",
					Namespace: "ns1",
					Labels: map[string]string{
						"pac.test.appstudio.openshift.io/event-type": "pull",
					},
				},
			}

			nonPr := isNonPrSnapshot(snap)
			Expect(nonPr).To(BeFalse())
		})
	})

	Describe("Test getSnapshotsForRemoval with minSnapShotsToKeepPerComponent", func() {
		It("Keeps the latest 5 snapshots per component", func() {
			currentTime := time.Now()

			// Component A: 11 push snapshots (should preserve latest 5)
			compASnap0 := &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "comp-a-snap-0",
					Namespace:         "ns1",
					CreationTimestamp: metav1.NewTime(currentTime),
					Labels: map[string]string{
						"pac.test.appstudio.openshift.io/event-type": "push",
						"test.appstudio.openshift.io/type":           "component",
						"appstudio.openshift.io/component":           "component-a"},
				},
			}

			compASnap1 := &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "comp-a-snap-1",
					Namespace:         "ns1",
					CreationTimestamp: metav1.NewTime(currentTime.Add(time.Hour * 1)),
					Labels: map[string]string{
						"pac.test.appstudio.openshift.io/event-type": "push",
						"test.appstudio.openshift.io/type":           "component",
						"appstudio.openshift.io/component":           "component-a"},
				},
			}

			compASnap2 := &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "comp-a-snap-2",
					Namespace:         "ns1",
					CreationTimestamp: metav1.NewTime(currentTime.Add(time.Hour * 2)),
					Labels: map[string]string{
						"pac.test.appstudio.openshift.io/event-type": "push",
						"test.appstudio.openshift.io/type":           "component",
						"appstudio.openshift.io/component":           "component-a"},
				},
			}

			compASnap3 := &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "comp-a-snap-3",
					Namespace:         "ns1",
					CreationTimestamp: metav1.NewTime(currentTime.Add(time.Hour * 3)),
					Labels: map[string]string{
						"pac.test.appstudio.openshift.io/event-type": "push",
						"test.appstudio.openshift.io/type":           "component",
						"appstudio.openshift.io/component":           "component-a"},
				},
			}

			compASnap4 := &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "comp-a-snap-4",
					Namespace:         "ns1",
					CreationTimestamp: metav1.NewTime(currentTime.Add(time.Hour * 4)),
					Labels: map[string]string{
						"pac.test.appstudio.openshift.io/event-type": "push",
						"test.appstudio.openshift.io/type":           "component",
						"appstudio.openshift.io/component":           "component-a"},
				},
			}

			compASnap5 := &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "comp-a-snap-5",
					Namespace:         "ns1",
					CreationTimestamp: metav1.NewTime(currentTime.Add(time.Hour * 5)),
					Labels: map[string]string{
						"pac.test.appstudio.openshift.io/event-type": "push",
						"test.appstudio.openshift.io/type":           "component",
						"appstudio.openshift.io/component":           "component-a"},
				},
			}

			compASnap6 := &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "comp-a-snap-6",
					Namespace:         "ns1",
					CreationTimestamp: metav1.NewTime(currentTime.Add(time.Hour * 6)),
					Labels: map[string]string{
						"pac.test.appstudio.openshift.io/event-type": "push",
						"test.appstudio.openshift.io/type":           "component",
						"appstudio.openshift.io/component":           "component-a"},
				},
			}

			compASnap7 := &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "comp-a-snap-7",
					Namespace:         "ns1",
					CreationTimestamp: metav1.NewTime(currentTime.Add(time.Hour * 7)),
					Labels: map[string]string{
						"pac.test.appstudio.openshift.io/event-type": "push",
						"test.appstudio.openshift.io/type":           "component",
						"appstudio.openshift.io/component":           "component-a"},
				},
			}

			compASnap8 := &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "comp-a-snap-8",
					Namespace:         "ns1",
					CreationTimestamp: metav1.NewTime(currentTime.Add(time.Hour * 8)),
					Labels: map[string]string{
						"pac.test.appstudio.openshift.io/event-type": "push",
						"test.appstudio.openshift.io/type":           "component",
						"appstudio.openshift.io/component":           "component-a"},
				},
			}

			compASnap9 := &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "comp-a-snap-9",
					Namespace:         "ns1",
					CreationTimestamp: metav1.NewTime(currentTime.Add(time.Hour * 9)),
					Labels: map[string]string{
						"pac.test.appstudio.openshift.io/event-type": "push",
						"test.appstudio.openshift.io/type":           "component",
						"appstudio.openshift.io/component":           "component-a"},
				},
			}

			compASnap10 := &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "comp-a-snap-10",
					Namespace:         "ns1",
					CreationTimestamp: metav1.NewTime(currentTime.Add(time.Hour * 10)),
					Labels: map[string]string{
						"pac.test.appstudio.openshift.io/event-type": "push",
						"test.appstudio.openshift.io/type":           "component",
						"appstudio.openshift.io/component":           "component-a"},
				},
			}

			// Component B has 3 push snapshots

			compBSnap0 := &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "comp-b-snap-0",
					Namespace:         "ns1",
					CreationTimestamp: metav1.NewTime(currentTime),
					Labels: map[string]string{
						"pac.test.appstudio.openshift.io/event-type": "push",
						"test.appstudio.openshift.io/type":           "component",
						"appstudio.openshift.io/component":           "component-b"},
				},
			}

			compBSnap1 := &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "comp-b-snap-1",
					Namespace:         "ns1",
					CreationTimestamp: metav1.NewTime(currentTime.Add(time.Hour * 1)),
					Labels: map[string]string{
						"pac.test.appstudio.openshift.io/event-type": "push",
						"test.appstudio.openshift.io/type":           "component",
						"appstudio.openshift.io/component":           "component-b"},
				},
			}

			compBSnap2 := &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "comp-b-snap-2",
					Namespace:         "ns1",
					CreationTimestamp: metav1.NewTime(currentTime.Add(time.Hour * 2)),
					Labels: map[string]string{
						"pac.test.appstudio.openshift.io/event-type": "push",
						"test.appstudio.openshift.io/type":           "component",
						"appstudio.openshift.io/component":           "component-b"},
				},
			}

			cl := fake.NewClientBuilder().
				WithScheme(scheme).
				WithLists(
					&applicationapiv1alpha1.SnapshotList{
						Items: []applicationapiv1alpha1.Snapshot{
							*compASnap0, *compASnap1, *compASnap2, *compASnap3, *compASnap4, *compASnap5, *compASnap6, *compASnap7, *compASnap8, *compASnap9, *compASnap10,
							*compBSnap0, *compBSnap1, *compBSnap2,
						},
					},
				).Build()
			candidates := []applicationapiv1alpha1.Snapshot{
				*compASnap0, *compASnap1, *compASnap2, *compASnap3, *compASnap4, *compASnap5, *compASnap6, *compASnap7, *compASnap8, *compASnap9, *compASnap10,
				*compBSnap0, *compBSnap1, *compBSnap2,
			}
			output := getSnapshotsForRemoval(cl, candidates, 512, 0, 5, logger)

			// Total: 6 snapshots should be marked for deletion (all from Component A)
			Expect(output).To(HaveLen(6))

			// Component A: oldest 6 should be deleted
			Expect(output).To(ContainElement(*compASnap0))
			Expect(output).To(ContainElement(*compASnap1))
			Expect(output).To(ContainElement(*compASnap2))
			Expect(output).To(ContainElement(*compASnap3))
			Expect(output).To(ContainElement(*compASnap4))
			Expect(output).To(ContainElement(*compASnap5))

			// Component A: latest 5 snapshots should be preserved (NOT in deletion list)
			Expect(output).NotTo(ContainElement(*compASnap6))
			Expect(output).NotTo(ContainElement(*compASnap7))
			Expect(output).NotTo(ContainElement(*compASnap8))
			Expect(output).NotTo(ContainElement(*compASnap9))
			Expect(output).NotTo(ContainElement(*compASnap10))

			// Component B: all 3 push snapshots should be preserved (NOT in deletion list)
			Expect(output).NotTo(ContainElement(*compBSnap0))
			Expect(output).NotTo(ContainElement(*compBSnap1))
			Expect(output).NotTo(ContainElement(*compBSnap2))

		})

		// When mixed with pull snapshots, only the latest 5 push snapshots should be preserved
		// the pull snapshots should follow the global limits which are set when calling getSnapshotsForRemoval()
		It("Only preserves latest 5 push snapshots when mixed with pull snapshots", func() {
			currentTime := time.Now()
			// Component A: Mix of push and PR snapshots
			// Push snapshots (should be preserved per-component)

			compAPushSnap0 := &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "comp-a-push-0",
					Namespace:         "ns1",
					CreationTimestamp: metav1.NewTime(currentTime),
					Labels: map[string]string{
						"pac.test.appstudio.openshift.io/event-type": "push",
						"test.appstudio.openshift.io/type":           "component",
						"appstudio.openshift.io/component":           "component-a"},
				},
			}

			compAPushSnap1 := &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "comp-a-push-1",
					Namespace:         "ns1",
					CreationTimestamp: metav1.NewTime(currentTime.Add(time.Hour * 1)),
					Labels: map[string]string{
						"pac.test.appstudio.openshift.io/event-type": "push",
						"test.appstudio.openshift.io/type":           "component",
						"appstudio.openshift.io/component":           "component-a"},
				},
			}

			compAPushSnap2 := &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "comp-a-push-2",
					Namespace:         "ns1",
					CreationTimestamp: metav1.NewTime(currentTime.Add(time.Hour * 2)),
					Labels: map[string]string{
						"pac.test.appstudio.openshift.io/event-type": "push",
						"test.appstudio.openshift.io/type":           "component",
						"appstudio.openshift.io/component":           "component-a"},
				},
			}

			compAPushSnap3 := &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "comp-a-push-3",
					Namespace:         "ns1",
					CreationTimestamp: metav1.NewTime(currentTime.Add(time.Hour * 3)),
					Labels: map[string]string{
						"pac.test.appstudio.openshift.io/event-type": "push",
						"test.appstudio.openshift.io/type":           "component",
						"appstudio.openshift.io/component":           "component-a"},
				},
			}

			compAPushSnap4 := &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "comp-a-push-4",
					Namespace:         "ns1",
					CreationTimestamp: metav1.NewTime(currentTime.Add(time.Hour * 4)),
					Labels: map[string]string{
						"pac.test.appstudio.openshift.io/event-type": "push",
						"test.appstudio.openshift.io/type":           "component",
						"appstudio.openshift.io/component":           "component-a"},
				},
			}

			compAPushSnap5 := &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "comp-a-push-5",
					Namespace:         "ns1",
					CreationTimestamp: metav1.NewTime(currentTime.Add(time.Hour * 5)),
					Labels: map[string]string{
						"pac.test.appstudio.openshift.io/event-type": "push",
						"test.appstudio.openshift.io/type":           "component",
						"appstudio.openshift.io/component":           "component-a"},
				},
			}

			// PR snapshots (should NOT be preserved per-component, follow global limits)
			compAPRSnap0 := &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "comp-a-pr-0",
					Namespace:         "ns1",
					CreationTimestamp: metav1.NewTime(currentTime.Add(time.Hour * 6)),
					Labels: map[string]string{
						"pac.test.appstudio.openshift.io/event-type": "pull_request",
						"test.appstudio.openshift.io/type":           "component",
						"appstudio.openshift.io/component":           "component-a"},
				},
			}

			compAPRSnap1 := &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "comp-a-pr-1",
					Namespace:         "ns1",
					CreationTimestamp: metav1.NewTime(currentTime.Add(time.Hour * 7)),
					Labels: map[string]string{
						"pac.test.appstudio.openshift.io/event-type": "pull_request",
						"test.appstudio.openshift.io/type":           "component",
						"appstudio.openshift.io/component":           "component-a"},
				},
			}

			cl := fake.NewClientBuilder().
				WithScheme(scheme).
				WithLists(
					&applicationapiv1alpha1.SnapshotList{
						Items: []applicationapiv1alpha1.Snapshot{
							*compAPushSnap0, *compAPushSnap1, *compAPushSnap2, *compAPushSnap3, *compAPushSnap4, *compAPushSnap5,
							*compAPRSnap0, *compAPRSnap1,
						},
					},
				).Build()

			candidates := []applicationapiv1alpha1.Snapshot{
				*compAPushSnap0, *compAPushSnap1, *compAPushSnap2, *compAPushSnap3, *compAPushSnap4, *compAPushSnap5,
				*compAPRSnap0, *compAPRSnap1,
			}

			// Global limits: 0 PR snapshots, 0 non-PR snapshots + per-component limit of 5 push snapshots
			output := getSnapshotsForRemoval(cl, candidates, 0, 0, 5, logger)

			// Push snapshots: oldest 1 should be deleted (compAPushSnap0), latest 5 preserved (per-component)
			Expect(output).To(ContainElement(*compAPushSnap0))
			Expect(output).NotTo(ContainElement(*compAPushSnap1))
			Expect(output).NotTo(ContainElement(*compAPushSnap2))
			Expect(output).NotTo(ContainElement(*compAPushSnap3))
			Expect(output).NotTo(ContainElement(*compAPushSnap4))
			Expect(output).NotTo(ContainElement(*compAPushSnap5))

			// PR snapshots: both should be deleted (global limit set to 0)
			Expect(output).To(ContainElement(*compAPRSnap0))
			Expect(output).To(ContainElement(*compAPRSnap1))

			// Total: 3 snapshots should be deleted (1 oldeest push + 2 PR)
			Expect(output).To(HaveLen(3))
		})
	})
})
