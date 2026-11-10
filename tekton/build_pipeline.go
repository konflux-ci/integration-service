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
	"github.com/konflux-ci/integration-service/pkg/keys"
	"github.com/konflux-ci/integration-service/tekton/consts"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// AnnotateBuildPipelineRun sets annotation for a build pipelineRun in defined context and returns that pipeline
func AnnotateBuildPipelineRun(ctx context.Context, pipelineRun *tektonv1.PipelineRun, key, value string, cl client.Client) error {
	patch := client.MergeFrom(pipelineRun.DeepCopy())

	if err := gitops.SetOwnedAnnotation(pipelineRun, key, value); err != nil {
		return fmt.Errorf("failed to set annotation %s on PipelineRun %s: %w", key, pipelineRun.Name, err)
	}

	err := cl.Patch(ctx, pipelineRun, patch)
	if err != nil {
		return fmt.Errorf("failed to patch PipelineRun %s after setting annotation %s: %w", pipelineRun.Name, key, err)
	}
	return nil
}

// AppendBuildPipelineRunAnnotation sets annotation for a build pipelineRun in defined context and
// returns that pipeline. The value of the annotation is treates as a comma-separated list.  If the
// annotation already has a value then the value is appended.  Otherwise it is added with no
// preceding comma
func AppendBuildPipelineRunAnnotation(ctx context.Context, pipelineRun *tektonv1.PipelineRun, key, value string, cl client.Client) error {
	newValue := value
	if existingValue, ok := gitops.AnnotationValue(pipelineRun, key); ok && existingValue != "" {
		newValue = fmt.Sprintf("%s,%s", existingValue, value)
	}
	return AnnotateBuildPipelineRun(ctx, pipelineRun, key, newValue, cl)
}

// LabelBuildPipelineRun sets annotation for a build pipelineRun in defined context and returns that pipeline
func LabelBuildPipelineRun(ctx context.Context, pipelineRun *tektonv1.PipelineRun, key, value string, cl client.Client) error {
	patch := client.MergeFrom(pipelineRun.DeepCopy())

	if err := gitops.SetOwnedLabel(pipelineRun, key, value); err != nil {
		return fmt.Errorf("failed to set label %s on PipelineRun %s: %w", key, pipelineRun.Name, err)
	}

	err := cl.Patch(ctx, pipelineRun, patch)
	if err != nil {
		return fmt.Errorf("failed to patch PipelineRun %s after setting label %s: %w", pipelineRun.Name, key, err)
	}
	return nil
}

// AnnotateBuildPipelineRunWithCreateSnapshotAnnotation sets annotation test.appstudio.openshift.io/create-snapshot-status to build pipelineRun with
// a message that reflects either success or failure for creating a snapshot
func AnnotateBuildPipelineRunWithCreateSnapshotAnnotation(ctx context.Context, pipelineRun *tektonv1.PipelineRun, cl client.Client, ensureSnapshotExistsErr error) error {
	message := ""
	status := ""

	if ensureSnapshotExistsErr == nil {
		if !gitops.HasOwnedAnnotation(pipelineRun, consts.SnapshotNameLabel) {
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
	if prGroup, found := gitops.AnnotationValue(pipelineRun, consts.PipelineAsCodeSourceBranchAnnotation); found {
		org, orgFound := gitops.AnnotationValue(pipelineRun, consts.PipelineAsCodeSourceRepoOrg)
		if prGroup == consts.MainBranch || prGroup == consts.MasterBranch && orgFound {
			prGroup = prGroup + "-" + org
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

// IsPLRCreatedByPACPushEvent checks whether a build PipelineRun should follow
// the push integration workflow (vs pull_request).
func IsPLRCreatedByPACPushEvent(plr *tektonv1.PipelineRun) bool {
	if branch, found := gitops.AnnotationValue(plr, consts.PipelineAsCodeSourceBranchAnnotation); found {
		if strings.HasPrefix(strings.TrimPrefix(branch, consts.GitRefBranchPrefix), consts.PipelineAsCodeGitHubMergeQueueBranchPrefix) {
			return false
		}
	}
	eventType, _ := gitops.LabelValue(plr, consts.PipelineAsCodeEventTypeLabel)
	targetBranch, _ := gitops.AnnotationValue(plr, consts.PipelineAsCodeTargetBranchAnnotation)
	sourceBranch, _ := gitops.AnnotationValue(plr, consts.PipelineAsCodeSourceBranchAnnotation)
	return !gitops.HasOwnedLabel(plr, consts.PipelineAsCodePullRequestLabel) ||
		gitops.HasOwnedLabelValue(plr, consts.PipelineAsCodeEventTypeLabel, consts.PipelineAsCodePushType) ||
		gitops.HasOwnedLabelValue(plr, consts.PipelineAsCodeEventTypeLabel, consts.PipelineAsCodeGLPushType) ||
		h.IsPACPushStyleGitOpsComment(eventType, targetBranch, sourceBranch)
}

// IsLatestBuildPipelineRunInComponent return true if pipelineRun is the latest pipelineRun
// for its component and pr group sha. Pipeline start timestamp is used for comparison because we care about
// time when pipeline was created.
func IsLatestBuildPipelineRunInComponent(pipelineRun *tektonv1.PipelineRun, pipelineRuns *[]tektonv1.PipelineRun) bool {
	pipelineStartTime := pipelineRun.CreationTimestamp.Time
	componentName, _ := keys.GetLabel(pipelineRun, keys.BuildComponent())
	componentVersion, _ := GetComponentVersionFromPipelineRun(pipelineRun)

	for _, run := range *pipelineRuns {
		if pipelineRun.Name == run.Name {
			// it's the same pipeline
			continue
		}
		runComponent, _ := keys.GetLabel(&run, keys.BuildComponent())
		if componentName != runComponent {
			continue
		}
		timestamp := run.CreationTimestamp.Time
		if componentVersion != "" {
			if componentVersion == run.Annotations[gitops.SnapshotComponentVersionLabel] {
				if pipelineStartTime.Before(timestamp) {
					// pipeline is not the latest
					// 1 second is minimal granularity, if both pipelines started at the same second, we cannot decide
					return false
				}
			} else {
				continue
			}
		}
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
	annotationValue, ok := gitops.AnnotationValue(pipelineRun, gitops.AddedToGlobalCandidateListAnnotation)
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
	if ctx, ok := pipelineRun.Annotations[consts.PipelineRunComponentVersionContextAnnotation]; ok && ctx != "" {
		componentSource.GitSource.Context = ctx
	}

	return &componentSource, nil
}

func GetComponentVersionFromPipelineRun(pipelineRun *tektonv1.PipelineRun) (string, error) {
	if version, found := pipelineRun.Annotations[consts.PipelineRunComponentVersionAnnotation]; found {
		return version, nil
	}
	return "", nil
}
