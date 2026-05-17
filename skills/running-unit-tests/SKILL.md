---
name: running-unit-tests
description: Use when running, writing, or troubleshooting unit tests for integration-service. Covers make test, envtest setup, suite patterns, mock loader, CRD discovery, coverage, and common test failures.
---

# Running Unit Tests

## Overview

Unit tests use Ginkgo v2 + Gomega with envtest (a local K8s API server). Every package has a `*_suite_test.go` that bootstraps envtest with CRDs, registers schemes, and starts a controller manager.

## When to Use

- Running or debugging unit tests
- Adding tests to a new or existing package
- Troubleshooting envtest or CRD-related test failures
- Understanding the mock loader pattern

## Quick Reference

| Command | What it does |
|---------|-------------|
| `make test` | Full pipeline: manifests, generate, fmt, vet, envtest, download-crds, then `go test ./... -coverprofile cover.out` |
| `make download-crds` | Downloads CRD YAMLs from dependency modules (vendoring prunes non-Go files) |
| `make envtest` | Downloads setup-envtest tool |
| `go test ./internal/controller/snapshot/...` | Run tests for a single package |
| `go test ./... -run TestName` | Run a specific test by name |

## Suite Setup Pattern

Every suite follows this structure (see any `*_suite_test.go`):

1. **CRD paths** via `CRDDirectoryPaths` — local CRDs from `config/crd/bases` plus dependency CRDs found via `toolkit.GetRelativeDependencyPath()`
2. **Scheme registration** — add all API types (`applicationapiv1alpha1`, `tektonv1`, `releasev1alpha1`, `v1beta2`)
3. **Manager creation** — `ctrl.NewManager()` with `Metrics.BindAddress: "0"` (disables metrics to avoid port conflicts) and `LeaderElection: false`
4. **Cache setup** — call `cache.Setup*Cache(k8sManager)` functions before starting manager
5. **Start manager in goroutine** — `go func() { defer GinkgoRecover(); k8sManager.Start(ctx) }()`
6. **Cleanup** — `cancel()` context and `testEnv.Stop()` in `AfterSuite`

CRD directory paths are relative to the suite file location. Adjust depth (`..`, `../..`) based on package nesting.

## Mock Loader Pattern

`loader/loader_mock.go` intercepts resource loading via context keys:

```go
mockContext := toolkit.GetMockedContext(ctx, []toolkit.MockData{
    {ContextKey: loader.SnapshotContextKey, Resource: mySnapshot},
})
result, err := adapter.loader.GetSnapshot(mockContext, client, name, namespace)
```

Each resource type has a corresponding `*ContextKey` constant (34 keys available).

## Test Conventions

- BDD structure: `Describe` > `Context("When...")` > `It("should...")`
- Use `Eventually` for async assertions, never `time.Sleep`
- Always `Get()` after `Create()`/`Update()` to ensure server-side state
- Test idempotency: calling Reconcile twice must produce the same result
- `ginkgolinter` is enabled — follow its conventions

## Common Mistakes

| Problem | Fix |
|---------|-----|
| `no matches for kind "X"` | Missing CRD in `CRDDirectoryPaths` — add the dependency path |
| Port conflict on metrics | Ensure `Metrics.BindAddress: "0"` in manager options |
| `make test` fails on missing CRDs | Run `make download-crds` — vendoring prunes YAML files |
| Stale generated code | Run `make generate manifests` before testing |
| Tests pass locally, fail in CI | Check `go mod tidy` and `make fmt` — CI fails on uncommitted changes |

## Coverage

- Tool: codecov with flag `unit-tests`
- Threshold: 3% drop allowed from base commit
- Ignores: `**/zz_generated*`, `e2e-tests/**`, vendor
