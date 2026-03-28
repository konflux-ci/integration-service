package v1alpha1

import "github.com/konflux-ci/operator-toolkit/conditions"

const (
	// finalProcessedConditionType is the type used to track the status of a Release Final Pipeline processing
	finalProcessedConditionType conditions.ConditionType = "FinalPipelineProcessed"

	// managedCollectorsProcessedConditionType is the type used to track the status of a Release Managed Collectors Pipeline processing
	managedCollectorsProcessedConditionType conditions.ConditionType = "ManagedCollectorsPipelineProcessed"

	// managedProcessedConditionType is the type used to track the status of a Release Managed Pipeline processing
	managedProcessedConditionType conditions.ConditionType = "ManagedPipelineProcessed"

	// tenantCollectorsProcessedConditionType is the type used to track the status of a Release Tenant Collectors Pipeline processing
	tenantCollectorsProcessedConditionType conditions.ConditionType = "TenantCollectorsPipelineProcessed"

	// tenantProcessedConditionType is the type used to track the status of a Release Tenant Pipeline processing
	tenantProcessedConditionType conditions.ConditionType = "TenantPipelineProcessed"

	// releasedConditionType is the type used to track the status of a Release
	releasedConditionType conditions.ConditionType = "Released"

	// validatedConditionType is the type used to track the status of a Release validation
	validatedConditionType conditions.ConditionType = "Validated"
)

const (
	// AttemptSucceededReason is the reason set when an attempt succeeds
	AttemptSucceededReason = "Succeeded"
	// AttemptFailedReason is the reason set when an attempt fails
	AttemptFailedReason = "Failed"
	// AttemptProgressingReason is the reason set when an attempt is progressing
	AttemptProgressingReason = "Progressing"
	// AttemptFailureErrorReason is the reason set when an attempt fails due to an error
	AttemptFailureErrorReason = "Error"
	// AttemptFailureOOMKillReason is the reason set when an attempt fails due to OOM kill
	AttemptFailureOOMKillReason = "OOMKill"
	// AttemptFailureTimeoutReason is the reason set when an attempt fails due to timeout
	AttemptFailureTimeoutReason = "Timeout"
)

const (
	// FailedReason is the reason set when a failure occurs
	FailedReason conditions.ConditionReason = "Failed"

	// ProgressingReason is the reason set when a phase is progressing
	ProgressingReason conditions.ConditionReason = "Progressing"

	// SkippedReason is the reason set when a phase is skipped
	SkippedReason conditions.ConditionReason = "Skipped"

	// SucceededReason is the reason set when a phase succeeds
	SucceededReason conditions.ConditionReason = "Succeeded"
)
