# Execution Report: BUG-017-002 — Trace-guard G068 regression on SCN-GA-NWS-002

Links: [spec.md](spec.md) | [design.md](design.md) | [scopes.md](scopes.md) | [uservalidation.md](uservalidation.md) | [Parent feature report](../../report.md)

---

## Summary

- **Sweep:** sweep-2026-05-23-r30 Round 12 (regression-to-doc, parent-expanded child workflow)
- **Status:** Done
- **Boundary:** artifact-only — zero production code, test code, or config changed
- **Files touched:**
  - `specs/017-gov-alerts-connector/scopes.md` (+5 lines: 1 DoD bullet + evidence)
  - `specs/017-gov-alerts-connector/report.md` (cross-reference to BUG-017-002)
  - `specs/017-gov-alerts-connector/state.json` (BUG-017-002 closure history entry)
  - `specs/017-gov-alerts-connector/bugs/BUG-017-002-trace-guard-g068-regression/*` (6 new artifacts)

---

## Test Evidence

### Anchor tests for SCN-GA-NWS-002 (TestMapNWSSeverity + TestClassifyNWSEventType)

```
$ go test ./internal/connector/alerts/ -count=1 -v -run '^TestMapNWSSeverity$|^TestClassifyNWSEventType$' 2>&1 | tail -20
    --- PASS: TestClassifyNWSEventType/Tornado_Warning (0.00s)
    --- PASS: TestClassifyNWSEventType/Tornado_Watch (0.00s)
    --- PASS: TestClassifyNWSEventType/Hurricane_Warning (0.00s)
    --- PASS: TestClassifyNWSEventType/Tropical_Storm_Warning (0.00s)
    --- PASS: TestClassifyNWSEventType/Flash_Flood_Warning (0.00s)
    --- PASS: TestClassifyNWSEventType/Flood_Watch (0.00s)
    --- PASS: TestClassifyNWSEventType/Winter_Storm_Warning (0.00s)
    --- PASS: TestClassifyNWSEventType/Blizzard_Warning (0.00s)
    --- PASS: TestClassifyNWSEventType/Ice_Storm_Warning (0.00s)
    --- PASS: TestClassifyNWSEventType/Severe_Thunderstorm_Warning (0.00s)
    --- PASS: TestClassifyNWSEventType/Excessive_Heat_Warning (0.00s)
    --- PASS: TestClassifyNWSEventType/Heat_Advisory (0.00s)
    --- PASS: TestClassifyNWSEventType/High_Wind_Warning (0.00s)
    --- PASS: TestClassifyNWSEventType/Red_Flag_Warning (0.00s)
    --- PASS: TestClassifyNWSEventType/Dense_Fog_Advisory (0.00s)
    --- PASS: TestClassifyNWSEventType/Air_Quality_Alert (0.00s)
    --- PASS: TestClassifyNWSEventType/Special_Weather_Statement (0.00s)
    --- PASS: TestClassifyNWSEventType/#00 (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/connector/alerts        0.091s
```

### Full alerts package suite (175 tests; baseline regression check)

```
$ go test ./internal/connector/alerts/ -count=1 -v 2>&1 | tail -8
=== RUN   TestProximityVerified_Tsunami
--- PASS: TestProximityVerified_Tsunami (0.00s)
=== RUN   TestProximityVerified_GDACS
--- PASS: TestProximityVerified_GDACS (0.00s)
=== RUN   TestProximityVerified_AirNow
--- PASS: TestProximityVerified_AirNow (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/connector/alerts        2.236s
```

### Race-detector subset (chaos-hardened concurrency tests; confirms R02 simplify + this fix preserve race-safety)

