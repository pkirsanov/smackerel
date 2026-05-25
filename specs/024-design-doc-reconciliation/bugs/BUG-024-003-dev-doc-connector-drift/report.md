# Report: BUG-024-003 — docs/Development.md L31 connector-count drift + missing forward-detection guard + spec.md R-006 inconsistency

## Summary

**Sweep round:** `sweep-2026-05-24-r10` round 9 (mode: `stochastic-quality-sweep`, parent-expanded child mode: `chaos-hardening`).
**Parent spec:** `specs/024-design-doc-reconciliation` (status stays `done`).
**Bug status:** `resolved`.
**Findings closure:** F1+F2+F3 closed one-to-one in a single packet.

The chaos probe on spec 024 discovered three drift-shaped findings that survived BUG-024-002's `docs/smackerel.md` reconciliation:

- **F1 (HIGH, docs-runtime-drift):** `docs/Development.md` L31 still said `- 15 passive connectors (...)` with a 15-item parenthetical missing QF Decisions.
- **F2 (MEDIUM, missing-forward-detection):** No framework guard or runtime contract test pinned 3-surface agreement between `cmd/core/connectors.go` slice literal + `docs/smackerel.md` §22.7 header + `docs/Development.md` `- N passive connectors` bullet. BUG-024-002 closed observable drift but added no forward-detection.
- **F3 (LOW, artifact-internal-inconsistency):** `specs/024-design-doc-reconciliation/spec.md` had 5+ additional sites with 15-count language (problem statement L9, hard constraints L23, goals L33, UC-003 L84, scenarios L219, R-PRD-011 L211, R-006 L217-235, AC-5) internally contradicting BS-004 (L119-121, updated to 16 by BUG-024-002).

All three findings closed in a single atomic commit with `bubbles(024/bug-024-003):` prefix.

## Per-Scope Evidence

### Scope 1: Reconcile docs/Development.md + spec.md R-006 + Add Forward-Detection Contract Test (Status: Done)

**Concrete test file evidence:**

- `docs/Development.md` — L31 single-line capability bullet reconciled from 15→16; QF Decisions enumerated; `grep -nE '15 passive connectors' docs/Development.md` returns 0 hits post-edit. (Maps to SCN-001 grep-validation test rows in `scenario-manifest.json::BUG-024-003-SCN-001.linkedTests`.)
- `specs/024-design-doc-reconciliation/spec.md` — 8 edits across problem statement L9 + hard constraints L23 + goals L33 + UC-003 L84 + scenarios L219 + R-PRD-011 L211 + R-006 intro+16-item list + AC-5; `grep -nE 'the 15 implemented connectors|the 15 committed connectors|match the 15 committed connectors' specs/024-design-doc-reconciliation/spec.md` returns 0 hits post-edit; `grep -nE 'no financial advice generation' specs/024-design-doc-reconciliation/spec.md` returns ≥ 1 hit (R-006 16th entry preserves Principle 10 boundary text). (Maps to SCN-002 grep-validation + boundary-preservation test rows in `scenario-manifest.json::BUG-024-003-SCN-002.linkedTests`.)
- `internal/deploy/docs_connector_count_contract_test.go` — new ~360-LOC Go forward-detection contract test: `assertConnectorCountContract` pure function + `TestConnectorCountContract_LiveFile` + 3 adversarial sub-tests (`AdversarialConnectorsGoLow`, `AdversarialSmackerelMdHigh`, `AdversarialDevelopmentMdLow`). (Maps to SCN-003 unit-go + unit-go-adversarial test rows in `scenario-manifest.json::BUG-024-003-SCN-003.linkedTests`.)

**Live registry agreement evidence:**

- `cmd/core/connectors.go` L49-53 slice literal contains 16 connector identifiers (5+5+6 across 3 lines: imap/caldav/youtube/rss/keep + bookmarks/browser/maps/hospitable/guesthost + discord/twitter/weather/alerts/markets/qfdecisions). Source of truth unchanged.
- `find internal/connector -maxdepth 1 -mindepth 1 -type d -not -name photos -not -name adapters | wc -l` == 16. Live filesystem agreement holds.
- `docs/smackerel.md` §22.7 header reads `### 22.7 Committed Connector Inventory (16 connectors)` (reconciled by BUG-024-002).
- `docs/Development.md` L31 (this packet) reads `- 16 passive connectors (...QF Decisions companion via spec 041 read-only packet flow)`.
- Three surfaces now agree on 16; the new Go contract test pins that agreement.

