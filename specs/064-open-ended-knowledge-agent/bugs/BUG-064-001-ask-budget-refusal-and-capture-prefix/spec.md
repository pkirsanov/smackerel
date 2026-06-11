# BUG-064-001 — Expected behavior specification

This specifies what the code SHOULD do. Tests validate THIS spec, not the
current broken behavior.

## Context

`/ask` is the v1 slash shortcut that routes to the `open_knowledge` scenario
(Spec 064 SCOPE-17, `internal/assistant/shortcuts.go`). The open-knowledge agent
plans over a tool allowlist (`internal_retrieval`, `web_search` via local
searxng, `unit_convert`, `calculator`) and returns a sourced answer or a typed
refusal. The home-lab default uses local Ollama + local searxng, wired with a
**zero-cost `CostFn`** (`cmd/core/wiring_assistant_openknowledge.go`), so no real
USD is ever spent.

## FR-1 — An enabled open-knowledge agent must not refuse pre-flight on a zero USD budget it can never spend against

**Given** `assistant.open_knowledge.enabled = true` on a deployment whose
open-knowledge `CostFn` charges `$0` per call (local Ollama + local searxng),
**When** a user asks any open-ended question via `/ask`,
**Then** the agent MUST proceed past the per-user-monthly USD pre-flight gate and
actually attempt to ground the answer (call the LLM / dispatch tools),
**And** MUST NOT terminate with `cap_usd` having done `0` iterations / `0` tool
calls.

### FR-1a — SST budget values must permit operation

`config/smackerel.yaml` MUST configure `assistant.open_knowledge.monthly_budget_usd`
and `assistant.open_knowledge.per_user_monthly_budget_usd` to values that allow
the pre-flight gate to pass when the capability is enabled. A value of `0`
(meaning "refuse all" under the current gate) is a misconfiguration for the
default self-hosted, zero-cost-provider deployment.

### FR-1b — The USD pre-flight gate semantics are preserved for paid providers

The `cap_usd` pre-flight refusal (`agent.go`, SCN-064-A08) MUST still fire when a
user genuinely has no remaining monthly allowance (a positive cap configured and
exhausted, i.e. spend ≥ cap). This bug does NOT relax the safety gate for paid
providers; it only corrects the default budget so a zero-cost local deployment is
not permanently gated.

## FR-2 — Captured ideas must not contain the slash-command prefix

**Given** any inbound message that begins with a v1 slash shortcut
(`/ask`, `/weather`, `/remind`, `/recipe`, `/cook`),
**When** the capability layer routes that turn to capture-as-fallback
(`AssistantResponse.CaptureRoute = true`),
**Then** the captured idea's stored text/title MUST be the natural-language tail
with the slash-command prefix removed (e.g. `/ask tide schedule …` is captured as
`tide schedule …`),
**And** a captured idea title MUST never contain a leading `/ask`, `/weather`,
`/remind`, `/recipe`, or `/cook` token.

### FR-2a — Non-shortcut text is unaffected

Plain-text captures that do NOT begin with a v1 shortcut MUST be captured
verbatim (no change). Stripping applies only to the closed v1 shortcut set
(`assistant.StripShortcutPrefix` semantics).

## Non-goals

- Re-pointing `/ask` to a different scenario (routing to `open_knowledge` is
  correct and stays).
- Introducing a new "unlimited budget" sentinel in the budget subsystem
  (considered and deferred — see `design.md`).
- Rewriting the budget accounting model or the SCN-064-A08 gate semantics.
- Fixing the stale `/ask → retrieval_qa` assumption baked into
  `tests/e2e/assistant_bs007_test.sh` (separate finding; routed as follow-up).

## Acceptance

- AC-1: With the corrected SST budget, a constructed open-knowledge agent (local
  zero-cost `CostFn`) does NOT refuse pre-flight with `cap_usd`; it proceeds to
  call the LLM. (FR-1, FR-1a)
- AC-2: The shipped `config/smackerel.yaml` open-knowledge monthly budgets are
  `> 0` whenever `enabled = true`. (FR-1a)
- AC-3: A `cap_usd` pre-flight refusal still fires when the configured per-user
  monthly budget is genuinely exhausted. (FR-1b)
- AC-4: A `/ask <query>` turn routed to capture stores the idea WITHOUT the
  `/ask ` prefix. (FR-2)
- AC-5: A plain-text capture (no shortcut) is stored verbatim. (FR-2a)
