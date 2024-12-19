/*
Copyright 2024 Red Hat Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package status

//go:generate mockgen -destination mock_reporter.go -package status github.com/konflux-ci/integration-service/status ReporterInterface

import (
	"context"
	"fmt"
	"time"

	pacv1alpha1 "github.com/openshift-pipelines/pipelines-as-code/pkg/apis/pipelinesascode/v1alpha1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/konflux-ci/integration-service/gitops"
	"github.com/konflux-ci/integration-service/helpers"
	intgteststat "github.com/konflux-ci/integration-service/pkg/integrationteststatus"
)

type TestReport struct {
	// FullName describing the snapshot/buildPLR and integration test
	FullName string
	// Name of scenario
	ScenarioName string
	// Name of snapshot or build pipelinerun
	ObjectName string
	// Name of Component that triggered snapshot creation (optional)
	ComponentName string
	// text with details of test results
	Text string
	// test status
	Status intgteststat.IntegrationTestStatus
	// short summary of test results
	Summary string
	// time when test started
	StartTime *time.Time
	// time when test completed
	CompletionTime *time.Time
	// pipelineRun Name
	TestPipelineRunName string
}

type ReporterInterface interface {
	// Detect if the reporter can be used with the snapshot or build pipelineRun
	Detect(metav1.Object) bool
	// Initialize reporter to be able to update statuses (authenticate, fetching snapshot or build PLR metadata)
	Initialize(context.Context, metav1.Object) error
	// Get plain reporter name
	GetReporterName() string
	// Update status of the integration test
	ReportStatus(context.Context, TestReport) error
}

// GetPACGitProviderToken lookup for configured repo and fetch token from namespace
func GetPACGitProviderToken(ctx context.Context, k8sClient client.Client, obj metav1.Object) (string, error) {
	log := log.FromContext(ctx)
	var err, unRecoverableError error

	// List all the Repository CRs in the namespace
	repos := pacv1alpha1.RepositoryList{}
	if err = k8sClient.List(ctx, &repos, &client.ListOptions{Namespace: obj.GetNamespace()}); err != nil {
		log.Error(err, fmt.Sprintf("failed to get repo from namespace %s", obj.GetNamespace()))
		return "", err
	}

	// Get the full repo URL
	url, found := GetPACAnnotation(obj, obj.GetAnnotations(), gitops.RepoURLAnnotationSuffix)
	if !found {
		unRecoverableError = helpers.NewUnrecoverableMetadataError("object annotation PipelineAsCodeRepoURLAnnotation not found")
		log.Error(unRecoverableError, fmt.Sprintf("object annotation PipelineAsCodeRepoURLAnnotation not found /%s", gitops.RepoURLAnnotationSuffix))
		return "", unRecoverableError
	}

	// Find a Repository CR with a matching URL and get its secret details
	var repoSecret *pacv1alpha1.Secret
	for _, repo := range repos.Items {
		if url == repo.Spec.URL {
			repoSecret = repo.Spec.GitProvider.Secret
			break
		}
	}

	if repoSecret == nil {
		unRecoverableError = helpers.NewUnrecoverableMetadataError(fmt.Sprintf("failed to find a Repository matching URL: %q", url))
		log.Error(unRecoverableError, fmt.Sprintf("failed to find a Repository matching URL: %q", url))
		return "", unRecoverableError
	}

	// Get the pipelines as code secret from the PipelineRun's namespace
	pacSecret := v1.Secret{}
	err = k8sClient.Get(ctx, types.NamespacedName{Namespace: obj.GetNamespace(), Name: repoSecret.Name}, &pacSecret)
	if err != nil {
		log.Error(err, fmt.Sprintf("failed to get secret %s/%s", obj.GetNamespace(), repoSecret.Name))
		return "", err
	}

	// Get the personal access token from the secret
	token, found := pacSecret.Data[repoSecret.Key]
	if !found {
		unRecoverableError = helpers.NewUnrecoverableMetadataError(fmt.Sprintf("failed to find %s secret key", repoSecret.Key))
		log.Error(unRecoverableError, fmt.Sprintf("failed to find %s secret key", repoSecret.Key))
		return "", unRecoverableError
	}

	return string(token), nil
}
