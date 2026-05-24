# Bug: BUG-026-004 Reconcile artifact drift to current gate standards (G022 / G040 / G053 / G068 / regression-E2E planning)

## Summary

Sweep round 20 of `sweep-2026-05-23-r30` (`mode: reconcile-to-doc`) validate-first reconciliation pass on `specs/026-domain-extraction/` surfaced 47 artifact-drift BLOCKS in `bash .github/bubbles/scripts/state-transition-guard.sh specs/026-domain-extraction` that survived the original `done` certification on 2026-04-24. Implementation code, tests, and runtime behavior are correct (verified across rounds 10/19/etc. of the same sweep and by `bubbles.spec-review` / `bubbles.audit` / `bubbles.chaos` / `bubbles.harden` probes). The drift is exclusively in the spec/scope/state artifacts because they were authored before the current gate standards were tightened.

The 47 BLOCKS fall into 5 categories:

1. **Gate G022 — missing required specialist phase records (4 BLOCKS):** `state.json` `certification.certifiedCompletedPhases` lacks `regression`, `simplify`, `stabilize`, `security` even though each probe ran in past sweep rounds (Security Probe 2026-04-20, Gaps Analysis 2026-04-21, Simplification Probe 2026-04-22, Hardening Probe 2026-05-13, Regression closure for BUG-026-003 on 2026-05-12). One additional rollup BLOCK ("4 specialist phase(s) missing — work was NOT executed through the full pipeline") is the same finding.
2. **Gate G022 extension — phase-claim provenance impersonation (3 BLOCKS):** `completedPhaseClaims` contains `bootstrap`, `test`, `validate` but `executionHistory` has no `bubbles.bootstrap:bootstrap`, `bubbles.test:test`, or `bubbles.validate:validate` entry — the original claims were collapsed under `bubbles.plan` (for `bootstrap`) and `bubbles.workflow` (for `test`/`validate`). One additional rollup BLOCK ("3 phase claim(s) lack proper agent provenance") is the same finding.
3. **Gate G053 — missing `### Code Diff Evidence` section (1 BLOCK):** `report.md` does not have the section header that implementation-bearing workflows must emit.
4. **Gate G040 — deferral language hits in `report.md` (1 BLOCK, 3 underlying matches):** Lines 56 and 95 use the literal word "placeholders" inside SQL/parameterized-query prose (false positive — the regex matches the substring without word boundary), and line 208 uses "are deferred to live-stack testing (spec 031)" (real deferral language).
5. **Check 8A regression E2E planning (27 BLOCKS = 9 scopes × 3 requirements):** Each scope in `scopes.md` lacks (a) DoD bullet `- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior`, (b) DoD bullet `- [x] Broader E2E regression suite passes`, and (c) Test Plan row containing `Regression E2E`. One additional rollup BLOCK ("27 regression E2E planning requirement(s) missing") is the same finding.
6. **Gate G068 — DoD-Gherkin content fidelity gaps (6 BLOCKS):** Six Gherkin scenarios across Scopes 4/5/7/8/9 have no DoD bullet that quotes the scenario name in `Scenario "<name>":` form. One additional rollup BLOCK ("6 Gherkin scenario(s) have no matching DoD item") is the same finding.
7. **Structured commit prefix (1 BLOCK):** No commit in `git log` matches `spec(026)` or `bubbles(026/...)`. This bug's close-out commit (`spec(026,bug-026-004): ...`) will satisfy the gate.

Total: 47 BLOCKS. No closed BUG-026-001/002/003 covers any of these residuals.

## Severity

- [ ] Critical - System unusable, data loss
- [ ] High - Major feature broken
- [x] Medium - Artifact-quality drift makes `bash .github/bubbles/scripts/state-transition-guard.sh specs/026-domain-extraction` exit with `🔴 TRANSITION BLOCKED: 47 failure(s), 2 warning(s)`. Spec 026 cannot legitimately re-promote to `done` under current gate standards even though its runtime code is correct. Sweep round 20 cannot reach `done` status without resolving this drift.
- [ ] Low - Minor issue, cosmetic

## Status

- [x] Reported
- [x] Confirmed by sweep round 20 validate-first reconciliation probe
- [ ] In Progress
- [ ] Fixed
- [ ] Verified
- [ ] Closed

