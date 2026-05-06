# AGENTS.md
## Overview

Integration Service is a Kubernetes operator that orchestrates integration testing pipelines in Konflux CI. It watches specific Tekton PipelineRun and Snapshot CRs, validates them, creates integration testing Tekton PipelineRuns, tracks their progress, records results, and reports them to the associated git provider if available. It also optionally creates Release CRs which start the automated release process for the given Snapshot. 
The [Konflux architecture documentation](https://github.com/konflux-ci/architecture) contains information about the broader scope of Konflux CI, while the [Konflux user documentation](https://github.com/konflux-ci/docs) contains the user facing docs.

## Technology Stack

- **Language**: Go
- **Framework:** controller-runtime
- **CRDs**: IntegrationTestScenario, ComponentGroup
- **Pipeline engine**: Tekton PipelineRuns
- **Testing**: Ginkgo/Gomega + envtest (local K8s API server)
- **Build**: `make test` (unit), `make manifests generate` (codegen)

## Repository Structure

```
api/v1beta2/           # CRD types
internal/controller/   # Reconcilers: buildpipeline/, component/, componentgroup/, integrationpipeline/, scenario/, snapshot/, statusreport/
internal/webhooks      # Webhooks
loader/                # ObjectLoader interface — abstracts K8s resource fetching
tekton/                # PipelineRun builders, status helpers, watch predicates
gitops/                # Snapshot and Release CR generation and management functions
pkg/                   # integrationteststatus/ and dag/ contain exported structs, metrics/ contains Prometheus gauges and histograms
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

### Resource Loading

`loader.ObjectLoader` centralizes all K8s Gets with error classification (retriable vs permanent) and KubeArchive fallback for deleted resources. A mock implementation exists for tests.

### Additional Key Patterns

- **PipelineRunBuilder** (`tekton/`): fluent API — `.WithExtraParams()`, `.WithSnapshot()`, `.WithIntegrationTimeouts()`, `.WithUpdatedPipelineGitResolver()`
- **Metrics**: registered during reconciliation — `RegisterCompletedSnapshot()`, `RegisterInvalidSnapshot()`, `RegisterPipelineRunStarted()`, `RegisterIntegrationResponse()`, with start/completion times

## Development Guidelines

- See `CONTRIBUTING.md` for overall guidelines for making contributions to this repository.
- **Git**: conventional commits with Jira ticket as scope — `type(issue-id): description` (e.g. `feat(STONEINTG-1519): create PR group snapshots from ComponentGroups`)
- Follow [Kubernetes coding conventions](https://github.com/kubernetes/community/blob/master/contributors/guide/coding-conventions.md)
- Log via `IntegrationLogger` from `helpers/logs` added as the adapter's `logger`, wrap errors with `fmt.Errorf("context: %w", err)`
- **API changes**: edit types in `api/v1beta2/`, then `make generate manifests`
- **Controller changes**: implement in `internal/controller/<resource>/<resource>_adapter.go`
- **Webhooks**: add to `internal/webhooks/<resource>/`
- **Tests**: unit tests alongside code using Ginkgo + envtest; E2E tests are located in `e2e-tests/tests`