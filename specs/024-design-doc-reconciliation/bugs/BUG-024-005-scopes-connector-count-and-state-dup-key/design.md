# Design: BUG-024-005 Residual reconciliation drift closure

## Current Truth (grounded in real probes, 2026-06-06)

| Surface | Pre-fix value | Source of truth | Probe |
|---------|---------------|-----------------|-------|
| `state.json` top-level `lastUpdatedAt` | duplicated (header `2026-06-06T00:00:00Z` + legacy `2026-05-25T00:00:00Z`) → effective stale `2026-05-25` | should be singular, fresh | `python3` `raw.count('"lastUpdatedAt"') == 2`; `object_pairs_hook` flags `['lastUpdatedAt']` |
| `scopes.md` Scope 2 body connector count | `15` at 8 substantive sites; `SCN-024-06` list omits `qfdecisions` | live registry = 16 | `grep -nE '15 (connectors|committed)' scopes.md` → 8 substantive + 7 historical hits |
| `cmd/core/connectors.go` | 16 connector constructors (`qfDecisionsConn` line 52) | runtime source of truth | `grep -c 'Conn :=' cmd/core/connectors.go` |
| `spec.md` R-006 / BS-004 / AC-5 | `16` (incl. `qfdecisions`) | already reconciled (BUG-024-003) | `grep -nE '16' spec.md` |
| `scenario-manifest.json` `SCN-024-06` | title "lists all 16 connectors"; `wc -l == 16` | already reconciled (BUG-024-002) | `grep -n '16' scenario-manifest.json` |
| `docs/smackerel.md` §22.7 / §24-A | `(16 connectors)` / `(16 committed)` | already reconciled (BUG-024-002) | `grep -n '16 connectors' docs/smackerel.md` |
| 5 framework guards at baseline | all GREEN | — | both findings are LATENT / guard-invisible |

## Architecture (3 Layers)

### Layer 1 — `state.json` F1 dedup + recert
- Remove the legacy `"lastUpdatedAt": "2026-05-25T00:00:00Z"` line after `failures[]` (keep the single header key).
- Advance header `certifiedAt` `2026-05-28T05:07:51Z` → `2026-06-06T18:00:00Z` and `lastUpdatedAt` `2026-06-06T00:00:00Z` → `2026-06-06T18:00:00Z`.
- Append a `bubbles.spec-review` `reviewStatus: CURRENT` `executionHistory` entry.
- Do NOT set `requiresRevalidation:true` (G089 on a done spec).

### Layer 2 — `scopes.md` F2 8-site reconciliation
- Change 8 substantive Scope 2 connector-count claims 15 → 16 (validation checkpoint L17, scope-summary table L24, `SCN-024-04` body L214, `SCN-024-06` title L223, `SCN-024-06` body L226 + insert `qfdecisions`, test-plan count row L250, DoD bullet L288, DoD evidence L289).
- Change the `SCN-024-06` Gherkin title AND its DoD bullet in lockstep so G068 fidelity stays green.
- Preserve the 7 historical "15" references verbatim (rollback `git revert` contract L356/L386, stale-ref grep searches L363/L443, BUG-024-002 red-phase TDD evidence L422, consumer-impact "zero stale 15-refs" assertions L433-435).

### Layer 3 — Parent governance backfill
- `executionHistory[]` += harden-sweep closure phases (bug, analyze, design, plan, harden, implement, test, regression, simplify, stabilize, security, chaos, validate, audit, docs, finalize) + the `bubbles.spec-review` CURRENT entry (17 entries).
- `resolvedBugs[]` += BUG-024-005 entry (`finalStatus: resolved`).
- `report.md` += `## BUG-024-005 Harden-Sweep Resolution (2026-06-06)` with Code Diff Evidence + Git-Backed Proof (PII-redacted to `~/`).

**No Layer 4 forward-detection guard.** The connector-count drift class is already covered by BUG-024-003's `TestConnectorCountContract` + the R-006 directory-count canary documented in `scopes.md`. The duplicate-JSON-key class is a one-off authoring slip from BUG-024-004's manual backfill, not a recurring drift vector that warrants a bespoke guard; adding one would be speculative over-engineering.

## Iteration Plan

