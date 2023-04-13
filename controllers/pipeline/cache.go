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

package pipeline

import (
	"context"

	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/api/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// SetupApplicationComponentCache adds a new index field to be able to search Components by application.
func SetupApplicationComponentCache(mgr ctrl.Manager) error {
	applicationComponentIndexFunc := func(obj client.Object) []string {
		return []string{obj.(*applicationapiv1alpha1.Component).Spec.Application}
	}

	return mgr.GetCache().IndexField(context.Background(), &applicationapiv1alpha1.Component{},
		"spec.application", applicationComponentIndexFunc)
}

// SetupSnapshotCache adds a new index field to be able to search Snapshots by Application.
func SetupSnapshotCache(mgr ctrl.Manager) error {
	snapshotIndexFunc := func(obj client.Object) []string {
		return []string{obj.(*applicationapiv1alpha1.Snapshot).Spec.Application}
	}

	return mgr.GetCache().IndexField(context.Background(), &applicationapiv1alpha1.Snapshot{},
		"spec.application", snapshotIndexFunc)
}

// SetupIntegrationTestScenarioCache adds a new index field to be able to search IntegrationTestScenarios by Application.
func SetupIntegrationTestScenarioCache(mgr ctrl.Manager) error {
	integrationTestScenariosIndexFunc := func(obj client.Object) []string {
		return []string{obj.(*v1beta1.IntegrationTestScenario).Spec.Application}
	}

	return mgr.GetCache().IndexField(context.Background(), &v1beta1.IntegrationTestScenario{},
		"spec.application", integrationTestScenariosIndexFunc)
}
