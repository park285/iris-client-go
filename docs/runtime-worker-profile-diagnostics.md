# Runtime worker profile diagnostics helpers

`iris-client-go` exposes Iris `/diagnostics/runtime` as raw JSON for full compatibility. For bot legacy fadeout work, consumers also need a stable typed path to the worker profile contract emitted at:

```text
workers.webhook.webhookPipeline
```

This SDK now provides:

```go
pipeline, err := iris.DecodeIrisBotWebhookPipelineDiagnostics(raw)
```

For concrete `*iris.H2CClient` values, the fetch and decode steps can be combined:

```go
pipeline, err := client.GetIrisBotWebhookPipelineDiagnostics(ctx)
```

The decoder validates that the strict-mode fields are present:

- `profileVersion`
- `profileId`
- `profileHash`
- `workerProfile.version`
- `workerProfile.profile_id`
- `workerProfile.bot_pool`
- breaker settings in both raw `workerProfile.delivery` and flattened `delivery`

Missing pipeline/profile data returns `ErrRuntimeWorkerProfileDiagnosticsMissing`, so bot consumers can fail startup intentionally instead of silently falling back to hardcoded worker settings.

The existing raw JSON method remains unchanged:

```go
raw, err := client.GetRuntimeDiagnostics(ctx)
```

That preserves compatibility for dashboards and ad-hoc diagnostics while giving runtime services a narrow strict contract for the Iris worker profile fadeout.
