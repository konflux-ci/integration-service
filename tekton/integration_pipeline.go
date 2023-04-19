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

package tekton

import (
	"encoding/json"
	"fmt"
	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/api/v1beta1"
	"github.com/redhat-appstudio/integration-service/helpers"
	tektonv1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

	// ApplicationNameLabel is the label of specific the name of the Application associated with PipelineRun
	ApplicationNameLabel = fmt.Sprintf("%s/%s", ResourceLabelSuffix, "application")

	// ComponentNameLabel is the label of specific the name of the component associated with PipelineRun
	ComponentNameLabel = fmt.Sprintf("%s/%s", ResourceLabelSuffix, "component")

	// OptionalLabel is the label used to specify if an IntegrationTestScenario is allowed to fail
	OptionalLabel = fmt.Sprintf("%s/%s", TestLabelPrefix, "optional")
)

// IntegrationPipelineRun is a PipelineRun alias, so we can add new methods to it in this file.
type IntegrationPipelineRun struct {
	tektonv1beta1.PipelineRun
}

// AsPipelineRun casts the IntegrationPipelineRun to PipelineRun, so it can be used in the Kubernetes client.
func (r *IntegrationPipelineRun) AsPipelineRun() *tektonv1beta1.PipelineRun {
	return &r.PipelineRun
}

// NewIntegrationPipelineRun creates an empty PipelineRun in the given namespace. The name will be autogenerated,
// using the prefix passed as an argument to the function.
func NewIntegrationPipelineRun(prefix, namespace string, integrationTestScenario v1beta1.IntegrationTestScenario) *IntegrationPipelineRun {
	resolverParams := []tektonv1beta1.Param{}

	for _, scenarioParam := range integrationTestScenario.Spec.ResolverRef.Params {
		resolverParam := tektonv1beta1.Param{
			Name: scenarioParam.Name,
			Value: tektonv1beta1.ParamValue{
				Type:      tektonv1beta1.ParamTypeString,
				StringVal: scenarioParam.Value,
			},
		}
		resolverParams = append(resolverParams, resolverParam)
	}

	pipelineRun := tektonv1beta1.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: prefix + "-",
			Namespace:    namespace,
		},
		Spec: tektonv1beta1.PipelineRunSpec{
			PipelineRef: &tektonv1beta1.PipelineRef{
				ResolverRef: tektonv1beta1.ResolverRef{
					Resolver: tektonv1beta1.ResolverName(integrationTestScenario.Spec.ResolverRef.Resolver),
					Params:   resolverParams,
				},
			},
		},
	}
	return &IntegrationPipelineRun{pipelineRun}
}

// WithExtraParam adds an extra param to the Integration PipelineRun. If the parameter is not part of the Pipeline
// definition, it will be silently ignored.
func (r *IntegrationPipelineRun) WithExtraParam(name string, value tektonv1beta1.ArrayOrString) *IntegrationPipelineRun {
	r.Spec.Params = append(r.Spec.Params, tektonv1beta1.Param{
		Name:  name,
		Value: value,
	})

	return r
}

// WithExtraParams adds all provided parameters to the Integration PipelineRun.
func (r *IntegrationPipelineRun) WithExtraParams(params []v1beta1.PipelineParameter) *IntegrationPipelineRun {
	for _, param := range params {
		var value tektonv1beta1.ArrayOrString
		switch {
		case param.Value != "":
			value.StringVal = param.Value
			value.Type = tektonv1beta1.ParamTypeString
		case len(param.Values) > 0:
			value.ArrayVal = param.Values
			value.Type = tektonv1beta1.ParamTypeArray
		default:
			value.Type = tektonv1beta1.ParamTypeObject
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

	r.WithExtraParam("SNAPSHOT", tektonv1beta1.ArrayOrString{
		Type:      tektonv1beta1.ParamTypeString,
		StringVal: string(snapshotString),
	})

	if r.ObjectMeta.Labels == nil {
		r.ObjectMeta.Labels = map[string]string{}
	}
	r.ObjectMeta.Labels[SnapshotNameLabel] = snapshot.Name

	return r
}

// WithIntegrationLabels adds the type, optional flag and IntegrationTestScenario name as labels to the Integration PipelineRun.
func (r *IntegrationPipelineRun) WithIntegrationLabels(integrationTestScenario *v1beta1.IntegrationTestScenario) *IntegrationPipelineRun {
	if r.ObjectMeta.Labels == nil {
		r.ObjectMeta.Labels = map[string]string{}
	}
	r.ObjectMeta.Labels[PipelinesTypeLabel] = PipelineTypeTest
	r.ObjectMeta.Labels[ScenarioNameLabel] = integrationTestScenario.Name

	if helpers.HasLabel(integrationTestScenario, OptionalLabel) {
		r.ObjectMeta.Labels[OptionalLabel] = integrationTestScenario.Labels[OptionalLabel]
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
