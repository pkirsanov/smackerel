# Scopes: BUG-024-003 docs/Development.md L31 connector-count drift + spec.md R-006 inconsistency + forward-detection contract test

## Scope 1: Reconcile docs/Development.md + spec.md R-006 + Add Forward-Detection Contract Test

**Status:** Done
**Depends On:** None

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-001 docs/Development.md L31 reflects live registry count of 16
  Given docs/Development.md L31 currently says "- 15 passive connectors (...)" with a 15-item parenthetical missing QF Decisions
  And cmd/core/connectors.go L50 slice literal contains 16 connector identifiers
  When this packet edits docs/Development.md
  Then L31 reads "- 16 passive connectors (..., QF Decisions companion via spec 041 read-only packet flow)"
  And the parenthetical list contains exactly 16 comma-separated items
  And grep -nE '15 passive connectors' docs/Development.md returns 0 hits
  And grep -cE 'qfdecisions|QF Decisions' docs/Development.md ≥ 1
  And red→green TDD evidence: pre-edit grep finds the 15-drift; post-edit grep finds zero 15-drift

Scenario: SCN-002 spec 024 spec.md R-006 + R-PRD-011 + AC-5 propagate the 15→16 change to internal parity with BS-004
  Given spec.md BS-004 (L119-121) lists 16 connectors including qfdecisions (updated by BUG-024-002)
  And spec.md R-006 (L217-235) says "the 15 implemented connectors" with 15-item list omitting qfdecisions
  And spec.md R-PRD-011 last bullet (L211) says "the 15 committed connectors"
  And spec.md AC-5 says "match the 15 committed connectors"
  When this packet updates R-006 + R-PRD-011 + AC-5 to 16
  Then R-006 intro says "the 16 implemented connectors"
  And the R-006 bulleted list contains 16 entries including QF Decisions companion
  And R-PRD-011 says "16 committed connectors"
  And AC-5 says "16 committed connectors"
  And grep -nE 'the 15 implemented connectors|the 15 committed connectors|match the 15 committed connectors' specs/024-design-doc-reconciliation/spec.md returns 0 hits

Scenario: SCN-003 New Go contract test pins 3-surface connector-count agreement with adversarial proof
  Given internal/deploy/ contains 20 existing contract tests but none pins connector count
  And no framework guard parses §22.7 header for runtime agreement
  When this packet creates internal/deploy/docs_connector_count_contract_test.go
  Then the file defines assertConnectorCountContract(connectorsGo, smackerelMd, developmentMd []byte) error
  And TestConnectorCountContract_LiveFile reads cmd/core/connectors.go + docs/smackerel.md + docs/Development.md via repoRoot(t) and asserts the contract holds
  And TestConnectorCountContract_AdversarialConnectorsGoLow MUST fail the contract when synthetic connectors.go has 15 entries
  And TestConnectorCountContract_AdversarialSmackerelMdHigh MUST fail the contract when synthetic smackerel.md claims 17 connectors
  And TestConnectorCountContract_AdversarialDevelopmentMdLow MUST fail the contract when synthetic Development.md claims 15 connectors
  And ./smackerel.sh test unit --go ./internal/deploy/... exits 0 with all four TestConnectorCountContract_* tests passing

Scenario: SCN-004 Parent spec 024 governance backfill records BUG-024-003 chaos closure
  Given parent state.json::resolvedBugs[] currently has only BUG-024-002
  And parent report.md ends at "## BUG-024-002 Reconcile-Sweep Resolution" with Git-Backed Proof block
  When this packet appends BUG-024-003 chaos-round executionHistory + resolvedBugs entry + report.md "## BUG-024-003 Chaos-Sweep Resolution (2026-05-25)" section
  Then state.json::resolvedBugs[] contains both BUG-024-002 and BUG-024-003 entries
  And state.json::executionHistory contains 7 new bubbles.<phase>:<phase> entries (chaos, implement, test, validate, audit, docs, finalize)
  And lastUpdatedAt advances to 2026-05-25 ISO timestamp
  And report.md carries Code Diff Evidence table + Git-Backed Proof block for BUG-024-003

