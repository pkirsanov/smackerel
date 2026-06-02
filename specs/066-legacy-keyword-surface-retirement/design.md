# Design: 066 Legacy Keyword Surface Retirement

Owner: `bubbles.design`  
Workflow mode: `product-to-planning`  
Status ceiling for this pass: `specs_hardened`  
Source requirements: [spec.md](spec.md)

## Design Brief

**Current State.** The assistant facade exists, but Telegram still exposes many scenario-specific slash commands. [internal/api/domain_intent.go](../../internal/api/domain_intent.go) parses `/find` domain intent with regex, and [internal/annotation/parser.go](../../internal/annotation/parser.go) classifies user-facing phrases through a keyword map.

**Target State.** User-facing domain work enters as natural language, compiles through spec 068 into `CompiledIntent`, and routes through the spec 061 facade plus spec 037 scenarios/tools. Telegram keeps deterministic operational commands and the spec 061 power shortcuts. Retired commands are intercepted only during a configured alias window, rewritten to natural language, and routed through the same facade as plain text.

**Patterns to Follow.** Keep alias handling in `internal/telegram/`; keep the facade transport-neutral. Use `contracts.AssistantMessage`, `Facade.Handle`, spec 065 `entity_resolve`, and spec 068 compiled slots. Preserve capture-as-fallback and confirmation behavior.

**Patterns to Avoid.** Do not keep retired handlers wired, add a new command grammar, add regex classifiers, add keyword maps for user free text, or let operational commands call the LLM. Do not hide missing alias-window config with a date default.

**Resolved Decisions.** Alias rewriting belongs in the Telegram adapter. One-time notice state is persisted. `/help`, `/status`, `/reset`, `/digest`, `/recent`, and `/done` stay deterministic. `/ask`, `/weather`, and `/remind` remain power shortcuts but spec 068 gives them synthetic compiled-intent traces.

**Open Questions.** The retirement date is a planning/operator value. BotCommands order follows the UX inventory in [spec.md](spec.md).

## Purpose And Scope

This design removes competing user request paths while preserving the underlying domain capabilities. It does not build new scenarios; it consumes specs 065, 068, and 064.

## Architecture Overview

```text
Telegram update
  -> command classifier
     -> operational command: deterministic handler
     -> retained shortcut: synthetic CompiledIntent + Facade.Handle
     -> retired command inside window: rewrite to NL + notice + Facade.Handle
     -> retired command after window: unknown-command response
     -> plain text: CompiledIntent + Facade.Handle
```

The facade never sees a retired slash token as a command. It receives a plain prompt or an operational message kind.

## Capability Foundation

The reusable capability is `LegacyAliasRetirement`: a transport-local policy that maps a finite set of retired command tokens to canonical prompts during a configured window.

```go
type LegacyAlias struct {
    Command         string
    PromptTemplate  string
    RetiredSurface  string
    SuccessorSpecs  []string
}
```

Policies:

- Alias output is natural-language text handed to the normal assistant facade.
- Alias rows never influence scenario selection directly.
- Notice state is one-time per `(user_id, transport, command, window_until)`.
- Expired aliases reject before facade invocation.

### Variation Axes

| Axis | Values | Enforcement |
|------|--------|-------------|
| Command class | operational, retained shortcut, retired alias, unknown | adapter command classifier |
| Time policy | inside window, after window | SST timestamp |
| Notice state | first use, already notified | notice table |
| Replacement target | prompt, deterministic rejection | closed alias table |

## Concrete Implementations

### Telegram Legacy Alias Adapter

Package: `internal/telegram/legacy_aliases.go`.

Retired commands and prompt shapes:

| Command | Replacement Prompt Shape |
|---------|--------------------------|
| `/find` | `find {args}` |
| `/rate` | `rate {args}` |
| `/concept` | `show me the concept {args}` |
| `/person` | `show me what I know about {args}` |
| `/list` | `manage my list: {args}` |
| `/expense` | `record or review expense: {args}` |
| `/watch` | `watch for {args}` |
| `/lint` | `show knowledge quality issues` |
| `/meal_plan` | `plan meals {args}` |
| `/recipe` | `find or use recipe {args}` |
| `/cook` | `start cooking {args}` |

### Domain Intent Replacement

`internal/api/domain_intent.go` is deleted. Request paths that need domain/entity resolution call the assistant compiler and spec 065 `entity_resolve` instead of regex parsing.

### Annotation Classification Replacement (SCOPE-5)

