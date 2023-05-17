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

package application

import (
	"context"

	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/api/v1alpha1"
	h "github.com/redhat-appstudio/integration-service/helpers"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/redhat-appstudio/operator-goodies/reconciler"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Adapter holds the objects needed to reconcile an Application.
type Adapter struct {
	application *applicationapiv1alpha1.Application
	scenario    *v1alpha1.IntegrationTestScenario
	logger      h.IntegrationLogger
	client      client.Client
	context     context.Context
}

// NewAdapter creates and returns an Adapter instance.
func NewAdapter(application *applicationapiv1alpha1.Application, scenario *v1alpha1.IntegrationTestScenario, logger h.IntegrationLogger, client client.Client,
	context context.Context) *Adapter {
	return &Adapter{
		application: application,
		scenario:    scenario,
		logger:      logger,
		client:      client,
		context:     context,
	}
}

// EnsureIntegrationTestScenarioExist TODO
func (a *Adapter) EnsureIntegrationTestScenarioExist() (reconciler.OperationResult, error) {
	logger := a.logger

	if a.scenario != nil {
		logger.Info("IntegrationTestScenario already exists for application")
		return reconciler.ContinueProcessing()
	}

	logger.Info("Missing IntegrationTestScenario for application")
	if err := a.createIntegrationTestScenario(); err != nil {
		a.logger.Error(err, "Failed to create IntegrationTestScenario for application")
		return reconciler.RequeueWithError(err)
	}
	a.logger.Info("IntegrationTestScenario %q created successfully for application")

	return reconciler.ContinueProcessing()
}

// createIntegrationTestScenario TODO
func (a *Adapter) createIntegrationTestScenario() error {
	// TODO: Move to v1beta1?
	scenario := &v1alpha1.IntegrationTestScenario{
		ObjectMeta: metav1.ObjectMeta{
			Name:      a.application.Name,
			Namespace: a.application.Namespace,
		},
		Spec: v1alpha1.IntegrationTestScenarioSpec{
			Application: a.application.Name,
			// TODO: Obviously don't hard code this... Use a config somewhere?
			Bundle: "quay.io/redhat-appstudio-tekton-catalog/pipeline-enterprise-contract:devel",
			// TODO: Obviously don't hard code this... Use a config somewhere?
			Pipeline: "enterprise-contract",
			// TODO: Is this needed?
			Contexts: []v1alpha1.TestContext{
				{Name: "application", Description: "Application testing"},
			},
			// TODO: What about an environment?
		},
	}

	if err := a.client.Create(a.context, scenario); err != nil {
		return err
	}

	return nil
}
