/*
Copyright 2023 Red Hat Inc.

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

package gitops_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"time"

	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/gitops"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Gitops functions for managing Snapshots", Ordered, func() {

	var (
		hasApp      *applicationapiv1alpha1.Application
		hasComp     *applicationapiv1alpha1.Component
		hasSnapshot *applicationapiv1alpha1.Snapshot
		hasBinding  *applicationapiv1alpha1.SnapshotEnvironmentBinding
		env         applicationapiv1alpha1.Environment
	)
	const (
		SampleRepoLink  = "https://github.com/devfile-samples/devfile-sample-java-springboot-basic"
		namespace       = "default"
		applicationName = "application-sample"
		componentName   = "component-sample"
		snapshotName    = "snapshot-sample"
		sampleImage     = "quay.io/redhat-appstudio/sample-image:latest"
	)

	BeforeAll(func() {
		hasApp = &applicationapiv1alpha1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      applicationName,
				Namespace: namespace,
			},
			Spec: applicationapiv1alpha1.ApplicationSpec{
				DisplayName: "application-sample",
				Description: "This is an example application",
			},
		}
		Expect(k8sClient.Create(ctx, hasApp)).Should(Succeed())

		hasComp = &applicationapiv1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{
				Name:      componentName,
				Namespace: namespace,
			},
			Spec: applicationapiv1alpha1.ComponentSpec{
				ComponentName:  componentName,
				Application:    applicationName,
				ContainerImage: "",
				Source: applicationapiv1alpha1.ComponentSource{
					ComponentSourceUnion: applicationapiv1alpha1.ComponentSourceUnion{
						GitSource: &applicationapiv1alpha1.GitSource{
							URL: SampleRepoLink,
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, hasComp)).Should(Succeed())

		env = applicationapiv1alpha1.Environment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "envname",
				Namespace: "default",
			},
			Spec: applicationapiv1alpha1.EnvironmentSpec{
				Type:               "POC",
				DisplayName:        "my-environment",
				DeploymentStrategy: applicationapiv1alpha1.DeploymentStrategy_Manual,
				ParentEnvironment:  "",
				Tags:               []string{},
				Configuration: applicationapiv1alpha1.EnvironmentConfiguration{
					Env: []applicationapiv1alpha1.EnvVarPair{
						{
							Name:  "var_name",
							Value: "test",
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, &env)).Should(Succeed())

		hasSnapshot = &applicationapiv1alpha1.Snapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      snapshotName,
				Namespace: namespace,
				Labels: map[string]string{
					gitops.SnapshotTypeLabel:      gitops.SnapshotComponentType,
					gitops.SnapshotComponentLabel: componentName,
				},
			},
			Spec: applicationapiv1alpha1.SnapshotSpec{
				Application: hasApp.Name,
				Components: []applicationapiv1alpha1.SnapshotComponent{
					{
						Name:           componentName,
						ContainerImage: sampleImage,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, hasSnapshot)).Should(Succeed())

		hasBinding = &applicationapiv1alpha1.SnapshotEnvironmentBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "snapshot-binding-sample",
				Namespace: "default",
			},
			Spec: applicationapiv1alpha1.SnapshotEnvironmentBindingSpec{
				Application: hasApp.Name,
				Snapshot:    hasSnapshot.Name,
				Environment: env.Name,
				Components:  []applicationapiv1alpha1.BindingComponent{},
			},
		}
		Expect(k8sClient.Create(ctx, hasBinding)).Should(Succeed())

		hasBinding.Status = applicationapiv1alpha1.SnapshotEnvironmentBindingStatus{
			ComponentDeploymentConditions: []metav1.Condition{
				{
					Reason:             "Completed",
					Status:             "True",
					Type:               gitops.BindingDeploymentStatusConditionType,
					LastTransitionTime: metav1.Time{Time: time.Now()},
				},
			},
		}

		Expect(k8sClient.Status().Update(ctx, hasBinding)).Should(Succeed())

		Eventually(func() bool {
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      hasBinding.Name,
				Namespace: "default",
			}, hasBinding)
			return err == nil && gitops.IsBindingDeployed(hasBinding)
		}, time.Second*20).Should(BeTrue())
	})

	AfterAll(func() {
		err := k8sClient.Delete(ctx, hasApp)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, hasComp)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, &env)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, hasSnapshot)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, hasBinding)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
	})

	It("ensures Binding Component is created", func() {
		gitops.MarkSnapshotAsPassed(k8sClient, ctx, hasSnapshot, "test passed")
		Expect(gitops.HaveAppStudioTestsSucceeded(hasSnapshot)).To(BeTrue())
		bindingComponents := gitops.NewBindingComponents(hasSnapshot)
		Expect(bindingComponents).NotTo(BeNil())
	})

	It("ensures a new SnapshotEnvironmentBinding is created", func() {
		newSnapshotEnvironmentBinding := gitops.NewSnapshotEnvironmentBinding("sample", namespace, hasApp.Name, env.Name, hasSnapshot)
		Expect(newSnapshotEnvironmentBinding).NotTo(BeNil())
		Expect(newSnapshotEnvironmentBinding.Spec.Snapshot).To(Equal(hasSnapshot.Name))
		Expect(newSnapshotEnvironmentBinding.Spec.Environment).To(Equal(env.Name))
		Expect(len(newSnapshotEnvironmentBinding.Spec.Components)).To(Equal(1))

		Expect(gitops.IsBindingDeployed(newSnapshotEnvironmentBinding)).NotTo(BeTrue())
		Expect(gitops.HaveBindingsFailed(newSnapshotEnvironmentBinding)).NotTo(BeTrue())
		gitops.PrepareAndRegisterSEBReady(newSnapshotEnvironmentBinding)
	})

	It("ensures an existing deployed SnapshotEnvironmentBinding conditions are recognized", func() {
		Expect(gitops.IsBindingDeployed(hasBinding)).To(BeTrue())
		Expect(gitops.HaveBindingsFailed(hasBinding)).NotTo(BeTrue())

		hasBinding.Status = applicationapiv1alpha1.SnapshotEnvironmentBindingStatus{
			ComponentDeploymentConditions: []metav1.Condition{
				{
					Reason:             "CommitsUnsynced",
					Status:             "False",
					Type:               gitops.BindingDeploymentStatusConditionType,
					LastTransitionTime: metav1.Time{Time: time.Now()},
				},
			},
			BindingConditions: []metav1.Condition{
				{
					Reason:             "ErrorOccurred",
					Status:             "True",
					Type:               gitops.BindingErrorOccurredStatusConditionType,
					LastTransitionTime: metav1.Time{Time: time.Now()},
				},
			},
		}

		Expect(gitops.IsBindingDeployed(hasBinding)).NotTo(BeTrue())
		failed := gitops.HaveBindingsFailed(hasBinding)
		Expect(failed).To(BeTrue())
	})
})
