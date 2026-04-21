# Feature: 024 Design Document Reconciliation

## Problem Statement

Smackerel's design document (`docs/smackerel.md` v2) has significant drift from the actual implementation. Three categories of inaccuracy undermine trust in the design doc as a reference:

1. **Fictional runtime platform (Critical):** §4 describes ~2000 words of OpenClaw integration — workspace structure, multi-agent architecture, node capabilities, cron/webhook config — none of which exists. The actual runtime is standalone Go + Docker Compose.
2. **Wrong storage layer (High):** §8 and §14 describe SQLite + LanceDB. The actual implementation uses PostgreSQL + pgvector (10 committed migrations under `internal/db/migrations/`).
3. **Inflated competitive claims (Medium):** §21.3 marks features as implemented (✅) that are aspirational — weekly synthesis, pre-meeting briefs, and connector counts that don't match the 15 committed connectors.

The design doc is the primary architecture reference for the project. Inaccurate content causes confusion for contributors and creates a false picture of system capabilities.

## Outcome Contract

**Intent:** Reconcile `docs/smackerel.md` so every section accurately reflects the committed codebase, with aspirational content clearly marked as future/planned rather than present.

**Success Signal:** A contributor reading any section of the design doc can trust that described components, storage layers, APIs, and capabilities match what exists in the repository — or are explicitly labeled as planned.

**Hard Constraints:**
- No code changes; this is a docs-only reconciliation
- Preserve the design document's vision and aspirational content — just label it honestly
- §3 (System Architecture) mermaid diagrams that already reflect the Go/Docker stack must be preserved as-is
- All 15 committed connectors must be accurately represented

**Failure Condition:** A reader encounters a described capability (e.g., OpenClaw workspace, SQLite storage, weekly synthesis) and believes it is implemented when it is not.

## Goals

- G1: Remove or clearly mark all OpenClaw references as historical/superseded
- G2: Update storage references from SQLite + LanceDB to PostgreSQL + pgvector
- G3: Update data model schemas to reflect actual PostgreSQL table structure
- G4: Mark aspirational competitive claims honestly in the differentiation matrix
- G5: Verify connector references match the 15 implemented connectors
- G6: Update the phased implementation plan to reflect actual delivery state
- G7: Ensure §3 system architecture is accurate (currently correct — verify only)

## Non-Goals

- Rewriting the entire design document
- Adding new design content or architecture proposals
- Code changes to any runtime, test, or config files
- Updating other docs (Development.md, Testing.md, Docker_Best_Practices.md)
- Redesigning the connector ecosystem section (§22)

## Actors & Personas

| Actor | Description | Key Goals | Permissions |
|-------|-------------|-----------|-------------|
| Contributor | Developer reading the design doc to understand system architecture | Accurate understanding of what exists vs. what is planned | Read access to all docs |
| Project Owner | Author of the design doc, decides what stays vs. what is removed | Maintain vision integrity while ensuring accuracy | Full edit access |
| Reviewer | Anyone auditing project claims for accuracy | Verify that documented capabilities match reality | Read access |

## Use Cases

### UC-001: Contributor Reads Architecture Overview
- **Actor:** Contributor
- **Preconditions:** Design doc exists at `docs/smackerel.md`
- **Main Flow:**
  1. Contributor opens §3 (System Architecture) and §4 (OpenClaw Integration)
  2. §3 accurately shows Go core + Python sidecar + PostgreSQL + pgvector + NATS + Docker
  3. §4 is clearly marked as superseded/historical or removed
  4. Contributor understands the actual runtime platform
- **Alternative Flows:**
  - §4 content is retained but labeled "Original Design — Superseded by standalone Go/Docker runtime"
- **Postconditions:** No confusion about runtime platform

### UC-002: Contributor Reads Storage Layer
- **Actor:** Contributor
- **Preconditions:** Design doc contains §8 and §14
- **Main Flow:**
  1. Contributor opens §8 (Knowledge Graph & Storage)
  2. Storage diagrams show PostgreSQL + pgvector, not SQLite + LanceDB
  3. §14 data models use PostgreSQL DDL (JSONB, TIMESTAMPTZ, vector(384))
  4. Contributor understands the actual data layer
- **Postconditions:** Storage references match `internal/db/migrations/`

### UC-003: Reviewer Audits Competitive Claims
- **Actor:** Reviewer
- **Preconditions:** §21.3 competitive matrix exists
- **Main Flow:**
  1. Reviewer reads differentiation matrix
  2. Each ✅ claim corresponds to implemented, running functionality
  3. Planned/aspirational features are marked with a distinct indicator (e.g., 🔜 or "Planned")
  4. Connector counts match the 15 committed connectors
