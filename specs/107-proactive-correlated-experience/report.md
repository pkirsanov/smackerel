# Report: 107 Proactive & Correlated Experience

## Summary

This packet records planning-owner (`bubbles.plan`) work only. It is the fourth
and final phase of the `product-to-planning` workflow (ceiling `specs_hardened`),
following the analyst (`spec.md`), UX (`## UI Wireframes` + `## User Flows` in
`spec.md`), and design (`design.md`) phases. It defines the execution scopes,
scenario contracts, test inventory, SST no-default decisions, scope DAG, and
implementation handoff for the proactive and correlated experience. It does NOT
claim source, authored test, test pass, migration, browser, deployment, commit,
or push execution.

The plan decomposes all six improvements P1–P6 across web PWA + Telegram +
WhatsApp into **9 sequential, dependency-ordered, independently testable scopes**
using the per-scope-directory layout (`scopes/NN-name/scope.md` + `scopes/_index.md`),
matching the spec-105 / spec-106 house style. All 20 SCN-107 scenarios are mapped
one-to-one to owning scopes; every scenario has concrete `unit`, `integration`,
`e2e-api`, and `e2e-ui` planning (80 scenario rows) plus supplemental canary,
stress, anti-leak, non-collision, restart, contrast, telemetry, and acceptance
rows. Every card originates from the single spec-078 surfacing controller; the
rail is a tighter-bounded call of the spec-105 neighborhood; the palette consumes
the spec-074/061 capture/turn; no second store, second budget, client cache, or
owner edit is introduced. `specs/105-*`, `specs/106-*`, `specs/072-*`, and
`specs/078-*` were referenced only and NOT modified.

## Scope Inventory (9 scopes, sequential)

| # | Scope | Owned Scenarios |
|---|---|---|
| 01 | Single-controller card projection & nudge-ack foundation (`foundation:true`) | SCN-107-004, 008, 009 |
| 02 | Web proactive card & authenticated action transport | SCN-107-003 |
| 03 | Telegram & WhatsApp nudge renderings + cross-channel parity | SCN-107-005, 006, 007 |
| 04 | Today cockpit composition (spec-106 `Today` body) | SCN-107-001, 002, 017 |
| 05 | Correlation rail (bounded spec-105 neighborhood + deep-link) | SCN-107-010, 011 |
| 06 | Ask-or-capture command palette | SCN-107-012, 013 |
| 07 | What-changed feed | SCN-107-014, 015 |
| 08 | Cross-surface accessibility, responsive & authorization hardening | SCN-107-018, 019, 020 |
| 09 | Real-stack acceptance & implementation handoff | SCN-107-016 (+ acceptance rerun SCN-107-001..020) |

## SST No-Default Decisions (Reserved For Implementation)

| SST key | Decision (MVP) | Owning scope |
|---|---|---|
| `nudge_ref_ttl_hours` | `6` (≥ max(suppression_window_hours=4, dedupe_window_hours=6)) | SCOPE-01 |
| snooze window | reuse `suppression_window_hours` (no distinct `snooze_window_hours` for MVP) | SCOPE-01 / SCOPE-03 |
| `RAIL_MAX` | `6` (neighborhood bound; never renders the full store) | SCOPE-05 |
| `what_changed_page_cap` | `25` (per column, per page) | SCOPE-07 |

All become fail-loud SST keys under `config/smackerel.yaml` at implementation
(NOT edited this phase); config-compile validates presence/type/bounds with no
`${VAR:-default}` / `os.getenv(k, default)` / `unwrap_or` fallback.

## Planning Provenance

- Requirements source: `spec.md` (20 SCN-107 scenarios, 30 FR-107, `## UI Wireframes`, `## User Flows`)
- Design source: `design.md` (typed contracts, single-controller routing, dependency/ownership map)
- Compose-over dependencies (not modified): `specs/106-coherent-product-experience`, `specs/105-connected-knowledge-graph-explorer`
- Consume-only dependencies (not modified): `specs/078-*`, `specs/072-*`, `specs/074-*`, `specs/061-*`, `specs/073-*`, `specs/054-*`, `specs/055-*`
- Planning owner: `bubbles.plan`
- Implementation / test / validation owners: `bubbles.implement` / `bubbles.test` / `bubbles.validate` at pickup

## Test Evidence

No implementation test evidence belongs to this planning invocation. The
following records the planning-artifact validation guards executed this session.

### Artifact Lint

