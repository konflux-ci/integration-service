---
name: running-e2e-tests
description: Use when building, configuring, or running e2e tests for integration-service against a real cluster (Kind or OpenShift). Covers environment variables, test framework, Ginkgo flags, CI pipeline, and test repos.
---

# Running E2E Tests

## Overview

E2E tests run against a real Kubernetes cluster (Kind for upstream, OpenShift for downstream). They use a custom framework in `e2e-tests/pkg/framework/` with specialized controllers for managing Konflux resources during tests.

## When to Use

- Running e2e tests locally or in CI
- Adding new e2e test cases
- Debugging e2e test failures
- Understanding the CI pipeline that provisions clusters and runs tests

## Quick Reference

| Command | What it does |
|---------|-------------|
| `make e2e-build` | Build test binary: `./e2e-tests/bin/e2e-appstudio` (build tag: `e2e`) |
| `make e2e-run` | Build and run with `--ginkgo.focus="integration-service" --ginkgo.vv` |
| `./e2e-tests/bin/e2e-appstudio --ginkgo.focus="test name"` | Run specific tests |
| `--ginkgo.label-filter="!slow"` | Filter by Ginkgo labels |
| `--ginkgo.junit-report=report.xml` | Generate JUnit report |

## Required Environment Variables

| Variable | Purpose |
|----------|---------|
| `GITHUB_TOKEN` | GitHub API access for test repos |
| `MY_GITHUB_ORG` | GitHub org containing test repos |
| `KUBECONFIG` | Path to cluster kubeconfig |

## Optional Environment Variables

| Variable | Default | Purpose |
|----------|---------|---------|
| `TEST_ENVIRONMENT` | `downstream` | `upstream` (Kind) or `downstream` (OpenShift) |
| `E2E_APPLICATIONS_NAMESPACE_ENV` | auto-created | Test namespace |
| `SKIP_PAC_TESTS` | `false` | Skip Pipelines-as-Code tests |
| `KLOG_VERBOSITY` | `1` | Log verbosity (1-5) |
| `GITLAB_BOT_TOKEN` | - | GitLab API access |
| `GITLAB_QE_ORG` / `GITLAB_API_URL` | - | GitLab org and endpoint |
| `CODEBERG_BOT_TOKEN` / `CODEBERG_QE_ORG` | - | Codeberg access |

## Test Framework

The framework (`e2e-tests/pkg/framework/framework.go`) provides:

- **`AsKubeAdmin` / `AsKubeDeveloper`** — permission contexts with specialized controllers
- **ControllerHub** — `HasController` (apps/components), `TektonController` (PipelineRuns), `IntegrationController` (ITS), `ReleaseController`, `CommonController` (namespaces)
- Auto-detects cluster type and domain from OpenShift console route
- Auto-creates test namespace if `E2E_APPLICATIONS_NAMESPACE_ENV` not set

## CI Pipeline

`integration-tests/pipelines/konflux-e2e-tests.yaml` runs the full flow:

1. Extract snapshot/component metadata
2. Provision Kind cluster on AWS via MAPT
3. Deploy full Konflux stack using `konflux-ci` deployment scripts
4. Run e2e tests (2h timeout, 10 parallel Ginkgo procs)
5. Collect coverage, report status, deprovision cluster

## Test Files

Tests live in `e2e-tests/tests/integration-service/`:
- `integration.go` — core integration test scenarios
- `status-reporting-to-pullrequest.go` — PR status reporting tests
- `group-snapshots-tests.go` — ComponentGroup snapshot tests
- `gitlab-integration-reporting.go` / `forgejo-integration-reporting.go` — multi-platform git provider tests

## Common Mistakes

| Problem | Fix |
|---------|-----|
| `GITHUB_TOKEN` rate limited | CI rotates tokens for highest rate limit — locally, use a PAT with sufficient scope |
| Tests hang waiting for PipelineRun | Check cluster has Tekton installed and ITS configured correctly |
| `TEST_ENVIRONMENT` wrong | Use `upstream` for Kind, `downstream` for OpenShift — affects cluster discovery |
| Namespace not cleaned up | Framework auto-cleans on success; failed runs may leave namespaces behind |
