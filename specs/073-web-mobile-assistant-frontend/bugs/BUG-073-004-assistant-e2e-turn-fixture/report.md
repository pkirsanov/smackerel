# Report: BUG-073-004 - Contract-correct assistant live E2E fixtures

## Summary

The packet is active. Code inspection identifies a stale live-test fixture:
the affected tests use `/reset`, while the production facade intentionally
short-circuits reset before creating an invocation trace. The HTTP adapter's
canonical response transport remains `web`; `transport_hint` is telemetry-only.

**Claim Source:** interpreted

## Completion Statement

The parity fixture and exact-row isolation implementation are delivered and
tested on the isolated branch. The affected focused E2E, full Go/Python units,
format/check/lint, regression guard, traceability, and artifact lint pass.
Packet status and validate-owned certification intentionally remain
`in_progress` for parent synthesis consolidation.

**Claim Source:** executed

## Test Evidence

### Before Fix - Named Live E2E

**Executed:** YES (current session)
**Command:** `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test e2e --go-run 'TestAssistantTransportHintParity_WebAndMobileShareResponseShape|TestAssistantWebPWAChatE2E_ServedRouteHasComposerTranscriptAndResponseMarkup_TP_073_09|TestAssistantWebPWARetryE2E_'`
**Exit Code:** 1
**Output:**

```text
go-e2e: applying -run selector: TestAssistantTransportHintParity_WebAndMobileShareResponseShape|TestAssistantWebPWAChatE2E_ServedRouteHasComposerTranscriptAndResponseMarkup_TP_073_09|TestAssistantWebPWARetryE2E_
=== RUN   TestAssistantTransportHintParity_WebAndMobileShareResponseShape
--- PASS: TestAssistantTransportHintParity_WebAndMobileShareResponseShape (0.02s)
=== RUN   TestAssistantWebPWAChatE2E_ServedRouteHasComposerTranscriptAndResponseMarkup_TP_073_09
    web_pwa_chat_e2e_test.go:107: assistant.js must not reference forbidden auth surface "localStorage" (SCN-073-A11)
--- FAIL: TestAssistantWebPWAChatE2E_ServedRouteHasComposerTranscriptAndResponseMarkup_TP_073_09 (0.01s)
=== RUN   TestAssistantWebPWARetryE2E_SameTransportMessageIDDedupes_TP_073_10
    web_pwa_retry_e2e_test.go:67: trace.assistant_turn_id must be non-empty: first="" second=""
--- FAIL: TestAssistantWebPWARetryE2E_SameTransportMessageIDDedupes_TP_073_10 (0.16s)
=== RUN   TestAssistantWebPWARetryE2E_DifferentTransportMessageIDsAreDistinct_TP_073_10_Adversarial
    web_pwa_retry_e2e_test.go:89: trace.assistant_turn_id must be non-empty: a="" b=""
--- FAIL: TestAssistantWebPWARetryE2E_DifferentTransportMessageIDsAreDistinct_TP_073_10_Adversarial (0.01s)
FAIL
FAIL    github.com/smackerel/smackerel/tests/e2e/assistant      0.238s
FAIL: go-e2e (exit=1)
Running project-scoped test stack teardown (exit cleanup, timeout 180s)...
Volume smackerel-test-nats-data Removed
Volume smackerel-test-ollama-data Removed
Volume smackerel-test-postgres-data Removed
Network smackerel-test_default Removed
Command exited with code 1
```

**Result:** FAIL (expected pre-fix)

**Claim Source:** executed

### Initial Failure Classification

- Transport parity passed in isolation, so this run found no production
  response corruption and no hint-driven transport rewrite. The test still
  exercises only the reset short circuit, so it does not prove ordinary-turn
  parity; a normal-turn sentinel is being added RED-first.
- Both retry failures are exact consequences of the reset fixture: reset has
  no invocation and therefore no `assistant_turn_id` to compare.
- No package-order contamination is established by this isolated run.
- A deterministic ordinary-weather retry later proved a separate production
  HTTP dedup defect; that finding is routed to BUG-069-004 and is not owned by
  this packet.

**Claim Source:** interpreted

### Before Fix - Package Order

The original broad closeout supplied the package-order finding. In this isolated
worktree, the post-fix assistant-package run executed all 60 declarations; the
parity and exact-row tests passed in package order. The package remained red on
six unrelated environment/policy tests listed under Open Findings.

**Claim Source:** executed

### Parity Sentinel RED