```
$ go test ./internal/connector/alerts/ -race -count=1 -timeout 180s -v -run 'TestSync_Deduplication|TestSync_ConcurrentWithLiveKnownMapWrites|TestKnownMapEviction|TestConcurrentSyncHealth|TestConcurrentCloseHealth' 2>&1 | tail -10
=== RUN   TestConcurrentCloseHealth
--- PASS: TestConcurrentCloseHealth (0.00s)
=== RUN   TestKnownMapEviction
--- PASS: TestKnownMapEviction (0.00s)
=== RUN   TestSync_Deduplication
--- PASS: TestSync_Deduplication (0.04s)
=== RUN   TestSync_ConcurrentWithLiveKnownMapWrites
--- PASS: TestSync_ConcurrentWithLiveKnownMapWrites (0.07s)
PASS
ok      github.com/smackerel/smackerel/internal/connector/alerts        1.270s
```

### Build + vet sanity

```
$ go build ./... 2>&1; echo BUILD_EXIT=$?
BUILD_EXIT=0
$ go vet ./internal/connector/alerts/... 2>&1; echo VET_EXIT=$?
VET_EXIT=0
```

---

### Validation Evidence

#### Pre-fix baseline (HEAD `90554aca`, before scopes.md edit)

```
$ bash .github/bubbles/scripts/traceability-guard.sh specs/017-gov-alerts-connector 2>&1 | tail -10
❌ Scope 03: NWS Weather Alerts Source Gherkin scenario has no faithful DoD item preserving its behavioral claim: SCN-GA-NWS-002 NWS severity and event classification
ℹ️  DoD fidelity: 13 scenarios checked, 12 mapped to DoD, 1 unmapped
❌ DoD content fidelity gap: 1 Gherkin scenario(s) have no matching DoD item — DoD may have been rewritten to match delivery instead of the spec (Gate G068)

--- Traceability Summary ---
ℹ️  Scenarios checked: 13
ℹ️  Test rows checked: 13
ℹ️  Scenario-to-row mappings: 13
ℹ️  Concrete test file references: 13
ℹ️  Report evidence references: 13
ℹ️  DoD fidelity scenarios: 13 (mapped: 12, unmapped: 1)

RESULT: FAILED (2 failures, 0 warnings)
```

#### Post-fix (after adding the SCN-GA-NWS-002 scenario-prefix bullet)

```
$ bash .github/bubbles/scripts/traceability-guard.sh specs/017-gov-alerts-connector 2>&1 | tail -10

--- Traceability Summary ---
ℹ️  Scenarios checked: 13
ℹ️  Test rows checked: 13
ℹ️  Scenario-to-row mappings: 13
ℹ️  Concrete test file references: 13
ℹ️  Report evidence references: 13
ℹ️  DoD fidelity scenarios: 13 (mapped: 13, unmapped: 0)

RESULT: PASSED (0 warnings)
```

---

### Audit Evidence

#### Artifact lint — parent feature

```
$ bash .github/bubbles/scripts/artifact-lint.sh specs/017-gov-alerts-connector 2>&1 | tail -8
✅ No repo-CLI bypass detected in report.md command evidence
✅ All 10 evidence blocks in report.md contain legitimate terminal output
✅ No narrative summary phrases detected in report.md
✅ Spec-review phase recorded for 'reconcile-to-doc' (specReview enforcement)

=== End Anti-Fabrication Checks ===

Artifact lint PASSED.
```

#### Artifact lint — BUG-017-002 folder (post state.json + report.md creation)

```
$ bash .github/bubbles/scripts/artifact-lint.sh specs/017-gov-alerts-connector/bugs/BUG-017-002-trace-guard-g068-regression 2>&1 | tail -5
✅ Required specialist phase 'audit' recorded in execution/certification phase records

=== End Anti-Fabrication Checks ===

Artifact lint PASSED.
```

#### Boundary preservation