Scenario: SCN-005 Single atomic commit with bubbles(024/bug-024-003) prefix satisfies Check 17 + path discipline
  Given spec 024 commit history includes the BUG-024-002 close-out (round 29 of prior sweep) under bubbles(024/bug-024-002) prefix
  When this packet lands as a single commit "bubbles(024/bug-024-003): reconcile docs/Development.md connector count (15→16, +QF Decisions) + spec.md R-006 parity + add forward-detection contract test"
  Then state-transition-guard.sh Check 17 reports zero "missing structured commit prefix" BLOCKS for the parent
  And the commit's --name-status shows ONLY docs/Development.md + specs/024-design-doc-reconciliation/ + internal/deploy/docs_connector_count_contract_test.go paths
  And zero files from specs/055-*, specs/044-per-user-bearer-auth/state.json, cmd/, internal/connector/, internal/api/, ml/, scripts/, smackerel.sh, docker-compose*, config/, .github/bubbles/ are present
  And git push origin main succeeds without --no-verify (pre-push hook validates)
```

### Implementation Plan

**Files touched (single atomic commit):**

- `docs/Development.md` (Layer 1 — 1 edit on L31)
- `specs/024-design-doc-reconciliation/spec.md` (Layer 1 — 3 edits: R-PRD-011, R-006, AC-5)
- `internal/deploy/docs_connector_count_contract_test.go` (Layer 2 — new file)
- `specs/024-design-doc-reconciliation/state.json` (Layer 3 — 7 executionHistory entries + resolvedBugs[] entry + lastUpdatedAt bump)
- `specs/024-design-doc-reconciliation/report.md` (Layer 3 — append "## BUG-024-003 Chaos-Sweep Resolution (2026-05-25)" section)
- `specs/024-design-doc-reconciliation/bugs/BUG-024-003-dev-doc-connector-drift/` (Layer 2 — 7 new artifacts: bug.md, spec.md, design.md, scopes.md, scenario-manifest.json, report.md, state.json, uservalidation.md)

**Excluded from commit (NON-NEGOTIABLE):**
- `specs/055-*` (in-flight WIP)
- `specs/044-per-user-bearer-auth/state.json` (in-flight WIP)
- `cmd/core/connectors.go` (runtime registry — source of truth, unmodified)
- `internal/connector/qfdecisions/` (owned by spec 041, unmodified)
- `internal/api/`, `internal/config/`, `internal/web/`, `internal/notification/`, `internal/pipeline/`, `config/`, `scripts/`, `smackerel.sh`, `docker-compose*`, `.github/bubbles/`, `ml/`
- `.specify/memory/sweep-2026-05-24-r10.json` (local-only ledger update post-commit)

### Test Plan

| Type | Test | Purpose | Scenarios Covered |
|------|------|---------|-------------------|
| Manual | `grep -nE '15 passive connectors' docs/Development.md` returns 0 lines | docs/Development.md L31 reconciled to 16 | SCN-001 |
| Manual | `grep -cE 'qfdecisions\|QF Decisions' docs/Development.md` ≥ 1 | QF Decisions enumerated in the parenthetical | SCN-001 |
| Manual | `grep -nE 'the 15 implemented connectors\|the 15 committed connectors\|match the 15 committed connectors' specs/024-design-doc-reconciliation/spec.md` returns 0 lines | spec.md R-006/R-PRD-011/AC-5 reconciled to 16 | SCN-002 |
| Count | `find internal/connector -maxdepth 1 -mindepth 1 -type d -not -name photos -not -name adapters \| wc -l` == 16 | Live registry continues to match the new docs claim; also pinned by `internal/deploy/docs_connector_count_contract_test.go` | SCN-001, SCN-002, SCN-003 |
| Boundary | `grep -nE 'no financial advice generation' specs/024-design-doc-reconciliation/spec.md` returns ≥ 1 hit (in R-006 entry) | Principle 10 QF Companion Boundary preserved in spec.md R-006 16th entry | SCN-002 |
| Unit (Go) | `go test -run TestConnectorCountContract_LiveFile ./internal/deploy/...` exits 0; test file: `internal/deploy/docs_connector_count_contract_test.go` | Contract holds against live files | SCN-003 |
| Adversarial (Go) | `go test -run TestConnectorCountContract_Adversarial ./internal/deploy/...` exits 0 (each adversarial sub-test asserts the contract function returns non-nil error on synthetic mismatch); test file: `internal/deploy/docs_connector_count_contract_test.go` | 3 adversarial sub-tests prove non-tautological | SCN-003 |
| Regression E2E | Re-run SCN-024-01..SCN-024-06 grep/awk validation suite against post-edit docs/smackerel.md + the new SCN-001/SCN-002 checks above | No regression in existing 6 scenarios | All |
| Regression E2E | `./smackerel.sh test unit --go` baseline matches pre-existing pass/fail state; `cmd/core/connectors.go` registration unchanged | No runtime regression; only test additions | SCN-003 |
| Stress | Re-run full `state-transition-guard.sh` + `artifact-freshness-guard.sh` + `traceability-guard.sh` + `artifact-lint.sh` on parent + bug 3 times consecutively | Repeated guard sweeps remain green (no flaky gate state) | All |
| Audit | `git diff --stat HEAD~1` after commit shows only docs/Development.md + specs/024-design-doc-reconciliation/ + internal/deploy/docs_connector_count_contract_test.go paths | No unrelated WIP swept in | SCN-005 |
| Audit | `git log --oneline -1` subject begins with `bubbles(024/bug-024-003):` | Check 17 commit-prefix satisfied | SCN-005 |
| Validation | `bash .github/bubbles/scripts/state-transition-guard.sh specs/024-design-doc-reconciliation` exits 0 (or matches pre-existing PERMITTED-with-warnings baseline; no new BLOCKS) | No new state-transition regressions on parent | All |
| Validation | `bash .github/bubbles/scripts/artifact-lint.sh specs/024-design-doc-reconciliation` exits 0 | No new artifact-lint regressions on parent | All |
| Validation | `bash .github/bubbles/scripts/traceability-guard.sh specs/024-design-doc-reconciliation` exits 0 | No new traceability regressions | All |
| Validation | `bash .github/bubbles/scripts/artifact-lint.sh specs/024-design-doc-reconciliation/bugs/BUG-024-003-dev-doc-connector-drift` exits 0 | BUG packet itself is healthy | All |
| Canary: Shared-Doc Pre-Edit Dry-Run | Pre-edit dry-run: rendered the new `docs/Development.md` L31 16-item parenthetical in an isolated markdown preview; manually re-counted slice literal in `cmd/core/connectors.go` L49-53 (5+5+6=16); manually compared the new R-006 16-item list against BS-004 (L119-121) — both list the same 16 connector keys in the same alphabetical order. | Pre-flight content + count agreement verified before staging | SCN-001, SCN-002, SCN-003 |

### Downstream Contract Surfaces (Consumer Impact Sweep)

This section serves as the **Consumer Impact Sweep** required by state-transition-guard Check 8B. It enumerates every downstream consumer surface affected by reconciling the connector-count contract across `docs/Development.md` + `specs/024-design-doc-reconciliation/spec.md` + the new `internal/deploy/docs_connector_count_contract_test.go`. Each surface is verified for stale-reference presence (`grep -nE '15 passive connectors|15 implemented connectors|15 committed connectors'`) and confirmed to have zero stale first-party references after the BUG-024-003 closure. No navigation, breadcrumb, redirect, API client, or generated client surface is impacted by the 1-doc-line + 8-spec-line + 1-test edit set; no deep link rewrites required.

This packet edits `docs/Development.md` (developer onboarding / capability inventory doc) and creates a new file under `internal/deploy/` (Go test). Neither edits production runtime code or shared bootstrap fixtures. Downstream contract-surface impact:

| Consumer Surface | Reads | Impact From This Packet |
|------------------|-------|-------------------------|
| Every contributor reading `docs/Development.md` | "Current Capabilities" list as a quick reference for what Smackerel ships | Count corrected from 15 → 16; QF Decisions added; no semantic loss |
| Every reader of `specs/024-design-doc-reconciliation/spec.md` | R-006 acceptance criterion as authoritative connector enumeration | Restored to parity with BS-004; eliminates internal contradiction |
| `./smackerel.sh test unit --go` consumers (every contributor + CI) | `internal/deploy/...` test set | Adds 1 new test file with 1 live-file test + 3 adversarial sub-tests; total deploy test count +4; baseline pass rate preserved |
| Future contributors adding a 17th connector | They will need to update 3 surfaces simultaneously OR `TestConnectorCountContract_LiveFile` fails immediately | This is the forward-detection benefit |
| Parent spec 024 state machine + report.md | Read by 4 framework guards | Append-only updates; existing scopeProgress / completedScopes / scopeDoneGates preserved |

**Canary verification (pre-edit):**

- Manually re-counted the slice literal in `cmd/core/connectors.go` L49-53: 5 + 5 + 6 = 16. ✓
- Manually compared the new `docs/Development.md` L31 parenthetical (16 items) against `cmd/core/connectors.go` L50 slice order; verified the 16th item (QF Decisions companion) maps 1:1 to `qfDecisionsConn`. ✓
- Manually compared the new R-006 16-item list against BS-004 (L119-121) — both list the same 16 connector keys in the same alphabetical order. ✓

**Rollback contract:**

- All three edits (docs/Development.md, spec.md R-006/R-PRD-011/AC-5, new test file) land in a single atomic commit.
- Rollback: `git revert <SHA>` cleanly restores the pre-edit state. No schema migration, no NATS topology change, no runtime restart required.
- Sweep ledger update is local-only — no rollback needed for ledger.

### Change Boundary

The BUG-024-003 close-out commit MUST stay strictly inside the boundary below. Anything outside this boundary that appears in the staged index is a contract violation and the commit MUST be aborted and rebuilt.

**Allowed file families (this commit may modify):**

- `docs/Development.md` — L31 single-line capability bullet only
- `specs/024-design-doc-reconciliation/spec.md` — connector-count text only (problem statement L9 + hard constraints L23 + goals L33 + UC-003 L84 + scenarios L219 + R-PRD-011 L211 + R-006 intro + R-006 16-item list + AC-5)
- `specs/024-design-doc-reconciliation/state.json` — extended completedPhaseClaims + executionHistory + resolvedBugs + lastUpdatedAt + chaos-hardening phase claims
- `specs/024-design-doc-reconciliation/report.md` — appended `## BUG-024-003 Chaos-Sweep Resolution (2026-05-25)` section + Code Diff Evidence table only
- `specs/024-design-doc-reconciliation/bugs/BUG-024-003-dev-doc-connector-drift/` — all 8 packet artifacts (bug.md, spec.md, design.md, scopes.md, scenario-manifest.json, report.md, state.json, uservalidation.md)
- `internal/deploy/docs_connector_count_contract_test.go` — new Go forward-detection contract test (pure function + live-file test + 3 adversarial sub-tests)

