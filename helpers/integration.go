package helpers

import (
	"context"
	"encoding/json"

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
	//HACBSTestOutputName is the name of the standardized HACBS Test output Tekton task result
	HACBSTestOutputName = "HACBS_TEST_OUTPUT"

	// HACBSTestOutputSuccess is the result that's set when the HACBS test succeeds.
	HACBSTestOutputSuccess = "SUCCESS"

	// HACBSTestOutputFailure is the result that's set when the HACBS test fails.
	HACBSTestOutputFailure = "FAILURE"

	// HACBSTestOutputWarning is the result that's set when the HACBS test passes with a warning.
	HACBSTestOutputWarning = "WARNING"

	// HACBSTestOutputSkipped is the result that's set when the HACBS test gets skipped.
	HACBSTestOutputSkipped = "SKIPPED"

	// HACBSTestOutputError is the result that's set when the HACBS test produces an error.
	HACBSTestOutputError = "ERROR"

	// AppStudioLabelSuffix is the suffix that's added to all HACBS label headings
	AppStudioLabelSuffix = "appstudio.openshift.io"
)

// HACBSTestResult matches HACBS TaskRun result contract
type HACBSTestResult struct {
	Result    string `json:"result"`
	Namespace string `json:"namespace"`
	Timestamp string `json:"timestamp"`
	Note      string `json:"note"`
	// Successes        int64  `json:"successes"`
	// Failures         int64  `json:"failures"`
	// Warnings         int64  `json:"warnings"`
	PipelineTaskName string
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
// If any of the tasks with the HACBS_TEST_OUTPUT result don't have the `result` field set to SUCCESS or SKIPPED, it returns false.
func CalculateIntegrationPipelineRunOutcome(logger logr.Logger, pipelineRun *tektonv1beta1.PipelineRun) (bool, error) {
	results, err := GetHACBSTestResultsFromPipelineRun(logger, pipelineRun)
	if err != nil {
		return false, err
	}
	for _, result := range results {
		if result.Result != HACBSTestOutputSuccess && result.Result != HACBSTestOutputSkipped {
			return false, nil
		}
	}
	return true, nil
}

// GetLatestPipelineRunForApplicationSnapshotAndScenario returns the latest Integration PipelineRun for the
// associated ApplicationSnapshot and IntegrationTestScenario. In the case the List operation fails,
// an error will be returned.
func GetLatestPipelineRunForApplicationSnapshotAndScenario(adapterClient client.Client, ctx context.Context, application *applicationapiv1alpha1.Application, applicationSnapshot *applicationapiv1alpha1.ApplicationSnapshot, integrationTestScenario *v1alpha1.IntegrationTestScenario) (*tektonv1beta1.PipelineRun, error) {
	integrationPipelineRuns := &tektonv1beta1.PipelineRunList{}
	var latestIntegrationPipelineRun = &tektonv1beta1.PipelineRun{}
	opts := []client.ListOption{
		client.InNamespace(application.Namespace),
		client.MatchingLabels{
			"pipelines.appstudio.openshift.io/type": "test",
			"test.appstudio.openshift.io/snapshot":  applicationSnapshot.Name,
			"test.appstudio.openshift.io/scenario":  integrationTestScenario.Name,
		},
	}

	err := adapterClient.List(ctx, integrationPipelineRuns, opts...)
	if err != nil {
		return nil, err
	}

	latestIntegrationPipelineRun = nil
	for _, pipelineRun := range integrationPipelineRuns.Items {
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

// GetHACBSTestResultsFromPipelineRun finds all TaskRuns with a HACBS_TEST_OUTPUT result and returns the parsed data
func GetHACBSTestResultsFromPipelineRun(logger logr.Logger, pipelineRun *tektonv1beta1.PipelineRun) ([]*HACBSTestResult, error) {
	results := []*HACBSTestResult{}
	for _, taskRun := range pipelineRun.Status.TaskRuns {
		for _, taskRunResult := range taskRun.Status.TaskRunResults {
			if taskRunResult.Name == HACBSTestOutputName {
				var result HACBSTestResult
				err := json.Unmarshal([]byte(taskRunResult.Value.StringVal), &result)
				if err != nil {
					return nil, err
				}
				result.PipelineTaskName = taskRun.PipelineTaskName
				results = append(results, &result)
				logger.Info("Found a HACBS test result", "Result", result)
			}
		}
	}
	return results, nil
}
