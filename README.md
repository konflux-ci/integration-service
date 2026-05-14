# Konflux Integration Service
The [Konflux](https://konflux-ci.dev/) Integration Service is a Kubernetes operator to control the integration and testing of Konflux-managed Component builds in Red Hat Konflux.

## Overview

The Integration Service monitors Konflux builds in the form of Tekton PipelineRun CRs which produce container images and automatically creates Snapshots, Tekton PipelineRuns, and Release CRs.
The service handles the complete lifecycle of integration testing including grouping of images into Snapshots, execution of integration test Tekton PipelineRuns, and optional auto-release of Snapshot resources via the [Konflux Release Service](https://github.com/konflux-ci/release-service).

### Key Features

- **Automated Snapshot Creation**: Automatically creates Snapshots in response to Component builds based on user's Application or ComponentGroup CR configuration
- **Integration Testing**: Executes integration testing Tekton pipelines based on the user's configuration provided in IntegrationTestScenario CRs
- **Status Reporting to Git Providers**: Reports testing outcomes to the supported git providers
- **Automated Releases**: Automatically releases individual Snapshots based on user's configuration

## Architecture

The Integration Service consists of six main controllers:

### Build Pipeline Controller

Monitors build Tekton PipelineRun CRs and manages the following:
- Automatically creates Snapshots for the newly built Component images/artifacts
- Updates the Component's candidate image in case of on-push builds
- Reports the expected testing pipelines to the git provider, including possible PR groups for linked build PipelineRuns
- Cancels in-flight testing for Snapshots that were superseded by newer ones

### Snapshot Controller

Monitors Snapshot CRs and manages the following:
- Ensures that all Integration pipeline runs associated with the Snapshot and IntegrationTestScenarios are started
- Ensures that any requested re-runs of integration testing pipelines are triggered
- Creates automated Releases for Snapshots that pass all required integration tests if so configured
- Creates PR Group Snapshots if necessary
- Manages override Snapshots and uses them to reset the Global Candidate List(s) for Components

### Integration Pipeline Controller

Monitors integration Tekton PipelineRun CRs and manages the following:
- Checks the status and parses the standardized outputs of the integration PipelineRun
- Records the integration testing outcome and testing details in the associated Snapshot
- Ensures that the integration pipeline run log URL is annotated if available

### Status Report Controller

Monitors Snapshot CRs and manages the following:
- Reports the Snapshot's testing status for individual integration PipelineRuns to associated git provider
- Records the Snapshot's overall testing status based on the status of required integration test pipelines
- Reports the PR group testing status to the associated git provider

### ComponentGroup Controller

Ensures that the ComponentGroup/s Global Candidate List is aligned with its component list.

### Component Controller

Regenerates the latest Snapshot in case of component deletion based on the Application contents. Planned to be deprecated in favor of the ComponentGroup model.

## Dependencies

The Integration Service depends on the following Konflux components:

### Required Dependencies

- **[Pipeline Service](https://github.com/konflux-ci/architecture/blob/main/architecture/core/pipeline-service.md)** (Core Service)
  - Provides Tekton Pipelines for pipeline execution
  - Provides Pipelines as Code for webhook-driven builds
  - Provides Tekton Chains for signing and attestations
  - Provides Tekton Results for pipeline logging and archival

- **[Application API](https://github.com/konflux-ci/application-api)**
  - Provides Application, Component, and Snapshot custom resource definitions
  - Integration service creates Snapshot CRs based on Component and Application information
  - Note: Support for Application CRs is planned to be deprecated

- **[Release Service](https://github.com/konflux-ci/release-service)**
  - Provides ReleasePlan and Release CRs needed for automated Release creation

## Running, building and testing the operator

This operator provides a [Makefile](Makefile) to run all the usual development tasks. This file can be used by cloning
the repository and running `make` over any of the provided targets.

### Running the operator locally

When testing locally (eg. a CRC cluster), the command `make run install` can be used to deploy and run the operator.
If any change has been done in the code, `make manifests generate` should be executed before to generate the new
resources and build the operator.

### Build and push a new image

To build the operator and push a new image to the registry, the following commands can be used:

```shell
$ make img-build
$ make img-push
```

These commands will use the default image and tag. To modify them, new values for `TAG` and `IMG` environment variables
can be passed. For example, to override the tag:

```shell
$ TAG=my-tag make img-build
$ TAG=my-tag make img-push
```

Or, in the case the image should be pushed to a different repository:

```shell
$ IMG=quay.io/user/integration-service:my-tag make img-build
$ IMG=quay.io/user/integration-service:my-tag make img-push
```

### Adding/updating a dependency

This repo uses vendoring, please add dependencies in the following way:

```shell
go get example.com/dep@v1.2.3
go mod tidy
go mod vendor
git add vendor/
```

If you don't vendor dependencies, `go vet` will fail build.

### Running tests

To test the code, simply run `make test`. This command will fetch all the required dependencies and test the code. The
test coverage will be reported at the end, once all the tests have been executed.
