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

// BuildPipelineRunSignedAndSucceededPredicate returns a predicate which filters out all objects except
// Build PipelineRuns which have finished, been signed and haven't had a Snapshot created for them.
func BuildPipelineRunSignedAndSucceededPredicate() predicate.Predicate {
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
			return IsBuildPipelineRun(e.ObjectNew) && isPipelineRunSigned(e.ObjectNew) &&
				helpers.HasPipelineRunSucceeded(e.ObjectNew) &&
				!helpers.HasAnnotation(e.ObjectNew, SnapshotNameLabel)
		},
	}
}
