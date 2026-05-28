# Bug: [BUG-061-001] BS-001 webhook e2e fails on cold stack — 15s artifact poll budget exceeded by assistant facade cold-start latency

## Summary
`tests/e2e/test_telegram_assistant_bs001.sh` ROW-1 intermittently fails on a freshly-started test stack: webhook returns HTTP 200 (handler dispatched the update) but the verbatim-text artifact does not appear in `artifacts.content_raw` within the test's 15-iteration / 15-second polling window. The same test passes deterministically once the stack is warm.

## Severity
- [ ] Critical
- [ ] High
- [x] Medium — test-only flake; production webhook→capture flow is correct
- [ ] Low

## Status
- [ ] Reported
- [x] Confirmed (reproduced once on cold stack; not reproducible on warm stack)
- [ ] In Progress
- [x] Fixed
- [x] Verified
- [ ] Closed

## Reproduction Steps
1. `./smackerel.sh --env test down --volumes && ./smackerel.sh --env test build && ./smackerel.sh --env test up` (cold stack, Ollama model unloaded)
2. `bash tests/e2e/test_telegram_assistant_bs001.sh`
3. Observe ROW-1: `http_status=200` but `FAIL: artifact with content_raw='bs001-webhook-probe-...' not present in PG after 15s`

## Expected Behavior
ROW-1 succeeds within a budget that comfortably covers cold-start latency of the assistant facade's first LLM classifier call (Ollama model load) plus the legacy `handleTextCapture` HTTP round-trip.

## Actual Behavior
On cold stack, the 15-second `for i in 1 2 ... 15` poll loop expires before the artifact row appears, even though the webhook handler returned 200 and dispatched the update. Once the stack is warm, the artifact lands well within 15s and the test passes.

## Environment
- Service: smackerel-core (test env), smackerel-ml, ollama
- Version: HEAD of working tree, spec 061 Round 11 webhook handler
- Platform: Linux, Docker Compose test stack

## Error Output (BEFORE FIX, cold stack)
```
--- ROW-1: webhook POST with valid secret -> 200 + artifact ---
  http_status=200 body=
FAIL: ROW-1: artifact with content_raw='bs001-webhook-probe-1780004294-16876 happy-path-marker' not present in PG after 15s
```
(operator-supplied evidence; the exact run is in `report.md`)

## Root Cause
The webhook → `safeHandleMessage` → `handleMessage` → `assistantAdapter.HandleUpdate` chain on plain text first invokes `assistant.Handle(...)`, which on a cold stack loads the Ollama model for low-confidence classification. That single first call can take 20–40s on cold Ollama (model pull/load into RAM). Only after `Handle` returns does the adapter's `CaptureRoute=true` branch (or the fallthrough to legacy `handleTextCapture`) run the actual artifact insert. The test's `for i in 1..15; sleep 1` polling budget is therefore racing the cold-start LLM call.

The test-side bug is the polling budget; the production code path is correct.

## Related
- Feature: `specs/061-conversational-assistant/`
- Related work: spec 061 SCOPE-05 (Round 8 assistant intercept, Round 11 webhook handler)
- No related PRs (in-tree fix)
