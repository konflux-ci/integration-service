package tekton

import (
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// IntegrationPipelineRunStartedPredicate returns a predicate which filters out all objects except
// integration PipelineRuns that have just started.
func IntegrationPipelineRunStartedPredicate() predicate.Predicate {
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
			return (IsIntegrationPipelineRun(e.ObjectNew) &&
				hasPipelineRunStateChangedToStarted(e.ObjectOld, e.ObjectNew))
		},
	}
}

// IntegrationOrBuildPipelineRunSucceededPredicate returns a predicate which filters out all objects except
// Integration and Build PipelineRuns which have just succeeded.
func IntegrationOrBuildPipelineRunSucceededPredicate() predicate.Predicate {
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
			return (IsIntegrationPipelineRun(e.ObjectNew) || IsBuildPipelineRun(e.ObjectNew)) &&
				hasPipelineRunStateChangedToSucceeded(e.ObjectOld, e.ObjectNew)
		},
	}
}
