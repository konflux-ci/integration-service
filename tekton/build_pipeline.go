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

	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	"github.com/konflux-ci/integration-service/gitops"
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
	if branch, found := plr.Annotations[consts.PipelineAsCodeSourceBranchAnnotation]; found {
		if strings.HasPrefix(strings.TrimPrefix(branch, consts.GitRefBranchPrefix), consts.PipelineAsCodeGitHubMergeQueueBranchPrefix) {
			return false
		}
	}

	return !metadata.HasLabel(plr, consts.PipelineAsCodePullRequestLabel) ||
		metadata.HasLabelWithValue(plr, consts.PipelineAsCodeEventTypeLabel, consts.PipelineAsCodePushType) ||
		metadata.HasLabelWithValue(plr, consts.PipelineAsCodeEventTypeLabel, consts.PipelineAsCodeGLPushType)
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

// MarkBuildPLRAsAddedToGlobalCandidateList updates the AddedToGlobalCandidateListAnnotation annotation for the build pipelineRun to true with reason 'Success'.
// If the patch command fails, an error will be returned.
func MarkBuildPLRAsAddedToGlobalCandidateList(ctx context.Context, adapterClient client.Client, pipelineRun *tektonv1.PipelineRun, message string) error {
	return AnnotateBuildPipelineRun(ctx, pipelineRun, gitops.AddedToGlobalCandidateListAnnotation, message, adapterClient)
}

// IsBuildPLRMarkedAsAddedToGlobalCandidateList returns true if pipelineRun's component is marked as added to global candidate list
func IsBuildPLRMarkedAsAddedToGlobalCandidateList(pipelineRun *tektonv1.PipelineRun) bool {
	annotationValue, ok := pipelineRun.GetAnnotations()[gitops.AddedToGlobalCandidateListAnnotation]
	if !ok || annotationValue == "" {
		return false
	}

	var addedToGlobalCandidateListStatus gitops.AddedToGlobalCandidateListStatus
	if err := json.Unmarshal([]byte(annotationValue), &addedToGlobalCandidateListStatus); err != nil {
		return false
	}
	return addedToGlobalCandidateListStatus.Result
}

// GetImagePullSpecFromPipelineRun gets the full image pullspec from the given build PipelineRun,
// In case the Image pullspec can't be composed, an error will be returned.
func GetImagePullSpecFromPipelineRun(pipelineRun *tektonv1.PipelineRun) (string, error) {
	outputImage, err := GetOutputImage(pipelineRun)
	if err != nil {
		return "", err
	}
	imageDigest, err := GetOutputImageDigest(pipelineRun)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s@%s", strings.Split(outputImage, ":")[0], imageDigest), nil
}

// GetComponentSourceFromPipelineRun gets the component Git Source for the Component built in the given build PipelineRun,
// In case the Git Source can't be composed, an error will be returned.
func GetComponentSourceFromPipelineRun(pipelineRun *tektonv1.PipelineRun) (*applicationapiv1alpha1.ComponentSource, error) {
	componentSourceGitUrl, err := GetComponentSourceGitUrl(pipelineRun)
	if err != nil {
		return nil, err
	}
	componentSourceGitCommit, err := GetComponentSourceGitCommit(pipelineRun)
	if err != nil {
		return nil, err
	}
	componentSource := applicationapiv1alpha1.ComponentSource{
		ComponentSourceUnion: applicationapiv1alpha1.ComponentSourceUnion{
			GitSource: &applicationapiv1alpha1.GitSource{
				URL:      componentSourceGitUrl,
				Revision: componentSourceGitCommit,
			},
		},
	}

	return &componentSource, nil
}

func GetComponentVersionFromPipelineRun(pipelineRun *tektonv1.PipelineRun) (string, error) {
	if version, found := pipelineRun.Annotations[consts.PipelineRunComponentVersionAnnotation]; found {
		return version, nil
	}
	return "", fmt.Errorf("PipelineRun '%s' in namespace '%s' does not have '%s' annotation", pipelineRun.Name, pipelineRun.Namespace, consts.PipelineRunComponentVersionAnnotation)
}
