# Feature: 041 — QF Companion Connector

> **Author:** bubbles.analyst  
> **Date:** May 2, 2026  
> **Status:** Draft (analyst-owned requirements sections)  
> **Related QF Feature:** `quantitativeFinance/specs/063-smackerel-companion-bridge`

## Problem Statement

Smackerel already captures personal context, financial-market artifacts, cross-source connections, semantic search results, daily digests, and multi-channel delivery. QuantitativeFinance is evolving toward a canonical decision system where every meaningful output flows through `Intent`, `Scenario`, `DecisionPacket`, trust badges, approval state, and later mandates/outcomes.

Without a QF companion connector, Smackerel cannot surface QF decisions in the user's daily context, and QF cannot use Smackerel's personal knowledge graph as evidence without ad-hoc copying. The integration should be built now while Smackerel is mostly complete, but it must preserve the boundary that QF is the decision authority and Smackerel is the companion/context surface.

## Current Capability Map

| Capability | Existing Evidence | Current Status | Requirement Impact |
|------------|-------------------|----------------|--------------------|
| Connector lifecycle | `internal/connector/connector.go`, supervisor, cursor state, health states. | Present | QF connector should be a normal passive connector. |
| Financial markets connector | `internal/connector/markets` pulls quotes, crypto, forex, FRED, company news, and daily summaries. | Present | QF companion can link QF packets to Smackerel market/news context. |
| Prompt contracts | `config/prompt_contracts` includes digest, query augment, cross-source connection, recommendation watches, and feedback. | Present | QF rendering/evidence bundle generation should use prompt contracts where reasoning is needed. |
| Digest/search/web/Telegram surfaces | Runtime docs list daily digest, Web UI, semantic search, Telegram bot, PWA share target. | Present | QF packets can surface through existing channels. |
| Approval/action safety | Recommendation watch/feedback scenarios have policy and side-effect classes. | Present | QF actions must remain disabled or clearly side-effect scoped until QF allows them. |

## Outcome Contract

**Intent:** Add a QF companion connector that ingests QF decision events, renders QF packets read-only with trust metadata intact, and lets users export Smackerel personal context back to QF as consent-scoped evidence bundles.

**Success Signal:** A user configures the QF connector, syncs at least one QF decision packet, sees it in Smackerel Web/Telegram/digest with QF trace and trust badges preserved, opens the authoritative QF deep link, and exports a personal evidence bundle for a symbol/topic research trail back to QF.

**Hard Constraints:** Smackerel MUST NOT generate financial advice, buy/sell recommendations, approval state, calibration badges, data-provenance badges, or execution actions for QF. QF is the system of record. Pre-MVP connector behavior is read-only for decisions.

**Failure Condition:** The feature fails if Smackerel invents or edits QF trust metadata, treats a QF packet as a local recommendation, allows approval/execution before QF supports it, loses trace IDs, or exports personal context without source artifact references and sensitivity/consent metadata.

## Goals

1. Add a `qf-decisions` connector that syncs QF decision events by cursor.
2. Normalize QF packets into Smackerel artifacts without flattening trust metadata.
3. Render read-only QF packet cards in Web, digest, and Telegram-compatible channels.
4. Generate `PersonalEvidenceBundle`s from Smackerel artifacts, concepts, entities, market/news context, and cross-source connections.
5. Keep QF approval/execution actions disabled in pre-MVP with clear phase-boundary copy.
6. Define later release upgrades for approval, standing watches, tenant-aware access, and voice/kill-switch parity.

## Non-Goals

- No Smackerel-generated financial recommendations.
- No portfolio tracking inside Smackerel.
- No trade execution or broker integration.
- No QF tenant/client companion behavior before QF v2.0.
- No voice or EmergencyStop action before QF supports attested parity.
- No direct database connection into QF.
- No Kafka/Redpanda client in Smackerel for pre-MVP.

## Functional Requirements

### FR-001: QF Connector Configuration
Smackerel MUST support a `qf-decisions` connector configuration with QF base URL, credential reference, enabled flag, sync schedule, and connector-specific source config through `config/smackerel.yaml` and generated env.

