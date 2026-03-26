<div align="center"><h1>Distributed tracing</h1></div>

The operator emits OpenTelemetry spans for build and integration-test PipelineRuns it reconciles, and propagates the trace context forward onto Snapshots and downstream Release CRs so a single trace can span the build → test → release lifecycle.

## Configuration

| Env var | Purpose | Default |
|---|---|---|
| `OTEL_EXPORTER_OTLP_ENDPOINT` | OTLP/gRPC collector URL. Unset disables tracing (noop provider). | *(unset)* |
| `OTEL_TRACES_SAMPLER` | `always_on`, `always_off`, `traceidratio`, `parentbased_always_off`, `parentbased_traceidratio`. | `parentbased_always_on` |
| `OTEL_TRACES_SAMPLER_ARG` | Ratio for ratio-based samplers (e.g. `0.1`). | *(unused unless a ratio sampler is selected)* |
| `TRACING_LABEL_ACTION` | PipelineRun label read to populate `cicd.pipeline.action.name`. Empty string disables the attribute. | `delivery.tekton.dev/action` |
| `TRACING_LABEL_APPLICATION` | PipelineRun label read to populate `delivery.tekton.dev.application`. Empty string disables the attribute. | `delivery.tekton.dev/application` |
| `TRACING_LABEL_COMPONENT` | PipelineRun label read to populate `delivery.tekton.dev.component`. Empty string disables the attribute. | `delivery.tekton.dev/component` |

## Emitted spans

Two spans are emitted per PipelineRun when it completes:

- `waitDuration` — `pr.CreationTimestamp` → `pr.Status.StartTime`
- `executeDuration` — `pr.Status.StartTime` → `pr.Status.CompletionTime`

The build-pipeline and integration-pipeline controllers each emit for their respective PipelineRun types. The `tekton.dev/timingEmitted` annotation guards against re-emission on subsequent reconciles.

## Trace-context propagation

Parenting follows the W3C Trace Context in the `tekton.dev/pipelinerunSpanContext` annotation. The annotation is propagated across resource boundaries so a single trace covers the full delivery flow:

```
build PipelineRun (annotation set by upstream)
   └── Snapshot (annotation copied from the build PipelineRun)
         ├── integration-test PipelineRun (annotation copied from the Snapshot)
         └── Release CR (annotation copied from the Snapshot)
```

When the annotation is absent, spans are still emitted but without a parent.

## Span attributes

| Attribute | Span | Source |
|---|---|---|
| `namespace` | both | `pr.GetNamespace()` |
| `pipelinerun` | both | `pr.GetName()` |
| `delivery.tekton.dev.pipelinerun_uid` | both | `pr.GetUID()` |
| `cicd.pipeline.action.name` | both | PipelineRun label (name configurable via `TRACING_LABEL_ACTION`) |
| `delivery.tekton.dev.application` | both | PipelineRun label (name configurable via `TRACING_LABEL_APPLICATION`) |
| `delivery.tekton.dev.component` | both | PipelineRun label (name configurable via `TRACING_LABEL_COMPONENT`) |
| `cicd.pipeline.result` | execute | `Succeeded` condition mapped to the semconv `cicd.pipeline.result` enum (`success` / `failure` / `timeout` / `cancellation` / `error`) |
| `delivery.tekton.dev.result_message` | execute | Earliest failing TaskRun's `Succeeded` condition message, falling back to the PipelineRun's own condition message. Omitted on success; truncated to 1024 bytes (UTF-8 safe). |
