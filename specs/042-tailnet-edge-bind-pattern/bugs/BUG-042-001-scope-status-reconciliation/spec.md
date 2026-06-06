# Spec: BUG-042-001 — Scope-Status Reconciliation (Certified-Done vs Not-Started Scopes)

**Parent spec:** 042-tailnet-edge-bind-pattern
**Type:** Bug (governance / artifact reconciliation)
**Workflow mode:** bugfix-fastlane
**Status:** done

---

## Problem Statement

`specs/042-tailnet-edge-bind-pattern` is certified `done` (`state.json status:
done`, `certification.status: done`) yet its `scopes.md` declared both active
scopes `Not started` with 26 unchecked DoD items. The certified status and the
scope statuses are inconsistent, and `artifact-lint.sh` rejects the spec with
`state.json status 'done' is invalid: DoD contains unchecked items`. The
inconsistency originates from the 2026-05-25 reconciliation commit `15e1c453`
which flipped the `HOST_BIND_ADDRESS` contract to fail-loud and reset both scopes
to `Not started` without recording the re-verification. The fail-loud
implementation HAS shipped and is enforced (the contract test
`internal/deploy/compose_contract_test.go` is GREEN with adversarial coverage).

## Goal

Reconcile the planning truth to the shipped+tested reality: re-verify every DoD
item against real code/docs evidence, re-tick only genuinely-satisfied items,
restore both scopes to `Done`, and recertify the parent so the certified `done`
status and the scope statuses agree — without force-ticking and without touching
runtime source.

## Behavioural Requirements

- **REQ-1 (No force-tick):** A DoD item is re-ticked `[x]` ONLY when a real
  command/file proves it is satisfied by shipped+tested code/docs. Any genuinely
  unsatisfied item stays `[ ]` and is reported as a true gap.
- **REQ-2 (Fail-loud preserved):** The reconciliation introduces no fallback; the
  NO-DEFAULTS / fail-loud SST contract (Gate G028) remains intact.
- **REQ-3 (Recertify on planning-truth edit):** Because the reconciliation edits
  planning truth (`scopes.md`) on a `done` spec, the parent is recertified with a
  top-level `certifiedAt`, a `bubbles.spec-review` CURRENT executionHistory entry,
  and an updated `lastUpdatedAt`.
- **REQ-4 (Artifact-only, zero runtime change):** No `.go`/`.py`/`.yaml`/`.sh`
  source is touched; the fail-loud contract is already shipped.
- **REQ-5 (Honest disclosure):** Any DoD item whose whole-repo gate is red for
  reasons outside spec 042's change boundary is disclosed as a non-042 caveat,
  not hidden and not force-ticked.

## Acceptance Criteria

- **AC-01:** `grep -cE '^- \[ \] ' specs/042-tailnet-edge-bind-pattern/scopes.md`
  returns `0`.
- **AC-02:** Scope 1 and Scope 2 both carry `**Status:** Done`; the Active Scope
  Inventory table shows both `Done`.
- **AC-03:** Parent `state.json` carries top-level `certifiedAt:
  2026-06-06T17:30:00Z` and `certifiedBy`.
- **AC-04:** Parent `state.json::executionHistory` carries a `bubbles.spec-review`
  entry with `reviewStatus: CURRENT` and `runCompletedAt: 2026-06-06T17:25:00Z`.
- **AC-05:** Parent `report.md` carries a `## Reconciliation Recertification`
  section with `### Validation Evidence` + `### Audit Evidence` subsections.
- **AC-06:** `artifact-lint.sh specs/042-tailnet-edge-bind-pattern` returns
  PASSED.
- **AC-07:** `artifact-lint.sh
  specs/042-tailnet-edge-bind-pattern/bugs/BUG-042-001-scope-status-reconciliation`
  returns PASSED.
- **AC-08:** `post-cert-spec-edit-guard.sh specs/042-tailnet-edge-bind-pattern`
  committed-history check is clean (`git log --since=certifiedAt` over the
  planning files returns nothing); only the uncommitted `scopes.md` edit is
  pending and clears on the parent's pre-`certifiedAt` commit.
- **AC-09:** The fail-loud compose contract regression (`internal/deploy/compose_
  contract_test.go`) stays GREEN by construction (`go test ... -run Compose` ok).
- **AC-10:** Exactly one DoD item carries a disclosed non-042 caveat
  (`./smackerel.sh test unit --go` full-suite red from `internal/assistant` +
  `tests/unit/clients`); it is NOT force-ticked as `EXIT=0`.

## Non-Goals

- Fixing the `internal/assistant` tool-registry failure or installing
  `node`/`dart` for `tests/unit/clients` — both belong to other specs.
- Migrating the deprecated `state.json` schema fields (`scopeProgress`,
  `statusDiscipline`, `scopeLayout`) — framework v2→v3 migration territory.
- Any change to the shipped fail-loud compose contract or its tests.
