/*
Copyright 2022 Red Hat Inc.

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
	"crypto/tls"
	"flag"
	"os"

	"github.com/konflux-ci/integration-service/internal/controller"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
	crwebhook "sigs.k8s.io/controller-runtime/pkg/webhook"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	zap2 "go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	integrationv1alpha1 "github.com/konflux-ci/integration-service/api/v1alpha1"
	integrationv1beta1 "github.com/konflux-ci/integration-service/api/v1beta1"
	integrationv1beta2 "github.com/konflux-ci/integration-service/api/v1beta2"
	pacv1alpha1 "github.com/openshift-pipelines/pipelines-as-code/pkg/apis/pipelinesascode/v1alpha1"
	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	releasev1alpha1 "github.com/redhat-appstudio/release-service/api/v1alpha1"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
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
	utilruntime.Must(integrationv1beta1.AddToScheme(scheme))
	utilruntime.Must(integrationv1beta2.AddToScheme(scheme))
	utilruntime.Must(tektonv1.AddToScheme(scheme))
	utilruntime.Must(releasev1alpha1.AddToScheme(scheme))
	utilruntime.Must(pacv1alpha1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableHttp2 bool
	var enableLeaderElection bool
	var probeAddr string
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableHttp2, "enable-http2", false, "Enable HTTP/2 for the metrics and webhook servers.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	opts := zap.Options{
		Development: false,
		TimeEncoder: zapcore.RFC3339TimeEncoder,
		ZapOpts:     []zap2.Option{zap2.WithCaller(true)},
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "f1944211.redhat.com",
		Metrics: server.Options{
			BindAddress: metricsAddr,
		},
		WebhookServer: crwebhook.NewServer(crwebhook.Options{
			Port: 9443,
			TLSOpts: []func(*tls.Config){
				func(c *tls.Config) {
					if !enableHttp2 {
						c.NextProtos = []string{"http/1.1"}
					}
				},
			},
		}),
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	err = controllers.SetupControllers(mgr)
	if err != nil {
		setupLog.Error(err, "unable to setup controllers")
		os.Exit(1)
	}
	if err = (&integrationv1beta2.IntegrationTestScenario{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "IntegrationTestScenario")
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
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
