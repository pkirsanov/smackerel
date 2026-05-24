# Bug: BUG-002-005 Reconcile post-closure artifact drift surfaced by sweep round 30 (Check 6A / Check 6B / Check 8A / Check 8D)

## Summary

Sweep round 30 (FINAL) of `sweep-2026-05-23-r30` (`mode: security-to-doc`, executionModel `parent-expanded-child-mode`) trigger-probe over `specs/002-phase1-foundation/` surfaced **65 BLOCKS** in `bash .github/bubbles/scripts/state-transition-guard.sh specs/002-phase1-foundation`. The probe's security-trigger pass returned NEGATIVE for real production defects in the allowed surface (`internal/auth/` is mature and clean; `internal/api/`, `internal/web/`, `internal/notification/`, `internal/pipeline/`, `internal/config/`, `cmd/core/`, and `config/` are owned by active WIP feature surfaces — spec 044 per-user PASETO, spec 053, spec 055 — and are out of bounds for this sweep round). All 65 BLOCKS are governance-drift findings against artifact-quality requirements that tightened after spec 002 was originally certified.

The 65 state-transition-guard BLOCKS fall into 4 categories:

1. **Check 6A — planning specialist dispatch records missing (4 BLOCKS = 3 + 1 rollup):** `state.json::executionHistory` had no strict entries from `bubbles.analyst`, `bubbles.design`, or `bubbles.plan`. The original 2026-04-07 multi-pass `full-delivery` lockdown collapsed planning under cross-spec agent identities (e.g., the 2026-03 design work landed under `bubbles.workflow`-driven entries) and the 2026-04-10 `bubbles.implement` and `bubbles.spec-review` records do not satisfy the strict-provenance predicate. One additional rollup BLOCK ("3 planning specialist dispatch record(s) missing — planning-first workflow compliance not proven") is the same finding.
2. **Check 6B — phase-claim provenance impersonation (5 BLOCKS = 4 + 1 rollup):** `state.json::execution.completedPhaseClaims` contains `plan`, `analyze`, `design`, `finalize` but `executionHistory` had no strict `bubbles.<phase>:<phase>` entries from `bubbles.plan`, `bubbles.analyze`, `bubbles.design`, or `bubbles.finalize`. Gate G022 treats that as phase impersonation. One additional rollup BLOCK ("4 phase claim(s) lack proper agent provenance — phase impersonation detected") is the same finding.
3. **Check 8A — scenario-specific regression E2E planning gaps (52 BLOCKS = 51 + 1 rollup = 17 scopes × 3 requirements):** Each of scopes 9-25 in `scopes.md` lacked (a) DoD bullet `- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior`, (b) DoD bullet `- [x] Broader E2E regression suite passes`, and (c) Test Plan row containing the literal substring `Regression E2E`. Scopes 1-8 were already compliant from the 2026-04-15 hardening pass that touched them. One additional rollup BLOCK ("51 regression E2E planning requirement(s) missing — every feature/fix/change needs persistent scenario-specific E2E regression coverage") is the same finding.
4. **Check 8D — Change Boundary containment requirements missing (4 BLOCKS = 3 + 1 rollup):** `scopes.md` was authored before Check 8D's refactor/repair scopes detection demanded (a) a `Change Boundary` section, (b) a DoD bullet asserting the boundary was respected, and (c) explicit enumeration of allowed and excluded file families. Several spec-002 scopes match Check 8D's refactor/repair heuristic (e.g., Scope 9 "Extract Shared Pipeline Constants", Scope 10 "Decompose Process() Into Pipeline Stages", Scope 19 "Supervisor Sleep Context Cancellation", Scope 20 "Remove Dead SYNTHESIS Stream"). One additional rollup BLOCK ("3 change-boundary containment requirement(s) missing") is the same finding.

Total: **65 state-transition-guard BLOCKS**, all governance-drift, zero production-code defects.

No closed BUG-002-NNN covers any of these residuals (BUG-002-001, BUG-002-002, BUG-002-003, BUG-002-004 resolved different findings — bootstrap/E2E/notification/auth-store regressions). The four 2026-04-12 → 2026-05-08 prior reconciliation passes documented in `report.md` (Hardening Pass H1, Improve-Existing Pass I1, Test-To-Doc Pass T1, Trace-Guard Remediation Iter 9) covered different dimensions.

## Severity

- [ ] Critical — System unusable, data loss
- [ ] High — Major feature broken
- [x] Medium — Artifact-quality drift makes `bash .github/bubbles/scripts/state-transition-guard.sh specs/002-phase1-foundation` exit with `🔴 TRANSITION BLOCKED: 65 failure(s)`. Spec 002 cannot legitimately re-promote to `done` under current gate standards even though its original 2026-04-07 → 2026-05-08 delivery + 4 reconciliation passes are functionally correct. Round 30 of sweep `sweep-2026-05-23-r30` cannot reach `completed_owned` without resolving this drift.
- [ ] Low — Minor issue, cosmetic

