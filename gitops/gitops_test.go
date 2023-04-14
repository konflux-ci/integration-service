package gitops_test

import (
	"fmt"

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
		env         applicationapiv1alpha1.Environment
		sampleImage string
	)
	const (
		SampleRepoLink  = "https://github.com/devfile-samples/devfile-sample-java-springboot-basic"
		namespace       = "default"
		applicationName = "application-sample"
		componentName   = "component-sample"
		snapshotName    = "snapshot-sample"
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
	})

	BeforeEach(func() {
		sampleImage = "quay.io/redhat-appstudio/sample-image:latest"

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

		Eventually(func() error {
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      hasSnapshot.Name,
				Namespace: namespace,
			}, hasSnapshot)
			return err
		}, time.Second*10).ShouldNot(HaveOccurred())
	})

	AfterEach(func() {
		err := k8sClient.Delete(ctx, hasSnapshot)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
	})

	AfterAll(func() {
		err := k8sClient.Delete(ctx, hasApp)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, hasComp)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, &env)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())

	})

	It("ensures Snapshot Environment Binding doesn't exist", func() {
		gitops.MarkSnapshotAsPassed(k8sClient, ctx, hasSnapshot, "test passed")
		Expect(gitops.HaveHACBSTestsSucceeded(hasSnapshot)).To(BeTrue())

		Eventually(func() bool {
			snapshotEnvironmentBinding, err := gitops.FindExistingSnapshotEnvironmentBinding(k8sClient, ctx, hasApp, &env)
			fmt.Fprintf(GinkgoWriter, "Err: %v\n", err)
			fmt.Fprintf(GinkgoWriter, "appenvbinding: %v\n", snapshotEnvironmentBinding)
			return snapshotEnvironmentBinding == nil && err == nil
		}, time.Second*10).Should(BeTrue())
	})

	It("ensures Binding Component is created", func() {
		gitops.MarkSnapshotAsPassed(k8sClient, ctx, hasSnapshot, "test passed")
		Expect(gitops.HaveHACBSTestsSucceeded(hasSnapshot)).To(BeTrue())
		bindingComponents := gitops.NewBindingComponents([]applicationapiv1alpha1.Component{*hasComp})
		Expect(bindingComponents).NotTo(BeNil())
	})

	It("ensures Snapshot Environment Binding is created and exists", func() {
		bindingName := hasApp.Name + "-" + env.Name + "-" + "binding"
		snapshotEnvironmentBinding := gitops.NewSnapshotEnvironmentBinding(
			bindingName, hasApp.Namespace, hasApp.Name,
			env.Name, hasSnapshot, []applicationapiv1alpha1.Component{*hasComp})
		Expect(snapshotEnvironmentBinding).NotTo(BeNil())
		Expect(k8sClient.Create(ctx, snapshotEnvironmentBinding)).Should(Succeed())
		Eventually(func() bool {
			snapshotEnvironmentBinding, err := gitops.FindExistingSnapshotEnvironmentBinding(k8sClient, ctx, hasApp, &env)
			return snapshotEnvironmentBinding != nil && err == nil
		}, time.Second*10).Should(BeTrue())
		Expect(k8sClient.Delete(ctx, snapshotEnvironmentBinding)).Should(Succeed())
	})
})
