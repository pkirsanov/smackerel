# BUG-061-004 — `assistant_turn` structured log collapses distinct executor failures into a single `error_cause`

> **Bug ID:** BUG-061-004
> **Spec:** 061-conversational-assistant
> **Severity:** S3 (observability gap — does not affect end-user behavior but turns every executor failure into a multi-minute docker-exec triage session)
> **Discovered:** 2026-06-09 during `bubbles.devops` triage of `/weather provider_unavailable` (BUG-015-001 sibling)
> **Affected surface:** every smackerel scenario that hits an external provider or the LLM driver

---

## Symptoms

Every failed `assistant_turn` log line reports the same blunt instrument:

```json
{
  "msg": "assistant_turn",
  "scenario_id": "weather_query",
  "status": "saved_as_idea",
  "error_cause": "provider_unavailable",
  "latency_ms": 4591,
  ...
}
```

`error_cause:"provider_unavailable"` is produced by
`translateOutcomeToErrorCause(outcome)` from a **3-way fan-in**:

1. `OutcomeProviderError` — `llm_driver_error` (Ollama returned an error)
2. `OutcomeProviderError` — `llm_returned_no_tool_calls_and_no_final` (Ollama returned a parseable response with no useful content)
3. `OutcomeTimeout` — `deadline_exceeded_after_provider_response` OR `provider_did_not_respond_before_deadline`

`OutcomeProviderError` itself is set in **two** places in
`internal/agent/executor.go` (lines 443 and 474):

- Line 443: `e.driver.Turn(ctx, req)` returned non-nil error
- Line 474: `len(resp.ToolCalls) == 0 && len(resp.Final) == 0` after a successful LLM call

The executor faithfully records the distinguishing detail in
`InvocationResult.OutcomeDetail` (e.g. `{"error":"llm_driver_error","detail":"dial tcp ollama:11434: connect: connection refused"}`),
but the facade's structured log line **discards `OutcomeDetail`
entirely**. Operators must `docker exec ... ollama ps` / `docker logs`
to find the actual failure — which is exactly what we did today in
BUG-015-001 triage.

---

## Reproduction

(Pre-fix) Take down ollama or any LLM dependency:

```bash
ssh <deploy-host> 'docker stop smackerel-home-lab-ollama-1'
# Send /weather <city> via Telegram
ssh <deploy-host> 'docker logs smackerel-home-lab-smackerel-core-1 --since 1m \
              | grep assistant_turn'
# Pre-fix: only error_cause:"provider_unavailable" — no clue ollama is down.
# Post-fix: includes outcome="provider-error", outcome_detail="error=llm_driver_error detail=dial tcp ollama:11434: connect: connection refused".
```

---

## Fix

`internal/assistant/facade.go` — extend the deferred
`slog.Info("assistant_turn", ...)` block to attach 3-5 additional
fields when `invocation.Outcome != OutcomeOK`:

| New field | Source | Purpose |
|-----------|--------|---------|
| `outcome` | `invocation.Outcome` (raw enum string) | Distinguishes OutcomeProviderError vs OutcomeTimeout vs OutcomeToolError vs OutcomeToolReturnInvalid vs OutcomeAllowlistViolation vs … |
| `outcome_iterations` | `invocation.Iterations` | Number of LLM round-trips before the failure (1 = first turn failed; >1 = the LLM tried tools, recovered, and still failed at the final turn) |
| `outcome_detail` | redacted summary of `invocation.OutcomeDetail` map | The discriminator inside OutcomeProviderError (`llm_driver_error` vs `llm_returned_no_tool_calls_and_no_final`) and any provider-side error text |
| `provider` | `invocation.Provider` | Which LLM driver produced the response (e.g. `ollama`) |
| `model` | `invocation.Model` | Which model was used (e.g. `gemma4:26b`) |

Helper `summarizeOutcomeDetail(detail map[string]any) string` renders
the map into deterministically-ordered `key=value` pairs, with
per-value cap of 200 runes and total cap of 512 runes.

### Why these fields are safe to log

`internal/agent/executor.go` builds `OutcomeDetail` at exactly 9
sites. Every value is one of:

- A static error string (`"executor.Run called with nil scenario"`,
  `"llm_returned_no_tool_calls_and_no_final"`)
- A tool name (`"weather_lookup"`)
- An integer (deadline, retry count, iteration count)
- A provider-side error text (`"open-meteo: 503 service unavailable"`,
  `"dial tcp ollama:11434: connect: connection refused"`)

**None of these contain user message bodies.** The
`body_redacted: true` Principle 8 affirmation in the log line stays
true. The per-value cap is a defense-in-depth limit in case a future
executor change accidentally inlines user content.

### Why the gate is `invocation.Outcome != OutcomeOK`

OK turns are the dominant case (logging 5 extra fields on every
healthy turn would bloat the log without value). Failure turns are
rare AND high-value-to-debug — that's the right place to spend
log volume.

---

## Verification

Build + facade tests:

```bash
$ cd ~/smackerel && go build ./... 2>&1 | tail -3
[go build exit 0]

$ go test -count=1 -timeout 60s ./internal/assistant/ 2>&1 | tail -3
ok  github.com/smackerel/smackerel/internal/assistant  0.566s
```

Live verification requires the next deploy + a forced failure
(e.g. `docker stop ollama` + `/weather`); will be exercised after
the deploy in this same session.

---

## Definition of Done

- [x] `assistant_turn` log adds `outcome` / `outcome_iterations` / `outcome_detail` / `provider` / `model` when outcome != OK
- [x] `summarizeOutcomeDetail` helper bounds each value to 200 runes and total to 512 runes
- [x] `body_redacted: true` Principle 8 affirmation preserved
- [x] Build + facade unit tests pass
- [ ] Live verification on <deploy-host> post-deploy (next CI cycle)

## Files changed

- `internal/assistant/facade.go` — moved `invocation` declaration up, added
  the new log fields, added `summarizeOutcomeDetail` helper, added `sort`
  and `unicode/utf8` imports

## Related work

- BUG-015-001 (ollama volume) — `provider_unavailable` masking made
  this 30-minute triage instead of a 30-second one
- BUG-061-005 (telegram bot DNS-race silent disable) — same theme:
  the smackerel-core silently disabled a transport without making
  the operator-visible signal loud enough
