/*
Copyright 2022 Red Hat Inc.

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

package snapshot

import (
	"bytes"
	"fmt"
	"time"

	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	"github.com/konflux-ci/integration-service/api/v1beta2"
	"github.com/konflux-ci/integration-service/helpers"
	"github.com/konflux-ci/integration-service/loader"
	tektonconsts "github.com/konflux-ci/integration-service/tekton/consts"
	toolkit "github.com/konflux-ci/operator-toolkit/loader"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	"github.com/tonglil/buflogr"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"knative.dev/pkg/apis"
	v1 "knative.dev/pkg/apis/duck/v1"
)

var _ = Describe("GCL manipulation functions", Ordered, func() {
	var (
		buildPipelineRun      *tektonv1.PipelineRun
		hasCompGroup          *v1beta2.ComponentGroup
		updatedComponentGroup *v1beta2.ComponentGroup
		buildStartTime        *metav1.Time
		oldEntry              v1beta2.ComponentState
		newEntry              v1beta2.ComponentState
		componentReference1   v1beta2.ComponentReference
		componentReference2   v1beta2.ComponentReference
	)
	const (
		componentName           = "component-sample"
		componentVersion        = "v1"
		componentURL            = "https://github.com/devfile-samples/devfile-sample-go-basic"
		builtCommit             = "c713067b0e65fb3de50d1f7c457eb51c2ab0dbb0"
		builtDigest             = "sha256:841328df1b9f8c4087adbdcfec6cc99ac8308805dea83f6d415d6fb8d40227c1"
		builtImageWithoutDigest = "quay.io/konflux-ci/sample-image"
		newCommit               = "a2ba645d50e471d5f084b"
		newImageWithDigest      = "quay.io/konflux-ci/integration-service@sha256:0987654321fedcba"
		overrideSnapshotURL     = "https://github.com/devfile-samples/devfile-sample-go-basic-override"
		overrideSnapshotCommit  = "5154ad273e1738d6fd0747d43e47c77b12da5f35"
		overrideSnapshotImage   = "quay.io/konflux-ci/override-snapshot@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
		anotherComponentURL     = "https://github.com/example/another-component-repo"
		anotherComponentCommit  = "0b59816a6f548f5a46e6aafb606f2aa178e14a22"
		anotherComponentImage   = "quay.io/konflux-ci/another-sample@sha256:deabe80a01dca3a8a0edb709324e30cbf0baa176f7a181bbb695323f506f7aac"
	)

	Context("testing GCL entry update", func() {
		BeforeAll(func() {
			buildPipelineRun = &tektonv1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pipelinerun-build-sample",
					Namespace: "default",
					Labels: map[string]string{
						"pipelines.appstudio.openshift.io/type":    "build",
						"pipelines.openshift.io/used-by":           "build-cloud",
						"pipelines.openshift.io/runtime":           "nodejs",
						"pipelines.openshift.io/strategy":          "s2i",
						"appstudio.openshift.io/component":         componentName,
						"build.appstudio.redhat.com/target_branch": "main",
						"pipelinesascode.tekton.dev/event-type":    "pull_request",
						"pipelinesascode.tekton.dev/pull-request":  "1",
					},
					Annotations: map[string]string{
						"appstudio.redhat.com/updateComponentOnSuccess":    "false",
						"pipelinesascode.tekton.dev/on-target-branch":      "[main,master]",
						"build.appstudio.openshift.io/repo":                "https://github.com/devfile-samples/devfile-sample-go-basic?rev=c713067b0e65fb3de50d1f7c457eb51c2ab0dbb0",
						"foo":                                              "bar",
						"chains.tekton.dev/signed":                         "true",
						"pipelinesascode.tekton.dev/source-branch":         "sourceBranch",
						"pipelinesascode.tekton.dev/url-org":               "redhat",
						tektonconsts.PipelineRunComponentVersionAnnotation: componentVersion,
					},
					CreationTimestamp: metav1.Time{Time: time.Now()},
				},
				Spec: tektonv1.PipelineRunSpec{
					PipelineRef: &tektonv1.PipelineRef{
						Name: "build-pipeline-pass",
						ResolverRef: tektonv1.ResolverRef{
							Resolver: "bundle",
							Params: tektonv1.Params{
								{Name: "bundle",
									Value: tektonv1.ParamValue{Type: "string", StringVal: "quay.io/redhat-appstudio/example-tekton-bundle:test"},
								},
								{Name: "name",
									Value: tektonv1.ParamValue{Type: "string", StringVal: "test-task"},
								},
							},
						},
					},
					Params: []tektonv1.Param{
						{
							Name: "output-image",
							Value: tektonv1.ParamValue{
								Type:      tektonv1.ParamTypeString,
								StringVal: builtImageWithoutDigest,
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, buildPipelineRun)).Should(Succeed())

			buildStartTime = &metav1.Time{Time: time.Now()}
			buildPipelineRun.Status = tektonv1.PipelineRunStatus{
				PipelineRunStatusFields: tektonv1.PipelineRunStatusFields{
					Results: []tektonv1.PipelineRunResult{
						{
							Name:  "IMAGE_DIGEST",
							Value: *tektonv1.NewStructuredValues(builtDigest),
						},
						{
							Name:  "IMAGE_URL",
							Value: *tektonv1.NewStructuredValues(builtImageWithoutDigest),
						},
						{
							Name:  "CHAINS-GIT_URL",
							Value: *tektonv1.NewStructuredValues(componentURL),
						},
						{
							Name:  "CHAINS-GIT_COMMIT",
							Value: *tektonv1.NewStructuredValues(newCommit),
						},
					},
					StartTime: buildStartTime,
				},
				Status: v1.Status{
					Conditions: v1.Conditions{
						apis.Condition{
							Reason: "Completed",
							Status: "True",
							Type:   apis.ConditionSucceeded,
						},
					},
				},
			}
			Expect(k8sClient.Status().Update(ctx, buildPipelineRun)).Should(Succeed())

			oldEntry = v1beta2.ComponentState{
				Name:                  componentName,
				Version:               componentVersion,
				URL:                   componentURL,
				LastPromotedImage:     fmt.Sprintf("%s@%s", builtImageWithoutDigest, builtDigest),
				LastPromotedCommit:    builtCommit,
				LastPromotedBuildTime: &metav1.Time{Time: time.Now()},
			}

			componentReference1 = v1beta2.ComponentReference{
				Name: componentName,
				ComponentVersion: v1beta2.ComponentVersionReference{
					Name:     componentVersion,
					Revision: "main",
				},
			}
			componentReference2 = v1beta2.ComponentReference{
				Name: "another-component-sample",
				ComponentVersion: v1beta2.ComponentVersionReference{
					Name:     "v1",
					Revision: "main",
				},
			}

			hasCompGroup = &v1beta2.ComponentGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "component-group-sample",
					Namespace: "default",
				},
				Spec: v1beta2.ComponentGroupSpec{
					Components: []v1beta2.ComponentReference{componentReference1, componentReference2},
				},
			}
			Expect(k8sClient.Create(ctx, hasCompGroup)).Should(Succeed())

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
					oldEntry,
				},
			}
			Expect(k8sClient.Status().Update(ctx, hasCompGroup)).Should(Succeed())
		})

		AfterAll(func() {
			err := k8sClient.Delete(ctx, buildPipelineRun)
			Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
			err = k8sClient.Delete(ctx, hasCompGroup)
			Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
		})

		When("an entry matching a componentVersion is added", func() {
			BeforeAll(func() {
				updatedComponentGroup = hasCompGroup.DeepCopy()
				newEntry = v1beta2.ComponentState{
					Name:                  componentName,
					Version:               componentVersion,
					URL:                   componentURL,
					LastPromotedCommit:    newCommit,
					LastPromotedImage:     newImageWithDigest,
					LastPromotedBuildTime: &metav1.Time{Time: time.Now()},
				}
				err := UpdateGCLEntry(ctx, k8sClient, updatedComponentGroup, newEntry)
				Expect(err).NotTo(HaveOccurred())

				Eventually(func() error {
					err := k8sClient.Get(ctx, types.NamespacedName{
						Name:      updatedComponentGroup.Name,
						Namespace: updatedComponentGroup.Namespace,
					}, updatedComponentGroup)
					return err
				}, time.Second*10).ShouldNot(HaveOccurred())
			})
			AfterAll(func() {
				err := k8sClient.Delete(ctx, updatedComponentGroup)
				Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
			})
			It("replaces the old entry in the list", func() {
				// updatedComponentGroup contains a Componentversion matching newEntry
				// ComponentVersion has correct URL, LastPromotedCommit, and LastPromotedImage
				found := false
				for _, component := range updatedComponentGroup.Status.GlobalCandidateList {
					if component.Name == newEntry.Name && component.Version == newEntry.Version {
						Expect(found).To(BeFalse(), "[ERROR] The new ComponentVersion was added to the GCL as a new entry. It should have replaced the old ComponentVersion")

						found = true
						Expect(component.URL).To(Equal(newEntry.URL))
						Expect(component.LastPromotedCommit).To(Equal(newEntry.LastPromotedCommit))
						Expect(component.LastPromotedImage).To(Equal(newEntry.LastPromotedImage))
					}
				}
				Expect(found).To(BeTrue(), "[ERROR] The ComponentVersion was not found in the GCL")
			})
		})

		When("an entry with no matching componentVersion is added", func() {
			BeforeAll(func() {
				newEntry = v1beta2.ComponentState{
					Name:                  componentName,
					Version:               componentVersion,
					URL:                   componentURL,
					LastPromotedCommit:    newCommit,
					LastPromotedImage:     newImageWithDigest,
					LastPromotedBuildTime: &metav1.Time{Time: time.Now()},
				}
			})
			It("returns an error", func() {
				compGroupNoGCL := hasCompGroup.DeepCopy()
				// Remove status from compGroupNoGCL
				compGroupNoGCL.Status = v1beta2.ComponentGroupStatus{
					GlobalCandidateList: []v1beta2.ComponentState{},
				}

				err := UpdateGCLEntry(ctx, k8sClient, compGroupNoGCL, newEntry)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("could not find ComponentVersion"))
			})
		})

		When("an entry with an older LastPromotedBuildTime is added", func() {
			It("does not add the entry to the GCL", func() {
				newEntryOldBuild := newEntry.DeepCopy()
				newEntryOldBuild.LastPromotedBuildTime = &metav1.Time{Time: time.Date(2025, 12, 31, 12, 0, 0, 0, time.UTC)}

				err := UpdateGCLEntry(ctx, k8sClient, hasCompGroup, *newEntryOldBuild)
				Expect(err).NotTo(HaveOccurred())
				for _, component := range hasCompGroup.Status.GlobalCandidateList {
					if component.Name == newEntry.Name && component.Version == newEntry.Version {
						Expect(component.URL).To(Equal(oldEntry.URL))
						Expect(component.LastPromotedCommit).To(Equal(oldEntry.LastPromotedCommit))
						Expect(component.LastPromotedImage).To(Equal(oldEntry.LastPromotedImage))
					}
				}
			})
		})
	})

	Context("Testing override snapshot GCL update", func() {
		var (
			overrideSnapshot            *applicationapiv1alpha1.Snapshot
			anotherGCL                  v1beta2.ComponentState
			secondGCLComponentBuildTime *metav1.Time
		)

		BeforeAll(func() {
			oldEntry = v1beta2.ComponentState{
				Name:                  componentName,
				Version:               componentVersion,
				URL:                   componentURL,
				LastPromotedImage:     fmt.Sprintf("%s@%s", builtImageWithoutDigest, builtDigest),
				LastPromotedCommit:    builtCommit,
				LastPromotedBuildTime: &metav1.Time{Time: time.Now()},
			}
			anotherGCL = v1beta2.ComponentState{
				Name:                  "another-component-sample",
				Version:               componentVersion,
				URL:                   anotherComponentURL,
				LastPromotedCommit:    anotherComponentCommit,
				LastPromotedImage:     anotherComponentImage,
				LastPromotedBuildTime: &metav1.Time{Time: time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)},
			}

			componentReference1 = v1beta2.ComponentReference{
				Name: componentName,
				ComponentVersion: v1beta2.ComponentVersionReference{
					Name:     componentVersion,
					Revision: "main",
				},
			}
			componentReference2 = v1beta2.ComponentReference{
				Name: "another-component-sample",
				ComponentVersion: v1beta2.ComponentVersionReference{
					Name:     "v1",
					Revision: "main",
				},
			}

			hasCompGroup = &v1beta2.ComponentGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "component-group-sample",
					Namespace: "default",
				},
				Spec: v1beta2.ComponentGroupSpec{
					Components: []v1beta2.ComponentReference{componentReference1, componentReference2},
				},
			}
			Expect(k8sClient.Create(ctx, hasCompGroup)).Should(Succeed())

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
					oldEntry,
					anotherGCL,
				},
			}
			Expect(k8sClient.Status().Update(ctx, hasCompGroup)).Should(Succeed())
		})

		AfterAll(func() {
			err := k8sClient.Delete(ctx, hasCompGroup)
			Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
		})

		When("The GCL is updated for override snapshot", func() {
			BeforeEach(func() {

				secondGCLComponentBuildTime = hasCompGroup.Status.GlobalCandidateList[1].LastPromotedBuildTime
				// Snapshot with one component matching componentName/componentVersion;
				// containerImage and source differ from baselineSample GCL entry.
				overrideSnapshot = &applicationapiv1alpha1.Snapshot{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "override-snapshot-gcl-test",
						Namespace: "default",
					},
					Spec: applicationapiv1alpha1.SnapshotSpec{
						ComponentGroup: hasCompGroup.Name,
						Components: []applicationapiv1alpha1.SnapshotComponent{
							{
								Name:           componentName,
								Version:        componentVersion,
								ContainerImage: overrideSnapshotImage,
								Source: applicationapiv1alpha1.ComponentSource{
									ComponentSourceUnion: applicationapiv1alpha1.ComponentSourceUnion{
										GitSource: &applicationapiv1alpha1.GitSource{
											URL:      overrideSnapshotURL,
											Revision: overrideSnapshotCommit,
										},
									},
								},
							},
						},
					},
				}

				Eventually(func() error {
					err := k8sClient.Get(ctx, types.NamespacedName{
						Name:      hasCompGroup.Name,
						Namespace: "default",
					}, hasCompGroup)
					return err
				}, time.Second*10).ShouldNot(HaveOccurred())

				log := helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(&bytes.Buffer{})}
				mockContext := toolkit.GetMockedContext(ctx, []toolkit.MockData{
					{
						ContextKey: loader.ComponentGroupContextKey,
						Resource:   hasCompGroup,
					},
				})
				err := UpdateGCLForOverrideSnapshot(mockContext, k8sClient, loader.NewLoader(), hasCompGroup, overrideSnapshot, log)
				Expect(err).NotTo(HaveOccurred())
				time.Sleep(3 * time.Second)
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      hasCompGroup.Name,
					Namespace: hasCompGroup.Namespace,
				}, hasCompGroup)).To(Succeed())
			})

			It("Can update the GCL without error", func() {
				Expect(true).To(BeTrue())
			})

			It("Should update the GCL for components in the snapshot", func() {
				var updatedComponent *v1beta2.ComponentState
				for i := range hasCompGroup.Status.GlobalCandidateList {
					c := &hasCompGroup.Status.GlobalCandidateList[i]
					if c.Name == componentName && c.Version == componentVersion {
						updatedComponent = c
						break
					}
				}
				Expect(updatedComponent).NotTo(BeNil(), "expected GCL entry for %s/%s", componentName, componentVersion)
				Expect(updatedComponent.URL).To(Equal(overrideSnapshotURL))
				Expect(updatedComponent.LastPromotedCommit).To(Equal(overrideSnapshotCommit))
				Expect(updatedComponent.LastPromotedImage).To(Equal(overrideSnapshotImage))
			})

			It("Should not have updated the GCL for a component not in the snapshot", func() {
				var secondComponent *v1beta2.ComponentState
				for i := range hasCompGroup.Status.GlobalCandidateList {
					c := &hasCompGroup.Status.GlobalCandidateList[i]
					if c.Name == "another-component-sample" && c.Version == componentVersion {
						secondComponent = c
						break
					}
				}
				Expect(secondComponent).NotTo(BeNil(), "expected GCL entry for another-component-sample/%s", componentVersion)
				Expect(secondComponent.URL).To(Equal(anotherGCL.URL))
				Expect(secondComponent.LastPromotedCommit).To(Equal(anotherGCL.LastPromotedCommit))
				Expect(secondComponent.LastPromotedImage).To(Equal(anotherGCL.LastPromotedImage))
				Expect(secondComponent.LastPromotedBuildTime).To(Equal(secondGCLComponentBuildTime))
			})
		})

	})
})