1. **Iteration 1 (F1):** Remove legacy `lastUpdatedAt`; recert header `certifiedAt`/`lastUpdatedAt`. Verify: `python3` duplicate-key detector → none; literal count 2→1.
2. **Iteration 2 (F2):** `multi_replace_string_in_file` the 8 substantive `scopes.md` sites 15→16 + insert `qfdecisions`. Verify: `grep` shows only historical "15" remain; `SCN-024-06` body lists `qfdecisions`.
3. **Iteration 3 (Layer 3):** Append 17 `executionHistory` entries + `resolvedBugs[]` BUG-024-005; `python3` re-validates JSON + entry counts.
4. **Iteration 4 (verify):** Run all 5 parent guards + `TestConnectorCountContract`. Content guards green; G088/STG in designed uncommitted-handoff state (postCertEdits=1 worktree scopes.md). Capture real evidence.
5. **Iteration 5 (packet + report):** Author 8 BUG-024-005 artifacts with real evidence; append parent `report.md`; run bug-folder `artifact-lint` until PASSED. NO commit, NO push.

## Verification Checklist

- [x] `state.json` valid JSON, single `lastUpdatedAt`, recert applied, `status: done` preserved.
- [x] `scopes.md` 8 substantive sites = 16; `qfdecisions` inserted; 7 historical "15" preserved.
- [x] STG Check 22 (G068) all 6 scenarios faithful (SCN-024-06 lockstep).
- [x] artifact-lint PASSED; artifact-freshness PASS; traceability PASSED (6/6).
- [x] `TestConnectorCountContract` 4/4 PASS.
- [x] G088/STG Check 30 honest worktree-edit state documented (not faked green).
- [x] `resolvedBugs[]` BUG-024-005; `report.md` closure section; zero `/home/<user>/`.
- [x] Bug-folder artifact-lint PASSED.
- [x] Nothing committed or pushed.

## Shared Infrastructure Impact Sweep

`scopes.md` is a spec-internal planning artifact; it has no downstream code consumers. The connector-count contract's authoritative product-truth surfaces (`docs/smackerel.md` §22.7/§24-A) already say 16 and are NOT touched by this packet. The only "consumers" of the `SCN-024-06` text are the traceability guard (mapped by `scenarioId`; kept green by lockstep title/DoD edit) and `scenario-manifest.json` (already 16). The `state.json` `lastUpdatedAt` is read by governance tooling that takes the last value; deduping it makes the read deterministic without changing the effective value's semantics going forward (now the fresh recert value).

**Canary:** the existing R-006 directory-count canary (`find internal/connector -maxdepth 1 -mindepth 1 -type d | wc -l == 16`) documented in `scopes.md` continues to guard the connector inventory; F2 simply aligns the Scope 2 narrative with that canary's expected value.

**Rollback:** reverting this packet's working-tree edits restores the prior `scopes.md` 15-claims + the duplicate `lastUpdatedAt` + the prior `certifiedAt`; because nothing is committed, rollback is `git checkout -- specs/024-design-doc-reconciliation/{scopes.md,state.json,report.md}` + `rm -rf` the new packet folder. No DB schema, no restart, no runtime impact.

## Change Boundary

**Allowed (this packet may modify):**
- `specs/024-design-doc-reconciliation/state.json` — F1 dedup + recert + governance backfill
- `specs/024-design-doc-reconciliation/scopes.md` — F2 8-site reconciliation
- `specs/024-design-doc-reconciliation/report.md` — append closure section
- `specs/024-design-doc-reconciliation/bugs/BUG-024-005-scopes-connector-count-and-state-dup-key/` — 8 packet artifacts

**Excluded (MUST NOT modify):**
- `cmd/`, `internal/` (incl. `internal/deploy/docs_connector_count_contract_test.go`), `ml/`, `web/`, `tests/`
- `docs/*`, `config/`, `scripts/`, `smackerel.sh`, `docker-compose*`, `deploy/`
- `.github/bubbles/` (framework-managed)
- `specs/055-*`, `specs/044-per-user-bearer-auth/state.json`, all other spec folders
- The closed `bugs/BUG-024-002-*`, `bugs/BUG-024-003-*`, `bugs/BUG-024-004-*` packets
