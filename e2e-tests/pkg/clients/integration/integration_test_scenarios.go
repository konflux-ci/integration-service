package integration

import (
	"context"
	"fmt"
	"strings"
	"time"

	integrationv1beta2 "github.com/konflux-ci/integration-service/api/v1beta2"
	"github.com/konflux-ci/integration-service/e2e-tests/pkg/constants"
	"github.com/konflux-ci/integration-service/e2e-tests/pkg/utils"
	tektonconsts "github.com/konflux-ci/integration-service/tekton/consts"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	gomega "github.com/onsi/gomega"
)

// createScenarioWithRetry creates the given IntegrationTestScenario, retrying on
// API errors. A scenario is typically created right after its Application, so a
// stale informer cache in the admission webhook (or a webhook call skipped
// during an API server hiccup) can make the first Create fail with an admission
// denial such as "could not find application ..." or "owner reference not set"
// (STONEINTG-1912). Retrying converges once the webhook can see the Application.
func (i *IntegrationController) createScenarioWithRetry(integrationTestScenario *integrationv1beta2.IntegrationTestScenario) (*integrationv1beta2.IntegrationTestScenario, error) {
	var lastErr error
	gomega.Eventually(func() bool {
		lastErr = i.KubeRest().Create(context.Background(), integrationTestScenario)
		if lastErr == nil {
			return true
		}
		if k8sErrors.IsAlreadyExists(lastErr) {
			lastErr = i.KubeRest().Get(context.Background(),
				client.ObjectKeyFromObject(integrationTestScenario), integrationTestScenario)
			return lastErr == nil
		}
		return false
	}, time.Minute, 5*time.Second).Should(gomega.BeTrue(),
		fmt.Sprintf("failed to create integration test scenario '%s' in namespace '%s'", integrationTestScenario.Name, integrationTestScenario.Namespace))
	return integrationTestScenario, lastErr
}

// CreateIntegrationTestScenario creates beta1 version integrationTestScenario.
func (i *IntegrationController) CreateIntegrationTestScenario(itsName, applicationName, namespace, gitURL, revision, pathInRepo, kind string, contexts []string) (*integrationv1beta2.IntegrationTestScenario, error) {
	if itsName == "" {
		itsName = "integration-test-" + utils.GenerateRandomString(4)
	}

	params := []integrationv1beta2.ResolverParameter{
		{
			Name:  "url",
			Value: gitURL,
		},
		{
			Name:  "revision",
			Value: revision,
		},
		{
			Name:  "pathInRepo",
			Value: pathInRepo,
		},
	}

	integrationTestScenario := &integrationv1beta2.IntegrationTestScenario{
		ObjectMeta: metav1.ObjectMeta{
			Name:      itsName,
			Namespace: namespace,
			Labels:    constants.IntegrationTestScenarioDefaultLabels,
		},
		Spec: integrationv1beta2.IntegrationTestScenarioSpec{
			Application: applicationName,
			ResolverRef: integrationv1beta2.ResolverRef{
				Resolver: "git",
				Params:   params,
			},
			Contexts: []integrationv1beta2.TestContext{},
		},
	}

	// Add kind parameter if provided and is "pipelineRun"
	if strings.EqualFold(kind, "pipelineRun") {
		integrationTestScenario.Spec.ResolverRef.ResourceKind = "pipelinerun"

	}

	if len(contexts) > 0 {
		for _, testContext := range contexts {
			integrationTestScenario.Spec.Contexts = append(integrationTestScenario.Spec.Contexts,
				integrationv1beta2.TestContext{Name: testContext, Description: testContext})
		}
	}

	return i.createScenarioWithRetry(integrationTestScenario)
}

// CreateOptionalIntegrationTestScenario creates a beta1 version integrationTestScenario with optional: true label.
// This function is identical to CreateIntegrationTestScenario except it sets the optional label to "true".
func (i *IntegrationController) CreateOptionalIntegrationTestScenario(itsName, applicationName, namespace, gitURL, revision, pathInRepo, kind string, contexts []string) (*integrationv1beta2.IntegrationTestScenario, error) {
	if itsName == "" {
		itsName = "integration-test-" + utils.GenerateRandomString(4)
	}

	params := []integrationv1beta2.ResolverParameter{
		{
			Name:  "url",
			Value: gitURL,
		},
		{
			Name:  "revision",
			Value: revision,
		},
		{
			Name:  "pathInRepo",
			Value: pathInRepo,
		},
	}

	integrationTestScenario := &integrationv1beta2.IntegrationTestScenario{
		ObjectMeta: metav1.ObjectMeta{
			Name:      itsName,
			Namespace: namespace,
			Labels:    map[string]string{tektonconsts.OptionalLabel: "true"},
		},
		Spec: integrationv1beta2.IntegrationTestScenarioSpec{
			Application: applicationName,
			ResolverRef: integrationv1beta2.ResolverRef{
				Resolver: "git",
				Params:   params,
			},
			Contexts: []integrationv1beta2.TestContext{},
		},
	}

	// Add kind parameter if provided and is "pipelineRun"
	if strings.EqualFold(kind, "pipelineRun") {
		integrationTestScenario.Spec.ResolverRef.ResourceKind = "pipelinerun"

	}

	if len(contexts) > 0 {
		for _, testContext := range contexts {
			integrationTestScenario.Spec.Contexts = append(integrationTestScenario.Spec.Contexts,
				integrationv1beta2.TestContext{Name: testContext, Description: testContext})
		}
	}

	return i.createScenarioWithRetry(integrationTestScenario)
}

// Get return the status from the Application Custom Resource object.
func (i *IntegrationController) GetIntegrationTestScenarios(applicationName, namespace string) (*[]integrationv1beta2.IntegrationTestScenario, error) {
	opts := []client.ListOption{
		client.InNamespace(namespace),
	}

	integrationTestScenarioList := &integrationv1beta2.IntegrationTestScenarioList{}
	err := i.KubeRest().List(context.Background(), integrationTestScenarioList, opts...)
	if err != nil {
		return nil, err
	}

	items := make([]integrationv1beta2.IntegrationTestScenario, 0)
	for _, t := range integrationTestScenarioList.Items {
		if t.Spec.Application == applicationName {
			items = append(items, t)
		}
	}
	return &items, nil
}

// DeleteIntegrationTestScenario removes given testScenario from specified namespace.
func (i *IntegrationController) DeleteIntegrationTestScenario(testScenario *integrationv1beta2.IntegrationTestScenario, namespace string) error {
	err := i.KubeRest().Delete(context.Background(), testScenario)
	return err
}
