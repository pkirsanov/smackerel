# Smackerel — Investor & Platform Overview

**Last updated**: 2026-05-07
**Status**: Surfaced from existing planning artifacts under [`docs/smackerel.md`](smackerel.md), [`.specify/memory/constitution.md`](../.specify/memory/constitution.md), and committed runtime scaffold. Authoritative product design lives in `docs/smackerel.md`; this file is the investor-facing roll-up.

---

## Reading Order

This file consolidates the investor-facing view. For the canonical product design:

- [`docs/smackerel.md`](smackerel.md) — Authoritative product and architecture design (Vision, 13 Design Principles, Architecture, Phased Implementation Plan §19)
- [`.specify/memory/constitution.md`](../.specify/memory/constitution.md) — Engineering constitution (10 numbered Core Principles)
- [`docs/Development.md`](Development.md) — Runtime command and configuration contract
- [`docs/Testing.md`](Testing.md) — Test taxonomy and environment isolation
- [`docs/Docker_Best_Practices.md`](Docker_Best_Practices.md) — Docker lifecycle, cleanup, freshness
- [`docs/Deployment.md`](Deployment.md) — Production deployment (TLS, reverse proxy, auth tokens)
- [`specs/`](../specs/) — Phased behavior specs

The product design (`docs/smackerel.md`) is canonical. If this file disagrees, the design doc wins.

---

## Executive Summary

Smackerel is a **passive intelligence layer for personal digital life** — a self-hosted knowledge engine that observes everything (email, video, calendar, browsing, maps, notes, purchases), processes it into refined knowledge, connects it into a living graph, and surfaces small actionable smackerels of insight when the user needs them.

Core thesis: existing tools (Notion, Obsidian, Evernote, bookmark managers) require **organizing work at the highest cognitive load moment**. They fail for ~95% of users. Smackerel inverts the model: the system observes, processes, and connects — the user's job is to live their life.

The product is **local-first** (user-controlled hardware, optional cloud LLMs), **Go-first runtime** (Python only as ML sidecar), and **Docker-first self-hosted** (single CLI, single config source).

A specific commercial boundary is the **QuantitativeFinance (QF) Companion**: Smackerel may ingest QF decision packets, preserve trust metadata, and export consent-scoped personal evidence bundles back to QF — but never executes trades, approves mandates, or gives financial advice.

---

## Phase Overview

Per [`docs/smackerel.md`](smackerel.md) §19 (Phased Implementation Plan):

| Phase | Codename | Status (per design doc) | Primary Objective |
|-------|----------|------------------------|-------------------|
| **Phase 1** | Foundation (MVP) | ✅ Delivered | Active capture + search + basic digest via Go core + Docker Compose |
| **Phase 2** | Passive Ingestion | ✅ Delivered | Background ingestion (Gmail, YouTube, Calendar) + knowledge graph |
| **Phase 3** | Intelligence | ✅ Delivered | Synthesis engine, weekly digest, pre-meeting briefs, contextual alerts |
| **Phase 4** | Expansion | ✅ Delivered | Maps timeline, browser history, trip dossier, people intelligence |
| **Phase 5** | Advanced Intelligence | ✅ Delivered | Expertise mapping, content fuel, learning paths, subscription tracking, serendipity |
| **MVP Gate** | Founding-Promise Gate (2026-06-03) | 📋 Planned (release packet: [`docs/releases/mvp/`](releases/mvp/)) | Global interruption-budget controller (M1a), calendar-triggered briefs (M1b), reminder/promise engine (M1c), wiki/graph-browse (M2), principle ratification (M3), portfolio drift cleanup (M4/M5) |
| **v1 Gate** | Personal-Productivity + Outbound-Action Gate (target date TBD) | 📋 Planned (release packet: [`docs/releases/v1/`](releases/v1/)) | Personal Productivity Sources (V1: Gmail SDK/Graph/Apple Calendar/Notes/Reminders/Messages/voice), Outbound Action capability (V2), native-mobile decision (V3), auto-generated capability map (V4), SLO alert promotion (V5), continuous drift cleanup (V6) |

