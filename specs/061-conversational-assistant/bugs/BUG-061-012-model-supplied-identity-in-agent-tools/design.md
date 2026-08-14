# Design: BUG-061-012 — Server-derived principal for agent tools

## Root cause

Two independent omissions combine:

1. `retrieval_search` declares `user_id` in its input schema, so the model is *asked* for an
   identity, and the handler validates only non-emptiness.
2. No surface except HTTP puts a principal into the context the executor already propagates.

Neither alone would be sufficient to read the corpus as an arbitrary user. Together they are.

## The seam already exists — this is wiring, not new architecture

The decisive constraint on the design is that **no agent-package signature needs to change**:

- `ToolHandler` is `func(ctx context.Context, args json.RawMessage) (json.RawMessage, error)`
  (`internal/agent/registry.go:69`).
- The executor derives the per-tool context from the caller's:
  `toolCtx, toolCancel := context.WithTimeout(ctx, ...)` (`internal/agent/executor.go:613`).
- `auth.WithSession(ctx, sess)` and `auth.SessionFromContext(ctx)` already exist
  (`internal/auth/session.go:116`).

So a principal injected by a surface flows to every tool automatically. Choosing this over adding
an identity field to `IntentEnvelope` is deliberate: an envelope field is *data the model's own
plumbing carries*, which is the same class of mistake as a tool argument. A context value set by
the surface is not reachable from the model's output at all.

`internal/auth` does not import `internal/agent` (verified: no matches), so
`internal/agent/tools/...` may import `internal/auth` without a cycle.

## Change plan

### C1 — Tools resolve identity from context, and fail closed

`internal/agent/tools/retrieval/tool.go`:
- Delete `user_id` from `inputSchema` and from `required`.
- Delete `UserID` from `retrievalInput`.
- Resolve `sess, ok := auth.SessionFromContext(ctx)`; on `!ok` return
  `retrieval_search_no_principal` — a distinct error, not the existing
  `retrieval_search_missing_user_id`, so the two failure modes stay separable in logs.
- Require the grant: if the session lacks `corpus:read`, return `retrieval_search_grant_required`.

Same treatment for `internal/agent/tools/notification/propose.go` and `execute.go`.

### C2 — Surfaces inject a principal

| Surface | Change |
|---|---|
| HTTP (`internal/api/agent_invoke.go`) | None. `r.Context()` already carries the session. Covered by a regression test so it cannot silently regress. |
| Telegram (`internal/telegram/agent_bridge.go`) | Resolve `chatID` → mapped user **before** `Invoke`, then `ctx = auth.WithSession(ctx, sess)`. An unmapped chat gets no principal, so corpus tools fail closed via C1. |
| Scheduler, Pipeline, Judgment | Inject an explicit system principal that does **not** carry `corpus:read`. |

The system-principal choice matters. Injecting *nothing* would also fail closed today, but it fails
closed by accident — the day someone adds a default it silently opens. An explicit principal with
no corpus grant states the intent, and R4's test can assert the absence of the grant.

### C3 — Mechanical protection

A schema-contract test that reads every registered tool's `inputSchema` as data and fails if any
declares a caller-identity property (`user_id`, `userId`, `user`, `principal`, `actor`). This is
the shape that survives a future tool being added by a different author — an assertion listing
today's three tools would not.

Per R4.3 the adversarial cases must fail against pre-fix code. The check "no tool schema contains
`user_id`" does fail today, which is the proof it is not vacuous.

## Risk and rollout

The real risk is **breaking a working surface by failing closed**. Telegram is the exposed one: if
chat→user resolution is wrong, corpus retrieval stops working there.

Mitigation is ordering, not a flag: land C1 together with C2's Telegram resolution in one change,
and cover the mapped-chat path with a test that asserts retrieval still succeeds. A feature flag
would leave the insecure path live and reachable, which is what this bug is about.

## Rollback

Single revert. The change adds no migration, no persisted state, and no config key. The prior
behaviour is restored by the revert alone.
