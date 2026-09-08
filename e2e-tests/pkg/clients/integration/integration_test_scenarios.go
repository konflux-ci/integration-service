package integration

import (
	"context"
	"strings"

	integrationv1beta2 "github.com/konflux-ci/integration-service/api/v1beta2"
	"github.com/konflux-ci/integration-service/e2e-tests/pkg/constants"
	"github.com/konflux-ci/integration-service/e2e-tests/pkg/utils"
	tektonconsts "github.com/konflux-ci/integration-service/tekton/consts"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CreateIntegrationTestScenario creates beta1 version integrationTestScenario.
// TODO: when we remove application-specific code, remove usingComponentGroups param and change parentName to componentGroupName
func (i *IntegrationController) CreateIntegrationTestScenario(itsName, parentName, namespace, gitURL, revision, pathInRepo, kind string, contexts []string, usingComponentGroups bool) (*integrationv1beta2.IntegrationTestScenario, error) {
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
			ResolverRef: integrationv1beta2.ResolverRef{
				Resolver: "git",
				Params:   params,
			},
			Contexts: []integrationv1beta2.TestContext{},
		},
	}
	if usingComponentGroups {
		integrationTestScenario.Spec.ComponentGroup = parentName
	} else {
		integrationTestScenario.Spec.Application = parentName
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

	err := i.KubeRest().Create(context.Background(), integrationTestScenario)
	if err != nil {
		return nil, err
	}
	return integrationTestScenario, nil
}

// CreateOptionalIntegrationTestScenario creates a beta1 version integrationTestScenario with optional: true label.
// This function is identical to CreateIntegrationTestScenario except it sets the optional label to "true".
// TODO: when we remove application-specific code, remove usingComponentGroups param and change parentName to componentGroupName
func (i *IntegrationController) CreateOptionalIntegrationTestScenario(itsName, parentName, namespace, gitURL, revision, pathInRepo, kind string, contexts []string, usingComponentGroups bool) (*integrationv1beta2.IntegrationTestScenario, error) {
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
			ResolverRef: integrationv1beta2.ResolverRef{
				Resolver: "git",
				Params:   params,
			},
			Contexts: []integrationv1beta2.TestContext{},
		},
	}
	if usingComponentGroups {
		integrationTestScenario.Spec.ComponentGroup = parentName
	} else {
		integrationTestScenario.Spec.Application = parentName
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

	err := i.KubeRest().Create(context.Background(), integrationTestScenario)
	if err != nil {
		return nil, err
	}
	return integrationTestScenario, nil
}

// Get return the status from the Application Custom Resource object.
// TODO: when we remove application-specific code, remove usingComponentGroups param and change parentName to componentGroupName
func (i *IntegrationController) GetIntegrationTestScenarios(parentName, namespace string, usingComponentGroups bool) (*[]integrationv1beta2.IntegrationTestScenario, error) {
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
		if usingComponentGroups {
			if t.Spec.ComponentGroup == parentName {
				items = append(items, t)
			}
		} else {
			if t.Spec.Application == parentName {
				items = append(items, t)
			}
		}
	}
	return &items, nil
}

// DeleteIntegrationTestScenario removes given testScenario from specified namespace.
func (i *IntegrationController) DeleteIntegrationTestScenario(testScenario *integrationv1beta2.IntegrationTestScenario, namespace string) error {
	err := i.KubeRest().Delete(context.Background(), testScenario)
	return err
}
