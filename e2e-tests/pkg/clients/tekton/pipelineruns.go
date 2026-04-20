package tekton

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/konflux-ci/integration-service/e2e-tests/pkg/logs"
	"github.com/konflux-ci/integration-service/e2e-tests/pkg/utils"

	pipeline "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/yaml"

	g "github.com/onsi/ginkgo/v2"
)

// CreatePipelineRun creates a tekton pipelineRun and returns the pipelineRun or error
func (t *TektonController) CreatePipelineRun(pipelineRun *pipeline.PipelineRun, ns string) (*pipeline.PipelineRun, error) {
	return t.PipelineClient().TektonV1().PipelineRuns(ns).Create(context.Background(), pipelineRun, metav1.CreateOptions{})
}

// GetPipelineRun returns a pipelineRun with a given name.
func (t *TektonController) GetPipelineRun(pipelineRunName, namespace string) (*pipeline.PipelineRun, error) {
	return t.PipelineClient().TektonV1().PipelineRuns(namespace).Get(context.Background(), pipelineRunName, metav1.GetOptions{})
}

// GetPipelineRunLogs returns logs of a given pipelineRun.
func (t *TektonController) GetPipelineRunLogs(prefix, pipelineRunName, namespace string) (string, error) {
	podClient := t.KubeInterface().CoreV1().Pods(namespace)
	podList, err := podClient.List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return "", err
	}
	podLog := ""
	for _, pod := range podList.Items {
		if !strings.HasPrefix(pod.Name, prefix) {
			continue
		}
		for _, c := range pod.Spec.InitContainers {
			var err error
			var cLog string
			cLog, err = t.fetchContainerLog(pod.Name, c.Name, namespace)
			podLog = podLog + fmt.Sprintf("\n pod: %s | init container: %s\n", pod.Name, c.Name) + cLog
			if err != nil {
				return podLog, err
			}
		}
		for _, c := range pod.Spec.Containers {
			var err error
			var cLog string
			cLog, err = t.fetchContainerLog(pod.Name, c.Name, namespace)
			podLog = podLog + fmt.Sprintf("\npod: %s | container %s: \n", pod.Name, c.Name) + cLog
			if err != nil {
				return podLog, err
			}
		}
	}
	return podLog, nil
}

func (t *TektonController) fetchContainerLog(podName, containerName, namespace string) (string, error) {
	return utils.GetContainerLogs(t.KubeInterface(), podName, containerName, namespace)
}

// WatchPipelineRunSucceeded waits until the pipelineRun succeeds.
func (t *TektonController) WatchPipelineRunSucceeded(pipelineRunName, namespace string, taskTimeout int) error {
	g.GinkgoWriter.Printf("Waiting for pipeline %q to finish\n", pipelineRunName)
	return utils.WaitUntil(t.CheckPipelineRunSucceeded(pipelineRunName, namespace), time.Duration(taskTimeout)*time.Second)
}

// CheckPipelineRunSucceeded checks if pipelineRun succeeded. Returns error if getting pipelineRun fails.
func (t *TektonController) CheckPipelineRunSucceeded(pipelineRunName, namespace string) wait.ConditionFunc {
	return func() (bool, error) {
		pr, err := t.GetPipelineRun(pipelineRunName, namespace)
		if err != nil {
			return false, err
		}
		if len(pr.Status.Conditions) > 0 {
			for _, c := range pr.Status.Conditions {
				if c.Type == "Succeeded" && c.Status == "True" {
					return true, nil
				}
			}
		}
		return false, nil
	}
}

// StorePipelineRun stores a given PipelineRun as an artifact.
func (t *TektonController) StorePipelineRun(prefix string, pipelineRun *pipeline.PipelineRun) error {
	artifacts := make(map[string][]byte)
	pipelineRunLog, err := t.GetPipelineRunLogs(prefix, pipelineRun.Name, pipelineRun.Namespace)
	if err != nil {
		g.GinkgoWriter.Printf("an error happened during storing pipelineRun log %s:%s: %s\n", pipelineRun.GetNamespace(), pipelineRun.GetName(), err.Error())
	}
	artifacts["pipelineRun-"+pipelineRun.Name+".log"] = []byte(pipelineRunLog)

	pipelineRunYaml, err := yaml.Marshal(pipelineRun)
	if err != nil {
		return err
	}
	artifacts["pipelineRun-"+pipelineRun.Name+".yaml"] = pipelineRunYaml

	if err := logs.StoreArtifacts(artifacts); err != nil {
		return err
	}

	return nil
}

func (t *TektonController) RemoveFinalizerFromPipelineRun(pipelineRun *pipeline.PipelineRun, finalizerName string) error {
	ctx := context.Background()
	kubeClient := t.KubeRest()
	patch := client.MergeFrom(pipelineRun.DeepCopy())
	if ok := controllerutil.RemoveFinalizer(pipelineRun, finalizerName); ok {
		err := kubeClient.Patch(ctx, pipelineRun, patch)
		if err != nil {
			return fmt.Errorf("error occurred while patching the updated PipelineRun after finalizer removal: %v", err)
		}
	}
	return nil
}
