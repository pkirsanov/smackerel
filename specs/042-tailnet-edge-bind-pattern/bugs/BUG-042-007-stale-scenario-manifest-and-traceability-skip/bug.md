# BUG-042-007 — Stale scenario-manifest.json + traceability-guard silently skips spec 042

**Spec:** 042-tailnet-edge-bind-pattern
**Severity:** Governance / planning-artifact drift (artifact-only; zero runtime change)
**Status:** open → resolved
**Discovered:** 2026-06-17
**Discovered by:** stochastic-quality-sweep harden probe Round 33 (findings HARDEN-042-R33-001, HARDEN-042-R33-003)
**Closure Mode:** bugfix-fastlane (planning-artifact reconciliation; mirrors BUG-042-001 precedent)

---

## Summary

Two coupled planning-artifact findings on the certified-`done`
`specs/042-tailnet-edge-bind-pattern`. Both are planning-artifact
accuracy/traceability drift, **NOT a runtime regression**: the deployment safety
surface (`deploy/compose.deploy.yml` fail-loud bind +
`internal/deploy/compose_contract_test.go` with nine `TestComposeContract_*`
functions) was intact and remained untouched.

- **F1 (HARDEN-042-R33-001, medium):** `scenario-manifest.json` was
  pre-supersession stale. `SCN-042-001`'s `then` clause carried the FORBIDDEN
  `${HOST_BIND_ADDRESS:-127.0.0.1}:` default-fallback form (contradicting the
  active NO-DEFAULTS / fail-loud SST policy in
  `.github/instructions/smackerel-no-defaults.instructions.md`); `SCN-042-003`
  was titled "Compose default is safe for local runs" (superseded loopback-default
  framing); `SCN-042-004/005` titles were shuffled relative to the active
  fail-loud set in `scopes.md`.
- **F2 (HARDEN-042-R33-003, medium):** active `scopes.md` used a
  `- **SCN-042-NNN - title**` header format, so the traceability guard's
  `extract_scenarios` (which greps `^[[:space:]]*Scenario:`) found ZERO scenarios.
  Under `set -euo pipefail` the per-scope loop's `scenarios="$(extract_scenarios …)"`
  assignment exited 1 silently, so spec 042 received NO traceability signal and the
  G057/G059 manifest cross-check was skipped. Peer specs 005 and 035 use
  `Scenario: SCN-NNN — title` inside a ```gherkin block and pass.

## Coupling

The two findings MUST be fixed together. Reformatting the `scopes.md` scenarios
(F2) activates the G057/G059 manifest cross-check, which then validates against the
manifest (F1). Fixing only one leaves either no validation (F2 alone) or a
newly-failing cross-check (F1 alone against the stale manifest).

## Root Cause

The 2026-06-06 reconciliation (`bubbles.spec-review`, see BUG-042-001) updated the
active `scopes.md` to the fail-loud contract but did NOT propagate the change to
`scenario-manifest.json`, and it left the active scenarios in the
`- **SCN-042-NNN - title**` bullet format that the traceability guard cannot
parse. The manifest therefore froze a pre-supersession snapshot (the forbidden
`:-127.0.0.1` form, the "Compose default is safe" framing, and shuffled
004/005 titles), and the guard silently no-op'd on spec 042.

## Scope

**In-scope (planning-artifact mutations only):**

- `specs/042-tailnet-edge-bind-pattern/scopes.md` — reformat active
  `SCN-042-001..006` to the working `Scenario: SCN-042-NNN — title` gherkin form;
  relabel the two stale HTML-commented duplicate `## Scope N:` headings to
  `## Superseded Scope N —`; neutralize the `#### Core Items` /
  `#### Build Quality Gate` subheadings to bold labels; add per-scenario trace-ID
  prefixes to one DoD item each.
- `specs/042-tailnet-edge-bind-pattern/scenario-manifest.json` — realign all six
  entries to the active fail-loud scopes.
- `specs/042-tailnet-edge-bind-pattern/report.md` — append the
  Planning-Artifact Reconciliation section.
- `specs/042-tailnet-edge-bind-pattern/state.json` — append a `bubbles.plan`
  executionHistory entry + a `resolvedBugs[]` entry; update `lastUpdatedAt`.
- `specs/042-tailnet-edge-bind-pattern/bugs/BUG-042-007-stale-scenario-manifest-and-traceability-skip/`
  — this packet (8 artifacts).

**Out-of-scope (NOT touched):**

- `deploy/compose.deploy.yml` and `internal/deploy/compose_contract_test.go` — the
  deployment surface is correct and intact; verifying intactness only.
- `specs/042-tailnet-edge-bind-pattern/spec.md`, `design.md`, `uservalidation.md` —
  not touched.
- Any `.go`, `.py`, `.yaml`, `.sql`, `.sh`, `.ts`, `.tsx` runtime source.
- `.github/bubbles/**` framework scripts.
- Any other spec folder; the ~130 accumulated uncommitted worktree files are not
  committed or otherwise touched.

## Acceptance

- No forbidden `${HOST_BIND_ADDRESS:-127.0.0.1}` form remains in the active
  `scopes.md` gherkin or anywhere in `scenario-manifest.json`.
- `bash .github/bubbles/scripts/traceability-guard.sh specs/042-tailnet-edge-bind-pattern`
  returns **PASSED (exit 0)** with the G057/G059 manifest cross-check ACTIVE (not
  skipped).
- `bash .github/bubbles/scripts/artifact-lint.sh specs/042-tailnet-edge-bind-pattern`
  returns **PASSED**.
- `./smackerel.sh test unit --go --go-run 'TestComposeContract'` exits 0 — all nine
  `TestComposeContract_*` functions PASS (deployment surface intact).
- No status or certification change on the parent — spec 042 remains `done`.
