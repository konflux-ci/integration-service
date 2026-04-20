package tekton

import (
	kubeCl "github.com/konflux-ci/integration-service/e2e-tests/pkg/clients/kubernetes"
)

// Create the struct for kubernetes clients
type TektonController struct {
	*kubeCl.CustomClient
}

// Create controller for Tekton Task/Pipeline CRUD operations
func NewSuiteController(kube *kubeCl.CustomClient) *TektonController {
	return &TektonController{
		kube,
	}
}