Runtime annotation classification no longer consults phrase maps for user-authored turns. Spec 068 produces `interaction_type`, `rating`, `target`, and `note` slots; borderline cases use spec 061 disambiguation.

**Decision (SCOPE-5).** Replace the `internal/annotation/parser.go` `interactionMap` with an LLM-driven compiled-intent classification routed through `agent.Bridge.Invoke`. Consumers stay synchronous: they already run on user-facing request paths (`internal/api/annotations.go` HTTP handler, `internal/telegram/annotation.go` ×2 handler call-sites) and can tolerate one network-bound classifier call per turn. No async queue, no background pipeline.

**Dispatch surface.** A new compiled-intent scenario `annotation_classify` (id `annotation.classify.v1`, owned by the spec 037/068 scenario registry under `config/prompt_contracts/`) accepts:

```text
Input slots:
  text            string  required   the raw annotation text the user typed
  source_channel  string  required   "api" | "telegram" | <future channel>
Output slots:
  interaction_type  enum    required   one of the InteractionType domain values
  confidence        float   required   [0,1] — compared against confidence floor
  rationale         string  optional   short LLM rationale for audit
```

`internal/annotation` exposes a new `Classifier` interface (single method `Classify(ctx, text, channel) (InteractionType, confidence, error)`). The production implementation wraps `agent.Bridge.Invoke` with an explicit-id `IntentEnvelope{ScenarioID: "annotation.classify.v1"}` so the router takes the BS-002 fast path and skips embed work. The api/telegram call-sites construct `ParsedAnnotation` in two steps: deterministic `Parse(text)` for rating/tags/note/target extraction (these regex paths stay — they are not user-classification keyword maps), then `Classifier.Classify(ctx, text, channel)` to fill `InteractionType`. The interactionMap and `sortedInteractionPhrasesList` are deleted from `parser.go`; `InteractionPhrases()` is retained only if a non-classification consumer still needs the canonical phrase set, otherwise removed.

**Warm-cache fallback.** A small, hard-coded fast-path table covering only the highest-frequency exact-token forms (`"made it"`, `"cooked it"`, `"bought it"`, `"read it"`, `"visited"`) lives in `internal/annotation/classifier_warmcache.go`. It is consulted ONLY when:

1. The input text, after lowercasing and whitespace collapse, is exactly one of the cached tokens (no substring matching, no multi-phrase composition).
2. The classifier interface is configured with `warm_cache_enabled = true` (SST key `assistant.annotation.classifier.warm_cache_enabled`, default `false` in test/CI, `true` in dev/prod).

The warm cache is documented in code and in `docs/Architecture.md` as **"latency cache, not source of truth"**: it MUST be derivable from the compiled-intent scenario's training examples. A separate consistency test (`TestAnnotationWarmCacheAgreesWithCompiledIntent`, unit, runs against a recorded `annotation_classify` fixture) fails if any warm-cache token would classify differently than the scenario. The warm cache is NOT a fallback for classifier errors — on `Classifier.Classify` failure the handler returns an operational error to the caller; it does not silently guess.

**Borderline handling.** When the LLM returns confidence below the configured floor (`assistant.annotation.classifier.confidence_floor`, fail-loud required, no default), the classifier returns `InteractionType("")` and `ErrBelowConfidenceFloor`. Handlers route the request through spec 061 disambiguation (existing facade path) instead of guessing.

**Why synchronous, not async.** The two call-sites (`api/annotations.go` HTTP `POST /api/annotations`, `telegram/annotation.go` rate-flow and pending-annotation finalizer) already block the user. Introducing a NATS round-trip + callback would force a state machine on Telegram pending annotations and break the HTTP request/response contract. The classifier call shares the same latency budget as the existing assistant facade calls already made from these paths.

**Why a tiny warm cache instead of pure-LLM.** Annotation traffic is dominated by `"made it"` / `"cooked it"` / `"bought it"` / `"read it"` / `"visited"` exact-token inputs from recipe and book flows. Routing those through the LLM doubles per-annotation latency without changing the answer. The cache is bounded (≤5 tokens), exact-match only, and consistency-tested against the compiled intent, so it cannot drift into a hidden grammar.

## Data Model

```sql
CREATE TABLE IF NOT EXISTS assistant_legacy_alias_notices (
    user_id TEXT NOT NULL,
    transport TEXT NOT NULL,
    legacy_command TEXT NOT NULL,
    window_until TIMESTAMPTZ NOT NULL,
    first_notified_at TIMESTAMPTZ NOT NULL,
    last_routed_at TIMESTAMPTZ NOT NULL,
    route_count INTEGER NOT NULL CHECK (route_count >= 1),
    schema_version INTEGER NOT NULL DEFAULT 1,
    PRIMARY KEY (user_id, transport, legacy_command, window_until)
);

CREATE INDEX IF NOT EXISTS idx_assistant_legacy_alias_notices_window
    ON assistant_legacy_alias_notices (window_until);
```

