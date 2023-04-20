package scenario

import (
	"reflect"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/api/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientsetscheme "k8s.io/client-go/kubernetes/scheme"
	klog "k8s.io/klog/v2"
)

var _ = Describe("ScenarioController", func() {
	var (
		manager            ctrl.Manager
		scenarioReconciler *Reconciler
		req                ctrl.Request
		scheme             runtime.Scheme
		hasApp             *applicationapiv1alpha1.Application
		hasScenario        *v1alpha1.IntegrationTestScenario
		failScenario       *v1alpha1.IntegrationTestScenario
	)
	const (
		SampleRepoLink = "https://github.com/devfile-samples/devfile-sample-java-springboot-basic"
	)

	BeforeEach(func() {

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

		scenarioName := "scenario-sample"
		pipeline := "integration-pipeline-pass"
		bundle := "quay.io/redhat-appstudio/example-tekton-bundle:integration-pipeline-pass"

		hasScenario = &v1alpha1.IntegrationTestScenario{
			ObjectMeta: metav1.ObjectMeta{
				Name:      scenarioName,
				Namespace: "default",
			},
			Spec: v1alpha1.IntegrationTestScenarioSpec{
				Application: applicationName,
				Pipeline:    pipeline,
				Bundle:      bundle,
				Environment: v1alpha1.TestEnvironment{
					Name: "envname",
					Type: "POC",
					Configuration: &applicationapiv1alpha1.EnvironmentConfiguration{
						Env: []applicationapiv1alpha1.EnvVarPair{},
					},
				},
			},
		}

		Expect(k8sClient.Create(ctx, hasScenario)).Should(Succeed())

		failScenario = &v1alpha1.IntegrationTestScenario{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "failscenario",
				Namespace: "default",
			},
			Spec: v1alpha1.IntegrationTestScenarioSpec{
				Application: "idontexist",
				Pipeline:    pipeline,
				Bundle:      bundle,
				Environment: v1alpha1.TestEnvironment{
					Name: "envname",
					Type: "POC",
					Configuration: &applicationapiv1alpha1.EnvironmentConfiguration{
						Env: []applicationapiv1alpha1.EnvVarPair{},
					},
				},
			},
		}

		Expect(k8sClient.Create(ctx, failScenario)).Should(Succeed())

		req = ctrl.Request{
			NamespacedName: types.NamespacedName{
				Namespace: "default",
				Name:      hasScenario.Name,
			},
		}

		webhookInstallOptions := &testEnv.WebhookInstallOptions

		klog.Info(webhookInstallOptions.LocalServingHost)
		klog.Info(webhookInstallOptions.LocalServingPort)
		klog.Info(webhookInstallOptions.LocalServingCertDir)

		var err error
		manager, err = ctrl.NewManager(cfg, ctrl.Options{
			Scheme:             clientsetscheme.Scheme,
			Host:               webhookInstallOptions.LocalServingHost,
			Port:               webhookInstallOptions.LocalServingPort,
			CertDir:            webhookInstallOptions.LocalServingCertDir,
			MetricsBindAddress: "0", // this disables metrics
			LeaderElection:     false,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(err).To(BeNil())

		scenarioReconciler = NewScenarioReconciler(k8sClient, &logf.Log, &scheme)

	})
	AfterEach(func() {
		err := k8sClient.Delete(ctx, hasApp)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, hasScenario)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, failScenario)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
	})

	It("can create and return a new Reconciler object", func() {
		Expect(reflect.TypeOf(scenarioReconciler)).To(Equal(reflect.TypeOf(&Reconciler{})))
		klog.Info("Test First Logic")
	})

	It("can Reconcile function prepare the adapter and return the result of the reconcile handling operation", func() {
		result, err := scenarioReconciler.Reconcile(ctx, req)
		Expect(reflect.TypeOf(result)).To(Equal(reflect.TypeOf(reconcile.Result{})))
		Expect(err).To(BeNil())
	})

	It("can setup a new controller manager with the given reconciler", func() {
		err := setupControllerWithManager(manager, scenarioReconciler)
		Expect(err).NotTo(HaveOccurred())
	})

	It("Run reconciler for scenario", func() {
		hasApp, err := scenarioReconciler.getApplicationFromScenario(ctx, failScenario)
		Expect(err).NotTo(BeNil())
		Expect(hasApp).To(BeNil())
		Expect(err).To(HaveOccurred())
	})

	It("can fail when Reconcile fails to prepare the adapter when app is not found", func() {
		Expect(k8sClient.Delete(ctx, failScenario)).Should(Succeed())
		Eventually(func() error {
			_, err := scenarioReconciler.Reconcile(ctx, req)
			return err
		}).Should(BeNil())
	})

	It("can setup a new Controller manager and start it", func() {
		err := SetupController(manager, &ctrl.Log)
		Expect(err).To(BeNil())
		go func() {
			defer GinkgoRecover()
			err = manager.Start(ctx)
			Expect(err).NotTo(HaveOccurred())
		}()
	})

})
