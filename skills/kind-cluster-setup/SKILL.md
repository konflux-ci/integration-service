---
name: kind-cluster-setup
description: Use when setting up a local development environment, deploying integration-service to a cluster, installing CRDs, or tearing down a Kind cluster. Covers make targets, kustomize deployment, and full Konflux stack setup.
---

# Kind Cluster Setup

## Overview

For local development you can run the controller against any cluster via `make run`, or deploy it fully with `make deploy`. The CI pipeline provisions Kind clusters on AWS for e2e tests using MAPT.

## When to Use

- Setting up a local dev environment
- Installing/uninstalling CRDs on a cluster
- Deploying/undeploying the full controller
- Understanding how CI provisions test clusters

## Quick Reference

| Command | What it does |
|---------|-------------|
| `make install` | Install CRDs into current cluster (from `config/crd/`) |
| `make uninstall` | Remove CRDs from cluster |
| `make run` | Run controller locally against current kubeconfig |
| `make deploy` | Deploy controller + RBAC + webhooks to cluster via kustomize |
| `make undeploy` | Remove deployed controller from cluster |
| `make img-build` | Build container image (default: docker) |
| `make img-push` | Push image to registry |
| `CONT_ENGINE=podman make img-build` | Build with podman instead of docker |

## Local Development (make run)

1. Ensure kubeconfig points to your target cluster
2. Install CRDs: `make install`
3. Install dependency CRDs (application-api, release-service, Tekton, PAC) — these must be on the cluster
4. Run: `make run` — starts controller with local binary, no container needed
5. Controller connects to cluster via kubeconfig, reconciles resources

## Full Deployment (make deploy)

1. Build and push image:
   ```
   make img-build IMG=quay.io/youruser/integration-service:dev
   make img-push IMG=quay.io/youruser/integration-service:dev
   ```
2. Deploy: `make deploy IMG=quay.io/youruser/integration-service:dev`
3. This creates namespace `integration-service-system` and deploys via kustomize
4. Teardown: `make undeploy`

## Required Dependency CRDs

The controller needs these CRDs on the cluster (not just locally for tests):

| Source | CRDs |
|--------|------|
| `konflux-ci/application-api` | Application, Component, Snapshot, ReleasePlan, Release |
| `konflux-ci/release-service` | Release, ReleasePlan, ReleasePlanAdmission |
| `tektoncd/pipeline` | PipelineRun, TaskRun, Pipeline, Task |
| `openshift-pipelines/pipelines-as-code` | Repository |
| This repo | IntegrationTestScenario, ComponentGroup |

## Webhook Considerations

- Webhooks require TLS certificates — cert-manager must be installed
- Cert-manager config is in `config/certmanager/` (commented out by default in kustomization)
- Without webhooks, the controller still works but won't validate ITS names or reject invalid resources
- Toggle via `ENABLE_WEBHOOKS` env var in deployment

## Common Mistakes

| Problem | Fix |
|---------|-----|
| Missing dependency CRDs | Install application-api, release-service, and Tekton CRDs on the cluster first |
| `make deploy` fails on image pull | Push image to accessible registry, or use `imagePullPolicy: IfNotPresent` for local images |
| Webhooks fail with TLS errors | Install cert-manager or disable webhooks |
| `make run` can't connect | Check `KUBECONFIG` env var or `~/.kube/config` points to correct cluster |
