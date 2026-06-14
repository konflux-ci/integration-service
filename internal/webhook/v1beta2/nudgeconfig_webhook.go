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

package v1beta2

import (
	"context"
	"fmt"
	"reflect"

	"github.com/konflux-ci/integration-service/api/v1beta2"
	"github.com/konflux-ci/integration-service/pkg/dag"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// nolint:unused
var nudgeconfiglog = logf.Log.WithName("nudgeconfig-webhook")

func SetupNudgeConfigWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&v1beta2.NudgeConfig{}).
		WithValidator(&NudgeConfigCustomValidator{Client: mgr.GetClient()}).
		Complete()
}

// NudgeConfigCustomValidator is a webhook handler and does not need deepcopy methods. (validator = validating webhook)
// +k8s:deepcopy-gen=false
type NudgeConfigCustomValidator struct {
	Client client.Client
}

// +kubebuilder:webhook:path=/validate-appstudio-redhat-com-v1beta2-nudgeconfig,mutating=false,failurePolicy=fail,sideEffects=None,groups=appstudio.redhat.com,resources=nudgeconfigs,verbs=create;update,versions=v1beta2,name=vnudgeconfig.kb.io,admissionReviewVersions=v1

var _ webhook.CustomValidator = &NudgeConfigCustomValidator{}

func (v *NudgeConfigCustomValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	nudgeConfig, ok := obj.(*v1beta2.NudgeConfig)
	if !ok {
		return nil, fmt.Errorf("expected a NudgeConfig object but got %T", obj)
	}

	nudgeconfiglog.Info("Validating NudgeConfig upon creation", "name", nudgeConfig.Name)

	if len(nudgeConfig.Spec.Nudges) > 0 {
		if err := dag.ValidateNudgeGraph(nudgeConfig.Spec.Nudges); err != nil {
			return nil, fmt.Errorf("error validating nudge graph: %v", err)
		}
	}

	return nil, nil
}

func (v *NudgeConfigCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	oldNudgeConfig, ok := oldObj.(*v1beta2.NudgeConfig)
	if !ok {
		return nil, fmt.Errorf("expected a NudgeConfig for oldObj but got %T", oldObj)
	}
	newNudgeConfig, ok := newObj.(*v1beta2.NudgeConfig)
	if !ok {
		return nil, fmt.Errorf("expected a NudgeConfig for newObj but got %T", newObj)
	}

	nudgeconfiglog.Info("Validating NudgeConfig upon update", "name", newNudgeConfig.Name)

	if !reflect.DeepEqual(oldNudgeConfig.Spec.Nudges, newNudgeConfig.Spec.Nudges) {
		if len(newNudgeConfig.Spec.Nudges) > 0 {
			if err := dag.ValidateNudgeGraph(newNudgeConfig.Spec.Nudges); err != nil {
				return nil, fmt.Errorf("error validating nudge graph: %v", err)
			}
		}
	}

	return nil, nil
}

func (v *NudgeConfigCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	return nil, nil
}
