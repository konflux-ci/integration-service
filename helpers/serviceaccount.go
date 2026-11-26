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

package helpers

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	imagerepositoryv1alpha1 "github.com/konflux-ci/image-controller/api/v1alpha1"
	tektonconsts "github.com/konflux-ci/integration-service/tekton/consts"
)

// ComponentIntegrationPipelineServiceAccountName returns the name of the per-component ServiceAccount
// that integration PipelineRuns on a component Snapshot run as (STONEINTG-1799). The name derives from
// the Component's metadata name, which is also the value of the Snapshot's component label that
// STONEINTG-1800 uses to select the account.
func ComponentIntegrationPipelineServiceAccountName(componentName string) string {
	return tektonconsts.ComponentIntegrationPipelineServiceAccountPrefix + componentName
}

// errStaleOwner reports that an object of the wanted name exists and every owner it lists is a
// Component of the owner's API group and name with another UID: the Component was deleted and
// recreated under the same name and the garbage collector has not removed the object yet. The caller
// requeues; the next pass creates a fresh object.
var errStaleOwner = errors.New("object is owned by a previous Component of the same name and is waiting for garbage collection")

// IsStaleOwnerError reports whether err wraps errStaleOwner.
func IsStaleOwnerError(err error) bool {
	return errors.Is(err, errStaleOwner)
}

// EnsureIntegrationPipelineServiceAccountExists makes sure the per-component integration pipeline
// ServiceAccount its-pipeline-<owner name> and the RoleBinding of the same name to the
// konflux-integration-runner ClusterRole exist in the owner's namespace and carry an owner reference to the
// owner (STONEINTG-1799). The owner is a Component of any kind. The reference is a plain owner
// reference, as build-service uses for build-pipeline-<component>, set when the object is created;
// garbage collection removes the object when the last owner goes. An object that already exists is
// never updated here: OpenShift's OwnerReferencesPermissionEnforcement admission plugin refuses a
// change to ownerReferences by a requester that may not delete the object, and integration-service is
// not granted delete. The push credential is linked to the account by LinkComponentPushSecrets, just
// before an integration PipelineRun that runs as the account is created (STONEINTG-1804).
// Returns an error that IsStaleOwnerError recognizes when the caller should requeue.
func EnsureIntegrationPipelineServiceAccountExists(ctx context.Context, adapterClient client.Client, logger IntegrationLogger, owner client.Object) error {
	name := ComponentIntegrationPipelineServiceAccountName(owner.GetName())
	namespace := owner.GetNamespace()

	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
	}
	if err := createObjectWithOwnerReferenceIfMissing(ctx, adapterClient, logger, owner, "ServiceAccount", sa); err != nil {
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
	if err := createObjectWithOwnerReferenceIfMissing(ctx, adapterClient, logger, owner, "RoleBinding", rb); err != nil {
		return fmt.Errorf("failed to ensure RoleBinding %s/%s: %w", namespace, name, err)
	}

	return nil
}

// LinkComponentPushSecrets adds the push secret of every ImageRepository in imageRepositories to the
// secrets list of its-pipeline-<component> (STONEINTG-1804), so that an integration PipelineRun
// running as that account can push to the component's image repository. The secret's name is read
// from status.credentials.push-secret. Together with a push secret, and in the same update, it adds
// components-namespace-pull and sharedSecretNames, the names the caller takes from the shared runner
// account (see SharedRunnerSecretNames); mergeSecretNames gives the order (STONEINTG-1800). No Secret
// is read and imagePullSecrets is left as it is. Nothing is unlinked: Tekton skips a listed Secret
// that no longer exists.
// It returns true when the account lists at least one such push secret after the call, and false
// with no error when no ImageRepository has a push secret name; nothing is added and the account is
// not read then. An account that does not exist is returned as the NotFound error of the read.
func LinkComponentPushSecrets(ctx context.Context, adapterClient client.Client, logger IntegrationLogger, namespace, componentName string, imageRepositories []imagerepositoryv1alpha1.ImageRepository, sharedSecretNames ...string) (bool, error) {
	var pushSecretNames []string
	for _, imageRepository := range imageRepositories {
		name := imageRepository.Status.Credentials.PushSecretName
		if name != "" && imageRepository.DeletionTimestamp.IsZero() {
			pushSecretNames = append(pushSecretNames, name)
		}
	}
	if len(pushSecretNames) == 0 {
		return false, nil
	}

	key := types.NamespacedName{Namespace: namespace, Name: ComponentIntegrationPipelineServiceAccountName(componentName)}
	sharedSecretNames = slices.Concat([]string{tektonconsts.DefaultIntegrationPipelineImagePullSecretName}, sharedSecretNames)
	var added []string
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		account := &corev1.ServiceAccount{}
		if err := adapterClient.Get(ctx, key, account); err != nil {
			return err
		}
		account.Secrets, added = mergeSecretNames(account.Secrets, pushSecretNames, sharedSecretNames)
		if len(added) == 0 {
			return nil
		}
		return adapterClient.Update(ctx, account)
	})
	if err != nil {
		return false, fmt.Errorf("failed to link secrets to ServiceAccount %s/%s: %w", key.Namespace, key.Name, err)
	}
	if len(added) > 0 {
		logger.Info("Linked secrets to the integration pipeline ServiceAccount",
			"serviceAccount", key.Name, "namespace", key.Namespace, "secrets", added)
	}

	return true, nil
}

