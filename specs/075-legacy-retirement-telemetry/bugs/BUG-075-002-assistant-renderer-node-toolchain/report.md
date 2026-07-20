# Report: BUG-075-002 Assistant renderer Node toolchain

## Summary

Both live notice scenarios reached the renderer step and failed because the repository's Go E2E container lacks Node. No host tool was used.

## Completion Statement

The packet is active. RED evidence is captured; implementation, specialist verification, and validate-owned certification remain unset.

## Test Evidence

### RED: Node absent in sanctioned E2E container

**Executed:** YES (current session)
**Command:** `cd ~/smackerel-assistant-environment-residuals-20260719 && SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test e2e --go-run '<seven-test residual selector>'`
**Exit Code:** 1
**Claim Source:** executed

```text
=== RUN   TestLegacyRetirementNoticeE2E_OpenWindowRendersAddendumWithoutBlockingBody
    legacy_retirement_notice_test.go:248: node not on PATH;
    spec 075 SCOPE-075-06.3 e2e requires node to run the PWA renderer:
    exec: "node": executable file not found in $PATH
--- FAIL: TestLegacyRetirementNoticeE2E_OpenWindowRendersAddendumWithoutBlockingBody (0.02s)
=== RUN   TestLegacyRetirementNoticeE2E_NonRetiredTurnOmitsNotice
    legacy_retirement_notice_test.go:319: node not on PATH;
    spec 075 SCOPE-075-06.3 e2e requires node to run the PWA renderer:
    exec: "node": executable file not found in $PATH
--- FAIL: TestLegacyRetirementNoticeE2E_NonRetiredTurnOmitsNotice (0.07s)
=== RUN   TestLegacyRetirementReport_E2E_RollingSevenDay
--- PASS: TestLegacyRetirementReport_E2E_RollingSevenDay (0.02s)
FAIL
FAIL    github.com/smackerel/smackerel/tests/e2e/assistant      1.282s
FAIL: go-e2e (exit=1)
```

## Invocation Audit

No subagent invocation tool is available in this runtime. No specialist phase or certification is claimed.

### GREEN: Containerized renderer execution

Concrete test files: `internal/deploy/assistant_e2e_package_contract_test.go` and `tests/e2e/assistant/legacy_retirement_notice_test.go`.

**Executed:** YES (current session)
**Command:** `./smackerel.sh test e2e --go-package assistant --go-run '<exact six-test selector>'`
**Exit Code:** 0
**Claim Source:** executed

```text
[go-e2e] node missing - installing nodejs inside the tooling container
[go-e2e] nodejs install OK
=== RUN   TestLegacyRetirementNoticeE2E_OpenWindowRendersAddendumWithoutBlockingBody
--- PASS: TestLegacyRetirementNoticeE2E_OpenWindowRendersAddendumWithoutBlockingBody (1.10s)
=== RUN   TestLegacyRetirementNoticeE2E_NonRetiredTurnOmitsNotice
--- PASS: TestLegacyRetirementNoticeE2E_NonRetiredTurnOmitsNotice (2.40s)
PASS
ok      github.com/smackerel/smackerel/tests/e2e/assistant      43.319s
PASS: go-e2e
Volume smackerel-test-postgres-data Removed
Volume smackerel-test-nats-data Removed
Network smackerel-test_default Removed
```