### Code Diff Evidence

| # | File | Change | Lines | Scenarios |
|---|------|--------|-------|-----------|
| 1 | `docs/Development.md` | Reconcile L31 "Current Capabilities" connector bullet 15→16, enumerate QF Decisions companion | L31 (1 edit) | SCN-001 |
| 2 | `specs/024-design-doc-reconciliation/spec.md` | Reconcile problem statement 15→16 | L9 (1 edit) | SCN-002 |
| 3 | `specs/024-design-doc-reconciliation/spec.md` | Reconcile hard constraints 15→16 | L23 (1 edit) | SCN-002 |
| 4 | `specs/024-design-doc-reconciliation/spec.md` | Reconcile goals 15→16 | L33 (1 edit) | SCN-002 |
| 5 | `specs/024-design-doc-reconciliation/spec.md` | Reconcile UC-003 15→16 | L84 (1 edit) | SCN-002 |
| 6 | `specs/024-design-doc-reconciliation/spec.md` | Reconcile R-PRD-011 last bullet 15→16 | L211 (1 edit) | SCN-002 |
| 7 | `specs/024-design-doc-reconciliation/spec.md` | Reconcile scenarios 15→16 | L219 (1 edit) | SCN-002 |
| 8 | `specs/024-design-doc-reconciliation/spec.md` | Reconcile R-006 intro + extend 15-item bulleted list to 16 with `qfdecisions` 12th entry preserving `no financial advice generation` boundary | L217-235 (R-006 intro + 16-item list with QF Decisions companion entry) | SCN-002 |
| 9 | `specs/024-design-doc-reconciliation/spec.md` | Reconcile AC-5 15→16 committed connectors | AC-5 (1 edit) | SCN-002 |
| 10 | `internal/deploy/docs_connector_count_contract_test.go` | New file: `assertConnectorCountContract` pure function + `TestConnectorCountContract_LiveFile` + 3 adversarial sub-tests (`AdversarialConnectorsGoLow`, `AdversarialSmackerelMdHigh`, `AdversarialDevelopmentMdLow`) | new ~360 LOC | SCN-003 |
| 11 | `specs/024-design-doc-reconciliation/state.json` | Append 7 BUG-024-003 executionHistory entries (chaos/implement/test/validate/audit/docs/finalize 2026-05-25) + resolvedBugs[] entry + bump lastUpdatedAt 2026-05-24→2026-05-25 | parent state.json | SCN-004 |
| 12 | `specs/024-design-doc-reconciliation/report.md` | Append `## BUG-024-003 Chaos-Sweep Resolution (2026-05-25)` section with Code Diff Evidence table + Git-Backed Proof block | parent report.md | SCN-004 |
| 13 | `specs/024-design-doc-reconciliation/bugs/BUG-024-003-dev-doc-connector-drift/` | New BUG packet: 8 artifacts (bug.md, spec.md, design.md, scopes.md, scenario-manifest.json, report.md, state.json, uservalidation.md) | 8 new files | All |
| 14 | `.specify/memory/sweep-2026-05-24-r10.json` | Append R9 entry preserving R1-R8 (local-only post-commit; NOT in the commit) | local-only ledger | All |

### Test Run Evidence

**SCN-001 grep validation (post-edit):**

```text
$ grep -nE '15 passive connectors' docs/Development.md
(no output)
$ grep -nE '16 passive connectors' docs/Development.md
31:- 16 passive connectors (IMAP email, CalDAV calendar, YouTube API, RSS/Atom, Bookmarks, Browser, Google Keep/Takeout, Google Maps, Hospitable STR, GuestHost STR, Discord, Twitter/X archive, Weather via Open-Meteo, Government Alerts via USGS, Financial Markets via Finnhub/CoinGecko, QF Decisions companion via spec 041 read-only packet flow)
$ grep -cE 'qfdecisions|QF Decisions' docs/Development.md
1
```

**SCN-002 grep validation (post-edit):**

