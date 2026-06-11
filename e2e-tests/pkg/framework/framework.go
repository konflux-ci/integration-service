package framework

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	"github.com/konflux-ci/integration-service/e2e-tests/pkg/clients/common"
	"github.com/konflux-ci/integration-service/e2e-tests/pkg/clients/has"
	"github.com/konflux-ci/integration-service/e2e-tests/pkg/clients/integration"
	"github.com/konflux-ci/integration-service/e2e-tests/pkg/clients/release"
	"github.com/konflux-ci/integration-service/e2e-tests/pkg/clients/tekton"
	"github.com/konflux-ci/integration-service/e2e-tests/pkg/constants"
)

type ControllerHub struct {
	HasController         *has.HasController
	CommonController      *common.SuiteController
	TektonController      *tekton.TektonController
	ReleaseController     *release.ReleaseController
	IntegrationController *integration.IntegrationController
}

type Framework struct {
	AsKubeAdmin          *ControllerHub
	AsKubeDeveloper      *ControllerHub
	ClusterAppDomain     string
	OpenshiftConsoleHost string
	UserNamespace        string
	UserName             string
}

func NewFramework(userName string) (*Framework, error) {
	return NewFrameworkWithTimeout(userName, time.Second*60)
}

func NewFrameworkWithTimeout(userName string, timeout time.Duration) (*Framework, error) {
	var clusterAppDomain, openshiftConsoleHost string

	if userName == "" {
		return nil, fmt.Errorf("userName cannot be empty when initializing a new framework instance")
	}

	client, err := common.NewAdminKubernetesClient()
	if err != nil {
		return nil, err
	}

	asAdmin, err := InitControllerHub(client)
	if err != nil {
		return nil, fmt.Errorf("error when initializing appstudio hub controllers for admin user: %v", err)
	}

	nsName := os.Getenv(constants.E2E_APPLICATIONS_NAMESPACE_ENV)
	if nsName == "" {
		nsName = userName

		_, err := asAdmin.CommonController.CreateTestNamespace(userName)
		if err != nil {
			return nil, fmt.Errorf("failed to create test namespace %s: %+v", nsName, err)
		}
	}

	if os.Getenv(constants.TEST_ENVIRONMENT_ENV) == constants.UpstreamTestEnvironment {
		kubeconfig, err := config.GetConfig()
		if err != nil {
			return nil, fmt.Errorf("error when getting kubeconfig: %+v", err)
		}

		parsedURL, err := url.Parse(kubeconfig.Host)
		if err != nil {
			return nil, fmt.Errorf("failed to parse kubeconfig host URL: %+v", err)
		}
		clusterAppDomain = parsedURL.Hostname()
		openshiftConsoleHost = clusterAppDomain
	} else {
		if os.Getenv(constants.E2E_APPLICATIONS_NAMESPACE_ENV) == "" {
			route := &unstructured.Unstructured{}
			route.SetGroupVersionKind(schema.GroupVersionKind{Group: "route.openshift.io", Version: "v1", Kind: "Route"})
			err := asAdmin.CommonController.CustomClient.KubeRest().Get(context.Background(), crclient.ObjectKey{Namespace: "openshift-console", Name: "console"}, route)
			if err != nil {
				return nil, fmt.Errorf("cannot get openshift console route in order to determine cluster app domain: %+v", err)
			}
			host, _, _ := unstructured.NestedString(route.Object, "spec", "host")
			openshiftConsoleHost = host
			clusterAppDomain = strings.Join(strings.Split(openshiftConsoleHost, ".")[1:], ".")
		}
	}

	return &Framework{
		AsKubeAdmin:          asAdmin,
		AsKubeDeveloper:      asAdmin,
		ClusterAppDomain:     clusterAppDomain,
		OpenshiftConsoleHost: openshiftConsoleHost,
		UserNamespace:        nsName,
		UserName:             userName,
	}, nil
}

func InitControllerHub(cc *common.CustomClient) (*ControllerHub, error) {
	// Initialize Common controller
	commonCtrl, err := common.NewSuiteController(cc)
	if err != nil {
		return nil, err
	}

	// Initialize Has controller
	hasController, err := has.NewSuiteController(cc)
	if err != nil {
		return nil, err
	}

	// Initialize Tekton controller
	tektonController := tekton.NewSuiteController(cc)

	// Initialize Release Controller
	releaseController, err := release.NewSuiteController(cc)
	if err != nil {
		return nil, err
	}

	// Initialize Integration Controller
	integrationController, err := integration.NewSuiteController(cc)
	if err != nil {
		return nil, err
	}

	return &ControllerHub{
		HasController:         hasController,
		CommonController:      commonCtrl,
		TektonController:      tektonController,
		ReleaseController:     releaseController,
		IntegrationController: integrationController,
	}, nil
}
