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

package snapshot

import (
	"crypto/rand"
	"math/big"
	"strings"
	"time"

	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	"github.com/konflux-ci/integration-service/api/v1beta2"
	"github.com/konflux-ci/integration-service/helpers"
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

func joinInvalidComponentNamesAndVersions(invalidComponents []v1beta2.ComponentState) string {
	var sb strings.Builder
	for _, component := range invalidComponents {
		sb.WriteString(helpers.GetComponentVersionLogString(component.Name, component.Version))
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
