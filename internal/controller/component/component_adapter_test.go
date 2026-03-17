package component

import (
	"bytes"
	"reflect"
	"time"

	"github.com/tonglil/buflogr"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	"github.com/konflux-ci/integration-service/gitops"
	"github.com/konflux-ci/integration-service/loader"
	toolkit "github.com/konflux-ci/operator-toolkit/loader"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/konflux-ci/integration-service/helpers"
	"k8s.io/apimachinery/pkg/api/errors"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Component Adapter", Ordered, func() {
	var (
		adapter *Adapter
		logger  helpers.IntegrationLogger

		hasApp   *applicationapiv1alpha1.Application
		hasComp  *applicationapiv1alpha1.Component
		hasComp2 *applicationapiv1alpha1.Component
	)
	const (
		SampleCommit             = "a2ba645d50e471d5f084b"
		SampleRepoLink           = "https://github.com/devfile-samples/devfile-sample-java-springboot-basic"
		sample_revision          = "random-value"
		SampleDigest             = "sha256:841328df1b9f8c4087adbdcfec6cc99ac8308805dea83f6d415d6fb8d40227c1"
		SampleImageWithoutDigest = "quay.io/redhat-appstudio/sample-image"
		SampleImage              = SampleImageWithoutDigest + "@" + SampleDigest
	)

	BeforeAll(func() {

		hasApp = &applicationapiv1alpha1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "application-sample",
				Namespace: "default",
			},
			Spec: applicationapiv1alpha1.ApplicationSpec{
				DisplayName: "application-sample",
				Description: "This is an example application",
			},
		}
		Expect(k8sClient.Create(ctx, hasApp)).Should(Succeed())

		hasComp = &applicationapiv1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "component-sample",
				Namespace: "default",
			},
			Spec: applicationapiv1alpha1.ComponentSpec{
				ComponentName:  "component-sample-2",
				Application:    hasApp.Name,
				ContainerImage: SampleImage,
				Source: applicationapiv1alpha1.ComponentSource{
					ComponentSourceUnion: applicationapiv1alpha1.ComponentSourceUnion{
						GitSource: &applicationapiv1alpha1.GitSource{
							URL:      SampleRepoLink,
							Revision: SampleCommit,
						},
					},
				},
			},
			Status: applicationapiv1alpha1.ComponentStatus{
				LastBuiltCommit:   "",
				LastPromotedImage: SampleImage,
			},
		}
		Expect(k8sClient.Create(ctx, hasComp)).Should(Succeed())

		hasComp2 = &applicationapiv1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "component-second-sample",
				Namespace: "default",
			},
			Spec: applicationapiv1alpha1.ComponentSpec{
				ComponentName:  "component-second-sample",
				Application:    "application-sample",
				ContainerImage: SampleImage,
				Source: applicationapiv1alpha1.ComponentSource{
					ComponentSourceUnion: applicationapiv1alpha1.ComponentSourceUnion{
						GitSource: &applicationapiv1alpha1.GitSource{
							URL:      SampleRepoLink,
							Revision: "revision",
						},
					},
				},
			},
			Status: applicationapiv1alpha1.ComponentStatus{
				LastBuiltCommit:   "",
				LastPromotedImage: SampleImage,
			},
		}
		Expect(k8sClient.Create(ctx, hasComp2)).Should(Succeed())
	})

	AfterEach(func() {
		// Clean up any snapshots created during tests
		snapshots := &applicationapiv1alpha1.SnapshotList{}
		Expect(k8sClient.List(ctx, snapshots, &client.ListOptions{Namespace: "default"})).To(Succeed())
		for _, snapshot := range snapshots.Items {
			Expect(k8sClient.Delete(ctx, &snapshot)).To(Succeed())
		}
	})

	AfterAll(func() {
		err := k8sClient.Delete(ctx, hasApp)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, hasComp)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, hasComp2)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
	})

	It("can create a new Adapter instance [APPLICATION]", func() {
		Expect(reflect.TypeOf(NewAdapter(ctx, hasComp, hasApp, logger, loader.NewMockLoader(), k8sClient))).To(Equal(reflect.TypeOf(&Adapter{})))
	})

	It("can create a new Adapter instance with nil application for ComponentGroup scenario", func() {
		Expect(reflect.TypeOf(NewAdapter(ctx, hasComp, nil, logger, loader.NewMockLoader(), k8sClient))).To(Equal(reflect.TypeOf(&Adapter{})))
	})
	It("ensures removing a component will result in a new snapshot being created [APPLICATION]", func() {
		buf := bytes.Buffer{}

		log := helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(&buf)}
		adapter = NewAdapter(ctx, hasComp, hasApp, log, loader.NewMockLoader(), k8sClient)
		adapter.context = toolkit.GetMockedContext(ctx, []toolkit.MockData{
			{
				ContextKey: loader.ApplicationContextKey,
				Resource:   hasApp,
			},
			{
				ContextKey: loader.ApplicationComponentsContextKey,
				Resource:   []applicationapiv1alpha1.Component{*hasComp, *hasComp2},
			},
		})
		snapshots := &applicationapiv1alpha1.SnapshotList{}
		Eventually(func() bool {
			Expect(k8sClient.List(ctx, snapshots, &client.ListOptions{Namespace: hasApp.Namespace})).To(Succeed())
			return len(snapshots.Items) == 0
		}, time.Second*20).Should(BeTrue())

		now := metav1.NewTime(metav1.Now().Add(time.Second * 1))
		hasComp.SetDeletionTimestamp(&now)

		result, err := adapter.EnsureComponentIsCleanedUp()

		Eventually(func() bool {
			Expect(k8sClient.List(ctx, snapshots, &client.ListOptions{Namespace: hasApp.Namespace})).To(Succeed())

			// check if the snapshot is labeled with auto-release=false
			Expect(snapshots.Items).To(HaveLen(1))
			Expect(snapshots.Items[0].Labels[gitops.AutoReleaseLabel]).To(Equal("false"))

			return !result.CancelRequest && len(snapshots.Items) == 1 && err == nil
		}, time.Second*30).Should(BeTrue())
	})

	It("ensures removing a component in ComponentGroup scenario skips Application-specific cleanup", func() {
		log := helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(&bytes.Buffer{})}
		adapter = NewAdapter(ctx, hasComp, nil, log, loader.NewMockLoader(), k8sClient)

		snapshots := &applicationapiv1alpha1.SnapshotList{}
		Eventually(func() bool {
			Expect(k8sClient.List(ctx, snapshots, &client.ListOptions{Namespace: hasComp.Namespace})).To(Succeed())
			return len(snapshots.Items) == 0
		}, time.Second*20).Should(BeTrue())

		now := metav1.NewTime(metav1.Now().Add(time.Second * 1))
		hasComp.SetDeletionTimestamp(&now)

		result, err := adapter.EnsureComponentIsCleanedUp()

		// Should continue processing without creating snapshots
		Expect(err).ToNot(HaveOccurred())
		Expect(result.CancelRequest).To(BeFalse())

		// No snapshots should be created for ComponentGroup scenarios
		Eventually(func() bool {
			Expect(k8sClient.List(ctx, snapshots, &client.ListOptions{Namespace: hasComp.Namespace})).To(Succeed())
			return len(snapshots.Items) == 0
		}, time.Second*10).Should(BeTrue())
	})

})