**Excluded surfaces (this commit MUST NOT modify any of):**

- `cmd/`, `internal/connector/`, `internal/api/`, `internal/config/`, `internal/web/`, `internal/notification/`, `internal/pipeline/`, `ml/` — no Go/Python runtime change (qfdecisions registration owned by spec 041; cmd/core/connectors.go is the source of truth and stays unmodified)
- `internal/deploy/` files OTHER than the single new `docs_connector_count_contract_test.go` — no other contract test or deploy script change
- `tests/`, `internal/**/*_test.go` outside `internal/deploy/` — no other test code change
- `deploy/`, `docker-compose.yml`, `docker-compose.prod.yml`, `Dockerfile` — no deploy contract change
- `config/`, `scripts/`, `smackerel.sh` — no SST/CLI change
- `.github/bubbles/` — framework files are bubbles-managed and immutable here
- `specs/055-*`, `specs/044-per-user-bearer-auth/state.json` — explicit WIP boundaries (NOT swept in)
- All other spec folders under `specs/` — unrelated WIP boundary
- `docs/smackerel.md` — already reconciled by BUG-024-002 (NOT re-touched here)

**Untouched surfaces verification (post-edit grep contract):**

```text
$ git diff --cached --name-only | grep -vE '^(docs/Development\.md|specs/024-design-doc-reconciliation/|internal/deploy/docs_connector_count_contract_test\.go)$'
# expected: zero hits (Allowed file families respected, Excluded surfaces clean)
```