## Reproduction Steps

1. From clean HEAD `1587df4d` (sweep round 19 commit), run `bash .github/bubbles/scripts/state-transition-guard.sh specs/026-domain-extraction 2>&1 | grep -E "BLOCK|RESULT"`.
2. Observe 47 `🔴 BLOCK` lines and `🔴 TRANSITION BLOCKED: 47 failure(s), 2 warning(s)`.
3. Run `bash .github/bubbles/scripts/traceability-guard.sh specs/026-domain-extraction 2>&1 | tail -5`.
4. Observe `RESULT: FAILED (7 failures, 0 warnings)` — 6 G068 fidelity failures plus rollup.
5. Run `bash .github/bubbles/scripts/artifact-lint.sh specs/026-domain-extraction 2>&1 | tail -3`.
6. Observe `Artifact lint PASSED.` (artifact-lint accepts the legacy scopeLayout and does not enforce regression-E2E planning).

## Expected Behavior

- `bash .github/bubbles/scripts/state-transition-guard.sh specs/026-domain-extraction` exits 0 with `🟢 TRANSITION ALLOWED` (or equivalent green verdict).
- `bash .github/bubbles/scripts/traceability-guard.sh specs/026-domain-extraction` exits 0 with `RESULT: PASSED`.
- `bash .github/bubbles/scripts/artifact-lint.sh specs/026-domain-extraction` continues to exit 0.
- Spec 026 stays `status: done`. No runtime code, schema, NATS topology, config value, web template, or Telegram command is changed.

## Actual Behavior

- `state-transition-guard.sh` exits non-zero with 47 BLOCKS distributed across Check 6 (G022 phases), Check 6B (G022 provenance), Check 8A (regression E2E planning), Check 13 (G053 Code Diff Evidence), Check 17 (structured commit prefix), Check 18 (G040 deferral language), and Check 22 (G068 DoD-Gherkin fidelity).
- `traceability-guard.sh` exits non-zero with 6 G068 fidelity failures plus rollup.

## Environment

- Branch: `main`, HEAD `1587df4d`
- Sweep: `sweep-2026-05-23-r30` round 20, mode `reconcile-to-doc`, executionModel `parent-expanded-child-mode`
- Parent feature: `specs/026-domain-extraction` (currently `status: done` per pre-G022-strict certification)
- State guard version: as of HEAD `1587df4d`
- Test runner: `./smackerel.sh test unit` baseline green; Go and Python domain-extraction unit suites all pass; `tests/e2e/domain_e2e_test.go::TestE2E_DomainExtraction` validated by sweep rounds 10 and 19

## Error Output

```text
$ bash .github/bubbles/scripts/state-transition-guard.sh specs/026-domain-extraction 2>&1 | grep -E "🔴 BLOCK|🔴 TRANSITION" | head -10
🔴 BLOCK: Required phase 'regression' NOT in execution/certification phase records (Gate G022 violation)
🔴 BLOCK: Required phase 'simplify' NOT in execution/certification phase records (Gate G022 violation)
🔴 BLOCK: Required phase 'stabilize' NOT in execution/certification phase records (Gate G022 violation)
🔴 BLOCK: Required phase 'security' NOT in execution/certification phase records (Gate G022 violation)
🔴 BLOCK: 4 specialist phase(s) missing — work was NOT executed through the full pipeline
🔴 BLOCK: Phase 'test' is in completedPhaseClaims but no executionHistory entry from bubbles.test — possible impersonation (Gate G022)
🔴 BLOCK: Phase 'bootstrap' is in completedPhaseClaims but no executionHistory entry from bubbles.bootstrap — possible impersonation (Gate G022)
🔴 BLOCK: Phase 'validate' is in completedPhaseClaims but no executionHistory entry from bubbles.validate — possible impersonation (Gate G022)
🔴 BLOCK: 3 phase claim(s) lack proper agent provenance — phase impersonation detected
🔴 BLOCK: Scope is missing DoD item for scenario-specific regression E2E coverage: Scope 1: DB Migration & Domain Data Types
```

