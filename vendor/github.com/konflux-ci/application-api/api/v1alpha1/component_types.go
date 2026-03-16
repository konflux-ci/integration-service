/*
Copyright 2021-2023 Red Hat, Inc.

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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// !!! Will be removed when we remove old model
type GitSource struct {
	// An HTTPS URL representing the git repository to create the component from.
	URL string `json:"url"`

	// Specify a branch/tag/commit id. If not specified, default is `main`/`master`.
	// Example: devel.
	// Optional.
	Revision string `json:"revision,omitempty"`

	// A relative path inside the git repo containing the component
	// Example: folderA/folderB/gitops.
	// Optional.
	Context string `json:"context,omitempty"`

	// If specified, the devfile at the URI will be used for the component. Can be a local path inside the repository, or an external URL.
	// Example: https://raw.githubusercontent.com/devfile-samples/devfile-sample-java-springboot-basic/main/devfile.yaml.
	// Optional.
	DevfileURL string `json:"devfileUrl,omitempty"`

	// If specified, the dockerfile at the URI will be used for the component. Can be a local path inside the repository, or an external URL.
	// Optional.
	DockerfileURL string `json:"dockerfileUrl,omitempty"`
}

// ComponentSource describes the Component source
type ComponentSource struct {
	ComponentSourceUnion `json:",inline"`
}

// +union
type ComponentSourceUnion struct {
	// Git Source for a Component.
	// Optional.
	// !!! Will be removed when we remove old model
	GitSource *GitSource `json:"git,omitempty"`

	// Git repository URL for the component.
	// Optional.
	// !!! Will be required when we remove old model
	// TODO replace Optional with Required when switching to the new model.
	// +kubebuilder:validation:Optional
	// + kubebuilder:validation:Required
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="Git repository URL cannot be changed"
	GitURL string `json:"url,omitempty"`

	// Dockerfile path for all versions, unless explicitly specified for a version.
	// Used only when sending a PR with build pipeline configuration was requested via 'spec.actions.create-pipeline-configuration-pr'.
	// Default: "Dockerfile"
	// Optional.
	DockerfileURI string `json:"dockerfileUri,omitempty"`

	// List of all versions for this component.
	// Optional.
	// !!! Will be required when we remove old model
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
	PullAndPush PipelineDefinition `json:"pull-and-push,omitempty"`

	// Pipeline used for pull pipeline run.
	// Can specify just one of: pipelinespec-from-bundle, pipelineref-by-name, pipelineref-by-git-resolver.
	// Optional.
	Pull PipelineDefinition `json:"pull,omitempty"`

	// Pipeline used for push pipeline run.
	// Can specify just one of: pipelinespec-from-bundle, pipelineref-by-name, pipelineref-by-git-resolver.
	// Optional.
	Push PipelineDefinition `json:"push,omitempty"`
}

type PipelineDefinition struct {
	// Will be used to fill out PipelineRef in pipeline runs to user specific pipeline via git resolver,
	// specifying repository with a pipeline definition.
	// Optional.
	PipelineRefGit PipelineRefGit `json:"pipelineref-by-git-resolver,omitempty"`

	// Will be used to fill out PipelineRef in pipeline runs to user specific pipeline.
	// Such pipeline definition has to be in .tekton.
	// Optional.
	PipelineRefName string `json:"pipelineref-by-name,omitempty"`

	// Will be used to fetch bundle and fill out PipelineSpec in pipeline runs.
	// Pipeline name is based on build-pipeline-config CM in build-service NS.
	// When 'latest' bundle is specified, bundle image will be used from CM.
	// When bundle is specified to specific image bundle, then that one will be used
	// and pipeline name will be used to fetch pipeline from that bundle.
	// Optional.
	PipelineSpecFromBundle PipelineSpecFromBundle `json:"pipelinespec-from-bundle,omitempty"`
}

type PipelineSpecFromBundle struct {
	// Bundle image reference. Use 'latest' to get bundle from build-pipeline-config CM,
	// or specify a specific bundle image.
	// Required.
	// +required
	Bundle string `json:"bundle"`

	// Pipeline name to fetch from the bundle, or from build-pipeline-config CM.
	// Required.
	// +required
	Name string `json:"name"`
}

type PipelineRefGit struct {
	// Path to the pipeline definition file within the repository.
	// Example: pipeline/push.yaml
	// Required.
	// +required
	PathInRepo string `json:"pathInRepo"`

	// Git revision (branch, tag, or commit) to use.
	// Example: main
	// Required.
	// +required
	Revision string `json:"revision"`

	// Git repository URL containing the pipeline definition.
	// Example: https://github.com/custom-pipelines/pipelines.git
	// Required.
	// +required
	Url string `json:"url"`
}

type ComponentVersion struct {
	// Used only when sending a PR with build pipeline configuration was requested via 'spec.actions.create-pipeline-configuration-pr'.
	// Pipeline used for the version; when omitted, the default pipeline will be used from 'spec.default-build-pipeline'.
	// Optional.
	BuildPipeline ComponentBuildPipeline `json:"build-pipeline,omitempty"`

	// Context directory for the version.
	// Used only when sending a PR with build pipeline configuration was requested via 'spec.actions.create-pipeline-configuration-pr'.
	// Default: "" (empty string, root of repository).
	// Optional.
	Context string `json:"context,omitempty"`

	// Dockerfile path for the version.
	// Used only when sending a PR with build pipeline configuration was requested via 'spec.actions.create-pipeline-configuration-pr'.
	// Default: "Dockerfile".
	// Optional.
	DockerfileURI string `json:"dockerfileUri,omitempty"`

	// User defined name for the version.
	// After sanitization (lower case, removing spaces, etc) all version names must be unique.
	// Required.
	// +required
	Name string `json:"name"`

	// Git branch to use for the version.
	// Required.
	// +required
	Revision string `json:"revision"`

	// When 'true' it will disable builds for a revision in the version.
	// Default: false.
	// Optional.
	SkipBuilds bool `json:"skip-builds,omitempty"`
}

type RepositorySettings struct {
	// When specified, will set value of `comment_strategy` in the Repository CR
	// Optional.
	CommentStrategy string `json:"comment-strategy,omitempty"`

	// When specified, will add values to `github_app_token_scope_repos` in the Repository CR
	// Optional.
	GithubAppTokenScopeRepos []string `json:"github-app-token-scope-repos,omitempty"`
}

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ComponentSpec defines the desired state of Component
type ComponentSpec struct {

	// +kubebuilder:validation:Pattern=^$|^[a-z0-9]([-a-z0-9]*[a-z0-9])?$
	// +kubebuilder:validation:MaxLength=63
	// ComponentName is name of the component to be added to the Application. The name must adhere to DNS-1123 validation.
	// Optional.
	// +optional
	// !!! Will be removed when we remove old model
	ComponentName string `json:"componentName,omitempty"`

	// +kubebuilder:validation:Pattern=^$|^[a-z0-9]([-a-z0-9]*[a-z0-9])?$
	// Application is the name of the application resource that the component belongs to.
	// Optional.
	// +optional
	// !!! Will be removed when we remove old model
	Application string `json:"application,omitempty"`

	// Secret describes the name of a Kubernetes secret containing either:
	// 1. A Personal Access Token to access the Component's git repostiory (if using a Git-source component) or
	// 2. An Image Pull Secret to access the Component's container image (if using an Image-source component).
	// Optional.
	// +optional
	// !!! Will be removed when we remove old model
	Secret string `json:"secret,omitempty"`

	// Source describes the Component source.
	// Required.
	// +required
	Source ComponentSource `json:"source"`

	// Compute Resources required by this component.
	// Optional.
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// The number of replicas to deploy the component with.
	// Optional.
	// +optional
	// !!! Will be removed when we remove old model
	Replicas *int `json:"replicas,omitempty"`

	// The port to expose the component over.
	// Optional.
	// +optional
	// !!! Will be removed when we remove old model
	TargetPort int `json:"targetPort,omitempty"`

	// The route to expose the component with.
	// Optional.
	// +optional
	// !!! Will be removed when we remove old model
	Route string `json:"route,omitempty"`

	// An array of environment variables to add to the component (ValueFrom not currently supported)
	// Optional
	// +optional
	Env []corev1.EnvVar `json:"env,omitempty"`

	// The container image repository to use for this component (without tag).
	// Either will be set by Image Repository, or explicitly specified with custom repo.
	// All versions of this component will use this single image repository.
	// Example: quay.io/org/tenant/component
	// Optional.
	// +optional
	ContainerImage string `json:"containerImage,omitempty"`

	// Whether or not to bypass the generation of GitOps resources for the Component. Defaults to false.
	// Optional.
	// +optional
	// !!! Will be removed when we remove old model
	SkipGitOpsResourceGeneration bool `json:"skipGitOpsResourceGeneration,omitempty"`

	// The list of components to be nudged by this components build upon a successful result.
	// Optional.
	// +optional
	// !!! Will be removed when we remove old model
	BuildNudgesRef []string `json:"build-nudges-ref,omitempty"`

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
	DefaultBuildPipeline ComponentBuildPipeline `json:"default-build-pipeline,omitempty"`
}

// ComponentStatus defines the observed state of Component
type ComponentStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Conditions is an array of the Component's status conditions
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Webhook URL generated by Builds
	// !!! Will be removed when we remove old model
	Webhook string `json:"webhook,omitempty"`

	// The devfile model for the Component CR
	// !!! Will be removed when we remove old model
	Devfile string `json:"devfile,omitempty"`

	// GitOps specific status for the Component CR
	// !!! Will be removed when we remove old model
	GitOps GitOpsStatus `json:"gitops,omitempty"`

	// The last built commit id (SHA-1 checksum) from the latest component build.
	// Example: 41fbdb124775323f58fd5ce93c70bb7d79c20650.
	// !!! Will be removed when we remove old model SHOULD this be in version specific section??
	LastBuiltCommit string `json:"lastBuiltCommit,omitempty"`

	// The last digest image component promoted with.
	// Example: quay.io/someorg/somerepository@sha256:5ca85b7f7b9da18a9c4101e81ee1d9bac35ac2b0b0221908ff7389204660a262.
	// !!! Will be removed when we remove old model SHOULD this be in version specific section??
	LastPromotedImage string `json:"lastPromotedImage,omitempty"`

	// The list of names of Components whose builds nudge this resource (their spec.build-nudges-ref[] references this component)
	// !!! Will be removed when we remove old model
	BuildNudgedBy []string `json:"build-nudged-by,omitempty"`

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

// GitOpsStatus contains GitOps repository-specific status for the component
// !!! Will be removed when we remove old model
type GitOpsStatus struct {
	// RepositoryURL is the gitops repository URL for the component
	RepositoryURL string `json:"repositoryURL,omitempty"`

	// Branch is the git branch used for the gitops repository
	Branch string `json:"branch,omitempty"`

	// Context is the path within the gitops repository used for the gitops resources
	Context string `json:"context,omitempty"`

	// ResourceGenerationSkipped is whether or not GitOps resource generation was skipped for the component
	ResourceGenerationSkipped bool `json:"resourceGenerationSkipped,omitempty"`

	// CommitID is the most recent commit ID in the GitOps repository for this component
	CommitID string `json:"commitID,omitempty"`
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

// Component is the Schema for the components API.    For description, refer to <a href="https://konflux-ci.dev/docs/reference/kube-apis/application-api/"> Hybrid Application Service Kube API </a>
// +kubebuilder:resource:path=components,shortName=cmp;comp
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.conditions[-1].status"
// +kubebuilder:printcolumn:name="Reason",type="string",JSONPath=".status.conditions[-1].reason"
// +kubebuilder:printcolumn:name="Type",type="string",JSONPath=".status.conditions[-1].type"
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
