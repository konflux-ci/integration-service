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
	hasv1alpha1 "github.com/redhat-appstudio/application-service/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/controllers/results"
	"github.com/redhat-appstudio/integration-service/tekton"
	appstudioshared "github.com/redhat-appstudio/managed-gitops/appstudio-shared/apis/appstudio.redhat.com/v1alpha1"
	tektonv1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

//+kubebuilder:rbac:groups=appstudio.redhat.com,resources=pipelineRuns,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=appstudio.redhat.com,resources=pipelineRuns/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=appstudio.redhat.com,resources=pipelineRuns/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.Log.WithValues("Integration", req.NamespacedName)

	pipelineRun := &tektonv1beta1.PipelineRun{}
	err := r.Get(ctx, req.NamespacedName, pipelineRun)
	if err != nil {
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

	application, err := r.getApplicationFromComponent(ctx, component)
	if err != nil {
		logger.Error(err, "Failed to get Application for ",
			"Component.Name ", component.Name, "Component.Namespace ", component.Namespace)
		return ctrl.Result{}, err
	}

	adapter := NewAdapter(pipelineRun, component, application, logger, r.Client, ctx)

	return r.ReconcileHandler(adapter)
}

// getComponentFromPipelineRun loads from the cluster the Component referenced in the given PipelineRun. If the PipelineRun doesn't
// specify a Component or this is not found in the cluster, an error will be returned.
func (r *Reconciler) getComponentFromPipelineRun(context context.Context, pipelineRun *tektonv1beta1.PipelineRun, pipelineType string) (*hasv1alpha1.Component, error) {
	componentLabel := fmt.Sprintf("%s.appstudio.openshift.io/component", pipelineType)
	if componentName, found := pipelineRun.Labels[componentLabel]; found {
		component := &hasv1alpha1.Component{}
		err := r.Get(context, types.NamespacedName{
			Namespace: pipelineRun.Namespace,
			Name:      componentName,
		}, component)

		if err != nil {
			return nil, err
		}

		return component, nil
	}

	return nil, fmt.Errorf("the pipeline has no component associated with it")
}

// getApplicationFromComponent loads from the cluster the Application referenced in the given Component. If the Component doesn't
// specify an Application or this is not found in the cluster, an error will be returned.
func (r *Reconciler) getApplicationFromComponent(context context.Context, component *hasv1alpha1.Component) (*hasv1alpha1.Application, error) {
	application := &hasv1alpha1.Application{}
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
	EnsureApplicationSnapshotExists() (results.OperationResult, error)
	EnsureApplicationSnapshotPassedAllTests() (results.OperationResult, error)
}

// ReconcileOperation defines the syntax of functions invoked by the ReconcileHandler
type ReconcileOperation func() (results.OperationResult, error)

// ReconcileHandler will invoke all the operations to be performed as part of an Integration reconcile, managing
// the queue based on the operations' results.
func (r *Reconciler) ReconcileHandler(adapter AdapterInterface) (ctrl.Result, error) {
	operations := []ReconcileOperation{
		adapter.EnsureApplicationSnapshotExists,
		adapter.EnsureApplicationSnapshotPassedAllTests,
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

// setupApplicationComponentCache adds a new index field to be able to search Components by application.
func setupApplicationComponentCache(mgr ctrl.Manager) error {
	applicationComponentIndexFunc := func(obj client.Object) []string {
		return []string{obj.(*hasv1alpha1.Component).Spec.Application}
	}

	return mgr.GetCache().IndexField(context.Background(), &hasv1alpha1.Component{},
		"spec.application", applicationComponentIndexFunc)
}

// setupApplicationSnapshotCache adds a new index field to be able to search ApplicationSnapshots by Application.
func setupApplicationSnapshotCache(mgr ctrl.Manager) error {
	applicationSnapshotIndexFunc := func(obj client.Object) []string {
		return []string{obj.(*appstudioshared.ApplicationSnapshot).Spec.Application}
	}

	return mgr.GetCache().IndexField(context.Background(), &appstudioshared.ApplicationSnapshot{},
		"spec.application", applicationSnapshotIndexFunc)
}

// setupIntegrationTestScenarioCache adds a new index field to be able to search IntegrationTestScenarios by Application.
func setupIntegrationTestScenarioCache(mgr ctrl.Manager) error {
	integrationTestScenariosIndexFunc := func(obj client.Object) []string {
		return []string{obj.(*v1alpha1.IntegrationTestScenario).Spec.Application}
	}

	return mgr.GetCache().IndexField(context.Background(), &v1alpha1.IntegrationTestScenario{},
		"spec.application", integrationTestScenariosIndexFunc)
}

// setupControllerWithManager sets up the controller with the Manager which monitors new PipelineRuns and filters
// out status updates.
func setupControllerWithManager(manager ctrl.Manager, reconciler *Reconciler) error {
	err := setupApplicationComponentCache(manager)
	if err != nil {
		return err
	}
	err = setupApplicationSnapshotCache(manager)
	if err != nil {
		return err
	}
	err = setupIntegrationTestScenarioCache(manager)
	if err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(manager).
		For(&tektonv1beta1.PipelineRun{}, builder.WithPredicates(tekton.IntegrationOrBuildPipelineRunSucceededPredicate())).
		Complete(reconciler)
}
