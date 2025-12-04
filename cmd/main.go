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
	"time"

	controllers "github.com/konflux-ci/integration-service/internal/controller"
	iswebhook "github.com/konflux-ci/integration-service/internal/webhook/v1beta2"
	imetrics "github.com/konflux-ci/integration-service/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
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

	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	integrationv1alpha1 "github.com/konflux-ci/integration-service/api/v1alpha1"
	integrationv1beta1 "github.com/konflux-ci/integration-service/api/v1beta1"
	integrationv1beta2 "github.com/konflux-ci/integration-service/api/v1beta2"
	releasev1alpha1 "github.com/konflux-ci/release-service/api/v1alpha1"
	pacv1alpha1 "github.com/openshift-pipelines/pipelines-as-code/pkg/apis/pipelinesascode/v1alpha1"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	resolutionv1beta1 "github.com/tektoncd/pipeline/pkg/apis/resolution/v1beta1"
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
	utilruntime.Must(resolutionv1beta1.AddToScheme(scheme))
	utilruntime.Must(releasev1alpha1.AddToScheme(scheme))
	utilruntime.Must(pacv1alpha1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	var (
		metricsAddr              string
		enableHTTP2              bool
		enableLeaderElection     bool
		probeAddr                string
		leaderRenewDeadline      time.Duration
		leaseDuration            time.Duration
		leaderElectorRetryPeriod time.Duration
		secureMetrics            bool
		tlsOpts                  []func(*tls.Config)
	)

	flag.BoolVar(&enableHTTP2, "enable-http2", false,
		"If set, HTTP/2 will be enabled for the metrics and webhook servers")
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.DurationVar(&leaderRenewDeadline, "leader-renew-deadline", 10*time.Second,
		"Leader RenewDeadline is the duration that the acting controlplane "+
			"will retry refreshing leadership before giving up.")
	flag.DurationVar(&leaseDuration, "lease-duration", 15*time.Second,
		"Lease Duration is the duration that non-leader candidates will wait to force acquire leadership.")
	flag.DurationVar(&leaderElectorRetryPeriod, "leader-elector-retry-period", 2*time.Second, "RetryPeriod is the duration the "+
		"LeaderElector clients should wait between tries of actions.")

	opts := zap.Options{
		Development: false,
		TimeEncoder: zapcore.RFC3339TimeEncoder,
		ZapOpts:     []zap2.Option{zap2.WithCaller(true)},
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// if the enable-http2 flag is false (the default), http/2 should be disabled
	// due to its vulnerabilities. More specifically, disabling http/2 will
	// prevent from being vulnerable to the HTTP/2 Stream Cancellation and
	// Rapid Reset CVEs. For more information see:
	// - https://github.com/advisories/GHSA-qppj-fm5r-hxr3
	// - https://github.com/advisories/GHSA-4374-p667-p6c8
	disableHTTP2 := func(c *tls.Config) {
		setupLog.Info("disabling http/2")
		c.NextProtos = []string{"http/1.1"}
	}

	if !enableHTTP2 {
		tlsOpts = append(tlsOpts, disableHTTP2)
	}

	webhookServer := crwebhook.NewServer(crwebhook.Options{
		TLSOpts: tlsOpts,
	})

	// Metrics endpoint is enabled in 'config/default/kustomization.yaml'. The Metrics options configure the server.
	// More info:
	// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.0/pkg/metrics/server
	// - https://book.kubebuilder.io/reference/metrics.html
	metricsServerOptions := server.Options{
		BindAddress:   metricsAddr,
		SecureServing: secureMetrics,
		// TODO(user): TLSOpts is used to allow configuring the TLS config used for the server. If certificates are
		// not provided, self-signed certificates will be generated by default. This option is not recommended for
		// production environments as self-signed certificates do not offer the same level of trust and security
		// as certificates issued by a trusted Certificate Authority (CA). The primary risk is potentially allowing
		// unauthorized access to sensitive metrics data. Consider replacing with CertDir, CertName, and KeyName
		// to provide certificates, ensuring the server communicates using trusted and secure certificates.
		TLSOpts: tlsOpts,
	}

	if secureMetrics {
		// FilterProvider is used to protect the metrics endpoint with authn/authz.
		// These configurations ensure that only authorized users and service accounts
		// can access the metrics endpoint. The RBAC are configured in 'config/rbac/kustomization.yaml'. More info:
		// https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.20.0/pkg/metrics/filters#WithAuthenticationAndAuthorization
		metricsServerOptions.FilterProvider = filters.WithAuthenticationAndAuthorization
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		Metrics:                metricsServerOptions,
		WebhookServer:          webhookServer,
		RenewDeadline:          &leaderRenewDeadline,
		LeaseDuration:          &leaseDuration,
		RetryPeriod:            &leaderElectorRetryPeriod,
		LeaderElectionID:       "03c7e15b.redhat.com",
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
	if err = iswebhook.SetupIntegrationTestScenarioWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "IntegrationTestScenario")
		os.Exit(1)
	}
	if err = iswebhook.SetupSnapshotWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "Snapshot")
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

	ctx := ctrl.SetupSignalHandler()
	integrationMetrics := imetrics.NewIntegrationMetrics([]imetrics.AvailabilityProbe{imetrics.NewGithubAppAvailabilityProbe(mgr.GetClient())})
	if err := integrationMetrics.InitMetrics(metrics.Registry); err != nil {
		setupLog.Error(err, "unable to initialize metrics")
		os.Exit(1)
	}
	integrationMetrics.StartAvailabilityProbes(ctx)

	setupLog.Info("starting manager")
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
