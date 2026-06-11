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
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"strings"
)

// AddAnnotations copies the map into the resource's Annotations map.
// When the destination map is nil, then the map will be created.
// The unexported function addEntries is called with args passed.
func AddAnnotations(obj v1.Object, entries map[string]string) {
	if obj.GetAnnotations() == nil {
		obj.SetAnnotations(map[string]string{})
	}
	addEntries(entries, obj.GetAnnotations())
}

// AddLabels copies the map into the resource's Labels map.
// When the destination map is nil, then the map will be created.
// The unexported function addEntries is called with args passed.
func AddLabels(obj v1.Object, entries map[string]string) {
	if obj.GetLabels() == nil {
		obj.SetLabels(map[string]string{})
	}
	addEntries(entries, obj.GetLabels())
}

// GetAnnotationsWithPrefix is a method that returns a map of key/value pairs matching a prefix string.
// The unexported function filterByPrefix is called with args passed.
func GetAnnotationsWithPrefix(obj v1.Object, prefix string) map[string]string {
	return filterByPrefix(obj.GetAnnotations(), prefix)
}

// GetLabelsWithPrefix is a method that returns a map of key/value pairs matching a prefix string.
// The unexported function filterByPrefix is called with args passed.
func GetLabelsWithPrefix(obj v1.Object, prefix string) map[string]string {
	return filterByPrefix(obj.GetLabels(), prefix)
}

// addEntries copies key/value pairs in the source map adding them into the destination map.
// The unexported function safeCopy is used to copy, and avoids clobbering existing keys in the destination map.
func addEntries(source, destination map[string]string) {
	for key, val := range source {
		safeCopy(destination, key, val)
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
func safeCopy(dst map[string]string, key, val string) {
	if _, err := dst[key]; !err {
		dst[key] = val
	}
}