- **Postconditions:** No false capability claims

### UC-004: Contributor Reads Phased Plan
- **Actor:** Contributor
- **Preconditions:** §19 describes 5 implementation phases
- **Main Flow:**
  1. Contributor reads §19
  2. Phase descriptions reflect actual delivery — no references to OpenClaw workspace setup, SQLite/LanceDB setup
  3. Completed phases are marked as such
  4. Current phase is identified
- **Postconditions:** Implementation plan matches repository state

## Business Scenarios

### BS-001: OpenClaw References Cause Confusion
Given the design doc describes OpenClaw as the runtime platform in §4
When a new contributor reads the document to understand how to develop locally
Then they find no OpenClaw workspace, no AGENTS.md, no SOUL.md, no skills/ directory
And they lose trust in the entire document

### BS-002: Storage Layer Mismatch
Given the design doc describes SQLite + LanceDB in §8 and §14
When a contributor tries to understand the data model
Then they find PostgreSQL with pgvector extensions, JSONB columns, and TIMESTAMPTZ types
And the documented SQL schemas don't match the actual migrations

### BS-003: Inflated Competitive Claims
Given the differentiation matrix marks weekly synthesis and pre-meeting briefs as ✅
When a reviewer audits what the system actually does
Then they find these features are not yet implemented
And the competitive comparison becomes unreliable

### BS-004: Connector Count Inflation
Given the design doc might reference more connectors than exist
When a reviewer counts implemented connectors
Then they find exactly 15: alerts, bookmarks, browser, caldav, discord, guesthost, hospitable, imap, keep, maps, markets, rss, twitter, weather, youtube
And any claimed connector not in this list is aspirational

### BS-005: Phased Plan References Superseded Technology
Given §19 references "OpenClaw workspace setup" and "SQLite + LanceDB setup" as Phase 1 tasks
When a contributor reads the plan to understand project history
Then the plan should reflect what was actually built (Go/Docker, PostgreSQL+pgvector)

## Requirements

### R-PRD-002: OpenClaw Reconciliation (Critical)
- §4 (OpenClaw Integration Strategy) must be either:
  - (a) Removed entirely, or
  - (b) Retained with a prominent "SUPERSEDED" header explaining the actual runtime is standalone Go + Docker Compose
- §4.1 through §4.5 subsections (Why OpenClaw, Workspace Structure, Configuration, Multi-Agent Architecture, Node Capabilities) must all be addressed
- The header metadata line `> **Runtime Platform:** OpenClaw` (line 14) must be updated
- Design principle references to OpenClaw in §2 (principles 9, 10) must be updated
- §6 references to "OpenClaw-connected channel" and "OpenClaw app" must be updated
- All scattered OpenClaw references in §7 (browser control), §16 (scenarios), §17 (security), §18 (privacy) must be updated

