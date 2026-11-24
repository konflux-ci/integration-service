package build

import (
	"fmt"
	"os"

	"github.com/konflux-ci/integration-service/e2e-tests/pkg/constants"
)

// pinnedDockerBuildBundle is the docker-build bundle from before the 2026-11-12
// konflux-ci Renovate bump (883945d6). Used when CUSTOM_DOCKER_BUILD_PIPELINE_BUNDLE is unset,
// so e2e builds do not pick up the newer bundle with roxctl-scan and inline syft.
const pinnedDockerBuildBundle = "quay.io/konflux-ci/tekton-catalog/pipeline-docker-build@sha256:8109de83b93c5004dceab17c57cb32eb128a1d418b213b7ff08d8c762dcd7ca4"

func GetBuildPipelineBundleAnnotation(buildPipelineName constants.BuildPipelineType) map[string]string {
	var bundleVersion string

	switch buildPipelineName {
	case constants.DockerBuild:
		bundleVersion = os.Getenv(constants.CUSTOM_DOCKER_BUILD_PIPELINE_BUNDLE_ENV)
		if bundleVersion == "" {
			bundleVersion = pinnedDockerBuildBundle
		}
	case constants.DockerBuildOciTA:
		bundleVersion = os.Getenv(constants.CUSTOM_DOCKER_BUILD_OCI_TA_PIPELINE_BUNDLE_ENV)
	case constants.DockerBuildOciTAMin:
		bundleVersion = os.Getenv(constants.CUSTOM_DOCKER_BUILD_OCI_TA_MIN_PIPELINE_BUNDLE_ENV)
	case constants.DockerBuildMultiPlatformOciTa:
		bundleVersion = os.Getenv(constants.CUSTOM_DOCKER_BUILD_OCI_MULTI_PLATFORM_TA_PIPELINE_BUNDLE_ENV)
	case constants.FbcBuilder:
		bundleVersion = os.Getenv(constants.CUSTOM_FBC_BUILDER_PIPELINE_BUNDLE_ENV)
	}
	if bundleVersion == "" {
		bundleVersion = "latest"
	}

	return map[string]string{
		"build.appstudio.openshift.io/pipeline": fmt.Sprintf(`{"name":"%s", "bundle": "%s"}`, buildPipelineName, bundleVersion),
	}
}
