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

package snapshot

import (
	"context"
	"github.com/go-logr/logr"
	hasv1alpha1 "github.com/redhat-appstudio/application-service/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/controllers/results"
	appstudioshared "github.com/redhat-appstudio/managed-gitops/appstudio-shared/apis/appstudio.redhat.com/v1alpha1"
	releasev1alpha1 "github.com/redhat-appstudio/release-service/api/v1alpha1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"math"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Adapter holds the objects needed to reconcile a Release.
type Adapter struct {
	snapshot    *appstudioshared.ApplicationSnapshot
	application *hasv1alpha1.Application
	component   *hasv1alpha1.Component
	logger      logr.Logger
	client      client.Client
	context     context.Context
}

// NewAdapter creates and returns an Adapter instance.
func NewAdapter(snapshot *appstudioshared.ApplicationSnapshot, application *hasv1alpha1.Application, component *hasv1alpha1.Component, logger logr.Logger, client client.Client,
	context context.Context) *Adapter {
	return &Adapter{
		snapshot:    snapshot,
		application: application,
		component:   component,
		logger:      logger,
		client:      client,
		context:     context,
	}
}

// EnsureAllIntegrationTestPipelinesExist is an operation that will ensure that all Integration test pipelines
// associated with the ApplicationSnapshot and the Application's IntegrationTestScenarios exist.
// Otherwise, it will create new Releases for each ReleasePlan.
func (a *Adapter) EnsureAllIntegrationTestPipelinesExist() (results.OperationResult, error) {
	if meta.FindStatusCondition(a.snapshot.Status.Conditions, "HACBSTestSucceeded") != nil {
		a.logger.Info("The Snapshot has finished testing.")
		return results.ContinueProcessing()
	}
	a.logger.Info("Placeholder for triggering all Integration pipelines (including optional ones)")

	requiredIntegrationTestScenarios, err := a.getRequiredIntegrationTestScenariosForApplication(a.application)
	if err != nil {
		a.logger.Error(err, "Failed to get all IntegrationTestScenarios",
			"Application.Name", a.application.Name,
			"Application.Namespace", a.application.Namespace)
		return results.RequeueOnErrorOrStop(a.updateStatus())
	}
	if len(*requiredIntegrationTestScenarios) == 0 {
		updatedSnapshot, err := a.markSnapshotAsPassed(a.snapshot, "No required IntegrationTestScenarios found, skipped testing")
		if err != nil {
			a.logger.Error(err, "Failed to update Snapshot status",
				"ApplicationSnapshot.Name", a.snapshot.Name,
				"ApplicationSnapshot.Namespace", a.snapshot.Namespace)
			return results.RequeueOnErrorOrStop(a.updateStatus())
		}
		a.logger.Info("No required IntegrationTestScenarios found, skipped testing and marked Snapshot as successful",
			"ApplicationSnapshot.Name", updatedSnapshot.Name,
			"ApplicationSnapshot.Namespace", updatedSnapshot.Namespace,
			"ApplicationSnapshot.Status", updatedSnapshot.Status)
	}

	return results.ContinueProcessing()
}

// EnsureAllReleasesExist is an operation that will ensure that all pipeline Releases associated
// to the ApplicationSnapshot and the Application's ReleasePlans exist.
// Otherwise, it will create new Releases for each ReleasePlan.
func (a *Adapter) EnsureAllReleasesExist() (results.OperationResult, error) {
	if !a.isSnapshotConditionMet(a.snapshot, "HACBSTestSucceeded") {
		a.logger.Info("The Snapshot hasn't been marked as HACBSTestSucceeded, holding off on releasing.")
		return results.ContinueProcessing()
	}

	releasePlans, err := a.getAutoReleasePlansForApplication(a.application)
	if err != nil {
		a.logger.Error(err, "Failed to get all ReleasePlans",
			"Application.Name", a.application.Name,
			"Application.Namespace", a.application.Namespace)
		return results.RequeueOnErrorOrStop(a.updateStatus())
	}

	err = a.createMissingReleasesForReleasePlans(releasePlans, a.snapshot)
	if err != nil {
		a.logger.Error(err, "Failed to create new Releases for",
			"ApplicationSnapshot.Name", a.snapshot.Name,
			"ApplicationSnapshot.Namespace", a.snapshot.Namespace)
		return results.RequeueOnErrorOrStop(a.updateStatus())
	}

	return results.ContinueProcessing()
}

// EnsureApplicationSnapshotEnvironmentBindingExist is an operation that will ensure that all
// ApplicationSnapshotEnvironmentBindings for non-ephemeral root environments point to the newly constructed snapshot.
// If the bindings don't already exist, it will create new ones for each of the environments.
func (a *Adapter) EnsureApplicationSnapshotEnvironmentBindingExist() (results.OperationResult, error) {
	if !a.isSnapshotConditionMet(a.snapshot, "HACBSTestSucceeded") {
		a.logger.Info("The Snapshot hasn't been marked as HACBSTestSucceeded, holding off on deploying.")
		return results.ContinueProcessing()
	}

	availableEnvironments, err := a.findAvailableEnvironments()
	if err != nil {
		return results.RequeueWithError(err)
	}

	components, err := a.getAllApplicationComponents(a.application)
	if err != nil {
		return results.RequeueWithError(err)
	}

	for _, availableEnvironment := range *availableEnvironments {
		bindingName := a.application.Name + "-" + availableEnvironment.Name + "-" + "binding"
		applicationSnapshotEnvironmentBinding, _ := a.findExistingApplicationSnapshotEnvironmentBinding(availableEnvironment.Name)
		if applicationSnapshotEnvironmentBinding != nil {
			applicationSnapshotEnvironmentBinding.Spec.Snapshot = a.snapshot.Name
			applicationSnapshotComponents := a.getApplicationSnapshotComponents(*components)
			applicationSnapshotEnvironmentBinding.Spec.Components = *applicationSnapshotComponents
			err := a.client.Update(a.context, applicationSnapshotEnvironmentBinding)
			if err != nil {
				a.logger.Error(err, "Failed to update ApplicationSnapshotEnvironmentBinding",
					"ApplicationSnapshotEnvironmentBinding.Application", applicationSnapshotEnvironmentBinding.Spec.Application,
					"ApplicationSnapshotEnvironmentBinding.Environment", applicationSnapshotEnvironmentBinding.Spec.Environment,
					"ApplicationSnapshotEnvironmentBinding.Snapshot", applicationSnapshotEnvironmentBinding.Spec.Snapshot)
				return results.RequeueOnErrorOrStop(a.updateStatus())
			}
		} else {
			applicationSnapshotEnvironmentBinding, err = a.CreateApplicationSnapshotEnvironmentBinding(
				bindingName, a.application.Namespace, a.application.Name,
				availableEnvironment.Name,
				a.snapshot, *components)

			if err != nil {
				a.logger.Error(err, "Failed to create ApplicationSnapshotEnvironmentBinding",
					"ApplicationSnapshotEnvironmentBinding.Application", applicationSnapshotEnvironmentBinding.Spec.Application,
					"ApplicationSnapshotEnvironmentBinding.Environment", applicationSnapshotEnvironmentBinding.Spec.Environment,
					"ApplicationSnapshotEnvironmentBinding.Snapshot", applicationSnapshotEnvironmentBinding.Spec.Snapshot)
				return results.RequeueOnErrorOrStop(a.updateStatus())
			}

		}
		a.logger.Info("Created/updated ApplicationSnapshotEnvironmentBinding",
			"ApplicationSnapshotEnvironmentBinding.Application", applicationSnapshotEnvironmentBinding.Spec.Application,
			"ApplicationSnapshotEnvironmentBinding.Environment", applicationSnapshotEnvironmentBinding.Spec.Environment,
			"ApplicationSnapshotEnvironmentBinding.Snapshot", applicationSnapshotEnvironmentBinding.Spec.Snapshot)
	}
	return results.ContinueProcessing()
}

// getApplicationSnapshotComponents gets all components from the ApplicationSnapshot and formats them to be used in the
// ApplicationSnapshotEnvironmentBinding as BindingComponents.
func (a *Adapter) getApplicationSnapshotComponents(components []hasv1alpha1.Component) *[]appstudioshared.BindingComponent {
	bindingComponents := []appstudioshared.BindingComponent{}
	for _, component := range components {
		bindingComponents = append(bindingComponents, appstudioshared.BindingComponent{
			Name: component.Spec.ComponentName,
			Configuration: appstudioshared.BindingComponentConfiguration{
				Replicas: int(math.Max(1, float64(component.Spec.Replicas))),
			},
		})
	}
	return &bindingComponents
}

// CreateApplicationSnapshotEnvironmentBinding creates a new ApplicationSnapshotEnvironmentBinding using the provided info
func (a *Adapter) CreateApplicationSnapshotEnvironmentBinding(bindingName string, namespace string, applicationName string, environmentName string, appSnapshot *appstudioshared.ApplicationSnapshot, components []hasv1alpha1.Component) (*appstudioshared.ApplicationSnapshotEnvironmentBinding, error) {
	bindingComponents := a.getApplicationSnapshotComponents(components)

	applicationSnapshotEnvironmentBinding := &appstudioshared.ApplicationSnapshotEnvironmentBinding{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: bindingName + "-",
			Namespace:    namespace,
		},
		Spec: appstudioshared.ApplicationSnapshotEnvironmentBindingSpec{
			Application: applicationName,
			Environment: environmentName,
			Snapshot:    appSnapshot.Name,
			Components:  *bindingComponents,
		},
	}

	err := a.client.Create(a.context, applicationSnapshotEnvironmentBinding)
	return applicationSnapshotEnvironmentBinding, err
}

