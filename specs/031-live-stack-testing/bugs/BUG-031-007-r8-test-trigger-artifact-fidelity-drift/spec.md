# Spec: BUG-031-007 R8 sweep test trigger artifact fidelity drift

Links: [bug.md](bug.md) | [design.md](design.md) | [scopes.md](scopes.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md) | [scenario-manifest.json](scenario-manifest.json) | [state.json](state.json)

## Problem

Stochastic-quality-sweep `sweep-2026-05-23-r30` round 8 ran `mode: test-to-doc` against spec 031 (the previous round 3 BUG-031-006 closure had just landed) and surfaced three artifact-fidelity drift items that the round 3 closure did not reach:

1. `specs/031-live-stack-testing/scenario-manifest.json` SCN-LST-005 references a function (`test_integration`) that does not exist in the referenced bash script (`scripts/runtime/go-integration.sh`).
2. `specs/031-live-stack-testing/state.json` lists BUG-031-006 in `activeBugs[]` while the BUG itself is `done`/`done`.
3. `specs/031-live-stack-testing/scenario-manifest.json` has no scenario entry covering the three new SLA stress test functions added by BUG-031-006 in `tests/stress/ml_readiness_timeout_stress_test.go`.

## Scope

Fix the three artifact-fidelity drift items above. Touch only:

- `specs/031-live-stack-testing/scenario-manifest.json`
- `specs/031-live-stack-testing/state.json` (the `activeBugs`/`resolvedBugs` bookkeeping and `lastUpdatedAt` only — no certification field changes)
- This BUG packet under `specs/031-live-stack-testing/bugs/BUG-031-007-r8-test-trigger-artifact-fidelity-drift/`

## Out of Scope

- Re-litigating any BUG-031-006 finding (closed in round 3, commit `8b3c9229`).
- Any production source change. The compile sweep (`go vet`, `go build`) is already clean across the live-stack test surface.
- Spec 055 ntfy adapter WIP (pre-existing unstaged changes in working tree — out of bounds for this BUG).
- New scenario IDs. SCN-LST-004 already anchors Scope 6 ML readiness; extending its `linkedTests` is the minimal change that preserves scenario-anchor stability and avoids manifest churn.

## Acceptance

- `python3` cross-check (linked-test function existence) reports zero `FUNC-MISSING` findings across the full SCN-LST-001..SCN-LST-012 set.
- `python3` cross-check (parent vs BUG-031-006 state) reports `BUG-031-006` is present in `resolvedBugs` and absent from `activeBugs`.
- `grep` against `specs/031-live-stack-testing/scenario-manifest.json` finds `ml_readiness_timeout_stress` and the three SLA stress test function names in SCN-LST-004 `linkedTests` and `evidenceRefs`.
- `bash .github/bubbles/scripts/state-transition-guard.sh specs/031-live-stack-testing` exits 0 (TRANSITION PERMITTED) with no new BLOCKs vs the baseline.
- `bash .github/bubbles/scripts/artifact-lint.sh specs/031-live-stack-testing/bugs/BUG-031-007-r8-test-trigger-artifact-fidelity-drift` exits 0.

## Cross-Spec / Cross-Product Impact

None. All edits are local to `specs/031-live-stack-testing/`. No cross-product (QF) packet metadata touched.
# Spec: BUG-031-007 R8 sweep test trigger artifact fidelity drift

## Source

- Parent feature: `specs/031-live-stack-testing/`
- Sweep: `sweep-2026-05-23-r30` round 8 (`mode: test-to-doc`)
- Trigger surface: `bubbles.test` probe of spec 031's live-stack test surface after BUG-031-006 closure
- Related sister bug (just closed in R3): `BUG-031-006-strict-guard-gate-drift`

## Goal

Restore artifact-level fidelity between spec 031's planning artifacts (scenario-manifest.json, state.json) and the on-disk test surface so that the manifest accurately models reality, the parent state.json bookkeeping reflects every BUG-NNN packet's certification state, and every new SLA-class test added by a closure pass is anchored to a Gherkin scenario.

## In Scope

- `specs/031-live-stack-testing/scenario-manifest.json` SCN-LST-005 `linkedTests` correction (remove phantom function reference).
- `specs/031-live-stack-testing/scenario-manifest.json` SCN-LST-004 `linkedTests` extension (add three SLA stress test functions and a stress evidenceRef).
- `specs/031-live-stack-testing/state.json` `activeBugs` / `resolvedBugs` reconciliation for BUG-031-006.
- `specs/031-live-stack-testing/state.json` `lastUpdatedAt` bump and `executionHistory` append for the R8 closure.
- BUG-031-007 packet artifacts (spec.md, design.md, scopes.md, report.md, state.json, scenario-manifest.json, uservalidation.md, bug.md).

## Out of Scope

- Any production source change. The change manifest is artifact-edit only.
- Any test source change. Existing tests, helpers, and stress functions are not modified.
- Any docs/* change outside the BUG packet. Published `docs/Testing.md` and `docs/Operations.md` already reference spec 031's live-stack pattern.
- Reopening BUG-031-006 closure work; this BUG only repairs three drift items that R3 did not address.
- Creating a new top-level scenario ID for the SLA stress probe — SCN-LST-004 is the existing anchor for the Scope 6 ML readiness gate and the SLA boundary is a tighter probe of the same contract.

## Success Criteria

- A scenario-manifest fidelity audit (cross-check `linkedTests` against on-disk function declarations) reports zero `FUNC-MISSING` findings against `specs/031-live-stack-testing/scenario-manifest.json`.
- `specs/031-live-stack-testing/state.json` `activeBugs` and `resolvedBugs` mirror the actual certification state of every BUG packet under `specs/031-live-stack-testing/bugs/` (BUG-001 + BUG-031-001..005 + BUG-031-006 all in `resolvedBugs`; this BUG-031-007 follows the same lifecycle and is moved on its own validate close).
- `specs/031-live-stack-testing/scenario-manifest.json` SCN-LST-004 `linkedTests` includes the three SLA stress functions and SCN-LST-004 `evidenceRefs` references the stress test file.
- `bash .github/bubbles/scripts/state-transition-guard.sh specs/031-live-stack-testing` exits 0 with `TRANSITION PERMITTED`.
- `bash .github/bubbles/scripts/state-transition-guard.sh specs/031-live-stack-testing/bugs/BUG-031-007-r8-test-trigger-artifact-fidelity-drift` exits 0 with `TRANSITION PERMITTED`.
- `bash .github/bubbles/scripts/artifact-lint.sh specs/031-live-stack-testing` and `bash .github/bubbles/scripts/artifact-lint.sh specs/031-live-stack-testing/bugs/BUG-031-007-r8-test-trigger-artifact-fidelity-drift` both exit 0.
- A single structured commit lands with the Check 17 prefix `spec(031,bug-031-007): ...` touching only the change-boundary surface.

## Non-Goals

- Re-running the live integration / e2e / stress suite. The change manifest is artifact-only; existing GREEN state on spec 031 is preserved by the empty production-source delta.
- Refactoring how scenario-manifest function references work. The fidelity audit is a one-off correction, not a tooling change.
