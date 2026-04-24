package release

import "github.com/konflux-ci/integration-service/e2e-tests/pkg/clients/common"

// Factory to initialize the comunication against different API like github or kubernetes.
type ReleaseController struct {
	// Generates a kubernetes client to interact with clusters.
	*common.CustomClient
}

// Initializes all the clients and return interface to operate with release controller.
func NewSuiteController(kube *common.CustomClient) (*ReleaseController, error) {
	return &ReleaseController{
		kube,
	}, nil
}
