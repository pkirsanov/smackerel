# BUG-061-012 — The language model supplies the identity the corpus is read under

**Spec:** 061-conversational-assistant
**Severity:** S1 (Critical)
**Reported:** 2026-08-14
**Source:** `docs/Product_Delivery_Plan.md` § P1 hole 2, Stage 1

## Status

- [x] Reported
- [x] Confirmed (reproduced)
- [x] In Progress
- [x] Fixed
- [x] Verified
- [ ] Closed

**Fixed** at commits `20b0376a` (server-derived principal across the tool and `Invoke` surfaces) and
`0dcb9d1f` (repair of the three consumers the narrowed schemas broke).

**Verified** by the eight Test Plan rows in `scopes.md`, each with its own inline evidence, plus five
green lanes on the current tree — `lint`, `format`, `unit` (146 packages), `integration`, and `e2e`
all exit `0`. The `stress` lane exits `1` for a reason proven pre-existing by a clean-room worktree
at `0f4b4826`; it is routed to a separate `bubbles.test`-owned packet and does not gate this bug.
See `report.md` § Discovered issues 5.

**Not closed.** Three things are deliberately left open rather than folded into this checkbox. The
Telegram principal resolver is correct and tested but has **no production caller**, so P1 hole #3
remains open pending the scope-10 router wiring (`report.md` § Discovered issues 3). The
`bugfix-fastlane` specialist pipeline is incomplete — `regression`, `simplify`, `stabilize`,
`security`, `validate`, and `audit` have not run — so the packet `state.json` stays `in_progress`
rather than `done`. And `Closed` is a human acceptance step, with `uservalidation.md` human-only
under Gate G136.

## Summary

`retrieval_search` takes `user_id` as a **required tool argument the model fills in**, checks only
that it is non-empty, and then **never uses it**. The corpus it searches is the single
operator-owned global corpus, and the tool requires **no grant** to read it.

The defect has two parts, and the second is the substantive one:

1. The tool demands a caller identity it does not enforce. It reads as an access control and is not
   one.
2. Any scenario the model can route to `retrieval_search` reads the global corpus, whatever the
   caller's grants are. `auth.GrantGlobalCorpusRead` exists and gates the HTTP routes; the agent
   tool consults nothing.

**Correction to the first filing of this packet.** It described the bug as "the model supplies the
identity the corpus is read under". That overstates the scoping and understates the exposure.
`api.SearchRequest` has no user or owner field (`internal/api/search.go:26-30`), and `in.UserID` is
referenced exactly twice in the tool — the struct field and the emptiness check. The corpus is not
per-user, so no argument can redirect it. The real hole is that reading it requires no grant.

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

**2. The only server-side check is non-emptiness, and the value is then discarded.**

```text
$ grep -n 'UserID' internal/agent/tools/retrieval/tool.go
156:	UserID string `json:"user_id"`
180:	if in.UserID == "" {
```

Two references: the struct field and the emptiness check. The search call that follows is
`api.SearchRequest{Query: in.Query, Limit: limit}` — `in.UserID` is never passed to it, and
`SearchRequest` has no field it could be passed as:

```text
$ sed -n '26,30p' internal/api/search.go
type SearchRequest struct {
        Query   string        `json:"query"`
        Limit   int           `json:"limit,omitempty"`
        Filters SearchFilters `json:"filters,omitempty"`
}
```

No comparison against an authenticated principal, and **no grant required at the retrieval
boundary** — which is the part that actually controls access.

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

The global corpus is readable through the agent by any caller whose scenario reaches
`retrieval_search`, with no grant consulted. This is the read-side counterpart to spec 108 — that
spec gates *routes* on `corpus:read`; nothing gates the *tool*, so the agent is a path around the
very gate spec 108 is mid-rollout on.

The decorative `user_id` compounds it: a reviewer scanning the schema sees a caller identity and may
reasonably conclude the tool is scoped when it is not.

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
