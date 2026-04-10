/*
Copyright 2024 Red Hat Inc.

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

package componentgroup

import (
	"context"

	"github.com/konflux-ci/integration-service/api/v1beta2"
	h "github.com/konflux-ci/integration-service/helpers"
	"github.com/konflux-ci/integration-service/loader"
	"github.com/konflux-ci/operator-toolkit/controller"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Adapter holds the objects needed to reconcile a ComponentGroup.
type Adapter struct {
	componentGroup *v1beta2.ComponentGroup
	logger         h.IntegrationLogger
	loader         loader.ObjectLoader
	client         client.Client
	context        context.Context
}

// NewAdapter creates and returns an Adapter instance.
func NewAdapter(
	ctx context.Context,
	componentGroup *v1beta2.ComponentGroup,
	logger h.IntegrationLogger,
	loader loader.ObjectLoader,
	client client.Client,
) *Adapter {
	return &Adapter{
		componentGroup: componentGroup,
		logger:         logger,
		loader:         loader,
		client:         client,
		context:        ctx,
	}
}

// EnsureGCLAlignedWithSpecComponents ensures the GlobalCandidateList in the ComponentGroup status
// reflects the components declared in spec.components.
func (a *Adapter) EnsureGCLAlignedWithSpecComponents() (controller.OperationResult, error) {
	retryError := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		cg, err := a.loader.GetComponentGroup(a.context, a.client, a.componentGroup.Name, a.componentGroup.Namespace)
		if err != nil {
			return err
		}
		err = a.alignGCLWithSpecComponents(cg)
		return err
	})
	if retryError != nil {
		return controller.OperationResult{}, retryError
	}
	return controller.ContinueProcessing()
}

// alignGCLWithSpecComponents aligns the GCL with the spec.components for the given componentGroup.  Uses optimistic locking
// to avoid conflicts.
// If items are added to spec.Components their ComponentVersion will be added to the GCL. Image, commit, and build time will be
// gathered from the build pipeline the next time an on-push pipeline for this componentversion completes. If items are removed from
// spec.Components their ComponentVersion will be removed from the GCL.
func (a *Adapter) alignGCLWithSpecComponents(mostRecentComponentGroup *v1beta2.ComponentGroup) error {
	a.logger.Info("Aligning GCL with spec.components")

	patch := client.MergeFromWithOptions(mostRecentComponentGroup.DeepCopy(), client.MergeFromWithOptimisticLock{})
	gclComponents := make(map[string]v1beta2.ComponentState)

	// Add existing components to GCL
	for _, component := range mostRecentComponentGroup.Status.GlobalCandidateList {
		componentVersion := h.GetComponentVersionString(component.Name, component.Version)
		gclComponents[componentVersion] = component
	}

	var newGCL []v1beta2.ComponentState

	for _, component := range mostRecentComponentGroup.Spec.Components {
		componentVersion := h.GetComponentVersionString(component.Name, component.ComponentVersion.Name)
		if existingComponent, ok := gclComponents[componentVersion]; ok {
			newGCL = append(newGCL, existingComponent)
		} else {
			// Create a new ComponentState for the componentVersion
			// All other fields are gathered from build pipeline the next time
			// an on-push pipeline for this componentversion completes
			newComponentState := v1beta2.ComponentState{
				Name:    component.Name,
				Version: component.ComponentVersion.Name,
			}
			newGCL = append(newGCL, newComponentState)
		}
	}

	mostRecentComponentGroup.Status.GlobalCandidateList = newGCL
	err := a.client.Status().Patch(a.context, mostRecentComponentGroup, patch)
	return err
}
