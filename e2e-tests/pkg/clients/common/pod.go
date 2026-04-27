package common

import (
	"context"

	"github.com/konflux-ci/integration-service/e2e-tests/pkg/utils"
	ginkgo "github.com/onsi/ginkgo/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ListAllPods returns a list of all pods in a namespace.
func (s *SuiteController) ListAllPods(namespace string) (*corev1.PodList, error) {
	return s.KubeInterface().CoreV1().Pods(namespace).List(context.Background(), metav1.ListOptions{})
}

func (s *SuiteController) GetPodLogs(pod *corev1.Pod) map[string][]byte {
	podLogs := make(map[string][]byte)

	var containers []corev1.Container
	containers = append(containers, pod.Spec.InitContainers...)
	containers = append(containers, pod.Spec.Containers...)
	for _, c := range containers {
		log, err := utils.GetContainerLogs(s.KubeInterface(), pod.Name, c.Name, pod.Namespace)
		if err != nil {
			ginkgo.GinkgoWriter.Printf("error getting logs for pod/container %s/%s: %v\n", pod.Name, c.Name, err.Error())
			continue
		}

		podLogs["pod-"+pod.Name+"-"+c.Name+".log"] = []byte(log)
	}

	return podLogs
}
