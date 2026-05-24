# Bug: BUG-028-003 Reconcile artifact drift to current gate standards (Check 5A / G016 regression E2E / G022 / G053 / Check 17 commit prefix)

## Summary

Sweep round 22 of `sweep-2026-05-23-r30` (`mode: stochastic-quality-sweep`, trigger `harden`, mapped child workflow mode `harden-to-doc`) reconciliation probe on `specs/028-actionable-lists/` surfaced 38 artifact-drift BLOCKS in `bash .github/bubbles/scripts/state-transition-guard.sh specs/028-actionable-lists` that survived the original `done` certification on 2026-04-19 and the 2026-05-09 trace closure. Implementation code, tests, and runtime behavior are correct (verified by `internal/list/` Go unit suites including `harden_test.go` adversarial coverage added by BUG-028-002, by `internal/api/lists_test.go`, by `internal/telegram/list_test.go`, by `internal/intelligence/lists_test.go`, and by `tests/integration/artifact_crud_test.go::TestList_CreateAndUpdateStatus` and `tests/integration/artifact_crud_test.go::TestList_Chaos_CascadeDeleteDuringConcurrentUpdates`). The drift is exclusively in the spec/scope/state artifacts because they were authored before the current gate standards were tightened.

The 38 BLOCKS fall into 6 categories:

1. **Gate G022 — missing required specialist phase records (4 + 1 rollup BLOCKS):** `state.json` `certification.certifiedCompletedPhases` lacks `regression`, `simplify`, `stabilize`, `security` even though each surface ran in past work (initial implementation pass 2026-04-17/19, BUG-028-001 G068 fidelity sweep 2026-04-27, BUG-028-002 harden silent-swallow remediation 2026-05-12 which added `harden_test.go` adversarial coverage for all three aggregators). One additional rollup BLOCK ("4 specialist phase(s) missing — work was NOT executed through the full pipeline") is the same finding.
2. **Gate G022 extension — phase-claim provenance impersonation (3 + 1 rollup BLOCKS):** `completedPhaseClaims` contains `bootstrap`, `test`, `validate` but `executionHistory` has no `bubbles.bootstrap:bootstrap`, `bubbles.test:test`, or `bubbles.validate:validate` entry — the original claims were collapsed under `bubbles.plan` (for `bootstrap`) and `bubbles.workflow` (for `test`/`validate`). One additional rollup BLOCK ("3 phase claim(s) lack proper agent provenance") is the same finding.
3. **Gate G053 — missing `### Code Diff Evidence` section (1 BLOCK):** `report.md` does not have the section header that implementation-bearing workflows must emit.
4. **Check 5A — SLA-sensitive scope missing explicit stress coverage (1 BLOCK):** `scopes.md` line 389 contains the substring `slo` inside `slog.Warn` (the Generator skip-with-warning logic for missing `domain_data`). Check 5A's case-insensitive plain-substring regex matches `slo` and demands a Stress test row. No real SLA / latency / throughput claim is made for spec 028 — this is a substring false-positive on the structured-log token.
5. **Check 8A regression E2E planning (24 + 1 rollup BLOCKS = 8 scopes × 3 requirements):** Each scope in `scopes.md` lacks (a) DoD bullet `- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior`, (b) DoD bullet `- [x] Broader E2E regression suite passes`, and (c) Test Plan row containing `Regression E2E`. One additional rollup BLOCK ("24 regression E2E planning requirement(s) missing") is the same finding.
6. **Check 17 — full-delivery requires at least one structured commit message for spec 028 (1 BLOCK):** `git log` shows zero commits with prefix `^spec\(028\)|^bubbles\(028/`. BUG-028-001 and BUG-028-002 landed under a different prefix discipline.

Total: 38 BLOCKS. Neither BUG-028-001 (G068 DoD-Gherkin fidelity, resolved 2026-04-27) nor BUG-028-002 (compare-aggregator silent JSON swallow, resolved 2026-05-12) covers any of these residuals.

## Severity

- [ ] Critical - System unusable, data loss
- [ ] High - Major feature broken
- [x] Medium - Artifact-quality drift makes `bash .github/bubbles/scripts/state-transition-guard.sh specs/028-actionable-lists` exit with `🔴 TRANSITION BLOCKED: 38 failure(s)`. Spec 028 cannot legitimately re-promote to `done` under current gate standards even though its runtime code is correct. Sweep round 22 cannot reach `completed_owned` without resolving this drift.
- [ ] Low - Minor issue, cosmetic

## Status

- [x] Reported
- [x] Confirmed by sweep round 22 harden-to-doc probe
- [ ] In Progress
- [ ] Fixed
- [ ] Verified
- [ ] Closed

## Reproduction Steps

1. From clean HEAD `42863de8` (last bulk-checkpoint commit), run `bash .github/bubbles/scripts/state-transition-guard.sh specs/028-actionable-lists 2>&1 | grep -cE "^🔴 BLOCK"`.
2. Observe 38 BLOCK lines.
3. Run `bash .github/bubbles/scripts/traceability-guard.sh specs/028-actionable-lists 2>&1 | tail -3`.
4. Observe `RESULT: PASSED` (already fixed by BUG-028-001 — 34/34 fidelity).
5. Run `bash .github/bubbles/scripts/artifact-lint.sh specs/028-actionable-lists 2>&1 | tail -3`.
6. Observe `Artifact lint PASSED.` (artifact-lint accepts the legacy scopeLayout and does not enforce regression-E2E planning).

## Expected Behavior

