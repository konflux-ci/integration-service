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

package nudging

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"

	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logger "sigs.k8s.io/controller-runtime/pkg/log"
)

// nudgeOwnerKind is the Kind of the nudge PipelineRun that owns the support objects.
const nudgeOwnerKind = "PipelineRun"

// errStaleNudgeOwner reports that a nudge support object of the wanted name exists and every
// owner it lists is a PipelineRun of the same name with another UID: the previous nudge
// PipelineRun was deleted and the garbage collector has not removed the object yet. The caller
// requeues; the next pass creates a fresh object (STONEINTG-1907).
var errStaleNudgeOwner = errors.New("nudge support object is owned by a previous PipelineRun of the same name")

// ensurePendingPipelineRun creates plr with spec.status=PipelineRunPending and returns the
// persisted object, so its UID can be used in the owner references of the nudge support objects.
// Tekton creates no TaskRun while a PipelineRun is pending, which keeps the nudge from starting
// before its Secret and ConfigMaps exist (a missing envFrom Secret makes Tekton fail the TaskRun
// permanently). The caller starts the PipelineRun with startPendingPipelineRun once the support
// objects exist.
//
// When a PipelineRun of that name already exists it is adopted instead: the name is derived from
// the build PipelineRun UID or the batch ID, so an existing one is either this nudge's own object
// from an earlier attempt or a duplicate fired by a retry. created reports which of the two
// happened. The returned object is always the persisted one: the local object after an
// AlreadyExists error carries no UID and an owner reference built from it would be rejected.
func ensurePendingPipelineRun(ctx context.Context, c client.Client, plr *tektonv1.PipelineRun) (*tektonv1.PipelineRun, bool, error) {
	log := logger.FromContext(ctx)

	plr.Spec.Status = tektonv1.PipelineRunSpecStatusPending
	if err := c.Create(ctx, plr); err == nil {
		log.Info("created pending nudge PipelineRun", "name", plr.Name)
		return plr, true, nil
	} else if !apierrors.IsAlreadyExists(err) {
		return nil, false, err
	}

	existing := &tektonv1.PipelineRun{}
	key := types.NamespacedName{Name: plr.Name, Namespace: plr.Namespace}
	if err := getWithNotFoundRetry(ctx, c, key, existing); err != nil {
		return nil, false, err
	}
	if existing.DeletionTimestamp != nil {
		return nil, false, fmt.Errorf("nudge PipelineRun %s is being deleted", key.Name)
	}

	log.Info("nudge PipelineRun already exists; adopting it", "name", key.Name, "pending", existing.IsPending())
	return existing, false, nil
}

// createOwnedObjectIfMissing creates obj with a plain owner reference to owner or, when an object
// of that name already exists, leaves it as it is (see checkForStaleNudgeOwner). The reference is a
// plain one — no Controller and no BlockOwnerDeletion — the same form
// helpers.createObjectWithOwnerReferenceIfMissing sets for the per-component ServiceAccount:
// blockOwnerDeletion would need delete on the owner, which integration-service is not granted.
// Owner references are set at creation time only, because OpenShift's
// OwnerReferencesPermissionEnforcement admission plugin refuses a change to ownerReferences by a
// requester that may not delete the object.
//
// The object is created first, deliberately: a cached Get can report an object the garbage
// collector has already removed, and Secrets are not cached at all (cmd/main.go).
func createOwnedObjectIfMissing(ctx context.Context, c client.Client, owner *tektonv1.PipelineRun, obj client.Object) error {
	log := logger.FromContext(ctx)

	obj.SetOwnerReferences(append(obj.GetOwnerReferences(), plainOwnerReference(owner)))

	if err := c.Create(ctx, obj); err == nil {
		log.Info("created nudge support object", "kind", objectKind(obj), "name", obj.GetName(), "namespace", obj.GetNamespace(), "owner", owner.Name)
		return nil
	} else if !apierrors.IsAlreadyExists(err) {
		return err
	}

	existing := obj.DeepCopyObject().(client.Object)
	key := types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}
	if err := getWithNotFoundRetry(ctx, c, key, existing); err != nil {
		return err
	}
	return checkForStaleNudgeOwner(ctx, owner, existing)
}

