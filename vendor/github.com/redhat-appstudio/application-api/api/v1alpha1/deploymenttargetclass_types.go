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

// DeploymentTargetClassSpec defines the desired state of DeploymentTargetClass
type DeploymentTargetClassSpec struct {
	Provisioner Provisioner `json:"provisioner"`

	// Parameters are used to forward additional information to the provisioner.
	Parameters DeploymentTargetParameters `json:"parameters,omitempty"`

	// The reclaimPolicy field will tell the provisioner what to do with the DT
	// once its corresponding DTC is deleted, the values can be Retain or Delete.
	ReclaimPolicy ReclaimPolicy `json:"reclaimPolicy"`
}

type Provisioner string

const (
	Provisioner_Devsandbox Provisioner = "appstudio.redhat.com/devsandbox"
)

type ReclaimPolicy string

const (
	ReclaimPolicy_Delete ReclaimPolicy = "Delete"
	ReclaimPolicy_Retain ReclaimPolicy = "Retain"
)

// Parameters are used to forward additional information to the provisioner.
type DeploymentTargetParameters struct {
}

// DeploymentTargetClassStatus defines the observed state of DeploymentTargetClass
type DeploymentTargetClassStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:scope=Cluster

// DeploymentTargetClass is the Schema for the deploymenttargetclasses API.
// Defines DeploymentTarget properties that should be abstracted from the controller/user
// that creates a DTC and wants a DT to be provisioned automatically for it.
type DeploymentTargetClass struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DeploymentTargetClassSpec   `json:"spec,omitempty"`
	Status DeploymentTargetClassStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// DeploymentTargetClassList contains a list of DeploymentTargetClass
type DeploymentTargetClassList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DeploymentTargetClass `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DeploymentTargetClass{}, &DeploymentTargetClassList{})
}
