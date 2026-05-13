# Design: Deploy Resource and Filesystem Hardening

## Current Truth

The review identified three hardening gaps: service CPU envelopes are not explicit, the ML model memory posture is not contractually tied to a deployment resource class, and container writable surfaces are not constrained with a read-only root filesystem contract.

This design keeps the product/adapter boundary intact. Smackerel should publish resource and filesystem requirements through its SST and deploy contract surfaces. The target adapter decides how those requirements map to the host.

## Proposed Design

### Resource Envelope

- Add product-owned configuration keys for CPU and memory expectations per runtime service.
- Generate those values into product-owned deployment contracts or compose surfaces.
- Add contract tests that parse the generated runtime/deploy surface and fail when required limits are absent.

### ML Model Envelope

- Define model memory profiles for accepted operator-provided model classes.
- Validate the configured ML model against an explicit deployment resource class or configured memory budget.
- Fail before runtime start if the configured model exceeds the declared envelope.

### Filesystem Envelope

- Set read-only root filesystem flags where services do not require root writes.
- Declare writable paths as explicit tmpfs mounts or named volumes.
- Keep PostgreSQL, NATS, model cache, and ingestion scratch paths writable only through named surfaces.

## Test Strategy

| Test ID | Type | Purpose |
|---------|------|---------|
| T-045-001 | unit/config | Validate resource limit values are parsed from SST and reject missing values. |
| T-045-002 | contract | Parse compose/deploy output and assert required service resource limits exist. |
| T-045-003 | unit/config | Reject ML model choices that exceed the declared memory envelope. |
| T-045-004 | integration | Start the stack and assert read-only-root containers cannot write outside explicit writable paths. |
| T-045-005 | artifact | Artifact lint passes for this planning packet. |

## Risk Controls

- Keep writable stateful-service data paths explicit and untouched by read-only-root tightening.
- Preserve generated config ownership: no hand-edited generated files.
- Treat model-envelope failure as startup validation, not runtime degradation.
