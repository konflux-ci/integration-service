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

package gitops

import (
	"github.com/redhat-appstudio/integration-service/helpers"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// DeploymentFinishedForIntegrationBindingPredicate returns a predicate which filters out update events to a
// SnapshotEnvironmentBinding that has the IntegrationTestScenario specified in the label and
// where the component deployment status goes from unknown to true/false.
func DeploymentFinishedForIntegrationBindingPredicate() predicate.Predicate {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			return hasDeploymentFinished(e.ObjectOld, e.ObjectNew) && helpers.HasLabel(e.ObjectNew, SnapshotTestScenarioLabel)
		},
	}
}
