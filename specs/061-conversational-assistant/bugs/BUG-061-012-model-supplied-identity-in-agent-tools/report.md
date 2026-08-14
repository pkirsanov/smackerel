# Report: BUG-061-012 — Server-derived principal for agent tools

### Summary

Filed 2026-08-14 from `docs/Product_Delivery_Plan.md` § P1 hole 2 (Stage 1, Critical). The defect is
verified from source, not inferred from the plan: `retrieval_search` declares `user_id` as a
required, model-filled tool argument and validates only that it is non-empty.

### Completion Statement

**NO FIX IMPLEMENTED.** This packet is artifacts and root-cause analysis only. Every DoD item in
`scopes.md` is unchecked and no source file has been modified.

### Test Evidence

None. No test has been written or executed for this bug. Nothing is claimed in either direction.

---

## Before Fix — Reproduction

### TREE

HEAD `0f4b4826`, working tree clean on every path named below.

### STEP 1 — the model is asked for the identity

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

`user_id` is in `required`, so the model MUST produce it on every call.

### STEP 2 — the server checks only that it is non-empty

```text
$ sed -n '178,184p' internal/agent/tools/retrieval/tool.go
        if in.UserID == "" {
                return nil, errors.New("retrieval_search_missing_user_id")
        }
        if in.Query == "" {
                return nil, errors.New("retrieval_search_empty_query")
        }
```

No comparison against an authenticated principal. No grant required.

### STEP 3 — no principal mechanism exists

```text
$ grep -rn 'AuthenticatedPrincipal\|PrincipalFromContext\|WithPrincipal' --include='*.go' internal/
$ echo "exit=$?"
exit=1
```

Exit `1` is the finding: the concept is absent, so there is nothing for the tool to have consulted.

### STEP 4 — the Telegram surface carries no identity into the agent

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

`chatID` is never resolved to a user. `IntentEnvelope` (`internal/agent/router.go:40-52`) has no
identity field. On this surface the model's `user_id` is therefore the **only** identity in play —
which is why this is filed S1 rather than S2.

### Reading of the transcript

Steps 1 and 2 establish that the tool trusts an argument. Step 3 establishes that it had no
alternative to trust. Step 4 establishes that on at least one live surface there is no other
identity anywhere in the call. The three together are the defect; none alone would be.

### What this packet does NOT claim

- It makes **no** claim about spec 108's corpus-grant route gate. That is a different boundary,
  mid-rollout behind a ratified OBSERVE window, and nothing here makes it more or less enforced.
- It makes no claim that the defect has been exploited.
- It does not assert the fix size beyond the seven files named in the Change Boundary; that estimate
  is derived from the five `Invoke` call sites plus three tool files, and the implementing agent
  should treat it as a floor rather than a budget.