// getAutoReleasePlansForApplication returns the ReleasePlans used by the application being processed. If matching
// ReleasePlans are not found, an error will be returned. A ReleasePlan will only be returned if it has the
// release.appstudio.openshift.io/auto-release label set to true or if it is missing the label entirely.
func (a *Adapter) getAutoReleasePlansForApplication(application *hasv1alpha1.Application) (*[]releasev1alpha1.ReleasePlan, error) {
	releasePlans := &releasev1alpha1.ReleasePlanList{}
	labelSelector := labels.NewSelector()
	labelRequirement, err := labels.NewRequirement("release.appstudio.openshift.io/auto-release", selection.NotIn, []string{"false"})
	if err != nil {
		return nil, err
	}
	labelSelector = labelSelector.Add(*labelRequirement)

	opts := &client.ListOptions{
		Namespace:     application.Namespace,
		FieldSelector: fields.OneTermEqualSelector("spec.application", application.Name),
		LabelSelector: labelSelector,
	}

	err = a.client.List(a.context, releasePlans, opts)
	if err != nil {
		return nil, err
	}

	return &releasePlans.Items, nil
}

// getReleasesWithApplicationSnapshot returns all Releases associated with the given applicationSnapshot.
// In the case the List operation fails, an error will be returned.
func (a *Adapter) getReleasesWithApplicationSnapshot(applicationSnapshot *appstudioshared.ApplicationSnapshot) (*[]releasev1alpha1.Release, error) {
	releases := &releasev1alpha1.ReleaseList{}
	opts := []client.ListOption{
		client.InNamespace(applicationSnapshot.Namespace),
		client.MatchingFields{"spec.applicationSnapshot": applicationSnapshot.Name},
	}

	err := a.client.List(a.context, releases, opts...)
	if err != nil {
		return nil, err
	}

	return &releases.Items, nil
}

