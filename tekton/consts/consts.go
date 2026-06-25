/*
Copyright 2022 Red Hat Inc.

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

package consts

import "fmt"

const (
	// PipelinesLabelPrefix is the prefix of the pipelines label
	PipelinesLabelPrefix = "pipelines.appstudio.openshift.io"

	// TestLabelPrefix contains the prefix applied to labels and annotations related to testing.
	TestLabelPrefix = "test.appstudio.openshift.io"

	// PipelinesAsCodePrefix contains the prefix applied to labels and annotations copied from Pipelines as Code resources.
	PipelinesAsCodePrefix = "pac.test.appstudio.openshift.io"

	// BuildPipelineRunPrefix contains the build pipeline run related labels and annotations
	BuildPipelineRunPrefix = "build.appstudio"

	// CustomLabelPrefix contains the prefix applied to custom user-defined labels and annotations.
	CustomLabelPrefix = "custom.appstudio.openshift.io"

	// resource labels for snapshot, application and component
	ResourceLabelSuffix = "appstudio.openshift.io"

	// PipelineTypeTest is the type for PipelineRuns created to run an integration Pipeline
	PipelineTypeTest = "test"

	// Name of tekton resolver for git
	TektonResolverGit = "git"

	// TektonResolverBundle is the name of Tekton resolver for bundles
	TektonResolverBundle = "bundle"

	// Name of tekton git resolver param url
	TektonResolverGitParamURL = "url"

	// Name of tekton git resolver param revision
	TektonResolverGitParamRevision = "revision"

	// Value of ResourceKind field for remote pipelines
	ResourceKindPipeline = "pipeline"

	// Value of ResourceKind field for remote pipelineruns
	ResourceKindPipelineRun = "pipelinerun"

	// DefaultIntegrationPipelineServiceAccount denotes the service account which is used by default in integration pipelines
	DefaultIntegrationPipelineServiceAccount = "konflux-integration-runner"

	// DefaultIntegrationPipelineImagePullSecretName is the namespace-scoped image pull secret linked to the integration pipeline service account
	DefaultIntegrationPipelineImagePullSecretName = "components-namespace-pull"

	// DefaultIntegrationPipelineRoleBindingName is the RoleBinding that grants the integration pipeline service account its permissions
	DefaultIntegrationPipelineRoleBindingName = "konflux-integration-runner"

	// DefaultIntegrationPipelineClusterRoleName is the ClusterRole referenced by the integration pipeline RoleBinding
	DefaultIntegrationPipelineClusterRoleName = "konflux-integration-runner"

	/*
	 * Build PipelineConstants
	 */
	// master branch in github/gitlab
	MasterBranch = "master"

	// main branch in github/gitlab
	MainBranch = "main"

	// GitRefBranchPrefix is the git prefix denoting a reference is a branch
	GitRefBranchPrefix = "refs/heads/"

	// PipelineAsCodeSourceBranchAnnotation is the branch name of the the pull request is created from
	PipelineAsCodeSourceBranchAnnotation = "pipelinesascode.tekton.dev/source-branch"

	// PipelineAsCodeSourceRepoOrg is the repo org build PLR is triggered by
	PipelineAsCodeSourceRepoOrg = "pipelinesascode.tekton.dev/url-org"

	// PipelineAsCodePullRequestLabel is the type of event which triggered the pipelinerun in build service
	PipelineAsCodePullRequestLabel = "pipelinesascode.tekton.dev/pull-request"

	// PipelineAsCodeEventTypeLabel is the type of event which triggered the pipelinerun in build service
	PipelineAsCodeEventTypeLabel = "pipelinesascode.tekton.dev/event-type"

	// PipelineAsCodePushType is the type of push event which triggered the pipelinerun in build service
	PipelineAsCodePushType = "push"

	// PipelineAsCodeGLPushType is the type of gitlab push event which triggered the pipelinerun in build service
	PipelineAsCodeGLPushType = "Push"

	// PipelineAsCodeGitHubMergeQueueBranchPrefix is the prefix added to temporary branches which are created for merge queues
	PipelineAsCodeGitHubMergeQueueBranchPrefix = "gh-readonly-queue/"

	/*
	 * Utils constants
	 */
	// PipelineRunTypeLabel contains the type of the PipelineRunTypeLabel.
	PipelineRunTypeLabel = "pipelines.appstudio.openshift.io/type"

	// PipelineRunBuildType is the type denoting a build PipelineRun.
	PipelineRunBuildType = "build"

	// PipelineRunTestType is the type denoting a test PipelineRun.
	PipelineRunTestType = "test"

	// PipelineRunComponentLabel is the label denoting the application.
	PipelineRunComponentLabel = "appstudio.openshift.io/component"

	// PipelineRunComponentVersionAnnotation denotes the componentVersion for the build
	PipelineRunComponentVersionAnnotation = "appstudio.openshift.io/version"

	PipelineRunComponentVersionContextAnnotation = "appstudio.openshift.io/context"

	// PipelineRunApplicationLabel is the label denoting the application.
	PipelineRunApplicationLabel = "appstudio.openshift.io/application"

	// PipelineRunChainsSignedAnnotation is the label added by Tekton Chains to signed PipelineRuns
	PipelineRunChainsSignedAnnotation = "chains.tekton.dev/signed"

	// PipelineRunImageUrlParamName name of image url output param
	PipelineRunImageUrlParamName = "IMAGE_URL"

	// PipelineRunImageDigestParamName name of image digest in PipelineRun result param
	PipelineRunImageDigestParamName = "IMAGE_DIGEST"

	// PipelineRunChainsGitUrlParamName name of param chains repo url
	PipelineRunChainsGitUrlParamName = "CHAINS-GIT_URL"

	// PipelineRunChainsGitCommitParamName name of param repo chains commit
	PipelineRunChainsGitCommitParamName = "CHAINS-GIT_COMMIT"

	// PipelineRunShouldReleaseResultName is the name of the SHOULD_RELEASE result in build PipelineRuns
	PipelineRunShouldReleaseResultName = "SHOULD_RELEASE"

	/*
	 * Nudge PipelineRun constants — build-service compatible annotation/label names
	 */

	// NudgeProcessedAnnotation marks a build PLR as having already been processed for nudging
	NudgeProcessedAnnotation = "build.appstudio.openshift.io/component-nudge-processed"

	// NudgeSimpleBranchAnnotation on a Component controls simplified branch naming for nudge PRs
	NudgeSimpleBranchAnnotation = "build.appstudio.openshift.io/build-nudge-simple-branch"

	// NudgeFilesAnnotation on a build PLR restricts which files Renovate updates
	NudgeFilesAnnotation = "build.appstudio.openshift.io/build-nudge-files"

	// NudgeTypeLabel labels a PipelineRun as a nudge pipeline
	NudgeTypeLabel = "build.appstudio.redhat.com/type"

	// NudgePipelineRunTypeValue is the value for the nudge type label
	NudgePipelineRunTypeValue = "nudge"

	// NudgedComponentsAnnotation lists target components being nudged
	NudgedComponentsAnnotation = "build.appstudio.redhat.com/nudged-components"

	// NudgingCommitAnnotation records the source commit hash
	NudgingCommitAnnotation = "build.appstudio.redhat.com/nudging-commit"

	// NudgingComponentAnnotation records the source component name
	NudgingComponentAnnotation = "build.appstudio.redhat.com/nudging-component"

	// NudgingImageAnnotation records the built image reference
	NudgingImageAnnotation = "build.appstudio.redhat.com/nudging-image"

	// NudgingPipelineAnnotation records the source build PLR name
	NudgingPipelineAnnotation = "build.appstudio.redhat.com/nudging-pipeline"

	// GitRepoAtShaAnnotation on a build PLR links to the source repo at the built commit
	GitRepoAtShaAnnotation = "build.appstudio.openshift.io/repo-url-at-sha"

	/*
	 * Renovate defaults
	 */

	// RenovateImageEnvName is the env var to override the Renovate image
	RenovateImageEnvName = "RENOVATE_IMAGE"

	// DefaultRenovateImageUrl is the default Renovate container image
	DefaultRenovateImageUrl = "quay.io/konflux-ci/mintmaker-renovate-image:29a2f31"

	// DefaultRenovateUser is the default git username for Renovate
	DefaultRenovateUser = "red-hat-konflux"

	// DefaultRenovateLabel is the label applied to Renovate PRs
	DefaultRenovateLabel = "konflux-nudge"

	// BranchPrefix is the branch prefix for nudge PRs
	BranchPrefix = "konflux/component-updates/"

	// DefaultNudgeFiles is the default file match pattern for Renovate
	DefaultNudgeFiles = ".*Dockerfile.*, .*.yaml, .*Containerfile.*"

	// NamespaceWideRenovateConfigMapName is the name of the namespace-wide custom Renovate config
	NamespaceWideRenovateConfigMapName = "namespace-wide-nudging-renovate-config"

	// CustomRenovateConfigMapAnnotation on a Component points to a per-component custom Renovate ConfigMap
	CustomRenovateConfigMapAnnotation = "build.appstudio.openshift.io/nudge_renovate_config_map"

	// CaConfigMapLabel identifies ConfigMaps containing trusted CA bundles
	CaConfigMapLabel = "config.openshift.io/inject-trusted-cabundle"

	// CaConfigMapKey is the key in CA ConfigMaps
	CaConfigMapKey = "ca-bundle.crt"

	// CaFilePath is the filename for the CA bundle mount
	CaFilePath = "tls-ca-bundle.pem"

	// CaMountPath is where the CA bundle is mounted in the Renovate container
	CaMountPath = "/etc/pki/ca-trust/extracted/pem"

	// CaVolumeMountName is the volume name for the CA bundle
	CaVolumeMountName = "trusted-ca"

	/*
	 * Build-service namespace and PaC secret constants (for GitHub App auth)
	 */

	// BuildServiceNamespaceName is the namespace where build-service is deployed
	BuildServiceNamespaceName = "build-service"

	// PipelinesAsCodeGitHubAppSecretName is the global PaC secret for GitHub App auth
	PipelinesAsCodeGitHubAppSecretName = "pipelines-as-code-secret"

	// PipelinesAsCodeGithubAppIdKey is the key for the GitHub App ID in the PaC secret
	PipelinesAsCodeGithubAppIdKey = "github-application-id"

	// PipelinesAsCodeGithubPrivateKey is the key for the GitHub App private key in the PaC secret
	PipelinesAsCodeGithubPrivateKey = "github-private-key"
)

