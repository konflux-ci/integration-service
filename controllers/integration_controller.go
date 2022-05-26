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

package controllers

import (
	"context"
	"fmt"
	hasv1alpha1 "github.com/redhat-appstudio/application-service/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/release"
	"github.com/redhat-appstudio/integration-service/tekton"
	releasev1alpha1 "github.com/redhat-appstudio/release-service/api/v1alpha1"
	"github.com/sirupsen/logrus"
	tektonv1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// IntegrationReconciler reconciles a Integration object
type IntegrationReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=appstudio.redhat.com,resources=integrations,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=appstudio.redhat.com,resources=integrations/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=appstudio.redhat.com,resources=integrations/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.0/pkg/reconcile
func (r *IntegrationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logrus.New()

	pipelineRun := &tektonv1beta1.PipelineRun{}
	err := r.Get(ctx, req.NamespacedName, pipelineRun)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("PipelineRun resource not found")

			return ctrl.Result{}, nil
		}

		log.Error(err, "Failed to get PipelineRun")

		return ctrl.Result{}, err
	}

	if tekton.IsBuildPipelineRun(pipelineRun) {
		component, err := r.getComponentFromPipelineRun(ctx, pipelineRun)
		if err != nil {
			log.Error(err, "Failed to get Component for ",
				"PipelineRun.Name ", pipelineRun.Name, "PipelineRun.Namespace ", pipelineRun.Namespace)
			return ctrl.Result{}, nil
		}

		application, err := r.getApplicationFromComponent(ctx, component)
		if err != nil {
			log.Error(err, "Failed to get Application for ",
				"Component.Name ", component.Name, "Component.Namespace ", component.Namespace)
			return ctrl.Result{}, nil
		}

		log.Info("PipelineRun resource for the build pipeline found! Component ", component.Name,
			" Application ", application.Name)

		applicationSnapshot, err := r.createApplicationSnapshotForPipelineRun(ctx, component, application, pipelineRun)
		if err != nil {
			log.Error(err, "Failed to create ApplicationSnapshot for ",
				"Application.Name ", application.Name, "Application.Namespace ", application.Namespace)
			return ctrl.Result{}, nil
		}
		log.Info("Created ApplicationSnapshot for Application ", application.Name,
			" with Images: ", applicationSnapshot.Spec.Images)

		releases, err := r.createReleasesForApplicationSnapshot(ctx, application, applicationSnapshot)
		if err != nil {
			log.Error(err, "Failed to create Releases for ",
				"Application.Name ", application.Name, "ApplicationSnapshot.Name ", applicationSnapshot.Name)
			return ctrl.Result{}, nil
		}
		for _, appRelease := range *releases {
			log.Info("Created Release.Name ", appRelease.Name, " for ApplicationSnapshot.Name ", applicationSnapshot.Name)
		}
	}

	return ctrl.Result{}, nil
}

// getComponentFromPipelineRun loads from the cluster the Component referenced in the given PipelineRun. If the PipelineRun doesn't
// specify a Component or this is not found in the cluster, an error will be returned.
func (r *IntegrationReconciler) getComponentFromPipelineRun(ctx context.Context, pipelineRun *tektonv1beta1.PipelineRun) (*hasv1alpha1.Component, error) {
	if componentName, found := pipelineRun.Labels["build.appstudio.openshift.io/component"]; found {
		component := &hasv1alpha1.Component{}
		err := r.Get(ctx, types.NamespacedName{
			Namespace: pipelineRun.Namespace,
			Name:      componentName,
		}, component)

		if err != nil {
			return nil, err
		}

		return component, nil
	}

	return nil, fmt.Errorf("the pipeline has no component associated with it")
}

// getApplicationFromComponent loads from the cluster the Application referenced in the given Component. If the Component doesn't
// specify an Application or this is not found in the cluster, an error will be returned.
func (r *IntegrationReconciler) getApplicationFromComponent(ctx context.Context, component *hasv1alpha1.Component) (*hasv1alpha1.Application, error) {
	application := &hasv1alpha1.Application{}
	err := r.Get(ctx, types.NamespacedName{
		Namespace: component.Namespace,
		Name:      component.Spec.Application,
	}, application)

	if err != nil {
		return nil, err
	}

	return application, nil
}

// getAllApplicationComponents loads from the cluster all Components associated with the given Application.
// If the Application doesn't have any Components or this is not found in the cluster, an error will be returned.
func (r *IntegrationReconciler) getAllApplicationComponents(ctx context.Context, application *hasv1alpha1.Application) (*[]hasv1alpha1.Component, error) {
	applicationComponents := &hasv1alpha1.ComponentList{}
	opts := []client.ListOption{
		client.InNamespace(application.Namespace),
		client.MatchingFields{"spec.application": application.Name},
	}

	err := r.List(ctx, applicationComponents, opts...)
	if err != nil {
		return nil, err
	}

	return &applicationComponents.Items, nil
}