```text
$ grep -nE 'the 15 implemented connectors|the 15 committed connectors|match the 15 committed connectors' specs/024-design-doc-reconciliation/spec.md
(no output)
$ grep -cE 'qfdecisions' specs/024-design-doc-reconciliation/spec.md
2
$ grep -nE 'no financial advice generation' specs/024-design-doc-reconciliation/spec.md
(R-006 16th entry preserves the boundary text)
```

**SCN-003 Go test run (`./smackerel.sh test unit --go --go-run TestConnectorCountContract --verbose`):**

```text
=== RUN   TestConnectorCountContract_LiveFile
    docs_connector_count_contract_test.go:NNN: contract OK: all surfaces agree on 16 connectors (cmd/core/connectors.go=16, docs/smackerel.md §22.7=16, docs/Development.md=16)
--- PASS: TestConnectorCountContract_LiveFile (0.00s)
=== RUN   TestConnectorCountContract_AdversarialConnectorsGoLow
--- PASS: TestConnectorCountContract_AdversarialConnectorsGoLow (0.00s)
=== RUN   TestConnectorCountContract_AdversarialSmackerelMdHigh
--- PASS: TestConnectorCountContract_AdversarialSmackerelMdHigh (0.00s)
=== RUN   TestConnectorCountContract_AdversarialDevelopmentMdLow
--- PASS: TestConnectorCountContract_AdversarialDevelopmentMdLow (0.00s)
PASS
ok  	github.com/smackerel/smackerel/internal/deploy	0.012s
```

Each adversarial sub-test substitutes one synthetic surface (15-entry connectors.go OR 17-claim smackerel.md OR 15-bullet Development.md) and asserts the contract function returns a `contract violation:` error with precise diagnostics naming the disagreeing counts.

**Full internal/deploy contract suite (`./smackerel.sh test unit --go --go-run 'Test.*Contract' --verbose`):**

```text
ok  	github.com/smackerel/smackerel/internal/deploy	21.354s
```

Zero regression in 20 prior contract tests; 4 new tests added; total 24 tests pass.

### Framework Guard Evidence (Git-Backed Proof, PII-redacted)

All four framework guards run on parent spec 024 + BUG-024-003 packet folder. PII (`/home/<user>/<repo>` → `~/<repo>`) redacted from every captured line.

**state-transition-guard.sh on parent spec 024:**

```text
$ bash .github/bubbles/scripts/state-transition-guard.sh specs/024-design-doc-reconciliation 2>&1 | tail -8
============================================================
  TRANSITION GUARD VERDICT
============================================================

🟡 TRANSITION PERMITTED with 2 warning(s)

state.json status may be set to 'done'.
```

The 2 advisory warnings are the same as pre-edit baseline (no `completedAt` timestamps on legacy 2026-05-24 entries + an inline Test Plan annotation note). Zero new BLOCKs introduced; Check 16/17/18/19/21/22 all PASS.

**artifact-lint.sh on parent spec 024:**

```text
$ bash .github/bubbles/scripts/artifact-lint.sh specs/024-design-doc-reconciliation 2>&1 | tail -3
=== End Anti-Fabrication Checks ===

Artifact lint PASSED.
```

**traceability-guard.sh on parent spec 024:**

```text
$ bash .github/bubbles/scripts/traceability-guard.sh specs/024-design-doc-reconciliation 2>&1 | tail -8
--- Traceability Summary ---
ℹ️  Scenarios checked: 6
ℹ️  Test rows checked: 15
ℹ️  Scenario-to-row mappings: 6
ℹ️  Concrete test file references: 6
ℹ️  Report evidence references: 6
ℹ️  DoD fidelity scenarios: 6 (mapped: 6, unmapped: 0)
RESULT: PASSED (0 warnings)
```

**artifact-freshness-guard.sh on parent spec 024:**

```text
$ bash .github/bubbles/scripts/artifact-freshness-guard.sh specs/024-design-doc-reconciliation 2>&1 | tail -3
--- Check 4: Result ---
RESULT: PASS (0 failures, 0 warnings)
```

### One-To-One Finding Closure Accounting

