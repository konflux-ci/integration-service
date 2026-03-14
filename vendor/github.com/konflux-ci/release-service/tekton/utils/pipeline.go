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

package utils

import (
	"fmt"
	"time"

	"github.com/go-logr/logr"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Param defines the parameters for a given resolver in PipelineRef
type Param struct {
	// Name is the name of the parameter
	Name string `json:"name"`

	// Value is the value of the parameter
	Value string `json:"value"`
}

// PipelineRef represents a reference to a Pipeline using a resolver.
// +kubebuilder:object:generate=true
type PipelineRef struct {
	// Resolver is the name of a Tekton resolver to be used (e.g. git)
	Resolver string `json:"resolver"`

	// Params is a slice of parameters for a given resolver
	Params []Param `json:"params"`

	// OciStorage specifies the OCI repository where the Trusted Artifacts are stored.
	// This value is passed to the Pipeline as the "ociStorage" parameter.
	// If not set, the default value from the Pipeline definition will be used.
	// This field is intended for use in ReleasePlanAdmissions.
	// +optional
	OciStorage string `json:"ociStorage,omitempty"`

	// UseEmptyDir specifies whether to use an empty dir volume for the workspace.
	// When true, the PipelineRun will use an empty dir volume. Otherwise, it will
	// use a volume claim template by default.
	// +optional
	UseEmptyDir bool `json:"useEmptyDir,omitempty"`
}

// Pipeline contains a reference to a Pipeline and the name of the service account to use while executing it.
// +kubebuilder:object:generate=true
type Pipeline struct {
	// PipelineRef is the reference to the Pipeline
	PipelineRef PipelineRef `json:"pipelineRef"`

	// ServiceAccountName is the ServiceAccount to use during the execution of the Pipeline
	// +kubebuilder:validation:Pattern=^[a-z0-9]([-a-z0-9]*[a-z0-9])?$
	// +optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`

	// TaskRunSpecs is the PipelineTaskRunSpec to be used in the PipelineRun execution
	// +optional
	TaskRunSpecs []tektonv1.PipelineTaskRunSpec `json:"taskRunSpecs,omitempty"`

	// MaxRetries sets the retry limit for the pipeline overriding the value in
	// ReleaseServiceConfig when specified. When unset the ReleaseServiceConfig
	// value is used and 0 disables retries for the pipeline.
	// +kubebuilder:validation:Minimum=0
	// +optional
	MaxRetries *int `json:"maxRetries,omitempty"`

	// Timeouts defines the different Timeouts to use in the PipelineRun execution
	// +optional
	Timeouts tektonv1.TimeoutFields `json:"timeouts,omitempty"`
}

// ParameterizedPipeline is an extension of the Pipeline struct, adding an array of parameters that will be passed to
// the Pipeline. Used by ReleasePlan's TenantPipeline and FinalPipeline.
// +kubebuilder:object:generate=true
type ParameterizedPipeline struct {
	Pipeline `json:",inline"`

	// Params is a slice of parameters to be passed to the Pipeline
	// +optional
	Params []Param `json:"params,omitempty"`
}

// GetGitResolverParams returns the common parameters found in a Git resolver. That is url, revision and pathInRepo.
// If the PipelineRef doesn't use a git resolver this function will return an error.
func (pr *PipelineRef) GetGitResolverParams() (string, string, string, error) {
	if pr.Resolver != "git" {
		return "", "", "", fmt.Errorf("not a git ref")
	}

	var url, revision, pathInRepo string
	for _, param := range pr.Params {
		switch param.Name {
		case "url":
			url = param.Value
		case "revision":
			revision = param.Value
		case "pathInRepo":
			pathInRepo = param.Value
		}
	}

	return url, revision, pathInRepo, nil
}

// GetRevision returns the value of the revision param. If not found an error will be raised.
func (pr *PipelineRef) GetRevision() (string, error) {
	for _, param := range pr.Params {
		if param.Name == "revision" {
			return param.Value, nil
		}
	}

	return "", fmt.Errorf("no revision found")
}

// GetUrl returns the value of the url param. If not found an error will be raised.
func (pr *PipelineRef) GetUrl() (string, error) {
	for _, param := range pr.Params {
		if param.Name == "url" {
			return param.Value, nil
		}
	}

	return "", fmt.Errorf("no url found")
}

// ToTektonPipelineRef converts a PipelineRef object to Tekton's own PipelineRef type and returns it.
func (pr *PipelineRef) ToTektonPipelineRef() *tektonv1.PipelineRef {
	params := tektonv1.Params{}

	for _, p := range pr.Params {
		params = append(params, tektonv1.Param{
			Name: p.Name,
			Value: tektonv1.ParamValue{
				Type:      tektonv1.ParamTypeString,
				StringVal: p.Value,
			},
		})
	}

	tektonPipelineRef := &tektonv1.PipelineRef{
		ResolverRef: tektonv1.ResolverRef{
			Resolver: tektonv1.ResolverName(pr.Resolver),
			Params:   params,
		},
	}

	return tektonPipelineRef
}

// GetOciStorageParam returns the ociStorage parameter as a Tekton param slice.
// Returns an empty slice if OciStorage is not set.
func (pr *PipelineRef) GetOciStorageParam() []tektonv1.Param {
	if pr.OciStorage != "" {
		return []tektonv1.Param{
			{
				Name: "ociStorage",
				Value: tektonv1.ParamValue{
					Type:      tektonv1.ParamTypeString,
					StringVal: pr.OciStorage,
				},
			},
		}
	}
	return []tektonv1.Param{}
}

// GetOciStorageParam returns the ociStorage parameter from the Pipeline's PipelineRef.
// Used by the release adapter to pass the ociStorage parameter to the managed PipelineRun.
func (p *Pipeline) GetOciStorageParam() []tektonv1.Param {
	return p.PipelineRef.GetOciStorageParam()
}

// GetTektonParams returns the ParameterizedPipeline's Params as []tektonv1.Param.
// This is used by ReleasePlan's TenantPipeline and FinalPipeline.
func (prp *ParameterizedPipeline) GetTektonParams() []tektonv1.Param {
	params := []tektonv1.Param{}

	for _, param := range prp.Params {
		params = append(params, tektonv1.Param{
			Name: param.Name,
			Value: tektonv1.ParamValue{
				Type:      tektonv1.ParamTypeString,
				StringVal: param.Value,
			},
		})
	}

	return params
}

// IsClusterScoped returns whether the PipelineRef uses a cluster resolver or not.
func (pr *PipelineRef) IsClusterScoped() bool {
	return pr.Resolver == "cluster"
}

// AdjustTimeouts adjusts the timeout values to ensure they follow the rule: pipeline >= tasks + finally.
// It modifies the timeouts as needed based on which values are set and logs the action taken.
// The adjustment buffer is 10 minutes, or half of pipeline timeout if pipeline is less than 20 minutes.
func AdjustTimeouts(timeouts *tektonv1.TimeoutFields, logger logr.Logger) *tektonv1.TimeoutFields {
	const defaultAdjustmentBuffer = 10 * time.Minute
	const shortPipelineThreshold = 20 * time.Minute
	const minPipelineWarningThreshold = 5 * time.Minute

	// If timeouts is nil, return as is
	if timeouts == nil {
		logger.Info("Timeouts not set, returning nil")
		return nil
	}

	// Check which fields are set
	pipelineSet := timeouts.Pipeline != nil && timeouts.Pipeline.Duration > 0
	tasksSet := timeouts.Tasks != nil && timeouts.Tasks.Duration > 0
	finallySet := timeouts.Finally != nil && timeouts.Finally.Duration > 0

	// If all are unset, return as is
	if !pipelineSet && !tasksSet && !finallySet {
		logger.Info("All timeout fields are unset, returning original timeouts")
		return timeouts
	}

	// Create a copy to avoid modifying the original
	result := &tektonv1.TimeoutFields{}
	if pipelineSet {
		result.Pipeline = &metav1.Duration{Duration: timeouts.Pipeline.Duration}
	}
	if tasksSet {
		result.Tasks = &metav1.Duration{Duration: timeouts.Tasks.Duration}
	}
	if finallySet {
		result.Finally = &metav1.Duration{Duration: timeouts.Finally.Duration}
	}

	// Calculate adjustment buffer: use pipeline/2 if pipeline < 20min, otherwise default 10min
	adjustmentBuffer := defaultAdjustmentBuffer
	if pipelineSet && result.Pipeline.Duration < shortPipelineThreshold {
		adjustmentBuffer = result.Pipeline.Duration / 2
	}

	// Handle all scenarios
	switch {
	case pipelineSet && tasksSet && finallySet:
		// All three are set
		if result.Pipeline.Duration >= result.Tasks.Duration+result.Finally.Duration {
			logger.Info("All timeouts set and valid (pipeline >= tasks + finally), returning original values",
				"pipeline", result.Pipeline.Duration, "tasks", result.Tasks.Duration, "finally", result.Finally.Duration)
		} else {
			// They don't follow the rule: set tasks and finally based on adjustment buffer
			result.Tasks = &metav1.Duration{Duration: result.Pipeline.Duration - adjustmentBuffer}
			result.Finally = &metav1.Duration{Duration: adjustmentBuffer}
			logger.Info("All timeouts set but invalid, adjusted tasks and finally",
				"pipeline", result.Pipeline.Duration, "tasks", result.Tasks.Duration, "finally", result.Finally.Duration,
				"adjustmentBuffer", adjustmentBuffer)
		}

	case pipelineSet && tasksSet && !finallySet:
		// Only pipeline and tasks are set
		if result.Tasks.Duration < result.Pipeline.Duration {
			logger.Info("Pipeline and tasks set, tasks is lower than pipeline, returning original values",
				"pipeline", result.Pipeline.Duration, "tasks", result.Tasks.Duration)
		} else {
			// tasks >= pipeline, set tasks based on adjustment buffer
			result.Tasks = &metav1.Duration{Duration: result.Pipeline.Duration - adjustmentBuffer}
			logger.Info("Pipeline and tasks set, tasks was >= pipeline, adjusted tasks",
				"pipeline", result.Pipeline.Duration, "tasks", result.Tasks.Duration,
				"adjustmentBuffer", adjustmentBuffer)
		}

	case pipelineSet && !tasksSet && finallySet:
		// Only pipeline and finally are set
		if result.Finally.Duration <= result.Pipeline.Duration/2 {
			// finally is at most half of pipeline, use current logic
			result.Tasks = &metav1.Duration{Duration: result.Pipeline.Duration - result.Finally.Duration}
			logger.Info("Pipeline and finally set (finally <= pipeline/2), calculated tasks as pipeline - finally",
				"pipeline", result.Pipeline.Duration, "tasks", result.Tasks.Duration, "finally", result.Finally.Duration)
		} else {
			// finally is more than half of pipeline, set tasks and finally based on adjustment buffer
			result.Tasks = &metav1.Duration{Duration: result.Pipeline.Duration - adjustmentBuffer}
			result.Finally = &metav1.Duration{Duration: adjustmentBuffer}
			logger.Info("Pipeline and finally set (finally > pipeline/2), adjusted tasks and finally based on adjustment buffer",
				"pipeline", result.Pipeline.Duration, "tasks", result.Tasks.Duration, "finally", result.Finally.Duration,
				"adjustmentBuffer", adjustmentBuffer)
		}

	case pipelineSet && !tasksSet && !finallySet:
		// Only pipeline is set
		result.Tasks = &metav1.Duration{Duration: result.Pipeline.Duration - adjustmentBuffer}
		logger.Info("Only pipeline set, calculated tasks as pipeline - adjustmentBuffer",
			"pipeline", result.Pipeline.Duration, "tasks", result.Tasks.Duration,
			"adjustmentBuffer", adjustmentBuffer)

	case !pipelineSet && tasksSet && !finallySet:
		// Only tasks is set - use default buffer since we don't have pipeline
		result.Pipeline = &metav1.Duration{Duration: result.Tasks.Duration + adjustmentBuffer}
		logger.Info("Only tasks set, calculated pipeline as tasks + adjustmentBuffer",
			"pipeline", result.Pipeline.Duration, "tasks", result.Tasks.Duration,
			"adjustmentBuffer", adjustmentBuffer)

	case !pipelineSet && !tasksSet && finallySet:
		// Only finally is set - return empty struct so defaults will be used
		logger.Info("WARNING: Only finally timeout is set, returning empty timeouts to use defaults",
			"finally", result.Finally.Duration)
		return &tektonv1.TimeoutFields{}

	case !pipelineSet && tasksSet && finallySet:
		// Only finally and tasks are set
		result.Pipeline = &metav1.Duration{Duration: result.Tasks.Duration + result.Finally.Duration}
		logger.Info("Tasks and finally set, calculated pipeline as tasks + finally",
			"pipeline", result.Pipeline.Duration, "tasks", result.Tasks.Duration, "finally", result.Finally.Duration)
	}

	// Warn if pipeline timeout is less than 5 minutes
	if result.Pipeline != nil && result.Pipeline.Duration < minPipelineWarningThreshold {
		logger.Info("WARNING: Pipeline timeout is less than 5 minutes, this may not be enough time for the pipeline to complete",
			"pipeline", result.Pipeline.Duration)
	}

	return result
}
