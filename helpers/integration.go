package helpers

import (
	"context"
	"encoding/json"
	"github.com/go-logr/logr"
	hasv1alpha1 "github.com/redhat-appstudio/application-service/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/api/v1alpha1"
	tektonv1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	//HACBSTestOutputName is the name of the standardized HACBS Test output Tekton task result
	HACBSTestOutputName = "HACBS_TEST_OUTPUT"

	// HACBSTestOutputSuccess is the result that's set when the HACBS test succeeds.
	HACBSTestOutputSuccess = "SUCCESS"

	// HACBSTestOutputSkipped is the result that's set when the HACBS test gets skipped.
	HACBSTestOutputSkipped = "SKIPPED"

	// AppStudioLabelSuffix is the suffix that's added to all HACBS label headings
	AppStudioLabelSuffix = "appstudio.openshift.io"
)

// GetRequiredIntegrationTestScenariosForApplication returns the IntegrationTestScenarios used by the application being processed.
// An IntegrationTestScenarios will only be returned if it has the release.appstudio.openshift.io/optional
// label set to true or if it is missing the label entirely.
func GetRequiredIntegrationTestScenariosForApplication(adapterClient client.Client, ctx context.Context, application *hasv1alpha1.Application) (*[]v1alpha1.IntegrationTestScenario, error) {
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

// CalculateIntegrationPipelineRunOutcome checks the Tekton results for a given PipelineRun and calculates the overall outcome.
// If any of the tasks with the HACBS_TEST_OUTPUT result don't have the `result` field set to SUCCESS or SKIPPED, it returns false.
func CalculateIntegrationPipelineRunOutcome(logger logr.Logger, pipelineRun *tektonv1beta1.PipelineRun) (bool, error) {
	for _, taskRun := range pipelineRun.Status.TaskRuns {
		for _, taskRunResult := range taskRun.Status.TaskRunResults {
			if taskRunResult.Name == HACBSTestOutputName {
				var testOutput map[string]interface{}
				err := json.Unmarshal([]byte(taskRunResult.Value), &testOutput)
				if err != nil {
					return false, err
				}
				logger.Info("Found a task with",
					"HACBS Test ouput Tekton result", HACBSTestOutputName,
					"taskRun.Name", taskRun.PipelineTaskName,
					"taskRun Result", testOutput["result"])
				if testOutput["result"] != HACBSTestOutputSuccess && testOutput["result"] != HACBSTestOutputSkipped {
					return false, nil
				}
			}
		}
	}
	return true, nil
}