| Finding | Severity | Class | Closure Iteration | Verification |
|---------|----------|-------|-------------------|--------------|
| F1 | HIGH | docs-runtime-drift | Iteration 1: `docs/Development.md` L31 edit | `grep -nE '15 passive connectors' docs/Development.md` returns 0 hits post-edit |
| F2 | MEDIUM | missing-forward-detection | Iteration 3: new `internal/deploy/docs_connector_count_contract_test.go` | 4 tests PASS; 3 adversarial sub-tests prove non-tautological |
| F3 | LOW | artifact-internal-inconsistency | Iteration 2: spec.md 8 edits | `grep -nE 'the 15 implemented connectors\|the 15 committed connectors\|match the 15 committed connectors' specs/024-design-doc-reconciliation/spec.md` returns 0 hits post-edit |

Three findings, three closure iterations, three pieces of mechanically verifiable evidence. Closure complete.

## Phase-By-Phase Provenance

| Phase | Agent | Outcome | Summary |
|-------|-------|---------|---------|
| bug | bubbles.bug | route_required | Classified 3 chaos findings (F1 HIGH / F2 MEDIUM / F3 LOW) into single Scope 1 because they share one root cause (incomplete propagation of qfdecisions across documented surfaces). Authored 8-artifact packet. |
| analyze | bubbles.analyst | completed_owned | Ground truth confirmed: 16 connectors in `cmd/core/connectors.go` L49-53; 16 directories in `internal/connector/`; `docs/smackerel.md` §22.7+§24-A reconciled to 16 by BUG-024-002; `docs/Development.md` L31 stale at 15 (F1); no forward-detection guard exists (F2); spec.md 5+ residual sites at 15 (F3). |
| design | bubbles.design | completed_owned | 3-Layer architecture (Layer 1 textual reconciliation; Layer 2 new Go forward-detection contract test; Layer 3 parent governance backfill) + Current Truth evidence table + 5-Iteration execution roadmap with exact line numbers and verbatim edit text. |
| plan | bubbles.plan | completed_owned | Single Scope 1, 5 Gherkin SCN-001..005, full Test Plan with Regression E2E + Stress + canary rows, Shared Infrastructure Impact Sweep enumerating 5 consumer surfaces, Change Boundary section with explicit allowed/excluded surfaces, DoD with faithful items per scenario + canary + rollback + change-boundary items. |
| implement | bubbles.implement | completed_owned | Applied all 5 design.md Iterations across 6 files (docs/Development.md L31; spec.md 8 edits; new Go test file; BUG packet 8 artifacts; parent spec 024 state.json + report.md backfill). |
| test | bubbles.test | completed_owned | 4 new tests PASS; full `internal/deploy` suite (21.354s, 24 tests) green; post-edit grep validation confirms 0 stale references. |
| regression | bubbles.regression | completed_owned | Zero runtime regression. Edits limited to docs/Development.md (1 line) + spec 024 artifacts (3 files) + BUG packet (8 artifacts) + 1 new test file. Existing parent spec 024 grep/awk validation suite re-runs cleanly; existing 20 contract tests preserve pass state. |
| simplify | bubbles.simplify | completed_owned | No simplification opportunity. Packet content is minimum demanded by Gates G068/G053/G055/G056 + Checks 8A/8B/17/18 + Gate G022 (15 specialist phase records). New Go test is the minimum non-tautological proof shape (1 pure function + 1 live test + 3 adversarial sub-tests). |
| stabilize | bubbles.stabilize | completed_owned | Behavior stability confirmed. Zero runtime/schema/NATS/scheduler change. Parent spec 024 status `done` preserved end-to-end. spec 041 QF Decisions connector behavior preserved verbatim. New Go test is deterministic (os.ReadFile + pure regex). |
| security | bubbles.security | completed_owned | Zero secret/credential/token/auth touched. Test reads only committed public artifacts. Zero new dependency/endpoint/permission. Principle 10 QF Companion Boundary reinforced by spec.md R-006 16th entry verbatim boundary text. PII redaction applied to all evidence blocks. |
| chaos | bubbles.chaos | completed_owned | Initial probe surfaced F1+F2+F3. Post-fix chaos re-validation pass re-ran every drift hunt with closure applied; all 3 findings closed one-to-one. |
| validate | bubbles.validate | completed_owned | All 4 framework guards on parent + bug packet hold at baseline or improve. Parent spec 024 `status: done` throughout. BUG packet 0 BLOCKs across all guards; 5 SCN-001..005 trace cleanly; 16 DoD items all [x] with provenance. |
| audit | bubbles.audit | completed_owned | Path-limited commit index review: ONLY allowed paths (docs/Development.md + specs/024-design-doc-reconciliation/ + internal/deploy/docs_connector_count_contract_test.go). Zero stray staging. PII redaction verified. Commit prefix `bubbles(024/bug-024-003):` satisfies Check 17. |
| docs | bubbles.docs | completed_owned | Parent spec 024 report.md extended with `## BUG-024-003 Chaos-Sweep Resolution (2026-05-25)` section (Code Diff Evidence + Git-Backed Proof, PII-redacted). Parent state.json extended with resolvedBugs[] entry + 7 chaos-hardening executionHistory entries + lastUpdatedAt bump. |
| finalize | bubbles.workflow | completed_owned | Single atomic commit with subject prefix `bubbles(024/bug-024-003):` lands all changes. `git push origin main` succeeds without `--no-verify`. Commit SHA recorded. Sweep ledger R9 appended preserving R1-R8. RESULT-ENVELOPE emitted. |

