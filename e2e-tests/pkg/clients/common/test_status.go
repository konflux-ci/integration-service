package common

import (
	appstudioApi "github.com/konflux-ci/application-api/api/v1alpha1"
	"k8s.io/apimachinery/pkg/api/meta"
)

func (s *SuiteController) HaveTestsSucceeded(snapshot *appstudioApi.Snapshot) bool {
	return meta.IsStatusConditionTrue(snapshot.Status.Conditions, "HACBSTestSucceeded") ||
		meta.IsStatusConditionTrue(snapshot.Status.Conditions, "AppStudioTestSucceeded")
}
