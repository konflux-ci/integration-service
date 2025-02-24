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

package v1alpha1

import (
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"regexp"
)

const ReleaseServiceConfigResourceName string = "release-service-config"

// ReleaseServiceConfigSpec defines the desired state of ReleaseServiceConfig.
type ReleaseServiceConfigSpec struct {
	// Debug is the boolean that specifies whether or not the Release Service should run
	// in debug mode
	// +optional
	Debug bool `json:"debug,omitempty"`

	// DefaultTimeouts contain the default Tekton timeouts to be used in case they are
	// not specified in the ReleasePlanAdmission resource.
	DefaultTimeouts tektonv1.TimeoutFields `json:"defaultTimeouts,omitempty"`

	// VolumeOverrides is a map containing the volume type for specific Pipeline git refs
	// +optional
	EmptyDirOverrides []EmptyDirOverrides `json:"EmptyDirOverrides,omitempty"`
}

// EmptyDirOverrides defines the values usually set in a PipelineRef using a git resolver.
type EmptyDirOverrides struct {
	// Url is the url to the git repo
	// +required
	Url string `json:"url"`

	// Revision is the git revision where the Pipeline definition can be found
	// +required
	Revision string `json:"revision"`

	// PathInRepo is the path within the git repository where the Pipeline definition can be found
	// +required
	PathInRepo string `json:"pathInRepo"`
}

// ReleaseServiceConfigStatus defines the observed state of ReleaseServiceConfig.
type ReleaseServiceConfigStatus struct {
}

//+kubebuilder:object:root=true
//+kubebuilder:resource:shortName=rsc
//+kubebuilder:subresource:status

// ReleaseServiceConfig is the Schema for the releaseserviceconfigs API
type ReleaseServiceConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ReleaseServiceConfigSpec   `json:"spec,omitempty"`
	Status ReleaseServiceConfigStatus `json:"status,omitempty"`
}

// IsPipelineOverridden checks whether there is a EmptyDirOverride matching the given url, revision and pathInRepo.
func (rsc *ReleaseServiceConfig) IsPipelineOverridden(url, revision, pathInRepo string) bool {
	for _, override := range rsc.Spec.EmptyDirOverrides {
		urlRegex, err := regexp.Compile(override.Url)
		if err != nil || !urlRegex.MatchString(url) {
			continue
		}

		revisionRegex, err := regexp.Compile(override.Revision)
		if err != nil || !revisionRegex.MatchString(revision) {
			continue
		}

		if override.PathInRepo == pathInRepo {
			return true
		}
	}

	return false
}

//+kubebuilder:object:root=true

// ReleaseServiceConfigList contains a list of ReleaseServiceConfig
type ReleaseServiceConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ReleaseServiceConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ReleaseServiceConfig{}, &ReleaseServiceConfigList{})
}