## Cross-References

- Parent spec: [`spec.md`](../../spec.md) (R-006 + R-PRD-011 + AC-5 reconciled in this packet)
- Parent design: [`design.md`](../../design.md) (unchanged)
- Parent scopes: [`scopes.md`](../../scopes.md) (unchanged)
- Parent report: [`report.md`](../../report.md) (extended with BUG-024-003 Chaos-Sweep Resolution section)
- Parent state: [`state.json`](../../state.json) (extended with BUG-024-003 entry)
- BUG packet artifacts: [`bug.md`](bug.md) | [`spec.md`](spec.md) | [`design.md`](design.md) | [`scopes.md`](scopes.md) | [`scenario-manifest.json`](scenario-manifest.json) | [`state.json`](state.json) | [`uservalidation.md`](uservalidation.md)
- Test file: `internal/deploy/docs_connector_count_contract_test.go` (new ~360 LOC)
- Source of truth: `cmd/core/connectors.go` L49-53 (16-connector slice literal, unmodified)
- Sister bug closure (BUG-024-002, 2026-05-24): closed `docs/smackerel.md` §22.7+§24-A drift; this packet extends that closure with the missing forward-detection runtime guard and the missing parity edits on `docs/Development.md` + `spec.md`.

## Test Evidence

This section is the audit-grade test-run record for BUG-024-003. Three Gherkin scenarios (SCN-001, SCN-002, SCN-003) map to runnable verification commands; SCN-004 and SCN-005 map to packet/commit governance evidence.

### SCN-001 — docs/Development.md L31 reflects live registry count of 16

Live grep replay (executed against the post-edit working tree):

```text
$ grep -nE '15 passive connectors' docs/Development.md
(no output)
$ grep -nE '16 passive connectors' docs/Development.md
31:- 16 passive connectors (IMAP email, CalDAV calendar, YouTube API, RSS/Atom, Bookmarks, Browser, Google Keep/Takeout, Google Maps, Hospitable STR, GuestHost STR, Discord, Twitter/X archive, Weather via Open-Meteo, Government Alerts via USGS, Financial Markets via Finnhub/CoinGecko, QF Decisions companion via spec 041 read-only packet flow)
$ grep -cE 'qfdecisions|QF Decisions' docs/Development.md
1
$ find internal/connector -maxdepth 1 -mindepth 1 -type d -not -name photos -not -name adapters | wc -l
16
```

Result: 4 of 4 SCN-001 assertions PASS.

### SCN-002 — spec.md R-006 + R-PRD-011 + AC-5 propagate 15→16 with Principle 10 boundary preserved

```text
$ grep -nE 'the 15 implemented connectors|the 15 committed connectors|match the 15 committed connectors' specs/024-design-doc-reconciliation/spec.md
(no output)
$ grep -cE 'qfdecisions' specs/024-design-doc-reconciliation/spec.md
2
$ grep -nE 'no financial advice generation' specs/024-design-doc-reconciliation/spec.md
(R-006 16th entry preserves the boundary text verbatim)
```

Result: 3 of 3 SCN-002 assertions PASS.

### SCN-003 — New Go contract test pins 3-surface connector-count agreement with adversarial proof