- `bash .github/bubbles/scripts/state-transition-guard.sh specs/028-actionable-lists` exits 0 with `🟢 TRANSITION ALLOWED` (or equivalent green verdict).
- `bash .github/bubbles/scripts/traceability-guard.sh specs/028-actionable-lists` continues to exit 0 with `RESULT: PASSED`.
- `bash .github/bubbles/scripts/artifact-lint.sh specs/028-actionable-lists` continues to exit 0.
- Spec 028 stays `status: done`. No runtime code, schema, NATS topology, config value, web template, prompt contract, or Telegram command is changed.

## Actual Behavior

- `state-transition-guard.sh` exits non-zero with 38 BLOCKS distributed across Check 5A (SLA false-positive substring), Check 6 (G022 phases), Check 6B (G022 provenance), Check 8A (regression E2E planning), Check 13B (G053 Code Diff Evidence), and Check 17 (structured commit prefix).
- `traceability-guard.sh` exits 0 (already remediated by BUG-028-001).
- `artifact-lint.sh` exits 0.

## Environment

- Branch: `main`, HEAD `42863de8`
- Sweep: `sweep-2026-05-23-r30` round 22, mode `stochastic-quality-sweep`, trigger `harden`, mapped child workflow mode `harden-to-doc`, executionModel `parent-expanded-child-mode`
- Parent feature: `specs/028-actionable-lists` (currently `status: done` per pre-G022-strict certification + BUG-028-001 G068 closure 2026-04-27 + BUG-028-002 harden remediation 2026-05-12)
- State guard version: as of HEAD `42863de8`
- Test runner: `./smackerel.sh test unit` baseline green; `go test ./internal/list/... ./internal/api/` PASS in 0.05s at probe time; `internal/list/harden_test.go` adversarial coverage (TestScanSources_PropagatesPerRowScanError, TestScanSources_PropagatesRowsErr, TestRecipeAggregator_LogsAndSkipsBadJSON, TestReadingAggregator_FallsBackOnBadJSON, TestCompareAggregator_LogsAndSkipsBadJSON) all green

## Error Output

```text
$ bash .github/bubbles/scripts/state-transition-guard.sh specs/028-actionable-lists 2>&1 | grep -cE "^🔴 BLOCK"
38
$ bash .github/bubbles/scripts/state-transition-guard.sh specs/028-actionable-lists 2>&1 | grep -E "^🔴 BLOCK" | head -20
🔴 BLOCK: SLA-sensitive scope is missing explicit stress coverage: scopes.md
🔴 BLOCK: Required phase 'regression' NOT in execution/certification phase records (Gate G022 violation)
🔴 BLOCK: Required phase 'simplify' NOT in execution/certification phase records (Gate G022 violation)
🔴 BLOCK: Required phase 'stabilize' NOT in execution/certification phase records (Gate G022 violation)
🔴 BLOCK: Required phase 'security' NOT in execution/certification phase records (Gate G022 violation)
🔴 BLOCK: 4 specialist phase(s) missing — work was NOT executed through the full pipeline
🔴 BLOCK: Phase 'bootstrap' is in completedPhaseClaims but no executionHistory entry from bubbles.bootstrap — possible impersonation (Gate G022)
🔴 BLOCK: Phase 'validate' is in completedPhaseClaims but no executionHistory entry from bubbles.validate — possible impersonation (Gate G022)
🔴 BLOCK: Phase 'test' is in completedPhaseClaims but no executionHistory entry from bubbles.test — possible impersonation (Gate G022)
🔴 BLOCK: 3 phase claim(s) lack proper agent provenance — phase impersonation detected
🔴 BLOCK: Scope is missing DoD item for scenario-specific regression E2E coverage: Scope 1: DB Migration & List Types
🔴 BLOCK: Scope is missing DoD item for broader E2E regression suite coverage: Scope 1: DB Migration & List Types
🔴 BLOCK: Scope Test Plan is missing explicit scenario-specific regression E2E row(s): Scope 1: DB Migration & List Types
🔴 BLOCK: Scope is missing DoD item for scenario-specific regression E2E coverage: Scope 2: List Store (CRUD)
🔴 BLOCK: Scope is missing DoD item for broader E2E regression suite coverage: Scope 2: List Store (CRUD)
🔴 BLOCK: Scope Test Plan is missing explicit scenario-specific regression E2E row(s): Scope 2: List Store (CRUD)
🔴 BLOCK: Scope is missing DoD item for scenario-specific regression E2E coverage: Scope 3: Aggregator Interface & Recipe Aggregator
🔴 BLOCK: Scope is missing DoD item for broader E2E regression suite coverage: Scope 3: Aggregator Interface & Recipe Aggregator
🔴 BLOCK: Scope Test Plan is missing explicit scenario-specific regression E2E row(s): Scope 3: Aggregator Interface & Recipe Aggregator
🔴 BLOCK: Scope is missing DoD item for scenario-specific regression E2E coverage: Scope 4: Reading & Comparison Aggregators
```

```text
$ bash .github/bubbles/scripts/state-transition-guard.sh specs/028-actionable-lists 2>&1 | tail -6
🔴 BLOCK: 24 regression E2E planning requirement(s) missing — every feature/fix/change needs persistent scenario-specific E2E regression coverage
🔴 BLOCK: Implementation-bearing workflow requires '### Code Diff Evidence' in report artifacts (Gate G053)
🔴 BLOCK: full-delivery requires at least one structured commit message for spec 028 (expected prefix: spec(028) or bubbles(028/...)
🔴 TRANSITION BLOCKED: 38 failure(s)

$ bash .github/bubbles/scripts/traceability-guard.sh specs/028-actionable-lists 2>&1 | tail -2
ℹ️  DoD fidelity scenarios: 34 (mapped: 34, unmapped: 0)
RESULT: PASSED
```
