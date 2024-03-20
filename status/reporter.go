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

//go:generate mockgen -destination mock_reporter.go -package status github.com/redhat-appstudio/integration-service/status ReporterInterface

import (
	"context"
	"fmt"
	"time"

	pacv1alpha1 "github.com/openshift-pipelines/pipelines-as-code/pkg/apis/pipelinesascode/v1alpha1"
	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/redhat-appstudio/integration-service/gitops"
	intgteststat "github.com/redhat-appstudio/integration-service/pkg/integrationteststatus"
)

type TestReport struct {
	// FullName describing the snapshot and integration test
	FullName string
	// Name of scenario
	ScenarioName string
	// Name of snapshot
	SnapshotName string
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
}

type ReporterInterface interface {
	// Detect if the reporter can be used with the snapshot
	Detect(*applicationapiv1alpha1.Snapshot) bool
	// Initialize reporter to be able update statuses (authenticate, fetching metadata)
	Initialize(context.Context, *applicationapiv1alpha1.Snapshot) error
	// Get plain reporter name
	GetReporterName() string
	// Update status of the integration test
	ReportStatus(context.Context, TestReport) error
}

// GetPACGitProviderToken lookup for configured repo and fetch token from namespace
func GetPACGitProviderToken(ctx context.Context, k8sClient client.Client, snapshot *applicationapiv1alpha1.Snapshot) (string, error) {
	var err error

	// List all the Repository CRs in the namespace
	repos := pacv1alpha1.RepositoryList{}
	if err = k8sClient.List(ctx, &repos, &client.ListOptions{Namespace: snapshot.Namespace}); err != nil {
		return "", err
	}

	// Get the full repo URL
	url, found := snapshot.GetAnnotations()[gitops.PipelineAsCodeRepoURLAnnotation]
	if !found {
		return "", fmt.Errorf("object annotation not found %q", gitops.PipelineAsCodeRepoURLAnnotation)
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
		return "", fmt.Errorf("failed to find a Repository matching URL: %q", url)
	}

	// Get the pipelines as code secret from the PipelineRun's namespace
	pacSecret := v1.Secret{}
	err = k8sClient.Get(ctx, types.NamespacedName{Namespace: snapshot.Namespace, Name: repoSecret.Name}, &pacSecret)
	if err != nil {
		return "", err
	}

	// Get the personal access token from the secret
	token, found := pacSecret.Data[repoSecret.Key]
	if !found {
		return "", fmt.Errorf("failed to find %s secret key", repoSecret.Key)
	}

	return string(token), nil
}