### FR-002: Cursor-Based Sync
The connector MUST fetch QF decision events incrementally and store cursor state through the existing connector supervisor/state store.

### FR-003: QF Artifact Types
Smackerel MUST normalize QF objects into content types such as `qf/decision-packet`, `qf/no-action-decision`, `qf/policy-denial`, and reserved future `qf/approval-request`.

### FR-004: Trust Metadata Preservation
Smackerel MUST preserve QF-provided `CalibrationBadge`, `DataProvenanceBadge`, trace ID, packet ID, intent ID, scenario ID, and deep link.

### FR-005: Read-Only Rendering
Smackerel MUST render QF packets as read-only companion cards in Web and digest surfaces, with Telegram-compatible summary formatting where channel limits allow.

### FR-006: No Local Financial Advice
Smackerel MUST NOT produce buy/sell/hold recommendations for QF packets. Any recommendation wording must originate from QF packet content.

### FR-007: PersonalEvidenceBundle Generation
Smackerel MUST generate a `PersonalEvidenceBundle` from selected artifacts or concept/entity context, including source artifact IDs, related symbols/entities, extracted claims, confidence, sensitivity tier, consent scope, and generated timestamp.

### FR-008: Evidence Export To QF
Smackerel MUST export evidence bundles to QF through the QF bridge API or private-alpha import path when configured.

### FR-009: Disabled Action Boundary
Approval, execution, mandate changes, and EmergencyStop actions MUST be disabled in pre-MVP unless QF exposes an explicit supported endpoint for that action class.

### FR-010: Connector Health
Smackerel MUST surface QF connector health, auth failures, schema/version mismatches, cursor lag, and packet validation failures.

### FR-011: Digest Integration
Smackerel's daily digest MAY include QF packets when they are high priority, but MUST preserve QF trust labels and MUST respect Smackerel quiet/sensitivity policies.

### FR-012: Search Integration
QF packets MUST be searchable by symbol, thesis, source entities, trace ID, packet ID, and related Smackerel concepts without exposing hidden sensitive evidence.

## Actors & Personas

| Actor | Description | Key Goals | Permissions |
|-------|-------------|-----------|-------------|
| Smackerel User / QF User | Same person running both systems. | See QF decisions in daily context; export personal context into QF. | Configure connector, view packets, export evidence. |
| QF Connector | Passive Smackerel connector. | Sync QF packet events reliably. | Read-only access to QF bridge endpoint. |
| Personal Evidence Curator | User selecting context for QF. | Bundle relevant research trail without dumping raw private data. | Select artifacts/concepts and consent scope. |
| Future Advisor/Operator | Later QF v2 persona. | Use Smackerel as client/family-office companion. | Deferred until tenant-aware QF bridge. |

## Business Scenarios

### BS-001: QF Packet In Digest
Given QF exposes a decision packet for the user  
When the QF connector syncs  
Then Smackerel includes the packet in search/digest with QF trust metadata intact.

### BS-002: Evidence Bundle Export
Given a user has email, bookmark, market/news, and notes context around a symbol  
When the user exports context to QF  
Then Smackerel creates a `PersonalEvidenceBundle` with source IDs, claims, symbols, sensitivity, confidence, and consent scope.

### BS-003: Action Boundary
Given a QF packet appears in Smackerel pre-MVP  
When the user tries to approve or execute from Smackerel  
Then Smackerel shows that actions must be completed in QF until the relevant release enables companion actions.

### BS-004: Schema Mismatch
Given QF sends a packet version Smackerel does not understand  
When the connector syncs  
Then Smackerel stores diagnostic metadata, marks connector degraded, and does not render an actionable card.

## UI Scenario Matrix

| Scenario | Actor | Entry Point | Steps | Expected Outcome | Screen(s) |
|----------|-------|-------------|-------|------------------|-----------|
| View QF packet | User | Digest/search/Telegram | Sync -> open card -> inspect badges -> open QF link | Same trace and trust metadata visible | Digest, Web, Telegram |
| Build evidence bundle | User | Search/concept/entity page | Select context -> generate bundle -> export to QF | QF receives consent-scoped evidence | Web knowledge/search |
| Connector health issue | User/operator | Settings connectors | Open QF connector status | Auth/schema/cursor issue visible | Settings, status |

