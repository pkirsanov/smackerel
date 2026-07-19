# Report: BUG-073-005 - Comment-aware PWA storage scan

## Summary

The packet is active. Code inspection shows the served-route E2E scans raw
JavaScript with `strings.Contains`, while the dedicated storage guard strips
comments first. `assistant.js` names forbidden storage APIs only in policy
comments at the current base commit.

**Claim Source:** interpreted

## Completion Statement

The shared lexical scanner and served-source migration are delivered and tested
on the isolated branch. Focused scanner/live E2E, full Go/Python units,
format/check/lint, regression guard, traceability, and artifact lint pass.
Packet status and validate-owned certification intentionally remain
`in_progress` for parent synthesis consolidation.

**Claim Source:** executed

## Test Evidence

### Before Fix - Served Route E2E

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

The failure names only the raw token `localStorage`; the committed executable
source contains no corresponding access. The token occurs in the leading
security-policy comment, while the dedicated unit guard already ignores that
comment and remains green.

**Claim Source:** interpreted

### After Fix - Scanner Unit And Live E2E

Concrete tests:

- `tests/e2e/assistant/web_pwa_chat_e2e_test.go`
- `internal/testsupport/jssource/comments_test.go`
- `web/pwa/tests/assistant_storage_guard_test.go`

**Executed:** YES (current session)
**Command:** `./smackerel.sh test unit --go --go-run 'TestWithoutComments|TestWebAssistantStorageGuard|TestWebAssistantRobustnessGuard' --verbose`
**Exit Code:** 0
**Output:**

```text
=== RUN   TestWithoutComments_IgnoresPolicyCommentsAndRetainsExecutableAccess
--- PASS: TestWithoutComments_IgnoresPolicyCommentsAndRetainsExecutableAccess (0.00s)
=== RUN   TestWithoutComments_PreservesStringsTemplatesAndFollowingCode
--- PASS: TestWithoutComments_PreservesStringsTemplatesAndFollowingCode (0.00s)
=== RUN   TestWebAssistantRobustnessGuard_BUG_073_002
--- PASS: TestWebAssistantRobustnessGuard_BUG_073_002 (0.00s)
=== RUN   TestWebAssistantRobustnessGuard_Adversarial_BUG_073_002
--- PASS: TestWebAssistantRobustnessGuard_Adversarial_BUG_073_002 (0.00s)
=== RUN   TestWebAssistantStorageGuard_TP_073_06
--- PASS: TestWebAssistantStorageGuard_TP_073_06 (0.01s)
=== RUN   TestWebAssistantStorageGuard_Adversarial_TP_073_06
--- PASS: TestWebAssistantStorageGuard_Adversarial_TP_073_06 (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/testsupport/jssource
ok      github.com/smackerel/smackerel/web/pwa/tests
[go-unit] go test ./... finished OK
```

The focused live E2E command recorded in BUG-073-004's report included
`TestAssistantWebPWAChatE2E_ServedRouteHasComposerTranscriptAndResponseMarkup_TP_073_09`,
which passed against the real served `/pwa/assistant.js` asset.

**Claim Source:** executed

### Quality And Packet Gates

Full Go units passed; Python units reported `708 passed, 2 deselected`; format,
check, and lint passed; regression-quality guard reported 0 violations and 0
warnings. Packet artifact lint passed. Traceability is rerun after this evidence
update.

**Claim Source:** executed

## Root Cause Evidence

- `web_pwa_chat_e2e_test.go` searches raw served JS for forbidden substrings.
- `assistant.js` documents those names in leading line comments.
- `assistant_storage_guard_test.go` already strips comments before applying the
  forbidden regex patterns and includes an executable-access adversary.

**Claim Source:** interpreted

## Open Findings

- The assistant package still has six unrelated environment/policy failures;
  this packet's scanner unit tests and served-route E2E pass in package order.
