package integration

import (
	"fmt"
	"time"

	"github.com/konflux-ci/integration-service/e2e-tests/pkg/constants"
	"github.com/konflux-ci/integration-service/e2e-tests/pkg/utils"
)

const (
	EnvironmentName                = "development"
	gitURL                         = "https://github.com/konflux-ci/integration-examples.git"
	revision                       = "6a7c81802e785aa869f82301afe61f4e9775772b"
	pathInRepoPass                 = "pipelines/integration_resolver_pipeline_pass.yaml"
	pathInRepoPassPipelinerun      = "pipelineruns/integration_resolver_pipelinerun_pass.yaml"
	pathIntegrationPipelineWithEnv = "pipelines/integration_resolver_pipeline_environment_pass.yaml"
	pathInRepoFail                 = "pipelines/integration_resolver_pipeline_fail.yaml"
	pathInRepoTask                 = "tasks/integration_resolver_task_pass.yaml"
	autoReleasePlan                = "auto-releaseplan"
	targetReleaseNamespace         = "default"

	componentRepoNameForResolution            = "konflux-test-integration-resolution"
	componentRepoNameForGeneralIntegration    = "konflux-test-integration"
	componentRepoNameForGroupIntegration      = "konflux-test-integration-clone"
	componentRepoNameForStatusReporting       = "konflux-test-integration-status-report"
	multiComponentRepoNameForGroupSnapshot    = "group-snapshot-multi-component"
	multiComponentDefaultBranch               = "onboarding"
	multiComponentGitRevision                 = "0d1835404efb8ab7bb1ab5b5b82cda1ebfda4b25"
	multiRepoComponentGitRevision             = "79402df023e646c5ad108abc879ad1b28799cbc4"
	gitlabComponentRepoName                   = "hacbs-test-project-integration"
	componentDefaultBranch                    = "onboarding"
	fallbackBranchName                        = "main"
	componentRevision                         = "79402df023e646c5ad108abc879ad1b28799cbc4"
	referenceDoesntExist                      = "Reference does not exist"
	checkrunStatusCompleted                   = "completed"
	checkrunConclusionSuccess                 = "success"
	checkrunConclusionFailure                 = "failure"
	integrationPipelineRunCommitStatusSuccess = "success"
	integrationPipelineRunCommitStatusFail    = "failed"
	spaceRequestCronJobNamespace              = "spacerequest-cleaner"
	spaceRequestCronJobName                   = "spacerequest-cleaner"
	spaceRequestNamePrefix                    = "task-spacerequest-"

	snapshotAnnotation                       = "appstudio.openshift.io/snapshot"
	scenarioAnnotation                       = "test.appstudio.openshift.io/scenario"
	groupSnapshotAnnotation                  = "test.appstudio.openshift.io/pr-group"
	testGroupSnapshotAnnotation              = "test.appstudio.openshift.io/group-test-info"
	snapshotStatusAnnotation                 = "test.appstudio.openshift.io/status"
	gitReportingFailureAnnotation            = "test.appstudio.openshift.io/git-reporting-failure"
	pipelinerunFinalizerByIntegrationService = "test.appstudio.openshift.io/pipelinerun"
	snapshotRerunLabel                       = "test.appstudio.openshift.io/run"
	snapshotCreationReport                   = "test.appstudio.openshift.io/snapshot-creation-report"
	pipelinesAsCodeGitProviderAnnotation     = "pac.test.appstudio.openshift.io/git-provider"

	chainsSignedAnnotation = "chains.tekton.dev/signed"

	shortTimeout     = time.Duration(10 * time.Minute)
	longTimeout      = time.Duration(15 * time.Minute)
	superLongTimeout = time.Duration(20 * time.Minute)
)

var (
	componentGitSourceURLForGeneralIntegration    = fmt.Sprintf("https://github.com/%s/%s", utils.GetEnv(constants.GITHUB_E2E_ORGANIZATION_ENV, "redhat-appstudio-qe"), componentRepoNameForGeneralIntegration)
	componentGitSourceURLForGroupIntegration      = fmt.Sprintf("https://github.com/%s/%s", utils.GetEnv(constants.GITHUB_E2E_ORGANIZATION_ENV, "redhat-appstudio-qe"), componentRepoNameForGroupIntegration)
	componentGitSourceURLForStatusReporting       = fmt.Sprintf("https://github.com/%s/%s", utils.GetEnv(constants.GITHUB_E2E_ORGANIZATION_ENV, "redhat-appstudio-qe"), componentRepoNameForStatusReporting)
	multiComponentGitSourceURLForGroupSnapshotA   = fmt.Sprintf("https://github.com/%s/%s", utils.GetEnv(constants.GITHUB_E2E_ORGANIZATION_ENV, "redhat-appstudio-qe"), multiComponentRepoNameForGroupSnapshot)
	multiComponentGitSourceURLForGroupSnapshotB   = fmt.Sprintf("https://github.com/%s/%s", utils.GetEnv(constants.GITHUB_E2E_ORGANIZATION_ENV, "redhat-appstudio-qe"), multiComponentRepoNameForGroupSnapshot)
	multiComponentContextDirs                     = []string{"go-component", "python-component"}
	gitlabOrg                                     = utils.GetEnv(constants.GITLAB_QE_ORG_ENV, constants.DefaultGitLabQEOrg)
	gitlabProjectIDForStatusReporting             = fmt.Sprintf("%s/%s", gitlabOrg, gitlabComponentRepoName)
	gitlabComponentGitSourceURLForStatusReporting = fmt.Sprintf("https://gitlab.com/%s/%s", gitlabOrg, gitlabComponentRepoName)
)
