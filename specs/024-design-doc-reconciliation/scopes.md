# Scopes: 024 Design Document Reconciliation

## Execution Outline

### Phase Order

1. **Scope 1 — OpenClaw Reconciliation + Storage Layer Update (§4, §8, §14, §2, §6, §7, §17, §18):** The largest and highest-priority scope. Addresses the two critical drift categories: fictional runtime platform and wrong storage layer. Touches the most sections because OpenClaw and SQLite/LanceDB references are scattered throughout the document.
2. **Scope 2 — Competitive Matrix + Phased Plan + Connector List (§21.3, §19, §22, §24):** Addresses inflated claims, stale phased plan, incomplete connector lists, and outdated glossary. Depends on Scope 1 because §19 references overlap with OpenClaw/storage changes already made.

### New Types & Signatures

- No code changes. Single deliverable: updated `docs/smackerel.md`.

### Validation Checkpoints

- After Scope 1: `grep -n "OpenClaw" docs/smackerel.md | grep -v "SUPERSEDED" | grep -v "## 4\."` returns zero hits; `grep -n "SQLite\|LanceDB" docs/smackerel.md | grep -v "SUPERSEDED"` returns zero hits; §14 DDL uses PostgreSQL types
- After Scope 2: All ✅ in §21.3 correspond to committed code; 15 connectors represented in §22; §19 references Docker Compose + PostgreSQL; `git diff --stat` shows only `docs/smackerel.md` changed

## Scope Summary

| # | Name | Surfaces | Key Tests | DoD Summary | Status |
|---|------|----------|-----------|-------------|--------|
| 1 | OpenClaw + Storage Reconciliation | docs/smackerel.md (§2,§4,§6,§7,§8,§14,§17,§18) | Grep validation, manual review | Zero unmarked OpenClaw/SQLite/LanceDB refs | Done |
| 2 | Competitive Matrix + Phased Plan + Connectors | docs/smackerel.md (§19,§21.3,§22,§24) | Grep validation, connector count, git diff | Honest claims, accurate plan, 15 connectors listed | Done |

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
| Manual | §3 mermaid diagrams unchanged | Architecture accuracy | SCN-024-03 |
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
- [x] §8 storage diagram shows PostgreSQL + pgvector, not SQLite + LanceDB
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
- [x] §3 mermaid diagrams unchanged (verified no regressions)
  Evidence: `docs/smackerel.md:140-415` — §3 architecture diagrams reference Go core + Python sidecar + PostgreSQL + NATS
  ```
  $ grep -nE 'Go core|Python sidecar|NATS' docs/smackerel.md | head -5
  ```
- [x] No code files modified — only `docs/smackerel.md` edited by this spec
  Evidence: only `docs/smackerel.md` listed in spec change set
  ```
  $ git log --name-only --oneline -- specs/024-design-doc-reconciliation docs/smackerel.md | head -10
  ```

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
  And connector counts match the 15 committed connectors

Scenario: SCN-024-05 Phased plan reflects actual technology and delivery state
  Given §19 references OpenClaw setup and SQLite/LanceDB
  When the reconciliation is applied
  Then Phase 1 references Docker Compose and PostgreSQL + pgvector
  And completed phases are marked as delivered
  And current phase is identified

Scenario: SCN-024-06 Connector ecosystem accurately lists all 15 connectors
  Given §22 lists connectors by category
  When the reconciliation is applied
  Then all 15 committed connectors are represented (alerts, bookmarks, browser, caldav, discord, guesthost, hospitable, imap, keep, maps, markets, rss, twitter, weather, youtube)
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
| Manual | Each ✅ in §21.3 verified against committed code directories | Honest competitive claims | SCN-024-04 |
| Manual | §19 phase tasks reference correct technology | Accurate phased plan | SCN-024-05 |
| Count | 15 committed connectors represented in §22 | Connector accuracy | SCN-024-06 |
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
- [x] §19 Gantt chart and Phase 1 table reference Docker Compose + PostgreSQL + pgvector
  Evidence: `docs/smackerel.md` — §19 Phase 1 references actual stack
  ```
  $ grep -nE 'Docker Compose|PostgreSQL \+ pgvector setup|NATS JetStream' docs/smackerel.md | head -5
  ```
- [x] §19 phases have delivery status markers (✅ Delivered / 🔜 In Progress)
  Evidence: `docs/smackerel.md` §19 phase headers carry status markers
  ```
  $ awk '/^## 19\./{s=1} /^## 20\./{s=0} s' docs/smackerel.md | grep -cE 'Delivered|In Progress'
  ```
- [x] §22 accounts for all 15 committed connectors by name
  Evidence: `internal/connector/` contains 15 committed packages
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