// getAllApplicationReleaseLinks returns the ReleaseLinks used by the application being processed. If matching
// ReleaseLinks are not found, an error will be returned.
func (r *IntegrationReconciler) getAllApplicationReleaseLinks(ctx context.Context, application *hasv1alpha1.Application) (*[]releasev1alpha1.ReleaseLink, error) {
	releaseLinks := &releasev1alpha1.ReleaseLinkList{}
	opts := []client.ListOption{
		client.InNamespace(application.Namespace),
		client.MatchingFields{"spec.application": application.Name},
	}

	err := r.List(ctx, releaseLinks, opts...)
	if err != nil {
		return nil, err
	}

	return &releaseLinks.Items, nil
}

// createApplicationSnapshotForPipelineRun returns the ReleaseLinks used by the application being processed. If matching
// ReleaseLinks are not found, an error will be returned.
func (r *IntegrationReconciler) createApplicationSnapshotForPipelineRun(ctx context.Context, component *hasv1alpha1.Component,
	application *hasv1alpha1.Application, pipelineRun *tektonv1beta1.PipelineRun) (*releasev1alpha1.ApplicationSnapshot, error) {

	applicationComponents, err := r.getAllApplicationComponents(ctx, application)
	if err != nil {
		return nil, fmt.Errorf("failed to get all Application Components for Application %s", application.Name)
	}
	var images []releasev1alpha1.Image

	for _, applicationComponent := range *applicationComponents {
		pullSpec := applicationComponent.Status.ContainerImage
		if applicationComponent.Name == component.Name {
			var err error
			pullSpec, err = tekton.GetOutputImage(pipelineRun)
			if err != nil {
				return nil, err
			}
		}
		image := releasev1alpha1.Image{
			Component: applicationComponent.Name,
			PullSpec:  pullSpec,
		}
		images = append(images, image)
	}

	applicationSnapshot, err := release.CreateApplicationSnapshot(application, &images)
	if err != nil {
		return nil, fmt.Errorf("failed to create Application Snapshot for Application %s", application.Name)
	}

	err = r.Client.Create(ctx, applicationSnapshot)
	if err != nil {
		return nil, fmt.Errorf("failed to create an ApplicationSnapshot for Application %s and pipelineRun %s: %v",
			application.Name, pipelineRun.Name, err)
	}

	return applicationSnapshot, nil
}

// createReleasesForApplicationSnapshot creates Releases for a given Application and ApplicationSnapshot
// If ReleaseLinks are not found, an error will be returned.
func (r *IntegrationReconciler) createReleasesForApplicationSnapshot(ctx context.Context, application *hasv1alpha1.Application,
	applicationSnapshot *releasev1alpha1.ApplicationSnapshot) (*[]releasev1alpha1.Release, error) {
	var releases []releasev1alpha1.Release
	releaseLinks, err := r.getAllApplicationReleaseLinks(ctx, application)
	if err != nil {
		return nil, fmt.Errorf("failed to get all ReleaseLinks for Application %s", application.Name)
	}
	for _, releaseLink := range *releaseLinks {
		appRelease, err := release.CreateRelease(applicationSnapshot, &releaseLink)
		if err != nil {
			return nil, fmt.Errorf("failed to create a Release for Application %s and ReleaseLink %s: %v",
				application.Name, releaseLink.Name, err)
		}
		err = r.Client.Create(ctx, appRelease)
		if err != nil {
			return nil, fmt.Errorf("failed to create a Release for Application %s and ReleaseLink %s: %v",
				application.Name, releaseLink.Name, err)
		}
		releases = append(releases, *appRelease)
	}

	return &releases, nil
}

// setupReleaseLinkCache adds a new index field to be able to search ReleaseLinks by application.
func setupReleaseLinkCache(mgr ctrl.Manager) error {
	releaseLinkTargetIndexFunc := func(obj client.Object) []string {
		return []string{obj.(*releasev1alpha1.ReleaseLink).Spec.Application}
	}

	return mgr.GetCache().IndexField(context.Background(), &releasev1alpha1.ReleaseLink{},
		"spec.application", releaseLinkTargetIndexFunc)
}

// setupApplicationComponentCache adds a new index field to be able to search Components by application.
func setupApplicationComponentCache(mgr ctrl.Manager) error {
	releaseLinkTargetIndexFunc := func(obj client.Object) []string {
		return []string{obj.(*hasv1alpha1.Component).Spec.Application}
	}

	return mgr.GetCache().IndexField(context.Background(), &hasv1alpha1.Component{},
		"spec.application", releaseLinkTargetIndexFunc)
}

// SetupWithManager sets up the controller with the Manager.
func (r *IntegrationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	err := setupReleaseLinkCache(mgr)
	if err != nil {
		return err
	}
	err = setupApplicationComponentCache(mgr)
	if err != nil {
		return err
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&tektonv1beta1.PipelineRun{}).
		WithEventFilter(tekton.BuildPipelineRunSucceededPredicate()).
		Complete(r)
}
