# Spec: BUG-042-007 — Stale scenario-manifest.json + traceability-guard skip

**Parent spec:** 042-tailnet-edge-bind-pattern
**Type:** Bug (governance / planning-artifact reconciliation)
**Workflow mode:** bugfix-fastlane
**Status:** done

---

## Problem Statement

`specs/042-tailnet-edge-bind-pattern` is certified `done` (`certifiedAt
2026-06-06`). A stochastic-quality-sweep harden probe (Round 33) surfaced two
coupled planning-artifact findings:

- **F1 (HARDEN-042-R33-001):** `scenario-manifest.json` is pre-supersession stale —
  `SCN-042-001`'s `then` clause carries the FORBIDDEN
  `${HOST_BIND_ADDRESS:-127.0.0.1}:` default-fallback form, `SCN-042-003` is titled
  "Compose default is safe for local runs" (superseded framing), and
  `SCN-042-004/005` titles are shuffled against the active fail-loud scopes.
- **F2 (HARDEN-042-R33-003):** active `scopes.md` uses a
  `- **SCN-042-NNN - title**` header format, so `traceability-guard.sh`'s
  `extract_scenarios` finds ZERO scenarios and the run exits 1 silently under
  `set -e`, skipping the G057/G059 manifest cross-check for spec 042.

The deployment safety surface (the fail-loud `deploy/compose.deploy.yml` bind and
the `internal/deploy/compose_contract_test.go` contract test) is correct and
intact; these are planning-artifact accuracy/traceability defects only.

## Goal

Reconcile the planning artifacts to the active fail-loud reality and make the
traceability signal real: reformat the active scenarios so the guard parses them,
realign the manifest so it reflects the fail-loud scopes, and confirm the
deployment contract is unchanged — without touching compose or the contract test
and without reintroducing any `:-127.0.0.1` fallback form.

## Behavioural Requirements

- **REQ-1 (Fail-loud preserved):** No `${HOST_BIND_ADDRESS:-127.0.0.1}` form is
  introduced into the active `scopes.md` gherkin or `scenario-manifest.json`; the
  NO-DEFAULTS / fail-loud SST contract (Gate G028) is preserved.
- **REQ-2 (Guard activation):** The active `SCN-042-001..006` scenarios are
  reformatted to `Scenario: SCN-042-NNN — title` inside ```gherkin blocks so
  `extract_scenarios` finds them and the G057/G059 manifest cross-check becomes
  ACTIVE.
- **REQ-3 (Manifest fidelity):** All six manifest entries are realigned to the
  active scopes — fail-loud `then` clauses, correct titles, `requiredTestType`
  reconciled to the actual linked tests (compose-contract `unit` and `doc-lint`,
  not `e2e`), and real `TestComposeContract_*` linked test IDs.
- **REQ-4 (Deployment intact):** `deploy/compose.deploy.yml` and
  `internal/deploy/compose_contract_test.go` are NOT modified; all nine
  `TestComposeContract_*` functions continue to PASS.
- **REQ-5 (No status change):** The parent's `done` status and certification are
  unchanged; only planning artifacts plus `report.md`/`state.json` are touched.

## Acceptance Criteria

- **AC-01:** `grep -cE '^[[:space:]]*Scenario( Outline)?:' scopes.md` returns `6`
  and `scenario-manifest.json` covers 6 scenarioIds.
- **AC-02:** `grep -rn 'HOST_BIND_ADDRESS:-' scenario-manifest.json` returns
  nothing (no forbidden fallback form in the manifest).
- **AC-03:** `traceability-guard.sh specs/042-tailnet-edge-bind-pattern` returns
  PASSED (exit 0) and prints the G057/G059 cross-check as ACTIVE (covers 6 scenario
  contracts), NOT "scenario manifest cross-check skipped".
- **AC-04:** `artifact-lint.sh specs/042-tailnet-edge-bind-pattern` returns PASSED.
- **AC-05:** `./smackerel.sh test unit --go --go-run 'TestComposeContract'` exits 0
  — all nine `TestComposeContract_*` functions PASS.
- **AC-06:** `artifact-lint.sh
  specs/042-tailnet-edge-bind-pattern/bugs/BUG-042-007-stale-scenario-manifest-and-traceability-skip`
  returns PASSED.
- **AC-07:** Parent `state.json::resolvedBugs[]` and `executionHistory[]` carry an
  entry for this packet; the parent `report.md` carries a Planning-Artifact
  Reconciliation section.

## Non-Goals

- Modifying `deploy/compose.deploy.yml` or `internal/deploy/compose_contract_test.go`
  — the deployment surface is correct.
- Removing the HTML-commented superseded duplicate scope block — only the two
  stale `## Scope N:` headings inside it are relabeled so the guard regex no longer
  matches them.
- Migrating the deprecated parent `state.json` schema fields (`scopeProgress`,
  `statusDiscipline`, `scopeLayout`) — framework v2→v3 territory, non-blocking.
- Committing or touching the ~130 accumulated uncommitted worktree files or any
  other spec.
