/*
Copyright 2026 Red Hat Inc.

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

package nudging

import (
	"context"
	"fmt"
	"strings"

	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	"github.com/konflux-ci/integration-service/api/v1beta2"
	tektonconsts "github.com/konflux-ci/integration-service/tekton/consts"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// RecordValidatedSnapshotForBatchedNudge records a passing Snapshot image digest for validated-mode batching.
// sourceRef is typically the Snapshot name (stored in accumulated.buildPipelineRun for traceability).
func RecordValidatedSnapshotForBatchedNudge(
	ctx context.Context,
	c client.Client,
	nudgeConfig *v1beta2.NudgeConfig,
	targetName string,
	sourceComponent string,
	sourceRef string,
	digest string,
) error {
	if digest == "" {
		return fmt.Errorf("digest is required for validated batched nudge target %q", targetName)
	}
	return recordSuccessForBatchedNudge(ctx, c, nudgeConfig, targetName, sourceComponent, sourceRef, digest, "")
}

// RecordFailedValidatedSnapshotForBatchedNudge records a failing Snapshot for validated-mode batching.
func RecordFailedValidatedSnapshotForBatchedNudge(
	ctx context.Context,
	c client.Client,
	nudgeConfig *v1beta2.NudgeConfig,
	targetName string,
	sourceComponent string,
	snapshotRef string,
	reason string,
) error {
	if reason == "" {
		reason = "integration tests failed"
	}
	return recordFailureForBatchedNudge(ctx, c, nudgeConfig, targetName, sourceComponent, snapshotRef, reason)
}

func buildResultForAccumulatedEntry(
	ctx context.Context,
	c client.Client,
	namespace string,
	accumulated v1beta2.AccumulatedEntry,
	sourceComponent *applicationapiv1alpha1.Component,
	fallbackPLR *tektonv1.PipelineRun,
) (*NudgeBuildResult, error) {
	buildPLR := &tektonv1.PipelineRun{}
	err := c.Get(ctx, types.NamespacedName{Name: accumulated.BuildPipelineRun, Namespace: namespace}, buildPLR)
	if err == nil {
		buildResult, err := ExtractBuildResultForNudging(buildPLR, sourceComponent)
		if err != nil {
			return nil, err
		}
		if accumulated.ImageDigest != "" {
			buildResult.Digest = accumulated.ImageDigest
		}
		return buildResult, nil
	}
	if !apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("loading build PipelineRun %q: %w", accumulated.BuildPipelineRun, err)
	}
	return buildResultFromComponentDigest(sourceComponent, accumulated.ImageDigest, fallbackPLR)
}

func buildResultFromComponentDigest(
	component *applicationapiv1alpha1.Component,
	digest string,
	fallbackPLR *tektonv1.PipelineRun,
) (*NudgeBuildResult, error) {
	repo := component.Spec.ContainerImage
	if repo == "" {
		return nil, fmt.Errorf("component %q has no container image for validated batched nudge", component.Name)
	}
	tag := ""
	if index := strings.LastIndex(repo, ":"); index != -1 {
		tag = repo[index+1:]
		repo = repo[:index]
	}
	gitRepoAtShaLink := ""
	if fallbackPLR != nil && fallbackPLR.Annotations != nil {
		gitRepoAtShaLink = fallbackPLR.Annotations[tektonconsts.GitRepoAtShaAnnotation]
	}
	nudgeFiles := tektonconsts.DefaultNudgeFiles
	if component.Annotations != nil && component.Annotations[tektonconsts.NudgeFilesAnnotation] != "" {
		nudgeFiles = component.Annotations[tektonconsts.NudgeFilesAnnotation]
	}
	return &NudgeBuildResult{
		BuiltImageRepository: repo,
		BuiltImageTag:        tag,
		Digest:               digest,
		FileMatches:          nudgeFiles,
		SourceComponentName:  component.Name,
		GitRepoAtShaLink:     gitRepoAtShaLink,
	}, nil
}
