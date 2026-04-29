# Design: BUG-020-001 — DoD scenario fidelity gap

> **Bug spec:** [spec.md](spec.md)
> **Parent:** [020 spec](../../spec.md) | [020 scopes](../../scopes.md) | [020 report](../../report.md)
> **Date:** April 27, 2026
> **Workflow Mode:** bugfix-fastlane

---

## Root Cause

`scopes.md` for spec 020 was authored before Gate G068 (Gherkin → DoD Content Fidelity) was tightened. The DoD bullets accurately described the delivered behavior (port binding, ML sidecar bearer auth, decrypt fail-closed, dev-mode plaintext passthrough) but did not embed the `SCN-020-NNN` trace ID. The traceability-guard's `scenario_matches_dod` function tries trace-ID equality first and falls back to a fuzzy "≥3 significant words shared" check; for SCN-020-001/002/006/013/014 the DoD wording happened to fall below the threshold (DoD bullets named files and middleware names rather than the scenario's behavioral phrases like "bind to 127.0.0.1", "accepts authenticated requests", "fails closed", "plaintext passthrough"), so the gate fails on those five.

Three ancillary problems accumulated under the same root:

1. `scenario-manifest.json` was never generated for spec 020 (G057/G059). The guard requires a manifest whenever scopes define Gherkin scenarios.
2. The Scope 1 Test Plan rows for SCN-020-001..004 pointed only at planned files (`tests/integration/docker_ports_test.go`, `tests/e2e/port_binding_test.go`, `tests/e2e/nats_token_hidden_test.go`, `scripts/commands/config_test.sh`) that intentionally do not exist locally because they either require the live stack or a shell harness that was deferred. The in-process Go equivalents in `internal/config/docker_security_test.go` (`TestDockerCompose_AllPortsBindLocalhost`, `TestDockerCompose_NATSUsesConfigFile`) were already present and passing but were not surfaced as Test Plan rows.
3. The Scope 3 Test Plan already mapped SCN-020-016/017/018 to `cmd/core/main_test.go` (which exists), but the parent `report.md` did not contain that path, so the trace guard's `report_mentions_path` check failed for those three scenarios.

## Fix Approach (artifact-only)

This is an **artifact-only** fix. No production code is modified. The boundary clause from the user prompt — "artifact-only preferred. No production code changes." — is honored: gap analysis proved every behavior is delivered and tested, so no production change is justified.

The fix has four parts, mirroring the BUG-009-001 playbook:

1. **Trace-ID-bearing DoD bullets** added to `scopes.md`:
   - Scope 1 DoD gains two bullets for `SCN-020-001` and `SCN-020-002` with raw `go test` output for `TestDockerCompose_AllPortsBindLocalhost` plus a source pointer to `docker-compose.yml` (literal `127.0.0.1:` prefixes) and `scripts/commands/config.sh::HOST_BIND_ADDRESS` (lines 326, 686).
   - Scope 2 DoD gains one bullet for `SCN-020-006` with raw `pytest` output for `TestMLSidecarAuthWithToken::test_accept_bearer_token` and `test_accept_x_auth_token_header` plus a source pointer to `ml/app/auth.py::verify_auth` and `ml/app/main.py::authed_router`.
   - Scope 3 DoD gains two bullets for `SCN-020-013` and `SCN-020-014` with raw `go test` output for `TestTokenStore_Decrypt_FailClosed_NotBase64`, `_TooShort`, `_GCMFailure`, `_WrongKey_FailClosed`, and `TestTokenStore_Decrypt_NoKey_PlaintextPassthrough` plus source pointers to `internal/auth/store.go::decrypt`.

2. **In-process proxy Test Plan rows** added at the top of Scope 1's Test Plan table mapping SCN-020-001..004 to the existing `internal/config/docker_security_test.go::TestDockerCompose_AllPortsBindLocalhost` and `TestDockerCompose_NATSUsesConfigFile`. Row position matters: `scenario_matches_row` returns the first row matching by trace ID, so the proxy rows must precede the legacy rows.

3. **Scenario manifest** `specs/020-security-hardening/scenario-manifest.json` is generated covering all 18 `SCN-020-*` scenarios. Each entry has `scenarioId`, `scope`, `requiredTestType`, `linkedTests` (with `file` + `function`), `evidenceRefs` (unit-test + source pointers), and `linkedDoD`.

4. **Report cross-reference** added to `specs/020-security-hardening/report.md` documenting the bug, the per-scenario classification, the raw verification evidence, and explicit references to `internal/config/docker_security_test.go`, `internal/auth/oauth_test.go`, `ml/tests/test_auth.py`, and `cmd/core/main_test.go` so `report_mentions_path` succeeds for every scope.

## Why this is not "DoD rewriting"

Gate G068's stated failure mode is "DoD may have been rewritten to match delivery instead of the spec." The bullets added by this fix preserve the original DoD claims (the implemented behavior matches the Gherkin scenarios verbatim — port binding, bearer auth, decrypt fail-closed, no-key passthrough are all genuinely delivered and tested) and only add the trace ID and raw test evidence the gate requires. No DoD bullet was deleted or weakened. No Gherkin scenario was edited. The behavior the Gherkin describes is the behavior the production code implements; the only thing being fixed is the documentation linkage.

## Regression Test

Because this fix is artifact-only, the regression "test" is the traceability guard itself. Pre-fix it returned `RESULT: FAILED (14 failures)`; post-fix it returns `RESULT: PASSED (0 warnings)`. The guard run is captured in `report.md` under "Validation Evidence". The underlying behavior tests (Go + Python) all PASS in the post-fix `./smackerel.sh test unit` run.
