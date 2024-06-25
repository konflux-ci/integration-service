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

package status_test

import (
	"os"
	"time"

	"github.com/go-logr/logr"
	"github.com/konflux-ci/integration-service/helpers"
	"github.com/konflux-ci/integration-service/status"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/apis"
	v1 "knative.dev/pkg/apis/duck/v1"
)

const expectedSummary = `<ul>
<li><b>Pipelinerun</b>: <a href="https://definetly.not.prod/preview/application-pipeline/ns/default/pipelinerun/pipelinerun-component-sample">pipelinerun-component-sample</a></li>
</ul>
<hr>

| Task | Duration | Test Suite | Status | Details |
| --- | --- | --- | --- | --- |
| <a href="https://definetly.not.prod/preview/application-pipeline/ns/default/pipelinerun/pipelinerun-component-sample/logs/example-task-1">example-task-1</a> | 5m30s | example-namespace-1 | :heavy_check_mark: SUCCESS | :heavy_check_mark: 2 success(es)<br>:warning: 1 warning(s) |
| <a href="https://definetly.not.prod/preview/application-pipeline/ns/default/pipelinerun/pipelinerun-component-sample/logs/example-task-2">example-task-2</a> | 2m0s |  | :heavy_check_mark: Reason: Succeeded | :heavy_check_mark: Reason: Succeeded |
| <a href="https://definetly.not.prod/preview/application-pipeline/ns/default/pipelinerun/pipelinerun-component-sample/logs/example-task-3">example-task-3[^example-task-3]</a> | 1s | example-namespace-3 | :x: FAILURE | :x: 1 failure(s) |
| <a href="https://definetly.not.prod/preview/application-pipeline/ns/default/pipelinerun/pipelinerun-component-sample/logs/example-task-4">example-task-4[^example-task-4]</a> | 1s | example-namespace-4 | :warning: WARNING | :warning: 1 warning(s) |
| <a href="https://definetly.not.prod/preview/application-pipeline/ns/default/pipelinerun/pipelinerun-component-sample/logs/example-task-5">example-task-5</a> | 5m0s | example-namespace-5 | :white_check_mark: SKIPPED |  |
| <a href="https://definetly.not.prod/preview/application-pipeline/ns/default/pipelinerun/pipelinerun-component-sample/logs/example-task-6">example-task-6</a> | 1s | example-namespace-6 | :heavy_exclamation_mark: ERROR |  |

[^example-task-3]: example note 3
[^example-task-4]: example note 4`

const expectedTaskLogURL = `https://definetly.not.prod/preview/application-pipeline/ns/default/pipelinerun/pipelinerun-component-sample/logs/example-task-1`

var message = "Taskrun Succeeded, lucky you!"

func newTaskRun(name string, startTime time.Time, completionTime time.Time) *helpers.TaskRun {
	return helpers.NewTaskRunFromTektonTaskRun(name, &tektonv1.TaskRunStatus{
		Status: v1.Status{
			Conditions: v1.Conditions{apis.Condition{
				Type:    apis.ConditionType("Succeeded"),
				Reason:  string(apis.ConditionSucceeded),
				Message: message,
			}},
		},
		TaskRunStatusFields: tektonv1.TaskRunStatusFields{
			StartTime:      &metav1.Time{Time: startTime},
			CompletionTime: &metav1.Time{Time: completionTime},
			Results:        []tektonv1.TaskRunResult{},
		},
	})
}

func newTaskRunWithAppStudioTestOutput(name string, startTime time.Time, completionTime time.Time, output string) *helpers.TaskRun {
	return helpers.NewTaskRunFromTektonTaskRun(name, &tektonv1.TaskRunStatus{
		Status: v1.Status{
			Conditions: v1.Conditions{apis.Condition{
				Type:    apis.ConditionType("Succeeded"),
				Reason:  string(apis.ConditionSucceeded),
				Message: message,
			}},
		},
		TaskRunStatusFields: tektonv1.TaskRunStatusFields{
			StartTime:      &metav1.Time{Time: startTime},
			CompletionTime: &metav1.Time{Time: completionTime},
			Results: []tektonv1.TaskRunResult{
				{
					Name:  "TEST_OUTPUT",
					Value: *tektonv1.NewStructuredValues(output),
				},
			},
		},
	})
}

