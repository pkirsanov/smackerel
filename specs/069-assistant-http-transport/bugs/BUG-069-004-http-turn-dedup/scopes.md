# Scopes: BUG-069-004 - HTTP assistant turn deduplication

## Scope 1: Add bounded auth-scoped response replay and live regression coverage

**Status:** Done

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
| Regression E2E API - SCN-BUG069004-001 | e2e-api | `tests/e2e/assistant/web_pwa_retry_e2e_test.go` | Regression: `TestAssistantWebPWARetryE2E_SameTransportMessageIDDedupes_TP_073_10` keeps a same-ID retry replaying one logical turn instead of executing twice | `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test e2e --go-run '^TestAssistantWebPWARetryE2E_SameTransportMessageIDDedupes_TP_073_10$'` | Yes |
| Regression E2E API - SCN-BUG069004-003 | e2e-api | `tests/e2e/assistant/web_pwa_retry_e2e_test.go` | Regression: `TestAssistantWebPWARetryE2E_DifferentTransportMessageIDsAreDistinct_TP_073_10_Adversarial` keeps distinct transport message IDs executing distinct turns, so replay cannot over-collapse | `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test e2e --go-run '^TestAssistantWebPWARetryE2E_DifferentTransportMessageIDsAreDistinct_TP_073_10_Adversarial$'` | Yes |
| Regression E2E API - shared-identity row isolation | e2e-api | `tests/e2e/assistant/conversation_isolation_test.go` | Regression: `TestAssistantConversationIsolation_RestoresExactTargetAndPreservesNeighbor_Adversarial` keeps the exact shared-identity conversation row restored and neighbour rows unchanged | `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test e2e --go-run '^TestAssistantConversationIsolation_RestoresExactTargetAndPreservesNeighbor_Adversarial$'` | Yes |
| Regression E2E API - broader assistant suite | e2e-api | `tests/e2e/assistant/` | Regression: the broader assistant E2E package runs in package order so dedup replay does not regress neighbouring HTTP turn, confirm, disambiguation, and capture flows | exact anchored selector generated from package test declarations and passed to `./smackerel.sh test e2e --go-run` | Yes |
| HTTP adapter integration | integration | `tests/integration/api/assistant_http_turn_test.go` | Same-ID retry invokes the adapter facade boundary once | `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test integration --go-run 'TestAssistantHTTPTurn|TestAssistantHTTPAuth_|TestAssistantTransportParity_'` | Yes |
| Assistant package order | e2e-api | `tests/e2e/assistant/` | Entire assistant package executes in package order | exact anchored selector generated from package test declarations and passed to `./smackerel.sh test e2e --go-run` | Yes |
| Impacted units | unit | `internal/assistant/httpadapter/`, `ml/tests/` | Full Go and Python regression lanes | `./smackerel.sh test unit --go`; `./smackerel.sh test unit --python` | No |
| Quality gates | guard | changed files and packet | Check, lint, format, regression and packet gates | repo CLI plus Bubbles guards | No |

### Definition of Done

