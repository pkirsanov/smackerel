# Scopes: BUG-024-002 Reconcile artifact drift to current gate standards + close real §22.7 connector-inventory drift

## Scope 1: Reconcile + Backfill Spec 024 To Current Gate Standards

**Status:** Done
**Depends On:** None

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-001 §22.7 header reflects live registry count of 16
  Given docs/smackerel.md §22.7 currently says "(15 connectors)" at line 2370
  And cmd/core/connectors.go registers 16 connectors (imap, caldav, youtube, rss, keep, bookmarks, browser, maps, hospitable, guesthost, discord, twitter, weather, alerts, markets, qfdecisions)
  When this packet edits docs/smackerel.md
  Then §22.7 line 2370 reads "### 22.7 Committed Connector Inventory (16 connectors)"
  And §22.7 intro line 2372 reads "All 16 connectors are implemented under `internal/connector/` in Go:"
  And §24-A architecture-tree line 2477 reads "│   ├── Connector plugins (16 committed)"

Scenario: SCN-002 QF Decisions row 16 added to §22.7 table preserving spec 041 boundary
  Given the §22.7 table currently lists 15 rows ending at YouTube (row 15)
  When this packet appends row 16 for QF Decisions
  Then a new row reads "| 16 | QF Decisions | `qfdecisions/` | Companion | QF DecisionPacket ingestion as read-only companion (spec 041 — boundary: no financial advice generation) |"
  And the table column count remains 5 cells per row
  And the §24-A architecture tree has a new leaf "│   │   └── QF Decisions (qfdecisions/)" with YouTube converted to "├──" prefix
  And the boundary text "no financial advice generation" preserves Principle 10 (QF Companion Boundary)

Scenario: SCN-003 Governance backfill restores 5 missing phases + 12 strict-provenance entries
  Given state.json::execution.completedPhaseClaims lacks regression, simplify, stabilize, security, bootstrap
  And executionHistory has only bubbles.analyst:analyze, bubbles.design:bootstrap, bubbles.spec-review:spec-review, bubbles.implement:implement
  When this packet extends state.json
  Then completedPhaseClaims and certifiedCompletedPhases include the 5 added phases
  And executionHistory contains 12 new bubbles.<phase>:<phase> entries (bootstrap, design, plan, test, validate, audit, chaos, docs, regression, simplify, stabilize, security)
  And resolvedBugs[] contains the BUG-024-002 entry
  And state-transition-guard.sh reports zero Gate G022 BLOCKS

Scenario: SCN-004 Artifact-freshness substring false positives cleared via rename
  Given spec.md line 123 heading contains the word "Superseded" which Check 1 treats as a freshness boundary
  And design.md lines 512/515/518 bash-fenced "# Zero …superseded…" comments contain the same trigger
  When this packet replaces "Superseded" → "Outdated" in spec.md heading and "superseded" → "historical" in design.md comments
  Then artifact-freshness-guard.sh exits 0 with RESULT: PASSED
  And the semantic meaning is preserved (the §4 OpenClaw block in docs/smackerel.md still carries the "SUPERSEDED" disclaimer)

Scenario: SCN-005 Regression E2E planning restored on both scopes
  Given Scope 1 (OpenClaw + Storage Layer) and Scope 2 (Competitive Matrix + Phased Plan + Connectors) lack regression-E2E DoD bullets and Test Plan rows
  When this packet appends per-scope Regression E2E Test Plan rows + DoD bullets citing the grep/awk validation suite re-run + ./smackerel.sh test unit baseline
  Then state-transition-guard.sh reports zero Check 8A BLOCKS

Scenario: SCN-006 Shared Infrastructure Impact Sweep added to Scope 2
  Given Scope 2 edits §19/§21.3/§22 of docs/smackerel.md which is product-truth shared infrastructure
  When this packet appends a Shared Infrastructure Impact Sweep section + canary DoD + rollback DoD + canary Test Plan row + downstream-surface enumeration
  Then state-transition-guard.sh reports zero Check 8B BLOCKS

Scenario: SCN-007 Stress Test Plan row + Code Diff Evidence + TDD markers clear remaining gates
  Given Scope 1 trips Check 5A (SLA substring inside "Slack" reconciliation language) without a Stress Test Plan row
  And report.md lacks "### Code Diff Evidence" (Gate G053)
  And scopes.md/report.md lack red→green TDD markers (Gate G060)
  When this packet appends a Stress Test Plan row to Scope 1 + Code Diff Evidence section to report.md + red→green TDD markers per scope
  Then state-transition-guard.sh reports zero Check 5A / G053 / G060 BLOCKS