// createReleaseForReleasePlan creates the Release for a given ReleasePlan.
// In case the Release can't be created, an error will be returned.
func (a *Adapter) createReleaseForReleasePlan(releasePlan *releasev1alpha1.ReleasePlan, applicationSnapshot *appstudioshared.ApplicationSnapshot) (*releasev1alpha1.Release, error) {
	newRelease := &releasev1alpha1.Release{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: applicationSnapshot.Name + "-",
			Namespace:    applicationSnapshot.Namespace,
		},
		Spec: releasev1alpha1.ReleaseSpec{
			ApplicationSnapshot: applicationSnapshot.Name,
			ReleasePlan:         releasePlan.Name,
		},
	}
	err := a.client.Create(a.context, newRelease)
	if err != nil {
		return nil, err
	}

	return newRelease, nil
}

// createMissingReleasesForReleasePlans checks if there's existing Releases for a given list of ReleasePlans and creates
// new ones if they are missing. In case the Releases can't be created, an error will be returned.
func (a *Adapter) createMissingReleasesForReleasePlans(releasePlans *[]releasev1alpha1.ReleasePlan, applicationSnapshot *appstudioshared.ApplicationSnapshot) error {
	releases, err := a.getReleasesWithApplicationSnapshot(applicationSnapshot)
	if err != nil {
		return err
	}

	for _, releasePlan := range *releasePlans {
		releasePlan := releasePlan
		var existingRelease *releasev1alpha1.Release = nil
		for _, snapshotRelease := range *releases {
			snapshotRelease := snapshotRelease
			if snapshotRelease.Spec.ReleasePlan == releasePlan.Name {
				existingRelease = &snapshotRelease
			}
		}
		if existingRelease != nil {
			a.logger.Info("Found existing Release",
				"ApplicationSnapshot.Name", applicationSnapshot.Name,
				"ReleasePlan.Name", releasePlan.Name,
				"Release.Name", existingRelease.Name)
		} else {
			newRelease, err := a.createReleaseForReleasePlan(&releasePlan, applicationSnapshot)
			if err != nil {
				return err
			}
			a.logger.Info("Created new Release",
				"Application.Name", a.application.Name,
				"ReleasePlan.Name", releasePlan.Name,
				"Release.Name", newRelease.Name)
		}
	}
	return nil
}

