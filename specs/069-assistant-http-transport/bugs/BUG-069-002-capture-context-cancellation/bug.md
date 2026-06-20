# Bug: BUG-069-002 Assistant HTTP capture-as-fallback is bound to the request context (`r.Context()`), so a client disconnect during a `CaptureRoute=true` turn cancels the durable capture write and silently loses the user's prompt — violating the inviolable capture-as-fallback contract

## Summary

Spec 069's HTTP transport adapter renders the capture-as-fallback path by calling `a.capture(r.Context(), …)` in `HTTPAdapter.ServeHTTP` (`internal/assistant/httpadapter/adapter.go`). The production capture function (`cmd/core/wiring_assistant_facade.go::newAssistantHTTPCaptureFn`) dispatches the user's text into `pipeline.Processor.Process(ctx, …)`, which performs a **context-honoring** Postgres `INSERT` (`storeInitialArtifact`) and a **context-honoring** NATS publish (`submitForProcessing`). `net/http` cancels `r.Context()` the instant the client connection drops. So if a web/mobile client disconnects (or its request deadline fires) **after** `Facade.Handle` returns `CaptureRoute=true` but **before** the capture write completes, `Process` aborts with `context.Canceled`, `newAssistantHTTPCaptureFn` only `slog.Error`s it, and **the user's prompt is silently lost** — the exact failure the capture-as-fallback contract exists to prevent.

Discovered by stochastic-quality-sweep **Round 39** (`chaos-hardening` on `specs/069-assistant-http-transport`). Finding id: **F-069-CHAOS39-CAPTURE-CTX-CANCEL**.

## Severity

- [ ] Critical — System unusable, data loss
- [ ] High
- [x] **Medium** — Silent, per-occurrence loss of user content (a captured idea/prompt) on a specific but realistic race: client disconnect / request-deadline expiry during a `CaptureRoute=true` turn. It violates the **inviolable** capture-as-fallback contract (`policySnapshot.captureAsFallback: "inviolable"`, Hard Constraint 5, BS-001 — "the user's prompt is never lost"), which keeps it well above Low. It is not High/Critical because it is bounded to capture-fallback turns under a disconnect race (not all turns, no systemic corruption, no multi-record loss), and flaky-network mobile clients are the realistic trigger. The Telegram adapter is **not** exposed (it processes updates under a long-poll background context, not a per-request HTTP context that dies on disconnect).
- [ ] Low

## Status

- [x] Reported
- [x] Confirmed (reproduced with an adversarial regression test that cancels the request context — RED captured below)
- [x] In Progress
- [x] Fixed (`a.capture(context.WithoutCancel(r.Context()), …)` — decouples the durable capture write from request cancellation while preserving request-scoped values)
- [x] Verified (RED-before / GREEN-after + full `httpadapter` package green, no regression)
- [ ] Closed (pending validate-owned certification / parent-spec report annotation)

## Reproduction Steps

1. Construct an `*HTTPAdapter` with a facade stub that returns `AssistantResponse{CaptureRoute: true}` and a capture stub that records the context it is handed.
2. Build a `POST /api/assistant/turn` request, inject a shared-token session (so the adapter resolves `SharedUserID` instead of 401-ing), then **cancel** the request context to model the client disconnecting mid-turn.
3. Call `adapter.ServeHTTP(w, r.WithContext(cancelledCtx))`.
4. Observe: the capture path IS invoked, but the context handed to it carries `context.Canceled` (`ctx.Err() != nil`). In production, `pipeline.Processor.Process` would then abort its Postgres `INSERT` / NATS publish on that cancelled context and the prompt would be dropped (only an `slog.Error` is emitted; no retry, no background re-dispatch).

Pre-fix automated reproduction (RED), `internal/assistant/httpadapter/capture_disconnect_test.go`:

