# User Validation: BUG-042-007 — Stale scenario-manifest.json + traceability-guard skip

**Closure status:** Resolved (planning-artifact reconciliation; zero runtime impact)

## User-facing impact

- **Operators / DevOps:** No change. The fail-loud `HOST_BIND_ADDRESS` deploy
  compose contract, Pattern P1 (`docker exec` over Tailscale SSH) infra access, and
  Pattern P5 (host Caddy) HTTP fronting operate identically. The shipped contract
  was already correct and enforced; this reconcile only fixed planning-artifact
  accuracy (`scenario-manifest.json`) and re-activated the traceability signal
  (`scopes.md` scenario format).
- **Auditors / framework:** Spec 042 now has a real traceability signal —
  `traceability-guard.sh specs/042-tailnet-edge-bind-pattern` exits 0 with the
  G057/G059 manifest cross-check ACTIVE (was exit 1 with the cross-check silently
  skipped). The manifest no longer contains the forbidden
  `${HOST_BIND_ADDRESS:-127.0.0.1}` form and its titles/types match the active
  fail-loud scopes. `artifact-lint.sh` remains PASSED.
- **End users:** Not applicable — spec 042 is internal deploy-contract
  infrastructure with no end-user surface.

## Acceptance

- AC-01..AC-07 from `spec.md` all pass; full evidence captured in `report.md`.
- The deployment surface (`deploy/compose.deploy.yml` +
  `internal/deploy/compose_contract_test.go`) is unchanged; all nine
  `TestComposeContract_*` functions PASS.
- The work is left uncommitted in the working tree.

## Sign-off

Planning-artifact reconcile (parent-expanded) terminates `completed_owned` for
`specs/042-tailnet-edge-bind-pattern`. The two coupled harden findings
(HARDEN-042-R33-001, HARDEN-042-R33-003) are remediated; the traceability guard
passes with the cross-check ACTIVE, artifact-lint is green, and the deployment
contract is intact. No status or certification change — spec 042 remains `done`.

## Checklist

- [x] AC-01: `grep -cE '^[[:space:]]*Scenario( Outline)?:' specs/042-tailnet-edge-bind-pattern/scopes.md` returns 6 and `scenario-manifest.json` covers 6 scenarioIds.
- [x] AC-02: `grep -rn 'HOST_BIND_ADDRESS:-' specs/042-tailnet-edge-bind-pattern/scenario-manifest.json` returns nothing (no forbidden fallback form in the manifest).
- [x] AC-03: `traceability-guard.sh specs/042-tailnet-edge-bind-pattern` returns PASSED (exit 0) with the G057/G059 cross-check ACTIVE (covers 6 scenario contracts), NOT "scenario manifest cross-check skipped".
- [x] AC-04: `artifact-lint.sh specs/042-tailnet-edge-bind-pattern` returns PASSED (EXIT=0).
- [x] AC-05: `./smackerel.sh test unit --go --go-run 'TestComposeContract'` exits 0 — all nine `TestComposeContract_*` functions PASS.
- [x] AC-06: `artifact-lint.sh specs/042-tailnet-edge-bind-pattern/bugs/BUG-042-007-stale-scenario-manifest-and-traceability-skip` returns PASSED.
- [x] AC-07: Parent `state.json::resolvedBugs[]` and `executionHistory[]` carry an entry for this packet; the parent `report.md` carries a Planning-Artifact Reconciliation section.
- [x] Zero `.go`, `.py`, `.yaml` (runtime config), `.sh`, `.ts`, `Dockerfile`, or `.github/workflows/*.yml` files are touched; `deploy/compose.deploy.yml` and `internal/deploy/compose_contract_test.go` are unchanged.
- [x] No `${HOST_BIND_ADDRESS:-127.0.0.1}` fallback form is reintroduced in the active `scopes.md` gherkin or `scenario-manifest.json`; the NO-DEFAULTS / fail-loud SST contract is preserved.
