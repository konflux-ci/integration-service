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
	"encoding/json"

	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	"github.com/konflux-ci/integration-service/gitops"
	tektonconsts "github.com/konflux-ci/integration-service/tekton/consts"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Snapshot component functions", Ordered, func() {
	var (
		componentSnapshot *applicationapiv1alpha1.Snapshot
		overrideSnapshot  *applicationapiv1alpha1.Snapshot
		groupSnapshot     *applicationapiv1alpha1.Snapshot
	)
	const (
		componentName           = "component-sample"
		component2Name          = "component-sample-2"
		componentVersion        = "v1"
		componentURL            = "https://github.com/devfile-samples/devfile-sample-go-basic"
		builtDigest             = "sha256:841328df1b9f8c4087adbdcfec6cc99ac8308805dea83f6d415d6fb8d40227c1"
		builtImageWithoutDigest = "quay.io/konflux-ci/sample-image"
		newCommit               = "a2ba645d50e471d5f084b"
		builtImageWithDigest    = builtImageWithoutDigest + "@" + builtDigest
	)

	BeforeAll(func() {
		componentSnapshot = &applicationapiv1alpha1.Snapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "component-snapshot",
				Namespace: "default",
				Labels: map[string]string{
					gitops.SnapshotTypeLabel:              "component",
					gitops.SnapshotComponentLabel:         componentName,
					"build.appstudio.redhat.com/pipeline": "enterprise-contract",
					gitops.PipelineAsCodeEventTypeLabel:   "push",
				},
				Annotations: map[string]string{
					gitops.PipelineAsCodeInstallationIDAnnotation:      "123",
					"build.appstudio.redhat.com/commit_sha":            "6c65b2fcaea3e1a0a92476c8b5dc89e92a85f025",
					"appstudio.redhat.com/updateComponentOnSuccess":    "false",
					tektonconsts.PipelineRunComponentVersionAnnotation: componentVersion,
				},
			},
			Spec: applicationapiv1alpha1.SnapshotSpec{
				ComponentGroup: "component-group",
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
					{
						Name:           "second-component",
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

		overrideSnapshot = componentSnapshot.DeepCopy()
		overrideSnapshot.Name = "override-snapshot"
		overrideSnapshot.Labels[gitops.SnapshotTypeLabel] = "override"

		infos := []gitops.ComponentSnapshotInfo{
			{
				Namespace: "default",
				Component: componentName,
				Version:   componentVersion,
				Snapshot:  "comp-snapshot-1",
			},
			{
				Namespace: "default",
				Component: component2Name,
				Version:   componentVersion,
				Snapshot:  "comp-snapshot-2",
			},
		}
		infoJSON, err := json.Marshal(infos)
		Expect(err).NotTo(HaveOccurred())

		groupSnapshot = &applicationapiv1alpha1.Snapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "group-snapshot",
				Namespace: "default",
				Labels: map[string]string{
					gitops.SnapshotTypeLabel: gitops.SnapshotGroupType,
				},
				Annotations: map[string]string{
					gitops.GroupSnapshotInfoAnnotation: string(infoJSON),
				},
			},
			Spec: applicationapiv1alpha1.SnapshotSpec{
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
					{
						Name:           component2Name,
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

		Expect(k8sClient.Create(ctx, componentSnapshot)).Should(Succeed())
		Expect(k8sClient.Create(ctx, overrideSnapshot)).Should(Succeed())
	})

	It("Can get the new component in a component snapshot", func() {
		newComponents, err := GetAllNewComponentsInSnapshot(componentSnapshot)
		Expect(err).NotTo(HaveOccurred())
		Expect(newComponents).To(HaveLen(1))
		Expect(newComponents[0].Name).To(Equal(componentName))
	})

	It("Returns all components when getting the new components in an override snapshot", func() {
		newComponents, err := GetAllNewComponentsInSnapshot(overrideSnapshot)
		Expect(err).NotTo(HaveOccurred())
		Expect(newComponents).To(Equal(overrideSnapshot.Spec.Components))
	})

	It("Can get the new components in a group snapshot", func() {
		// The group snapshot's Spec.Components contains both components referenced in its
		// GroupSnapshotInfoAnnotation, so both should be returned.
		newComponents, err := GetAllNewComponentsInSnapshot(groupSnapshot)
		Expect(err).NotTo(HaveOccurred())
		Expect(newComponents).To(HaveLen(2))
		componentNames := []string{newComponents[0].Name, newComponents[1].Name}
		Expect(componentNames).To(ConsistOf(componentName, component2Name))
	})
})
