# Report: BUG-069-004 - HTTP assistant turn deduplication

## Summary

Deterministic live RED confirms a production adapter defect: two identical
authenticated `/weather in barcelona` requests with one
`transport_message_id` executed twice and returned different non-empty
assistant turn IDs. The different-ID adversary passed.

**Claim Source:** executed

## Completion Statement

Bounded auth-scoped HTTP response replay is delivered and tested on the
isolated branch. Sequential/concurrent replay, privacy/payload adversaries,
capture-once, accepted-error replay, strict capacity/expiry, focused live E2E,
integration, full units, and quality/packet gates pass. Packet status and
validate-owned certification intentionally remain `in_progress` for parent
synthesis consolidation.

**Claim Source:** executed

## Test Evidence

### Before Fix - Deterministic Same-ID Retry

**Executed:** YES (current session)
**Command:** `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test e2e --go-run '^TestAssistantWebPWARetryE2E_'`
**Exit Code:** 1
**Output:**

```text
serialization guard: no processes, containers, networks, or volumes
go-e2e: applying -run selector: ^TestAssistantWebPWARetryE2E_
=== RUN   TestAssistantWebPWARetryE2E_SameTransportMessageIDDedupes_TP_073_10
    web_pwa_retry_e2e_test.go:65: turn_id="spec-073-scope-2-a03-tp-073-10-retry-20260719T164001.263526" assistant_turn_id="trace_20260719T164001.274269423_6" agent_trace_id="trace_20260719T164001.274269423_6" status="checking_weather"
    web_pwa_retry_e2e_test.go:68: turn_id="spec-073-scope-2-a03-tp-073-10-retry-20260719T164001.263526" assistant_turn_id="trace_20260719T164009.110344202_8" agent_trace_id="trace_20260719T164009.110344202_8" status="checking_weather"
    web_pwa_retry_e2e_test.go:74: retry with same transport_message_id produced different assistant_turn_id ("trace_20260719T164001.274269423_6" vs "trace_20260719T164009.110344202_8") - dedup contract violated
--- FAIL: TestAssistantWebPWARetryE2E_SameTransportMessageIDDedupes_TP_073_10 (17.72s)
=== RUN   TestAssistantWebPWARetryE2E_DifferentTransportMessageIDsAreDistinct_TP_073_10_Adversarial
    web_pwa_retry_e2e_test.go:89: turn_id="spec-073-scope-2-a03-tp-073-10-adv-A-20260719T164018.985753" assistant_turn_id="trace_20260719T164018.990322031_10" agent_trace_id="trace_20260719T164018.990322031_10" status="checking_weather"
    web_pwa_retry_e2e_test.go:90: turn_id="spec-073-scope-2-a03-tp-073-10-adv-B-20260719T164018.985753" assistant_turn_id="trace_20260719T164029.020935514_11" agent_trace_id="trace_20260719T164029.020935514_11" status="checking_weather"
--- PASS: TestAssistantWebPWARetryE2E_DifferentTransportMessageIDsAreDistinct_TP_073_10_Adversarial (15.10s)
FAIL
FAIL    github.com/smackerel/smackerel/tests/e2e/assistant      32.822s
FAIL: go-e2e (exit=1)
Running project-scoped test stack teardown (exit cleanup, timeout 180s)...
Volume smackerel-test-postgres-data Removed
Volume smackerel-test-nats-data Removed
Volume smackerel-test-ollama-data Removed
Network smackerel-test_default Removed
```

**Result:** FAIL (expected pre-fix; genuine duplicate execution)

**Claim Source:** executed

### After Fix - Focused And Package E2E

Concrete tests:

- `tests/e2e/assistant/web_pwa_retry_e2e_test.go`
- `internal/assistant/httpadapter/dedup_test.go`
- `tests/integration/api/assistant_http_turn_test.go`

**Executed:** YES (current session)
**Command:** `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test e2e --go-run 'TestAssistantConversationIsolation_RestoresExactTargetAndPreservesNeighbor_Adversarial|TestAssistantTransportHintParity_WebAndMobileShareResponseShape|TestAssistantWebPWAChatE2E_ServedRouteHasComposerTranscriptAndResponseMarkup_TP_073_09|TestAssistantWebPWARetryE2E_'`
**Exit Code:** 0
**Output:**

