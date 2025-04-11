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
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"context"
	"errors"
	"fmt"
	neturl "net/url"
	"regexp"
	"strings"
)

// nolint:unused
// log is for logging in this package.
var integrationtestscenariolog = logf.Log.WithName("integrationtestscenario-webhook")

func (r *IntegrationTestScenario) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		WithDefaulter(&IntegrationTestScenarioCustomDefaulter{
			DefaultResolverRefResourceKind: "pipeline",
		}).
		Complete()
}

type IntegrationTestScenarioCustomDefaulter struct {
	DefaultResolverRefResourceKind string
}

//+kubebuilder:webhook:path=/validate-appstudio-redhat-com-v1beta2-integrationtestscenario,mutating=false,failurePolicy=fail,sideEffects=None,groups=appstudio.redhat.com,resources=integrationtestscenarios,verbs=create;update;delete,versions=v1beta2,name=vintegrationtestscenario.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &IntegrationTestScenario{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *IntegrationTestScenario) ValidateCreate() (warnings admission.Warnings, err error) {
	integrationtestscenariolog.Info("Validating IntegrationTestScenario upon creation", "name", r.GetName())
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

	if r.Spec.ResolverRef.Resolver == "git" {
		var paramErrors error
		for _, param := range r.Spec.ResolverRef.Params {
			switch key := param.Name; key {
			case "url", "serverURL":
				paramErrors = errors.Join(paramErrors, validateUrl(key, param.Value))
			case "token":
				paramErrors = errors.Join(paramErrors, validateToken(param.Value))
			default:
				paramErrors = errors.Join(paramErrors, validateNoWhitespace(key, param.Value))
			}
		}
		if paramErrors != nil {
			return nil, paramErrors
		}
	}

	return nil, nil
}

// Returns an error if 'value' contains leading or trailing whitespace
func validateNoWhitespace(key, value string) error {
	r, _ := regexp.Compile(`(^\s+)|(\s+$)`)
	if r.MatchString(value) {
		return fmt.Errorf("integration Test Scenario Git resolver param with name '%s', cannot have leading or trailing whitespace", key)
	}
	return nil
}

// Returns an error if the string is not a syntactically valid URL. URL must
// begin with 'https://'
func validateUrl(key, url string) error {
	err := validateNoWhitespace(key, url)
	if err != nil {
		// trim whitespace so we can validate the rest of the url
		url = strings.TrimSpace(url)
	}
	_, uriErr := neturl.ParseRequestURI(url)
	if uriErr != nil {
		return errors.Join(err, uriErr)
	}

	if !strings.HasPrefix(url, "https://") {
		return errors.Join(err, fmt.Errorf("'%s' param value must begin with 'https://'", key))
	}

	return err
}

// Returns an error if the string is not a valid name based on RFC1123. See
// https://kubernetes.io/docs/concepts/overview/working-with-objects/names
func validateToken(token string) error {
	validationErrors := validation.IsDNS1123Label(token)
	if len(validationErrors) > 0 {
		var err error
		for _, e := range validationErrors {
			err = errors.Join(err, errors.New(e))
		}
		return err
	}

	return nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *IntegrationTestScenario) ValidateUpdate(old runtime.Object) (warnings admission.Warnings, err error) {
	return nil, nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *IntegrationTestScenario) ValidateDelete() (warnings admission.Warnings, err error) {
	return nil, nil
}

// +kubebuilder:webhook:path=/mutate-appstudio-redhat-com-v1beta2-integrationtestscenario,mutating=true,failurePolicy=fail,sideEffects=None,groups=appstudio.redhat.com,resources=integrationtestscenarios,verbs=create;update;delete,versions=v1beta2,name=dintegrationtestscenario.kb.io,admissionReviewVersions=v1

var _ webhook.CustomDefaulter = &IntegrationTestScenarioCustomDefaulter{}

func (d *IntegrationTestScenarioCustomDefaulter) Default(ctx context.Context, obj runtime.Object) error {
	integrationtestscenariolog.Info("In Default() function", "object", obj)
	scenario, ok := obj.(*IntegrationTestScenario)
	if !ok {
		return fmt.Errorf("expected an IntegrationTestScenario but got %T", obj)
	}

	d.applyDefaults(scenario)
	return nil
}

func (d *IntegrationTestScenarioCustomDefaulter) applyDefaults(scenario *IntegrationTestScenario) {
	integrationtestscenariolog.Info("Applying default resolver type", "name", scenario.GetName())
	if scenario.Spec.ResolverRef.ResourceKind == "" {
		scenario.Spec.ResolverRef.ResourceKind = d.DefaultResolverRefResourceKind
	}
}
