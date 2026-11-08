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

package nudging_test

import (
	"context"
	"errors"
	"strings"

	tektonconsts "github.com/konflux-ci/integration-service/tekton/consts"
	nudging "github.com/konflux-ci/integration-service/tekton/nudging"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

// STONEINTG-1907: the nudge Secret and ConfigMaps must be garbage-collected together with the
// nudge PipelineRun, so they carry an owner reference to it. The PipelineRun is therefore created
// first, pending, and started only after the support objects exist.

const ownershipNamespace = "test-ns"

// nudgeCreateRecorder records, in order, the creates and the start patch that the nudge builders
// issue, and whether the PipelineRun was still pending when each support object was created. It
// also injects the failures the repair tests need.
type nudgeCreateRecorder struct {
	events          []string
	plrName         string
	pendingAtCreate map[string]bool
	failPatch       bool
	failCreateKind  string
}

func newNudgeCreateRecorder() *nudgeCreateRecorder {
	return &nudgeCreateRecorder{pendingAtCreate: map[string]bool{}}
}

func (r *nudgeCreateRecorder) pipelineRunPending(ctx context.Context, c client.Client) bool {
	if r.plrName == "" {
		return false
	}
	plr := &tektonv1.PipelineRun{}
	if err := c.Get(ctx, types.NamespacedName{Name: r.plrName, Namespace: ownershipNamespace}, plr); err != nil {
		return false
	}
	return plr.IsPending()
}

func (r *nudgeCreateRecorder) record(ctx context.Context, c client.Client, event string) {
	r.events = append(r.events, event)
	r.pendingAtCreate[event] = r.pipelineRunPending(ctx, c)
}

// learnOwner notes the PipelineRun name from an owner reference. The specs that pre-create the
// PipelineRun never intercept a PipelineRun create, so the support objects' owner references are
// the only place the recorder can learn the name it needs to test IsPending.
func (r *nudgeCreateRecorder) learnOwner(obj client.Object) {
	if r.plrName != "" {
		return
	}
	for _, ref := range obj.GetOwnerReferences() {
		if ref.Kind == "PipelineRun" && ref.Name != "" {
			r.plrName = ref.Name
			return
		}
	}
}

// client wraps base. uid is assigned to the PipelineRun on create because the fake client does
// not, and an owner reference without a UID is useless to the garbage collector.
func (r *nudgeCreateRecorder) client(base client.WithWatch, uid types.UID) client.WithWatch {
	return interceptor.NewClient(base, interceptor.Funcs{
		Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
			r.learnOwner(obj)
			switch o := obj.(type) {
			case *tektonv1.PipelineRun:
				if o.UID == "" {
					o.UID = uid
				}
				if r.failCreateKind == "PipelineRun" {
					return errors.New("injected PipelineRun create failure")
				}
				if err := c.Create(ctx, obj, opts...); err != nil {
					return err
				}
				r.plrName = o.Name
				r.record(ctx, c, "create-pipelinerun")
				return nil
			case *corev1.Secret:
				r.record(ctx, c, "create-secret")
				if r.failCreateKind == "Secret" {
					return errors.New("injected Secret create failure")
				}
			case *corev1.ConfigMap:
				// The CA ConfigMap is the only one whose name starts with "renovate-ca-";
				// the renovate config ConfigMap is named after the PipelineRun.
				if strings.HasPrefix(o.Name, "renovate-ca-") {
					r.record(ctx, c, "create-ca-configmap")
					if r.failCreateKind == "CAConfigMap" {
						return errors.New("injected CA ConfigMap create failure")
					}
				} else {
					r.record(ctx, c, "create-configmap")
					if r.failCreateKind == "ConfigMap" {
						return errors.New("injected ConfigMap create failure")
					}
				}
			}
			return c.Create(ctx, obj, opts...)
		},
		Patch: func(ctx context.Context, c client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
			r.events = append(r.events, "start-pipelinerun")
			if r.failPatch {
				return errors.New("injected patch failure")
			}
			return c.Patch(ctx, obj, patch, opts...)
		},
	})
}

