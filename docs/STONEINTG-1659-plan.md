# STONEINTG-1659: Define NudgeConfig CRD schema with stateless CEL validation rules

Parent epic: STONEINTG-1495 (Implement component nudging logic in integration service)
Follow-up stories: STONEINTG-1660 (webhook, cycle detection), STONEINTG-1661 (VAP, non-existent components)

## Scope

Schema-only story — no controller/reconciler. No webhook in this story.
CEL validation rules run in-process in the API server (zero webhook overhead).

## Context

STONEINTG-1659 introduces a **NudgeConfig** CRD — a namespace-scoped singleton that stores component-to-component nudging relationships. It replaces the `BuildNudgesRef` field on Component CRs (ADR-0067). The CRD uses **stateless CEL validation rules** (`x-kubernetes-validations`) enforced by the API server with zero webhook overhead.

## API Decision

Package: `api/v1beta2` — same as ComponentGroup (`appstudio.redhat.com/v1beta2`).
Rationale: existing pattern for newest CRDs in this repo; avoids introducing new API group.

---

## Implementation Status

### DONE

| File | Status |
|------|--------|
| *(none yet)* | |

### PENDING

| Task | Notes |
|------|-------|
| `api/v1beta2/nudgeconfig_types.go` | Go types with CEL markers |
| `Makefile` ENVTEST_K8S_VERSION 1.23→1.29 | CEL is GA in 1.29 |
| `make manifests generate` | CRD YAML + deepcopy generated |
| `config/crd/bases/appstudio.redhat.com_nudgeconfigs.yaml` | Auto-generated |
| `config/crd/kustomization.yaml` | Updated by generator |
| `config/rbac/nudgeconfig_{admin,editor,viewer}_role.yaml` | Auto-generated RBAC roles |
| `config/rbac/kustomization.yaml` | Updated by generator |
| `config/samples/appstudio_v1beta2_nudgeconfig.yaml` | Auto-generated sample |
| `config/samples/kustomization.yaml` | Updated by generator |
| `api/v1beta2/nudgeconfig_types_test.go` (pure struct tests) | Pure Go, testing.T |
| `api/v1beta2/nudgeconfig_validation_test.go` (envtest CEL tests) | Ginkgo, shared suite |
| `make build` | Must pass cleanly |

---

## Files to Create / Modify

### 1. `api/v1beta2/nudgeconfig_types.go` (new)

Follow patterns from `integrationtestscenario_types.go` and `componentgroup_types.go`.

**Types:**

```go
// NudgeModeType defines when the nudge is triggered.
// +kubebuilder:validation:Enum=immediate;validated
type NudgeModeType string

const (
    NudgeModeImmediate NudgeModeType = "immediate"
    NudgeModeValidated NudgeModeType = "validated"
)

// NudgeConfigSingletonName is the required name for the singleton NudgeConfig per namespace.
const NudgeConfigSingletonName = "nudge-config"

// NudgeRelationship defines a single nudge from one component to another.
type NudgeRelationship struct {
    // From is the source component name that triggers the nudge.
    // +kubebuilder:validation:MinLength=1
    // +kubebuilder:validation:MaxLength=63
    // +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
    // +required
    From string `json:"from"`

    // To is the target component name that receives the nudge.
    // +kubebuilder:validation:MinLength=1
    // +kubebuilder:validation:MaxLength=63
    // +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
    // +required
    To string `json:"to"`

    // Mode defines when the nudge is triggered.
    // +kubebuilder:default=immediate
    // +optional
    Mode NudgeModeType `json:"mode,omitempty"`

    // GatingGroup is reserved for Phase 2 group-based gating.
    // +kubebuilder:validation:MaxLength=63
    // +optional
    GatingGroup string `json:"gatingGroup,omitempty"`
}

// NudgeConfigSpec defines the desired nudging relationships for components in a namespace.
// +kubebuilder:validation:XValidation:rule="!has(self.nudges) || self.nudges.all(n, n.from != n.to)",message="self-nudge not allowed: from and to must be different"
// +kubebuilder:validation:XValidation:rule="!has(self.nudges) || self.nudges.all(i, self.nudges.exists_one(j, i.from == j.from && i.to == j.to))",message="duplicate (from, to) pair not allowed"
type NudgeConfigSpec struct {
    // Nudges is the list of component nudge relationships.
    // +kubebuilder:validation:MaxItems=256
    // +optional
    Nudges []NudgeRelationship `json:"nudges,omitempty"`
}

// NudgeConfigStatus defines the observed state of NudgeConfig.
type NudgeConfigStatus struct {
    // Conditions represent the latest available observations of the NudgeConfig's state.
    // +optional
    Conditions []metav1.Condition `json:"conditions,omitempty"`

    // LastValidationTime is the timestamp of the last successful validation of the nudge graph.
    // +optional
    LastValidationTime *metav1.Time `json:"lastValidationTime,omitempty"`
}
```

**NudgeConfig root type with CEL singleton enforcement:**

