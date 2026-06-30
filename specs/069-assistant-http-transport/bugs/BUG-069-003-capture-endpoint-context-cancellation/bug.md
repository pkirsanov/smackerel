# Bug: BUG-069-003 The direct `POST /api/capture` handler binds the durable capture write to the request context (`r.Context()`), so a client disconnect during a capture cancels the durable write and silently loses the capture — violating the inviolable capture-as-fallback contract

Links: [spec.md](spec.md) | [design.md](design.md) | [scopes.md](scopes.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md) | [scenario-manifest.json](scenario-manifest.json)

## Summary

The direct capture endpoint `POST /api/capture` is served by
`internal/api/capture.go::CaptureHandler`, which dispatches the user's content
into the durable pipeline with the HTTP **request** context:

```go
result, err := d.Pipeline.Process(r.Context(), &pipeline.ProcessRequest{...})
```

`pipeline.Processor.Process` performs a **context-honoring** Postgres `INSERT`
(`storeInitialArtifact`) and a **context-honoring** NATS publish
(`submitForProcessing`, `internal/pipeline/processor.go` ~L471). `net/http`
cancels `r.Context()` the instant the client connection drops. So if a capture
client (chrome extension / PWA / API caller) disconnects, or its request
deadline fires, **after** the body is received but **before** the durable write
completes, `Process` aborts with `context.Canceled` and **the capture is
silently lost** — the exact failure the capture-as-fallback contract exists to
prevent.

This is the **same root cause** as the already-fixed
[BUG-069-002](../BUG-069-002-capture-context-cancellation/bug.md)
(`internal/assistant/httpadapter/adapter.go` was changed to
`a.capture(context.WithoutCancel(r.Context()), …)`), but at the **DIRECT
`/api/capture` endpoint**, which was **never in BUG-069-002's scope** (that
packet only touched the assistant HTTP adapter's capture-as-fallback branch).
It violates the **same** inviolable capture-as-fallback contract
(`policySnapshot.captureAsFallback: "inviolable"`, Hard Constraint 5 / BS-001 —
"the user's prompt is never lost").

Discovered by an independent **bubbles.code-review MVP/evo-x2 readiness sweep**,
finding **F-01**. Finding id: **F-069-CR-CAPTURE-ENDPOINT-CTX-CANCEL**.

## Severity

- [ ] Critical — System unusable, data loss
- [ ] High
- [x] **Medium** — Silent, per-occurrence loss of user content (a captured idea/page/voice note) on a specific but realistic race: client disconnect / request-deadline expiry during a `POST /api/capture` after the body is received but before the durable write completes. It violates the **inviolable** capture-as-fallback contract (`policySnapshot.captureAsFallback: "inviolable"`, Hard Constraint 5, BS-001), which keeps it well above Low. It is not High/Critical because it is bounded to the disconnect race on a single in-flight capture (not all captures, no systemic corruption, no multi-record loss); flaky-network extension/PWA/mobile clients are the realistic trigger. The Telegram adapter is **not** exposed (long-poll background context, not a per-request HTTP context that dies on disconnect).
- [ ] Low

## Status

- [x] Reported
- [x] Confirmed (reproduced with an adversarial regression test that cancels the request context — RED captured below)
- [x] In Progress
- [x] Fixed (`d.Pipeline.Process(context.WithoutCancel(r.Context()), …)` + added the missing `context` import — decouples the durable capture write from request cancellation while preserving request-scoped values)
- [x] Verified (unit) — RED-before / GREEN-after + handler-family regression green, no regression
- [ ] Verified (live stack) — shared live-stack E2E disconnect-race regression routed to `bubbles.test` (no live stack this session)
- [ ] Closed (pending validate-owned done-ceiling certification)

## Reproduction Steps

1. Build a `Dependencies` with a healthy stub `DB` (`mockDB{healthy: true}`) and a stub `Pipeline` (`recordingPipeline`) whose `Process` records the context it receives.
2. Construct a valid `POST /api/capture` request (`{"text":"durable idea"}`, `Content-Type: application/json`), then **cancel** its context to model the client disconnecting after the body is received but before the durable write.
3. Call `deps.CaptureHandler(w, r.WithContext(cancelledCtx))`.
4. Observe: the pipeline IS invoked, but the context handed to `Process` carries `context.Canceled` (`ctx.Err() != nil`). In production, `pipeline.Processor.Process` then aborts its Postgres `INSERT` / NATS publish on that cancelled context and the capture is dropped (the synchronous handler may still try to render a response to the gone client, but the durable capture is lost).

Pre-fix automated reproduction (RED), `internal/api/capture_disconnect_test.go`:

