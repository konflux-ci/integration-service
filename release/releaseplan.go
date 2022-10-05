/*
Copyright 2022.

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

package release

import (
	"context"
	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	releasev1alpha1 "github.com/redhat-appstudio/release-service/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CreateReleaseForReleasePlan creates the Release for a given ReleasePlan.
func CreateReleaseForReleasePlan(releasePlan *releasev1alpha1.ReleasePlan, applicationSnapshot *applicationapiv1alpha1.ApplicationSnapshot) *releasev1alpha1.Release {
	newRelease := &releasev1alpha1.Release{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: applicationSnapshot.Name + "-",
			Namespace:    applicationSnapshot.Namespace,
		},
		Spec: releasev1alpha1.ReleaseSpec{
			ApplicationSnapshot: applicationSnapshot.Name,
			ReleasePlan:         releasePlan.Name,
		},
	}
	return newRelease
}

// FindMatchingReleaseWithReleasePlan finds a Release with given ReleasePlan given a list of Releases.
func FindMatchingReleaseWithReleasePlan(releases *[]releasev1alpha1.Release, releasePlan releasev1alpha1.ReleasePlan) *releasev1alpha1.Release {
	for _, snapshotRelease := range *releases {
		snapshotRelease := snapshotRelease // G601
		if snapshotRelease.Spec.ReleasePlan == releasePlan.Name {
			return &snapshotRelease
		}
	}
	return nil
}

// GetAutoReleasePlansForApplication returns the ReleasePlans used by the application being processed. If matching
// ReleasePlans are not found, an error will be returned. A ReleasePlan will only be returned if it has the
// release.appstudio.openshift.io/auto-release label set to true or if it is missing the label entirely.
func GetAutoReleasePlansForApplication(adapterClient client.Client, ctx context.Context, application *applicationapiv1alpha1.Application) (*[]releasev1alpha1.ReleasePlan, error) {
	releasePlans := &releasev1alpha1.ReleasePlanList{}
	labelRequirement, err := labels.NewRequirement("release.appstudio.openshift.io/auto-release", selection.NotIn, []string{"false"})
	if err != nil {
		return nil, err
	}
	labelSelector := labels.NewSelector().Add(*labelRequirement)

	opts := &client.ListOptions{
		Namespace:     application.Namespace,
		FieldSelector: fields.OneTermEqualSelector("spec.application", application.Name),
		LabelSelector: labelSelector,
	}

	err = adapterClient.List(ctx, releasePlans, opts)
	if err != nil {
		return nil, err
	}

	return &releasePlans.Items, nil
}
