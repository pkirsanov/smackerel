# Bug: BUG-053-001 — Post-Hardening Trim Broke State.json/Scopes.md Integrity Markers

> **Parent Spec:** [specs/053-ci-ops-evidence-hardening](../../spec.md)
> **Severity:** Medium
> **Found By:** bubbles.gaps (sweep-2026-05-24-r10 round 4, trigger=gaps, mappedMode=gaps-to-doc)
> **Date:** 2026-05-25

## Problem

Stochastic sweep round 4 ran `bubbles.gaps` (as the trigger phase of the
parent-expanded `gaps-to-doc` child workflow mode) against
`specs/053-ci-ops-evidence-hardening` and discovered 25 individual
`state-transition-guard.sh` BLOCK findings, grouped into 5 finding classes,
all introduced by trim commit `d4596c45` "spec(053): trim report + scopes
after promote-to-hardened" (HEAD-1 at sweep entry). The trim's stated
purpose was post-promotion cleanup of `report.md` (−285 lines) and
`scopes.md` (−97 lines), but the cleanup over-aggressively removed
structurally-required content that the spec's prior `harden` phase had
explicitly added (recorded in `state.json` executionHistory) to clear the
final 13 `state-transition-guard.sh` advisories before the
`finalize` promotion to `specs_hardened`.

The current strict guard surfaces the following 5 finding classes on
baseline (atomic finding counts in parentheses; 25 BLOCKs total):

1. **F1 — Bold `**Status:**` markers stripped from all 5 scope headers
   (3 BLOCKs).** The trim converted `**Status:** Done` (bold markdown) to
   plain `Status: Done` text on every scope header at lines 123, 212, 311,
   435, and 570. `state-transition-guard.sh` Check 5 grep is
   `'\*\*Status:\*\*.*Done'` (literal bold markers required), so the
   guard now reports `total=0, Done=0` across all scope artifacts and
   fails three checks: "Resolved scope artifacts have no scope status
   markers", "completedScopes count (5) does not match artifact Done
   scope count (0) — state.json integrity failure", and the per-scope
   "Not Started"/"In Progress" cross-reference. This is a state.json
   integrity drift because `state.json.certification.completedScopes`
   still claims all 5 scopes are Done.

2. **F2 — Regression E2E DoD items + Test Plan rows stripped from all
   5 scopes (13 BLOCKs).** The trim removed the 10 per-scope DoD items
   the prior `harden` phase had added (S1-D6/D7 + S2-D7/D8 + S3-D8/D9 +
   S4-D7/D8 + S5-D10/D11) and the 3 regression E2E Test Plan rows
   (V-053-S1-004, V-053-S3-005, V-053-S4-006). Each scope now fails:
   "Scope is missing DoD item for scenario-specific regression E2E
   coverage", "Scope is missing DoD item for broader E2E regression
   suite coverage", and (for 3 scopes) "Scope Test Plan is missing
   explicit scenario-specific regression E2E row(s)". The summary
   aggregate: "13 regression E2E planning requirement(s) missing —
   every feature/fix/change needs persistent scenario-specific E2E
   regression coverage".

3. **F3 — Scope 5 Consumer Impact Sweep + change-boundary DoD items
   stripped (3 BLOCKs).** The trim removed the Scope 5 Consumer Impact
   Sweep section + S5-D12 consumer-impact DoD item (Check 8B
   rename/removal detection now fires because Scope 5's "Change
   Boundary + G040 Wrapper Disposition" title contains "Change
   Boundary" which Check 8B reads as a renames/removes interfaces
   trigger) and the S5-D13 change-boundary DoD item (Check 8D
   refactor/repair detection fires on the same Scope 5 title).
   Resulting failures: "Scope renames/removes interfaces but has no
   Consumer Impact Sweep section", "Scope renames/removes interfaces
   but is missing DoD item for consumer impact sweep", "Scope is a
   refactor/repair but is missing the change-boundary DoD item", and
   two aggregate "consumer-trace planning requirement(s) missing" and
   "change-boundary containment requirement(s) missing" rollups.

