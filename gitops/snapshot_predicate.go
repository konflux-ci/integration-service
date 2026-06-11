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

package gitops

import (
	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	"github.com/konflux-ci/operator-toolkit/metadata"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// IntegrationSnapshotChangePredicate returns a predicate which filters out all objects except
// snapshot is deleted and requires HasSnapshotTestingChangedToFinished for update events.
func IntegrationSnapshotChangePredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(createEvent event.CreateEvent) bool {
			return true
		},
		DeleteFunc: func(deleteEvent event.DeleteEvent) bool {
			return false
		},
		GenericFunc: func(genericEvent event.GenericEvent) bool {
			return true
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return HasSnapshotTestingChangedToFinished(e.ObjectOld, e.ObjectNew)
		},
	}
}

// SnapshotIntegrationTestRerunTriggerPredicate returns a predicate which filters out all objects except
// when label for rerunning an integration test is added.
func SnapshotIntegrationTestRerunTriggerPredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(createEvent event.CreateEvent) bool {
			return false
		},
		DeleteFunc: func(deleteEvent event.DeleteEvent) bool {
			return false
		},
		GenericFunc: func(genericEvent event.GenericEvent) bool {
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return HasSnapshotRerunLabelChanged(e.ObjectOld, e.ObjectNew)
		},
	}
}

// SnapshotTestAnnotationChangePredicate returns a predicate which filters out all objects except
// when Snapshot annotation "test.appstudio.openshift.io/status" is changed for update events.
func SnapshotTestAnnotationChangePredicate() predicate.Predicate {
	return predicate.Funcs{
		// Create events are triggered upon service re-sync (either every 10hrs or every restart)
		// This allows for recovery if an event is missed during those times
		CreateFunc: func(createEvent event.CreateEvent) bool {
			if snapshot, ok := createEvent.Object.(*applicationapiv1alpha1.Snapshot); ok {
				return !HaveAppStudioTestsFinished(snapshot) && metadata.HasAnnotation(snapshot, SnapshotTestsStatusAnnotation)
			}
			return false
		},
		DeleteFunc: func(deleteEvent event.DeleteEvent) bool {
			return false
		},
		GenericFunc: func(genericEvent event.GenericEvent) bool {
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return HasSnapshotTestAnnotationChanged(e.ObjectOld, e.ObjectNew)
		},
	}
}
