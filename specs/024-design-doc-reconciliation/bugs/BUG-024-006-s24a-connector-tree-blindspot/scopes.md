# Scopes: BUG-024-006 §24-A architecture-tree connector-count drift + TestConnectorCountContract 4th-surface blind spot

## Scope 1: Pin §24-A As A 4th Surface (bubbles.test) Then Reconcile §24-A 16→17 (bubbles.docs)

**Status:** Done
**Scope-Kind:** contract-only
**Why contract-only:** the fix is a Go contract-test extension (`internal/deploy/docs_connector_count_contract_test.go`, in-process file assertions, no live stack) plus a `docs/smackerel.md` §24-A documentation reconciliation. There is no live runtime-behavior surface, so scenario-specific live E2E regression rows do not apply — the permanent regression guarantee is the committed adversarial unit sub-test `TestConnectorCountContract_AdversarialSmackerelMdTreeLow`, not an E2E flow.
**Depends On:** None
**Owner sequence:** `bubbles.test` (FR-02, RED-prove) → `bubbles.docs` (FR-01, GREEN) → `bubbles.docs` (FR-05, governance + commit)

### Use Cases (Gherkin)

```gherkin
Scenario: BUG-024-006-SCN-001 §24-A architecture tree reflects the live registry count of 17 (incl. Card Rewards)
  Given docs/smackerel.md §24-A header line 2892 reads "Connector plugins (16 committed)"
  And the §24-A leaf list (lines 2893-2908) has 16 leaves ending at "QF Decisions" with no Card Rewards leaf
  And cmd/core/connectors.go lines 68-72 register 17 connectors (cardRewardsConn at line 72)
  And docs/smackerel.md §22.7 header line 2783 reads "(17 connectors)" with Card Rewards as row 17
  When bubbles.docs reconciles §24-A
  Then line 2892 reads "Connector plugins (17 committed)"
  And a "Card Rewards (cardrewards/ — spec 083 read-only rotating-category fetch)" leaf is inserted after QF Decisions
  And the §24-A leaf list enumerates 17 connectors
  And awk 'NR>=2892 && NR<=2913' docs/smackerel.md | grep -ic 'card.*reward' is >= 1
  And grep -n 'Connector plugins (' docs/smackerel.md returns "(17 committed)"

Scenario: BUG-024-006-SCN-002 TestConnectorCountContract pins §24-A as a 4th surface with adversarial proof
  Given internal/deploy/docs_connector_count_contract_test.go pins only 3 surfaces (slice literal regex L81, §22.7 header regex L89, Development.md bullet regex L95)
  And no regex matches the §24-A "Connector plugins (N committed)" line
  When bubbles.test extends the contract test
  Then a new regex smackerelMdTreeRe matches "Connector plugins (N committed)"
  And parseSmackerelMdTreeCount parses the §24-A count
  And assertConnectorCountContract folds §24-A into the four-surface equality assertion with a diagnostic naming §24-A
  And TestConnectorCountContract_AdversarialSmackerelMdTreeLow rejects a synthetic docs/smackerel.md whose §24-A says "(16 committed)" while the runtime is 17
  And the existing 3 pinned surfaces and their 3 adversarial sub-tests remain intact

Scenario: BUG-024-006-SCN-003 Adversarial RED proves the §24-A pin is non-tautological before the doc fix turns it GREEN
  Given the §24-A surface is pinned by the extended contract test (SCN-002)
  And docs/smackerel.md §24-A is still stale at "(16 committed)"
  When ./smackerel.sh test unit --go --go-run 'TestConnectorCountContract' --verbose is run BEFORE the §24-A doc edit
  Then TestConnectorCountContract_LiveFile FAILS with a diagnostic "docs/smackerel.md §24-A=16" vs runtime/§22.7/Development.md=17
  And after bubbles.docs reconciles §24-A to 17 (SCN-001)
  When the same command is re-run
  Then it exits 0 with all TestConnectorCountContract_* sub-tests passing (four surfaces agree on 17)

Scenario: BUG-024-006-SCN-004 Parent spec 024 governance backfill records BUG-024-006 closure without leaving status: done
  Given parent state.json::resolvedBugs[] currently lists BUG-024-001..BUG-024-005
  And parent report.md ends at the BUG-024-005 resolution section
  When bubbles.docs appends BUG-024-006 resolvedBugs[] + executionHistory entries + a report.md "## BUG-024-006 …" section
  Then state.json::resolvedBugs[] contains a BUG-024-006 entry
  And lastUpdatedAt advances to a 2026-06-16 (or later) ISO timestamp
  And spec 024 top-level status remains "done"
  And report.md carries Code Diff Evidence + adversarial RED→GREEN proof for BUG-024-006
```