```go
// +kubebuilder:object:root=true
// +kubebuilder:resource:shortName=nc
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:storageversion
// +kubebuilder:validation:XValidation:rule="self.metadata.name == 'nudge-config'",message="NudgeConfig must be named 'nudge-config' (singleton per namespace)"
type NudgeConfig struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`

    Spec   NudgeConfigSpec   `json:"spec,omitempty"`
    Status NudgeConfigStatus `json:"status,omitempty"`
}
```

**Registration:**
```go
func init() {
    SchemeBuilder.Register(&NudgeConfig{}, &NudgeConfigList{})
}
```

### 2. `Makefile` — Bump ENVTEST_K8S_VERSION

**Line 55:** `ENVTEST_K8S_VERSION = 1.23` → `ENVTEST_K8S_VERSION = 1.29`

**Why:** CEL validation (`x-kubernetes-validations`) is GA in k8s 1.29. The current 1.23 API server binaries don't enforce CEL rules — validation tests would silently pass with invalid data. The project already depends on k8s v0.35.3 APIs, so this is a safe bump.

### 3. Run `make manifests generate` — auto-generates:

- `config/crd/bases/appstudio.redhat.com_nudgeconfigs.yaml`
- `api/v1beta2/zz_generated.deepcopy.go` (updated)
- RBAC roles: `config/rbac/nudgeconfig_{admin,editor,viewer}_role.yaml`
- Sample: `config/samples/appstudio_v1beta2_nudgeconfig.yaml`
- Updated `config/crd/kustomization.yaml` and `config/rbac/kustomization.yaml`
- Updated `config/samples/kustomization.yaml`

### 4. `api/v1beta2/nudgeconfig_types_test.go` (new) — pure Go struct tests

Uses `testing.T` (no envtest, no Ginkgo). Fast, no API server needed.

- Valid full NudgeConfig construction (all fields populated)
- Minimal NudgeConfig (only required fields)
- NudgeModeType constants verification
- DeepCopy correctness (modify copy, verify original unchanged)
- NudgeConfigList DeepCopy
- NudgeConfigSpec DeepCopy
- NudgeConfigStatus DeepCopy

### 5. `api/v1beta2/nudgeconfig_validation_test.go` (new) — envtest Ginkgo CEL tests

Reuses existing suite from `integrationtestscenario_suite_test.go` (shared `k8sClient`, `testEnv`, `ctx`).

Covers all 3 CEL rules + AC items:

| Test | What it creates | Expected result |
|------|----------------|-----------------|
| Valid NudgeConfig | name="nudge-config", valid nudges | `Succeed()` |
| Empty nudges | name="nudge-config", no nudges | `Succeed()` |
| Invalid name | name="wrong-name" | `ShouldNot(Succeed())`, error contains "nudge-config" |
| Self-nudge | from="comp-a", to="comp-a" | `ShouldNot(Succeed())`, error contains "self-nudge" |
| Duplicate pair | two entries with same from/to | `ShouldNot(Succeed())`, error contains "duplicate" |
| Mode default | nudge with no mode set → read back | mode == "immediate" |
| Mode "validated" | nudge with mode="validated" | `Succeed()` |
| MaxItems exceeded | 257 entries | `ShouldNot(Succeed())` |

**Pattern:**
```go
var _ = Describe("NudgeConfig CEL validation", Ordered, func() {
    AfterEach(func() {
        nc := &NudgeConfig{}
        _ = k8sClient.Get(ctx, types.NamespacedName{Name: NudgeConfigSingletonName, Namespace: "default"}, nc)
        _ = k8sClient.Delete(ctx, nc)
    })

    It("should reject creation with a name other than 'nudge-config'", func() {
        nc := &NudgeConfig{
            ObjectMeta: metav1.ObjectMeta{Name: "wrong-name", Namespace: "default"},
            Spec: NudgeConfigSpec{Nudges: []NudgeRelationship{
                {From: "a", To: "b"},
            }},
        }
        Expect(k8sClient.Create(ctx, nc)).ShouldNot(Succeed())
    })
    // ... more tests
})
```

---

## Acceptance Criteria Mapping

| AC | Covered by |
|----|-----------|
| Name != "nudge-config" rejected | nudgeconfig_validation_test.go |
| Go CRD types created | nudgeconfig_types.go |
| from == to rejected | nudgeconfig_validation_test.go |
| Duplicate (from,to) rejected | nudgeconfig_validation_test.go |
| spec.nudges capped at 256 | nudgeconfig_validation_test.go |
| mode defaults to immediate | nudgeconfig_validation_test.go |
| make manifests generate passes | step 3 |
| Unit tests cover all 3 CEL rules | nudgeconfig_validation_test.go |

## Key Design Decisions

1. **CEL over webhooks**: Story explicitly requires "no webhook overhead." CEL rules are embedded in the CRD YAML and enforced by the API server. Webhooks come in STONEINTG-1660.

2. **Self-nudge CEL on NudgeConfigSpec**: `!has(self.nudges) || self.nudges.all(n, n.from != n.to)` on the spec, with null-safety guard. Placing it on the spec (not on NudgeRelationship) keeps all validation at one level and avoids potential issues with per-entry CEL on nested structs.

3. **Null-safety guards (`!has(self.nudges) ||`)**: Both spec-level CEL rules are prefixed with `!has(self.nudges) ||` to prevent CEL evaluation errors when the nudges array is nil or empty.

### CEL Cost Budget Constraints — Why maxItems=256 and MaxLength=63

When the Kubernetes API server registers a CRD that contains CEL validation rules
(`x-kubernetes-validations`), it performs a **static cost estimation** *before* any
real data is ever validated. The estimator looks at the worst-case scenario using the
schema-declared bounds (`maxItems`, `maxLength`, etc.) to decide whether the rule
could ever be too expensive to run. If the estimated cost exceeds the budget, the API
server **rejects the CRD itself** — not just bad data, but the entire CRD definition.

**How the cost estimator works (simplified):**

The estimator multiplies together:

```
estimated cost ≈ iterations × (string-comparison cost per iteration)
```

- **iterations**: for `array.all(...)` it's `maxItems`. For *nested*
  `array.all(i, array.all(j, ...))` it's `maxItems × maxItems` (O(n²)).
- **string-comparison cost**: when you compare two strings (`i.from != j.from`),
  the cost is proportional to the *maximum possible length* of those strings.
  If you don't declare `maxLength`, the estimator assumes a very large default
  (~1 million characters), which blows up the cost immediately.

**What happened with the original maxItems=5000, no maxLength:**

Our duplicate-detection rule is O(n²):

```cel
self.nudges.all(i, self.nudges.all(j, i == j || i.from != j.from || i.to != j.to))
```

The estimator computed:

```
5000 × 5000 × ~1,000,000 (unbounded string length) × 2 (two string comparisons)
= astronomical number → rejected ("exceeds budget by factor of more than 100x")
```

**First fix attempt — maxItems=1024, maxLength=253:**

```
1024 × 1024 × 253 × 2 ≈ 530 million → still rejected ("exceeds budget by 22.8x")
```

**Final fix — maxItems=256, maxLength=63:**

```
256 × 256 × 63 × 2 ≈ 8.3 million → accepted (within budget)
```

**Why these specific values are fine for production:**

- **maxLength=63**: Kubernetes resource names (including Konflux Component names)
  follow DNS label rules, which cap at 63 characters. No Component name can ever
  exceed this, so 63 is not a real limitation — it just tells the truth about the
  data model.
- **maxItems=256**: A Konflux namespace typically has 10-50 Components. Even with
  complex multi-tier applications, 256 nudge relationships is generous. If a
  future tenant genuinely needs more, the limit can be raised once the
  duplicate-detection rule is moved from CEL to the validating webhook
  (STONEINTG-1660), where there is no static cost estimation — the webhook runs
  real Go code with no artificial budget.

4. **Duplicate pair detection via `exists_one`**: `self.nudges.all(i, self.nudges.exists_one(j, i.from == j.from && i.to == j.to))` — ensures that for every relationship, there is exactly one entry with that (from, to) pair (itself). Unlike the earlier nested `all()` approach with `i == j`, this correctly catches exact-value duplicates because `exists_one` counts matches rather than relying on structural identity. O(n²) but bounded by maxItems=256.

5. **`NudgeModeType` named type**: String type alias with constants (`NudgeModeImmediate`, `NudgeModeValidated`) for type safety and reuse in future controller code.

6. **Mode defaulting via kubebuilder**: `+kubebuilder:default=immediate` on the Mode field handles defaulting at schema level (no webhook needed).

7. **gatingGroup**: Defined in the type but not validated in Phase 1 (reserved for Phase 2).

8. **Two separate test files**: Struct tests (`_types_test.go`, pure Go, instant) separated from CEL validation tests (`_validation_test.go`, envtest, slower). Follows existing pattern of `componentgroup_types_test.go` alongside envtest tests.

---

## To Run Envtest CEL Tests Locally

```bash
bin/setup-envtest use 1.29   # download binaries once
make test                    # or: KUBEBUILDER_ASSETS=$(bin/setup-envtest use 1.29 -p path) go test ./api/v1beta2/...
```

---

## Verification Checklist

- [ ] `make manifests generate` passes
- [ ] `make build` passes cleanly
- [ ] Pure struct tests all pass (7/7)
- [ ] Envtest CEL validation tests pass (needs setup-envtest 1.29 download)
- [ ] CRD YAML has `x-kubernetes-validations` at resource and spec level
- [ ] CRD YAML has `maxItems: 256` on nudges
- [ ] CRD YAML has `maxLength: 63` on from and to
- [ ] CRD YAML has `default: immediate` on mode
- [ ] CRD YAML has `enum: [immediate, validated]` on mode

---

## Reference Files

- `api/v1beta2/integrationtestscenario_types.go` — type pattern template
- `api/v1beta2/componentgroup_types.go` — latest CRD type example
- `api/v1beta2/componentgroup_types_test.go` — pure Go struct test pattern
- `api/v1beta2/integrationtestscenario_suite_test.go` — envtest suite (shared)
- `api/v1beta2/groupversion_info.go` — SchemeBuilder registration
- git commit `8e8e1e45` — ComponentGroup CRD introduction (reference for all generated artifacts)
