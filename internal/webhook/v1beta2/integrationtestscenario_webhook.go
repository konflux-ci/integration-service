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
	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	"github.com/konflux-ci/integration-service/api/v1beta2"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

func SetupIntegrationTestScenarioWebhookWithManager(mgr ctrl.Manager) error {
	defaulter := &IntegrationTestScenarioCustomDefaulter{
		DefaultResolverRefResourceKind: "pipeline",
		client:                         mgr.GetClient(),
	}
	return ctrl.NewWebhookManagedBy(mgr).
		For(&v1beta2.IntegrationTestScenario{}).
		WithDefaulter(defaulter).
		WithValidator(&IntegrationTestScenarioCustomValidator{}).
		Complete()
}

// IntegrationTestScenarioCustomValidator is a webhook handler and does not need deepcopy methods.
// +k8s:deepcopy-gen=false
type IntegrationTestScenarioCustomValidator struct {
	// TODO(user): Add more fields as needed for validation
}

// IntegrationTestScenarioCustomDefaulter is a webhook handler and does not need deepcopy methods.
// +k8s:deepcopy-gen=false
type IntegrationTestScenarioCustomDefaulter struct {
	DefaultResolverRefResourceKind string
	client                         client.Client
}

// +kubebuilder:webhook:path=/validate-appstudio-redhat-com-v1beta2-integrationtestscenario,mutating=false,failurePolicy=fail,sideEffects=None,groups=appstudio.redhat.com,resources=integrationtestscenarios,verbs=create;update;delete,versions=v1beta2,name=vintegrationtestscenario.kb.io,admissionReviewVersions=v1

