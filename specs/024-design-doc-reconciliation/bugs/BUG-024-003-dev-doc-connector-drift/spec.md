# Specification: BUG-024-003 docs/Development.md L31 connector-count drift + spec 024 R-006 internal inconsistency + missing automated forward-detection contract test

## Business Context

Sweep round 9 of `sweep-2026-05-24-r10` (`mode: chaos-hardening`, parent-expanded) ran the chaos trigger probe on `specs/024-design-doc-reconciliation` and surfaced three findings that survived BUG-024-002's reconcile-to-doc close-out on 2026-05-24:

1. **F1 (HIGH).** `docs/Development.md` line 31 carries `- 15 passive connectors (...)` with a 15-item parenthetical list that omits `QF Decisions` (`internal/connector/qfdecisions/`). The runtime registers 16 in `cmd/core/connectors.go` (imports L11-26, instantiate L30-47, registration slice L49-53). The same connector inventory drift that BUG-024-002 fixed in `docs/smackerel.md` §22.7 + §24-A is present in `docs/Development.md`. R-006 in spec 024 `spec.md` only enumerated `docs/smackerel.md` surfaces, so BUG-024-002's pass did not visit `docs/Development.md`.
2. **F2 (MEDIUM).** No automated framework guard or Go contract test pins agreement between the connector count in `cmd/core/connectors.go` and the count claimed by `docs/smackerel.md` §22.7 header / `docs/Development.md` line 31. A scratch simulation flipping `(16 connectors)` to `(17 connectors)` in a `/tmp/` copy passes every Bubbles guard. The drift that produced F1 here can recur the next time a connector is added.
3. **F3 (LOW).** `specs/024-design-doc-reconciliation/spec.md` is internally inconsistent: `BS-004` (L119-121) lists 16 connectors including `qfdecisions` (updated by BUG-024-002); `R-006` (L217-235) says `the 15 implemented connectors` with the same 15-item list as the docs surfaces and explicitly omits `qfdecisions`. BUG-024-002 updated BS-004 but did not propagate the change to R-006.

This packet runs as `mode: chaos-hardening` rather than `validate-to-doc` because F2's closure adds a forward-detection Go contract test under `internal/deploy/`, which is a runtime artifact that legitimately belongs to the hardening dimension (chaos found drift; hardening prevents recurrence).

## Use Cases

### UC-01: Reconcile `docs/Development.md` Connector Count With Live Registry (15 → 16)
**Actor**: A reader of `docs/Development.md` (engineer, auditor, new contributor)
**Goal**: `docs/Development.md` accurately enumerates the 16 connectors registered in `cmd/core/connectors.go`, including QF Decisions companion ingestion from spec 041.
**Outcome**: The R-006 contract that spec 024 owns is restored for this previously-overlooked surface. `grep -nE '15 passive connectors' docs/Development.md` returns 0 hits; `grep -cE 'qfdecisions|QF Decisions' docs/Development.md` ≥ 1.

### UC-02: Restore Internal Consistency Between BS-004 and R-006 in Spec 024
**Actor**: A reader of `specs/024-design-doc-reconciliation/spec.md`
**Goal**: BS-004 and R-006 agree on the connector count and enumeration.
**Outcome**: R-006 list updated from 15 to 16 with `qfdecisions` added in the same canonical ordering as BS-004 and the freshly-corrected `docs/Development.md` L31.

### UC-03: Add Automated Forward-Detection Contract Test
**Actor**: `./smackerel.sh test unit --go` (every contributor, every CI pre-push hook)
**Goal**: A Go contract test under `internal/deploy/docs_connector_count_contract_test.go` fails the test suite if the connector count claimed by `cmd/core/connectors.go` disagrees with the count claimed by `docs/smackerel.md` §22.7 header OR by `docs/Development.md` line 31.
**Outcome**: The next time a connector is added or removed without reconciling the docs, `go test ./internal/deploy/...` exits non-zero with a precise diagnostic naming which surface drifted. The test has at least three adversarial sub-tests that would fail if the contract were tautological.

### UC-04: Preserve Runtime Behavior Verbatim
**Actor**: Connector registry, every committed connector, all REST endpoints, NATS publishers, search/digest paths
**Goal**: Runtime code (`cmd/core/`, `internal/connector/qfdecisions/`, `internal/api/`, `internal/web/`, `internal/notification/`, `internal/pipeline/`), schema, NATS topology, prompt contracts, web templates, Telegram commands, deploy scripts, compose files, and `smackerel.yaml` are unchanged.
**Outcome**: `./smackerel.sh test unit --go` continues to pass with the new contract test included; baseline behavior preserved.