var (
	// PipelinesTypeLabel is the label used to describe the type of pipeline
	PipelinesTypeLabel = fmt.Sprintf("%s/%s", PipelinesLabelPrefix, "type")

	// TestNameLabel is the label used to specify the name of the Test associated with the PipelineRun
	TestNameLabel = fmt.Sprintf("%s/%s", TestLabelPrefix, "name")

	// ScenarioNameLabel is the label used to specify the name of the IntegrationTestScenario associated with the PipelineRun
	ScenarioNameLabel = fmt.Sprintf("%s/%s", TestLabelPrefix, "scenario")

	// SnapshotNameLabel is the label of specific the name of the snapshot associated with PipelineRun
	SnapshotNameLabel = fmt.Sprintf("%s/%s", ResourceLabelSuffix, "snapshot")

	// SnapshotNamesLabel is the label of the list of snapshots associated with PipelineRun
	SnapshotNamesLabel = fmt.Sprintf("%s/%s", ResourceLabelSuffix, "snapshots")

	// EnvironmentNameLabel is the label of specific the name of the environment associated with PipelineRun
	EnvironmentNameLabel = fmt.Sprintf("%s/%s", ResourceLabelSuffix, "environment")

	// ApplicationNameLabel is the label of specific the name of the Application associated with PipelineRun
	ApplicationNameLabel = fmt.Sprintf("%s/%s", ResourceLabelSuffix, "application")

	// ComponentGroupNameLabel is the label containing the name of the ComponentGroup associated with PipelineRun
	ComponentGroupNameLabel = fmt.Sprintf("%s/%s", ResourceLabelSuffix, "component-group")

	// ComponentNameLabel is the label of specific the name of the component associated with PipelineRun
	ComponentNameLabel = fmt.Sprintf("%s/%s", ResourceLabelSuffix, "component")

	// OptionalLabel is the label used to specify if an IntegrationTestScenario is allowed to fail
	OptionalLabel = fmt.Sprintf("%s/%s", TestLabelPrefix, "optional")
)
