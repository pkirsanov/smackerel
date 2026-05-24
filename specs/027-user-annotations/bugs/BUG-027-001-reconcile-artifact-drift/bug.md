# Bug: BUG-027-001 Reconcile artifact drift to current gate standards (Check 5A / G016 regression E2E / G022 / G053 / G068 / Check 8B consumer-trace)

## Summary

Sweep round 21 of `sweep-2026-05-23-r30` (`mode: improve-existing`) reconciliation probe on `specs/027-user-annotations/` surfaced 51 artifact-drift BLOCKS in `bash .github/bubbles/scripts/state-transition-guard.sh specs/027-user-annotations` that survived the original `done` certification on 2026-04-24 and the 2026-05-09 MIT-027-TRACE-001 closure. Implementation code, tests, and runtime behavior are correct (verified by the May 2026 spec 044 Scope 02/03/spec-level-finalize cross-spec closures, by `internal/annotation/`, `internal/api/`, `internal/telegram/`, `internal/intelligence/` unit suites, and by `tests/integration/auth_annotation_test.go` and `tests/integration/db_migration_test.go::TestMigrations_AnnotationsConstraints`). The drift is exclusively in the spec/scope/state artifacts because they were authored before the current gate standards were tightened.

The 51 BLOCKS fall into 7 categories:

1. **Gate G022 — missing required specialist phase records (4 + 1 rollup BLOCKS):** `state.json` `certification.certifiedCompletedPhases` lacks `regression`, `simplify`, `stabilize`, `security` even though each surface ran in past work (Simplification Pass 2026-04-21 in `report.md`, Improvement Pass 2026-04-21, Security Pass 2026-04-22, Reconciliation Pass 2026-04-22, and the spec 044 cross-spec security closures of 2026-05-10/11). One additional rollup BLOCK ("4 specialist phase(s) missing — work was NOT executed through the full pipeline") is the same finding.
2. **Gate G022 extension — phase-claim provenance impersonation (3 + 1 rollup BLOCKS):** `completedPhaseClaims` contains `bootstrap`, `test`, `validate` but `executionHistory` has no `bubbles.bootstrap:bootstrap`, `bubbles.test:test`, or `bubbles.validate:validate` entry — the original claims were collapsed under `bubbles.plan` (for `bootstrap`) and `bubbles.workflow` (for `test`/`validate`). One additional rollup BLOCK ("3 phase claim(s) lack proper agent provenance") is the same finding.
3. **Gate G053 — missing `### Code Diff Evidence` section (1 BLOCK):** `report.md` does not have the section header that implementation-bearing workflows must emit.
4. **Check 5A — SLA-sensitive scope missing explicit stress coverage (1 BLOCK):** `scopes.md` line 168 contains the substring `slo` inside `TestMigrations_ExtensionsLoaded`. Check 5A's case-insensitive plain-substring regex matches `slo` and demands a Stress test row. No real SLA / latency / throughput claim is made for spec 027 — this is a substring false-positive.
5. **Check 8A regression E2E planning (24 + 1 rollup BLOCKS = 8 scopes × 3 requirements):** Each scope in `scopes.md` lacks (a) DoD bullet `- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior`, (b) DoD bullet `- [x] Broader E2E regression suite passes`, and (c) Test Plan row containing `Regression E2E`. One additional rollup BLOCK ("24 regression E2E planning requirement(s) missing") is the same finding.
6. **Check 8B consumer-trace planning for rename/removal scope (3 + 1 rollup BLOCKS):** Scope 4 contains the `DELETE /api/artifacts/{id}/tags/{tag}` removal action ("removed=\"weeknight\""), triggering Check 8B's rename/removal heuristic. The scope lacks (a) a `Consumer Impact Sweep` section, (b) a DoD item containing `zero stale first-party references remain`, and (c) explicit enumeration of consumer surfaces (API client, navigation, redirect, stale-reference). One additional rollup BLOCK ("3 consumer-trace planning requirement(s) missing") is the same finding.
7. **Gate G068 — DoD-Gherkin content fidelity gaps (10 + 1 rollup BLOCKS):** Ten Gherkin scenarios across Scopes 2/4/5/6 have no DoD bullet that quotes the scenario name in `Scenario "<name>":` form (Parse tags only, Parse tag removal, Parse interaction only, Parse note only, GET annotation history, Record message-artifact mapping after capture confirmation, Reply-to annotation with rating, Reply-to annotation with tags, Disambiguation resolution by number, Annotation confirmation formatting). One additional rollup BLOCK ("10 Gherkin scenario(s) have no matching DoD item") is the same finding.

