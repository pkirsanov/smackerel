# Spec: BUG-069-002 Assistant HTTP capture-as-fallback MUST survive client disconnect

Links: [bug.md](bug.md) | [design.md](design.md) | [scopes.md](scopes.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

## Context

Parent spec `specs/069-assistant-http-transport` introduced the assistant HTTP
transport adapter. Hard Constraint 5 ("Capture-as-fallback preserved") and the
parent `state.json` `policySnapshot.captureAsFallback: "inviolable"` require
that when `AssistantResponse.CaptureRoute == true` the user's prompt is durably
captured exactly once and never lost. The adapter renders capture synchronously
inside `HTTPAdapter.ServeHTTP` by calling the capture path with the HTTP request
context (`r.Context()`), which `net/http` cancels on client disconnect. This bug
spec defines the production behavior the fix must guarantee.

## Expected Behavior

### EB-1 — Capture survives client disconnect

When the facade returns `CaptureRoute=true`, the HTTP adapter MUST invoke the
capture path on a context that is NOT cancelled by client disconnect or request
deadline expiry, so the durable capture write (production: a Postgres `INSERT`
plus a NATS publish, both context-honoring) completes and the user's prompt is
persisted. A client that disconnects after the facade decision MUST NOT cause
the prompt to be dropped.

### EB-2 — Request-scoped values preserved

The decoupled capture context MUST still carry request-scoped values — in
particular the chi request id read by `middleware.GetReqID` (used by
`pipeline.submitForProcessing` for the artifact `TraceID`) — so capture
provenance/trace correlation is unbroken.

### EB-3 — Capture invoked exactly once; no double-capture

The fix MUST NOT change the capture invocation count. Capture is still invoked
exactly once per `CaptureRoute=true` turn (BS-001 idempotency); the change is
solely the cancellation semantics of the context handed to it.

### EB-4 — Telegram path and non-capture turns unchanged

Turns with `CaptureRoute=false` and the Telegram transport MUST be unaffected.
The change is confined to the HTTP adapter's capture-as-fallback branch.

## Acceptance Criteria

- AC-1: With the request context cancelled before the capture write, the capture
  path is still invoked AND the context it receives is not cancelled
  (`ctx.Err() == nil`), and the original prompt text + resolved user id are
  passed through intact. [EB-1, EB-2]
- AC-2: A pre-fix RED proof exists — the adversarial regression test FAILS
  against `a.capture(r.Context(), …)` (captured `context canceled`). [EB-1]
- AC-3: A post-fix GREEN proof exists — the same test PASSES after the swap to
  `context.WithoutCancel(r.Context())`. [EB-1, EB-2]
- AC-4: The full `internal/assistant/httpadapter` package (existing
  `TestChaos069`, golden-contract, adapter, transport-hint tests + the new
  regression) passes — no regression, capture still invoked exactly once. [EB-3, EB-4]
- AC-5: The change is confined to `internal/assistant/httpadapter/adapter.go`
  (one line + comment) plus the new test file; no other spec folder, the
  Telegram adapter, the wire schema, the SST contract, or `config/smackerel.yaml`
  is touched. [EB-4]

## Out Of Scope

- Adding a capture timeout / new SST key — `context.WithoutCancel` strips the
  deadline; the downstream pgx pool and NATS client carry their own timeouts. A
  configurable capture deadline is a separate enhancement, not this defect.
- Making capture asynchronous / fire-and-forget — capture remains synchronous so
  the rendered acknowledgement still reflects a completed capture; only the
  cancellation coupling is removed.
- The `BUG-069-001` PreFacadeChain wiring concern (separate finding, separate
  packet).
- Any edit to `internal/assistant/httpadapter/middleware.go`, `late_binding.go`,
  `schema.go`, the Telegram adapter, `internal/pipeline/*`, or `config/smackerel.yaml`.

## Product Principle Alignment

- **P9 Design For Restart, Not Perfection** — a flaky mobile connection must not
  punish the user by silently dropping a captured idea; capture-as-fallback is
  the safety net and must hold even when the client vanishes mid-turn.
- **P8 Trust Through Transparency** — the inviolable capture contract the
  product advertises must actually hold under client disconnect, not only on the
  happy path where the client stays connected.
