# Scopes: BUG-031-006 Strict-Guard Gate Drift Closure

Closure is broken into 5 scopes that close the 38 BLOCK findings.

---

## Scope 1: Planning Edits — Regression E2E Coverage Across 6 Scopes

**Status:** Not Started
**Owner:** bubbles.design + bubbles.plan
**Closes findings:** 18 (G016 / Check 8A)

### DoD

- [ ] Spec 031 Scope 1 has a scenario-specific regression E2E DoD item referencing `tests/e2e/<existing-file>_test.go`
- [ ] Spec 031 Scope 1 has a broader regression E2E suite DoD item
- [ ] Spec 031 Scope 1 has an explicit regression Test Plan row matching `regression.*E2E.*specs/031.*scope-1`
- [ ] Spec 031 Scope 2 has a scenario-specific regression E2E DoD item
- [ ] Spec 031 Scope 2 has a broader regression E2E suite DoD item
- [ ] Spec 031 Scope 2 has an explicit regression Test Plan row
- [ ] Spec 031 Scope 3 has a scenario-specific regression E2E DoD item
- [ ] Spec 031 Scope 3 has a broader regression E2E suite DoD item
- [ ] Spec 031 Scope 3 has an explicit regression Test Plan row
- [ ] Spec 031 Scope 4 has a scenario-specific regression E2E DoD item
- [ ] Spec 031 Scope 4 has a broader regression E2E suite DoD item
- [ ] Spec 031 Scope 4 has an explicit regression Test Plan row
- [ ] Spec 031 Scope 5 has a scenario-specific regression E2E DoD item
- [ ] Spec 031 Scope 5 has a broader regression E2E suite DoD item
- [ ] Spec 031 Scope 5 has an explicit regression Test Plan row
- [ ] Spec 031 Scope 6 has a scenario-specific regression E2E DoD item
- [ ] Spec 031 Scope 6 has a broader regression E2E suite DoD item
- [ ] Spec 031 Scope 6 has an explicit regression Test Plan row

### Test Plan

| Scope | Test Type | File |
|-------|-----------|------|
| Scope 1 | regression E2E | `tests/e2e/capture_process_search_test.go` |
| Scope 2 | regression E2E | `tests/e2e/capture_process_search_test.go` |
| Scope 3 | regression E2E | `tests/e2e/capture_process_search_test.go` |
| Scope 4 | regression E2E | `tests/integration/nats_stream_test.go` |
| Scope 5 | regression E2E | `tests/integration/db_migration_test.go` |
| Scope 6 | regression E2E | `tests/integration/ml_readiness_test.go` + new stress |

### Gherkin

```gherkin
Scenario: All 6 scopes have complete regression E2E planning
  Given specs/031-live-stack-testing/scopes.md has been edited
  When state-transition-guard.sh runs
  Then Check 8A passes for all 6 scopes
  And no regression-E2E-related BLOCK finding remains
```

---

## Scope 2: Planning Edits — Change Boundary Section

**Status:** Not Started
**Owner:** bubbles.design + bubbles.plan
**Closes findings:** 3 (Check 8D)

### DoD

- [ ] Spec 031 scopes.md has a `## Change Boundary` section enumerating allowed surfaces (test files, `internal/api/ml_readiness.go`, scripts)
- [ ] The `## Change Boundary` section enumerates excluded surfaces (spec 055 notification code, `cmd/core/**`, `config/smackerel.yaml`, framework files)
- [ ] Each of the 6 scopes has a change-boundary DoD item referencing the section

### Test Plan

| Scope | Test Type | Verification |
|-------|-----------|--------------|
| All | gate | `state-transition-guard.sh` Check 8D passes |

### Gherkin

```gherkin
Scenario: Change Boundary contains the cleanup-helper context
  Given specs/031-live-stack-testing/scopes.md has the new Change Boundary section
  When state-transition-guard.sh runs
  Then Check 8D passes
  And no change-boundary BLOCK finding remains
```

