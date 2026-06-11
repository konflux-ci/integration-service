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

// ImageRepositorySpec defines the desired state of ImageRepository
type ImageRepositorySpec struct {
	// Requested image repository configuration.
	// +optional
	Image ImageParameters `json:"image,omitempty"`

	// Credentials management.
	// +optional
	Credentials *ImageCredentials `json:"credentials,omitempty"`

	// Notifications defines configuration for image repository notifications.
	// +optional
	Notifications []Notifications `json:"notifications,omitempty"`
}

// ImageParameters describes requested image repository configuration.
type ImageParameters struct {
	// Name of the image within configured Quay organization.
	// If ommited, then defaults to "cr-namespace/cr-name".
	// This field cannot be changed after the resource creation.
	// +optional
	// +kubebuilder:validation:Pattern="^[a-z0-9][.a-z0-9_-]*(/[a-z0-9][.a-z0-9_-]*)*$"
	Name string `json:"name,omitempty"`

	// Visibility defines whether the image is publicly visible.
	// Allowed values are public and private.
	// "public" is the default.
	// +optional
	Visibility ImageVisibility `json:"visibility,omitempty"`
}

// +kubebuilder:validation:Enum=public;private
type ImageVisibility string

const (
	ImageVisibilityPublic  ImageVisibility = "public"
	ImageVisibilityPrivate ImageVisibility = "private"
)

type ImageCredentials struct {
	// RegenerateToken defines a request to refresh image accessing credentials.
	// Refreshes both, push and pull tokens.
	// The field gets cleared after the refresh.
	RegenerateToken *bool `json:"regenerate-token,omitempty"`
	// VerifyLinking defines a request to verify and fix
	// secret linking in pipeline service account.
	// The field gets cleared after fixing.
	VerifyLinking *bool `json:"verify-linking,omitempty"`
}

type Notifications struct {
	Title string `json:"title,omitempty"`
	// +kubebuilder:validation:Enum=repo_push
	Event NotificationEvent `json:"event,omitempty"`
	// +kubebuilder:validation:Enum=email;webhook
	Method NotificationMethod `json:"method,omitempty"`
	Config NotificationConfig `json:"config,omitempty"`
}

type NotificationEvent string

const (
	NotificationEventRepoPush NotificationEvent = "repo_push"
)

type NotificationMethod string

const (
	NotificationMethodEmail   NotificationMethod = "email"
	NotificationMethodWebhook NotificationMethod = "webhook"
)

type NotificationConfig struct {
	// Email is the email address to send notifications to.
	// +optional
	Email string `json:"email,omitempty"`
	// Webhook is the URL to send notifications to.
	// +optional
	Url string `json:"url,omitempty"`
}

// ImageRepositoryStatus defines the observed state of ImageRepository
type ImageRepositoryStatus struct {
	// State shows if image repository could be used.
	// "ready" means repository was created and usable,
	// "failed" means that the image repository creation request failed.
	State ImageRepositoryState `json:"state,omitempty"`

	// Message shows error information for the request.
	// It could contain non critical error, like failed to change image visibility,
	// while the state is ready and image resitory could be used.
	Message string `json:"message,omitempty"`

	// Image describes actual state of the image repository.
	Image ImageStatus `json:"image,omitempty"`

	// Credentials contain information related to image repository credentials.
	Credentials CredentialsStatus `json:"credentials,omitempty"`

	// Notifications shows the status of the notifications configuration.
	// +optional
	Notifications []NotificationStatus `json:"notifications,omitempty"`
}

type ImageRepositoryState string

const (
	ImageRepositoryStateReady  ImageRepositoryState = "ready"
	ImageRepositoryStateFailed ImageRepositoryState = "failed"
)

// ImageStatus shows actual generated image repository parameters.
type ImageStatus struct {
	// URL is the full image repository url to push into / pull from.
	URL string `json:"url,omitempty"`

	// Visibility shows actual generated image repository visibility.
	// +kubebuilder:validation:Enum=public;private
	Visibility ImageVisibility `json:"visibility,omitempty"`
}

// CredentialsStatus shows information about generated image repository credentials.
type CredentialsStatus struct {
	// GenerationTime shows timestamp when the current credentials were generated.
	GenerationTimestamp *metav1.Time `json:"generationTimestamp,omitempty"`

	// PushSecretName holds name of the dockerconfig secret with credentials to push (and pull) into the generated repository.
	PushSecretName string `json:"push-secret,omitempty"`

	// PullSecretName is present only if ImageRepository has labels that connect it to Application and Component.
	// Holds name of the dockerconfig secret with credentials to pull only from the generated repository.
	// The secret might not be present in the same namespace as ImageRepository, but created in other environments.
	PullSecretName string `json:"pull-secret,omitempty"`

	// PushRobotAccountName holds name of the quay robot account with write (push and pull) permissions into the generated repository.
	PushRobotAccountName string `json:"push-robot-account,omitempty"`

	// PullRobotAccountName is present only if ImageRepository has labels that connect it to Application and Component.
	// Holds name of the quay robot account with real (pull only) permissions from the generated repository.
	PullRobotAccountName string `json:"pull-robot-account,omitempty"`
}

// NotificationStatus shows the status of the notification configuration.
type NotificationStatus struct {
	Title string `json:"title,omitempty"`
	UUID  string `json:"uuid,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// ImageRepository is the Schema for the imagerepositories API
// +kubebuilder:printcolumn:name="Image",type="string",JSONPath=".status.image.url"
// +kubebuilder:printcolumn:name="Visibility",type="string",JSONPath=".status.image.visibility"
type ImageRepository struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ImageRepositorySpec   `json:"spec,omitempty"`
	Status ImageRepositoryStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ImageRepositoryList contains a list of ImageRepository
type ImageRepositoryList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ImageRepository `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ImageRepository{}, &ImageRepositoryList{})
}
