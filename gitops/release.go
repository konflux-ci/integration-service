/*
Copyright 2024 Red Hat Inc.

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
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
)

// EvaluateSnapshotAutoReleaseAnnotation evaluates the provided auto-release annotation CEL-like expression
// against the given Snapshot and returns whether it allows auto-release.
func EvaluateSnapshotAutoReleaseAnnotation(autoReleaseExpr string, snapshot *applicationapiv1alpha1.Snapshot) (bool, error) {
	// Empty or missing annotation: do not auto-release
	if len(autoReleaseExpr) == 0 {
		return false, nil
	}

	// Convert snapshot to a JSON-like map so field selections like
	// snapshot.metadata.creationTimestamp work naturally with CEL and
	// to allow custom functions to access the snapshot contents.
	objMap, err := convertToCELObjectMap(snapshot)
	if err != nil {
		return false, fmt.Errorf("failed to convert snapshot: %w", err)
	}

	// Build a CEL environment
	// The with a dynamic 'snapshot' variable represents the snapshot object
	funcs := snapshotCELFunctions{snapshot: objMap}
	env, err := cel.NewEnv(
		cel.DefaultUTCTimeZone(true),
		cel.Variable("snapshot", cel.DynType),
		cel.Variable("now", cel.TimestampType),
		// Register custom function: updatedComponentIs(name: string) -> bool
		cel.Function("updatedComponentIs",
			cel.Overload("updatedComponentIs_string_bool",
				[]*cel.Type{cel.StringType},
				cel.BoolType,
				cel.UnaryBinding(funcs.updatedComponentIs),
			),
		),
	)
	if err != nil {
		return false, fmt.Errorf("failed to create CEL env: %w", err)
	}

	// Compile the expression
	ast, iss := env.Compile(autoReleaseExpr)
	if iss != nil && iss.Err() != nil {
		return false, fmt.Errorf("invalid cel expression: %w", iss.Err())
	}

	prog, err := env.Program(ast)
	if err != nil {
		return false, fmt.Errorf("failed to create cel program: %w", err)
	}

	activation := map[string]any{}
	activation["snapshot"] = objMap
	activation["now"] = time.Now().UTC()

	// Evaluate
	out, _, err := prog.ContextEval(context.Background(), activation)
	if err != nil {
		return false, err
	}

	// Expect a boolean result
	if b, ok := out.Value().(bool); ok {
		return b, nil
	}
	return false, fmt.Errorf("cel expression did not evaluate to a boolean")
}

// snapshotCELFunctions contains named implementations of custom CEL functions which
// may access the current snapshot via the captured map.
// Note: The method value used during registration is not an anonymous function.
// It captures the receiver 'snapshotCELFunctions' instance with the prepared snapshot map.
// This helps avoid global state while keeping a named function.

type snapshotCELFunctions struct {
	snapshot map[string]any
}

func (sf snapshotCELFunctions) updatedComponentIs(arg ref.Val) ref.Val {
	name, ok := arg.Value().(string)
	if !ok {
		return types.Bool(false)
	}
	// Navigate snapshot.spec.components[].name
	spec, ok := sf.snapshot["spec"].(map[string]any)
	if !ok {
		return types.Bool(false)
	}
	components, ok := spec["components"].([]any)
	if !ok {
		return types.Bool(false)
	}
	for _, c := range components {
		cm, ok := c.(map[string]any)
		if !ok {
			continue
		}
		if compName, ok := cm["name"].(string); ok && compName == name {
			return types.Bool(true)
		}
	}
	return types.Bool(false)
}

// convertToCELObjectMap marshals the typed object to JSON and back to a
// map[string]any, ensuring JSON field names (e.g., metadata.creationTimestamp)
// are available to CEL.
func convertToCELObjectMap(obj any) (map[string]any, error) {
	raw, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}
