---
name: pr-definition-of-done
description: Use when preparing a pull request for review, before pushing, or when reviewing someone else's PR. Checklist of CI checks, commit conventions, code generation, testing, and documentation requirements.
---

# PR Definition of Done

## Overview

Every PR must pass CI checks, follow commit conventions, include tests, and keep generated code in sync. This checklist covers what CI enforces and what reviewers expect.

## When to Use

- Before pushing a PR for review
- When CI checks fail and you need to understand why
- When reviewing someone else's PR

## Pre-Push Checklist

### Commits
- [ ] Conventional format: `type(JIRA-ID): description` (e.g., `feat(STONEINTG-1519): create PR group snapshots`)
- [ ] Types: `feat`, `fix`, `chore`, `refactor`, `test`, `docs`
- [ ] Signed off (DCO): `git commit -s`
- [ ] Title < 72 chars, description lines < 72 chars
- [ ] AI-assisted work: add `Assisted-by: <tool-name>` trailer

### Code Generation (CI will fail if stale — see [ci-cd-quirks](../ci-cd-quirks/SKILL.md) for details)
- [ ] After API type changes in `api/v1beta2/`: `make generate manifests`
- [ ] After dependency changes: `go mod tidy && go mod vendor`
- [ ] Run `make fmt` — CI fails on uncommitted formatting changes
- [ ] Run `make vet` — catches common Go mistakes

### Testing
- [ ] Unit tests alongside code changes (Ginkgo + envtest)
- [ ] Cover: happy path, error conditions, edge cases, idempotency
- [ ] `make test` passes locally
- [ ] Coverage does not decrease (codecov allows 3% drop threshold)

### Security & RBAC
- [ ] No secrets, keys, or credentials committed
- [ ] No RBAC wildcards in `config/` (CI checks this)
- [ ] New resource types: add `+kubebuilder:rbac` markers

### Documentation
- [ ] PR description explains the "why", not just the "what"
- [ ] PR title < 72 chars
- [ ] Controller diagram in `docs/` updated if controller logic changed

## What CI Checks

| Check | Workflow | What fails it |
|-------|----------|---------------|
| Go tests | `pr.yaml` | `make test` failures |
| Formatting | `pr.yaml` | Uncommitted `make fmt` changes |
| Code generation | `pr.yaml` | Uncommitted `make generate manifests` output |
| `go mod tidy` | `pr.yaml` | Uncommitted tidy changes |
| RBAC wildcards | `pr.yaml` | Wildcard `*` in config RBAC files |
| Dockerfile lint | `pr.yaml` | hadolint violations |
| Go linters | `pr.yaml` | gosec, golangci-lint, staticcheck failures |
| CodeQL | `codeql.yml` | Security vulnerabilities |
| PR size | `size.yaml` | Oversized PRs (informational) |
| Coverage | `codecov.yml` | >3% coverage drop from base |
| Tekton build | `.tekton/` | Image build failure, security scans |

## Common Mistakes

| Mistake | Fix |
|---------|-----|
| Forgot `go mod vendor` | Run `go mod tidy && go mod vendor` after any dep change |
| Stale deepcopy | Run `make generate` after editing types in `api/v1beta2/` |
| Stale RBAC/CRD manifests | Run `make manifests` after changing `+kubebuilder` markers |
| Unsigned commit | Use `git commit -s` or amend with `git commit --amend -s` |
| Coverage dropped | Add tests for new code paths — check `cover.out` for uncovered lines |
