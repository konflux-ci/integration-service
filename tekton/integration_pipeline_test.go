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
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/konflux-ci/integration-service/api/v1beta2"
	"github.com/konflux-ci/integration-service/helpers"
	"github.com/konflux-ci/integration-service/loader"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/tonglil/buflogr"

	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	"github.com/konflux-ci/integration-service/gitops"
	tekton "github.com/konflux-ci/integration-service/tekton"
	tektonconsts "github.com/konflux-ci/integration-service/tekton/consts"
	knative "knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"

	toolkit "github.com/konflux-ci/operator-toolkit/loader"
	"github.com/konflux-ci/operator-toolkit/metadata"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	resolutionv1beta1 "github.com/tektoncd/pipeline/pkg/apis/resolution/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// the pipelinerun yaml gotten from git resolver is prepared for resolutionRequest unittest
const expectedPipelineYAML = `---
apiVersion: tekton.dev/v1
kind: PipelineRun
metadata:
  generateName: integration-pipelinerun-
spec:
  pipelineRef:
    resolver: git
    params:
      - name: url
        value: http://github.com/test/integration-examples.git
      - name: revision
        value: main
      - name: pathInRepo
        value: pipelines/integration_test_app.yaml`

type ExtraParams struct {
	Name  string
	Value tektonv1.ParamValue
}

func getParamValue(params tektonv1.Params, name string) string {
	for _, p := range params {
		if p.Name == name {
			return p.Value.StringVal
		}
	}
	return ""
}

