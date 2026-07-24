# Report: BUG-002-006 Secure Progressive Search Submission

Links: [scopes.md](scopes.md) | [scenario-manifest.json](scenario-manifest.json) | [test-plan.json](test-plan.json) | [uservalidation.md](uservalidation.md)

## Summary

On 2026-07-23, `bubbles.plan` replaced the preliminary single-scope handoff with four dependency-ordered executable scopes: source-locked shared HTMX delivery, semantic baseline Search, exactly-once enhancement, and disposable real-browser acceptance. Shared-head canaries and atomic rollback precede Search browser proof.

No source, test, dependency, configuration, runtime, production data, requirements, design, certification, commit, push, or deployment mutation is claimed.

## Completion Statement

Planning-owned artifacts are complete for implementation routing only after packet-local artifact lint and traceability guard pass. The bug remains `in_progress`; no runtime repair is complete.

## Bug Reproduction - Before Fix

- **Claim Source:** interpreted historical input.
- **Executed by this invocation:** no.
- **Input preserved:** Search renders; the browser blocks HTMX for a wrong SHA-384 SRI value; entering a query sends zero `/search` requests.
- **Evidence status:** no browser console, network trace, DOM snapshot, or command output was captured here.

## Decision Record

- The design-owned same-origin, source-locked HTMX approach is implemented first because the shared head has high fan-out.
- `internal/web/handler_test.go`, `tests/e2e/recommendations_web_test.go`, and `tests/e2e/test_web_ui.sh` are existing read/mutation/server-rendered canary anchors; future tests are recorded separately as planned targets.
- The semantic native form and typed server model precede enhancement; enhancement observes lifecycle and cannot originate requests.
- The adversarial proof includes both a one-byte/stale-digest release failure and the exact pre-fix zero-request browser reproduction.
- Every runtime scope has five Test Plan rows and five matching test-evidence DoD items.
- Real Playwright uses the disposable stack and real session, with no request interception, auth injection, response stubbing, or bailout.
- No stress row is planned because this bug packet defines no latency, throughput, or availability SLA.

## Code Diff Evidence

Not applicable to this planning-only invocation.

## Test Evidence

No implementation or behavior test was run during planning, and no runtime result is claimed. The planned execution matrix is in [test-plan.json](test-plan.json).

## Planning Validation

### Artifact Lint

**Phase:** plan  
**Command:** `bash .github/bubbles/scripts/artifact-lint.sh specs/002-phase1-foundation/bugs/BUG-002-006-search-htmx-sri-blocks-submit`  
**Exit Code:** 0  
**Claim Source:** executed

```text
✅ Required artifact exists: spec.md
✅ Required artifact exists: design.md
✅ Required artifact exists: uservalidation.md
✅ Required artifact exists: state.json
✅ Required artifact exists: scopes.md
✅ Required artifact exists: report.md
✅ No forbidden sidecar artifacts present
✅ Found DoD section in scopes.md
✅ scopes.md DoD contains checkbox items
✅ All DoD bullet items use checkbox syntax in scopes.md
✅ Found Checklist section in uservalidation.md
✅ uservalidation checklist contains checkbox entries
✅ report.md contains required Summary, Completion Statement, and Test Evidence sections
✅ Anti-Fabrication Evidence Checks passed
Artifact lint PASSED.
```

### Traceability Guard

**Phase:** plan  
**Command:** `bash .github/bubbles/scripts/traceability-guard.sh specs/002-phase1-foundation/bugs/BUG-002-006-search-htmx-sri-blocks-submit`  
**Exit Code:** 0  
**Claim Source:** executed

```text
✅ scenario-manifest.json covers 8 scenario contract(s)
✅ All linked tests from scenario-manifest.json exist
✅ Scope 01 scenario maps to concrete test file: internal/web/handler_test.go
✅ Scope 02 scenarios map to concrete test file: internal/web/handler_test.go
✅ Scope 03 scenarios map to concrete test file: internal/web/handler_test.go
✅ Scope 04 scenario maps to concrete test file: internal/web/handler_test.go
✅ DoD fidelity: 8 scenarios checked, 8 mapped to DoD, 0 unmapped
ℹ️  Scenarios checked: 8
ℹ️  Test rows checked: 24
ℹ️  Scenario-to-row mappings: 8
ℹ️  Concrete test file references: 8
ℹ️  Report evidence references: 8
ℹ️  Edge confidence: declared=9 inferred=0 ambiguous=7
RESULT: PASSED (0 warnings)
```

## Uncertainty Declarations

- No before-fix browser/network execution exists; Scope 04 requires the exact strict-integrity zero-request reproduction before implementation.
- No after-fix behavior verification exists.
- The disposable degraded/timeout controls are implementation targets that harden must verify against existing test-stack seams.
- Planned test paths are executable handoff targets, not claims that those files already exist.

## Scenario Contract Evidence

The eight scenarios are assigned to Scopes 01-04 in [scenario-manifest.json](scenario-manifest.json). Existing canaries are linked; not-yet-authored targets use `plannedTests`; evidence references remain empty until execution.

### Existing Canary Anchors

The report references the concrete planning anchors used by traceability: `internal/web/handler_test.go`, `tests/e2e/recommendations_web_test.go`, `tests/e2e/test_web_ui.sh`, `tests/e2e/test_search_empty.sh`, `tests/e2e/auth/browser_login_test.go`, and `tests/e2e/capture_process_search_test.go`.

## Coverage Report

Planning covers source integrity/CSP, shared HTMX read/mutation compatibility, no-JavaScript submission, validation, results, empty, filtered-empty, unauthorized, timeout, network, server error, degraded results, request cardinality, retry, privacy, responsive layout, keyboard, screen reader, and reduced motion. No runtime coverage percentage is claimed.

## Lint/Quality

Only actual packet-local validator outcomes are recorded after execution.

## Spot-Check Recommendations

- Harden must verify the lock-derived HTMX asset path/digest and disposable failure controls against repository reality.
- Test must run anti-interception and bugfix regression-quality guards over all planned Playwright files.
- Validate must inspect shared-head rollback/restore and zero-residue disposable teardown.

## Validation Summary

Planning validation only. No state transition or certification is requested.

## Audit Verdict

Not audited. No terminal verdict is claimed.