**Executed:** YES (current session)
**Command:** `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test e2e --go-run '^TestAssistantTransportHintParity_WebAndMobileShareResponseShape$'`
**Exit Code:** 1
**Output:**

```text
go-e2e: applying -run selector: ^TestAssistantTransportHintParity_WebAndMobileShareResponseShape$
testing: warning: no tests to run
PASS
ok      github.com/smackerel/smackerel/tests/e2e        0.129s [no tests to run]
testing: warning: no tests to run
PASS
ok      github.com/smackerel/smackerel/tests/e2e/admin  0.005s [no tests to run]
testing: warning: no tests to run
PASS
ok      github.com/smackerel/smackerel/tests/e2e/agent  0.122s [no tests to run]
=== RUN   TestAssistantTransportHintParity_WebAndMobileShareResponseShape
  transport_hint_parity_test.go:150: hint="web" parity fixture reached the /reset short circuit; parity requires an ordinary text turn
--- FAIL: TestAssistantTransportHintParity_WebAndMobileShareResponseShape (0.01s)
FAIL
FAIL    github.com/smackerel/smackerel/tests/e2e/assistant      0.052s
FAIL: go-e2e (exit=1)
Running project-scoped test stack teardown (exit cleanup, timeout 180s)...
Volume smackerel-test-postgres-data Removed
Network smackerel-test_default Removed
```

**Result:** FAIL (expected pre-fix; proves stale reset fixture)

**Claim Source:** executed

### After Fix - Focused And Package E2E

Concrete tests:

- `tests/e2e/assistant/transport_hint_parity_test.go`
- `tests/e2e/assistant/conversation_isolation_test.go`

**Executed:** YES (current session)
**Command:** `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test e2e --go-run 'TestAssistantConversationIsolation_RestoresExactTargetAndPreservesNeighbor_Adversarial|TestAssistantTransportHintParity_WebAndMobileShareResponseShape|TestAssistantWebPWAChatE2E_ServedRouteHasComposerTranscriptAndResponseMarkup_TP_073_09|TestAssistantWebPWARetryE2E_'`
**Exit Code:** 0
**Output:**

```text
serialization guard: no processes, containers, networks, or volumes
=== RUN   TestAssistantConversationIsolation_RestoresExactTargetAndPreservesNeighbor_Adversarial
--- PASS: TestAssistantConversationIsolation_RestoresExactTargetAndPreservesNeighbor_Adversarial (0.02s)
=== RUN   TestAssistantTransportHintParity_WebAndMobileShareResponseShape
--- PASS: TestAssistantTransportHintParity_WebAndMobileShareResponseShape (17.29s)
=== RUN   TestAssistantWebPWAChatE2E_ServedRouteHasComposerTranscriptAndResponseMarkup_TP_073_09
--- PASS: TestAssistantWebPWAChatE2E_ServedRouteHasComposerTranscriptAndResponseMarkup_TP_073_09 (0.01s)
=== RUN   TestAssistantWebPWARetryE2E_SameTransportMessageIDDedupes_TP_073_10
--- PASS: TestAssistantWebPWARetryE2E_SameTransportMessageIDDedupes_TP_073_10 (10.18s)
=== RUN   TestAssistantWebPWARetryE2E_DifferentTransportMessageIDsAreDistinct_TP_073_10_Adversarial
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

Full Go units passed after documenting the new helper package; Python units
reported `708 passed, 2 deselected`. Format reported `75 files already
formatted`; check passed with explicit `SMACKEREL_HARDWARE_TIER=cpu`; lint
reported `All checks passed!` and `Web validation passed`; regression-quality
guard reported 0 violations and 0 warnings.

**Claim Source:** executed

## Root Cause Evidence

- `internal/assistant/facade.go` handles reset before routing/invocation and
  returns `context reset.` after `DeleteByKey(user_id, transport)`.
- `internal/assistant/httpadapter/adapter.go` populates trace IDs only when
  `resp.Invocation != nil` and always emits `TransportName` (`web`).
- The parity E2E currently sends `Text: "/reset"` while claiming
  ordinary-turn parity.

**Claim Source:** interpreted

## Open Findings

- HTTP response dedup is tracked independently in BUG-069-004.
- Assistant-package order was executed. Six unrelated tests remain red because
  their required environment/policy wiring is absent: metrics URL, replay
  enablement, legacy residual metric registration, and node for two legacy PWA
  renderer tests. The repaired parity/isolation tests pass in that same run.