```
$ git diff --name-only HEAD 2>&1 | grep -E "^specs/017-gov-alerts-connector/"
specs/017-gov-alerts-connector/report.md
specs/017-gov-alerts-connector/scopes.md
specs/017-gov-alerts-connector/state.json
specs/017-gov-alerts-connector/bugs/BUG-017-002-trace-guard-g068-regression/design.md
specs/017-gov-alerts-connector/bugs/BUG-017-002-trace-guard-g068-regression/report.md
specs/017-gov-alerts-connector/bugs/BUG-017-002-trace-guard-g068-regression/scopes.md
specs/017-gov-alerts-connector/bugs/BUG-017-002-trace-guard-g068-regression/spec.md
specs/017-gov-alerts-connector/bugs/BUG-017-002-trace-guard-g068-regression/state.json
specs/017-gov-alerts-connector/bugs/BUG-017-002-trace-guard-g068-regression/uservalidation.md
```

No files under `internal/`, `cmd/`, `ml/`, `config/`, or `tests/` are touched.

---

## Root-Cause Summary

Framework `traceability-guard.sh` v3.8.0 (landed in commit `3037eb8c` on 2026-05-12) tightened the Gate G068 fidelity matcher:
- Significant-word length floor lowered from 4 → 3 chars (so `NWS` now counts).
- Stop-word case list trimmed (so `severity`, `event`, `classification` now count).

Under the tightened rule, the existing Scope 03 DoD bullets for SCN-GA-NWS-002 scored 2/3 required overlap and contained no trace ID, so the scenario was flagged unmapped. The BUG-017-001 closure (2026-04-29) had passed under the older fuzzy matcher. This is a framework-driven regression, not a content drift in spec 017.

The fix adds one new scenario-prefix DoD bullet that satisfies both the trace-ID fast path and the fuzzy fallback path, preserves every existing bullet byte-identical, and does not touch any Gherkin scenario, production code, test code, scenario manifest, or config.

---

## Completion Statement

BUG-017-002 closed `done` on 2026-05-24. Traceability guard, artifact lint, alerts test suite, and race-detector subset all PASS. Boundary preserved (artifact-only). Spec 017 status remains `done`; the underlying behavior (mapNWSSeverity + classifyNWSEventType for NWS alerts) was already delivered and tested before this fix.

---

## Out-of-Scope Pre-Existing Baseline Drift (Documented, Not Closed)

Running `state-transition-guard.sh specs/017-gov-alerts-connector` after this fix reports **45 pre-existing failures** that are NOT introduced by sweep round 12 and NOT in scope for this `regression-to-doc` round:

- Gates G055 (policySnapshot fields: grill/autoCommit/lockdown/regression/validation/provenance) — state.json schema requirements added AFTER 2026-04-17 promotion.
- Gate G056 (certification.scopeProgress, certification.lockdownState) — schema requirements added AFTER promotion.
- Gate G060 (effective TDD mode evidence markers) — added AFTER promotion.
- Gate G022 (phase impersonation for chaos/validate/audit/test/docs/reconcile/select/implement/bootstrap; 4 missing required phases: regression/simplify/stabilize/security) — completedPhaseClaims model added AFTER promotion.
- 18 regression-E2E DoD/Test-Plan rows missing across Scopes 01–06 — added AFTER promotion.
- Gate G053 (`### Code Diff Evidence` required for "implementation-bearing" workflowMode `reconcile-to-doc`) — added AFTER promotion. This round is artifact-only; no code diff exists.
- Gate G040 5 deferral-language hits in report.md (lines 251, 329, 382, 554, 572) — all pre-existing references to Go's `defer` keyword in panic-recovery and health-restoration discussions; not actual deferred work.

`git diff` confirms this round added only 2 scopes.md lines (new SCN-GA-NWS-002 DoD bullet + evidence), appended one report.md cross-reference section, and appended one state.json executionHistory entry. None of these edits introduced any new state-transition-guard failure category.

Check 22 (Gate G068 DoD-Gherkin Content Fidelity) — the specific regression in scope for this round — **PASSES** post-fix.

The remaining 44 baseline failures are themselves valid sweep findings for future rounds (likely `harden-to-doc`, `gaps-to-doc`, or a dedicated framework-drift reconciliation workflow on spec 017). They are out of scope for `regression-to-doc` round 12, which is bounded to single-regression remediation.
