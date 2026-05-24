# Bug: BUG-019-002 — DoD scenario fidelity gap (SCN-019-004 + SCN-019-005)

## Classification

- **Type:** Artifact-only documentation/traceability bug
- **Severity:** MEDIUM (governance gate failure on a feature already marked `done`; no runtime impact)
- **Parent Spec:** 019 — Connector Wiring (Register 5 Unwired Connectors)
- **Workflow Mode:** bugfix-fastlane
- **Sweep:** sweep-2026-05-23-r30 round 28 (test trigger → test-to-doc child mode)
- **Status:** Fixed (artifact-only)

## Problem Statement

The stochastic-quality-sweep round 28 `test` trigger on `specs/019-connector-wiring` re-ran the connector unit + integration tests (all green) and then exercised the governance guards. Production code is fine; all behavior tests for the 5 wired connectors PASS. However, `traceability-guard.sh` flagged 2 fresh Gate G068 (DoD-Gherkin content fidelity) gaps that BUG-019-001 had not addressed:

1. **SCN-019-004 (`Config entries exist for all 5 connectors in smackerel.yaml`):** no DoD bullet preserved enough scenario-distinguishing words. The pre-existing DoD items "4 new YAML config blocks added to `config/smackerel.yaml` (Discord already existed)" and "`./smackerel.sh config generate` produces env vars for all 5 connectors in `config/generated/dev.env`" each shared only 3 of the 9 significant scenario words; the v3.8.0 percentage-based threshold requires `score >= ceil(word_count / 2)` AND `score >= 3`, i.e. >= 5 for SCN-019-004. Both DoD items fell below the bar.
2. **SCN-019-005 (`Health endpoint shows all 15 connectors`):** the pre-existing DoD item "Health endpoint lists all 15 connectors — Evidence: …" shared 3 of the 7 significant scenario words (health, endpoint, connectors); the threshold for a 7-word scenario is `score >= 4`. The word "shows" is the scenario's discriminating verb and was never preserved in any DoD item. Note that `15` (length 2) is stripped by the matcher's length filter.

## Reproduction (Pre-fix)

```
$ timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/019-connector-wiring 2>&1 | tail -10
❌ Scope 1: Wire All 5 Connectors Gherkin scenario has no faithful DoD item preserving its behavioral claim: SCN-019-004 Config entries exist for all 5 connectors in smackerel.yaml
❌ Scope 1: Wire All 5 Connectors Gherkin scenario has no faithful DoD item preserving its behavioral claim: SCN-019-005 Health endpoint shows all 15 connectors
ℹ️  DoD fidelity: 6 scenarios checked, 4 mapped to DoD, 2 unmapped
❌ DoD content fidelity gap: 2 Gherkin scenario(s) have no matching DoD item — DoD may have been rewritten to match delivery instead of the spec (Gate G068)

RESULT: FAILED (3 failures, 0 warnings)
```

## Gap Analysis (per scenario)

For each flagged scenario, the bug investigator re-ran the underlying behavior tests and inspected the production wiring. Both scenarios are **delivered-but-undocumented at the trace-ID/word-fidelity level** — no production code is missing, no test fixture is missing; the only gap is documentation linkage.

| Scenario | Behavior delivered? | Tests pass? | Concrete test file | Concrete source |
|---|---|---|---|---|
| SCN-019-004 Config entries exist for all 5 connectors in smackerel.yaml | Yes — `config/smackerel.yaml` lines 263, 277, 284, 295, 318 define `discord/twitter/weather/gov-alerts/financial-markets`; each defaults to `enabled: false` | Yes — `tests/integration/test_connector_wiring.sh` (32 PASS / 0 FAIL, exit 0, `SCN-019-004: PASS`) | `tests/integration/test_connector_wiring.sh` | `config/smackerel.yaml`, `scripts/commands/config.sh` |
| SCN-019-005 Health endpoint shows all 15 connectors | Yes — `internal/api/health.go` defines `ConnectorHealthLister`; the registry holding all 15 connector instances is wired into the handler | Yes — `TestHealthHandler_ConnectorHealth` PASS (focused run exits 0) | `internal/api/health_test.go` | `internal/api/health.go::ConnectorHealthLister`, `cmd/core/connectors.go::registerConnectors` |

**Disposition:** Both scenarios are **delivered-but-undocumented at the fidelity-matcher level** — artifact-only fix.

## Acceptance Criteria

- [x] Parent `specs/019-connector-wiring/scopes.md` Scope 1 DoD has 2 new trace-ID-bearing bullets `Scenario SCN-019-004 (...)` and `Scenario SCN-019-005 (...)` that each preserve enough scenario-distinguishing words to satisfy the v3.8.0 G068 matcher's `score >= ceil(word_count/2) AND score >= 3` threshold
- [x] No pre-existing DoD bullet in `specs/019-connector-wiring/scopes.md` is deleted, weakened, or rewritten (additive fix only — Gate G068's stated failure mode "DoD may have been rewritten to match delivery instead of the spec" is not triggered)
- [x] Parent `specs/019-connector-wiring/scenario-manifest.json` already covers SCN-019-004 and SCN-019-005 with concrete linked tests (`tests/integration/test_connector_wiring.sh` and `internal/api/health_test.go::TestHealthHandler_ConnectorHealth`) — no change needed; verified
- [x] `bash .github/bubbles/scripts/artifact-lint.sh specs/019-connector-wiring` PASS
- [x] `bash .github/bubbles/scripts/artifact-lint.sh specs/019-connector-wiring/bugs/BUG-019-002-scn-019-004-005-fidelity-gap` PASS
- [x] `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/019-connector-wiring` PASS (was FAILED 3/0; expected `DoD fidelity: 6 mapped, 0 unmapped` and `RESULT: PASSED (0 warnings)`)
- [x] Underlying behavior tests for SCN-019-004 and SCN-019-005 still PASS (`tests/integration/test_connector_wiring.sh` 32/32 and `internal/api/health_test.go::TestHealthHandler_ConnectorHealth`)
- [x] No production code changed (boundary preserved: `git diff --name-only` shows zero changes under `internal/`, `cmd/`, `ml/`, `config/`, `scripts/`, `tests/`)
