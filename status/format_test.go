package status_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redhat-appstudio/integration-service/helpers"
	"github.com/redhat-appstudio/integration-service/status"
	tektonv1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const expectedSummary = `| Task | Duration | Test Suite | Status | Details |
| --- | --- | --- | --- | --- |
| example-task-1 | 5m30s | example-namespace-1 | :heavy_check_mark: SUCCESS | :heavy_check_mark: 2 success(es)<br>:warning: 1 warning(s) |
| example-task-2 | 2m0s |  |  |  |
| example-task-3[^example-task-3] | 1s | example-namespace-3 | :x: FAILURE | :x: 1 failure(s) |
| example-task-4[^example-task-4] | 1s | example-namespace-4 | :warning: WARNING | :warning: 1 warning(s) |
| example-task-5 | 5m0s | example-namespace-5 | :white_check_mark: SKIPPED |  |
| example-task-6 | 1s | example-namespace-6 | :heavy_exclamation_mark: ERROR |  |

[^example-task-3]: example note 3
[^example-task-4]: example note 4`

func newTaskRun(name string, startTime time.Time, completionTime time.Time) *helpers.TaskRun {
	return helpers.NewTaskRunFromTektonTaskRun(name, &tektonv1beta1.TaskRunStatus{
		TaskRunStatusFields: tektonv1beta1.TaskRunStatusFields{
			StartTime:      &metav1.Time{Time: startTime},
			CompletionTime: &metav1.Time{Time: completionTime},
			TaskRunResults: []tektonv1beta1.TaskRunResult{},
		},
	})
}

func newTaskRunWithAppStudioTestOutput(name string, startTime time.Time, completionTime time.Time, output string) *helpers.TaskRun {
	return helpers.NewTaskRunFromTektonTaskRun(name, &tektonv1beta1.TaskRunStatus{
		TaskRunStatusFields: tektonv1beta1.TaskRunStatusFields{
			StartTime:      &metav1.Time{Time: startTime},
			CompletionTime: &metav1.Time{Time: completionTime},
			TaskRunResults: []tektonv1beta1.TaskRunResult{
				{
					Name:  "TEST_OUTPUT",
					Value: *tektonv1beta1.NewStructuredValues(output),
				},
			},
		},
	})
}

var _ = Describe("Formatters", func() {

	var taskRuns []*helpers.TaskRun

	BeforeEach(func() {
		now := time.Now()
		taskRuns = []*helpers.TaskRun{
			newTaskRunWithAppStudioTestOutput(
				"example-task-1",
				now,
				now.Add(time.Minute*5).Add(time.Second*30),
				`{
					"result": "SUCCESS",
					"timestamp": "1665405318",
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
					"timestamp": "1665405318",
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
					"timestamp": "1665405318",
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
					"timestamp": "1665405318",
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
					"timestamp": "1665405318",
					"namespace": "example-namespace-6",
					"successes": 0,
					"warnings": 0,
					"failures": 0
				}`,
			),
		}
	})

	It("can construct a comment", func() {
		comment, err := status.FormatComment("example-title", taskRuns)
		Expect(err).To(BeNil())
		Expect(comment).To(ContainSubstring("### example-title"))
		Expect(comment).To(ContainSubstring(expectedSummary))
	})

	It("can construct a summary", func() {
		summary, err := status.FormatSummary(taskRuns)
		Expect(err).To(BeNil())
		Expect(summary).To(Equal(expectedSummary))
	})
})
