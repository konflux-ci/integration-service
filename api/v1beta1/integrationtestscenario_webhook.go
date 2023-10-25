/*
Copyright 2023 Red Hat Inc.

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

package v1beta1

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

func (r *IntegrationTestScenario) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

//+kubebuilder:webhook:path=/validate-appstudio-redhat-com-v1beta1-integrationtestscenario,mutating=false,failurePolicy=fail,sideEffects=None,groups=appstudio.redhat.com,resources=integrationtestscenarios,verbs=create;update;delete,versions=v1beta1,name=vintegrationtestscenario.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &IntegrationTestScenario{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *IntegrationTestScenario) ValidateCreate() error {
	// We use the DNS-1035 format for application names, so ensure it conforms to that specification

	if len(validation.IsDNS1035Label(r.Name)) != 0 {
		return fmt.Errorf("invalid scenario name: %q: an IntegrationTestScenario resource name must start with a lower case "+
			"alphabetical character, be under 63 characters, and can only consist of lower case alphanumeric characters or ‘-’,",
			r.Name)
	}
	return nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *IntegrationTestScenario) ValidateUpdate(old runtime.Object) error {
	return nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *IntegrationTestScenario) ValidateDelete() error {
	return nil
}
