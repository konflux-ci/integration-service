/*
Copyright 2023.

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

package statusreport

import (
	"context"
	"os"

	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/helpers"
	"github.com/redhat-appstudio/integration-service/status"

	"github.com/redhat-appstudio/integration-service/loader"
	"github.com/redhat-appstudio/operator-toolkit/controller"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const FeatureFlagStatusReprotingEnabled = "FEATURE_STATUS_REPORTING_ENABLED"

// Adapter holds the objects needed to reconcile a snapshot's test status report.
type Adapter struct {
	snapshot    *applicationapiv1alpha1.Snapshot
	application *applicationapiv1alpha1.Application
	logger      helpers.IntegrationLogger
	loader      loader.ObjectLoader
	client      client.Client
	context     context.Context
	status      status.Status
}

// NewAdapter creates and returns an Adapter instance.
func NewAdapter(snapshot *applicationapiv1alpha1.Snapshot, application *applicationapiv1alpha1.Application, logger helpers.IntegrationLogger, loader loader.ObjectLoader, client client.Client,
	context context.Context) *Adapter {
	return &Adapter{
		snapshot:    snapshot,
		application: application,
		logger:      logger,
		loader:      loader,
		client:      client,
		context:     context,
		status:      status.NewAdapter(logger.Logger, client),
	}
}

// EnsureSnapshotTestStatusReported will ensure that integration test status including env provision and snapshotEnvironmentBinding error is reported to the git provider
// which (indirectly) triggered its execution.
func (a *Adapter) EnsureSnapshotTestStatusReported() (controller.OperationResult, error) {
	if !isFeatureEnabled() {
		return controller.ContinueProcessing()
	}

	reporters, err := a.status.GetReporters(a.snapshot)
	if err != nil {
		return controller.RequeueWithError(err)
	}

	for _, reporter := range reporters {
		if err := reporter.ReportStatusForSnapshot(a.client, a.context, &a.logger, a.snapshot); err != nil {
			a.logger.Error(err, "failed to report test status to github for snapshot",
				"snapshot.Namespace", a.snapshot.Namespace, "snapshot.Name", a.snapshot.Name)
			return controller.RequeueWithError(err)
		}
	}

	return controller.ContinueProcessing()
}

// isFeatureEnabled returns true when the feature flag FEATURE_STATUS_REPORTING_ENABLED has been defined in env vars
func isFeatureEnabled() bool {
	if _, found := os.LookupEnv(FeatureFlagStatusReprotingEnabled); found {
		return true
	}
	return false
}
