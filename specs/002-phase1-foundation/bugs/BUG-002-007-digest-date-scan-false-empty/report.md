# Report: [BUG-002-007] Digest Date Scan Produces False Empty

Links: [scopes.md](scopes.md) | [scenario-manifest.json](scenario-manifest.json) | [uservalidation.md](uservalidation.md)

## Summary

On 2026-07-23, `bubbles.plan` replaced the preliminary single-scope handoff with four dependency-ordered executable scopes: the operator-owned global-corpus grant + freshness contract gate, the canonical typed reader, truthful server-rendered states, and disposable real-PostgreSQL browser acceptance. On 2026-07-24 a planning reconciliation re-derived the plan against the revised `spec.md`/`design.md`: the digest authorization contradiction is resolved to one operator-owned global corpus with `digest:read`/`digest:generate` grants (no tenant/per-user row isolation), `scenario-manifest.json` now covers all ten scopes-defined scenarios with existing linked-test anchors, and this report records the concrete canary anchors.

No source, test, database, browser, production, requirements, design, certification, commit, push, or deployment mutation occurred.

## Completion Statement

Incomplete and non-terminal. Status remains `in_progress`. Implementation routing is permitted only after packet-local artifact lint and traceability guard pass; owner reproduction, implementation, testing, validation, and audit have not occurred.

## Bug Reproduction - Before Fix

- **Claim Source:** interpreted historical input.
- **Executed by this invocation:** no.
- **Input preserved:** the database has a current approximately 380-word digest, but the legacy page shows "No digest generated yet" after a date-to-string scan error is silently replaced with an empty model.
- **Evidence status:** no SQL, server log, browser, or command output was captured here.

## Decision Record

- Digest ownership is the single operator-owned global corpus. `digests` has no ownership column; authorization is grant-gated (`digest:read` reader, `digest:generate` producer) at the capability boundary, never a per-user row predicate. This resolves the earlier product-session-versus-per-user contradiction and is now consistently named across `spec.md`, `design.md`, `scopes.md`, and `scenario-manifest.json`.
- Stale age is an explicit required SST value (no hidden default); missing or invalid freshness config fails startup.
- Only wrapped `pgx.ErrNoRows` maps to an empty state; every other query/scan/decode/connection fault stays a typed error and never a false empty.
- True-empty, selected-date-miss, quiet, stale/degraded, unauthorized, and read-error remain distinct, individually testable states.
- The stored row and scan failure are treated as operator-supplied findings, not locally executed proof.
- Real PostgreSQL round-trip coverage is mandatory because template-only tests cannot catch scan-type drift; no database mock, response interception, auth injection, or bailout may satisfy live rows.

## Code Diff Evidence

Not applicable to this planning-only invocation.

## Test Evidence

**Phase:** planning  
**Command:** none  
**Exit Code:** not applicable  
**Claim Source:** not-run

No test result is claimed.

## Uncertainty Declarations

- Exact SQL query, date type, nullable behavior, and error-swallowing branch are not locally confirmed.
- No red or green regression output exists.

## Scenario Contract Evidence

The ten scenarios are assigned to Scopes 01-04 in [scenario-manifest.json](scenario-manifest.json): Scope 01 (SCN-002-007-09, 10), Scope 02 (SCN-002-007-01, 02, 03), Scope 03 (SCN-002-007-04, 05, 06, 07), Scope 04 (SCN-002-007-08). Existing canaries are linked; not-yet-authored targets use `plannedTests`; evidence references remain empty until execution.

### Existing Canary Anchors

The report references the concrete planning anchors used by traceability: `internal/config/validate_test.go`, `internal/digest/generator_test.go`, `internal/api/digest.go`, `internal/web/handler_test.go`, `tests/integration/guesthost_digest_test.go`, `tests/e2e/test_digest.sh`, `tests/e2e/test_digest_quiet.sh`, `tests/e2e/test_digest_pipeline.sh`, `tests/e2e/test_web_ui.sh`, `web/pwa/tests/unified_journey.spec.ts`, and `web/pwa/tests/auth_login.spec.ts`.

## Planning Validation

Packet-local validators were re-run during this reconciliation and both pass.

### Artifact Lint

**Command:** `bash .github/bubbles/scripts/artifact-lint.sh specs/002-phase1-foundation/bugs/BUG-002-007-digest-date-scan-false-empty`  
**Exit Code:** 0  
**Claim Source:** executed  
**Result:** `Artifact lint PASSED.`

### Traceability Guard

**Command:** `bash .github/bubbles/scripts/traceability-guard.sh specs/002-phase1-foundation/bugs/BUG-002-007-digest-date-scan-false-empty`  
**Exit Code:** 0  
**Claim Source:** executed  
**Result:** `RESULT: PASSED (0 warnings)` — 10 scenarios checked, 10 mapped to DoD, 22 test rows, 10 report evidence references, and all linked tests exist (baseline was 17 failures).

## Validation Summary

No completion certification was performed. This packet remains planning-only and `in_progress`; implementation routing awaits clean packet-local validators.

## Audit Verdict

Not audited. No terminal verdict is claimed.