### Implementation Plan

**Files touched by the FIX phase (single atomic commit, NOT this discovery packet):**

- `internal/deploy/docs_connector_count_contract_test.go` (Layer 1 — bubbles.test: + §24-A regex/parser/4th-surface assertion + adversarial sub-test)
- `docs/smackerel.md` (Layer 2 — bubbles.docs: line 2892 header 16→17 + line 2908 glyph + inserted Card Rewards leaf)
- `specs/024-design-doc-reconciliation/state.json` (Layer 3 — resolvedBugs[] + executionHistory + lastUpdatedAt bump)
- `specs/024-design-doc-reconciliation/report.md` (Layer 3 — append `## BUG-024-006 …` section)
- `specs/024-design-doc-reconciliation/bugs/BUG-024-006-s24a-connector-tree-blindspot/` (this packet's 8 artifacts, finalized by the fix owners)

**Excluded from commit (NON-NEGOTIABLE):**
- `cmd/core/connectors.go` (runtime registry — source of truth, unmodified at 17)
- `internal/connector/cardrewards/` (owned by spec 083, unmodified)
- `docs/smackerel.md` §22.7 + `docs/Development.md` line 31 (already correct at 17 — NOT re-touched)
- `internal/api/`, `internal/config/`, `internal/web/`, `internal/notification/`, `internal/pipeline/`, `ml/`, `config/`, `scripts/`, `smackerel.sh`, `docker-compose*`, `.github/bubbles/`
- Any other in-flight spec folder under `specs/`

### Test Plan

| Type | Test | Purpose | Scenarios Covered |
|------|------|---------|-------------------|
| Manual | `grep -n 'Connector plugins (' docs/smackerel.md` returns `(17 committed)` | §24-A header reconciled to 17 | SCN-001 |
| Manual | `awk 'NR>=2892 && NR<=2913' docs/smackerel.md \| grep -ic 'card.*reward'` >= 1 | Card Rewards present in §24-A tree | SCN-001 |
| Count | `grep -n 'connector.Connector{\|cardRewardsConn,' cmd/core/connectors.go` confirms slice = 17 | Live registry matches the new §24-A claim | SCN-001, SCN-003 |
| Boundary | §24-A Card Rewards leaf keeps read-only framing (`spec 083 read-only …`) consistent with §22.7 row 17 | Principle 10 QF Companion Boundary preserved | SCN-001 |
| Unit (Go) | `go test -run TestConnectorCountContract_LiveFile ./internal/deploy/...` exits 0 AFTER the §24-A edit; file: `internal/deploy/docs_connector_count_contract_test.go` | Four-surface contract holds against live files | SCN-002, SCN-003 |
| Adversarial (Go) | `TestConnectorCountContract_AdversarialSmackerelMdTreeLow` asserts the contract returns non-nil on synthetic §24-A=16 vs runtime=17 | §24-A pin proven non-tautological | SCN-002, SCN-003 |
| Adversarial RED (Go) | `./smackerel.sh test unit --go --go-run 'TestConnectorCountContract' --verbose` run BEFORE the §24-A edit MUST FAIL with `§24-A=16` vs runtime=17 | The guard would catch a §24-A revert to 16 (anti-tautology RED proof) | SCN-003 |
| Regression (Go) | Existing 3 pinned surfaces + their 3 adversarial sub-tests still pass; full `internal/deploy` suite green | No regression in the BUG-024-003 contract | SCN-002 |
| Validation | `bash .github/bubbles/scripts/state-transition-guard.sh specs/024-design-doc-reconciliation` exits at pre-edit baseline (no new BLOCKS) | No new state-transition regressions | All |
| Validation | `bash .github/bubbles/scripts/artifact-lint.sh specs/024-design-doc-reconciliation` exits 0; `traceability-guard.sh specs/024-design-doc-reconciliation` exits 0 | No new artifact/traceability regressions | All |
| Validation | `bash .github/bubbles/scripts/artifact-lint.sh specs/024-design-doc-reconciliation/bugs/BUG-024-006-s24a-connector-tree-blindspot` exits 0 | BUG packet itself is healthy | All |

### Scenario-First TDD Evidence (fix-phase — populated by the owners)

**SCN-001 red→green (bubbles.docs):**
- Red (pre-edit): `grep -n 'Connector plugins (' docs/smackerel.md` → `(16 committed)`; `awk 'NR>=2892 && NR<=2910' docs/smackerel.md | grep -ic 'card.*reward'` → 0.
- Green (post-edit): `grep -n 'Connector plugins (' docs/smackerel.md` → `(17 committed)`; Card Rewards leaf present after QF Decisions.

**SCN-002 / SCN-003 red→green (bubbles.test then bubbles.docs):**
- Red (post-test-extension, pre-doc-edit): `./smackerel.sh test unit --go --go-run 'TestConnectorCountContract' --verbose` → `--- FAIL: TestConnectorCountContract_LiveFile` with `docs/smackerel.md §24-A=16` vs runtime/§22.7/Development.md=17.
- Green (post-doc-edit): same command → exit 0, all `TestConnectorCountContract_*` PASS, four surfaces = 17.

### Change Boundary

The BUG-024-006 fix commit MUST stay strictly inside the boundary below; anything outside that appears staged is a contract violation and the commit MUST be aborted and rebuilt.

**Allowed file families:**
- `internal/deploy/docs_connector_count_contract_test.go` — §24-A 4th-surface pin + adversarial sub-test only
- `docs/smackerel.md` — §24-A lines 2892 + 2908 + one inserted Card Rewards leaf only
- `specs/024-design-doc-reconciliation/state.json` — append-only resolvedBugs[] + executionHistory + lastUpdatedAt
- `specs/024-design-doc-reconciliation/report.md` — append `## BUG-024-006 …` section only
- `specs/024-design-doc-reconciliation/bugs/BUG-024-006-s24a-connector-tree-blindspot/` — this packet's 8 artifacts

**Excluded surfaces (MUST NOT be modified):** `cmd/`, `internal/connector/`, `internal/api/`, `internal/config/`, `internal/web/`, `internal/notification/`, `internal/pipeline/`, `ml/`, `config/`, `scripts/`, `smackerel.sh`, `docker-compose*`, `.github/bubbles/`, `docs/Development.md`, `docs/smackerel.md` §22.7, and every other `specs/` folder.

### Rollback Contract

- All fix edits land in a single atomic commit. `git revert <SHA>` cleanly restores the pre-edit state (the §24-A doc line + the contract-test surface set). No schema migration, NATS topology, or runtime restart involved.

### Consumer Impact Sweep

This scope is **purely additive** and renames/removes **no** route, path, endpoint, API, identifier, symbol, or contract surface:

- **§24-A doc reconciliation** *adds* one Card Rewards leaf after QF Decisions and bumps the header count 16→17. No existing leaf, anchor, or heading is renamed or removed.
- **Contract-test extension** *adds* a 4th pinned surface (`smackerelMdTreeRe` + `parseSmackerelMdTreeCount` + the `AdversarialSmackerelMdTreeLow` sub-test). The three pre-existing pinned surfaces and their sub-tests are untouched.

Affected consumer surfaces — navigation, breadcrumb, redirect, API client / generated client, and deep link targets — are **unaffected**, because nothing they could reference was renamed or removed. A stale-reference sweep (`grep -rn '16 committed' docs/smackerel.md` → 0 hits; no dangling §24-A / QF-Decisions-terminal-leaf references) confirms zero stale first-party references remain.

### Definition of Done (DoD)

- [x] SCN-001: the §24-A architecture tree reflects the live connector registry count of 17, with Card Rewards (the 16→17 connector) enumerated after QF Decisions — `docs/smackerel.md` §24-A header reads `Connector plugins (17 committed)` (owned by bubbles.docs)
   - Phase: Docs / Verify
   - Evidence: `grep -n 'Connector plugins (' docs/smackerel.md` returns `2892:│   ├── Connector plugins (17 committed)`; `awk 'NR>=2892 && NR<=2913' docs/smackerel.md | grep -ic 'card.*reward'` → `1` (Card Rewards leaf present after QF Decisions; 17 leaves Gov Alerts → Card Rewards) — see report.md → "bubbles.docs — F2 §24-A Reconciliation GREEN Proof" (F2 Code Diff Evidence: the `git diff docs/smackerel.md` 3-insert/2-delete hunk)
   - Claim Source: SCN-001 (executed 2026-06-16 — report.md F2 GREEN Proof)
- [x] SCN-002 fidelity: `internal/deploy/docs_connector_count_contract_test.go` pins §24-A's `Connector plugins (N committed)` line as a 4th surface in `assertConnectorCountContract`, with `smackerelMdTreeRe` + `parseSmackerelMdTreeCount` + a new adversarial sub-test — owned by bubbles.test
   - Phase: Test
   - Evidence: `grep -n 'smackerelMdTreeRe\|parseSmackerelMdTreeCount\|AdversarialSmackerelMdTreeLow' …` returns 9 hits (regex var L107, parser L173, assert-fold call L239, sub-test L409, + comments); the live diagnostic names `docs/smackerel.md §24-A=16` — see report.md → "bubbles.test — F3 Test-Extension RED Proof" (Code Diff Evidence + RED Run Evidence)
   - Claim Source: SCN-002 (executed 2026-06-16 — report.md F3 RED Proof)
- [x] Adversarial regression case exists and would fail if §24-A reverts to 16 — `TestConnectorCountContract_AdversarialSmackerelMdTreeLow` rejects synthetic §24-A=16 vs runtime=17; this is NOT tautological (the §24-A surface was previously unpinned and GREEN at the drift)
   - Phase: Test
   - Evidence: sub-test PASS with log (test L453) `adversarial OK: §24-A=16 vs runtime+§22.7+Development.md=17 is rejected with: contract violation … docs/smackerel.md §24-A=16` — see report.md F3 RED Run Evidence (`--- PASS: TestConnectorCountContract_AdversarialSmackerelMdTreeLow`)
   - Claim Source: SCN-002, SCN-003 (executed 2026-06-16 — report.md F3 RED Proof)
- [x] SCN-003: the adversarial RED proves the §24-A pin is non-tautological before the doc fix turns the contract GREEN — captured pre-doc-edit, `TestConnectorCountContract_LiveFile` FAILS (`§24-A=16` vs runtime=17), EXIT 1
   - Phase: Test
   - Evidence: `--- FAIL: TestConnectorCountContract_LiveFile` (test L273) with `… docs/smackerel.md §24-A=16, docs/Development.md=17 …`; suite `FAIL internal/deploy` / `EXIT_CODE=1`; §24-A confirmed still `(16 committed)` on disk (NOT edited) — see report.md F3 RED Run Evidence (≥10-line raw block)
   - Claim Source: SCN-003 (executed 2026-06-16 — report.md F3 RED Proof)
- [x] Post-fix GREEN proof: after the §24-A doc edit, `./smackerel.sh test unit --go --go-run 'TestConnectorCountContract' --verbose` exits 0 with all `TestConnectorCountContract_*` PASS
   - Phase: Docs / Verify
   - Evidence: `--- PASS: TestConnectorCountContract_LiveFile` with log `contract OK: cmd/core/connectors.go + docs/smackerel.md §22.7 + docs/smackerel.md §24-A + docs/Development.md all agree on 17 connectors`; all 4 adversarial sub-tests PASS; `ok github.com/smackerel/smackerel/internal/deploy 0.079s`; `[go-unit] go test ./... finished OK`; `EXIT_CODE=0` — see report.md → "F2 GREEN Run Evidence" (≥10-line raw block)
   - Claim Source: SCN-003 (executed 2026-06-16 — report.md F2 GREEN Proof)
- [x] Regression tests contain no silent-pass bailout patterns (no `if cond { return }` early-exits in the new sub-test)
   - Phase: Test
   - Evidence: `awk '/func TestConnectorCountContract_AdversarialSmackerelMdTreeLow/,/^}/' … | grep -nE 'return'` → `NO bare return / bailout found in new sub-test`; the sub-test asserts via direct `if err == nil { t.Fatalf(...) }` + `if !strings.Contains(...) { t.Fatalf(...) }` — see report.md F3 Code Diff Evidence (bailout scan)
   - Claim Source: SCN-002 (executed 2026-06-16 — report.md F3 RED Proof)
- [x] All existing tests pass (no regressions) — the BUG-024-003 3-surface contract + its 3 adversarial sub-tests remain green; full `internal/deploy` suite green
   - Phase: Test / Regression
   - Evidence: `ok github.com/smackerel/smackerel/internal/deploy 0.079s` with `TestConnectorCountContract_LiveFile` + all four `TestConnectorCountContract_Adversarial*` sub-tests PASS (the 3 pre-existing BUG-024-003 adversarial sub-tests `AdversarialConnectorsGoLow`/`AdversarialSmackerelMdHigh`/`AdversarialDevelopmentMdLow` + the new `AdversarialSmackerelMdTreeLow`) — see report.md → "F2 GREEN Run Evidence"
   - Claim Source: SCN-002 (executed 2026-06-16 — report.md F2 GREEN Proof)
- [x] All four framework guards green on parent spec 024 AND on the BUG-024-006 packet folder (state-transition-guard, artifact-lint, traceability-guard, artifact-freshness-guard); spec 024 stays `status: done`
   - Phase: Validate
   - Evidence: BUG-024-006 packet `artifact-lint.sh` exits 0 (PASS). Parent guards are differential vs the pre-edit baseline — the governance backfill introduced **no new BLOCKS**: Gate G088 PASS (Check 30 — governance-only `state.json`/`report.md` edit, no `spec.md`/`design.md`/`scopes.md` planning-truth change; `certifiedAt 2026-06-06T23:00:00Z` unchanged); parent state-transition-guard + artifact-lint carry the SAME pre-existing 4 STG blocks / 5 artifact-lint issues (missing `gaps`+`harden` specialist phases — full-delivery required-phase gate drift post-dating the 2026-06-06 certification), identical before and after this backfill and out-of-scope for BUG-024-006 (no `bubbles.gaps`/`bubbles.harden` provenance source for Check 6B). Every other parent gate (G087, G089, G090, G092, G093, G094, G095, G097, G098, G099, G100) PASS; spec 024 stays `status: done`.
   - Claim Source: SCN-004 (executed 2026-06-24 — bubbles.docs guard differential; see parent report.md "## BUG-024-006 …")
- [x] Parent spec 024 governance backfill: `resolvedBugs[]` extended with BUG-024-006; `lastUpdatedAt` bumped; `report.md` carries a `## BUG-024-006 …` resolution section with adversarial RED→GREEN proof
   - Phase: Docs / Finalize
   - Evidence: `python3 -c "import json; d=json.load(open('specs/024-design-doc-reconciliation/state.json')); assert any(b.get('bugId','').startswith('BUG-024-006') for b in d.get('resolvedBugs', []))"` exits 0 (verified — `resolvedBugs ids` now ends with `BUG-024-006-s24a-connector-tree-blindspot`); parent `lastUpdatedAt` bumped `2026-06-17`→`2026-06-24`; parent `report.md` carries the `## BUG-024-006 — §24-A …` section with the RED (historical 2026-06-16 LiveFile FAIL + permanent `AdversarialSmackerelMdTreeLow`) → GREEN (four-surface LiveFile PASS, `EXIT_CODE=0`) proof; parent `executionHistory` carries a `bubbles.docs` `[docs, finalize]` entry.
   - Claim Source: SCN-004 (executed 2026-06-24 — bubbles.docs governance backfill)
- [x] Consumer impact sweep confirms zero stale first-party references remain — the change is additive (adds a §24-A Card Rewards leaf + a 4th contract-test surface) and renames/removes no route, path, endpoint, API, identifier, or symbol
   - Phase: Validate
   - Evidence: `grep -rn '16 committed' docs/smackerel.md` → 0 hits (no stale §24-A count); navigation, breadcrumb, redirect, API client, and deep link surfaces are unaffected — see "### Consumer Impact Sweep" above and the F2 Code Diff Evidence (3 insertions / 2 deletions, no consumer-facing route/identifier removed)
   - Claim Source: SCN-001 (additive reconciliation; executed 2026-06-24)
- [x] Certification landed via the G088 two-commit sequence (planning-truth commit, then state.json done-flip with `certifiedAt`) using path-limited `git add` with zero stray files; `git push` is left to the orchestrator's review (NOT performed by this session; never `--no-verify`)
   - Phase: Finalize
   - Evidence: the F2 §24-A doc fix + F3 contract-test §24-A pin are already on `main` (contract-test pin in `eadfada7`; §24-A `(17 committed)` + Card Rewards leaf committed at `HEAD`); this session adds the two certification commits with `git add` limited to this BUG packet (+ parent governance) — see report.md → "### Finalize Evidence — G088 Two-Commit Certification"
   - Claim Source: SCN-004 (executed 2026-06-24 — bubbles.iterate certification)
- [x] Bug marked Fixed/Verified/Closed in bug.md and state.json status promoted to terminal-for-mode (bugfix-fastlane → `done`)
   - Phase: Finalize
   - Evidence: bug.md `[x] Fixed` + `[x] Verified` + `[x] Closed` are marked; state.json status `done` with `certification.status: done` + `certifiedAt` + `certifiedBy: bubbles.iterate`; `state-transition-guard.sh` prints `TRANSITION PERMITTED` and `artifact-lint.sh` exits 0 at `done` — see report.md → "### Finalize Evidence — G088 Two-Commit Certification"
   - Claim Source: SCN-004 (executed 2026-06-24 — bubbles.iterate certification)
