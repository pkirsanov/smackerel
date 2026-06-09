# BUG-061-004 — Design

## Root cause analysis

See `bug.md` § Symptoms. Summary: `translateOutcomeToErrorCause`
maps 5+ distinct executor failure paths into the single
`ErrProviderUnavailable` cause; the deferred log block at
`internal/assistant/facade.go:474` records only `error_cause`,
discarding the executor's `InvocationResult.OutcomeDetail` map
which carries the discriminator.

## Solution design

### Append-only log enrichment

The log line is the operator's first window into a failed turn.
Adding `outcome`, `outcome_iterations`, `outcome_detail`,
`provider`, `model` (5 fields) gives the operator the failure
class + retry depth + provider identity in one read.

### Conditional emission

Wrap the new fields in `if invocation != nil && invocation.Outcome != agent.OutcomeOK`.
Rationale:

- OK turns are 99%+ of traffic; adding 5 fields to every line
  bloats Loki / Promtail / shell-grep workflows.
- Failure turns are the high-value debug case; that's where
  log volume is justified.

### Closure capture of `invocation`

`invocation *agent.InvocationResult` is declared earlier in
`Handle()` (moved up to live alongside `turnBand`, `turnScenarioID`,
`turnTopScore`, `turnAssistantTurnID`) so the deferred log block
captures it by name. It's nil-checked because the deferred block
runs even on early-return paths (auth failure, context load
failure) where the executor never ran.

### `summarizeOutcomeDetail` helper

Pure function. No I/O. Deterministic:

1. Sort keys lexically (so the same failure produces the same
   summary across hosts / runs).
2. Render each value with `fmt.Sprintf("%v", ...)`.
3. Cap per-value rendering at 200 runes (defense-in-depth in case
   a future executor change inlines user content).
4. Concatenate as space-separated `key=value` pairs.
5. Cap total at 512 runes.

Lives at the bottom of `internal/assistant/facade.go` next to
other facade-private helpers.

### Why not a separate "errlog" subsystem

The log line is the operator's contract. Adding fields to the
existing line is the smallest, safest change. A dedicated error
log channel would require:

- New ingestion config in Loki
- New dashboard panels
- New alerting rules

None of which is justified for closing this observability gap.

### Body-redaction safety audit

Every `OutcomeDetail = map[string]any{...}` site in
`internal/agent/executor.go`:

| Line | Value | User content? |
|------|-------|---------------|
| 300 | `{"error": "executor.Run called with nil scenario"}` | No (static) |
| 336 | `{"deadline_s": int, "reason": "provider_did_not_respond_before_deadline"}` | No |
| 345 | `{"reason": "deadline_exceeded_after_provider_response", "deadline_s": int}` | No |
| 408 | `{"error": "llm_returned_no_tool_calls_and_no_final"}` | No (static) |
| 437 | `{"error": "schema retry budget exhausted", "retries": int, "detail": err.Error()}` | err is from JSON unmarshal of LLM output — no user content |
| 444 | `{"error": "llm_driver_error", "detail": err.Error()}` | err is network/HTTP error from LLM driver — no user content |
| 455 | `{"error": "tool_arg_schema_violation", "tool": <tool-name>, "retries": int}` | No (static + tool name) |
| 475 | `{"reason": "iteration limit exceeded", "iterations": int}` | No |
| 487 | `{"tool": <tool-name>, "detail": <tool-error-text>}` | tool-error-text comes from external provider (e.g. open-meteo "503") — no user content |
| 502 | `{"reason": "tool_return_schema_violation", "tool": <tool-name>}` | No |
| 638 | (allowlist violation) | No |

None contain user message bodies. The `body_redacted: true`
affirmation in the log line remains accurate.

The per-value 200-rune cap is paranoia, not necessity — added
in case a future executor change accidentally inlines tool
arguments (which CAN contain user content) into OutcomeDetail.
