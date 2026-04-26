package constants

import "time"

type BuildPipelineType string

// Global constants
const (
	// A github token is required to run the tests. The token need to have permissions to the given github organization. By default the e2e use redhat-appstudio-qe github organization.
	GITHUB_TOKEN_ENV string = "GITHUB_TOKEN" // #nosec

	// The github organization is used to create the gitops repositories in Red Hat Appstudio.
	GITHUB_E2E_ORGANIZATION_ENV string = "MY_GITHUB_ORG" // #nosec

	// E2E test namespace where the app and component CRs will be created
	E2E_APPLICATIONS_NAMESPACE_ENV string = "E2E_APPLICATIONS_NAMESPACE"

	// Bundle ref for custom docker-build, format example: quay.io/redhat-appstudio-qe/test-images:pipeline-bundle-1715584704-fftb
	CUSTOM_DOCKER_BUILD_PIPELINE_BUNDLE_ENV string = "CUSTOM_DOCKER_BUILD_PIPELINE_BUNDLE"

	// Bundle ref for custom docker-build-oci-ta, format example: quay.io/redhat-appstudio-qe/test-images:pipeline-bundle-1715584704-fftb
	CUSTOM_DOCKER_BUILD_OCI_TA_PIPELINE_BUNDLE_ENV string = "CUSTOM_DOCKER_BUILD_OCI_TA_PIPELINE_BUNDLE"

	// Bundle ref for custom docker-build-oci-ta-min, format example: quay.io/redhat-appstudio-qe/test-images:pipeline-bundle-1715584704-fftb
	CUSTOM_DOCKER_BUILD_OCI_TA_MIN_PIPELINE_BUNDLE_ENV string = "CUSTOM_DOCKER_BUILD_OCI_TA_MIN_PIPELINE_BUNDLE"

	// Bundle ref for custom docker-build-multi-platform-oci-ta, format example: quay.io/redhat-appstudio-qe/test-images:pipeline-bundle-1715584704-fftb
	CUSTOM_DOCKER_BUILD_OCI_MULTI_PLATFORM_TA_PIPELINE_BUNDLE_ENV string = "CUSTOM_DOCKER_BUILD_OCI_MULTI_PLATFORM_TA_PIPELINE_BUNDLE"

	// Bundle ref for custom fbc-builder, format example: quay.io/redhat-appstudio-qe/test-images:pipeline-bundle-1715584704-fftb
	CUSTOM_FBC_BUILDER_PIPELINE_BUNDLE_ENV string = "CUSTOM_FBC_BUILDER_PIPELINE_BUNDLE"

	// This variable is set by an automation in case Spray Proxy configuration fails in CI
	SKIP_PAC_TESTS_ENV = "SKIP_PAC_TESTS"

	// A gitlab bot token is required to run tests against gitlab.com. The token need to have permissions to the Gitlab repository.
	GITLAB_BOT_TOKEN_ENV string = "GITLAB_BOT_TOKEN" // #nosec

	// The GitLab org which owns the test repositories
	GITLAB_QE_ORG_ENV string = "GITLAB_QE_ORG"

	// The gitlab API URL used to run e2e tests against
	GITLAB_API_URL_ENV string = "GITLAB_API_URL" // #nosec

	// A Codeberg bot token is required to run tests against codeberg.org. The token needs to have permissions to the Codeberg organization/repositories.
	CODEBERG_BOT_TOKEN_ENV string = "CODEBERG_BOT_TOKEN" // #nosec

	// The Codeberg org which owns the test repositories
	CODEBERG_QE_ORG_ENV string = "CODEBERG_QE_ORG"

	// The Codeberg base URL used to run e2e tests against (defaults to https://codeberg.org)
	CODEBERG_API_URL_ENV string = "CODEBERG_API_URL" // #nosec

	// We are running tests against 2 types of test environments:
	//
	// * downstream - Konflux deployed from infra-deployments repo, typically on OCP or ROSA
	//
	// * upstream - Konflux deployed from konflux-ci repo, typically running on Kind cluster
	//
	// This env var is meant to be used in the framework to apply a different framework init
	// or a test configuration based on the provided value
	// By default it should use "downstream"
	TEST_ENVIRONMENT_ENV = "TEST_ENVIRONMENT"

	// Test environments
	UpstreamTestEnvironment string = "upstream"

	// Test namespace's required labels
	ArgoCDLabelKey   string = "argocd.argoproj.io/managed-by"
	ArgoCDLabelValue string = "gitops-service-argocd"
	// Label for marking a namespace as a tenant namespace
	TenantLabelKey   string = "konflux-ci.dev/type"
	TenantLabelValue string = "tenant"
	// Label for marking a namespace with the workspace label
	WorkspaceLabelKey string = "appstudio.redhat.com/workspace_name"

	DefaultGitLabAPIURL   = "https://gitlab.com/api/v4"
	DefaultGitLabQEOrg    = "konflux-qe"
	DefaultCodebergAPIURL = "https://codeberg.org"
	DefaultCodebergQEOrg  = "konflux-qe"
	DefaultGilabGroupId   = "85150202"

	DefaultPipelineServiceAccount = "konflux-integration-runner"

	PaCPullRequestBranchPrefix = "konflux-"

	DockerFilePath = "docker/Dockerfile"

	PipelineRunPollingInterval = 20 * time.Second

	// Name of the finalizer used for blocking pruning of E2E test PipelineRuns
	E2ETestFinalizerName = "e2e-test"

	CheckrunConclusionSuccess = "success"
	CheckrunConclusionFailure = "failure"
	CheckrunStatusCompleted   = "completed"
	CheckrunConclusionNeutral = "neutral"

	DockerBuild                   BuildPipelineType = "docker-build"
	DockerBuildOciTA              BuildPipelineType = "docker-build-oci-ta"
	DockerBuildOciTAMin           BuildPipelineType = "docker-build-oci-ta-min"
	DockerBuildMultiPlatformOciTa BuildPipelineType = "docker-build-multi-platform-oci-ta"
	FbcBuilder                    BuildPipelineType = "fbc-builder"

	// A cluster role used to be bound to a user that has admin access to all Konflux resources in a specific namespace
	// https://github.com/konflux-ci/konflux-ci/blob/2772e3b648ce1c1ae05f31e77732063c4103de09/konflux-ci/rbac/core/konflux-admin-user-actions.yaml
	KonfluxAdminUserActionsClusterRoleName = "konflux-admin-user-actions"
	// Default role binding name
	DefaultKonfluxAdminRoleBindingName = "user2-konflux-admin"
	// Default user name available after deploying upstream version of konflux-ci
	DefaultKonfluxCIUserName = "user2@konflux.dev"
)

var (
	ComponentPaCRequestAnnotation              = map[string]string{"build.appstudio.openshift.io/request": "configure-pac"}
	ComponentTriggerSimpleBuildAnnotation      = map[string]string{"build.appstudio.openshift.io/request": "trigger-simple-build"}
	ImageControllerAnnotationRequestPublicRepo = map[string]string{"image.redhat.com/generate": `{"visibility": "public"}`}
	IntegrationTestScenarioDefaultLabels       = map[string]string{"test.appstudio.openshift.io/optional": "false"}
)
