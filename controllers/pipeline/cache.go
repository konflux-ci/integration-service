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
	hasv1alpha1 "github.com/redhat-appstudio/application-service/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/api/v1alpha1"
	appstudioshared "github.com/redhat-appstudio/managed-gitops/appstudio-shared/apis/appstudio.redhat.com/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// SetupApplicationComponentCache adds a new index field to be able to search Components by application.
func SetupApplicationComponentCache(mgr ctrl.Manager) error {
	applicationComponentIndexFunc := func(obj client.Object) []string {
		return []string{obj.(*hasv1alpha1.Component).Spec.Application}
	}

	return mgr.GetCache().IndexField(context.Background(), &hasv1alpha1.Component{},
		"spec.application", applicationComponentIndexFunc)
}

// SetupApplicationSnapshotCache adds a new index field to be able to search ApplicationSnapshots by Application.
func SetupApplicationSnapshotCache(mgr ctrl.Manager) error {
	applicationSnapshotIndexFunc := func(obj client.Object) []string {
		return []string{obj.(*appstudioshared.ApplicationSnapshot).Spec.Application}
	}

	return mgr.GetCache().IndexField(context.Background(), &appstudioshared.ApplicationSnapshot{},
		"spec.application", applicationSnapshotIndexFunc)
}

// SetupIntegrationTestScenarioCache adds a new index field to be able to search IntegrationTestScenarios by Application.
func SetupIntegrationTestScenarioCache(mgr ctrl.Manager) error {
	integrationTestScenariosIndexFunc := func(obj client.Object) []string {
		return []string{obj.(*v1alpha1.IntegrationTestScenario).Spec.Application}
	}

	return mgr.GetCache().IndexField(context.Background(), &v1alpha1.IntegrationTestScenario{},
		"spec.application", integrationTestScenariosIndexFunc)
}