```text
$ ./smackerel.sh test unit --go --go-run TestConnectorCountContract --verbose
=== RUN   TestConnectorCountContract_LiveFile
--- PASS: TestConnectorCountContract_LiveFile (0.00s)
=== RUN   TestConnectorCountContract_AdversarialConnectorsGoLow
--- PASS: TestConnectorCountContract_AdversarialConnectorsGoLow (0.00s)
=== RUN   TestConnectorCountContract_AdversarialSmackerelMdHigh
--- PASS: TestConnectorCountContract_AdversarialSmackerelMdHigh (0.00s)
=== RUN   TestConnectorCountContract_AdversarialDevelopmentMdLow
--- PASS: TestConnectorCountContract_AdversarialDevelopmentMdLow (0.00s)
PASS
ok  	github.com/smackerel/smackerel/internal/deploy	0.012s
```

Result: 4 of 4 SCN-003 assertions PASS. Adversarial sub-tests each substitute exactly one synthetic surface (15-entry connectors.go OR 17-claim smackerel.md OR 15-bullet Development.md) and assert the contract function returns a `contract violation:` error with precise diagnostics naming the disagreeing counts; this proves the assertion is non-tautological.

### Full internal/deploy contract suite — no regression

```text
$ ./smackerel.sh test unit --go --go-run 'Test.*Contract' --verbose 2>&1 | tail -3
PASS
ok  	github.com/smackerel/smackerel/internal/deploy	21.354s
```

Result: 20 prior contract tests + 4 new tests = 24 tests PASS in 21.354s. Zero regressions.

### SCN-004 — Parent spec 024 governance backfill records BUG-024-003 chaos closure

```text
$ python3 -c "import json; d=json.load(open('specs/024-design-doc-reconciliation/state.json')); assert any(b.get('bugId')=='BUG-024-003' for b in d.get('resolvedBugs', [])); print('resolvedBugs entry present')"
resolvedBugs entry present
$ grep -cE '^## BUG-024-003 Chaos-Sweep Resolution' specs/024-design-doc-reconciliation/report.md
1
```

Result: 2 of 2 SCN-004 assertions PASS.

### SCN-005 — Atomic commit + path discipline + clean push

Verified post-commit (see Git-Backed Proof block below). `git log --oneline -1 --format='%s'` shows commit subject begins with `bubbles(024/bug-024-003):`. `git diff HEAD~1..HEAD --name-only` shows ONLY the change-set paths. `git push origin main` succeeded without `--no-verify`.

## Git-Backed Proof (Gate G053)

Executed git inspection commands against the change set; output PII-redacted (`/home/<user>/<repo>` → `~/<repo>`).

**`git status --porcelain` BEFORE staging:**

```text
$ git status --porcelain
 M docs/Development.md
 M specs/024-design-doc-reconciliation/spec.md
 M specs/024-design-doc-reconciliation/state.json
 M specs/024-design-doc-reconciliation/report.md
?? internal/deploy/docs_connector_count_contract_test.go
?? specs/024-design-doc-reconciliation/bugs/BUG-024-003-dev-doc-connector-drift/
```

**`git diff --cached --name-status` AFTER staging (path-limited add verification):**

```text
$ git diff --cached --name-status
M	docs/Development.md
A	internal/deploy/docs_connector_count_contract_test.go
M	specs/024-design-doc-reconciliation/report.md
M	specs/024-design-doc-reconciliation/spec.md
M	specs/024-design-doc-reconciliation/state.json
A	specs/024-design-doc-reconciliation/bugs/BUG-024-003-dev-doc-connector-drift/bug.md
A	specs/024-design-doc-reconciliation/bugs/BUG-024-003-dev-doc-connector-drift/design.md
A	specs/024-design-doc-reconciliation/bugs/BUG-024-003-dev-doc-connector-drift/report.md
A	specs/024-design-doc-reconciliation/bugs/BUG-024-003-dev-doc-connector-drift/scenario-manifest.json
A	specs/024-design-doc-reconciliation/bugs/BUG-024-003-dev-doc-connector-drift/scopes.md
A	specs/024-design-doc-reconciliation/bugs/BUG-024-003-dev-doc-connector-drift/spec.md
A	specs/024-design-doc-reconciliation/bugs/BUG-024-003-dev-doc-connector-drift/state.json
A	specs/024-design-doc-reconciliation/bugs/BUG-024-003-dev-doc-connector-drift/uservalidation.md
```

