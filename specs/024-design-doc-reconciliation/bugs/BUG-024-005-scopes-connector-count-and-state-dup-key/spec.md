# Specification: BUG-024-005 Residual reconciliation drift (state.json duplicate `lastUpdatedAt` + scopes.md 15→16 connector drift)

**Status:** Done

## Business Context

Sweep round 7 of `sweep-2026-06-06-r20b` (`mode: harden-to-doc`, parent-expanded) ran the `harden` probe over `specs/024-design-doc-reconciliation`. All 5 framework guards were GREEN at baseline, but the probe surfaced 2 latent (guard-invisible) findings — residual drift from the BUG-024-002/003/004 series:

1. **F1 (LOW).** `state.json` carried a duplicate top-level `lastUpdatedAt` (header `2026-06-06T00:00:00Z` + legacy `2026-05-25T00:00:00Z` after `failures[]`). JSON last-wins silently shadowed the intended value. No guard detects duplicate keys.
2. **F2 (MEDIUM).** `scopes.md` Scope 2 body + `SCN-024-04`/`SCN-024-06` still said "15 connectors" / listed 15 names omitting `qfdecisions`, contradicting `spec.md` (16), `scenario-manifest.json` (16), `docs/smackerel.md` §22.7 (16), the live registry (16), and `scopes.md`'s own BUG-024-002 addenda (16). Traceability maps by `scenarioId`, so the count divergence passed silently.

Both are the same class (residual reconciliation drift) and are bundled into a single Scope 1. The fix is documentation/governance-only: one key removal + a recert in `state.json`, and 8 substantive text edits in `scopes.md`. Because planning truth (`scopes.md`) is edited on a `done` spec, the spec is recertified (fresh `certifiedAt` + a `bubbles.spec-review` `reviewStatus: CURRENT` entry) so the parent's eventual commit satisfies Gate G088.

## Use Cases

### UC-01: Make `state.json` Metadata Unambiguous
**Actor**: Any reader/parser of `state.json` (`jq`, Python, Go, auditor, future agent)
**Goal**: `state.json` has exactly one top-level `lastUpdatedAt`; the value is the fresh recert timestamp; no duplicate-key ambiguity.
**Outcome**: The effective `lastUpdatedAt` is deterministic and reflects the real last-update moment.

### UC-02: Make scopes.md Internally and Externally Consistent on Connector Count
**Actor**: A contributor reading spec 024's Scope 2 to understand the connector-reconciliation contract
**Goal**: Scope 2 body + `SCN-024-04`/`SCN-024-06` say "16 connectors" and list `qfdecisions`, matching `spec.md`, `scenario-manifest.json`, `docs/smackerel.md` §22.7, and the live registry; the BUG-024-002 addenda no longer contradict the original body.
**Outcome**: No reader encounters a "15 connectors" claim in spec 024 that disagrees with reality.

### UC-03: Preserve Historical and Runtime Truth Verbatim
**Actor**: The rollback contract, stale-ref grep searches, BUG-024-002 TDD evidence, and the connector registry/contract test
**Goal**: The 7 "15"-as-old-value references in `scopes.md` (rollback `git revert` contract, `grep` searches for stale refs, BUG-024-002 red-phase evidence) stay verbatim; `cmd/core/connectors.go`, `internal/connector/*`, and `TestConnectorCountContract` are untouched.
**Outcome**: History stays accurate; `./smackerel.sh test unit --go` and `go test -run TestConnectorCountContract ./internal/deploy/...` still pass 4/4.

## Functional Requirements

### FR-01: Remove The Duplicate Top-Level `lastUpdatedAt` From `state.json`
**Description**: Delete the legacy `"lastUpdatedAt": "2026-05-25T00:00:00Z"` key that follows the `failures[]` array, keeping the single header `lastUpdatedAt`.
**Acceptance**: `python3 -c "import json; raw=open('specs/024-design-doc-reconciliation/state.json').read(); assert raw.count('\"lastUpdatedAt\"')==1"` exits 0; a `json.load` with `object_pairs_hook` duplicate detector reports no duplicate-key sets.

