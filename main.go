/*
Copyright 2022.

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

package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/redhat-appstudio/integration-service/controllers"
	"os"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"github.com/go-logr/logr"
	kcpv1alpha1 "github.com/kcp-dev/kcp/pkg/apis/apis/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/discovery"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/kcp"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	integrationv1alpha1 "github.com/redhat-appstudio/integration-service/api/v1alpha1"
	releasev1alpha1 "github.com/redhat-appstudio/release-service/api/v1alpha1"
	tektonv1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(applicationapiv1alpha1.AddToScheme(scheme))
	utilruntime.Must(integrationv1alpha1.AddToScheme(scheme))
	utilruntime.Must(tektonv1beta1.AddToScheme(scheme))
	utilruntime.Must(releasev1alpha1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	var apiExportName string
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var mgr ctrl.Manager
	var err error
	flag.StringVar(&apiExportName, "api-export-name", "", "The name of the APIExport.")
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	ctx := ctrl.SetupSignalHandler()
	restConfig := ctrl.GetConfigOrDie()
	setupLog = setupLog.WithValues("api-export-name", apiExportName)

	options := ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		Port:                   9443,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "f1944211.redhat.com",
		LeaderElectionConfig:   restConfig,
	}
	if IsKcpAPIsGroupPresent(restConfig, setupLog) {
		setupLog.Info("Looking up virtual workspace URL")
		cfg, err := restConfigForAPIExport(ctx, restConfig, apiExportName, setupLog)

		if err != nil {
			setupLog.Error(err, "error looking up virtual workspace URL")
		}

		setupLog.Info("Using virtual workspace URL", "url", cfg.Host)

		options.LeaderElectionConfig = restConfig
		mgr, err = kcp.NewClusterAwareManager(cfg, options)
		if err != nil {
			setupLog.Error(err, "unable to start cluster aware manager")
			os.Exit(1)
		}
	} else {
		setupLog.Info("The apis.kcp.dev group is not present - creating standard manager")
		mgr, err = ctrl.NewManager(restConfig, options)
		if err != nil {
			setupLog.Error(err, "unable to start manager")
			os.Exit(1)
		}
	}

	err = controllers.SetupControllers(mgr)
	if err != nil {
		setupLog.Error(err, "unable to setup controllers")
		os.Exit(1)
	}
	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func IsKcpAPIsGroupPresent(restConfig *rest.Config, setupLog logr.Logger) bool {
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(restConfig)
	if err != nil {
		setupLog.Error(err, "failed to create discovery client")
		os.Exit(1)
	}
	apiGroupList, err := discoveryClient.ServerGroups()
	if err != nil {
		setupLog.Error(err, "failed to get server groups")
		os.Exit(1)
	}

	for _, group := range apiGroupList.Groups {
		if group.Name == kcpv1alpha1.SchemeGroupVersion.Group {
			for _, version := range group.Versions {
				if version.Version == kcpv1alpha1.SchemeGroupVersion.Version {
					return true
				}
			}
		}
	}
	return false
}

// restConfigForAPIExport returns a *rest.Config properly configured to communicate with the endpoint for the
// APIExport's virtual workspace.
func restConfigForAPIExport(ctx context.Context, cfg *rest.Config, apiExportName string, setupLog logr.Logger) (*rest.Config, error) {
	scheme := runtime.NewScheme()
	if err := kcpv1alpha1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("error adding apis.kcp.dev/v1alpha1 to scheme: %w", err)
	}

	apiExportClient, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return nil, fmt.Errorf("error creating APIExport client: %w", err)
	}

	var apiExport kcpv1alpha1.APIExport

	if apiExportName != "" {
		if err := apiExportClient.Get(ctx, types.NamespacedName{Name: apiExportName}, &apiExport); err != nil {
			return nil, fmt.Errorf("error getting APIExport %q: %w", apiExportName, err)
		}
	} else {
		setupLog.Info("api-export-name is empty - listing")
		exports := &kcpv1alpha1.APIExportList{}
		if err := apiExportClient.List(ctx, exports); err != nil {
			return nil, fmt.Errorf("error listing APIExports: %w", err)
		}
		if len(exports.Items) == 0 {
			return nil, fmt.Errorf("no APIExport found")
		}
		if len(exports.Items) > 1 {
			return nil, fmt.Errorf("more than one APIExport found")
		}
		apiExport = exports.Items[0]
	}

	if len(apiExport.Status.VirtualWorkspaces) < 1 {
		return nil, fmt.Errorf("APIExport %q status.virtualWorkspaces is empty", apiExportName)
	}

	cfg = rest.CopyConfig(cfg)
	// TODO(ncdc): sharding support
	cfg.Host = apiExport.Status.VirtualWorkspaces[0].URL

	return cfg, nil
}
