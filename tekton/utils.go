package tekton

import (
	"fmt"

	"github.com/redhat-appstudio/integration-service/helpers"
	tektonv1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	"knative.dev/pkg/apis"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// PipelineRunTypeLabel contains the type of the PipelineRunTypeLabel.
	PipelineRunTypeLabel = "pipelines.appstudio.openshift.io/type"

	// PipelineRunBuildType is the type denoting a build PipelineRun.
	PipelineRunBuildType = "build"

	// PipelineRunTestType is the type denoting a test PipelineRun.
	PipelineRunTestType = "test"

	// PipelineRunComponentLabel is the label denoting the application.
	PipelineRunComponentLabel = "appstudio.openshift.io/component"

	// PipelineRunApplicationLabel is the label denoting the application.
	PipelineRunApplicationLabel = "appstudio.openshift.io/application"
)

// IsBuildPipelineRun returns a boolean indicating whether the object passed is a PipelineRun from
// the Build service or not.
func IsBuildPipelineRun(object client.Object) bool {
	if pipelineRun, ok := object.(*tektonv1beta1.PipelineRun); ok {
		return helpers.HasLabelWithValue(pipelineRun,
			PipelineRunTypeLabel,
			PipelineRunBuildType)
	}

	return false
}

// IsIntegrationPipelineRun returns a boolean indicating whether the object passed is an Integration
// Component PipelineRun
func IsIntegrationPipelineRun(object client.Object) bool {
	if pipelineRun, ok := object.(*tektonv1beta1.PipelineRun); ok {
		return helpers.HasLabelWithValue(pipelineRun,
			PipelineRunTypeLabel,
			PipelineRunTestType)
	}

	return false
}

// hasPipelineRunStateChangedToSucceeded returns a boolean indicating whether the PipelineRun status changed to succeeded or not.
// If the objects passed to this function are not PipelineRuns, the function will return false.
func hasPipelineRunStateChangedToSucceeded(objectOld, objectNew client.Object) bool {
	if oldPipelineRun, ok := objectOld.(*tektonv1beta1.PipelineRun); ok {
		if newPipelineRun, ok := objectNew.(*tektonv1beta1.PipelineRun); ok {
			return oldPipelineRun.Status.GetCondition(apis.ConditionSucceeded).IsUnknown() &&
				newPipelineRun.Status.GetCondition(apis.ConditionSucceeded).IsTrue()
		}
	}

	return false
}

// hasPipelineRunStateChangedToStarted returns a boolean indicating whether the PipelineRun just started.
// If the objects passed to this function are not PipelineRuns, the function will return false.
func hasPipelineRunStateChangedToStarted(objectOld, objectNew client.Object) bool {
	if oldPipelineRun, ok := objectOld.(*tektonv1beta1.PipelineRun); ok {
		if newPipelineRun, ok := objectNew.(*tektonv1beta1.PipelineRun); ok {
			return (oldPipelineRun.Status.StartTime == nil || oldPipelineRun.Status.StartTime.IsZero()) &&
				(newPipelineRun.Status.StartTime != nil && !newPipelineRun.Status.StartTime.IsZero())
		}
	}

	return false
}

// GetTypeFromPipelineRun extracts the pipeline type from the pipelineRun labels.
func GetTypeFromPipelineRun(object client.Object) (string, error) {
	if pipelineRun, ok := object.(*tektonv1beta1.PipelineRun); ok {
		if pipelineType, found := pipelineRun.Labels[PipelineRunTypeLabel]; found {
			return pipelineType, nil
		}
	}
	return "", fmt.Errorf("the pipelineRun has no type associated with it")
}

// GetOutputImage returns a string containing the output-image parameter value from a given PipelineRun.
func GetOutputImage(object client.Object) (string, error) {
	if pipelineRun, ok := object.(*tektonv1beta1.PipelineRun); ok {
		for _, pipelineResult := range pipelineRun.Status.PipelineResults {
			if pipelineResult.Name == "IMAGE_URL" {
				return pipelineResult.Value.StringVal, nil
			}
		}
	}
	return "", fmt.Errorf("couldn't find the output-image PipelineRun param")
}

// GetOutputImageDigest returns a string containing the IMAGE_DIGEST result value from a given PipelineRun.
func GetOutputImageDigest(object client.Object) (string, error) {
	if pipelineRun, ok := object.(*tektonv1beta1.PipelineRun); ok {
		for _, pipelineResult := range pipelineRun.Status.PipelineResults {
			if pipelineResult.Name == "IMAGE_DIGEST" {
				return pipelineResult.Value.StringVal, nil
			}
		}
	}
	return "", fmt.Errorf("couldn't find the IMAGE_DIGEST TaskRun result")
}

// GetComponentSourceGitUrl returns a string containing the CHAINS-GIT_URL result value from a given PipelineRun.
func GetComponentSourceGitUrl(object client.Object) (string, error) {
	if pipelineRun, ok := object.(*tektonv1beta1.PipelineRun); ok {
		for _, pipelineResult := range pipelineRun.Status.PipelineResults {
			if pipelineResult.Name == "CHAINS-GIT_URL" {
				return pipelineResult.Value.StringVal, nil
			}
		}
	}
	return "", fmt.Errorf("couldn't find the CHAINS-GIT_URL PipelineRun result")
}

// GetComponentSourceGitCommit returns a string containing the CHAINS-GIT_COMMIT result value from a given PipelineRun.
func GetComponentSourceGitCommit(object client.Object) (string, error) {
	if pipelineRun, ok := object.(*tektonv1beta1.PipelineRun); ok {
		for _, pipelineResult := range pipelineRun.Status.PipelineResults {
			if pipelineResult.Name == "CHAINS-GIT_COMMIT" {
				return pipelineResult.Value.StringVal, nil
			}
		}
	}
	return "", fmt.Errorf("couldn't find the CHAINS-GIT_COMMIT PipelineRun result")
}
