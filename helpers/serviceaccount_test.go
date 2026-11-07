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

package helpers_test

import (
	"bytes"
	"context"
	"errors"

	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	imagerepositoryv1alpha1 "github.com/konflux-ci/image-controller/api/v1alpha1"
	"github.com/konflux-ci/integration-service/helpers"
	tektonconsts "github.com/konflux-ci/integration-service/tekton/consts"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/tonglil/buflogr"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

var _ = Describe("Helpers for the per-component integration pipeline ServiceAccount", func() {

	Context("ComponentIntegrationPipelineServiceAccountName", func() {
		It("prefixes the Component name with its-pipeline-", func() {
			Expect(helpers.ComponentIntegrationPipelineServiceAccountName("my-component")).To(Equal("its-pipeline-my-component"))
		})

		It("uses the shared prefix constant", func() {
			Expect(helpers.ComponentIntegrationPipelineServiceAccountName("x")).To(HavePrefix(tektonconsts.ComponentIntegrationPipelineServiceAccountPrefix))
		})
	})

	// The adapter specs run the same function against envtest. These specs use the fake client, which can
	// also produce what a live API server does not produce on demand: a read that fails, a create that
	// fails, and an object that appears between the read and the create.
	Context("EnsureIntegrationPipelineServiceAccountExists", func() {
		const (
			namespace   = "default"
			ownerName   = "my-component"
			accountName = "its-pipeline-my-component"
			ownerUID    = types.UID("11111111-2222-3333-4444-555555555555")
			previousUID = types.UID("99999999-8888-7777-6666-555555555555")
		)

		var (
			scheme *runtime.Scheme
			owner  *applicationapiv1alpha1.Component
			log    helpers.IntegrationLogger
			key    = types.NamespacedName{Name: accountName, Namespace: namespace}
			errAPI = errors.New("the API server said no")
		)

		// componentRef is an owner reference to a Component named ownerName with the given UID.
		componentRef := func(uid types.UID) metav1.OwnerReference {
			return metav1.OwnerReference{
				APIVersion: applicationapiv1alpha1.GroupVersion.String(),
				Kind:       "Component",
				Name:       ownerName,
				UID:        uid,
			}
		}

		account := func(refs ...metav1.OwnerReference) *corev1.ServiceAccount {
			return &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: accountName, Namespace: namespace, OwnerReferences: refs}}
		}

		binding := func(refs ...metav1.OwnerReference) *rbacv1.RoleBinding {
			return &rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{Name: accountName, Namespace: namespace, OwnerReferences: refs},
				RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: tektonconsts.DefaultIntegrationPipelineClusterRoleName},
				Subjects:   []rbacv1.Subject{{Kind: rbacv1.ServiceAccountKind, Name: accountName, Namespace: namespace}},
			}
		}

		// ownerWithoutKind has the owner's name and namespace but no kind, so the scheme cannot resolve it.
		ownerWithoutKind := func() *metav1.PartialObjectMetadata {
			return &metav1.PartialObjectMetadata{ObjectMeta: metav1.ObjectMeta{Name: ownerName, Namespace: namespace, UID: ownerUID}}
		}

		newClient := func(funcs interceptor.Funcs, objs ...client.Object) client.Client {
			return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).WithInterceptorFuncs(funcs).Build()
		}

		// notFoundOnFirstAccountRead answers NotFound to the first read of the ServiceAccount, as a cache
		// that has not seen the object yet does, and then fails later reads with laterErr when it is set.
		notFoundOnFirstAccountRead := func(laterErr error) interceptor.Funcs {
			reads := 0
			return interceptor.Funcs{
				Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
					if _, ok := obj.(*corev1.ServiceAccount); ok {
						reads++
						if reads == 1 {
							return apierrors.NewNotFound(corev1.Resource("serviceaccounts"), key.Name)
						}
						if laterErr != nil {
							return laterErr
						}
					}
					return c.Get(ctx, key, obj, opts...)
				},
			}
		}

		BeforeEach(func() {
			scheme = runtime.NewScheme()
			Expect(corev1.AddToScheme(scheme)).To(Succeed())
			Expect(rbacv1.AddToScheme(scheme)).To(Succeed())
			Expect(applicationapiv1alpha1.AddToScheme(scheme)).To(Succeed())

			owner = &applicationapiv1alpha1.Component{ObjectMeta: metav1.ObjectMeta{Name: ownerName, Namespace: namespace, UID: ownerUID}}
			log = helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(&bytes.Buffer{})}
		})

		It("creates the ServiceAccount and the RoleBinding with an owner reference to the owner", func() {
			c := newClient(interceptor.Funcs{})
			Expect(helpers.EnsureIntegrationPipelineServiceAccountExists(ctx, c, log, owner)).To(Succeed())

			sa := &corev1.ServiceAccount{}
			Expect(c.Get(ctx, key, sa)).To(Succeed())
			Expect(sa.OwnerReferences).To(ConsistOf(HaveField("UID", ownerUID)))

			rb := &rbacv1.RoleBinding{}
			Expect(c.Get(ctx, key, rb)).To(Succeed())
			Expect(rb.OwnerReferences).To(ConsistOf(HaveField("UID", ownerUID)))
			Expect(rb.RoleRef).To(Equal(binding().RoleRef))
			Expect(rb.Subjects).To(Equal(binding().Subjects))
		})

		It("changes nothing on a second call", func() {
			c := newClient(interceptor.Funcs{})
			Expect(helpers.EnsureIntegrationPipelineServiceAccountExists(ctx, c, log, owner)).To(Succeed())
			first := &corev1.ServiceAccount{}
			Expect(c.Get(ctx, key, first)).To(Succeed())

			Expect(helpers.EnsureIntegrationPipelineServiceAccountExists(ctx, c, log, owner)).To(Succeed())
			second := &corev1.ServiceAccount{}
			Expect(c.Get(ctx, key, second)).To(Succeed())
			Expect(second.OwnerReferences).To(HaveLen(1))
			Expect(second.ResourceVersion).To(Equal(first.ResourceVersion))
		})

		It("leaves an existing object as it is when it has no owner or something else owns it", func() {
			otherComponent := componentRef(previousUID)
			otherComponent.Name = "another-component"
			c := newClient(interceptor.Funcs{}, account(), binding(otherComponent))
			before := &rbacv1.RoleBinding{}
			Expect(c.Get(ctx, key, before)).To(Succeed())

			Expect(helpers.EnsureIntegrationPipelineServiceAccountExists(ctx, c, log, owner)).To(Succeed())

			sa := &corev1.ServiceAccount{}
			Expect(c.Get(ctx, key, sa)).To(Succeed())
			Expect(sa.OwnerReferences).To(BeEmpty())
			rb := &rbacv1.RoleBinding{}
			Expect(c.Get(ctx, key, rb)).To(Succeed())
			Expect(rb.OwnerReferences).To(ConsistOf(otherComponent))
			Expect(rb.ResourceVersion).To(Equal(before.ResourceVersion))
		})

		It("reports a stale owner when the ServiceAccount belongs to a previous Component of the same name", func() {
			c := newClient(interceptor.Funcs{}, account(componentRef(previousUID)))

			err := helpers.EnsureIntegrationPipelineServiceAccountExists(ctx, c, log, owner)
			Expect(err).To(HaveOccurred())
			Expect(helpers.IsStaleOwnerError(err)).To(BeTrue())

			sa := &corev1.ServiceAccount{}
			Expect(c.Get(ctx, key, sa)).To(Succeed())
			Expect(sa.OwnerReferences).To(ConsistOf(componentRef(previousUID)))
			Expect(apierrors.IsNotFound(c.Get(ctx, key, &rbacv1.RoleBinding{}))).To(BeTrue())
		})

		It("reports a stale owner when only the RoleBinding belongs to a previous Component of the same name", func() {
			c := newClient(interceptor.Funcs{}, binding(componentRef(previousUID)))

			err := helpers.EnsureIntegrationPipelineServiceAccountExists(ctx, c, log, owner)
			Expect(err).To(HaveOccurred())
			Expect(helpers.IsStaleOwnerError(err)).To(BeTrue())

			sa := &corev1.ServiceAccount{}
			Expect(c.Get(ctx, key, sa)).To(Succeed())
			Expect(sa.OwnerReferences).To(ConsistOf(HaveField("UID", ownerUID)))
		})

		It("returns the error and creates nothing when the scheme cannot resolve the owner's kind", func() {
			c := newClient(interceptor.Funcs{})

			err := helpers.EnsureIntegrationPipelineServiceAccountExists(ctx, c, log, ownerWithoutKind())
			Expect(err).To(MatchError(ContainSubstring("Kind")))
			Expect(helpers.IsStaleOwnerError(err)).To(BeFalse())
			Expect(apierrors.IsNotFound(c.Get(ctx, key, &corev1.ServiceAccount{}))).To(BeTrue())
		})

		It("returns the error when the scheme cannot resolve the owner's kind and the object already has an owner", func() {
			c := newClient(interceptor.Funcs{}, account(componentRef(previousUID)))

			err := helpers.EnsureIntegrationPipelineServiceAccountExists(ctx, c, log, ownerWithoutKind())
			Expect(err).To(MatchError(ContainSubstring("Kind")))
			Expect(helpers.IsStaleOwnerError(err)).To(BeFalse())
		})

		It("returns the error when an owner reference carries a malformed apiVersion", func() {
			malformed := componentRef(previousUID)
			malformed.APIVersion = "appstudio.redhat.com/v1alpha1/extra"
			c := newClient(interceptor.Funcs{}, account(malformed))

			err := helpers.EnsureIntegrationPipelineServiceAccountExists(ctx, c, log, owner)
			Expect(err).To(MatchError(ContainSubstring("unexpected GroupVersion string")))
		})

		It("returns a read error that is not NotFound as it is", func() {
			c := newClient(interceptor.Funcs{
				Get: func(_ context.Context, _ client.WithWatch, _ client.ObjectKey, _ client.Object, _ ...client.GetOption) error {
					return errAPI
				},
			})

			err := helpers.EnsureIntegrationPipelineServiceAccountExists(ctx, c, log, owner)
			Expect(err).To(MatchError(errAPI))
			Expect(err).To(MatchError(ContainSubstring("failed to ensure ServiceAccount default/its-pipeline-my-component")))
		})

		It("returns the create error for the RoleBinding and keeps the ServiceAccount it created", func() {
			c := newClient(interceptor.Funcs{
				Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
					if _, ok := obj.(*rbacv1.RoleBinding); ok {
						return errAPI
					}
					return c.Create(ctx, obj, opts...)
				},
			})

			err := helpers.EnsureIntegrationPipelineServiceAccountExists(ctx, c, log, owner)
			Expect(err).To(MatchError(errAPI))
			Expect(err).To(MatchError(ContainSubstring("failed to ensure RoleBinding default/its-pipeline-my-component")))
			Expect(c.Get(ctx, key, &corev1.ServiceAccount{})).To(Succeed())
		})

		It("reads the object again and leaves it as it is when it appears between the read and the create", func() {
			c := newClient(notFoundOnFirstAccountRead(nil), account())

			Expect(helpers.EnsureIntegrationPipelineServiceAccountExists(ctx, c, log, owner)).To(Succeed())

			sa := &corev1.ServiceAccount{}
			Expect(c.Get(ctx, key, sa)).To(Succeed())
			Expect(sa.OwnerReferences).To(BeEmpty())
			Expect(c.Get(ctx, key, &rbacv1.RoleBinding{})).To(Succeed())
		})

		It("returns the error when the read after an AlreadyExists fails", func() {
			c := newClient(notFoundOnFirstAccountRead(errAPI), account())

			err := helpers.EnsureIntegrationPipelineServiceAccountExists(ctx, c, log, owner)
			Expect(err).To(MatchError(errAPI))
		})
	})

	// These specs use the fake client for the same reason as the specs above.
	Context("LinkComponentPushSecrets", func() {
		const (
			namespace      = "default"
			componentName  = "my-component"
			accountName    = "its-pipeline-my-component"
			pushSecretName = "imagerepository-for-my-app-my-component-image-push"
		)

		var (
			scheme *runtime.Scheme
			log    helpers.IntegrationLogger
			key    = types.NamespacedName{Name: accountName, Namespace: namespace}
			errAPI = errors.New("the API server said no")
		)

		account := func(secretNames ...string) *corev1.ServiceAccount {
			sa := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: accountName, Namespace: namespace}}
			for _, name := range secretNames {
				sa.Secrets = append(sa.Secrets, corev1.ObjectReference{Name: name})
			}
			return sa
		}

		// imageRepositories is what the caller loads: one ImageRepository with the component label.
		// pushSecret is the name image-controller writes to the status when it provisions; empty means
		// not provisioned yet.
		imageRepositories := func(pushSecret string) []imagerepositoryv1alpha1.ImageRepository {
			ir := imagerepositoryv1alpha1.ImageRepository{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ir",
					Namespace: namespace,
					Labels:    map[string]string{tektonconsts.ImageRepositoryComponentLabel: componentName},
				},
			}
			ir.Status.Credentials.PushSecretName = pushSecret
			return []imagerepositoryv1alpha1.ImageRepository{ir}
		}

		newClient := func(funcs interceptor.Funcs, objs ...client.Object) client.Client {
			return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).WithInterceptorFuncs(funcs).Build()
		}

		secretNames := func(c client.Client) []string {
			sa := &corev1.ServiceAccount{}
			Expect(c.Get(ctx, key, sa)).To(Succeed())
			names := []string{}
			for _, ref := range sa.Secrets {
				names = append(names, ref.Name)
			}
			return names
		}

		BeforeEach(func() {
			scheme = runtime.NewScheme()
			Expect(corev1.AddToScheme(scheme)).To(Succeed())
			log = helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(&bytes.Buffer{})}
		})

		It("changes nothing on a second call", func() {
			c := newClient(interceptor.Funcs{}, account())
			_, err := helpers.LinkComponentPushSecrets(ctx, c, log, namespace, componentName, imageRepositories(pushSecretName))
			Expect(err).NotTo(HaveOccurred())
			first := &corev1.ServiceAccount{}
			Expect(c.Get(ctx, key, first)).To(Succeed())

			linked, err := helpers.LinkComponentPushSecrets(ctx, c, log, namespace, componentName, imageRepositories(pushSecretName))
			Expect(err).NotTo(HaveOccurred())
			Expect(linked).To(BeTrue())
			second := &corev1.ServiceAccount{}
			Expect(c.Get(ctx, key, second)).To(Succeed())
			Expect(second.Secrets).To(HaveLen(1))
			Expect(second.ResourceVersion).To(Equal(first.ResourceVersion))
		})

		It("reports nothing to link and does not read the account when no ImageRepository has a push secret name", func() {
			funcs := interceptor.Funcs{
				Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
					return errAPI
				},
			}

			linked, err := helpers.LinkComponentPushSecrets(ctx, newClient(funcs), log, namespace, componentName, imageRepositories(""))
			Expect(err).NotTo(HaveOccurred())
			Expect(linked).To(BeFalse())
		})

		It("returns NotFound when the account does not exist", func() {
			c := newClient(interceptor.Funcs{})

			linked, err := helpers.LinkComponentPushSecrets(ctx, c, log, namespace, componentName, imageRepositories(pushSecretName))
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
			Expect(linked).To(BeFalse())
		})

		It("returns the update error and links nothing", func() {
			funcs := interceptor.Funcs{
				Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
					return errAPI
				},
			}
			c := newClient(funcs, account())

			linked, err := helpers.LinkComponentPushSecrets(ctx, c, log, namespace, componentName, imageRepositories(pushSecretName))
			Expect(err).To(MatchError(errAPI))
			Expect(linked).To(BeFalse())
			Expect(secretNames(c)).To(BeEmpty())
		})
	})
})
