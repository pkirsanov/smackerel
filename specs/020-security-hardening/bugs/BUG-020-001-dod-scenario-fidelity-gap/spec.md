# Spec: BUG-020-001 — DoD Scenario Fidelity Gap (Spec 020)

> **Bug:** [bug.md](bug.md)
> **Parent:** [020 spec](../../spec.md) | [020 scopes](../../scopes.md) | [020 report](../../report.md)
> **Workflow Mode:** bugfix-fastlane
> **Status:** Fixed (artifact-only)

---

## Expected Behavior

Running the Bubbles traceability guard against `specs/020-security-hardening` MUST exit with `RESULT: PASSED` and report:

- `Scenarios checked: 18` and `Scenario-to-row mappings: 18`
- All 18 scenarios mapped to a Test Plan row whose first concrete file path exists on disk
- Every concrete test file referenced from `report.md`
- `DoD fidelity: 18 scenarios checked, 18 mapped to DoD, 0 unmapped`
- `scenario-manifest.json covers 18 scenario contract(s)` and `All linked tests from scenario-manifest.json exist`

## Actors & Personas

| Actor | Goal |
|-------|------|
| Bubbles governance pipeline | Verify every Gherkin scenario in `scopes.md` is anchored to a DoD bullet (Gate G068) and to an existing test file referenced in `report.md` |
| Smackerel maintainer | Reach `RESULT: PASSED` for spec 020 without altering production code, since every flagged scenario is already implemented and exercised by passing tests |

## Acceptance Criteria

```gherkin
Scenario: SCN-020-FIX-001 Trace guard accepts SCN-020-001/002/006/013/014 as faithfully covered
  Given specs/020-security-hardening/scopes.md DoD entries that name each Gherkin scenario by trace ID
  And specs/020-security-hardening/scenario-manifest.json mapping all 18 SCN-020-* scenarios with linkedTests and evidenceRefs
  And specs/020-security-hardening/report.md referencing internal/config/docker_security_test.go, internal/auth/oauth_test.go, ml/tests/test_auth.py, and cmd/core/main_test.go
  When the workflow runs `bash .github/bubbles/scripts/traceability-guard.sh specs/020-security-hardening`
  Then Gate G068 reports "18 scenarios checked, 18 mapped to DoD, 0 unmapped"
  And the overall result is PASSED
```

## Boundary

This is an artifact-only fix. No file under `internal/`, `cmd/`, `ml/`, `config/`, `tests/`, or any other production-code path may be modified. Every flagged behavior is already delivered and tested.
