package tekton

import (
	"github.com/redhat-appstudio/integration-service/helpers"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// IntegrationPipelineRunPredicate returns a predicate which filters out all objects except
// integration PipelineRuns that have just started or finished.
func IntegrationPipelineRunPredicate() predicate.Predicate {
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
			return IsIntegrationPipelineRun(e.ObjectNew) &&
				(hasPipelineRunStateChangedToStarted(e.ObjectOld, e.ObjectNew) || hasPipelineRunStateChangedToFinished(e.ObjectOld, e.ObjectNew))
		},
	}
}

// BuildPipelineRunFinishedPredicate returns a predicate which filters out all objects except
// Build PipelineRuns which have finished and been just signed.
func BuildPipelineRunFinishedPredicate() predicate.Predicate {
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
			return IsBuildPipelineRun(e.ObjectNew) && isPipelineRunSigned(e.ObjectNew) && helpers.HasPipelineRunSucceeded(e.ObjectNew)
		},
	}
}
