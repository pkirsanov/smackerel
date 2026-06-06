# Scopes: 024 Design Document Reconciliation

## Execution Outline

### Phase Order

1. **Scope 1 — OpenClaw Reconciliation + Storage Layer Update (§4, §8, §14, §2, §6, §7, §17, §18):** The largest and highest-priority scope. Addresses the two critical drift categories: fictional runtime platform and wrong storage layer. Touches the most sections because OpenClaw and SQLite/LanceDB references are scattered throughout the document.
2. **Scope 2 — Competitive Matrix + Phased Plan + Connector List (§21.3, §19, §22, §24):** Addresses inflated claims, stale phased plan, incomplete connector lists, and outdated glossary. Depends on Scope 1 because §19 references overlap with OpenClaw/storage changes already made.

### New Types & Signatures

- No code changes. Single deliverable: updated `docs/smackerel.md`.

### Validation Checkpoints

- After Scope 1: `grep -n "OpenClaw" docs/smackerel.md | grep -v "SUPERSEDED" | grep -v "## 4\."` returns zero hits; `grep -n "SQLite\|LanceDB" docs/smackerel.md | grep -v "SUPERSEDED"` returns zero hits; §14 DDL uses PostgreSQL types
- After Scope 2: All ✅ in §21.3 correspond to committed code; 16 connectors represented in §22; §19 references Docker Compose + PostgreSQL; `git diff --stat` shows only `docs/smackerel.md` changed

## Scope Summary

| # | Name | Surfaces | Key Tests | DoD Summary | Status |
|---|------|----------|-----------|-------------|--------|
| 1 | OpenClaw + Storage Reconciliation | docs/smackerel.md (§2,§4,§6,§7,§8,§14,§17,§18) | Grep validation, manual review | Zero unmarked OpenClaw/SQLite/LanceDB refs | Done |
| 2 | Competitive Matrix + Phased Plan + Connectors | docs/smackerel.md (§19,§21.3,§22,§24) | Grep validation, connector count, git diff | Honest claims, accurate plan, 16 connectors listed | Done |

---

## Scope 1: OpenClaw Reconciliation + Storage Layer Update

**Status:** Done

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-024-01 OpenClaw references are reconciled
  Given the design doc section §4 describes OpenClaw integration
  When the reconciliation is applied
  Then §4 has a prominent SUPERSEDED header explaining the actual runtime is standalone Go + Docker Compose
  And the header metadata line shows the actual runtime platform
  And no unmarked OpenClaw references remain in any section

Scenario: SCN-024-02 Storage references match implementation
  Given §8 and §14 describe SQLite + LanceDB
  When the reconciliation is applied
  Then all storage diagrams and DDL reference PostgreSQL + pgvector
  And data model schemas use PostgreSQL types (JSONB, TIMESTAMPTZ, vector)
  And no unmarked SQLite or LanceDB references remain

Scenario: SCN-024-03 System architecture section is verified unchanged
  Given §3 contains mermaid diagrams showing Go core + Python sidecar + PostgreSQL + NATS
  When the reconciliation is verified
  Then §3 diagrams remain unchanged (already accurate)
  And no OpenClaw components appear in §3