```text
$ bash .github/bubbles/scripts/traceability-guard.sh specs/026-domain-extraction 2>&1 | grep -E "❌ Scope|RESULT"
❌ Scope 4: ML Sidecar Domain Extraction Handler Gherkin scenario has no faithful DoD item preserving its behavioral claim: ML sidecar builds domain extraction prompt from contract and artifact
❌ Scope 5: Recipe Extraction Prompt Contract Gherkin scenario has no faithful DoD item preserving its behavioral claim: Recipe prompt contract loads and validates (BS-007 partial)
❌ Scope 7: Pipeline Integration Gherkin scenario has no faithful DoD item preserving its behavioral claim: Domain extraction is skipped for non-matching artifact (BS-004)
❌ Scope 8: Search Extension Gherkin scenario has no faithful DoD item preserving its behavioral claim: Search detects product price intent (BS-002 partial)
❌ Scope 9: Telegram Display Gherkin scenario has no faithful DoD item preserving its behavioral claim: Recipe artifact renders recipe card in Telegram (BS-001 display)
❌ Scope 9: Telegram Display Gherkin scenario has no faithful DoD item preserving its behavioral claim: Product artifact renders product card in Telegram (BS-002 display)
RESULT: FAILED (7 failures, 0 warnings)
```

```text
$ awk 'BEGIN{in_fence=0} /^```/{in_fence=1-in_fence; next} {if(!in_fence) print NR": "$0}' specs/026-domain-extraction/report.md \
    | grep -iE 'deferred|defer to|placeholder' \
    | grep -viE 'no deferred items|followUpOwner|followUpAction|follow-up section'
56: | A03 Injection (SQL) | Clean | All DB queries use parameterized placeholders (`$N`). Domain filters ...
95: - All SQL queries remain parameterized (`$N` placeholders with args arrays)
208: ... Full integration tests are deferred to live-stack testing (spec 031).
```

## Workaround

None — sweep round 20 cannot proceed past `validate-first` reconciliation without resolving this drift. Spec 026 stays functionally `done`, but artifact-level promotion under current gate standards is blocked.

## Root Cause Analysis (Five Whys)

- **Why did 47 BLOCKS appear?** Because the state-transition-guard, the traceability-guard, and the regression-E2E planning check have all tightened standards since spec 026 was originally certified `done` on 2026-04-24.
- **Why didn't earlier sweep rounds catch it?** Rounds 10 and 19 ran `regression-to-doc`, `harden-to-doc`, and `test-to-doc` trigger probes that found and closed runtime gaps (BUG-026-003 coverage, HARDEN-026-1/2 invariants) but did not run the validate-first reconciliation that catches artifact-quality drift.
- **Why are the regression-E2E DoD bullets and Test Plan rows missing?** Scope 026 was authored before Gate G028's regression-E2E planning requirement was added to state-transition-guard Check 8A.
- **Why are the G068 fidelity gaps present?** Six Gherkin scenarios were paraphrased into DoD bullets without quoting the scenario name in the `Scenario "<name>":` prefix form that G068 requires.
- **Why is `bootstrap`/`test`/`validate` provenance missing?** Original `bubbles.plan` and `bubbles.workflow` entries claimed multiple phases under a single agent identity; Gate G022 extension was tightened later to require `bubbles.<phase>:<phase>` strict provenance.
- **Why are `regression`/`simplify`/`stabilize`/`security` phases missing from `certifiedCompletedPhases`?** The Security Probe, Gaps Analysis, Simplification Probe, and Hardening Probe sections were added to `report.md` during stochastic-quality-sweep rounds, but `certification.certifiedCompletedPhases` was not updated each time.

## Related

- Parent: `specs/026-domain-extraction/`
- Sibling bugs: `specs/026-domain-extraction/bugs/BUG-026-001-dod-scenario-fidelity-gap/` (closed; addressed an earlier subset of fidelity gaps); `specs/026-domain-extraction/bugs/BUG-026-002-domain-e2e-status-timeout/` (closed); `specs/026-domain-extraction/bugs/BUG-026-003-handle-domain-extracted-uncovered/` (closed; added unit coverage for `handleDomainExtracted`)
- Sweep ledger entry: `.specify/memory/sweep-2026-05-23-r30.json` round 20
- Reference pattern: `specs/025-knowledge-synthesis-layer/bugs/BUG-025-004-test-trigger-probe-residuals/` (analogous artifact-quality reconciliation pattern closed in round 19)
