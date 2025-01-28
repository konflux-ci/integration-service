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

package release_test

import (
	"strings"

	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	"github.com/konflux-ci/integration-service/gitops"
	integrationservicerelease "github.com/konflux-ci/integration-service/release"
	releasev1alpha1 "github.com/konflux-ci/release-service/api/v1alpha1"
	releasemetadata "github.com/konflux-ci/release-service/metadata"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Release functions for managing Releases", Ordered, func() {

	var (
		hasSnapshot *applicationapiv1alpha1.Snapshot
		hasApp      *applicationapiv1alpha1.Application
		releasePlan *releasev1alpha1.ReleasePlan
	)

	const (
		namespace = "default"
	)

	BeforeAll(func() {
		hasApp = &applicationapiv1alpha1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "application-sample",
				Namespace: namespace,
			},
			Spec: applicationapiv1alpha1.ApplicationSpec{
				DisplayName: "application-sample",
				Description: "This is an example application",
			},
		}
		Expect(k8sClient.Create(ctx, hasApp)).Should(Succeed())

		hasSnapshot = &applicationapiv1alpha1.Snapshot{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "snapshot-sample-",
				Namespace:    namespace,
			},
			Spec: applicationapiv1alpha1.SnapshotSpec{
				Application: "application-sample",
				Components:  []applicationapiv1alpha1.SnapshotComponent{},
			},
		}
		Expect(k8sClient.Create(ctx, hasSnapshot)).Should(Succeed())

		releasePlan = &releasev1alpha1.ReleasePlan{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "releaseplan-sample-",
				Namespace:    namespace,
				Labels: map[string]string{
					releasemetadata.AutoReleaseLabel: "true",
				},
			},
			Spec: releasev1alpha1.ReleasePlanSpec{
				Application: "application-sample",
				Target:      "default",
			},
		}
		Expect(k8sClient.Create(ctx, releasePlan)).Should(Succeed())
	})

	AfterAll(func() {
		err := k8sClient.Delete(ctx, hasApp)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, hasSnapshot)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, releasePlan)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
	})

	It("ensures the Release can be created for ReleasePlan and is labelled as automated", func() {
		createdRelease := integrationservicerelease.NewReleaseForReleasePlan(ctx, releasePlan, hasSnapshot)
		Expect(createdRelease.Spec.ReleasePlan).To(Equal(releasePlan.Name))
		Expect(createdRelease.GetLabels()[releasemetadata.AutomatedLabel]).To(Equal("true"))
	})

	It("ensures that due to missing SHA annotation in Snapshot, Release name doesnt contain SHA in it", func() {
		createdRelease := integrationservicerelease.NewReleaseForReleasePlan(ctx, releasePlan, hasSnapshot)
		Expect(k8sClient.Create(ctx, createdRelease)).Should(Succeed())
		Expect(strings.Split(createdRelease.Name, "-")).To(HaveLen(4)) // SHA value missing from Release's name
	})

	It("ensures that due to presence of SHA annotation in Snapshot, Release name contains SHA in it", func() {
		ann := map[string]string{}
		ann[gitops.PipelineAsCodeSHAAnnotation] = "imshahahaha"
		hasSnapshot.SetAnnotations(ann)
		Expect(k8sClient.Update(ctx, hasSnapshot)).Should(Succeed())

		createdRelease := integrationservicerelease.NewReleaseForReleasePlan(ctx, releasePlan, hasSnapshot)
		Expect(k8sClient.Create(ctx, createdRelease)).Should(Succeed())
		Expect(strings.Split(createdRelease.Name, "-")).To(HaveLen(5)) // must include SHA value in Name
		Expect(createdRelease.Name).To(ContainSubstring("imshaha"))    // Release name should contain only first 7 chars of SHA
	})

	It("ensures that due to presence of SHA annotation with empty value, Release name doesn't have SHA in it", func() {
		ann := map[string]string{}
		ann[gitops.PipelineAsCodeSHAAnnotation] = ""
		hasSnapshot.SetAnnotations(ann)
		Expect(k8sClient.Update(ctx, hasSnapshot)).Should(Succeed())

		createdRelease := integrationservicerelease.NewReleaseForReleasePlan(ctx, releasePlan, hasSnapshot)
		Expect(k8sClient.Create(ctx, createdRelease)).Should(Succeed())
		Expect(strings.Split(createdRelease.Name, "-")).To(HaveLen(4)) // SHA value missing from Release's name
	})

	It("ensures the matching Release can be found for ReleasePlan", func() {
		releases := []releasev1alpha1.Release{
			{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "release-sample-",
					Namespace:    namespace,
				},
				Spec: releasev1alpha1.ReleaseSpec{
					Snapshot:    hasSnapshot.GetName(),
					ReleasePlan: releasePlan.GetName(),
				},
			},
		}
		foundMatchingRelease := integrationservicerelease.FindMatchingReleaseWithReleasePlan(&releases, *releasePlan)
		Expect(foundMatchingRelease.Spec.ReleasePlan).To(Equal(releasePlan.Name))
	})
})
