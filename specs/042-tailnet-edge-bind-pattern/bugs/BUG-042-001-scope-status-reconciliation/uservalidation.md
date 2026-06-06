# User Validation: BUG-042-001 — Scope-Status Reconciliation (Certified-Done vs Not-Started Scopes)

**Closure status:** Resolved (artifact-only reconciliation; zero runtime impact)

## User-facing impact

- **Operators / DevOps:** No change. The fail-loud `HOST_BIND_ADDRESS` deploy
  compose contract, the Pattern P1 (`docker exec` over Tailscale SSH) infra access
  path, and the Pattern P5 (host Caddy) HTTP fronting continue to operate
  identically. The shipped contract was already enforced; this reconcile only
  aligns the planning truth (`scopes.md` statuses + DoD) with that shipped reality.
- **Auditors:** Spec 042's certified `done` status and its scope statuses now
  agree. `artifact-lint.sh specs/042-tailnet-edge-bind-pattern` returns PASSED. The
  `state.json` carries top-level `certifiedAt: 2026-06-06T17:30:00Z` plus a
  `bubbles.spec-review` CURRENT executionHistory entry verifying the spec is
  CURRENT against the fail-loud contract. One DoD item carries a transparent
  non-042 caveat (full unit suite red from `internal/assistant` + `tests/unit/
  clients`) rather than a fabricated pass.
- **End users:** Not applicable — spec 042 is internal deploy-contract
  infrastructure with no end-user surface.

## Acceptance

- AC-01..AC-10 from `spec.md` all pass; full evidence captured in `report.md`.
- The work is left uncommitted in the working tree; the parent batch-commits.

## Sign-off

Reconcile-to-doc (parent-expanded) terminates `completed_owned` for
`specs/042-tailnet-edge-bind-pattern`. The spec is internally consistent (`done` +
both scopes `Done` + `artifact-lint` PASSED). The single disclosed non-042 caveat
(unit-suite red owned by other specs) is tracked separately and does not represent
a spec-042 deliverable gap.

## Checklist

- [x] AC-01: `grep -cE '^- \[ \] ' specs/042-tailnet-edge-bind-pattern/scopes.md` returns 0 (no unchecked DoD items).
- [x] AC-02: Scope 1 and Scope 2 both carry `**Status:** Done`; the Active Scope Inventory table shows both `Done`.
- [x] AC-03: Parent `state.json` declares top-level `certifiedAt: 2026-06-06T17:30:00Z` and `certifiedBy: bubbles.workflow`.
- [x] AC-04: Parent `state.json::executionHistory` carries a `bubbles.spec-review` entry with `reviewStatus: CURRENT` and `runCompletedAt: 2026-06-06T17:25:00Z`.
- [x] AC-05: Parent `report.md` carries a `## Reconciliation Recertification` section with `### Validation Evidence` + `### Audit Evidence` subsections.
- [x] AC-06: `artifact-lint.sh specs/042-tailnet-edge-bind-pattern` returns PASSED (EXIT=0).
- [x] AC-07: `artifact-lint.sh specs/042-tailnet-edge-bind-pattern/bugs/BUG-042-001-scope-status-reconciliation` returns PASSED.
- [x] AC-08: `post-cert-spec-edit-guard.sh specs/042-tailnet-edge-bind-pattern` committed-history check is clean; only the uncommitted `scopes.md` edit is pending and clears on the parent's pre-`certifiedAt` commit.
- [x] AC-09: `internal/deploy/compose_contract_test.go` stays GREEN by construction (`go test -count=1 ./internal/deploy/ -run Compose` ok 0.040s).
- [x] AC-10: Exactly one DoD item carries a disclosed non-042 caveat (`./smackerel.sh test unit --go` full-suite red from `internal/assistant` + `tests/unit/clients`); it is NOT force-ticked as `EXIT=0`.
- [x] Zero `.go`, `.py`, `.yaml` (runtime config), `.sh`, `.ts`, `Dockerfile`, or `.github/workflows/*.yml` files are touched by BUG-042-001.
- [x] No DoD item was force-ticked: each re-tick is backed by a real command/file captured in `report.md`; the one whole-repo-gate item that is red is disclosed honestly.
