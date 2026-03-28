package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RetryablePipeline is a pipeline that is safe to automatically retry on failure.
type RetryablePipeline struct {
	// Url is the url to the git repo.
	// +required
	Url string `json:"url"`

	// Revision is the git revision where the Pipeline definition can be found.
	// +required
	Revision string `json:"revision"`

	// PathInRepo is the path within the git repository where the Pipeline definition
	// can be found.
	// +required
	PathInRepo string `json:"pathInRepo"`

	// RetryPolicy defines how and when the pipeline is retried after a failure.
	// +required
	RetryPolicy RetryPolicy `json:"retryPolicy"`
}

// RetryPolicy defines how and when the pipeline is retried after a failure.
type RetryPolicy struct {
	// MaxRetries is the maximum number of retries allowed for a pipeline.
	// +kubebuilder:validation:Minimum=0
	// +required
	MaxRetries int `json:"maxRetries"`

	// DisableOn defines conditions that disable automatic retries.
	// +optional
	DisableOn *DisableConditions `json:"disableOn,omitempty"`

	// Mitigations defines adjustment strategies to apply on retry based on failure type
	// +optional
	Mitigations *Mitigations `json:"mitigations,omitempty"`
}

// DisableConditions defines conditions that disable automatic retries.
type DisableConditions struct {
	// Tags is a list of values which disable retries when present in the ReleasePlanAdmission
	// mapping data.
	// +optional
	Tags []string `json:"tags,omitempty"`
}

// Mitigations defines adjustment strategies for different failure types.
type Mitigations struct {
	// OOMKill defines adjustment for out-of-memory failures
	// +optional
	OOMKill *MemoryMitigation `json:"oomKill,omitempty"`

	// Timeout defines adjustment for timeout failures.
	// +optional
	Timeout *TimeoutMitigation `json:"timeout,omitempty"`
}

// MemoryMitigation defines how memory limits should be adjusted on a retry.
type MemoryMitigation struct {
	// Multiplier is a factor used to multiply memory limits on each retry
	// +kubebuilder:validation:Pattern=^[0-9]+(\.[0-9]+)?$
	// +required
	Multiplier string `json:"multiplier"`

	// MaxComputeResources sets a upper limit on resources across retries.
	// +optional
	MaxComputeResources *corev1.ResourceRequirements `json:"maxComputeResources,omitempty"`
}

// TimeoutMitigation defines how timeout limits should be adjusted on a retry.
type TimeoutMitigation struct {
	// Increment is the duration to add to the timeout on each retry.
	// +required
	Increment metav1.Duration `json:"increment"`

	// MaxTimeout is the maximum time allowed across retries.
	// +optional
	MaxTimeout *metav1.Duration `json:"maxTimeout,omitempty"`
}