## Status

- [x] Reported
- [x] Confirmed by sweep round 30 security-to-doc probe
- [x] In Progress
- [x] Fixed
- [x] Verified
- [x] Closed

## Reproduction Steps

1. From clean HEAD prior to this BUG packet's commit, run `bash .github/bubbles/scripts/state-transition-guard.sh specs/002-phase1-foundation 2>&1 | grep -cE "^🔴 BLOCK"`.
2. Observe 65 BLOCK lines.
3. Run `bash .github/bubbles/scripts/state-transition-guard.sh specs/002-phase1-foundation 2>&1 | grep -E "^🔴 BLOCK" | sort | uniq -c | sort -rn | head -10` to see the bucket breakdown:
   - 17× `Scope is missing DoD item for scenario-specific regression E2E coverage` (scopes 9-25)
   - 17× `Scope is missing DoD item for broader E2E regression suite coverage` (scopes 9-25)
   - 17× `Scope Test Plan is missing explicit scenario-specific regression E2E row(s)` (scopes 9-25)
   - 3× `Planning specialist '<name>' missing from executionHistory` (analyst, design, plan)
   - 4× `Phase '<name>' is in completedPhaseClaims but no executionHistory entry from bubbles.<name>` (analyze, design, plan, finalize)
   - 3× `Scope is a refactor/repair but ... Change Boundary` (missing section, missing DoD bullet, missing allowed/excluded enumeration)
   - rollup BLOCKS for each bucket
