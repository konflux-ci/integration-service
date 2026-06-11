/*
Copyright 2022.

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

package metadata

import (
	"errors"
	"strings"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AddAnnotations copies the map into the resource's Annotations map.
// When the destination map is nil, then the map will be created.
// The unexported function addEntries is called with args passed.
func AddAnnotations(obj v1.Object, entries map[string]string) error {
	if obj == nil {
		return errors.New("object cannot be nil")
	}

	if obj.GetAnnotations() == nil {
		obj.SetAnnotations(map[string]string{})
	}
	addEntries(entries, obj.GetAnnotations())

	return nil
}

// AddLabels copies the map into the resource's Labels map.
// When the destination map is nil, then the map will be created.
// The unexported function addEntries is called with args passed.
func AddLabels(obj v1.Object, entries map[string]string) error {
	if obj == nil {
		return errors.New("object cannot be nil")
	}

	if obj.GetLabels() == nil {
		obj.SetLabels(map[string]string{})
	}
	addEntries(entries, obj.GetLabels())

	return nil
}

// CopyAnnotationsByPrefix copies all annotations from a source object to a destination object where the key matches
// the specified sourcePrefix.
func CopyAnnotationsByPrefix(source, destination v1.Object, prefix string) error {
	if source == nil || destination == nil {
		return errors.New("object cannot be nil")
	}

	if destination.GetAnnotations() == nil {
		destination.SetAnnotations(make(map[string]string))
	}
	copyByPrefix(source.GetAnnotations(), destination.GetAnnotations(), prefix)

	return nil
}

// CopyAnnotationsWithPrefixReplacement copies all annotations from a source object to a destination object where the
// key matches the specified sourcePrefix. The source prefix will be replaced with the destination prefix.
func CopyAnnotationsWithPrefixReplacement(source, destination v1.Object, sourcePrefix, destinationPrefix string) error {
	if source == nil || destination == nil {
		return errors.New("object cannot be nil")
	}

	if destination.GetAnnotations() == nil {
		destination.SetAnnotations(make(map[string]string))
	}

	copyWithPrefixReplacement(source.GetAnnotations(), destination.GetAnnotations(), sourcePrefix, destinationPrefix)

	return nil
}

// CopyLabelsByPrefix copies all labels from a source object to a destination object where the key matches the
// specified sourcePrefix.
func CopyLabelsByPrefix(source, destination v1.Object, prefix string) error {
	if source == nil || destination == nil {
		return errors.New("object cannot be nil")
	}

	if destination.GetLabels() == nil {
		destination.SetLabels(make(map[string]string))
	}

	copyByPrefix(source.GetLabels(), destination.GetLabels(), prefix)

	return nil
}

// CopyLabelsWithPrefixReplacement copies all labels from a source object to a destination object where the key matches
// the specified sourcePrefix. If destinationPrefix is different from sourcePrefix, the sourcePrefix will be replaced
// while performing the copy.
func CopyLabelsWithPrefixReplacement(source, destination v1.Object, sourcePrefix, destinationPrefix string) error {
	if source == nil || destination == nil {
		return errors.New("object cannot be nil")
	}

	if destination.GetLabels() == nil {
		destination.SetLabels(make(map[string]string))
	}

	copyWithPrefixReplacement(source.GetLabels(), destination.GetLabels(), sourcePrefix, destinationPrefix)

	return nil
}

// DeleteAnnotation deletes the annotation specified by name from the referenced object.
// If the annotation doesn't exist it's a no-op.
func DeleteAnnotation(obj v1.Object, key string) error {
	if obj == nil {
		return errors.New("object cannot be nil")
	}

	annotations := obj.GetAnnotations()
	if annotations == nil {
		return nil // nothing to delete
	}

	delete(annotations, key)

	return nil
}

// DeleteLabel deletes the label specified by name from the referenced object.
// If the label doesn't exist it's a no-op.
func DeleteLabel(obj v1.Object, key string) error {
	if obj == nil {
		return errors.New("object cannot be nil")
	}

	labels := obj.GetLabels()
	if labels == nil {
		return nil // nothing to delete
	}

	delete(labels, key)

	return nil
}

// GetAnnotationsWithPrefix is a method that returns a map of key/value pairs matching a prefix string.
// The unexported function filterByPrefix is called with args passed.
func GetAnnotationsWithPrefix(obj v1.Object, prefix string) (map[string]string, error) {
	if obj == nil {
		return map[string]string{}, errors.New("object cannot be nil")
	}

	return filterByPrefix(obj.GetAnnotations(), prefix), nil
}

// GetLabelsWithPrefix is a method that returns a map of key/value pairs matching a prefix string.
// The unexported function filterByPrefix is called with args passed.
func GetLabelsWithPrefix(obj v1.Object, prefix string) (map[string]string, error) {
	if obj == nil {
		return map[string]string{}, errors.New("object cannot be nil")
	}

	return filterByPrefix(obj.GetLabels(), prefix), nil
}

// HasAnnotation checks whether a given annotation exists or not.
func HasAnnotation(obj v1.Object, key string) bool {
	_, ok := obj.GetAnnotations()[key]
	return ok
}

// HasAnnotationWithValue checks if an annotation exists and has the given value.
func HasAnnotationWithValue(obj v1.Object, key, value string) bool {
	val, ok := obj.GetAnnotations()[key]
	return ok && val == value
}

// HasLabel checks whether a given Label exists or not.
func HasLabel(obj v1.Object, key string) bool {
	_, ok := obj.GetLabels()[key]
	return ok
}

// HasLabelWithValue checks if a label exists and has the given value.
func HasLabelWithValue(obj v1.Object, key, value string) bool {
	val, ok := obj.GetLabels()[key]
	return ok && val == value
}

// SetAnnotation adds a new annotation to the referenced object or updates its value if it already exists.
func SetAnnotation(obj v1.Object, key string, value string) error {
	if obj == nil {
		return errors.New("object cannot be nil")
	}

	if annotations := obj.GetAnnotations(); annotations == nil {
		obj.SetAnnotations(map[string]string{key: value})
	} else {
		annotations[key] = value
	}

	return nil
}

// SetLabel adds a new label to the referenced object or updates its value if it already exists.
func SetLabel(obj v1.Object, key string, value string) error {
	if obj == nil {
		return errors.New("object cannot be nil")
	}

	if labels := obj.GetLabels(); labels == nil {
		obj.SetLabels(map[string]string{key: value})
	} else {
		labels[key] = value
	}

	return nil
}

// addEntries copies key/value pairs in the source map adding them into the destination map.
// The unexported function safeCopy is used to copy, and avoids clobbering existing keys in the destination map.
func addEntries(source, destination map[string]string) {
	for key, val := range source {
		safeCopy(destination, key, val)
	}
}

// copyByPrefix copies key/value pairs from a source map to a destination map where the key matches the specified prefix.
func copyByPrefix(source, destination map[string]string, prefix string) {
	copyWithPrefixReplacement(source, destination, prefix, prefix)
}

// copyWithPrefixReplacement copies key/value pairs from a source map to a destination map where the key matches the
// specified sourcePrefix. The source prefix will be replaced with the destination prefix.
func copyWithPrefixReplacement(source, destination map[string]string, sourcePrefix, destinationPrefix string) {
	for key, value := range source {
		if strings.HasPrefix(key, sourcePrefix) {
			newKey := key
			if sourcePrefix != destinationPrefix {
				newKey = strings.Replace(key, sourcePrefix, destinationPrefix, 1)
			}
			destination[newKey] = value
		}
	}
}

// filterByPrefix returns a map of key/value pairs contained in src that matches the prefix.
// When the prefix is empty/nil, the source map is returned.
// When source key does not contain the prefix string, no copy happens.
func filterByPrefix(entries map[string]string, prefix string) map[string]string {
	if len(prefix) == 0 {
		return entries
	}
	dst := map[string]string{}
	for key, val := range entries {
		if strings.HasPrefix(key, prefix) {
			dst[key] = val
		}
	}
	return dst
}

// safeCopy conditionally copies a given key/value pair into a map.
// When a key is already present in the map, no copy happens.
func safeCopy(destination map[string]string, key, val string) {
	if _, err := destination[key]; !err {
		destination[key] = val
	}
}
