# Report: BUG-020-001 — DoD Scenario Fidelity Gap

Links: [spec.md](spec.md) | [design.md](design.md) | [scopes.md](scopes.md) | [uservalidation.md](uservalidation.md)

---

## Summary

Traceability-guard Gate G068 (Gherkin → DoD Content Fidelity) reported that 5 of 18 Gherkin scenarios in `specs/020-security-hardening` had no faithful matching DoD item: `SCN-020-001`, `SCN-020-002`, `SCN-020-006`, `SCN-020-013`, `SCN-020-014`. Investigation confirmed the gap is artifact-only — every scenario is fully delivered in production code (`docker-compose.yml`, `scripts/commands/config.sh`, `ml/app/auth.py`, `ml/app/main.py`, `internal/auth/store.go`) and exercised by passing unit tests. The DoD bullets simply did not embed the `SCN-020-NNN` trace IDs that the guard's content-fidelity matcher requires. Three ancillary failures were resolved at the same time: a missing `scenario-manifest.json` for spec 020 (Gates G057/G059); Scope 1 Test Plan rows for SCN-020-001..004 that pointed only at planned-but-not-yet-existing live-stack files (`tests/integration/docker_ports_test.go`, `tests/e2e/port_binding_test.go`, `tests/e2e/nats_token_hidden_test.go`, `scripts/commands/config_test.sh`); and a missing `cmd/core/main_test.go` evidence reference in the parent `report.md` for SCN-020-016/017/018.

The fix added 5 trace-ID-bearing DoD bullets to `specs/020-security-hardening/scopes.md`, generated `specs/020-security-hardening/scenario-manifest.json` covering all 18 `SCN-020-*` scenarios, inserted two in-process proxy Test Plan rows at the top of Scope 1 mapping SCN-020-001..004 to existing tests in `internal/config/docker_security_test.go`, and appended a cross-reference section to `specs/020-security-hardening/report.md` containing the `cmd/core/main_test.go` reference. No production code was modified; the boundary clause in the user prompt was honored.

## Completion Statement

All 11 DoD items in `scopes.md` Scope 1 are checked `[x]` with inline raw evidence. The traceability-guard's pre-fix state (5 unmapped scenarios, 14 failures) has been replaced with a clean `RESULT: PASSED (0 warnings)` post-fix. Both `artifact-lint.sh` invocations (parent and bug folder) succeed. The 9 underlying behavior tests for the previously-flagged scenarios still pass with no regressions.

## Test Evidence

### Underlying Go behavior tests (regression-protection for the artifact fix)

```
$ go test -count=1 -v -run 'TestDockerCompose_AllPortsBindLocalhost$|TestDockerCompose_NATSUsesConfigFile$' ./internal/config/
=== RUN   TestDockerCompose_AllPortsBindLocalhost
--- PASS: TestDockerCompose_AllPortsBindLocalhost (0.00s)
=== RUN   TestDockerCompose_NATSUsesConfigFile
--- PASS: TestDockerCompose_NATSUsesConfigFile (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/config  0.012s

$ go test -count=1 -v -run 'TestTokenStore_Decrypt_FailClosed_NotBase64$|TestTokenStore_Decrypt_FailClosed_TooShort$|TestTokenStore_Decrypt_FailClosed_GCMFailure$|TestTokenStore_Decrypt_NoKey_PlaintextPassthrough$|TestTokenStore_Decrypt_WrongKey_FailClosed$' ./internal/auth/
=== RUN   TestTokenStore_Decrypt_FailClosed_NotBase64
--- PASS: TestTokenStore_Decrypt_FailClosed_NotBase64 (0.00s)
=== RUN   TestTokenStore_Decrypt_FailClosed_TooShort
--- PASS: TestTokenStore_Decrypt_FailClosed_TooShort (0.00s)
=== RUN   TestTokenStore_Decrypt_FailClosed_GCMFailure
--- PASS: TestTokenStore_Decrypt_FailClosed_GCMFailure (0.00s)
=== RUN   TestTokenStore_Decrypt_NoKey_PlaintextPassthrough
2026/04/27 02:30:00 WARN TokenStore: encryption key is empty — OAuth tokens will be stored in PLAINTEXT. Set SMACKEREL_AUTH_TOKEN for encrypted storage.
--- PASS: TestTokenStore_Decrypt_NoKey_PlaintextPassthrough (0.00s)
=== RUN   TestTokenStore_Decrypt_WrongKey_FailClosed
--- PASS: TestTokenStore_Decrypt_WrongKey_FailClosed (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/auth    0.012s
```

**Claim Source:** executed.

### Underlying Python ML auth tests (regression-protection for the artifact fix)

