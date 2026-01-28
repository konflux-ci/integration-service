package v1alpha1

const (
	DefaultReleaseCatalogUrl      = "https://github.com/konflux-ci/release-service-catalog.git"
	DefaultReleaseCatalogRevision = "production"
	DefaultCollectorPipelinePath  = "pipelines/run-collectors/run-collectors.yaml"
)

// Collectors holds the list of collectors to be executed as part of the release workflow along with the
// ServiceAccount to be used in the PipelineRun.
// +kubebuilder:object:generate=true
type Collectors struct {
	// Items is the list of Collectors to be executed as part of the release workflow
	// +required
	Items []CollectorItem `json:"items"`

	// Secrets is the list of secrets to be used in the Collector's Pipeline
	// +optional
	Secrets []string `json:"secrets,omitempty"`

	// ServiceAccountName is the ServiceAccount to use during the execution of the Collectors Pipeline
	// +kubebuilder:validation:Pattern=^[a-z0-9]([-a-z0-9]*[a-z0-9])?$
	// +optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`
}

// CollectorItem represents all the information about an specific collector which will be executed in the
// CollectorsPipeline.
type CollectorItem struct {
	// Name of the collector
	// +kubebuilder:validation:Pattern=^[a-z0-9]([-a-z0-9]*[a-z0-9])?$
	// +required
	Name string `json:"name"`

	// Timeout in seconds for the collector to execute
	// +optional
	Timeout int `json:"timeout,omitempty"`

	// Type is the type of collector to be used
	// +kubebuilder:validation:Pattern=^[a-z0-9]([-a-z0-9]*[a-z0-9])?$
	// +required
	Type string `json:"type"`

	// Params is a slice of parameters for a given collector
	Params []Param `json:"params"`
}

// Param represents a parameter for a collector
type Param struct {
	// Name is the name of the parameter
	Name string `json:"name"`

	// Value is the value of the parameter
	Value string `json:"value"`
}