## Functional Requirements

### FR-01: Update `docs/Development.md` L31 From 15 to 16 Passive Connectors
**Description**: Line 31 of `docs/Development.md` must reflect the 16 connectors registered in `cmd/core/connectors.go`.
**Acceptance**: Line 31 reads `- 16 passive connectors (IMAP email, CalDAV calendar, YouTube API, RSS/Atom, Bookmarks, Browser, Google Keep/Takeout, Google Maps, Hospitable STR, GuestHost STR, Discord, Twitter/X archive, Weather via Open-Meteo, Government Alerts via USGS, Financial Markets via Finnhub/CoinGecko, QF Decisions companion via spec 041 read-only packet flow)`. The parenthetical list contains exactly 16 comma-separated items matching the canonical registration order in `cmd/core/connectors.go` line 50.

### FR-02: Update `specs/024-design-doc-reconciliation/spec.md` R-006 From 15 to 16 Connectors
**Description**: R-006 enumeration must agree with BS-004 and with the freshly-corrected `docs/Development.md` and `docs/smackerel.md` §22.7.
**Acceptance**: R-006 intro says `the 16 implemented connectors`; the bulleted list contains 16 connector entries including `- QF Decisions companion (`qfdecisions/`) — spec 041 read-only DecisionPacket ingestion, no financial advice generation` placed last to match the registration order.

### FR-03: Add `internal/deploy/docs_connector_count_contract_test.go`
**Description**: A new Go test file under `internal/deploy/` parses the connector-registration slice in `cmd/core/connectors.go`, the `### 22.7 Committed Connector Inventory (N connectors)` header in `docs/smackerel.md`, and the `- N passive connectors (...)` bullet at the top of `docs/Development.md`. The pure function `assertConnectorCountContract(connectorsGoBytes, smackerelMdBytes, developmentMdBytes []byte) error` returns nil iff all three counts agree.
**Acceptance**: File exists. Live-file test `TestConnectorCountContract_LiveFile` reads the three real files via the existing `repoRoot(t)` helper and asserts the contract holds today. At least three adversarial sub-tests prove non-tautological:
- `TestConnectorCountContract_AdversarialConnectorsGoLow` — synthetic `connectors.go` with one fewer registration than docs claim → contract MUST fail.
- `TestConnectorCountContract_AdversarialSmackerelMdHigh` — synthetic `smackerel.md` with header `(17 connectors)` → contract MUST fail.
- `TestConnectorCountContract_AdversarialDevelopmentMdLow` — synthetic `Development.md` with `- 15 passive connectors` while the other two surfaces claim 16 → contract MUST fail.

### FR-04: New Contract Test Passes Under `./smackerel.sh test unit --go`
**Description**: After FR-01 / FR-02 / FR-03 are applied together, `./smackerel.sh test unit --go ./internal/deploy/...` passes.
**Acceptance**: `go test -run TestConnectorCountContract ./internal/deploy/...` exits 0; the three adversarial sub-tests fail in synthetic mode and pass in live-file mode (i.e., the test verifies that the adversarial input triggers the expected error).

### FR-05: Single Atomic Commit Lands All Changes With Structured Prefix
**Description**: All BUG-024-003 packet artifacts + the `docs/Development.md` edit + the spec.md R-006 edit + the parent spec 024 governance backfill + the new Go contract test land in a single atomic commit whose subject begins with `bubbles(024/bug-024-003):`.
**Acceptance**: `git log --oneline -1 --format='%s'` after commit begins with `bubbles(024/bug-024-003):`. `git diff --cached --name-status` before commit shows only the files in the change set described in Scope 1's Implementation Plan; zero stray staging.

## Behavioral Specifications (Gherkin)

See `scenario-manifest.json` for the canonical 5 scenarios (BUG-024-003-SCN-001..SCN-005) that drive this packet. Their Gherkin rendering lives in `scopes.md`.

## Acceptance Criteria