### FR-02: Recertify `state.json` For The Planning-Truth Edit
**Description**: Advance top-level `certifiedAt` and `lastUpdatedAt` to `2026-06-06T18:00:00Z`; append a `bubbles.spec-review` `reviewStatus: CURRENT` `executionHistory` entry; do NOT set `requiresRevalidation:true` (would trip G089 on a done spec). The fresh `certifiedAt` is chosen so the parent's commit of the `scopes.md` edit (which lands before 18:00:00Z) is excluded from G088's post-cert window.
**Acceptance**: `python3 -c "import json; d=json.load(open('specs/024-design-doc-reconciliation/state.json')); assert d['certifiedAt']=='2026-06-06T18:00:00Z' and d['lastUpdatedAt']=='2026-06-06T18:00:00Z' and any(e.get('agent')=='bubbles.spec-review' and e.get('reviewStatus')=='CURRENT' for e in d['executionHistory']) and 'requiresRevalidation' not in d"` exits 0.

### FR-03: Reconcile The 8 Substantive Stale-15 Sites In `scopes.md` To 16
**Description**: Change the 8 substantive Scope 2 connector-count claims (validation checkpoint, scope-summary table, `SCN-024-04` body, `SCN-024-06` title + body, test-plan count row, DoD bullet + evidence) from 15 to 16, inserting `qfdecisions` alphabetically (between `markets` and `rss`) in the `SCN-024-06` name list. Preserve the 7 historical/rollback/grep-contract "15" references verbatim.
**Acceptance**: `grep -cE '15 (connectors|committed)' specs/024-design-doc-reconciliation/scopes.md` returns the historical-only count (rollback + grep-contract lines, not the 8 substantive sites); `grep -c 'qfdecisions' specs/024-design-doc-reconciliation/scopes.md` ≥ 1 in the `SCN-024-06` body.

### FR-04: Keep DoD-Gherkin Fidelity (G068) Green
**Description**: The `SCN-024-06` Gherkin title and its DoD bullet are changed to "16 connectors" in lockstep so traceability Gate G068 still maps the scenario to its DoD item.
**Acceptance**: `bash .github/bubbles/scripts/traceability-guard.sh specs/024-design-doc-reconciliation` exits 0 with `RESULT: PASSED`; STG Check 22 (G068) reports all 6 scenarios faithful.

### FR-05: Backfill BUG-024-005 Closure Provenance Into Parent `state.json` + `report.md`
**Description**: Extend `state.json` `executionHistory[]` with the harden-sweep closure phases (incl. the `bubbles.spec-review` CURRENT entry) and add a `resolvedBugs[]` BUG-024-005 entry; append `## BUG-024-005 Harden-Sweep Resolution (2026-06-06)` to `report.md` with Code Diff Evidence + Git-Backed Proof, all PII-redacted to `~/`.
**Acceptance**: `python3 -c "import json; d=json.load(open('specs/024-design-doc-reconciliation/state.json')); assert any('BUG-024-005' in (b.get('bugId') or '') for b in d.get('resolvedBugs',[]))"` exits 0; `grep -cE '^## BUG-024-005 Harden-Sweep Resolution' specs/024-design-doc-reconciliation/report.md` == 1; `grep -cE '/home/<user>/' specs/024-design-doc-reconciliation/report.md` == 0.

### FR-06: No Commit, No Push — Leave Changes In The Working Tree
**Description**: Per the round hard rule, all edits stay in the working tree for the parent stochastic-quality-sweep / operator to commit. Consequently G088 / STG Check 30 show the uncommitted `scopes.md` worktree edit as `postCertEdits=1` until the parent commits (the recert makes that commit pass G088). The other 4 guards stay green.
**Acceptance**: Nothing committed (`git log -1` HEAD unchanged); `git status --short specs/024-design-doc-reconciliation/` shows modified parent files + the new BUG-024-005 packet; content guards (artifact-lint, artifact-freshness, traceability) exit 0; G088/STG show the documented designed pre-commit state.

## Behavioral Specifications (Gherkin)

See `scenario-manifest.json` for the canonical 5 scenarios (BUG-024-005-SCN-001..SCN-005). Their Gherkin rendering lives in `scopes.md`.

## Acceptance Criteria

