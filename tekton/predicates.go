package tekton

import (
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// BuildPipelineRunSucceededPredicate returns a predicate which filters out all objects except
// PipelineRuns from the Build service which have just succeeded.
func BuildPipelineRunSucceededPredicate() predicate.Predicate {
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
			return isBuildPipelineRun(e.ObjectNew) &&
				hasPipelineSucceeded(e.ObjectOld, e.ObjectNew)
		},
	}
}