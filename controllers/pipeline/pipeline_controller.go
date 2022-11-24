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

package pipeline

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/controllers/results"
	"github.com/redhat-appstudio/integration-service/tekton"
	tektonv1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// Reconciler reconciles a PipelineRun object
type Reconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// NewIntegrationReconciler creates and returns a Reconciler.
func NewIntegrationReconciler(client client.Client, logger *logr.Logger, scheme *runtime.Scheme) *Reconciler {
	return &Reconciler{
		Client: client,
		Log:    logger.WithName("pipeline"),
		Scheme: scheme,
	}
}

//+kubebuilder:rbac:groups=tekton.dev,resources=pipelineruns,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=tekton.dev,resources=pipelineruns/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=tekton.dev,resources=pipelineruns/finalizers,verbs=update
//+kubebuilder:rbac:groups=appstudio.redhat.com,resources=applications/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.Log.WithValues("Integration", req.NamespacedName)

	pipelineRun := &tektonv1beta1.PipelineRun{}
	err := r.Get(ctx, req.NamespacedName, pipelineRun)
	if err != nil {
		logger.Error(err, "Failed to get pipelineRun for", "req", req.NamespacedName)
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, err
	}

	pipelineType, err := tekton.GetTypeFromPipelineRun(pipelineRun)
	if err != nil {
		logger.Error(err, "Failed to get Pipeline Type for",
			"PipelineRun.Name", pipelineRun.Name, "PipelineRun.Namespace", pipelineRun.Namespace)
		return ctrl.Result{}, err
	}
	component, err := r.getComponentFromPipelineRun(ctx, pipelineRun, pipelineType)
	if err != nil {
		logger.Error(err, "Failed to get Component for",
			"PipelineRun.Name", pipelineRun.Name, "PipelineRun.Namespace", pipelineRun.Namespace)
		return ctrl.Result{}, err
	}

	application := &applicationapiv1alpha1.Application{}
	if component != nil {
		application, err = r.getApplicationFromComponent(ctx, component)
		if err != nil {
			logger.Error(err, "Failed to get Application for",
				"Component.Name ", component.Name, "Component.Namespace ", component.Namespace)
			return ctrl.Result{}, err
		}
	} else if pipelineType == tekton.PipelineRunTestType {
		application, err = r.getApplicationFromPipelineRun(ctx, pipelineRun, pipelineType)
		if err != nil {
			logger.Error(err, "Failed to get Application for",
				"PipelineRun.Name", pipelineRun.Name, "PipelineRun.Namespace", pipelineRun.Namespace)
			return ctrl.Result{}, err
		}
	}

	adapter := NewAdapter(pipelineRun, component, application, logger, r.Client, ctx)

	return r.ReconcileHandler(adapter)
}

// getComponentFromPipelineRun loads from the cluster the Component referenced in the given PipelineRun. If the PipelineRun doesn't
// specify a Component or this is not found in the cluster, an error will be returned.
func (r *Reconciler) getComponentFromPipelineRun(context context.Context, pipelineRun *tektonv1beta1.PipelineRun, pipelineType string) (*applicationapiv1alpha1.Component, error) {
	componentLabel := fmt.Sprintf("%s.%s", pipelineType, tekton.PipelineRunComponentLabel)
	if componentName, found := pipelineRun.Labels[componentLabel]; found {
		component := &applicationapiv1alpha1.Component{}
		err := r.Get(context, types.NamespacedName{
			Namespace: pipelineRun.Namespace,
			Name:      componentName,
		}, component)

		if err != nil {
			return nil, err
		}

		return component, nil
	}

	return nil, nil
}

// getApplicationFromPipelineRun loads from the cluster the Component referenced in the given PipelineRun. If the PipelineRun doesn't
// specify a Component or this is not found in the cluster, an error will be returned.
func (r *Reconciler) getApplicationFromPipelineRun(context context.Context, pipelineRun *tektonv1beta1.PipelineRun, pipelineType string) (*applicationapiv1alpha1.Application, error) {
	applicationLabel := fmt.Sprintf("%s.%s", pipelineType, tekton.PipelineRunApplicationLabel)
	if componentName, found := pipelineRun.Labels[applicationLabel]; found {
		application := &applicationapiv1alpha1.Application{}
		err := r.Get(context, types.NamespacedName{
			Namespace: pipelineRun.Namespace,
			Name:      componentName,
		}, application)

		if err != nil {
			return nil, err
		}

		return application, nil
	}

	return nil, nil
}

// getApplicationFromComponent loads from the cluster the Application referenced in the given Component. If the Component doesn't
// specify an Application or this is not found in the cluster, an error will be returned.
func (r *Reconciler) getApplicationFromComponent(context context.Context, component *applicationapiv1alpha1.Component) (*applicationapiv1alpha1.Application, error) {
	application := &applicationapiv1alpha1.Application{}
	err := r.Get(context, types.NamespacedName{
		Namespace: component.Namespace,
		Name:      component.Spec.Application,
	}, application)

	if err != nil {
		return nil, err
	}

	return application, nil
}

// AdapterInterface is an interface defining all the operations that should be defined in an Integration adapter.
type AdapterInterface interface {
	EnsureSnapshotExists() (results.OperationResult, error)
	EnsureSnapshotPassedAllTests() (results.OperationResult, error)
	EnsureStatusReported() (results.OperationResult, error)
}

// ReconcileOperation defines the syntax of functions invoked by the ReconcileHandler
type ReconcileOperation func() (results.OperationResult, error)

// ReconcileHandler will invoke all the operations to be performed as part of an Integration reconcile, managing
// the queue based on the operations' results.
func (r *Reconciler) ReconcileHandler(adapter AdapterInterface) (ctrl.Result, error) {
	operations := []ReconcileOperation{
		adapter.EnsureSnapshotExists,
		adapter.EnsureSnapshotPassedAllTests,
		adapter.EnsureStatusReported,
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
	return setupControllerWithManager(manager, NewIntegrationReconciler(manager.GetClient(), log, manager.GetScheme()))
}

// setupCache indexes fields for each of the resources used in the release adapter in those cases where filtering by
// field is required.
func setupCache(mgr ctrl.Manager) error {
	if err := SetupApplicationComponentCache(mgr); err != nil {
		return err
	}

	if err := SetupSnapshotCache(mgr); err != nil {
		return err
	}

	return SetupIntegrationTestScenarioCache(mgr)
}

// setupControllerWithManager sets up the controller with the Manager which monitors new PipelineRuns and filters
// out status updates.
func setupControllerWithManager(manager ctrl.Manager, reconciler *Reconciler) error {
	err := setupCache(manager)
	if err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(manager).
		For(&tektonv1beta1.PipelineRun{}).
		WithEventFilter(predicate.Or(
			tekton.IntegrationPipelineRunStartedPredicate(),
			tekton.IntegrationOrBuildPipelineRunSucceededPredicate())).
		Complete(reconciler)
}
