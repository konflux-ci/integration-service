/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions andF
limitations under the License.
*/

package application

import (
	"context"

	"github.com/go-logr/logr"
	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/helpers"
	"github.com/redhat-appstudio/operator-goodies/reconciler"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Reconciler reconciles an Application object
type Reconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// NewApplicationReconciler creates and returns a Reconciler.
func NewApplicationReconciler(client client.Client, logger *logr.Logger, scheme *runtime.Scheme) *Reconciler {
	return &Reconciler{
		Client: client,
		Log:    logger.WithName("application"),
		Scheme: scheme,
	}
}

//+kubebuilder:rbac:groups=appstudio.redhat.com,resources=applications,verbs=get;list;watch
//+kubebuilder:rbac:groups=appstudio.redhat.com,resources=integrationtestscenarios,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := helpers.IntegrationLogger{Logger: r.Log.WithValues("application", req.NamespacedName)}

	application := &applicationapiv1alpha1.Application{}
	if err := r.Get(ctx, req.NamespacedName, application); err != nil {
		// TODO: Add logic to ignore if it has been deleted
		if errors.IsNotFound(err) {
			// Application no longer exists, do nothing.
			// TODO: Delete the IntegrationTestScenario?
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get application for", "req", req.NamespacedName)
		return ctrl.Result{}, err
	}

	logger = logger.WithApp(*application)

	scenario := &v1alpha1.IntegrationTestScenario{}
	if err := r.Get(ctx, req.NamespacedName, scenario); err != nil {
		if !errors.IsNotFound(err) {
			logger.Error(err, "Failed to get IntegrationTestScenario from request", "req", req.NamespacedName)
			return ctrl.Result{}, err
		}
		scenario = nil
	}

	adapter := NewAdapter(application, scenario, logger, r.Client, ctx)

	return reconciler.ReconcileHandler([]reconciler.ReconcileOperation{
		adapter.EnsureIntegrationTestScenarioExist,
	})
}

// TODO: what is this used for?
// // AdapterInterface is an interface defining all the operations that should be defined in an Integration adapter.
// type AdapterInterface interface {
// 	EnsureIntegrationTestScenarioExist() (reconciler.OperationResult, error)
// }

// SetupController creates a new Integration reconciler and adds it to the Manager.
func SetupController(manager ctrl.Manager, log *logr.Logger) error {
	return setupControllerWithManager(manager, NewApplicationReconciler(manager.GetClient(), log, manager.GetScheme()))
}

// setupControllerWithManager sets up the controller with the Manager which monitors new Applications
func setupControllerWithManager(manager ctrl.Manager, reconciler *Reconciler) error {
	// TODO: setupCache?
	return ctrl.NewControllerManagedBy(manager).
		For(&applicationapiv1alpha1.Application{}).
		// TODO: What sort of filter should be applied here?
		// WithEventFilter(predicate.Or(
		// 	gitops.IntegrationSnapshotChangePredicate())).
		Complete(reconciler)
}
