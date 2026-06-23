# Scopes: BUG-067-001 — ML_LOG_LEVEL SST fail-loud + over-age exception elimination

Two scopes close the two entangled findings. Scope 1 closes **G01** (the live NO-DEFAULTS violation + over-age exception) by SST-ifying `ml.log_level`. Scope 2 closes **G02** (the magic-constant test caps that masked G01) and lands the adversarial regression.

---

## Scope 1: SST-ify ML_LOG_LEVEL + fail-loud sidecar + delete the policy exception (closes G01)

**Status:** Done
**Priority:** P0
**Closes findings:** GAP-067-G01 (live-but-latent NO-DEFAULTS violation `ml/app/main.py:22` + over-age exception `G067-A05-ml-log-level`).

### Gherkin
- SCN-BUG-067-001-001 — ML sidecar fails loud when `ML_LOG_LEVEL` is missing.
- SCN-BUG-067-001-002 — ML sidecar applies a valid SST log level with no default fallback and no policy-exception marker.
- SCN-BUG-067-001-003 — config generation emits `ML_LOG_LEVEL` for dev and test.

### Test Plan

| Test Type | Scenarios | Test Functions | Location |
|-----------|-----------|----------------|----------|
| Unit (Python) | Missing `ML_LOG_LEVEL` → `SystemExit`; valid value accepted | `test_check_required_config_requires_named_keys`, `test_check_required_config_allows_ollama_without_api_key` | `ml/tests/test_main.py` |
| Unit (static) | `ml/app/main.py` has no `os.environ.get("ML_LOG_LEVEL", <literal>)` and no `# smackerel:policy-exception` marker | `PythonNoDefaultsGuard` real-corpus scan + grep | `ml/app/main.py`, `tests/integration/policy/no_defaults_python_guard_test.go` |
| Integration (file-system) | Real `ml/app/` produces zero `G067-A05` findings at the real SST cap | `TestPythonNoDefaultsGuard_RealCorpusIsClean` | `tests/integration/policy/no_defaults_python_guard_test.go` |
| Static (config-gen) | `./smackerel.sh config generate` emits non-empty `ML_LOG_LEVEL=` in `dev.env` and `test.env` | `grep ML_LOG_LEVEL config/generated/{dev,test}.env` | `config/generated/` |
| Regression E2E | `ML_LOG_LEVEL` SST emission + fail-loud read persists; the deleted exception does not reappear (closes BUG-067-001:Scope-1) — the full `tests/integration/policy/...` package + `ml/tests/...` package re-run as the scope-level regression contract | `go test -tags integration ./tests/integration/policy/...` + `./smackerel.sh test unit --python` | `tests/integration/policy/`, `ml/tests/` |

### Definition of Done

- [x] `config/smackerel.yaml` declares `services.ml.log_level` (ml-owned; distinct from `runtime.log_level`). → Evidence: report.md "Phase: implement".
- [x] `scripts/commands/config.sh` extracts `ML_LOG_LEVEL` via `required_value services.ml.log_level` and emits `ML_LOG_LEVEL=${ML_LOG_LEVEL}` in the env-render heredoc (single point feeding dev/test/bundle). → Evidence: report.md "Phase: implement".
- [x] `./smackerel.sh config generate` emits a non-empty `ML_LOG_LEVEL=` into `config/generated/dev.env` AND `config/generated/test.env`. → Evidence: report.md "Phase: implement" (config-gen block).
- [x] `ml/app/main.py` reads `ML_LOG_LEVEL` fail-loud (added to `_check_required_config` keys; no literal fallback; allowlist-validated) and has no `# smackerel:policy-exception` markers. → Evidence: report.md "Phase: implement".
- [x] `policy-exception-baseline.json` no longer contains `G067-A05-ml-log-level` (`exceptions: []`). → Evidence: report.md "Phase: implement".
- [x] `ml/tests/test_main.py` wires `ML_LOG_LEVEL` into the autouse fixture + success-path tests so existing assertions still execute. → Evidence: report.md "Phase: test".
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior in BUG-067-001 Scope 1 run against `ml/tests/test_main.py` + `tests/integration/policy/no_defaults_python_guard_test.go` (`TestPythonNoDefaultsGuard_RealCorpusIsClean`) and stay GREEN as the persistent regression contract (closes BUG-067-001:Scope-1 finding) — **Phase:** regression — see report.md regression phase Evidence section.
- [x] Broader E2E regression suite passes for BUG-067-001 Scope 1 via `go test -tags integration ./tests/integration/policy/...` (full policy-guard package) + `./smackerel.sh test unit --python` (full ml sidecar unit suite) returning clean; browser/live-stack E2E is not applicable — this change has zero runtime/UI surface (config-SST + Python config-read + Go-test change), so there is no browser/runtime behavior to exercise (closes BUG-067-001:Scope-1 broader-suite finding) — **Phase:** regression — see report.md regression phase Evidence section.

