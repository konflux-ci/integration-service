/*
Copyright 2023 Red Hat, Inc.

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

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// DeploymentTargetSpec defines the desired state of DeploymentTarget
type DeploymentTargetSpec struct {
	DeploymentTargetClassName DeploymentTargetClassName `json:"deploymentTargetClassName"`

	KubernetesClusterCredentials DeploymentTargetKubernetesClusterCredentials `json:"kubernetesCredentials"`

	ClaimRef string `json:"claimRef,omitempty"`
}

// DeploymentTargetKubernetesClusterCredentials defines the K8s cluster credentials for the DeploymentTarget.
type DeploymentTargetKubernetesClusterCredentials struct {
	DefaultNamespace string `json:"defaultNamespace"`

	// APIURL is a reference to a cluster API url.
	APIURL string `json:"apiURL"`

	// ClusterCredentialsSecret is a reference to the name of k8s Secret that contains a kubeconfig.
	ClusterCredentialsSecret string `json:"clusterCredentialsSecret"`

	// Indicates that a Service should not check the TLS certificate when connecting to this target.
	AllowInsecureSkipTLSVerify bool `json:"allowInsecureSkipTLSVerify"`
}

// DeploymentTargetStatus defines the observed state of DeploymentTarget
type DeploymentTargetStatus struct {
	Phase DeploymentTargetPhase `json:"phase,omitempty"`
}

type DeploymentTargetPhase string

const (
	// DT is not yet available for binding.
	DeploymentTargetPhase_Pending DeploymentTargetPhase = "Pending"

	// DT waits for a Claim to be bound to.
	DeploymentTargetPhase_Available DeploymentTargetPhase = "Available"

	// The DT was bounded to a DTC.
	DeploymentTargetPhase_Bound DeploymentTargetPhase = "Bound"

	// The DT was previously bound to a DTC which got deleted. external resources were not freed.
	DeploymentTargetPhase_Released DeploymentTargetPhase = "Released"

	// DT was released from its claim, but there was a failure during the release of external resources.
	DeploymentTargetPhase_Failed DeploymentTargetPhase = "Failed"
)

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// DeploymentTarget is the Schema for the deploymenttargets API.
// A deployment target, usually a K8s api endpoint. The credentials for connecting
// to the target will be stored in a secret which will be referenced in the clusterCredentialsSecret field
type DeploymentTarget struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DeploymentTargetSpec   `json:"spec,omitempty"`
	Status DeploymentTargetStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// DeploymentTargetList contains a list of DeploymentTarget
type DeploymentTargetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DeploymentTarget `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DeploymentTarget{}, &DeploymentTargetList{})
}
