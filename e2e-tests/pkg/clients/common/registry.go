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
// (build-pipeline-<component>) exists and carries the registry pull secret as an imagePullSecret.
// build-service creates the same ServiceAccount while reconciling the Component and keeps existing
// imagePullSecrets, so pre-creating it before the Component is safe and avoids a race with the
// build PipelineRun. When REDHAT_REGISTRY_DOCKERCONFIGJSON is not set, it does nothing.
func (s *SuiteController) EnsureBuildPipelineServiceAccount(namespace, componentName string) error {
	if strings.TrimSpace(os.Getenv(constants.RedHatRegistryDockerConfigJSONEnv)) == "" {
		return nil
	}

	saName := constants.BuildPipelineServiceAccountPrefix + componentName
	pullSecretRefs := []corev1.LocalObjectReference{{Name: constants.RedHatRegistryPullSecretName}}
	serviceAccounts := s.KubeInterface().CoreV1().ServiceAccounts(namespace)

	serviceAccount, err := serviceAccounts.Get(context.Background(), saName, metav1.GetOptions{})
	if err != nil {
		if !k8sErrors.IsNotFound(err) {
			return fmt.Errorf("error getting service account %s in namespace %s: %w", saName, namespace, err)
		}
		serviceAccount = &corev1.ServiceAccount{
			ObjectMeta:       metav1.ObjectMeta{Name: saName, Namespace: namespace},
			ImagePullSecrets: pullSecretRefs,
		}
		if _, err := serviceAccounts.Create(context.Background(), serviceAccount, metav1.CreateOptions{}); err == nil {
			return nil
		} else if !k8sErrors.IsAlreadyExists(err) {
			return fmt.Errorf("error creating service account %s in namespace %s: %w", saName, namespace, err)
		}
		// Another creator won the race; fall through and make sure the pull secret is linked.
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
	if !changed {
		return nil
	}
	if _, err := serviceAccounts.Update(context.Background(), serviceAccount, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("error updating service account %s in namespace %s: %w", saName, namespace, err)
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
