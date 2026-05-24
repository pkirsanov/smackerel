# Bug: BUG-031-007 R8 sweep test trigger surfaced artifact fidelity drift on spec 031

## Summary

Stochastic-quality-sweep `sweep-2026-05-23-r30` round 8 (mappedMode `test-to-doc`) probed the spec 031 test surface after the round 3 BUG-031-006 closure landed. The compile sweep (`go vet -tags="integration stress"` and `go build -tags="integration stress"`) was clean across `./tests/integration/...`, `./tests/e2e/...`, `./tests/stress/...`, and `./internal/api/...`, but a scenario-manifest fidelity audit and a parent-state bookkeeping audit surfaced three artifact-level drift items that were missed by the round 3 closure pass:

1. `specs/031-live-stack-testing/scenario-manifest.json` SCN-LST-005 `linkedTests` lists `{file: scripts/runtime/go-integration.sh, function: test_integration}`, but `scripts/runtime/go-integration.sh` is a 17-line top-level script with zero functions defined. The function-name claim is a manifest fidelity defect.
2. `specs/031-live-stack-testing/state.json` shows `activeBugs: ["BUG-031-006-strict-guard-gate-drift"]` and `resolvedBugs: []`, but `specs/031-live-stack-testing/bugs/BUG-031-006-strict-guard-gate-drift/state.json` has `status: done` and `certification.status: done`. The round 3 `bubbles.validate` executionHistory entry textually claimed "BUG-031-006 moved from activeBugs to resolvedBugs" but the parent state.json field write did not land.
3. `specs/031-live-stack-testing/scenario-manifest.json` has no scenario entry that links to the three new SLA stress tests added by BUG-031-006 in `tests/stress/ml_readiness_timeout_stress_test.go` (`TestMLReadinessTimeoutBoundary`, `TestMLReadinessTimeoutSilentBypass`, `TestMLReadinessAlways200Regression`). Existing SCN-LST-004 covers ML readiness integration tests only.

The change manifest for this bug is artifact-edit only: `specs/031-live-stack-testing/scenario-manifest.json` (T-031-001, T-031-003), `specs/031-live-stack-testing/state.json` (T-031-002), and this BUG packet. Zero production source modified.

## Severity

- [ ] Critical - System unusable, data loss
- [ ] High - Shared validation blocks deliveries
- [ ] Medium - Feature broken, workaround exists
- [x] Low - Artifact fidelity drift; no runtime impact, no test execution regression. State-transition-guard.sh on spec 031 already permits the spec-level transition (2 warnings, 0 blocks); drift is below the spec-level fail threshold but above the manifest-fidelity correctness bar.

## Status

- [x] Reported
- [x] Confirmed
- [x] In Progress
- [x] Fixed
- [x] Verified
- [x] Closed

## Reproduction Steps

1. Cross-check `specs/031-live-stack-testing/scenario-manifest.json` against the actual functions defined in each referenced file.
2. Observe that SCN-LST-005 `linkedTests` claims `test_integration` exists in `scripts/runtime/go-integration.sh` but `grep -nE "^(function )?test_integration|^test_integration\(\)" scripts/runtime/go-integration.sh` returns zero matches.
3. Cross-check `specs/031-live-stack-testing/state.json` `activeBugs` and `resolvedBugs` against `specs/031-live-stack-testing/bugs/BUG-031-006-strict-guard-gate-drift/state.json` status fields.
4. Observe parent has BUG-031-006 in `activeBugs` while the BUG itself is `done`/`done`.
5. Inspect `tests/stress/ml_readiness_timeout_stress_test.go` and confirm the three test functions exist and were recorded in `report.md` as added by BUG-031-006, yet have no scenario-manifest linkage.

## Expected Behavior

`specs/031-live-stack-testing/scenario-manifest.json` must reference only functions that exist on disk. `specs/031-live-stack-testing/state.json` `activeBugs`/`resolvedBugs` must reflect the actual certification status of every BUG packet under `specs/031-live-stack-testing/bugs/`. Every SLA-class test added by a BUG-031-006-shaped closure must have a scenario entry that links the implementation back to the planning artifacts.

## Actual Behavior

- SCN-LST-005 `linkedTests` includes a phantom function reference that breaks scenario-manifest fidelity audits.
- Parent state.json activeBugs/resolvedBugs is stale (round 3 closure recorded the move in narrative but not in fields).
- New SLA stress test surface has zero scenario coverage in the manifest even though it is the GREEN proof for the Scope 6 ML readiness gate.

## Environment

- Service: spec 031 artifact-fidelity governance (scenario-manifest + parent state.json bookkeeping)
- Parent owner: `specs/031-live-stack-testing/`
- Trigger: stochastic-quality-sweep `sweep-2026-05-23-r30` round 8 (`mode: test-to-doc`)
- Platform: parent-expanded child workflow under the sweep parent

