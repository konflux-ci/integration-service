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
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const newSnapshotGroup = "konflux-ci.dev"

// Test returns the integration-service test label or annotation pair for name.
// Old: test.appstudio.openshift.io/<name>. New: integration.konflux-ci.dev/<name>.
func Test(name string) Pair {
	return Pair{Old: PrefixTestOld + "/" + name, New: PrefixTestNew + "/" + name}
}

// AutoRelease returns the auto-release pair integration-service stamps on Snapshots.
func AutoRelease() Pair {
	return Pair{Old: PrefixRelease + "/auto-release", New: PrefixReleaseNew + "/auto-release"}
}

// ComponentGroupName returns the component-group name pair stamped on Snapshots
// and integration PipelineRuns.
func ComponentGroupName() Pair {
	return Pair{Old: PrefixAppstudio + "/component-group", New: PrefixTestNew + "/component-group"}
}

// SnapshotName returns the snapshot name pair stamped on integration PipelineRuns.
func SnapshotName() Pair {
	return Pair{Old: PrefixAppstudio + "/snapshot", New: PrefixTestNew + "/snapshot"}
}

// BuildPipelineRunName returns the build PipelineRun name pair stamped on Snapshots.
func BuildPipelineRunName() Pair {
	return Pair{Old: PrefixAppstudio + "/build-pipelinerun", New: PrefixTestNew + "/build-pipelinerun"}
}

// PAC returns the Pipelines-as-Code destination pair copied onto Snapshots.
func PAC(name string) Pair {
	return Pair{Old: PrefixPAC + "/" + name, New: PrefixPACNew + "/" + name}
}

// Same returns a pair whose old and new keys are identical. ListEither then
// requires that key in both style queries.
func Same(key string) Pair {
	return Pair{Old: key, New: key}
}

// StyleOf reports which key family obj should be written with.
// A konflux-ci.dev object, or one that already carries a new owned key, is new.
// Everything else stays old, so existing App Studio objects keep their keys.
func StyleOf(obj metav1.Object) Style {
	if obj == nil {
		return StyleOld
	}
	if ro, ok := obj.(runtime.Object); ok {
		if ro.GetObjectKind().GroupVersionKind().Group == newSnapshotGroup {
			return StyleNew
		}
	}
	if hasNewOwnedKey(obj) {
		return StyleNew
	}
	return StyleOld
}

// PairForKey returns the owned pair for key when key is one of its names.
func PairForKey(key string) (Pair, bool) {
	switch key {
	case AutoRelease().Old, AutoRelease().New:
		return AutoRelease(), true
	case ComponentGroupName().Old, ComponentGroupName().New:
		return ComponentGroupName(), true
	case SnapshotName().Old, SnapshotName().New:
		return SnapshotName(), true
	case BuildPipelineRunName().Old, BuildPipelineRunName().New:
		return BuildPipelineRunName(), true
	case BuildComponent().Old, BuildComponent().New:
		return BuildComponent(), true
	case PipelineType().Old, PipelineType().New:
		return PipelineType(), true
	}
	if suffix, ok := cutOwnedPrefix(key, PrefixPAC+"/"); ok {
		return PAC(suffix), true
	}
	if suffix, ok := cutOwnedPrefix(key, PrefixPACNew+"/"); ok {
		return PAC(suffix), true
	}
	if suffix, ok := cutOwnedPrefix(key, PrefixTestOld+"/"); ok {
		return Test(suffix), true
	}
	if suffix, ok := cutOwnedPrefix(key, PrefixTestNew+"/"); ok {
		return Test(suffix), true
	}
	return Pair{}, false
}

// ReadLabel returns the value for key, preferring the new owned key.
func ReadLabel(obj metav1.Object, key string) (string, bool) {
	if obj == nil {
		return "", false
	}
	if pair, ok := PairForKey(key); ok {
		return GetLabel(obj, pair)
	}
	value, found := obj.GetLabels()[key]
	return value, found
}

