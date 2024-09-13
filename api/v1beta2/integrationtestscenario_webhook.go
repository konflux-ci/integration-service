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

package v1beta2

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func (r *IntegrationTestScenario) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

//+kubebuilder:webhook:path=/validate-appstudio-redhat-com-v1beta2-integrationtestscenario,mutating=false,failurePolicy=fail,sideEffects=None,groups=appstudio.redhat.com,resources=integrationtestscenarios,verbs=create;update;delete,versions=v1beta2,name=vintegrationtestscenario.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &IntegrationTestScenario{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *IntegrationTestScenario) ValidateCreate() (warnings admission.Warnings, err error) {
	// We use the DNS-1035 format for application names, so ensure it conforms to that specification
	if len(validation.IsDNS1035Label(r.Name)) != 0 {
		return nil, field.Invalid(field.NewPath("metadata").Child("name"), r.Name,
			"an IntegrationTestScenario resource name must start with a lower case "+
				"alphabetical character, be under 63 characters, and can only consist "+
				"of lower case alphanumeric characters or ‘-’")
	}

	// see stoneintg-896
	for _, param := range r.Spec.Params {
		if param.Name == "SNAPSHOT" {
			return nil, field.Invalid(field.NewPath("Spec").Child("Params"), param.Name,
				"an IntegrationTestScenario resource should not have the SNAPSHOT "+
					"param manually defined because it will be automatically generated"+
					"by the integration service")
		}
		// we won't enable ITS if git resolver with url & repo+org
		urlResolverExist := false
		repoResolverExist := false
		orgResolverExist := false

		for _, gitResolverParam := range r.Spec.ResolverRef.Params {
			if gitResolverParam.Name == "url" && gitResolverParam.Value != "" {
				urlResolverExist = true
			}
			if gitResolverParam.Name == "repo" && gitResolverParam.Value != "" {
				repoResolverExist = true
			}
			if gitResolverParam.Name == "org" && gitResolverParam.Value != "" {
				orgResolverExist = true
			}
		}

		if urlResolverExist {
			if repoResolverExist || orgResolverExist {
				return nil, field.Invalid(field.NewPath("Spec").Child("ResolverRef").Child("Params"), param.Name,
					"an IntegrationTestScenario resource can only have one of the gitResolver parameters,"+
						"either url or repo (with org), but not both.")

			}
		} else {
			if !repoResolverExist || !orgResolverExist {
				return nil, field.Invalid(field.NewPath("Spec").Child("ResolverRef").Child("Params"), param.Name,
					"IntegrationTestScenario is invalid: missing mandatory repo or org parameters."+
						"If both are absent, a valid url is highly recommended.")

			}
		}

	}

	return nil, nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *IntegrationTestScenario) ValidateUpdate(old runtime.Object) (warnings admission.Warnings, err error) {
	return nil, nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *IntegrationTestScenario) ValidateDelete() (warnings admission.Warnings, err error) {
	return nil, nil
}
