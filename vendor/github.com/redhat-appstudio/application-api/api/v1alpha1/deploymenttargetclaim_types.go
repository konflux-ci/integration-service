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

// DeploymentTargetClaimSpec defines the desired state of DeploymentTargetClaim
type DeploymentTargetClaimSpec struct {
	DeploymentTargetClassName DeploymentTargetClassName `json:"deploymentTargetClassName"`
	TargetName                string                    `json:"targetName,omitempty"`
}

type DeploymentTargetClassName string

// DeploymentTargetClaimStatus defines the observed state of DeploymentTargetClaim
type DeploymentTargetClaimStatus struct {
	Phase DeploymentTargetClaimPhase `json:"phase,omitempty"`
}

type DeploymentTargetClaimPhase string

const (
	// DTC wait for the binding controller or user to bind it with a DT that satisfies it.
	DeploymentTargetClaimPhase_Pending DeploymentTargetClaimPhase = "Pending"

	// The DTC was bounded to a DT that satisfies it by the binding controller.
	DeploymentTargetClaimPhase_Bound DeploymentTargetClaimPhase = "Bound"

	// The DTC lost its bounded DT. The DT doesn’t exist anymore because it got deleted.
	DeploymentTargetClaimPhase_Lost DeploymentTargetClaimPhase = "Lost"
)

const (
	// Annotation to indicate that the binding controller completed the binding process.
	AnnBindCompleted string = "dt.appstudio.redhat.com/bind-complete"

	// Annotation to indicate that the binding controller bind the DTC to a DT.
	// In practice it means that the controller set the DTC.spec.Target to the value of DT.Name
	AnnBoundByController string = "dt.appstudio.redhat.com/bound-by-controller"

	// Annotation to indicate that a DT should be dynamically provisioned for the
	// DTC by the provisioner whose name appear in the value.
	AnnTargetProvisioner string = "provisioner.appstudio.redhat.com/dt-provisioner"

	// Annotation to indicate the name of the provisioner that provisioned the DT.
	AnnDynamicallyProvisioned string = "provisioner.appstudio.redhat.com/provisioned-by"

	// Finalizer added by the binding controller to handle the deletion of DeploymentTargetClaim.
	FinalizerBinder string = "binder.appstudio.redhat.com/finalizer"

	// AnnBinderValueTrue is a true value to annotations bind-complete and bound-by-controller.
	AnnBinderValueTrue string = "true"
)

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// DeploymentTargetClaim is the Schema for the deploymenttargetclaims API.
// It represents a request for a DeploymentTarget.
type DeploymentTargetClaim struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DeploymentTargetClaimSpec   `json:"spec,omitempty"`
	Status DeploymentTargetClaimStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// DeploymentTargetClaimList contains a list of DeploymentTargetClaim
type DeploymentTargetClaimList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DeploymentTargetClaim `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DeploymentTargetClaim{}, &DeploymentTargetClaimList{})
}
