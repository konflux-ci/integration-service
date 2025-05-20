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
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"

	"github.com/konflux-ci/integration-service/gitops"
	"github.com/konflux-ci/integration-service/helpers"

	"os"
	"time"

	"github.com/ghodss/yaml" // used instead of gopkg.in/yaml.v3 because it treats json tags as yaml tags
	"github.com/go-logr/logr"
	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	"github.com/konflux-ci/integration-service/api/v1beta2"
	"github.com/konflux-ci/operator-toolkit/metadata"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	resolutionv1beta1 "github.com/tektoncd/pipeline/pkg/apis/resolution/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	knative "knative.dev/pkg/apis"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	// PipelinesLabelPrefix is the prefix of the pipelines label
	PipelinesLabelPrefix = "pipelines.appstudio.openshift.io"

	// TestLabelPrefix contains the prefix applied to labels and annotations related to testing.
	TestLabelPrefix = "test.appstudio.openshift.io"

	// PipelinesAsCodePrefix contains the prefix applied to labels and annotations copied from Pipelines as Code resources.
	PipelinesAsCodePrefix = "pac.test.appstudio.openshift.io"

	// BuildPipelineRunPrefix contains the build pipeline run related labels and annotations
	BuildPipelineRunPrefix = "build.appstudio"

	// CustomLabelPrefix contains the prefix applied to custom user-defined labels and annotations.
	CustomLabelPrefix = "custom.appstudio.openshift.io"

	// resource labels for snapshot, application and component
	ResourceLabelSuffix = "appstudio.openshift.io"

	// PipelineTypeTest is the type for PipelineRuns created to run an integration Pipeline
	PipelineTypeTest = "test"

	// Name of tekton resolver for git
	TektonResolverGit = "git"

	// Name of tekton git resolver param url
	TektonResolverGitParamURL = "url"

	// Name of tekton git resolver param revision
	TektonResolverGitParamRevision = "revision"

	// Value of ResourceKind field for remote pipelines
	ResourceKindPipeline = "pipeline"

	// Value of ResourceKind field for remote pipelineruns
	ResourceKindPipelineRun = "pipelinerun"
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

// SummaryTemplateData holds the data necessary to construct a PipelineRun summary.
type SummaryTemplateData struct {
	TaskRuns               []*helpers.TaskRun
	PipelineRunName        string
	Namespace              string
	PRGroup                string
	ComponentSnapshotInfos []*gitops.ComponentSnapshotInfo
	Logger                 logr.Logger
}

// AsPipelineRun casts the IntegrationPipelineRun to PipelineRun, so it can be used in the Kubernetes client.
func (r *IntegrationPipelineRun) AsPipelineRun() *tektonv1.PipelineRun {
	return &r.PipelineRun
}

// NewIntegrationPipelineRun creates an empty PipelineRun in the given namespace. The name will be autogenerated,
// using the prefix passed as an argument to the function.
func NewIntegrationPipelineRun(client client.Client, ctx context.Context, prefix, namespace string, integrationTestScenario v1beta2.IntegrationTestScenario) (*IntegrationPipelineRun, error) {
	resolverParams := integrationTestScenario.Spec.ResolverRef.Params

	resolver := integrationTestScenario.Spec.ResolverRef.Resolver
	resourceKind := integrationTestScenario.Spec.ResolverRef.ResourceKind
	tektonResolverParams := generateTektonResolverParams(resolverParams)
	if resourceKind == ResourceKindPipeline || resourceKind == "" {
		return generateIntegrationPipelineRunFromPipelineResolver(prefix, namespace, resolver, tektonResolverParams), nil
	} else if resourceKind == ResourceKindPipelineRun {
		base64plr, err := getPipelineRunYamlFromPipelineRunResolver(client, ctx, prefix, namespace, resolver, tektonResolverParams)
		if err != nil {
			return nil, err
		}
		return generateIntegrationPipelineRunFromBase64(base64plr, namespace, prefix)
	} else {
		return nil, fmt.Errorf("unrecognized resolver type '%s'", resourceKind)
	}
}