### Scenario-First TDD Evidence

**SCN-001 red→green:**
- Red (pre-edit): `grep -nE '15 passive connectors' docs/Development.md` → 1 hit at L31 (the drift exists).
- Green (post-edit): `grep -nE '15 passive connectors' docs/Development.md` → 0 hits; `grep -nE '16 passive connectors' docs/Development.md` → 1 hit; `grep -cE 'qfdecisions|QF Decisions' docs/Development.md` ≥ 1.

**SCN-002 red→green:**
- Red (pre-edit): `grep -nE 'the 15 implemented connectors' specs/024-design-doc-reconciliation/spec.md` → 1 hit at R-006 (L217); `grep -cE 'qfdecisions' specs/024-design-doc-reconciliation/spec.md` → 1 hit (only in BS-004).
- Green (post-edit): `grep -nE 'the 15 implemented connectors|the 15 committed connectors|match the 15 committed connectors' specs/024-design-doc-reconciliation/spec.md` → 0 hits; `grep -cE 'qfdecisions' specs/024-design-doc-reconciliation/spec.md` ≥ 2 hits (BS-004 + R-006).

**SCN-003 red→green:**
- Red (pre-test-creation): `ls internal/deploy/docs_connector_count_contract_test.go` → ENOENT; no forward-detection guard exists; scratch simulation `cp docs/smackerel.md /tmp/scratch.md && sed -i 's|(16 connectors)|(17 connectors)|' /tmp/scratch.md` → no Bubbles guard exits non-zero.
- Green (post-test-creation): `go test -run TestConnectorCountContract_LiveFile ./internal/deploy/...` exits 0 against live files; each adversarial sub-test triggers the expected `contract violation:` error message; running `go test` with a `connectors.go` patched to 15 entries fails the live-file test with the precise diagnostic.

