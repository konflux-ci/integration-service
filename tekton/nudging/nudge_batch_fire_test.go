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
	"encoding/json"
	"fmt"
	"time"

	ghapi "github.com/google/go-github/v45/github"
	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	konfluxv1beta2 "github.com/konflux-ci/integration-service/api/konflux/v1beta2"
	"github.com/konflux-ci/integration-service/api/v1beta2"
	tektonconsts "github.com/konflux-ci/integration-service/tekton/consts"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	pacv1alpha1 "github.com/openshift-pipelines/pipelines-as-code/pkg/apis/pipelinesascode/v1alpha1"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"knative.dev/pkg/apis"
	v1 "knative.dev/pkg/apis/duck/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("Nudge batch firing integration", func() {
	const (
		namespace     = "test-ns"
		targetName    = "nudge-target"
		sourceName    = "nudge-source"
		batchID       = "fire-batch"
		sampleImage   = "quay.io/org/source:v1"
		sampleDigest  = "sha256:abc123"
		defaultSAName = tektonconsts.DefaultPipelineServiceAccount
	)

	var (
		savedNewClientFn    func(context.Context, string, []byte) (*ghapi.Client, string, error)
		savedInstallationFn func(context.Context, *ghapi.Client, string) (*applicationInstallation, error)
		savedBotIDFn        func(context.Context, string) (int64, error)
		scheme              *runtime.Scheme
	)

	BeforeEach(func() {
		savedNewClientFn = newGitHubAppClientFn
		savedInstallationFn = gitHubAppInstallationForRepo
		savedBotIDFn = getGitHubBotUserIDFn
		newGitHubAppClientFn = func(_ context.Context, _ string, _ []byte) (*ghapi.Client, string, error) {
			return nil, "my-app", nil
		}
		defaultBranch := "main"
		repoID := int64(99999)
		fakeRepo := &ghapi.Repository{DefaultBranch: &defaultBranch, ID: &repoID}
		gitHubAppInstallationForRepo = func(_ context.Context, _ *ghapi.Client, _ string) (*applicationInstallation, error) {
			return &applicationInstallation{
				Token:        "ghs_test-token",
				ID:           12345,
				Repositories: []*ghapi.Repository{fakeRepo},
			}, nil
		}
		getGitHubBotUserIDFn = func(_ context.Context, _ string) (int64, error) {
			return int64(67890), nil
		}

		scheme = runtime.NewScheme()
		Expect(v1beta2.AddToScheme(scheme)).To(Succeed())
		Expect(konfluxv1beta2.AddToScheme(scheme)).To(Succeed())
		Expect(tektonv1.AddToScheme(scheme)).To(Succeed())
		Expect(applicationapiv1alpha1.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
	})

	AfterEach(func() {
		newGitHubAppClientFn = savedNewClientFn
		gitHubAppInstallationForRepo = savedInstallationFn
		getGitHubBotUserIDFn = savedBotIDFn
	})

	batchFireClient := func(nudgeConfig *v1beta2.NudgeConfig, extra ...client.Object) client.Client {
		dockerConfig := dockerConfigJSON{
			Auths: map[string]repositoryConfigAuth{
				"quay.io/org/source": {Username: "quay-user", Password: "quay-pass"},
			},
		}
		dockerConfigBytes, err := json.Marshal(dockerConfig)
		Expect(err).NotTo(HaveOccurred())

		objects := []client.Object{
			nudgeConfig,
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      tektonconsts.PipelinesAsCodeGitHubAppSecretName,
					Namespace: "integration-service",
				},
				Data: map[string][]byte{
					tektonconsts.PipelinesAsCodeGithubAppIdKey:   []byte("12345"),
					tektonconsts.PipelinesAsCodeGithubPrivateKey: []byte("fake-key"),
				},
			},
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "docker-secret", Namespace: namespace},
				Type:       corev1.SecretTypeDockerConfigJson,
				Data:       map[string][]byte{corev1.DockerConfigJsonKey: dockerConfigBytes},
			},
			&corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{Name: defaultSAName, Namespace: namespace},
				Secrets:    []corev1.ObjectReference{{Name: "docker-secret"}},
			},
			&applicationapiv1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{Name: sourceName, Namespace: namespace},
				Spec: applicationapiv1alpha1.ComponentSpec{
					ContainerImage: sampleImage,
					Source: applicationapiv1alpha1.ComponentSource{
						ComponentSourceUnion: applicationapiv1alpha1.ComponentSourceUnion{
							GitSource: &applicationapiv1alpha1.GitSource{URL: "https://github.com/org/source-repo", Revision: "main"},
						},
					},
				},
			},
			&applicationapiv1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{Name: targetName, Namespace: namespace},
				Spec: applicationapiv1alpha1.ComponentSpec{
					Source: applicationapiv1alpha1.ComponentSource{
						ComponentSourceUnion: applicationapiv1alpha1.ComponentSourceUnion{
							GitSource: &applicationapiv1alpha1.GitSource{URL: "https://github.com/org/target-repo", Revision: "main"},
						},
					},
				},
			},
		}
		objects = append(objects, extra...)
		return fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&v1beta2.NudgeConfig{}, &tektonv1.PipelineRun{}).
			WithObjects(objects...).
			Build()
	}

	buildPLRWithResults := func(name string) *tektonv1.PipelineRun {
		return &tektonv1.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Spec: tektonv1.PipelineRunSpec{
				TaskRunTemplate: tektonv1.PipelineTaskRunTemplate{ServiceAccountName: defaultSAName},
			},
			Status: tektonv1.PipelineRunStatus{
				PipelineRunStatusFields: tektonv1.PipelineRunStatusFields{
					Results: []tektonv1.PipelineRunResult{
						{Name: "IMAGE_DIGEST", Value: *tektonv1.NewStructuredValues(sampleDigest)},
						{Name: "IMAGE_URL", Value: *tektonv1.NewStructuredValues(sampleImage)},
					},
				},
				Status: v1.Status{
					Conditions: v1.Conditions{{
						Type:   apis.ConditionSucceeded,
						Status: "True",
					}},
				},
			},
		}
	}

	dueBatchNudgeConfig := func(plrName string) *v1beta2.NudgeConfig {
		now := metav1.Now()
		pastFire := metav1.NewTime(now.Add(-time.Minute))
		plannedPLR := BatchedNudgePipelineRunName(batchID, 0)
		return &v1beta2.NudgeConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      v1beta2.NudgeConfigSingletonName,
				Namespace: namespace,
			},
			Spec: v1beta2.NudgeConfigSpec{
				TargetConfig: []v1beta2.TargetConfig{
					{Target: targetName, BatchPolicy: &v1beta2.BatchPolicy{}},
				},
			},
			Status: v1beta2.NudgeConfigStatus{
				ActiveBatches: []v1beta2.ActiveBatch{
					{
						Target:           targetName,
						BatchID:          batchID,
						Phase:            v1beta2.BatchPhaseAccumulating,
						CreatedAt:        now,
						FireAt:           &pastFire,
						HardDeadline:     metav1.NewTime(now.Add(time.Hour)),
						NudgePipelineRun: plannedPLR,
						Accumulated: []v1beta2.AccumulatedEntry{
							{
								From:                    sourceName,
								ImageDigest:             sampleDigest,
								BuildPipelineRun:        plrName,
								CapturedAt:              now,
								BuildServiceAccountName: defaultSAName,
							},
						},
					},
				},
			},
		}
	}

	It("fires a due batch and creates the batched nudge PipelineRun", func() {
		buildPLR := buildPLRWithResults("batch-build-plr")
		nudgeConfig := dueBatchNudgeConfig(buildPLR.Name)
		c := batchFireClient(nudgeConfig, buildPLR)

		err := ProcessDueNudgeBatches(ctx, c, nudgeConfig)
		Expect(err).NotTo(HaveOccurred())

		updated := &v1beta2.NudgeConfig{}
		Expect(c.Get(ctx, types.NamespacedName{Name: nudgeConfig.Name, Namespace: namespace}, updated)).To(Succeed())
		Expect(updated.Status.ActiveBatches[0].Phase).To(Equal(v1beta2.BatchPhaseFiring))
		Expect(updated.Status.ActiveBatches[0].NudgePipelineRun).To(Equal(BatchedNudgePipelineRunName(batchID, 0)))

		nudgePLR := &tektonv1.PipelineRun{}
		Expect(c.Get(ctx, types.NamespacedName{Name: updated.Status.ActiveBatches[0].NudgePipelineRun, Namespace: namespace}, nudgePLR)).To(Succeed())
		Expect(nudgePLR.Annotations[tektonconsts.NudgeBatchIDAnnotation]).To(Equal(batchID))
	})

	It("reconciles when the planned nudge PipelineRun already exists", func() {
		buildPLR := buildPLRWithResults("batch-build-plr-existing")
		nudgeConfig := dueBatchNudgeConfig(buildPLR.Name)
		plannedName := BatchedNudgePipelineRunName(batchID, 0)
		existing := &tektonv1.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{Name: plannedName, Namespace: namespace},
		}
		c := batchFireClient(nudgeConfig, buildPLR, existing)

		err := ProcessDueNudgeBatches(ctx, c, nudgeConfig)
		Expect(err).NotTo(HaveOccurred())

		updated := &v1beta2.NudgeConfig{}
		Expect(c.Get(ctx, types.NamespacedName{Name: nudgeConfig.Name, Namespace: namespace}, updated)).To(Succeed())
		Expect(updated.Status.ActiveBatches[0].Phase).To(Equal(v1beta2.BatchPhaseFiring))
		Expect(updated.Status.ActiveBatches[0].Message).To(ContainSubstring("reconciled existing nudge PipelineRun"))
	})

	It("marks a batch completed when the nudge PipelineRun succeeds", func() {
		buildPLR := buildPLRWithResults("batch-build-plr-complete")
		nudgeConfig := dueBatchNudgeConfig(buildPLR.Name)
		c := batchFireClient(nudgeConfig, buildPLR)
		err := ProcessDueNudgeBatches(ctx, c, nudgeConfig)
		Expect(err).NotTo(HaveOccurred())

		updated := &v1beta2.NudgeConfig{}
		Expect(c.Get(ctx, types.NamespacedName{Name: nudgeConfig.Name, Namespace: namespace}, updated)).To(Succeed())
		nudgePLRName := updated.Status.ActiveBatches[0].NudgePipelineRun

		nudgePLR := &tektonv1.PipelineRun{}
		Expect(c.Get(ctx, types.NamespacedName{Name: nudgePLRName, Namespace: namespace}, nudgePLR)).To(Succeed())
		now := metav1.Now()
		nudgePLR.Status.CompletionTime = &now
		nudgePLR.Status.SetCondition(&apis.Condition{Type: apis.ConditionSucceeded, Status: "True"})
		Expect(c.Status().Update(ctx, nudgePLR)).To(Succeed())

		Expect(ProcessFiringNudgeBatches(ctx, c, nudgeConfig)).To(Succeed())
		Expect(c.Get(ctx, types.NamespacedName{Name: nudgeConfig.Name, Namespace: namespace}, updated)).To(Succeed())
		Expect(updated.Status.ActiveBatches[0].Phase).To(Equal(v1beta2.BatchPhaseCompleted))
	})

	It("does not fire a batch that has no accumulated builds when the debounce timer is due", func() {
		now := metav1.Now()
		pastFire := metav1.NewTime(now.Add(-time.Minute))
		nudgeConfig := &v1beta2.NudgeConfig{
			ObjectMeta: metav1.ObjectMeta{Name: v1beta2.NudgeConfigSingletonName, Namespace: namespace},
			Status: v1beta2.NudgeConfigStatus{
				ActiveBatches: []v1beta2.ActiveBatch{{
					Target:       targetName,
					BatchID:      "empty-batch",
					Phase:        v1beta2.BatchPhaseAccumulating,
					FireAt:       &pastFire,
					HardDeadline: metav1.NewTime(now.Add(time.Hour)),
				}},
			},
		}
		c := batchFireClient(nudgeConfig)

		err := ProcessDueNudgeBatches(ctx, c, nudgeConfig)
		Expect(err).NotTo(HaveOccurred())

		updated := &v1beta2.NudgeConfig{}
		Expect(c.Get(ctx, types.NamespacedName{Name: nudgeConfig.Name, Namespace: namespace}, updated)).To(Succeed())
		Expect(updated.Status.ActiveBatches[0].Phase).To(Equal(v1beta2.BatchPhaseFailed))
		Expect(updated.Status.ActiveBatches[0].Message).To(ContainSubstring("no successful builds"))
	})

	It("force-fires an accumulating batch through spec.actions.forceFire", func() {
		buildPLR := buildPLRWithResults("force-fire-build-plr")
		now := metav1.Now()
		nudgeConfig := &v1beta2.NudgeConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      v1beta2.NudgeConfigSingletonName,
				Namespace: namespace,
			},
			Spec: v1beta2.NudgeConfigSpec{
				TargetConfig: []v1beta2.TargetConfig{
					{Target: targetName, BatchPolicy: &v1beta2.BatchPolicy{}},
				},
				Actions: &v1beta2.Actions{
					ForceFire: &v1beta2.ForceFireAction{Target: targetName},
				},
			},
			Status: v1beta2.NudgeConfigStatus{
				ActiveBatches: []v1beta2.ActiveBatch{{
					Target:           targetName,
					BatchID:          "force-batch",
					Phase:            v1beta2.BatchPhaseAccumulating,
					CreatedAt:        now,
					HardDeadline:     metav1.NewTime(now.Add(time.Hour)),
					NudgePipelineRun: BatchedNudgePipelineRunName("force-batch", 0),
					Accumulated: []v1beta2.AccumulatedEntry{{
						From:                    sourceName,
						ImageDigest:             sampleDigest,
						BuildPipelineRun:        buildPLR.Name,
						CapturedAt:              now,
						BuildServiceAccountName: defaultSAName,
					}},
				}},
			},
		}
		c := batchFireClient(nudgeConfig, buildPLR)

		Expect(ProcessForceFireAction(ctx, c, nudgeConfig)).To(Succeed())

		updated := &v1beta2.NudgeConfig{}
		Expect(c.Get(ctx, types.NamespacedName{Name: nudgeConfig.Name, Namespace: namespace}, updated)).To(Succeed())
		Expect(updated.Status.ActiveBatches[0].Phase).To(Equal(v1beta2.BatchPhaseFiring))
	})

	It("retries a failed nudge PipelineRun while the batch is still firing", func() {
		buildPLR := buildPLRWithResults("retry-build-plr")
		now := metav1.Now()
		plrName := BatchedNudgePipelineRunName("retry-batch", 0)
		failedPLR := &tektonv1.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{Name: plrName, Namespace: namespace},
			Status: tektonv1.PipelineRunStatus{
				PipelineRunStatusFields: tektonv1.PipelineRunStatusFields{CompletionTime: &now},
				Status: v1.Status{Conditions: v1.Conditions{{
					Type:   apis.ConditionSucceeded,
					Status: "False",
				}}},
			},
		}
		nudgeConfig := &v1beta2.NudgeConfig{
			ObjectMeta: metav1.ObjectMeta{Name: v1beta2.NudgeConfigSingletonName, Namespace: namespace},
			Spec: v1beta2.NudgeConfigSpec{
				TargetConfig: []v1beta2.TargetConfig{{Target: targetName, BatchPolicy: &v1beta2.BatchPolicy{}}},
			},
			Status: v1beta2.NudgeConfigStatus{
				ActiveBatches: []v1beta2.ActiveBatch{{
					Target:           targetName,
					BatchID:          "retry-batch",
					Phase:            v1beta2.BatchPhaseFiring,
					NudgePipelineRun: plrName,
					Accumulated: []v1beta2.AccumulatedEntry{{
						From:                    sourceName,
						ImageDigest:             sampleDigest,
						BuildPipelineRun:        buildPLR.Name,
						CapturedAt:              now,
						BuildServiceAccountName: defaultSAName,
					}},
				}},
			},
		}
		c := batchFireClient(nudgeConfig, buildPLR, failedPLR)

		Expect(ProcessFiringNudgeBatches(ctx, c, nudgeConfig)).To(Succeed())

		updated := &v1beta2.NudgeConfig{}
		Expect(c.Get(ctx, types.NamespacedName{Name: nudgeConfig.Name, Namespace: namespace}, updated)).To(Succeed())
		Expect(updated.Status.ActiveBatches[0].RetryCount).To(Equal(int32(1)))
		Expect(updated.Status.ActiveBatches[0].NudgePipelineRun).To(Equal(BatchedNudgePipelineRunName("retry-batch", 1)))
	})
})

