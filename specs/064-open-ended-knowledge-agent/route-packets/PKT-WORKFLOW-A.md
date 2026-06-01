# Route Packet PKT-WORKFLOW-A — SCOPE-17 e2e Infrastructure Findings

| Field              | Value |
|--------------------|-------|
| **Packet ID**      | PKT-WORKFLOW-A |
| **Routed from**    | `bubbles.implement` on `specs/064-open-ended-knowledge-agent` SCOPE-17 |
| **Routed to**      | `bubbles.workflow` (multi-finding dispatch) |
| **Status**         | `pending` |
| **Date**           | 2026-05-31 |
| **Kind**           | `infrastructure_gap_set` |
| **Blocks**         | spec 064 SCOPE-17 close-out; SCOPE-18 docs section that references "e2e green" |
| **Does NOT block** | the rest of spec 064 (SCOPE-01..16 already shipped) |

---

## 1. Context

SCOPE-17 owns the end-to-end live-stack proof that POST `/v1/agent/invoke`
→ router → executor → `open_knowledge_invoke` substrate tool → real
*okagent.Agent → real LLM bridge + real SearxNG + real PostgreSQL
exercises every spec scenario (SCN-064-A01..A08) including the
adversarial G021 fabricated-citation regression.

While implementing the e2e file
(`tests/e2e/agent/openknowledge_e2e_test.go`), I discovered the
following SUBSTANTIVE infrastructure gaps that prevent the suite from
running for real. None of them are SCOPE-17 implementation work —
each is owned by an upstream scope/spec whose deliverables were
either deferred or incomplete. The e2e file is wired correctly and
skips with explicit `t.Skip("…")` messages naming the routed finding;
it will execute automatically once each finding lands.

---

## 2. Findings

### Finding #1 — ML sidecar `/llm/chat` has no real Ollama dispatch path

