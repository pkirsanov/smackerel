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

### Current-Session Mutation Verification — Do The Dedup Tests Bind Their Claim? (2026-08-23)

**Executed:** YES (current session) · **Claim Source:** executed

The 2026-07-19 RED→GREEN above is a prior-session record. This section re-establishes, in the current session, that the delivered dedup mechanism is real and that its tests would actually fail if it were removed — an exit 0 alone cannot show that, which is the failure class the sibling packet BUG-069-005 exists to police.

**Step 1 — baseline GREEN at clean HEAD.**

```text
$ ./smackerel.sh test unit --go --go-run 'TestHTTPTurnDedup' --verbose
=== RUN   TestHTTPTurnDedup_SequentialReplayExecutesFacadeOnce
--- PASS: TestHTTPTurnDedup_SequentialReplayExecutesFacadeOnce (0.01s)
=== RUN   TestHTTPTurnDedup_ConcurrentReplayExecutesFacadeOnce
--- PASS: TestHTTPTurnDedup_ConcurrentReplayExecutesFacadeOnce (0.01s)
=== RUN   TestHTTPTurnDedup_SameIDIsIsolatedAcrossUsers
--- PASS: TestHTTPTurnDedup_SameIDIsIsolatedAcrossUsers (0.00s)
=== RUN   TestHTTPTurnDedup_ChangedPayloadConflictsWithoutReexecution
--- PASS: TestHTTPTurnDedup_ChangedPayloadConflictsWithoutReexecution (0.00s)
=== RUN   TestHTTPTurnDedup_AcceptedFailureIsReplayed
--- PASS: TestHTTPTurnDedup_AcceptedFailureIsReplayed (0.02s)
=== RUN   TestHTTPTurnDedup_CaptureRouteRunsCaptureOnce
--- PASS: TestHTTPTurnDedup_CaptureRouteRunsCaptureOnce (0.01s)
=== RUN   TestHTTPTurnDedup_CacheExpiresAndEvictsCompletedEntries
--- PASS: TestHTTPTurnDedup_CacheExpiresAndEvictsCompletedEntries (0.00s)
=== RUN   TestHTTPTurnDedup_CacheRejectsUniqueWorkWhenAllSlotsAreInFlight
--- PASS: TestHTTPTurnDedup_CacheRejectsUniqueWorkWhenAllSlotsAreInFlight (0.00s)
ok      github.com/smackerel/smackerel/internal/assistant/httpadapter   0.134s
UNIT_RC=0
```

All eight EXECUTED — each carries a `=== RUN` line, so this is not a vacuous `[no tests to run]` pass under a selector that matched nothing.

**Step 2 — RED under a controlled mutation.** The cache-hit branch in `internal/assistant/httpadapter/dedup.go` `begin()` was short-circuited (`if entry, ok := c.entries[key]; ok && false`), which disables replay while leaving every other code path intact.

```text
$ ./smackerel.sh test unit --go --go-run 'TestHTTPTurnDedup' --verbose
--- FAIL: TestHTTPTurnDedup_SequentialReplayExecutesFacadeOnce (0.02s)
    dedup_test.go:133: Facade.Handle calls=2, want 1
--- FAIL: TestHTTPTurnDedup_ConcurrentReplayExecutesFacadeOnce (0.02s)
    dedup_test.go:156: concurrent Facade.Handle calls=2, want 1
--- PASS: TestHTTPTurnDedup_SameIDIsIsolatedAcrossUsers (0.01s)
--- FAIL: TestHTTPTurnDedup_ChangedPayloadConflictsWithoutReexecution (0.01s)
    dedup_test.go:183: statuses first=200 second=200, want 200/409
--- FAIL: TestHTTPTurnDedup_AcceptedFailureIsReplayed (0.01s)
    dedup_test.go:201: Facade.Handle calls=2, want 1 for replayed failure
--- FAIL: TestHTTPTurnDedup_CaptureRouteRunsCaptureOnce (0.01s)
    dedup_test.go:221: facade calls=2 capture calls=2, want 1/1
--- PASS: TestHTTPTurnDedup_CacheExpiresAndEvictsCompletedEntries (0.00s)
--- PASS: TestHTTPTurnDedup_CacheRejectsUniqueWorkWhenAllSlotsAreInFlight (0.00s)
FAIL    github.com/smackerel/smackerel/internal/assistant/httpadapter   0.142s
MUTATED_RC=1
```

Five of eight fail, and they fail with exactly this bug's signature — `Facade.Handle calls=2, want 1`. Removing the fix reproduces duplicate execution, so the tests bind their claim rather than passing regardless. The three that still pass are correct to pass: `SameIDIsIsolatedAcrossUsers`, `CacheExpiresAndEvictsCompletedEntries`, and `CacheRejectsUniqueWorkWhenAllSlotsAreInFlight` do not depend on the cache-HIT branch that was disabled. A mutation that killed all eight would have been the weaker result, because it would suggest the suite cannot separate the replay path from cache bookkeeping.