func generateTektonResolverParams(resolverParams []v1beta2.ResolverParameter) []tektonv1.Param {
	tektonResolverParams := []tektonv1.Param{}
	for _, scenarioParam := range resolverParams {
		tektonResolverParam := tektonv1.Param{
			Name: scenarioParam.Name,
			Value: tektonv1.ParamValue{
				Type:      tektonv1.ParamTypeString,
				StringVal: scenarioParam.Value,
			},
		}
		tektonResolverParams = append(tektonResolverParams, tektonResolverParam)
	}
	return tektonResolverParams
}

func generateIntegrationPipelineRunFromPipelineResolver(prefix, namespace, resolver string, resolverParams []tektonv1.Param) *IntegrationPipelineRun {
	pipelineRun := tektonv1.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: prefix + "-",
			Namespace:    namespace,
		},
		Spec: tektonv1.PipelineRunSpec{
			PipelineRef: &tektonv1.PipelineRef{
				ResolverRef: tektonv1.ResolverRef{
					Resolver: tektonv1.ResolverName(resolver),
					Params:   resolverParams,
				},
			},
		},
	}

	return &IntegrationPipelineRun{pipelineRun}
}

func getPipelineRunYamlFromPipelineRunResolver(client client.Client, ctx context.Context, prefix, namespace, resolver string, resolverParams []tektonv1.Param) (string, error) {
	request := resolutionv1beta1.ResolutionRequest{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: prefix + "-",
			Namespace:    namespace,
			Labels: map[string]string{
				"resolution.tekton.dev/type": resolver,
				"konflux-ci.dev/created-by":  "integration-service", // for backup garbage collection
			},
		},
		Spec: resolutionv1beta1.ResolutionRequestSpec{
			Params: resolverParams,
		},
	}

	err := retry.OnError(retry.DefaultBackoff, func(e error) bool { return true }, func() error {
		return client.Create(ctx, &request)
	})
	if err != nil {
		return "", err
	}

	resolutionRequestName := request.ObjectMeta.Name
	defer func() {
		_ = retry.OnError(retry.DefaultBackoff, func(e error) bool { return true }, func() error {
			return client.Delete(ctx, &request)
		})
	}()

	resolverBackoff := wait.Backoff{
		Steps:    60,
		Duration: 1 * time.Second,
		Factor:   1.0,
		Jitter:   0.5,
	}
	var resolvedRequest resolutionv1beta1.ResolutionRequest
	err = retry.OnError(resolverBackoff, func(e error) bool { return true }, func() error {
		err = client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: resolutionRequestName}, &resolvedRequest)
		if err != nil {
			return err
		}
		for _, cond := range resolvedRequest.Status.Conditions {
			if cond.Type == knative.ConditionSucceeded {
				if cond.Status == corev1.ConditionTrue {
					// request resolved successfully, we can terminate retry block
					return nil
				}
			}
		}
		return fmt.Errorf("resolution for '%s' in namespace '%s' did not complete", namespace, resolutionRequestName)
	})

	if err != nil {
		return "", err
	}
	return resolvedRequest.Status.Data, nil
}

func generateIntegrationPipelineRunFromBase64(base64plr, namespace, defaultPrefix string) (*IntegrationPipelineRun, error) {
	plrYaml, err := base64.StdEncoding.DecodeString(base64plr)
	if err != nil {
		return nil, err
	}

	var pipelineRun tektonv1.PipelineRun
	err = yaml.Unmarshal(plrYaml, &pipelineRun)
	if err != nil {
		return nil, err
	}

	pipelineRun.ObjectMeta.Namespace = namespace
	setGenerateNameForPipelineRun(&pipelineRun, defaultPrefix)
	return &IntegrationPipelineRun{pipelineRun}, nil
}

