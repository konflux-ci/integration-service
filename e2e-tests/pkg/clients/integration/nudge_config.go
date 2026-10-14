package integration

import (
	"context"
	"fmt"

	integrationv1beta2 "github.com/konflux-ci/integration-service/api/v1beta2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CreateNudgeConfig creates a NudgeConfig with the given spec
func (i *IntegrationController) CreateNudgeConfig(
	namespace string,
	nudges []integrationv1beta2.NudgeRelationship,
) (*integrationv1beta2.NudgeConfig, error) {
	nudgeConfig := &integrationv1beta2.NudgeConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      integrationv1beta2.NudgeConfigSingletonName,
			Namespace: namespace,
		},
		Spec: integrationv1beta2.NudgeConfigSpec{
			Nudges: nudges,
		},
	}

	err := i.KubeRest().Create(context.Background(), nudgeConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create NudgeConfig: %w", err)
	}

	return nudgeConfig, nil
}

// DeleteNudgeConfig deletes the NudgeConfig
func (i *IntegrationController) DeleteNudgeConfig(
	nudgeConfig *integrationv1beta2.NudgeConfig,
) error {
	err := i.KubeRest().Delete(context.Background(), nudgeConfig)
	if err != nil {
		return fmt.Errorf("failed to delete NudgeConfig: %w", err)
	}
	return nil
}