func newTaskRunWithoutAppStudioTestOutput(name string, startTime time.Time, completionTime time.Time) *helpers.TaskRun {
	return helpers.NewTaskRunFromTektonTaskRun(name, &tektonv1.TaskRunStatus{
		Status: v1.Status{
			Conditions: v1.Conditions{apis.Condition{
				Type:    apis.ConditionType("Succeeded"),
				Reason:  string(apis.ConditionSucceeded),
				Message: message,
			}},
		},
		TaskRunStatusFields: tektonv1.TaskRunStatusFields{
			StartTime:      &metav1.Time{Time: startTime},
			CompletionTime: &metav1.Time{Time: completionTime},
			Results:        []tektonv1.TaskRunResult{},
		},
	})
}

var _ = Describe("Formatters", func() {

	var taskRuns []*helpers.TaskRun
	var pipelineRun *tektonv1.PipelineRun

	BeforeEach(func() {
		now := time.Now()
		os.Setenv("CONSOLE_URL", "https://definetly.not.prod/preview/application-pipeline/ns/{{ .Namespace }}/pipelinerun/{{ .PipelineRunName }}")
		os.Setenv("CONSOLE_URL_TASKLOG", "https://definetly.not.prod/preview/application-pipeline/ns/{{ .Namespace }}/pipelinerun/{{ .PipelineRunName }}/logs/{{ .TaskName }}")
		taskRuns = []*helpers.TaskRun{
			newTaskRunWithAppStudioTestOutput(
				"example-task-1",
				now,
				now.Add(time.Minute*5).Add(time.Second*30),
				`{
					"result": "SUCCESS",
					"timestamp": "2024-05-22T06:42:21+00:00",
					"namespace": "example-namespace-1",
					"successes": 2,
					"warnings": 1,
					"failures": 0
				}`,
			),
			newTaskRun(
				"example-task-2",
				now.Add(time.Minute*-2),
				now,
			),
			newTaskRunWithAppStudioTestOutput(
				"example-task-3",
				now.Add(time.Second*3),
				now.Add(time.Second*4),
				`{
					"result": "FAILURE",
					"timestamp": "2024-05-22T06:42:21+00:00",
					"namespace": "example-namespace-3",
					"successes": 0,
					"warnings": 0,
					"failures": 1,
					"note": "example note 3"
				}`,
			),
			newTaskRunWithAppStudioTestOutput(
				"example-task-4",
				now.Add(time.Second*4),
				now.Add(time.Second*5),
				`{
					"result": "WARNING",
					"timestamp": "2024-05-22T06:42:21+00:00",
					"namespace": "example-namespace-4",
					"successes": 0,
					"warnings": 1,
					"failures": 0,
					"note": "example note 4"
				}`,
			),
			newTaskRunWithAppStudioTestOutput(
				"example-task-5",
				now.Add(time.Minute*-5),
				now,
				`{
					"result": "SKIPPED",
					"timestamp": "2024-05-22T06:42:21+00:00",
					"namespace": "example-namespace-5",
					"successes": 0,
					"warnings": 0,
					"failures": 0
				}`,
			),
			newTaskRunWithAppStudioTestOutput(
				"example-task-6",
				now.Add(time.Second*6),
				now.Add(time.Second*7),
				`{
					"result": "ERROR",
					"timestamp": "2024-05-22T06:42:21+00:00",
					"namespace": "example-namespace-6",
					"successes": 0,
					"warnings": 0,
					"failures": 0
				}`,
			),
		}
		pipelineRun = &tektonv1.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pipelinerun-component-sample",
				Namespace: "default",
				Labels: map[string]string{
					"pipelines.appstudio.openshift.io/type":           "test",
					"pac.test.appstudio.openshift.io/url-org":         "redhat-appstudio",
					"pac.test.appstudio.openshift.io/original-prname": "build-service-on-push",
					"pac.test.appstudio.openshift.io/url-repository":  "build-service",
					"pac.test.appstudio.openshift.io/repository":      "build-service-pac",
				},
				Annotations: map[string]string{
					"pac.test.appstudio.openshift.io/on-target-branch": "[main]",
				},
			},
			Spec: tektonv1.PipelineRunSpec{
				PipelineRef: &tektonv1.PipelineRef{
					Name: "component-pipeline-pass",
					ResolverRef: tektonv1.ResolverRef{
						Resolver: "bundle",
						Params: tektonv1.Params{
							{
								Name:  "bundle",
								Value: tektonv1.ParamValue{Type: "string", StringVal: "quay.io/redhat-appstudio/test-bundle:component-pipeline-pass"},
							},
							{
								Name:  "name",
								Value: tektonv1.ParamValue{Type: "string", StringVal: "test-task"},
							},
						},
					},
				},
			},
		}
	})
	AfterEach(func() {
		os.Setenv("CONSOLE_URL", "")
		os.Setenv("CONSOLE_URL_TASKLOG", "")
	})

	It("CONSOLE_URL env var not set", func() {
		os.Setenv("CONSOLE_URL", "")
		text, err := status.FormatTestsSummary(taskRuns, pipelineRun.Name, pipelineRun.Namespace, logr.Discard())
		Expect(err).To(Succeed())
		Expect(text).To(ContainSubstring("https://CONSOLE_URL_NOT_AVAILABLE"))
	})

	It("CONSOLE_URL_TASKLOG env var not set", func() {
		os.Setenv("CONSOLE_URL_TASKLOG", "")
		text := status.FormatTaskLogURL(taskRuns[0], pipelineRun.Name, pipelineRun.Namespace, logr.Discard())
		Expect(text).To(ContainSubstring("https://CONSOLE_URL_TASKLOG_NOT_AVAILABLE"))
	})

	It("can construct a comment", func() {
		text, err := status.FormatTestsSummary(taskRuns, pipelineRun.Name, pipelineRun.Namespace, logr.Discard())
		Expect(err).To(Succeed())
		comment, err := status.FormatComment("example-title", text)
		Expect(err).To(BeNil())
		Expect(comment).To(ContainSubstring("### example-title"))
		Expect(comment).To(ContainSubstring(expectedSummary))
	})

	It("can construct a taskLogURL", func() {
		taskLogUrl := status.FormatTaskLogURL(taskRuns[0], pipelineRun.Name, pipelineRun.Namespace, logr.Discard())
		Expect(taskLogUrl).To(Equal(expectedTaskLogURL))
	})

	It("can construct a summary", func() {
		summary, err := status.FormatTestsSummary(taskRuns, pipelineRun.Name, pipelineRun.Namespace, logr.Discard())
		Expect(err).To(BeNil())
		Expect(summary).To(Equal(expectedSummary))
	})
	//when TEST_OUTPUT == "" is also invalid
	When("task TEST_OUTPUT is invalid", func() {

		var taskRun *helpers.TaskRun

		BeforeEach(func() {
			now := time.Now()
			taskRun = newTaskRunWithAppStudioTestOutput(
				"example-task-1",
				now,
				now.Add(time.Minute*5).Add(time.Second*30),
				`{
					"result": "INVALID",
				}`,
			)
		})

		It("can construct a detail from an invalid result", func() {
			detail, err := status.FormatDetails(taskRun)
			Expect(err).To(Succeed())
			Expect(detail).Should(ContainSubstring("Invalid result:"))
		})

		It("won't fail when summary is generated from invalid result", func() {
			_, err := status.FormatTestsSummary([]*helpers.TaskRun{taskRun}, pipelineRun.Name, pipelineRun.Namespace, logr.Discard())
			Expect(err).To(Succeed())
		})
	})
	When("task TEST_OUTPUT is nil", func() {
		var taskRun *helpers.TaskRun

		BeforeEach(func() {
			now := time.Now()
			taskRun = newTaskRunWithoutAppStudioTestOutput(
				"example-task-1",
				now,
				now.Add(time.Minute*5).Add(time.Second*30),
			)
		})
		It("can construct a detail from an taskrun without TEST_OUTPUT", func() {
			detail, err := status.FormatDetails(taskRun)
			Expect(err).To(Succeed())
			Expect(detail).Should(ContainSubstring("Reason: Succeeded"))
		})

		It("won't fail when summary is generated from taskrun without TEST_OUTPUT", func() {
			_, err := status.FormatTestsSummary([]*helpers.TaskRun{taskRun}, pipelineRun.Name, pipelineRun.Namespace, logr.Discard())
			Expect(err).To(Succeed())
		})
	})

})