// Ensures that the PipelineRun will use GenerateName rather than name in order
// to avoid collisions. If the resolved PipelineRun has a GenerateName we use
// that.  If it has a Name we convert it to a GenerateName.  Otherwise we use
// the name of the scenario just like with a remote Pipeline
func setGenerateNameForPipelineRun(pipelineRun *tektonv1.PipelineRun, defaultPrefix string) {
	if pipelineRun.ObjectMeta.GenerateName == "" {
		if pipelineRun.ObjectMeta.Name != "" {
			pipelineRun.ObjectMeta.GenerateName = fmt.Sprintf("%s-", pipelineRun.ObjectMeta.Name)
			pipelineRun.ObjectMeta.Name = ""
		} else {
			pipelineRun.ObjectMeta.GenerateName = defaultPrefix
		}
	}
}

// Updates git resolver values parameters with values of params specified in the input map
// updates only exsitings parameters, doens't create new ones
func (iplr *IntegrationPipelineRun) WithUpdatedTestsGitResolver(params map[string]string) *IntegrationPipelineRun {
	if iplr.Spec.PipelineRef.ResolverRef.Resolver != TektonResolverGit {
		// if the resolver is not git-resolver, we cannot update the git ref
		return iplr
	}

	for originalParamIndex, originalParam := range iplr.Spec.PipelineRef.ResolverRef.Params {
		if _, ok := params[originalParam.Name]; ok {
			// remeber to use the original index to update the value, we cannot update value given by range directly
			iplr.Spec.PipelineRef.ResolverRef.Params[originalParamIndex].Value.StringVal = params[originalParam.Name]
		}
	}

	return iplr
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

// WithSnapshot adds a param containing the Snapshot as a json string to the integration PipelineRun.
// It also adds the Snapshot name label and copies the Component name label if it exists
func (r *IntegrationPipelineRun) WithSnapshot(snapshot *applicationapiv1alpha1.Snapshot, logger logr.Logger) *IntegrationPipelineRun {
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

	componentLabel, found := snapshot.GetLabels()[ComponentNameLabel]
	if found {
		r.ObjectMeta.Labels[ComponentNameLabel] = componentLabel
	}

	// copy PipelineRun PAC, build, test and custom annotations/labels from Snapshot to integration test PipelineRun
	prefixes := []string{PipelinesAsCodePrefix, BuildPipelineRunPrefix, TestLabelPrefix, CustomLabelPrefix}
	for _, prefix := range prefixes {
		// Copy labels and annotations prefixed with defined prefix
		_ = metadata.CopyAnnotationsByPrefix(&snapshot.ObjectMeta, &r.ObjectMeta, prefix)
		_ = metadata.CopyLabelsByPrefix(&snapshot.ObjectMeta, &r.ObjectMeta, prefix)
	}

	// If the Annotation is PaC log-url, we need to set the value to the console URL of the PipelineRun
	if metadata.HasAnnotation(r, PipelinesAsCodePrefix+"/log-url") {

		pipelineRunConsoleURL := FormatConsolePipelineURL(r.Name, r.Namespace, logger)
		_ = metadata.SetAnnotation(r, PipelinesAsCodePrefix+"/log-url", pipelineRunConsoleURL)
	}
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

// WithApplication adds the name of application as a label to the Integration PipelineRun.
func (r *IntegrationPipelineRun) WithApplication(application *applicationapiv1alpha1.Application) *IntegrationPipelineRun {
	if r.ObjectMeta.Labels == nil {
		r.ObjectMeta.Labels = map[string]string{}
	}
	r.ObjectMeta.Labels[ApplicationNameLabel] = application.Name

	return r
}

// WithIntegrationTimeouts fetches the Integration timeouts from either the integrationTestScenario annotations or
// the environment variables and adds them to the integration PipelineRun.
func (r *IntegrationPipelineRun) WithIntegrationTimeouts(integrationTestScenario *v1beta2.IntegrationTestScenario, logger logr.Logger) *IntegrationPipelineRun {
	pipelineTimeoutStr := os.Getenv("PIPELINE_TIMEOUT")
	if metadata.HasAnnotation(integrationTestScenario, v1beta2.PipelineTimeoutAnnotation) {
		pipelineTimeoutStr = integrationTestScenario.Annotations[v1beta2.PipelineTimeoutAnnotation]
	}
	taskTimeoutStr := os.Getenv("TASKS_TIMEOUT")
	if metadata.HasAnnotation(integrationTestScenario, v1beta2.TasksTimeoutAnnotation) {
		taskTimeoutStr = integrationTestScenario.Annotations[v1beta2.TasksTimeoutAnnotation]
	}
	finallyTimeoutStr := os.Getenv("FINALLY_TIMEOUT")
	if metadata.HasAnnotation(integrationTestScenario, v1beta2.FinallyTimeoutAnnotation) {
		finallyTimeoutStr = integrationTestScenario.Annotations[v1beta2.FinallyTimeoutAnnotation]
	}

	r.Spec.Timeouts = &tektonv1.TimeoutFields{}
	if pipelineTimeoutStr != "" {
		pipelineRunTimeout, err := time.ParseDuration(pipelineTimeoutStr)
		if err == nil {
			r.Spec.Timeouts.Pipeline = &metav1.Duration{Duration: pipelineRunTimeout}
		} else {
			logger.Error(err, "failed to parse the PIPELINE_TIMEOUT")
		}
	}
	if taskTimeoutStr != "" {
		taskTimeout, err := time.ParseDuration(taskTimeoutStr)
		if err == nil {
			r.Spec.Timeouts.Tasks = &metav1.Duration{Duration: taskTimeout}
		} else {
			logger.Error(err, "failed to parse the TASKS_TIMEOUT")
		}
	}
	if finallyTimeoutStr != "" {
		finallyTimeout, err := time.ParseDuration(finallyTimeoutStr)
		if err == nil {
			r.Spec.Timeouts.Finally = &metav1.Duration{Duration: finallyTimeout}
		} else {
			logger.Error(err, "failed to parse the FINALLY_TIMEOUT")
		}
	}

	// If the sum of tasks and finally timeout durations is greater than the pipeline timeout duration,
	// increase the pipeline timeout to prevent a pipelineRun validation failure
	if r.Spec.Timeouts.Tasks != nil && r.Spec.Timeouts.Finally != nil && r.Spec.Timeouts.Pipeline != nil &&
		r.Spec.Timeouts.Tasks.Duration+r.Spec.Timeouts.Finally.Duration > r.Spec.Timeouts.Pipeline.Duration {
		r.Spec.Timeouts.Pipeline = &metav1.Duration{Duration: r.Spec.Timeouts.Tasks.Duration + r.Spec.Timeouts.Finally.Duration}
		logger.Info(fmt.Sprintf("Setting the pipeline timeout for %s to be the sum of tasks + finally: %.1f hours", r.Name,
			r.Spec.Timeouts.Pipeline.Duration.Hours()))
	}

	return r
}

// FormatConsolePipelineURL accepts a name of application, pipelinerun, namespace and returns a complete pipelineURL.
func FormatConsolePipelineURL(pipelinerun string, namespace string, logger logr.Logger) string {
	console_url := os.Getenv("CONSOLE_URL")
	if console_url == "" {
		return "https://CONSOLE_URL_NOT_AVAILABLE"
	}
	buf := bytes.Buffer{}
	data := SummaryTemplateData{PipelineRunName: pipelinerun, Namespace: namespace}
	t := template.Must(template.New("").Parse(console_url))
	if err := t.Execute(&buf, data); err != nil {
		logger.Error(err, "Error occured when executing template.")
	}
	return buf.String()
}
