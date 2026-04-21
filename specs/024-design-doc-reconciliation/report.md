# Report: 024 Design Document Reconciliation

## Summary

**Feature:** 024-design-doc-reconciliation
**Scopes:** 2
**Status:** Done
**Mode:** delivery-lockdown
**Completed:** 2026-04-10

| Scope | Name | Status |
|-------|------|--------|
| 1 | OpenClaw + Storage Reconciliation | ✅ Done |
| 2 | Competitive Matrix + Phased Plan + Connectors | ✅ Done |

## Test Evidence

### Scope 1: OpenClaw + Storage Reconciliation

**Grep validation — OpenClaw references:**
- `awk` scan for OpenClaw outside §4 body: only line 23 (TOC link to retained §4 heading) — expected and correct
- All other OpenClaw refs confirmed inside §4 SUPERSEDED block

**Grep validation — SQLite/LanceDB references:**
- `awk` scan for SQLite/LanceDB outside §4: only line 2155 (Apple Notes' own SQLite DB) — factual, not about Smackerel

**§3 preservation check:**
- §3 contains 7 references to PostgreSQL/pgvector — unchanged from before
- §3 contains 0 references to SQLite/LanceDB — confirmed no regressions

**Edits applied (Scope 1):**
1. Header metadata: `Runtime Platform: OpenClaw` → `Go + Docker Compose (self-hosted)`
2. §2 Principle 9: "via OpenClaw" → "via Docker Compose"
3. §2 Principle 10: Replaced OpenClaw-specific language with modular architecture description
4. §4: Added prominent ⚠️ SUPERSEDED disclaimer; §4.1–§4.5 retained as historical context
5. §6.1: "OpenClaw-connected channel" → "connected channel"; table updated (Telegram bot, Web UI)
6. §7 Stage 2: "OpenClaw browser control" → "go-readability"
7. §7 Stage 4: "LanceDB" → "PostgreSQL via pgvector"; updated embedding dimensions to 384
8. §8.1: Replaced SQLite+LanceDB+Workspace diagram with PostgreSQL+pgvector unified diagram
9. §14: All 6 table DDLs rewritten in PostgreSQL syntax (JSONB, TIMESTAMPTZ, BOOLEAN, vector(384), IF NOT EXISTS, indexes)
10. §16.2: "OpenClaw app" → "Telegram bot"
11. §17.2: Updated access control, data-at-rest, and API key management rows
12. §18.3: "SQLite + LanceDB export" → "PostgreSQL pg_dump export"

### Scope 2: Competitive Matrix + Phased Plan + Connectors

**§21.3 competitive matrix audit:**
- Pre-meeting briefs: ✅ → 🔜 (meeting_briefs table exists but delivery not complete)
- Daily/weekly digest: ✅ → ✅ Daily / 🔜 Weekly (daily generator committed, weekly synthesis in progress)
- Location/travel: ✅ → ✅ Maps / 🔜 Trip dossiers
- Multi-channel: ✅ → ✅ Telegram + Web / 🔜 Slack, Discord

**§19 phased plan updates:**
- Gantt: "OpenClaw workspace setup" → "Docker Compose + config setup"; "SQLite + LanceDB setup" → "PostgreSQL + pgvector setup"
- Phase 1 table: Replaced 11 OpenClaw/skill tasks with actual Docker Compose + PostgreSQL + CLI tasks
- Delivery status markers added: Phase 1 ✅, Phase 2 ✅, Phase 3 🔜, Phase 4 ✅, Phase 5 ✅

**§22 connector inventory:**
- 15 committed connectors verified in §22.7 inventory table
- Each verified against `internal/connector/` directory (including guesthost, added in hardening pass)
- Status column added to all existing connector tables (Email, Calendar, Chat, Notes)
- IMAP listed as primary email connector (✅ Committed); Gmail/Outlook SDKs as 🔜 Planned
- Google Keep added to Notes section (✅ Committed)
- Notion, Obsidian, Slack, Teams marked as 🔜 Planned

**§24 glossary + migration table:**
- "Smackerel Agent" → "Smackerel Core"
- "Ingestion Agent" → "Ingestion Layer"
- "Synthesis Agent" → "Synthesis Engine"
- Migration table: OpenClaw refs → multi-channel, Go cron, PostgreSQL + pgvector

**Files modified:** Only `docs/smackerel.md` (plus spec artifacts in `specs/024-design-doc-reconciliation/`)

## Completion Statement

Feature 024 is done. Both scopes delivered: `docs/smackerel.md` is fully reconciled with the committed codebase. All OpenClaw, SQLite, and LanceDB references outside the §4 SUPERSEDED block have been replaced. Competitive claims are honest. All 15 committed connectors are inventoried. No code files were modified.

---

## Hardening Pass (2026-04-12, harden-to-doc)

### Findings & Resolutions

| # | Severity | Finding | Resolution |
|---|----------|---------|------------|
| H1 | HIGH | Connector count claimed 14 but codebase has 15 — `guesthost/` connector omitted from spec, design doc §22.7, and scopes | Added guesthost to §22.7 inventory (now 15 connectors), updated spec.md R-006/BS-004/G5/AC-5, updated scopes.md Gherkin and DoD |
| H2 | MEDIUM | state.json had 4 scopeProgress entries but scopes.md defines only 2 scopes | Reconciled state.json to match the 2 merged scopes in scopes.md |
| H3 | LOW | §19 Phase 1 table had duplicate step 1.10 and stale "WhatsApp" reference | Fixed step numbering: merged duplicate 1.10/1.11 into clean 1.10 (Connect Telegram) + 1.11 (Test) |
| H4 | LOW | DoD grep validation patterns (`grep -v "## 4\."`) don't accurately filter §4 body | Replaced with awk-based section-aware patterns in scopes.md DoD |

### Verification

```
# Connector count verification
$ find internal/connector -maxdepth 1 -mindepth 1 -type d | wc -l → 15
$ grep "Committed Connector Inventory" docs/smackerel.md → "(15 connectors)"

# OpenClaw outside §4 (awk-based)
$ awk '/^## 4\./{s=1} /^## 5\./{s=0} s{next} /OpenClaw/{print NR": "$0}' docs/smackerel.md
→ Only line 23 (TOC link to retained §4 heading)

# SQLite/LanceDB outside §4
$ awk '/^## 4\./{s=1} /^## 5\./{s=0} s{next} /SQLite|LanceDB/{print NR": "$0}' docs/smackerel.md
→ Only line 2155 (Apple Notes factual reference)

# state.json scope count matches scopes.md
$ python3 -c "import json; d=json.load(open('specs/024-design-doc-reconciliation/state.json')); print(len(d['certification']['scopeProgress']))" → 2
```

---

## Improve-Existing Pass (2026-04-15, stochastic-quality-sweep round)

### Analysis

An `improve-existing` sweep examined `docs/smackerel.md` for remaining drift between the design document and committed codebase since the original reconciliation (024) and subsequent hardening.

### Findings & Resolutions

| # | Severity | Finding | Resolution |
|---|----------|---------|------------|
| I1 | HIGH | §3.1 Passive Ingestion diagram listed `Gmail API`, `Outlook/Teams`, `Notion/Obsidian` as present alongside committed connectors — no distinction between committed and planned | Replaced with two subgraphs: "✅ Committed" (15 actual connectors) and "🔜 Planned" (Gmail SDK, Outlook, Notion/Obsidian) |
| I2 | HIGH | §3.1 Active Capture diagram listed `Slack Bot`, `Browser Extension`, `Voice Input`, `Mobile Share Sheet` as committed when only Telegram, Discord, Email Forward, and Web UI are | Added 🔜 indicator to planned channels; added `WEB_CAP[Smackerel Web UI]` as committed |
| I3 | HIGH | §23.4 Architecture tree listed 6 planned connectors (Gmail API, Google Calendar API, Outlook, Slack, Notion, Obsidian) while missing 10 committed ones (alerts, bookmarks, browser, guesthost, hospitable, keep, maps, markets, twitter, weather) | Replaced connector list with all 15 committed connectors by directory name, plus planned connectors labeled with 🔜 |
| I4 | MEDIUM | "Chi/Gin" references in §3.1, §3.2, §23.3, §23.4, §24-A — codebase only uses Chi (confirmed in `router.go`) | Changed all 5 occurrences from "Chi/Gin" or "Gin or Chi" to "Chi" |
| I5 | LOW | Phase 1 table had duplicate step: steps 1.8 and 1.10 both "Connect Telegram bot" (introduced during reconciliation) | Removed duplicate, renumbered: 1.8 → Telegram, 1.9 → CLI, 1.10 → Test |
| I6 | MEDIUM | §3.1 Mermaid node ID collision: `ALERTS` used for both Gov Alerts connector and Contextual Alerts in Surfacing Layer | Renamed connector node to `ALERTS_CONN`, preserved surfacing `ALERTS` |
| I7 | MEDIUM | §3.2 Layer Separation diagram showed Slack as committed channel alongside Telegram/Discord | Added 🔜 indicator to Slack and Voice in §3.2; reordered to show committed (Telegram, Discord, WebChat) before planned |

### Verification

```
# No Chi/Gin references remain
$ grep -c "Chi/Gin\|Gin or Chi" docs/smackerel.md → 0

# No duplicate Phase 1 steps
$ grep -c "Connect Telegram bot" docs/smackerel.md → 1 (step 1.8 only)

# Only doc file modified
$ git diff --stat -- docs/smackerel.md → 1 file changed, 67 insertions(+), 35 deletions(-)

# OpenClaw outside §4 (re-verified)
$ awk check → only TOC link (line 23)

# All unit tests green (docs-only change)
$ ./smackerel.sh test unit → all PASS
```

---

## Harden-to-Doc Pass (2026-04-21, stochastic-quality-sweep child)

### Probe Summary

Probed Gherkin scenario quality, DoD completeness, test depth, and internal consistency across all spec artifacts. Verified design doc (`docs/smackerel.md`) against all 6 Gherkin scenarios.

**Design doc verification (all clean):**
- OpenClaw outside §4: only TOC link (line 23) — correct
- SQLite/LanceDB outside §4: only Apple Notes factual ref (line 2176) — correct
- Connector count: 15 in §22.7 — correct
- Chi/Gin ambiguous refs: 0 — correct
- Phase delivery markers: Phase 1-5 all present — correct
- Competitive matrix: pre-meeting briefs 🔜, weekly 🔜, trip dossiers 🔜 — honest

**Gherkin & DoD audit:** All 6 scenarios have matching DoD items (all checked). Test plan rows align 1:1 with scenarios. No missing coverage.

### Findings & Resolutions

| # | Severity | Finding | Resolution |
|---|----------|---------|------------|
| H5 | MEDIUM | Stale "14 connectors" in 7 locations across spec artifacts — H1 hardening fix was incomplete. Affected: spec.md (lines 9, 84), scopes.md (lines 114, 123, 150), design.md (lines 397, 520). design.md connector enumeration also missing `guesthost/` | Updated all 7 references from 14→15. Added `guesthost/` to design.md connector list (now 15 items matching `internal/connector/` directory) |

### Verification

```
# Zero stale "14 connectors" references in spec artifacts (excluding historical state.json log)
$ grep -rn "14 connectors\|14 committed" specs/024-design-doc-reconciliation/ --include="*.md" → 0 hits

# design.md now lists 15 connectors including guesthost
$ grep -c "guesthost" specs/024-design-doc-reconciliation/design.md → 1

# Design doc still clean
$ awk '/^## 4\./{s=1} /^## 5\./{s=0} s{next} /OpenClaw/{print NR": "$0}' docs/smackerel.md → only line 23 (TOC)
$ awk '/^## 4\./{s=1} /^## 5\./{s=0} s{next} /SQLite|LanceDB/{print NR": "$0}' docs/smackerel.md → only Apple Notes ref
$ find internal/connector -maxdepth 1 -mindepth 1 -type d | wc -l → 15
```
