package tekton

import (
	"github.com/konflux-ci/integration-service/e2e-tests/pkg/clients/common"
)

// Create the struct for kubernetes clients
type TektonController struct {
	*common.CustomClient
}

// Create controller for Tekton Task/Pipeline CRUD operations
func NewSuiteController(kube *common.CustomClient) *TektonController {
	return &TektonController{
		kube,
	}
}
