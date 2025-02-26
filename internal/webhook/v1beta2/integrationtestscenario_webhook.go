/*
Copyright 2025.

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

package v1beta2

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	appstudiov1beta2 "github.com/konflux-ci/integration-service/api/v1beta2"
)

// nolint:unused
// log is for logging in this package.
var integrationtestscenariolog = logf.Log.WithName("integrationtestscenario-resource")

// SetupIntegrationTestScenarioWebhookWithManager registers the webhook for IntegrationTestScenario in the manager.
func SetupIntegrationTestScenarioWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&appstudiov1beta2.IntegrationTestScenario{}).
		WithValidator(&IntegrationTestScenarioCustomValidator{}).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-appstudio-redhat-com-v1beta2-integrationtestscenario,mutating=false,failurePolicy=fail,sideEffects=None,groups=appstudio.redhat.com,resources=integrationtestscenarios,verbs=create;update,versions=v1beta2,name=vintegrationtestscenario-v1beta2.kb.io,admissionReviewVersions=v1

// IntegrationTestScenarioCustomValidator struct is responsible for validating the IntegrationTestScenario resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type IntegrationTestScenarioCustomValidator struct {
	// TODO(user): Add more fields as needed for validation
}

var _ webhook.CustomValidator = &IntegrationTestScenarioCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type IntegrationTestScenario.
func (v *IntegrationTestScenarioCustomValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	integrationtestscenario, ok := obj.(*appstudiov1beta2.IntegrationTestScenario)
	if !ok {
		return nil, fmt.Errorf("expected a IntegrationTestScenario object but got %T", obj)
	}
	integrationtestscenariolog.Info("Validation for IntegrationTestScenario upon creation", "name", integrationtestscenario.GetName())

	// TODO(user): fill in your validation logic upon object creation.

	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type IntegrationTestScenario.
func (v *IntegrationTestScenarioCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	integrationtestscenario, ok := newObj.(*appstudiov1beta2.IntegrationTestScenario)
	if !ok {
		return nil, fmt.Errorf("expected a IntegrationTestScenario object for the newObj but got %T", newObj)
	}
	integrationtestscenariolog.Info("Validation for IntegrationTestScenario upon update", "name", integrationtestscenario.GetName())

	// TODO(user): fill in your validation logic upon object update.

	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type IntegrationTestScenario.
func (v *IntegrationTestScenarioCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	integrationtestscenario, ok := obj.(*appstudiov1beta2.IntegrationTestScenario)
	if !ok {
		return nil, fmt.Errorf("expected a IntegrationTestScenario object but got %T", obj)
	}
	integrationtestscenariolog.Info("Validation for IntegrationTestScenario upon deletion", "name", integrationtestscenario.GetName())

	// TODO(user): fill in your validation logic upon object deletion.

	return nil, nil
}