## Release Mapping

| Capability | Pre-MVP | MVP | v1.0 | v2.0 | v3.0 |
|------------|---------|-----|------|------|------|
| `qf-decisions` connector | Implement private-alpha read-only | Harden under QF auth/entitlements | Carry as delivery channel | Tenant/service-account scoped | Certified external surface |
| QF packet cards | Read-only | Read-only + limited QF-supported actions | Full approval feedback for paper workflows | Policy-scoped advisor/client mode | Full parity where allowed |
| Evidence bundles | Manual/export path | Attach to guided/Rhai workflows | Committee grounding | Retention/consent/audit policy | Universal asset context |
| Standing watches | Design/reserved only | Limited alert surfacing | Mandate/watch evaluation | AttentionBudget scoped | Full omni-channel watches |
| Voice/EmergencyStop | Not supported | Not supported | Paper/status only if QF supports | Tenant-scoped parity prep | Attested voice/mobile parity |

## Non-Functional Requirements

- **Privacy:** Evidence bundles must include sensitivity and consent metadata.
- **Security:** QF credentials must come from Smackerel config/env generation, never hardcoded.
- **Reliability:** Connector failure must not mutate existing QF packet artifacts.
- **Auditability:** Trace ID, packet ID, and source artifact IDs must be preserved.
- **UX clarity:** Smackerel must clearly distinguish QF-authored decisions from Smackerel-local recommendations.

## UI Wireframes

### Screen Inventory

| Screen | Actor(s) | Status | Scenarios Served |
|--------|----------|--------|------------------|
| QF Connector Status | Smackerel User / QF User, QF Connector | Modify existing `/settings` and `/status` | BS-004 |
| QF Packet Search And Digest Card | Smackerel User / QF User | Modify existing search, digest, and result cards | BS-001, BS-003, BS-004 |
| QF Packet Artifact Detail | Smackerel User / QF User | Modify existing `/artifact/{id}` detail | BS-001, BS-003 |
| Personal Evidence Bundle Builder | Personal Evidence Curator | New flow from search/concept/entity/artifact pages | BS-002 |
| Telegram QF Packet Summary | Smackerel User / QF User | Modify existing Telegram/digest formatting | BS-001, BS-003 |

### Screen: QF Connector Status

**Actor:** Smackerel User / QF User | **Route:** `/settings` and `/status` | **Status:** Modify

```text
┌──────────────────────────────────────────────────────────────┐
│ Search  Digest  Topics  Knowledge  Settings  Status  [Theme] │
├──────────────────────────────────────────────────────────────┤
│ Settings / Connectors                                        │
│                                                              │
│ ┌──────────────────────────────────────────────────────────┐ │
│ │ QF Decisions                                             │ │
│ │ Status: [healthy/degraded/error]     Connector: qf-decisions │
│ │ Last sync: [timestamp]               Cursor lag: [count/time]│
│ │ Contract: [packet_version]           QF schema: [compatible] │
│ │ Required metadata: packet trace badges approval deep link     │
│ │                                                          │ │
│ │ Health Details                                           │ │
│ │ [auth ok] [schema ok] [cursor ok] [validation failures n]│ │
│ │                                                          │ │
│ │ [Sync Now] [View diagnostics] [Disable connector]        │ │
│ └──────────────────────────────────────────────────────────┘ │
│                                                              │
│ ┌──────────────────────────────────────────────────────────┐ │
│ │ Diagnostics                                              │ │
│ │ Time | Event | Packet ID | Trace ID | Reason             │ │
│ │ [row]| schema mismatch | [id] | [trace] | degraded       │ │
│ └──────────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────────┘
```

**Interactions:**
- `[Sync Now]` triggers the existing connector sync pattern for `qf-decisions` and updates status without exposing QF credential material.
- `[View diagnostics]` expands per-packet validation failures, schema/version mismatch, cursor lag, and auth errors.
- `[Disable connector]` routes through the existing config-owned enable/disable workflow and never deletes synced artifacts.

