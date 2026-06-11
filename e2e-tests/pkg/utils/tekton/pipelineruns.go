package tekton

import (
	"context"
	"fmt"
	"strings"

	"github.com/konflux-ci/integration-service/e2e-tests/pkg/utils"

	pipeline "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type FailedPipelineRunDetails struct {
	FailedTaskRunName   string
	PodName             string
	FailedContainerName string
}

// GetFailedPipelineRunLogs gets the logs of the pipelinerun failed task
func GetFailedPipelineRunLogs(c crclient.Client, ki kubernetes.Interface, pipelineRun *pipeline.PipelineRun) (string, error) {
	var d *FailedPipelineRunDetails
	var err error

	failMessage := fmt.Sprintf("Pipelinerun '%s' didn't succeed\n", pipelineRun.Name)

	for _, cond := range pipelineRun.Status.Conditions {
		if cond.Reason == "CouldntGetPipeline" {
			failMessage += fmt.Sprintf("CouldntGetPipeline message: %s", cond.Message)
		}
	}
	if d, err = GetFailedPipelineRunDetails(c, pipelineRun); err != nil {
		return "", err
	}

	if d != nil && d.FailedContainerName != "" {
		logs, err := utils.GetContainerLogs(ki, d.PodName, d.FailedContainerName, pipelineRun.Namespace)

		switch {
		case logs != "":
			failMessage += fmt.Sprintf("Logs from failed container '%s/%s': \n%s",
				d.FailedTaskRunName, d.FailedContainerName, logs)
		case err != nil:
			failMessage += fmt.Sprintf("Failed to get logs for container '%s/%s': %v",
				d.FailedTaskRunName, d.FailedContainerName, err)
		default:
			failMessage += fmt.Sprintf("Failed container '%s/%s' (no logs available)",
				d.FailedTaskRunName, d.FailedContainerName)
		}
	}
	return failMessage, nil
}

func GetFailedPipelineRunDetails(c crclient.Client, pipelineRun *pipeline.PipelineRun) (*FailedPipelineRunDetails, error) {
	d := &FailedPipelineRunDetails{}
	for _, chr := range pipelineRun.Status.ChildReferences {
		taskRun := &pipeline.TaskRun{}
		taskRunKey := types.NamespacedName{Namespace: pipelineRun.Namespace, Name: chr.Name}
		if err := c.Get(context.Background(), taskRunKey, taskRun); err != nil {
			return nil, fmt.Errorf("failed to get details for PR %s: %+v", pipelineRun.GetName(), err)
		}
		for _, c := range taskRun.Status.Conditions {
			if c.Reason == "Failed" {
				d.FailedTaskRunName = taskRun.Name
				d.PodName = taskRun.Status.PodName
				for _, s := range taskRun.Status.Steps {
					if s.Terminated != nil && (s.Terminated.Reason == "Error" || strings.Contains(s.Terminated.Reason, "Failed")) {
						d.FailedContainerName = s.Container
						return d, nil
					}
				}
			}
		}
	}
	return d, nil
}
