# BUG-061-012 — The language model supplies the identity the corpus is read under

**Spec:** 061-conversational-assistant
**Severity:** S1 (Critical)
**Reported:** 2026-08-14
**Source:** `docs/Product_Delivery_Plan.md` § P1 hole 2, Stage 1

## Status

- [x] Reported
- [x] Confirmed (reproduced)
- [ ] In Progress
- [ ] Fixed
- [ ] Verified
- [ ] Closed

## Summary

`retrieval_search` takes `user_id` as a **required tool argument the model fills in**. The handler
checks only that the string is non-empty and then searches. Nothing server-side establishes that the
value corresponds to the caller. The language model is, in effect, supplying the identity that the
knowledge corpus is read under.

On the Telegram surface this is not merely weak — it is the *only* identity present, because that
bridge invokes the agent with no principal at all.

## Evidence — verified from source at HEAD `0f4b4826`

**1. `user_id` is a model-filled required argument.**

```text
$ sed -n '104,113p' internal/agent/tools/retrieval/tool.go
var inputSchema = json.RawMessage(`{
  "type": "object",
  "additionalProperties": false,
  "required": ["query", "user_id"],
  "properties": {
    "query":   {"type": "string", "minLength": 1},
    "user_id": {"type": "string", "minLength": 1},
    "top_k":   {"type": "integer", "minimum": 1, "maximum": 50}
  }
}`)
```

**2. The only server-side check is non-emptiness.**

```text
$ sed -n '178,184p' internal/agent/tools/retrieval/tool.go
        if in.UserID == "" {
                return nil, errors.New("retrieval_search_missing_user_id")
        }
```

There is no comparison against an authenticated principal, and no grant is required at the
retrieval boundary.

**3. No principal reaches the tool today.**

```text
$ grep -rn 'AuthenticatedPrincipal|PrincipalFromContext|WithPrincipal' internal/
(no matches)
```

**4. The Telegram surface carries no identity into the agent at all.**

```text
$ sed -n '84,94p' internal/telegram/agent_bridge.go
func (b *AgentBridge) Handle(ctx context.Context, chatID int64, text string) (*agent.InvocationResult, error) {
        ...
        env := agent.IntentEnvelope{
                Source:   "telegram",
                RawInput: text,
        }
        result, decision := b.Runner.Invoke(ctx, env)
```

`chatID` is never resolved to a user before invocation, and `IntentEnvelope` has no identity field
(`internal/agent/router.go:40-52`). So on this surface the model's `user_id` is the sole identity.

## Impact

The corpus can be read under an identity the model chose. Any prompt path that can influence the
`user_id` argument can influence whose knowledge is retrieved. This is the read-side counterpart to
the corpus-grant work in spec 108 — that spec gates *routes*; nothing gates the *tool*.

## Why the existing seam makes this tractable

`ToolHandler` already takes a `context.Context`
(`internal/agent/registry.go:69`), and the executor derives the per-tool context from the caller's
(`internal/agent/executor.go:613`). `auth.WithSession` / `auth.SessionFromContext` already exist
(`internal/auth/session.go:116`). A server-derived principal injected at each surface therefore
reaches every tool with **no signature changes anywhere in the agent package**.

The HTTP surface already injects one: `internal/api/agent_invoke.go:208` invokes with
`r.Context()`, which `auth.RequestAuthenticator` has already populated
(`internal/auth/request_authenticator.go:197`).

## Affected surfaces (all five callers of `Invoke`)

| Surface | File | Identity available today |
|---|---|---|
| HTTP API | `internal/api/agent_invoke.go:208` | **Yes** — auth session already in `r.Context()` |
| Telegram | `internal/telegram/agent_bridge.go:91` | **No** — only `chatID`, unresolved |
| Scheduler | `internal/scheduler/agent_bridge.go:63` | No — system-triggered |
| Pipeline | `internal/pipeline/agent_bridge.go` | No — system-triggered |
| Judgment | `internal/agent/judgment.go:63` | No — internal evaluation |

## Not to be confused with

This is **not** spec 108. Spec 108 gates HTTP routes on a corpus grant and is mid-rollout behind a
ratified OBSERVE window. This bug is about the agent-tool boundary, which spec 108's spec.md does
not cover — confirmed by grep against that spec.
