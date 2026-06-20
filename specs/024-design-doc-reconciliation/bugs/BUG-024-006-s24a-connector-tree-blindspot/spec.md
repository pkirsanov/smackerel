# Specification: BUG-024-006 docs/smackerel.md §24-A architecture-tree connector-count drift (16 vs live 17, missing Card Rewards) + TestConnectorCountContract 4th-surface blind spot

## Business Context

Round R29 of the stochastic-quality-sweep (`mode: stochastic-quality-sweep`, parent-expanded child mode `gaps-to-doc`) ran the gaps probe on `specs/024-design-doc-reconciliation` and found a genuine committed-tree defect on the exact accuracy dimension spec 024's **R-006 connector-inventory contract** governs for `docs/smackerel.md`:

1. **F2 (MEDIUM, PRIMARY).** `docs/smackerel.md` §24-A architecture tree (header line 2892 `Connector plugins (16 committed)`, leaf list lines 2893-2908) omits Card Rewards (`cardrewards/`, spec 083) and is stale at 16. The same document's §22.7 inventory (line 2783) says `(17 connectors)`; `docs/Development.md` line 31 says `17 passive connectors`; `cmd/core/connectors.go` lines 68-72 register 17. Three surfaces agree on 17; §24-A is the one stale surface at 16.
2. **F3 (MEDIUM, ROOT CAUSE).** `internal/deploy/docs_connector_count_contract_test.go` (`TestConnectorCountContract`) pins only three surfaces (slice literal regex line 81, §22.7 header regex line 89, `docs/Development.md` bullet regex line 95). §24-A's `Connector plugins (N committed)` line is an uncovered 4th surface, so §24-A drifted to 16 while the three pinned surfaces moved to 17 and the live-file test still passes GREEN (test line 236 logs "all agree on 17").

This packet runs as `mode: bugfix-fastlane`. The defect class is identical to the two sibling closures: F2 is the docs↔runtime drift class BUG-024-002 fixed (for §22.7 + §24-A at 15→16), and F3 is the forward-detection-test-gap class BUG-024-003 fixed (by creating the test this packet now extends).

## Use Cases

### UC-01: Reconcile §24-A Architecture Tree With The Live Registry (16 → 17, +Card Rewards)
**Actor**: A reader of `docs/smackerel.md` §24-A (engineer, auditor, investor, new contributor scanning the architecture diagram).
**Goal**: The §24-A architecture tree accurately enumerates all 17 connectors registered in `cmd/core/connectors.go`, including Card Rewards from spec 083.
**Outcome**: The R-006 contract spec 024 owns is restored for the §24-A surface. The §24-A header reads `Connector plugins (17 committed)`; the leaf list contains a Card Rewards entry after QF Decisions; `awk 'NR>=2892 && NR<=2912' docs/smackerel.md | grep -ic 'card.*reward'` ≥ 1.

### UC-02: Pin §24-A As A 4th Surface In The Forward-Detection Contract Test
**Actor**: `./smackerel.sh test unit --go` (every contributor, every CI pre-push hook).
**Goal**: `TestConnectorCountContract` fails the test suite if the connector count claimed by §24-A's `Connector plugins (N committed)` line disagrees with the runtime slice, §22.7 header, or `docs/Development.md` bullet.
**Outcome**: The next time a connector is added (or §24-A is reverted) without reconciling §24-A, `go test ./internal/deploy/...` exits non-zero with a precise diagnostic naming §24-A as the disagreeing surface. A new adversarial sub-test proves the §24-A pin is non-tautological.

### UC-03: Prove The Blind Spot Before Fixing It (Adversarial RED → GREEN)
**Actor**: `bubbles.test` then `bubbles.docs`.
**Goal**: The §24-A surface pin is added FIRST and demonstrated RED against the still-stale doc (16 ≠ 17); the doc reconciliation THEN turns it GREEN.
**Outcome**: Captured evidence shows `TestConnectorCountContract_LiveFile` failing while §24-A=16 and the other three surfaces=17, then passing after §24-A→17. This is the anti-tautology guarantee: the test that "would have caught this" is shown actually catching it.

### UC-04: Preserve Runtime Behavior Verbatim
**Actor**: Connector registry, every committed connector, all REST endpoints, NATS publishers, search/digest paths.
**Goal**: Runtime code (`cmd/core/`, `internal/connector/cardrewards/`, `internal/api/`, `internal/web/`, `internal/notification/`, `internal/pipeline/`), schema, NATS topology, prompt contracts, web templates, Telegram commands, deploy scripts, compose files, and `smackerel.yaml` are unchanged.
**Outcome**: `./smackerel.sh test unit --go` continues to pass with the extended contract test; baseline behavior preserved.

