/*
Copyright 2022.

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

package snapshot

import (
	"github.com/go-logr/logr"
	hasv1alpha1 "github.com/redhat-appstudio/application-service/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/controllers/results"
	appstudioshared "github.com/redhat-appstudio/managed-gitops/appstudio-shared/apis/appstudio.redhat.com/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

import (
	"context"
)

// Reconciler reconciles an ApplicationSnapshot object
type Reconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// NewSnapshotReconciler creates and returns a Reconciler.
func NewSnapshotReconciler(client client.Client, logger *logr.Logger, scheme *runtime.Scheme) *Reconciler {
	return &Reconciler{
		Client: client,
		Log:    logger.WithName("snapshot"),
		Scheme: scheme,
	}
}

//+kubebuilder:rbac:groups=appstudio.redhat.com,resources=applicationSnapshots,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=appstudio.redhat.com,resources=applicationSnapshots/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=appstudio.redhat.com,resources=applicationSnapshots/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.Log.WithValues("Integration", req.NamespacedName)

	snapshot := &appstudioshared.ApplicationSnapshot{}
	err := r.Get(ctx, req.NamespacedName, snapshot)
	if err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, err
	}

	application, err := r.getApplicationFromSnapshot(ctx, snapshot)
	if err != nil {
		logger.Error(err, "Failed to get Application for ",
			"ApplicationSnapshot.Name ", snapshot.Name, "ApplicationSnapshot.Namespace ", snapshot.Namespace)
		return ctrl.Result{}, err
	}

	component, err := r.getComponentFromSnapshot(ctx, snapshot)
	if err != nil {
		logger.Error(err, "Failed to get Application for ",
			"Component.Name ", snapshot.Name, "Component.Namespace ", snapshot.Namespace)
		return ctrl.Result{}, err
	}

	adapter := NewAdapter(snapshot, application, component, logger, r.Client, ctx)

	return r.ReconcileHandler(adapter)
}

// getApplicationFromSnapshot loads from the cluster the Application referenced in the given ApplicationSnapshot.
// If the ApplicationSnapshot doesn't specify an Component or this is not found in the cluster, an error will be returned.
func (r *Reconciler) getApplicationFromSnapshot(context context.Context, snapshot *appstudioshared.ApplicationSnapshot) (*hasv1alpha1.Application, error) {
	application := &hasv1alpha1.Application{}
	err := r.Get(context, types.NamespacedName{
		Namespace: snapshot.Namespace,
		Name:      snapshot.Spec.Application,
	}, application)

	if err != nil {
		return nil, err
	}

	return application, nil
}

// getComponentFromSnapshot loads from the cluster the Component referenced in the given ApplicationSnapshot.
// If the ApplicationSnapshot doesn't specify an Application or this is not found in the cluster, an error will be returned.
func (r *Reconciler) getComponentFromSnapshot(context context.Context, snapshot *appstudioshared.ApplicationSnapshot) (*hasv1alpha1.Component, error) {
	if componentLabel, ok := snapshot.Labels["component"]; ok {
		component := &hasv1alpha1.Component{}
		err := r.Get(context, types.NamespacedName{
			Namespace: snapshot.Namespace,
			Name:      componentLabel,
		}, component)

		if err != nil {
			return nil, err
		}

		return component, nil
	} else {
		return nil, nil
	}
}

// AdapterInterface is an interface defining all the operations that should be defined in an Integration adapter.
type AdapterInterface interface {
	EnsureAllReleasesExist() (results.OperationResult, error)
	EnsureApplicationSnapshotEnvironmentBindingExist() (results.OperationResult, error)
	EnsureAllIntegrationTestPipelinesExist() (results.OperationResult, error)
}

// ReconcileOperation defines the syntax of functions invoked by the ReconcileHandler
type ReconcileOperation func() (results.OperationResult, error)

// ReconcileHandler will invoke all the operations to be performed as part of an Integration reconcile, managing
// the queue based on the operations' results.
func (r *Reconciler) ReconcileHandler(adapter AdapterInterface) (ctrl.Result, error) {
	operations := []ReconcileOperation{
		adapter.EnsureAllReleasesExist,
		adapter.EnsureApplicationSnapshotEnvironmentBindingExist,
		adapter.EnsureAllIntegrationTestPipelinesExist,
	}

	for _, operation := range operations {
		result, err := operation()
		if err != nil || result.RequeueRequest {
			return ctrl.Result{RequeueAfter: result.RequeueDelay}, err
		}
		if result.CancelRequest {
			return ctrl.Result{}, nil
		}
	}

	return ctrl.Result{}, nil
}

// SetupController creates a new Integration reconciler and adds it to the Manager.
func SetupController(manager ctrl.Manager, log *logr.Logger) error {
	return setupControllerWithManager(manager, NewSnapshotReconciler(manager.GetClient(), log, manager.GetScheme()))
}

// setupCache indexes fields for each of the resources used in the release adapter in those cases where filtering by
// field is required.
func setupCache(mgr ctrl.Manager) error {
	if err := SetupReleasePlanCache(mgr); err != nil {
		return err
	}

	if err := SetupReleaseCache(mgr); err != nil {
		return err
	}

	if err := SetupApplicationCache(mgr); err != nil {
		return err
	}

	return SetupEnvironmentCache(mgr)
}

// setupControllerWithManager sets up the controller with the Manager which monitors new ApplicationSnapshots
func setupControllerWithManager(manager ctrl.Manager, reconciler *Reconciler) error {
	err := setupCache(manager)
	if err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(manager).
		For(&appstudioshared.ApplicationSnapshot{}).
		Complete(reconciler)
}
