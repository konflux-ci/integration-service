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

package gitops

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/konflux-ci/integration-service/pkg/keys"
	"github.com/konflux-ci/operator-toolkit/metadata"
)

// LabelValue reads key from obj, preferring the new owned key and falling back to the old one.
func LabelValue(obj metav1.Object, key string) (string, bool) {
	return keys.ReadLabel(obj, key)
}

// AnnotationValue reads key from obj, preferring the new owned key and falling back to the old one.
func AnnotationValue(obj metav1.Object, key string) (string, bool) {
	return keys.ReadAnnotation(obj, key)
}

// HasOwnedLabel reports whether key is set under either owned name.
func HasOwnedLabel(obj metav1.Object, key string) bool {
	_, found := LabelValue(obj, key)
	return found
}

// HasOwnedAnnotation reports whether key is set under either owned name.
func HasOwnedAnnotation(obj metav1.Object, key string) bool {
	_, found := AnnotationValue(obj, key)
	return found
}

// HasOwnedLabelValue reports whether key is set to value under either owned name.
func HasOwnedLabelValue(obj metav1.Object, key, value string) bool {
	got, found := LabelValue(obj, key)
	return found && got == value
}

// HasOwnedAnnotationValue reports whether key is set to value under either owned name.
func HasOwnedAnnotationValue(obj metav1.Object, key, value string) bool {
	got, found := AnnotationValue(obj, key)
	return found && got == value
}

// SetOwnedAnnotation writes key in the object's style and removes the other owned key.
func SetOwnedAnnotation(obj metav1.Object, key, value string) error {
	return SetOwnedAnnotationWithStyle(obj, key, value, keys.StyleOf(obj))
}

// SetOwnedAnnotationWithStyle writes key using style and removes the other owned key.
func SetOwnedAnnotationWithStyle(obj metav1.Object, key, value string, style keys.Style) error {
	pair, ok := keys.PairForKey(key)
	if !ok {
		return metadata.SetAnnotation(obj, key, value)
	}
	return keys.SetAnnotation(obj, pair, value, style)
}

// SetOwnedLabel writes key in the object's style and removes the other owned key.
func SetOwnedLabel(obj metav1.Object, key, value string) error {
	return SetOwnedLabelWithStyle(obj, key, value, keys.StyleOf(obj))
}

// SetOwnedLabelWithStyle writes key using style and removes the other owned key.
func SetOwnedLabelWithStyle(obj metav1.Object, key, value string, style keys.Style) error {
	pair, ok := keys.PairForKey(key)
	if !ok {
		return metadata.SetLabel(obj, key, value)
	}
	return keys.SetLabel(obj, pair, value, style)
}

// DeleteOwnedLabel removes key and the other name of the same owned pair.
func DeleteOwnedLabel(obj metav1.Object, key string) error {
	pair, ok := keys.PairForKey(key)
	if !ok {
		if err := metadata.DeleteLabel(obj, key); err != nil {
			return fmt.Errorf("failed to delete label %s: %w", key, err)
		}
		return nil
	}
	if err := metadata.DeleteLabel(obj, pair.Old); err != nil {
		return fmt.Errorf("failed to delete old owned label %s for pair %s/%s: %w", pair.Old, pair.Old, pair.New, err)
	}
	if pair.New != "" && pair.New != pair.Old {
		if err := metadata.DeleteLabel(obj, pair.New); err != nil {
			return fmt.Errorf("failed to delete new owned label %s for pair %s/%s: %w", pair.New, pair.Old, pair.New, err)
		}
	}
	return nil
}

// DeleteOwnedAnnotation removes key and the other name of the same owned pair.
func DeleteOwnedAnnotation(obj metav1.Object, key string) error {
	pair, ok := keys.PairForKey(key)
	if !ok {
		return metadata.DeleteAnnotation(obj, key)
	}
	if err := metadata.DeleteAnnotation(obj, pair.Old); err != nil {
		return err
	}
	if pair.New != "" && pair.New != pair.Old {
		return metadata.DeleteAnnotation(obj, pair.New)
	}
	return nil
}
