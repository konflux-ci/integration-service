/*
Copyright 2023.

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

package statusreport

import (
	"reflect"
	"time"

	"github.com/konflux-ci/integration-service/api/v1beta2"
	"github.com/konflux-ci/integration-service/gitops"
	toolkit "github.com/konflux-ci/operator-toolkit/loader"
	"github.com/konflux-ci/operator-toolkit/metadata"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
	crwebhook "sigs.k8s.io/controller-runtime/pkg/webhook"

	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientsetscheme "k8s.io/client-go/kubernetes/scheme"
	klog "k8s.io/klog/v2"
)

var _ = Describe("StatusReportController", func() {
	var (
		manager                ctrl.Manager
		statusReportReconciler *Reconciler
		scheme                 runtime.Scheme
		req                    ctrl.Request
		hasApp                 *applicationapiv1alpha1.Application
		hasSnapshot            *applicationapiv1alpha1.Snapshot
	)

	BeforeEach(func() {

		applicationName := "application-sample"

		hasApp = &applicationapiv1alpha1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      applicationName,
				Namespace: "default",
			},
			Spec: applicationapiv1alpha1.ApplicationSpec{
				DisplayName: "application-sample",
				Description: "This is an example application",
			},
		}

		Expect(k8sClient.Create(ctx, hasApp)).Should(Succeed())

		hasSnapshot = &applicationapiv1alpha1.Snapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "snapshot-sample",
				Namespace: "default",
				Labels: map[string]string{
					gitops.SnapshotTypeLabel:      "component",
					gitops.SnapshotComponentLabel: "component-sample",
				},
				Annotations: map[string]string{
					gitops.PRGroupCreationAnnotation: "failed to create group snapshot due to error",
				},
			},
			Spec: applicationapiv1alpha1.SnapshotSpec{
				Application: hasApp.Name,
				Components: []applicationapiv1alpha1.SnapshotComponent{
					{
						Name:           "component-sample",
						ContainerImage: "testimage",
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, hasSnapshot)).Should(Succeed())

		req = ctrl.Request{
			NamespacedName: types.NamespacedName{
				Namespace: "default",
				Name:      hasSnapshot.Name,
			},
		}

		webhookInstallOptions := &testEnv.WebhookInstallOptions

		klog.Info(webhookInstallOptions.LocalServingHost)
		klog.Info(webhookInstallOptions.LocalServingPort)
		klog.Info(webhookInstallOptions.LocalServingCertDir)

		var err error
		manager, err = ctrl.NewManager(cfg, ctrl.Options{
			Scheme: clientsetscheme.Scheme,
			WebhookServer: crwebhook.NewServer(crwebhook.Options{
				CertDir: webhookInstallOptions.LocalServingCertDir,
				Host:    webhookInstallOptions.LocalServingHost,
				Port:    webhookInstallOptions.LocalServingPort,
			}),
			Metrics: server.Options{
				BindAddress: "0", // disables metrics
			},
			LeaderElection: false,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(err).ToNot(HaveOccurred())

		statusReportReconciler = NewStatusReportReconciler(k8sClient, &logf.Log, &scheme)
	})
	AfterEach(func() {
		err := k8sClient.Delete(ctx, hasApp)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, hasSnapshot)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
	})

	It("can create and return a new Reconciler object", func() {
		Expect(reflect.TypeOf(statusReportReconciler)).To(Equal(reflect.TypeOf(&Reconciler{})))
	})

	It("can Reconcile when Reconcile fails to prepare the adapter when snapshot is not found", func() {
		Expect(k8sClient.Delete(ctx, hasSnapshot)).Should(Succeed())
		Eventually(func() error {
			_, err := statusReportReconciler.Reconcile(ctx, req)
			return err
		}).Should(Succeed())
	})

	It("can Reconcile function prepare the adapter and return the result of the reconcile handling operation", func() {
		req := ctrl.Request{
			NamespacedName: types.NamespacedName{
				Name:      "non-existent",
				Namespace: "default",
			},
		}
		result, err := statusReportReconciler.Reconcile(ctx, req)
		Expect(reflect.TypeOf(result)).To(Equal(reflect.TypeOf(reconcile.Result{})))
		Expect(err).ToNot(HaveOccurred())
	})

	It("can setup a new controller manager via SetupController", func() {
		Expect(SetupController(manager, &logf.Log)).To(Succeed())
	})

	When("snapshot is restored from backup", func() {

		BeforeEach(func() {
			hasSnapshot.Labels["velero.io/restore-name"] = "something"
			Expect(k8sClient.Update(ctx, hasSnapshot)).To(Succeed())

			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Namespace: hasSnapshot.Namespace,
					Name:      hasSnapshot.Name,
				}, hasSnapshot)
				return err == nil && metadata.HasLabel(hasSnapshot, "velero.io/restore-name")
			}, time.Second*20).Should(BeTrue())
		})

		It("stops reconciliation without error", func() {
			result, err := statusReportReconciler.Reconcile(ctx, req)
			Expect(result).To(Equal(ctrl.Result{}))
			Expect(err).ToNot(HaveOccurred())
		})
	})

	When("Reconcile encounters client errors", func() {
		newReconciler := func(mocks ...toolkit.ClientCallMock) *Reconciler {
			_, mockClient := toolkit.GetMockedContextWithClient(ctx, k8sClient, nil, mocks)
			return NewStatusReportReconciler(mockClient, &logf.Log, &scheme)
		}

		It("requeues with an error when the initial snapshot Get fails with a non-NotFound error", func() {
			r := newReconciler(toolkit.ClientCallMock{Operation: toolkit.OperationGet, ObjectType: &applicationapiv1alpha1.Snapshot{}, Err: errBoom})
			_, err := r.Reconcile(ctx, req)
			Expect(err).To(HaveOccurred())
		})

		It("returns an error when the Application cannot be fetched from the snapshot", func() {
			waitForCached(hasSnapshot)

			r := newReconciler(toolkit.ClientCallMock{Operation: toolkit.OperationGet, ObjectType: &applicationapiv1alpha1.Application{}, Err: errBoom})
			_, err := r.Reconcile(ctx, req)
			Expect(err).To(HaveOccurred())
		})

		It("reconciles the application branch to completion", func() {
			waitForCached(hasSnapshot)
			waitForCached(hasApp)

			r := newReconciler(toolkit.ClientCallMock{Operation: toolkit.OperationList, ObjectType: &v1beta2.IntegrationTestScenarioList{}, Err: nil})
			result, err := r.Reconcile(ctx, req)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			Eventually(func() bool {
				s := &applicationapiv1alpha1.Snapshot{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: hasSnapshot.Namespace, Name: hasSnapshot.Name}, s); err != nil {
					return false
				}
				return gitops.IsSnapshotIntegrationStatusMarkedAsFinished(s) && gitops.IsSnapshotMarkedAsPassed(s)
			}, time.Second*20).Should(BeTrue())
		})

		When("the snapshot references a ComponentGroup", func() {
			var (
				cg         *v1beta2.ComponentGroup
				cgSnapshot *applicationapiv1alpha1.Snapshot
				cgReq      ctrl.Request
			)

			BeforeEach(func() {
				cg = &v1beta2.ComponentGroup{
					ObjectMeta: metav1.ObjectMeta{Name: "cg-sample", Namespace: "default"},
					Spec: v1beta2.ComponentGroupSpec{
						Components: []v1beta2.ComponentReference{
							{Name: "component-sample", ComponentVersion: v1beta2.ComponentVersionReference{Name: "v1", Revision: "main"}},
						},
					},
				}
				Expect(k8sClient.Create(ctx, cg)).Should(Succeed())

				cgSnapshot = &applicationapiv1alpha1.Snapshot{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "snapshot-cg-sample",
						Namespace: "default",
						Labels: map[string]string{
							gitops.SnapshotTypeLabel:      "component",
							gitops.SnapshotComponentLabel: "component-sample",
						},
					},
					Spec: applicationapiv1alpha1.SnapshotSpec{
						ComponentGroup: cg.Name,
						Components: []applicationapiv1alpha1.SnapshotComponent{
							{Name: "component-sample", ContainerImage: "testimage"},
						},
					},
				}
				Expect(k8sClient.Create(ctx, cgSnapshot)).Should(Succeed())

				cgReq = ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: cgSnapshot.Name}}
				waitForCached(cg)
				waitForCached(cgSnapshot)
			})

			AfterEach(func() {
				err := k8sClient.Delete(ctx, cg)
				Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
				err = k8sClient.Delete(ctx, cgSnapshot)
				Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
			})

			It("reconciles the component group branch to completion", func() {
				r := newReconciler(toolkit.ClientCallMock{Operation: toolkit.OperationList, ObjectType: &v1beta2.IntegrationTestScenarioList{}, Err: nil})
				result, err := r.Reconcile(ctx, cgReq)
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(Equal(ctrl.Result{}))

				Eventually(func() bool {
					s := &applicationapiv1alpha1.Snapshot{}
					if err := k8sClient.Get(ctx, cgReq.NamespacedName, s); err != nil {
						return false
					}
					return gitops.IsSnapshotIntegrationStatusMarkedAsFinished(s) && gitops.IsSnapshotMarkedAsPassed(s)
				}, time.Second*20).Should(BeTrue())
			})

			It("returns an error when the ComponentGroup cannot be fetched from the snapshot", func() {
				r := newReconciler(toolkit.ClientCallMock{Operation: toolkit.OperationGet, ObjectType: &v1beta2.ComponentGroup{}, Err: errBoom})
				_, err := r.Reconcile(ctx, cgReq)
				Expect(err).To(HaveOccurred())
			})
		})
	})

})