Total: 51 BLOCKS. No closed BUG-027-NNN covers any of these residuals (no prior bug folder exists under spec 027). Check 17 (structured commit prefix) PASSES — spec 027 has 19 existing commits matching `^spec\(027\)|^bubbles\(027/`.

## Severity

- [ ] Critical - System unusable, data loss
- [ ] High - Major feature broken
- [x] Medium - Artifact-quality drift makes `bash .github/bubbles/scripts/state-transition-guard.sh specs/027-user-annotations` exit with `🔴 TRANSITION BLOCKED: 51 failure(s)`. Spec 027 cannot legitimately re-promote to `done` under current gate standards even though its runtime code is correct. Sweep round 21 cannot reach `done` status without resolving this drift.
- [ ] Low - Minor issue, cosmetic

## Status

- [x] Reported
- [x] Confirmed by sweep round 21 improve-existing probe
- [ ] In Progress
- [ ] Fixed
- [ ] Verified
- [ ] Closed

## Reproduction Steps

1. From clean HEAD `012a9f9a` (sweep round 20 close-out commit lineage), run `bash .github/bubbles/scripts/state-transition-guard.sh specs/027-user-annotations 2>&1 | grep -cE "^🔴 BLOCK"`.
2. Observe 51 BLOCK lines.
3. Run `bash .github/bubbles/scripts/traceability-guard.sh specs/027-user-annotations 2>&1 | tail -5`.
4. Observe `RESULT: FAILED (11 failures, 0 warnings)` — 10 G068 fidelity failures plus rollup.
5. Run `bash .github/bubbles/scripts/artifact-lint.sh specs/027-user-annotations 2>&1 | tail -3`.
6. Observe `Artifact lint PASSED.` (artifact-lint accepts the legacy scopeLayout and does not enforce regression-E2E planning).

## Expected Behavior

- `bash .github/bubbles/scripts/state-transition-guard.sh specs/027-user-annotations` exits 0 with `🟢 TRANSITION ALLOWED` (or equivalent green verdict).
- `bash .github/bubbles/scripts/traceability-guard.sh specs/027-user-annotations` exits 0 with `RESULT: PASSED`.
- `bash .github/bubbles/scripts/artifact-lint.sh specs/027-user-annotations` continues to exit 0.
- Spec 027 stays `status: done`. No runtime code, schema, NATS topology, config value, web template, prompt contract, or Telegram command is changed.

## Actual Behavior

- `state-transition-guard.sh` exits non-zero with 51 BLOCKS distributed across Check 5A (SLA false-positive substring), Check 6 (G022 phases), Check 6B (G022 provenance), Check 8A (regression E2E planning), Check 8B (consumer trace for Scope 4 tag-delete), Check 13B (G053 Code Diff Evidence), and Check 22 (G068 DoD-Gherkin fidelity).
- `traceability-guard.sh` exits non-zero with 10 G068 fidelity failures plus rollup.

## Environment

