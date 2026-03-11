package snapshot

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	"github.com/konflux-ci/integration-service/api/v1beta2"
	"github.com/konflux-ci/integration-service/gitops"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Snapshot validation function", Ordered, func() {
	var (
		hasCompGroup *v1beta2.ComponentGroup
		hasSnapshot  *applicationapiv1alpha1.Snapshot
	)
	const (
		componentName           = "component-sample"
		componentVersion        = "v1"
		componentURL            = "https://github.com/devfile-samples/devfile-sample-go-basic"
		builtCommit             = "c713067b0e65fb3de50d1f7c457eb51c2ab0dbb0"
		builtDigest             = "sha256:841328df1b9f8c4087adbdcfec6cc99ac8308805dea83f6d415d6fb8d40227c1"
		builtImageWithoutDigest = "quay.io/konflux-ci/sample-image"
		newCommit               = "a2ba645d50e471d5f084b"
		builtImageWithDigest    = builtImageWithoutDigest + "@" + builtDigest
		snapshotName            = "componentgroup-snapshot-sample"
	)

	BeforeAll(func() {
		hasCompGroup = &v1beta2.ComponentGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "component-group-sample",
				Namespace: "default",
			},
			Spec: v1beta2.ComponentGroupSpec{
				Components: []v1beta2.ComponentReference{
					v1beta2.ComponentReference{
						Name: componentName,
						ComponentVersion: v1beta2.ComponentVersionReference{
							Name:     componentVersion,
							Revision: "main",
						},
					},
					v1beta2.ComponentReference{
						Name: "another-component-sample",
						ComponentVersion: v1beta2.ComponentVersionReference{
							Name:     "v1",
							Revision: "main",
						},
					},
				},
			},
		}

		hasCompGroup.Status = v1beta2.ComponentGroupStatus{
			Conditions: []metav1.Condition{
				metav1.Condition{
					Type:               "Succeeded",
					Status:             metav1.ConditionTrue,
					Reason:             "testing",
					Message:            "test condition",
					LastTransitionTime: metav1.Time{Time: time.Now()},
				},
			},
			GlobalCandidateList: []v1beta2.ComponentState{
				v1beta2.ComponentState{
					Name:                  componentName,
					Version:               componentVersion,
					URL:                   componentURL,
					LastPromotedImage:     fmt.Sprintf("%s@%s", builtImageWithoutDigest, builtDigest),
					LastPromotedCommit:    builtCommit,
					LastPromotedBuildTime: &metav1.Time{Time: time.Now()},
				},
			},
		}

		hasSnapshot = &applicationapiv1alpha1.Snapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      snapshotName,
				Namespace: "default",
				Labels: map[string]string{
					gitops.SnapshotTypeLabel:              "component",
					gitops.SnapshotComponentLabel:         "component-sample",
					"build.appstudio.redhat.com/pipeline": "enterprise-contract",
					gitops.PipelineAsCodeEventTypeLabel:   "push",
				},
				Annotations: map[string]string{
					gitops.PipelineAsCodeInstallationIDAnnotation:   "123",
					"build.appstudio.redhat.com/commit_sha":         "6c65b2fcaea3e1a0a92476c8b5dc89e92a85f025",
					"appstudio.redhat.com/updateComponentOnSuccess": "false",
				},
			},
			Spec: applicationapiv1alpha1.SnapshotSpec{
				ComponentGroup: hasCompGroup.Name,
				Components: []applicationapiv1alpha1.SnapshotComponent{
					{
						Name:           componentName,
						Version:        componentVersion,
						ContainerImage: builtImageWithDigest,
						Source: applicationapiv1alpha1.ComponentSource{
							ComponentSourceUnion: applicationapiv1alpha1.ComponentSourceUnion{
								GitSource: &applicationapiv1alpha1.GitSource{
									URL:      componentURL,
									Revision: newCommit,
								},
							},
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, hasSnapshot)).Should(Succeed())
	})

	AfterAll(func() {
		err := k8sClient.Delete(ctx, hasCompGroup)
		Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, hasSnapshot)
		Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
	})

	When("A valid override snapshot is created", func() {
		It("Does not return an error", func() {
			err := ValidateOverrideSnapshotComponents(ctx, hasSnapshot, hasCompGroup)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	When("An invalid override snapshot is created", func() {
		BeforeAll(func() {
			// Add component to snapshot that is not in ComponentGroup
			nonexistentComponent := applicationapiv1alpha1.SnapshotComponent{
				Name:           "component-not-in-component-group",
				Version:        "v1",
				ContainerImage: builtImageWithDigest,
				Source: applicationapiv1alpha1.ComponentSource{
					ComponentSourceUnion: applicationapiv1alpha1.ComponentSourceUnion{
						GitSource: &applicationapiv1alpha1.GitSource{
							URL:      componentURL,
							Revision: newCommit,
						},
					},
				},
			}

			hasSnapshot.Spec.Components = append(hasSnapshot.Spec.Components, nonexistentComponent)
		})
		It("Returns an error", func() {
			err := ValidateOverrideSnapshotComponents(ctx, hasSnapshot, hasCompGroup)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("doesn't exist in componentGroup"))
		})
	})

})