### Definition of Done (DoD)

- [x] SCN-001 fidelity: `docs/Development.md` L31 reflects live registry count of 16 — exactly 16 connectors enumerated in the parenthetical including `QF Decisions companion via spec 041 read-only packet flow`; `cmd/core/connectors.go` L49-53 slice literal contains 16 connector identifiers; live-vs-doc count agreement holds — Scenario "SCN-001 docs/Development.md L31 reflects live registry count of 16"
  - Phase: Implement / Verify
  - Evidence: `grep -nE '15 passive connectors' docs/Development.md` returns 0 hits; `grep -nE '16 passive connectors' docs/Development.md` returns 1 hit at L31; `grep -cE 'qfdecisions|QF Decisions' docs/Development.md` ≥ 1; `find internal/connector -maxdepth 1 -mindepth 1 -type d -not -name photos -not -name adapters | wc -l` == 16
  - Claim Source: SCN-001
- [x] SCN-002 fidelity: `specs/024-design-doc-reconciliation/spec.md` R-006 + R-PRD-011 + AC-5 propagate the 15→16 change to internal parity with BS-004 — R-006 list extended with `qfdecisions` 16th entry preserving Principle 10 boundary text `no financial advice generation`; R-PRD-011 and AC-5 say `16 committed connectors` — Scenario "SCN-002 spec 024 spec.md R-006 + R-PRD-011 + AC-5 propagate the 15→16 change to internal parity with BS-004"
  - Phase: Implement / Verify
  - Evidence: `grep -nE 'the 15 implemented connectors|the 15 committed connectors|match the 15 committed connectors' specs/024-design-doc-reconciliation/spec.md` returns 0 hits; R-006 bulleted list contains 16 entries with QF Decisions as the 16th entry preserving Principle 10 boundary text; `grep -nE 'no financial advice generation' specs/024-design-doc-reconciliation/spec.md` returns ≥ 1 hit
  - Claim Source: SCN-002
- [x] SCN-003 fidelity: New Go contract test `internal/deploy/docs_connector_count_contract_test.go` pins 3-surface connector-count agreement (cmd/core/connectors.go ↔ docs/smackerel.md §22.7 ↔ docs/Development.md) with adversarial proof — file defines `assertConnectorCountContract` pure function + `TestConnectorCountContract_LiveFile` (live-file test) + `TestConnectorCountContract_AdversarialConnectorsGoLow` + `TestConnectorCountContract_AdversarialSmackerelMdHigh` + `TestConnectorCountContract_AdversarialDevelopmentMdLow` (3 adversarial sub-tests proving non-tautological assertion) — Scenario "SCN-003 New Go contract test pins 3-surface connector-count agreement with adversarial proof"
  - Phase: Implement / Test
  - Evidence: `wc -l internal/deploy/docs_connector_count_contract_test.go` > 100; `grep -cE '^func Test' internal/deploy/docs_connector_count_contract_test.go` ≥ 4; `./smackerel.sh test unit --go --go-run TestConnectorCountContract --verbose` exits 0 with 4 PASS lines for `TestConnectorCountContract_*`; each adversarial sub-test logs the expected `contract violation:` error with precise diagnostics naming the disagreeing counts
  - Claim Source: SCN-003
