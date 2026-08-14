# Design: BUG-061-012 — Server-derived principal for agent tools

## Root cause

Two independent omissions combine:

1. `retrieval_search` declares `user_id` in its input schema and validates it, but never uses it —
   `api.SearchRequest` has no user field and the corpus is a single global one. The argument is
   decorative, and it makes the tool *look* access-controlled.
2. The tool consults no grant. `auth.GrantGlobalCorpusRead` gates the equivalent HTTP routes;
   nothing gates the tool, so the agent is a path around that gate.

The second is what actually grants access. The first is why the gap was easy to miss on review.

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

### C1 — Tools resolve the session from context, require the grant, and fail closed

`internal/agent/tools/retrieval/tool.go`:
- Delete `user_id` from `inputSchema` and from `required`, delete `UserID` from `retrievalInput`,
  and delete the emptiness check and its error. The removal is complete, not cosmetic (R1.2).
- Resolve `sess, ok := auth.SessionFromContext(ctx)`; on `!ok` return
  `retrieval_search_no_principal`.
- Gate on `auth.GateGlobalCorpusRead(sess).Allowed`; on false return
  `retrieval_search_grant_required`. Reusing that helper rather than re-deriving the grant test is
  what keeps the agent boundary and the HTTP boundary from drifting (R2.4).

The two errors are distinct on purpose: "nobody is here" and "this caller may not" are different
operational conditions, and collapsing them would hide a misconfigured surface behind what looks
like a permissions problem.

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
