package v1alpha1

import "context"

// ManagedByLabelKey is the label key used to mark what is managing this resource
const ManagedByLabelKey = "app.kubernetes.io/managed-by"

// SetDefaults walks a ResolutionRequest object and sets any default
// values that are required to be set before a reconciler sees it.
func (rr *ResolutionRequest) SetDefaults(ctx context.Context) {
	if rr.TypeMeta.Kind == "" {
		rr.TypeMeta.Kind = "ResolutionRequest"
	}
	if rr.TypeMeta.APIVersion == "" {
		rr.TypeMeta.APIVersion = "resolution.tekton.dev/v1alpha1"
	}
}