**IMPORTANT STATUS NOTE**: The design document marks Phases 1, 2, 3, 4, 5 as ✅ Delivered. Phase 3 (Intelligence) was certified delivered on 2026-05-25 (`specs/004-phase3-intelligence/state.json`, status=done); synthesis, digest, briefs, and contextual alerts are implemented under `internal/intelligence/` and `internal/digest/`. The committed runtime scaffold (per `docs/smackerel.md` §20.2) confirms the foundation operational surface exists (`./smackerel.sh` commands all wired). Capability claims should be cross-validated against committed specs under `specs/` before any external claim of "delivered" — this overview defers to the design doc as the current authority.

For detailed exit criteria per phase, read [`docs/smackerel.md`](smackerel.md) §19.

---

## Phase 1: Foundation (MVP) — ✅ Delivered

**Source**: [`docs/smackerel.md`](smackerel.md) §19 Phase 1

### Goal

Active capture + search + basic digest via Go core + Docker Compose.

### Key Capabilities

- Docker Compose stack: Go core + PostgreSQL (with pgvector + pg_trgm) + NATS + Python ML sidecar
- Capture API: Telegram + Web UI active capture
- Universal processing pipeline via NATS → ML sidecar
- Semantic search via pgvector + LLM re-ranking
- Daily digest generation + Telegram delivery
- Repo CLI: `./smackerel.sh` for all runtime operations

### Exit Criteria

User can capture URLs/text from a chat channel, find them later with vague queries, and receive a useful daily digest.

---

## Phase 2: Passive Ingestion — ✅ Delivered

**Source**: [`docs/smackerel.md`](smackerel.md) §19 Phase 2

### Goal

Background ingestion from Gmail, YouTube, Calendar. Knowledge graph connections forming.

### Key Capabilities

- Gmail connector (OAuth2 + Pub/Sub)
- YouTube connector (watch history + transcripts)
- Calendar connector (CalDAV via go-webdav)
- Knowledge graph linking (per design doc §7.2 Stage 5)
- Topic lifecycle (emerging → active → hot → cooling → dormant → archived)
- Cron schedules for all connectors

### Exit Criteria

System silently ingesting emails, videos, calendar events. Topics forming. Connections linking. User can search across all source types.

---

## Phase 3: Intelligence — ✅ Delivered

**Source**: [`docs/smackerel.md`](smackerel.md) §19 Phase 3

### Goal

System generates insights the user wouldn't produce on their own.

### Key Capabilities (Delivered)

- Synthesis engine (cross-domain connection detection)
- Weekly synthesis digest
- Pre-meeting brief generation (calendar → people context → deliver)
- Contextual alerts (bill reminders, promise tracking, trip prep)
- Promise/commitment detection from emails

### Exit Criteria

Weekly synthesis surfaces genuine cross-domain insights. Pre-meeting briefs are useful. Bill reminders accurate.

### Investment Risk

Phase 3 was certified delivered on 2026-05-25 (`specs/004-phase3-intelligence/state.json`). Ongoing risk: synthesis quality is hard to measure objectively. Mitigation: the design doc requires "explainable synthesis" (constitution Principle 4) — every synthesis claim cites source artifacts. Quality regression detectable via lost source links.

---

## Phase 4: Expansion — ✅ Delivered

**Source**: [`docs/smackerel.md`](smackerel.md) §19 Phase 4

### Goal

Location intelligence, browser integration, trip assembly, people intelligence.

### Key Capabilities

- Maps timeline connector
- Browser history connector (opt-in)
- Trip dossier auto-assembly
- People intelligence (interaction frequency, relationship radar)
- Trail/route journal

### Exit Criteria

Trip dossiers auto-assemble from email + calendar + saved places. Hike/drive routes searchable. People context includes interaction patterns.

---

## Phase 5: Advanced Intelligence — ✅ Delivered

**Source**: [`docs/smackerel.md`](smackerel.md) §19 Phase 5

### Goal

Deep self-knowledge, content creation support, advanced patterns.

### Key Capabilities

- Expertise mapping
- Content creation fuel (topic → writing angles)
- Learning path assembly
- Subscription/spending tracking
- Serendipity engine (weekly archive resurface)
- Energy/productivity pattern detection
- Seasonal pattern detection (requires 6+ months of data)

---

## QF Companion Boundary (Cross-Product Surface)

**Source**: [`docs/smackerel.md`](smackerel.md) §1.6

Smackerel acts as a **companion surface** for QuantitativeFinance, not a financial-decision system. QF remains system of record for intents, scenarios, decision packets, approval state, mandates, execution attempts, calibration, and provenance.

