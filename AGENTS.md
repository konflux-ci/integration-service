# AGENTS.md
## Overview

Integration Service is a Kubernetes operator that orchestrates integration testing pipelines in Konflux CI. It watches specific Tekton PipelineRun and Snapshot CRs, validates them, creates integration testing Tekton PipelineRuns, tracks their progress, records results, and reports them to the associated git provider if available. It also optionally creates Release CRs which start the automated release process for the given Snapshot. 
The [Konflux architecture documentation](https://github.com/konflux-ci/architecture) contains information about the broader scope of Konflux CI, while the [Konflux user documentation](https://github.com/konflux-ci/docs) contains the user facing docs.

## Technology Stack

- **Language**: Go
- **Framework:** controller-runtime
- **CRDs**: IntegrationTestScenario, ComponentGroup, NudgeConfig
- **Pipeline engine**: Tekton PipelineRuns
- **Testing**: Ginkgo/Gomega + envtest (local K8s API server)
- **Build**: `make test` (unit), `make manifests generate` (codegen)

## Repository Structure

```
api/v1beta2/           # CRD types
internal/controller/   # Reconcilers: buildpipeline/, component/, componentgroup/, integrationpipeline/, nudgeconfig/, scenario/, snapshot/, statusreport/
internal/webhooks      # Webhooks
loader/                # ObjectLoader interface — abstracts K8s resource fetching
tekton/                # PipelineRun builders, status helpers, watch predicates
gitops/                # Snapshot and Release CR generation and management functions
pkg/                   # integrationteststatus/ and dag/ contain exported structs, metrics/ contains Prometheus gauges and histograms, conversion/ contains functions to convert between appstudio.redhat.com and konflux-ci.dev versions of the same resource
helpers/               # Additional utility functions
config/                # Kustomize manifests (CRDs, RBAC, webhooks, samples)
e2e-tests/             # Ginkgo e2e test definitions for the integration service suite
integration-tests/     # Tekton pipeline and task definitions for running integration and e2e tests
docs/                  # Controller documentation and mermaid flow diagrams
main.go                # Entry point — registers controllers and webhooks
```

## Architecture

### Adapter Pattern

Each controller delegates to an **adapter** (`<controller_name>_adapter.go`) that holds the K8s client, resource under reconciliation, an `ObjectLoader`, and a `IntegrationLogger`. All domain logic lives in adapter methods, not in the controller.

### Dual-Model Architecture

The service supports two parallel resource flows: **ComponentGroup** (new model) and **Application** (legacy, planned for deprecation). Controllers and adapters implement both flows side-by-side (e.g., `EnsureSnapshotExists` vs `EnsureSnapshotExistsApplication` in `buildpipeline_adapter.go`, `NewAdapter` vs `NewAdapterWithApplication` in `snapshot_adapter.go`). Any cross-cutting feature — tracing, metrics, annotations, error handling — **must cover both flows** until the Application model is fully removed. Key dual-path controllers include `buildpipeline`, `snapshot`, and `statusreport`. Look for `TODO: remove when we deprecate old application model` comments to identify Application-model code paths.

### Resource Loading

`loader.ObjectLoader` centralizes all K8s Gets with error classification (retriable vs permanent) and KubeArchive fallback for deleted resources. A mock implementation exists for tests.

### Nudging / Batching Subsystem

The nudging subsystem (`tekton/nudging/`, `internal/controller/nudgeconfig/`) automates downstream component updates via Renovate PipelineRuns when upstream source components are rebuilt. It is governed by ADR-0056 (one-shot actions) and ADR-0072 (batch aggregation semantics).

**Singleton constraint:** Exactly one NudgeConfig per namespace, named `nudge-config`. Enforced by CEL validation on the CRD.

**Nudge modes:** Each `NudgeRelationship` in `spec.nudges` has a mode — `immediate` (fires on build success) or `validated` (fires after integration tests pass for the source component's Snapshot).

**Batch lifecycle phases:** `Accumulating → Blocked → Firing → Failed/Completed`. All switch/if-chains on batch phase must handle every phase. An incomplete state machine is a correctness bug.

- **Accumulating** — collecting build events within the batch window (debounce timer resets on each new event; hard deadline does not reset).
- **Blocked** — a source build failed and `FailurePolicy` is `Block`. The batch cannot fire until the failed component rebuilds successfully or the user force-fires.
- **Firing** — the aggregated Renovate PipelineRun has been created. The controller polls for PipelineRun completion.
- **Failed** — the nudge PipelineRun failed after exhausting retries (`maxBatchNudgePLRRetries = 3`) or the batch had no accumulated builds when the timer expired.
- **Completed** — the nudge PipelineRun succeeded.

**Batch aggregation contract:** A single batched Renovate PipelineRun must contain ALL accumulated `(component, digest)` pairs. The `fireBatch` function iterates `batch.Accumulated` to build the payload — implementation must append entries, not overwrite. The `upsertAccumulatedEntry` function matches by `From` (source component name) so that rebuilds of the same component replace the previous entry.

**Dedup rule:** When a component is rebuilt, the latest result replaces the previous entry by component identity (`From` field = source component name), not by PipelineRun name. This is enforced by `upsertAccumulatedEntry` and `upsertFailedEntry`.

**Failure recording:** Failed builds must be recorded in `batch.Failed[]` via `RecordFailedBuildForBatchedNudge` / `recordFailureForBatchedNudge` for `FailurePolicyType` (`Block` / `ProceedWithPartial`) to function. Without failure recording, `FailurePolicyType` is dead code.

**Concurrency guard:** Only one batch per target may be in `Firing` phase at a time. Enforced by `isBatchFiringForTarget` — checked before both timer-based and force-fire batch creation.

**One-shot action pattern (ADR-0056):** `spec.actions.forceFire` must be cleared after processing regardless of outcome. The controller calls `ClearNudgeConfigActions` unconditionally after processing forceFire, even on error. Leaving it set causes infinite reconciliation.

**BatchPolicy inheritance:** Resolution order: per-target `TargetConfig.BatchPolicy` → `spec.batchDefaults` → runtime defaults (debounce: 30m, maxWait: 4h, failurePolicy: Block). Implemented in `NudgeConfigSpec.EffectiveBatchPolicy`.

**Hard deadline:** Once an accumulating batch's `HardDeadline` passes, it fires immediately when unblocked rather than waiting for the debounce timer. If past deadline while blocked, the batch fires as soon as the block is cleared (successful rebuild of the failed component).

**Key source files:**
- `api/v1beta2/nudgeconfig_types.go` — CRD types, batch phases, policy defaults, `EffectiveBatchPolicy`
- `internal/controller/nudgeconfig/nudgeconfig_controller.go` — Reconciler, event predicates, requeue logic
- `tekton/nudging/nudge_batch.go` — Batch lifecycle: record builds, process due/firing batches, forceFire
- `tekton/nudging/nudge_batch_validated.go` — Validated-mode batch recording
- `tekton/nudging/nudge_pipeline.go` — Renovate PipelineRun creation, config generation
- `tekton/nudging/nudge_credentials.go` — Git provider credential resolution
- `docs/nudgeconfig-controller.md` — Controller flow diagram

### Additional Key Patterns

- **PipelineRunBuilder** (`tekton/`): fluent API — `.WithExtraParams()`, `.WithSnapshot()`, `.WithIntegrationTimeouts()`, `.WithUpdatedPipelineGitResolver()`
- **Metrics**: registered during reconciliation — `RegisterCompletedSnapshot()`, `RegisterInvalidSnapshot()`, `RegisterPipelineRunStarted()`, `RegisterIntegrationResponse()`, with start/completion times

## Development Guidelines

- See `CONTRIBUTING.md` for overall guidelines for making contributions to this repository.
- **Git**: conventional commits with Jira ticket as scope — `type(issue-id): description` — use the Jira ticket as scope when one exists (e.g. `feat(STONEINTG-1519): create PR group snapshots`), or the GitHub issue number when no Jira ticket is associated (e.g. `fix(#1790): skip no-newline markers in diff parser`)
- Follow [Kubernetes coding conventions](https://github.com/kubernetes/community/blob/master/contributors/guide/coding-conventions.md)
- Log via `IntegrationLogger` from `helpers/logs` added as the adapter's `logger`, wrap errors with `fmt.Errorf("context: %w", err)`
- **API changes**: edit types in `api/v1beta2/`, then `make generate manifests`
- **Controller changes**: implement in `internal/controller/<resource>/<resource>_adapter.go`
- **Webhooks**: add to `internal/webhooks/<resource>/`
- **Tests**: unit tests alongside code using Ginkgo + envtest; E2E tests are located in `e2e-tests/tests`. When running unit tests, prefer `make test` so envtest assets are configured automatically. If running `go test` directly for packages that use envtest/webhooks, first export `KUBEBUILDER_ASSETS` with the setup-envtest path, for example: `export KUBEBUILDER_ASSETS="$(./bin/setup-envtest use 1.29 -p path)"`.
- **Package and tooling changes**: When adding, removing or moving Go package directories, check for explicit package paths in Makefile targets, CI workflows, linting, code coverage and scripts. Use recursive patterns like  `./...` wherever possible.

