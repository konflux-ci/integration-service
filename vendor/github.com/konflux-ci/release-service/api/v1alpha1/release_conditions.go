package v1alpha1

import "github.com/konflux-ci/operator-toolkit/conditions"

const (
	// deployedConditionType is the type used to track the status of a Release deployment
	deployedConditionType conditions.ConditionType = "Deployed"

	// postActionsExecutedConditionType is the type used to track the status of Release post-actions
	postActionsExecutedConditionType conditions.ConditionType = "PostActionsExecuted"

	// processedConditionType is the type used to track the status of a Release processing
	processedConditionType conditions.ConditionType = "Processed"

	// releasedConditionType is the type used to track the status of a Release
	releasedConditionType conditions.ConditionType = "Released"

	// validatedConditionType is the type used to track the status of a Release validation
	validatedConditionType conditions.ConditionType = "Validated"
)

const (
	// FailedReason is the reason set when a failure occurs
	FailedReason conditions.ConditionReason = "Failed"

	// ProgressingReason is the reason set when an action is progressing
	ProgressingReason conditions.ConditionReason = "Progressing"

	// SucceededReason is the reason set when an action succeeds
	SucceededReason conditions.ConditionReason = "Succeeded"
)