- AC-01: `state.json` has exactly one top-level `lastUpdatedAt`; duplicate-key detector reports none (FR-01).
- AC-02: `state.json` `certifiedAt` and `lastUpdatedAt` == `2026-06-06T18:00:00Z`; a `bubbles.spec-review` `reviewStatus: CURRENT` `executionHistory` entry exists; `requiresRevalidation` is absent (FR-02).
- AC-03: The 8 substantive Scope 2 connector-count claims read "16"; `SCN-024-06` body lists `qfdecisions`; the 7 historical "15" references are preserved verbatim (FR-03).
- AC-04: `traceability-guard.sh` exits 0 `RESULT: PASSED`; STG Check 22 (G068) all 6 scenarios faithful (FR-04).
- AC-05: `artifact-lint.sh specs/024-design-doc-reconciliation` exits 0 `Artifact lint PASSED.`
- AC-06: `artifact-freshness-guard.sh specs/024-design-doc-reconciliation` exits 0 `RESULT: PASS (0 failures, 0 warnings)`.
- AC-07: `go test -run TestConnectorCountContract ./internal/deploy/...` exits 0 with all 4 sub-tests PASS (BUG-024-003 contract preserved).
- AC-08: Spec 024 `status` remains `done`; `certification.status`, `completedScopes`, `certifiedCompletedPhases`, and both per-scope `certifiedAt` entries preserved verbatim.
- AC-09: `resolvedBugs[]` has a BUG-024-005 entry with `finalStatus: resolved`; `report.md` has exactly 1 `## BUG-024-005 Harden-Sweep Resolution` section; zero `/home/<user>/` paths (FR-05).
- AC-10: BUG-024-005 packet's own gate passes: `artifact-lint.sh specs/024-design-doc-reconciliation/bugs/BUG-024-005-scopes-connector-count-and-state-dup-key` exits 0.
- AC-11: G088 / STG Check 30 show the documented uncommitted `scopes.md` worktree edit (`postCertEdits=1`, `certifiedAt=2026-06-06T18:00:00Z`) — the designed pre-commit handoff state that clears when the parent commits before 18:00:00Z; reported honestly, not as a false PASS (FR-06).
- AC-12: Nothing committed or pushed; all changes left in the working tree.

## Product Principle Alignment

**Principle 8 — Trust Through Transparency.** F2 directly serves accuracy: a contributor reading spec 024's Scope 2 must not encounter a "15 connectors" claim that disagrees with reality. F1 removes a silent metadata ambiguity. Both restore the transparency the spec exists to enforce.

**Principle 3 — Knowledge Breathes (Lifecycle, Not Static).** The recert (fresh `certifiedAt` + `bubbles.spec-review` CURRENT) keeps the certification lifecycle honest: the planning truth was touched, so the certification anchor moves forward and a current review attests the spec is still trustworthy.

**Principle 10 — QF Companion Boundary (NON-NEGOTIABLE Cross-Product).** Adding `qfdecisions` to the `SCN-024-06` name list uses the same connector identifier as `spec.md` R-006 and `docs/smackerel.md` §22.7 row 16; no financial-advice-generation capability is introduced, and the Principle 10 boundary text elsewhere is untouched. Spec 041's contract is preserved.

## Non-Goals

- No production runtime code, schema, NATS topology, web template, prompt contract, Telegram command, integration/unit test, deploy script, compose file, or `smackerel.yaml` value is modified. `internal/deploy/docs_connector_count_contract_test.go` (BUG-024-003) is preserved verbatim.
- No `docs/*` file is modified. `docs/smackerel.md` §22.7/§24-A already enumerate 16 (BUG-024-002); `docs/Development.md` L31 already 16 (BUG-024-003). F2 only reconciles the spec-internal `scopes.md` narrative.
- Spec 024 `status` is not changed away from `done`; `certification.status`/`completedScopes`/`certifiedCompletedPhases`/per-scope `certifiedAt` are not changed.
- No framework guard is weakened. F2 keeps G068 green by editing the Gherkin title and DoD bullet in lockstep, not by relaxing the gate.
- `requiresRevalidation:true` is NOT set (would trip G089 on a done spec).
- This packet does NOT commit or push. The pre-push hook and the single atomic commit are deferred to the parent stochastic-quality-sweep / operator step.
- This packet does NOT re-open the closed BUG-024-004 packet to fix its internal `2026-05-28T05:07:50Z`-vs-`:51Z` validation-checkpoint prose mismatch — that is a cosmetic note in a closed packet whose implemented value (`:51Z`) is correct and gate-passing; touching it would churn a closed artifact for no gate benefit.
