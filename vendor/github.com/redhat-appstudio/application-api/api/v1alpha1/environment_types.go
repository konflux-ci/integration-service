/*
Copyright 2022-2023.

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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EnvironmentSpec defines the desired state of Environment
type EnvironmentSpec struct {

	// DEPRECATED: Type is whether the Environment is a POC or non-POC environment
	// - This field is deprecated, and should not be used.
	Type EnvironmentType `json:"type,omitempty"`

	// DisplayName is the user-visible, user-definable name for the environment (but not used for functional requirements)
	DisplayName string `json:"displayName"`

	// DeploymentStrategy is the promotion strategy for the Environment
	// See Environment API doc for details.
	DeploymentStrategy DeploymentStrategyType `json:"deploymentStrategy"`

	// ParentEnvironment references another Environment defined in the namespace: when automated promotion is enabled,
	// promotions to the parent environment will cause this environment to be promoted to.
	// See Environment API doc for details.
	ParentEnvironment string `json:"parentEnvironment,omitempty"`

	// Tags are a user-visisble, user-definable set of tags that can be applied to the environment
	Tags []string `json:"tags,omitempty"`

	// Configuration contains environment-specific details for Applications/Components that are deployed to
	// the Environment.
	Configuration EnvironmentConfiguration `json:"configuration,omitempty"`

	// UnstableConfigurationFields are experimental/prototype: the API has not been finalized here, and is subject to breaking changes.
	// See comment on UnstableEnvironmentConfiguration for details.
	UnstableConfigurationFields *UnstableEnvironmentConfiguration `json:"unstableConfigurationFields,omitempty"`
}

// DEPRECATED: EnvironmentType should no longer be used, and has no replacement.
// - It's original purpose was to indicate whether an environment is POC/Non-POC, but these data were ultimately not required.
type EnvironmentType string

const (
	// DEPRECATED: EnvironmentType_POC should no longer be used, and has no replacement.
	EnvironmentType_POC EnvironmentType = "POC"

	// DEPRECATED: EnvironmentType_NonPOC should no longer be used, and has no replacement.
	EnvironmentType_NonPOC EnvironmentType = "Non-POC"
)

// DeploymentStrategyType defines the available promotion/deployment strategies for an Environment
// See Environment API doc for details.
type DeploymentStrategyType string

const (
	// DeploymentStrategy_Manual: Promotions to an Environment with this strategy will occur due to explicit user intent
	DeploymentStrategy_Manual DeploymentStrategyType = "Manual"

	// DeploymentStrategy_AppStudioAutomated: Promotions to an Environment with this strategy will occur if a previous ("parent")
	// environment in the environment graph was successfully promoted to.
	// See Environment API doc for details.
	DeploymentStrategy_AppStudioAutomated DeploymentStrategyType = "AppStudioAutomated"
)

// UnstableEnvironmentConfiguration contains fields that are related to configuration of the target environment:
// - credentials for connecting to the cluster
//
// Note: as of this writing (Jul 2022), I expect the contents of this struct to undergo major changes, and the API should not be considered
// complete, or even a reflection of final desired state.
type UnstableEnvironmentConfiguration struct {
	// ClusterType indicates whether the target environment is Kubernetes or OpenShift
	ClusterType ConfigurationClusterType `json:"clusterType,omitempty"`

	// KubernetesClusterCredentials contains cluster credentials for a target Kubernetes/OpenShift cluster.
	KubernetesClusterCredentials `json:"kubernetesCredentials,omitempty"`
}

type ConfigurationClusterType string

const (
	// ConfigurationClusterType_Kubernetes indicates the target environment is generic Kubernetes
	ConfigurationClusterType_Kubernetes ConfigurationClusterType = "Kubernetes"

	// ConfigurationClusterType_OpenShift indicates the target environment is OpenShift
	ConfigurationClusterType_OpenShift ConfigurationClusterType = "OpenShift"
)

// KubernetesClusterCredentials contains cluster credentials for a target Kubernetes/OpenShift cluster.
//
// See this temporary URL for details on what values to provide for the APIURL and Secret:
// https://github.com/redhat-appstudio/managed-gitops/tree/main/examples/m6-demo#gitopsdeploymentmanagedenvironment-resource
type KubernetesClusterCredentials struct {

	// TargetNamespace is the default destination target on the cluster for deployments. This Namespace will be used
	// for any GitOps repository K8s resources where the `.metadata.Namespace` field is not specified.
	TargetNamespace string `json:"targetNamespace"`

	// APIURL is a reference to a cluster API url defined within the kube config file of the cluster credentials secret.
	APIURL string `json:"apiURL"`

	// IngressDomain is the cluster's ingress domain.
	// For example, in minikube it would be $(minikube ip).nip.io and in OCP it would look like apps.xyz.rhcloud.com.
	// If clusterType == "Kubernetes", ingressDomain is mandatory and is enforced by the webhook validation
	IngressDomain string `json:"ingressDomain,omitempty"`

	// ClusterCredentialsSecret is a reference to the name of k8s Secret, defined within the same namespace as the Environment resource,
	// that contains a kubeconfig.
	// The Secret must be of type 'managed-gitops.redhat.com/managed-environment'
	//
	// See this temporary URL for details:
	// https://github.com/redhat-appstudio/managed-gitops/tree/main/examples/m6-demo#gitopsdeploymentmanagedenvironment-resource
	ClusterCredentialsSecret string `json:"clusterCredentialsSecret"`

	// Indicates that ArgoCD/GitOps Service should not check the TLS certificate.
	AllowInsecureSkipTLSVerify bool `json:"allowInsecureSkipTLSVerify"`

	// Namespaces allows one to indicate which Namespaces the Secret's ServiceAccount has access to.
	//
	// Optional, defaults to empty. If empty, it is assumed that the ServiceAccount has access to all Namespaces.
	//
	// The ServiceAccount that GitOps Service/Argo CD uses to deploy may not have access to all of the Namespaces on a cluster.
	// If not specified, it is assumed that the Argo CD ServiceAccount has read/write at cluster-scope.
	// - If you are familiar with Argo CD: this field is equivalent to the field of the same name in the Argo CD Cluster Secret.
	Namespaces []string `json:"namespaces,omitempty"`

	// ClusterResources is used in conjuction with the Namespace field.
	// If the Namespaces field is non-empty, this field will be used to determine whether Argo CD should
	// attempt to manage cluster-scoped resources.
	// - If Namespaces field is empty, this field is not used.
	// - If you are familiar with Argo CD: this field is equivalent to the field of the same name in the Argo CD Cluster Secret.
	//
	// Optional, default to false.
	ClusterResources bool `json:"clusterResources,omitempty"`
}

// EnvironmentConfiguration contains Environment-specific configurations details, to be used when generating
// Component/Application GitOps repository resources.
type EnvironmentConfiguration struct {
	// Env is an array of standard environment vairables
	Env []EnvVarPair `json:"env,omitempty"`

	// Target is used to reference a DeploymentTargetClaim for a target Environment.
	// The Environment controller uses the referenced DeploymentTargetClaim to access its bounded
	// DeploymentTarget with cluster credential secret.
	Target EnvironmentTarget `json:"target,omitempty"`
}

// EnvironmentTarget provides the configuration for a deployment target.
type EnvironmentTarget struct {
	DeploymentTargetClaim DeploymentTargetClaimConfig `json:"deploymentTargetClaim"`
}

// DeploymentTargetClaimConfig specifies the DeploymentTargetClaim details for a given Environment.
type DeploymentTargetClaimConfig struct {
	ClaimName string `json:"claimName"`
}

// EnvironmentStatus defines the observed state of Environment
type EnvironmentStatus struct {
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Environment is the Schema for the environments API
// +kubebuilder:resource:path=environments,shortName=env
type Environment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EnvironmentSpec   `json:"spec,omitempty"`
	Status EnvironmentStatus `json:"status,omitempty"`
}

// GetDeploymentTargetClaimName returns the name of the DeploymentTargetClaim
// associated with this Environment
func (e *Environment) GetDeploymentTargetClaimName() string {
	return e.Spec.Configuration.Target.DeploymentTargetClaim.ClaimName
}

//+kubebuilder:object:root=true

// EnvironmentList contains a list of Environment
type EnvironmentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Environment `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Environment{}, &EnvironmentList{})
}
