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

package conditions

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ConditionReason is a string representing a Kubernetes condition reason.
type ConditionReason string

func (cr ConditionReason) String() string {
	return string(cr)
}

// ConditionType is a string representing a Kubernetes condition type.
type ConditionType string

// String returns a string representation of the ConditionType.
func (ct ConditionType) String() string {
	return string(ct)
}

// SetCondition creates a new condition with the given conditionType, status and reason. Then, it sets this new condition,
// unsetting previous conditions with the same type as necessary.
func SetCondition(conditions *[]metav1.Condition, conditionType ConditionType, status metav1.ConditionStatus, reason ConditionReason) {
	SetConditionWithMessage(conditions, conditionType, status, reason, "")
}

// SetConditionWithMessage creates a new condition with the given conditionType, status, reason and message. Then, it sets this new condition,
// unsetting previous conditions with the same type as necessary.
func SetConditionWithMessage(conditions *[]metav1.Condition, conditionType ConditionType, status metav1.ConditionStatus, reason ConditionReason, message string) {
	meta.SetStatusCondition(conditions, metav1.Condition{
		Type:    conditionType.String(),
		Status:  status,
		Reason:  reason.String(),
		Message: message,
	})
}
