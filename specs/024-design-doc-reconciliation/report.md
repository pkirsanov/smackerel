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
- 14 committed connectors verified in §22.7 inventory table
- Each verified against `internal/connector/` directory
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

Feature 024 is done. Both scopes delivered: `docs/smackerel.md` is fully reconciled with the committed codebase. All OpenClaw, SQLite, and LanceDB references outside the §4 SUPERSEDED block have been replaced. Competitive claims are honest. All 14 committed connectors are inventoried. No code files were modified.
