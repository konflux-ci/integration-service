/*
Copyright 2022 Red Hat Inc.

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

package tekton

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/go-logr/logr"
	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/api/v1beta2"
	"github.com/redhat-appstudio/operator-toolkit/metadata"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"os"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"time"
)

const (
	// PipelinesLabelPrefix is the prefix of the pipelines label
	PipelinesLabelPrefix = "pipelines.appstudio.openshift.io"

	// TestLabelPrefix is the prefix of the test labels
	TestLabelPrefix = "test.appstudio.openshift.io"

	// resource labels for snapshot, application and component
	ResourceLabelSuffix = "appstudio.openshift.io"

	// PipelineTypeTest is the type for PipelineRuns created to run an integration Pipeline
	PipelineTypeTest = "test"
)

var (
	// PipelinesTypeLabel is the label used to describe the type of pipeline
	PipelinesTypeLabel = fmt.Sprintf("%s/%s", PipelinesLabelPrefix, "type")

	// TestNameLabel is the label used to specify the name of the Test associated with the PipelineRun
	TestNameLabel = fmt.Sprintf("%s/%s", TestLabelPrefix, "name")

	// ScenarioNameLabel is the label used to specify the name of the IntegrationTestScenario associated with the PipelineRun
	ScenarioNameLabel = fmt.Sprintf("%s/%s", TestLabelPrefix, "scenario")

	// SnapshotNameLabel is the label of specific the name of the snapshot associated with PipelineRun
	SnapshotNameLabel = fmt.Sprintf("%s/%s", ResourceLabelSuffix, "snapshot")

	// EnvironmentNameLabel is the label of specific the name of the environment associated with PipelineRun
	EnvironmentNameLabel = fmt.Sprintf("%s/%s", ResourceLabelSuffix, "environment")

	// ApplicationNameLabel is the label of specific the name of the Application associated with PipelineRun
	ApplicationNameLabel = fmt.Sprintf("%s/%s", ResourceLabelSuffix, "application")

	// ComponentNameLabel is the label of specific the name of the component associated with PipelineRun
	ComponentNameLabel = fmt.Sprintf("%s/%s", ResourceLabelSuffix, "component")

	// OptionalLabel is the label used to specify if an IntegrationTestScenario is allowed to fail
	OptionalLabel = fmt.Sprintf("%s/%s", TestLabelPrefix, "optional")
)

// IntegrationPipelineRun is a PipelineRun alias, so we can add new methods to it in this file.
type IntegrationPipelineRun struct {
	tektonv1.PipelineRun
}

// AsPipelineRun casts the IntegrationPipelineRun to PipelineRun, so it can be used in the Kubernetes client.
func (r *IntegrationPipelineRun) AsPipelineRun() *tektonv1.PipelineRun {
	return &r.PipelineRun
}

// NewIntegrationPipelineRun creates an empty PipelineRun in the given namespace. The name will be autogenerated,
// using the prefix passed as an argument to the function.
func NewIntegrationPipelineRun(prefix, namespace string, integrationTestScenario v1beta2.IntegrationTestScenario) *IntegrationPipelineRun {
	resolverParams := []tektonv1.Param{}

	for _, scenarioParam := range integrationTestScenario.Spec.ResolverRef.Params {
		resolverParam := tektonv1.Param{
			Name: scenarioParam.Name,
			Value: tektonv1.ParamValue{
				Type:      tektonv1.ParamTypeString,
				StringVal: scenarioParam.Value,
			},
		}
		resolverParams = append(resolverParams, resolverParam)
	}

	pipelineRun := tektonv1.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: prefix + "-",
			Namespace:    namespace,
		},
		Spec: tektonv1.PipelineRunSpec{
			PipelineRef: &tektonv1.PipelineRef{
				ResolverRef: tektonv1.ResolverRef{
					Resolver: tektonv1.ResolverName(integrationTestScenario.Spec.ResolverRef.Resolver),
					Params:   resolverParams,
				},
			},
		},
	}
	return &IntegrationPipelineRun{pipelineRun}
}

// WithFinalizer adds a Finalizer on the Integration PipelineRun
func (iplr *IntegrationPipelineRun) WithFinalizer(finalizer string) *IntegrationPipelineRun {
	controllerutil.AddFinalizer(iplr, finalizer)

	return iplr
}

// WithExtraParam adds an extra param to the Integration PipelineRun. If the parameter is not part of the Pipeline
// definition, it will be silently ignored.
func (r *IntegrationPipelineRun) WithExtraParam(name string, value tektonv1.ParamValue) *IntegrationPipelineRun {
	r.Spec.Params = append(r.Spec.Params, tektonv1.Param{
		Name:  name,
		Value: value,
	})

	return r
}

// WithExtraParams adds all provided parameters to the Integration PipelineRun.
func (r *IntegrationPipelineRun) WithExtraParams(params []v1beta2.PipelineParameter) *IntegrationPipelineRun {
	for _, param := range params {
		var value tektonv1.ParamValue
		switch {
		case param.Value != "":
			value.StringVal = param.Value
			value.Type = tektonv1.ParamTypeString
		case len(param.Values) > 0:
			value.ArrayVal = param.Values
			value.Type = tektonv1.ParamTypeArray
		default:
			value.Type = tektonv1.ParamTypeObject
		}
		r.WithExtraParam(param.Name, value)
	}

	return r
}

// WithSnapshot adds a param containing the Snapshot as a json string
// to the integration PipelineRun.
func (r *IntegrationPipelineRun) WithSnapshot(snapshot *applicationapiv1alpha1.Snapshot) *IntegrationPipelineRun {
	// We ignore the error here because none should be raised when marshalling the spec of a CRD.
	// If we end up deciding it is useful, we will need to pass the errors through the chain and
	// add something like a `Complete` function that returns the final object and error.
	snapshotString, _ := json.Marshal(snapshot.Spec)

	r.WithExtraParam("SNAPSHOT", tektonv1.ParamValue{
		Type:      tektonv1.ParamTypeString,
		StringVal: string(snapshotString),
	})

	if r.ObjectMeta.Labels == nil {
		r.ObjectMeta.Labels = map[string]string{}
	}
	r.ObjectMeta.Labels[SnapshotNameLabel] = snapshot.Name

	return r
}

// WithIntegrationLabels adds the type, optional flag and IntegrationTestScenario name as labels to the Integration PipelineRun.
func (r *IntegrationPipelineRun) WithIntegrationLabels(integrationTestScenario *v1beta2.IntegrationTestScenario) *IntegrationPipelineRun {
	if r.ObjectMeta.Labels == nil {
		r.ObjectMeta.Labels = map[string]string{}
	}
	r.ObjectMeta.Labels[PipelinesTypeLabel] = PipelineTypeTest
	r.ObjectMeta.Labels[ScenarioNameLabel] = integrationTestScenario.Name

	if metadata.HasLabel(integrationTestScenario, OptionalLabel) {
		r.ObjectMeta.Labels[OptionalLabel] = integrationTestScenario.Labels[OptionalLabel]
	}

	return r
}

// WithIntegrationAnnotations copies the App Studio annotations from the
// IntegrationTestScenario to the PipelineRun
func (r *IntegrationPipelineRun) WithIntegrationAnnotations(its *v1beta2.IntegrationTestScenario) *IntegrationPipelineRun {
	for k, v := range its.GetAnnotations() {
		if strings.Contains(k, "appstudio.openshift.io/") {
			if err := metadata.SetAnnotation(r, k, v); err != nil {
				// this will only happen if we pass IntegrationPipelineRun as nil
				panic(err)
			}
		}
	}

	return r
}

// WithApplicationAndComponent adds the name of both application and component as lables to the Integration PipelineRun.
func (r *IntegrationPipelineRun) WithApplicationAndComponent(application *applicationapiv1alpha1.Application, component *applicationapiv1alpha1.Component) *IntegrationPipelineRun {
	if r.ObjectMeta.Labels == nil {
		r.ObjectMeta.Labels = map[string]string{}
	}
	if component != nil {
		r.ObjectMeta.Labels[ComponentNameLabel] = component.Name
	}
	r.ObjectMeta.Labels[ApplicationNameLabel] = application.Name

	return r
}

// WithEnvironmentAndDeploymentTarget adds a param containing the DeploymentTarget connection details and Environment name
// to the integration PipelineRun.
func (r *IntegrationPipelineRun) WithEnvironmentAndDeploymentTarget(dt *applicationapiv1alpha1.DeploymentTarget, environmentName string) *IntegrationPipelineRun {
	if !reflect.ValueOf(dt.Spec.KubernetesClusterCredentials).IsZero() {
		// Add the NAMESPACE parameter to the pipeline
		r.WithExtraParam("NAMESPACE", tektonv1.ParamValue{
			Type:      tektonv1.ParamTypeString,
			StringVal: dt.Spec.KubernetesClusterCredentials.DefaultNamespace,
		})

		// Create a new Workspace binding which will allow mounting the ClusterCredentialsSecret in the Tekton pipelineRun
		workspace := tektonv1.WorkspaceBinding{
			Name:   "cluster-credentials",
			Secret: &corev1.SecretVolumeSource{SecretName: dt.Spec.KubernetesClusterCredentials.ClusterCredentialsSecret},
		}
		// Add the new workspace to the pipelineRun Spec
		if r.Spec.Workspaces == nil {
			r.Spec.Workspaces = []tektonv1.WorkspaceBinding{}
		}
		r.Spec.Workspaces = append(r.Spec.Workspaces, workspace)
	}

	// Add the environment label to the pipelineRun
	if r.ObjectMeta.Labels == nil {
		r.ObjectMeta.Labels = map[string]string{}
	}
	r.ObjectMeta.Labels[EnvironmentNameLabel] = environmentName

	return r
}

// WithDefaultIntegrationTimeouts fetches the default Integration timeouts from the environment variables and adds them
// to the integration PipelineRun.
func (r *IntegrationPipelineRun) WithDefaultIntegrationTimeouts(logger logr.Logger) *IntegrationPipelineRun {
	pipelineTimeoutStr := os.Getenv("PIPELINE_TIMEOUT")
	taskTimeoutStr := os.Getenv("TASKS_TIMEOUT")
	finallyTimeoutStr := os.Getenv("FINALLY_TIMEOUT")
	r.Spec.Timeouts = &tektonv1.TimeoutFields{}
	if pipelineTimeoutStr != "" {
		pipelineRunTimeout, err := time.ParseDuration(pipelineTimeoutStr)
		if err == nil {
			r.Spec.Timeouts.Pipeline = &metav1.Duration{Duration: pipelineRunTimeout}
		} else {
			logger.Error(err, "failed to parse default PIPELINE_TIMEOUT")
		}
	}
	if taskTimeoutStr != "" {
		taskTimeout, err := time.ParseDuration(taskTimeoutStr)
		if err == nil {
			r.Spec.Timeouts.Tasks = &metav1.Duration{Duration: taskTimeout}
		} else {
			logger.Error(err, "failed to parse default TASKS_TIMEOUT")
		}
	}
	if finallyTimeoutStr != "" {
		finallyTimeout, err := time.ParseDuration(finallyTimeoutStr)
		if err == nil {
			r.Spec.Timeouts.Finally = &metav1.Duration{Duration: finallyTimeout}
		} else {
			logger.Error(err, "failed to parse default FINALLY_TIMEOUT")
		}
	}

	return r
}
