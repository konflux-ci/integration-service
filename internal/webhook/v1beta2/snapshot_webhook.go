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
	"github.com/konflux-ci/integration-service/gitops"
	"github.com/konflux-ci/integration-service/helpers"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"context"
	"fmt"
	"reflect"
	"strings"
)

// nolint:unused
// log is for logging in this package.
var snapshotlog = logf.Log.WithName("snapshot-webhook")

func SetupSnapshotWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&applicationapiv1alpha1.Snapshot{}).
		WithDefaulter(&SnapshotCustomDefaulter{}).
		WithValidator(&SnapshotCustomValidator{Client: mgr.GetClient()}).
		Complete()
}

// SnapshotCustomValidator is a webhook handler and does not need deepcopy methods. (validator = validating webhook)
// +k8s:deepcopy-gen=false
type SnapshotCustomValidator struct {
	Client client.Client
	// TODO(user): Add more fields as needed for validation
}

// +kubebuilder:webhook:path=/validate-appstudio-redhat-com-v1alpha1-snapshot,mutating=false,failurePolicy=fail,sideEffects=None,groups=appstudio.redhat.com,resources=snapshots,verbs=create;update;delete,versions=v1alpha1,name=vsnapshot.kb.io,admissionReviewVersions=v1

var _ webhook.CustomValidator = &SnapshotCustomValidator{}

// SnapshotCustomDefaulter is a webhook handler and does not need deepcopy methods. (defaulter = mutating webhook)
// +k8s:deepcopy-gen=false
type SnapshotCustomDefaulter struct {
	// no fields required at this time
}

// +kubebuilder:webhook:path=/mutate-appstudio-redhat-com-v1alpha1-snapshot,mutating=true,failurePolicy=ignore,sideEffects=None,groups=appstudio.redhat.com,resources=snapshots,verbs=create,versions=v1alpha1,name=vsnapshot.kb.io,admissionReviewVersions=v1

var _ webhook.CustomDefaulter = &SnapshotCustomDefaulter{}

func (d *SnapshotCustomDefaulter) Default(ctx context.Context, obj runtime.Object) error {
	snapshot, ok := obj.(*applicationapiv1alpha1.Snapshot)
	if !ok {
		return fmt.Errorf("expected a Snapshot object but got %T", obj)
	}

	// TODO: remove "application or" when we remove old application-specific code
	snapshotlog.Info("Setting application or componentgroup label on Snapshot", "name", snapshot.GetName())

	if snapshot.Labels == nil {
		snapshot.Labels = make(map[string]string)
	}

	if snapshot.Spec.Application != "" {
		snapshot.Labels[gitops.ApplicationNameLabel] = snapshot.Spec.Application
	} else {
		snapshot.Labels[gitops.ComponentGroupNameLabel] = snapshot.Spec.ComponentGroup
	}

	snapshotlog.Info("Set application label", "snapshot", snapshot.GetName(), "application", snapshot.Spec.Application)

	return nil
}

// validateSnapshotProvenance rejects PAC annotations when the Snapshot does not
// contain the required Integration Service provenance marker.
func validateSnapshotProvenance(snapshot *applicationapiv1alpha1.Snapshot) error {
	provenance := snapshot.Annotations[gitops.SnapshotProvenanceAnnotation]

	for annotationKey := range snapshot.Annotations {
		if !strings.HasPrefix(annotationKey, gitops.PipelinesAsCodePrefix+"/") {
			continue
		}

		if provenance == gitops.SnapshotProvenanceValue {
			continue
		}

		snapshotlog.Info(
			"Rejecting Snapshot with PAC annotations and invalid provenance",
			"namespace", snapshot.Namespace,
			"name", snapshot.Name,
			"pacAnnotation", annotationKey,
			"provenance", provenance,
		)

		return field.Invalid(
			field.NewPath("metadata").Child("annotations"),
			provenance,
			fmt.Sprintf(
				"PAC annotations require %s=%s",
				gitops.SnapshotProvenanceAnnotation,
				gitops.SnapshotProvenanceValue,
			),
		)
	}

	return nil
}