Rows are audit/support state only and may be swept after the alias window plus retention.

## API And Contracts

No new public HTTP endpoint is introduced. Contract changes:

- BotCommands after the window contain only `/help`, `/status`, `/reset`, `/digest`, `/recent`, `/done`, `/ask`, `/weather`, and `/remind`.
- Retired command inside the window returns a one-time notice plus the assistant response for the replacement prompt.
- Retired command after the window returns a deterministic unknown-command response and does not invoke the facade.

Unknown-command envelope:

```json
{
  "status": "unknown_command",
  "retired_command": "/find",
  "replacement_example": "find my notes about ACL tags",
  "help_command": "/help",
  "facade_invoked": false
}
```

## Configuration

Required SST keys:

| Key | Purpose |
|-----|---------|
| `assistant.transports.telegram.legacy_alias_window_until` | RFC3339 end timestamp |
| `assistant.transports.telegram.legacy_alias_notice_retention_days` | notice row retention |
| `assistant.transports.telegram.retired_command_rejection_code` | closed response code |
| `assistant.annotation.classifier.confidence_floor` | minimum LLM confidence to accept an interaction classification; below this routes to disambiguation |
| `assistant.annotation.classifier.warm_cache_enabled` | enables the bounded exact-token latency cache for the 5 hottest annotation tokens |

Missing or malformed config fails startup.

## Security And Compliance

- Expired retired commands fail before scenario/tool execution.
- Alias rewrite preserves audit metadata but does not route from slash tokens.
- Operational commands remain deterministic.
- Write-like replacements still require spec 068 side-effect gating and confirmation.
- Notice state stores command token and timestamps, not command arguments.

## Observability And Failure Handling

Metrics:

| Metric | Labels | Meaning |
|--------|--------|---------|
| `smackerel_assistant_legacy_alias_total` | `command,outcome` | rewrite, notice, rejection count |
| `smackerel_assistant_retired_command_rejected_total` | `command` | after-window rejections |
| `smackerel_assistant_legacy_handler_refs_total` | `surface` | guard-detected references |

Notice-store write failure returns an operational error and does not invoke the facade. Alias parse failure returns unknown-command help.

## Testing And Validation Strategy

| Scenario | Test Type | Test Location | Assertion |
|----------|-----------|---------------|-----------|
| SCN-066-A01 | unit + integration | `internal/telegram/legacy_aliases_test.go` | BotCommands contains retained set only |
| SCN-066-A02 | e2e-api | `tests/e2e/assistant/legacy_retirement_http_test.go` | NL retrieval replaces `/find` and cites artifacts |
| SCN-066-A03 | e2e-api | same | rating without context produces disambiguation |
| SCN-066-A04 | integration | `tests/integration/telegram/legacy_alias_test.go` | inside window rewrites, records notice, invokes facade |
| SCN-066-A05 | integration | same | after window rejects and facade is not invoked |
| SCN-066-A06 | unit | `internal/telegram/help_test.go` | help lists examples and no retired command instructions |
| SCN-066-A07 | guard | `tests/integration/policy/legacy_absence_test.go` | `domain_intent.go` and `parseDomainIntent` absent |
| SCN-066-A08 | e2e-api | `tests/e2e/assistant/annotation_intent_test.go` | annotation classification comes from compiled intent |
| SCN-066-A09 | unit | `internal/telegram/operational_commands_test.go` | `/status` bypasses LLM |

`/bubbles.plan` must include a consumer impact sweep for help text, BotCommands, docs, tests, API clients, and fixtures.

## Alternatives And Tradeoffs

| Option | Decision | Rationale |
|--------|----------|-----------|
| Alias rewrite in facade | Rejected | Would make the facade transport-aware |
| No alias window | Rejected | Too abrupt for existing muscle memory |
| Hide old handlers but keep them | Rejected | Still violates the owner architecture |
| Standalone LLM annotation classifier | Rejected | Spec 068 compiler already owns user-turn slots |

## Risks And Open Questions

| Risk | Mitigation |
|------|------------|
| Alias mapping becomes a permanent grammar | Window guard and finite alias table |
| Docs/tests keep teaching retired commands | Consumer sweep and stale-reference tests |
| Write actions become too easy | Confirmation and side-effect gates |