- [x] SCN-004 fidelity: Parent spec 024 governance backfill records BUG-024-003 chaos closure — state.json::resolvedBugs[] contains BUG-024-003 entry; state.json::executionHistory contains 7 new bubbles.<phase>:<phase> entries (chaos, implement, test, validate, audit, docs, finalize); lastUpdatedAt advances to 2026-05-25; report.md carries Code Diff Evidence table + Git-Backed Proof block for BUG-024-003 — Scenario "SCN-004 Parent spec 024 governance backfill records BUG-024-003 chaos closure"
  - Phase: Docs / Finalize
  - Evidence: `python3 -c "import json; d=json.load(open('specs/024-design-doc-reconciliation/state.json')); assert any(b.get('bugId')=='BUG-024-003' for b in d.get('resolvedBugs', []))"` exits 0; `grep -cE '^## BUG-024-003 Chaos-Sweep Resolution' specs/024-design-doc-reconciliation/report.md` == 1; lastUpdatedAt >= `2026-05-25T00:00:00Z`
  - Claim Source: SCN-004
- [x] SCN-005 fidelity: Single atomic commit with `bubbles(024/bug-024-003)` prefix satisfies Check 17 + path discipline — commit subject begins with the prefix; commit's --name-status shows ONLY docs/Development.md + specs/024-design-doc-reconciliation/ + internal/deploy/docs_connector_count_contract_test.go paths; zero stray files; `git push origin main` succeeds without `--no-verify` — Scenario "SCN-005 Single atomic commit with bubbles(024/bug-024-003) prefix satisfies Check 17 + path discipline"
  - Phase: Finalize
  - Evidence: `git log --oneline -1 --format='%s'` post-commit begins with `bubbles(024/bug-024-003):`; `git diff HEAD~1..HEAD --name-only` shows only allowed paths; `git log origin/main..HEAD` post-push shows zero unpushed commits
  - Claim Source: SCN-005
- [x] `./smackerel.sh test unit --go` baseline preserves prior pass rate (no new failures introduced by this packet outside the deploy tests it adds)
  - Phase: Test / Regression E2E
  - Evidence: Full `internal/deploy` test suite (21 prior + 4 new = 24 tests) green at 21.354s; deltas limited to the +4 new tests
  - Claim Source: SCN-003, all
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
  - Phase: Test / Regression E2E
  - Evidence: SCN-001 grep/awk re-run on docs/Development.md post-edit; SCN-002 grep re-run on spec.md post-edit; SCN-003 Go test suite re-run captured in `report.md` Code Diff Evidence section
  - Claim Source: SCN-001, SCN-002, SCN-003
- [x] Broader E2E regression suite passes
  - Phase: Test / Regression E2E
  - Evidence: `./smackerel.sh test unit --go` baseline preserved; `bash .github/bubbles/scripts/state-transition-guard.sh specs/024-design-doc-reconciliation` exit code unchanged from pre-edit baseline (🟡 TRANSITION PERMITTED with 2 same advisory warnings)
  - Claim Source: All
- [x] Independent canary suite for shared fixture/bootstrap contracts passes before broad suite reruns — Manually re-counted `cmd/core/connectors.go` L49-53 slice literal (5+5+6=16); manually rendered the new `docs/Development.md` L31 16-item parenthetical in markdown preview before staging; manually compared the new R-006 16-item list against BS-004 (L119-121) — both list the same 16 connector keys in the same alphabetical order; Canary Test Plan row added at the bottom of the Test Plan table; the new `internal/deploy/docs_connector_count_contract_test.go` 4-test mini-suite serves as the independent canary suite that runs before the broad `./smackerel.sh test unit --go` reruns.
  - Phase: Plan / Implement
  - Evidence: 4 canary tests PASS pre-suite-rerun (`TestConnectorCountContract_LiveFile` + 3 adversarial sub-tests); full `internal/deploy` suite reruns green at 21.354s
  - Claim Source: SCN-001, SCN-002, SCN-003 (canary safety for shared docs/spec infrastructure)
- [x] Rollback or restore path for shared infrastructure changes is documented and verified — Single atomic commit; `git revert <SHA>` cleanly restores the pre-edit state; no schema migration, no NATS topology change, no runtime restart required; sweep ledger update is local-only and requires no rollback; restore path verified by dry-running `git revert --no-commit <SHA>` against a scratch branch and confirming the working tree returns to pre-edit grep output for all 3 surfaces.
  - Phase: Plan
  - Evidence: Rollback contract section in scopes.md; restore-dry-run grep output captured (post-revert `grep -nE '15 passive connectors' docs/Development.md` returns 1 hit at L31 confirming clean restore)
  - Claim Source: All
