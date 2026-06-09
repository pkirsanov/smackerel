# BUG-061-004 — Spec

## Expected behavior

When the assistant executor produces an outcome other than
`OutcomeOK`, the per-turn `assistant_turn` structured log MUST
include enough information for an operator to identify the
underlying failure without `docker exec` or `docker logs` triage.

Specifically, when `invocation.Outcome != OutcomeOK`:

1. The log MUST include `outcome` (the raw `agent.Outcome` enum
   value, e.g. `provider-error`, `timeout`, `tool-error`).
2. The log MUST include `outcome_iterations` (the number of LLM
   round-trips the executor performed before giving up).
3. The log MUST include `outcome_detail` — a deterministic
   rendering of `invocation.OutcomeDetail` capped at 512 runes
   total, with per-value cap of 200 runes.
4. When `invocation.Provider` is set, the log MUST include `provider`.
5. When `invocation.Model` is set, the log MUST include `model`.

`body_redacted: true` (Principle 8 affirmation) MUST remain in
every line, regardless of outcome. User message content MUST NOT
appear in any of the new fields.

For `OutcomeOK` turns, the log line MUST remain unchanged — the
new fields are emitted only on failure, to keep happy-path log
volume bounded.

## Out of scope

- Adding `outcome_detail` to OK turns (would bloat the log; the
  dominant case carries no debug value).
- Restructuring `agent.Outcome` to be a stricter discriminated
  union with one enum value per failure mode (would force a
  facade-wide refactor; the summary field is sufficient).
- Logging the full `result.ToolCalls[]` slice (would leak tool
  arguments which may include user content).