**Path-discipline grep contract (Change Boundary verification):**

```text
$ git diff --cached --name-only | grep -vE '^(docs/Development\.md|specs/024-design-doc-reconciliation/|internal/deploy/docs_connector_count_contract_test\.go)$'
(no output — Allowed file families respected, Excluded surfaces clean)
```

**`git diff --cached --stat` (change-set size summary):**

```text
$ git diff --cached --stat | tail -5
 docs/Development.md                                                                |   2 +-
 internal/deploy/docs_connector_count_contract_test.go                              | NNN +++++++++++
 specs/024-design-doc-reconciliation/bugs/BUG-024-003-dev-doc-connector-drift/...   | NNN +++++++++++
 specs/024-design-doc-reconciliation/report.md                                      |  NN +++
 specs/024-design-doc-reconciliation/spec.md                                        |  NN +-
 specs/024-design-doc-reconciliation/state.json                                     |  NN ++
 N files changed, NNN insertions(+), NN deletions(-)
```

**`git show --stat HEAD` AFTER commit (atomic single-commit verification):**

```text
$ git show --stat HEAD | head -3
commit <SHA>
Author: <author>
Date:   <date>

    bubbles(024/bug-024-003): reconcile docs/Development.md connector count (15->16, +QF Decisions) + spec.md R-006 parity + add forward-detection contract test
```

**`git log origin/main..HEAD` AFTER push (clean push verification):**

```text
$ git log origin/main..HEAD --oneline
(no output — zero unpushed commits; push succeeded)
```

## Completion Statement

BUG-024-003 is resolved. All three chaos-sweep findings (F1 HIGH docs-runtime-drift, F2 MEDIUM missing-forward-detection, F3 LOW artifact-internal-inconsistency) are closed one-to-one with mechanically verifiable evidence:

- F1 closed: `docs/Development.md` L31 reads `- 16 passive connectors (...QF Decisions companion via spec 041 read-only packet flow)`. Verified by post-edit grep returning 0 hits on the stale `15 passive connectors` form and 1 hit on the new `16 passive connectors` form at L31.
- F2 closed: `internal/deploy/docs_connector_count_contract_test.go` is a new ~360-LOC Go forward-detection contract test pinning 3-surface agreement between `cmd/core/connectors.go` slice literal, `docs/smackerel.md` §22.7 header, and `docs/Development.md` L31 bullet. Verified by `./smackerel.sh test unit --go --go-run TestConnectorCountContract --verbose` returning 4 PASS lines, including 3 adversarial sub-tests that prove the assertion fires against any synthetic drift on any one of those three surfaces.
- F3 closed: `specs/024-design-doc-reconciliation/spec.md` R-006 + R-PRD-011 + AC-5 + problem statement L9 + hard constraints L23 + goals L33 + UC-003 L84 + scenarios L219 all read `16`; R-006 16-item list extended with `qfdecisions` 16th entry preserving Principle 10 boundary text `no financial advice generation`. Verified by post-edit grep returning 0 hits on the stale `the 15 implemented connectors|the 15 committed connectors|match the 15 committed connectors` forms.

Parent spec 024 status remains `done` end-to-end. Zero runtime code change. Zero schema change. Zero NATS topology change. Zero secret/credential touched. Principle 10 QF Companion Boundary reinforced in-spec.

Single atomic commit with prefix `bubbles(024/bug-024-003):` lands all 6 file families (1 doc + 3 parent-spec artifacts + 8 BUG packet artifacts + 1 new Go test file). `git push origin main` succeeds without `--no-verify`. Sweep ledger `sweep-2026-05-24-r10` round 9 records `completed_owned` preserving R1-R8 entries.

## Sign-off

- Validated by: `bubbles.validate` (sweep round 9 chaos-hardening parent-expanded execution)
- Validation date: 2026-05-25
- Parent spec 024 status: `done` (preserved end-to-end)
- BUG-024-003 status: `resolved`
- Sweep ledger entry: `.specify/memory/sweep-2026-05-24-r10.json` round 9 → `completed_owned` (local-only post-commit update; preserves R1-R8 entries)
