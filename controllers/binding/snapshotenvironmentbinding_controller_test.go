/*
Copyright 2023 Red Hat Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package binding

import (
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
	crwebhook "sigs.k8s.io/controller-runtime/pkg/webhook"
	"time"

	"github.com/redhat-appstudio/integration-service/api/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/gitops"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientsetscheme "k8s.io/client-go/kubernetes/scheme"
	klog "k8s.io/klog/v2"
)

var _ = Describe("BindingController", func() {
	var (
		manager                 ctrl.Manager
		bindingReconciler       *Reconciler
		scheme                  runtime.Scheme
		req                     ctrl.Request
		hasApp                  *applicationapiv1alpha1.Application
		hasComp                 *applicationapiv1alpha1.Component
		hasSnapshot             *applicationapiv1alpha1.Snapshot
		hasEnv                  *applicationapiv1alpha1.Environment
		hasBinding              *applicationapiv1alpha1.SnapshotEnvironmentBinding
		deploymentTargetClaim   *applicationapiv1alpha1.DeploymentTargetClaim
		deploymentTarget        *applicationapiv1alpha1.DeploymentTarget
		integrationTestScenario *v1beta1.IntegrationTestScenario
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

		deploymentTarget = &applicationapiv1alpha1.DeploymentTarget{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "dt" + "-",
				Namespace:    "default",
			},
			Spec: applicationapiv1alpha1.DeploymentTargetSpec{
				DeploymentTargetClassName: "dtcls-name",
				KubernetesClusterCredentials: applicationapiv1alpha1.DeploymentTargetKubernetesClusterCredentials{
					DefaultNamespace:           "default",
					APIURL:                     "https://url",
					ClusterCredentialsSecret:   "secret-sample",
					AllowInsecureSkipTLSVerify: false,
				},
			},
		}
		Expect(k8sClient.Create(ctx, deploymentTarget)).Should(Succeed())

		deploymentTargetClaim = &applicationapiv1alpha1.DeploymentTargetClaim{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "dtc" + "-",
				Namespace:    "default",
			},
			Spec: applicationapiv1alpha1.DeploymentTargetClaimSpec{
				DeploymentTargetClassName: applicationapiv1alpha1.DeploymentTargetClassName("dtcls-name"),
				TargetName:                deploymentTarget.Name,
			},
		}
		Expect(k8sClient.Create(ctx, deploymentTargetClaim)).Should(Succeed())

		hasEnv = &applicationapiv1alpha1.Environment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "envname",
				Namespace: "default",
			},
			Spec: applicationapiv1alpha1.EnvironmentSpec{
				Type:               "POC",
				DisplayName:        "my-environment",
				DeploymentStrategy: applicationapiv1alpha1.DeploymentStrategy_Manual,
				ParentEnvironment:  "",
				Tags:               []string{},
				Configuration: applicationapiv1alpha1.EnvironmentConfiguration{
					Env: []applicationapiv1alpha1.EnvVarPair{
						{
							Name:  "var_name",
							Value: "test",
						},
					},
					Target: applicationapiv1alpha1.EnvironmentTarget{
						DeploymentTargetClaim: applicationapiv1alpha1.DeploymentTargetClaimConfig{
							ClaimName: deploymentTargetClaim.Name,
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, hasEnv)).Should(Succeed())

		integrationTestScenario = &v1beta1.IntegrationTestScenario{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "example-pass",
				Namespace: "default",

				Labels: map[string]string{
					"test.appstudio.openshift.io/optional": "false",
				},
			},
			Spec: v1beta1.IntegrationTestScenarioSpec{
				Application: "application-sample",
				ResolverRef: v1beta1.ResolverRef{
					Resolver: "git",
					Params: []v1beta1.ResolverParameter{
						{
							Name:  "url",
							Value: "https://github.com/redhat-appstudio/integration-examples.git",
						},
						{
							Name:  "revision",
							Value: "main",
						},
						{
							Name:  "pathInRepo",
							Value: "pipelineruns/integration_pipelinerun_pass.yaml",
						},
					},
				},
				Environment: v1beta1.TestEnvironment{
					Name: hasEnv.Name,
					Type: "POC",
					Configuration: &applicationapiv1alpha1.EnvironmentConfiguration{
						Env: []applicationapiv1alpha1.EnvVarPair{},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, integrationTestScenario)).Should(Succeed())

		hasSnapshot = &applicationapiv1alpha1.Snapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "snapshot-sample",
				Namespace: "default",
				Labels: map[string]string{
					gitops.SnapshotTypeLabel:      "component",
					gitops.SnapshotComponentLabel: "component-sample",
				},
			},
			Spec: applicationapiv1alpha1.SnapshotSpec{
				Application: hasApp.Name,
				Components: []applicationapiv1alpha1.SnapshotComponent{
					{
						Name:           "component-sample",
						ContainerImage: "testimage",
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, hasSnapshot)).Should(Succeed())

		hasBinding = &applicationapiv1alpha1.SnapshotEnvironmentBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "snapshot-binding-sample",
				Namespace: "default",
				Labels: map[string]string{
					gitops.SnapshotTestScenarioLabel: integrationTestScenario.Name,
				},
			},
			Spec: applicationapiv1alpha1.SnapshotEnvironmentBindingSpec{
				Application: hasApp.Name,
				Snapshot:    hasSnapshot.Name,
				Environment: hasEnv.Name,
				Components:  []applicationapiv1alpha1.BindingComponent{},
			},
		}
		Expect(k8sClient.Create(ctx, hasBinding)).Should(Succeed())

		hasBinding.Status = applicationapiv1alpha1.SnapshotEnvironmentBindingStatus{
			BindingConditions: []metav1.Condition{
				{
					Reason:             "Completed",
					Status:             "True",
					Type:               gitops.BindingDeploymentStatusConditionType,
					LastTransitionTime: metav1.Time{Time: time.Now()},
				},
			},
		}

		Expect(k8sClient.Status().Update(ctx, hasBinding)).Should(Succeed())

		req = ctrl.Request{
			NamespacedName: types.NamespacedName{
				Namespace: "default",
				Name:      hasBinding.Name,
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
		Expect(err).To(BeNil())

		bindingReconciler = NewBindingReconciler(k8sClient, &logf.Log, &scheme)
	})
	AfterEach(func() {
		err := k8sClient.Delete(ctx, hasApp)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, hasComp)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, hasSnapshot)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, hasEnv)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, hasBinding)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, integrationTestScenario)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
	})

	It("can create and return a new Reconciler object", func() {
		Expect(reflect.TypeOf(bindingReconciler)).To(Equal(reflect.TypeOf(&Reconciler{})))
		klog.Info("Test First Logic")
	})

	It("can fail when Reconcile fails to prepare the adapter when SnapshotEnvironmentBinding is not found", func() {
		Expect(k8sClient.Delete(ctx, hasBinding)).Should(Succeed())
		Eventually(func() error {
			_, err := bindingReconciler.Reconcile(ctx, req)
			return err
		}).Should(BeNil())
	})

	It("can skip reconcile when SnapshotEnvironmentBinding is being deleted", func() {
		Expect(k8sClient.Delete(ctx, hasBinding)).Should(Succeed())

		// DeletionTimestamp is not nil
		now := metav1.NewTime(metav1.Now().Add(time.Second * 1))
		hasBinding.SetDeletionTimestamp(&now)

		result, err := bindingReconciler.Reconcile(ctx, req)
		Expect(result).To(Equal(ctrl.Result{}))
		Expect(err).To(BeNil())
	})

	It("can fail when Reconcile fails to prepare the adapter when Application is not found", func() {
		err := k8sClient.Delete(ctx, hasApp)
		Eventually(func() bool {
			err := k8sClient.Get(ctx, types.NamespacedName{
				Namespace: hasApp.ObjectMeta.Namespace,
				Name:      hasApp.ObjectMeta.Name,
			}, hasApp)
			return err != nil && errors.IsNotFound(err)
		}).Should(BeTrue())
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())

		result, err := bindingReconciler.Reconcile(ctx, req)
		Expect(result).To(Equal(ctrl.Result{}))
		Expect(err).To(BeNil())
	})

	It("can fail when Reconcile fails to prepare the adapter when Snapshot is not found", func() {
		err := k8sClient.Delete(ctx, hasSnapshot)
		Eventually(func() bool {
			err := k8sClient.Get(ctx, types.NamespacedName{
				Namespace: hasSnapshot.ObjectMeta.Namespace,
				Name:      hasSnapshot.ObjectMeta.Name,
			}, hasSnapshot)
			return err != nil && errors.IsNotFound(err)
		}).Should(BeTrue())
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())

		result, err := bindingReconciler.Reconcile(ctx, req)
		Expect(result).To(Equal(ctrl.Result{}))
		Expect(err).To(BeNil())
	})

	It("can fail when Reconcile fails to prepare the adapter when Environment is not found", func() {
		Expect(k8sClient.Delete(ctx, hasEnv)).Should(Succeed())
		Eventually(func() error {
			_, err := bindingReconciler.Reconcile(ctx, req)
			return err
		}).ShouldNot(BeNil())
	})

	It("can fail when Reconcile fails to prepare the adapter when IntegrationTestScenario is not found", func() {
		err := k8sClient.Delete(ctx, integrationTestScenario)
		Expect(err).ToNot(HaveOccurred())
		Eventually(func() bool {
			err := k8sClient.Get(ctx, types.NamespacedName{
				Namespace: integrationTestScenario.ObjectMeta.Namespace,
				Name:      integrationTestScenario.ObjectMeta.Name,
			}, integrationTestScenario)
			return err != nil && errors.IsNotFound(err)
		}).Should(BeTrue())

		result, err := bindingReconciler.Reconcile(ctx, req)
		Expect(result).To(Equal(ctrl.Result{}))
		Expect(err).ToNot(HaveOccurred())
	})

	It("can Reconcile function prepare the adapter and return the result of the reconcile handling operation", func() {
		result, err := bindingReconciler.Reconcile(ctx, req)
		Expect(reflect.TypeOf(result)).To(Equal(reflect.TypeOf(reconcile.Result{})))
		Expect(err).To(BeNil())
	})

	It("can setup a new controller manager with the given reconciler", func() {
		err := setupControllerWithManager(manager, bindingReconciler)
		Expect(err).NotTo(HaveOccurred())
	})

	It("can setup a new Controller manager and start it", func() {
		err := SetupController(manager, &ctrl.Log)
		Expect(err).To(BeNil())
	})

})