### R-PRD-006: Storage Layer Update (High)
- §8.1 Storage Architecture diagram must replace SQLite with PostgreSQL and LanceDB with pgvector
- §8.2 Knowledge Graph Model can stay (it's conceptual)
- §14.1 through §14.6 table DDL must be updated to PostgreSQL syntax:
  - `TEXT` → `TEXT` (same)
  - `INTEGER` → `INTEGER` or `BOOLEAN` as appropriate
  - JSON columns → `JSONB`
  - Datetime → `TIMESTAMPTZ`
  - Add `embedding vector(384)` to artifacts table
  - Use `CREATE EXTENSION IF NOT EXISTS vector` and `pg_trgm`
- §17 and §18 references to "SQLite + LanceDB" data-at-rest must be updated
- §19 Phase 1 tasks referencing SQLite/LanceDB setup must be updated
- §24 (Appendix) / technology stack references must be updated

### R-PRD-011: Competitive Matrix Audit (Medium)
- §21.3 must distinguish implemented vs. planned features
- Features to audit:
  - "Cross-domain synthesis" — implementation exists in `internal/intelligence/` but verify completeness
  - "Daily/weekly digest" — daily digest exists in `internal/digest/`; weekly synthesis status unclear
  - "Pre-meeting briefs" — verify implementation status
  - "Topic lifecycle (hot/cooling/dormant)" — verify implementation in `internal/topics/`
  - "Location/travel intelligence" — maps connector exists but full travel intelligence unclear
  - "Multi-channel delivery" — Telegram exists; verify others
- Connector ecosystem references (§22) must match the 15 committed connectors

### R-004: System Architecture Verification
- §3 mermaid diagrams already show Go core + Python sidecar + PostgreSQL + pgvector + NATS
- Verify no OpenClaw components appear in §3 diagrams
- If accurate, no changes needed to §3

### R-005: Phased Implementation Plan Update
- §19 Gantt chart and phase tables must reflect actual technology choices
- Phase 1 tasks should reference PostgreSQL + pgvector, not SQLite + LanceDB
- Phase 1 tasks should reference Docker Compose setup, not OpenClaw workspace
- Mark completed phases as delivered

### R-006: Connector Reference Accuracy
- All connector lists in the document must account for the 15 implemented connectors:
  1. alerts (gov alerts)
  2. bookmarks
  3. browser (browser history)
  4. caldav (calendar)
  5. discord
  6. guesthost (property management)
  7. hospitable
  8. imap (email)
  9. keep (Google Keep)
  10. maps (Google Maps)
  11. markets (financial markets)
  12. rss (podcast/feed)
  13. twitter
  14. weather
  15. youtube
- §22 (Connector Ecosystem) references to Gmail, Outlook connectors should note actual implementation is IMAP-based
- Connectors mentioned in design but not implemented (e.g., Notion, Photos, SMS) must be marked as planned/future

## User Scenarios (Gherkin)

```gherkin
Scenario: OpenClaw references are reconciled
  Given the design doc section §4 describes OpenClaw integration
  When the reconciliation is applied
  Then §4 is either removed or clearly marked as superseded
  And the header metadata shows the actual runtime platform
  And no unmarked OpenClaw references remain in any section

Scenario: Storage references match implementation
  Given §8 and §14 describe SQLite + LanceDB
  When the reconciliation is applied
  Then all storage diagrams and DDL reference PostgreSQL + pgvector
  And data model schemas use PostgreSQL types (JSONB, TIMESTAMPTZ, vector)
  And no unmarked SQLite or LanceDB references remain

Scenario: Competitive matrix is honest
  Given §21.3 marks features as implemented with ✅
  When the reconciliation is applied
  Then only actually implemented features retain ✅
  And aspirational features use a distinct planned indicator
  And connector counts match the 15 committed connectors

Scenario: Phased plan reflects reality
  Given §19 references OpenClaw setup and SQLite/LanceDB
  When the reconciliation is applied
  Then Phase 1 references Docker Compose and PostgreSQL + pgvector
  And completed phases are marked as delivered

Scenario: System architecture section is verified
  Given §3 contains mermaid diagrams
  When the reconciliation is verified
  Then §3 diagrams correctly show Go core, Python sidecar, PostgreSQL + pgvector, NATS JetStream
  And no OpenClaw components appear in §3
```

## Acceptance Criteria

- AC-1: Zero unmarked/unlabeled OpenClaw references remain in the design doc
- AC-2: All storage references in §8, §14, §17, §18, §19 reflect PostgreSQL + pgvector
- AC-3: §14 DDL matches PostgreSQL syntax consistent with `internal/db/migrations/`
- AC-4: §21.3 competitive matrix distinguishes implemented vs. planned features
- AC-5: All connector references match the 15 committed connectors
- AC-6: §19 phased plan reflects actual technology and delivery state
- AC-7: §3 system architecture diagrams remain accurate (no regressions introduced)
- AC-8: No code files are modified — docs-only changes

## Competitive Analysis

Not applicable — this is an internal documentation reconciliation feature.

## Improvement Proposals

### IP-001: Add Implementation Status Markers
- **Impact:** High
- **Effort:** S
- **Competitive Advantage:** Builds contributor trust in documentation accuracy
- **Actors Affected:** All contributors and reviewers
- **Business Scenarios:** BS-003, BS-004
- Introduce a consistent notation throughout the design doc: ✅ Implemented, 🔜 Planned, ❌ Removed

### IP-002: Add "Document vs. Reality" Changelog
- **Impact:** Medium
- **Effort:** S
- **Competitive Advantage:** Makes drift visible and trackable
- **Actors Affected:** Project Owner, Contributors
- **Business Scenarios:** BS-001, BS-002
- Add a reconciliation changelog section at the top of the design doc listing what changed and why

## UI Scenario Matrix

Not applicable — this is a documentation-only feature with no UI changes.

## Non-Functional Requirements

- **Accuracy:** Every factual claim in the design doc must be verifiable against the committed codebase
- **Readability:** Superseded content must be clearly distinguishable from current content without cluttering the document
- **Completeness:** All sections containing drift must be addressed — no partial reconciliation