var _ webhook.CustomValidator = &IntegrationTestScenarioCustomValidator{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (v *IntegrationTestScenarioCustomValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (warnings admission.Warnings, err error) {
	scenario, ok := obj.(*v1beta2.IntegrationTestScenario)
	if !ok {
		return nil, fmt.Errorf("expected a IntegrationTestScenario object but got %T", obj)
	}

	integrationtestscenariolog.Info("Validating IntegrationTestScenario upon creation", "name", scenario.GetName())

	// Validate that exactly one of application or componentGroup is specified
	if err := validateOwnerField(scenario); err != nil {
		return nil, err
	}

	// We use the DNS-1035 format for application names, so ensure it conforms to that specification
	if len(validation.IsDNS1035Label(scenario.Name)) != 0 {
		return nil, field.Invalid(field.NewPath("metadata").Child("name"), scenario.Name,
			"an IntegrationTestScenario resource name must start with a lower case "+
				"alphabetical character, be under 63 characters, and can only consist "+
				"of lower case alphanumeric characters or '-'")
	}

	// see stoneintg-896
	for _, param := range scenario.Spec.Params {
		if param.Name == "SNAPSHOT" {
			errString := "an IntegrationTestScenario resource should not have the SNAPSHOT " +
				"param manually defined because it will be atomatically generated " +
				"by the integration service"
			integrationtestscenariolog.Info(errString)
			return nil, field.Invalid(field.NewPath("Spec").Child("Params"), param.Name, errString)
		}
		// we won't enable ITS if git resolver with url & repo+org
		urlResolverExist := false
		repoResolverExist := false
		orgResolverExist := false

		for _, gitResolverParam := range scenario.Spec.ResolverRef.Params {
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

	integrationtestscenariolog.Info("Validated params")

	if scenario.Spec.ResolverRef.Resolver == "git" {
		var paramErrors error
		for _, param := range scenario.Spec.ResolverRef.Params {
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

	// Ensure the ownerReference was set by the mutating webhook
	if ref := scenario.GetOwnerReferences(); len(ref) == 0 {
		integrationtestscenariolog.Info("Owner reference not set for scenario", scenario.Name)
		return nil, fmt.Errorf("owner reference not set for scenario '%s' in namespace '%s'", scenario.Name, scenario.Namespace)
	}

	return nil, nil
}

// validateOwnerField ensures exactly one of application or componentGroup is specified
func validateOwnerField(scenario *v1beta2.IntegrationTestScenario) error {
	hasApplication := scenario.Spec.Application != ""
	hasComponentGroup := scenario.Spec.ComponentGroup != ""

	if hasApplication && hasComponentGroup {
		return field.Invalid(field.NewPath("spec"),
			fmt.Sprintf("application=%s, componentGroup=%s", scenario.Spec.Application, scenario.Spec.ComponentGroup),
			"exactly one of 'application' or 'componentGroup' must be specified, not both")
	}

	if !hasApplication && !hasComponentGroup {
		return field.Required(field.NewPath("spec"),
			"exactly one of 'application' or 'componentGroup' must be specified")
	}

	return nil
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
func (v *IntegrationTestScenarioCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (warnings admission.Warnings, err error) {
	return nil, nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (v *IntegrationTestScenarioCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

//+kubebuilder:webhook:path=/mutate-appstudio-redhat-com-v1beta2-integrationtestscenario,mutating=true,failurePolicy=ignore,sideEffects=None,groups=appstudio.redhat.com,resources=integrationtestscenarios,verbs=create;update;delete,versions=v1beta2,name=dintegrationtestscenario.kb.io,admissionReviewVersions=v1

var _ webhook.CustomDefaulter = &IntegrationTestScenarioCustomDefaulter{}

func (d *IntegrationTestScenarioCustomDefaulter) Default(ctx context.Context, obj runtime.Object) error {
	integrationtestscenariolog.Info("In Default() function", "object", obj)
	scenario, ok := obj.(*v1beta2.IntegrationTestScenario)
	if !ok {
		return fmt.Errorf("expected an IntegrationTestScenario but got %T", obj)
	}

	err := addOwnerReference(scenario, d.client)
	if err != nil {
		return err
	}

	d.applyDefaults(scenario)
	return nil
}

func addOwnerReference(scenario *v1beta2.IntegrationTestScenario, c client.Client) error {
	if len(scenario.OwnerReferences) == 0 && scenario.DeletionTimestamp.IsZero() {
		var ownerReference metav1.OwnerReference

		if scenario.HasApplication() {
			// Set owner reference to Application
			application := applicationapiv1alpha1.Application{}
			err := c.Get(context.Background(), types.NamespacedName{Name: scenario.Spec.Application, Namespace: scenario.Namespace}, &application)
			if err != nil {
				if k8serrors.IsNotFound(err) {
					return fmt.Errorf("could not find application '%s' in namespace '%s'", scenario.Spec.Application, scenario.Namespace)
				}
				return err
			}
			ownerReference = metav1.OwnerReference{
				APIVersion:         application.APIVersion,
				Kind:               application.Kind,
				Name:               application.Name,
				UID:                application.UID,
				BlockOwnerDeletion: ptr.To(false),
				Controller:         ptr.To(false),
			}
		} else if scenario.HasComponentGroup() {
			// Set owner reference to ComponentGroup
			componentGroup := v1beta2.ComponentGroup{}
			err := c.Get(context.Background(), types.NamespacedName{Name: scenario.Spec.ComponentGroup, Namespace: scenario.Namespace}, &componentGroup)
			if err != nil {
				if k8serrors.IsNotFound(err) {
					return fmt.Errorf("could not find componentGroup '%s' in namespace '%s'", scenario.Spec.ComponentGroup, scenario.Namespace)
				}
				return err
			}
			ownerReference = metav1.OwnerReference{
				APIVersion:         componentGroup.APIVersion,
				Kind:               componentGroup.Kind,
				Name:               componentGroup.Name,
				UID:                componentGroup.UID,
				BlockOwnerDeletion: ptr.To(false),
				Controller:         ptr.To(false),
			}
		} else {
			return fmt.Errorf("scenario '%s' must specify either application or componentGroup", scenario.Name)
		}

		scenario.SetOwnerReferences(append(scenario.GetOwnerReferences(), ownerReference))
	}
	return nil
}

func (d *IntegrationTestScenarioCustomDefaulter) applyDefaults(scenario *v1beta2.IntegrationTestScenario) {
	integrationtestscenariolog.Info("Applying default resolver type", "name", scenario.GetName())
	if scenario.Spec.ResolverRef.ResourceKind == "" {
		scenario.Spec.ResolverRef.ResourceKind = d.DefaultResolverRefResourceKind
	}
}
