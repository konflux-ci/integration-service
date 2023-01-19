package helpers

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"github.com/go-logr/logr"
	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/api/v1alpha1"
	tektonv1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"knative.dev/pkg/apis"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// TODO check effects of modifying output name
	//StonesoupTestOutputName is the name of the standardized Stonesoup Test output Tekton task result
	StonesoupTestOutputName = "STONESOUP_TEST_OUTPUT"

	// StonesoupTestOutputSuccess is the result that's set when the Stonesoup test succeeds.
	StonesoupTestOutputSuccess = "SUCCESS"

	// StonesoupTestOutputFailure is the result that's set when the Stonesoup test fails.
	StonesoupTestOutputFailure = "FAILURE"

	// StonesoupTestOutputWarning is the result that's set when the Stonesoup test passes with a warning.
	StonesoupTestOutputWarning = "WARNING"

	// StonesoupTestOutputSkipped is the result that's set when the Stonesoup test gets skipped.
	StonesoupTestOutputSkipped = "SKIPPED"

	// StonesoupTestOutputError is the result that's set when the Stonesoup test produces an error.
	StonesoupTestOutputError = "ERROR"

	// AppStudioLabelSuffix is the suffix that's added to all Stonesoup label headings
	AppStudioLabelSuffix = "appstudio.openshift.io"
)

// TODO understand the TaskRun result contract and implications of making the following changes
// StonesoupTestResult matches Stonesoup TaskRun result contract
type StonesoupTestResult struct {
	Result    string `json:"result"`
	Namespace string `json:"namespace"`
	Timestamp string `json:"timestamp"`
	Note      string `json:"note"`
	Successes int    `json:"successes"`
	Failures  int    `json:"failures"`
	Warnings  int    `json:"warnings"`
}

// TaskRun is an integration specific wrapper around the status of a Tekton TaskRun.
type TaskRun struct {
	logger     logr.Logger
	trStatus   *tektonv1beta1.PipelineRunTaskRunStatus
	testResult *StonesoupTestResult
}

// NewTaskRun creates and returns am integration TaskRun.
func NewTaskRun(logger logr.Logger, status *tektonv1beta1.PipelineRunTaskRunStatus) *TaskRun {
	return &TaskRun{logger: logger, trStatus: status}
}

// GetPipelinesTaskName returns the name of the PipelineTask.
func (t *TaskRun) GetPipelineTaskName() string {
	return t.trStatus.PipelineTaskName
}

// GetStartTime returns the start time of the TaskRun.
// If the start time is unknown, the zero start time is returned.
func (t *TaskRun) GetStartTime() time.Time {
	if t.trStatus.Status.StartTime == nil {
		return time.Time{}
	}
	return t.trStatus.Status.StartTime.Time
}

// GetDuration returns the time it took to execute the Task.
// If the start or end times are unknown, a duration of 0 is returned.
func (t *TaskRun) GetDuration() time.Duration {
	var end time.Time
	start := t.GetStartTime()
	if t.trStatus.Status.CompletionTime != nil {
		end = t.trStatus.Status.CompletionTime.Time
	} else {
		end = start
	}
	return end.Sub(start)
}

// GetTestResult returns a StonesoupTestResult if the TaskRun produced the result. It will return nil otherwise.
func (t *TaskRun) GetTestResult() (*StonesoupTestResult, error) {
	// Check for an already parsed result.
	if t.testResult != nil {
		return t.testResult, nil
	}

	for _, taskRunResult := range t.trStatus.Status.TaskRunResults {
		if taskRunResult.Name == StonesoupTestOutputName {
			var result StonesoupTestResult
			err := json.Unmarshal([]byte(taskRunResult.Value.StringVal), &result)
			if err != nil {
				return nil, err
			}
			t.logger.Info("Found a Stonesoup test result", "Result", result)
			t.testResult = &result
			return &result, nil
		}
	}
	return nil, nil
}

// SortTaskRunsByStartTime can sort TaskRuns by their start time. It implements sort.Interface.
type SortTaskRunsByStartTime []*TaskRun

// Len returns the length of the slice being sorted.
func (s SortTaskRunsByStartTime) Len() int {
	return len(s)
}

// Swap switches the position of two elements in the slice.
func (s SortTaskRunsByStartTime) Swap(i int, j int) {
	s[i], s[j] = s[j], s[i]
}

// Less determines if TaskRun in position i started before TaskRun in position j.
func (s SortTaskRunsByStartTime) Less(i int, j int) bool {
	return s[i].GetStartTime().Before(s[j].GetStartTime())
}

// GetRequiredIntegrationTestScenariosForApplication returns the IntegrationTestScenarios used by the application being processed.
// An IntegrationTestScenarios will only be returned if it has the test.appstudio.openshift.io/optional
// label not set to true or if it is missing the label entirely.
func GetRequiredIntegrationTestScenariosForApplication(adapterClient client.Client, ctx context.Context, application *applicationapiv1alpha1.Application) (*[]v1alpha1.IntegrationTestScenario, error) {
	integrationList := &v1alpha1.IntegrationTestScenarioList{}
	labelRequirement, err := labels.NewRequirement("test.appstudio.openshift.io/optional", selection.NotIn, []string{"true"})
	if err != nil {
		return nil, err
	}
	labelSelector := labels.NewSelector().Add(*labelRequirement)

	opts := &client.ListOptions{
		Namespace:     application.Namespace,
		FieldSelector: fields.OneTermEqualSelector("spec.application", application.Name),
		LabelSelector: labelSelector,
	}

	err = adapterClient.List(ctx, integrationList, opts)
	if err != nil {
		return nil, err
	}

	return &integrationList.Items, nil
}