4. Run `bash .github/bubbles/scripts/artifact-lint.sh specs/002-phase1-foundation 2>&1 | tail -3` and observe `Artifact lint PASSED.`
5. Run `bash .github/bubbles/scripts/traceability-guard.sh specs/002-phase1-foundation 2>&1 | tail -3` and observe `RESULT: PASSED` (82/82 scenarios mapped from the 2026-05-08 Trace-Guard Remediation Iter 9).
6. Run `bash .github/bubbles/scripts/artifact-freshness-guard.sh specs/002-phase1-foundation 2>&1 | tail -3` and observe whatever current freshness state is (not part of this BUG's bucket list).

## Expected Behavior

- `bash .github/bubbles/scripts/state-transition-guard.sh specs/002-phase1-foundation` exits 0 with `🟢 TRANSITION ALLOWED` (or equivalent green verdict).
- `bash .github/bubbles/scripts/artifact-lint.sh specs/002-phase1-foundation` continues to exit 0.
- `bash .github/bubbles/scripts/traceability-guard.sh specs/002-phase1-foundation` continues to exit 0 with `RESULT: PASSED`.
- `bash .github/bubbles/scripts/artifact-freshness-guard.sh specs/002-phase1-foundation` continues to exit 0.
- Spec 002 remains `status: done`. No runtime code, schema, NATS topology, web template, prompt contract, Telegram command, config value, deploy script, or compose file is modified by this BUG packet.

## Actual Behavior

- `state-transition-guard.sh` exits non-zero with **65 BLOCKS** distributed across Check 6A (4 = 3 missing planning specialists + 1 rollup), Check 6B (5 = 4 phase-claim impersonations + 1 rollup), Check 8A (52 = 17 scopes × 3 requirements + 1 rollup), Check 8D (4 = 3 Change Boundary requirements + 1 rollup).

## Environment

- Branch: `main`, HEAD prior to this BUG packet's commit
- Sweep: `sweep-2026-05-23-r30` round 30 (FINAL), mode `security-to-doc`, executionModel `parent-expanded-child-mode`
- Parent feature: `specs/002-phase1-foundation` (currently `status: done` per pre-G022-strict certification on 2026-04-10, hardening passes through 2026-04-22, trace-guard remediation 2026-05-08)
- State guard version: as of HEAD prior to this BUG packet's commit
- Reference patterns: `specs/024-design-doc-reconciliation/bugs/BUG-024-002-reconcile-artifact-drift/` (round 29 close-out, same artifact-quality reconciliation pattern); `specs/027-user-annotations/bugs/BUG-027-001-reconcile-artifact-drift/` (round 21 close-out, analogous); `specs/026-domain-extraction/bugs/BUG-026-004-reconcile-artifact-drift/` (round 20 close-out, analogous)
- Test runner: `./smackerel.sh test unit` baseline green; no test depends on the parent spec's `scopes.md` DoD/Test-Plan text shape

## Error Output

```text
$ bash .github/bubbles/scripts/state-transition-guard.sh specs/002-phase1-foundation 2>&1 | grep -cE "^🔴 BLOCK"
65
$ bash .github/bubbles/scripts/state-transition-guard.sh specs/002-phase1-foundation 2>&1 | grep -E "^🔴 BLOCK" | head -12
🔴 BLOCK: Planning specialist 'bubbles.analyst' missing from executionHistory (workflow may have bypassed required dispatch)
🔴 BLOCK: Planning specialist 'bubbles.design' missing from executionHistory (workflow may have bypassed required dispatch)
🔴 BLOCK: Planning specialist 'bubbles.plan' missing from executionHistory (workflow may have bypassed required dispatch)
🔴 BLOCK: 3 planning specialist dispatch record(s) missing — planning-first workflow compliance not proven
🔴 BLOCK: Phase 'plan' is in completedPhaseClaims but no executionHistory entry from bubbles.plan — possible impersonation (Gate G022)
🔴 BLOCK: Phase 'analyze' is in completedPhaseClaims but no executionHistory entry from bubbles.analyze — possible impersonation (Gate G022)
🔴 BLOCK: Phase 'finalize' is in completedPhaseClaims but no executionHistory entry from bubbles.finalize — possible impersonation (Gate G022)
🔴 BLOCK: Phase 'design' is in completedPhaseClaims but no executionHistory entry from bubbles.design — possible impersonation (Gate G022)
🔴 BLOCK: 4 phase claim(s) lack proper agent provenance — phase impersonation detected
🔴 BLOCK: Scope is missing DoD item for scenario-specific regression E2E coverage: Scope 9: Extract Shared Pipeline Constants
🔴 BLOCK: Scope is missing DoD item for broader E2E regression suite coverage: Scope 9: Extract Shared Pipeline Constants
🔴 BLOCK: Scope Test Plan is missing explicit scenario-specific regression E2E row(s): Scope 9: Extract Shared Pipeline Constants
```

## Workaround

None — sweep round 30 cannot proceed past the `security-to-doc` probe without resolving the 65 artifact-quality BLOCKS. Spec 002 stays functionally `done`, but artifact-level promotion under current gate standards is blocked.

## Root Cause Analysis (Five Whys)

- **Why did 65 BLOCKS appear?** Because the state-transition-guard's Check 6A (planning-specialist dispatch records), Check 6B (phase-claim provenance), Check 8A (scenario-specific regression E2E planning), and Check 8D (Change Boundary containment for refactor/repair scopes) have all tightened standards since spec 002 was originally certified on 2026-04-10.
- **Why didn't the four 2026-04-12 → 2026-05-08 reconciliation passes catch this drift?** Those passes targeted (a) bootstrap recovery (Hardening Pass H1), (b) E2E lockdown (Improve-Existing I1), (c) test-trigger residual closure (Test-To-Doc T1), and (d) cross-spec trace-guard repair (Trace-Guard Remediation Iter 9). None of them ran the reconcile-first validate that catches artifact-quality drift across Check 6A / 6B / 8A / 8D.
- **Why are the regression-E2E DoD bullets and Test Plan rows missing on scopes 9-25?** Scopes 1-8 were authored during the original 2026-04-07 → 2026-04-10 delivery and were touched again by the 2026-04-15 hardening pass that backfilled Gate G016 / Check 8A's regression-E2E planning requirement. Scopes 9-25 were added by the 2026-04-20 → 2026-05-08 ENG-001..011 + SEC-001 follow-up workstream and the Check 8A backfill never re-touched them.
- **Why are several scopes 9-25 lacking Change Boundary sections?** Because Check 8D's refactor/repair recognition (scope-title keywords like `extract`, `decompose`, `remove`, `fix`, `cleanup`) was tightened after the ENG-* / SEC-* follow-up workstream landed.
- **Why are `analyst`/`design`/`plan` provenance entries missing?** Because the original `bubbles.workflow`-driven full-delivery run for spec 002 collapsed planning under cross-spec agent identities. Gate G022 / Check 6A was tightened later to require strict `bubbles.<phase>:<phase>` provenance.
- **Why are `analyze`/`design`/`plan`/`finalize` phase-claim provenance entries missing?** Same root cause as above — the original `bubbles.workflow` run collapsed those phases under cross-spec identities and Gate G022 was tightened later to require strict per-phase provenance.

## Related

- Parent: `specs/002-phase1-foundation/`
- Prior bugs (different findings):
  - `specs/002-phase1-foundation/bugs/BUG-002-001-bootstrap-recovery/` (resolved bootstrap regression)
  - `specs/002-phase1-foundation/bugs/BUG-002-002-e2e-stack-lockdown/` (resolved E2E lockdown drift)
  - `specs/002-phase1-foundation/bugs/BUG-002-003-notification-pipeline/` (resolved notification pipeline drift)
  - `specs/002-phase1-foundation/bugs/BUG-002-004-auth-store-encryption/` (resolved auth store encryption drift)
- Sweep ledger entry: `.specify/memory/sweep-2026-05-23-r30.json` round 30
- Reference patterns:
  - `specs/024-design-doc-reconciliation/bugs/BUG-024-002-reconcile-artifact-drift/` (round 29 close-out, primary template for this packet)
  - `specs/027-user-annotations/bugs/BUG-027-001-reconcile-artifact-drift/` (round 21 close-out)
  - `specs/026-domain-extraction/bugs/BUG-026-004-reconcile-artifact-drift/` (round 20 close-out)
