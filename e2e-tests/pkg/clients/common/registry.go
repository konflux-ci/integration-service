package common

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/konflux-ci/integration-service/e2e-tests/pkg/constants"
	corev1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// commonSecretLabels mark a secret that build-service links to per-component build
// ServiceAccounts. The old label is still read by build-service, the new one is preferred.
var commonSecretLabels = map[string]string{
	"build.appstudio.openshift.io/common-secret": "true",
	"build.konflux-ci.dev/common-secret":         "true",
}

// EnsureRedHatRegistryPullSecret creates (or refreshes) the dockerconfigjson secret used to pull
// entitlement-gated images such as registry.redhat.io in the given namespace. When the
// REDHAT_REGISTRY_DOCKERCONFIGJSON env var is not set, it does nothing, so clusters that do not
// need entitlement credentials are unaffected.
func (s *SuiteController) EnsureRedHatRegistryPullSecret(namespace string) error {
	dockerConfigJSON := strings.TrimSpace(os.Getenv(constants.RedHatRegistryDockerConfigJSONEnv))
	if dockerConfigJSON == "" {
		return nil
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.RedHatRegistryPullSecretName,
			Namespace: namespace,
			Labels:    commonSecretLabels,
		},
		Type: corev1.SecretTypeDockerConfigJson,
		Data: map[string][]byte{
			corev1.DockerConfigJsonKey: []byte(dockerConfigJSON),
		},
	}

	secrets := s.KubeInterface().CoreV1().Secrets(namespace)
	if _, err := secrets.Create(context.Background(), secret, metav1.CreateOptions{}); err == nil {
		return nil
	} else if !k8sErrors.IsAlreadyExists(err) {
		return fmt.Errorf("error creating secret %s in namespace %s: %w", secret.Name, namespace, err)
	}

	existing, err := secrets.Get(context.Background(), secret.Name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("error getting secret %s in namespace %s: %w", secret.Name, namespace, err)
	}
	existing.Type = secret.Type
	existing.Data = secret.Data
	existing.Labels = mergeSecretLabels(existing.Labels, commonSecretLabels)
	if _, err := secrets.Update(context.Background(), existing, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("error updating secret %s in namespace %s: %w", secret.Name, namespace, err)
	}
	return nil
}

// EnsureBuildPipelineServiceAccount makes sure the per-component build ServiceAccount
// (build-pipeline-<component>) exists before the Component is created, so a build PipelineRun
// cannot be rejected with `serviceaccounts "build-pipeline-<component>" not found` while
// build-service is still reconciling the Component and creating that ServiceAccount.
// build-service creates the same ServiceAccount while reconciling the Component and keeps existing
// imagePullSecrets, so pre-creating it before the Component is safe. The registry pull secret is
// attached as an imagePullSecret only when REDHAT_REGISTRY_DOCKERCONFIGJSON is configured, because
// the secret itself is created only in that case.
func (s *SuiteController) EnsureBuildPipelineServiceAccount(namespace, componentName string) error {
	saName := constants.BuildPipelineServiceAccountPrefix + componentName
	serviceAccounts := s.KubeInterface().CoreV1().ServiceAccounts(namespace)

	var pullSecretRefs []corev1.LocalObjectReference
	if strings.TrimSpace(os.Getenv(constants.RedHatRegistryDockerConfigJSONEnv)) != "" {
		pullSecretRefs = []corev1.LocalObjectReference{{Name: constants.RedHatRegistryPullSecretName}}
	}

	serviceAccount, err := serviceAccounts.Get(context.Background(), saName, metav1.GetOptions{})
	if err != nil {
		if !k8sErrors.IsNotFound(err) {
			return fmt.Errorf("error getting service account %s in namespace %s: %w", saName, namespace, err)
		}
		serviceAccount = &corev1.ServiceAccount{
			ObjectMeta:       metav1.ObjectMeta{Name: saName, Namespace: namespace},
			ImagePullSecrets: pullSecretRefs,
		}
		if _, err := serviceAccounts.Create(context.Background(), serviceAccount, metav1.CreateOptions{}); err != nil && !k8sErrors.IsAlreadyExists(err) {
			return fmt.Errorf("error creating service account %s in namespace %s: %w", saName, namespace, err)
		}
		// Re-read the object so the pull-secret reconciliation below runs against the persisted
		// version, no matter whether this call or build-service created the ServiceAccount.
		serviceAccount, err = serviceAccounts.Get(context.Background(), saName, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("error getting service account %s in namespace %s: %w", saName, namespace, err)
		}
	}

	changed := false
	for _, pullSecretRef := range pullSecretRefs {
		linked := false
		for _, existing := range serviceAccount.ImagePullSecrets {
			if existing.Name == pullSecretRef.Name {
				linked = true
				break
			}
		}
		if !linked {
			serviceAccount.ImagePullSecrets = append(serviceAccount.ImagePullSecrets, pullSecretRef)
			changed = true
		}
	}
	if changed {
		if _, err := serviceAccounts.Update(context.Background(), serviceAccount, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("error updating service account %s in namespace %s: %w", saName, namespace, err)
		}
	}

	// Verify the ServiceAccount is readable before the Component - and with it the first build
	// PipelineRun - is created; a missing ServiceAccount fails the build's TaskRun pod creation.
	if _, err := serviceAccounts.Get(context.Background(), saName, metav1.GetOptions{}); err != nil {
		return fmt.Errorf("service account %s in namespace %s is not available after creation: %w", saName, namespace, err)
	}
	return nil
}

func mergeSecretLabels(existing, desired map[string]string) map[string]string {
	if existing == nil {
		existing = map[string]string{}
	}
	for key, value := range desired {
		existing[key] = value
	}
	return existing
}
