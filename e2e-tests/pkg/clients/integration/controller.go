package integration

import (
	"github.com/konflux-ci/integration-service/e2e-tests/pkg/clients/common"
)

type IntegrationController struct {
	*common.CustomClient
}

func NewSuiteController(kube *common.CustomClient) (*IntegrationController, error) {
	return &IntegrationController{
		kube,
	}, nil
}
