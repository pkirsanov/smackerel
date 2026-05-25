# Bug: BUG-006-005 — Governance Baseline Drift (executionHistory, claim arrays, deferral language, commit prefix)

> **Parent Spec:** [specs/006-phase5-advanced](../../spec.md)
> **Severity:** Medium
> **Found By:** bubbles.gaps (sweep-2026-05-24-r10 round 2, trigger=gaps, mappedMode=gaps-to-doc)
> **Date:** 2026-05-24

## Problem

Stochastic sweep round 2 ran `bubbles.gaps` against `specs/006-phase5-advanced`
and discovered four classes of governance baseline drift that prevent the
parent spec from passing the current `state-transition-guard.sh` even though
its `status` is already `done`. The drift accumulated under older governance
where individual checks (Check 6B phase-claim provenance, full-delivery
required-phase list, commit-prefix Check 17, deferral-language Gate G040 / Check
18) were either looser or absent. The current strict guard surfaces 16 BLOCK
findings on baseline:

1. **F1 — Deferral language hits in `report.md` (Gate G040, Check 18).** Three
   passages in `specs/006-phase5-advanced/report.md` (lines 710, 745, 792)
   contained the forbidden tokens `deferred` / `deferred remainder`. Two were
   historical NOTED/REMOVED narrations about the `ResurfaceScore` dead-code
   finding; the third described a scheduler-loop cap that intentionally
   processes overflow on the next tick.
2. **F2 — `execution.executionHistory` truncated to one entry (Check 6B / Gate
   G022 extension).** The nested array contained only the manual `bubbles.spec-review`
   re-cert entry from 2026-04-23. All other historical specialist entries lived
   under the top-level `executionHistory` archive. `state-transition-guard.sh`
   Check 6B prefers `execution.executionHistory` when present and falls back to
   the top level only when nested is missing. With one nested entry, the
   fallback never triggered, and 8 claimed phases (`select`, `bootstrap`,
   `implement`, `test`, `validate`, `audit`, `chaos`, `docs`) appeared to lack
   provenance from their expected `bubbles.<phase>` agents and were flagged as
   phase impersonation.
3. **F3 — Required full-delivery phases missing from claim arrays (Check 6
   Gate G022).** `state-transition-guard.sh` `required_specialists` for
   `workflowMode: full-delivery` is `(implement, test, regression, simplify,
   gaps, harden, stabilize, security, validate, audit, chaos, docs)`. The
   parent `execution.completedPhaseClaims` and
   `certification.certifiedCompletedPhases` were missing `regression`,
   `simplify`, `gaps`, `harden`, `stabilize`, and `security` even though all
   six had historical provenance in the top-level `executionHistory` archive.
4. **F4 — No structured commit with `spec(006)` or `bubbles(006/...)` prefix
   (Check 17).** Existing commits used `feat(006)`, `feat(004,005,006)`, etc.
   None matched the prefix the guard requires for full-delivery specs.

These findings are artifact-integrity and traceability gaps, not runtime
defects. Production Phase 5 intelligence code in `internal/intelligence/` and
`internal/scheduler/` continues to build clean, pass unit tests, and behave as
described in `design.md`. `artifact-lint.sh` and `traceability-guard.sh` both
PASS on baseline.

## Impact

`state-transition-guard.sh` BLOCKs any future re-certification, downstream
delivery work, or audit on spec 006 because the parent is no longer
guard-clean under current gates. Without structured remediation:

- Any future workflow run that re-touches spec 006 would have to either inherit
  these BLOCKs as terminal failures or rationalize a baseline skip (which the
  stochastic-quality-sweep contract explicitly forbids).
- Audit/traceability narratives downstream of spec 006 would carry forward
  the impersonation appearance: the provenance check would suggest specialist
  phases were never invoked.
- The sweep ledger `.specify/memory/sweep-2026-05-24-r10.json` cannot record
  round 2 as `clean` without remediation; findings-only output is not a valid
  outcome for the trigger.

## Expected Behavior

`bash .github/bubbles/scripts/state-transition-guard.sh specs/006-phase5-advanced`
exits 0 with a final `🟡 TRANSITION PERMITTED` line and zero BLOCK findings.

## Acceptance Criteria

- AC-1: `grep -nE "(defer(red|ral)?|postpone|push(ed)? to|future iteration|punt(ed)?|skip(ped)? for now|will address|todo|out of scope.*follow)" specs/006-phase5-advanced/report.md` returns no matches.
- AC-2: `execution.executionHistory` contains a `bubbles.<phase>:<phase>` entry for every value in `execution.completedPhaseClaims` and `certification.certifiedCompletedPhases`.
- AC-3: `execution.completedPhaseClaims` and `certification.certifiedCompletedPhases` each contain every required phase for full-delivery: `implement`, `test`, `regression`, `simplify`, `gaps`, `harden`, `stabilize`, `security`, `validate`, `audit`, `chaos`, `docs`.
- AC-4: `git log --format='%s' -- specs/006-phase5-advanced/ | grep -Ec '^spec\(006\)|^bubbles\(006/'` returns a count ≥ 1.
- AC-5: `bash .github/bubbles/scripts/state-transition-guard.sh specs/006-phase5-advanced` exits 0 with a `🟡 TRANSITION PERMITTED` line and zero `❌ BLOCK` findings.
- AC-6: This bug packet has full 6-artifact set (`spec.md`, `design.md`, `scopes.md`, `report.md`, `uservalidation.md`, `state.json`) and the bug `state.json.status` is `done` with its own guard-clean state.