```text
✅ Evidence captured in report.md. Config-generation emission verified for dev.env AND test.env.
   Python sidecar fail-loud read + allowlist verified by ml/tests/test_main.py.
   No G041 manipulation: no DoD checkboxes deleted, no scope statuses renamed, no claim sentences weakened.
```

---

## Scope 2: Close the magic-constant test hole + adversarial regression (closes G02)

**Status:** Done
**Priority:** P0
**Closes findings:** GAP-067-G02 (real-baseline guard tests override `ExceptionMaxAgeDays` to magic constants, masking G01; spec 067 Hard Constraint 2 violation).

### Gherkin
- SCN-BUG-067-001-004 — the committed baseline is clean at the real SST cap.
- SCN-BUG-067-001-005 — the guard flags a future over-age exception at the real cap (RED-if-reintroduced); an in-range exception is not flagged.

### Test Plan

| Test Type | Scenarios | Test Functions | Location |
|-----------|-----------|----------------|----------|
| Integration (file-system) | Real-baseline tests validate at the REAL SST cap (90, read from `config/smackerel.yaml`), not magic constants | `TestPythonNoDefaultsGuard_RealCorpusIsClean`, `TestGoNoDefaultsGuard_RealCorpusIsClean`, `TestKeywordMapGuard_RealCorpusRunsAndProducesWellFormedFindings`, `TestKeywordRoutingGuard_RealCorpusRunsAndProducesWellFormedFindings` | `tests/integration/policy/*_guard_test.go` |
| Integration (adversarial) | Committed baseline has zero over-age exceptions at the real cap; a 162-day fixture IS flagged `G067-A07`, an 80-day fixture is NOT | `TestRealBaselineHasNoOverAgeExceptionsAtRealCap`, `TestValidateExceptionFlagsOverAgeAtRealCap` | `tests/integration/policy/no_defaults_python_guard_test.go` |
| Integration (helper) | The real cap is read from SST, not re-hardcoded | `realPolicyExceptionMaxAgeDays` | `tests/integration/policy/no_defaults_python_guard_test.go` |
| Regression E2E | The cap-anchoring + adversarial regression persist; the masking cannot silently return (closes BUG-067-001:Scope-2) — the full `tests/integration/policy/...` package re-runs as the scope-level regression contract | `go test -tags integration ./tests/integration/policy/...` | `tests/integration/policy/` |

### Definition of Done

- [x] A package helper `realPolicyExceptionMaxAgeDays(t)` reads `policy.policy_exception_max_age_days` from `config/smackerel.yaml` (SST-anchored, no re-hardcoded magic number). → Evidence: report.md "Phase: implement".
- [x] The 4 real-baseline caps (`no_defaults_python_guard_test.go` L26, `no_defaults_go_guard_test.go` L26, `keyword_map_guard_test.go` L43, `keyword_routing_guard_test.go` L50) use the helper instead of `365*10` / `180`. → Evidence: report.md "Phase: implement".
- [x] `TestRealBaselineHasNoOverAgeExceptionsAtRealCap` exists, was proven RED against the unfixed baseline (G067-A05 present) and GREEN after removal, same session. → Evidence: report.md "Phase: test" (RED→GREEN).
- [x] `TestValidateExceptionFlagsOverAgeAtRealCap` exists and proves the guard flags a 162-day fixture `G067-A07` at the real cap and does not flag an 80-day fixture (non-tautological teeth). → Evidence: report.md "Phase: test".
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior in BUG-067-001 Scope 2 run against `tests/integration/policy/no_defaults_python_guard_test.go` (the 2 adversarial tests + the 4 cap-anchored real-baseline tests) and stay GREEN as the persistent regression contract (closes BUG-067-001:Scope-2 finding) — **Phase:** regression — see report.md regression phase Evidence section.
- [x] Broader E2E regression suite passes for BUG-067-001 Scope 2 via `go test -tags integration ./tests/integration/policy/...` (full policy-guard package, all guards GREEN at the real cap) returning clean; browser/live-stack E2E is not applicable — this change exercises file-system policy tests only, with no browser/runtime behavior to exercise (closes BUG-067-001:Scope-2 broader-suite finding) — **Phase:** regression — see report.md regression phase Evidence section.

```text
✅ Evidence captured in report.md. RED→GREEN proof for TestRealBaselineHasNoOverAgeExceptionsAtRealCap captured same session.
   Real SST cap (90) read from config/smackerel.yaml via realPolicyExceptionMaxAgeDays; no magic constant re-introduced.
   No G041 manipulation: no DoD checkboxes deleted, no scope statuses renamed, no claim sentences weakened.
```
