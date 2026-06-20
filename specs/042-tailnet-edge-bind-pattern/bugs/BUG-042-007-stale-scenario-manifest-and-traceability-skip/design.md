# Design: BUG-042-007 — Stale scenario-manifest.json + traceability-guard skip

**Parent spec:** 042-tailnet-edge-bind-pattern
**Workflow mode:** bugfix-fastlane (planning-artifact reconcile; zero runtime change)

---

## Current Truth (grounded in real probes)

| Probe | Result (pre-fix) |
|-------|------------------|
| `grep -cE '^[[:space:]]*Scenario( Outline)?:' scopes.md` | `0` (bullet format `- **SCN-042-NNN - title**`) |
| `traceability-guard.sh specs/042-tailnet-edge-bind-pattern` | exit 1; "No scope-defined Gherkin scenarios found — scenario manifest cross-check skipped" then silent `set -e` exit on the next stale `## Scope N:` heading |
| `scenario-manifest.json` SCN-042-001 `then` | carries forbidden `${HOST_BIND_ADDRESS:-127.0.0.1}:` |
| `scenario-manifest.json` SCN-042-003 title | "Compose default is safe for local runs" (superseded) |
| `scenario-manifest.json` SCN-042-004/005 | titles/scope shuffled vs active scopes; `requiredTestType` `e2e-api`/`e2e-ui` |
| `deploy/compose.deploy.yml` L128/L190 | fail-loud `${HOST_BIND_ADDRESS:?...}` for core + ml (CORRECT, untouched) |
| `go`/`./smackerel.sh test unit --go --go-run TestComposeContract` | nine `TestComposeContract_*` PASS, `ok internal/deploy` (deployment intact) |

The guard's `extract_scenarios` greps `^[[:space:]]*Scenario:`; the active
`scopes.md` used `- **SCN-042-NNN - title**` bullets so it matched nothing. The
file also carried a second, HTML-commented duplicate scope set whose
`## Scope N:` headings the guard still matched (it does not honour HTML comments),
triggering the silent `set -e` exit.

## Fix Architecture (2 layers, artifact-only)

### Layer 1 — scopes.md (activate the guard, preserve fail-loud semantics)

1. Reformat active `SCN-042-001..006` from `- **SCN-042-NNN - title**` bullets to a
   single `Scenario: SCN-042-NNN — title` per scope inside a column-0 ```gherkin
   block (mirroring `specs/035-recipe-enhancements/scopes.md`), preserving the
   fail-loud Given/When/Then verbatim (no `:-127.0.0.1` reintroduced).
2. Relabel the two stale HTML-commented duplicate `## Scope N:` headings to
   `## Superseded Scope N —` so they no longer match the guard's active-scope regex
   `^##[[:space:]]+Scope[[:space:]]+[0-9]+:`.
3. Neutralize `#### Core Items` / `#### Build Quality Gate` to `**bold**` labels so
   the guard's `extract_dod_items` (which exits at any `####` heading) returns the
   DoD items for the G068 fidelity pass.
4. Prefix one DoD item per scenario with `Scenario SCN-042-NNN (title) —` so the
   G068 fidelity matcher binds each scenario to a DoD item by trace ID.

### Layer 2 — scenario-manifest.json (realign to active fail-loud scopes)

Rewrite all six entries to mirror the active scopes: remove the forbidden
`:-127.0.0.1` from `SCN-042-001`; retitle `SCN-042-003` → "Missing bind address
fails loud"; reassign/retitle `SCN-042-004` (explicit-loopback, scope 01) and
`SCN-042-005` (ops-doc, scope 02); reconcile `requiredTestType` from
`e2e-api`/`e2e-ui` to `unit`/`doc-lint` to match the linked compose-contract and
doc-lint checks; correct the linked test IDs to real `TestComposeContract_*`
function names; drop the placeholder `gherkinHash` fields.

No Layer 3 runtime change: `deploy/compose.deploy.yml` and
`internal/deploy/compose_contract_test.go` are NOT touched; the fix is verified
against them read-only.

## Anti-Fabrication Discipline

The fix is proven scenario-first red→green: the **red** state is
`traceability-guard.sh` exit 1 with the cross-check skipped (0 scenarios); the
**green** state is exit 0 with the G057/G059 cross-check ACTIVE (6 contracts,
6/6 scenario→row→file→report, 6/6 G068 DoD fidelity). The deployment-intact claim
is backed by a real `./smackerel.sh test unit --go --go-run TestComposeContract`
run (nine `TestComposeContract_*` PASS, `ok internal/deploy`). No DoD item is
force-ticked.

## Rollback

Pure `git revert` of the parent artifact edits restores the stale manifest +
bullet-format scenarios (guard skip); zero runtime impact.
