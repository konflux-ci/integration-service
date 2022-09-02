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

package snapshot

import (
	"context"
	appstudioshared "github.com/redhat-appstudio/managed-gitops/appstudio-shared/apis/appstudio.redhat.com/v1alpha1"
	releasev1alpha1 "github.com/redhat-appstudio/release-service/api/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// SetupReleasePlanCache adds a new index field to be able to search ReleasePlans by application.
func SetupReleasePlanCache(mgr ctrl.Manager) error {
	releasePlanIndexFunc := func(obj client.Object) []string {
		return []string{obj.(*releasev1alpha1.ReleasePlan).Spec.Application}
	}

	return mgr.GetCache().IndexField(context.Background(), &releasev1alpha1.ReleasePlan{},
		"spec.application", releasePlanIndexFunc)
}

// SetupReleaseCache adds a new index field to be able to search Releases by ApplicationSnapshot.
func SetupReleaseCache(mgr ctrl.Manager) error {
	releaseIndexFunc := func(obj client.Object) []string {
		return []string{obj.(*releasev1alpha1.Release).Spec.ApplicationSnapshot}
	}

	return mgr.GetCache().IndexField(context.Background(), &releasev1alpha1.Release{},
		"spec.applicationSnapshot", releaseIndexFunc)
}

// SetupApplicationCache adds a new index field to be able to search Applications by Environment.
func SetupApplicationCache(mgr ctrl.Manager) error {
	applicationIndexFunc := func(obj client.Object) []string {
		return []string{obj.(*appstudioshared.ApplicationSnapshotEnvironmentBinding).Spec.Environment}
	}

	return mgr.GetCache().IndexField(context.Background(), &appstudioshared.ApplicationSnapshotEnvironmentBinding{},
		"spec.environment", applicationIndexFunc)
}

// SetupEnvironmentCache adds a new index field to be able to search Environments by Application.
func SetupEnvironmentCache(mgr ctrl.Manager) error {
	environmentIndexFunc := func(obj client.Object) []string {
		return []string{obj.(*appstudioshared.ApplicationSnapshotEnvironmentBinding).Spec.Application}
	}

	return mgr.GetCache().IndexField(context.Background(), &appstudioshared.ApplicationSnapshotEnvironmentBinding{},
		"spec.application", environmentIndexFunc)
}
