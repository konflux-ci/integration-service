/*
Copyright 2022 Red Hat Inc.

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

package tekton_test

import (
	"bytes"
	"fmt"
	"os"
	"time"

	"github.com/konflux-ci/integration-service/api/v1beta2"
	"github.com/konflux-ci/integration-service/helpers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/tonglil/buflogr"

	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	"github.com/konflux-ci/integration-service/gitops"
	tekton "github.com/konflux-ci/integration-service/tekton"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ExtraParams struct {
	Name  string
	Value tektonv1.ParamValue
}

var _ = Describe("Integration pipeline", func() {

	const (
		prefix          = "testpipeline"
		namespace       = "default"
		applicationName = "application-sample"
		SampleRepoLink  = "https://github.com/devfile-samples/devfile-sample-java-springboot-basic"
	)
	var (
		hasApp                          *applicationapiv1alpha1.Application
		hasSnapshot                     *applicationapiv1alpha1.Snapshot
		hasComp                         *applicationapiv1alpha1.Component
		newIntegrationPipelineRun       *tekton.IntegrationPipelineRun
		newIntegrationBundlePipelineRun *tekton.IntegrationPipelineRun
		enterpriseContractPipelineRun   *tekton.IntegrationPipelineRun
		integrationTestScenarioGit      *v1beta2.IntegrationTestScenario
		integrationTestScenarioBundle   *v1beta2.IntegrationTestScenario
		enterpriseContractTestScenario  *v1beta2.IntegrationTestScenario
		extraParams                     *ExtraParams
	)

	BeforeEach(func() {

		extraParams = &ExtraParams{
			Name: "extraConfigPath",
			Value: tektonv1.ParamValue{
				Type:      tektonv1.ParamTypeString,
				StringVal: "path/to/extra/config.yaml",
			},
		}

		integrationTestScenarioGit = &v1beta2.IntegrationTestScenario{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "example-pass",
				Namespace: "default",

				Labels: map[string]string{
					"test.appstudio.openshift.io/optional": "false",
				},
			},
			Spec: v1beta2.IntegrationTestScenarioSpec{
				Application: "application-sample",
				ResolverRef: v1beta2.ResolverRef{
					Resolver: "git",
					Params: []v1beta2.ResolverParameter{
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
			},
		}
		Expect(k8sClient.Create(ctx, integrationTestScenarioGit)).Should(Succeed())

		//create new integration pipeline run from integration test scenario
		newIntegrationPipelineRun = tekton.NewIntegrationPipelineRun(prefix, namespace, *integrationTestScenarioGit)
		Expect(k8sClient.Create(ctx, newIntegrationPipelineRun.AsPipelineRun())).Should(Succeed())

		integrationTestScenarioBundle = &v1beta2.IntegrationTestScenario{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-bundle",
				Namespace: "default",

				Labels: map[string]string{
					"test.appstudio.openshift.io/optional": "false",
				},
			},
			Spec: v1beta2.IntegrationTestScenarioSpec{
				Application: "application-sample",
				ResolverRef: v1beta2.ResolverRef{
					Resolver: "bundles",
					Params: []v1beta2.ResolverParameter{
						{
							Name:  "bundle",
							Value: "quay.io/redhat-appstudio/example-tekton-bundle:integration-pipeline-pass",
						},
						{
							Name:  "name",
							Value: "integration-pipeline-pass",
						},
						{
							Name:  "kind",
							Value: "pipeline",
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, integrationTestScenarioBundle)).Should(Succeed())

		enterpriseContractTestScenario = &v1beta2.IntegrationTestScenario{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "enterprise-contract",
				Namespace: "default",

				Labels: map[string]string{
					"test.appstudio.openshift.io/optional": "false",
				},

				Annotations: map[string]string{
					"test.appstudio.openshift.io/kind": "enterprise-contract",
				},
			},
			Spec: v1beta2.IntegrationTestScenarioSpec{
				Application: "application-sample",
				ResolverRef: v1beta2.ResolverRef{
					Resolver: "git",
					Params: []v1beta2.ResolverParameter{
						{
							Name:  "url",
							Value: "https://github.com/konflux-ci/build-definitions.git",
						},
						{
							Name:  "revision",
							Value: "main",
						},
						{
							Name:  "pathInRepo",
							Value: "pipelines/enterprise-contract.yaml",
						},
					},
				},
				Params: []v1beta2.PipelineParameter{
					{
						Name:  "POLICY_CONFIGURATION",
						Value: "default/default",
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, enterpriseContractTestScenario)).Should(Succeed())

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

		hasSnapshot = &applicationapiv1alpha1.Snapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "snapshot-sample",
				Namespace: "default",
				Labels: map[string]string{
					gitops.SnapshotTypeLabel:                   "component",
					gitops.SnapshotComponentLabel:              "component-sample",
					gitops.CustomLabelPrefix + "/custom-label": "custom-label",
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

		hasComp = &applicationapiv1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "component-sample",
				Namespace: "default",
			},
			Spec: applicationapiv1alpha1.ComponentSpec{
				ComponentName:  "component-sample",
				Application:    "application-sample",
				ContainerImage: "",
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

		newIntegrationBundlePipelineRun = tekton.NewIntegrationPipelineRun(prefix, namespace, *integrationTestScenarioBundle).
			WithIntegrationLabels(integrationTestScenarioBundle).
			WithSnapshot(hasSnapshot).
			WithApplication(hasApp)
		Expect(k8sClient.Create(ctx, newIntegrationBundlePipelineRun.AsPipelineRun())).Should(Succeed())

		enterpriseContractPipelineRun = tekton.NewIntegrationPipelineRun(prefix, namespace, *enterpriseContractTestScenario).
			WithIntegrationLabels(enterpriseContractTestScenario).
			WithIntegrationAnnotations(enterpriseContractTestScenario).
			WithSnapshot(hasSnapshot).
			WithExtraParams(enterpriseContractTestScenario.Spec.Params).
			WithApplication(hasApp)
		Expect(k8sClient.Create(ctx, enterpriseContractPipelineRun.AsPipelineRun())).Should(Succeed())

		os.Setenv("PIPELINE_TIMEOUT", "2h")
		os.Setenv("TASKS_TIMEOUT", "2h")
		os.Setenv("FINALLY_TIMEOUT", "2h")
	})

	AfterEach(func() {
		_ = k8sClient.Delete(ctx, integrationTestScenarioGit)
		_ = k8sClient.Delete(ctx, integrationTestScenarioBundle)
		_ = k8sClient.Delete(ctx, enterpriseContractTestScenario)
		_ = k8sClient.Delete(ctx, newIntegrationPipelineRun)
		_ = k8sClient.Delete(ctx, newIntegrationBundlePipelineRun)
		_ = k8sClient.Delete(ctx, enterpriseContractPipelineRun)
		_ = k8sClient.Delete(ctx, hasApp)
		_ = k8sClient.Delete(ctx, hasSnapshot)
		_ = k8sClient.Delete(ctx, hasComp)
		_ = k8sClient.Delete(ctx, newIntegrationPipelineRun.AsPipelineRun())

		os.Setenv("PIPELINE_TIMEOUT", "")
		os.Setenv("TASKS_TIMEOUT", "")
		os.Setenv("FINALLY_TIMEOUT", "")
	})

	Context("When managing a new IntegrationPipelineRun", func() {
		It("can create a IntegrationPipelineRun and the returned object name is prefixed with the provided GenerateName", func() {
			Expect(newIntegrationPipelineRun.ObjectMeta.Name).
				Should(HavePrefix(prefix))
			Expect(newIntegrationPipelineRun.ObjectMeta.Namespace).To(Equal(namespace))
			Expect(string(enterpriseContractPipelineRun.Spec.PipelineRef.ResolverRef.Resolver)).To(Equal("git"))
		})

		It("can add timeouts to the IntegrationPipelineRun according to the environment variables", func() {
			var buf bytes.Buffer
			expectedDuration, _ := time.ParseDuration("2h")
			newIntegrationPipelineRun.WithIntegrationTimeouts(integrationTestScenarioGit, buflogr.NewWithBuffer(&buf))

			Expect(newIntegrationPipelineRun.Spec.Timeouts.Tasks.Duration).To(Equal(expectedDuration))
			Expect(newIntegrationPipelineRun.Spec.Timeouts.Finally.Duration).To(Equal(expectedDuration))
			// Pipeline timeout duration should be set to be the sum of tasks + finally
			Expect(newIntegrationPipelineRun.Spec.Timeouts.Pipeline.Duration).To(Equal(expectedDuration + expectedDuration))

			// The pipelineRun timeouts should be empty if environment vars are not set
			os.Setenv("PIPELINE_TIMEOUT", "")
			os.Setenv("TASKS_TIMEOUT", "")
			os.Setenv("FINALLY_TIMEOUT", "")
			newIntegrationPipelineRun.WithIntegrationTimeouts(integrationTestScenarioGit, buflogr.NewWithBuffer(&buf))

			Expect(newIntegrationPipelineRun.Spec.Timeouts.Pipeline).To(BeNil())
			Expect(newIntegrationPipelineRun.Spec.Timeouts.Tasks).To(BeNil())
			Expect(newIntegrationPipelineRun.Spec.Timeouts.Finally).To(BeNil())

			// Set the timeouts to invalid strings, which should skip setting the timeouts
			os.Setenv("PIPELINE_TIMEOUT", "thisIsNotAValidDuration!")
			os.Setenv("TASKS_TIMEOUT", "thisIsNotAValidDuration!")
			os.Setenv("FINALLY_TIMEOUT", "thisIsNotAValidDuration!")
			newIntegrationPipelineRun.WithIntegrationTimeouts(integrationTestScenarioGit, buflogr.NewWithBuffer(&buf))

			Expect(newIntegrationPipelineRun.Spec.Timeouts.Pipeline).To(BeNil())
			Expect(newIntegrationPipelineRun.Spec.Timeouts.Tasks).To(BeNil())
			Expect(newIntegrationPipelineRun.Spec.Timeouts.Finally).To(BeNil())

			expectedLogEntryPrefix := "failed to parse the"
			Expect(buf.String()).Should(ContainSubstring(expectedLogEntryPrefix + " PIPELINE_TIMEOUT"))
			Expect(buf.String()).Should(ContainSubstring(expectedLogEntryPrefix + " TASKS_TIMEOUT"))
			Expect(buf.String()).Should(ContainSubstring(expectedLogEntryPrefix + " FINALLY_TIMEOUT"))
		})

		It("can add timeouts to the IntegrationPipelineRun according to the integrationTestScenario annotations", func() {
			var buf bytes.Buffer
			// Use the environment variables by default, override if annotations are set
			expectedDurationFromEnv, _ := time.ParseDuration("2h")
			expectedDurationFromScenario, _ := time.ParseDuration("8h")
			integrationTestScenarioGit.Annotations = map[string]string{}
			integrationTestScenarioGit.Annotations[v1beta2.PipelineTimeoutAnnotation] = "8h"
			newIntegrationPipelineRun.WithIntegrationTimeouts(integrationTestScenarioGit, buflogr.NewWithBuffer(&buf))

			Expect(newIntegrationPipelineRun.Spec.Timeouts.Pipeline.Duration).To(Equal(expectedDurationFromScenario))
			Expect(newIntegrationPipelineRun.Spec.Timeouts.Tasks.Duration).To(Equal(expectedDurationFromEnv))
			Expect(newIntegrationPipelineRun.Spec.Timeouts.Finally.Duration).To(Equal(expectedDurationFromEnv))

			// Override the environment if all annotations are set
			integrationTestScenarioGit.Annotations[v1beta2.TasksTimeoutAnnotation] = "8h"
			integrationTestScenarioGit.Annotations[v1beta2.FinallyTimeoutAnnotation] = "8h"
			newIntegrationPipelineRun.WithIntegrationTimeouts(integrationTestScenarioGit, buflogr.NewWithBuffer(&buf))

			Expect(newIntegrationPipelineRun.Spec.Timeouts.Tasks.Duration).To(Equal(expectedDurationFromScenario))
			Expect(newIntegrationPipelineRun.Spec.Timeouts.Finally.Duration).To(Equal(expectedDurationFromScenario))
			// Pipeline timeout duration should be set to be the sum of tasks + finally
			Expect(newIntegrationPipelineRun.Spec.Timeouts.Pipeline.Duration).To(Equal(expectedDurationFromScenario + expectedDurationFromScenario))

			expectedLogEntry := fmt.Sprintf("to be the sum of tasks + finally: %.1f hours", (expectedDurationFromScenario + expectedDurationFromScenario).Hours())
			Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))

			// Set the task timeout to invalid strings, which should skip setting the timeout for it
			integrationTestScenarioGit.Annotations[v1beta2.TasksTimeoutAnnotation] = "thisIsNotAValidDuration!"
			newIntegrationPipelineRun.WithIntegrationTimeouts(integrationTestScenarioGit, buflogr.NewWithBuffer(&buf))

			Expect(newIntegrationPipelineRun.Spec.Timeouts.Tasks).To(BeNil())
			Expect(newIntegrationPipelineRun.Spec.Timeouts.Finally.Duration).To(Equal(expectedDurationFromScenario))
			Expect(newIntegrationPipelineRun.Spec.Timeouts.Pipeline.Duration).To(Equal(expectedDurationFromScenario))

			expectedLogEntryPrefix := "failed to parse the"
			Expect(buf.String()).Should(ContainSubstring(expectedLogEntryPrefix + " TASKS_TIMEOUT"))

			// Set all timeouts to invalid strings, which should skip setting the timeouts entirely
			integrationTestScenarioGit.Annotations[v1beta2.PipelineTimeoutAnnotation] = "thisIsNotAValidDuration!"
			integrationTestScenarioGit.Annotations[v1beta2.TasksTimeoutAnnotation] = "thisIsNotAValidDuration!"
			integrationTestScenarioGit.Annotations[v1beta2.FinallyTimeoutAnnotation] = "thisIsNotAValidDuration!"
			newIntegrationPipelineRun.WithIntegrationTimeouts(integrationTestScenarioGit, buflogr.NewWithBuffer(&buf))

			Expect(newIntegrationPipelineRun.Spec.Timeouts.Pipeline).To(BeNil())
			Expect(newIntegrationPipelineRun.Spec.Timeouts.Tasks).To(BeNil())
			Expect(newIntegrationPipelineRun.Spec.Timeouts.Finally).To(BeNil())

			Expect(buf.String()).Should(ContainSubstring(expectedLogEntryPrefix + " PIPELINE_TIMEOUT"))
			Expect(buf.String()).Should(ContainSubstring(expectedLogEntryPrefix + " TASKS_TIMEOUT"))
			Expect(buf.String()).Should(ContainSubstring(expectedLogEntryPrefix + " FINALLY_TIMEOUT"))
		})

		It("can add and remove finalizer from IntegrationPipelineRun", func() {
			var buf bytes.Buffer
			logEntry := "Removed Finalizer from the PipelineRun"

			newIntegrationPipelineRun.WithFinalizer(helpers.IntegrationPipelineRunFinalizer)
			Expect(newIntegrationPipelineRun.Finalizers).To(ContainElement(ContainSubstring(helpers.IntegrationPipelineRunFinalizer)))

			// calling RemoveFinalizerFromPipelineRun() when the PipelineRun contains the finalizer
			log := helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(&buf)}
			Expect(helpers.RemoveFinalizerFromPipelineRun(ctx, k8sClient, log, &newIntegrationPipelineRun.PipelineRun, helpers.IntegrationPipelineRunFinalizer)).To(Succeed())
			Expect(newIntegrationPipelineRun.Finalizers).To(BeNil())
			Expect(buf.String()).Should(ContainSubstring(logEntry))

			// calling RemoveFinalizerFromPipelineRun() when the PipelineRun doesn't contain the finalizer
			buf = bytes.Buffer{}
			log = helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(&buf)}
			Expect(helpers.RemoveFinalizerFromPipelineRun(ctx, k8sClient, log, &newIntegrationPipelineRun.PipelineRun, helpers.IntegrationPipelineRunFinalizer)).To(Succeed())
			Expect(newIntegrationPipelineRun.Finalizers).To(BeNil())
			Expect(buf.String()).ShouldNot(ContainSubstring(logEntry))
		})

		It("can append extra params to IntegrationPipelineRun and these parameters are present in the object Specs", func() {
			newIntegrationPipelineRun.WithExtraParam(extraParams.Name, extraParams.Value)
			Expect(newIntegrationPipelineRun.Spec.Params[0].Name).To(Equal(extraParams.Name))
			Expect(newIntegrationPipelineRun.Spec.Params[0].Value.StringVal).
				To(Equal(extraParams.Value.StringVal))
		})

		It("can append the scenario Name, optional flag and Namespace to a IntegrationPipelineRun object and that these label key names match the correct label format", func() {
			newIntegrationPipelineRun.WithIntegrationLabels(integrationTestScenarioGit)
			Expect(newIntegrationPipelineRun.Labels["test.appstudio.openshift.io/scenario"]).
				To(Equal(integrationTestScenarioGit.Name))
			Expect(newIntegrationPipelineRun.Labels["pipelines.appstudio.openshift.io/type"]).
				To(Equal("test"))
			Expect(newIntegrationPipelineRun.Labels["test.appstudio.openshift.io/optional"]).
				To(Equal("false"))
			Expect(newIntegrationPipelineRun.Namespace).
				To(Equal(integrationTestScenarioGit.Namespace))
		})

		It("can append labels that comes from Snapshot to IntegrationPipelineRun and make sure that label value matches the snapshot and component names", func() {
			newIntegrationPipelineRun.WithSnapshot(hasSnapshot)
			Expect(newIntegrationPipelineRun.Labels["appstudio.openshift.io/snapshot"]).
				To(Equal(hasSnapshot.Name))
			Expect(newIntegrationPipelineRun.Labels["appstudio.openshift.io/component"]).
				To(Equal(hasComp.Name))
			Expect(newIntegrationPipelineRun.Labels[gitops.CustomLabelPrefix+"/custom_label"]).
				To(Equal(hasSnapshot.Labels[gitops.CustomLabelPrefix+"/custom_label"]))
			Expect(newIntegrationPipelineRun.Labels[gitops.SnapshotTypeLabel]).
				To(Equal(hasSnapshot.Labels[gitops.SnapshotTypeLabel]))
		})

		It("updates the log-url annotation when copied from the snapshot", func() {
			// Set up a snapshot with the log-url annotation
			snapshotWithLogURL := hasSnapshot.DeepCopy()
			if snapshotWithLogURL.Annotations == nil {
				snapshotWithLogURL.Annotations = make(map[string]string)
			}
			snapshotWithLogURL.Annotations["pac.test.appstudio.openshift.io/log-url"] = "https://console.redhat.com/preview/application-pipeline/ns/rhtap-shared-team-tenant/pipelinerun/build-pipeline-run-123"

			// Apply the snapshot to the integration pipeline run
			newIntegrationPipelineRun.WithSnapshot(snapshotWithLogURL)

			// Verify that the log-url annotation exists but has been reset
			Expect(newIntegrationPipelineRun.Annotations).To(HaveKey("pac.test.appstudio.openshift.io/log-url"))
			Expect(newIntegrationPipelineRun.Annotations["pac.test.appstudio.openshift.io/log-url"]).To(Equal("https://console.redhat.com/preview/application-pipeline/ns/rhtap-shared-team-tenant/pipelinerun/build-pipeline-run-123"))
		})

		It("can append labels coming from Application to IntegrationPipelineRun and making sure that label values matches application", func() {
			newIntegrationPipelineRun.WithApplication(hasApp)
			Expect(newIntegrationPipelineRun.Labels["appstudio.openshift.io/application"]).
				To(Equal(hasApp.Name))
		})

		It("provides parameters from IntegrationTestScenario to the PipelineRun", func() {
			scenarioParams := []v1beta2.PipelineParameter{
				{
					Name:  "ADDITIONAL_PARAMETER",
					Value: "custom value",
				},
				{
					Name:   "MULTIVALUE_PARAMETER",
					Values: []string{"value1", "value2"},
				},
			}

			newIntegrationPipelineRun.WithExtraParams(scenarioParams)
			Expect(newIntegrationPipelineRun.Spec.Params[0].Name).To(Equal(scenarioParams[0].Name))
			Expect(newIntegrationPipelineRun.Spec.Params[0].Value.StringVal).To(Equal(scenarioParams[0].Value))
			Expect(newIntegrationPipelineRun.Spec.Params[1].Name).To(Equal(scenarioParams[1].Name))
			Expect(newIntegrationPipelineRun.Spec.Params[1].Value.ArrayVal).To(Equal(scenarioParams[1].Values))
		})

	})

	Context("When managing a new pipelineRun from a bundle-based IntegrationTestScenario", func() {
		It("has set all required labels to be used for integration testing of the EC pipeline", func() {
			Expect(newIntegrationBundlePipelineRun.ObjectMeta.Name).Should(HavePrefix(prefix))
			Expect(newIntegrationBundlePipelineRun.ObjectMeta.Namespace).To(Equal(namespace))
			Expect(newIntegrationBundlePipelineRun.Labels["test.appstudio.openshift.io/scenario"]).
				To(Equal(integrationTestScenarioBundle.Name))
			Expect(newIntegrationBundlePipelineRun.Labels["pipelines.appstudio.openshift.io/type"]).
				To(Equal("test"))
			Expect(newIntegrationBundlePipelineRun.Labels["test.appstudio.openshift.io/optional"]).
				To(Equal("false"))

			Expect(string(newIntegrationBundlePipelineRun.Spec.PipelineRef.ResolverRef.Resolver)).To(Equal("bundles"))
			Expect(newIntegrationBundlePipelineRun.Spec.PipelineRef.ResolverRef.Params).To(HaveLen(3))
		})
	})

	Context("When managing a new Enterprise Contract PipelineRun", func() {
		It("has set all required labels and annotations to be used for integration testing of the EC pipeline", func() {
			Expect(enterpriseContractPipelineRun.ObjectMeta.Name).Should(HavePrefix(prefix))
			Expect(enterpriseContractPipelineRun.ObjectMeta.Namespace).To(Equal(namespace))
			Expect(enterpriseContractPipelineRun.Labels["test.appstudio.openshift.io/scenario"]).
				To(Equal(enterpriseContractTestScenario.Name))
			Expect(enterpriseContractPipelineRun.Labels["pipelines.appstudio.openshift.io/type"]).
				To(Equal("test"))
			Expect(enterpriseContractPipelineRun.Labels["test.appstudio.openshift.io/optional"]).
				To(Equal("false"))
			Expect(enterpriseContractPipelineRun.Annotations["test.appstudio.openshift.io/kind"]).
				To(Equal("enterprise-contract"))
		})

		It("has set all parameters required for executing the EC pipeline", func() {
			Expect(string(enterpriseContractPipelineRun.Spec.PipelineRef.ResolverRef.Resolver)).To(Equal("git"))
			Expect(enterpriseContractPipelineRun.Spec.PipelineRef.ResolverRef.Params).To(HaveLen(3))

			Expect(enterpriseContractPipelineRun.Spec.Params[0].Name).To(Equal("SNAPSHOT"))
			Expect(enterpriseContractPipelineRun.Spec.Params[1].Name).To(Equal("POLICY_CONFIGURATION"))
			Expect(enterpriseContractPipelineRun.Spec.Params[1].Value.StringVal).To(Equal("default/default"))
		})

		It("copies the annotations", func() {
			its := v1beta2.IntegrationTestScenario{}
			its.Annotations = map[string]string{
				"unrelated":                          "unrelated",
				"test.appstudio.openshift.io/kind":   "kind",
				"test.appstudio.openshift.io/future": "future",
			}

			ipr := tekton.IntegrationPipelineRun{}

			ipr.WithIntegrationAnnotations(&its)

			Expect(ipr.Annotations).To(Equal(map[string]string{
				"test.appstudio.openshift.io/kind":   "kind",
				"test.appstudio.openshift.io/future": "future",
			}))
		})
	})
})