// plainOwnerReference returns the reference the nudge support objects carry to their PipelineRun.
// It is deliberately constructed rather than taken from controllerutil.SetOwnerReference so that
// the absence of Controller and BlockOwnerDeletion — which keep the reference create-only, without
// extra permissions — is explicit at the call site.
func plainOwnerReference(owner *tektonv1.PipelineRun) metav1.OwnerReference {
	return metav1.OwnerReference{
		APIVersion: tektonv1.SchemeGroupVersion.String(),
		Kind:       nudgeOwnerKind,
		Name:       owner.Name,
		UID:        owner.UID,
	}
}

// checkForStaleNudgeOwner looks at the owners of a support object that already exists. It returns
// errStaleNudgeOwner when every owner it lists is a PipelineRun of owner's API group and name with
// another UID, so the caller requeues and the garbage collector removes the object in the
// meantime. It returns nil when the object lists owner's UID, has no owner, or has an owner that
// is not a PipelineRun of owner's API group and name: an unowned object is migration residue from
// before STONEINTG-1907 and cannot be adopted without delete permission, so it is left as it is.
func checkForStaleNudgeOwner(ctx context.Context, owner *tektonv1.PipelineRun, existing client.Object) error {
	log := logger.FromContext(ctx)
	refs := existing.GetOwnerReferences()
	if len(refs) == 0 {
		log.Info("nudge support object already exists without an owner reference; leaving it unchanged",
			"kind", objectKind(existing), "name", existing.GetName(), "namespace", existing.GetNamespace())
		return nil
	}

	for _, ref := range refs {
		if ref.UID == owner.UID {
			return nil
		}
		// Anything that is not a PipelineRun of this API group and name belongs to someone else,
		// so the object is left as it is rather than reported as belonging to a previous nudge.
		if !strings.HasPrefix(ref.APIVersion, tektonv1.SchemeGroupVersion.Group+"/") ||
			ref.Kind != nudgeOwnerKind || ref.Name != owner.Name {
			return nil
		}
	}

	return errStaleNudgeOwner
}

// startPendingPipelineRun clears spec.status on a PipelineRun created by ensurePendingPipelineRun,
// which starts it. It is a no-op for a PipelineRun that is not pending: an object created before
// STONEINTG-1907 (and therefore already started) or one another actor has started already. The
// field is patched with an optimistic lock, so a concurrent start by a second writer surfaces as a
// conflict that RetryOnConflict turns into a no-op.
func startPendingPipelineRun(ctx context.Context, c client.Client, plr *tektonv1.PipelineRun) error {
	log := logger.FromContext(ctx)

	if !plr.IsPending() {
		return nil
	}

	key := types.NamespacedName{Name: plr.Name, Namespace: plr.Namespace}
	if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		current := &tektonv1.PipelineRun{}
		if err := getWithNotFoundRetry(ctx, c, key, current); err != nil {
			return err
		}
		if !current.IsPending() {
			return nil
		}
		if current.DeletionTimestamp != nil {
			return fmt.Errorf("nudge PipelineRun %s is being deleted", key.Name)
		}
		original := current.DeepCopy()
		current.Spec.Status = ""
		return c.Patch(ctx, current, client.MergeFromWithOptions(original, client.MergeFromWithOptimisticLock{}))
	}); err != nil {
		return err
	}

	plr.Spec.Status = ""
	log.Info("started pending nudge PipelineRun", "name", plr.Name)
	return nil
}

// getWithNotFoundRetry reads obj, retrying while the API reports it as missing. PipelineRuns and
// ConfigMaps are read through the informer cache, which lags a create made moments earlier in
// another request; Secrets are read uncached.
func getWithNotFoundRetry(ctx context.Context, c client.Client, key types.NamespacedName, obj client.Object) error {
	return retry.OnError(retry.DefaultBackoff, apierrors.IsNotFound, func() error {
		return c.Get(ctx, key, obj)
	})
}

// objectKind returns the Go type name of obj ("Secret", "ConfigMap") for log lines: typed objects
// returned by the client carry no TypeMeta.
func objectKind(obj client.Object) string {
	return reflect.TypeOf(obj).Elem().Name()
}
