# Scopes: BUG-069-004 - HTTP assistant turn deduplication

## Scope 1: Add bounded auth-scoped response replay and live regression coverage

**Status:** In Progress

**Scope-Kind:** runtime-behavior

### Gherkin Scenarios

```gherkin
Scenario: Sequential same-ID retry replays one logical turn
  Given an authenticated user submits a deterministic weather turn
  When the exact request is repeated with the same transport message ID
  Then the facade executes once and both responses share assistant turn ID and body

Scenario: Concurrent same-ID retries collapse
  Given two matching authenticated requests arrive together
  When the first facade invocation is still running
  Then the second waits and receives the first logical response

Scenario: Different IDs execute distinct turns
  Given equivalent requests use different message IDs
  When both are processed
  Then both non-empty assistant turn IDs differ

Scenario: Identity and payload collisions do not leak or re-execute
  Given two users reuse one message ID or one user changes the request body
  When the adapter resolves the idempotency key
  Then users remain isolated and changed-payload reuse is rejected before execution
```

### Implementation Plan

1. Preserve the deterministic ordinary-weather RED evidence.
2. Add bounded concurrency-safe response replay inside `httpadapter`.
3. Add unit/integration adversaries for sequential, concurrent, cross-user,
   changed-payload, failure replay, expiry, and capacity behavior.
4. Add exact-row E2E state isolation and run live retry regressions.
5. Run assistant package E2E, impacted units, quality, and packet gates.

### Test Plan

| Test Type | Category | File/Location | Description | Command | Live System |
|-----------|----------|---------------|-------------|---------|-------------|
| SCN-BUG069004-001 | e2e-api | `tests/e2e/assistant/web_pwa_retry_e2e_test.go` | Sequential same-ID retry replays one logical turn | `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test e2e --go-run '^TestAssistantWebPWARetryE2E_'` | Yes |
| SCN-BUG069004-002 | unit | `internal/assistant/httpadapter/dedup_test.go` | Concurrent same-ID retries collapse | `./smackerel.sh test unit --go --go-run 'TestHTTPTurnDedup' --verbose` | No |
| SCN-BUG069004-003 | e2e-api | `tests/e2e/assistant/web_pwa_retry_e2e_test.go` | Different IDs execute distinct turns | focused PWA retry E2E command | Yes |
| SCN-BUG069004-004 | unit | `internal/assistant/httpadapter/dedup_test.go` | Identity and payload collisions do not leak or re-execute | focused dedup unit command | No |
| HTTP adapter integration | integration | `tests/integration/api/assistant_http_turn_test.go` | Same-ID retry invokes the adapter facade boundary once | `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test integration --go-run 'TestAssistantHTTPTurn|TestAssistantHTTPAuth_|TestAssistantTransportParity_'` | Yes |
| Assistant package order | e2e-api | `tests/e2e/assistant/` | Entire assistant package executes in package order | exact anchored selector generated from package test declarations and passed to `./smackerel.sh test e2e --go-run` | Yes |
| Impacted units | unit | `internal/assistant/httpadapter/`, `ml/tests/` | Full Go and Python regression lanes | `./smackerel.sh test unit --go`; `./smackerel.sh test unit --python` | No |
| Quality gates | guard | changed files and packet | Check, lint, format, regression and packet gates | repo CLI plus Bubbles guards | No |

### Definition of Done

- [ ] Deterministic same-ID ordinary-turn RED proves duplicate facade execution.
- [ ] SCN-BUG069004-001 - Sequential same-ID retry replays one logical turn: an exact retry executes the facade once and replays assistant turn ID, body, status, and emitted time.
- [ ] SCN-BUG069004-002 - Concurrent same-ID retries collapse: one request owns execution and all matching waiters receive its response.
- [ ] Retries replay logical response fields with a current request ID.
- [ ] SCN-BUG069004-003 - Different IDs execute distinct turns: the adversary returns different non-empty assistant turn IDs.
- [ ] SCN-BUG069004-004 - Identity and payload collisions do not leak or re-execute: cross-user same-ID requests remain isolated and changed-payload reuse is rejected.
- [ ] Cache expiry/capacity and accepted-error replay are covered.
- [ ] Exact shared-identity conversation row is restored; unrelated rows are unchanged.
- [ ] Focused and assistant-package E2E pass on the disposable stack.
- [ ] Impacted units and check/lint/format/regression/packet gates pass.

All items remain unchecked until current-session execution evidence is recorded.
