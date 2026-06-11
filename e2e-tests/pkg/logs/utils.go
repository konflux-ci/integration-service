package logs

import (
	"fmt"
	"os"
	"time"

	"github.com/konflux-ci/integration-service/e2e-tests/pkg/utils"
	"github.com/onsi/ginkgo/v2"
)

// createArtifactDirectory creates directory for storing artifacts of current spec.
func createArtifactDirectory() (string, error) {
	wd, _ := os.Getwd()
	artifactDir := utils.GetEnv("ARTIFACT_DIR", fmt.Sprintf("%s/tmp", wd))
	classname := ShortenStringAddHash(ginkgo.CurrentSpecReport())
	testLogsDir := fmt.Sprintf("%s/%s", artifactDir, classname)

	if err := os.MkdirAll(testLogsDir, 0750); err != nil {
		return "", err
	}

	return testLogsDir, nil
}

// StoreArtifacts stores given artifacts under artifact directory.
func StoreArtifacts(artifacts map[string][]byte) error {
	artifactsDirectory, err := createArtifactDirectory()
	if err != nil {
		return err
	}

	for artifact_name, artifact_value := range artifacts {
		filePath := fmt.Sprintf("%s/%s", artifactsDirectory, artifact_name)
		if err := os.WriteFile(filePath, []byte(artifact_value), 0600); err != nil {
			return err
		}
	}

	return nil
}

func StoreTestTiming() error {
	artifactsDirectory, err := createArtifactDirectory()
	if err != nil {
		return err
	}

	testTime := "Test started at: " + ginkgo.CurrentSpecReport().StartTime.String() + "\nTest ended at: " + time.Now().String()
	filePath := fmt.Sprintf("%s/test-timing", artifactsDirectory)
	if err := os.WriteFile(filePath, []byte(testTime), 0600); err != nil {
		return fmt.Errorf("failed to store test timing: %v", err)
	}

	return nil
}
