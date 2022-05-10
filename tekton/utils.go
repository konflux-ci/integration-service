package tekton

import (
	"github.com/redhat-appstudio/integration-service/helpers"
	tektonv1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	"knative.dev/pkg/apis"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// isBuildPipelineRun returns a boolean indicating whether the object passed is a PipelineRun from
// the Build service or not.
func isBuildPipelineRun(object client.Object) bool {
	if pipelineRun, ok := object.(*tektonv1beta1.PipelineRun); ok {
		return helpers.HasAnnotationWithValue(pipelineRun,
			"pipelines.appstudio.openshift.io/type",
			"build")
	}

	return false
}

// hasPipelineSucceeded returns a boolean indicating whether the PipelineRun succeeded or not.
// If the object passed to this function is not a PipelineRun, the function will return false.
func hasPipelineSucceeded(objectOld, objectNew client.Object) bool {
	if oldPipelineRun, ok := objectOld.(*tektonv1beta1.PipelineRun); ok {
		if newPipelineRun, ok := objectNew.(*tektonv1beta1.PipelineRun); ok {
			return oldPipelineRun.Status.GetCondition(apis.ConditionSucceeded).IsUnknown() &&
				newPipelineRun.Status.GetCondition(apis.ConditionSucceeded).IsTrue()
		}
	}

	return false
}