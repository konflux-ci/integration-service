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

package component

import (
	"context"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	h "github.com/konflux-ci/integration-service/helpers"
	tektonconsts "github.com/konflux-ci/integration-service/tekton/consts"
)

// errStaleOwner reports that an object of the wanted name exists and every owner it lists is a
// Component of the owner's API group and name with another UID: the Component was deleted and
// recreated under the same name and the garbage collector has not removed the object yet. The caller
// requeues; the next pass creates a fresh object.
var errStaleOwner = errors.New("object is owned by a previous Component of the same name and is waiting for garbage collection")

// isStaleOwnerError reports whether err wraps errStaleOwner.
func isStaleOwnerError(err error) bool {
	return errors.Is(err, errStaleOwner)
}

// ensureIntegrationPipelineServiceAccount makes sure the per-component integration pipeline
// ServiceAccount its-pipeline-<owner name> and the RoleBinding of the same name to the
// konflux-integration-runner ClusterRole exist in the owner's namespace and carry an owner reference to the
// owner (STONEINTG-1799). The owner is a Component of any kind. The reference is a plain owner
// reference, as build-service uses for build-pipeline-<component>, set when the object is created;
// garbage collection removes the object when the last owner goes. An object that already exists is
// never updated here: OpenShift's OwnerReferencesPermissionEnforcement admission plugin refuses a
// change to ownerReferences by a requester that may not delete the object, and this controller is not
// granted delete. The push credential is linked to the account by image-controller (STONEINTG-1804).
// Returns an error wrapping errStaleOwner when the caller should requeue.
func ensureIntegrationPipelineServiceAccount(ctx context.Context, c client.Client, logger h.IntegrationLogger, owner client.Object) error {
	name := h.ComponentIntegrationPipelineServiceAccountName(owner.GetName())
	namespace := owner.GetNamespace()

	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
	}
	if err := ensureOwnedObject(ctx, c, logger, owner, "ServiceAccount", sa); err != nil {
		return fmt.Errorf("failed to ensure ServiceAccount %s/%s: %w", namespace, name, err)
	}

	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "ClusterRole",
			Name:     tektonconsts.DefaultIntegrationPipelineClusterRoleName,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      rbacv1.ServiceAccountKind,
				Name:      name,
				Namespace: namespace,
			},
		},
	}
	if err := ensureOwnedObject(ctx, c, logger, owner, "RoleBinding", rb); err != nil {
		return fmt.Errorf("failed to ensure RoleBinding %s/%s: %w", namespace, name, err)
	}

	return nil
}

// ensureOwnedObject creates obj with an owner reference to owner or, when an object of that name
// already exists, leaves it as it is (see checkExistingObject). kind is only used in log lines.
func ensureOwnedObject(ctx context.Context, c client.Client, logger h.IntegrationLogger, owner client.Object, kind string, obj client.Object) error {
	key := types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}
	existing := obj.DeepCopyObject().(client.Object)

	err := c.Get(ctx, key, existing)
	if err == nil {
		return checkExistingObject(c, owner, existing)
	}
	if !apierrors.IsNotFound(err) {
		return err
	}

	if err := controllerutil.SetOwnerReference(owner, obj, c.Scheme()); err != nil {
		return err
	}
	if err := c.Create(ctx, obj); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return err
		}
		// Created concurrently or not yet in the cache: read it and leave it as it is.
		if err := c.Get(ctx, key, existing); err != nil {
			return err
		}
		return checkExistingObject(c, owner, existing)
	}

	logger.Info("Created object for component", "kind", kind, "name", key.Name, "namespace", key.Namespace)
	return nil
}

// checkExistingObject decides what to do with an object that already exists. Nothing, when it lists
// owner's UID, has no owner, or has an owner that is not a Component of owner's API group and name:
// the name is all the account selection needs. errStaleOwner when every owner it lists is a Component
// of owner's API group and name with another UID, so the caller requeues and the garbage collector
// removes the object in the meantime.
func checkExistingObject(c client.Client, owner client.Object, existing client.Object) error {
	refs := existing.GetOwnerReferences()
	if len(refs) == 0 {
		return nil
	}

	gvk, err := apiutil.GVKForObject(owner, c.Scheme())
	if err != nil {
		return err
	}

	for _, ref := range refs {
		if ref.UID == owner.GetUID() {
			return nil
		}
		gv, err := schema.ParseGroupVersion(ref.APIVersion)
		if err != nil {
			return err
		}
		if gv.Group != gvk.Group || ref.Kind != gvk.Kind || ref.Name != owner.GetName() {
			return nil
		}
	}

	return errStaleOwner
}
