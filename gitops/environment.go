/*
Copyright 2023.

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

package gitops

import (
	"reflect"

	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/api/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type CopiedEnvironment struct {
	applicationapiv1alpha1.Environment
}

func (r *CopiedEnvironment) AsEnvironment() *applicationapiv1alpha1.Environment {
	return &r.Environment
}

// NewCopyOfExistingEnvironment gets the existing environment from current namespace and makes copy of it with the DeploymentTargetClaimName as Target
// new name is generated consisting of existing environment name and integrationTestScenario name
func NewCopyOfExistingEnvironment(existingEnvironment *applicationapiv1alpha1.Environment, namespace string, integrationTestScenario *v1beta1.IntegrationTestScenario, deploymentTargetClaimName string) *CopiedEnvironment {
	copiedEnvConfiguration := applicationapiv1alpha1.EnvironmentConfiguration{}
	copiedEnvConfiguration = *existingEnvironment.Spec.Configuration.DeepCopy()

	if !reflect.ValueOf(integrationTestScenario.Spec.Environment.Configuration).IsZero() {
		copiedEnvConfigFromIntTestScenario := *integrationTestScenario.Spec.Environment.Configuration.DeepCopy()
		// if existing environment does not contain EnvVars, copy ones from IntegrationTestScenario
		if existingEnvironment.Spec.Configuration.Env == nil {
			copiedEnvConfiguration.Env = copiedEnvConfigFromIntTestScenario.Env
		} else if len(copiedEnvConfigFromIntTestScenario.Env) != 0 {
			for intEnvVars := range copiedEnvConfigFromIntTestScenario.Env {
				envVarFound := false
				for existingEnvVar := range copiedEnvConfiguration.Env {
					// envVar names are matching? overwrite existing environment with one from ITS
					if copiedEnvConfigFromIntTestScenario.Env[intEnvVars].Name == copiedEnvConfiguration.Env[existingEnvVar].Name {
						copiedEnvConfiguration.Env[existingEnvVar].Value = copiedEnvConfigFromIntTestScenario.Env[intEnvVars].Value
						envVarFound = true
					}
				}
				if !envVarFound {
					// in case that EnvVar from IntegrationTestScenario is not matching any EnvVar from existingEnv, add this ITS EnvVar to copied Environment
					copiedEnvConfiguration.Env = append(copiedEnvConfiguration.Env, applicationapiv1alpha1.EnvVarPair{Name: copiedEnvConfigFromIntTestScenario.Env[intEnvVars].Name, Value: copiedEnvConfigFromIntTestScenario.Env[intEnvVars].Value})
				}
			}
		}
	}

	copiedEnvConfiguration.Target.DeploymentTargetClaim.ClaimName = deploymentTargetClaimName

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
		},
	}

	return &CopiedEnvironment{copyOfEnvironment}
}

// WithIntegrationLabels adds IntegrationTestScenario name as label to the copied environment.
func (e *CopiedEnvironment) WithIntegrationLabels(integrationTestScenario *v1beta1.IntegrationTestScenario) *CopiedEnvironment {
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

// NewDeploymentTargetClaim prepares a new DeploymentTargetClaim using the provided info
func NewDeploymentTargetClaim(namespace string, deploymentTargetClassName string) *applicationapiv1alpha1.DeploymentTargetClaim {
	dtc := &applicationapiv1alpha1.DeploymentTargetClaim{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "dtc" + "-",
			Namespace:    namespace,
		},
		Spec: applicationapiv1alpha1.DeploymentTargetClaimSpec{
			DeploymentTargetClassName: applicationapiv1alpha1.DeploymentTargetClassName(deploymentTargetClassName),
		},
	}

	return dtc
}
