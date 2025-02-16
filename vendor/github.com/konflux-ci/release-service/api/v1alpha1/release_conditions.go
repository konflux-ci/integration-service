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
	// FailedReason is the reason set when a failure occurs
	FailedReason conditions.ConditionReason = "Failed"

	// ProgressingReason is the reason set when a phase is progressing
	ProgressingReason conditions.ConditionReason = "Progressing"

	// SkippedReason is the reason set when a phase is skipped
	SkippedReason conditions.ConditionReason = "Skipped"

	// SucceededReason is the reason set when a phase succeeds
	SucceededReason conditions.ConditionReason = "Succeeded"
)