4. **F4 — G040 skip-sentinel wraps stripped (2 BLOCKs).** The trim
   removed 16 `<!-- bubbles:g040-skip-begin -->` /
   `<!-- bubbles:g040-skip-end -->` sentinel marker lines that the
   prior `harden` phase had wrapped around structurally-required
   narrative passages (Framework-Boundary opening field list,
   Implementation step 6 schema field list, Wrapper Disposition Record
   tables W-053-001/003/004/006, Framework-Boundary final
   crossRepoFollowUp table, plus 4 report.md narrative passages).
   Gate G040 Check 18 now re-flags 8 deferral-language hits in
   `scopes.md` ("Scope artifact contains 8 deferral language hit(s):
   scopes.md — SPEC CANNOT BE DONE WITH DEFERRED WORK (Gate G040)")
   and 2 hits in `report.md` ("Report artifact contains 2 deferral
   language hit(s): report.md — evidence of deferred work").

5. **F5 — SLA-stress reword reverted (1 BLOCK).** The trim reverted
   two `harden`-phase rewords: "Stale-reference scans recorded for any
   signal scheduled to change" → "...slated to change" (line 34) and
   "Reserve a row for the current traceability evidence" → "Reserve a
   slot..." (line 145). Check 5A SLA-detection grep
   `'latency|throughput|p95|p99|response time|sla|slo'` matches `sla`
   inside `slated` AND `slo` inside `slot`, marking Scope 4 (and
   adjacent regions of `scopes.md`) as SLA-sensitive. The trim left
   no `stress` keyword anywhere in `scopes.md`, so Check 5A fails:
   "SLA-sensitive scope is missing explicit stress coverage:
   scopes.md".

These findings are artifact-integrity gaps with **zero runtime code
surface**. Spec 053 is an artifact-only operations planning packet
(proven by the original `harden`/`validate` Scope 5 no-source-delta
proof recorded in `report.md` → "Scope 5 No-Source-Delta Proof —
Corrected Framing (validate-phase, 2026-05-18)"). The packet's product
content (TR matrix rows, consumer inventory, blast-radius records,
boundary records, wrapper disposition records, framework-boundary
record) is fully intact and unaffected by this drift; only the
guard-recognizable structural markers were lost.

## Impact

`state-transition-guard.sh` BLOCKs every future sweep round, audit,
re-certification, or downstream delivery work that touches spec 053
because the parent is no longer guard-clean under the post-trim
artifact state. Without structured remediation:

- Any future workflow run that re-enters spec 053 has to either inherit
  these BLOCKs as terminal failures or rationalize a baseline skip.
  The stochastic-quality-sweep contract `noBaselineSkip` explicitly
  forbids rationalizing baseline skips, which means later sweep rounds
  hitting spec 053 would terminate non-clean instead of advancing.
- `state.json.certification.completedScopes` continues to claim all 5
  scopes are Done while `scopes.md` parses as 0 Done scopes, producing
  an audit-visible integrity contradiction.
- The spec parks at `specs_hardened` ceiling but cannot honestly be
  re-validated as still hardened against the gates the prior `harden`
  phase explicitly satisfied.

The runtime/source/CI/deploy/framework surfaces of the repository are
unaffected. The Scope 5 no-source-delta proof (commit `edcd8836`
introduced zero out-of-boundary changes) remains valid; the trim
commit `d4596c45` itself also only touched
`specs/053-ci-ops-evidence-hardening/{report.md, scopes.md}` per its
diff summary "2 files changed, 13 insertions(+), 369 deletions(-)".

## Goal

Restore the post-`harden`-phase structural integrity to `scopes.md`
and `report.md` without re-bloating the artifacts back to their
pre-trim verbose form. Specifically:

- Restore bold `**Status:**` markers on all 5 scope headers.
- Re-add the 13 missing regression E2E planning requirements (10 DoD
  items + 3 Test Plan rows), all marked N/A with explicit
  artifact-only justification per the established BUG-020-006
  precedent (sweep R3).
- Restore the Scope 5 Consumer Impact Sweep section + S5-D12 DoD item
  and the S5-D13 change-boundary DoD item.
- Restore the 16 G040 skip-sentinel marker lines around the
  structurally-required narrative passages in `scopes.md` and
  `report.md`.
- Restore the two `harden`-phase rewords ("scheduled", "row") so
  Check 5A no longer mis-detects the spec as SLA-sensitive.

Workflow ceiling is `validate-to-doc` — artifact-only governance
closure with no runtime surface delta. The parent spec's
`status=specs_hardened` ceiling is preserved (NOT promoted to `done`).

## Acceptance Criteria

- AC-1: `state-transition-guard.sh specs/053-ci-ops-evidence-hardening`
  exits 0 with `🟡 TRANSITION PERMITTED` (or `🟢` permissive verdict)
  and **zero `🔴 BLOCK` findings**.
- AC-2: `state-transition-guard.sh
  specs/053-ci-ops-evidence-hardening/bugs/BUG-053-001-trim-broke-hardened-integrity`
  exits 0 with `🟡 TRANSITION PERMITTED` and zero `🔴 BLOCK` findings.
- AC-3: `artifact-lint.sh specs/053-ci-ops-evidence-hardening` and
  `artifact-lint.sh
  specs/053-ci-ops-evidence-hardening/bugs/BUG-053-001-trim-broke-hardened-integrity`
  both exit 0 (PASSED).
- AC-4: `traceability-guard.sh
  specs/053-ci-ops-evidence-hardening` exits 0 (RESULT: PASSED) with
  G068 fidelity still showing 7/7 mapped, 0 unmapped.
- AC-5: No file outside `specs/053-ci-ops-evidence-hardening/` is
  modified by the remediation commit. Specifically: zero
  runtime/source/CI/contract-test/framework files staged.

## Out of Scope

- The 13 advisory warnings the prior `harden` phase deliberately left
  in place (workflow-mode ceiling under spec-scope-hardening, plus the
  deprecated `scopeProgress` field-name drift between artifact-lint and
  state-transition-guard). Those remain owned by the framework
  follow-up routed to upstream Bubbles via the existing
  Framework-Boundary Record in Scope 5.
- Any runtime, source, CI, contract-test, deploy, or framework-file
  change. This bug is artifact-only by design.
- Restoring the verbose narrative the trim removed beyond what the
  guards structurally require. The trim's volume reduction intent is
  preserved; only its accidental structural damage is repaired.

## Linked Specifications

- Parent spec: [specs/053-ci-ops-evidence-hardening](../../spec.md)
- Parent design: [specs/053-ci-ops-evidence-hardening/design.md](../../design.md)
- Parent scopes: [specs/053-ci-ops-evidence-hardening/scopes.md](../../scopes.md)
- Parent report: [specs/053-ci-ops-evidence-hardening/report.md](../../report.md)
- Sibling precedent: [specs/020-security-hardening/bugs/BUG-020-006-governance-baseline-drift](../../../020-security-hardening/bugs/BUG-020-006-governance-baseline-drift/spec.md) (same validate-to-doc artifact-only closure pattern, sweep R3 2026-05-24)
- Sibling precedent: [specs/006-phase5-advanced/bugs/BUG-006-005-governance-baseline-drift](../../../006-phase5-advanced/bugs/BUG-006-005-governance-baseline-drift/spec.md) (same pattern, sweep R2 2026-05-24)
- Trim commit (root cause): `d4596c45` "spec(053): trim report + scopes after promote-to-hardened"
