---
name: ci-cd-quirks
description: Use when CI checks fail unexpectedly, when preparing code for CI, or when encountering non-obvious build and pipeline behavior. Covers vendoring, hermetic builds, security scans, code generation checks, coverage, and webhook validation gotchas.
---

# CI/CD Quirks and Gotchas

## Overview

Integration-service CI runs on Konflux (Tekton-based) for builds and GitHub Actions for linting/testing. Several non-obvious requirements catch developers off guard.

## When to Use

- CI failed and you don't understand why
- Preparing a change that touches dependencies, CRDs, or RBAC
- Understanding what the build pipeline actually does
- Debugging webhook validation behavior

## Must-Run Commands Before Push

| When you changed... | Run this | Why |
|---------------------|----------|-----|
| Any `go.mod` / `go.sum` | `go mod tidy && go mod vendor` | Vendor dir is committed; CI fails on drift |
| Types in `api/v1beta2/` | `make generate manifests` | DeepCopy and CRD/RBAC manifests must be in sync |
| Any `.go` file | `make fmt` | CI fails on uncommitted gofmt changes |
| `+kubebuilder:rbac` markers | `make manifests` | RBAC ClusterRole must reflect markers |

## CI Checks That Surprise People

### Vendoring (most common failure)
Go vendoring prunes non-Go files. After `go mod vendor`, CRD YAML files from dependencies are gone. `make download-crds` re-downloads them for tests. CI runs this automatically, but if you updated deps and didn't vendor, CI fails.

### RBAC Wildcard Check
CI greps `config/` for RBAC wildcards (`*`) and fails if found. Never use wildcard verbs or resource names in `+kubebuilder:rbac` markers.

### Code Generation Drift
CI runs `make generate manifests` and diffs the result. Any uncommitted generated files = failure. Always commit generated output.

### `go mod tidy` Drift
CI runs `go mod tidy` and checks for changes. If your `go.mod`/`go.sum` differ after tidy, CI fails.

## Build Pipeline Details

### PR Pipeline (`.tekton/integration-service-pull-request.yaml`)
- Injects `ENABLE_COVERAGE=true` — binary built with `-cover -covermode=atomic -tags=coverage`
- Hermetic build (network-isolated), deps prefetched via Cachi2
- Multi-platform: x86_64 + arm64
- Image tag: `on-pr-{{revision}}`
- Expires after 5 days, max 3 kept
- Security scans: Clair, Roxctl, Snyk SAST, ClamAV, ShellCheck, Unicode, RPM signature

### Push Pipeline (`.tekton/integration-service-push.yaml`)
- Regular build (no coverage) + extra `build-instrumented-image` task (with coverage)
- Image tag: `{{revision}}`
- Same security scans as PR

### Dockerfile Gotchas
- Builds **two binaries**: `manager` (controller) and `snapshotgc` (garbage collector)
- Both ship in the same image, entrypoint is `/manager`
- `ENABLE_COVERAGE` build arg switches between instrumented and production builds
- Base image: UBI9 minimal, runs as user 65532 (non-root)

## Webhook Validation Gotchas

| Rule | What happens if violated |
|------|------------------------|
| ITS name must be DNS-1035 | Webhook rejects with validation error (lowercase alphanumeric + hyphen, <63 chars) |
| `SNAPSHOT` param in ITS | Cannot be set manually — auto-injected by the service. Webhook rejects. |
| Git resolver: `url` vs `repo+org` | Must use one or the other, not both. Webhook rejects conflicting params. |
| Snapshot validator | Has `failurePolicy: Ignore` — validation failures pass through silently (by design) |

## Coverage

- Tool: codecov, flag: `unit-tests`
- Threshold: 3% drop from base commit allowed
- Ignores: `**/zz_generated*`, `e2e-tests/**`, vendor
- Patch coverage: informational only (non-blocking)

## Common Mistakes

| Problem | Root cause |
|---------|-----------|
| CI fails with "uncommitted changes" | Didn't run `make fmt`, `make generate manifests`, or `go mod tidy` |
| Build fails on missing deps | Didn't run `go mod vendor` after dependency change |
| RBAC check fails | Used wildcard `*` in kubebuilder RBAC marker |
| Coverage dropped | New code paths not covered by tests (check `cover.out`) |
| Webhook rejects valid-looking ITS | Name contains uppercase, underscore, or is >63 chars |