---

## Scope 3: Implementation — Scope 6 SLA Stress Test

**Status:** Not Started
**Owner:** bubbles.implement + bubbles.test
**Closes findings:** 2 (Check 5A SLA + G060 TDD red→green)

### DoD

- [ ] `tests/stress/ml_readiness_timeout_stress_test.go` exists
- [ ] Test consumes `CORE_EXTERNAL_URL`, `ML_BASE_URL`, `SMACKEREL_ML_READINESS_TIMEOUT` from SST env (no hardcoded fallbacks)
- [ ] Test asserts the 60-second timeout boundary fires at the configured value
- [ ] Test runs against the disposable test stack only (verified by Compose project name + named volume prefix)
- [ ] Adversarial case 1: silent timeout bypass detection (test fails if timeout removed from `internal/api/ml_readiness.go`)
- [ ] Adversarial case 2: always-200 regression (test fails if `/ml/readyz` returns 200 unconditionally)
- [ ] Adversarial case 3: wrong-stack URL fails fast (test fails if pointed at dev stack)
- [ ] Adversarial case 4: missing SST env fails loud (test fails if `SMACKEREL_ML_READINESS_TIMEOUT` is empty)
- [ ] Scenario-first TDD red commit lands first (test failing) with `spec(031)` prefix
- [ ] Scenario-first TDD green commit lands second (test passing) with `spec(031)` prefix
- [ ] `bubbles.test` `executionHistory` entry recorded with `completedPhaseClaimDetails`

### Test Plan

| Test | File | Type |
|------|------|------|
| SLA boundary stress | `tests/stress/ml_readiness_timeout_stress_test.go` | stress |
| Adversarial silent-bypass | same file | stress (adversarial) |
| Adversarial always-200 | same file | stress (adversarial) |
| Adversarial wrong-stack | same file | stress (adversarial) |
| Adversarial missing-env | same file | stress (adversarial) |

### Gherkin

```gherkin
Scenario: SLA timeout test exists and is wired to SST env
  Given tests/stress/ml_readiness_timeout_stress_test.go exists
  When ./smackerel.sh test stress runs
  Then the SLA test reads SMACKEREL_ML_READINESS_TIMEOUT from env
  And the test asserts the configured boundary
  And the test never touches the persistent dev stack

Scenario: Adversarial silent-bypass detection
  Given a hypothetical edit removes the 60-second timeout from internal/api/ml_readiness.go
  When ./smackerel.sh test stress runs
  Then ml_readiness_timeout_stress_test.go fails before merge
```

---

## Scope 4: Implementation — Code Diff Evidence Section

**Status:** Not Started
**Owner:** bubbles.implement + bubbles.docs
**Closes findings:** 1 (G053 / Check 13B)

### DoD

- [ ] `specs/031-live-stack-testing/report.md` has a `### Code Diff Evidence` section
- [ ] Section enumerates real implementation deltas: file path, line counts, gate-verifiable references
- [ ] Section is placed inside the standard report.md execution-evidence area (not the reconcile-pass appendix)
- [ ] `bubbles.docs` `executionHistory` entry recorded with `completedPhaseClaimDetails`

### Test Plan

| Test | Type | Verification |
|------|------|--------------|
| Code Diff Evidence presence | gate | `grep -n '^### Code Diff Evidence' report.md` returns ≥ 1 hit |
| Section non-empty | gate | section content has ≥ 1 file-path reference |

### Gherkin

```gherkin
Scenario: report.md has Code Diff Evidence section
  Given specs/031-live-stack-testing/report.md has been updated
  When state-transition-guard.sh runs
  Then Check 13B passes (G053 satisfied)
```

---

## Scope 5: Specialist Phase Re-Runs — G022 Provenance Closure