// failingGetClient makes Get fail for the objects want selects, to exercise the read-back error
// paths that a real API server can return but a fake client never does.
func failingGetClient(base client.WithWatch, want func(client.Object) bool, err error) client.WithWatch {
	return interceptor.NewClient(base, interceptor.Funcs{
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			if want(obj) {
				return err
			}
			return c.Get(ctx, key, obj, opts...)
		},
	})
}

// rewritingGetClient lets rewrite change what a Get returns after it succeeds, to simulate another
// writer changing the PipelineRun between two reads.
func rewritingGetClient(base client.WithWatch, rewrite func(client.Object)) client.WithWatch {
	return interceptor.NewClient(base, interceptor.Funcs{
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			if err := c.Get(ctx, key, obj, opts...); err != nil {
				return err
			}
			rewrite(obj)
			return nil
		},
	})
}

func isPipelineRun(obj client.Object) bool {
	_, ok := obj.(*tektonv1.PipelineRun)
	return ok
}

func isSecret(obj client.Object) bool {
	_, ok := obj.(*corev1.Secret)
	return ok
}

func ownershipFakeClient(objects ...client.Object) client.WithWatch {
	return fake.NewClientBuilder().
		WithScheme(nudgeTestScheme()).
		WithObjects(objects...).
		Build()
}

func ownershipPipelineServiceAccount() *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: tektonconsts.DefaultPipelineServiceAccount, Namespace: ownershipNamespace},
	}
}

func ownershipBuildPLR() *tektonv1.PipelineRun {
	return &tektonv1.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{Name: "build-plr-source", Namespace: ownershipNamespace},
	}
}

func ownershipTargets() []nudging.NudgeTarget {
	return []nudging.NudgeTarget{
		{
			ComponentName:           "downstream-comp",
			GitProvider:             "github",
			Username:                "test-user",
			GitAuthor:               "Test <test@example.com>",
			Token:                   "ghp_token",
			Endpoint:                "https://api.github.com",
			Repositories:            []nudging.RenovateRepository{{Repository: "org/downstream"}},
			ImageRepositoryHost:     "quay.io",
			ImageRepositoryUsername: "robot",
			ImageRepositoryPassword: "secret",
		},
	}
}

func ownershipBuildResult() *nudging.NudgeBuildResult {
	return &nudging.NudgeBuildResult{
		BuiltImageRepository: "quay.io/org/image",
		BuiltImageTag:        "v1",
		Digest:               "sha256:aaa",
		FileMatches:          ".*.yaml",
		SourceComponentName:  "source-comp",
		GitRepoAtShaLink:     "https://github.com/org/repo/commit/aabbccddaabbccddaabbccddaabbccddaabbccdd",
	}
}

func ownershipBatchedTarget() nudging.NudgeTarget {
	return nudging.NudgeTarget{
		ComponentName:           "downstream-comp",
		GitProvider:             "github",
		Username:                "test-user",
		GitAuthor:               "Test <test@example.com>",
		Token:                   "ghp_token",
		Endpoint:                "https://api.github.com",
		Repositories:            []nudging.RenovateRepository{{Repository: "org/downstream"}},
		ImageRepositoryHost:     "quay.io",
		ImageRepositoryUsername: "robot",
		ImageRepositoryPassword: "secret",
	}
}

func ownershipBatchedEntries() []nudging.BatchedRenovateEntry {
	return []nudging.BatchedRenovateEntry{{
		BuildResult: &nudging.NudgeBuildResult{
			BuiltImageRepository: "quay.io/org/image-a",
			BuiltImageTag:        "v1",
			Digest:               "sha256:aaa",
			FileMatches:          ".*.yaml",
			SourceComponentName:  "source-a",
			GitRepoAtShaLink:     "https://github.com/org/repo/commit/aabbccddaabbccddaabbccddaabbccddaabbccdd",
		},
	}}
}

func ownershipCABundleConfigMap() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "trusted-ca-bundle",
			Namespace: "integration-service",
			Labels:    map[string]string{tektonconsts.CaConfigMapLabel: "true"},
		},
		Data: map[string]string{
			tektonconsts.CaConfigMapKey: "-----BEGIN CERTIFICATE-----\nfake\n-----END CERTIFICATE-----",
		},
	}
}