// mergeSecretNames returns the secrets list of its-pipeline-<component> with the names that are
// missing from listed, and those names. A missing push secret name goes to the front and a missing
// shared name to the end, so one call never puts a shared name ahead of a push secret: when two
// listed logins match the image repository equally well, Tekton Chains signs with the one listed
// first, and that should be the push secret. An entry that is already listed keeps its place, also a
// push secret that is listed behind another name, and no name is listed twice.
func mergeSecretNames(listed []corev1.ObjectReference, pushSecretNames, sharedSecretNames []string) ([]corev1.ObjectReference, []string) {
	isListed := func(refs []corev1.ObjectReference, name string) bool {
		return slices.ContainsFunc(refs, func(ref corev1.ObjectReference) bool { return ref.Name == name })
	}

	var added []string
	var front []corev1.ObjectReference
	for _, name := range pushSecretNames {
		if !isListed(listed, name) && !isListed(front, name) {
			front = append(front, corev1.ObjectReference{Name: name})
			added = append(added, name)
		}
	}
	merged := slices.Concat(front, listed)
	for _, name := range sharedSecretNames {
		if name != "" && !isListed(merged, name) {
			merged = append(merged, corev1.ObjectReference{Name: name})
			added = append(added, name)
		}
	}

	return merged, added
}

// SharedRunnerSecretNames returns the names in the secrets list of runner, the shared account
// konflux-integration-runner, that its-pipeline-<component> lists too, so that a test running as the
// component's account has the registry logins users linked to the shared account. The runner's own
// Secrets are left out (see isAccountOwnSecret). imagePullSecrets is not read.
func SharedRunnerSecretNames(runner *corev1.ServiceAccount) []string {
	var names []string
	for _, ref := range runner.Secrets {
		if ref.Name != "" && !isAccountOwnSecret(runner.Name, ref.Name) {
			names = append(names, ref.Name)
		}
	}
	return names
}

// isAccountOwnSecret reports whether secretName is a Secret the platform made for the ServiceAccount
// accountName itself: its token (<account>-token-<suffix>) or, on OpenShift, its login to the internal
// registry (<account>-dockercfg-<suffix>). Such a Secret is the identity of that one account, so it
// is never listed on another account.
func isAccountOwnSecret(accountName, secretName string) bool {
	return strings.HasPrefix(secretName, accountName+"-token-") || strings.HasPrefix(secretName, accountName+"-dockercfg-")
}

// createObjectWithOwnerReferenceIfMissing creates obj with an owner reference to owner or, when an
// object of that name already exists, leaves it as it is (see checkForStaleOwner). kind is only used
// in log lines.
func createObjectWithOwnerReferenceIfMissing(ctx context.Context, adapterClient client.Client, logger IntegrationLogger, owner client.Object, kind string, obj client.Object) error {
	key := types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}
	existing := obj.DeepCopyObject().(client.Object)

	err := adapterClient.Get(ctx, key, existing)
	if err == nil {
		return checkForStaleOwner(adapterClient, owner, existing)
	}
	if !apierrors.IsNotFound(err) {
		return err
	}

	if err := controllerutil.SetOwnerReference(owner, obj, adapterClient.Scheme()); err != nil {
		return err
	}
	if err := adapterClient.Create(ctx, obj); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return err
		}
		// Created concurrently or not yet in the cache: read it and leave it as it is.
		if err := adapterClient.Get(ctx, key, existing); err != nil {
			return err
		}
		return checkForStaleOwner(adapterClient, owner, existing)
	}

	logger.Info("Created object for component", "kind", kind, "name", key.Name, "namespace", key.Namespace)
	return nil
}

// checkForStaleOwner looks at the owners of an object that already exists. It returns errStaleOwner
// when every owner it lists is a Component of owner's API group and name with another UID, so the
// caller requeues and the garbage collector removes the object in the meantime. It returns nil when
// the object lists owner's UID, has no owner, or has an owner that is not a Component of owner's API
// group and name: the object is left as it is, because the name is all the account selection needs.
func checkForStaleOwner(adapterClient client.Client, owner client.Object, existing client.Object) error {
	refs := existing.GetOwnerReferences()
	if len(refs) == 0 {
		return nil
	}

	gvk, err := apiutil.GVKForObject(owner, adapterClient.Scheme())
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
