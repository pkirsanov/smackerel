# Spec: BUG-069-003 Direct `POST /api/capture` MUST survive client disconnect

Links: [bug.md](bug.md) | [design.md](design.md) | [scopes.md](scopes.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md) | [scenario-manifest.json](scenario-manifest.json)

## Context

Parent spec `specs/069-assistant-http-transport` introduced the assistant HTTP
transport. Hard Constraint 5 ("Capture-as-fallback preserved") and the parent
`state.json` `policySnapshot.captureAsFallback: "inviolable"` require that a
user's capture is durably persisted exactly once and never lost. The **direct**
capture endpoint `POST /api/capture` is served by
`internal/api/capture.go::(*Dependencies).CaptureHandler`, which dispatches the
user's content into the durable pipeline with the HTTP **request** context
(`d.Pipeline.Process(r.Context(), …)`). `net/http` cancels `r.Context()` the
instant the client connection drops, so a capture client (chrome extension /
PWA / API caller) that disconnects after the body is received but before the
durable write completes aborts `Process` with `context.Canceled` and the capture
is silently lost.

This is the **same root cause** as the already-fixed
[BUG-069-002](../BUG-069-002-capture-context-cancellation/bug.md) (the assistant
HTTP adapter's capture-as-fallback branch, `internal/assistant/httpadapter/adapter.go`),
but at the **DIRECT `/api/capture` endpoint**, which was **never in
BUG-069-002's scope**. Discovered by an independent **bubbles.code-review
MVP/<deploy-host> readiness sweep**, finding **F-01**; finding id
**F-069-CR-CAPTURE-ENDPOINT-CTX-CANCEL**. This bug spec defines the production
behavior the fix must guarantee.

## Expected Behavior

### EB-1 — Capture survives client disconnect

`POST /api/capture` MUST invoke the durable pipeline write on a context that is
NOT cancelled by client disconnect or request-deadline expiry, so the durable
write (production: a Postgres `INSERT` plus a NATS publish, both
context-honoring) completes and the user's content is persisted. A client that
disconnects after the body is received MUST NOT cause the capture to be dropped.

### EB-2 — Request-scoped values preserved

The decoupled capture context MUST still carry request-scoped values — in
particular the chi request id read by `middleware.GetReqID` (consumed by
`pipeline.submitForProcessing` for the artifact `TraceID`) — so capture
provenance / trace correlation is unbroken.

### EB-3 — Capture invoked exactly once; no double-capture

The fix MUST NOT change the capture invocation count. `Process` is still invoked
exactly once per `POST /api/capture` (BS-001 idempotency); the change is solely
the cancellation semantics of the context handed to it.

### EB-4 — Response path and other endpoints unchanged

The response-path DB-health re-check (`d.DB.Healthy(r.Context())`) and all
`writeJSON` / `writeError` calls stay request-scoped (correct best-effort
response behavior). Non-capture handlers and the Telegram path MUST be
unaffected. The change is confined to the durable `Process` call in
`CaptureHandler`.

## Acceptance Criteria

- AC-1: With the request context cancelled before the durable write, the
  pipeline is still invoked AND the context it receives is not cancelled
  (`ctx.Err() == nil`), and the original prompt text is passed through intact.
  [EB-1, EB-2]
- AC-2: A pre-fix RED proof exists — the adversarial regression test FAILS
  against `d.Pipeline.Process(r.Context(), …)` (captured `context canceled`). [EB-1]
- AC-3: A post-fix GREEN proof exists — the same test PASSES after the swap to
  `d.Pipeline.Process(context.WithoutCancel(r.Context()), …)`. [EB-1, EB-2]
- AC-4: The `internal/api` package (the existing `TestCaptureHandler_*` family
  plus the new regression) passes — no regression, capture still invoked exactly
  once. [EB-3, EB-4]
- AC-5: The change is confined to `internal/api/capture.go` (the `"context"`
  import + the one durable `Process` call line + an explanatory comment) plus the
  new test file; no other spec folder, the Telegram adapter, the wire schema, the
  SST contract, `internal/pipeline/*`, or `config/smackerel.yaml` is touched. [EB-4]

## Out Of Scope

- Adding a capture timeout / new SST key — `context.WithoutCancel` strips the
  deadline; the downstream pgx pool and NATS client carry their own timeouts. A
  configurable capture deadline is a separate enhancement, not this defect.
- Making capture asynchronous / fire-and-forget — capture remains synchronous so
  the rendered `CaptureResponse` still reflects a completed capture; only the
  cancellation coupling is removed.
- The assistant `/api/assistant/turn` capture-as-fallback branch — already fixed
  in [BUG-069-002](../BUG-069-002-capture-context-cancellation/spec.md).
- Any edit to `internal/pipeline/*`, the Telegram adapter, the wire schema, the
  SST contract, or `config/smackerel.yaml`.

## Product Principle Alignment

- **P9 Design For Restart, Not Perfection** — a flaky extension / PWA / mobile
  connection must not punish the user by silently dropping a captured idea/page/
  voice note; capture-as-fallback is the safety net and must hold even when the
  client vanishes mid-capture.
- **P8 Trust Through Transparency** — the inviolable capture contract the product
  advertises must actually hold under client disconnect at EVERY capture surface,
  not only on the happy path and not only at the assistant endpoint.
