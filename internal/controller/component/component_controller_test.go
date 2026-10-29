package component

import (
	"reflect"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
	crwebhook "sigs.k8s.io/controller-runtime/pkg/webhook"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	"github.com/konflux-ci/integration-service/helpers"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientsetscheme "k8s.io/client-go/kubernetes/scheme"
	klog "k8s.io/klog/v2"
)

var _ = Describe("ComponentController", Ordered, func() {
	var (
		manager             ctrl.Manager
		componentReconciler *Reconciler
		scheme              runtime.Scheme
		req                 ctrl.Request
		hasApp              *applicationapiv1alpha1.Application
		hasComp             *applicationapiv1alpha1.Component
	)
	const (
		SampleRepoLink = "https://github.com/devfile-samples/devfile-sample-java-springboot-basic"
	)

	BeforeAll(func() {

		applicationName := "application-sample"

		hasApp = &applicationapiv1alpha1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      applicationName,
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
				ComponentName: "component-sample",
				Application:   applicationName,
				Source: applicationapiv1alpha1.ComponentSource{
					ComponentSourceUnion: applicationapiv1alpha1.ComponentSourceUnion{
						GitSource: &applicationapiv1alpha1.GitSource{
							URL: SampleRepoLink,
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, hasComp)).Should(Succeed())

		req = ctrl.Request{
			NamespacedName: types.NamespacedName{
				Namespace: "default",
				Name:      hasComp.Name,
			},
		}

		webhookInstallOptions := &testEnv.WebhookInstallOptions

		klog.Info(webhookInstallOptions.LocalServingHost)
		klog.Info(webhookInstallOptions.LocalServingPort)
		klog.Info(webhookInstallOptions.LocalServingCertDir)

		var err error
		manager, err = ctrl.NewManager(cfg, ctrl.Options{
			Scheme: clientsetscheme.Scheme,
			WebhookServer: crwebhook.NewServer(crwebhook.Options{
				CertDir: webhookInstallOptions.LocalServingCertDir,
				Host:    webhookInstallOptions.LocalServingHost,
				Port:    webhookInstallOptions.LocalServingPort,
			}),
			Metrics: server.Options{
				BindAddress: "0", // disables metrics
			},
			LeaderElection: false,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(err).ToNot(HaveOccurred())

		componentReconciler = NewComponentReconciler(k8sClient, &logf.Log, &scheme)
	})
	AfterAll(func() {
		err := k8sClient.Delete(ctx, hasApp)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, hasComp)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
	})

	It("can create and return a new Reconciler object", func() {
		Expect(reflect.TypeOf(componentReconciler)).To(Equal(reflect.TypeOf(&Reconciler{})))
	})

	It("creates the account and binding for a Component without an Application and adds no finalizer", func() {
		hasCompNoApp := &applicationapiv1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "component-no-app",
				Namespace: "default",
			},
			Spec: applicationapiv1alpha1.ComponentSpec{
				ComponentName: "component-no-app",
				Application:   "",
				Source: applicationapiv1alpha1.ComponentSource{
					ComponentSourceUnion: applicationapiv1alpha1.ComponentSourceUnion{
						GitSource: &applicationapiv1alpha1.GitSource{
							URL: SampleRepoLink,
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, hasCompNoApp)).Should(Succeed())
		accountName := helpers.ComponentIntegrationPipelineServiceAccountName(hasCompNoApp.Name)
		accountKey := types.NamespacedName{Name: accountName, Namespace: "default"}

		noAppReq := ctrl.Request{
			NamespacedName: types.NamespacedName{
				Namespace: "default",
				Name:      hasCompNoApp.Name,
			},
		}
		// The reconciler reads through the manager cache, and a reconcile of a Component that has not
		// arrived there yet is a no-op that still returns an empty result. Wait for it to be visible.
		Eventually(func() error {
			return k8sClient.Get(ctx, noAppReq.NamespacedName, &applicationapiv1alpha1.Component{})
		}, time.Second*10).Should(Succeed())

		result, err := componentReconciler.Reconcile(ctx, noAppReq)
		Expect(err).ToNot(HaveOccurred())
		Expect(result).To(Equal(ctrl.Result{}))

		sa := &corev1.ServiceAccount{}
		rb := &rbacv1.RoleBinding{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, accountKey, sa)).To(Succeed())
			g.Expect(sa.OwnerReferences).To(HaveLen(1))
			g.Expect(sa.OwnerReferences[0].UID).To(Equal(hasCompNoApp.UID))
			g.Expect(k8sClient.Get(ctx, accountKey, rb)).To(Succeed())
			g.Expect(rb.OwnerReferences).To(HaveLen(1))
			g.Expect(rb.OwnerReferences[0].UID).To(Equal(hasCompNoApp.UID))
		}, time.Second*10).Should(Succeed())

		updated := &applicationapiv1alpha1.Component{}
		Expect(k8sClient.Get(ctx, noAppReq.NamespacedName, updated)).To(Succeed())
		Expect(controllerutil.ContainsFinalizer(updated, helpers.ComponentFinalizer)).To(BeFalse())

		// envtest has no garbage collector
		Expect(k8sClient.Delete(ctx, sa)).Should(Succeed())
		Expect(k8sClient.Delete(ctx, rb)).Should(Succeed())
		Expect(k8sClient.Delete(ctx, hasCompNoApp)).Should(Succeed())
	})

	It("runs the account operation after the finalizer for a Component with an Application", func() {
		accountName := helpers.ComponentIntegrationPipelineServiceAccountName(hasComp.Name)
		accountKey := types.NamespacedName{Name: accountName, Namespace: "default"}
		Eventually(func() error {
			return k8sClient.Get(ctx, req.NamespacedName, &applicationapiv1alpha1.Component{})
		}, time.Second*10).Should(Succeed())

		result, err := componentReconciler.Reconcile(ctx, req)
		Expect(err).ToNot(HaveOccurred())
		Expect(result).To(Equal(ctrl.Result{}))

		sa := &corev1.ServiceAccount{}
		rb := &rbacv1.RoleBinding{}
		Eventually(func(g Gomega) {
			updated := &applicationapiv1alpha1.Component{}
			g.Expect(k8sClient.Get(ctx, req.NamespacedName, updated)).To(Succeed())
			g.Expect(controllerutil.ContainsFinalizer(updated, helpers.ComponentFinalizer)).To(BeTrue())
			g.Expect(k8sClient.Get(ctx, accountKey, sa)).To(Succeed())
			g.Expect(sa.OwnerReferences).To(HaveLen(1))
			g.Expect(sa.OwnerReferences[0].UID).To(Equal(hasComp.UID))
			g.Expect(k8sClient.Get(ctx, accountKey, rb)).To(Succeed())
			g.Expect(rb.OwnerReferences).To(HaveLen(1))
			g.Expect(rb.OwnerReferences[0].UID).To(Equal(hasComp.UID))
		}, time.Second*10).Should(Succeed())

		// envtest has no garbage collector, and the later specs delete the Component without a reconcile,
		// so take the finalizer off again here
		Expect(k8sClient.Delete(ctx, sa)).Should(Succeed())
		Expect(k8sClient.Delete(ctx, rb)).Should(Succeed())
		Eventually(func() error {
			updated := &applicationapiv1alpha1.Component{}
			if err := k8sClient.Get(ctx, req.NamespacedName, updated); err != nil {
				return err
			}
			controllerutil.RemoveFinalizer(updated, helpers.ComponentFinalizer)
			return k8sClient.Update(ctx, updated)
		}, time.Second*10).Should(Succeed())
	})

	It("can fail when Reconcile fails to prepare the adapter when Component is not found", func() {
		Expect(k8sClient.Delete(ctx, hasComp)).Should(Succeed())
		Eventually(func() error {
			_, err := componentReconciler.Reconcile(ctx, req)
			return err
		}).Should(Succeed())
	})

	It("Should stop Reconciliation when Application is not found", func() {
		Expect(k8sClient.Delete(ctx, hasApp)).Should(Succeed())
		Eventually(func() error {
			_, err := componentReconciler.Reconcile(ctx, req)
			return err
		}).Should(Succeed())
	})

	It("can Reconcile function prepare the adapter and return the result of the reconcile handling operation", func() {
		req := ctrl.Request{
			NamespacedName: types.NamespacedName{
				Name:      "non-existent",
				Namespace: "default",
			},
		}
		result, err := componentReconciler.Reconcile(ctx, req)
		Expect(reflect.TypeOf(result)).To(Equal(reflect.TypeOf(reconcile.Result{})))
		Expect(err).ToNot(HaveOccurred())
	})

	It("can setup a new controller manager with the given componentReconciler", func() {
		err := setupControllerWithManager(manager, componentReconciler)
		Expect(err).NotTo(HaveOccurred())
	})
})
