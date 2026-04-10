/*
Copyright 2023 Red Hat Inc.

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
	"reflect"

	"github.com/konflux-ci/integration-service/api/v1beta2"
	"github.com/konflux-ci/integration-service/pkg/dag"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"context"
	"fmt"
)

// nolint:unused
// log is for logging in this package.
var componentgrouplog = logf.Log.WithName("componentgroup-webhook")

func SetupComponentGroupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&v1beta2.ComponentGroup{}).
		WithValidator(&ComponentGroupCustomValidator{Client: mgr.GetClient()}).
		Complete()
}

// ComponentGroupCustomValidator is a webhook handler and does not need deepcopy methods. (validator = validating webhook)
// +k8s:deepcopy-gen=false
type ComponentGroupCustomValidator struct {
	Client client.Client
	// TODO(user): Add more fields as needed for validation
}

// +kubebuilder:webhook:path=/validate-appstudio-redhat-com-v1beta2-componentgroup,mutating=false,failurePolicy=fail,sideEffects=None,groups=appstudio.redhat.com,resources=componentgroups,verbs=create;update;delete,versions=v1beta2,name=vcomponentgroup.kb.io,admissionReviewVersions=v1

var _ webhook.CustomValidator = &ComponentGroupCustomValidator{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (v *ComponentGroupCustomValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (warnings admission.Warnings, err error) {
	// Validate testGraph
	componentGroup, ok := obj.(*v1beta2.ComponentGroup)
	if !ok {
		return nil, fmt.Errorf("expected a ComponentGroup object but got %T", obj)
	}

	componentgrouplog.Info("Validating created component")
	if componentGroup.Spec.TestGraph != nil {
		err := dag.ValidateTestGraph(componentGroup.Spec.TestGraph)
		if err != nil {
			return nil, fmt.Errorf("error validating test graph: %v", err)
		}
	}
	return nil, nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (v *ComponentGroupCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (warnings admission.Warnings, err error) {
	// If testGraph is updated, validate testGraph
	oldComponentGroup, ok := oldObj.(*v1beta2.ComponentGroup)
	if !ok {
		return nil, fmt.Errorf("expected a ComponentGroup for oldObj but got %T", oldObj)
	}
	newComponentGroup, ok := newObj.(*v1beta2.ComponentGroup)
	if !ok {
		return nil, fmt.Errorf("expected a ComponentGroup for newObj but got %T", newObj)
	}

	componentgrouplog.Info("Validating updated component")
	if !reflect.DeepEqual(oldComponentGroup.Spec.TestGraph, newComponentGroup.Spec.TestGraph) {
		componentgrouplog.Info("Validating updated test graph")
		err := dag.ValidateTestGraph(newComponentGroup.Spec.TestGraph)
		if err != nil {
			return nil, fmt.Errorf("error validating test graph: %v", err)
		}
	}

	return nil, nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (v *ComponentGroupCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	return nil, nil
}
