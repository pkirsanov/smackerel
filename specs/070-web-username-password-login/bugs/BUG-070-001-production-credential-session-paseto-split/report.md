# Report: BUG-070-001 Unified Production Browser Session

Links: [scopes.md](scopes.md) | [scenario-manifest.json](scenario-manifest.json) | [test-plan.json](test-plan.json) | [uservalidation.md](uservalidation.md)

## Summary

On 2026-07-23, `bubbles.plan` replaced the preliminary single-scope handoff with dependency-ordered executable scopes. On 2026-07-24, `bubbles.plan` reconciled the planning packet to the analyst-updated `spec.md` (which had added SCN-070-001-09 CSRF/Origin, SCN-070-001-10 roles, and SCN-070-001-11 global-corpus grant-gating after the earlier planning artifacts were authored). The reconciled plan now has six dependency-ordered executable scopes: (01) account binding and role/grant model, (02) purpose-bound token lifecycle, (03) unified request authentication, (04) login/logout UX, (05) product-wide CSRF/Origin mutation protection, and (06) role/grant acceptance and global-corpus gating on the disposable production stack. The plan preserves the required production-session-before-Assistant ordering and defines protected-auth canaries plus a non-destructive fail-closed rollback.

No source, test, configuration, runtime, deployment, production data, business requirement, design, certification, commit, or push mutation is claimed.

### Reconciliation (2026-07-24)

The stale `SCN-070-001-09` middleware-parity mislabel was corrected to the spec's CSRF/Origin scenario across `scopes.md`, `scenario-manifest.json`, and `test-plan.json`. Planning was split from five to six scopes: a new SCOPE-05 owns product-wide CSRF/Origin (SCN-09); SCOPE-06 is the former disposable-acceptance scope, renumbered and expanded to own SCN-01 plus the newly added SCN-10 (roles) and SCN-11 (global corpus). SCOPE-03 rows were re-pointed to SCN-06/07, and the AUTH-015 single-authoritative-session-contract was made an explicit SCOPE-03 Core Outcome, resolving the auth predecessor-amendment consistency finding so dependents (for example BUG-073-006) can bind to one authoritative session contract. Test-Plan-row/DoD-test-item parity (5 rows / 5 items per scope; 30 rows total) and `certification.scopeProgress` (6 scopes) were restored.

## Completion Statement

Planning-owned artifacts are complete for implementation routing only after both packet-local planning validators pass. The bug remains `in_progress`; no runtime behavior or repair is complete.

## Bug Reproduction - Before Fix

- **Claim Source:** interpreted historical input.
- **Executed by this invocation:** no.
- **Input preserved:** On `<deploy-host>` production, username/password login succeeds and legacy server pages render, but the cookie contains or represents the shared runtime token while production shared-token fallback is false; modern `/api` and `/v1` PASETO middleware rejects it as malformed, leaving Assistant, Connectors, Photos, and model picker/admin unusable after accepted login. The concrete target name is normalized per the product deployment boundary.
- **Evidence status:** no command, browser, HTTP, database, or host output was captured by this invocation.

## Decision Record

- The reported behavior remains interpreted operator input, not locally reproduced evidence.
- The final plan follows the design-owned architecture without changing it: explicit principal/grant binding, purpose-bound browser/API PASETOs, one request authenticator, revocation-backed logout, and real-cookie browser acceptance.
- Account and session foundations precede middleware and UX consumers; full product acceptance follows all focused canaries.
- Every runtime scope has five concrete Test Plan rows and five matching test-evidence DoD items.
- Real Playwright uses the disposable stack, real login form, and browser cookie jar with no request interception, auth injection, storage-state token injection, or bailout return.
- No stress row is planned because this bug packet defines no latency, throughput, or availability SLA.
- Smackerel declares only the `core.health` trace workflow; no auth row is mislabeled with an unrelated observability workflow.

## Code Diff Evidence

Not applicable. This invocation is restricted to planning artifacts under this bug directory.

## Test Evidence

No implementation or behavior test was run during planning, and no runtime pass/fail result is claimed. The planned execution matrix is in [test-plan.json](test-plan.json).

## Planning Validation

Guards re-run during the 2026-07-24 planning reconciliation (six-scope plan; SCN-070-001-09 CSRF/Origin, SCN-070-001-10 roles, SCN-070-001-11 global corpus). Both passed with exit 0.

### Artifact Lint (reconciliation run)

