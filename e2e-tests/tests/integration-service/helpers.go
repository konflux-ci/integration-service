// TODO: when we remove old application-specific code:
//   - change parentName to componentGroupName
//   - remove usingComponentGroups parameters
//   - Delete application-specific function calls such as HasController.DeleteApplication
package integration

import (
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"

	appstudioApi "github.com/konflux-ci/application-api/api/v1alpha1"
	"github.com/konflux-ci/image-controller/pkg/quay"
	"github.com/konflux-ci/integration-service/e2e-tests/pkg/constants"
	"github.com/konflux-ci/integration-service/e2e-tests/pkg/framework"
	"github.com/konflux-ci/integration-service/e2e-tests/pkg/utils"
	"github.com/konflux-ci/integration-service/e2e-tests/pkg/utils/build"
	ginkgo "github.com/onsi/ginkgo/v2"
	gomega "github.com/onsi/gomega"
)

func cleanup(f framework.Framework, testNamespace, parentName, componentName string, snapshot *appstudioApi.Snapshot, usingComponentGroups bool) {
	if !ginkgo.CurrentSpecReport().Failed() {
		gomega.Expect(f.AsKubeAdmin.IntegrationController.DeleteSnapshot(snapshot, testNamespace)).To(gomega.Succeed())
		integrationTestScenarios, err := f.AsKubeAdmin.IntegrationController.GetIntegrationTestScenarios(parentName, testNamespace, usingComponentGroups)
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

		for _, testScenario := range *integrationTestScenarios {
			gomega.Expect(f.AsKubeAdmin.IntegrationController.DeleteIntegrationTestScenario(&testScenario, testNamespace)).To(gomega.Succeed())
		}
		gomega.Expect(f.AsKubeAdmin.HasController.DeleteComponent(componentName, testNamespace, false)).To(gomega.Succeed())
		if usingComponentGroups {
			gomega.Expect(f.AsKubeAdmin.HasController.DeleteComponentGroup(parentName, testNamespace, false)).To(gomega.Succeed())
		} else {
			gomega.Expect(f.AsKubeAdmin.HasController.DeleteApplication(parentName, testNamespace, false)).To(gomega.Succeed())
		}
		err = deleteQuayRepo(componentName, testNamespace)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	}
}

func deleteQuayRepo(componentName string, testNamespace string) error {
	quayOrgToken := os.Getenv("DEFAULT_QUAY_ORG_TOKEN")
	if quayOrgToken == "" {
		return fmt.Errorf("%s", "DEFAULT_QUAY_ORG_TOKEN env var was not found")
	}
	quayOrg := utils.GetEnv("DEFAULT_QUAY_ORG", "redhat-appstudio-qe")

	quayClient := quay.NewQuayClient(&http.Client{Transport: utils.NewRetryTransport(&http.Transport{})}, quayOrgToken, "https://quay.io/api/v1")

	r, err := regexp.Compile(fmt.Sprintf(`^(%s)`, testNamespace))
	if err != nil {
		return err
	}

	repos, err := quayClient.GetAllRepositories(quayOrg)
	if err != nil {
		return err
	}
	// Key is the repo name without slashes which is the same as robot name
	// Value is the repo name with slashes
	reposMap := make(map[string]string)

	for _, repo := range repos {
		if r.MatchString(repo.Name) {
			sanitizedRepoName := strings.ReplaceAll(repo.Name, "/", "") // repo name without slashes
			reposMap[sanitizedRepoName] = repo.Name
		}
	}

	sanitizedName := testNamespace + componentName
	if repo, exists := reposMap[sanitizedName]; exists {
		deleted, err := quayClient.DeleteRepository(quayOrg, repo)
		if err != nil {
			return fmt.Errorf("failed to delete repository %s, error: %s", repo, err)
		}
		if !deleted {
			fmt.Printf("repository %s has already been deleted, skipping\n", repo)
		}
	}
	return nil
}

// NOTE: this function is only used in the group test e2e tests
// TODO: update this function to be backward-compatible when we create componentGroup e2e tests for group testing
func createComponentWithCustomBranch(f framework.Framework, testNamespace, applicationName, componentName, componentRepoURL string, toBranchName string, contextDir string) *appstudioApi.Component {
	// get the build pipeline bundle annotation
	buildPipelineAnnotation := build.GetBuildPipelineBundleAnnotation(constants.DockerBuild)
	dockerFileURL := constants.DockerFilePath
	if contextDir == "" {
		dockerFileURL = "Dockerfile"
	}
	componentObj := appstudioApi.ComponentSpec{
		ComponentName: componentName,
		Application:   applicationName,
		Source: appstudioApi.ComponentSource{
			ComponentSourceUnion: appstudioApi.ComponentSourceUnion{
				GitSource: &appstudioApi.GitSource{
					URL:           componentRepoURL,
					Revision:      toBranchName,
					Context:       contextDir,
					DockerfileURL: dockerFileURL,
				},
			},
		},
	}

	originalComponent, err := f.AsKubeAdmin.HasController.CreateComponentCheckImageRepository(componentObj, componentName, testNamespace, "", "", applicationName, true, utils.MergeMaps(utils.MergeMaps(constants.ComponentPaCRequestAnnotation, constants.ImageControllerAnnotationRequestPublicRepo), buildPipelineAnnotation))
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	return originalComponent
}