```text
=== RUN   TestAssistantWebPWARetryE2E_SameTransportMessageIDDedupes_TP_073_10
    web_pwa_retry_e2e_test.go:66: turn_id="spec-073-scope-2-a03-tp-073-10-retry-20260719T165628.978945" assistant_turn_id="trace_20260719T165628.985808947_10" status="checking_weather"
    web_pwa_retry_e2e_test.go:69: turn_id="spec-073-scope-2-a03-tp-073-10-retry-20260719T165628.978945" assistant_turn_id="trace_20260719T165628.985808947_10" status="checking_weather"
--- PASS: TestAssistantWebPWARetryE2E_SameTransportMessageIDDedupes_TP_073_10 (10.18s)
=== RUN   TestAssistantWebPWARetryE2E_DifferentTransportMessageIDsAreDistinct_TP_073_10_Adversarial
    web_pwa_retry_e2e_test.go:91: turn_id="spec-073-scope-2-a03-tp-073-10-adv-A-20260719T165639.159826" assistant_turn_id="trace_20260719T165639.167438960_11" status="checking_weather"
    web_pwa_retry_e2e_test.go:92: turn_id="spec-073-scope-2-a03-tp-073-10-adv-B-20260719T165639.159826" assistant_turn_id="trace_20260719T165644.018243356_12" status="checking_weather"
--- PASS: TestAssistantWebPWARetryE2E_DifferentTransportMessageIDsAreDistinct_TP_073_10_Adversarial (9.71s)
PASS
ok      github.com/smackerel/smackerel/tests/e2e/assistant
PASS: go-e2e
Running project-scoped test stack teardown (exit cleanup, timeout 180s)...
Volume smackerel-test-postgres-data Removed
Volume smackerel-test-nats-data Removed
Volume smackerel-test-ollama-data Removed
Network smackerel-test_default Removed
```

**Result:** PASS

**Claim Source:** executed

### Units And Quality Gates

**Executed:** YES (current session)
**Command:** `./smackerel.sh test unit --go --go-run 'TestHTTPTurnDedup' --verbose`
**Exit Code:** 0
**Output:**

```text
=== RUN   TestHTTPTurnDedup_SequentialReplayExecutesFacadeOnce
--- PASS: TestHTTPTurnDedup_SequentialReplayExecutesFacadeOnce (0.01s)
=== RUN   TestHTTPTurnDedup_ConcurrentReplayExecutesFacadeOnce
--- PASS: TestHTTPTurnDedup_ConcurrentReplayExecutesFacadeOnce (0.00s)
=== RUN   TestHTTPTurnDedup_SameIDIsIsolatedAcrossUsers
--- PASS: TestHTTPTurnDedup_SameIDIsIsolatedAcrossUsers (0.00s)
=== RUN   TestHTTPTurnDedup_ChangedPayloadConflictsWithoutReexecution
--- PASS: TestHTTPTurnDedup_ChangedPayloadConflictsWithoutReexecution (0.00s)
=== RUN   TestHTTPTurnDedup_AcceptedFailureIsReplayed
--- PASS: TestHTTPTurnDedup_AcceptedFailureIsReplayed (0.00s)
=== RUN   TestHTTPTurnDedup_CacheExpiresAndEvictsCompletedEntries
--- PASS: TestHTTPTurnDedup_CacheExpiresAndEvictsCompletedEntries (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/assistant/httpadapter  0.038s
[go-unit] go test ./... finished OK
```

Focused integration returned `PASS: go-integration` and removed all disposable
resources. Full Go units passed; Python units reported `708 passed, 2
deselected`; format reported `75 files already formatted`; check and lint
passed; regression-quality guard reported 0 violations and 0 warnings.

**Claim Source:** executed

**Claim Source:** not-run

## Open Findings

- Process-local response replay is appropriate for the current single-ingress
  deployment; any future multi-replica topology requires a separate durable
  dedup design before scale-out.
- The assistant package run executed all 60 tests. The fixed retry tests passed
    in package order; six unrelated environment/policy tests remain red (missing
    metrics URL, disabled replay, missing legacy metric, and missing node for two
    legacy renderer tests).
