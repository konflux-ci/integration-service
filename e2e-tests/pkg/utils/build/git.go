package build

import (
	"fmt"

	v1 "k8s.io/api/core/v1"

	"github.com/konflux-ci/integration-service/e2e-tests/pkg/framework"
)

func createSCMBuildSecret(f *framework.Framework, secretName, host string, annotations map[string]string, token string) error {
	buildSecret := v1.Secret{}
	buildSecret.Name = secretName
	buildSecret.Labels = map[string]string{
		"appstudio.redhat.com/credentials": "scm",
		"appstudio.redhat.com/scm.host":    host,
	}
	if annotations != nil {
		buildSecret.Annotations = annotations
	}
	buildSecret.Type = "kubernetes.io/basic-auth"
	buildSecret.StringData = map[string]string{
		"password": token,
	}
	_, err := f.AsKubeAdmin.CommonController.CreateSecret(f.UserNamespace, &buildSecret)
	if err != nil {
		return fmt.Errorf("error creating build secret: %v", err)
	}
	return nil
}

// CreateGithubBuildSecret creates a Kubernetes secret for GitHub build credentials.
func CreateGithubBuildSecret(f *framework.Framework, secretName string, annotations map[string]string, token string) error {
	return createSCMBuildSecret(f, secretName, "github.com", annotations, token)
}

// CreateGitlabBuildSecret creates a Kubernetes secret for GitLab build credentials.
func CreateGitlabBuildSecret(f *framework.Framework, secretName string, annotations map[string]string, token string) error {
	return createSCMBuildSecret(f, secretName, "gitlab.com", annotations, token)
}

// CreateCodebergBuildSecret creates a Kubernetes secret for Codeberg/Forgejo build credentials.
func CreateCodebergBuildSecret(f *framework.Framework, secretName string, annotations map[string]string, token string) error {
	return createSCMBuildSecret(f, secretName, "codeberg.org", annotations, token)
}
