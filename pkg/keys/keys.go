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

// Package keys is the source of migrated Konflux label and annotation key pairs
// (appstudio.* to konflux-ci.dev). It is not CRD/GVK conversion (STONEINTG-1747).
package keys

import "fmt"

const (
	// Migrated prefixes have both old and new key families.

	// PrefixPipelinesOld is the old PipelineRun type prefix.
	// Migration note: retained during ComponentGroup migration; remove after migration is complete.
	PrefixPipelinesOld = "pipelines.appstudio.openshift.io"
	// PrefixPipelinesNew is the new PipelineRun type prefix.
	PrefixPipelinesNew = "pipelines.konflux-ci.dev"

	// PrefixTestOld is the old integration-service test prefix.
	// Migration note: retained during ComponentGroup migration; remove after migration is complete.
	PrefixTestOld = "test.appstudio.openshift.io"
	// PrefixTestNew is the new integration-service test prefix.
	PrefixTestNew = "integration.konflux-ci.dev"

	// PrefixBuildOld is the build-owned metadata copy prefix (build.appstudio*).
	// New build.konflux-ci.dev keys are not copied onto Snapshots yet.
	// Migration note: retained during ComponentGroup migration; remove after migration is complete.
	PrefixBuildOld = "build.appstudio"
	// PrefixBuildNew is the new build-owned metadata prefix.
	PrefixBuildNew = "build.konflux-ci.dev"
)

const (
	// Legacy-only prefixes have no new-key counterpart in this migration.

	// PrefixPAC is the Pipelines-as-Code metadata prefix copied onto Snapshots.
	// Migration note: retained during ComponentGroup migration; remove after migration is complete.
	PrefixPAC = "pac.test.appstudio.openshift.io"
	// PrefixCustom is the custom user-defined metadata prefix.
	// Migration note: retained during ComponentGroup migration; remove after migration is complete.
	PrefixCustom = "custom.appstudio.openshift.io"
	// PrefixRelease is the release metadata prefix.
	// Migration note: retained during ComponentGroup migration; remove after migration is complete.
	PrefixRelease = "release.appstudio.openshift.io"
	// PrefixAppstudio is the old generic resource prefix (application, component, snapshot).
	// There is no konflux-ci.dev/application key; a missing application label is the ComponentGroup path.
	// Migration note: retained during ComponentGroup migration; remove after migration is complete.
	PrefixAppstudio = "appstudio.openshift.io"
)

// Style selects which key of a Pair to write. One style per object; never both.
type Style int

const (
	// StyleOld selects the old key in a migrated key pair.
	// Migration note: retained during ComponentGroup migration; remove after migration is complete.
	StyleOld Style = iota
	StyleNew
)

// Pair is an old/new label or annotation key for the same meaning.
type Pair struct {
	// Old is the pre-migration key.
	// Migration note: retained during ComponentGroup migration; remove after migration is complete.
	Old string
	New string
}

// pipelineType is pipelines.*/type on build and integration PipelineRuns.
var pipelineType = Pair{
	Old: PrefixPipelinesOld + "/type", // retained until ComponentGroup migration is complete
	New: PrefixPipelinesNew + "/type",
}

// PipelineType returns the pipelines.*/type pair on build and integration PipelineRuns.
func PipelineType() Pair {
	return pipelineType
}

var buildComponent = Pair{
	Old: PrefixAppstudio + "/component", // retained until ComponentGroup migration is complete
	New: PrefixBuildNew + "/component",
}

// BuildComponent returns the component name pair on a build PipelineRun.
// Old: appstudio.openshift.io/component. New: build.konflux-ci.dev/component.
func BuildComponent() Pair {
	return buildComponent
}

// Key returns the label/annotation key for style.
func (p Pair) Key(style Style) (string, error) {
	switch style {
	case StyleOld:
		if p.Old == "" {
			return "", fmt.Errorf("pair has no old key")
		}
		return p.Old, nil
	case StyleNew:
		if p.New == "" {
			return "", fmt.Errorf("pair has no new key")
		}
		return p.New, nil
	default:
		return "", fmt.Errorf("invalid style %d", style)
	}
}

// listKeys returns the non-empty old and new keys for DualList selectors.
func (p Pair) listKeys() []string {
	out := make([]string, 0, 2)
	if p.Old != "" {
		out = append(out, p.Old)
	}
	if p.New != "" && p.New != p.Old {
		out = append(out, p.New)
	}
	return out
}

// otherKey returns the key not selected by style, used to clear the opposite label.
func (p Pair) otherKey(style Style) string {
	if style == StyleNew {
		return p.Old
	}
	return p.New
}
