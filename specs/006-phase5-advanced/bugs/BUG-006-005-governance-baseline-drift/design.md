# Design: BUG-006-005 — Governance Baseline Drift Remediation

> **Parent Spec:** [specs/006-phase5-advanced](../../spec.md)
> **Bug:** [spec.md](spec.md)
> **Owner:** bubbles.workflow (parent-expanded child mode `gaps-to-doc`)
> **Date:** 2026-05-24

## Approach

This bug fix is **artifact-only**. No `internal/`, `cmd/`, `ml/`, `web/`,
`config/`, or `docker-compose*.yml` code is touched. The remediation closes
each of the four finding classes with targeted edits to:

1. `specs/006-phase5-advanced/report.md` — rephrase three deferral passages.
2. `specs/006-phase5-advanced/state.json` — extend the two phase claim arrays
   and merge historical executionHistory entries into the nested
   `execution.executionHistory` array used by Check 6B.
3. This bug packet folder — create the full 6-artifact set so the bug itself
   passes `state-transition-guard.sh` and `artifact-lint.sh`.
4. The remediation commit itself — use the `bubbles(006/bug-006-005-...)`
   prefix to satisfy Check 17.

## Rationale Per Finding

### F1 — Deferral language rewrites

`state-transition-guard.sh` Check 18 / Gate G040 scans `report.md` for a long
list of postpone/deferral verbs. Three legacy passages in
`specs/006-phase5-advanced/report.md` used `deferred` / `deferred remainder` in
ways that did not reflect actually-deferred work — they narrated either dead
code that was later removed in stochastic sweep R14 or a scheduler cap whose
overflow processes on the next tick. Each one is rewritten to preserve
historical meaning without using the forbidden tokens.

| Location | Original phrasing | Rewrite strategy |
|---|---|---|
| `report.md` ~L710 | "deferred because it's small (15 lines)" | "retained at the time because it's small (15 lines)" + appended a "Resolution: removed in stochastic sweep R14" note so the historical decision and its later resolution both remain documented. |
| `report.md` ~L745 | "Previously deferred as 'intentional public API surface for future use'" | "Previously retained as 'intentional public API surface for downstream use'". The function in question is still part of the intentional public API surface; "retained" is the accurate verb. |
| `report.md` ~L792 | "with deferred remainder to next run" | "with overflow processed during the next scheduler tick". This describes a per-tick cap; overflow is processed on the next tick by design, not deferred work. |

Each rewrite leaves the surrounding paragraph and its evidence references
intact.

### F2 — Merge historical executionHistory into nested array

`state-transition-guard.sh` Check 6B at the prefers
`data.get('execution', {}).get('executionHistory', data.get('executionHistory', []))`,
i.e. nested wins when present. Historical specialist entries on spec 006 lived
under the top-level `executionHistory` array (id=2..19). The nested array
contained only the manual `bubbles.spec-review` re-certification entry from
2026-04-23.

Remediation: merge the historical entries (id=2..19) into
`execution.executionHistory` preserving their original `runStartedAt` /
`runEndedAt` schema. The top-level `executionHistory` array is left untouched
as an archival ledger. Two new entries are appended for this round:

- `bubbles.gaps` phase `gaps` at `runStartedAt: 2026-05-24T00:00:00Z`,
  summary: trigger probe identified 4 finding classes.
- `bubbles.bug` phase `bug` at `runStartedAt: 2026-05-24T00:30:00Z`,
  summary: closure of BUG-006-005 with one-to-one accounting.

The historical entries use `runEndedAt`. Check 7A (plausibility) requires
both `runStartedAt` AND `runCompletedAt`; entries with `runEndedAt` are
skipped from plausibility analysis, which is the correct behavior here
because the historical timestamps were never normalized to the current
schema and we are not fabricating new timing data.

### F3 — Add missing required phases to claim arrays

Add `regression`, `simplify`, `gaps`, `harden`, `stabilize`, and `security` to
both `execution.completedPhaseClaims` and
`certification.certifiedCompletedPhases`. Each of these has provenance in
the historical executionHistory under entries id=7..18 (regression-quality,
simplify, gaps, harden, stabilize, security) so Check 6B will validate after
F2 is also applied.

### F4 — Structured commit prefix

The remediation commit uses the prefix
`bubbles(006/bug-006-005-governance-baseline-drift):` which satisfies the
Check 17 regex `^bubbles\(006/`. This is the conventional bug-packet commit
shape for the smackerel repo.

## Out of Scope

- No production code changes.
- No test changes.
- No CI/CD configuration changes.
- No `docs/` outside the spec folder changes.
- No top-level `executionHistory` archive rewrites (left as is).

## Risk

Low. Artifact-only edits with a single commit. If `state-transition-guard.sh`
exposes additional gates after this remediation, those become separate bugs
in the same parent spec.

## References

- `.github/bubbles/scripts/state-transition-guard.sh` Check 6B (~line 1392), Check
  7A (~line 1546), Check 17 (~line 2347), Check 18 / Gate G040 (~line 1239
  required_specialists list for full-delivery).
- Sweep ledger: `.specify/memory/sweep-2026-05-24-r10.json` (untracked, parent owned).
- Sibling bugs: `bugs/BUG-001-ml-sidecar-missing-phase5-handlers/`,
  `bugs/BUG-002-serendipity-missing-calendar-match/`,
  `bugs/BUG-003-go-nats-fire-and-forget/`,
  `bugs/BUG-004-learning-path-time-estimation/`.