**States:**
- Empty state: connector not configured; show required config keys and no QF packet previews.
- Loading state: connector row shows syncing state and disables duplicate sync triggers.
- Error state: auth, missing config, incompatible schema, or QF read-surface outage appears as connector error with no packet rendering.
- Degraded state: missing packet IDs, trace IDs, or trust badges marks connector degraded and packet cards non-actionable.

**Responsive:**
- Mobile: connector facts collapse into stacked rows; diagnostics table becomes cards keyed by packet ID.
- Tablet: diagnostics panel sits below the connector health card.
- Desktop: status and diagnostics use a two-card vertical layout inside the existing Settings page width.

**Accessibility:**
- Health is conveyed with text and status icons, never color alone.
- Sync result messages use `aria-live` status text.
- Diagnostic rows are keyboard-expandable and identify packet ID, trace ID, and failure reason.

### Screen: QF Packet Search And Digest Card

**Actor:** Smackerel User / QF User | **Route:** `/`, `/digest` | **Status:** Modify

```text
┌──────────────────────────────────────────────────────────────┐
│ Search  Digest  Topics  Knowledge  Settings  Status  [Theme] │
├──────────────────────────────────────────────────────────────┤
│ Search your knowledge                                        │
│ [ query: symbol / thesis / packet / trace id              ]   │
│                                                              │
│ ┌──────────────────────────────────────────────────────────┐ │
│ │ QuantitativeFinance · qf-decisions                       │ │
│ │ [QF-authored packet title]                               │ │
│ │ Approval: [display-only]  Calibration: [badge text]      │ │
│ │ Provenance: [badge text]  Trace: [trace_id]              │ │
│ │ Thesis excerpt: [QF-authored excerpt, not rewritten]     │ │
│ │ [Open detail] [Open in QF] [Build evidence bundle]       │ │
│ │ Action boundary: approval and execution happen in QF     │ │
│ └──────────────────────────────────────────────────────────┘ │
│                                                              │
│ Daily Digest                                                 │
│ ┌──────────────────────────────────────────────────────────┐ │
│ │ QF Packet: [title] · [approval state] · [trust summary]  │ │
│ │ [one-line QF-authored why-now excerpt] [Open in QF]      │ │
│ └──────────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────────┘
```

**Interactions:**
- Search result card opens the normal artifact detail while preserving QF source label and trust metadata.
- `[Open in QF]` uses the QF deep link exactly as supplied in packet metadata.
- `[Build evidence bundle]` starts a user-selected context bundle flow; it does not treat the QF packet itself as Smackerel advice.

**States:**
- Empty state: no QF packets match the query; keep normal Smackerel search empty copy.
- Loading state: existing HTMX search spinner remains visible until cards render.
- Error state: search failure does not show stale QF packets as fallback results.
- Degraded state: missing trust metadata renders a diagnostic card without QF thesis excerpt or action affordances.

**Responsive:**
- Mobile: badges wrap below title; trace and packet IDs collapse behind a details toggle.
- Tablet: card actions stack below metadata.
- Desktop: source/trust row stays visible without expanding the card.

**Accessibility:**
- Source and connector labels are text, not icon-only.
- Badge text includes semantic state and does not rely on color.
- Action boundary sentence remains visible to screen readers before any action controls.

### Screen: QF Packet Artifact Detail

**Actor:** Smackerel User / QF User | **Route:** `/artifact/{id}` | **Status:** Modify