## Functional Requirements

### FR-01: Reconcile `docs/smackerel.md` §24-A Header + Leaf List From 16 To 17 (+Card Rewards) — owned by bubbles.docs
**Description**: The §24-A architecture-tree header at line 2892 must reflect the 17 connectors registered in `cmd/core/connectors.go`, and the leaf list must enumerate Card Rewards.
**Acceptance**: Line 2892 reads `│   ├── Connector plugins (17 committed)`. A Card Rewards leaf (`Card Rewards (cardrewards/ — spec 083 read-only rotating-category fetch)` or equivalent) is inserted after the QF Decisions leaf (currently line 2908), with the tree branch glyphs corrected so the list closes on Card Rewards (`└──`) and QF Decisions becomes a mid-branch (`├──`). The 17 §24-A leaves correspond 1:1 to the §22.7 inventory rows.

### FR-02: Extend `TestConnectorCountContract` To Pin §24-A As A 4th Surface — owned by bubbles.test
**Description**: `internal/deploy/docs_connector_count_contract_test.go` must parse the §24-A `Connector plugins (N committed)` count and include it in `assertConnectorCountContract`'s agreement assertion, with an adversarial sub-test proving the §24-A pin is non-tautological.
**Acceptance**:
- A new regex (e.g. `smackerelMdTreeRe = regexp.MustCompile(\`Connector plugins \((\d+) committed\)\`)`) and a `parseSmackerelMdTreeCount([]byte) (int, error)` helper are added.
- `assertConnectorCountContract` parses §24-A (from the same `docs/smackerel.md` bytes it already receives) and fails if §24-A's count differs from the runtime/§22.7/Development.md count, with a diagnostic naming the §24-A surface and the observed counts.
- A new adversarial sub-test `TestConnectorCountContract_AdversarialSmackerelMdTreeLow` (name may vary) synthesizes a `docs/smackerel.md` buffer whose §22.7 header is at the runtime count but whose §24-A header says `Connector plugins (16 committed)` while the runtime is 17 → the contract MUST return a non-nil error naming §24-A. (This is the exact regression that recurs if §24-A reverts to 16.)
- The existing three pinned surfaces and their three adversarial sub-tests remain intact.

### FR-03: Adversarial RED → GREEN Evidence Captured — owned by bubbles.test (RED) then bubbles.docs (GREEN)
**Description**: The §24-A pin (FR-02) is added before the doc reconciliation (FR-01) and shown RED against the stale doc, then GREEN after.
**Acceptance**: `report.md` (populated by the fix owners) carries:
- A RED run of `TestConnectorCountContract_LiveFile` (or the adversarial-against-live variant) failing with a §24-A=16 vs runtime=17 diagnostic, captured BEFORE the §24-A edit.
- A GREEN run of `./smackerel.sh test unit --go --go-run 'TestConnectorCountContract' --verbose` (exit 0) captured AFTER the §24-A edit, with the live-file log updated to reference four-surface agreement.

### FR-04: Extended Contract Test Passes Under `./smackerel.sh test unit --go`
**Description**: After FR-01 + FR-02 are applied together, the full `internal/deploy` suite is green.
**Acceptance**: `./smackerel.sh test unit --go --go-run 'TestConnectorCountContract' --verbose` exits 0; the new §24-A adversarial sub-test fails in synthetic mode and the live-file test passes in live mode.

