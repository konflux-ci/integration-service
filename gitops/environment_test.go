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

package gitops_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/api/v1beta1"
	"github.com/redhat-appstudio/integration-service/gitops"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("Gitops functions for managing Snapshots", Ordered, func() {

	var (
		hasApp                   *applicationapiv1alpha1.Application
		hasSnapshot              *applicationapiv1alpha1.Snapshot
		envWithEnvVars           *applicationapiv1alpha1.Environment
		deploymentTarget         *applicationapiv1alpha1.DeploymentTarget
		deploymentTargetClaim    *applicationapiv1alpha1.DeploymentTargetClaim
		copiedEnvWithEnvVars     *gitops.CopiedEnvironment
		copiedEnvWithEnvVarsITS  *gitops.CopiedEnvironment
		copiedEnvWithEnvVarsDiff *gitops.CopiedEnvironment
		expectEnv                *applicationapiv1alpha1.Environment
		hasIntTestSc             *v1beta1.IntegrationTestScenario
		hasIntTestScWithNoEnv    *v1beta1.IntegrationTestScenario
		hasIntTestScDiff         *v1beta1.IntegrationTestScenario
		deploymentTargetClass    *applicationapiv1alpha1.DeploymentTargetClass
		sampleImage              string
	)
	const (
		namespace       = "default"
		applicationName = "application-sample"
		componentName   = "component-sample"
		snapshotName    = "snapshot-sample"
	)

	BeforeAll(func() {
		expectEnv = &applicationapiv1alpha1.Environment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "expect-envname",
				Namespace: "default",
			},
			Spec: applicationapiv1alpha1.EnvironmentSpec{
				Configuration: applicationapiv1alpha1.EnvironmentConfiguration{
					Env: []applicationapiv1alpha1.EnvVarPair{
						{
							Name:  "VAR_NAME",
							Value: "VAR_VALUE_ENV",
						},
						{
							Name:  "VAR_NAME_INT",
							Value: "VAR_VALUE_INT",
						},
					},
				},
			},
		}

		Expect(k8sClient.Create(ctx, expectEnv)).Should(Succeed())

		hasApp = &applicationapiv1alpha1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      applicationName,
				Namespace: namespace,
			},
			Spec: applicationapiv1alpha1.ApplicationSpec{
				DisplayName: "application-sample",
				Description: "This is an example application",
			},
		}
		Expect(k8sClient.Create(ctx, hasApp)).Should(Succeed())

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

		envWithEnvVars = &applicationapiv1alpha1.Environment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "envname-with-env-vars",
				Namespace: "default",
			},
			Spec: applicationapiv1alpha1.EnvironmentSpec{
				Type:               "POC",
				DisplayName:        "my-environment",
				DeploymentStrategy: applicationapiv1alpha1.DeploymentStrategy_Manual,
				Tags:               []string{},
				Configuration: applicationapiv1alpha1.EnvironmentConfiguration{
					Env: []applicationapiv1alpha1.EnvVarPair{
						{
							Name:  "VAR_NAME",
							Value: "VAR_VALUE_ENV",
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
		Expect(k8sClient.Create(ctx, envWithEnvVars)).Should(Succeed())

		deploymentTargetClass = &applicationapiv1alpha1.DeploymentTargetClass{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "dtcls" + "-",
			},
			Spec: applicationapiv1alpha1.DeploymentTargetClassSpec{
				Provisioner: "appstudio.redhat.com/devsandbox",
			},
		}
		Expect(k8sClient.Create(ctx, deploymentTargetClass)).Should(Succeed())

		hasIntTestSc = &v1beta1.IntegrationTestScenario{
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
					Name: "envname",
					Type: "POC",
					Configuration: &applicationapiv1alpha1.EnvironmentConfiguration{
						Env: []applicationapiv1alpha1.EnvVarPair{
							{
								Name:  "VAR_NAME",
								Value: "VAR_VALUE_INT",
							},
						},
					},
				},
			},
		}

		Expect(k8sClient.Create(ctx, hasIntTestSc)).Should(Succeed())

		hasIntTestScWithNoEnv = &v1beta1.IntegrationTestScenario{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "example-pass-no-env",
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
					Name: "envname",
					Type: "POC",
					Configuration: &applicationapiv1alpha1.EnvironmentConfiguration{
						Env: []applicationapiv1alpha1.EnvVarPair{},
					},
				},
			},
		}

		Expect(k8sClient.Create(ctx, hasIntTestScWithNoEnv)).Should(Succeed())

		hasIntTestScDiff = &v1beta1.IntegrationTestScenario{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "example-pass-diff",
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
					Name: "envname",
					Type: "POC",
					Configuration: &applicationapiv1alpha1.EnvironmentConfiguration{
						Env: []applicationapiv1alpha1.EnvVarPair{
							{
								Name:  "VAR_NAME_INT",
								Value: "VAR_VALUE_INT",
							},
						},
					},
				},
			},
		}

		Expect(k8sClient.Create(ctx, hasIntTestScDiff)).Should(Succeed())
	})

	BeforeEach(func() {
		sampleImage = "quay.io/redhat-appstudio/sample-image:latest"

		hasSnapshot = &applicationapiv1alpha1.Snapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      snapshotName,
				Namespace: namespace,
				Labels: map[string]string{
					gitops.SnapshotTypeLabel:      gitops.SnapshotComponentType,
					gitops.SnapshotComponentLabel: componentName,
				},
			},
			Spec: applicationapiv1alpha1.SnapshotSpec{
				Application: hasApp.Name,
				Components: []applicationapiv1alpha1.SnapshotComponent{
					{
						Name:           componentName,
						ContainerImage: sampleImage,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, hasSnapshot)).Should(Succeed())

		//create copy of environment with env Vars from ITS
		copiedEnvWithEnvVars = gitops.NewCopyOfExistingEnvironment(envWithEnvVars, namespace, hasIntTestSc, "dtcName")
		Expect(k8sClient.Create(ctx, copiedEnvWithEnvVars.AsEnvironment())).Should(Succeed())
		Expect(copiedEnvWithEnvVars.Spec.Configuration.Target.DeploymentTargetClaim.ClaimName).To(Equal("dtcName"))
		//create copy of environment with env Vars from existing env
		copiedEnvWithEnvVarsITS = gitops.NewCopyOfExistingEnvironment(envWithEnvVars, namespace, hasIntTestScWithNoEnv, "dtcName")
		Expect(k8sClient.Create(ctx, copiedEnvWithEnvVarsITS.AsEnvironment())).Should(Succeed())
		//create copy of environment with env Vars from both existing env and ITS
		copiedEnvWithEnvVarsDiff = gitops.NewCopyOfExistingEnvironment(envWithEnvVars, namespace, hasIntTestScDiff, "dtcName")
		Expect(k8sClient.Create(ctx, copiedEnvWithEnvVarsDiff.AsEnvironment())).Should(Succeed())

		Eventually(func() error {
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      hasSnapshot.Name,
				Namespace: namespace,
			}, hasSnapshot)
			return err
		}, time.Second*10).ShouldNot(HaveOccurred())
	})

	AfterEach(func() {
		err := k8sClient.Delete(ctx, hasSnapshot)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
	})

	AfterAll(func() {
		err := k8sClient.Delete(ctx, hasApp)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, hasIntTestSc)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, hasIntTestScWithNoEnv)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, hasIntTestScDiff)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, envWithEnvVars)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, deploymentTarget)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, deploymentTargetClaim)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())

	})
	Context("When copying an existing environment", func() {
		It("can create a IntegrationPipelineRun and the returned object name is prefixed with the provided GenerateName", func() {
			Expect(copiedEnvWithEnvVars.ObjectMeta.Name).
				Should(HavePrefix(envWithEnvVars.Name + "-" + hasIntTestSc.Name + "-"))
			Expect(copiedEnvWithEnvVars.ObjectMeta.Namespace).To(Equal(hasApp.ObjectMeta.Namespace))
		})
		It("existing env has envVars defined, ITS(integrationTestScenario) has the same envVar but different value, copied env should have envVar from ITS", func() {
			Expect(copiedEnvWithEnvVars.Spec.Configuration.Env).To(Equal(hasIntTestSc.Spec.Environment.Configuration.Env))
		})
		It("existing env has envVars defined, ITS has NO envVar defined, copied env should have envVar exisitng env", func() {
			Expect(copiedEnvWithEnvVarsITS.Spec.Configuration.Env).To(Equal(envWithEnvVars.Spec.Configuration.Env))
		})
		It("existing env has envVars defines, ITS has envVars defined, copied env should have updated envVars from existing evironment and new ones from ITS", func() {
			Expect(copiedEnvWithEnvVarsDiff.Spec.Configuration.Env).To(Equal(expectEnv.Spec.Configuration.Env))
		})

		It("can append labels that comes from Snapshot to Environment and make sure that label value matches the snapshot name", func() {
			copiedEnvWithEnvVarsDiff.WithSnapshot(hasSnapshot)
			Expect(copiedEnvWithEnvVarsDiff.Labels["appstudio.openshift.io/snapshot"]).
				To(Equal(hasSnapshot.Name))
		})

		It("can append labels that comes from IntegrationTestScenario to Environment and make sure that label value matches the snapshot name", func() {
			copiedEnvWithEnvVarsDiff.WithIntegrationLabels(hasIntTestSc)
			Expect(copiedEnvWithEnvVarsDiff.Labels["test.appstudio.openshift.io/scenario"]).
				To(Equal(hasIntTestSc.Name))
		})

		It("Can return DeploymentTargetClaim object", func() {
			dtc := gitops.NewDeploymentTargetClaim("default", deploymentTargetClass.Name)
			Expect(dtc.Spec.DeploymentTargetClassName == applicationapiv1alpha1.DeploymentTargetClassName(deploymentTargetClass.Name)).To(BeTrue())
		})
	})

})