```
$ docker run --rm -v "$PWD:/workspace" -v smackerel-pip-cache:/root/.cache/pip -w /workspace python:3.12-slim bash -c "pip install --no-cache-dir -e ./ml[dev] -q && pytest ml/tests/test_auth.py::TestMLSidecarAuthWithToken::test_accept_bearer_token ml/tests/test_auth.py::TestMLSidecarAuthWithToken::test_accept_x_auth_token_header -v"
============================= test session starts ==============================
platform linux -- Python 3.12.13, pytest-9.0.3, pluggy-1.6.0 -- /usr/local/bin/python3.12
cachedir: .pytest_cache
rootdir: /workspace/ml
configfile: pyproject.toml
plugins: anyio-4.13.0
collecting ... collected 2 items

ml/tests/test_auth.py::TestMLSidecarAuthWithToken::test_accept_bearer_token PASSED [ 50%]
ml/tests/test_auth.py::TestMLSidecarAuthWithToken::test_accept_x_auth_token_header PASSED [100%]

============================== 2 passed in 0.54s ===============================
```

**Claim Source:** executed.

### Validation Evidence

> Phase agent: bubbles.validate
> Executed: YES

```
$ bash .github/bubbles/scripts/traceability-guard.sh specs/020-security-hardening 2>&1 | tail -15
ℹ️  DoD fidelity: 18 scenarios checked, 18 mapped to DoD, 0 unmapped

--- Traceability Summary ---
ℹ️  Scenarios checked: 18
ℹ️  Test rows checked: 27
ℹ️  Scenario-to-row mappings: 18
ℹ️  Concrete test file references: 18
ℹ️  Report evidence references: 18
ℹ️  DoD fidelity scenarios: 18 (mapped: 18, unmapped: 0)

RESULT: PASSED (0 warnings)
```

**Claim Source:** executed. Pre-fix run on the same revision (with the unfixed artifacts) reported `RESULT: FAILED (14 failures)` including `DoD fidelity: 18 scenarios checked, 13 mapped to DoD, 5 unmapped` — see Section "Pre-fix Reproduction" below.

### Audit Evidence

> Phase agent: bubbles.audit
> Executed: YES

```
$ bash .github/bubbles/scripts/artifact-lint.sh specs/020-security-hardening 2>&1 | tail -5
=== End Anti-Fabrication Checks ===

Artifact lint PASSED.

$ bash .github/bubbles/scripts/artifact-lint.sh specs/020-security-hardening/bugs/BUG-020-001-dod-scenario-fidelity-gap 2>&1 | tail -5
=== End Anti-Fabrication Checks ===

Artifact lint PASSED.
```

```
$ git diff --name-only
specs/020-security-hardening/bugs/BUG-020-001-dod-scenario-fidelity-gap/bug.md
specs/020-security-hardening/bugs/BUG-020-001-dod-scenario-fidelity-gap/design.md
specs/020-security-hardening/bugs/BUG-020-001-dod-scenario-fidelity-gap/report.md
specs/020-security-hardening/bugs/BUG-020-001-dod-scenario-fidelity-gap/scopes.md
specs/020-security-hardening/bugs/BUG-020-001-dod-scenario-fidelity-gap/spec.md
specs/020-security-hardening/bugs/BUG-020-001-dod-scenario-fidelity-gap/state.json
specs/020-security-hardening/bugs/BUG-020-001-dod-scenario-fidelity-gap/uservalidation.md
specs/020-security-hardening/report.md
specs/020-security-hardening/scenario-manifest.json
specs/020-security-hardening/scopes.md
```

**Claim Source:** executed. Boundary preserved: zero changes under `internal/`, `cmd/`, `ml/`, `config/`, `tests/`, or any other production-code path.

## Pre-fix Reproduction

```
$ bash .github/bubbles/scripts/traceability-guard.sh specs/020-security-hardening 2>&1 | tail -15
ℹ️  DoD fidelity: 18 scenarios checked, 13 mapped to DoD, 5 unmapped
❌ DoD content fidelity gap: 5 Gherkin scenario(s) have no matching DoD item — DoD may have been rewritten to match delivery instead of the spec (Gate G068)

--- Traceability Summary ---
ℹ️  Scenarios checked: 18
ℹ️  Test rows checked: 23
ℹ️  Scenario-to-row mappings: 18
ℹ️  Concrete test file references: 14
ℹ️  Report evidence references: 11
ℹ️  DoD fidelity scenarios: 18 (mapped: 13, unmapped: 5)

RESULT: FAILED (14 failures, 0 warnings)
```

**Claim Source:** executed (initial guard invocation captured at the start of this bug investigation, before any artifact edits — see `/tmp/g020-before.log` and the agent transcript).
