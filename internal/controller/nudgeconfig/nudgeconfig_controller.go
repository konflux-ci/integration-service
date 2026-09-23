/*
Copyright 2026 Red Hat Inc.

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

package nudgeconfig

import (
	"context"
	"reflect"
	"time"

	"github.com/go-logr/logr"
	"github.com/konflux-ci/integration-service/api/v1beta2"
	"github.com/konflux-ci/integration-service/tekton/nudging"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const minBatchRequeue = 5 * time.Second

// Reconciler processes scheduled nudge batch windows and one-shot spec.actions for NudgeConfig resources.
type Reconciler struct {
	client.Client
	Log logr.Logger
}

// SetupController registers the NudgeConfig reconciler with the manager.
func SetupController(mgr ctrl.Manager, logger *logr.Logger) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1beta2.NudgeConfig{}, builder.WithPredicates(predicate.Or(
			predicate.GenerationChangedPredicate{},
			predicate.AnnotationChangedPredicate{},
			nudgeBatchStatusChanged{},
		))).
		Complete(&Reconciler{
			Client: mgr.GetClient(),
			Log:    logger.WithName("nudgeconfig"),
		})
}

//+kubebuilder:rbac:groups=appstudio.redhat.com,resources=nudgeconfigs,verbs=get;list;watch;patch;update
//+kubebuilder:rbac:groups=appstudio.redhat.com,resources=nudgeconfigs/status,verbs=get;update;patch

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.Log.WithValues("nudgeConfig", req.NamespacedName)

	nudgeConfig := &v1beta2.NudgeConfig{}
	if err := r.Get(ctx, req.NamespacedName, nudgeConfig); err != nil {
		if apierrors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	hadForceFire := nudgeConfig.Spec.Actions != nil && nudgeConfig.Spec.Actions.ForceFire != nil
	if hadForceFire {
		if err := nudging.ProcessForceFireAction(ctx, r.Client, nudgeConfig); err != nil {
			logger.Error(err, "Failed to process forceFire action")
			return reconcile.Result{}, err
		}
		if err := nudging.ClearNudgeConfigActions(ctx, r.Client, nudgeConfig); err != nil {
			logger.Error(err, "Failed to clear processed nudge config actions")
			return reconcile.Result{}, err
		}
	}

	if _, err := nudging.ProcessDueNudgeBatches(ctx, r.Client, nudgeConfig); err != nil {
		logger.Error(err, "Failed to process due nudge batches")
		return reconcile.Result{}, err
	}

	if err := r.Get(ctx, req.NamespacedName, nudgeConfig); err != nil {
		return reconcile.Result{}, err
	}
	nextWake := nudging.NextBatchWakeDuration(nudgeConfig.Status.ActiveBatches)
	if nextWake <= 0 {
		return reconcile.Result{}, nil
	}
	if nextWake < minBatchRequeue {
		nextWake = minBatchRequeue
	}
	return reconcile.Result{RequeueAfter: nextWake}, nil
}

type nudgeBatchStatusChanged struct{}

func (nudgeBatchStatusChanged) Create(e event.CreateEvent) bool {
	return true
}

func (nudgeBatchStatusChanged) Delete(event.DeleteEvent) bool {
	return false
}

func (nudgeBatchStatusChanged) Generic(event.GenericEvent) bool {
	return true
}

func (nudgeBatchStatusChanged) Update(e event.UpdateEvent) bool {
	if e.ObjectOld == nil || e.ObjectNew == nil {
		return true
	}
	oldNC, okOld := e.ObjectOld.(*v1beta2.NudgeConfig)
	newNC, okNew := e.ObjectNew.(*v1beta2.NudgeConfig)
	if !okOld || !okNew {
		return true
	}
	if !reflect.DeepEqual(oldNC.Status.ActiveBatches, newNC.Status.ActiveBatches) {
		return true
	}
	if (oldNC.Spec.Actions == nil) != (newNC.Spec.Actions == nil) {
		return true
	}
	if oldNC.Spec.Actions != nil && newNC.Spec.Actions != nil &&
		!reflect.DeepEqual(oldNC.Spec.Actions, newNC.Spec.Actions) {
		return true
	}
	return false
}
