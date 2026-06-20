# Bug Fix Design: BUG-069-002 — decouple assistant HTTP capture-as-fallback from request cancellation

Links: [bug.md](bug.md) | [spec.md](spec.md) | [scopes.md](scopes.md) | [report.md](report.md)

## Root Cause Analysis

### Investigation Summary

`chaos-hardening` Round 39 probed the assistant HTTP transport for resilience /
race / edge-case failures. Reading the live request path top-to-bottom:

1. `internal/assistant/httpadapter/adapter.go` — `HTTPAdapter.ServeHTTP` handles
   `CaptureRoute=true` by calling `a.capture(r.Context(), userID, req.TransportMessageID, req.Text)`.
2. `cmd/core/wiring_assistant_facade.go::newAssistantHTTPCaptureFn` — the
   production `CaptureFn` dispatches `svc.proc.Process(ctx, &pipeline.ProcessRequest{Text, SourceID: SourceCapture})`
   and only `slog.Error`s on failure (no retry, no background re-dispatch).
3. `internal/pipeline/processor.go::Process` → `submitForProcessing` →
   `storeInitialArtifact(ctx, …)` runs a Postgres `INSERT` and
   `p.NATS.Publish(ctx, …)` runs a NATS publish — **both honor `ctx`**.
4. `net/http` cancels `r.Context()` the moment the client connection drops or the
   request deadline fires.

### Root Cause

The **inviolable** capture-as-fallback side-effect was coupled to HTTP request
liveness. The adapter reused the request context (`r.Context()`) — correct for
"respond to this client" but wrong for "persist the user's prompt", which must
outlive the client. When the client disconnects between the `Facade.Handle`
decision (`CaptureRoute=true`) and the completion of the capture write, the
cancelled context aborts the Postgres `INSERT` / NATS publish and the prompt is
silently dropped. This directly violates Hard Constraint 5, BS-001, and
`policySnapshot.captureAsFallback: "inviolable"`.

### Why Tests Missed It (test-gap analysis)

- `internal/assistant/httpadapter/chaos_069_test.go` (`TestChaos069`) uses a
  **no-op** capture stub (`Capture: func(context.Context, string, string, string) {}`)
  and never cancels the request context, so it cannot observe a cancelled-context
  capture.
- The e2e/integration capture tests drive a live client that stays connected for
  the whole turn, so the disconnect race never fires.
- No test asserted the capture-context cancellation invariant. The structural fix
  for the gap is a regression test that cancels the request context and asserts
  the capture still runs with a live context.

### Impact Analysis

- **Affected component:** the HTTP adapter capture-as-fallback branch
  (`HTTPAdapter.ServeHTTP`).
- **Affected behavior:** durable persistence of a captured prompt when the client
  disconnects mid-turn.
- **Affected users:** any HTTP/web/mobile client on a flaky network whose request
  is cancelled during a `CaptureRoute=true` turn. Telegram is **not** affected
  (long-poll background context, not a per-request HTTP context).
- **Affected data:** the single in-flight user prompt for that turn (silent loss;
  no corruption, no multi-record loss).
- **Existing mitigations:** none — `newAssistantHTTPCaptureFn` logs and returns;
  there is no retry or background re-dispatch, which is exactly why the loss is
  silent.

## Fix Design

### Solution Approach (chosen)

Hand the capture path a context that is decoupled from request cancellation but
preserves request-scoped values, at the single capture call site:

```go
// internal/assistant/httpadapter/adapter.go — HTTPAdapter.ServeHTTP
//
// before (the defect):
a.capture(r.Context(), userID, req.TransportMessageID, req.Text)

// after (the fix):
// Capture-as-fallback is inviolable (Hard Constraint 5 / BS-001): the user's
// prompt MUST persist even if the client has already disconnected. net/http
// cancels r.Context() the instant the connection drops, which would abort the
// downstream pipeline.Process Postgres INSERT / NATS publish and silently lose
// the prompt. Decouple the durable capture write from request cancellation
// while preserving request-scoped values (request id, trace correlation via
// middleware.GetReqID). Spec 069 chaos Round 39 — F-069-CHAOS39-CAPTURE-CTX-CANCEL.
a.capture(context.WithoutCancel(r.Context()), userID, req.TransportMessageID, req.Text)
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

### Alternatives Considered (and rejected)

- **Fix inside `newAssistantHTTPCaptureFn` (cmd/core):** leakier — every future
  `CaptureFn` implementation would have to re-remember to decouple. The adapter
  owns the "calls the existing capture path before returning, prompt never lost"
  contract (Hard Constraint 5), so the defense belongs there and protects all
  capture implementations.
- **Asynchronous / fire-and-forget capture:** would let the synchronous
  acknowledgement render before the capture is durable, weakening the
  exactly-once acknowledgement semantics. Rejected — keep capture synchronous,
  only remove the cancellation coupling.
- **Add a configurable capture timeout SST key:** scope creep beyond the defect
  and would require a NO-DEFAULTS config addition. Deferred (Out Of Scope).

## Change Boundary

`internal/assistant/httpadapter/adapter.go` (one line + explanatory comment) and
the new regression test `internal/assistant/httpadapter/capture_disconnect_test.go`.
Nothing else: not `middleware.go`, `late_binding.go`, `schema.go`, the Telegram
adapter, `internal/pipeline/*`, `config/smackerel.yaml`, or any other `specs/`
folder.

## Rollback Contract

The fix is a single-line change plus a test; `git revert <SHA>` cleanly restores
the prior behavior. No schema migration, NATS topology change, or restart
semantics involved. Reverting re-opens F-069-CHAOS39-CAPTURE-CTX-CANCEL, which
the regression test would then catch (RED).