**Step 3 — explicit restore, verified as its own step.** The mutation was reverted by editing the file back rather than by relying on a shell `trap`, which does not fire reliably in a persistent agent shell.

```text
$ git status --porcelain
porcelain_lines=0
$ grep -c 'ok && false' internal/assistant/httpadapter/dedup.go
mutation_present=0
```

**Step 4 — GREEN restored.** All eight PASS again, `GREEN_RC=0`, `ok github.com/smackerel/smackerel/internal/assistant/httpadapter 0.197s`.

### Current-Session E2E — A Cold-Start False RED Found And Fixed (2026-08-23)

**Executed:** YES (current session) · **Claim Source:** executed

Re-running this packet's own required E2E set in the current session exposed a second test-integrity defect, this time the mirror image of the false-green class: a false RED. Four of the five required tests failed on a cold stack in ~0.04s each — far too fast for a real HTTP turn.

**RED — before the harness fix.**

```text
$ SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test e2e --go-run 'TestAssistantConversationIsolation_RestoresExactTargetAndPreservesNeighbor_Adversarial|TestAssistantTransportHintParity_WebAndMobileShareResponseShape|TestAssistantWebPWAChatE2E_ServedRouteHasComposerTranscriptAndResponseMarkup_TP_073_09|TestAssistantWebPWARetryE2E_'
--- PASS: TestAssistantConversationIsolation_RestoresExactTargetAndPreservesNeighbor_Adversarial (0.03s)
    transport_hint_parity_test.go:151: status = 503, want 200 (hint="web"); body={"error_cause":"assistant_http_not_ready","facade_invoked":false}
--- FAIL: TestAssistantTransportHintParity_WebAndMobileShareResponseShape (0.05s)
    web_pwa_chat_e2e_test.go:123: status=503, want 200; body={"error_cause":"assistant_http_not_ready","facade_invoked":false}
--- FAIL: TestAssistantWebPWAChatE2E_ServedRouteHasComposerTranscriptAndResponseMarkup_TP_073_09 (0.05s)
    web_pwa_retry_e2e_test.go:66: status=503, want 200; body={"error_cause":"assistant_http_not_ready","facade_invoked":false}
--- FAIL: TestAssistantWebPWARetryE2E_SameTransportMessageIDDedupes_TP_073_10 (0.04s)
    web_pwa_retry_e2e_test.go:91: status=503, want 200; body={"error_cause":"assistant_http_not_ready","facade_invoked":false}
--- FAIL: TestAssistantWebPWARetryE2E_DifferentTransportMessageIDsAreDistinct_TP_073_10_Adversarial (0.04s)
FAIL    github.com/smackerel/smackerel/tests/e2e/assistant      0.251s
E2E_RC=1
```

**Diagnosis, not a retry.** `error_cause="assistant_http_not_ready"` with `facade_invoked=false` is not a dedup regression — the turn never reached the facade. `/api/health` returns 200 as soon as the core HTTP server listens, but the assistant facade wires asynchronously after the ML sidecar reports ready, so `waitHTTPTurnHealthy` alone lets a test fire into the gap. `TestAssistantConversationIsolation_...` passed because it never posts a turn. Re-running would not have fixed this: the stack is torn down after every run, so every run is a cold start.

**Fix — reuse the existing fail-loud helper rather than add a new escape.** `waitAssistantFacadeReady` (`tests/e2e/assistant/nl_facade_readiness_helper_test.go`, spec 076 SCOPE-4a) already polls a benign `/reset` turn until `facade_invoked` is true, and on deadline calls `t.Fatalf` with the last status and body. It was already used by two other specs and was simply not wired into these four. One call was added to each, after the existing health wait.

The choice of helper matters. A `t.Skipf` on 503 would also have turned the lane green, and would have been the wrong fix — it is precisely the pattern BUG-069-005 exists to remove. `waitAssistantFacadeReady` fails loud instead, so a genuine wiring failure still surfaces as a red test rather than a silent skip.

**GREEN — after the harness fix.**