**Status:** Not Started
**Owner:** bubbles.regression + bubbles.simplify + bubbles.stabilize + bubbles.security + bubbles.test + bubbles.audit + bubbles.chaos + bubbles.docs + bubbles.validate
**Closes findings:** 9 (G022 / Check 6 + 6B)

### DoD

- [ ] `bubbles.regression` runs and emits structured `executionHistory` entry with `completedPhaseClaimDetails`
- [ ] `bubbles.simplify` runs (or records `n/a` with provenance) and emits structured entry
- [ ] `bubbles.stabilize` runs and emits structured entry
- [ ] `bubbles.security` runs and emits structured entry
- [ ] `bubbles.test` re-runs and emits structured entry replacing the impersonation claim
- [ ] `bubbles.audit` runs and emits structured entry replacing the impersonation claim
- [ ] `bubbles.chaos` re-runs and emits structured entry replacing the impersonation claim
- [ ] `bubbles.docs` runs and emits structured entry replacing the impersonation claim
- [ ] `bubbles.validate` re-certifies and emits structured entry replacing the impersonation claim
- [ ] Structured commit `spec(031): close strict-guard gate drift (BUG-031-006)` lands closure (Check 17)

### Test Plan

| Phase | Verification |
|-------|--------------|
| regression | `state.json.executionHistory[].agent == "bubbles.regression"` exists |
| simplify | same for `bubbles.simplify` |
| stabilize | same for `bubbles.stabilize` |
| security | same for `bubbles.security` |
| test | same for `bubbles.test` |
| audit | same for `bubbles.audit` |
| chaos | same for `bubbles.chaos` |
| docs | same for `bubbles.docs` |
| validate | same for `bubbles.validate` |
| commit | `git log --oneline | grep -E '^[0-9a-f]+ (spec\(031\)|bubbles\(031/)'` returns ≥ 1 hit |

### Gherkin

```gherkin
Scenario: All required specialist phases have real executionHistory entries
  Given specs/031-live-stack-testing/state.json has been updated
  When state-transition-guard.sh runs
  Then Check 6 passes (all required phases present)
  And Check 6B passes (no phase impersonation)
  And Check 17 passes (structured commit found)

Scenario: state-transition-guard accepts the full closure
  Given Scopes 1-5 are all Done with real evidence
  And no G041 manipulation pattern is in the closure diff
  When state-transition-guard.sh specs/031-live-stack-testing runs
  Then the script exits 0 with zero BLOCK findings
  And artifact-lint.sh continues to exit 0
  And regression-baseline-guard.sh exits 0
```

---

## Change Boundary

**Allowed surfaces for this bug:**

- `specs/031-live-stack-testing/scopes.md`
- `specs/031-live-stack-testing/report.md`
- `specs/031-live-stack-testing/state.json`
- `specs/031-live-stack-testing/bugs/BUG-031-006-strict-guard-gate-drift/**`
- `tests/stress/ml_readiness_timeout_stress_test.go` (new file)
- `scripts/runtime/go-stress.sh` (only if the new stress file requires harness wiring)

**Excluded surfaces:**

- `internal/api/ml_readiness.go` — no behavioral change permitted; only existing logic is exercised
- `internal/notification/**` and `cmd/core/services.go` / `cmd/core/wiring.go` — spec 055 ntfy adapter is in-flight; do not touch
- `config/smackerel.yaml` — SST contract frozen for this bug
- `config/generated/**` — never hand-edit
- `.github/bubbles/**` — framework-managed
- `internal/api/notifications*.go`, `internal/notification/source/**`, `internal/db/migrations/038_*.sql`, `tests/e2e/notification_ntfy_source_*`, `tests/stress/notification_ntfy_source_*` — spec 055 in-flight surface
- All other specs under `specs/` — out of scope

### Change Boundary DoD (applies to every scope in this bug)

- [ ] Closure edits respect the Change Boundary section (only allowed surfaces touched, all excluded surfaces verified untouched in the closure commit diff via `git diff --cached --name-status`)
