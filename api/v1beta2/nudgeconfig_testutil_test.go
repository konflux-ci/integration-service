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

package v1beta2

import (
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func durationPtr(d time.Duration) *metav1.Duration {
	return &metav1.Duration{Duration: d}
}

func failurePolicyPtr(p FailurePolicyType) *FailurePolicyType {
	return &p
}

func kubernetesDurationLiteral(d time.Duration) (string, error) {
	switch {
	case d%time.Hour == 0:
		return fmt.Sprintf("%dh", d/time.Hour), nil
	case d%time.Minute == 0:
		return fmt.Sprintf("%dm", d/time.Minute), nil
	case d%time.Second == 0:
		return fmt.Sprintf("%ds", d/time.Second), nil
	default:
		return "", fmt.Errorf("duration %s has no CEL-friendly hour/minute/second literal", d)
	}
}