```text
┌──────────────────────────────────────────────────────────────┐
│ < Back to search                                             │
│ QF Packet: [QF-authored title]                               │
│ QuantitativeFinance · qf-decisions · qf/decision-packet      │
├──────────────────────────────────────────────────────────────┤
│ Trust Metadata                                               │
│ [CalibrationBadge] [DataProvenanceBadge] [Approval state]    │
│ Packet [packet_id]  Trace [trace_id]                         │
│ Intent [intent_id]  Scenario [scenario_id]                   │
│                                                              │
│ ┌──────────────────────────────────────────────────────────┐ │
│ │ QF-authored content                                     │ │
│ │ Thesis: [verbatim/faithful packet thesis]               │ │
│ │ Why now: [QF why-now excerpt]                           │ │
│ │ Quantified impact: [QF impact summary]                  │ │
│ └──────────────────────────────────────────────────────────┘ │
│                                                              │
│ ┌──────────────────────────────────────────────────────────┐ │
│ │ Companion Context                                       │ │
│ │ Related artifacts/concepts/entities from Smackerel       │ │
│ │ [select] Email note      [select] Bookmark               │ │
│ │ [select] Market news     [select] Concept page           │ │
│ │ [Build PersonalEvidenceBundle] [Open in QF]              │ │
│ └──────────────────────────────────────────────────────────┘ │
│                                                              │
│ Actions unavailable here: approve, execute, mandate changes, │
│ and EmergencyStop remain QF-owned release-gated actions.     │
└──────────────────────────────────────────────────────────────┘
```

**Interactions:**
- Related context selection adds Smackerel artifacts to the evidence builder draft.
- `[Build PersonalEvidenceBundle]` opens the builder with selected sources prefilled.
- `[Open in QF]` opens the authoritative QF packet deep link.
- Approval/execution/mandate/EmergencyStop controls are absent in pre-MVP; if a later endpoint appears, the screen must still show QF-supported action class and policy text.

**States:**
- Empty state: no related Smackerel context; show packet trust metadata and QF deep link only.
- Loading state: related context area shows loading while packet metadata remains visible.
- Error state: related context failure does not hide QF trust metadata.
- Degraded state: if packet validation failed, detail shows diagnostic metadata and suppresses evidence-building entry points.

**Responsive:**
- Mobile: related context checkboxes become full-width rows; QF metadata appears before content.
- Tablet: trust metadata and QF content stack above companion context.
- Desktop: QF content and companion context can use two stacked panels inside the existing 800px body width.

**Accessibility:**
- Checkboxes have source titles, content type, sensitivity, and selection purpose in their labels.
- The action-boundary notice uses alert/information semantics and is present in reading order.
- QF deep link text includes packet title or ID.

### Screen: Personal Evidence Bundle Builder

**Actor:** Personal Evidence Curator | **Route:** `/evidence-bundles/new` | **Status:** New

```text
┌──────────────────────────────────────────────────────────────┐
│ Search  Digest  Topics  Knowledge  Settings  Status  [Theme] │
├──────────────────────────────────────────────────────────────┤
│ Build PersonalEvidenceBundle                                 │
│ Target QF packet/context: [packet_id or analysis context]     │
│                                                              │
│ ┌──────────────────────────────┐ ┌─────────────────────────┐ │
│ │ Selected Sources             │ │ Bundle Metadata         │ │
│ │ [x] [artifact title] email   │ │ Consent scope [select]  │ │
│ │ [x] [artifact title] note    │ │ Sensitivity [select]    │ │
│ │ [x] [artifact title] market  │ │ Confidence [summary]    │ │
│ │ [remove] [preview source]    │ │ Redaction [summary]     │ │
│ └──────────────────────────────┘ └─────────────────────────┘ │
│                                                              │
│ ┌──────────────────────────────────────────────────────────┐ │
│ │ Extracted Claims                                         │ │
│ │ Claim | Source IDs | Symbol/Entity | Confidence          │ │
│ │ [row] | [ids]      | [symbol]      | [score]             │ │
│ └──────────────────────────────────────────────────────────┘ │
│                                                              │
│ [Validate bundle] [Export to QF] [Cancel]                    │
└──────────────────────────────────────────────────────────────┘
```

**Interactions:**
- Source rows can be removed before export; source preview is read-only.
- Consent scope and sensitivity must be explicitly selected before validation can pass.
- `[Validate bundle]` checks source IDs, claims, consent, sensitivity, provenance, and redaction summary.
- `[Export to QF]` submits only after validation passes and then shows QF import status.

**States:**
- Empty state: no sources selected; primary action returns user to search/knowledge to choose sources.
- Loading state: claim extraction and export status use explicit progress messages.
- Error state: missing consent, missing source provenance, unsupported sensitivity, or QF export failure appears beside the failing section.
- Success state: bundle ID, QF import status, and source count are shown with a link back to the target packet/context.