```text
$ SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test e2e --go-run '<same selector>'
go-e2e: applying -run selector: TestAssistantConversationIsolation_...|TestAssistantTransportHintParity_...|TestAssistantWebPWAChatE2E_...|TestAssistantWebPWARetryE2E_
=== RUN   TestAssistantConversationIsolation_RestoresExactTargetAndPreservesNeighbor_Adversarial
--- PASS: TestAssistantConversationIsolation_RestoresExactTargetAndPreservesNeighbor_Adversarial (0.02s)
=== RUN   TestAssistantTransportHintParity_WebAndMobileShareResponseShape
--- PASS: TestAssistantTransportHintParity_WebAndMobileShareResponseShape (0.11s)
=== RUN   TestAssistantWebPWAChatE2E_ServedRouteHasComposerTranscriptAndResponseMarkup_TP_073_09
--- PASS: TestAssistantWebPWAChatE2E_ServedRouteHasComposerTranscriptAndResponseMarkup_TP_073_09 (0.04s)
=== RUN   TestAssistantWebPWARetryE2E_SameTransportMessageIDDedupes_TP_073_10
--- PASS: TestAssistantWebPWARetryE2E_SameTransportMessageIDDedupes_TP_073_10 (0.21s)
=== RUN   TestAssistantWebPWARetryE2E_DifferentTransportMessageIDsAreDistinct_TP_073_10_Adversarial
--- PASS: TestAssistantWebPWARetryE2E_DifferentTransportMessageIDsAreDistinct_TP_073_10_Adversarial (0.06s)
ok      github.com/smackerel/smackerel/tests/e2e/assistant      0.493s
PASS: go-e2e
E2E2_RC=0
```

Five of five PASS, zero SKIP, exit 0. The dedup assertions now actually execute against a bound facade, which is what makes `SameTransportMessageIDDedupes` meaningful evidence rather than an unreached assertion.

### Current-Session Broader Suite And Quality Gates (2026-08-23)

**Executed:** YES (current session) · **Claim Source:** executed

**Broader assistant E2E package.**

```text
$ SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test e2e --go-package assistant
PASS=52 FAIL=0 SKIP=12
PKG_RC=0
```

Zero failures. The 12 skips were enumerated by name rather than counted, because a skip count alone cannot show whether a *required* test skipped — the precise blind spot BUG-069-005 exists to close:

```text
--- SKIP: TestAssistantE2E_CaptureAcknowledgementIsCrossTransportIdentical_TP_074_17
--- SKIP: TestAssistantHTTPE2E_CaptureFallbackDedupWithinWindow_TP_074_11
--- SKIP: TestAssistantHTTPE2E_CaptureFallbackIsInviolable_TP_074_04
--- SKIP: TestAssistantHTTPE2E_CaptureFallbackOpenKnowledgeNoGround
--- SKIP: TestAssistantHTTPE2E_CaptureProvenanceIsDistinct_TP_074_07
--- SKIP: TestAssistantHTTPE2E_CaptureRouteInvokesCaptureOnceAndAcknowledges
--- SKIP: TestAssistantHTTPE2E_CaptureAcknowledgementMatchesTelegramShape
--- SKIP: TestIntentCompilerE2E_MalformedJSONBlocksRoutingAndCaptures
--- SKIP: TestLegacyRetirementE2E_AliasWindowRoutesPlainEnglishWithNotice
--- SKIP: TestLegacyRetirementE2E_ExpiredSlashCommandDoesNotInvokeScenario
--- SKIP: TestMicroToolsE2E_ConvertsThreeCupsFlourToGrams
--- SKIP: TestMicroToolsE2E_CalculatorRejectsUnsafeExpression
```

All 12 are capture-fallback, legacy-retirement, or micro-tools tests. **None is one of this packet's five required tests**, all of which PASS above. Worth noting for the sibling packet: `TestAssistantHTTPE2E_CaptureFallbackOpenKnowledgeNoGround` appears here as a SKIP, which is exactly the defect BUG-074-002 documents — a `t.Skipf` keyed on the very status the test hunts. That observation is recorded, not fixed here; it belongs to `specs/074-capture-as-fallback-policy/bugs/BUG-074-002-noground-e2e-skip-guard-masks-canonical-ack-regression`.

**Quality gates.**

```text
$ ./smackerel.sh lint            → LINT_RC=0   ("Web validation passed")
$ ./smackerel.sh format --check  → FMT_RC=0    ("78 files already formatted")
$ ./smackerel.sh check           → CHECK_RC=0  ("Config is in sync with SST", "scenario-lint: OK")
$ bash .github/bubbles/scripts/regression-quality-guard.sh \
    tests/e2e/assistant/web_pwa_retry_e2e_test.go \
    tests/e2e/assistant/conversation_isolation_test.go \
    tests/e2e/assistant/transport_hint_parity_test.go \
    tests/e2e/assistant/web_pwa_chat_e2e_test.go
  REGRESSION QUALITY RESULT: 0 violation(s), 0 warning(s)
  Files scanned: 4
RQG_RC=0
```

The regression-quality guard is the check that matters most after adding a readiness wait: it confirms the four required files carry no silent-pass bailout, so the wait did not become an escape hatch.

### Test Phase — Specialist Audit Of The Current-Session Record (2026-08-23)

**Phase:** test · **Agent:** `bubbles.test` · **Claim Source:** executed (audit commands and the unit re-run below); the E2E figures in the two preceding sections are **audited, not re-executed**