| Boundary | Smackerel Behavior |
|----------|--------------------|
| QF packet ingestion | Treat QF packets as external authoritative artifacts, not local recommendations |
| Trust metadata | Preserve QF `CalibrationBadge`, `DataProvenanceBadge`, packet IDs, intent/scenario IDs, trace IDs, deep links without modification |
| Personal context | Export `PersonalEvidenceBundle`s with source, sensitivity, consent, provenance |
| Actions | NO trade approval, mandate change, execution, or financial advice |
| Source of truth | QF owns decisions; Smackerel owns personal memory, reminders, digest, retrieval, context assembly |

This boundary protects both products: QF's decision integrity stays unmediated; Smackerel's personal-knowledge focus stays uncluttered by financial-action liability.

---

## Risk Assessment

| Risk | Phase Most Affected | Mitigation |
|------|---------------------|------------|
| Synthesis hallucination | Phase 3 | Constitution Principle 4 (Explainable Synthesis); persist only after schema validation + source-link attachment (per Model Compensations table) |
| Local-only reliability | All phases | Constitution Principle 6 (Docker-First Self-Hosting); restart-safe stateful services; deployment doc must match committed topology |
| Connector OAuth fragility | Phase 2, 3, 4 | Connector development guide ([`docs/Connector_Development.md`](Connector_Development.md)); refresh token handling; isolated connector testing |
| Cloud LLM lock-in | All phases | Constitution Principle 1 (Local-First); Ollama as primary; cloud LLMs are optional helpers, not core dependency |
| Python sprawl into core | All phases | Constitution Principle 2 (Go-First Runtime, Python-Only ML Sidecar); enforced by review |
| QF decision contamination | QF Companion only | Strict §1.6 boundary; no trade actions in companion connector |
| User data exfiltration | All phases | Constitution Business Invariant: user data remains local by default; remote services optional |

---

## Capital Requirements Summary

Smackerel is a **local-first, self-hosted** product. Capital exposure is dominated by:

