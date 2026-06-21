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

package scenario

import (
	"context"

	"github.com/konflux-ci/integration-service/api/v1beta2"
	h "github.com/konflux-ci/integration-service/helpers"
	"github.com/konflux-ci/integration-service/loader"
	tektonconsts "github.com/konflux-ci/integration-service/tekton/consts"
	"github.com/konflux-ci/operator-toolkit/controller"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Adapter holds the objects needed to reconcile an IntegrationTestScenario.
type Adapter struct {
	scenario *v1beta2.IntegrationTestScenario
	logger   h.IntegrationLogger
	loader   loader.ObjectLoader
	client   client.Client
	context  context.Context
}

// NewAdapter creates and returns an Adapter instance.
func NewAdapter(context context.Context, scenario *v1beta2.IntegrationTestScenario, logger h.IntegrationLogger, loader loader.ObjectLoader, client client.Client,
) *Adapter {
	return &Adapter{
		scenario: scenario,
		logger:   logger,
		loader:   loader,
		client:   client,
		context:  context,
	}
}

// EnsureIntegrationPipelineServiceAccountCreated ensures the integration pipeline service account,
// image pull secret, and role binding exist in the IntegrationTestScenario's namespace.
func (a *Adapter) EnsureIntegrationPipelineServiceAccountCreated() (controller.OperationResult, error) {
	namespace := a.scenario.Namespace
	saName := tektonconsts.DefaultIntegrationPipelineServiceAccount
	secretName := tektonconsts.DefaultIntegrationPipelineImagePullSecretName

	sa := &corev1.ServiceAccount{}
	err := a.client.Get(a.context, types.NamespacedName{Name: saName, Namespace: namespace}, sa)
	if err != nil {
		if !errors.IsNotFound(err) {
			a.logger.Error(err, "Failed to get ServiceAccount", "serviceAccount", saName)
			return controller.RequeueWithError(err)
		}
		sa = &corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      saName,
				Namespace: namespace,
			},
			ImagePullSecrets: []corev1.LocalObjectReference{
				{Name: secretName},
			},
		}
		if err := a.client.Create(a.context, sa); err != nil {
			if !errors.IsAlreadyExists(err) {
				a.logger.Error(err, "Failed to create ServiceAccount", "serviceAccount", saName)
				return controller.RequeueWithError(err)
			}
			// SA was created concurrently, re-fetch to get the current state
			if err := a.client.Get(a.context, types.NamespacedName{Name: saName, Namespace: namespace}, sa); err != nil {
				a.logger.Error(err, "Failed to re-fetch ServiceAccount after conflict", "serviceAccount", saName)
				return controller.RequeueWithError(err)
			}
		} else {
			a.logger.Info("Created ServiceAccount", "serviceAccount", saName)
		}
	}

	secret := &corev1.Secret{}
	err = a.client.Get(a.context, types.NamespacedName{Name: secretName, Namespace: namespace}, secret)
	if err != nil {
		if !errors.IsNotFound(err) {
			a.logger.Error(err, "Failed to get Secret", "secret", secretName)
			return controller.RequeueWithError(err)
		}
		secret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: namespace,
			},
			Type: corev1.SecretTypeDockerConfigJson,
			Data: map[string][]byte{
				corev1.DockerConfigJsonKey: []byte(`{"auths":{}}`),
			},
		}
		if err := a.client.Create(a.context, secret); err != nil {
			if !errors.IsAlreadyExists(err) {
				a.logger.Error(err, "Failed to create Secret", "secret", secretName)
				return controller.RequeueWithError(err)
			}
		} else {
			a.logger.Info("Created image pull secret", "secret", secretName)
		}
	}

	imagePullSecretLinked := false
	for _, ref := range sa.ImagePullSecrets {
		if ref.Name == secretName {
			imagePullSecretLinked = true
			break
		}
	}
	if !imagePullSecretLinked {
		updated := false
		err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
			if err := a.client.Get(a.context, types.NamespacedName{Name: saName, Namespace: namespace}, sa); err != nil {
				return err
			}
			for _, ref := range sa.ImagePullSecrets {
				if ref.Name == secretName {
					return nil
				}
			}
			sa.ImagePullSecrets = append(sa.ImagePullSecrets, corev1.LocalObjectReference{Name: secretName})
			updated = true
			return a.client.Update(a.context, sa)
		})
		if err != nil {
			a.logger.Error(err, "Failed to update ServiceAccount with secret link", "serviceAccount", saName, "secret", secretName)
			return controller.RequeueWithError(err)
		}
		if updated {
			a.logger.Info("Linked secret to ServiceAccount", "serviceAccount", saName, "secret", secretName)
		}
	}

	// The ClusterRole is provisioned by the Konflux operator (konflux-ci/konflux-ci)
	rbName := tektonconsts.DefaultIntegrationPipelineRoleBindingName
	rb := &rbacv1.RoleBinding{}
	err = a.client.Get(a.context, types.NamespacedName{Name: rbName, Namespace: namespace}, rb)
	if err != nil {
		if !errors.IsNotFound(err) {
			a.logger.Error(err, "Failed to get RoleBinding", "roleBinding", rbName)
			return controller.RequeueWithError(err)
		}
		rb = &rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      rbName,
				Namespace: namespace,
			},
			RoleRef: rbacv1.RoleRef{
				APIGroup: rbacv1.GroupName,
				Kind:     "ClusterRole",
				Name:     tektonconsts.DefaultIntegrationPipelineClusterRoleName,
			},
			Subjects: []rbacv1.Subject{
				{
					Kind:      rbacv1.ServiceAccountKind,
					Name:      saName,
					Namespace: namespace,
				},
			},
		}
		if err := a.client.Create(a.context, rb); err != nil {
			if !errors.IsAlreadyExists(err) {
				a.logger.Error(err, "Failed to create RoleBinding", "roleBinding", rbName)
				return controller.RequeueWithError(err)
			}
		} else {
			a.logger.Info("Created RoleBinding", "roleBinding", rbName)
		}
	}

	return controller.ContinueProcessing()
}