The two sections above were produced under `parent-expanded` provenance. This section records the owning specialist's audit of that record. The E2E suite was deliberately not re-run: it serializes on a `flock` suite lock returning exit 73 when held, and the claims that carry risk here are structural — whether the fix is the fail-loud kind and whether a required test hid in the skip list — which the working tree answers directly and more cheaply than a re-run would.

**Claim 1 — the four `waitAssistantFacadeReady` call sites exist. CONFIRMED.**

```text
$ git diff --stat -- tests/e2e/assistant/transport_hint_parity_test.go \
    tests/e2e/assistant/web_pwa_chat_e2e_test.go tests/e2e/assistant/web_pwa_retry_e2e_test.go
 tests/e2e/assistant/transport_hint_parity_test.go | 2 ++
 tests/e2e/assistant/web_pwa_chat_e2e_test.go      | 2 ++
 tests/e2e/assistant/web_pwa_retry_e2e_test.go     | 4 ++++
 3 files changed, 8 insertions(+)
```

Four calls across three files — `web_pwa_retry_e2e_test.go` carries two because it holds two of the four affected tests. Eight insertions is four calls plus four comment lines; the diff adds nothing else. Each call sits immediately after the existing `waitHTTPTurnHealthy` line, so the readiness wait supplements the health wait rather than replacing it.

**Claim 2 — the helper fails loud rather than skipping. CONFIRMED, and it is pre-existing.**

```text
$ git diff --stat -- tests/e2e/assistant/nl_facade_readiness_helper_test.go
(empty)
$ grep -nE 't\.Skip|Skipf|SkipNow' tests/e2e/assistant/nl_facade_readiness_helper_test.go
SKIP_GREP_RC=1        # zero matches
$ grep -nE 't\.Fatalf' tests/e2e/assistant/nl_facade_readiness_helper_test.go
54:     t.Fatalf("e2e: assistant facade did not become ready within %s; last_status=%d body=%s",
```

The helper's only loop exit other than success is line 54, and it is `t.Fatalf` carrying the last status and body. There is no skip path anywhere in the file, and the file itself is unmodified in this tree — it was wired in, not written for this packet. Five pre-existing call sites corroborate that: `nl_facade_routing_e2e_test.go`, `annotation_classifier_e2e_test.go`, `nl_rate_disambig_test.go`, `nl_find_replacement_test.go`, `legacy_retirement_notice_test.go`. This is the distinction that decides whether the fix was legitimate: a `t.Skipf` on 503 would have turned the lane green while erasing the signal, which is the pattern `specs/069-assistant-http-transport/bugs/BUG-069-005-e2e-skip-guard-masks-required-coverage` exists to remove.

**Claim 3 — no required test hid among the 12 skips. CONFIRMED mechanically, not by eye.**

```text
$ grep -oE '^--- SKIP: [A-Za-z0-9_]+' report.md | wc -l
12
$ grep -oE '^--- SKIP: [A-Za-z0-9_]+' report.md | grep -E '<the five required test names>'
INTERSECTION_RC=1     # empty intersection
```

The recorded skip set intersected against this packet's five required tests is empty. All 12 are capture-fallback, legacy-retirement, intent-compiler, or micro-tools tests.

**Unit re-confirmation — executed in this session.**

```text
$ ./smackerel.sh test unit --go --go-run 'TestHTTPTurnDedup' --verbose
--- PASS: TestHTTPTurnDedup_SequentialReplayExecutesFacadeOnce
--- PASS: TestHTTPTurnDedup_ConcurrentReplayExecutesFacadeOnce
--- PASS: TestHTTPTurnDedup_SameIDIsIsolatedAcrossUsers
--- PASS: TestHTTPTurnDedup_ChangedPayloadConflictsWithoutReexecution
--- PASS: TestHTTPTurnDedup_AcceptedFailureIsReplayed
--- PASS: TestHTTPTurnDedup_CaptureRouteRunsCaptureOnce
--- PASS: TestHTTPTurnDedup_CacheExpiresAndEvictsCompletedEntries
--- PASS: TestHTTPTurnDedup_CacheRejectsUniqueWorkWhenAllSlotsAreInFlight
ok      github.com/smackerel/smackerel/internal/assistant/httpadapter   0.111s
RUN_COUNT=8  PASS_COUNT=8  FAIL_COUNT=0  SKIP_COUNT=0
UNIT_RC=0
```

`RUN_COUNT` is reported alongside `PASS_COUNT` because a selector matching nothing also exits 0; eight `=== RUN` lines against eight `--- PASS` lines is what makes this a real pass rather than a vacuous one. The `ok` line confirms the `httpadapter` package itself executed and did not report `[no tests to run]`.

