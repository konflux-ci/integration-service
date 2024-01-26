package main

import (
	"fmt"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	releasev1alpha1 "github.com/redhat-appstudio/release-service/api/v1alpha1"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("Test garbage collection for snapshots", func() {
	var logger logr.Logger

	BeforeEach(func() {
		zapLog, err := zap.NewDevelopment()
		if err != nil {
			panic(fmt.Sprintf("who watches the watchmen (%v)?", err))
		}
		logger = zapr.NewLogger(zapLog)
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

	Describe("Test getSnapshotsForNSBindings", func() {
		It("Finds no snapshot when there are no bindings", func() {

			cl := fake.NewClientBuilder().
				WithScheme(scheme).
				WithLists(
					&applicationapiv1alpha1.SnapshotEnvironmentBindingList{
						Items: []applicationapiv1alpha1.SnapshotEnvironmentBinding{},
					}).Build()
			snapToData := make(map[string]snapshotData)
			output, err := getSnapshotsForNSBindings(cl, snapToData, "ns1", logger)

			Expect(err).ShouldNot(HaveOccurred())
			Expect(output).To(BeEmpty())
		})

		It("Finds a snapshot for a single binding", func() {
			bind1 := &applicationapiv1alpha1.SnapshotEnvironmentBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-binding",
					Namespace: "ns1",
				},
				Spec: applicationapiv1alpha1.SnapshotEnvironmentBindingSpec{
					Snapshot: "my-snapshot",
				},
			}
			bind2 := &applicationapiv1alpha1.SnapshotEnvironmentBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "another-ns-binding",
					Namespace: "another-ns",
				},
				Spec: applicationapiv1alpha1.SnapshotEnvironmentBindingSpec{
					Snapshot: "another-snapshot",
				},
			}

			cl := fake.NewClientBuilder().
				WithScheme(scheme).
				WithLists(
					&applicationapiv1alpha1.SnapshotEnvironmentBindingList{
						Items: []applicationapiv1alpha1.SnapshotEnvironmentBinding{*bind1, *bind2},
					}).Build()
			snapToData := make(map[string]snapshotData)
			output, err := getSnapshotsForNSBindings(cl, snapToData, "ns1", logger)

			Expect(err).ShouldNot(HaveOccurred())
			Expect(output).To(HaveLen(1))
			Expect(output["my-snapshot"].environmentBinding.Name).To(Equal("my-binding"))
		})

		It("Finds multiple snapshots for multiple bindings", func() {
			bind1 := &applicationapiv1alpha1.SnapshotEnvironmentBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "bind1",
					Namespace: "ns1",
				},
				Spec: applicationapiv1alpha1.SnapshotEnvironmentBindingSpec{
					Snapshot: "snap1",
				},
			}
			bind2 := &applicationapiv1alpha1.SnapshotEnvironmentBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "bind2",
					Namespace: "ns1",
				},
				Spec: applicationapiv1alpha1.SnapshotEnvironmentBindingSpec{
					Snapshot: "snap2",
				},
			}

			cl := fake.NewClientBuilder().
				WithScheme(scheme).
				WithLists(
					&applicationapiv1alpha1.SnapshotEnvironmentBindingList{
						Items: []applicationapiv1alpha1.SnapshotEnvironmentBinding{*bind1, *bind2},
					}).Build()
			snapToData := make(map[string]snapshotData)
			output, err := getSnapshotsForNSBindings(cl, snapToData, "ns1", logger)

			Expect(err).ShouldNot(HaveOccurred())
			Expect(output).To(HaveLen(2))
			Expect(output["snap1"].environmentBinding.Name).To(Equal("bind1"))
			Expect(output["snap2"].environmentBinding.Name).To(Equal("bind2"))
		})

		It("Returns error when fails API call", func() {

			cl := fake.NewClientBuilder().Build()
			snapToData := make(map[string]snapshotData)
			_, err := getSnapshotsForNSBindings(cl, snapToData, "ns1", logger)

			Expect(err).Should(HaveOccurred())
			Expect(err).To(MatchError(ContainSubstring(
				"no kind is registered for the type " +
					"v1alpha1.SnapshotEnvironmentBindingList in scheme",
			)))
		})
	})
})
