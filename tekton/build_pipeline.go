/*
Copyright 2024 Red Hat Inc.

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

package tekton

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"

	h "github.com/konflux-ci/integration-service/helpers"
	"github.com/konflux-ci/integration-service/tekton/consts"
	"github.com/konflux-ci/operator-toolkit/metadata"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// AnnotateBuildPipelineRun sets annotation for a build pipelineRun in defined context and returns that pipeline
func AnnotateBuildPipelineRun(ctx context.Context, pipelineRun *tektonv1.PipelineRun, key, value string, cl client.Client) error {
	patch := client.MergeFrom(pipelineRun.DeepCopy())

	_ = metadata.SetAnnotation(&pipelineRun.ObjectMeta, key, value)

	err := cl.Patch(ctx, pipelineRun, patch)
	if err != nil {
		return err
	}
	return nil
}

// LabelBuildPipelineRun sets annotation for a build pipelineRun in defined context and returns that pipeline
func LabelBuildPipelineRun(ctx context.Context, pipelineRun *tektonv1.PipelineRun, key, value string, cl client.Client) error {
	patch := client.MergeFrom(pipelineRun.DeepCopy())

	_ = metadata.SetLabel(&pipelineRun.ObjectMeta, key, value)

	err := cl.Patch(ctx, pipelineRun, patch)
	if err != nil {
		return err
	}
	return nil
}

// AnnotateBuildPipelineRunWithCreateSnapshotAnnotation sets annotation test.appstudio.openshift.io/create-snapshot-status to build pipelineRun with
// a message that reflects either success or failure for creating a snapshot
func AnnotateBuildPipelineRunWithCreateSnapshotAnnotation(ctx context.Context, pipelineRun *tektonv1.PipelineRun, cl client.Client, ensureSnapshotExistsErr error) error {
	message := ""
	status := ""

	if ensureSnapshotExistsErr == nil {
		if !metadata.HasAnnotation(pipelineRun, consts.SnapshotNameLabel) {
			// do nothing for in progress build PLR
			return nil
		}
		message = fmt.Sprintf("Sucessfully created snapshot. See annotation %s for name", consts.SnapshotNameLabel)
		status = "success"
	} else {
		message = fmt.Sprintf("Failed to create snapshot. Error: %s", ensureSnapshotExistsErr.Error())
		status = "failed"
	}

	jsonResult, err := json.Marshal(map[string]string{
		"status":  status,
		"message": message,
	})
	if err != nil {
		return err
	}
	return AnnotateBuildPipelineRun(ctx, pipelineRun, h.CreateSnapshotAnnotationName, string(jsonResult), cl)
}

// GetPRGroupFromBuildPLR gets the PR group from the substring before @ from
// the source-branch pac annotation, for main, it generate PR group with {source-branch}-{url-org}
func GetPRGroupFromBuildPLR(pipelineRun *tektonv1.PipelineRun) string {
	if prGroup, found := pipelineRun.Annotations[consts.PipelineAsCodeSourceBranchAnnotation]; found {
		if prGroup == consts.MainBranch || prGroup == consts.MasterBranch && metadata.HasAnnotation(pipelineRun, consts.PipelineAsCodeSourceRepoOrg) {
			prGroup = prGroup + "-" + pipelineRun.Annotations[consts.PipelineAsCodeSourceRepoOrg]
		}
		return strings.Split(prGroup, "@")[0]
	}
	return ""
}

// GenerateSHA generate a 63 charactors sha string used by pipelineRun and snapshot label
func GenerateSHA(str string) string {
	hash := sha256.Sum256([]byte(str))
	return fmt.Sprintf("%x", hash)[0:62]
}

// IsPLRCreatedByPACPushEvent checks if a PLR has label PipelineAsCodeEventTypeLabel and with push or Push value
func IsPLRCreatedByPACPushEvent(plr *tektonv1.PipelineRun) bool {
	return !metadata.HasLabel(plr, consts.PipelineAsCodePullRequestLabel)
}

// IsLatestBuildPipelineRunInComponent return true if pipelineRun is the latest pipelineRun
// for its component and pr group sha. Pipeline start timestamp is used for comparison because we care about
// time when pipeline was created.
func IsLatestBuildPipelineRunInComponent(pipelineRun *tektonv1.PipelineRun, pipelineRuns *[]tektonv1.PipelineRun) bool {
	pipelineStartTime := pipelineRun.CreationTimestamp.Time
	componentName := pipelineRun.Labels[consts.PipelineRunComponentLabel]
	for _, run := range *pipelineRuns {
		if pipelineRun.Name == run.Name {
			// it's the same pipeline
			continue
		}
		if componentName != run.Labels[consts.PipelineRunComponentLabel] {
			continue
		}
		timestamp := run.CreationTimestamp.Time
		if pipelineStartTime.Before(timestamp) {
			// pipeline is not the latest
			// 1 second is minimal granularity, if both pipelines started at the same second, we cannot decide
			return false
		}
	}
	return true
}