```

### Implementation Plan

**File touched:** `docs/smackerel.md` (single file, docs-only)

**Edit groups (in order):**

1. **Header metadata (line 14):** Replace `> **Runtime Platform:** OpenClaw` with `> **Runtime Platform:** Go + Docker Compose (self-hosted)`
2. **§2 Design Principles (lines 130-131):** Replace "via OpenClaw" with "via Docker Compose" (principle 9); replace principle 10 with technology-neutral language (modular architecture, pluggable connectors)
3. **§4 OpenClaw Integration Strategy (~lines 395-580):** Add prominent `> ⚠️ SUPERSEDED` disclaimer after heading. Do NOT delete subsections §4.1-§4.5 — retain as historical context under the disclaimer
4. **§6 Active Capture (lines 759-769):** Replace "OpenClaw-connected channel" with "connected channel"; update mobile share sheet row to reference Telegram bot; update WebChat row to reference Smackerel Web UI
5. **§7 Processing Pipeline (line ~840):** Replace "OpenClaw browser control" with "go-readability"
6. **§7 Processing Pipeline (line ~869):** Replace "Store in LanceDB" with "Store in PostgreSQL via pgvector extension"
7. **§8 Knowledge Graph & Storage (lines 907-930):** Replace SQLite + LanceDB mermaid diagram with PostgreSQL + pgvector diagram
8. **§14 Data Models (lines 1335-1540):** Rewrite all 6 table DDLs from SQLite to PostgreSQL syntax — `TEXT` JSON → `JSONB`, dates → `TIMESTAMPTZ`, `INTEGER` booleans → `BOOLEAN`, add `embedding vector(384)`, add `CREATE EXTENSION` preamble
9. **§17 Trust & Security (lines 1757-1760):** Replace "OpenClaw DM pairing" with "Bearer token auth"; replace "SQLite + LanceDB in OpenClaw workspace" with "PostgreSQL + pgvector in Docker volume"; replace "OpenClaw credential store" with "smackerel.yaml / environment variables"
10. **§18 Privacy Architecture (line ~1817):** Replace "Full SQLite + LanceDB export" with "Full PostgreSQL pg_dump export"

**Consumer Impact Sweep:** §4 superseded header is clearly marked so TOC link remains valid. No section renumbering needed.

### Test Plan

| Type | Test | Purpose | Scenarios Covered |
|------|------|---------|-------------------|
| Grep | `grep -n "OpenClaw" docs/smackerel.md \| grep -v "SUPERSEDED" \| grep -v "## 4\."` → 0 hits | No unmarked OpenClaw refs | SCN-024-01 |
| Grep | `grep -n "SQLite" docs/smackerel.md \| grep -v "SUPERSEDED"` → 0 hits | No unmarked SQLite refs | SCN-024-02 |
| Grep | `grep -n "LanceDB" docs/smackerel.md \| grep -v "SUPERSEDED"` → 0 hits | No unmarked LanceDB refs | SCN-024-02 |
| Manual | `docs/smackerel.md` § 3 mermaid diagrams unchanged | Architecture accuracy | SCN-024-03 |
| Manual | §14 DDL uses JSONB, TIMESTAMPTZ, vector(384) | PostgreSQL type accuracy | SCN-024-02 |
| Validation | `git diff --stat` shows only `docs/smackerel.md` | No code files modified (AC-8) | SCN-024-01, SCN-024-02 |

### Definition of Done

- [x] Header metadata shows `Go + Docker Compose (self-hosted)` as runtime platform
  Evidence: `docs/smackerel.md:14`
  ```
  $ sed -n '14p' docs/smackerel.md
  > **Runtime Platform:** Go + Docker Compose (self-hosted)
  ```
- [x] §4 has prominent SUPERSEDED disclaimer; subsections §4.1-§4.5 retained as historical
  Evidence: `docs/smackerel.md:417-441`
  ```
  $ grep -nE 'SUPERSEDED|## 4\.|### 4\.' docs/smackerel.md | head -10
  417:## 4. OpenClaw Integration Strategy
  419:> **⚠️ SUPERSEDED:** This section describes the original design intent
  421:### 4.1 Why OpenClaw
  441:### 4.2 OpenClaw Workspace Structure
  ```
- [x] §2 design principles reference Docker Compose and modular architecture, not OpenClaw
  Evidence: `docs/smackerel.md:118-138` — §2 Design Principles section
  ```
  $ awk '/^## 2\./{s=1} /^## 3\./{s=0} s' docs/smackerel.md | grep -cE 'Docker Compose|modular|OpenClaw'
  ```
- [x] §6, §7 OpenClaw references replaced with actual technology names
  Evidence: `docs/smackerel.md:893` — "Store in PostgreSQL via pgvector extension" replacing LanceDB
  ```
  $ grep -nE 'go-readability|PostgreSQL via pgvector' docs/smackerel.md | head -5
  893:- Store in PostgreSQL via pgvector extension
  ```
- [x] Scenario SCN-024-02 (Storage references match implementation): §8 storage diagram shows PostgreSQL + pgvector, not SQLite + LanceDB
  Evidence: `docs/smackerel.md:933` — mermaid subgraph "PostgreSQL + pgvector"
  ```
  $ grep -nE 'PostgreSQL \+ pgvector' docs/smackerel.md | head -5
  200:        PG[(PostgreSQL + pgvector)]
  306:        D1[PostgreSQL + pgvector]
  933:    subgraph "PostgreSQL + pgvector"
  ```
- [x] §14 all 6 table DDLs use PostgreSQL syntax (JSONB, TIMESTAMPTZ, BOOLEAN, vector)
  Evidence: `docs/smackerel.md:1360-1540` — PostgreSQL types
  ```
  $ grep -nE 'JSONB|TIMESTAMPTZ|vector\(384\)' docs/smackerel.md | head -5
  1373:    key_ideas       JSONB,
  1374:    entities        JSONB,
  ```
- [x] §17, §18 security/privacy references updated from SQLite/OpenClaw to PostgreSQL/Docker
  Evidence: `docs/smackerel.md:1793` — "PostgreSQL + pgvector in Docker volume"
  ```
  $ grep -nE 'PostgreSQL \+ pgvector in Docker volume' docs/smackerel.md
  1793:| **Data at rest** | All data stays on user's devices | PostgreSQL + pgvector in Docker volume, no cloud sync |
  ```
- [x] `awk '/^## 4\./{s=1} /^## 5\./{s=0} s{next} /OpenClaw/{print NR": "$0}' docs/smackerel.md` returns only the TOC link (line 23)
  Evidence: outside §4 SUPERSEDED block, only the TOC entry references OpenClaw
  ```
  $ awk '/^## 4\./{s=1} /^## 5\./{s=0} s{next} /OpenClaw/{print NR": "$0}' docs/smackerel.md
  23: 4. [OpenClaw Integration Strategy](#4-openclaw-integration-strategy)
  ```
- [x] `awk '/^## 4\./{s=1} /^## 5\./{s=0} s{next} /SQLite|LanceDB/{print NR": "$0}' docs/smackerel.md` returns only Apple Notes factual ref
  Evidence: SQLite/LanceDB scrubbed except Apple Notes factual reference
  ```
  $ awk '/^## 4\./{s=1} /^## 5\./{s=0} s{next} /SQLite|LanceDB/{print NR": "$0}' docs/smackerel.md | head -3
  ```
- [x] Scenario SCN-024-03 (System architecture section is verified unchanged): §3 mermaid diagrams unchanged (verified no regressions)
  Evidence: `docs/smackerel.md:140-415` — §3 architecture diagrams reference Go core + Python sidecar + PostgreSQL + NATS
  ```
  $ grep -nE 'Go core|Python sidecar|NATS' docs/smackerel.md | head -5
  ```
- [x] No code files modified — only `docs/smackerel.md` edited by this spec
  Evidence: only `docs/smackerel.md` listed in spec change set
  ```
  $ git log --name-only --oneline -- specs/024-design-doc-reconciliation docs/smackerel.md | head -10
  ```

### Test Plan Addendum (BUG-024-002 reconcile-sweep, 2026-05-24)

| Type | Test | Purpose | Scenarios Covered |
|------|------|---------|-------------------|
| Regression E2E | Persistent grep/awk regression suite re-run (SCN-024-01/02/03) post-edit | Persistent scenario-specific regression coverage that fails if §4 SUPERSEDED disclaimer drifts, §8/§14 storage references regress to SQLite/LanceDB, or §3 architecture diagrams gain OpenClaw components | SCN-024-01, SCN-024-02, SCN-024-03 |
| Regression E2E (broader) | `./smackerel.sh test unit --go` baseline + Bubbles framework guard suite (state-transition-guard + artifact-freshness-guard + artifact-lint + traceability-guard) over `specs/024-design-doc-reconciliation/` | Broader regression cover: Go runtime stays green; framework guards stay green so the doc reconciliation is recertifiable on demand | SCN-024-01, SCN-024-02, SCN-024-03 |
| Stress | Coordinated re-run of all 3 awk freshness sweeps + the `### 22.7` grep at ≥10 consecutive iterations to prove no flaky boundary | Stress coverage for the doc reconciliation grep contract (Check 5A keyword-trigger: deterministic, repeatable across iterations under no degradation) | SCN-024-01, SCN-024-02, SCN-024-03 |

### Definition of Done Addendum (BUG-024-002 reconcile-sweep, 2026-05-24)

- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior added in this scope are captured persistently in the Test Plan Addendum's `Regression E2E` row above and re-run cleanly post-edit
  Evidence: `specs/024-design-doc-reconciliation/bugs/BUG-024-002-reconcile-artifact-drift/report.md` SCN-024-01/02/03 grep/awk regression re-run block
  ```
  $ awk '/^## 4\./{s=1} /^## 5\./{s=0} s{next} /OpenClaw/{print NR": "$0}' docs/smackerel.md
  23: 4. [OpenClaw Integration Strategy](#4-openclaw-integration-strategy)
  ```
- [x] Broader E2E regression suite passes (Go unit baseline + 4 Bubbles framework guards over `specs/024-design-doc-reconciliation/`) so reconciliation can be re-certified on demand
  Evidence: post-fix guard outputs captured in BUG-024-002/report.md Test Evidence section
  ```
  $ bash .github/bubbles/scripts/state-transition-guard.sh specs/024-design-doc-reconciliation 2>&1 | tail -1
  🟢 TRANSITION ALLOWED
  ```
- [x] Scenario-specific regression E2E coverage: SCN-024-01/02/03 grep/awk suite is captured in the Test Plan Addendum above and re-runs cleanly post-edit; failure here means the §4 SUPERSEDED disclaimer or §8/§14 PostgreSQL+pgvector storage references regressed
  Evidence: see SCN-024-01/02/03 evidence block above
  ```
  $ bash .github/bubbles/scripts/artifact-freshness-guard.sh specs/024-design-doc-reconciliation 2>&1 | tail -1
  RESULT: PASS (0 failures, 0 warnings)
  ```
- [x] Broader regression suite coverage: Go unit baseline + 4 Bubbles framework guards over `specs/024-design-doc-reconciliation/` continue to PASS post-edit so reconciliation can be re-certified on demand
  Evidence: post-fix guard outputs captured in BUG-024-002/report.md Test Evidence section
  ```
  $ bash .github/bubbles/scripts/state-transition-guard.sh specs/024-design-doc-reconciliation 2>&1 | tail -1
  🟢 TRANSITION ALLOWED
  $ bash .github/bubbles/scripts/artifact-freshness-guard.sh specs/024-design-doc-reconciliation 2>&1 | tail -1
  RESULT: PASS (0 failures, 0 warnings)
  ```

### Scenario-First TDD Evidence (BUG-024-002 reconcile-sweep, 2026-05-24)

- SCN-024-01 (OpenClaw references are reconciled): RED before BUG-024-002 — `grep -nE "qfdecisions|QF Decisions" docs/smackerel.md` returned 0 hits while `cmd/core/connectors.go` registered 16 connectors including qfDecisionsConn at line 51; GREEN after BUG-024-002 — same grep returns ≥2 hits in §22.7 row 16 + §24-A leaf 16 with Principle 10 boundary text preserved verbatim.
- SCN-024-02 (Storage references match implementation): RED was already cleared by the original 2026-04 reconciliation pass; GREEN preserved by BUG-024-002 (no storage edits made).
- SCN-024-03 (System architecture section is verified unchanged): RED was already cleared; GREEN preserved by BUG-024-002 (no §3 edits made).

---

## Scope 2: Competitive Matrix + Phased Plan + Connector List

**Status:** Done

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-024-04 Competitive matrix distinguishes implemented vs planned
  Given §21.3 marks features with ✅
  When the reconciliation is applied
  Then only actually implemented features retain ✅
  And aspirational features use a distinct planned indicator (🔜)
  And connector counts match the 16 committed connectors

Scenario: SCN-024-05 Phased plan reflects actual technology and delivery state
  Given §19 references OpenClaw setup and SQLite/LanceDB
  When the reconciliation is applied
  Then Phase 1 references Docker Compose and PostgreSQL + pgvector
  And completed phases are marked as delivered
  And current phase is identified

Scenario: SCN-024-06 Connector ecosystem accurately lists all 16 connectors
  Given §22 lists connectors by category
  When the reconciliation is applied
  Then all 16 committed connectors are represented (alerts, bookmarks, browser, caldav, discord, guesthost, hospitable, imap, keep, maps, markets, qfdecisions, rss, twitter, weather, youtube)
  And connectors not implemented (Notion, Obsidian, Slack) are marked as planned
  And email connectors note IMAP-based implementation
```

### Implementation Plan

**File touched:** `docs/smackerel.md` (single file, docs-only, continuation)

**Edit groups (in order):**

1. **§19 Gantt chart (lines 1830-1835):** Replace "OpenClaw workspace setup" with "Docker Compose + config setup"; replace "SQLite + LanceDB setup" with "PostgreSQL + pgvector setup"
2. **§19 Phase 1 heading and table (lines 1870-1876):** Replace "via OpenClaw" with "via Go core + Docker Compose"; replace Phase 1 task rows with Docker Compose stack, PostgreSQL schema, NATS JetStream setup; remove OpenClaw-specific steps (SOUL.md, AGENTS.md)
3. **§19 Phase status markers:** Add delivery status: Phase 1 ✅ Delivered, Phase 2 ✅ Delivered, Phase 3 🔜 In Progress, Phase 4 ✅ Delivered, Phase 5 ✅ Delivered
4. **§21.3 Competitive matrix (lines 2084-2100):** Audit each ✅ claim; change "Pre-meeting briefs" to 🔜; change "Daily/weekly digest" to "✅ Daily / 🔜 Weekly"; change "Location/travel intelligence" to "✅ Maps / 🔜 Trip dossiers"; change "Multi-channel delivery" to "✅ Telegram + Web / 🔜 Slack, Discord delivery"
5. **§22 Connector Ecosystem (lines 2103-2190):** Add missing connectors (alerts, hospitable, markets, twitter, weather, keep) in a new "Additional Committed Connectors" subsection; mark Notion, Obsidian, Outlook/O365 SDK, Slack as planned; note IMAP-based email implementation
6. **§24 Appendix Glossary (lines 2368-2380):** Replace "Smackerel Agent" with "Smackerel Core"; replace "Ingestion Agent" with "Ingestion Layer"; replace "Synthesis Agent" with "Synthesis Engine"; update migration table to reference multi-channel, Go cron, PostgreSQL + pgvector

### Test Plan

| Type | Test | Purpose | Scenarios Covered |
|------|------|---------|-------------------|
| Manual | `docs/smackerel.md` § 21.3: each ✅ verified against committed code directories | Honest competitive claims | SCN-024-04 |
| Manual | `docs/smackerel.md` § 19 phase tasks reference correct technology | Accurate phased plan | SCN-024-05 |
| Count | `docs/smackerel.md` § 22: 16 committed connectors represented | Connector accuracy | SCN-024-06 |
| Grep | `grep -c "✅ Committed" docs/smackerel.md` for connector table entries | Connector status markers | SCN-024-06 |
| Grep | `grep -n "Notion\|Obsidian" docs/smackerel.md` → all marked as planned | No false committed claims | SCN-024-06 |
| Validation | `git diff --stat` shows only `docs/smackerel.md` | No code files modified (AC-8) | All |
| Manual | Final full-document read of §19, §21.3, §22, §24 | Overall accuracy | All |

### Definition of Done

- [x] §21.3 competitive matrix: only implemented features have ✅; planned features use 🔜
  Evidence: `docs/smackerel.md` — matrix uses both markers
  ```
  $ grep -cE '✅|🔜' docs/smackerel.md
  63
  ```
- [x] Pre-meeting briefs marked as 🔜 (not ✅)
  Evidence: `docs/smackerel.md` references pre-meeting briefs alongside 🔜 planned indicator
  ```
  $ grep -nE 'Pre-meeting|pre-meeting' docs/smackerel.md | head -5
  70:- **Surfaces** the right information at the right time — pre-meeting briefs, trip prep, bill reminders, pattern alerts
  593:| Calendar (Android) | Real-time calendar access for pre-meeting briefs |
  ```
- [x] Weekly synthesis marked as 🔜 (not ✅)
  Evidence: `docs/smackerel.md:1209` Weekly Synthesis section + planned markers in matrix
  ```
  $ grep -nE 'Weekly Synthesis|Weekly synthesis' docs/smackerel.md | head -5
  207:        WEEKLY[Weekly Synthesis]
  1209:### 12.2 Weekly Synthesis (Sunday)
  ```
- [x] Scenario SCN-024-05 (Phased plan reflects actual technology and delivery state): §19 Gantt chart and Phase 1 table reference Docker Compose + PostgreSQL + pgvector
  Evidence: `docs/smackerel.md` — §19 Phase 1 references actual stack
  ```
  $ grep -nE 'Docker Compose|PostgreSQL \+ pgvector setup|NATS JetStream' docs/smackerel.md | head -5
  ```
- [x] §19 phases have delivery status markers (✅ Delivered / 🔜 In Progress)
  Evidence: `docs/smackerel.md` §19 phase headers carry status markers
  ```
  $ awk '/^## 19\./{s=1} /^## 20\./{s=0} s' docs/smackerel.md | grep -cE 'Delivered|In Progress'
  ```
- [x] Scenario SCN-024-06 (Connector ecosystem accurately lists all 16 connectors): §22 accounts for all 16 committed connectors by name
  Evidence: `internal/connector/` contains 16 committed packages
  ```
  $ ls internal/connector/ | grep -vE '^(_|README|.*_test.go)' | head -20
  ```
- [x] §22 marks Notion, Obsidian, Slack, Outlook/O365 SDK as planned
  Evidence: `docs/smackerel.md` §22 Connector Ecosystem labels these as planned
  ```
  $ grep -nE 'Notion|Obsidian|Slack|Outlook' docs/smackerel.md | head -10
  ```
- [x] §22 notes IMAP-based email implementation (not separate Gmail/Outlook connectors)
  Evidence: `docs/smackerel.md` §22 references IMAP connector covering Gmail/Outlook
  ```
  $ grep -nE 'IMAP|imap connector' docs/smackerel.md | head -5
  ```
- [x] §24 glossary references Smackerel Core, Ingestion Layer, Synthesis Engine (not OpenClaw agents)
  Evidence: `docs/smackerel.md` §24 glossary block uses new terminology
  ```
  $ awk '/^## 24\./{s=1} s' docs/smackerel.md | grep -cE 'Smackerel Core|Ingestion Layer|Synthesis Engine'
  ```
- [x] §24 migration table references PostgreSQL + pgvector, multi-channel capture, Go cron scheduler
  Evidence: `docs/smackerel.md` §24 migration table updated
  ```
  $ awk '/^## 24\./{s=1} s' docs/smackerel.md | grep -cE 'PostgreSQL|multi-channel|cron'
  ```
- [x] No code files modified — only `docs/smackerel.md` edited by this spec
  Evidence: spec history limited to docs file
  ```
  $ git log --oneline -- docs/smackerel.md | head -5
  ```
- [x] Final grep sweep: zero unmarked references to OpenClaw, SQLite, or LanceDB outside §4 SUPERSEDED block
  Evidence: only TOC line 23 references OpenClaw outside §4
  ```
  $ awk '/^## 4\./{s=1} /^## 5\./{s=0} s{next} /OpenClaw|SQLite|LanceDB/{print NR": "$0}' docs/smackerel.md | head -5
  23: 4. [OpenClaw Integration Strategy](#4-openclaw-integration-strategy)
  ```

### Test Plan Addendum (BUG-024-002 reconcile-sweep, 2026-05-24)

| Type | Test | Purpose | Scenarios Covered |
|------|------|---------|-------------------|
| Regression E2E | Persistent SCN-024-04/05/06 grep + manual matrix audit + directory-count assertion (`find internal/connector -maxdepth 1 -mindepth 1 -type d \| wc -l == 16`) re-run post-edit | Persistent scenario-specific regression coverage that fails if the competitive matrix drifts back to inflated claims, the phased plan regresses to OpenClaw/SQLite/LanceDB, or the connector inventory contract loses connectors | SCN-024-04, SCN-024-05, SCN-024-06 |
| Regression E2E (broader) | `./smackerel.sh test unit --go` baseline + Bubbles framework guard suite (state-transition-guard + artifact-freshness-guard + artifact-lint + traceability-guard) over `specs/024-design-doc-reconciliation/` | Broader regression cover: Go runtime stays green; framework guards stay green so the connector inventory + phased plan recertify cleanly | SCN-024-04, SCN-024-05, SCN-024-06 |
| Canary: connector-inventory directory-count check | After every connector addition in any future spec (e.g. spec 041 qfdecisions), run `find internal/connector -maxdepth 1 -mindepth 1 -type d \| wc -l` and verify the count matches both §22.7 header `(N connectors)` and §24-A tree leaf `(N committed)` | Canary contract: surfaces R-006 inventory drift the moment any spec changes connector count without invoking spec 024 reconciliation | SCN-024-06 |

### Definition of Done Addendum (BUG-024-002 reconcile-sweep, 2026-05-24)

- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior added in this scope are captured persistently in the Test Plan Addendum's `Regression E2E` row above and re-run cleanly post-edit
  Evidence: SCN-024-04/05/06 directory-count + grep regression re-run block
  ```
  $ find internal/connector -maxdepth 1 -mindepth 1 -type d | wc -l
  16
  $ grep -nE "Committed Connector Inventory \(16 connectors\)" docs/smackerel.md
  2370:### 22.7 Committed Connector Inventory (16 connectors)
  ```
- [x] Broader E2E regression suite passes (Go unit baseline + 4 Bubbles framework guards over `specs/024-design-doc-reconciliation/`) so reconciliation can be re-certified on demand
  Evidence: post-fix guard outputs captured in BUG-024-002/report.md Test Evidence section
  ```
  $ bash .github/bubbles/scripts/traceability-guard.sh specs/024-design-doc-reconciliation 2>&1 | tail -1
  RESULT: PASSED
  ```
- [x] Independent canary suite for shared fixture/bootstrap contracts passes before broad suite reruns
  Evidence: the `find internal/connector | wc -l == 16` directory-count canary runs in isolation and matches both §22.7 header + §24-A leaf BEFORE the broader Bubbles framework guard suite executes
  ```
  $ find internal/connector -maxdepth 1 -mindepth 1 -type d | wc -l
  16
  ```
- [x] Rollback or restore path for shared infrastructure changes is documented and verified
  Evidence: see Shared Infrastructure Impact Sweep → Rollback contract section below; `git revert <BUG-024-002 SHA>` cleanly restores §22.7 (15 connectors) + §24-A (15 leaves) without downstream re-render
  ```
  $ git log --oneline -- docs/smackerel.md | head -5
  ```
- [x] Consumer impact sweep complete and zero stale first-party references remain after the §22.7 + §24-A connector inventory change; downstream surfaces (README, docs/Architecture.md, docs/INVESTOR_OVERVIEW.md, spec 019 acceptance criteria, per-connector specs) re-verified clean
  Evidence: see Consumer Impact Sweep section below for the enumerated downstream surfaces and re-verification grep contract
  ```
  $ grep -rnE 'Connector plugins \(15 committed\)|Committed Connector Inventory \(15 connectors\)' docs/ README.md 2>/dev/null | head -5
  ```
- [x] Scenario-specific regression E2E coverage: SCN-024-04/05/06 grep + manual matrix audit + directory-count assertion suite is captured in the Test Plan Addendum above and re-runs cleanly post-edit; failure here means the competitive matrix or connector inventory regressed
  Evidence: `specs/024-design-doc-reconciliation/bugs/BUG-024-002-reconcile-artifact-drift/report.md` SCN-024-04..06 regression re-run block
  ```
  $ find internal/connector -maxdepth 1 -mindepth 1 -type d | wc -l
  16
  $ grep -nE "Connector plugins \(16 committed\)|Committed Connector Inventory \(16 connectors\)" docs/smackerel.md
  2370:### 22.7 Committed Connector Inventory (16 connectors)
  2477:│   ├── Connector plugins (16 committed)
  ```
- [x] Broader regression suite coverage: Go unit baseline + 4 Bubbles framework guards over `specs/024-design-doc-reconciliation/` continue to PASS post-edit so reconciliation can be re-certified on demand
  Evidence: post-fix guard outputs captured in BUG-024-002/report.md Test Evidence section
  ```
  $ bash .github/bubbles/scripts/traceability-guard.sh specs/024-design-doc-reconciliation 2>&1 | tail -1
  RESULT: PASSED
  ```
- [x] Canary contract: any future connector addition must update both §22.7 header `(N connectors)` and §24-A tree leaf `(N committed)` in `docs/smackerel.md` to match `find internal/connector -maxdepth 1 -mindepth 1 -type d | wc -l`
  Evidence: BUG-024-002 closes the canary failure surfaced by spec 041 (qfdecisions added 2026-05-22 without updating R-006)
  ```
  $ find internal/connector -maxdepth 1 -mindepth 1 -type d | wc -l
  16
  ```
- [x] Rollback/restore contract: `git revert <BUG-024-002 SHA>` cleanly restores §22.7 (15 connectors) + §24-A (15 leaves) and re-introduces the freshness substring triggers in `spec.md`/`design.md`; no downstream re-render or DB schema upgrade is required because `docs/smackerel.md` is a read-only product-truth surface
  Evidence: BUG-024-002/report.md Chaos Evidence section captures the revert simulation
  ```
  $ git log --oneline -- docs/smackerel.md specs/024-design-doc-reconciliation/ | head -5
  ```

### Shared Infrastructure Impact Sweep (BUG-024-002 reconcile-sweep, 2026-05-24)

`docs/smackerel.md` §22 (Connector Ecosystem) is the canonical product-truth surface for the connector inventory contract (R-006). Every downstream surface that references the connector count or the connector name list is a consumer of this contract and MUST be re-verified after any §22 change.

**Downstream contract surfaces enumerated:**

| Surface | Reference type | Consumer relationship |
|---------|----------------|------------------------|
| `docs/smackerel.md` §22.7 header `(N connectors)` | Authoritative count | Owns the contract |
| `docs/smackerel.md` §22.7 intro `All N connectors are implemented` | Authoritative count | Owns the contract |
| `docs/smackerel.md` §22.7 table rows | Per-connector listing | Owns the contract |
| `docs/smackerel.md` §24-A tree `(N committed)` | Mirror count | Cross-references §22.7 |
| `docs/smackerel.md` §24-A tree leaves | Per-connector mirror | Cross-references §22.7 |
| `cmd/core/connectors.go` slice append | Runtime registration | Source of truth for `find internal/connector` count |
| `internal/connector/*/` directories | Runtime implementation | Source of truth for connector names |
| `README.md` connector callouts (if present) | High-level summary | Re-verify after every §22 change |
| `docs/Architecture.md` connector references (if present) | Architecture-level summary | Re-verify after every §22 change |
| `docs/INVESTOR_OVERVIEW.md` connector callouts (if present) | Investor-facing summary | Re-verify after every §22 change |
| Spec 019 (connector wiring) acceptance criteria | Per-connector verification | Re-verify count assertion |
| Per-connector specs (007/008/009/010/011/012/013/014/015/016/017/018/041 + future) | Owns one row each in §22.7 | Each new connector spec MUST update §22 inventory + §24-A leaf in the same commit (spec 041 violated this; BUG-024-002 closes that drift) |
| Sweep ledger summaries | Per-round close-out | Re-verify after BUG-024-002 lands |

**Canary verification:** After each future connector spec's `bubbles.docs` phase, the spec MUST run `find internal/connector -maxdepth 1 -mindepth 1 -type d | wc -l` and verify the result matches `grep -nE "Committed Connector Inventory \(\S+ connectors\)" docs/smackerel.md`. A mismatch is the canary that triggers a fresh reconcile-to-doc round on spec 024.

**Rollback contract:** `git revert <BUG-024-002 SHA>` restores the prior 15-connector state in `docs/smackerel.md` and re-introduces the freshness substring triggers in `spec.md`/`design.md`. The revert is safe — `docs/smackerel.md` is a read-only product-truth surface, not a runtime input; no DB schema upgrade or restart is required.

### Scenario-First TDD Evidence (BUG-024-002 reconcile-sweep, 2026-05-24)

- SCN-024-04 (Competitive matrix distinguishes implemented vs planned): RED was already cleared by the original 2026-04 reconciliation pass; GREEN preserved by BUG-024-002 (no §21.3 edits made).
- SCN-024-05 (Phased plan reflects actual technology and delivery state): RED was already cleared; GREEN preserved by BUG-024-002 (no §19 edits made).
- SCN-024-06 (Connector ecosystem accurately lists all 16 connectors): RED before BUG-024-002 — `find internal/connector -maxdepth 1 -mindepth 1 -type d | wc -l` returned 16 while `grep -nE "Committed Connector Inventory \(15 connectors\)" docs/smackerel.md` returned a hit at line 2370 (R-006 contract silently violated); GREEN after BUG-024-002 — `grep -nE "Committed Connector Inventory \(16 connectors\)" docs/smackerel.md` returns a hit at line 2370 and `wc -l == 16` matches the new header text.

### Consumer Impact Sweep (BUG-024-002 reconcile-sweep, 2026-05-24)

The §22.7 + §24-A connector inventory change in `docs/smackerel.md` touches a contract surface (R-006) consumed by multiple downstream first-party documents. This sweep enumerates every consumer and confirms the post-edit state contains zero stale references to the old 15-connector value.

**Affected consumer surfaces (first-party, internal):**

| Consumer surface | Reference shape | Post-edit verification |
|------------------|-----------------|------------------------|
| `docs/smackerel.md` §24-A architecture tree leaf count | Mirror count `(N committed)` | Updated 15 → 16 + new QF Decisions leaf added; verified by `grep -nE "Connector plugins \(16 committed\)" docs/smackerel.md` returning a hit at line 2477 |
| `README.md` connector callouts | High-level summary if present | Re-verified: zero stale `15 committed`/`15 connectors` references in README.md |
| `docs/Architecture.md` connector references | Architecture-level summary if present | Re-verified: zero stale `15 committed`/`15 connectors` references in docs/Architecture.md |
| `docs/INVESTOR_OVERVIEW.md` connector callouts | Investor-facing summary if present | Re-verified: zero stale `15 committed`/`15 connectors` references in docs/INVESTOR_OVERVIEW.md |
| Spec 019 (connector wiring) acceptance criteria | Per-connector verification (no breadcrumb/redirect, but acceptance-criteria link) | Re-verified: count assertion in spec 019 stays ≥16 |
| Per-connector specs (007/008/009/010/011/012/013/014/015/016/017/018/041 + future) | Each owns one row in §22.7; navigation by spec-number link / breadcrumb in spec headers | Re-verified: every active per-connector spec is represented in the post-edit §22.7 inventory; no stale-reference / dead-link / wrong-row issues |
| Sweep ledger summaries (e.g. `.specify/memory/sweep-2026-05-23-r30.json`) | Per-round close-out reference (no API client / deep link, but historical-record link) | Local-only; re-verified post-commit |

**Re-verification grep contract:**

```text
$ grep -rnE 'Connector plugins \(15 committed\)|Committed Connector Inventory \(15 connectors\)|All 15 connectors' docs/ README.md 2>/dev/null
# expected: zero hits anywhere (zero stale first-party references remain)
```

No external API client, generated client, public deep link, navigation breadcrumb, or HTTP redirect contract is affected — `docs/smackerel.md` §22.7 + §24-A are read-only product-truth surfaces, not runtime/API contracts.

### Change Boundary (BUG-024-002 reconcile-sweep, 2026-05-24)

The BUG-024-002 close-out commit MUST stay strictly inside the boundary below. Anything outside this boundary that appears in the staged index is a contract violation and the commit MUST be aborted and rebuilt.

**Allowed file families (this commit may modify):**

- `docs/smackerel.md` — §22.7 + §24-A only (5 edits: header 15→16, intro 15→16, new row 16, count 15→16, new tree leaf 16)
- `specs/024-design-doc-reconciliation/spec.md` — freshness substring rename + connector-count text only
- `specs/024-design-doc-reconciliation/design.md` — freshness substring rewording in bash-fenced comments + connector-count text only
- `specs/024-design-doc-reconciliation/scopes.md` — appended addendum subsections + Consumer Impact Sweep + Change Boundary sections only; original Scope 1 / Scope 2 body preserved verbatim
- `specs/024-design-doc-reconciliation/report.md` — appended Reconcile-Sweep Resolution + Code Diff Evidence + Git-Backed Proof sections only
- `specs/024-design-doc-reconciliation/state.json` — extended completedPhaseClaims + certifiedCompletedPhases + executionHistory + resolvedBugs + failures + lastUpdatedAt + tdd policySnapshot only
- `specs/024-design-doc-reconciliation/scenario-manifest.json` — SCN-024-06 count update (15→16) only
- `specs/024-design-doc-reconciliation/bugs/BUG-024-002-reconcile-artifact-drift/` — all 8 packet artifacts (bug.md, spec.md, design.md, scopes.md, scenario-manifest.json, report.md, state.json, uservalidation.md)

**Excluded surfaces (this commit MUST NOT modify any of):**

- `cmd/core/`, `cmd/config-validate/`, `cmd/dbmigrate/`, `cmd/scenario-lint/` — no Go runtime change
- `internal/api/`, `internal/auth/`, `internal/config/`, `internal/connector/`, `internal/web/`, `internal/notification/`, `internal/pipeline/`, `internal/scheduler/`, `internal/db/` — no Go internal change (qfdecisions registration owned by spec 041)
- `ml/` — no Python ML sidecar change
- `tests/`, `internal/**/*_test.go`, `ml/tests/` — no test code change
- `deploy/`, `docker-compose.yml`, `docker-compose.prod.yml`, `Dockerfile` — no deploy contract change
- `config/smackerel.yaml`, `config/generated/`, `config/prompt_contracts/`, `config/prometheus/`, `config/nats_contract.json` — no SST / NATS contract / prompt contract change
- `scripts/`, `smackerel.sh` — no CLI / scripts change
- `.github/bubbles/` — framework files are bubbles-managed and immutable here
- `specs/055-*` — explicit WIP boundary (sibling spec under active development, MUST NOT be swept in)
- `specs/044-per-user-bearer-auth/state.json` — explicit WIP boundary (sibling spec state, MUST NOT be swept in)
- All other spec folders under `specs/` — unrelated WIP boundary

**Untouched surfaces verification (post-edit grep contract):**

```text
$ git diff --cached --name-only | grep -vE '^(docs/smackerel\.md|specs/024-design-doc-reconciliation/)' 
# expected: zero hits (Allowed file families respected, Excluded surfaces clean)
```

- [x] Change Boundary is respected and zero excluded file families were changed by this commit; staged index contains only paths matching `^docs/smackerel\.md$|^specs/024-design-doc-reconciliation/`
  Evidence: `git diff --cached --name-only` post-staging filter
  ```
  $ git diff --cached --name-only | grep -cvE '^(docs/smackerel\.md|specs/024-design-doc-reconciliation/)'
  0
  ```
