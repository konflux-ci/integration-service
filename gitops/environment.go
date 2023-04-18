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
	"context"
	"fmt"

	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type CopiedEnvironment struct {
	applicationapiv1alpha1.Environment
}

func (r *CopiedEnvironment) AsEnvironment() *applicationapiv1alpha1.Environment {
	return &r.Environment
}

// NewCopyOfExistingEnvironment gets the existing environment from current namespace and makes copy of it with the DeploymentTargetClaimName as Target
// new name is generated consisting of existing environment name and integrationTestScenario name
func NewCopyOfExistingEnvironment(existingEnvironment *applicationapiv1alpha1.Environment, namespace string, integrationTestScenario *v1alpha1.IntegrationTestScenario, deploymentTargetClaimName string) *CopiedEnvironment {
	copiedEnvConfiguration := applicationapiv1alpha1.EnvironmentConfiguration{}
	copiedEnvConfiguration = *existingEnvironment.Spec.Configuration.DeepCopy()
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

// GetDeploymentTargetForEnvironment gets the DeploymentTarget associated with Environment, if the DeploymentTarget is not found, an error will be returned
func GetDeploymentTargetForEnvironment(adapterClient client.Client, ctx context.Context, environment *applicationapiv1alpha1.Environment) (*applicationapiv1alpha1.DeploymentTarget, error) {
	deploymentTargetClaim, err := GetDeploymentTargetClaimForEnvironment(adapterClient, ctx, environment)
	if err != nil {
		return nil, fmt.Errorf("failed to find deploymentTargetClaim defined in environment %s: %w", environment.Name, err)
	}

	deploymentTarget, err := GetDeploymentTargetForDeploymentTargetClaim(adapterClient, ctx, deploymentTargetClaim)
	if err != nil {
		return nil, fmt.Errorf("failed to find deploymentTarget defined in deploymentTargetClaim %s: %w", deploymentTargetClaim.Name, err)
	}

	return deploymentTarget, nil
}

// GetDeploymentTargetClaimForEnvironment try to find the DeploymentTargetClaim whose name is defined in Environment
// if not found, an error is returned
func GetDeploymentTargetClaimForEnvironment(adapterClient client.Client, ctx context.Context, environment *applicationapiv1alpha1.Environment) (*applicationapiv1alpha1.DeploymentTargetClaim, error) {
	if (environment.Spec.Configuration.Target != applicationapiv1alpha1.EnvironmentTarget{}) {
		dtcName := environment.Spec.Configuration.Target.DeploymentTargetClaim.ClaimName
		if dtcName != "" {
			deploymentTargetClaim := &applicationapiv1alpha1.DeploymentTargetClaim{}
			err := adapterClient.Get(ctx, types.NamespacedName{
				Namespace: environment.Namespace,
				Name:      dtcName,
			}, deploymentTargetClaim)

			if err != nil {
				return nil, err
			}

			return deploymentTargetClaim, nil
		}
	}

	return nil, fmt.Errorf("deploymentTargetClaim is not defined in .Spec.Configuration.Target.DeploymentTargetClaim.ClaimName for Environment: %s/%s", environment.Namespace, environment.Name)
}

// GetDeploymentTargetForDeploymentTargetClaim try to find the DeploymentTarget whose name is defined in DeploymentTargetClaim
// if not found, an error is returned
func GetDeploymentTargetForDeploymentTargetClaim(adapterClient client.Client, ctx context.Context, dtc *applicationapiv1alpha1.DeploymentTargetClaim) (*applicationapiv1alpha1.DeploymentTarget, error) {
	dtName := dtc.Spec.TargetName
	if dtName == "" {
		return nil, fmt.Errorf("deploymentTarget is not defined in .Spec.TargetName for deploymentTargetClaim: %s/%s", dtc.Namespace, dtc.Name)
	}

	deploymentTarget := &applicationapiv1alpha1.DeploymentTarget{}
	err := adapterClient.Get(ctx, types.NamespacedName{
		Namespace: dtc.Namespace,
		Name:      dtName,
	}, deploymentTarget)

	if err != nil {
		return nil, err
	}

	return deploymentTarget, nil
}
