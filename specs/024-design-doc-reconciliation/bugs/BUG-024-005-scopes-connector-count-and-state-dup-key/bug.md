# Bug: BUG-024-005 Residual reconciliation drift — (F1) parent state.json duplicate top-level `lastUpdatedAt`; (F2) scopes.md Scope-2 still claims 15 connectors while every other surface says 16

## Summary

Sweep round 7 of `sweep-2026-06-06-r20b` (`mode: harden-to-doc`, parent-expanded) ran the `harden` trigger probe on `specs/024-design-doc-reconciliation`. All 5 framework guards were GREEN at baseline, but the probe surfaced **2 LATENT (guard-invisible) findings** on the exact documentation-accuracy dimension spec 024 governs — residual drift left behind by the BUG-024-002 / BUG-024-003 / BUG-024-004 sweep series:

1. **F1 (LOW) — `specs/024-design-doc-reconciliation/state.json` carried a duplicate top-level `lastUpdatedAt` key.** BUG-024-004 added `"lastUpdatedAt": "2026-06-06T00:00:00Z"` in the header block (line 8) but the legacy `"lastUpdatedAt": "2026-05-25T00:00:00Z"` key (after the `failures[]` array, line 539) was never removed. JSON last-wins parsing (`jq`, Python `json.load`, Go `encoding/json`) silently took the trailing value, so the effective `lastUpdatedAt` was the stale `2026-05-25T00:00:00Z` — shadowing the value BUG-024-004 intended to publish. No framework guard detects duplicate JSON keys, so the defect was invisible to the gate suite.

2. **F2 (MEDIUM) — `scopes.md` Scope 2 original body + `SCN-024-04`/`SCN-024-06` Gherkin still claimed "15 connectors" and listed 15 names omitting `qfdecisions`.** This contradicted `spec.md` R-006 (16), `scenario-manifest.json` `SCN-024-06` (16), `docs/smackerel.md` §22.7 (16), the live 16-connector registry (`cmd/core/connectors.go`, `qfDecisionsConn` at line 52), AND `scopes.md`'s own BUG-024-002 addenda (which already say 16). BUG-024-002/003 reconciled the 15→16 connector count across `docs/smackerel.md`, `docs/Development.md`, and `spec.md`, and updated `scenario-manifest.json` `SCN-024-06` to 16 — but they left the ORIGINAL Scope 2 body in `scopes.md` at 15. The traceability guard maps scenarios by `scenarioId`, not by count, so the 15-vs-16 title/body divergence passed silently.

