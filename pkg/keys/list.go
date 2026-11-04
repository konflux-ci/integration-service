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
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/selection"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// DualList lists objects whose effective pair value (new key preferred, else
// old) is In values. It runs two AND-only selectors (old key, then new key)
// plus dedup — label selectors cannot OR. The old-key query also requires the
// new key DoesNotExist so a conflicting new label wins, matching GetLabel.
// Extra requirements are applied to both queries. The result is a fully
// merged, non-pageable snapshot: Continue and other pagination ListMeta from
// a prior use of list are cleared and must not be treated as a Kubernetes
// list continuation.
func DualList(ctx context.Context, c client.Client, list client.ObjectList, namespace string, pair Pair, values []string, extras ...labels.Requirement) error {
	if list == nil {
		return fmt.Errorf("list cannot be nil")
	}

	merged := make([]runtime.Object, 0)
	seen := map[string]struct{}{}

	for _, key := range pair.listKeys() {
		req, err := labels.NewRequirement(key, selection.In, values)
		if err != nil {
			return fmt.Errorf("selector for %s: %w", key, err)
		}
		selector := labels.NewSelector().Add(*req)
		// Prefer new: when querying the old key, skip objects that already have the new key.
		if key == pair.Old && pair.New != "" && pair.New != pair.Old {
			absent, err := labels.NewRequirement(pair.New, selection.DoesNotExist, nil)
			if err != nil {
				return fmt.Errorf("selector for absent %s: %w", pair.New, err)
			}
			selector = selector.Add(*absent)
		}
		for _, extra := range extras {
			selector = selector.Add(extra)
		}

		tmp, err := emptyList(list)
		if err != nil {
			return fmt.Errorf("copying list: %w", err)
		}
		opts := []client.ListOption{client.MatchingLabelsSelector{Selector: selector}}
		if namespace != "" {
			opts = append(opts, client.InNamespace(namespace))
		}
		if err := c.List(ctx, tmp, opts...); err != nil {
			return fmt.Errorf("list %s: %w", key, err)
		}
		items, err := meta.ExtractList(tmp)
		if err != nil {
			return fmt.Errorf("extracting list: %w", err)
		}
		for _, item := range items {
			obj, ok := item.(client.Object)
			if !ok {
				continue
			}
			id := obj.GetNamespace() + "/" + obj.GetName()
			if _, dup := seen[id]; dup {
				continue
			}
			seen[id] = struct{}{}
			merged = append(merged, item)
		}
	}

	if err := meta.SetList(list, merged); err != nil {
		return fmt.Errorf("setting list: %w", err)
	}
	clearListPaginationMeta(list)
	return nil
}

// clearListPaginationMeta clears Continue and related ListMeta that cannot
// describe DualList's merged, non-pageable result.
func clearListPaginationMeta(list client.ObjectList) {
	accessor, err := meta.ListAccessor(list)
	if err != nil {
		return
	}
	accessor.SetContinue("")
	accessor.SetResourceVersion("")
	accessor.SetRemainingItemCount(nil)
}

// Requirement is one label selector clause. Each query in ListEither uses the
// key for one style, so an object that carries only old keys matches the old
// query and an object that carries only new keys matches the new query.
type Requirement struct {
	Pair     Pair
	Operator selection.Operator
	Values   []string
}

// ListEither lists objects twice, once per metadata style, and merges the
// results. Requirements are AND-ed within a style. extra options (namespace
// overrides, field selectors) are applied to both queries. The result is not
// pageable.
func ListEither(ctx context.Context, c client.Client, list client.ObjectList, namespace string, reqs []Requirement, extra ...client.ListOption) error {
	if list == nil {
		return fmt.Errorf("list cannot be nil")
	}
	if c == nil {
		return fmt.Errorf("client cannot be nil")
	}
	if len(reqs) == 0 {
		return fmt.Errorf("at least one requirement is required")
	}

	merged := make([]runtime.Object, 0)
	seen := map[string]struct{}{}
	for _, style := range []Style{StyleOld, StyleNew} {
		selector := labels.NewSelector()
		for _, req := range reqs {
			key, err := req.Pair.Key(style)
			if err != nil {
				return fmt.Errorf("selector key: %w", err)
			}
			parsed, err := requirementFor(key, req)
			if err != nil {
				return fmt.Errorf("selector for %s: %w", key, err)
			}
			selector = selector.Add(parsed)
		}
		// Prefer new: an object that already has the new key must not match the old query,
		// even when the old key is also set to a searched value.
		if style == StyleOld {
			seenNew := map[string]struct{}{}
			for _, req := range reqs {
				if req.Pair.New == "" || req.Pair.New == req.Pair.Old {
					continue
				}
				if _, ok := seenNew[req.Pair.New]; ok {
					continue
				}
				seenNew[req.Pair.New] = struct{}{}
				absent, err := labels.NewRequirement(req.Pair.New, selection.DoesNotExist, nil)
				if err != nil {
					return fmt.Errorf("selector for absent %s: %w", req.Pair.New, err)
				}
				selector = selector.Add(*absent)
			}
		}
		tmp, err := emptyList(list)
		if err != nil {
			return fmt.Errorf("copying list: %w", err)
		}
		opts := []client.ListOption{client.MatchingLabelsSelector{Selector: selector}}
		if namespace != "" {
			opts = append(opts, client.InNamespace(namespace))
		}
		opts = append(opts, extra...)
		if err := c.List(ctx, tmp, opts...); err != nil {
			return fmt.Errorf("list style %d: %w", style, err)
		}
		items, err := meta.ExtractList(tmp)
		if err != nil {
			return fmt.Errorf("extracting list: %w", err)
		}
		for _, item := range items {
			obj, ok := item.(client.Object)
			if !ok {
				continue
			}
			id := obj.GetNamespace() + "/" + obj.GetName()
			if _, dup := seen[id]; dup {
				continue
			}
			seen[id] = struct{}{}
			merged = append(merged, item)
		}
	}
	if err := meta.SetList(list, merged); err != nil {
		return fmt.Errorf("setting list: %w", err)
	}
	clearListPaginationMeta(list)
	return nil
}

// requirementFor builds one selector clause. A single In/Equals value uses the
// same unvalidated set selector as client.MatchingLabels, so an over-long
// snapshot name fails later at PipelineRun creation instead of at list time.
func requirementFor(key string, req Requirement) (labels.Requirement, error) {
	singleValue := len(req.Values) == 1 && (req.Operator == selection.In || req.Operator == selection.Equals || req.Operator == selection.DoubleEquals)
	if singleValue {
		set := labels.SelectorFromValidatedSet(labels.Set{key: req.Values[0]})
		parsed, _ := set.Requirements()
		if len(parsed) == 1 {
			return parsed[0], nil
		}
	}
	parsed, err := labels.NewRequirement(key, req.Operator, req.Values)
	if err != nil {
		return labels.Requirement{}, err
	}
	return *parsed, nil
}

// emptyList returns a deep copy of list with no items, used as a scratch
// ObjectList for each DualList query.
func emptyList(list client.ObjectList) (client.ObjectList, error) {
	copied := list.DeepCopyObject()
	tmp, ok := copied.(client.ObjectList)
	if !ok {
		return nil, fmt.Errorf("unable to copy list")
	}
	if err := meta.SetList(tmp, []runtime.Object{}); err != nil {
		return nil, fmt.Errorf("clearing list: %w", err)
	}
	return tmp, nil
}