// ReadAnnotation returns the value for key, preferring the new owned key.
func ReadAnnotation(obj metav1.Object, key string) (string, bool) {
	if obj == nil {
		return "", false
	}
	if pair, ok := PairForKey(key); ok {
		return GetAnnotation(obj, pair)
	}
	value, found := obj.GetAnnotations()[key]
	return value, found
}

// ApplyAnnotation writes key on obj in StyleOf(obj) and removes the other owned key.
// Keys that are not owned are written as given.
func ApplyAnnotation(obj metav1.Object, key, value string) error {
	if obj == nil {
		return fmt.Errorf("object cannot be nil")
	}
	pair, ok := PairForKey(key)
	if !ok {
		return setRawAnnotation(obj, key, value)
	}
	return SetAnnotation(obj, pair, value, StyleOf(obj))
}

// ApplyAnnotationWith writes key using style and removes the other owned key.
func ApplyAnnotationWith(obj metav1.Object, key, value string, style Style) error {
	if obj == nil {
		return fmt.Errorf("object cannot be nil")
	}
	pair, ok := PairForKey(key)
	if !ok {
		return setRawAnnotation(obj, key, value)
	}
	return SetAnnotation(obj, pair, value, style)
}

// HasFinalizer reports whether obj has either name of an owned finalizer.
// Names that are not owned are matched exactly.
func HasFinalizer(obj metav1.Object, name string) bool {
	if obj == nil {
		return false
	}
	for _, candidate := range finalizerNames(name) {
		for _, current := range obj.GetFinalizers() {
			if current == candidate {
				return true
			}
		}
	}
	return false
}

// FinalizerToAdd returns the finalizer name that matches StyleOf(obj).
func FinalizerToAdd(obj metav1.Object, name string) (string, error) {
	pair, ok := finalizerPair(name)
	if !ok {
		return name, nil
	}
	return pair.Key(StyleOf(obj))
}

// FinalizerNames returns both names when name belongs to an owned finalizer.
func FinalizerNames(name string) []string {
	return finalizerNames(name)
}

func finalizerNames(name string) []string {
	pair, ok := finalizerPair(name)
	if !ok {
		return []string{name}
	}
	names := make([]string, 0, 2)
	if pair.Old != "" {
		names = append(names, pair.Old)
	}
	if pair.New != "" && pair.New != pair.Old {
		names = append(names, pair.New)
	}
	return names
}

func finalizerPair(name string) (Pair, bool) {
	for _, pair := range ownedFinalizers() {
		if name == pair.Old || name == pair.New {
			return pair, true
		}
	}
	return Pair{}, false
}

func ownedFinalizers() []Pair {
	return []Pair{
		Test("pipelinerun"),
		Test("nudge-pipelinerun"),
		Test("scenario"),
		Test("component"),
	}
}

func hasNewOwnedKey(obj metav1.Object) bool {
	if obj == nil {
		return false
	}
	if entriesHaveNew(obj.GetLabels()) || entriesHaveNew(obj.GetAnnotations()) {
		return true
	}
	for _, finalizer := range obj.GetFinalizers() {
		pair, ok := finalizerPair(finalizer)
		if ok && finalizer == pair.New && pair.New != pair.Old {
			return true
		}
	}
	return false
}

func entriesHaveNew(entries map[string]string) bool {
	for key := range entries {
		pair, ok := PairForKey(key)
		if ok && key == pair.New && pair.New != pair.Old {
			return true
		}
	}
	return false
}

func cutOwnedPrefix(key, prefix string) (string, bool) {
	suffix, ok := strings.CutPrefix(key, prefix)
	if !ok || suffix == "" || strings.Contains(suffix, "/") {
		return "", false
	}
	return suffix, true
}

func setRawAnnotation(obj metav1.Object, key, value string) error {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[key] = value
	obj.SetAnnotations(annotations)
	return nil
}