**Responsive:**
- Mobile: source selection, metadata, claims, and export status become a single-column stepper.
- Tablet: selected sources and metadata stack above claims.
- Desktop: selected sources and metadata sit side by side above claims.

**Accessibility:**
- Step headings identify source selection, metadata, claims, and export status.
- Validation summary focuses the first invalid section.
- Export success and failure are announced through an `aria-live` region.

### Screen: Telegram QF Packet Summary

**Actor:** Smackerel User / QF User | **Route:** Telegram digest/message surface | **Status:** Modify

```text
┌──────────────────────────────────────────────┐
│ Smackerel Daily Digest                       │
│                                              │
│ QuantitativeFinance · qf-decisions           │
│ [QF packet title]                            │
│ Approval: [display-only state]               │
│ Calibration: [badge text]                    │
│ Provenance: [badge text]                     │
│ Trace: [short trace_id]                      │
│                                              │
│ [QF-authored why-now excerpt]                │
│ Open in QF: [deep_link]                      │
│                                              │
│ Actions: open/read only in Smackerel         │
└──────────────────────────────────────────────┘
```

**Interactions:**
- User can open QF deep link from the message.
- No Telegram approval, execution, mandate, watch, or EmergencyStop buttons appear in pre-MVP.
- If packet is degraded, message uses diagnostic wording and links to connector status rather than packet detail.

**States:**
- Empty state: no QF packets selected for digest; digest omits the QF block.
- Loading state: not shown in Telegram; packet inclusion is decided before delivery.
- Error state: connector health warnings appear in status/admin surfaces, not as stale packet advice.
- Degraded state: degraded packet summary contains source, reason, trace/packet identifiers if safe, and no thesis excerpt.

**Responsive:**
- Telegram summary is single-column and limited to compact labels.
- Long IDs are shortened visually while full IDs remain available in Web detail.

**Accessibility:**
- Text order is source, title, trust, trace, excerpt, link, boundary notice.
- Links use descriptive labels and do not rely on emoji or icons.

## User Flows

### User Flow: QF Packet Sync And Read-Only Surfacing

```mermaid
stateDiagram-v2
	[*] --> ConnectorConfigured
	ConnectorConfigured --> Syncing: qf-decisions sync starts
	Syncing --> ValidationFailed: required QF metadata missing or schema mismatch
	Syncing --> ArtifactStored: packet envelope validates
	ValidationFailed --> ConnectorDegraded: health records reason and no trusted card renders
	ArtifactStored --> SearchDigestTelegram: packet appears read-only with QF labels
	SearchDigestTelegram --> QFDeepLink: user opens authoritative QF view
	QFDeepLink --> [*]
```

### User Flow: Evidence Bundle Export

```mermaid
stateDiagram-v2
	[*] --> ContextSelection
	ContextSelection --> BundleBuilder: user selects artifacts concepts entities or market context
	BundleBuilder --> BundleInvalid: consent sensitivity provenance or sources missing
	BundleBuilder --> BundleValid: validation passes
	BundleInvalid --> ContextSelection: user revises selection or metadata
	BundleValid --> ExportToQF: user exports PersonalEvidenceBundle
	ExportToQF --> ExportFailed: QF import rejects or is unavailable
	ExportToQF --> ExportRecorded: QF import status recorded
	ExportFailed --> BundleBuilder
	ExportRecorded --> [*]
```

### User Flow: Action Boundary And Schema Mismatch

```mermaid
stateDiagram-v2
	[*] --> PacketVisible
	PacketVisible --> UserLooksForAction: user inspects packet detail or Telegram summary
	UserLooksForAction --> BoundaryNotice: screen explains read-only companion behavior
	PacketVisible --> SchemaMismatch: connector receives incompatible packet version
	SchemaMismatch --> DiagnosticOnly: connector stores diagnostic metadata
	DiagnosticOnly --> StatusScreen: user reviews connector status and reason
	BoundaryNotice --> QFDeepLink: user completes supported actions only in QF
	StatusScreen --> [*]
	QFDeepLink --> [*]
```