- Branch: `main`, HEAD `012a9f9a`
- Sweep: `sweep-2026-05-23-r30` round 21, mode `improve-existing`, executionModel `parent-expanded-child-mode`
- Parent feature: `specs/027-user-annotations` (currently `status: done` per pre-G022-strict certification + 2026-05-09 MIT-027-TRACE-001 closure + 2026-05-10/11 spec 044 cross-spec security closures)
- State guard version: as of HEAD `012a9f9a`
- Test runner: `./smackerel.sh test unit` baseline green; Go and Python annotation unit suites all pass; `tests/integration/auth_annotation_test.go::{TestAnnotation_BodyActorSourceInProduction_Rejected, TestAnnotation_BodyActorIDInProduction_Rejected}` and `tests/integration/db_migration_test.go::TestMigrations_AnnotationsConstraints` confirm the live behavior

## Error Output

```text
$ bash .github/bubbles/scripts/state-transition-guard.sh specs/027-user-annotations 2>&1 | grep -cE "^🔴 BLOCK"
51
$ bash .github/bubbles/scripts/state-transition-guard.sh specs/027-user-annotations 2>&1 | grep -E "^🔴 BLOCK" | head -20
🔴 BLOCK: DoD-Gherkin content fidelity gap in Scope 2: Annotation Types & Parser — scenario has no faithful DoD item: Parse tags only
🔴 BLOCK: DoD-Gherkin content fidelity gap in Scope 2: Annotation Types & Parser — scenario has no faithful DoD item: Parse tag removal
🔴 BLOCK: DoD-Gherkin content fidelity gap in Scope 2: Annotation Types & Parser — scenario has no faithful DoD item: Parse interaction only
🔴 BLOCK: DoD-Gherkin content fidelity gap in Scope 2: Annotation Types & Parser — scenario has no faithful DoD item: Parse note only
🔴 BLOCK: DoD-Gherkin content fidelity gap in Scope 4: REST API Endpoints — scenario has no faithful DoD item: GET annotation history
🔴 BLOCK: DoD-Gherkin content fidelity gap in Scope 5: Telegram Message-Artifact Mapping — scenario has no faithful DoD item: Record message-artifact mapping after capture confirmation
🔴 BLOCK: DoD-Gherkin content fidelity gap in Scope 6: Telegram Annotation Handler — scenario has no faithful DoD item: Reply-to annotation with rating
🔴 BLOCK: DoD-Gherkin content fidelity gap in Scope 6: Telegram Annotation Handler — scenario has no faithful DoD item: Reply-to annotation with tags
🔴 BLOCK: DoD-Gherkin content fidelity gap in Scope 6: Telegram Annotation Handler — scenario has no faithful DoD item: Disambiguation resolution by number
🔴 BLOCK: DoD-Gherkin content fidelity gap in Scope 6: Telegram Annotation Handler — scenario has no faithful DoD item: Annotation confirmation formatting
🔴 BLOCK: 10 Gherkin scenario(s) have no matching DoD item — DoD may have been rewritten to match delivery instead of spec (Gate G068)
🔴 BLOCK: SLA-sensitive scope is missing explicit stress coverage: scopes.md
🔴 BLOCK: Required phase 'regression' NOT in execution/certification phase records (Gate G022 violation)
🔴 BLOCK: Required phase 'simplify' NOT in execution/certification phase records (Gate G022 violation)
🔴 BLOCK: Required phase 'stabilize' NOT in execution/certification phase records (Gate G022 violation)
🔴 BLOCK: Required phase 'security' NOT in execution/certification phase records (Gate G022 violation)
🔴 BLOCK: 4 specialist phase(s) missing — work was NOT executed through the full pipeline
🔴 BLOCK: Phase 'validate' is in completedPhaseClaims but no executionHistory entry from bubbles.validate — possible impersonation (Gate G022)
🔴 BLOCK: Phase 'bootstrap' is in completedPhaseClaims but no executionHistory entry from bubbles.bootstrap — possible impersonation (Gate G022)
🔴 BLOCK: Phase 'test' is in completedPhaseClaims but no executionHistory entry from bubbles.test — possible impersonation (Gate G022)
```