// GetAllIntegrationTestScenariosForApplication returns all IntegrationTestScenarios used by the application being processed.
func GetAllIntegrationTestScenariosForApplication(adapterClient client.Client, ctx context.Context, application *applicationapiv1alpha1.Application) (*[]v1alpha1.IntegrationTestScenario, error) {
	integrationList := &v1alpha1.IntegrationTestScenarioList{}

	opts := &client.ListOptions{
		Namespace:     application.Namespace,
		FieldSelector: fields.OneTermEqualSelector("spec.application", application.Name),
	}

	err := adapterClient.List(ctx, integrationList, opts)
	if err != nil {
		return nil, err
	}

	return &integrationList.Items, nil
}

// CalculateIntegrationPipelineRunOutcome checks the Tekton results for a given PipelineRun and calculates the overall outcome.
// If any of the tasks with the STONESOUP_TEST_OUTPUT result don't have the `result` field set to SUCCESS or SKIPPED, it returns false.
func CalculateIntegrationPipelineRunOutcome(logger logr.Logger, pipelineRun *tektonv1beta1.PipelineRun) (bool, error) {
	results, err := GetStonesoupTestResultsFromPipelineRun(logger, pipelineRun)
	if err != nil {
		return false, err
	}
	for _, result := range results {
		if result.Result != StonesoupTestOutputSuccess && result.Result != StonesoupTestOutputSkipped {
			return false, nil
		}
	}
	return true, nil
}

// GetAllPipelineRunsForSnapshotAndScenario returns all Integration PipelineRun for the
// associated Snapshot and IntegrationTestScenario. In the case the List operation fails,
// an error will be returned.
func GetAllPipelineRunsForSnapshotAndScenario(adapterClient client.Client, ctx context.Context, snapshot *applicationapiv1alpha1.Snapshot, integrationTestScenario *v1alpha1.IntegrationTestScenario) (*[]tektonv1beta1.PipelineRun, error) {
	integrationPipelineRuns := &tektonv1beta1.PipelineRunList{}
	opts := []client.ListOption{
		client.InNamespace(snapshot.Namespace),
		client.MatchingLabels{
			"pipelines.appstudio.openshift.io/type": "test",
			"appstudio.openshift.io/snapshot":       snapshot.Name,
			"test.appstudio.openshift.io/scenario":  integrationTestScenario.Name,
		},
	}

	err := adapterClient.List(ctx, integrationPipelineRuns, opts...)
	if err != nil {
		return nil, err
	}
	return &integrationPipelineRuns.Items, nil
}

// GetLatestPipelineRunForSnapshotAndScenario returns the latest Integration PipelineRun for the
// associated Snapshot and IntegrationTestScenario. In the case the List operation fails,
// an error will be returned.
func GetLatestPipelineRunForSnapshotAndScenario(adapterClient client.Client, ctx context.Context, snapshot *applicationapiv1alpha1.Snapshot, integrationTestScenario *v1alpha1.IntegrationTestScenario) (*tektonv1beta1.PipelineRun, error) {
	var latestIntegrationPipelineRun = &tektonv1beta1.PipelineRun{}
	integrationPipelineRuns, err := GetAllPipelineRunsForSnapshotAndScenario(adapterClient, ctx, snapshot, integrationTestScenario)
	if err != nil {
		return nil, err
	}

	latestIntegrationPipelineRun = nil
	for _, pipelineRun := range *integrationPipelineRuns {
		pipelineRun := pipelineRun // G601
		if pipelineRun.Status.GetCondition(apis.ConditionSucceeded).IsTrue() {
			if latestIntegrationPipelineRun == nil {
				latestIntegrationPipelineRun = &pipelineRun
			} else {
				if pipelineRun.Status.CompletionTime.Time.After(latestIntegrationPipelineRun.Status.CompletionTime.Time) {
					latestIntegrationPipelineRun = &pipelineRun
				}
			}
		}
	}
	if latestIntegrationPipelineRun != nil {
		return latestIntegrationPipelineRun, nil
	}

	return nil, err
}

// GetStonesoupTestResultsFromPipelineRun finds all TaskRuns with a STONESOUP_TEST_OUTPUT result and returns the parsed data
func GetStonesoupTestResultsFromPipelineRun(logger logr.Logger, pipelineRun *tektonv1beta1.PipelineRun) ([]*StonesoupTestResult, error) {
	taskRuns := GetTaskRunsFromPipelineRun(logger, pipelineRun)
	results := []*StonesoupTestResult{}
	for _, tr := range taskRuns {
		r, err := tr.GetTestResult()
		if err != nil {
			return nil, err
		}
		if r != nil {
			results = append(results, r)
		}
	}
	return results, nil
}

// GetTaskRunsFromPipelineRun returns integration TaskRun wrappers for all Tekton TaskRuns in a PipelineRun sorted by start time.
func GetTaskRunsFromPipelineRun(logger logr.Logger, pipelineRun *tektonv1beta1.PipelineRun) []*TaskRun {
	taskRuns := []*TaskRun{}
	for _, tr := range pipelineRun.Status.TaskRuns {
		taskRuns = append(taskRuns, NewTaskRun(logger, tr))
	}
	sort.Sort(SortTaskRunsByStartTime(taskRuns))
	return taskRuns
}