**One qualification the record did not state.** Two of the required test files carry pre-existing environment-gate skips — `transport_hint_parity_test.go:48` on `CORE_EXTERNAL_URL` and `conversation_isolation_test.go:43` on `DATABASE_URL`, both inside stack loaders. Neither is in this session's diff and neither fired in the recorded run, where all five tests show `=== RUN` and `--- PASS`. They are env-availability gates, not the skip-on-the-hunted-status pattern. They are recorded as a Discovered Issues row below rather than left implicit, because they are the same class of masking the sibling packet addresses.

### Regression Phase — Independent Mutation Kill-Test (2026-08-23)

**Phase:** regression · **Executed:** YES (current session) · **Claim Source:** executed · **Agent:** `bubbles.regression`

[Current-Session Mutation Verification](#current-session-mutation-verification--do-the-dedup-tests-bind-their-claim-2026-08-23) above was produced by `bubbles.goal` under `parent-expanded` provenance. This section re-executes the same kill-test as the owning specialist and reports what *this* run observed. The prior section's numbers were treated as a hypothesis to test, not as a result to restate.

**Verdict: CONFIRMED.** The mutation kills 5 of 8, with the claimed signature reproduced independently.

**Step 1 — baseline at the current tree.** All eight cases ran and passed, each paired with a `=== RUN` line, package `ok`, exit 0.

```text
$ ./smackerel.sh test unit --go --go-run 'TestHTTPTurnDedup' --verbose
=== RUN   TestHTTPTurnDedup_SequentialReplayExecutesFacadeOnce
--- PASS: TestHTTPTurnDedup_SequentialReplayExecutesFacadeOnce (0.03s)
=== RUN   TestHTTPTurnDedup_ConcurrentReplayExecutesFacadeOnce
--- PASS: TestHTTPTurnDedup_ConcurrentReplayExecutesFacadeOnce (0.01s)
=== RUN   TestHTTPTurnDedup_SameIDIsIsolatedAcrossUsers
--- PASS: TestHTTPTurnDedup_SameIDIsIsolatedAcrossUsers (0.03s)
=== RUN   TestHTTPTurnDedup_ChangedPayloadConflictsWithoutReexecution
--- PASS: TestHTTPTurnDedup_ChangedPayloadConflictsWithoutReexecution (0.00s)
=== RUN   TestHTTPTurnDedup_AcceptedFailureIsReplayed
--- PASS: TestHTTPTurnDedup_AcceptedFailureIsReplayed (0.00s)
=== RUN   TestHTTPTurnDedup_CaptureRouteRunsCaptureOnce
--- PASS: TestHTTPTurnDedup_CaptureRouteRunsCaptureOnce (0.00s)
=== RUN   TestHTTPTurnDedup_CacheExpiresAndEvictsCompletedEntries
--- PASS: TestHTTPTurnDedup_CacheExpiresAndEvictsCompletedEntries (0.00s)
=== RUN   TestHTTPTurnDedup_CacheRejectsUniqueWorkWhenAllSlotsAreInFlight
--- PASS: TestHTTPTurnDedup_CacheRejectsUniqueWorkWhenAllSlotsAreInFlight (0.00s)
ok      github.com/smackerel/smackerel/internal/assistant/httpadapter   0.110s
BASELINE_EXIT=0
```

**Step 2 — mutate.** `internal/assistant/httpadapter/dedup.go` line 85, inside `begin()`, was changed from `if entry, ok := c.entries[key]; ok {` to `if entry, ok := c.entries[key]; ok && false {`. This disables the cache-HIT replay branch and leaves every other path — key construction, fingerprinting, lease ownership, TTL, and capacity eviction — untouched.

Observed under mutation, assembled from three runs because of the capture defect recorded below:

| # | Test | Verdict | Assertion |
|---|------|---------|-----------|
| 1 | `SequentialReplayExecutesFacadeOnce` | **FAIL** | `dedup_test.go:133: Facade.Handle calls=2, want 1` |
| 2 | `ConcurrentReplayExecutesFacadeOnce` | **FAIL** | `dedup_test.go:156: concurrent Facade.Handle calls=2, want 1` |
| 3 | `SameIDIsIsolatedAcrossUsers` | PASS | — |
| 4 | `ChangedPayloadConflictsWithoutReexecution` | **FAIL** | — |
| 5 | `AcceptedFailureIsReplayed` | **FAIL** | `dedup_test.go:201: Facade.Handle calls=2, want 1 for replayed failure` |
| 6 | `CaptureRouteRunsCaptureOnce` | **FAIL** | `dedup_test.go:221: facade calls=2 capture calls=2, want 1/1` |
| 7 | `CacheExpiresAndEvictsCompletedEntries` | PASS | — |
| 8 | `CacheRejectsUniqueWorkWhenAllSlotsAreInFlight` | PASS | — |

5 FAIL / 3 PASS, package `FAIL`, exit 1. The `dedup_test.go:133: Facade.Handle calls=2, want 1` signature is confirmed against an independent execution. The three survivors are correct survivors: none of them exercises the disabled cache-HIT branch, so a mutation that killed all eight would have been the *weaker* result — it would mean the suite cannot distinguish the replay path from cache bookkeeping.

Row 3's PASS is asserted rather than inferred from the absence of a `FAIL` line. It was run alone under the live mutation and returned exit 0:

```text
$ grep -n 'ok && false' internal/assistant/httpadapter/dedup.go
85:     if entry, ok := c.entries[key]; ok && false {
$ ./smackerel.sh test unit --go --go-run 'TestHTTPTurnDedup_SameIDIsIsolatedAcrossUsers'
[go-unit] applying -run selector: TestHTTPTurnDedup_SameIDIsIsolatedAcrossUsers
[go-unit] go test ./... finished OK
SAMEID_PIPESTATUS=0
```

A whole-run hash of the mutated sweep, re-derivable with `.github/bubbles/scripts/evidence-capture.sh --verify`:

```text
# MUTANT dedup begin() cache-hit short-circuited
$ ./smackerel.sh test unit --go --go-run TestHTTPTurnDedup
exit: 1
lines: 229
sha256: 4b5156a7c6cd97126087cea57bad045964ce20439b1ddf38d90827c8c03fed25
--- failure-shaped lines from the omitted region ---
FAIL
FAIL    github.com/smackerel/smackerel/internal/assistant/httpadapter   0.196s
```

**Step 3 — restore, verified as its own step.** Reverted by an explicit file edit. No shell `trap` was used: a `trap` does not fire reliably in a persistent agent shell and can leave a mutation live in the tree.

```text
$ grep -c 'ok && false' internal/assistant/httpadapter/dedup.go
0
$ sed -n '85p' internal/assistant/httpadapter/dedup.go
        if entry, ok := c.entries[key]; ok {
$ git diff --name-only -- internal/assistant/httpadapter/dedup.go
(empty)
$ git status --porcelain
 M specs/069-assistant-http-transport/bugs/BUG-069-004-http-turn-dedup/report.md
 M specs/069-assistant-http-transport/bugs/BUG-069-004-http-turn-dedup/scopes.md
 M specs/069-assistant-http-transport/bugs/BUG-069-004-http-turn-dedup/state.json
 M tests/e2e/assistant/transport_hint_parity_test.go
 M tests/e2e/assistant/web_pwa_chat_e2e_test.go
 M tests/e2e/assistant/web_pwa_retry_e2e_test.go
```

`dedup.go` does not appear. The six entries that do appear are this packet's artifacts plus the three `waitAssistantFacadeReady` source edits already attributed in [Current-Session E2E](#current-session-e2e--a-cold-start-false-red-found-and-fixed-2026-08-23).

**Step 4 — GREEN restored.** All eight PASS again, each with a `=== RUN` line, `[go-unit] go test ./... finished OK`, exit 0.

The E2E suite was deliberately left unexecuted by this phase. It serializes on a `flock` suite lock and returns exit 73 when the lock is held, and a mutation confined to one unit-level branch cannot be evidenced by it. The required E2E green for this tree is the one recorded in [Current-Session E2E](#current-session-e2e--a-cold-start-false-red-found-and-fixed-2026-08-23), produced from the same working tree.

## Discovered Issues

| Date | Finding | Severity | Concrete disposition |
|------|---------|----------|----------------------|
| 2026-08-23 | Restore verification in [Current-Session Mutation Verification](#current-session-mutation-verification--do-the-dedup-tests-bind-their-claim-2026-08-23) Step 3 asserts `porcelain_lines=0`. A whole-tree porcelain check is not a sound restore assertion for this packet, because the tree legitimately carries modified packet artifacts and three `waitAssistantFacadeReady` source edits; the assertion does not hold against the tree as it now stands, and when it does fail it cannot separate an un-restored mutation from unrelated dirt. | Medium — evidence precision, not a code defect | This section's Step 3 supersedes that method with the path-scoped form `git diff --name-only -- internal/assistant/httpadapter/dedup.go`, which isolates the mutated file. No code change is implied; the dedup fix itself is unaffected. |
| 2026-08-23 | A full-repository verbose `go test ./...` sweep produces output large enough that the captured log arrives already elided. The elided artifact was internally inconsistent — `grep -c '^=== RUN.*TestHTTPTurnDedup'` returned 4 while the same file carried 5 `--- PASS/FAIL` verdicts — so three of the eight per-test verdicts were absent from evidence that otherwise looked complete. | Medium — evidence integrity | Recovered in this section by re-running the missing cases under narrowed `--go-run` selectors, and by hashing the full sweep with `.github/bubbles/scripts/evidence-capture.sh` so the omitted region stays verifiable. `scripts/runtime/go-unit.sh` accepts only `--run` and `--verbose`, with the package set fixed at `./...`, so a per-package selector is the change that would remove the hazard at source; that belongs to `scripts/runtime/go-unit.sh` and is out of this packet's fix boundary. |
| 2026-08-23 | Two of this packet's five required E2E test files carry environment-gate skips inside their stack loaders: `tests/e2e/assistant/transport_hint_parity_test.go:48` skips when `CORE_EXTERNAL_URL` is unset, and `tests/e2e/assistant/conversation_isolation_test.go:43` skips when `DATABASE_URL` is unset. Neither fired in the recorded run — all five required tests show `=== RUN` and `--- PASS` — and neither appears in this session's diff, so they are pre-existing. They are nonetheless a masking surface: a required test that silently skips on a misconfigured stack reports the same green as one that ran. | Low for this packet — inert under the harness, which supplies both variables; the class matters repo-wide | Recorded, not changed here. Editing a required test's skip posture is the fix boundary of `specs/069-assistant-http-transport/bugs/BUG-069-005-e2e-skip-guard-masks-required-coverage`, which owns skip-guard removal across the assistant E2E surface; changing it from this packet would move that packet's evidence out from under it. This packet's own exposure is bounded by [Test Phase — Specialist Audit](#test-phase--specialist-audit-of-the-current-session-record-2026-08-23), which shows both guards inert in the run of record. |

## Implementation Delta

### Code Diff Evidence

**Phase:** implement · **Recorded:** 2026-08-23 by `bubbles.goal` · **Claim Source:** executed — every sha, path, and count below was read from `git` in this session. This section records WHAT CHANGED, not what passed.

The commit of record is `60dfcfe7af65f758f3abddcb1dc78f982010d19a` (`2026-07-19T17:41:57+00:00`, *fix(assistant): harden broad e2e retries and source scans*). Two commits carry that identical subject and both introduce `dedup.go`; `git merge-base --is-ancestor` settles which is the delivery — `60dfcfe7` is reachable from `HEAD` and `56dea60c` is NOT, being a pre-rebase copy that must never be cited as the delivery.

Delta split, because the raw headline overstates the code delivery: packet-artifact files=24 +1423 -0, source files=22 +976 -34. The 22 non-spec paths are:

docs/Development.md
internal/assistant/httpadapter/adapter.go
internal/assistant/httpadapter/adapter_test.go
internal/assistant/httpadapter/dedup.go
internal/assistant/httpadapter/dedup_test.go
internal/assistant/httpadapter/transport_hint_test.go
internal/testsupport/jssource/comments.go
internal/testsupport/jssource/comments_test.go
tests/e2e/assistant/conversation_isolation_test.go
tests/e2e/assistant/transport_hint_parity_test.go
tests/e2e/assistant/web_pwa_chat_e2e_test.go
tests/e2e/assistant/web_pwa_retry_e2e_test.go
tests/integration/api/assistant_http_auth_test.go
tests/integration/api/assistant_http_turn_test.go
tests/integration/assistant/http_adapter_canary_test.go
tests/integration/assistant/http_pending_state_test.go
tests/integration/assistant/legacy_retirement_mobile_renderer_test.go
tests/integration/assistant/transport_parity_test.go
tests/stress/assistant/http_turn_stress_test.go
web/pwa/tests/assistant_robustness_guard_test.go
web/pwa/tests/assistant_storage_guard_test.go
web/pwa/tests/model_connections_pwa_guard_test.go

The load-bearing file is `internal/assistant/httpadapter/dedup.go` (+161), which introduces the process-local turn-response cache: `turnDedupKey{userDigest, transport, messageID}`, a lease whose owner executes while matching waiters block on `wait`, and TTL plus capacity eviction. `adapter.go` (+52 -4) wires the lease around facade invocation.

### Independent Implementation Verification (2026-08-23)

**Phase:** implement · **Recorded:** 2026-08-23 by `bubbles.implement` · **Claim Source:** executed — every source assertion below was read from the working tree in this session, and the result was produced by the command shown.

The section above records WHAT CHANGED. This section records that a specialist re-derived the claim instead of inheriting it.

**Source read — `dedup.go`.** The cache key is `turnDedupKey{userDigest [sha256.Size]byte, transport string, messageID string}`. `begin()` hashes the caller's user ID via `sha256.Sum256` and pins `transport` to `TransportName`, so the cache is auth-scoped *by construction* — a same-ID turn from a different identity cannot collide, which is the property the isolation scenario asserts. The returned `turnDedupLease` carries `owner bool`: the first caller for a key owns execution; every later caller for that key receives a non-owner lease whose `wait()` selects on `entry.ready` against `ctx.Done()`, so a duplicate blocks on the owner rather than re-executing, and still unblocks if its own client disconnects. `complete()` is owner-guarded and re-checks identity under the lock before stamping `expiresAt = now + ttl` and closing `ready`, so a stale lease cannot publish over a newer entry. Retention is bounded on both axes: `removeExpiredLocked` drops completed entries past TTL, and `evictOldestCompletedLocked` walks the LRU list from the back and evicts only entries already marked `completed` — an in-flight turn is therefore never evicted out from under its waiters, and a cache full of in-flight work refuses new unique work rather than corrupting existing leases. `HTTPTurnDedupCapacity = 16384`.

**Source read — `adapter.go` wiring.** Line 75 constructs the cache with that capacity, `opts.Config.ConversationTTL`, and the injected clock. Line 383 calls `begin()` with `sha256.Sum256(fingerprintBody)` over the marshalled request — that fingerprint is what turns a *changed payload under a reused ID* into `409 transport_message_id_conflict` (line 385) rather than a silent second execution, and capacity exhaustion into `503 assistant_turn_capacity_exceeded` (line 388). The non-owner branch (line 396) replays the cached status and body with a freshly stamped `Trace.RequestID` and returns *without reaching* `a.facade.Handle`. The owner branch calls `a.facade.Handle` exactly once and completes the lease on both outcomes — `500` at line 409, `200` at line 428 — so an accepted failure is replayed to duplicates instead of being silently retried.

**Confirmation run.**

```text
$ ./smackerel.sh test unit --go --go-run 'TestHTTPTurnDedup' --verbose
=== RUN   TestHTTPTurnDedup_SequentialReplayExecutesFacadeOnce
--- PASS: TestHTTPTurnDedup_SequentialReplayExecutesFacadeOnce (0.01s)
=== RUN   TestHTTPTurnDedup_ConcurrentReplayExecutesFacadeOnce
--- PASS: TestHTTPTurnDedup_ConcurrentReplayExecutesFacadeOnce (0.01s)
=== RUN   TestHTTPTurnDedup_SameIDIsIsolatedAcrossUsers
--- PASS: TestHTTPTurnDedup_SameIDIsIsolatedAcrossUsers (0.01s)
=== RUN   TestHTTPTurnDedup_ChangedPayloadConflictsWithoutReexecution
--- PASS: TestHTTPTurnDedup_ChangedPayloadConflictsWithoutReexecution (0.01s)
=== RUN   TestHTTPTurnDedup_AcceptedFailureIsReplayed
--- PASS: TestHTTPTurnDedup_AcceptedFailureIsReplayed (0.00s)
=== RUN   TestHTTPTurnDedup_CaptureRouteRunsCaptureOnce
--- PASS: TestHTTPTurnDedup_CaptureRouteRunsCaptureOnce (0.01s)
=== RUN   TestHTTPTurnDedup_CacheExpiresAndEvictsCompletedEntries
--- PASS: TestHTTPTurnDedup_CacheExpiresAndEvictsCompletedEntries (0.00s)
=== RUN   TestHTTPTurnDedup_CacheRejectsUniqueWorkWhenAllSlotsAreInFlight
--- PASS: TestHTTPTurnDedup_CacheRejectsUniqueWorkWhenAllSlotsAreInFlight (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/assistant/httpadapter   0.083s
[go-unit] go test ./... finished OK
UNIT_EXIT=0
```

8 RUN / 8 PASS / 0 FAIL / 0 SKIP, exit 0, and each PASS is paired with a `=== RUN` line. The `ok` line for the httpadapter package carries a real duration and does **not** carry the `[no tests to run]` suffix — that absence is the check separating this from a vacuous pass, and it is meaningful rather than incidental because the same run emits that suffix 203 times for the other packages this selector correctly matches nothing in.

The E2E suite was intentionally not re-executed here: it takes a `flock` suite lock, and the required set already has a recorded green from this same working tree in [Current-Session E2E — A Cold-Start False RED Found And Fixed](#current-session-e2e--a-cold-start-false-red-found-and-fixed-2026-08-23). That green was produced against exactly this tree, which is the claim that matters and which was checked rather than assumed: `git status --porcelain` carries three modified source paths, `tests/e2e/assistant/transport_hint_parity_test.go`, `web_pwa_chat_e2e_test.go`, and `web_pwa_retry_e2e_test.go`, and their diff is precisely the four added `waitAssistantFacadeReady` calls of that same fix. Their mtimes are 19:24Z, before this phase began at 19:44Z. Everything this phase wrote is packet artifacts.

## Open Findings

- Process-local response replay is appropriate for the current single-ingress
  deployment; any future multi-replica topology requires a separate durable
  dedup design before scale-out.
- The assistant package run executed all 60 tests. The fixed retry tests passed
    in package order; six unrelated environment/policy tests remain red (missing
    metrics URL, disabled replay, missing legacy metric, and missing node for two
    legacy renderer tests).