- AC-01: `grep -nE '15 passive connectors' docs/Development.md` returns 0 hits.
- AC-02: `grep -nE '16 passive connectors' docs/Development.md` returns 1 hit; `grep -cE 'qfdecisions|QF Decisions' docs/Development.md` ≥ 1.
- AC-03: `specs/024-design-doc-reconciliation/spec.md` R-006 intro line says `the 16 implemented connectors`; the R-006 bullet list contains 16 entries matching the canonical registration order; BS-004 and R-006 are internally consistent.
- AC-04: `internal/deploy/docs_connector_count_contract_test.go` exists with `assertConnectorCountContract` + `TestConnectorCountContract_LiveFile` + 3 adversarial sub-tests.
- AC-05: `go test -run TestConnectorCountContract ./internal/deploy/...` exits 0 against live files; adversarial sub-tests fail when synthetic mismatch is injected and pass when the contract is preserved.
- AC-06: `./smackerel.sh test unit --go` continues to exit 0 (or matches pre-existing red baseline if any unrelated tests are already red — no new failures introduced by this packet).
- AC-07: `bash .github/bubbles/scripts/state-transition-guard.sh specs/024-design-doc-reconciliation` continues to exit 0 (or matches pre-existing PERMITTED-with-warnings state) — no new BLOCKS introduced.
- AC-08: `bash .github/bubbles/scripts/artifact-lint.sh specs/024-design-doc-reconciliation` continues to exit 0; `bash .github/bubbles/scripts/traceability-guard.sh specs/024-design-doc-reconciliation` continues to exit 0.
- AC-09: Spec 024 `status` remains `done`. `cmd/core/connectors.go`, `internal/connector/qfdecisions/`, schema, NATS topology, web template, prompt contract, Telegram command, deploy script, compose file, and `smackerel.yaml` are unchanged.
- AC-10: BUG-024-003 packet's own gates pass: `bash .github/bubbles/scripts/artifact-lint.sh specs/024-design-doc-reconciliation/bugs/BUG-024-003-dev-doc-connector-drift` exits 0.
- AC-11: Bug `state.json::status` is `resolved` with `executionHistory[]` containing complete provenance for `bug`, `analyst`, `design`, `plan`, `implement`, `test`, `validate`, `audit`, `chaos`, `docs`, `finalize` phases.
- AC-12: Single commit subject prefix `bubbles(024/bug-024-003):` confirmed; path-limited `git add` discipline confirmed via `git diff --cached --name-status`; zero files from `specs/055-*`, `specs/044-per-user-bearer-auth/state.json`, `cmd/`, `internal/connector/`, `internal/api/`, `internal/config/`, `internal/web/`, `internal/notification/`, `internal/pipeline/`, `ml/`, `config/`, `scripts/`, `smackerel.sh`, `docker-compose*`, `.github/bubbles/` are present (except the new `internal/deploy/docs_connector_count_contract_test.go` file which is the FR-03 deliverable).

## Product Principle Alignment

**Principle 4 — Source-Qualified Processing.** Doc-vs-runtime connector-count agreement is a source-qualification contract: every connector named in any user-facing doc must trace 1:1 to a registered `connector.Connector` implementation. F1 silently broke that contract for `docs/Development.md`. FR-01 restores it; FR-03 prevents recurrence by adding the forward-detection contract test.

**Principle 8 — Trust Through Transparency.** Adding a Go contract test that fails loudly on drift is a transparency contract: any contributor (or auditor) who adds a connector and forgets to reconcile any of the three pinned surfaces gets an immediate mechanical signal at `./smackerel.sh test unit --go` time instead of waiting for a stochastic sweep round to catch it months later.

**Principle 10 — QF Companion Boundary (NON-NEGOTIABLE Cross-Product).** The new `docs/Development.md` parenthetical text and R-006 bullet both explicitly call out `QF Decisions companion via spec 041 read-only packet flow` and `spec 041 read-only DecisionPacket ingestion, no financial advice generation` respectively, preserving the boundary that spec 041 owns.

## Non-Goals

- This packet is **not** introducing or modifying the QF Decisions connector code, schema, packet shape, NATS topology, or boundary semantics. Spec 041 owns all of those; this packet only reconciles `docs/Development.md` and spec 024 `spec.md` R-006 to accurately describe what spec 041 already shipped.
- This packet will **not** modify any production runtime code (`cmd/core/*.go`, `internal/connector/*`, `internal/api/*`, `internal/config/*`, `internal/web/*`, `internal/notification/*`, `internal/pipeline/*`), schema, NATS topology, web template, prompt contract, Telegram command, integration test, unit test, deploy script, or compose file. The single Go file added under `internal/deploy/` is a test-only file (`_test.go` suffix) that does not compile into any binary.
- This packet will not change spec 024's overall `status` away from `done`.
- This packet will not weaken any framework guard.
- This packet will not touch any in-flight WIP under `specs/055-*` or `specs/044-per-user-bearer-auth/state.json` even if those files appear in `git status` at HEAD. Path-limited `git add` enforces this.
- This packet will not update the sweep ledger `.specify/memory/sweep-2026-05-24-r10.json` inside the same commit; the ledger update is a local-only post-commit step (matching round-29 / round 7 / round 8 precedent).