**Executed:** YES
**Command:** `bash .github/bubbles/scripts/artifact-lint.sh specs/107-proactive-correlated-experience`
**Exit Code:** 0

```text
✅ Required artifact exists: spec.md
✅ Required artifact exists: design.md
✅ Required artifact exists: uservalidation.md
✅ Required artifact exists: state.json
✅ Required artifact exists: scopes/_index.md
✅ Per-scope layout contains 9 scope file(s)
✅ Scope report exists: scopes/01-single-controller-card-projection-foundation/report.md
✅ Scope report exists: scopes/02-web-proactive-card-action-transport/report.md
✅ Scope report exists: scopes/03-telegram-whatsapp-nudge-parity/report.md
✅ Scope report exists: scopes/04-today-cockpit-composition/report.md
✅ Scope report exists: scopes/05-correlation-rail/report.md
✅ Scope report exists: scopes/06-ask-or-capture-command-palette/report.md
✅ Scope report exists: scopes/07-what-changed-feed/report.md
✅ Scope report exists: scopes/08-cross-surface-accessibility-responsive-authorization/report.md
✅ Scope report exists: scopes/09-real-stack-acceptance-handoff/report.md
✅ Every per-scope directory has a report.md file
✅ No forbidden sidecar artifacts present
✅ Found DoD section in scopes/01-single-controller-card-projection-foundation/scope.md
✅ All DoD bullet items use checkbox syntax in scopes/01-single-controller-card-projection-foundation/scope.md
... (DoD checkbox + checklist + report-section checks pass for every scope) ...
✅ Found Checklist section in uservalidation.md
✅ uservalidation checklist has checked-by-default entries
✅ Detected state.json status: not_started
✅ Detected state.json workflowMode: full-delivery
✅ Top-level status matches certification.status

=== Anti-Fabrication Evidence Checks ===
✅ All checked DoD items in scopes/01-...-foundation/scope.md have evidence blocks
✅ No unfilled evidence template placeholders in scopes/09-real-stack-acceptance-handoff/report.md
✅ No repo-CLI bypass detected in scopes/09-real-stack-acceptance-handoff/report.md command evidence
=== End Anti-Fabrication Checks ===

Artifact lint PASSED.
ARTIFACT_LINT_EXIT=0
```

### Traceability Guard

**Executed:** YES
**Command:** `bash .github/bubbles/scripts/traceability-guard.sh specs/107-proactive-correlated-experience`
**Exit Code:** 0

```text
============================================================
  BUBBLES TRACEABILITY GUARD
  Feature: <repo-root>/specs/107-proactive-correlated-experience
  Timestamp: 2026-07-24T18:32:34Z
============================================================

--- Scenario Manifest Cross-Check (G057/G059) ---
✅ scenario-manifest.json covers 20 scenario contract(s)
✅ scenario-manifest.json records evidenceRefs
✅ All linked tests from scenario-manifest.json exist

(every scope: each scenario maps to a Test Plan row, to a concrete test file `./smackerel.sh`, and to a report evidence reference; each scenario maps to a DoD item)

--- Gherkin → DoD Content Fidelity (Gate G068) ---
ℹ️  DoD fidelity: 20 scenarios checked, 20 mapped to DoD, 0 unmapped

--- Traceability Summary ---
ℹ️  Scenarios checked: 20
ℹ️  Test rows checked: 107
ℹ️  Scenario-to-row mappings: 20
ℹ️  Concrete test file references: 20
ℹ️  Report evidence references: 20
ℹ️  DoD fidelity scenarios: 20 (mapped: 20, unmapped: 0)
ℹ️  Edge confidence (IMP-015 Scope B): declared=24 inferred=0 ambiguous=16

RESULT: PASSED (0 warnings)
TRACEABILITY_EXIT=0
```

## Completion Statement

The planning package is complete: `spec.md` + `design.md` + `scopes/` (9 scopes,
per-scope-directory layout) + `scenario-manifest.json` (20 scenarios, all tests
PLANNED / 0 linked) + `test-plan.json` + `state.json` + `report.md` +
`uservalidation.md`. Both requested planning guards exit 0. The feature is
`not_started` under the planning-complete convention (orchestration ceiling
`specs_hardened`); every scope remains `Not Started` and every Definition of Done
item remains unchecked. No implementation, authored-test, test-pass, migration,
browser, deployment, commit, or push claim is made. The feature is ready for
implementation pickup by `bubbles.implement`.