```text
=== RUN   TestHTTPAdapter_CaptureSurvivesClientDisconnect
    capture_disconnect_test.go:97: capture ran with a cancelled context (err=context canceled); a client disconnect MUST NOT abort the durable capture write (F-069-CHAOS39-CAPTURE-CTX-CANCEL)
--- FAIL: TestHTTPAdapter_CaptureSurvivesClientDisconnect (0.00s)
FAIL    github.com/smackerel/smackerel/internal/assistant/httpadapter   0.028s
```

## Expected Behavior

Capture-as-fallback is **inviolable**: when `AssistantResponse.CaptureRoute == true`, the user's text MUST be durably captured exactly once, **even if the HTTP client has already disconnected**. The capture write MUST run on a context that is NOT cancelled by client disconnect / request-deadline expiry, while still preserving request-scoped values (request id / trace correlation via `middleware.GetReqID`). The response to the (possibly gone) client is best-effort and moot; the durable capture is not.

## Actual Behavior

`HTTPAdapter.ServeHTTP` calls `a.capture(r.Context(), …)`. `r.Context()` is cancelled by `net/http` on client disconnect. The production capture (`newAssistantHTTPCaptureFn` → `pipeline.Processor.Process`) runs a ctx-honoring Postgres `INSERT` + NATS publish, both of which abort with `context.Canceled` on a cancelled context. `newAssistantHTTPCaptureFn` logs `"assistant HTTP capture: pipeline.Process failed"` and returns — the prompt is gone. No retry. No background re-dispatch.

## Environment

- Repo: smackerel (Go core runtime), Go 1.25.10, current working tree (unrelated in-flight sweep WIP present elsewhere — untouched by this packet).
- Sweep: stochastic-quality-sweep **Round 39**, mode `chaos-hardening`, parent spec `specs/069-assistant-http-transport` (`status: done`, `workflowMode: full-delivery`).
- Defect site: `internal/assistant/httpadapter/adapter.go` — `HTTPAdapter.ServeHTTP`, the `if resp.CaptureRoute { a.capture(r.Context(), …) }` branch.
- Production capture path: `cmd/core/wiring_assistant_facade.go::newAssistantHTTPCaptureFn` → `internal/pipeline/processor.go::Process` → `storeInitialArtifact` (Postgres `INSERT`, ctx-honoring) + `submitForProcessing` (`NATS.Publish(ctx, …)`, ctx-honoring).
- Contract violated: spec 069 Hard Constraint 5 (capture-as-fallback preserved); BS-001; `state.json policySnapshot.captureAsFallback: "inviolable"`.

## Five Whys

1. **Why is the prompt lost?** The capture write aborts with `context.Canceled`.
2. **Why does it abort?** It runs `pipeline.Process` on `r.Context()`, which `net/http` cancels on client disconnect.
3. **Why is `r.Context()` used for a durable side-effect?** The adapter renders the response synchronously and reused the request context for the capture call without distinguishing "respond to client" (request-scoped) from "persist the prompt" (must outlive the client).
4. **Why wasn't it caught?** The existing chaos test (`TestChaos069`) uses a no-op capture stub and never cancels the request context, and the e2e/integration capture tests run against a live client that does not disconnect mid-turn.
5. **Root cause:** the inviolable capture-as-fallback side-effect was coupled to HTTP request liveness; a durable write must be decoupled from request cancellation.

## Fix

`internal/assistant/httpadapter/adapter.go` — capture the prompt on a cancellation-decoupled context that preserves request-scoped values:

```go
// before (the defect):
a.capture(r.Context(), userID, req.TransportMessageID, req.Text)

// after (the fix):
a.capture(context.WithoutCancel(r.Context()), userID, req.TransportMessageID, req.Text)
```

`context.WithoutCancel` (Go 1.21+) returns a context that is never cancelled by the parent but still carries the parent's Values — so `middleware.GetReqID(ctx)` (used by `submitForProcessing` for `TraceID`) and any downstream request-scoped values survive, while a client disconnect can no longer abort the durable capture write. Single-line change; `context` is already imported. The Telegram path is unaffected (it never used the HTTP request context).
