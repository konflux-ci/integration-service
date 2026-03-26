package snapshot

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"
	"time"

	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	"github.com/konflux-ci/integration-service/api/v1beta2"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
)

// generateRandomSuffix generates a random 2-character alphanumeric suffix for collision handling
func generateRandomSuffix() (string, error) {
	const charset = "0123456789abcdefghijklmnopqrstuvwxyz"
	const suffixLength = 2
	suffix := make([]byte, suffixLength)
	for i := range suffix {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", err
		}
		suffix[i] = charset[num.Int64()]
	}
	return string(suffix), nil
}

func getComponentVersionString(name, version string) string {
	return fmt.Sprintf("%s/%s", name, version)
}

func getComponentVersionLogString(name, version string) string {
	return fmt.Sprintf("%s (version %s)", name, version)
}

func joinInvalidComponentNamesAndVersions(invalidComponents []v1beta2.ComponentState) string {
	var sb strings.Builder
	for _, component := range invalidComponents {
		sb.WriteString(getComponentVersionLogString(component.Name, component.Version))
	}
	return sb.String()
}

func snapshotComponentToComponentState(snapshotComponent applicationapiv1alpha1.SnapshotComponent) v1beta2.ComponentState {
	return v1beta2.ComponentState{
		Name:                  snapshotComponent.Name,
		Version:               snapshotComponent.Version,
		URL:                   snapshotComponent.Source.GitSource.URL,
		LastPromotedImage:     snapshotComponent.ContainerImage,
		LastPromotedCommit:    snapshotComponent.Source.GitSource.Revision,
		LastPromotedBuildTime: nil, // this will be inherited from old value not change
	}
}

func getPipelineRunStartTimeMillis(pipelineRun *tektonv1.PipelineRun) int64 {
	if pipelineRun.Status.StartTime != nil {
		return pipelineRun.Status.StartTime.UnixMilli()
	}
	return time.Now().UnixMilli()
}