```text
$ bash .github/bubbles/scripts/traceability-guard.sh specs/027-user-annotations 2>&1 | tail -4
ℹ️  DoD fidelity scenarios: 70 (mapped: 60, unmapped: 10)
❌ FAIL: 10 G068 scenario DoD bullets missing the literal `Scenario "<exact-name>": ` prefix.
RESULT: FAILED (11 failures, 0 warnings)
$ echo "Exit Code: $?"
Exit Code: 1
```

## Workaround

None — sweep round 21 cannot proceed past the `improve-existing` reconciliation pass without resolving this drift. Spec 027 stays functionally `done`, but artifact-level promotion under current gate standards is blocked.

## Root Cause Analysis (Five Whys)

- **Why did 51 BLOCKS appear?** Because the state-transition-guard, the traceability-guard, the Check 8A regression-E2E planning check, and Check 8B consumer-trace check have all tightened standards since spec 027 was originally certified `done` on 2026-04-24.
- **Why didn't the May 2026 MIT-027-TRACE-001 closure and the spec 044 cross-spec security closures catch it?** Those closures targeted (a) G068 trace prefixes for the original 33-scenario subset, (b) `actor_source`/`actor_id` body-defense at the API entry, and (c) Telegram + NATS bridge coverage. None of them ran the validate-first reconciliation that catches artifact-quality drift across Check 5A/6/6B/8A/8B/13B/22.
- **Why are the regression-E2E DoD bullets and Test Plan rows missing?** Spec 027 was authored before Gate G016 / Check 8A's regression-E2E planning requirement was added to state-transition-guard.
- **Why are the G068 fidelity gaps present?** Ten Gherkin scenarios were paraphrased into DoD bullets without quoting the scenario name in the `Scenario "<name>":` prefix form that G068 requires. The 2026-05-09 MIT-027-TRACE-001 trace-cleanup closed 33 prefixes but did not enumerate these ten because they describe scope-level capability ("Parse tags only", "GET annotation history", etc.) where the matching DoD bullet was paraphrased rather than quoted.
- **Why is `bootstrap`/`test`/`validate` provenance missing?** Original `bubbles.plan` and `bubbles.workflow` entries claimed multiple phases under a single agent identity; Gate G022 extension was tightened later to require `bubbles.<phase>:<phase>` strict provenance.
- **Why are `regression`/`simplify`/`stabilize`/`security` phases missing from `certifiedCompletedPhases`?** The Simplification Pass, Improvement Pass, Security Pass, and Reconciliation Pass sections were added to `report.md` during reconciliation work, but `certification.certifiedCompletedPhases` was not updated each time. The spec 044 cross-spec security closures were recorded in executionHistory but did not retroactively backfill the parent-spec certification field.
- **Why does Check 5A SLA stress fire?** Because the case-insensitive plain-substring regex `slo` matches the substring inside `TestMigrations_ExtensionsLoaded` at `scopes.md` line 168. The spec makes no SLA / latency / throughput claim; this is a substring false-positive that can be cleared by adding a Stress Test Plan row to Scope 1 (which is what Check 5A's pair predicate looks for).
- **Why does Check 8B consumer-trace fire on Scope 4?** Because Scope 4 contains the `DELETE /api/artifacts/{id}/tags/{tag}` endpoint ("removed=\"weeknight\""), which Check 8B treats as an interface removal/rename. The scope was authored before Check 8B required Consumer Impact Sweep sections for rename/removal scopes.

## Related

- Parent: `specs/027-user-annotations/`
- Prior bugs: NONE — this is the first bug under spec 027.
- Sweep ledger entry: `.specify/memory/sweep-2026-05-23-r30.json` round 21
- Reference patterns: `specs/026-domain-extraction/bugs/BUG-026-004-reconcile-artifact-drift/` (round 20 close-out, same artifact-quality reconciliation pattern); `specs/025-knowledge-synthesis-layer/bugs/BUG-025-004-test-trigger-probe-residuals/` (round 19 close-out, analogous pattern)
