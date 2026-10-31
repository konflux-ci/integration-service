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

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/konflux-ci/integration-service/helpers"
	tektonconsts "github.com/konflux-ci/integration-service/tekton/consts"
	"k8s.io/apimachinery/pkg/api/errors"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
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
				LastBuiltCommit: "",
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
		}
		Expect(k8sClient.Create(ctx, hasComp2)).Should(Succeed())
	})

	AfterAll(func() {
		err := k8sClient.Delete(ctx, hasApp)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, hasComp)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, hasComp2)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
	})

	It("can create a new Adapter instance", func() {
		Expect(reflect.TypeOf(NewAdapter(ctx, hasComp, hasApp, logger, loader.NewMockLoader(), k8sClient))).To(Equal(reflect.TypeOf(&Adapter{})))
	})
	It("ensures removing a component will result in a new snapshot being created", func() {
		log := helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(&bytes.Buffer{})}
		adapter = NewAdapter(ctx, hasComp, hasApp, log, loader.NewMockLoader(), k8sClient)
		snapshots := &applicationapiv1alpha1.SnapshotList{}
		Eventually(func() bool {
			Expect(k8sClient.List(ctx, snapshots, &client.ListOptions{Namespace: hasApp.Namespace})).To(Succeed())
			return len(snapshots.Items) == 0
		}, time.Second*20).Should(BeTrue())

		now := metav1.NewTime(metav1.Now().Add(time.Second * 1))
		hasComp.SetDeletionTimestamp(&now)

		// Use explicit specs for the loader mock: cluster Create may mutate in-memory Components (e.g. Git
		// source shape), which makes HaveGitSourceInComponent flaky on *hasComp2. The adapter only needs a
		// stable remaining component with git metadata for the post-deletion snapshot.
		mockApplicationComponents := []applicationapiv1alpha1.Component{
			{
				ObjectMeta: metav1.ObjectMeta{Name: hasComp.Name, Namespace: hasComp.Namespace},
				Spec: applicationapiv1alpha1.ComponentSpec{
					Application: hasApp.Name,
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: hasComp2.Name, Namespace: hasComp2.Namespace},
				Spec: applicationapiv1alpha1.ComponentSpec{
					ComponentName:  hasComp2.Spec.ComponentName,
					Application:    hasApp.Name,
					ContainerImage: SampleImage,
					Source: applicationapiv1alpha1.ComponentSource{
						ComponentSourceUnion: applicationapiv1alpha1.ComponentSourceUnion{
							GitSource: &applicationapiv1alpha1.GitSource{
								URL:      SampleRepoLink,
								Revision: sample_revision,
							},
						},
					},
				},
				Status: applicationapiv1alpha1.ComponentStatus{
					LastPromotedImage: SampleImage,
				},
			},
		}
		adapter.context = toolkit.GetMockedContext(ctx, []toolkit.MockData{
			{
				ContextKey: loader.ApplicationContextKey,
				Resource:   hasApp,
			},
			{
				ContextKey: loader.ApplicationComponentsContextKey,
				Resource:   mockApplicationComponents,
			},
			{
				ContextKey: loader.GetComponentContextKey,
				Resource:   hasComp,
			},
		})

		result, err := adapter.EnsureComponentIsCleanedUp()
		Expect(err).ToNot(HaveOccurred())
		Expect(result.CancelRequest).To(BeFalse())

		Eventually(func(g Gomega) {
			g.Expect(k8sClient.List(ctx, snapshots, &client.ListOptions{Namespace: hasApp.Namespace})).To(Succeed())
			g.Expect(snapshots.Items).To(HaveLen(1))
			g.Expect(snapshots.Items[0].Labels[gitops.AutoReleaseLabel]).To(Equal("false"))
		}).WithTimeout(time.Second * 30).Should(Succeed())
	})

	// STONEINTG-1799: the per-component integration pipeline ServiceAccount. These specs use their own
	// Components because the spec above marks hasComp for deletion in memory. envtest has no garbage
	// collector, so AfterAll deletes every object by name.
	When("ensuring the per-component integration pipeline ServiceAccount", Ordered, func() {
		var (
			saComp      *applicationapiv1alpha1.Component
			saCompOther *applicationapiv1alpha1.Component
			saCompStale *applicationapiv1alpha1.Component
			saCompRB    *applicationapiv1alpha1.Component
			saLog       helpers.IntegrationLogger
		)
		const staleUID = "11111111-2222-3333-4444-555555555555"

		newSAComponent := func(name string) *applicationapiv1alpha1.Component {
			return &applicationapiv1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
				Spec: applicationapiv1alpha1.ComponentSpec{
					ComponentName: name,
					Application:   hasApp.Name,
					Source: applicationapiv1alpha1.ComponentSource{
						ComponentSourceUnion: applicationapiv1alpha1.ComponentSourceUnion{
							GitSource: &applicationapiv1alpha1.GitSource{URL: SampleRepoLink},
						},
					},
				},
			}
		}
		accountName := func(comp *applicationapiv1alpha1.Component) string {
			return helpers.ComponentIntegrationPipelineServiceAccountName(comp.Name)
		}
		getSA := func(name string) (*corev1.ServiceAccount, error) {
			sa := &corev1.ServiceAccount{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, sa)
			return sa, err
		}
		getRB := func(name string) (*rbacv1.RoleBinding, error) {
			rb := &rbacv1.RoleBinding{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, rb)
			return rb, err
		}
		waitVisible := func(obj client.Object) {
			// k8sClient reads from the manager cache; wait for a freshly created object to arrive there.
			key := types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}
			Eventually(func() error {
				return k8sClient.Get(ctx, key, obj.DeepCopyObject().(client.Object))
			}, time.Second*10).Should(Succeed())
		}
		deleteIgnoringNotFound := func(obj client.Object) {
			err := k8sClient.Delete(ctx, obj)
			Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		}

		BeforeAll(func() {
			saLog = helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(&bytes.Buffer{})}
			saComp = newSAComponent("component-sa-sample")
			saCompOther = newSAComponent("component-sa-other")
			saCompStale = newSAComponent("component-sa-stale")
			saCompRB = newSAComponent("component-sa-rb-stale")
			for _, comp := range []*applicationapiv1alpha1.Component{saComp, saCompOther, saCompStale, saCompRB} {
				Expect(k8sClient.Create(ctx, comp)).Should(Succeed())
				waitVisible(comp)
			}
		})

		AfterAll(func() {
			for _, comp := range []*applicationapiv1alpha1.Component{saComp, saCompOther, saCompStale, saCompRB} {
				name := accountName(comp)
				deleteIgnoringNotFound(&corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"}})
				deleteIgnoringNotFound(&rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"}})
				deleteIgnoringNotFound(comp)
			}
		})

		It("creates the ServiceAccount and the RoleBinding, both with an owner reference to the Component", func() {
			adapter = NewAdapter(ctx, saComp, hasApp, saLog, loader.NewMockLoader(), k8sClient)
			result, err := adapter.EnsureComponentServiceAccountCreated()
			Expect(err).ToNot(HaveOccurred())
			Expect(result.RequeueRequest).To(BeFalse())
			Expect(result.CancelRequest).To(BeFalse())

			Eventually(func(g Gomega) {
				sa, err := getSA(accountName(saComp))
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(sa.OwnerReferences).To(HaveLen(1))
				g.Expect(sa.OwnerReferences[0].UID).To(Equal(saComp.UID))
				g.Expect(sa.OwnerReferences[0].Kind).To(Equal("Component"))
				g.Expect(sa.OwnerReferences[0].APIVersion).To(Equal(applicationapiv1alpha1.GroupVersion.String()))
				g.Expect(sa.OwnerReferences[0].Controller).To(BeNil())

				rb, err := getRB(accountName(saComp))
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(rb.OwnerReferences).To(HaveLen(1))
				g.Expect(rb.OwnerReferences[0].UID).To(Equal(saComp.UID))
				g.Expect(rb.OwnerReferences[0].Controller).To(BeNil())
			}, time.Second*10).Should(Succeed())
		})

		It("binds the account to the konflux-integration-runner ClusterRole with the account as the only subject", func() {
			rb, err := getRB(accountName(saComp))
			Expect(err).ToNot(HaveOccurred())
			Expect(rb.RoleRef.APIGroup).To(Equal(rbacv1.GroupName))
			Expect(rb.RoleRef.Kind).To(Equal("ClusterRole"))
			Expect(rb.RoleRef.Name).To(Equal(tektonconsts.DefaultIntegrationPipelineClusterRoleName))
			Expect(rb.Subjects).To(HaveLen(1))
			Expect(rb.Subjects[0].Kind).To(Equal(rbacv1.ServiceAccountKind))
			Expect(rb.Subjects[0].Name).To(Equal(accountName(saComp)))
			Expect(rb.Subjects[0].Namespace).To(Equal("default"))
		})

		It("is idempotent on a second call and keeps a single owner reference", func() {
			result, err := adapter.EnsureComponentServiceAccountCreated()
			Expect(err).ToNot(HaveOccurred())
			Expect(result.RequeueRequest).To(BeFalse())

			sa, err := getSA(accountName(saComp))
			Expect(err).ToNot(HaveOccurred())
			Expect(sa.OwnerReferences).To(HaveLen(1))
			rb, err := getRB(accountName(saComp))
			Expect(err).ToNot(HaveOccurred())
			Expect(rb.OwnerReferences).To(HaveLen(1))
		})

		It("leaves an existing object alone when it has no owner or another Component owns it", func() {
			bare := &rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{Name: accountName(saCompOther), Namespace: "default"},
				RoleRef: rbacv1.RoleRef{
					APIGroup: rbacv1.GroupName,
					Kind:     "ClusterRole",
					Name:     tektonconsts.DefaultIntegrationPipelineClusterRoleName,
				},
				Subjects: []rbacv1.Subject{{Kind: rbacv1.ServiceAccountKind, Name: accountName(saCompOther), Namespace: "default"}},
			}
			Expect(k8sClient.Create(ctx, bare)).Should(Succeed())
			waitVisible(bare)
			// and a ServiceAccount that another Component already owns
			owned := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: accountName(saCompOther), Namespace: "default"}}
			Expect(controllerutil.SetOwnerReference(hasComp2, owned, k8sClient.Scheme())).To(Succeed())
			Expect(k8sClient.Create(ctx, owned)).Should(Succeed())
			waitVisible(owned)

			adapter = NewAdapter(ctx, saCompOther, hasApp, saLog, loader.NewMockLoader(), k8sClient)
			result, err := adapter.EnsureComponentServiceAccountCreated()
			Expect(err).ToNot(HaveOccurred())
			Expect(result.RequeueRequest).To(BeFalse())

			Consistently(func(g Gomega) {
				rb, err := getRB(accountName(saCompOther))
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(rb.OwnerReferences).To(BeEmpty())
				sa, err := getSA(accountName(saCompOther))
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(sa.OwnerReferences).To(HaveLen(1))
				g.Expect(sa.OwnerReferences[0].UID).To(Equal(hasComp2.UID))
			}, time.Second*3).Should(Succeed())
		})

		It("requeues when the only owner is a previous Component of the same name and changes nothing", func() {
			stale := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{
				Name:      accountName(saCompStale),
				Namespace: "default",
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion: applicationapiv1alpha1.GroupVersion.String(),
					Kind:       "Component",
					Name:       saCompStale.Name,
					UID:        staleUID,
				}},
			}}
			Expect(k8sClient.Create(ctx, stale)).Should(Succeed())
			waitVisible(stale)

			adapter = NewAdapter(ctx, saCompStale, hasApp, saLog, loader.NewMockLoader(), k8sClient)
			result, err := adapter.EnsureComponentServiceAccountCreated()
			Expect(err).ToNot(HaveOccurred())
			Expect(result.RequeueRequest).To(BeTrue())
			Expect(result.RequeueDelay).To(Equal(10 * time.Second))

			sa, err := getSA(accountName(saCompStale))
			Expect(err).ToNot(HaveOccurred())
			Expect(sa.OwnerReferences).To(HaveLen(1))
			Expect(sa.OwnerReferences[0].UID).To(Equal(types.UID(staleUID)))
			// the ensure stops at the stale ServiceAccount, so no RoleBinding was created
			_, err = getRB(accountName(saCompStale))
			Expect(errors.IsNotFound(err)).To(BeTrue())
		})

		It("requeues when only the RoleBinding belongs to a previous Component of the same name", func() {
			stale := &rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      accountName(saCompRB),
					Namespace: "default",
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion: applicationapiv1alpha1.GroupVersion.String(),
						Kind:       "Component",
						Name:       saCompRB.Name,
						UID:        staleUID,
					}},
				},
				RoleRef: rbacv1.RoleRef{
					APIGroup: rbacv1.GroupName,
					Kind:     "ClusterRole",
					Name:     tektonconsts.DefaultIntegrationPipelineClusterRoleName,
				},
				Subjects: []rbacv1.Subject{{Kind: rbacv1.ServiceAccountKind, Name: accountName(saCompRB), Namespace: "default"}},
			}
			Expect(k8sClient.Create(ctx, stale)).Should(Succeed())
			waitVisible(stale)

			adapter = NewAdapter(ctx, saCompRB, hasApp, saLog, loader.NewMockLoader(), k8sClient)
			result, err := adapter.EnsureComponentServiceAccountCreated()
			Expect(err).ToNot(HaveOccurred())
			Expect(result.RequeueRequest).To(BeTrue())
			Expect(result.RequeueDelay).To(Equal(10 * time.Second))

			// the ServiceAccount was fresh, so it was created before the RoleBinding stopped the ensure
			Eventually(func(g Gomega) {
				sa, err := getSA(accountName(saCompRB))
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(sa.OwnerReferences).To(HaveLen(1))
				g.Expect(sa.OwnerReferences[0].UID).To(Equal(saCompRB.UID))
			}, time.Second*10).Should(Succeed())
			rb, err := getRB(accountName(saCompRB))
			Expect(err).ToNot(HaveOccurred())
			Expect(rb.OwnerReferences).To(HaveLen(1))
			Expect(rb.OwnerReferences[0].UID).To(Equal(types.UID(staleUID)))
		})

		It("requeues with the error when the ServiceAccount cannot be created", func() {
			// envtest keeps the NamespaceLifecycle admission plugin on, so a create in a namespace
			// that does not exist is refused. The Component itself is never created on the cluster.
			elsewhere := newSAComponent("component-sa-no-namespace")
			elsewhere.Namespace = "namespace-that-does-not-exist"
			adapter = NewAdapter(ctx, elsewhere, hasApp, saLog, loader.NewMockLoader(), k8sClient)
			result, err := adapter.EnsureComponentServiceAccountCreated()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to ensure ServiceAccount"))
			Expect(result.RequeueRequest).To(BeTrue())
			Expect(result.RequeueDelay).To(BeZero())
		})

		It("creates nothing when the Component is marked for deletion", func() {
			now := metav1.Now()
			deleting := newSAComponent("component-sa-deleting") // never created on the cluster
			deleting.SetDeletionTimestamp(&now)
			adapter = NewAdapter(ctx, deleting, hasApp, saLog, loader.NewMockLoader(), k8sClient)
			result, err := adapter.EnsureComponentServiceAccountCreated()
			Expect(err).ToNot(HaveOccurred())
			Expect(result.RequeueRequest).To(BeFalse())
			Expect(result.CancelRequest).To(BeFalse())

			_, err = getSA(accountName(deleting))
			Expect(errors.IsNotFound(err)).To(BeTrue())
		})
	})

})