| Cost Category | Profile |
|--------------|---------|
| Per-user infrastructure | Near-zero (runs on user's own hardware via Docker Compose) |
| LLM inference | Variable: Ollama local (zero marginal cost) OR optional cloud LLM (per-user opt-in) |
| Connector OAuth | Per-API-provider OAuth app fees (Gmail, YouTube, etc.) — fixed annual cost |
| Engineering effort | Phases 1–5 delivered; ongoing investment is connector breadth, synthesis-quality tuning, and operational hardening |
| Distribution | Self-hosted reduces cloud-hosting capital requirement; documentation + onboarding effort scales |

There is no SaaS hosting cost model in the constitution. Pricing/monetization is intentionally not surfaced in the current design doc — that is a planning artifact yet to be created.

---

## Strategic Recommendations

These recommendations are surfaced from existing repo evidence. They are NOT new investment proposals.

1. **Defend synthesis quality as the primary differentiator.** Phases 1–5 are marked Delivered (Phase 3 certified 2026-05-25); the synthesis engine, pre-meeting briefs, and contextual alerts are the durable differentiator and require continued quality investment (explainability, source-link integrity, regression detection).
2. **Defend Local-First as a brand commitment.** The market will pressure cloud-hosting and per-user pricing. Constitution Principle 1 is a moat, not a constraint.
3. **Hold Python at the ML-sidecar boundary.** Constitution Principle 2 prevents Python sprawl into the orchestrator. Sliding this boundary erodes the Go-first operational guarantee.
4. **QF Companion is bounded by §1.6 — do NOT extend beyond it.** Trade approval, mandate changes, execution, and financial advice MUST NOT enter Smackerel. Cross-product trust depends on this boundary.
5. **Source-qualified processing is a quality multiplier.** Design Principle 5 ("Use metadata from source systems") is the primary lever for retrieval quality. Invest in connector metadata fidelity, not just content extraction.
6. **Surface lifecycle (promote/decay) is the freshness contract.** Design Principle 4 ("Knowledge breathes") prevents the product from becoming a stale dump like Notion/Obsidian. This is the long-term retention lever.

---

## What's Actually Working Today

**Source**: [`docs/smackerel.md`](smackerel.md) §20.2 (Current Repo State)

- ✅ Foundation runtime scaffold committed (Go core + PostgreSQL + NATS + Python ML sidecar via Docker Compose)
- ✅ Repo CLI (`./smackerel.sh`) operational: config generate, build, check, lint, format, test (unit/integration/e2e/stress), up/down/status/logs, clean smart/full/status/measure
- ✅ Bubbles framework governance committed (`bash .github/bubbles/scripts/cli.sh doctor`, framework-validate, artifact-lint, traceability-guard)
- ✅ Phases 1, 2, 3, 4, 5 marked delivered in design doc (cross-validate against `specs/` before external claims)
- ✅ Phase 3 (Intelligence) certified delivered 2026-05-25 (`specs/004-phase3-intelligence/state.json`)
- ⚠️ "Capability claims should be cross-validated against committed specs under `specs/` before any external claim of 'delivered'" (per design doc §20.2)

---

## Documentation Map

| Document | Purpose |
|----------|---------|
| [`docs/smackerel.md`](smackerel.md) | Authoritative product and architecture design (canonical) |
| [`docs/Development.md`](Development.md) | Runtime command and configuration contract |
| [`docs/Testing.md`](Testing.md) | Test taxonomy and environment isolation |
| [`docs/Docker_Best_Practices.md`](Docker_Best_Practices.md) | Docker lifecycle, cleanup, freshness |
| [`docs/Deployment.md`](Deployment.md) | Production deployment (TLS, reverse proxy, auth) |
| [`docs/Connector_Development.md`](Connector_Development.md) | Connector authoring guide |
| [`docs/Operations.md`](Operations.md) | Operational runbook |
| [`.specify/memory/constitution.md`](../.specify/memory/constitution.md) | Engineering principles (NON-NEGOTIABLE) |
| [`docs/Product-Principles.md`](Product-Principles.md) | Product principles (surfaced for owner approval — not yet ratified) |

---

## Legend

| Symbol | Meaning |
|--------|---------|
| ✅ Delivered | Per design doc §19 status — cross-validate against `specs/` before external claims |
| 🔜 In Progress | Active investment phase (no phases currently in this state) |
| TBD | Investor-facing summary intentionally defers detail to canonical design doc |
| **Phased ✅/🔜 statuses** | Authoritative source is `docs/smackerel.md` §19 — that document wins on conflict |

---

## Appendix: Spec Index

This appendix surfaces every committed spec under [`specs/`](../specs/) for investor traceability. Each row links a spec ID to its capability name (from the spec's H1) and current lifecycle status (from its `state.json`). Grouping mirrors the phase narrative above; all rows below are `done` (delivered and certified per Bubbles governance).

### Phase Specs (Foundation)

| Spec | Capability | Status |
|------|------------|--------|
| [001](../specs/001-smackerel-mvp/) | Smackerel MVP | done |
| [002](../specs/002-phase1-foundation/) | Phase 1: Foundation (Active Capture + Search + Digest) | done |
| [003](../specs/003-phase2-ingestion/) | Phase 2: Passive Ingestion (Gmail + YouTube + Calendar + Topic Lifecycle) | done |
| [004](../specs/004-phase3-intelligence/) | Phase 3: Intelligence (Synthesis + Alerts + Pre-Meeting Briefs) | done |
| [005](../specs/005-phase4-expansion/) | Phase 4: Expansion (Maps + Browser + Trips + People Intelligence) | done |
| [006](../specs/006-phase5-advanced/) | Phase 5: Advanced Intelligence (Expertise + Learning + Serendipity) | done |

### Connectors

| Spec | Capability | Status |
|------|------------|--------|
| [007](../specs/007-google-keep-connector/) | Google Keep Connector | done |
| [008](../specs/008-telegram-share-capture/) | Telegram Share & Chat Capture | done |
| [009](../specs/009-bookmarks-connector/) | Bookmarks Connector | done |
| [010](../specs/010-browser-history-connector/) | Browser History Connector | done |
| [011](../specs/011-maps-connector/) | Google Maps Timeline Connector | done |
| [012](../specs/012-hospitable-connector/) | Hospitable Connector | done |
| [013](../specs/013-guesthost-connector/) | GuestHost Connector & Hospitality Intelligence | done |
| [014](../specs/014-discord-connector/) | Discord Connector | done |
| [015](../specs/015-twitter-connector/) | Twitter/X Connector | done |
| [016](../specs/016-weather-connector/) | Weather Connector | done |
| [017](../specs/017-gov-alerts-connector/) | Government Alerts Connector | done |
| [018](../specs/018-financial-markets-connector/) | Financial Markets Connector | done |
| [019](../specs/019-connector-wiring/) | Connector Wiring — Register 5 Unwired Connectors | done |
| [040](../specs/040-cloud-photo-libraries/) | Cloud Photo Libraries | done |
| [041](../specs/041-qf-companion-connector/) | QF Companion Connector | done |
| [056](../specs/056-twitter-api-connector/) | Twitter API Connector | done |

### Security & Auth

| Spec | Capability | Status |
|------|------------|--------|
| [020](../specs/020-security-hardening/) | Security Hardening — Docker Binding, Auth Enforcement, Crypto Hygiene | done |
| [044](../specs/044-per-user-bearer-auth/) | Per-User Bearer Auth Foundation | done |
| [051](../specs/051-deployment-secret-auth-contract/) | Deployment Secret and Auth Contract | done |
| [052](../specs/052-bundle-secret-injection-contract/) | Bundle Secret Injection Contract | done |

### Intelligence & Synthesis

| Spec | Capability | Status |
|------|------------|--------|
| [021](../specs/021-intelligence-delivery/) | Intelligence Delivery | done |
| [025](../specs/025-knowledge-synthesis-layer/) | Knowledge Synthesis Layer (LLM Wiki Pattern) | done |
| [026](../specs/026-domain-extraction/) | Domain-Aware Structured Extraction | done |
| [039](../specs/039-recommendations-engine/) | Recommendations Engine | done |
| [054](../specs/054-notification-intelligence-handler/) | Notification Intelligence Handler Service | done |
| [055](../specs/055-notification-source-ntfy-adapter/) | Notification Source ntfy Adapter | done |

### DevOps & Operations

| Spec | Capability | Status |
|------|------------|--------|
| [022](../specs/022-operational-resilience/) | Operational Resilience | done |
| [029](../specs/029-devops-pipeline/) | DevOps Pipeline & Image Governance | done |
| [030](../specs/030-observability/) | Observability: Metrics & Tracing | done |
| [045](../specs/045-deploy-resource-filesystem-hardening/) | Deploy Resource and Filesystem Hardening | done |
| [046](../specs/046-nats-production-hardening/) | NATS Production Hardening | done |
| [047](../specs/047-ci-image-vulnerability-gate/) | CI Image Vulnerability Gate | done |
| [049](../specs/049-monitoring-stack/) | Monitoring Stack | done |
| [050](../specs/050-ml-sidecar-health-isolation/) | ML Sidecar Health Isolation | done |
| [053](../specs/053-ci-ops-evidence-hardening/) | CI Ops Evidence Hardening | done |

### Quality & Engineering

| Spec | Capability | Status |
|------|------------|--------|
| [023](../specs/023-engineering-quality/) | Engineering Quality | done |
| [024](../specs/024-design-doc-reconciliation/) | Design Document Reconciliation | done |
| [031](../specs/031-live-stack-testing/) | Live-Stack Integration & E2E Testing | done |
| [032](../specs/032-documentation-freshness/) | Documentation Freshness & Operational Guides | done |
| [037](../specs/037-llm-agent-tools/) | LLM Scenario Agent & Tool Registry | done |

### User Features

| Spec | Capability | Status |
|------|------------|--------|
| [027](../specs/027-user-annotations/) | User Annotations & Interaction Tracking | done |
| [028](../specs/028-actionable-lists/) | Actionable Lists & Resource Tracking | done |
| [033](../specs/033-mobile-capture/) | Mobile & Browser Capture Surfaces | done |
| [034](../specs/034-expense-tracking/) | Expense Tracking | done |
| [035](../specs/035-recipe-enhancements/) | Recipe Enhancements — Serving Scaler & Cook Mode | done |
| [036](../specs/036-meal-planning/) | Meal Planning Calendar | done |

### Infrastructure & Deployment

| Spec | Capability | Status |
|------|------------|--------|
| [038](../specs/038-cloud-drives-integration/) | Cloud Drives Integration | done |
| [042](../specs/042-tailnet-edge-bind-pattern/) | Tailnet-Edge Bind Pattern (Home-Lab Compose Readiness) | done |
| [043](../specs/043-ollama-test-infrastructure/) | Ollama Test Infrastructure | done |
| [048](../specs/048-backup-restore-automation/) | Backup and Restore Automation | done |