// pacAnnotationsChanged reports whether the PAC annotations changed between
// two Snapshot versions.
func pacAnnotationsChanged(oldAnnotations, newAnnotations map[string]string) bool {
	for annotationKey, oldValue := range oldAnnotations {
		if !strings.HasPrefix(annotationKey, gitops.PipelinesAsCodePrefix+"/") {
			continue
		}

		newValue, exists := newAnnotations[annotationKey]
		if !exists || newValue != oldValue {
			return true
		}
	}

	for annotationKey, newValue := range newAnnotations {
		if !strings.HasPrefix(annotationKey, gitops.PipelinesAsCodePrefix+"/") {
			continue
		}

		oldValue, exists := oldAnnotations[annotationKey]
		if !exists || oldValue != newValue {
			return true
		}
	}

	return false
}

// annotationChanged reports whether one annotation was added, removed, or
// changed between two Snapshot versions.
func annotationChanged(oldAnnotations, newAnnotations map[string]string, annotationKey string) bool {
	oldValue, oldExists := oldAnnotations[annotationKey]
	newValue, newExists := newAnnotations[annotationKey]

	return oldExists != newExists || oldValue != newValue
}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (v *SnapshotCustomValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (warnings admission.Warnings, err error) {
	snapshot, ok := obj.(*applicationapiv1alpha1.Snapshot)
	if !ok {
		return nil, fmt.Errorf("expected a Snapshot object but got %T", obj)
	}

	snapshotlog.Info("Validating Snapshot upon creation", "name", snapshot.GetName())

	const maxK8sNameLength = 63
	// validate user created override snapshots do not exceed max name length
	if snapshot.Name != "" && len(snapshot.Name) > maxK8sNameLength {
		return nil, field.Invalid(
			field.NewPath("metadata").Child("name"),
			snapshot.Name,
			fmt.Sprintf("name is too long (%d characters); must be at most %d characters",
				len(snapshot.Name), maxK8sNameLength),
		)
	}

	if err := validateSnapshotProvenance(snapshot); err != nil {
		return nil, err
	}

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

	if pacAnnotationsChanged(oldSnapshot.Annotations, newSnapshot.Annotations) ||
		annotationChanged(oldSnapshot.Annotations, newSnapshot.Annotations, gitops.SnapshotProvenanceAnnotation) {
		if err := validateSnapshotProvenance(newSnapshot); err != nil {
			return nil, err
		}
	}

	return nil, nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (v *SnapshotCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	var logger helpers.IntegrationLogger
	logger.Logger = snapshotlog
	snapshot, ok := obj.(*applicationapiv1alpha1.Snapshot)
	if !ok {
		return nil, fmt.Errorf("expected a Snapshot object but got %T", obj)
	}

	snapshotlog.Info("Validating Snapshot upon deletion", "name", snapshot.GetName())
	integrationPipelineList := &tektonv1.PipelineRunList{}
	opts := []client.ListOption{
		client.InNamespace(snapshot.Namespace),
		client.MatchingLabels{
			"pipelines.appstudio.openshift.io/type": "test",
			"appstudio.openshift.io/snapshot":       snapshot.Name,
		},
	}
	err := v.Client.List(ctx, integrationPipelineList, opts...)
	if err != nil {
		return nil, err
	}
	for i := range integrationPipelineList.Items {
		plr := &integrationPipelineList.Items[i]
		err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
			fresh := &tektonv1.PipelineRun{}
			if getErr := v.Client.Get(ctx, client.ObjectKeyFromObject(plr), fresh); getErr != nil {
				if apierrors.IsNotFound(getErr) {
					return nil
				}
				return getErr
			}
			return helpers.RemoveFinalizerFromPipelineRun(ctx, v.Client, logger, fresh, helpers.IntegrationPipelineRunFinalizer)
		})
		if err != nil {
			snapshotlog.Error(err, "failed to remove finalizer from integration pipelinerun", "integration pipelineRun", plr.Name)
			return nil, err
		}
	}
	snapshotlog.Info("finalizer has been removed from integration pipelineRuns before deleting Snapshot", "snapshot.Namespace", snapshot.Namespace, "snapshot", snapshot.Name)

	return nil, nil
}