// ADR 0072 Phase 2 happy path: accumulate builds, fire aggregated Renovate PLR, complete on PLR success.
var _ = Describe("Nudge batch Phase 2 happy path (ADR 0072)", func() {
	const (
		happyNamespace = "happy-path-ns"
		happyTarget    = "nudge-target"
	)

	type buildFixture struct {
		sourceName string
		plrName    string
		digest     string
	}

	builds := []buildFixture{
		{sourceName: "nudge-source-a", plrName: "build-plr-a", digest: "sha256:aaa"},
		{sourceName: "nudge-source-b", plrName: "build-plr-b", digest: "sha256:bbb"},
		{sourceName: "nudge-source-c", plrName: "build-plr-c", digest: "sha256:ccc"},
	}

	happyPathScheme := func() *runtime.Scheme {
		s := runtime.NewScheme()
		Expect(v1beta2.AddToScheme(s)).To(Succeed())
		Expect(konfluxv1beta2.AddToScheme(s)).To(Succeed())
		Expect(tektonv1.AddToScheme(s)).To(Succeed())
		Expect(corev1.AddToScheme(s)).To(Succeed())
		Expect(applicationapiv1alpha1.AddToScheme(s)).To(Succeed())
		Expect(pacv1alpha1.AddToScheme(s)).To(Succeed())
		return s
	}

	happyPathClient := func(nc *v1beta2.NudgeConfig, buildPLRs ...*tektonv1.PipelineRun) client.Client {
		dockerConfigBytes, err := json.Marshal(dockerConfigJSON{
			Auths: map[string]repositoryConfigAuth{
				"quay.io/org/source": {Username: "quay-user", Password: "quay-pass"},
			},
		})
		Expect(err).NotTo(HaveOccurred())

		objects := []client.Object{
			nc,
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "docker-secret", Namespace: happyNamespace},
				Type:       corev1.SecretTypeDockerConfigJson,
				Data:       map[string][]byte{corev1.DockerConfigJsonKey: dockerConfigBytes},
			},
			&corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{Name: tektonconsts.DefaultPipelineServiceAccount, Namespace: happyNamespace},
				Secrets:    []corev1.ObjectReference{{Name: "docker-secret"}},
			},
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "scm-github-secret", Namespace: happyNamespace},
				Type:       corev1.SecretTypeBasicAuth,
				Data: map[string][]byte{
					corev1.BasicAuthUsernameKey: []byte("bot-user"),
					corev1.BasicAuthPasswordKey: []byte("ghp_secret-token"),
				},
			},
			&pacv1alpha1.Repository{
				ObjectMeta: metav1.ObjectMeta{Name: "downstream-repo", Namespace: happyNamespace},
				Spec: pacv1alpha1.RepositorySpec{
					URL: "https://github.com/org/downstream",
					GitProvider: &pacv1alpha1.GitProvider{
						Secret: &pacv1alpha1.Secret{Name: "scm-github-secret"},
					},
				},
			},
			&applicationapiv1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{Name: happyTarget, Namespace: happyNamespace},
				Spec: applicationapiv1alpha1.ComponentSpec{
					Application:   "test-app",
					ComponentName: happyTarget,
					Source: applicationapiv1alpha1.ComponentSource{
						ComponentSourceUnion: applicationapiv1alpha1.ComponentSourceUnion{
							GitSource: &applicationapiv1alpha1.GitSource{
								URL:      "https://github.com/org/downstream",
								Revision: "main",
							},
						},
					},
				},
			},
		}

		for _, b := range builds {
			objects = append(objects, &applicationapiv1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{Name: b.sourceName, Namespace: happyNamespace},
				Spec: applicationapiv1alpha1.ComponentSpec{
					Application:    "test-app",
					ComponentName:  b.sourceName,
					ContainerImage: "quay.io/org/source:latest",
				},
			})
		}
		for _, plr := range buildPLRs {
			objects = append(objects, plr)
		}

		return fake.NewClientBuilder().
			WithScheme(happyPathScheme()).
			WithStatusSubresource(&v1beta2.NudgeConfig{}, &tektonv1.PipelineRun{}).
			WithObjects(objects...).
			Build()
	}

	newNudgeConfig := func() *v1beta2.NudgeConfig {
		debounce := metav1.Duration{Duration: time.Minute}
		maxWait := metav1.Duration{Duration: 2 * time.Hour}
		return &v1beta2.NudgeConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      v1beta2.NudgeConfigSingletonName,
				Namespace: happyNamespace,
			},
			Spec: v1beta2.NudgeConfigSpec{
				BatchDefaults: &v1beta2.BatchDefaults{
					DebounceTimeout: &debounce,
					MaxWaitTime:     &maxWait,
				},
				TargetConfig: []v1beta2.TargetConfig{
					{Target: happyTarget, BatchPolicy: &v1beta2.BatchPolicy{}},
				},
			},
		}
	}

	buildPLRWithSA := func(name string) *tektonv1.PipelineRun {
		return &tektonv1.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: happyNamespace},
			Spec: tektonv1.PipelineRunSpec{
				TaskRunTemplate: tektonv1.PipelineTaskRunTemplate{
					ServiceAccountName: tektonconsts.DefaultPipelineServiceAccount,
				},
			},
		}
	}

	buildResultFor := func(sourceName, digest string) *NudgeBuildResult {
		return &NudgeBuildResult{
			SourceComponentName:  sourceName,
			Digest:               digest,
			BuiltImageRepository: "quay.io/org/source",
			BuiltImageTag:        "latest",
			FileMatches:          tektonconsts.DefaultNudgeFiles,
		}
	}

	advanceDebounce := func(c client.Client, nc *v1beta2.NudgeConfig) {
		key := types.NamespacedName{Name: nc.Name, Namespace: nc.Namespace}
		Expect(retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			latest := &v1beta2.NudgeConfig{}
			if err := c.Get(ctx, key, latest); err != nil {
				return err
			}
			if len(latest.Status.ActiveBatches) != 1 {
				return fmt.Errorf("expected one active batch, got %d", len(latest.Status.ActiveBatches))
			}
			original := latest.DeepCopy()
			past := metav1.NewTime(time.Now().Add(-time.Minute))
			latest.Status.ActiveBatches[0].FireAt = &past
			return c.Status().Patch(ctx, latest, client.MergeFrom(original))
		})).To(Succeed())
	}

	It("should run create → accumulate → debounce → PLR → success → Completed", func() {
		nc := newNudgeConfig()
		var plrs []*tektonv1.PipelineRun
		for _, b := range builds {
			plrs = append(plrs, buildPLRWithSA(b.plrName))
		}
		c := happyPathClient(nc, plrs...)

		for i, b := range builds {
			Expect(RecordBuildForBatchedNudge(ctx, c, nc, happyTarget, plrs[i], buildResultFor(b.sourceName, b.digest))).To(Succeed())
		}

		accumulating := &v1beta2.NudgeConfig{}
		Expect(c.Get(ctx, types.NamespacedName{Name: nc.Name, Namespace: happyNamespace}, accumulating)).To(Succeed())
		Expect(accumulating.Status.ActiveBatches).To(HaveLen(1))
		batch := accumulating.Status.ActiveBatches[0]
		Expect(batch.Phase).To(Equal(v1beta2.BatchPhaseAccumulating))
		Expect(batch.Accumulated).To(HaveLen(3))
		Expect(batch.FireAt).NotTo(BeNil())
		Expect(batch.BatchID).NotTo(BeEmpty())
		for i, entry := range batch.Accumulated {
			Expect(entry.From).To(Equal(builds[i].sourceName))
			Expect(entry.ImageDigest).To(Equal(builds[i].digest))
			Expect(entry.BuildPipelineRun).To(Equal(builds[i].plrName))
		}

		advanceDebounce(c, nc)

		err := ProcessDueNudgeBatches(ctx, c, nc)
		Expect(err).NotTo(HaveOccurred())

		firing := &v1beta2.NudgeConfig{}
		Expect(c.Get(ctx, types.NamespacedName{Name: nc.Name, Namespace: happyNamespace}, firing)).To(Succeed())
		Expect(firing.Status.ActiveBatches[0].Phase).To(Equal(v1beta2.BatchPhaseFiring))
		plrName := firing.Status.ActiveBatches[0].NudgePipelineRun
		Expect(plrName).To(Equal(BatchedNudgePipelineRunName(batch.BatchID, 0)))

		nudgePLR := &tektonv1.PipelineRun{}
		Expect(c.Get(ctx, types.NamespacedName{Name: plrName, Namespace: happyNamespace}, nudgePLR)).To(Succeed())
		Expect(nudgePLR.Annotations[tektonconsts.NudgeBatchIDAnnotation]).To(Equal(batch.BatchID))
		Expect(nudgePLR.Annotations[tektonconsts.NudgedComponentsAnnotation]).To(Equal(happyTarget))

		var digestMap map[string]string
		Expect(json.Unmarshal([]byte(nudgePLR.Annotations[tektonconsts.NudgingDigestsAnnotation]), &digestMap)).To(Succeed())
		for _, b := range builds {
			Expect(digestMap[b.sourceName]).To(Equal(b.digest))
		}

		nudgePLR.Status.MarkSucceeded(tektonv1.PipelineRunReasonCompleted.String(), "done")
		Expect(c.Status().Update(ctx, nudgePLR)).To(Succeed())

		Expect(ProcessFiringNudgeBatches(ctx, c, nc)).To(Succeed())

		completed := &v1beta2.NudgeConfig{}
		Expect(c.Get(ctx, types.NamespacedName{Name: nc.Name, Namespace: happyNamespace}, completed)).To(Succeed())
		Expect(completed.Status.ActiveBatches[0].Phase).To(Equal(v1beta2.BatchPhaseCompleted))
		Expect(completed.Status.ActiveBatches[0].NudgePipelineRun).To(Equal(plrName))
		Expect(completed.Status.ActiveBatches[0].Message).To(ContainSubstring("succeeded"))

		// KFLUXSE-490: completed batches past the 24h TTL are pruned on the next pass.
		key := types.NamespacedName{Name: nc.Name, Namespace: happyNamespace}
		Expect(retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			latest := &v1beta2.NudgeConfig{}
			if err := c.Get(ctx, key, latest); err != nil {
				return err
			}
			original := latest.DeepCopy()
			expired := metav1.NewTime(time.Now().Add(-25 * time.Hour))
			latest.Status.ActiveBatches[0].CreatedAt = expired
			return c.Status().Patch(ctx, latest, client.MergeFrom(original))
		})).To(Succeed())
		Expect(PruneCompletedBatches(ctx, c, nc)).To(Succeed())

		pruned := &v1beta2.NudgeConfig{}
		Expect(c.Get(ctx, key, pruned)).To(Succeed())
		Expect(pruned.Status.ActiveBatches).To(BeEmpty())
	})
})
