/*
Copyright 2024 Red Hat Inc.

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

package componentgroup

import (
	"context"
	"reflect"

	"github.com/go-logr/logr"
	"github.com/konflux-ci/integration-service/api/v1beta2"
	"github.com/konflux-ci/integration-service/helpers"
	"github.com/konflux-ci/integration-service/loader"
	"github.com/konflux-ci/operator-toolkit/controller"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// Reconciler reconciles a ComponentGroup object.
type Reconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// NewComponentGroupReconciler creates and returns a Reconciler.
func NewComponentGroupReconciler(client client.Client, logger *logr.Logger, scheme *runtime.Scheme) *Reconciler {
	return &Reconciler{
		Client: client,
		Log:    logger.WithName("componentgroup"),
		Scheme: scheme,
	}
}

//+kubebuilder:rbac:groups=appstudio.redhat.com,resources=componentgroups,verbs=get;list;watch;update;patch
//+kubebuilder:rbac:groups=appstudio.redhat.com,resources=componentgroups/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=appstudio.redhat.com,resources=componentgroups/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := helpers.IntegrationLogger{Logger: r.Log.WithValues("componentgroup", req.NamespacedName)}
	loader := loader.NewLoader()

	componentGroup := &v1beta2.ComponentGroup{}
	err := r.Get(ctx, req.NamespacedName, componentGroup)
	if err != nil {
		logger.Error(err, "Failed to get ComponentGroup for", "req", req.NamespacedName)
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	adapter := NewAdapter(ctx, componentGroup, logger, loader, r.Client)

	return controller.ReconcileHandler([]controller.Operation{
		adapter.EnsureGCLAlignedWithSpecComponents,
	})
}

// AdapterInterface is an interface defining all the operations that should be defined in a ComponentGroup adapter.
type AdapterInterface interface {
	EnsureGCLAlignedWithSpecComponents() (controller.OperationResult, error)
}

// SetupController creates a new ComponentGroup controller and adds it to the Manager.
func SetupController(manager ctrl.Manager, log *logr.Logger) error {
	return setupControllerWithManager(manager, NewComponentGroupReconciler(manager.GetClient(), log, manager.GetScheme()))
}

// setupControllerWithManager sets up the controller with the Manager which monitors ComponentGroups
// and filters events to only create events and spec.components updates.
func setupControllerWithManager(manager ctrl.Manager, controller *Reconciler) error {
	return ctrl.NewControllerManagedBy(manager).
		For(&v1beta2.ComponentGroup{}).
		Named("componentgroup").
		WithEventFilter(predicate.Or(
			componentGroupCreatedPredicate(),
			componentGroupSpecComponentsChangedPredicate(),
		)).
		Complete(controller)
}

// componentGroupCreatedPredicate returns a predicate that passes only for ComponentGroup create events.
func componentGroupCreatedPredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(createEvent event.CreateEvent) bool {
			return true
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return false
		},
		DeleteFunc: func(deleteEvent event.DeleteEvent) bool {
			return false
		},
		GenericFunc: func(genericEvent event.GenericEvent) bool {
			return false
		},
	}
}

// componentGroupSpecComponentsChangedPredicate returns a predicate that passes only when
// spec.components has changed on an update event.
func componentGroupSpecComponentsChangedPredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(createEvent event.CreateEvent) bool {
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldCG, ok1 := e.ObjectOld.(*v1beta2.ComponentGroup)
			newCG, ok2 := e.ObjectNew.(*v1beta2.ComponentGroup)
			if !ok1 || !ok2 {
				return false
			}
			return !reflect.DeepEqual(oldCG.Spec.Components, newCG.Spec.Components)
		},
		DeleteFunc: func(deleteEvent event.DeleteEvent) bool {
			return false
		},
		GenericFunc: func(genericEvent event.GenericEvent) bool {
			return false
		},
	}
}
