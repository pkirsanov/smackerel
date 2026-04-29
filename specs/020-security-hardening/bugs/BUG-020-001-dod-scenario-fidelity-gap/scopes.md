# Scopes: BUG-020-001 — DoD scenario fidelity gap

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

---

## Scope 1: Restore Gherkin → DoD trace-ID fidelity for spec 020

**Status:** Done
**Priority:** P0
**Depends On:** None

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-020-FIX-001 Trace guard accepts SCN-020-001/002/006/013/014 as faithfully covered
  Given specs/020-security-hardening/scopes.md DoD entries that name each Gherkin scenario by ID
  And specs/020-security-hardening/scenario-manifest.json mapping all 18 SCN-020-* scenarios
  And specs/020-security-hardening/report.md referencing internal/config/docker_security_test.go, internal/auth/oauth_test.go, ml/tests/test_auth.py, and cmd/core/main_test.go
  When the workflow runs `bash .github/bubbles/scripts/traceability-guard.sh specs/020-security-hardening`
  Then Gate G068 reports "18 scenarios checked, 18 mapped to DoD, 0 unmapped"
  And the overall result is PASSED
```

### Implementation Plan

1. Append SCN-020-001 DoD bullet (with raw `go test` output for `TestDockerCompose_AllPortsBindLocalhost` and source pointer to `docker-compose.yml` literal `127.0.0.1:` prefixes) to Scope 1 DoD in `specs/020-security-hardening/scopes.md`
2. Append SCN-020-002 DoD bullet (raw `go test` output for the same proxy plus source pointer to `scripts/commands/config.sh::HOST_BIND_ADDRESS`) to Scope 1 DoD
3. Append SCN-020-006 DoD bullet (raw `pytest` output for `TestMLSidecarAuthWithToken::test_accept_bearer_token` and `test_accept_x_auth_token_header` + source pointer to `ml/app/auth.py::verify_auth`) to Scope 2 DoD
4. Append SCN-020-013 DoD bullet (raw `go test` output for `TestTokenStore_Decrypt_FailClosed_*` + source pointer to `internal/auth/store.go::decrypt`) to Scope 3 DoD
5. Append SCN-020-014 DoD bullet (raw `go test` output for `TestTokenStore_Decrypt_NoKey_PlaintextPassthrough` + source pointer to `internal/auth/store.go::decrypt` no-key branch) to Scope 3 DoD
6. Generate `specs/020-security-hardening/scenario-manifest.json` covering all 18 `SCN-020-*` scenarios with `linkedTests`, `evidenceRefs`, and `linkedDoD`
7. Insert in-process proxy Test Plan rows at the top of Scope 1's Test Plan table mapping SCN-020-001/002 → `internal/config/docker_security_test.go::TestDockerCompose_AllPortsBindLocalhost` and SCN-020-003/004 → `internal/config/docker_security_test.go::TestDockerCompose_NATSUsesConfigFile` so the trace guard finds an existing concrete test file before evaluating the legacy planned-file rows
8. Append a "BUG-020-001 — DoD Scenario Fidelity Gap" section to `specs/020-security-hardening/report.md` with per-scenario classification, raw test evidence, and full-path references to `internal/config/docker_security_test.go`, `internal/auth/oauth_test.go`, `ml/tests/test_auth.py`, and `cmd/core/main_test.go`
9. Run `bash .github/bubbles/scripts/artifact-lint.sh` against both the parent and bug folder; run `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/020-security-hardening` and confirm PASS

### Test Plan

| ID | Test Name | Type | Location | Assertion | Mapped Scenario |
|---|---|---|---|---|---|
| T-FIX-1-01 | traceability-guard.sh PASS | artifact | `.github/bubbles/scripts/traceability-guard.sh` | `RESULT: PASSED (0 warnings)` and `DoD fidelity: 18 mapped, 0 unmapped` | SCN-020-FIX-001 |
| T-FIX-1-02 | artifact-lint.sh PASS (parent) | artifact | `.github/bubbles/scripts/artifact-lint.sh` | exit 0 against `specs/020-security-hardening` | SCN-020-FIX-001 |
| T-FIX-1-03 | artifact-lint.sh PASS (bug) | artifact | `.github/bubbles/scripts/artifact-lint.sh` | exit 0 against `specs/020-security-hardening/bugs/BUG-020-001-dod-scenario-fidelity-gap` | SCN-020-FIX-001 |
| T-FIX-1-04 | Underlying Go behavior tests still pass | unit | `internal/auth/oauth_test.go`, `internal/config/docker_security_test.go` | `go test -count=1 -v -run '...' ./internal/auth/ ./internal/config/` exit 0; the named tests for SCN-020-001/002/013/014 all PASS | SCN-020-FIX-001 |
| T-FIX-1-05 | Underlying Python ML auth tests still pass | unit | `ml/tests/test_auth.py` | `pytest ml/tests/test_auth.py::TestMLSidecarAuthWithToken::test_accept_bearer_token ml/tests/test_auth.py::TestMLSidecarAuthWithToken::test_accept_x_auth_token_header -v` exit 0 | SCN-020-FIX-001 |

### Definition of Done

- [x] Scope 1 DoD in parent `scopes.md` contains a bullet citing `Scenario SCN-020-001` with inline raw `go test` evidence — **Phase:** implement
  > Evidence: `grep -n "SCN-020-001" specs/020-security-hardening/scopes.md` shows the new DoD bullet at the bottom of Scope 1 DoD (post-edit) plus the existing Gherkin/Test Plan references; full raw test output recorded inline.
- [x] Scope 1 DoD in parent `scopes.md` contains a bullet citing `Scenario SCN-020-002` with inline raw `go test` evidence — **Phase:** implement
  > Evidence: `grep -n "SCN-020-002" specs/020-security-hardening/scopes.md` shows the new DoD bullet at the bottom of Scope 1 DoD (post-edit); full raw test output recorded inline.
- [x] Scope 2 DoD in parent `scopes.md` contains a bullet citing `Scenario SCN-020-006` with inline raw `pytest` evidence — **Phase:** implement
  > Evidence: `grep -n "Scenario SCN-020-006" specs/020-security-hardening/scopes.md` returns one match in the Scope 2 DoD section; full raw pytest output recorded inline.
- [x] Scope 3 DoD in parent `scopes.md` contains bullets citing `Scenario SCN-020-013` and `SCN-020-014` with inline raw `go test` evidence — **Phase:** implement
  > Evidence: `grep -n "Scenario SCN-020-013\|Scenario SCN-020-014" specs/020-security-hardening/scopes.md` returns two matches in the Scope 3 DoD section; full raw test output recorded inline.
- [x] `specs/020-security-hardening/scenario-manifest.json` exists and lists all 18 `SCN-020-*` scenarios — **Phase:** implement
  > Evidence: `grep -c '"scenarioId"' specs/020-security-hardening/scenario-manifest.json` returns `18`.
- [x] `specs/020-security-hardening/report.md` references `internal/config/docker_security_test.go`, `internal/auth/oauth_test.go`, `ml/tests/test_auth.py`, and `cmd/core/main_test.go` by full relative path — **Phase:** implement
  > Evidence: `grep -n "internal/config/docker_security_test.go\|internal/auth/oauth_test.go\|ml/tests/test_auth.py\|cmd/core/main_test.go" specs/020-security-hardening/report.md` returns matches in the new BUG-020-001 section (the existing references for the first three are also preserved).
- [x] Proxy Test Plan rows precede the legacy planned-file rows in Scope 1 and point at the existing `internal/config/docker_security_test.go` — **Phase:** implement
  > Evidence: `awk` line-number check shows the new proxy rows above the legacy rows referencing `tests/integration/docker_ports_test.go`, `tests/e2e/port_binding_test.go`, `tests/e2e/nats_token_hidden_test.go`, `scripts/commands/config_test.sh`.
- [x] Underlying behavior tests still pass — **Phase:** test
  > Evidence:
  > ```
  > $ go test -count=1 -v -run 'TestDockerCompose_AllPortsBindLocalhost$|TestDockerCompose_NATSUsesConfigFile$' ./internal/config/
  > === RUN   TestDockerCompose_AllPortsBindLocalhost
  > --- PASS: TestDockerCompose_AllPortsBindLocalhost (0.00s)
  > === RUN   TestDockerCompose_NATSUsesConfigFile
  > --- PASS: TestDockerCompose_NATSUsesConfigFile (0.00s)
  > PASS
  > ok      github.com/smackerel/smackerel/internal/config  0.012s
  >
  > $ go test -count=1 -v -run 'TestTokenStore_Decrypt_FailClosed_NotBase64$|TestTokenStore_Decrypt_FailClosed_TooShort$|TestTokenStore_Decrypt_FailClosed_GCMFailure$|TestTokenStore_Decrypt_NoKey_PlaintextPassthrough$|TestTokenStore_Decrypt_WrongKey_FailClosed$' ./internal/auth/
  > === RUN   TestTokenStore_Decrypt_FailClosed_NotBase64
  > --- PASS: TestTokenStore_Decrypt_FailClosed_NotBase64 (0.00s)
  > === RUN   TestTokenStore_Decrypt_FailClosed_TooShort
  > --- PASS: TestTokenStore_Decrypt_FailClosed_TooShort (0.00s)
  > === RUN   TestTokenStore_Decrypt_FailClosed_GCMFailure
  > --- PASS: TestTokenStore_Decrypt_FailClosed_GCMFailure (0.00s)
  > === RUN   TestTokenStore_Decrypt_NoKey_PlaintextPassthrough
  > 2026/04/27 02:30:00 WARN TokenStore: encryption key is empty — OAuth tokens will be stored in PLAINTEXT. Set SMACKEREL_AUTH_TOKEN for encrypted storage.
  > --- PASS: TestTokenStore_Decrypt_NoKey_PlaintextPassthrough (0.00s)
  > === RUN   TestTokenStore_Decrypt_WrongKey_FailClosed
  > --- PASS: TestTokenStore_Decrypt_WrongKey_FailClosed (0.00s)
  > PASS
  > ok      github.com/smackerel/smackerel/internal/auth    0.012s
  >
  > $ docker run --rm -v "$PWD:/workspace" -v smackerel-pip-cache:/root/.cache/pip -w /workspace python:3.12-slim bash -c "pip install --no-cache-dir -e ./ml[dev] -q && pytest ml/tests/test_auth.py::TestMLSidecarAuthWithToken::test_accept_bearer_token ml/tests/test_auth.py::TestMLSidecarAuthWithToken::test_accept_x_auth_token_header -v"
  > ============================= test session starts ==============================
  > platform linux -- Python 3.12.13, pytest-9.0.3, pluggy-1.6.0 -- /usr/local/bin/python3.12
  > cachedir: .pytest_cache
  > rootdir: /workspace/ml
  > configfile: pyproject.toml
  > plugins: anyio-4.13.0
  > collecting ... collected 2 items
  >
  > ml/tests/test_auth.py::TestMLSidecarAuthWithToken::test_accept_bearer_token PASSED [ 50%]
  > ml/tests/test_auth.py::TestMLSidecarAuthWithToken::test_accept_x_auth_token_header PASSED [100%]
  >
  > ============================== 2 passed in 0.54s ===============================
  > ```
- [x] Traceability-guard PASSES against `specs/020-security-hardening` — **Phase:** validate
  > Evidence: see report.md `### Validation Evidence` for the full guard output. Final lines:
  > ```
  > ℹ️  DoD fidelity: 18 scenarios checked, 18 mapped to DoD, 0 unmapped
  > ℹ️  Concrete test file references: 18
  > ℹ️  Report evidence references: 18
  > RESULT: PASSED (0 warnings)
  > ```
- [x] Artifact-lint PASSES against parent and bug folder — **Phase:** validate
  > Evidence: see report.md `### Audit Evidence` for both runs.
- [x] No production code changed (boundary preserved) — **Phase:** audit
  > Evidence: `git diff --name-only` (post-fix) shows changes confined to `specs/020-security-hardening/scopes.md`, `specs/020-security-hardening/report.md`, `specs/020-security-hardening/scenario-manifest.json`, and `specs/020-security-hardening/bugs/BUG-020-001-dod-scenario-fidelity-gap/*`. No files under `internal/`, `cmd/`, `ml/`, `config/` are touched.
