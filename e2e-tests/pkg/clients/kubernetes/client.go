package client

import (
	appstudioApi "github.com/konflux-ci/application-api/api/v1alpha1"
	imagecontroller "github.com/konflux-ci/image-controller/api/v1alpha1"
	integrationservicev1beta2 "github.com/konflux-ci/integration-service/api/v1beta2"

	release "github.com/konflux-ci/release-service/api/v1alpha1"
	tekton "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	pipelineclientset "github.com/tektoncd/pipeline/pkg/client/clientset/versioned"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

type CustomClient struct {
	kubeClient     *kubernetes.Clientset
	crClient       crclient.Client
	pipelineClient pipelineclientset.Interface
}

var (
	scheme = runtime.NewScheme()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(appstudioApi.AddToScheme(scheme))
	utilruntime.Must(tekton.AddToScheme(scheme))
	utilruntime.Must(release.AddToScheme(scheme))
	utilruntime.Must(integrationservicev1beta2.AddToScheme(scheme))
	utilruntime.Must(imagecontroller.AddToScheme(scheme))
}

// Kube returns the clientset for Kubernetes upstream.
func (c *CustomClient) KubeInterface() kubernetes.Interface {
	return c.kubeClient
}

// Return a rest client to perform CRUD operations on Kubernetes objects
func (c *CustomClient) KubeRest() crclient.Client {
	return c.crClient
}

func (c *CustomClient) PipelineClient() pipelineclientset.Interface {
	return c.pipelineClient
}

// Creates a kubernetes client from default kubeconfig. Will take it from KUBECONFIG env if it is defined and if in case is not defined
// will create the client from $HOME/.kube/config
func NewAdminKubernetesClient() (*CustomClient, error) {
	adminKubeconfig, err := config.GetConfig()
	if err != nil {
		return nil, err
	}
	clientSets, err := createClientSetsFromConfig(adminKubeconfig)
	if err != nil {
		return nil, err
	}

	crClient, err := crclient.New(adminKubeconfig, crclient.Options{
		Scheme: scheme,
	})
	if err != nil {
		return nil, err
	}

	return &CustomClient{
		kubeClient:     clientSets.kubeClient,
		pipelineClient: clientSets.pipelineClient,
		crClient:       crClient,
	}, nil
}

func createClientSetsFromConfig(cfg *rest.Config) (*CustomClient, error) {
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}

	pipelineClient, err := pipelineclientset.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}

	return &CustomClient{
		kubeClient:     client,
		pipelineClient: pipelineClient,
	}, nil
}
