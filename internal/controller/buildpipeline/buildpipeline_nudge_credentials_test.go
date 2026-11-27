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

package buildpipeline

import (
	"context"
	"errors"
	"time"

	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	konfluxv1beta2 "github.com/konflux-ci/integration-service/api/konflux/v1beta2"
	"github.com/konflux-ci/integration-service/api/v1beta2"
	"github.com/konflux-ci/integration-service/helpers"
	"github.com/konflux-ci/integration-service/loader"
	tektonconsts "github.com/konflux-ci/integration-service/tekton/consts"
	"github.com/konflux-ci/integration-service/tekton/nudging"
	toolkit "github.com/konflux-ci/operator-toolkit/loader"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// STONEINTG-1915: temporary GitHub App credential failures must requeue the nudge (keeping the
// build PipelineRun's nudge finalizer) for up to nudgeCredentialsRetryWindow after the build
// finished, and only then give up with a Warning event on the affected target components.
//
// These specs live in their own file and use their own fixtures (prefixed "ghcred-") so they are
// independent of the shared fixtures in buildpipeline_adapter_test.go.
var _ = Describe("Nudge GitHub App credential failures", Ordered, func() {
	const (
		namespace     = "default"
		sourceName    = "ghcred-source"
		targetAName   = "ghcred-target-a"
		targetBName   = "ghcred-target-b"
		saName        = "ghcred-nudge-sa"
		dockerSecName = "ghcred-nudge-image-pull"
		sourceImage   = "quay.io/redhat-appstudio/ghcred-image"
		sourceDigest  = "sha256:841328df1b9f8c4087adbdcfec6cc99ac8308805dea83f6d415d6fb8d40227c1"
		sourceRepo    = "https://github.com/ghcred-org/source"
	)

	var (
		logger         helpers.IntegrationLogger
		sourceComp     *applicationapiv1alpha1.Component
		targetA        *applicationapiv1alpha1.Component
		targetB        *applicationapiv1alpha1.Component
		sa             *corev1.ServiceAccount
		dockerSecret   *corev1.Secret
		pushPLR        *tektonv1.PipelineRun
		savedGitHubApp func(context.Context, client.Client, loader.ObjectLoader, []applicationapiv1alpha1.Component, string, string, string) ([]nudging.NudgeTarget, error)
	)

	newComponent := func(name, repoURL, image string) *applicationapiv1alpha1.Component {
		return &applicationapiv1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Spec: applicationapiv1alpha1.ComponentSpec{
				ComponentName:  name,
				Application:    "ghcred-application",
				ContainerImage: image,
				Source: applicationapiv1alpha1.ComponentSource{
					ComponentSourceUnion: applicationapiv1alpha1.ComponentSourceUnion{
						GitSource: &applicationapiv1alpha1.GitSource{URL: repoURL},
					},
				},
			},
		}
	}

	// nudgeConfigFor builds the NudgeConfig edges from the source to the given targets.
	// It is the only place that depends on the NudgeRelationship shape.
	nudgeConfigFor := func(targets ...string) *v1beta2.NudgeConfig {
		nudges := make([]v1beta2.NudgeRelationship, 0, len(targets))
		for _, t := range targets {
			nudges = append(nudges, v1beta2.NudgeRelationship{
				From: v1beta2.ComponentVersion{Component: sourceName},
				To:   v1beta2.ComponentVersion{Component: t},
				Mode: v1beta2.NudgeModeImmediate,
			})
		}
		return &v1beta2.NudgeConfig{
			ObjectMeta: metav1.ObjectMeta{Name: v1beta2.NudgeConfigSingletonName, Namespace: namespace},
			Spec:       v1beta2.NudgeConfigSpec{Nudges: nudges},
		}
	}

	// createPushPLR creates a succeeded push build PipelineRun that finished at completedAt.
	createPushPLR := func(name string, completedAt time.Time) *tektonv1.PipelineRun {
		plr := &tektonv1.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
				Labels: map[string]string{
					"pipelines.appstudio.openshift.io/type": "build",
					"appstudio.openshift.io/component":      sourceName,
					"pipelinesascode.tekton.dev/event-type": "push",
				},
				Annotations: map[string]string{},
			},
			Spec: tektonv1.PipelineRunSpec{
				PipelineRef:     &tektonv1.PipelineRef{Name: "build-pipeline-pass"},
				TaskRunTemplate: tektonv1.PipelineTaskRunTemplate{ServiceAccountName: saName},
			},
		}
		Expect(k8sClient.Create(ctx, plr)).To(Succeed())
		plr.Status = tektonv1.PipelineRunStatus{
			PipelineRunStatusFields: tektonv1.PipelineRunStatusFields{
				Results: []tektonv1.PipelineRunResult{
					{Name: "IMAGE_DIGEST", Value: *tektonv1.NewStructuredValues(sourceDigest)},
					{Name: "IMAGE_URL", Value: *tektonv1.NewStructuredValues(sourceImage)},
					{Name: "CHAINS-GIT_URL", Value: *tektonv1.NewStructuredValues(sourceRepo)},
					{Name: "CHAINS-GIT_COMMIT", Value: *tektonv1.NewStructuredValues("a2ba645d50e471d5f084b")},
				},
				StartTime:      &metav1.Time{Time: completedAt.Add(-time.Minute)},
				CompletionTime: &metav1.Time{Time: completedAt},
			},
			Status: duckv1.Status{
				Conditions: duckv1.Conditions{{Type: apis.ConditionSucceeded, Status: "True", Reason: "Completed"}},
			},
		}
		Expect(k8sClient.Status().Update(ctx, plr)).To(Succeed())
		// The suite's client reads through the informer cache; wait until it serves the status.
		Eventually(func(g Gomega) {
			cached := &tektonv1.PipelineRun{}
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(plr), cached)).To(Succeed())
			g.Expect(cached.Status.CompletionTime).NotTo(BeNil())
		}, 10*time.Second).Should(Succeed())
		return plr
	}

	newAdapter := func(plr *tektonv1.PipelineRun, nudgeConfig *v1beta2.NudgeConfig) *Adapter {
		a := NewAdapter(ctx, plr, sourceName, &[]konfluxv1beta2.ComponentGroup{}, logger, loader.NewMockLoader(), k8sClient)
		a.context = toolkit.GetMockedContext(ctx, []toolkit.MockData{
			{ContextKey: loader.NudgeConfigContextKey, Resource: nudgeConfig},
		})
		return a
	}

	getPLR := func(g Gomega, plr *tektonv1.PipelineRun) *tektonv1.PipelineRun {
		updated := &tektonv1.PipelineRun{}
		g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(plr), updated)).To(Succeed())
		return updated
	}

	warningEventsFor := func(g Gomega, componentName string) []corev1.Event {
		events := &corev1.EventList{}
		g.Expect(k8sClient.List(ctx, events, client.InNamespace(namespace))).To(Succeed())
		var matching []corev1.Event
		for _, e := range events.Items {
			if e.InvolvedObject.Name == componentName && e.Reason == nudgeCredentialsUnavailableEventReason {
				matching = append(matching, e)
			}
		}
		return matching
	}

	transientErr := func(components ...string) error {
		return &nudging.GitHubAppCredentialsError{ComponentNames: components, Err: errors.New("API rate limit exceeded")}
	}

	BeforeAll(func() {
		logger = helpers.IntegrationLogger{Logger: ctrl.Log}

		sourceComp = newComponent(sourceName, sourceRepo, sourceImage)
		targetA = newComponent(targetAName, "https://github.com/ghcred-org/target-a", "")
		targetB = newComponent(targetBName, "https://github.com/ghcred-org/target-b", "")
		for _, comp := range []*applicationapiv1alpha1.Component{sourceComp, targetA, targetB} {
			Expect(k8sClient.Create(ctx, comp)).To(Succeed())
		}

		dockerSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: dockerSecName, Namespace: namespace},
			Type:       corev1.SecretTypeDockerConfigJson,
			Data: map[string][]byte{
				corev1.DockerConfigJsonKey: []byte(`{"auths":{"quay.io/redhat-appstudio":{"username":"user","password":"pass"}}}`),
			},
		}
		Expect(k8sClient.Create(ctx, dockerSecret)).To(Succeed())
		sa = &corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{Name: saName, Namespace: namespace},
			Secrets:    []corev1.ObjectReference{{Name: dockerSecret.Name}},
		}
		Expect(k8sClient.Create(ctx, sa)).To(Succeed())

		Eventually(func(g Gomega) {
			for _, comp := range []*applicationapiv1alpha1.Component{sourceComp, targetA, targetB} {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(comp), &applicationapiv1alpha1.Component{})).To(Succeed())
			}
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(sa), &corev1.ServiceAccount{})).To(Succeed())
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(dockerSecret), &corev1.Secret{})).To(Succeed())
		}, 10*time.Second).Should(Succeed())
	})

	AfterAll(func() {
		for _, obj := range []client.Object{sourceComp, targetA, targetB, sa, dockerSecret} {
			err := k8sClient.Delete(ctx, obj)
			Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
		}
	})

	BeforeEach(func() {
		savedGitHubApp = getNudgeTargetsGithubAppFn
	})

	// deleteNudgeObjectsOwnedBy removes the nudge PipelineRuns and support objects created for plr.
	// envtest runs no garbage collector, so owner references alone would leave them behind and
	// leak into other specs that count PipelineRuns in the namespace.
	deleteNudgeObjectsOwnedBy := func(plr *tektonv1.PipelineRun) {
		ownedBy := func(obj client.Object) bool {
			for _, ref := range obj.GetOwnerReferences() {
				if ref.Kind == "PipelineRun" && ref.Name == plr.Name {
					return true
				}
			}
			return obj.GetAnnotations()[tektonconsts.NudgingPipelineAnnotation] == plr.Name
		}
		plrs := &tektonv1.PipelineRunList{}
		Expect(k8sClient.List(ctx, plrs, client.InNamespace(namespace))).To(Succeed())
		for i := range plrs.Items {
			if ownedBy(&plrs.Items[i]) {
				Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, &plrs.Items[i]))).To(Succeed())
			}
		}
		secrets := &corev1.SecretList{}
		Expect(k8sClient.List(ctx, secrets, client.InNamespace(namespace))).To(Succeed())
		for i := range secrets.Items {
			if ownedBy(&secrets.Items[i]) {
				Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, &secrets.Items[i]))).To(Succeed())
			}
		}
		configMaps := &corev1.ConfigMapList{}
		Expect(k8sClient.List(ctx, configMaps, client.InNamespace(namespace))).To(Succeed())
		for i := range configMaps.Items {
			if ownedBy(&configMaps.Items[i]) {
				Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, &configMaps.Items[i]))).To(Succeed())
			}
		}
		Eventually(func(g Gomega) {
			remaining := &tektonv1.PipelineRunList{}
			g.Expect(k8sClient.List(ctx, remaining, client.InNamespace(namespace))).To(Succeed())
			for i := range remaining.Items {
				g.Expect(ownedBy(&remaining.Items[i])).To(BeFalse())
			}
		}, 10*time.Second).Should(Succeed())
	}

	AfterEach(func() {
		getNudgeTargetsGithubAppFn = savedGitHubApp
		if pushPLR == nil {
			return
		}
		deleteNudgeObjectsOwnedBy(pushPLR)
		latest := &tektonv1.PipelineRun{}
		if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(pushPLR), latest); err == nil {
			controllerutil.RemoveFinalizer(latest, helpers.NudgePipelineRunFinalizer)
			_ = k8sClient.Update(ctx, latest)
		}
		err := k8sClient.Delete(ctx, pushPLR)
		Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
		pushPLR = nil
	})

	It("requeues and keeps the nudge finalizer on a temporary failure inside the retry window", func() {
		getNudgeTargetsGithubAppFn = func(context.Context, client.Client, loader.ObjectLoader, []applicationapiv1alpha1.Component, string, string, string) ([]nudging.NudgeTarget, error) {
			return nil, transientErr(targetAName)
		}
		pushPLR = createPushPLR("ghcred-plr-retry", time.Now().Add(-5*time.Minute))

		result, err := newAdapter(pushPLR, nudgeConfigFor(targetAName)).EnsureNudgePipelineRunsExist()
		Expect(err).To(HaveOccurred())
		Expect(result.RequeueRequest).To(BeTrue())

		Eventually(func(g Gomega) {
			updated := getPLR(g, pushPLR)
			g.Expect(controllerutil.ContainsFinalizer(updated, helpers.NudgePipelineRunFinalizer)).To(BeTrue())
		}, 10*time.Second).Should(Succeed())
		Consistently(func(g Gomega) {
			updated := getPLR(g, pushPLR)
			g.Expect(updated.Annotations).NotTo(HaveKey(tektonconsts.NudgeProcessedAnnotation))
			g.Expect(warningEventsFor(g, targetAName)).To(BeEmpty())
		}, 2*time.Second).Should(Succeed())
	})

	It("gives up after the retry window: annotates credentials-unavailable, emits a Warning event and removes the finalizer", func() {
		getNudgeTargetsGithubAppFn = func(context.Context, client.Client, loader.ObjectLoader, []applicationapiv1alpha1.Component, string, string, string) ([]nudging.NudgeTarget, error) {
			return nil, transientErr(targetAName)
		}
		pushPLR = createPushPLR("ghcred-plr-giveup", time.Now().Add(-nudgeCredentialsRetryWindow-time.Minute))

		result, err := newAdapter(pushPLR, nudgeConfigFor(targetAName)).EnsureNudgePipelineRunsExist()
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueRequest).To(BeFalse())

		Eventually(func(g Gomega) {
			updated := getPLR(g, pushPLR)
			g.Expect(updated.Annotations[tektonconsts.NudgeProcessedAnnotation]).To(Equal("credentials-unavailable"))
			g.Expect(controllerutil.ContainsFinalizer(updated, helpers.NudgePipelineRunFinalizer)).To(BeFalse())
		}, 10*time.Second).Should(Succeed())
		Eventually(func(g Gomega) {
			events := warningEventsFor(g, targetAName)
			g.Expect(events).To(HaveLen(1))
			g.Expect(events[0].Type).To(Equal(corev1.EventTypeWarning))
			g.Expect(events[0].InvolvedObject.Kind).To(Equal("Component"))
			g.Expect(events[0].Message).To(ContainSubstring(pushPLR.Name))
		}, 10*time.Second).Should(Succeed())
	})

	It("after the retry window still nudges the components that resolved and warns only about the others", func() {
		getNudgeTargetsGithubAppFn = func(_ context.Context, _ client.Client, _ loader.ObjectLoader, _ []applicationapiv1alpha1.Component, host, user, pwd string) ([]nudging.NudgeTarget, error) {
			return []nudging.NudgeTarget{{
				ComponentName:           targetBName,
				GitProvider:             "github",
				Username:                "my-app[bot]",
				GitAuthor:               "my-app <67890+my-app[bot]@users.noreply.github.com>",
				Token:                   "ghs_test-token",
				Endpoint:                "https://api.github.com/",
				Repositories:            []nudging.RenovateRepository{{Repository: "ghcred-org/target-b"}},
				ImageRepositoryHost:     host,
				ImageRepositoryUsername: user,
				ImageRepositoryPassword: pwd,
			}}, transientErr(targetAName)
		}
		pushPLR = createPushPLR("ghcred-plr-partial", time.Now().Add(-nudgeCredentialsRetryWindow-time.Minute))

		result, err := newAdapter(pushPLR, nudgeConfigFor(targetAName, targetBName)).EnsureNudgePipelineRunsExist()
		Expect(err).NotTo(HaveOccurred())
		Expect(result.CancelRequest).To(BeFalse())

		Eventually(func(g Gomega) {
			updated := getPLR(g, pushPLR)
			g.Expect(updated.Annotations[tektonconsts.NudgeProcessedAnnotation]).To(Equal(targetBName))
			g.Expect(controllerutil.ContainsFinalizer(updated, helpers.NudgePipelineRunFinalizer)).To(BeFalse())
		}, 10*time.Second).Should(Succeed())
		Eventually(func(g Gomega) {
			g.Expect(warningEventsFor(g, targetAName)).NotTo(BeEmpty())
		}, 10*time.Second).Should(Succeed())
		Consistently(func(g Gomega) {
			g.Expect(warningEventsFor(g, targetBName)).To(BeEmpty())
		}, 2*time.Second).Should(Succeed())
	})

	It("keeps the existing no-credentials behaviour when there is no GitHub App failure", func() {
		getNudgeTargetsGithubAppFn = func(context.Context, client.Client, loader.ObjectLoader, []applicationapiv1alpha1.Component, string, string, string) ([]nudging.NudgeTarget, error) {
			return nil, nil
		}
		pushPLR = createPushPLR("ghcred-plr-nocreds", time.Now())

		result, err := newAdapter(pushPLR, nudgeConfigFor(targetAName)).EnsureNudgePipelineRunsExist()
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueRequest).To(BeFalse())

		Eventually(func(g Gomega) {
			updated := getPLR(g, pushPLR)
			g.Expect(updated.Annotations[tektonconsts.NudgeProcessedAnnotation]).To(Equal("no-credentials"))
			g.Expect(controllerutil.ContainsFinalizer(updated, helpers.NudgePipelineRunFinalizer)).To(BeFalse())
		}, 10*time.Second).Should(Succeed())
	})

	It("also requeues on an untyped GitHub App error inside the retry window", func() {
		getNudgeTargetsGithubAppFn = func(context.Context, client.Client, loader.ObjectLoader, []applicationapiv1alpha1.Component, string, string, string) ([]nudging.NudgeTarget, error) {
			return nil, errors.New("unexpected failure")
		}
		pushPLR = createPushPLR("ghcred-plr-untyped", time.Now())

		result, err := newAdapter(pushPLR, nudgeConfigFor(targetAName)).EnsureNudgePipelineRunsExist()
		Expect(err).To(HaveOccurred())
		Expect(result.RequeueRequest).To(BeTrue())
		Eventually(func(g Gomega) {
			updated := getPLR(g, pushPLR)
			g.Expect(controllerutil.ContainsFinalizer(updated, helpers.NudgePipelineRunFinalizer)).To(BeTrue())
			g.Expect(updated.Annotations).NotTo(HaveKey(tektonconsts.NudgeProcessedAnnotation))
		}, 10*time.Second).Should(Succeed())
	})

	It("emitNudgeCredentialsUnavailableEvents does not fail when the event cannot be created", func() {
		a := &Adapter{
			context:       ctx,
			client:        fake.NewClientBuilder().WithScheme(runtime.NewScheme()).Build(),
			logger:        logger,
			pipelineRun:   &tektonv1.PipelineRun{ObjectMeta: metav1.ObjectMeta{Name: "ghcred-plr", Namespace: namespace}},
			componentName: sourceName,
		}
		Expect(func() {
			a.emitNudgeCredentialsUnavailableEvents([]applicationapiv1alpha1.Component{*targetA})
		}).NotTo(Panic())
	})

	It("nudgeCredentialsRetryWindowExpired measures from the build completion time", func() {
		now := time.Now()
		plr := &tektonv1.PipelineRun{}
		a := &Adapter{pipelineRun: plr}

		plr.Status.CompletionTime = &metav1.Time{Time: now.Add(-30 * time.Minute)}
		Expect(a.nudgeCredentialsRetryWindowExpired(now)).To(BeFalse())

		plr.Status.CompletionTime = &metav1.Time{Time: now.Add(-nudgeCredentialsRetryWindow - time.Second)}
		Expect(a.nudgeCredentialsRetryWindowExpired(now)).To(BeTrue())

		// No completion time: fall back to the creation time.
		plr.Status.CompletionTime = nil
		plr.CreationTimestamp = metav1.Time{Time: now.Add(-2 * nudgeCredentialsRetryWindow)}
		Expect(a.nudgeCredentialsRetryWindowExpired(now)).To(BeTrue())
	})

	It("undeliveredNudgeComponents returns failed components that no auth method resolved", func() {
		comps := []applicationapiv1alpha1.Component{*targetA, *targetB}
		Expect(undeliveredNudgeComponents(comps, nil, nil)).To(BeEmpty())

		credsErr := &nudging.GitHubAppCredentialsError{ComponentNames: []string{targetAName, targetBName}}
		undelivered := undeliveredNudgeComponents(comps, credsErr, []nudging.NudgeTarget{{ComponentName: targetBName}})
		Expect(undelivered).To(HaveLen(1))
		Expect(undelivered[0].Name).To(Equal(targetAName))
	})
})