### FR-05: Single Atomic Commit Lands All Changes With Structured Prefix (fix-phase deliverable, NOT this discovery packet)
**Description**: The fix owners land all BUG-024-006 changes (the §24-A doc edit + the contract-test extension + this packet's artifacts + parent governance backfill) in a single atomic commit whose subject begins with `bubbles(024/bug-024-006):`.
**Acceptance**: `git log --oneline -1 --format='%s'` after the fix commit begins with `bubbles(024/bug-024-006):`. `git diff --cached --name-status` shows only `docs/smackerel.md` + `internal/deploy/docs_connector_count_contract_test.go` + `specs/024-design-doc-reconciliation/` paths; zero stray staging.

## Behavioral Specifications (Gherkin)

See `scenario-manifest.json` for the canonical scenarios (BUG-024-006-SCN-001..SCN-004). Their Gherkin rendering lives in `scopes.md`.

## Acceptance Criteria

- AC-01: `grep -n 'Connector plugins (' docs/smackerel.md` returns `(17 committed)` (not `(16 committed)`).
- AC-02: `awk 'NR>=2892 && NR<=2913' docs/smackerel.md | grep -ic 'card.*reward'` ≥ 1; the §24-A leaf list enumerates 17 connectors with Card Rewards after QF Decisions.
- AC-03: All four surfaces agree on 17 — §22.7 header (line 2783), §24-A header (line 2892), `docs/Development.md` line 31, `cmd/core/connectors.go` slice literal (lines 68-72).
- AC-04: `internal/deploy/docs_connector_count_contract_test.go` parses §24-A's `Connector plugins (N committed)` count and asserts it in `assertConnectorCountContract`; a new adversarial sub-test drives the contract RED when §24-A disagrees with the runtime.
- AC-05: BEFORE the §24-A doc edit, the §24-A pin makes `TestConnectorCountContract_LiveFile` go RED with a §24-A=16 vs runtime=17 diagnostic (adversarial RED evidence captured in report.md).
- AC-06: AFTER the §24-A doc edit, `./smackerel.sh test unit --go --go-run 'TestConnectorCountContract' --verbose` exits 0 with all `TestConnectorCountContract_*` sub-tests passing (GREEN evidence captured in report.md).
- AC-07: `bash .github/bubbles/scripts/state-transition-guard.sh specs/024-design-doc-reconciliation` continues to exit at its pre-edit baseline — no new BLOCKS introduced.
- AC-08: `bash .github/bubbles/scripts/artifact-lint.sh specs/024-design-doc-reconciliation` and `bash .github/bubbles/scripts/traceability-guard.sh specs/024-design-doc-reconciliation` continue to exit 0.
- AC-09: Spec 024 `status` remains `done`. `cmd/core/connectors.go`, `internal/connector/cardrewards/`, schema, NATS topology, web template, prompt contract, Telegram command, deploy script, compose file, and `smackerel.yaml` are unchanged.
- AC-10: BUG-024-006 packet's own gate passes: `bash .github/bubbles/scripts/artifact-lint.sh specs/024-design-doc-reconciliation/bugs/BUG-024-006-s24a-connector-tree-blindspot` exits 0.
- AC-11: One-to-one finding closure: F2 closed by FR-01 (bubbles.docs); F3 closed by FR-02 (bubbles.test). No cherry-picking; both findings closed in the same packet.
- AC-12: Single commit subject prefix `bubbles(024/bug-024-006):`; path-limited `git add` discipline; zero files from unrelated in-flight specs swept in.

## Product Principle Alignment

**Principle 4 — Source-Qualified Processing.** Doc-vs-runtime connector agreement is a source-qualification contract: every connector named (or counted) in any user-facing surface of `docs/smackerel.md` must trace 1:1 to a registered `connector.Connector`. F2 silently broke that contract for the §24-A architecture tree. FR-01 restores it; FR-02 prevents recurrence by pinning §24-A.

**Principle 8 — Trust Through Transparency.** Extending the contract test to fail loudly on §24-A drift is a transparency contract: a contributor who adds the 18th connector and forgets §24-A gets an immediate mechanical signal at `./smackerel.sh test unit --go` time instead of waiting for a stochastic sweep round.

**Principle 10 — QF Companion Boundary (NON-NEGOTIABLE Cross-Product).** The §24-A tree already names `QF Decisions (qfdecisions/ — spec 041 read-only companion)`; inserting Card Rewards after it (a spec 083 read-only fetch, recommendation-only) preserves the QF boundary and the read-only framing of both finance-category companions. The Card Rewards leaf text MUST keep the read-only/no-financial-advice framing consistent with §22.7 row 17.

## Non-Goals

- This packet (the discovery+documentation phase) does **not** edit `docs/smackerel.md` or `internal/deploy/docs_connector_count_contract_test.go`. Those edits are the fix-phase deliverables owned by `bubbles.docs` (FR-01) and `bubbles.test` (FR-02) respectively.
- This packet does **not** introduce or modify the Card Rewards connector code, schema, fetch logic, or boundary semantics. Spec 083 owns all of those; this packet only reconciles the §24-A documentation surface to describe what spec 083 already shipped.
- This packet will **not** modify any production runtime code (`cmd/core/*.go`, `internal/connector/*`, `internal/api/*`, `internal/config/*`, `internal/web/*`, `internal/notification/*`, `internal/pipeline/*`), schema, NATS topology, web template, prompt contract, Telegram command, integration test, deploy script, or compose file.
- This packet will not change spec 024's overall `status` away from `done`.
- This packet will not touch §22.7, `docs/Development.md`, or the slice literal (those three surfaces are already correct at 17).
- This packet will not commit. Commit + push are the fix owners' finalize-phase responsibility.