The packet runs as `mode: harden-to-doc` (the sweep round's mapped child mode for the `harden` trigger). Both findings are residual-reconciliation-drift of the same class, so they are bundled into a single Scope 1.

## Severity

- [ ] Critical — System unusable, data loss
- [ ] High
- [x] Medium — F2 is a documentation-accuracy defect (the spec's own scope artifact claims "accurately lists all 15 connectors" while reality is 16) on the exact dimension spec 024 exists to police; F1 is a JSON-hygiene defect that silently shadows a metadata field. Neither breaks runtime; neither was caught by any gate; both are genuine spec/scope inconsistencies that a harden probe must surface.
- [ ] Low

## Status

- [x] Reported
- [x] Confirmed by sweep round 7 harden-to-doc probe
- [ ] In Progress
- [x] Fixed (working tree; commit deferred to parent sweep per round hard rule)
- [x] Verified (5 content guards green; G088/STG in designed uncommitted-handoff state)
- [ ] Closed

## Reproduction Steps

1. From the spec 024 state BEFORE this round, detect the duplicate `lastUpdatedAt` (F1):
   ```bash
   python3 -c "import json; raw=open('specs/024-design-doc-reconciliation/state.json').read(); print('lastUpdatedAt literal count:', raw.count('\"lastUpdatedAt\"'))"
   # pre-fix: 2  (one in header, one after failures[])
   ```
2. Confirm last-wins shadowing:
   ```bash
   python3 -c "import json; d=json.load(open('specs/024-design-doc-reconciliation/state.json')); print('effective lastUpdatedAt:', d.get('lastUpdatedAt'))"
   # pre-fix: 2026-05-25T00:00:00Z  (the stale trailing value, NOT the intended 2026-06-06)
   ```
3. Detect the F2 connector-count drift in scopes.md:
   ```bash
   grep -nE '15 (connectors|committed)' specs/024-design-doc-reconciliation/scopes.md
   # pre-fix: 8 substantive hits in the Scope 2 body (lines 17, 24, 214, 223, 226, 250, 288, 289)
   ```
4. Confirm every other surface says 16:
   ```bash
   grep -c 'Conn :=\|Conn =' cmd/core/connectors.go            # 16 connector constructors
   grep -nE '16 connectors|all 16' specs/024-design-doc-reconciliation/spec.md            # R-006 / BS-004 / AC-5 say 16
   grep -nE '"16 connectors"|lists all 16' specs/024-design-doc-reconciliation/scenario-manifest.json  # SCN-024-06 title says 16
   grep -nE 'Committed Connector Inventory \(16 connectors\)' docs/smackerel.md            # §22.7 says 16
   ```
5. Confirm the divergence is guard-invisible: all 5 framework guards (`post-cert-spec-edit-guard`, `state-transition-guard`, `artifact-lint`, `artifact-freshness-guard`, `traceability-guard`) exit 0 at baseline despite both F1 and F2 being present.

## Expected Behavior

- `state.json` has exactly ONE top-level `lastUpdatedAt` key; `python3` duplicate-key detection reports no duplicate-key sets.
- `scopes.md` Scope 2 body + `SCN-024-04`/`SCN-024-06` consistently say "16 connectors" and the `SCN-024-06` name list includes `qfdecisions`, matching `spec.md`, `scenario-manifest.json`, `docs/smackerel.md` §22.7, and the live registry.
- Historical/rollback/grep-contract references that cite "15" as the pre-BUG-024-002 value (rollback contract, stale-ref grep searches, BUG-024-002 red-phase TDD evidence) are preserved verbatim.
- All 5 content-level guards stay green; the spec is recertified (fresh `certifiedAt` + a `bubbles.spec-review` `reviewStatus: CURRENT` entry) so the parent's commit of the planning-truth edit satisfies Gate G088.

## Actual Behavior

- Pre-fix: `state.json` had two top-level `lastUpdatedAt` keys; the effective value was the stale `2026-05-25T00:00:00Z`.
- Pre-fix: `scopes.md` Scope 2 body claimed "15 connectors" and listed 15 names omitting `qfdecisions`, internally contradicting its own BUG-024-002 addenda and every external surface.
- Both defects were invisible to all 5 framework guards.

## Environment

- Branch: `main`
- Sweep: `sweep-2026-06-06-r20b` round 7 of 20, trigger `harden`, mapped child mode `harden-to-doc`, executionModel `parent-expanded-child-mode` (subagent runtime lacks nested `runSubagent`; `harden-to-doc` is single-spec and not `requiresTopLevelRuntime`, so parent-expansion is policy-compliant per `bubbles-workflow-execution-loops`).
- Parent feature: `specs/024-design-doc-reconciliation` (`status: done` end-to-end; original cert 2026-04-10; BUG-024-002 closed 2026-05-24; BUG-024-003 closed 2026-05-27; BUG-024-004 closed 2026-06-06).
- F1 source: BUG-024-004's 3-key top-level backfill added `lastUpdatedAt` at the header without removing the legacy key after `failures[]`.
- F2 source: BUG-024-002/003 reconciled the connector count across docs + spec + manifest but did not touch the original Scope 2 body in `scopes.md`.
- Runtime source-of-truth: `cmd/core/connectors.go` registers 16 connectors (`qfDecisionsConn := qfDecisionsConnector.New("qf-decisions")` at line 52).

## Root Cause Analysis (Five Whys)

- **Why did spec 024 carry residual drift after three prior bug rounds?** Because each prior round closed a specific, narrowly-scoped finding (BUG-024-002: docs §22.7/§24-A inventory + freshness BLOCKS; BUG-024-003: docs/Development.md + spec.md R-006 + contract test; BUG-024-004: state.json top-level `certifiedAt`) and did not sweep the full corpus for sibling inconsistencies introduced by their own edits.
- **Why did F1 appear?** BUG-024-004 inserted `lastUpdatedAt` in the header to satisfy the "top-level fields" pattern but did not delete the pre-existing legacy key, because nothing in its DoD enumerated "ensure `lastUpdatedAt` is singular."
- **Why did F2 survive?** BUG-024-002 updated `scenario-manifest.json` `SCN-024-06` to 16 and appended Scope 2 addenda saying 16, but it treated the original Scope 2 body as immutable history rather than reconciling it — leaving a contradiction inside the same file.
- **Why didn't a guard catch either?** No guard detects duplicate JSON keys (F1); the traceability guard maps by `scenarioId` and the DoD-fidelity check matched because the Gherkin title and DoD bullet were BOTH wrong in the same way (F2) — a consistent-but-wrong pair passes fidelity.
- **Why is this MEDIUM not HIGH?** No runtime behavior is affected; no test fails; the spec's substantive certification is intact. The impact is documentation accuracy and metadata correctness — exactly the harden probe's remit, but not a blocking runtime defect.

## Related

- Parent: `specs/024-design-doc-reconciliation/`
- Prior sibling bugs: `bugs/BUG-024-002-reconcile-artifact-drift/` (closed 2026-05-24), `bugs/BUG-024-003-dev-doc-connector-drift/` (closed 2026-05-27), `bugs/BUG-024-004-state-missing-certifiedat-g088/` (closed 2026-06-06)
- Gate sources: `.github/bubbles/scripts/post-cert-spec-edit-guard.sh` (G088 worktree-edit detection → drives the recert), `.github/bubbles/scripts/traceability-guard.sh` (G068 DoD-Gherkin fidelity → kept green by lockstep SCN-024-06 edit)
- Runtime contract preserved: `internal/deploy/docs_connector_count_contract_test.go` (`TestConnectorCountContract`, BUG-024-003) — 4/4 PASS, untouched
- Sweep ledger: round 7 of 20, harden trigger, mapped child mode `harden-to-doc`, executionModel `parent-expanded-child-mode`
