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

package keys

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GetLabel returns the value for pair, preferring the new key and falling back to the old key.
func GetLabel(obj metav1.Object, pair Pair) (string, bool) {
	if obj == nil {
		return "", false
	}
	return getFrom(obj.GetLabels(), pair)
}

// GetAnnotation returns the value for pair, preferring the new key and falling back to the old key.
func GetAnnotation(obj metav1.Object, pair Pair) (string, bool) {
	if obj == nil {
		return "", false
	}
	return getFrom(obj.GetAnnotations(), pair)
}

// HasLabelValue is true when GetLabel finds pair and the value matches.
func HasLabelValue(obj metav1.Object, pair Pair, value string) bool {
	got, found := GetLabel(obj, pair)
	return found && got == value
}

// HasNew is true when any pair has its new key set as a label or annotation.
func HasNew(obj metav1.Object, pairs ...Pair) bool {
	if obj == nil {
		return false
	}
	labels := obj.GetLabels()
	annotations := obj.GetAnnotations()
	for _, pair := range pairs {
		if pair.New == "" {
			continue
		}
		if _, ok := labels[pair.New]; ok {
			return true
		}
		if _, ok := annotations[pair.New]; ok {
			return true
		}
	}
	return false
}

// SetLabel writes pair on obj using exactly one style and removes the other key.
func SetLabel(obj metav1.Object, pair Pair, value string, style Style) error {
	if obj == nil {
		return fmt.Errorf("object cannot be nil")
	}
	updated, err := setIn(obj.GetLabels(), pair, value, style)
	if err != nil {
		return fmt.Errorf("set label: %w", err)
	}
	obj.SetLabels(updated)
	return nil
}

// SetAnnotation writes pair on obj using exactly one style and removes the other key.
func SetAnnotation(obj metav1.Object, pair Pair, value string, style Style) error {
	if obj == nil {
		return fmt.Errorf("object cannot be nil")
	}
	updated, err := setIn(obj.GetAnnotations(), pair, value, style)
	if err != nil {
		return fmt.Errorf("set annotation: %w", err)
	}
	obj.SetAnnotations(updated)
	return nil
}

// getFrom returns the value for pair from entries, preferring New over Old.
func getFrom(entries map[string]string, pair Pair) (string, bool) {
	if entries == nil {
		return "", false
	}
	if pair.New != "" {
		if value, ok := entries[pair.New]; ok {
			return value, true
		}
	}
	if pair.Old == "" {
		return "", false
	}
	value, ok := entries[pair.Old]
	return value, ok
}

// setIn writes pair on entries using exactly one style and removes the other key.
func setIn(entries map[string]string, pair Pair, value string, style Style) (map[string]string, error) {
	key, err := pair.Key(style)
	if err != nil {
		return entries, fmt.Errorf("select key: %w", err)
	}
	if entries == nil {
		entries = map[string]string{}
	}
	if other := pair.otherKey(style); other != "" && other != key {
		delete(entries, other)
	}
	entries[key] = value
	return entries, nil
}