// getRequiredIntegrationTestScenariosForApplication returns the IntegrationTestScenarios used by the application being processed.
// An IntegrationTestScenarios will only be returned if it has the release.appstudio.openshift.io/optional
// label set to true or if it is missing the label entirely.
func (a *Adapter) getRequiredIntegrationTestScenariosForApplication(application *hasv1alpha1.Application) (*[]v1alpha1.IntegrationTestScenario, error) {
	labelSelector := labels.NewSelector()
	integrationList := &v1alpha1.IntegrationTestScenarioList{}
	labelRequirement, err := labels.NewRequirement("test.appstudio.openshift.io/optional", selection.NotIn, []string{"true"})
	if err != nil {
		return nil, err
	}
	labelSelector = labelSelector.Add(*labelRequirement)

	opts := &client.ListOptions{
		Namespace:     application.Namespace,
		FieldSelector: fields.OneTermEqualSelector("spec.application", application.Name),
		LabelSelector: labelSelector,
	}

	err = a.client.List(a.context, integrationList, opts)
	if err != nil {
		return nil, err
	}

	return &integrationList.Items, nil
}

func (a *Adapter) getAllEnvironments() (*[]appstudioshared.Environment, error) {

	environmentList := &appstudioshared.EnvironmentList{}
	opts := []client.ListOption{
		client.InNamespace(a.application.Namespace),
	}
	err := a.client.List(a.context, environmentList, opts...)
	return &environmentList.Items, err
}

// findAvailableEnvironments gets all environments that don't have a ParentEnvironment and are not tagged as ephemeral
func (a *Adapter) findAvailableEnvironments() (*[]appstudioshared.Environment, error) {
	allEnvironments, err := a.getAllEnvironments()
	if err != nil {
		return nil, err
	}
	availableEnvironments := []appstudioshared.Environment{}
	for _, environment := range *allEnvironments {
		if environment.Spec.ParentEnvironment == "" {
			isEphemeral := false
			for _, tag := range environment.Spec.Tags {
				if tag == "ephemeral" {
					isEphemeral = true
					break
				}
			}
			if !isEphemeral {
				availableEnvironments = append(availableEnvironments, environment)
			}
		}
	}
	return &availableEnvironments, nil
}

// findExistingApplicationSnapshotEnvironmentBinding attempts to find an ApplicationSnapshotEnvironmentBinding that's
// associated with the provided environmentName
func (a *Adapter) findExistingApplicationSnapshotEnvironmentBinding(environmentName string) (*appstudioshared.ApplicationSnapshotEnvironmentBinding, error) {
	applicationSnapshotEnvironmentBindingList := &appstudioshared.ApplicationSnapshotEnvironmentBindingList{}
	opts := []client.ListOption{
		client.InNamespace(a.application.Namespace),
		client.MatchingFields{"spec.environment": environmentName},
	}

	err := a.client.List(a.context, applicationSnapshotEnvironmentBindingList, opts...)
	if err != nil {
		return nil, err
	}

	for _, binding := range applicationSnapshotEnvironmentBindingList.Items {
		if binding.Spec.Application == a.application.Name {
			return &binding, nil
		}
	}

	return nil, nil
}

// getAllApplicationComponents loads from the cluster all Components associated with the given Application.
// If the Application doesn't have any Components or this is not found in the cluster, an error will be returned.
func (a *Adapter) getAllApplicationComponents(application *hasv1alpha1.Application) (*[]hasv1alpha1.Component, error) {
	applicationComponents := &hasv1alpha1.ComponentList{}
	opts := []client.ListOption{
		client.InNamespace(application.Namespace),
		client.MatchingFields{"spec.application": application.Name},
	}

	err := a.client.List(a.context, applicationComponents, opts...)
	if err != nil {
		return nil, err
	}

	return &applicationComponents.Items, nil
}

// markSnapshotAsPassed updates the result label for the ApplicationSnapshot
// If the update command fails, an error will be returned
func (a *Adapter) markSnapshotAsPassed(applicationSnapshot *appstudioshared.ApplicationSnapshot, message string) (*appstudioshared.ApplicationSnapshot, error) {
	patch := client.MergeFrom(applicationSnapshot.DeepCopy())
	meta.SetStatusCondition(&applicationSnapshot.Status.Conditions, metav1.Condition{
		Type:    "HACBSTestSucceeded",
		Status:  metav1.ConditionTrue,
		Reason:  "Passed",
		Message: message,
	})
	err := a.client.Status().Patch(a.context, applicationSnapshot, patch)
	if err != nil {
		return nil, err
	}
	return applicationSnapshot, nil
}

// isSnapshotConditionMet returns a boolean indicating whether the Snapshot's status has the conditionType set to true.
func (a *Adapter) isSnapshotConditionMet(snapshot *appstudioshared.ApplicationSnapshot, conditionType string) bool {
	condition := meta.FindStatusCondition(snapshot.Status.Conditions, conditionType)
	return condition != nil && condition.Status != metav1.ConditionUnknown
}

// updateStatus updates the status of the PipelineRun being processed.
func (a *Adapter) updateStatus() error {
	return a.client.Status().Update(a.context, a.snapshot)
}
