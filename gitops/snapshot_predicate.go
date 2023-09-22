package gitops

import (
	"github.com/redhat-appstudio/operator-toolkit/metadata"
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

// PRSnapshotTestAnnotationChangePredicate returns a predicate which filters out all objects except
// pull_request snapshot annotation "test.appstudio.openshift.io/status" is changed for update events.
func PRSnapshotTestAnnotationChangePredicate() predicate.Predicate {
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
			return HasSnapshotTestAnnotationChanged(e.ObjectOld, e.ObjectNew) && metadata.HasLabelWithValue(e.ObjectNew, PipelineAsCodeEventTypeLabel, PipelineAsCodePullRequestType)
		},
	}
}
