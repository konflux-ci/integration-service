/*
Copyright 2023-2026 Red Hat, Inc.

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

// ComponentSource describes the Component source
type ComponentSource struct {
	// Git repository URL for the component.
	// Required.
	// +required
	// +kubebuilder:validation:MaxLength=2048
	// +kubebuilder:validation:XValidation:rule="self.startsWith('https://')",message="git repository URL can't be empty and must use https"
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="git repository URL cannot be changed"
	GitURL string `json:"url"`

	// Dockerfile path for all versions, unless explicitly specified for a version.
	// Used only when sending a PR with build pipeline configuration was requested via 'spec.actions.create-pipeline-configuration-pr'.
	// Default: "Dockerfile"
	// Optional.
	DockerfilePath string `json:"dockerfilePath,omitempty"`

	// List of all versions for this component.
	// Optional.
	Versions []ComponentVersion `json:"versions,omitempty"`
}

type ComponentActions struct {
	// Send a PR with build pipeline configuration proposal for Component version(s).
	// If not set, version onboarding will be done without pipeline configuration PR.
	// Could be used after onboarding to create / renew build pipeline definition.
	// Optional.
	CreateConfiguration ComponentCreatePipelineConfiguration `json:"create-pipeline-configuration-pr,omitempty"`

	// Specify name of component version to restart the push build for.
	// Can be specified together with 'trigger-push-builds' and any duplicates will be removed.
	// Optional.
	TriggerBuild string `json:"trigger-push-build,omitempty"`

	// Specify names of component versions to restart the push build for.
	// Can be specified together with 'trigger-push-build' and any duplicates will be removed.
	// Optional.
	TriggerBuilds []string `json:"trigger-push-builds,omitempty"`
}

type ComponentCreatePipelineConfiguration struct {
	// When specified it will send a PR with build pipeline configuration proposal for all Component versions.
	// Has precedence over 'version' and 'versions'.
	// Optional.
	AllVersions bool `json:"all-versions,omitempty"`

	// When specified it will send a PR with build pipeline configuration proposal for the Component version.
	// Can be specified together with 'versions' and any duplicates will be removed.
	// Optional.
	Version string `json:"version,omitempty"`

	// When specified it will send a PR with build pipeline configuration proposal for Component versions.
	// Can be specified together with 'version' and any duplicates will be removed.
	// Optional.
	Versions []string `json:"versions,omitempty"`
}

type ComponentBuildPipeline struct {
	// Pipeline used for pull and push pipeline runs.
	// Can specify just one of: pipelinespec-from-bundle, pipelineref-by-name, pipelineref-by-git-resolver.
	// Optional.
	// +optional
	// +nullable
	PullAndPush *PipelineDefinition `json:"pull-and-push,omitempty"`

	// Pipeline used for pull pipeline run.
	// Can specify just one of: pipelinespec-from-bundle, pipelineref-by-name, pipelineref-by-git-resolver.
	// Optional.
	// +optional
	// +nullable
	Pull *PipelineDefinition `json:"pull,omitempty"`

	// Pipeline used for push pipeline run.
	// Can specify just one of: pipelinespec-from-bundle, pipelineref-by-name, pipelineref-by-git-resolver.
	// Optional.
	// +optional
	// +nullable
	Push *PipelineDefinition `json:"push,omitempty"`
}

type PipelineDefinition struct {
	// Will be used to fill out PipelineRef in pipeline runs to user specific pipeline via git resolver,
	// specifying repository with a pipeline definition.
	// Optional.
	// +optional
	// +nullable
	PipelineRefGit *PipelineRefGit `json:"pipelineref-by-git-resolver,omitempty"`

	// Will be used to fill out PipelineRef in pipeline runs to user specific pipeline.
	// Such pipeline definition has to be in .tekton.
	// Optional.
	// +optional
	PipelineRefName string `json:"pipelineref-by-name,omitempty"`

	// Will be used to fetch bundle and fill out PipelineSpec in pipeline runs.
	// Pipeline name is based on build-pipeline-config CM in build-service NS.
	// When 'latest' bundle is specified, bundle image will be used from CM.
	// When bundle is specified to specific image bundle, then that one will be used
	// and pipeline name will be used to fetch pipeline from that bundle.
	// Optional.
	// +optional
	// +nullable
	PipelineSpecFromBundle *PipelineSpecFromBundle `json:"pipelinespec-from-bundle,omitempty"`
}

type PipelineSpecFromBundle struct {
	// Bundle image reference. Use 'latest' to get bundle from build-pipeline-config CM,
	// or specify a specific bundle image.
	// Required.
	// +required
	// +kubebuilder:validation:MinLength=1
	Bundle string `json:"bundle"`

	// Pipeline name to fetch from the bundle, or from build-pipeline-config CM.
	// Required.
	// +required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

type PipelineRefGit struct {
	// Path to the pipeline definition file within the repository.
	// Example: pipeline/push.yaml
	// Required.
	// +required
	// +kubebuilder:validation:MinLength=1
	PathInRepo string `json:"pathInRepo"`

	// Git revision (branch, tag, or commit) to use.
	// Example: main
	// Required.
	// +required
	// +kubebuilder:validation:MinLength=1
	Revision string `json:"revision"`

	// Git repository URL containing the pipeline definition.
	// Example: https://github.com/custom-pipelines/pipelines.git
	// Required.
	// +required
	// +kubebuilder:validation:MinLength=1
	Url string `json:"url"`
}

type ComponentVersion struct {
	// Used only when sending a PR with build pipeline configuration was requested via 'spec.actions.create-pipeline-configuration-pr'.
	// Pipeline used for the version; when omitted, the default pipeline will be used from 'spec.default-build-pipeline'.
	// Optional.
	// +optional
	// +nullable
	BuildPipeline *ComponentBuildPipeline `json:"build-pipeline,omitempty"`

	// Context directory for the version.
	// Used only when sending a PR with build pipeline configuration was requested via 'spec.actions.create-pipeline-configuration-pr'.
	// Default: "" (empty string, root of repository).
	// Optional.
	Context string `json:"context,omitempty"`

	// Dockerfile path for the version.
	// Used only when sending a PR with build pipeline configuration was requested via 'spec.actions.create-pipeline-configuration-pr'.
	// Default: "Dockerfile".
	// Optional.
	DockerfilePath string `json:"dockerfilePath,omitempty"`

	// User defined name for the version.
	// After sanitization (lower case, removing spaces, etc) all version names must be unique.
	// Required.
	// +required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Git branch to use for the version.
	// Required.
	// +required
	// +kubebuilder:validation:MinLength=1
	Revision string `json:"revision"`

	// When 'true' it will disable builds for a revision in the version.
	// Default: false.
	// Optional.
	// +optional
	SkipBuilds bool `json:"skip-builds"`
}

type RepositorySettings struct {
	// When specified, will set value of `comment_strategy` in the Repository CR
	// Optional.
	CommentStrategy string `json:"comment-strategy,omitempty"`

	// When specified, will add values to `github_app_token_scope_repos` in the Repository CR
	// Optional.
	GithubAppTokenScopeRepos []string `json:"github-app-token-scope-repos,omitempty"`
}

// ComponentSpec defines the desired state of Component
type ComponentSpec struct {
	// Source describes the Component source.
	// Required.
	// +required
	Source ComponentSource `json:"source"`

	// The container image repository to use for this component (without tag).
	// Either will be set by Image Repository, or explicitly specified with custom repo.
	// All versions of this component will use this single image repository.
	// Example: quay.io/org/tenant/component
	// Optional.
	// +optional
	ContainerImage string `json:"containerImage,omitempty"`

	// Specific actions that will be processed by the controller and then removed from 'spec.actions'.
	// Used for triggering builds or creating pipeline configuration PRs.
	// Optional.
	// +optional
	Actions ComponentActions `json:"actions,omitempty"`

	// When 'true', during offboarding a cleaning PR won't be created.
	// Default: false.
	// Optional.
	// +optional
	SkipOffboardingPr bool `json:"skip-offboarding-pr,omitempty"`

	// Used for setting additional settings for the Repository CR.
	// Optional.
	// +optional
	RepositorySettings RepositorySettings `json:"repository-settings,omitempty"`

	// Used only when sending a PR with build pipeline configuration was requested via 'spec.actions.create-pipeline-configuration-pr'.
	// Pipeline used for all versions, unless explicitly specified for a specific version.
	// When omitted it has to be specified in all versions.
	// Optional.
	// +optional
	// +nullable
	DefaultBuildPipeline *ComponentBuildPipeline `json:"default-build-pipeline,omitempty"`
}

// ComponentStatus defines the observed state of Component
type ComponentStatus struct {
	// Identifies which additional settings are used for the Repository CR.
	RepositorySettings RepositorySettings `json:"repository-settings,omitempty"`

	// General error message, not specific to any version (version-specific errors are in versions[].message).
	// Example: "Spec.ContainerImage is not set" or "GitHub App is not installed".
	Message string `json:"message,omitempty"`

	// Name of Repository CR for the component.
	PacRepository string `json:"pac-repository,omitempty"`

	// All versions which were processed by onboarding.
	// When version is removed from the spec, offboarding will remove it from the status.
	Versions []ComponentVersionStatus `json:"versions,omitempty"`
}

type ComponentVersionStatus struct {
	// Link with onboarding PR if requested by 'spec.actions.create-pipeline-configuration-pr'.
	// Only present if onboarding was successful.
	// Example: https://github.com/user/repo/pull/1
	ConfigurationMergeURL string `json:"configuration-merge-url,omitempty"`

	// Version specific error message.
	// Example: "pipeline for this version doesn't exist"
	Message string `json:"message,omitempty"`

	// Name for the version.
	Name string `json:"name,omitempty"`

	// Onboarding status will be either 'succeeded' or 'failed' ('disabled' won't be there because we will just remove specific version section).
	OnboardingStatus string `json:"onboarding-status,omitempty"`

	// Timestamp for when onboarding happened.
	// Only present if onboarding was successful.
	// Example: "29 May 2024 15:11:16 UTC"
	OnboardingTime string `json:"onboarding-time,omitempty"`

	// Git revision (branch) for the version.
	Revision string `json:"revision,omitempty"`

	// Identifies that builds for the revision in the version are disabled.
	SkipBuilds bool `json:"skip-builds,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Component is the Schema for the components API.
// +kubebuilder:resource:path=components,shortName=cmp;comp
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:printcolumn:name="URL",type="string",JSONPath=".spec.source.url"
// +kubebuilder:printcolumn:name="Repository",type="string",JSONPath=".status.pac-repository"
// +kubebuilder:printcolumn:name="Message",type="string",JSONPath=".status.message",priority=1
type Component struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ComponentSpec   `json:"spec"`
	Status ComponentStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ComponentList contains a list of Component
type ComponentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Component `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Component{}, &ComponentList{})
}