**Status:** ✅ Resolved 2026-05-31. `ml/app/routes/chat.py` now
dispatches a real LLM turn via `litellm.acompletion` against the
Ollama backend (`OLLAMA_URL`, G028 fail-loud) when no fixture header
is present. Provider errors map to typed JSON envelopes
(`llm_provider_unreachable` / `llm_provider_error` / `llm_misconfigured`).
The two fixture modes are preserved. See spec
`report.md` → "PKT-WORKFLOW-A Finding #1 — Closed (2026-05-31)" for
the inline test + live-curl evidence (gemma3:4b end_turn + tool_use
turns succeeded). The remaining five findings (#2–#6) are unaffected.

**Owner candidate:** spec 064 SCOPE-09 (agent loop integration) OR
spec 045 / spec 061 (LLM provider wiring). The route's own docstring
attributes the real dispatch to SCOPE-09, but the implementation
shipped fixture-only.

`ml/app/routes/chat.py` currently returns **HTTP 501** for any
request that does not carry the `X-OpenKnowledge-Test-Mode`
fixture header. The two supported modes (`fixture-final-text`,
`fixture-tool-use`) are deterministic stubs used by the SCOPE-04
contract tests. There is no code path that calls Ollama (or any
real LLM provider) from this route.

Consequence: UC-064-A01 (web answer), A02 (unit_convert), A03
(hybrid), A04 (per-turn budget) cannot run end-to-end because they
all require a real LLM to drive the tool loop.

Acceptance for closure:
- `/llm/chat` performs a real provider call when no fixture header
  is present, honoring `llm.provider`, `llm.model`, and
  `llm.ollama_url` from SST.
- Errors from the provider surface as the existing typed
  ChatResponse error shape (NOT bare HTTP 5xx).
- A new e2e env knob `SMACKEREL_OPEN_KNOWLEDGE_LLM_REAL=true` (set
  by the e2e runner when the real path is wired) flips the live
  test suite from skip to execute.

### Finding #2 — Capture-as-fallback is not exercised by `/v1/agent/invoke`

**Owner candidate:** spec 061 conversational-assistant facade OR a
new spec amendment to lift capture-as-fallback above the
Telegram-surface boundary.

Per `internal/assistant/openknowledge/agent/agent.go` package doc,
capture-as-fallback is the **Telegram facade's** responsibility,
NOT the substrate executor's. The /v1/agent/invoke API surface
bypasses the facade and goes directly to executor → substrate
tool → agent, so no Idea artifact is persisted when the agent
refuses or fails.

Consequence: every refusal scenario (A04/A05/A06/A06-fabrication)
asserts "Idea artifact persisted with status=saved-as-idea-only"
per spec.md, but the API surface has no path that creates that
row. The e2e tests assert the refusal envelope shape but skip the
capture-as-fallback DB assertion until this is fixed.

Acceptance for closure: either (a) the API surface composes a
capture-as-fallback wrapper around the executor (preferred), or
(b) the e2e suite drops to a Telegram-facade entry point. Knob:
`SMACKEREL_OPEN_KNOWLEDGE_CAPTURE_FALLBACK_ON_API=true`.

### Finding #3 — No fixture mode for fabricated-citation in `/llm/chat`

**Owner candidate:** spec 064 SCOPE-08 (cite-back verifier) or a
small extension to chat.py owned by SCOPE-17 itself.

The adversarial G021 path needs the LLM to return a final response
that cites a URL NOT in any tool result. Real Ollama cannot be
asked to fabricate deterministically. The clean answer is a third
fixture mode `fixture-fabricated-cite` that returns a stop_reason=
end_turn response with a fabricated URL.

Acceptance for closure: `chat.py` honors
`X-OpenKnowledge-Test-Mode: fixture-fabricated-cite` by returning
a final response citing `https://fabricated.example.com/never-fetched`.
Knob: `SMACKEREL_OPEN_KNOWLEDGE_FIXTURE_FABRICATION=true`.

### Finding #4 — No per-test budget override knob

**Owner candidate:** spec 064 SCOPE-09 wiring (or a new SCOPE-17.1
amendment).

The per-query token budget and per-user monthly budget are loaded
from SST at startup. There is no way to set them very low for
ONE request from an e2e test. Without a request-scoped override
(e.g., a header `X-OpenKnowledge-Override-Budget` accepted only
in test mode, OR a per-test env knob the wiring honors), UC-064-A04
and A06 cannot deterministically trigger the budget-exhausted
refusal.

Knobs: `SMACKEREL_OPEN_KNOWLEDGE_TEST_PER_QUERY_TOKEN_BUDGET`,
`SMACKEREL_OPEN_KNOWLEDGE_TEST_PER_USER_MONTHLY_BUDGET`.

### Finding #5 — No per-test tool-allowlist override knob

**Owner candidate:** same as #4.

UC-064-A05 (operator disables web_search) needs the allowlist
restricted for ONE request. The allowlist is also loaded from SST
at startup. Either a request-scoped allowlist override OR a way
to spin up a second runtime instance with a different SST is
needed. Knob: `SMACKEREL_OPEN_KNOWLEDGE_TEST_TOOL_ALLOWLIST`.

### Finding #6 — e2e harness does not export AGENT_INVOKE_URL

**Owner candidate:** runtime e2e harness owner (spec 023 engineering
quality, or a small smackerel.sh extension).

The shell runner (`smackerel.sh test e2e`) already injects
`DATABASE_URL`, `NATS_URL`, and `SMACKEREL_AUTH_TOKEN` for the
live-stack Go tests, but does not export the in-network URL of the
core runtime's `/v1/agent/invoke` endpoint. Adding one line to
`smackerel.sh` that exports `AGENT_INVOKE_URL=http://core:${CORE_CONTAINER_PORT}/v1/agent/invoke`
into the `docker run` invocation would flip all seven tests from
"skip on missing URL" to "actually run".

---

## 3. Proposed routing

`bubbles.workflow` should dispatch the findings to the appropriate
owners:

- Finding #1 → spec 064 SCOPE-09 (or new bug folder under spec 064)
- Finding #2 → spec 061 conversational-assistant
- Finding #3 → spec 064 SCOPE-08 (small fixture-mode extension)
- Finding #4 + #5 → spec 064 SCOPE-09 wiring (or new SCOPE-17.1)
- Finding #6 → smackerel.sh test e2e maintainer (likely spec 023)

When all six land, the e2e file flips from honest-skip to honest-run
without any further code change on the SCOPE-17 side.

---

## 4. Evidence

- Test file: `tests/e2e/agent/openknowledge_e2e_test.go` (~520 lines)
- Test run (no infra): `./tests/e2e/agent/...` runs in 0.247s, all
  seven tests SKIP with explicit messages naming the routed finding.
- `go vet -tags e2e ./tests/e2e/agent/...` is clean.

---

## 5. SCOPE-17 disposition

SCOPE-17 is **Blocked** pending the six findings above. The e2e
scaffolding is in place and will activate automatically as the
infrastructure lands. No SCOPE-17 DoD item can be honestly checked
today; the scope-isolation rule forbids closure with skipped tests
counting as passes.
