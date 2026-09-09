/*
Copyright 2026 Red Hat Inc.

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

package helpers

import (
	"fmt"
	"strings"

	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	"github.com/konflux-ci/integration-service/api/v1beta2"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// StaleReferencesStatusCondition is the status condition indicating if a given NudgeConfig has stale Component references
	StaleReferencesStatusCondition = "StaleReferences"
	// StaleReferencesDetectedReason is the status condition reason indicating that stale references were detected in the NudgeConfig
	StaleReferencesDetectedReason = "StaleReferencesDetected"
	// NoStaleReferencesReason is the status condition reason indicating that no stale references were detected in the NudgeConfig
	NoStaleReferencesReason = "NoStaleReferences"

	// BatchDefaultsSupportedStatusCondition indicates whether spec.batchDefaults is applied by integration-service.
	BatchDefaultsSupportedStatusCondition = "BatchDefaultsSupported"
	// BatchDefaultsNotImplementedReason indicates batchDefaults is set but not yet enforced.
	BatchDefaultsNotImplementedReason = "NotImplemented"
	// BatchDefaultsNotImplementedMessage is used in status and admission warnings when batchDefaults is set but not enforced.
	BatchDefaultsNotImplementedMessage = "spec.batchDefaults is reserved; integration-service does not apply these defaults yet."
)

// FindMissingNudgeConfigReferences returns a boolean indicating any missing nudgeConfig references and an accompanying message
// for a given list of components and nudge relationships
func FindMissingNudgeConfigReferences(components []applicationapiv1alpha1.Component, nudges []v1beta2.NudgeRelationship, namespace string) (bool, string) {
	existing := make(map[string]struct{}, len(components))
	for i := range components {
		existing[components[i].Name] = struct{}{}
	}

	var missingFrom, missingTo []string
	seenFrom, seenTo := map[string]struct{}{}, map[string]struct{}{}
	for _, n := range nudges {
		if _, ok := existing[n.From]; !ok {
			if _, dup := seenFrom[n.From]; !dup {
				seenFrom[n.From] = struct{}{}
				missingFrom = append(missingFrom, n.From)
			}
		}
		if _, ok := existing[n.To]; !ok {
			if _, dup := seenTo[n.To]; !dup {
				seenTo[n.To] = struct{}{}
				missingTo = append(missingTo, n.To)
			}
		}
	}

	if len(missingFrom) == 0 && len(missingTo) == 0 {
		return false, ""
	}

	msg := fmt.Sprintf("NudgeConfig references non-existent Component(s) in namespace %q", namespace)
	if len(missingFrom) > 0 {
		msg += fmt.Sprintf("; missing 'from' component(s): %s", strings.Join(missingFrom, ", "))
	}
	if len(missingTo) > 0 {
		msg += fmt.Sprintf("; missing 'to' component(s): %s", strings.Join(missingTo, ", "))
	}
	return true, msg
}

// ApplyBatchDefaultsSupportedStatusCondition updates status.conditions for spec.batchDefaults.
// It returns true when conditions were modified and a status patch is required.
func ApplyBatchDefaultsSupportedStatusCondition(conditions *[]metav1.Condition, batchDefaults *v1beta2.BatchDefaults) bool {
	if batchDefaults == nil {
		return meta.RemoveStatusCondition(conditions, BatchDefaultsSupportedStatusCondition)
	}

	existing := meta.FindStatusCondition(*conditions, BatchDefaultsSupportedStatusCondition)

	desired := metav1.Condition{
		Type:    BatchDefaultsSupportedStatusCondition,
		Status:  metav1.ConditionFalse,
		Reason:  BatchDefaultsNotImplementedReason,
		Message: BatchDefaultsNotImplementedMessage,
	}
	if existing != nil &&
		existing.Status == desired.Status &&
		existing.Reason == desired.Reason &&
		existing.Message == desired.Message {
		return false
	}

	meta.SetStatusCondition(conditions, desired)
	return true
}
