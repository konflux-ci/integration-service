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
	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	releasev1alpha1 "github.com/redhat-appstudio/release-service/api/v1alpha1"
	releasemetadata "github.com/redhat-appstudio/release-service/metadata"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NewReleaseForReleasePlan creates the Release for a given ReleasePlan.
func NewReleaseForReleasePlan(releasePlan *releasev1alpha1.ReleasePlan, snapshot *applicationapiv1alpha1.Snapshot) *releasev1alpha1.Release {
	newRelease := &releasev1alpha1.Release{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: snapshot.Name + "-",
			Namespace:    snapshot.Namespace,
			Labels: map[string]string{
				releasemetadata.AutomatedLabel: "true",
			},
		},
		Spec: releasev1alpha1.ReleaseSpec{
			Snapshot:    snapshot.Name,
			ReleasePlan: releasePlan.Name,
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