func ownershipGetPLR(c client.Client, name string) *tektonv1.PipelineRun {
	plr := &tektonv1.PipelineRun{}
	Expect(c.Get(context.Background(), types.NamespacedName{Name: name, Namespace: ownershipNamespace}, plr)).To(Succeed())
	return plr
}

func ownershipGetSecret(c client.Client, name string) *corev1.Secret {
	secret := &corev1.Secret{}
	Expect(c.Get(context.Background(), types.NamespacedName{Name: name, Namespace: ownershipNamespace}, secret)).To(Succeed())
	return secret
}

func ownershipGetConfigMap(c client.Client, name string) *corev1.ConfigMap {
	cm := &corev1.ConfigMap{}
	Expect(c.Get(context.Background(), types.NamespacedName{Name: name, Namespace: ownershipNamespace}, cm)).To(Succeed())
	return cm
}

// expectPlainOwnerRef asserts obj carries exactly one owner reference to the nudge PipelineRun
// and that it is a plain reference: no controller flag and no blockOwnerDeletion, both of which
// would need permissions integration-service is not granted.
func expectPlainOwnerRef(refs []metav1.OwnerReference, owner *tektonv1.PipelineRun) {
	Expect(refs).To(HaveLen(1))
	ref := refs[0]
	Expect(ref.APIVersion).To(Equal("tekton.dev/v1"))
	Expect(ref.Kind).To(Equal("PipelineRun"))
	Expect(ref.Name).To(Equal(owner.Name))
	Expect(ref.UID).To(Equal(owner.UID))
	Expect(ref.UID).NotTo(BeEmpty())
	Expect(ref.Controller).To(BeNil())
	Expect(ref.BlockOwnerDeletion).To(BeNil())
}

