package v1alpha1

import "github.com/konflux-ci/operator-toolkit/conditions"

const (
	// matchedConditionType is the type used to track the status of the ReleasePlan being matched to a
	// ReleasePlanAdmission or vice versa
	MatchedConditionType conditions.ConditionType = "Matched"
)

const (
	// MatchedReason is the reason set when a resource is matched
	MatchedReason conditions.ConditionReason = "Matched"
)