```text
=== RUN   TestCaptureHandler_CaptureSurvivesClientDisconnect
    capture_disconnect_test.go:95: durable capture write ran with a cancelled context (err=context canceled); a client disconnect MUST NOT abort the /api/capture durable write (F-069-CR-CAPTURE-ENDPOINT-CTX-CANCEL)
--- FAIL: TestCaptureHandler_CaptureSurvivesClientDisconnect (0.00s)
FAIL
FAIL    github.com/smackerel/smackerel/internal/api     0.076s
=== RED run exit: 1 ===
```

## Expected Behavior

Capture is **inviolable**: a `POST /api/capture` MUST durably persist the user's
content exactly once, **even if the HTTP client has already disconnected**. The
durable write (a Postgres `INSERT` plus a NATS publish, both context-honoring)
MUST run on a context that is NOT cancelled by client disconnect /
request-deadline expiry, while still preserving request-scoped values (request
id / trace correlation via `middleware.GetReqID`, consumed by
`submitForProcessing` for the artifact `TraceID`). The synchronous response to
the (possibly gone) client is best-effort and moot; the durable capture is not.

## Actual Behavior

`CaptureHandler` calls `d.Pipeline.Process(r.Context(), …)`. `r.Context()` is
cancelled by `net/http` on client disconnect. The pipeline write
(`Process` → `submitForProcessing` → `storeInitialArtifact` Postgres `INSERT` +
`NATS.Publish(ctx, …)`) is context-honoring and aborts with `context.Canceled`,
and the capture is gone. No retry, no background re-dispatch.

## Environment

- Repo: smackerel (Go core runtime), Go 1.25.10, current working tree.
- Sweep: **bubbles.code-review MVP/evo-x2 readiness sweep**, finding **F-01**, parent spec `specs/069-assistant-http-transport` (`status: done`, `workflowMode: full-delivery`), release train `mvp`.
- Defect site: `internal/api/capture.go` — `(*Dependencies).CaptureHandler`, the `d.Pipeline.Process(r.Context(), …)` durable dispatch.
- Production durable path: `internal/pipeline/processor.go::Process` → `submitForProcessing` → `storeInitialArtifact` (Postgres `INSERT`, ctx-honoring) + `p.NATS.Publish(ctx, …)` (ctx-honoring).
- Contract violated: spec 069 Hard Constraint 5 (capture-as-fallback preserved); BS-001; parent `state.json policySnapshot.captureAsFallback: "inviolable"`.
- Sibling: [BUG-069-002](../BUG-069-002-capture-context-cancellation/bug.md) fixed the SAME root cause at `internal/assistant/httpadapter/adapter.go` (`/api/assistant/turn`); this packet covers the direct `/api/capture` endpoint that BUG-069-002 did not.

## Five Whys

1. **Why is the capture lost?** The durable write aborts with `context.Canceled`.
2. **Why does it abort?** It runs `pipeline.Process` on `r.Context()`, which `net/http` cancels on client disconnect.
3. **Why is `r.Context()` used for a durable side-effect?** `CaptureHandler` renders the result synchronously and reused the request context for the durable `Process` call without distinguishing "respond to this client" (request-scoped) from "persist the capture" (must outlive the client).
4. **Why wasn't it caught (incl. by BUG-069-002)?** BUG-069-002 fixed the assistant adapter's capture-as-fallback branch but its scope and regression were confined to `internal/assistant/httpadapter`; the **direct** `/api/capture` handler shares the identical anti-pattern and had no disconnect regression. The existing `internal/api` capture tests never cancel the request context.
5. **Root cause:** the inviolable capture side-effect was coupled to HTTP request liveness at a *second* call site; a durable write must be decoupled from request cancellation everywhere it occurs.

## Fix

`internal/api/capture.go::CaptureHandler` — run the durable `Process` call on a
cancellation-decoupled context that preserves request-scoped values, and add the
(previously absent) `context` import:

```go
// before (the defect):
result, err := d.Pipeline.Process(r.Context(), &pipeline.ProcessRequest{...})

// after (the fix):
result, err := d.Pipeline.Process(context.WithoutCancel(r.Context()), &pipeline.ProcessRequest{...})
```

`context.WithoutCancel` (Go 1.21+; repo is Go 1.25.10) returns a context that is
never cancelled by the parent but still carries the parent's Values — so
`middleware.GetReqID(ctx)` (used by `submitForProcessing` for the artifact
`TraceID`) and any downstream request-scoped values survive, while a client
disconnect can no longer abort the durable capture write. `capture.go` did **not**
previously import `context`, so the fix also adds `"context"` to the import
block. Only the durable `Process` call changes; the response-path DB health
re-check (`d.DB.Healthy(r.Context())`) and all `writeJSON` / `writeError` calls
stay request-scoped (correct best-effort response-path behavior). The Telegram
path is unaffected.
