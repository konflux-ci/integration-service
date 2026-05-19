---
name: debugging-running-instance
description: Use when debugging integration-service running on a cluster. Covers pod logs, health probes, metrics, webhooks, network policies, environment variables, snapshot GC, and common failure modes.
---

# Debugging a Running Instance

## Overview

The integration-service runs as a Deployment in the `integration-service` namespace. It exposes health probes, Prometheus metrics, and optional webhooks. A separate CronJob handles snapshot garbage collection.

## When to Use

- Controller not reconciling resources as expected
- Webhooks rejecting or silently ignoring mutations/validations
- Metrics not being scraped
- Status reports not appearing on PRs
- Snapshot GC not cleaning up old snapshots

## Quick Reference

| What | How |
|------|-----|
| Controller logs | `kubectl logs -n integration-service deploy/controller-manager -f` |
| Health check | `kubectl get -n integration-service deploy/controller-manager -o jsonpath='{.status.conditions}'` |
| Readiness probe | HTTP GET `/readyz` on port 8081 |
| Liveness probe | HTTP GET `/healthz` on port 8081 |
| Metrics | Port 8080 (HTTPS, self-signed cert), scraped at `/metrics` |
| Webhook port | 9443 (when enabled) |
| Leader election | Lease-based, 15s lease duration, 10s renew deadline, 2s retry period |
| Pod label | `control-plane: controller-manager` |

## Environment Variables

### Controller

| Variable | Default | Effect if wrong |
|----------|---------|----------------|
| `CONSOLE_URL` | none | PR status comments show `CONSOLE_URL_NOT_AVAILABLE` |
| `CONSOLE_URL_TASKLOG` | none | Task log links show `CONSOLE_URL_TASKLOG_NOT_AVAILABLE` |
| `CONSOLE_NAME` | none | Console display name missing from PR comments |
| `PIPELINE_TIMEOUT` | none | Invalid duration is logged as error and skipped |
| `TASKS_TIMEOUT` | none | Invalid duration is logged as error and skipped |
| `FINALLY_TIMEOUT` | none | Invalid duration is logged as error and skipped |
| `INTEGRATION_NS` | `integration-service` | Wrong namespace for PAC secret lookup |
| `PAC_SECRET` | `pipelines-as-code-secret` | Can't authenticate to git providers |

## Network Policies

Namespaces must be explicitly labeled for traffic to reach the controller:

| Traffic | Required label |
|---------|---------------|
| Metrics scraping | `metrics: enabled` on source namespace |
| Webhook calls | `webhook: enabled` on source namespace |

Missing labels = silently blocked traffic. Check with: `kubectl get ns <name> --show-labels`

## Webhook Behavior

| Webhook | Type | Failure Policy | Effect |
|---------|------|---------------|--------|
| IntegrationTestScenario | Mutating | **Ignore** | Defaults applied silently on failure |
| IntegrationTestScenario | Validating | **Fail** | Rejects invalid ITS (bad names, conflicting resolver params) |
| ComponentGroup | Validating | **Fail** | Blocks invalid ComponentGroups |
| Snapshot | Mutating + Validating | **Ignore** | Failures silently pass through (by design) |

## Debugging Checklist

1. **Controller not starting?** Check env var values and RBAC — timeout parsing errors are logged, not fatal
2. **Not reconciling?** Check leader election lease, RBAC, and that CRDs are installed
3. **PR status missing?** Check `CONSOLE_URL` env var and PAC secret availability
4. **Webhooks not firing?** Check namespace labels (`webhook: enabled`) and cert-manager status
5. **Metrics missing?** Check namespace labels (`metrics: enabled`) and ServiceMonitor exists
6. **Snapshots piling up?** Check `snapshotgc` CronJob logs — runs every 6h with separate service account

## Common Mistakes

| Problem | Fix |
|---------|-----|
| `CONSOLE_URL_NOT_AVAILABLE` in PR comments | Set `CONSOLE_URL` env var with `{{NAMESPACE}}` and `{{PIPELINE_RUN_NAME}}` placeholders |
| Controller crash loop | Check env var values — timeout vars must be valid Go durations (e.g., `2h`, `30m`) |
| Webhook silently not validating Snapshots | By design — Snapshot webhook has `failurePolicy: Ignore` |
| Metrics endpoint unreachable | Verify namespace label `metrics: enabled` and that ServiceMonitor target matches |