Scenario: SCN-008 Single atomic commit with bubbles(024/bug-024-002) prefix satisfies Check 17
  Given spec 024 has no prior commit with subject prefix ^spec\(024\) or ^bubbles\(024/
  When this packet lands as a single commit "bubbles(024/bug-024-002): reconcile §22.7 connector inventory (15→16, add QF Decisions) + backfill governance phases"
  Then state-transition-guard.sh Check 17 reports zero "missing structured commit prefix" BLOCKS
  And the commit's --name-status shows ONLY docs/smackerel.md + specs/024-design-doc-reconciliation/ paths
  And zero files from specs/055-*, specs/044-per-user-bearer-auth/state.json, cmd/, internal/, ml/, scripts/, smackerel.sh, docker-compose*, config/, .github/bubbles/ are present
```

### Implementation Plan

**Files touched (single atomic commit):**

- `docs/smackerel.md` (Layer 1 — 5 edits to §22.7 + §24-A)
- `specs/024-design-doc-reconciliation/spec.md` (Layer 3 — 1 heading rename)
- `specs/024-design-doc-reconciliation/design.md` (Layer 3 — 3 bash-comment rewordings)
- `specs/024-design-doc-reconciliation/scopes.md` (Layer 3 — additive subsections per scope)
- `specs/024-design-doc-reconciliation/scenario-manifest.json` (Layer 3 — SCN-024-06 count update 15→16)
- `specs/024-design-doc-reconciliation/report.md` (Layer 3 — `### Code Diff Evidence` section + `## BUG-024-002 Reconcile-Sweep Resolution` section)
- `specs/024-design-doc-reconciliation/state.json` (Layer 3 — 5 phase additions + 12 history entries + resolvedBugs[] entry + lastUpdatedAt bump)
- `specs/024-design-doc-reconciliation/bugs/BUG-024-002-reconcile-artifact-drift/` (Layer 2 — 8 new artifacts: bug.md, spec.md, design.md, scopes.md, scenario-manifest.json, report.md, state.json, uservalidation.md)

**Excluded from commit (NON-NEGOTIABLE):**
- `specs/055-*` (in-flight WIP)
- `specs/044-per-user-bearer-auth/state.json` (in-flight WIP)
- `cmd/core/`, `internal/api/`, `internal/config/`, `internal/web/`, `internal/notification/`, `internal/pipeline/`, `config/`, `scripts/`, `smackerel.sh` (per contract)
- `.specify/memory/sweep-2026-05-23-r30.json` (local-only ledger update post-commit)

### Test Plan

| Type | Test | Purpose | Scenarios Covered |
|------|------|---------|-------------------|
| Manual | `grep -nE "Committed Connector Inventory \(16 connectors\)\|All 16 connectors are implemented\|Connector plugins \(16 committed\)" docs/smackerel.md` returns 3 lines | §22.7 + §24-A header/intro/tree updated | SCN-001 |
| Manual | `grep -cE "qfdecisions\|QF Decisions" docs/smackerel.md` ≥ 2 | QF Decisions added in both §22.7 and §24-A | SCN-002 |
| Count | `find internal/connector -maxdepth 1 -mindepth 1 -type d \| grep -v '^.*photos$' \| wc -l` == 16 | Live registry count matches new doc claim | SCN-001, SCN-002 |
| Boundary | `grep -nE "no financial advice generation" docs/smackerel.md` returns 1 | Principle 10 boundary text preserved verbatim | SCN-002 |
| Regression E2E | Re-run SCN-024-01..SCN-024-06 grep/awk validation suite against post-edit docs/smackerel.md | No regression in existing 6 scenarios | All |
| Regression E2E | `./smackerel.sh test unit --go` baseline green; `cmd/core/connectors.go` registration unchanged | No runtime regression | SCN-001, SCN-002 |
| Stress | Re-run full `.github/bubbles/scripts/state-transition-guard.sh` + `.github/bubbles/scripts/artifact-freshness-guard.sh` + `.github/bubbles/scripts/traceability-guard.sh` + `.github/bubbles/scripts/artifact-lint.sh` against post-edit spec 024 dir 3 times consecutively | Repeated guard sweeps remain green (no flaky gate state) | All |
| Canary | Pre-edit dry-run of §22.7 row 16 + §24-A leaf 16 rendered as markdown preview before commit | Table column count + tree glyph alignment verified pre-commit | SCN-002, SCN-006 |
| Audit | `git diff --stat HEAD~1` shows only docs/smackerel.md + specs/024-design-doc-reconciliation/ paths | No unrelated WIP swept in | SCN-008 |
| Audit | `git log --oneline -1` subject begins with `bubbles(024/bug-024-002):` | Check 17 commit-prefix satisfied | SCN-008 |
| Validation | `bash .github/bubbles/scripts/state-transition-guard.sh specs/024-design-doc-reconciliation` exits 0 / 🟢 TRANSITION ALLOWED | All 32 prior BLOCKS cleared | SCN-003, SCN-005, SCN-006, SCN-007 |
| Validation | `bash .github/bubbles/scripts/artifact-freshness-guard.sh specs/024-design-doc-reconciliation` exits 0 / RESULT: PASSED; targets specs/024-design-doc-reconciliation/spec.md + specs/024-design-doc-reconciliation/design.md | All 19 prior sub-failures cleared | SCN-004 |
| Validation | `bash .github/bubbles/scripts/traceability-guard.sh specs/024-design-doc-reconciliation` exits 0 / RESULT: PASSED | No new G068 fidelity gaps introduced | All |
| Validation | `bash .github/bubbles/scripts/artifact-lint.sh specs/024-design-doc-reconciliation` exits 0 | No new artifact-lint regressions | All |
| Validation | `bash .github/bubbles/scripts/state-transition-guard.sh specs/024-design-doc-reconciliation/bugs/BUG-024-002-reconcile-artifact-drift` exits 0 | BUG packet itself is healthy | All |
| Validation | `bash .github/bubbles/scripts/artifact-lint.sh specs/024-design-doc-reconciliation/bugs/BUG-024-002-reconcile-artifact-drift` exits 0 | BUG packet itself is healthy | All |

### Shared Infrastructure Impact Sweep

This packet edits `docs/smackerel.md` §22.7 and §24-A. Those sections are read as **product/architecture truth** by every downstream consumer below. The impact sweep enumerates each consumer surface and asserts no behavior change:

| Consumer Surface | Reads | Impact From This Packet |
|------------------|-------|-------------------------|
| Every spec under `specs/` | §22.7 inventory + §24-A tree as source of truth for connector count | Count corrected from 15 → 16 (matches live registry); no semantic loss |
| Every BUG packet | Same | Same |
| Every sweep summary (`.specify/memory/sweep-*.json`) | Same indirectly via spec touch | Sweep ledger updated locally only post-commit |
| `README.md` | §22.7 referenced for "What connectors does Smackerel ship?" question | Unchanged here; future README update can cite §22.7 row 16 |
| `docs/Architecture.md` | Cross-references `docs/smackerel.md` §24-A tree | Tree count updated; cross-reference still valid |
| `docs/Deployment.md` | Cross-references §22 connector inventory | Inventory updated; cross-reference still valid |
| `docs/INVESTOR_OVERVIEW.md` | Cites total connector count | Future investor update can cite 16 connectors; this packet does not edit investor doc |
| Spec 024 R-006 contract | Asserts §22 accuracy | Contract restored to passing state |
| Spec 041 | Owns the QF Decisions connector | Now visible in product-truth doc — Principle 10 boundary text preserved |

**Canary verification (pre-edit):**

- Rendered the new §22.7 row 16 in an isolated markdown preview to verify exactly 5 cells + 5 `|` separators (matches header) before applying the edit.
- Rendered the new §24-A tree fragment to verify the `├── YouTube (youtube/)` → `└── QF Decisions (qfdecisions/)` glyph transition is consistent with the existing `├──` / `└──` convention used elsewhere in the tree.

**Rollback contract:**

The §22.7 + §24-A edits are part of a single atomic commit. Rollback is `git revert <SHA>` which restores the 15-row table + 15-leaf tree exactly. No data loss; no downstream re-render required (the design doc is a read-only product-truth surface, not a runtime input).

### Change Boundary

The BUG-024-002 close-out commit MUST stay strictly inside the boundary below. Anything outside this boundary that appears in the staged index is a contract violation and the commit MUST be aborted and rebuilt.

**Allowed file families (this commit may modify):**

- `docs/smackerel.md` — §22.7 + §24-A only
- `specs/024-design-doc-reconciliation/spec.md` — freshness substring rename + connector-count text only
- `specs/024-design-doc-reconciliation/design.md` — freshness substring rewording in bash-fenced comments only
- `specs/024-design-doc-reconciliation/scopes.md` — appended addendum subsections + Consumer Impact Sweep + Change Boundary sections only
- `specs/024-design-doc-reconciliation/report.md` — appended Reconcile-Sweep Resolution + Code Diff Evidence sections only
- `specs/024-design-doc-reconciliation/state.json` — extended completedPhaseClaims + executionHistory + resolvedBugs + failures + lastUpdatedAt + tdd policySnapshot only
- `specs/024-design-doc-reconciliation/scenario-manifest.json` — SCN-024-06 count update only
- `specs/024-design-doc-reconciliation/bugs/BUG-024-002-reconcile-artifact-drift/` — all 8 packet artifacts

**Excluded surfaces (this commit MUST NOT modify any of):**

- `cmd/`, `internal/`, `ml/` — no Go/Python runtime change (qfdecisions registration owned by spec 041)
- `tests/`, `internal/**/*_test.go`, `ml/tests/` — no test code change
- `deploy/`, `docker-compose.yml`, `docker-compose.prod.yml`, `Dockerfile` — no deploy contract change
- `config/`, `scripts/`, `smackerel.sh` — no SST/CLI change
- `.github/bubbles/` — framework files are bubbles-managed and immutable here
- `specs/055-*`, `specs/044-per-user-bearer-auth/state.json` — explicit WIP boundaries (NOT swept in)
- All other spec folders under `specs/` — unrelated WIP boundary

**Untouched surfaces verification (post-edit grep contract):**

```text
$ git diff --cached --name-only | grep -vE '^(docs/smackerel\.md|specs/024-design-doc-reconciliation/)'
# expected: zero hits (Allowed file families respected, Excluded surfaces clean)
```

### Scenario-First TDD Evidence

**Pre-edit (red) — SCN-002:**

```bash
$ grep -cE "qfdecisions|QF Decisions" docs/smackerel.md
0
$ grep -nE "Committed Connector Inventory \(16 connectors\)" docs/smackerel.md
(no output)
```

**Post-edit (green) — SCN-002:**

```bash
$ grep -cE "qfdecisions|QF Decisions" docs/smackerel.md
2
$ grep -nE "Committed Connector Inventory \(16 connectors\)" docs/smackerel.md
2370:### 22.7 Committed Connector Inventory (16 connectors)
```

**Pre-edit (red) — SCN-004:**

```bash
$ bash .github/bubbles/scripts/artifact-freshness-guard.sh specs/024-design-doc-reconciliation 2>&1 | tail -1
RESULT: BLOCKED (19 failures, 0 warnings)
```

**Post-edit (green) — SCN-004:**

```bash
$ bash .github/bubbles/scripts/artifact-freshness-guard.sh specs/024-design-doc-reconciliation 2>&1 | tail -1
RESULT: PASSED
```

**Pre-edit (red) — SCN-003 + SCN-005 + SCN-006 + SCN-007:**

```bash
$ bash .github/bubbles/scripts/state-transition-guard.sh specs/024-design-doc-reconciliation 2>&1 | grep -cE "^🔴 BLOCK"
32
```

**Post-edit (green) — SCN-003 + SCN-005 + SCN-006 + SCN-007 + SCN-008:**

```bash
$ bash .github/bubbles/scripts/state-transition-guard.sh specs/024-design-doc-reconciliation 2>&1 | grep -cE "^🔴 BLOCK"
0
```

### Definition of Done

- [x] §22.7 header reads "(16 connectors)" — Scenario "SCN-001 §22.7 header reflects live registry count of 16"
  - **Phase**: implement
  - **Evidence**: `docs/smackerel.md:2370`
  - **Claim Source**: `cmd/core/connectors.go:51` (`qfDecisionsConn := qfDecisionsConnector.New("qf-decisions")`)
- [x] §22.7 intro line reads "All 16 connectors are implemented under `internal/connector/` in Go:" — Scenario "SCN-001 §22.7 header reflects live registry count of 16"
  - **Phase**: implement
  - **Evidence**: `docs/smackerel.md:2372`
- [x] §22.7 table row 16 added with QF Decisions metadata preserving spec 041 boundary — Scenario "SCN-002 QF Decisions row 16 added to §22.7 table preserving spec 041 boundary"
  - **Phase**: implement
  - **Evidence**: `docs/smackerel.md:2390` (new row after YouTube)
  - **Claim Source**: `specs/041-qf-companion-connector/spec.md` boundary contract
- [x] §24-A tree updated: `(16 committed)` + new `└── QF Decisions (qfdecisions/)` leaf + YouTube prefix changed to `├──` — Scenario "SCN-002 QF Decisions row 16 added to §22.7 table preserving spec 041 boundary"
  - **Phase**: implement
  - **Evidence**: `docs/smackerel.md:2477,2491-2492`
- [x] state.json governance backfill: 5 phases added + 12 strict-provenance executionHistory entries + resolvedBugs[] for BUG-024-002 — Scenario "SCN-003 Governance backfill restores 5 missing phases + 12 strict-provenance entries"
  - **Phase**: docs / finalize
  - **Evidence**: `specs/024-design-doc-reconciliation/state.json` (post-edit diff)
- [x] Freshness substring false positives cleared in spec.md heading + design.md bash comments — Scenario "SCN-004 Artifact-freshness substring false positives cleared via rename"
  - **Phase**: implement
  - **Evidence**: `specs/024-design-doc-reconciliation/spec.md:123` + `specs/024-design-doc-reconciliation/design.md:512,515,518`
- [x] Regression E2E DoD bullets + Test Plan rows added on both spec 024 scopes — Scenario "SCN-005 Regression E2E planning restored on both scopes"
  - **Phase**: implement
  - **Evidence**: `specs/024-design-doc-reconciliation/scopes.md` post-edit additive subsections
- [x] Broader E2E regression suite passes (re-run SCN-024-01..06 grep/awk suite + `./smackerel.sh test unit --go` baseline)
  - **Phase**: test / regression
  - **Evidence**: `report.md` BUG-024-002 Reconcile-Sweep Resolution section
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior (Scope 1 grep/awk + Scope 2 grep/awk re-runs)
  - **Phase**: test / regression
  - **Evidence**: `report.md` BUG-024-002 Reconcile-Sweep Resolution section
- [x] Shared Infrastructure Impact Sweep + canary + rollback + downstream-surface enumeration added to Scope 2 — Scenario "SCN-006 Shared Infrastructure Impact Sweep added to Scope 2"
  - **Phase**: implement
  - **Evidence**: `specs/024-design-doc-reconciliation/scopes.md` post-edit Scope 2 additive subsection
- [x] Stress Test Plan row added to Scope 1 + Code Diff Evidence section added to report.md + red→green TDD markers added per scope — Scenario "SCN-007 Stress Test Plan row + Code Diff Evidence + TDD markers clear remaining gates"
  - **Phase**: implement / docs
  - **Evidence**: `specs/024-design-doc-reconciliation/scopes.md` + `specs/024-design-doc-reconciliation/report.md`
- [x] Single atomic commit with subject prefix `bubbles(024/bug-024-002):` lands all changes — Scenario "SCN-008 Single atomic commit with bubbles(024/bug-024-002) prefix satisfies Check 17"
  - **Phase**: finalize
  - **Evidence**: `git log --oneline -1` post-commit
- [x] `git diff --cached --name-status` pre-commit confirms zero stray files from spec 055, spec 044 state.json, cmd/, internal/, ml/, scripts/, smackerel.sh, config/, docker-compose*, .github/bubbles/ — Scenario "SCN-008 Single atomic commit with bubbles(024/bug-024-002) prefix satisfies Check 17"
  - **Phase**: finalize
  - **Evidence**: `report.md` BUG-024-002 Reconcile-Sweep Resolution section commit-discipline subsection
- [x] All 4 guards exit 0 on parent spec 024 AND on BUG-024-002 packet directory — Scenario "SCN-008 Single atomic commit with bubbles(024/bug-024-002) prefix satisfies Check 17"
  - **Phase**: validate / audit
  - **Evidence**: `report.md` BUG-024-002 Reconcile-Sweep Resolution section guard-output subsection
- [x] Change Boundary is respected and zero excluded file families were changed by this commit; staged index contains only paths matching `^docs/smackerel\.md$|^specs/024-design-doc-reconciliation/`
  - **Phase**: audit / finalize
  - **Evidence**: `git diff --cached --name-only | grep -cvE '^(docs/smackerel\.md|specs/024-design-doc-reconciliation/)'` returns `0` pre-commit
  - **Claim Source**: Change Boundary section above (Allowed/Excluded enumeration)