- [x] Deterministic same-ID ordinary-turn RED proves duplicate facade execution.
  - Evidence: [report.md → Current-Session Mutation Verification](report.md#current-session-mutation-verification--do-the-dedup-tests-bind-their-claim-2026-08-23). The cache-hit branch in `dedup.go` `begin()` was short-circuited and the suite re-run: `--- FAIL: TestHTTPTurnDedup_SequentialReplayExecutesFacadeOnce` with `dedup_test.go:133: Facade.Handle calls=2, want 1`, `MUTATED_RC=1`. Duplicate facade execution is therefore demonstrated, not asserted. Restored by editing the file back (not a shell `trap`, which does not fire reliably in a persistent agent shell) and verified as its own step: `porcelain_lines=0`, `mutation_present=0`, then `GREEN_RC=0`.
- [x] SCN-BUG069004-001 - Sequential same-ID retry replays one logical turn: an exact retry executes the facade once and replays assistant turn ID, body, status, and emitted time.
  - Evidence: unit `--- PASS: TestHTTPTurnDedup_SequentialReplayExecutesFacadeOnce (0.01s)` and live E2E `--- PASS: TestAssistantWebPWARetryE2E_SameTransportMessageIDDedupes_TP_073_10 (0.21s)` in [report.md → Current-Session E2E](report.md#current-session-e2e--a-cold-start-false-red-found-and-fixed-2026-08-23). The E2E asserts both `assistant_turn_id` equality and `body` equality across the retry; under mutation this same test failed, so the assertion binds.
- [x] SCN-BUG069004-002 - Concurrent same-ID retries collapse: one request owns execution and all matching waiters receive its response.
  - Evidence: `--- PASS: TestHTTPTurnDedup_ConcurrentReplayExecutesFacadeOnce (0.01s)`. Under mutation: `dedup_test.go:156: concurrent Facade.Handle calls=2, want 1`. The mechanism is the owner lease in `dedup.go` — the owner executes while matching waiters block in `wait()` and receive the owner's result.
- [x] Retries replay logical response fields with a current request ID.
  - Evidence: asserted inside `TestHTTPTurnDedup_SequentialReplayExecutesFacadeOnce` at `dedup_test.go:138` — `if first.Trace.RequestID == "" || second.Trace.RequestID == "" || first.Trace.RequestID == second.Trace.RequestID { t.Fatalf("HTTP request IDs must be current per request: ...") }`. The production seam is `adapter.go:401`, `replayed.Trace.RequestID = requestID`, which overwrites the replayed trace's request ID with the current one. So the logical turn replays while the transport-level request ID stays current.
- [x] SCN-BUG069004-003 - Different IDs execute distinct turns: the adversary returns different non-empty assistant turn IDs.
  - Evidence: `--- PASS: TestAssistantWebPWARetryE2E_DifferentTransportMessageIDsAreDistinct_TP_073_10_Adversarial (0.06s)`. This is the adversarial guard against over-collapsing: it fails if the dedup fix were to merge turns it should not.
- [x] SCN-BUG069004-004 - Identity and payload collisions do not leak or re-execute: cross-user same-ID requests remain isolated and changed-payload reuse is rejected.
  - Evidence: `--- PASS: TestHTTPTurnDedup_SameIDIsIsolatedAcrossUsers (0.00s)` and `--- PASS: TestHTTPTurnDedup_ChangedPayloadConflictsWithoutReexecution (0.00s)`. Under mutation the payload-collision case failed with `dedup_test.go:183: statuses first=200 second=200, want 200/409`, proving the 409 rejection is real. Isolation is structural: the cache key is `{userDigest, transport, messageID}`, so a second user's identical message id hashes to a different key.
- [x] Cache expiry/capacity and accepted-error replay are covered.
  - Evidence: `--- PASS: TestHTTPTurnDedup_CacheExpiresAndEvictsCompletedEntries (0.00s)`, `--- PASS: TestHTTPTurnDedup_CacheRejectsUniqueWorkWhenAllSlotsAreInFlight (0.00s)`, `--- PASS: TestHTTPTurnDedup_AcceptedFailureIsReplayed (0.02s)`. The accepted-error case failed under mutation (`dedup_test.go:201: Facade.Handle calls=2, want 1 for replayed failure`), so replay of an accepted failure is genuinely exercised rather than incidentally true.
- [x] Exact shared-identity conversation row is restored; unrelated rows are unchanged.
  - Evidence: `--- PASS: TestAssistantConversationIsolation_RestoresExactTargetAndPreservesNeighbor_Adversarial (0.02s)` on the live disposable stack. This test passed in BOTH the pre-fix and post-fix E2E runs, which is expected and correct — it never posts an assistant turn, so it was unaffected by the facade-readiness defect that failed its four neighbours.
- [x] Focused and assistant-package E2E pass on the disposable stack.
  - Evidence: focused set `E2E2_RC=0` with 5/5 PASS; package run `PKG_RC=0` with `PASS=52 FAIL=0 SKIP=12`. Both in [report.md → Current-Session Broader Suite And Quality Gates](report.md#current-session-broader-suite-and-quality-gates-2026-08-23). All 12 skips are enumerated by name there and none is one of this packet's five required tests.
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior pass.
  - Evidence: the four Regression E2E API rows in the Test Plan above all executed and passed in the focused run — same-ID dedup, the adversarial distinct-ID case, shared-identity row isolation, and the broader package. `E2E2_RC=0`, zero SKIP in the required set.
- [x] Broader E2E regression suite passes.
  - Evidence: `./smackerel.sh test e2e --go-package assistant` → `PKG_RC=0`, `PASS=52 FAIL=0 SKIP=12`, zero failures.
- [x] Impacted units and check/lint/format/regression/packet gates pass.
  - Evidence: `UNIT_RC=0` with all eight `TestHTTPTurnDedup_` cases carrying a `=== RUN` line (not a vacuous `[no tests to run]` pass); `CHECK_RC=0`; `LINT_RC=0`; `FMT_RC=0` ("78 files already formatted"); regression-quality guard `RQG_RC=0` with `0 violation(s), 0 warning(s)` across the four required E2E files — which is the check that matters most here, since it confirms the newly added readiness wait did not become a silent-pass escape hatch.

Every item above carries current-session execution evidence. The mutation kill-test is what separates these from a passing-by-accident suite: removing the fix turns five of the eight unit cases red with this bug's exact signature.
