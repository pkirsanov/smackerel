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

### Annotation Classification Replacement

Runtime annotation classification no longer consults phrase maps for user-authored turns. Spec 068 produces `interaction_type`, `rating`, `target`, and `note` slots; borderline cases use spec 061 disambiguation.

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
