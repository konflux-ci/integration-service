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
	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"context"
	"fmt"
	"reflect"
)

// nolint:unused
// log is for logging in this package.
var snapshotlog = logf.Log.WithName("snapshot-webhook")

func SetupSnapshotWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&applicationapiv1alpha1.Snapshot{}).
		WithValidator(&SnapshotCustomValidator{}).
		Complete()
}

// SnapshotCustomValidator is a webhook handler and does not need deepcopy methods.
// +k8s:deepcopy-gen=false
type SnapshotCustomValidator struct {
	// TODO(user): Add more fields as needed for validation
}

// +kubebuilder:webhook:path=/validate-appstudio-redhat-com-v1alpha1-snapshot,mutating=false,failurePolicy=fail,sideEffects=None,groups=appstudio.redhat.com,resources=snapshots,verbs=create;update;delete,versions=v1alpha1,name=vsnapshot.kb.io,admissionReviewVersions=v1

var _ webhook.CustomValidator = &SnapshotCustomValidator{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (v *SnapshotCustomValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (warnings admission.Warnings, err error) {
	snapshot, ok := obj.(*applicationapiv1alpha1.Snapshot)
	if !ok {
		return nil, fmt.Errorf("expected a Snapshot object but got %T", obj)
	}

	snapshotlog.Info("Validating Snapshot upon creation", "name", snapshot.GetName())

	// No specific validation needed for create operations
	// Components can be set during creation

	return nil, nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (v *SnapshotCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (warnings admission.Warnings, err error) {
	oldSnapshot, ok := oldObj.(*applicationapiv1alpha1.Snapshot)
	if !ok {
		return nil, fmt.Errorf("expected a Snapshot object for oldObj but got %T", oldObj)
	}

	newSnapshot, ok := newObj.(*applicationapiv1alpha1.Snapshot)
	if !ok {
		return nil, fmt.Errorf("expected a Snapshot object for newObj but got %T", newObj)
	}

	snapshotlog.Info("Validating Snapshot upon update", "name", newSnapshot.GetName())

	// Check if components field has been modified
	if !reflect.DeepEqual(oldSnapshot.Spec.Components, newSnapshot.Spec.Components) {
		snapshotlog.Info("Components field modification detected", "name", newSnapshot.GetName())
		return nil, field.Invalid(
			field.NewPath("spec").Child("components"),
			newSnapshot.Spec.Components,
			"components field is immutable and cannot be modified after creation",
		)
	}

	return nil, nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (v *SnapshotCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	snapshot, ok := obj.(*applicationapiv1alpha1.Snapshot)
	if !ok {
		return nil, fmt.Errorf("expected a Snapshot object but got %T", obj)
	}

	snapshotlog.Info("Validating Snapshot upon deletion", "name", snapshot.GetName())

	// No specific validation needed for delete operations

	return nil, nil
}
