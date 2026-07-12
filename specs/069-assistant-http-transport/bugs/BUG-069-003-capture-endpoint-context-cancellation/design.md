# Bug Fix Design: BUG-069-003 — decouple the direct `/api/capture` durable write from request cancellation

Links: [bug.md](bug.md) | [spec.md](spec.md) | [scopes.md](scopes.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md) | [scenario-manifest.json](scenario-manifest.json)

## Root Cause Analysis

### Investigation Summary

A **bubbles.code-review MVP/<deploy-host> readiness sweep** (finding **F-01**) read the
direct capture path top-to-bottom:

1. `internal/api/capture.go::(*Dependencies).CaptureHandler` handles
   `POST /api/capture` by dispatching the user's content into the durable
   pipeline with the HTTP request context:
   `d.Pipeline.Process(r.Context(), &pipeline.ProcessRequest{…})`.
2. `internal/pipeline/processor.go::Process` → `submitForProcessing` →
   `storeInitialArtifact(ctx, …)` runs a Postgres `INSERT` and
   `p.NATS.Publish(ctx, …)` runs a NATS publish — **both honor `ctx`**
   (processor.go ~L471).
3. `net/http` cancels `r.Context()` the moment the client connection drops or the
   request deadline fires.

This is the **identical anti-pattern** already fixed at the assistant endpoint in
[BUG-069-002](../BUG-069-002-capture-context-cancellation/design.md)
(`a.capture(context.WithoutCancel(r.Context()), …)`), but at the **direct**
`/api/capture` endpoint, which BUG-069-002 never touched.

### Root Cause

The **inviolable** capture side-effect was coupled to HTTP request liveness at a
**second** call site. `CaptureHandler` renders the `CaptureResponse`
synchronously and reused the request context (`r.Context()`) — correct for
"respond to this client" but wrong for "persist the user's capture", which must
outlive the client. When the client disconnects between body receipt and the
completion of the durable write, the cancelled context aborts the Postgres
`INSERT` / NATS publish and the capture is silently dropped. This directly
violates Hard Constraint 5, BS-001, and
`policySnapshot.captureAsFallback: "inviolable"`.

### Why Tests Missed It (test-gap analysis)

- The existing `internal/api` capture tests (`capture_test.go`,
  `health_test.go`) drive a live, connected client and **never cancel the request
  context**, so the disconnect race never fires.
- BUG-069-002's regression was confined to `internal/assistant/httpadapter`; it
  proved the assistant branch but could not observe the **direct** `/api/capture`
  handler, which shares the identical defect.
- No `internal/api` test asserted the capture-context cancellation invariant. The
  structural fix for the gap is a regression test that cancels the request
  context and asserts the durable write still runs with a live context.

### Impact Analysis

- **Affected component:** the direct capture endpoint
  (`internal/api/capture.go::CaptureHandler`).
- **Affected behavior:** durable persistence of a captured idea/page/voice note
  when the client disconnects mid-capture.
- **Affected users:** any extension / PWA / API capture client on a flaky network
  whose `POST /api/capture` is cancelled during the durable write. The Telegram
  path is **not** affected (long-poll background context, not a per-request HTTP
  context).
- **Affected data:** the single in-flight capture for that request (silent loss;
  no corruption, no multi-record loss).
- **Existing mitigations:** none — `Process` returns the cancellation error and
  the handler renders an error to the (already-gone) client; there is no retry or
  background re-dispatch, which is exactly why the loss is silent.

## Fix Design

### Solution Approach (chosen)

Hand the durable `Process` call a context that is decoupled from request
cancellation but preserves request-scoped values, at the single durable call
site, and add the (previously absent) `context` import:

```go
// internal/api/capture.go — (*Dependencies).CaptureHandler
//
// before (the defect):
result, err := d.Pipeline.Process(r.Context(), &pipeline.ProcessRequest{…})

// after (the fix):
// Capture-as-fallback is inviolable (Hard Constraint 5 / BS-001 /
// policySnapshot.captureAsFallback="inviolable"): the user's capture MUST
// persist even if the client has already disconnected. net/http cancels
// r.Context() the instant the connection drops, which would abort the
// downstream pipeline.Process Postgres INSERT (storeInitialArtifact) and NATS
// publish (submitForProcessing) — both context-honoring — and silently lose
// the capture. Decouple the durable write from request cancellation while
// preserving request-scoped values (request id / trace correlation via
// middleware.GetReqID, consumed by submitForProcessing for the artifact
// TraceID). Same root cause as BUG-069-002 (assistant /api/assistant/turn),
// here at the direct /api/capture endpoint — F-069-CR-CAPTURE-ENDPOINT-CTX-CANCEL.
result, err := d.Pipeline.Process(context.WithoutCancel(r.Context()), &pipeline.ProcessRequest{…})
```

### Why `context.WithoutCancel`

- Go 1.21+ primitive (repo is Go 1.25.10). Returns a context that is **never**
  cancelled by the parent, but **retains the parent's Values**.
- Preserves `middleware.GetReqID(ctx)` (chi request id) consumed by
  `submitForProcessing` for the artifact `TraceID` → capture provenance / trace
  correlation is unbroken (EB-2).
- Strips the request deadline too — acceptable and desirable for a durable
  capture; the pgx pool and NATS client enforce their own connection-level
  timeouts, so the call cannot hang indefinitely.
- `capture.go` did **not** previously import `context` (its only context use was
  `r.Context()`, a method on `*http.Request`), so the fix also adds `"context"`
  to the import block.

### Alternatives Considered (and rejected)

- **Fix inside the pipeline (`internal/pipeline/processor.go`):** wrong layer —
  the pipeline correctly honors its caller's context; making it ignore
  cancellation would break legitimate cancellable callers. The decision "this
  particular capture must outlive the client" belongs at the HTTP handler, which
  owns the request lifecycle.
- **Asynchronous / fire-and-forget capture:** would let the synchronous
  `CaptureResponse` render before the capture is durable, weakening the
  exactly-once acknowledgement semantics. Rejected — keep capture synchronous,
  only remove the cancellation coupling.
- **Add a configurable capture timeout SST key:** scope creep beyond the defect
  and would require a NO-DEFAULTS config addition. Deferred (Out Of Scope).

## Complexity Tracking

None — simplest viable fix used (one import + one call-site argument + an
explanatory comment, matching the already-ratified BUG-069-002 remedy).

## Change Boundary

`internal/api/capture.go` (the `"context"` import + the one durable `Process`
call line + the explanatory comment) and the new regression test
`internal/api/capture_disconnect_test.go`. Nothing else: not `internal/pipeline/*`,
not the Telegram adapter, not the wire schema, not the SST contract, not
`config/smackerel.yaml`, and no other `specs/` folder. The response-path
`d.DB.Healthy(r.Context())` re-check and all `writeJSON` / `writeError` calls
stay request-scoped.

## Rollback Contract

The fix is a single import + a single-line argument change plus a test;
`git revert <SHA>` cleanly restores the prior behavior. No schema migration, NATS
topology change, or restart semantics involved. Reverting re-opens
F-069-CR-CAPTURE-ENDPOINT-CTX-CANCEL, which the regression test would then catch
(RED).
