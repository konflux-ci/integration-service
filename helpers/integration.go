package helpers

import (
	"context"
	"encoding/json"
	"fmt"
	"k8s.io/apimachinery/pkg/types"
	"reflect"
	"sort"
	"time"

	"github.com/go-logr/logr"
	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/api/v1alpha1"
	"github.com/santhosh-tekuri/jsonschema/v5"
	tektonv1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"knative.dev/pkg/apis"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (

	//TestOutputName is the name of the standardized Test output Tekton task result
	TestOutputName = "TEST_OUTPUT"

	//LegacyTestOutputName is the previous name of the standardized AppStudio Test output Tekton task result
	LegacyTestOutputName = "HACBS_TEST_OUTPUT"

	// AppStudioTestOutputSuccess is the result that's set when the AppStudio test succeeds.
	AppStudioTestOutputSuccess = "SUCCESS"

	// AppStudioTestOutputFailure is the result that's set when the AppStudio test fails.
	AppStudioTestOutputFailure = "FAILURE"

	// AppStudioTestOutputWarning is the result that's set when the AppStudio test passes with a warning.
	AppStudioTestOutputWarning = "WARNING"

	// AppStudioTestOutputSkipped is the result that's set when the AppStudio test gets skipped.
	AppStudioTestOutputSkipped = "SKIPPED"

	// AppStudioTestOutputError is the result that's set when the AppStudio test produces an error.
	AppStudioTestOutputError = "ERROR"
)

// AppStudioTestResult matches AppStudio TaskRun result contract
type AppStudioTestResult struct {
	Result    string `json:"result"`
	Namespace string `json:"namespace"`
	Timestamp string `json:"timestamp"`
	Note      string `json:"note"`
	Successes int    `json:"successes"`
	Failures  int    `json:"failures"`
	Warnings  int    `json:"warnings"`
}

var testResultSchema = `{
  "$schema": "http://json-schema.org/draft/2020-12/schema#",
  "type": "object",
  "properties": {
    "result": {
      "type": "string",
      "enum": ["SUCCESS", "FAILURE", "WARNING", "SKIPPED", "ERROR"]
    },
    "namespace": {
      "type": "string"
    },
    "timestamp": {
      "type": "string",
      "pattern": "^[0-9]{10}$"
    },
    "successes": {
      "type": "integer",
      "minimum": 0
    },
    "note": {
      "type": "string"
    },
    "failures": {
      "type": "integer",
      "minimum": 0
    },
    "warnings": {
      "type": "integer",
      "minimum": 0
    }
  },
  "required": ["result", "timestamp", "successes", "failures", "warnings"]
}`

// TaskRun is an integration specific wrapper around the status of a Tekton TaskRun.
type TaskRun struct {
	logger           logr.Logger
	pipelineTaskName string
	trStatus         *tektonv1beta1.TaskRunStatus
	testResult       *AppStudioTestResult
}

// NewTaskRunFromTektonTaskRun creates and returns am integration TaskRun from the TaskRunStatus.
func NewTaskRunFromTektonTaskRun(logger logr.Logger, pipelineTaskName string, status *tektonv1beta1.TaskRunStatus) *TaskRun {
	return &TaskRun{logger: logger, pipelineTaskName: pipelineTaskName, trStatus: status}
}

// GetPipelineTaskName returns the name of the PipelineTask.
func (t *TaskRun) GetPipelineTaskName() string {
	return t.pipelineTaskName
}

// GetStartTime returns the start time of the TaskRun.
// If the start time is unknown, the zero start time is returned.
func (t *TaskRun) GetStartTime() time.Time {
	if t.trStatus.StartTime == nil {
		return time.Time{}
	}
	return t.trStatus.StartTime.Time
}

// GetDuration returns the time it took to execute the Task.
// If the start or end times are unknown, a duration of 0 is returned.
func (t *TaskRun) GetDuration() time.Duration {
	var end time.Time
	start := t.GetStartTime()
	if t.trStatus.CompletionTime != nil {
		end = t.trStatus.CompletionTime.Time
	} else {
		end = start
	}
	return end.Sub(start)
}

// GetTestResult returns a AppStudioTestResult if the TaskRun produced the result. It will return nil otherwise.
func (t *TaskRun) GetTestResult() (*AppStudioTestResult, error) {
	// Check for an already parsed result.
	if t.testResult != nil {
		return t.testResult, nil
	}
	// load schema for test validation
	sch, err := jsonschema.CompileString("schema.json", testResultSchema)
	if err != nil {
		return nil, fmt.Errorf("error while compiling json data for schema validation: %w", err)
	}

	for _, taskRunResult := range t.trStatus.TaskRunResults {
		if taskRunResult.Name == LegacyTestOutputName || taskRunResult.Name == TestOutputName {
			var result AppStudioTestResult
			var v interface{}
			err := json.Unmarshal([]byte(taskRunResult.Value.StringVal), &result)
			if err != nil {
				return nil, fmt.Errorf("error while mapping json data from taskRun %s: to AppStudioTestResult %w", taskRunResult.Name, err)
			}
			if err := json.Unmarshal([]byte(taskRunResult.Value.StringVal), &v); err != nil {
				return nil, fmt.Errorf("error while mapping json data from taskRun %s: %w", taskRunResult.Name, err)
			}
			if err = sch.Validate(v); err != nil {
				return nil, fmt.Errorf("error validating schema of results from taskRun %s: %w", taskRunResult.Name, err)
			}
			t.logger.Info("Found a AppStudio test result", "Result", result)
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
// If any of the tasks with the TEST_OUTPUT result don't have the `result` field set to SUCCESS or SKIPPED, it returns false.
func CalculateIntegrationPipelineRunOutcome(adapterClient client.Client, ctx context.Context, logger logr.Logger, pipelineRun *tektonv1beta1.PipelineRun) (bool, error) {
	var results []*AppStudioTestResult
	var err error
	// Check if the pipelineRun failed from the conditions of status
	if HasPipelineRunFinished(pipelineRun) && !HasPipelineRunSucceeded(pipelineRun) {
		logger.Error(fmt.Errorf("PipelineRun %s in namespace %s failed for %s", pipelineRun.Name, pipelineRun.Namespace, GetPipelineRunFailedReason(pipelineRun)), "PipelineRun failed without test results of TaskRuns")
		return false, nil
	}
	// Check if the pipelineRun.Status contains the childReferences to TaskRuns
	if !reflect.ValueOf(pipelineRun.Status.ChildReferences).IsZero() {
		// If the pipelineRun.Status contains the childReferences, parse them in the new way by querying for TaskRuns
		results, err = GetAppStudioTestResultsFromPipelineRunWithChildReferences(adapterClient, ctx, logger, pipelineRun)
		if err != nil {
			return false, fmt.Errorf("error while getting test results from pipelineRun %s: %w", pipelineRun.Name, err)
		}
	}

	for _, result := range results {
		if result.Result != AppStudioTestOutputSuccess && result.Result != AppStudioTestOutputSkipped {
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

// GetAllBuildPipelineRunsForComponent returns all PipelineRun for the
// associated component. In the case the List operation fails,
// an error will be returned.
func GetAllBuildPipelineRunsForComponent(adapterClient client.Client, ctx context.Context, component *applicationapiv1alpha1.Component) (*[]tektonv1beta1.PipelineRun, error) {
	buildPipelineRuns := &tektonv1beta1.PipelineRunList{}
	opts := []client.ListOption{
		client.InNamespace(component.Namespace),
		client.MatchingLabels{
			"pipelines.appstudio.openshift.io/type": "build",
			"appstudio.openshift.io/component":      component.Name,
		},
	}

	err := adapterClient.List(ctx, buildPipelineRuns, opts...)
	if err != nil {
		return nil, err
	}
	return &buildPipelineRuns.Items, nil
}

// GetSucceededBuildPipelineRunsForComponent returns all  succeeded PipelineRun for the
// associated component. In the case the List operation fails,
// an error will be returned.
func GetSucceededBuildPipelineRunsForComponent(adapterClient client.Client, ctx context.Context, component *applicationapiv1alpha1.Component) (*[]tektonv1beta1.PipelineRun, error) {
	var succeededPipelineRuns []tektonv1beta1.PipelineRun

	buildPipelineRuns, err := GetAllBuildPipelineRunsForComponent(adapterClient, ctx, component)
	if err != nil {
		return nil, err
	}

	for _, pipelineRun := range *buildPipelineRuns {
		pipelineRun := pipelineRun // G601
		if HasPipelineRunSucceeded(&pipelineRun) {
			succeededPipelineRuns = append(succeededPipelineRuns, pipelineRun)
		}
	}
	return &succeededPipelineRuns, nil
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

// GetAppStudioTestResultsFromPipelineRunWithChildReferences finds all TaskRuns from childReferences of the PipelineRun
// that also contain a TEST_OUTPUT result and returns the parsed data
func GetAppStudioTestResultsFromPipelineRunWithChildReferences(adapterClient client.Client, ctx context.Context, logger logr.Logger, pipelineRun *tektonv1beta1.PipelineRun) ([]*AppStudioTestResult, error) {
	taskRuns, err := GetAllChildTaskRunsForPipelineRun(adapterClient, ctx, logger, pipelineRun)
	if err != nil {
		return nil, err
	}

	results := []*AppStudioTestResult{}
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

// GetAllChildTaskRunsForPipelineRun finds all Child TaskRuns for a given PipelineRun and
// returns integration TaskRun wrappers for them sorted by start time.
func GetAllChildTaskRunsForPipelineRun(adapterClient client.Client, ctx context.Context, logger logr.Logger, pipelineRun *tektonv1beta1.PipelineRun) ([]*TaskRun, error) {
	taskRuns := []*TaskRun{}
	// If there are no childReferences, skip trying to get tasks
	if reflect.ValueOf(pipelineRun.Status.ChildReferences).IsZero() {
		return nil, nil
	}
	for _, childReference := range pipelineRun.Status.ChildReferences {
		pipelineTaskRun := &tektonv1beta1.TaskRun{}
		err := adapterClient.Get(ctx, types.NamespacedName{
			Namespace: pipelineRun.Namespace,
			Name:      childReference.Name,
		}, pipelineTaskRun)
		if err != nil {
			return nil, fmt.Errorf("error while getting the child taskRun %s from pipelineRun: %w", childReference.Name, err)
		}

		integrationTaskRun := NewTaskRunFromTektonTaskRun(logger, childReference.PipelineTaskName, &pipelineTaskRun.Status)
		taskRuns = append(taskRuns, integrationTaskRun)
	}
	sort.Sort(SortTaskRunsByStartTime(taskRuns))
	return taskRuns, nil
}

// HasPipelineRunSucceeded returns a boolean indicating whether the PipelineRun succeeded or not.
// If the object passed to this function is not a PipelineRun, the function will return false.
func HasPipelineRunSucceeded(object client.Object) bool {
	if pr, ok := object.(*tektonv1beta1.PipelineRun); ok {
		return pr.Status.GetCondition(apis.ConditionSucceeded).IsTrue()
	}

	return false
}

// GetPipelineRunFailedReason returns a string indicating why the PipelineRun failed.
// If the object passed to this function is not a PipelineRun, the function will return "".
func GetPipelineRunFailedReason(object client.Object) string {
	var reason string
	reason = ""
	if pr, ok := object.(*tektonv1beta1.PipelineRun); ok {
		if pr.Status.GetCondition(apis.ConditionSucceeded).IsFalse() {
			reason = pr.Status.GetCondition(apis.ConditionSucceeded).GetReason()
		}
	}
	return reason
}

// HasPipelineRunFinished returns a boolean indicating whether the PipelineRun finished or not.
// If the object passed to this function is not a PipelineRun, the function will return false.
func HasPipelineRunFinished(object client.Object) bool {
	if pr, ok := object.(*tektonv1beta1.PipelineRun); ok {
		return (pr.Status.GetCondition(apis.ConditionSucceeded).IsFalse() || pr.Status.GetCondition(apis.ConditionSucceeded).IsTrue())
	}

	return false
}
