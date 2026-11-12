# Nudge batch Phase 2 happy path (ADR 0072)

Manual verification on a Konflux dev cluster (L5 tenant). Complements the automated happy-path test in `tekton/nudging/nudge_batch_happy_path_test.go`, which runs in CI on every PR.

## Prerequisites

- integration-service deployed with Phase 2 batch nudging (`NudgeConfig` CRD, nudgeconfig controller, build-pipeline batch hooks).
- A tenant namespace with the singleton `NudgeConfig` named **`nudge-config`** (required by CRD validation), at least one **target** component listed under `spec.targetConfig` with `batchPolicy`, and three **source** components with `spec.nudges` relationships into that target (same layout as your usual nudge app).
- RBAC to read `NudgeConfig` / `PipelineRun` and to run or observe build pipelines in that namespace. You do **not** need to patch `NudgeConfig.status` manually; the operator updates it.

**Note:** If a nudge uses `mode: validated`, builds are recorded for batching only after integration tests pass for that source. Use immediate nudge mode or pre-passing integration status when following this runbook literally.

## 1. Configure batching

Apply or patch the singleton in the tenant namespace:

```yaml
apiVersion: appstudio.redhat.com/v1beta2
kind: NudgeConfig
metadata:
  name: nudge-config   # required singleton name
  namespace: <tenant-ns>
spec:
  batchDefaults:
    debounceTimeout: 2m
    maxWaitTime: 4h
  targetConfig:
    - target: <target-component-name>
      batchPolicy: {}   # empty object opts the target into batching
  nudges:
    # keep or add from/to edges: each source → target
    - from: <source-a>
      to: <target-component-name>
    - from: <source-b>
      to: <target-component-name>
    - from: <source-c>
      to: <target-component-name>
```

Per-target overrides (optional): set `debounceTimeout` / `maxWaitTime` on `targetConfig[].batchPolicy` instead of `batchDefaults`; resolved values must still satisfy ADR 0072 (debounce ≥ 1m, maxWait > debounce).

Confirm webhook acceptance:

```bash
kubectl get nudgeconfig nudge-config -n <tenant-ns> -o yaml
```

## 2. Trigger three build completions

For each source, run or wait for a **successful** build `PipelineRun`. Wait **~30 seconds** between completions if you want to see debounce resets in status (optional for the final outcome).

After each build finishes:

```bash
kubectl get nudgeconfig nudge-config -n <tenant-ns> -o jsonpath='{.status.activeBatches}' | jq .
```

Expect one batch with `"phase": "Accumulating"` for the target, growing to **three** objects in `accumulated` (distinct `from`, `imageDigest`, `buildPipelineRun`).

## 3. Wait for debounce (2 minutes of silence)

Stop new builds for that target. After **2 minutes** from the last recorded build, the nudgeconfig reconciler should call `ProcessDueNudgeBatches` on a wake-up (requeue interval is at least **5s** while batches are pending, so allow a short buffer after the 2m debounce).

Watch phase and nudge PLR name:

```bash
watch -n5 "kubectl get nudgeconfig nudge-config -n <tenant-ns> -o jsonpath='{range .status.activeBatches[*]}{.phase}{\" \"}{.nudgePipelineRun}{\"\\n\"}{end}'"
```

Expect:

- `phase`: `Firing`
- `nudgePipelineRun`: name of the aggregated Renovate `PipelineRun`

Inspect the PLR:

```bash
NPLR=$(kubectl get nudgeconfig nudge-config -n <tenant-ns> -o jsonpath='{.status.activeBatches[0].nudgePipelineRun}')
kubectl get pipelinerun "$NPLR" -n <tenant-ns> -o yaml
```

Verify annotations (see `tekton/consts/consts.go` and `CreateBatchedNudgePipelineRun`):

| Annotation | Expected |
|------------|----------|
| `build.konflux-ci.dev/batch-id` | Same as `.status.activeBatches[0].batchID` |
| `build.appstudio.redhat.com/nudged-components` | Target component **name** (single string) |
| `build.konflux-ci.dev/nudging-components` | JSON **array** of the three source component names |
| `build.konflux-ci.dev/nudging-digests` | JSON **object** mapping each source name → image digest |

## 4. Complete the nudge PipelineRun

**If Renovate runs successfully in the cluster:** wait for the PLR condition `Succeeded=True` and a `completionTime` (required before the batch moves to `Completed`).

**If Renovate cannot run in dev**, patch status like the envtest (condition type `Succeeded`, status `True`, plus `completionTime`):

```bash
kubectl patch pipelinerun "$NPLR" -n <tenant-ns> --type=merge --subresource=status -p \
  '{"status":{"conditions":[{"type":"Succeeded","status":"True","reason":"Completed"}],"completionTime":"'"$(date -u +%Y-%m-%dT%H:%M:%SZ)"'"}}'
```

While the batch is `Firing`, the nudgeconfig controller requeues about every **5s** and runs `ProcessFiringNudgeBatches`; you should not need to touch the `NudgeConfig` unless reconciliation is stuck.

Expect:

```bash
kubectl get nudgeconfig nudge-config -n <tenant-ns> -o jsonpath='{.status.activeBatches[0].phase}'
# Completed
```

## 5. Sign-off checklist

| Step | Expected |
|------|----------|
| `nudge-config` + `batchPolicy` valid | Webhook accepts; target batched |
| 3 successful build PLRs (optional 30s spacing) | 3 `accumulated` entries |
| 2m debounce (+ short reconcile buffer) | One aggregated Renovate PLR; batch `Firing` |
| PLR annotations | batch-id, target on `nudged-components`, 3 sources/digests |
| Nudge PLR succeeded | Batch `Completed` |

Record namespace, component names, batch ID, and PLR name in [KFLUXSE-491](https://redhat.atlassian.net/browse/KFLUXSE-491) when demoing.

## Automated CI counterpart

`make test` runs the `Nudge batch Phase 2 happy path (ADR 0072)` spec in `tekton/nudging/nudge_batch_fire_test.go`, which walks the same lifecycle with a fake client and patches `fireAt` into the past instead of waiting on a real debounce timer.