var _ = Describe("Nudge support object ownership (STONEINTG-1907)", func() {
	var testCtx context.Context

	BeforeEach(func() {
		testCtx = context.Background()
	})

	Describe("CreateNudgePipelineRun", func() {
		var buildPLR *tektonv1.PipelineRun

		BeforeEach(func() {
			buildPLR = ownershipBuildPLR()
		})

		create := func(c client.Client) error {
			return nudging.CreateNudgePipelineRun(testCtx, c, buildPLR, ownershipTargets(), ownershipBuildResult(), false)
		}

		It("creates the PipelineRun pending, creates the owned support objects, then starts it", func() {
			base := ownershipFakeClient(ownershipPipelineServiceAccount())
			rec := newNudgeCreateRecorder()

			Expect(create(rec.client(base, "uid-nudge"))).To(Succeed())

			Expect(rec.events).To(Equal([]string{
				"create-pipelinerun", "create-secret", "create-configmap", "start-pipelinerun",
			}))
			Expect(rec.pendingAtCreate["create-secret"]).To(BeTrue(), "Secret must be created while the PipelineRun is pending")
			Expect(rec.pendingAtCreate["create-configmap"]).To(BeTrue(), "ConfigMap must be created while the PipelineRun is pending")

			name := nudging.NudgePipelineRunNameForBuild(buildPLR)
			plr := ownershipGetPLR(base, name)
			Expect(plr.UID).To(Equal(types.UID("uid-nudge")))
			Expect(plr.Spec.Status).To(BeEmpty(), "the PipelineRun must be started once its support objects exist")

			expectPlainOwnerRef(ownershipGetSecret(base, name).OwnerReferences, plr)
			expectPlainOwnerRef(ownershipGetConfigMap(base, name).OwnerReferences, plr)
		})

		It("gives the CA ConfigMap the same plain owner reference to the PipelineRun", func() {
			base := ownershipFakeClient(ownershipPipelineServiceAccount(), ownershipCABundleConfigMap())
			rec := newNudgeCreateRecorder()

			Expect(create(rec.client(base, "uid-nudge"))).To(Succeed())

			Expect(rec.events).To(Equal([]string{
				"create-pipelinerun", "create-secret", "create-configmap", "create-ca-configmap", "start-pipelinerun",
			}))
			Expect(rec.pendingAtCreate["create-ca-configmap"]).To(BeTrue())

			plr := ownershipGetPLR(base, nudging.NudgePipelineRunNameForBuild(buildPLR))
			caConfigMaps := &corev1.ConfigMapList{}
			Expect(base.List(testCtx, caConfigMaps, client.InNamespace(ownershipNamespace))).To(Succeed())
			Expect(caConfigMaps.Items).To(HaveLen(2))
			for _, cm := range caConfigMaps.Items {
				if _, isConfig := cm.Data[tektonconsts.CaConfigMapKey]; !isConfig {
					continue
				}
				expectPlainOwnerRef(cm.OwnerReferences, plr)
			}
		})

		It("is idempotent: the second call creates nothing new and leaves the started PipelineRun alone", func() {
			base := ownershipFakeClient(ownershipPipelineServiceAccount())
			rec := newNudgeCreateRecorder()
			c := rec.client(base, "uid-nudge")

			Expect(create(c)).To(Succeed())
			eventsAfterFirstCall := append([]string(nil), rec.events...)
			Expect(create(c)).To(Succeed())

			// The support objects are created rather than looked up, so the second call repeats
			// those two creates and learns from AlreadyExists that they exist. What it must not do
			// is create a second PipelineRun or start the one already running.
			Expect(rec.events[len(eventsAfterFirstCall):]).To(Equal([]string{"create-secret", "create-configmap"}))

			plrList := &tektonv1.PipelineRunList{}
			Expect(base.List(testCtx, plrList, client.InNamespace(ownershipNamespace))).To(Succeed())
			Expect(plrList.Items).To(HaveLen(1))
			secretList := &corev1.SecretList{}
			Expect(base.List(testCtx, secretList, client.InNamespace(ownershipNamespace))).To(Succeed())
			Expect(secretList.Items).To(HaveLen(1))
			cmList := &corev1.ConfigMapList{}
			Expect(base.List(testCtx, cmList, client.InNamespace(ownershipNamespace))).To(Succeed())
			Expect(cmList.Items).To(HaveLen(1))

			Expect(plrList.Items[0].Spec.Status).To(BeEmpty())
		})

		It("adopts and starts a pending PipelineRun left by an earlier attempt", func() {
			name := nudging.NudgePipelineRunNameForBuild(buildPLR)
			pending := &tektonv1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ownershipNamespace, UID: "uid-existing"},
				Spec:       tektonv1.PipelineRunSpec{Status: tektonv1.PipelineRunSpecStatusPending},
			}
			base := ownershipFakeClient(ownershipPipelineServiceAccount(), pending)
			rec := newNudgeCreateRecorder()

			Expect(create(rec.client(base, "uid-nudge"))).To(Succeed())

			Expect(rec.events).To(Equal([]string{"create-secret", "create-configmap", "start-pipelinerun"}))
			Expect(rec.pendingAtCreate["create-secret"]).To(BeTrue())

			plr := ownershipGetPLR(base, name)
			Expect(plr.UID).To(Equal(types.UID("uid-existing")))
			Expect(plr.Spec.Status).To(BeEmpty())
			expectPlainOwnerRef(ownershipGetSecret(base, name).OwnerReferences, plr)
		})

		It("does not patch a PipelineRun that is already running", func() {
			name := nudging.NudgePipelineRunNameForBuild(buildPLR)
			running := &tektonv1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ownershipNamespace, UID: "uid-existing"},
			}
			base := ownershipFakeClient(ownershipPipelineServiceAccount(), running)
			rec := newNudgeCreateRecorder()

			Expect(create(rec.client(base, "uid-nudge"))).To(Succeed())

			Expect(rec.events).To(Equal([]string{"create-secret", "create-configmap"}))
			Expect(ownershipGetPLR(base, name).Spec.Status).To(BeEmpty())
		})

		It("tolerates a support object that already exists without an owner reference", func() {
			name := nudging.NudgePipelineRunNameForBuild(buildPLR)
			unowned := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ownershipNamespace},
				StringData: map[string]string{"token_0": "leftover"},
			}
			base := ownershipFakeClient(ownershipPipelineServiceAccount(), unowned)
			rec := newNudgeCreateRecorder()

			Expect(create(rec.client(base, "uid-nudge"))).To(Succeed())

			plr := ownershipGetPLR(base, name)
			Expect(plr.Spec.Status).To(BeEmpty(), "the nudge must still run")
			Expect(ownershipGetSecret(base, name).OwnerReferences).To(BeEmpty(), "an unowned leftover is left untouched; it cannot be adopted without delete permission")
			expectPlainOwnerRef(ownershipGetConfigMap(base, name).OwnerReferences, plr)
		})

		It("leaves a support object owned by something else alone", func() {
			name := nudging.NudgePipelineRunNameForBuild(buildPLR)
			foreign := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: ownershipNamespace,
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion: "appstudio.redhat.com/v1alpha1",
						Kind:       "Component",
						Name:       "some-component",
						UID:        "uid-component",
					}},
				},
			}
			base := ownershipFakeClient(ownershipPipelineServiceAccount(), foreign)
			rec := newNudgeCreateRecorder()

			Expect(create(rec.client(base, "uid-nudge"))).To(Succeed())

			Expect(ownershipGetPLR(base, name).Spec.Status).To(BeEmpty(), "the nudge must still run")
			owners := ownershipGetSecret(base, name).OwnerReferences
			Expect(owners).To(HaveLen(1))
			Expect(owners[0].Kind).To(Equal("Component"), "an object owned by something else is not ours to adopt")
		})

		It("leaves the PipelineRun pending when the start patch fails, and a retry starts it", func() {
			base := ownershipFakeClient(ownershipPipelineServiceAccount())
			rec := newNudgeCreateRecorder()
			c := rec.client(base, "uid-nudge")

			rec.failPatch = true
			Expect(create(c)).To(MatchError(ContainSubstring("starting nudge PipelineRun")))

			name := nudging.NudgePipelineRunNameForBuild(buildPLR)
			Expect(ownershipGetPLR(base, name).IsPending()).To(BeTrue())

			rec.failPatch = false
			Expect(create(c)).To(Succeed())
			Expect(ownershipGetPLR(base, name).Spec.Status).To(BeEmpty())
		})

		It("leaves the PipelineRun pending when a support object cannot be created, and a retry repairs it", func() {
			base := ownershipFakeClient(ownershipPipelineServiceAccount())
			rec := newNudgeCreateRecorder()
			c := rec.client(base, "uid-nudge")

			rec.failCreateKind = "Secret"
			Expect(create(c)).To(MatchError(ContainSubstring("creating Secret")))

			name := nudging.NudgePipelineRunNameForBuild(buildPLR)
			Expect(ownershipGetPLR(base, name).IsPending()).To(BeTrue())

			rec.failCreateKind = ""
			Expect(create(c)).To(Succeed())

			plr := ownershipGetPLR(base, name)
			Expect(plr.Spec.Status).To(BeEmpty())
			expectPlainOwnerRef(ownershipGetSecret(base, name).OwnerReferences, plr)
			expectPlainOwnerRef(ownershipGetConfigMap(base, name).OwnerReferences, plr)
		})

		It("refuses a support object owned by a previous PipelineRun of the same name", func() {
			name := nudging.NudgePipelineRunNameForBuild(buildPLR)
			stale := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: ownershipNamespace,
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion: "tekton.dev/v1",
						Kind:       "PipelineRun",
						Name:       name,
						UID:        "uid-previous",
					}},
				},
			}
			base := ownershipFakeClient(ownershipPipelineServiceAccount(), stale)
			rec := newNudgeCreateRecorder()

			err := create(rec.client(base, "uid-nudge"))
			Expect(err).To(MatchError(ContainSubstring("owned by a previous PipelineRun of the same name")))
			Expect(ownershipGetPLR(base, name).IsPending()).To(BeTrue(), "the PipelineRun must not start while a stale dependent is still present")
		})

		It("returns an error when the nudge PipelineRun cannot be created", func() {
			base := ownershipFakeClient(ownershipPipelineServiceAccount())
			rec := newNudgeCreateRecorder()
			rec.failCreateKind = "PipelineRun"

			Expect(create(rec.client(base, "uid-nudge"))).To(MatchError(ContainSubstring("creating or loading nudge PipelineRun")))
		})

		It("returns an error when the renovate ConfigMap cannot be created", func() {
			base := ownershipFakeClient(ownershipPipelineServiceAccount())
			rec := newNudgeCreateRecorder()
			rec.failCreateKind = "ConfigMap"

			Expect(create(rec.client(base, "uid-nudge"))).To(MatchError(ContainSubstring("creating ConfigMap")))
			Expect(ownershipGetPLR(base, nudging.NudgePipelineRunNameForBuild(buildPLR)).IsPending()).To(BeTrue(),
				"a support object failure must leave the PipelineRun pending so a retry can repair it")
		})

		It("returns an error when the CA ConfigMap cannot be created", func() {
			base := ownershipFakeClient(ownershipPipelineServiceAccount(), ownershipCABundleConfigMap())
			rec := newNudgeCreateRecorder()
			rec.failCreateKind = "CAConfigMap"

			Expect(create(rec.client(base, "uid-nudge"))).To(MatchError(ContainSubstring("creating CA ConfigMap")))
		})

		It("returns an error when an existing PipelineRun cannot be read back", func() {
			name := nudging.NudgePipelineRunNameForBuild(buildPLR)
			existing := &tektonv1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ownershipNamespace, UID: "uid-existing"},
			}
			base := ownershipFakeClient(ownershipPipelineServiceAccount(), existing)
			c := failingGetClient(base, isPipelineRun, errors.New("injected PipelineRun read failure"))

			Expect(create(c)).To(MatchError(ContainSubstring("injected PipelineRun read failure")))
		})

		It("returns an error when the existing PipelineRun is being deleted", func() {
			name := nudging.NudgePipelineRunNameForBuild(buildPLR)
			existing := &tektonv1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name: name, Namespace: ownershipNamespace, UID: "uid-existing",
					Finalizers: []string{"test/keep"},
				},
			}
			base := ownershipFakeClient(ownershipPipelineServiceAccount(), existing)
			Expect(base.Delete(testCtx, existing)).To(Succeed())

			Expect(create(base)).To(MatchError(ContainSubstring("is being deleted")))
		})

		It("returns an error when an existing support object cannot be read back", func() {
			name := nudging.NudgePipelineRunNameForBuild(buildPLR)
			existing := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ownershipNamespace}}
			base := ownershipFakeClient(ownershipPipelineServiceAccount(), existing)
			c := failingGetClient(base, isSecret, errors.New("injected Secret read failure"))

			Expect(create(c)).To(MatchError(ContainSubstring("injected Secret read failure")))
		})

		It("returns an error when the pending PipelineRun cannot be read before starting", func() {
			base := ownershipFakeClient(ownershipPipelineServiceAccount())
			rec := newNudgeCreateRecorder()
			c := failingGetClient(rec.client(base, "uid-nudge"), isPipelineRun, errors.New("injected PipelineRun read failure"))

			Expect(create(c)).To(MatchError(ContainSubstring("starting nudge PipelineRun")))
		})

		It("does not start a PipelineRun another writer already started", func() {
			base := ownershipFakeClient(ownershipPipelineServiceAccount())
			rec := newNudgeCreateRecorder()
			c := rewritingGetClient(rec.client(base, "uid-nudge"), func(obj client.Object) {
				if plr, ok := obj.(*tektonv1.PipelineRun); ok {
					plr.Spec.Status = ""
				}
			})

			Expect(create(c)).To(Succeed())
			Expect(rec.events).NotTo(ContainElement("start-pipelinerun"))
		})

		It("returns an error when the pending PipelineRun is being deleted before starting", func() {
			base := ownershipFakeClient(ownershipPipelineServiceAccount())
			rec := newNudgeCreateRecorder()
			c := rewritingGetClient(rec.client(base, "uid-nudge"), func(obj client.Object) {
				if plr, ok := obj.(*tektonv1.PipelineRun); ok {
					now := metav1.Now()
					plr.DeletionTimestamp = &now
				}
			})

			Expect(create(c)).To(MatchError(ContainSubstring("is being deleted")))
		})
	})

	Describe("CreateBatchedNudgePipelineRun", func() {
		var referencePLR *tektonv1.PipelineRun

		BeforeEach(func() {
			referencePLR = &tektonv1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{Name: "build-plr-source", Namespace: ownershipNamespace},
			}
		})

		create := func(c client.Client, batchID string, retry int32) (string, error) {
			return nudging.CreateBatchedNudgePipelineRun(testCtx, c, referencePLR, ownershipBatchedTarget(), ownershipBatchedEntries(), batchID, retry)
		}

		It("creates the PipelineRun pending, creates the owned support objects, then starts it", func() {
			base := ownershipFakeClient(ownershipPipelineServiceAccount())
			rec := newNudgeCreateRecorder()

			name, err := create(rec.client(base, "uid-batch"), "ownership-batch", 0)
			Expect(err).NotTo(HaveOccurred())
			Expect(name).To(Equal("nudge-batch-ownership-batch"))

			Expect(rec.events).To(Equal([]string{
				"create-pipelinerun", "create-secret", "create-configmap", "start-pipelinerun",
			}))
			Expect(rec.pendingAtCreate["create-secret"]).To(BeTrue())
			Expect(rec.pendingAtCreate["create-configmap"]).To(BeTrue())

			plr := ownershipGetPLR(base, name)
			Expect(plr.UID).To(Equal(types.UID("uid-batch")))
			Expect(plr.Spec.Status).To(BeEmpty())
			expectPlainOwnerRef(ownershipGetSecret(base, name).OwnerReferences, plr)
			expectPlainOwnerRef(ownershipGetConfigMap(base, name).OwnerReferences, plr)
		})

		It("gives each retry attempt its own CA ConfigMap, owned by and mounted in its own PipelineRun", func() {
			base := ownershipFakeClient(ownershipPipelineServiceAccount(), ownershipCABundleConfigMap())
			rec := newNudgeCreateRecorder()

			first, err := create(rec.client(base, "uid-first"), "ca-ownership-batch", 0)
			Expect(err).NotTo(HaveOccurred())
			second, err := create(rec.client(base, "uid-retry"), "ca-ownership-batch", 1)
			Expect(err).NotTo(HaveOccurred())
			Expect(second).NotTo(Equal(first))

			caNames := map[string]string{}
			for _, batchName := range []string{first, second} {
				plr := ownershipGetPLR(base, batchName)
				var caName string
				for _, volume := range plr.Spec.PipelineSpec.Tasks[0].TaskSpec.Volumes {
					if volume.Name == tektonconsts.CaVolumeMountName && volume.ConfigMap != nil {
						caName = volume.ConfigMap.Name
					}
				}
				Expect(caName).NotTo(BeEmpty(), "each attempt must mount a CA ConfigMap")
				caNames[batchName] = caName
				expectPlainOwnerRef(ownershipGetConfigMap(base, caName).OwnerReferences, plr)
			}
			Expect(caNames[first]).NotTo(Equal(caNames[second]), "a shared CA ConfigMap would be deleted under a running retry")
		})

		It("adopts and starts a pending retry PipelineRun left by an earlier attempt", func() {
			name := nudging.BatchedNudgePipelineRunName("adopt-batch", 1)
			pending := &tektonv1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ownershipNamespace, UID: "uid-existing"},
				Spec:       tektonv1.PipelineRunSpec{Status: tektonv1.PipelineRunSpecStatusPending},
			}
			base := ownershipFakeClient(ownershipPipelineServiceAccount(), pending)
			rec := newNudgeCreateRecorder()

			created, err := create(rec.client(base, "uid-retry"), "adopt-batch", 1)
			Expect(err).NotTo(HaveOccurred())
			Expect(created).To(Equal(name))

			Expect(rec.events).To(Equal([]string{"create-secret", "create-configmap", "start-pipelinerun"}))
			Expect(rec.pendingAtCreate["create-secret"]).To(BeTrue())

			plr := ownershipGetPLR(base, name)
			Expect(plr.UID).To(Equal(types.UID("uid-existing")))
			Expect(plr.Spec.Status).To(BeEmpty())
			expectPlainOwnerRef(ownershipGetSecret(base, name).OwnerReferences, plr)
		})

		It("does not patch a PipelineRun that is already running", func() {
			name := nudging.BatchedNudgePipelineRunName("running-batch", 0)
			running := &tektonv1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ownershipNamespace, UID: "uid-existing"},
			}
			base := ownershipFakeClient(ownershipPipelineServiceAccount(), running)
			rec := newNudgeCreateRecorder()

			created, err := create(rec.client(base, "uid-batch"), "running-batch", 0)
			Expect(err).NotTo(HaveOccurred())
			Expect(created).To(Equal(name))

			Expect(rec.events).To(Equal([]string{"create-secret", "create-configmap"}))
		})

		It("tolerates a support object that already exists without an owner reference", func() {
			name := nudging.BatchedNudgePipelineRunName("leftover-batch", 0)
			unowned := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ownershipNamespace},
				Data:       map[string]string{"downstream-comp-batch.json": "{}"},
			}
			base := ownershipFakeClient(ownershipPipelineServiceAccount(), unowned)
			rec := newNudgeCreateRecorder()

			created, err := create(rec.client(base, "uid-batch"), "leftover-batch", 0)
			Expect(err).NotTo(HaveOccurred())
			Expect(created).To(Equal(name))

			plr := ownershipGetPLR(base, name)
			Expect(plr.Spec.Status).To(BeEmpty())
			Expect(ownershipGetConfigMap(base, name).OwnerReferences).To(BeEmpty())
			expectPlainOwnerRef(ownershipGetSecret(base, name).OwnerReferences, plr)
		})

		It("returns an error when the batched PipelineRun cannot be created", func() {
			base := ownershipFakeClient(ownershipPipelineServiceAccount())
			rec := newNudgeCreateRecorder()
			rec.failCreateKind = "PipelineRun"

			_, err := create(rec.client(base, "uid-batch"), "failing-batch", 0)
			Expect(err).To(MatchError(ContainSubstring("creating or loading batched nudge PipelineRun")))
		})

		It("returns an error when the batched Secret cannot be created", func() {
			base := ownershipFakeClient(ownershipPipelineServiceAccount())
			rec := newNudgeCreateRecorder()
			rec.failCreateKind = "Secret"

			_, err := create(rec.client(base, "uid-batch"), "failing-batch", 0)
			Expect(err).To(MatchError(ContainSubstring("creating Secret")))
		})

		It("returns an error when the batched ConfigMap cannot be created", func() {
			base := ownershipFakeClient(ownershipPipelineServiceAccount())
			rec := newNudgeCreateRecorder()
			rec.failCreateKind = "ConfigMap"

			_, err := create(rec.client(base, "uid-batch"), "failing-batch", 0)
			Expect(err).To(MatchError(ContainSubstring("creating ConfigMap")))
		})

		It("returns an error when the batched CA ConfigMap cannot be created", func() {
			base := ownershipFakeClient(ownershipPipelineServiceAccount(), ownershipCABundleConfigMap())
			rec := newNudgeCreateRecorder()
			rec.failCreateKind = "CAConfigMap"

			_, err := create(rec.client(base, "uid-batch"), "failing-batch", 0)
			Expect(err).To(MatchError(ContainSubstring("creating CA ConfigMap")))
		})

		It("returns an error when the batched PipelineRun cannot be started", func() {
			base := ownershipFakeClient(ownershipPipelineServiceAccount())
			rec := newNudgeCreateRecorder()
			rec.failPatch = true

			_, err := create(rec.client(base, "uid-batch"), "failing-batch", 0)
			Expect(err).To(MatchError(ContainSubstring("starting batched nudge PipelineRun")))
			Expect(ownershipGetPLR(base, nudging.BatchedNudgePipelineRunName("failing-batch", 0)).IsPending()).To(BeTrue())
		})
	})
})
