/*
Copyright 2023.

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

// Package loader contains functions used to load resource from the cluster
package loader

import (
	"context"
	"fmt"

	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/gitops"
	"github.com/redhat-appstudio/integration-service/tekton"
	releasev1alpha1 "github.com/redhat-appstudio/release-service/api/v1alpha1"
	tektonv1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GetAllEnvironments gets all environments in the namespace
func GetAllEnvironments(c client.Client, ctx context.Context, application *applicationapiv1alpha1.Application) (*[]applicationapiv1alpha1.Environment, error) {

	environmentList := &applicationapiv1alpha1.EnvironmentList{}
	opts := []client.ListOption{
		client.InNamespace(application.Namespace),
	}
	err := c.List(ctx, environmentList, opts...)
	if err != nil {
		return nil, err
	}
	return &environmentList.Items, err
}

// GetReleasesWithSnapshot returns all Releases associated with the given snapshot.
// In the case the List operation fails, an error will be returned.
func GetReleasesWithSnapshot(c client.Client, ctx context.Context, snapshot *applicationapiv1alpha1.Snapshot) (*[]releasev1alpha1.Release, error) {
	releases := &releasev1alpha1.ReleaseList{}
	opts := []client.ListOption{
		client.InNamespace(snapshot.Namespace),
		client.MatchingFields{"spec.snapshot": snapshot.Name},
	}

	err := c.List(ctx, releases, opts...)
	if err != nil {
		return nil, err
	}

	return &releases.Items, nil
}

// GetAllApplicationComponents loads from the cluster all Components associated with the given Application.
// If the Application doesn't have any Components or this is not found in the cluster, an error will be returned.
func GetAllApplicationComponents(c client.Client, ctx context.Context, application *applicationapiv1alpha1.Application) (*[]applicationapiv1alpha1.Component, error) {
	applicationComponents := &applicationapiv1alpha1.ComponentList{}
	opts := []client.ListOption{
		client.InNamespace(application.Namespace),
		client.MatchingFields{"spec.application": application.Name},
	}

	err := c.List(ctx, applicationComponents, opts...)
	if err != nil {
		return nil, err
	}

	return &applicationComponents.Items, nil
}

// GetApplicationFromSnapshot loads from the cluster the Application referenced in the given Snapshot.
// If the Snapshot doesn't specify an Component or this is not found in the cluster, an error will be returned.
func GetApplicationFromSnapshot(c client.Client, ctx context.Context, snapshot *applicationapiv1alpha1.Snapshot) (*applicationapiv1alpha1.Application, error) {
	application := &applicationapiv1alpha1.Application{}
	err := c.Get(ctx, types.NamespacedName{
		Namespace: snapshot.Namespace,
		Name:      snapshot.Spec.Application,
	}, application)

	if err != nil {
		return nil, err
	}

	return application, nil
}

// GetComponentFromSnapshot loads from the cluster the Component referenced in the given Snapshot.
// If the Snapshot doesn't specify an Application or this is not found in the cluster, an error will be returned.
func GetComponentFromSnapshot(c client.Client, ctx context.Context, snapshot *applicationapiv1alpha1.Snapshot) (*applicationapiv1alpha1.Component, error) {
	if componentLabel, ok := snapshot.Labels[gitops.SnapshotComponentLabel]; ok {
		component := &applicationapiv1alpha1.Component{}
		err := c.Get(ctx, types.NamespacedName{
			Namespace: snapshot.Namespace,
			Name:      componentLabel,
		}, component)

		if err != nil {
			return nil, err
		}

		return component, nil
	} else {
		return nil, nil
	}
}

// GetComponentFromPipelineRun loads from the cluster the Component referenced in the given PipelineRun. If the PipelineRun doesn't
// specify a Component or this is not found in the cluster, an error will be returned.
func GetComponentFromPipelineRun(c client.Client, ctx context.Context, pipelineRun *tektonv1beta1.PipelineRun) (*applicationapiv1alpha1.Component, error) {
	if componentName, found := pipelineRun.Labels[tekton.PipelineRunComponentLabel]; found {
		component := &applicationapiv1alpha1.Component{}
		err := c.Get(ctx, types.NamespacedName{
			Namespace: pipelineRun.Namespace,
			Name:      componentName,
		}, component)

		if err != nil {
			return nil, err
		}

		return component, nil
	}

	return nil, nil
}

// GetApplicationFromPipelineRun loads from the cluster the Application referenced in the given PipelineRun. If the PipelineRun doesn't
// specify an Application or this is not found in the cluster, an error will be returned.
func GetApplicationFromPipelineRun(c client.Client, ctx context.Context, pipelineRun *tektonv1beta1.PipelineRun) (*applicationapiv1alpha1.Application, error) {
	if applicationName, found := pipelineRun.Labels[tekton.PipelineRunApplicationLabel]; found {
		application := &applicationapiv1alpha1.Application{}
		err := c.Get(ctx, types.NamespacedName{
			Namespace: pipelineRun.Namespace,
			Name:      applicationName,
		}, application)

		if err != nil {
			return nil, err
		}

		return application, nil
	}

	return nil, nil
}

// GetApplicationFromComponent loads from the cluster the Application referenced in the given Component. If the Component doesn't
// specify an Application or this is not found in the cluster, an error will be returned.
func GetApplicationFromComponent(c client.Client, ctx context.Context, component *applicationapiv1alpha1.Component) (*applicationapiv1alpha1.Application, error) {
	application := &applicationapiv1alpha1.Application{}
	err := c.Get(ctx, types.NamespacedName{
		Namespace: component.Namespace,
		Name:      component.Spec.Application,
	}, application)

	if err != nil {
		return nil, err
	}

	return application, nil
}

// GetEnvironmentFromIntegrationPipelineRun loads from the cluster the Environment referenced in the given PipelineRun.
// If the PipelineRun doesn't specify an Environment or this is not found in the cluster, an error will be returned.
func GetEnvironmentFromIntegrationPipelineRun(c client.Client, ctx context.Context, pipelineRun *tektonv1beta1.PipelineRun) (*applicationapiv1alpha1.Environment, error) {
	if environmentLabel, ok := pipelineRun.Labels[tekton.EnvironmentNameLabel]; ok {
		environment := &applicationapiv1alpha1.Environment{}
		err := c.Get(ctx, types.NamespacedName{
			Namespace: pipelineRun.Namespace,
			Name:      environmentLabel,
		}, environment)

		if err != nil {
			return nil, err
		}

		return environment, nil
	} else {
		return nil, nil
	}
}

// GetSnapshotFromPipelineRun loads from the cluster the Snapshot referenced in the given PipelineRun.
// If the PipelineRun doesn't specify an Snapshot or this is not found in the cluster, an error will be returned.
func GetSnapshotFromPipelineRun(c client.Client, ctx context.Context, pipelineRun *tektonv1beta1.PipelineRun) (*applicationapiv1alpha1.Snapshot, error) {
	if snapshotName, found := pipelineRun.Labels[tekton.SnapshotNameLabel]; found {
		snapshot := &applicationapiv1alpha1.Snapshot{}
		err := c.Get(ctx, types.NamespacedName{
			Namespace: pipelineRun.Namespace,
			Name:      snapshotName,
		}, snapshot)

		if err != nil {
			return nil, err
		}

		return snapshot, nil
	}

	return nil, fmt.Errorf("the pipeline has no snapshot associated with it")
}
