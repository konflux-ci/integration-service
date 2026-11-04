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
	"encoding/json"
	"fmt"
	"time"

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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// ADR 0072 Phase 2 happy path: accumulate builds, fire aggregated Renovate PLR, complete on PLR success.
var _ = Describe("Nudge batch Phase 2 happy path (ADR 0072)", func() {
	const (
		namespace = "happy-path-ns"
		target    = "nudge-target"
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
				ObjectMeta: metav1.ObjectMeta{Name: "docker-secret", Namespace: namespace},
				Type:       corev1.SecretTypeDockerConfigJson,
				Data:       map[string][]byte{corev1.DockerConfigJsonKey: dockerConfigBytes},
			},
			&corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{Name: tektonconsts.DefaultPipelineServiceAccount, Namespace: namespace},
				Secrets:    []corev1.ObjectReference{{Name: "docker-secret"}},
			},
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "scm-github-secret", Namespace: namespace},
				Type:       corev1.SecretTypeBasicAuth,
				Data: map[string][]byte{
					corev1.BasicAuthUsernameKey: []byte("bot-user"),
					corev1.BasicAuthPasswordKey: []byte("ghp_secret-token"),
				},
			},
			&pacv1alpha1.Repository{
				ObjectMeta: metav1.ObjectMeta{Name: "downstream-repo", Namespace: namespace},
				Spec: pacv1alpha1.RepositorySpec{
					URL: "https://github.com/org/downstream",
					GitProvider: &pacv1alpha1.GitProvider{
						Secret: &pacv1alpha1.Secret{Name: "scm-github-secret"},
					},
				},
			},
			&applicationapiv1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{Name: target, Namespace: namespace},
				Spec: applicationapiv1alpha1.ComponentSpec{
					Application:   "test-app",
					ComponentName: target,
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
				ObjectMeta: metav1.ObjectMeta{Name: b.sourceName, Namespace: namespace},
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
				Namespace: namespace,
			},
			Spec: v1beta2.NudgeConfigSpec{
				BatchDefaults: &v1beta2.BatchDefaults{
					DebounceTimeout: &debounce,
					MaxWaitTime:     &maxWait,
				},
				TargetConfig: []v1beta2.TargetConfig{
					{Target: target, BatchPolicy: &v1beta2.BatchPolicy{}},
				},
			},
		}
	}

	buildPLRWithSA := func(name string) *tektonv1.PipelineRun {
		return &tektonv1.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
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
			Expect(RecordBuildForBatchedNudge(ctx, c, nc, target, plrs[i], buildResultFor(b.sourceName, b.digest))).To(Succeed())
		}

		accumulating := &v1beta2.NudgeConfig{}
		Expect(c.Get(ctx, types.NamespacedName{Name: nc.Name, Namespace: namespace}, accumulating)).To(Succeed())
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

		_, err := ProcessDueNudgeBatches(ctx, c, nc)
		Expect(err).NotTo(HaveOccurred())

		firing := &v1beta2.NudgeConfig{}
		Expect(c.Get(ctx, types.NamespacedName{Name: nc.Name, Namespace: namespace}, firing)).To(Succeed())
		Expect(firing.Status.ActiveBatches[0].Phase).To(Equal(v1beta2.BatchPhaseFiring))
		plrName := firing.Status.ActiveBatches[0].NudgePipelineRun
		Expect(plrName).To(Equal(BatchedNudgePipelineRunName(batch.BatchID, 0)))

		nudgePLR := &tektonv1.PipelineRun{}
		Expect(c.Get(ctx, types.NamespacedName{Name: plrName, Namespace: namespace}, nudgePLR)).To(Succeed())
		Expect(nudgePLR.Annotations[tektonconsts.NudgeBatchIDAnnotation]).To(Equal(batch.BatchID))
		Expect(nudgePLR.Annotations[tektonconsts.NudgedComponentsAnnotation]).To(Equal(target))

		var digestMap map[string]string
		Expect(json.Unmarshal([]byte(nudgePLR.Annotations[tektonconsts.NudgingDigestsAnnotation]), &digestMap)).To(Succeed())
		for _, b := range builds {
			Expect(digestMap[b.sourceName]).To(Equal(b.digest))
		}

		nudgePLR.Status.MarkSucceeded(tektonv1.PipelineRunReasonCompleted.String(), "done")
		Expect(c.Status().Update(ctx, nudgePLR)).To(Succeed())

		Expect(ProcessFiringNudgeBatches(ctx, c, nc)).To(Succeed())

		completed := &v1beta2.NudgeConfig{}
		Expect(c.Get(ctx, types.NamespacedName{Name: nc.Name, Namespace: namespace}, completed)).To(Succeed())
		Expect(completed.Status.ActiveBatches[0].Phase).To(Equal(v1beta2.BatchPhaseCompleted))
		Expect(completed.Status.ActiveBatches[0].NudgePipelineRun).To(Equal(plrName))
		Expect(completed.Status.ActiveBatches[0].Message).To(ContainSubstring("succeeded"))
	})
})
