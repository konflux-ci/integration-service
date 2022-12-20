package gitops

import (
	"github.com/google/uuid"
	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type CopiedEnvironment struct {
	applicationapiv1alpha1.Environment
}

// AsEnvironment casts the IntegrationPipelineRun to PipelineRun, so it can be used in the Kubernetes client.
func (r *CopiedEnvironment) AsEnvironment() *applicationapiv1alpha1.Environment {
	return &r.Environment
}

// NewCopyOfExistingEnvironment gets the existing environment from current namespace and makes copy of it
// new name is generated consisting of existing environment name and integrationTestScenario name
// targetNamespace gets name of integrationTestScenario and uuid
func NewCopyOfExistingEnvironment(existingEnvironment *applicationapiv1alpha1.Environment, namespace string, integrationTestScenario *v1alpha1.IntegrationTestScenario) *CopiedEnvironment {
	id := uuid.New()
	existingApiURL := existingEnvironment.Spec.UnstableConfigurationFields.KubernetesClusterCredentials.APIURL
	existingClusterCreds := existingEnvironment.Spec.UnstableConfigurationFields.KubernetesClusterCredentials.ClusterCredentialsSecret

	copiedEnvConfiguration := applicationapiv1alpha1.EnvironmentConfiguration{}
	copiedEnvConfiguration = *existingEnvironment.Spec.Configuration.DeepCopy()
	copiedIntTestScenario := *integrationTestScenario.Spec.Environment.Configuration.DeepCopy()
	// if existing environment does not contain EnvVars, copy ones from IntegrationTestScenario
	if existingEnvironment.Spec.Configuration.Env == nil {
		copiedEnvConfiguration.Env = copiedIntTestScenario.Env
	} else if len(copiedIntTestScenario.Env) != 0 {
		for intEnvVars := range copiedIntTestScenario.Env {
			envVarFound := false
			for existingEnvVar := range copiedEnvConfiguration.Env {
				// envVar names are matching? overwrite existing environment with one from ITS
				if copiedIntTestScenario.Env[intEnvVars].Name == copiedEnvConfiguration.Env[existingEnvVar].Name {
					copiedEnvConfiguration.Env[existingEnvVar].Value = copiedIntTestScenario.Env[intEnvVars].Value
					envVarFound = true
				}
			}
			if !envVarFound {
				// in case that EnvVar from IntegrationTestScenario is not matching any EnvVar from existingEnv, add this ITS EnvVar to copied Environment
				copiedEnvConfiguration.Env = append(copiedEnvConfiguration.Env, applicationapiv1alpha1.EnvVarPair{Name: copiedIntTestScenario.Env[intEnvVars].Name, Value: copiedIntTestScenario.Env[intEnvVars].Value})
			}
		}
	}

	copyOfEnvironment := applicationapiv1alpha1.Environment{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: existingEnvironment.Name + "-" + integrationTestScenario.Name + "-",
			Namespace:    namespace,
		},
		Spec: applicationapiv1alpha1.EnvironmentSpec{
			Type:               applicationapiv1alpha1.EnvironmentType_POC,
			DisplayName:        existingEnvironment.Name + "-" + integrationTestScenario.Name,
			Tags:               []string{"ephemeral"},
			DeploymentStrategy: applicationapiv1alpha1.DeploymentStrategy_Manual,
			Configuration:      copiedEnvConfiguration,
			UnstableConfigurationFields: &applicationapiv1alpha1.UnstableEnvironmentConfiguration{
				KubernetesClusterCredentials: applicationapiv1alpha1.KubernetesClusterCredentials{
					TargetNamespace:          integrationTestScenario.Name + "-" + id.String(),
					APIURL:                   existingApiURL,
					ClusterCredentialsSecret: existingClusterCreds,
				},
			},
		},
	}
	return &CopiedEnvironment{copyOfEnvironment}
}

// WithIntegrationLabels adds IntegrationTestScenario name as label to the copied environment.
func (e *CopiedEnvironment) WithIntegrationLabels(integrationTestScenario *v1alpha1.IntegrationTestScenario) *CopiedEnvironment {
	if e.ObjectMeta.Labels == nil {
		e.ObjectMeta.Labels = map[string]string{}
	}
	e.ObjectMeta.Labels[SnapshotTestScenarioLabel] = integrationTestScenario.Name

	return e

}

// WithSnapshot adds the name of snapshot as label to the copied environment.
func (e *CopiedEnvironment) WithSnapshot(snapshot *applicationapiv1alpha1.Snapshot) *CopiedEnvironment {

	if e.ObjectMeta.Labels == nil {
		e.ObjectMeta.Labels = map[string]string{}
	}
	e.ObjectMeta.Labels[SnapshotLabel] = snapshot.Name

	return e
}
