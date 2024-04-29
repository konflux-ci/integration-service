package component

import (
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// ComponentCreatedPredicate returns a predicate which filters out
// only components that are the result of a create event
func ComponentCreatedPredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(createEvent event.CreateEvent) bool {
			return true
		},
		DeleteFunc: func(deleteEvent event.DeleteEvent) bool {
			return false
		},
		GenericFunc: func(genericEvent event.GenericEvent) bool {
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return false
		},
	}
}

// ComponentDeletedPredicate returns a predicate which filters out
// only components that have been marked for deletion.
// This is achieved by watching updates to components with an update to
// the components DeletionTimestamp has been updated
func ComponentDeletedPredicate() predicate.Predicate {
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
			return hasComponentChangedToDeleting(e.ObjectOld, e.ObjectNew)
		},
	}
}
