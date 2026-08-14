# Report: BUG-061-012 — Server-derived principal for agent tools

### Summary

Filed 2026-08-14 from `docs/Product_Delivery_Plan.md` § P1 hole 2 (Stage 1, Critical). The defect is
verified from source, not inferred from the plan: `retrieval_search` declares `user_id` as a
required, model-filled tool argument and validates only that it is non-empty.

### Completion Statement

**PARTIALLY IMPLEMENTED.** The contract is in place and the two tools where the argument was
decorative are fixed. The two where it is load-bearing are held by a shrinking ratchet and still
need surface wiring. Details and evidence below; no DoD item is checked beyond what was executed.

### Test Evidence

`./smackerel.sh lint` exit `0`, `./smackerel.sh format --check` exit `0`,
`./smackerel.sh test unit --go` exit `0` across **146** packages with zero failures.

---

## After Fix — What landed, and what did not

### The set was wrong in the delivery plan, and wrong in this packet's first filing

The plan named `retrieval`, `notification/propose`, `notification/execute`. The actual set is four
tools, and `notification/execute` is not among them:

```text
$ grep -rn '"user_id"' --include='*.go' internal/agent/tools/ | grep -v _test
internal/agent/tools/microtools/entity_resolve.go:153:  "required": ["input", "user_id"],
internal/agent/tools/recipesearch/tool.go:76:  "required": ["query", "user_id"],
internal/agent/tools/retrieval/tool.go:107:  "required": ["query", "user_id"],
internal/agent/tools/notification/propose.go:18:  "required": ["user_id", "what"],
```

They divide on whether the value is *used*, and that division decides the fix:

| Tool | `user_id` | Consequence |
|---|---|---|
| `retrieval` | validated, then discarded | Decorative. Removal changes no behaviour. |
| `recipesearch` | validated, then discarded | Decorative. Removal changes no behaviour. |
| `microtools/entity_resolve` | `Resolver.Resolve(callCtx, in.UserID, ...)` | Load-bearing — the model chooses whose entities resolve. |
| `notification/propose` | `UserID: in.UserID` on the proposal | Load-bearing — the model chooses who is notified. |

So the "model supplies the identity" framing is correct — for the two tools **neither** the plan
nor this packet's first filing identified.

### The contract, proven twice

Non-vacuous BEFORE the fix — it named all four:

```text
$ ./smackerel.sh test unit --go --go-run 'TestToolSchemas_DeclareNoCallerIdentity' --verbose
EXIT=1
    schema_contract_test.go:79: agent tool input schemas declare a caller identity (4):
          ../microtools/entity_resolve.go: user_id
          ../notification/propose.go: user_id
          ../recipesearch/tool.go: user_id
          ../retrieval/tool.go: user_id
--- FAIL: TestToolSchemas_DeclareNoCallerIdentity (0.01s)
```

Teeth AFTER the fix — a new offender was introduced deliberately and caught:

```text
# temporarily added "user_id" to internal/agent/tools/weather/tool.go
$ ./smackerel.sh test unit --go --go-run 'TestToolSchemas_DeclareNoCallerIdentity' --verbose
EXIT=1
    schema_contract_test.go:119: agent tool input schemas declare a caller identity (1):
          ../weather/tool.go: user_id
$ git diff --stat internal/agent/tools/weather/tool.go
(empty — mutation reverted)
```

A guard that only fails on the state it was written against proves nothing about the state it will
face. Both directions are recorded because either alone would be weak evidence.

### Lane state

```text
$ ./smackerel.sh test unit --go
UNIT_EXIT=0
ok packages: 146
FAIL lines: (none)
```

### What did NOT land, and why it was not forced

`entity_resolve` and `propose` still declare `user_id`. Removing it there requires the caller's
session to reach the tool through the request context, which is surface wiring across four `Invoke`
call sites — and on Telegram there is no principal to wire yet, because
`internal/telegram/agent_bridge.go:84` invokes the agent with only `chatID`.

Landing the removal without that wiring would make those tools fail closed on every non-HTTP
surface. The design (§ Risk and rollout) already rejected a flag for this: a flag leaves the
insecure path live and reachable, which is the thing being fixed. So the ratchet holds the two
files, fails on any new offender, and additionally fails if an allowlisted file is fixed but left
listed — so it cannot quietly stop ratcheting.

The grant requirement (R2) is also not yet enforced. It has the same dependency: refusing a caller
without `corpus:read` requires knowing who the caller is.


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