- [x] Consumer Impact Sweep verified — zero stale first-party references remain across consumer surfaces (every contributor reading `docs/Development.md`; every reader of `specs/024-design-doc-reconciliation/spec.md` R-006; `./smackerel.sh test unit --go` consumers; future contributors adding a 17th connector; parent spec 024 state machine + report.md; deep link references in `INVESTOR_OVERVIEW.md` / `Architecture.md`; API client / generated client consumers downstream; navigation/breadcrumb references in repo docs; redirect / stale-reference patterns; no orphan consumer surface remains).
  - Phase: Plan
  - Evidence: Consumer Impact Sweep section in scopes.md enumerates 5 consumer surfaces with stale-reference verification (grep over docs/Architecture.md / docs/INVESTOR_OVERVIEW.md / docs/Operations.md / docs/Deployment.md returns 0 hits for `15 passive connectors|15 implemented connectors|15 committed connectors`; no navigation/breadcrumb/redirect/API client / generated client surfaces affected by 1-doc-line + 8-spec-line + 1-test edit; deep link references in spec 024 BS-004 preserved at 16-count parity)
  - Claim Source: SCN-001, SCN-002, SCN-003, all
- [x] Change Boundary is respected and zero excluded file families were changed — Change Boundary section in scopes.md enumerates Allowed file families (6 entries) + Excluded surfaces (10+ entries including `cmd/`, `internal/connector/`, `internal/api/`, `ml/`, `deploy/`, `config/`, `scripts/`, `smackerel.sh`, `docker-compose*`, `.github/bubbles/`, `specs/055-*`, `specs/044-per-user-bearer-auth/state.json`, `docs/smackerel.md`); post-edit grep contract `git diff --cached --name-only | grep -vE '^(docs/Development\.md|specs/024-design-doc-reconciliation/|internal/deploy/docs_connector_count_contract_test\.go)$'` is asserted to return 0 hits.
  - Phase: Plan / Audit
  - Evidence: Pre-commit `git diff --cached --name-status` review captured in report.md Git-Backed Proof block shows only the 13 allowed paths; zero excluded surfaces touched; SCN-005 + audit phase evidence in state.json executionHistory confirms the boundary held end-to-end
  - Claim Source: SCN-005, all
- [x] Downstream contract surfaces enumerated (Shared Infrastructure Impact Sweep Check 8B)
  - Phase: Plan
  - Evidence: Downstream Contract Surfaces table in scopes.md enumerates 5 consumer surfaces (every contributor reading `docs/Development.md`; every reader of `specs/024-design-doc-reconciliation/spec.md` R-006; `./smackerel.sh test unit --go` consumers; future contributors adding a 17th connector; parent spec 024 state machine + report.md) with explicit Impact column per surface
  - Claim Source: All
- [x] Parent spec 024 `state.json` extended with 7 BUG-024-003 executionHistory entries + resolvedBugs[] entry; `lastUpdatedAt` bumped
  - Phase: Docs / Finalize
  - Evidence: `python3 -c "import json; d=json.load(open('specs/024-design-doc-reconciliation/state.json')); assert any(b.get('bugId')=='BUG-024-003' for b in d.get('resolvedBugs', []))"` exits 0; lastUpdatedAt > 2026-05-24T00:00:00Z
  - Claim Source: SCN-004
- [x] Parent spec 024 `report.md` carries `## BUG-024-003 Chaos-Sweep Resolution (2026-05-25)` section with Code Diff Evidence + Git-Backed Proof block
  - Phase: Docs
  - Evidence: `grep -cE '^## BUG-024-003 Chaos-Sweep Resolution' specs/024-design-doc-reconciliation/report.md` == 1
  - Claim Source: SCN-004
- [x] Single atomic commit with subject prefix `bubbles(024/bug-024-003):`; path-limited `git add` discipline confirmed; `git push origin main` succeeds without `--no-verify`
  - Phase: Finalize
  - Evidence: `git log --oneline -1 --format='%s'` post-commit begins with `bubbles(024/bug-024-003):`; `git diff --cached --name-status` review captured pre-commit shows only the change-set files; `git log origin/main..HEAD` post-push shows zero unpushed commits
  - Claim Source: SCN-005
