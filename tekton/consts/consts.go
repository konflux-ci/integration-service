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

	/*
	 * Build PipelineConstants
	 */
	// master branch in github/gitlab
	MasterBranch = "master"

	// main branch in github/gitlab
	MainBranch = "main"

	// PipelineAsCodeSourceBranchAnnotation is the branch name of the the pull request is created from
	PipelineAsCodeSourceBranchAnnotation = "pipelinesascode.tekton.dev/source-branch"

	// PipelineAsCodeSourceRepoOrg is the repo org build PLR is triggered by
	PipelineAsCodeSourceRepoOrg = "pipelinesascode.tekton.dev/url-org"

	// PipelineAsCodePullRequestLabel is the type of event which triggered the pipelinerun in build service
	PipelineAsCodePullRequestLabel = "pipelinesascode.tekton.dev/pull-request"

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

	// EnvironmentNameLabel is the label of specific the name of the environment associated with PipelineRun
	EnvironmentNameLabel = fmt.Sprintf("%s/%s", ResourceLabelSuffix, "environment")

	// ApplicationNameLabel is the label of specific the name of the Application associated with PipelineRun
	ApplicationNameLabel = fmt.Sprintf("%s/%s", ResourceLabelSuffix, "application")

	// ComponentNameLabel is the label of specific the name of the component associated with PipelineRun
	ComponentNameLabel = fmt.Sprintf("%s/%s", ResourceLabelSuffix, "component")

	// OptionalLabel is the label used to specify if an IntegrationTestScenario is allowed to fail
	OptionalLabel = fmt.Sprintf("%s/%s", TestLabelPrefix, "optional")
)