## Error Output

```text
$ python3 -c "
import json, re, os
m = json.load(open('specs/031-live-stack-testing/scenario-manifest.json'))
missing = []
for s in m['scenarios']:
    for lt in s.get('linkedTests', []):
        f = lt.get('file', '')
        fn = lt.get('function', '')
        if not f or not fn: continue
        if not os.path.exists(f):
            missing.append(('FILE-MISSING', s['scenarioId'], f, fn))
            continue
        content = open(f).read()
        pat = re.compile(r'^func\\s+' + re.escape(fn) + r'\\b', re.MULTILINE)
        if not pat.search(content):
            if f.endswith('.sh'):
                pat2 = re.compile(r'^(function\\s+)?' + re.escape(fn) + r'\\s*\\(\\s*\\)', re.MULTILINE)
                if pat2.search(content): continue
            missing.append(('FUNC-MISSING', s['scenarioId'], f, fn))
print(f'Total scenarios: {len(m[\"scenarios\"])}'); print(f'Findings: {len(missing)}')
for x in missing: print(' -', x)
"
Total scenarios: 12
Findings: 1
 - ('FUNC-MISSING', 'SCN-LST-005', 'scripts/runtime/go-integration.sh', 'test_integration')
```

```text
$ python3 -c "
import json
b = json.load(open('specs/031-live-stack-testing/bugs/BUG-031-006-strict-guard-gate-drift/state.json'))
p = json.load(open('specs/031-live-stack-testing/state.json'))
print('BUG status:', b.get('status'), 'cert:', b.get('certification',{}).get('status'))
print('Parent activeBugs:', p.get('activeBugs'))
print('Parent resolvedBugs:', p.get('resolvedBugs'))
"
BUG status: done cert: done
Parent activeBugs: ['BUG-031-006-strict-guard-gate-drift']
Parent resolvedBugs: []
```

```text
$ ls tests/stress/ml_readiness_timeout_stress_test.go
tests/stress/ml_readiness_timeout_stress_test.go
$ grep -nE "^func Test" tests/stress/ml_readiness_timeout_stress_test.go
17:func TestMLReadinessTimeoutBoundary(t *testing.T) {
164:func TestMLReadinessTimeoutSilentBypass(t *testing.T) {
241:func TestMLReadinessAlways200Regression(t *testing.T) {
$ grep -nE "ml_readiness_timeout_stress|MLReadinessTimeoutBoundary" specs/031-live-stack-testing/scenario-manifest.json
(no matches)
```

## Root Cause

R3 closure for BUG-031-006 was focused on phase-provenance + DoD-checkbox + structured-commit landing under heavy state-transition-guard pressure. Three orthogonal artifact-fidelity items fell outside the explicit R3 finding ledger:

- **T-031-001** is a pre-existing manifest fidelity defect that predates BUG-031-006 (the SCN-LST-005 entry has always referenced a function that the script never had).
- **T-031-002** is an R3 closure regression — the narrative move in the validate executionHistory entry was not paired with the actual `json.dump` of the field write.
- **T-031-003** is an R3 coverage gap — the new SLA stress test surface was added by R3 (and recorded in `report.md` `## Code Diff Evidence`) but the planning artifact (scenario-manifest.json) was not extended to link the new tests.

## Fix

- **T-031-001:** Remove the phantom `function: test_integration` entry from SCN-LST-005 `linkedTests`. The remaining two entries (`testPool`, `testJetStream` in `tests/integration/helpers_test.go`) faithfully prove the scenario; the integration script entry-point is recorded in `evidenceRefs` already.
- **T-031-002:** Move `BUG-031-006-strict-guard-gate-drift` from `activeBugs` to `resolvedBugs` in `specs/031-live-stack-testing/state.json`. Bump `lastUpdatedAt`.
- **T-031-003:** Extend `SCN-LST-004` (`Search works after cold start`) `linkedTests` with the three new SLA stress test function entries and add a stress-test `evidenceRef`. This keeps the scope-6 ML readiness gate as the single scenario anchor and avoids inventing a new scenario ID for an additive SLA-boundary probe of the same readiness contract.

## References

- Sweep envelope: `sweep-2026-05-23-r30` round 8, mappedMode `test-to-doc`
- Parent spec: `specs/031-live-stack-testing/spec.md`
- Sister bug (just closed in R3): `specs/031-live-stack-testing/bugs/BUG-031-006-strict-guard-gate-drift/`
- Test surface probed: `tests/integration/`, `tests/e2e/`, `tests/stress/`, `internal/api/`
- Compile sweep evidence: `go vet -tags="integration stress" ./...` EXIT=0, `go build -tags="integration stress" ./...` EXIT=0 (zero output, no warnings)
