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
	"strings"

	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	"github.com/konflux-ci/integration-service/api/v1beta2"
	"github.com/konflux-ci/integration-service/pkg/dag"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

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

	if err := v.validateComponentsExist(ctx, nudgeConfig.Namespace, nudgeConfig.Spec.Nudges); err != nil {
		return nil, err
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

	// Cycle detection: whole-graph validation when anything changes.
	// A new edge can close a cycle through pre-existing edges, so the
	// entire graph must be re-validated.
	if !reflect.DeepEqual(oldNudgeConfig.Spec.Nudges, newNudgeConfig.Spec.Nudges) {
		if len(newNudgeConfig.Spec.Nudges) > 0 {
			if err := dag.ValidateNudgeGraph(newNudgeConfig.Spec.Nudges); err != nil {
				return nil, fmt.Errorf("error validating nudge graph: %v", err)
			}
		}
	}

	// Existence: per-pair validation of only newly-added (from,to) pairs.
	// Pre-existing pairs are NOT re-validated — a Component deleted after
	// its entry was added does not block updates (staleness is the
	// stale-reference controller's job).
	oldKeys := nudgeKeySet(oldNudgeConfig.Spec.Nudges)
	var added []v1beta2.NudgeRelationship
	for _, n := range newNudgeConfig.Spec.Nudges {
		if _, existed := oldKeys[nudgeKey(n)]; !existed {
			added = append(added, n)
		}
	}
	if err := v.validateComponentsExist(ctx, newNudgeConfig.Namespace, added); err != nil {
		return nil, err
	}

	return nil, nil
}

func (v *NudgeConfigCustomValidator) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

func (v *NudgeConfigCustomValidator) validateComponentsExist(ctx context.Context, namespace string, nudges []v1beta2.NudgeRelationship) error {
	if len(nudges) == 0 {
		return nil
	}

	componentList := &applicationapiv1alpha1.ComponentList{}
	if err := v.Client.List(ctx, componentList, client.InNamespace(namespace)); err != nil {
		return fmt.Errorf("failed to list Components in namespace %q: %w", namespace, err)
	}

	existing := make(map[string]struct{}, len(componentList.Items))
	for i := range componentList.Items {
		existing[componentList.Items[i].Name] = struct{}{}
	}

	var missingFrom, missingTo []string
	seenFrom, seenTo := map[string]struct{}{}, map[string]struct{}{}
	for _, n := range nudges {
		if _, ok := existing[n.From]; !ok {
			if _, dup := seenFrom[n.From]; !dup {
				seenFrom[n.From] = struct{}{}
				missingFrom = append(missingFrom, n.From)
			}
		}
		if _, ok := existing[n.To]; !ok {
			if _, dup := seenTo[n.To]; !dup {
				seenTo[n.To] = struct{}{}
				missingTo = append(missingTo, n.To)
			}
		}
	}

	if len(missingFrom) == 0 && len(missingTo) == 0 {
		return nil
	}

	msg := fmt.Sprintf("NudgeConfig references non-existent Component(s) in namespace %q", namespace)
	if len(missingFrom) > 0 {
		msg += fmt.Sprintf("; missing 'from' component(s): %s", strings.Join(missingFrom, ", "))
	}
	if len(missingTo) > 0 {
		msg += fmt.Sprintf("; missing 'to' component(s): %s", strings.Join(missingTo, ", "))
	}
	nudgeconfiglog.Info(msg)
	return fmt.Errorf("%s", msg)
}

func nudgeKey(n v1beta2.NudgeRelationship) string { return n.From + "\x00" + n.To }

func nudgeKeySet(nudges []v1beta2.NudgeRelationship) map[string]struct{} {
	keys := make(map[string]struct{}, len(nudges))
	for _, n := range nudges {
		keys[nudgeKey(n)] = struct{}{}
	}
	return keys
}
