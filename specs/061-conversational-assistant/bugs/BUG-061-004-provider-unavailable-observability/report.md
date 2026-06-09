# BUG-061-004 — Execution evidence

## Pre-fix observation (recorded from live BUG-015-001 triage)

Failed `/weather 98569` at 2026-06-09T15:37:18Z (telegram correlation_id 560669442):

```json
{
  "time": "2026-06-09T15:37:18.802006287Z",
  "level": "INFO",
  "msg": "assistant_turn",
  "user_id": "<operator>",
  "transport": "telegram",
  "correlation_id": "560669442",
  "assistant_turn_id": "asst-1781019434211894170",
  "scenario_id": "weather_query",
  "top_score": 0,
  "band": "high",
  "status": "saved_as_idea",
  "error_cause": "provider_unavailable",
  "latency_ms": 4591.026435,
  "agent_trace_id": "trace-asst-1781019434211894170",
  "body_redacted": true
}
```

`error_cause: "provider_unavailable"` matched **three** distinct
underlying failures that could have produced it:

1. `OutcomeProviderError` with `OutcomeDetail.error="llm_driver_error"`
   (ollama unreachable — what actually happened)
2. `OutcomeProviderError` with `OutcomeDetail.error="llm_returned_no_tool_calls_and_no_final"`
   (ollama responded but LLM produced no useful output)
3. `OutcomeTimeout` with `OutcomeDetail.reason="deadline_exceeded_..."`
   (LLM call deadline-exceeded)

Operator had to `docker ps` to find ollama in `Exited (127)` state —
which took several minutes of investigation.

## Fix application

### Code changes

```bash
$ cd ~/smackerel && git --no-pager diff --stat internal/assistant/facade.go
 internal/assistant/facade.go | 88 ++++++++++++++++++++++++++++++++++++--------
 1 file changed, 73 insertions(+), 15 deletions(-)
```

Three modifications:

1. **Imports** — added `sort` and `unicode/utf8`.
2. **Variable scope** — moved `var invocation *agent.InvocationResult`
   up into the turn-scoped `var (...)` block so the deferred log
   block captures it cleanly.
3. **Log block** — built `logAttrs []any` slice, conditionally
   appended 3-5 new fields when outcome != OK.
4. **Helper** — added `summarizeOutcomeDetail(map[string]any) string`
   at end of file (60 lines including doc comment).

### Build verification

```bash
$ cd ~/smackerel && go build ./... 2>&1
[go build exit 0]
```

### Facade unit test verification

```bash
$ cd ~/smackerel && go test -count=1 -timeout 60s ./internal/assistant/ 2>&1 | tail
ok      github.com/smackerel/smackerel/internal/assistant       0.566s
[exit 0]
```

`facade_correlation_id_test.go` (which JSON-decodes the
`assistant_turn` log line) passes — the new fields are
append-only and don't break existing field assertions.

## Expected post-deploy behavior

After the next deploy of HEAD-of-main containing this fix, an
ollama-down `/weather` invocation would log:

```json
{
  "msg": "assistant_turn",
  "scenario_id": "weather_query",
  "status": "saved_as_idea",
  "error_cause": "provider_unavailable",
  "outcome": "provider-error",
  "outcome_iterations": 0,
  "outcome_detail": "detail=dial tcp ollama:11434: connect: connection refused error=llm_driver_error",
  "provider": "ollama",
  "model": "gemma4:26b",
  "body_redacted": true,
  ...
}
```

Operator reads `outcome_detail=dial tcp ollama:...` and immediately
investigates ollama — no docker-exec triage required.

## Files changed

- `internal/assistant/facade.go` — imports, var scope, log block,
  helper function (4 changes total in one file)

## Live verification

Deferred to the next deploy + a real failure event. The unit-level
evidence (build + facade tests) is sufficient to merge.