var _ = Describe("Integration pipeline", Ordered, func() {

	const (
		prefix          = "testpipeline"
		namespace       = "default"
		applicationName = "application-sample"
		targetRepoUrl   = "https://github.com/redhat-appstudio/integration-examples.git"
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
		mockLoader                      loader.ObjectLoader
		logger                          helpers.IntegrationLogger
	)

	BeforeAll(func() {
		mockLoader = loader.NewMockLoader()

	})

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
					ResourceKind: tektonconsts.ResourceKindPipeline,
					Resolver:     "git",
					Params: []v1beta2.ResolverParameter{
						{
							Name:  "url",
							Value: targetRepoUrl,
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
					ResourceKind: tektonconsts.ResourceKindPipeline,
					Resolver:     "git",
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
				Annotations: map[string]string{
					gitops.PipelineAsCodeGitSourceURLAnnotation: "https://test-repo.example.com",
					gitops.PipelineAsCodeSHAAnnotation:          "test-commit",
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

		//create new integration pipeline run from integration test scenario
		var err error
		var buf bytes.Buffer
		log := helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(&buf)}
		newIntegrationPipelineRun, err = tekton.NewIntegrationPipelineRun(k8sClient, ctx, loader.NewMockLoader(), log, prefix, namespace, integrationTestScenarioGit, hasSnapshot)
		Expect(err).NotTo(HaveOccurred())
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
					ResourceKind: tektonconsts.ResourceKindPipeline,
					Resolver:     "bundles",
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

		newIntegrationBundlePipelineRun, err = tekton.NewIntegrationPipelineRun(k8sClient, ctx, mockLoader, log, prefix, namespace, integrationTestScenarioBundle, hasSnapshot)
		Expect(err).NotTo(HaveOccurred())
		newIntegrationBundlePipelineRun = newIntegrationBundlePipelineRun.WithIntegrationLabels(integrationTestScenarioBundle).
			WithSnapshot(hasSnapshot, integrationTestScenarioBundle).
			WithApplication(hasApp)
		snapshotString, _ := json.Marshal(hasSnapshot.Spec)
		Expect(k8sClient.Create(ctx, newIntegrationBundlePipelineRun.AsPipelineRun())).Should(Succeed())
		actualValue := getParamValue(newIntegrationBundlePipelineRun.Spec.Params, "SNAPSHOT")
		Expect(actualValue).To(Equal(string(snapshotString)))

		Expect(metadata.SetAnnotation(enterpriseContractTestScenario, tekton.SnapshotParamAsNameAnnotation, "true")).Should(Succeed())
		enterpriseContractPipelineRun, err = tekton.NewIntegrationPipelineRun(k8sClient, ctx, mockLoader, log, prefix, namespace, enterpriseContractTestScenario, hasSnapshot)
		Expect(err).NotTo(HaveOccurred())
		enterpriseContractPipelineRun = enterpriseContractPipelineRun.WithIntegrationLabels(enterpriseContractTestScenario).
			WithIntegrationAnnotations(enterpriseContractTestScenario).
			WithSnapshot(hasSnapshot, enterpriseContractTestScenario).
			WithExtraParams(enterpriseContractTestScenario.Spec.Params).
			WithApplication(hasApp)
		Expect(k8sClient.Create(ctx, enterpriseContractPipelineRun.AsPipelineRun())).Should(Succeed())
		actualValue = getParamValue(enterpriseContractPipelineRun.Spec.Params, "SNAPSHOT")
		Expect(actualValue).To(Equal(string(hasSnapshot.Name)))

		os.Setenv("PIPELINE_TIMEOUT", "2h")
		os.Setenv("TASKS_TIMEOUT", "2h")
		os.Setenv("FINALLY_TIMEOUT", "2h")
	})

	AfterEach(func() {
		_ = k8sClient.Delete(ctx, integrationTestScenarioGit)
		_ = k8sClient.Delete(ctx, integrationTestScenarioBundle)
		_ = k8sClient.Delete(ctx, enterpriseContractTestScenario)
		_ = k8sClient.Delete(ctx, newIntegrationPipelineRun.AsPipelineRun())
		_ = k8sClient.Delete(ctx, newIntegrationPipelineRun)
		_ = k8sClient.Delete(ctx, newIntegrationBundlePipelineRun)
		_ = k8sClient.Delete(ctx, enterpriseContractPipelineRun)
		_ = k8sClient.Delete(ctx, hasApp)
		_ = k8sClient.Delete(ctx, hasSnapshot)
		_ = k8sClient.Delete(ctx, hasComp)

		os.Setenv("PIPELINE_TIMEOUT", "")
		os.Setenv("TASKS_TIMEOUT", "")
		os.Setenv("FINALLY_TIMEOUT", "")
	})

	Context("When creating a new IntegrationPipelineRun", func() {
		It("can create an IntegrationPipelineRun", func() {
			buf := bytes.Buffer{}
			log := helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(&buf)}
			plr, err := tekton.NewIntegrationPipelineRun(k8sClient, ctx, mockLoader, log, prefix, namespace, integrationTestScenarioGit, hasSnapshot)
			Expect(err).NotTo(HaveOccurred())
			Expect(plr).NotTo(BeNil())
			Expect(k8sClient.Create(ctx, plr.AsPipelineRun())).Should(Succeed())
		})

		It("can create an IntegrationPipelineRun with 'pipelinerun' ResourceKind", func() {
			hasSnapshot.Annotations[gitops.PipelineAsCodeRepoURLAnnotation] = targetRepoUrl
			hasSnapshot.Annotations[gitops.PipelineAsCodeTargetBranchAnnotation] = "main"
			hasSnapshot.Labels[gitops.PipelineAsCodePullRequestAnnotation] = "1"
			hasSnapshot.Labels[gitops.PipelineAsCodeEventTypeLabel] = "pull_request"

			integrationTestScenarioGit.Spec.ResolverRef.ResourceKind = tektonconsts.ResourceKindPipelineRun
			resolutionRequest := resolutionv1beta1.ResolutionRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "sample-resolutionrequest",
					Namespace: "default",
				},
				Spec: resolutionv1beta1.ResolutionRequestSpec{
					Params: []tektonv1.Param{
						{
							Name: "url",
							Value: tektonv1.ParamValue{
								Type:      tektonv1.ParamTypeString,
								StringVal: "http://github.com/test/integration-examples.git",
							},
						},
						{
							Name: "pathInRepo",
							Value: tektonv1.ParamValue{
								Type:      tektonv1.ParamTypeString,
								StringVal: "pipelineruns/integration_pipelinerun_pass.yaml",
							},
						},
						{
							Name: "revision",
							Value: tektonv1.ParamValue{
								Type:      tektonv1.ParamTypeString,
								StringVal: "main",
							},
						},
					},
				},
				Status: resolutionv1beta1.ResolutionRequestStatus{
					ResolutionRequestStatusFields: resolutionv1beta1.ResolutionRequestStatusFields{
						Data: tekton.GenerateCleanData(expectedPipelineYAML),
					},
					Status: duckv1.Status{
						Conditions: []knative.Condition{
							{
								Type:   knative.ConditionSucceeded,
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
			}
			mockContext := toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: loader.ResolutionRequestContextKey,
					Resource:   resolutionRequest,
				},
			})
			Expect(mockContext).NotTo(BeNil())
			plr, err := tekton.NewIntegrationPipelineRun(k8sClient, mockContext, loader.NewMockLoader(), logger, prefix, namespace, integrationTestScenarioGit, hasSnapshot)
			Expect(err).NotTo(HaveOccurred())
			Expect(plr).NotTo(BeNil())
			Expect(k8sClient.Create(ctx, plr.AsPipelineRun())).Should(Succeed())

			foundUrl := false
			foundRevision := false
			expectedPipelinerun, err := tekton.ConvertStringToPipelinerun(expectedPipelineYAML)
			Expect(err).NotTo(HaveOccurred())

			for _, param := range plr.Spec.PipelineRef.Params {
				for _, expectedParam := range expectedPipelinerun.Spec.PipelineRef.Params {
					if param.Name == "url" && expectedParam.Name == "url" {
						foundUrl = true
						Expect(helpers.UrlToGitUrl(param.Value.StringVal)).To(Equal(helpers.UrlToGitUrl(expectedParam.Value.StringVal))) // must have .git suffix
					}
					if param.Name == "revision" && expectedParam.Name == "revision" {
						foundRevision = true
						Expect(param.Value.StringVal).To(Equal(expectedParam.Value.StringVal))
					}
				}
			}
			Expect(foundUrl).To(BeTrue())
			Expect(foundRevision).To(BeTrue())
		})
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
			newIntegrationPipelineRun.WithSnapshot(hasSnapshot, integrationTestScenarioGit)
			Expect(newIntegrationPipelineRun.Labels["appstudio.openshift.io/snapshot"]).
				To(Equal(hasSnapshot.Name))
			Expect(newIntegrationPipelineRun.Labels["appstudio.openshift.io/component"]).
				To(Equal(hasComp.Name))
			Expect(newIntegrationPipelineRun.Labels[gitops.CustomLabelPrefix+"/custom_label"]).
				To(Equal(hasSnapshot.Labels[gitops.CustomLabelPrefix+"/custom_label"]))
			Expect(newIntegrationPipelineRun.Labels[gitops.SnapshotTypeLabel]).
				To(Equal(hasSnapshot.Labels[gitops.SnapshotTypeLabel]))
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

		It("sets the service account correctly", func() {
			serviceAccountName := "konflux-integration-runner"

			ipr := tekton.IntegrationPipelineRun{}
			ipr.WithDefaultServiceAccount(serviceAccountName)

			Expect(ipr.Spec.TaskRunTemplate.ServiceAccountName).To(Equal(serviceAccountName))
		})

		It("doesn't overwrite the service account if it's already set", func() {
			serviceAccountName := "konflux-integration-runner"

			ipr := tekton.IntegrationPipelineRun{}
			ipr.Spec.TaskRunTemplate.ServiceAccountName = "someotherserviceaccount"
			ipr.WithDefaultServiceAccount(serviceAccountName)

			Expect(ipr.Spec.TaskRunTemplate.ServiceAccountName).To(Equal("someotherserviceaccount"))
		})
	})

	Context("When testing WithUpdatedTestsGitResolver with edge cases", func() {
		It("should handle pipeline run created from base64 with missing resolver ref", func() {
			// This simulates a pipeline run created via generateIntegrationPipelineRunFromBase64
			// where the structure might not be fully initialized
			pipelineRun := &tekton.IntegrationPipelineRun{
				tektonv1.PipelineRun{
					Spec: tektonv1.PipelineRunSpec{
						PipelineRef: &tektonv1.PipelineRef{
							Name:        "some-pipeline", // Has name but no resolver
							ResolverRef: tektonv1.ResolverRef{
								// Empty resolver ref - this was causing the panic
							},
						},
					},
				},
			}

			params := map[string]string{
				"url":      "https://github.com/test/repo.git",
				"revision": "main",
			}

			// This should not panic - it should return early because resolver is empty
			result := pipelineRun.WithUpdatedPipelineGitResolver(params)
			Expect(result).NotTo(BeNil())
			Expect(result).To(Equal(pipelineRun)) // Should return the same instance unchanged
		})

		It("should handle pipeline run with git resolver but empty params", func() {
			pipelineRun := &tekton.IntegrationPipelineRun{
				tektonv1.PipelineRun{
					Spec: tektonv1.PipelineRunSpec{
						PipelineRef: &tektonv1.PipelineRef{
							ResolverRef: tektonv1.ResolverRef{
								Resolver: tektonv1.ResolverName(tektonconsts.TektonResolverGit),
								Params:   nil, // This was causing the panic
							},
						},
					},
				},
			}

			params := map[string]string{
				"url":      "https://github.com/test/repo.git",
				"revision": "main",
			}

			// This should not panic - should return early because params is nil
			result := pipelineRun.WithUpdatedPipelineGitResolver(params)
			Expect(result).NotTo(BeNil())
			Expect(result.Spec.PipelineRef.ResolverRef.Params).To(BeNil())
		})

		It("should handle pipeline run with git resolver but empty params slice", func() {
			pipelineRun := &tekton.IntegrationPipelineRun{
				tektonv1.PipelineRun{
					Spec: tektonv1.PipelineRunSpec{
						PipelineRef: &tektonv1.PipelineRef{
							ResolverRef: tektonv1.ResolverRef{
								Resolver: tektonv1.ResolverName(tektonconsts.TektonResolverGit),
								Params:   []tektonv1.Param{}, // Empty slice
							},
						},
					},
				},
			}

			params := map[string]string{
				"url":      "https://github.com/test/repo.git",
				"revision": "main",
			}

			// This should not panic - should return early because params is empty
			result := pipelineRun.WithUpdatedPipelineGitResolver(params)
			Expect(result).NotTo(BeNil())
			Expect(result.Spec.PipelineRef.ResolverRef.Params).To(BeEmpty())
		})

		It("should handle pipeline run with non-git resolver (bundles)", func() {
			pipelineRun := &tekton.IntegrationPipelineRun{
				tektonv1.PipelineRun{
					Spec: tektonv1.PipelineRunSpec{
						PipelineRef: &tektonv1.PipelineRef{
							ResolverRef: tektonv1.ResolverRef{
								Resolver: tektonv1.ResolverName("bundles"), // Not git
								Params: []tektonv1.Param{
									{
										Name: "bundle",
										Value: tektonv1.ParamValue{
											Type:      tektonv1.ParamTypeString,
											StringVal: "quay.io/test/bundle:latest",
										},
									},
								},
							},
						},
					},
				},
			}

			params := map[string]string{
				"url":      "https://github.com/test/repo.git",
				"revision": "main",
			}

			// Should not modify non-git resolvers
			result := pipelineRun.WithUpdatedPipelineGitResolver(params)
			Expect(result).NotTo(BeNil())
			Expect(result.Spec.PipelineRef.ResolverRef.Params[0].Value.StringVal).To(Equal("quay.io/test/bundle:latest"))
		})

		It("should successfully update git resolver parameters", func() {
			pipelineRun := &tekton.IntegrationPipelineRun{
				tektonv1.PipelineRun{
					Spec: tektonv1.PipelineRunSpec{
						PipelineRef: &tektonv1.PipelineRef{
							ResolverRef: tektonv1.ResolverRef{
								Resolver: tektonv1.ResolverName(tektonconsts.TektonResolverGit),
								Params: []tektonv1.Param{
									{
										Name: "url",
										Value: tektonv1.ParamValue{
											Type:      tektonv1.ParamTypeString,
											StringVal: "https://github.com/old/repo.git",
										},
									},
									{
										Name: "revision",
										Value: tektonv1.ParamValue{
											Type:      tektonv1.ParamTypeString,
											StringVal: "old-branch",
										},
									},
								},
							},
						},
					},
				},
			}

			params := map[string]string{
				"url":      "https://github.com/new/repo.git",
				"revision": "main",
			}

			result := pipelineRun.WithUpdatedPipelineGitResolver(params)
			Expect(result).NotTo(BeNil())
			Expect(result.Spec.PipelineRef.ResolverRef.Params[0].Value.StringVal).To(Equal("https://github.com/new/repo.git"))
			Expect(result.Spec.PipelineRef.ResolverRef.Params[1].Value.StringVal).To(Equal("main"))
		})

		It("should handle nil params map gracefully", func() {
			// This simulates a pipeline run created via generateIntegrationPipelineRunFromBase64
			// where the structure might not be fully initialized
			pipelineRun := &tekton.IntegrationPipelineRun{
				tektonv1.PipelineRun{
					Spec: tektonv1.PipelineRunSpec{
						PipelineRef: &tektonv1.PipelineRef{
							Name: "some-pipeline", // Has name but no resolver
							ResolverRef: tektonv1.ResolverRef{
								Resolver: tektonv1.ResolverName(tektonconsts.TektonResolverGit),
								Params: []tektonv1.Param{
									{
										Name: "url",
										Value: tektonv1.ParamValue{
											Type:      tektonv1.ParamTypeString,
											StringVal: "https://github.com/test/repo.git",
										},
									},
									{
										Name: "revision",
										Value: tektonv1.ParamValue{
											Type:      tektonv1.ParamTypeString,
											StringVal: "main",
										},
									},
								},
							},
						},
					},
				},
			}

			result := pipelineRun.WithUpdatedPipelineGitResolver(nil)
			Expect(result).NotTo(BeNil())
			// Should return unchanged
		})

		It("should successfully update task git resolver parameters", func() {
			pipelineRun := &tekton.IntegrationPipelineRun{
				tektonv1.PipelineRun{
					Spec: tektonv1.PipelineRunSpec{
						PipelineSpec: &tektonv1.PipelineSpec{
							Params: []tektonv1.ParamSpec{},
							Tasks: []tektonv1.PipelineTask{
								{
									Name:   "some-task",
									Params: []tektonv1.Param{},
									TaskRef: &tektonv1.TaskRef{
										Name: "some-task",
										ResolverRef: tektonv1.ResolverRef{
											Resolver: tektonv1.ResolverName(tektonconsts.TektonResolverGit),
											Params: []tektonv1.Param{
												{
													Name: "url",
													Value: tektonv1.ParamValue{
														Type:      tektonv1.ParamTypeString,
														StringVal: "https://github.com/test/repo.git",
													},
												},
												{
													Name: "revision",
													Value: tektonv1.ParamValue{
														Type:      tektonv1.ParamTypeString,
														StringVal: "main",
													},
												},
											},
										},
									},
								},
								{
									Name:   "some-task2",
									Params: []tektonv1.Param{},
									TaskRef: &tektonv1.TaskRef{
										Name: "some-task2",
										ResolverRef: tektonv1.ResolverRef{
											Resolver: tektonv1.ResolverName(tektonconsts.TektonResolverGit),
											Params: []tektonv1.Param{
												{
													Name: "url",
													Value: tektonv1.ParamValue{
														Type:      tektonv1.ParamTypeString,
														StringVal: "https://github.com/someotherrepo/repo.git",
													},
												},
												{
													Name: "revision",
													Value: tektonv1.ParamValue{
														Type:      tektonv1.ParamTypeString,
														StringVal: "main",
													},
												},
											},
										},
									},
								},
								{
									Name:   "some-task3",
									Params: []tektonv1.Param{},
									TaskRef: &tektonv1.TaskRef{
										Name: "some-task2",
										ResolverRef: tektonv1.ResolverRef{
											Resolver: tektonv1.ResolverName(tektonconsts.TektonResolverBundle),
											Params: []tektonv1.Param{
												{
													Name: "bundle",
													Value: tektonv1.ParamValue{
														Type:      tektonv1.ParamTypeString,
														StringVal: "quay.io/somerepo/some-bundle",
													},
												},
											},
										},
									},
								},
								{
									Name:   "some-task4",
									Params: []tektonv1.Param{},
									TaskSpec: &tektonv1.EmbeddedTask{
										TaskSpec: tektonv1.TaskSpec{
											DisplayName: "some-task4",
											Steps:       []tektonv1.Step{},
										},
									},
								},
							},
						},
					},
				},
			}

			params := map[string]string{
				"url":      "https://github.com/new/repo.git",
				"revision": "feature-branch",
			}

			hasSnapshot.Annotations = map[string]string{}
			hasSnapshot.Annotations[gitops.PipelineAsCodeTargetBranchAnnotation] = "main"
			hasSnapshot.Annotations[gitops.PipelineAsCodeRepoURLAnnotation] = "https://github.com/test/repo.git"

			result := pipelineRun.WithUpdatedTasksGitResolver(hasSnapshot, params)
			Expect(result).NotTo(BeNil())
			// We expect the first task to be updated as it matches the target repo and branch
			Expect(result.Spec.PipelineSpec.Tasks[0].TaskRef.Params[0].Value.StringVal).To(Equal("https://github.com/new/repo.git"))
			Expect(result.Spec.PipelineSpec.Tasks[0].TaskRef.Params[1].Value.StringVal).To(Equal("feature-branch"))
			// We expect the second task to not be updated since it originates from a different repo
			Expect(result.Spec.PipelineSpec.Tasks[1].TaskRef.Params[0].Value.StringVal).To(Equal("https://github.com/someotherrepo/repo.git"))
			Expect(result.Spec.PipelineSpec.Tasks[1].TaskRef.Params[1].Value.StringVal).To(Equal("main"))
			// We expect the third task to not be updated since it uses the bundle resolver
			Expect(result.Spec.PipelineSpec.Tasks[2].TaskRef.Params[0].Value.StringVal).To(Equal("quay.io/somerepo/some-bundle"))
		})
	})
})