**Phase:** harden (planning reconciliation)  
**Command:** `bash .github/bubbles/scripts/artifact-lint.sh specs/070-web-username-password-login/bugs/BUG-070-001-production-credential-session-paseto-split`  
**Exit Code:** 0  
**Claim Source:** executed 2026-07-24 in this session

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
✅ uservalidation checklist has checked-by-default entries
✅ All checklist bullet items use checkbox syntax
✅ Detected state.json status: in_progress
✅ Detected state.json workflowMode: bugfix-fastlane
✅ Top-level status matches certification.status
✅ report.md contains section matching: ###[[:space:]]+Summary|^##[[:space:]]+Summary
✅ report.md contains section matching: ###[[:space:]]+Completion Statement|^##[[:space:]]+Completion Statement
✅ report.md contains section matching: ###[[:space:]]+Test Evidence|^##[[:space:]]+Test Evidence
✅ Mode-specific report gates skipped (status not in promotion set)
✅ All checked DoD items in scopes.md have evidence blocks
✅ No unfilled evidence template placeholders in scopes.md
✅ No unfilled evidence template placeholders in report.md
✅ No repo-CLI bypass detected in report.md command evidence
Artifact lint PASSED.
ARTIFACT_LINT_EXIT=0
```

### Traceability Guard (reconciliation run)

**Phase:** harden (planning reconciliation)  
**Command:** `bash .github/bubbles/scripts/traceability-guard.sh specs/070-web-username-password-login/bugs/BUG-070-001-production-credential-session-paseto-split`  
**Exit Code:** 0  
**Claim Source:** executed 2026-07-24 in this session

```text
--- Scenario Manifest Cross-Check (G057/G059) ---
✅ scenario-manifest.json covers 11 scenario contract(s)
✅ All linked tests from scenario-manifest.json exist
✅ Scope 01 scenario maps to concrete test file: internal/auth/webcreds/repo_test.go
✅ Scope 02 scenarios map to concrete test file: internal/auth/issue_test.go
✅ Scope 03 scenarios map to concrete test file: internal/api/router_auth_middleware_test.go
✅ Scope 04 scenarios map to concrete test file: internal/api/web_login_credential_test.go
✅ Scope 05 scenario maps to concrete test file: internal/api/web_login_test.go
✅ Scope 06 scenarios map to concrete test file: internal/api/model_connections_operator_gate_test.go
--- Gherkin → DoD Content Fidelity (Gate G068) ---
ℹ️  DoD fidelity: 11 scenarios checked, 11 mapped to DoD, 0 unmapped
--- Traceability Summary ---
ℹ️  Scenarios checked: 11
ℹ️  Test rows checked: 36
ℹ️  Scenario-to-row mappings: 11
ℹ️  Concrete test file references: 11
ℹ️  Report evidence references: 11
ℹ️  DoD fidelity scenarios: 11 (mapped: 11, unmapped: 0)
ℹ️  Edge confidence (IMP-015 Scope B): declared=19 inferred=0 ambiguous=3
RESULT: PASSED (0 warnings)
TRACEABILITY_EXIT=0
```

### Residual Findings Routed From This Reconciliation

- **RF-070-001-01 (route: `bubbles.design`, adjudicate: `bubbles.analyst`):** `spec.md` AUTH-011 requires a server-validated **session-bound anti-CSRF proof** and states SameSite alone is insufficient, while `design.md` §Security relies on `SameSite Lax` + same-origin/CORS and states "No token is exposed for a custom CSRF scheme." SCOPE-05 is planned to the spec; the concrete session-bound-proof mechanism must be specified by design before implementation.
- **RF-070-001-02 (route: `bubbles.design`):** `design.md` §Testing And Validation Strategy enumerates only SCN-070-001-01..08; it omits the newly added SCN-070-001-09/10/11. The design testing table should be extended for internal consistency with the current spec.
- **Resolved in this reconciliation (auth predecessor-amendment consistency):** the AUTH-015 single authoritative production username/password-to-browser-session contract is now an explicit SCOPE-03 Core Outcome, so dependents (for example `BUG-073-006`) can bind to one authoritative auth-session contract.

## Uncertainty Declarations

- No before-fix red execution exists; Scope 05 requires the exact production-mode trust-split reproduction before implementation.
- No after-fix behavior verification exists.
- Existing production credential-to-principal and grant mappings require explicit owner-supplied values; the plan forbids inference.
- The planned source and test paths are executable targets for implementation and hardening, not claims that those files already exist.

## Scenario Contract Evidence

[scenario-manifest.json](scenario-manifest.json) assigns all eleven scope scenarios to persistent live regression targets. Evidence references remain empty until the tests execute; [test-plan.json](test-plan.json) is the machine-readable test handoff.

### Existing Canary Anchors

The following existing files are planning anchors, not delivery evidence: `internal/auth/webcreds/repo_test.go`, `internal/auth/issue_test.go`, `internal/api/router_auth_middleware_test.go`, `internal/api/web_login_credential_test.go`, `internal/api/web_login_test.go`, `internal/api/model_connections_operator_gate_test.go`, and `web/pwa/tests/auth_login.spec.ts`. Each final Test Plan row extends or preserves a real auth surface; not-yet-authored regression targets are recorded as `plannedTests` in the scenario manifest.

### Existing Canary Anchors

The following existing files are planning anchors, not delivery evidence: `internal/auth/webcreds/repo_test.go`, `internal/auth/issue_test.go`, `internal/api/router_auth_middleware_test.go`, `internal/api/web_login_credential_test.go`, `internal/api/web_login_test.go`, `internal/api/model_connections_operator_gate_test.go`, and `web/pwa/tests/auth_login.spec.ts`. Each final Test Plan row extends or preserves one of these real surfaces; not-yet-authored regression targets are recorded as `plannedTests` in the scenario manifest.

## Coverage Report

Planning coverage includes account/persistence, purpose-bound token lifecycle, middleware parity, login/recovery/logout UX, privacy, accessibility, and product-wide acceptance. Each scope includes unit, integration, e2e-api, focused e2e-ui, and broader e2e-ui rows. No runtime coverage percentage is claimed.

## Lint/Quality

The 2026-07-24 planning-reconciliation artifact lint and traceability guard both passed with exit 0 (see Planning Validation above). No runtime lint, build, or test is claimed.

## Spot-Check Recommendations

- Harden must confirm planned test paths and exact titles against the repository before implementation.
- Test must run anti-interception and bugfix regression-quality guards against every required Playwright file.
- Validate must inspect the disposable stack's teardown and the fail-closed auth rollback canary.

## Validation Summary

Planning validation only. No state transition or certification is requested.

## Audit Verdict

Not audited. No terminal verdict is claimed.
