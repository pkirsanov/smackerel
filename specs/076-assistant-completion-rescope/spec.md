# Feature: 076 Assistant Completion & Rescope Follow-Up

**Status:** in_progress (analyst bootstrap; ceiling = `done`)
**Workflow Mode:** `full-delivery`
**Owner Directive (2026-06-02):** Consolidate every scope rescoped out of
the 2026-06-02 convergence session into a single follow-on spec so the
work has one owner, one manifest, one execution plan, and one terminal
status — instead of being scattered across six predecessor specs whose
engineering cores have already shipped.

**Depends On:**
- [spec 061 — Conversational Assistant](../061-conversational-assistant/spec.md) (facade, tool registry, IntentTrace)
- [spec 064 — Open-Ended Knowledge Agent](../064-open-ended-knowledge-agent/spec.md) (registry, agent loop, web search, capture-fallback hook)
- [spec 065 — Generic Micro-Tools](../065-generic-micro-tools/spec.md) (micro-tool foundation, envelope, SST config)
- [spec 066 — Legacy Keyword Surface Retirement](../066-legacy-keyword-surface-retirement/spec.md) (Telegram retirement core)
- [spec 073 — Web/Mobile Assistant Frontend](../073-web-mobile-assistant-frontend/spec.md) (web PWA, shared-mobile Dart core, render-descriptor v1)
- [spec 074 — Capture-As-Fallback Policy](../074-capture-as-fallback-policy/spec.md) (facade hook, eligibility gate, no-ground + abandoned-clarify triggers)
- [spec 075 — Legacy Retirement Telemetry](../075-legacy-retirement-telemetry/spec.md) (facade Policy dispatch + Telegram coexistence)

**Inherits (canonical predecessor planning preserved verbatim under each
predecessor's `## Superseded Scopes` / `## Rescope Close-Out` /
`## Rescope Decision` section):**
- Spec 064 scopes 02, 03, 04, 05, 06, 08, 09, 11
- Spec 065 scopes 02, 03, 04
- Spec 066 scopes 03, 05
- Spec 073 scopes 03, 04
- Spec 074 scopes 02, 03, 05
- Spec 075 scopes 01, 02, 03, 04, 05

---

## 1. Problem Statement

The 2026-06-02 convergence session shipped the engineering core of six
parallel specs (assistant tool registry, micro-tool foundation,
Telegram retirement, web PWA + shared-mobile Dart core, capture-fallback
facade hook, legacy-retirement facade Policy dispatch). To close those
predecessors cleanly while preserving the user-visible behaviors that
were planned but not executed, work that did not fit each predecessor's
engineering-core change boundary was rescoped to a follow-on spec.

This spec is that follow-on. Without consolidating the rescoped work
into one tracked artifact, the surfaced behaviors — full open-knowledge
tool surface, generic micro-tool overlays, NL replacements for retired
slash commands, native mobile chat + cross-surface parity, capture
provenance + dedup + telemetry, and the legacy-retirement deprecation
window + residual telemetry + automatic pause — are at risk of falling
between specs with no single owner.

---

## 2. Actors & Personas

| Actor | Description | Goals | Permissions |
|-------|-------------|-------|-------------|
| **Human user (general)** | Talks to the assistant in their own words across PWA, Telegram, WhatsApp, and (new) iOS/Android shared-mobile clients. | Get accurate, cited answers; have the right intent inferred from plain English; never feel punished for typing a retired command. | Existing transport permissions. |
| **Human user (returning long-time)** | Knew the old slash commands. | Be told once which NL phrasing replaces each retired command; not be re-spammed. | Existing transport permissions. |
| **Operator** | Owns SST config, dashboards, alerts. | Track tool usage, capture-fallback rates, residual legacy-command usage; pause or roll back the legacy-retirement window if a threshold is crossed. | Edits `config/smackerel.yaml`; reads Grafana / alerting. |
| **Assistant Facade** | Spec 061. | Compose tool dispatch, capture-fallback, legacy-retirement Policy, and per-transport rendering on one canonical `AssistantResponse`. | Reads conversation state; writes to `assistant_conversations`. |
| **Open-Knowledge Agent** | Spec 064. | Run the bounded tool loop, enforce per-turn / per-user budgets, refuse with capture instead of fabricating, and emit cite-back evidence. | Calls registered tools; persists tool traces with lifecycle. |
| **Telemetry Pipeline** | Spec 030 + spec 049. | Count tool calls, capture-fallback events, residual legacy-command usage by command + privacy-preserving user bucket. | Standard telemetry permissions. |

---

## 3. Outcome Contract

**Intent:** The assistant surface is feature-complete across the six
predecessor specs — open-knowledge tools are typed, budgeted, cited,
persisted with lifecycle, and surfaced through the same micro-tool
envelope as generic overlays; retired slash commands have humane NL
replacements + measurable deprecation windows; mobile chat reaches
parity with web; capture-as-fallback carries provenance + per-user
dedup + cross-transport acknowledgement parity + IntentTrace links.

**Success Signal:**
- A user typing a unit-conversion / calculator / location / entity
  question receives a tool-driven answer with cited evidence
  (SCN-064-A02, SCN-065-A04, SCN-065-A05, SCN-065-A01..A03, SCN-065-A06).
- A user typing a previously-retired slash command (e.g. `/find …`,
  `/rate …`) receives the NL-equivalent answer plus exactly one
  per-(user, command, window) deprecation notice; subsequent invocations
  receive the NL answer without re-notice; the dedup ledger survives
  across transports (SCN-066-A02, SCN-066-A03, SCN-075-A01..A03,
  SCN-075-A09).
- Annotation classification runs through the LLM-extraction path with
  warm-cache consistency, no `interactionMap` lookups
  (SCN-066-A08).
- A user on iPhone/iOS or Android (shared-mobile build) sends an
  authenticated turn that renders identically to PWA, retries
  idempotently on transient failure, passes the VoiceOver/TalkBack a11y
  floor, and fails loud at build/start time when the backend base URL
  is missing (SCN-073-A02, SCN-073-A03, SCN-073-A10, SCN-073-A11).
- Disambiguation, confirm cards, capture-as-fallback acknowledgement,
  source-detail citations, and closed-vocabulary `transport_hint` all
  render identically across web + mobile + Telegram + WhatsApp
  (SCN-073-A04..A08).
- Capture-as-fallback Ideas carry explicit-vs-fallback provenance,
  honor per-user same-text dedup inside an SST-defined window without
  ever deduping across users, attach an IntentTrace link, and produce
  an identical acknowledgement shape on every transport (SCN-074-A02,
  SCN-074-A03..A05, SCN-074-A07, SCN-074-A11).
- The legacy-retirement deprecation window emits residual telemetry
  with HMAC user buckets, auto-pauses when a threshold is crossed for
  N consecutive days, resets the counter on resume, returns the
  canonical unknown-command response after close, and gates final
  legacy-handler deletion on a zero-invocation observation report
  (SCN-075-A04..A08).
- The open-knowledge agent runs the typed-registry tool loop with
  enforced per-turn + per-user-monthly budgets, refuses-with-capture
  on budget exhaustion or tool failure, refuses on fabricated sources,
  honors operator tool-disable flags, and persists tool traces in a
  lifecycle-tracked table (SCN-064-A02..A08 inherited behaviors).

**Hard Constraints:**
1. **Behavior preservation.** Every inherited SCN identifier keeps its
   canonical Given/When/Then text. Any change to the canonical text
   MUST be recorded as a `replaces` link in `scenario-manifest.json`.
2. **Single execution plan.** This spec is the only place these
   scenarios are *executed*. Predecessor specs retain the planning
   text under their `## Superseded Scopes` (or equivalent) sections
   as historical context only.
3. **SST fail-loud.** Every new or inherited config key
   (`assistant.tools.*`, `assistant.annotation.classifier.*`,
   `legacy_retirement.*`, `assistant.capture_fallback.*`,
   `openknowledge.agent.budgets.*`) MUST fail loud at startup if
   missing. NO defaults, NO fallback values.
4. **Capture inviolability.** A fallback-eligible turn ALWAYS persists
   exactly one Idea once per `(user, normalized_text, time_bucket)`
   regardless of dedup outcome. Capture is never silently dropped.
5. **Cross-transport parity.** Disambiguation, confirm cards, capture
   acknowledgement, source-detail citations, and deprecation notices
   MUST render via the same `render-descriptor-v1` payload on web,
   iOS, Android, Telegram, and WhatsApp. No client-side scenario
   branching.
6. **Privacy.** Residual-usage telemetry and capture-fallback telemetry
   MUST use HMAC user buckets; no raw user ids or raw turn text in
   metric labels, dashboards, or logs.
7. **Per-user dedup.** Capture dedup and notice dedup are keyed on
   `user_id`. Cross-user dedup is forbidden.
8. **Cite-back invariant.** When the open-knowledge agent emits an
   answer that depends on tool output, the response MUST carry
   citations to the actual tool sources. Fabricated or missing
   citations MUST flip the answer to refusal-with-capture
   (SCN-064-A06).
9. **One persistence story.** Tool traces, notice ledger, pause state,
   and observation reports MUST live in `assistant_conversations` /
   `assistant_legacy_retirement_*` / `assistant_tool_traces` row
   families. No parallel storage.

**Failure Condition:** Any inherited SCN remains unexecuted after this
spec reaches terminal status; OR canonical Gherkin text drifts without a
`replaces` link; OR cross-transport parity diverges; OR capture
dedups across users; OR a config key acquires a silent default; OR
residual telemetry leaks raw identifiers.

---

## 4. Product Principle Alignment

| Principle | Alignment | Evidence |
|-----------|-----------|----------|
| **P1 Observe First, Ask Second** | NL replacements absorb retired commands; capture-as-fallback persists every prompt before classification. | SCN-066-A02/A03, SCN-074-A01 (via 074), SCN-064-A04/A05. |
| **P2 Vague In, Precise Out** | Location/entity micro-tools resolve ambiguity instead of demanding exact field input. | SCN-065-A01..A03, SCN-065-A06. |
| **P3 Knowledge Breathes** | Tool traces persist with a lifecycle column; capture-fallback Ideas enter the standard lifecycle. | Open-knowledge persistence (064 scope 11). |
| **P5 One Graph, Many Views** | Notice ledger and capture dedup share `assistant_conversations`; no parallel store. | Hard Constraint 9. |
| **P6 Invisible By Default, Felt Not Heard** | At most one deprecation notice per (user, retired_command, window). | SCN-075-A01..A03. |
| **P7 Small, Frequent, Actionable Output** | Cited tool answers + concise deprecation addenda; no long-form essays inserted by this spec. | SCN-064-A01..A03. |
| **P8 Trust Through Transparency** | Cite-back invariant; residual telemetry dashboards; explicit vs fallback provenance. | Hard Constraints 6, 8; SCN-074-A02. |
| **P9 Design For Restart, Not Perfection** | Returning user is gently told what to type, intent still served. | SCN-075-A01. |

---

## 5. Functional Requirements (Inherited BDD Scenarios)

All scenarios below preserve the canonical Given/When/Then text from
the predecessor specs. The full Gherkin lives in
`scenario-manifest.json` (this spec) and remains readable in each
predecessor's `## Superseded Scopes` section for historical context.

### 5.1 Open-Knowledge Agent Hardening (inherited from spec 064)

| Scenario | One-line behavior |
|---|---|
| SCN-064-A02 | Unit / math question is answered via the deterministic micro-tool with cited result. |
| SCN-064-A03 | Hybrid question uses internal-graph retrieval + web search and cites both. |
| SCN-064-A04 | Per-turn budget exhaustion produces refusal-with-capture, never a partial fabrication. |
| SCN-064-A05 | Tool failure (timeout / error envelope) produces refusal-with-capture. |
| SCN-064-A06 | Detected fabricated source flips the answer to refusal. |
| SCN-064-A07 | Operator-disabled `web_search` tool is not invoked; agent falls back cleanly. |
| SCN-064-A08 | Per-user monthly budget exceeded produces refusal-with-capture and operator-visible signal. |

### 5.2 Generic Micro-Tool Overlays (inherited from spec 065)

| Scenario | One-line behavior |
|---|---|
| SCN-065-A01 | `location_normalize` returns the canonical place for an unambiguous user phrase. |
| SCN-065-A02 | `location_normalize` returns the disambiguation list for an ambiguous phrase. |
| SCN-065-A03 | `location_normalize` overlay rules apply project-specific aliases without leaking PII. |
| SCN-065-A04 | `unit_convert` covers the additional adversarial cases (mixed units, locale separators, negative magnitudes). |
| SCN-065-A05 | `calculator` covers the additional adversarial cases (precedence, overflow, divide-by-zero). |
| SCN-065-A06 | `entity_resolve` returns a graph entity or a disambiguation list; scenario-manifest registers the tool. |

### 5.3 NL Replacements & Annotation Classifier (inherited from spec 066)

| Scenario | One-line behavior |
|---|---|
| SCN-066-A02 | "Find me notes about X" (NL) returns the same results as the retired `/find` command. |
| SCN-066-A03 | "Rate this" (NL, ambiguous) enters disambiguation instead of erroring. |
| SCN-066-A08 | Annotation classification uses the LLM-extraction path (`annotation.classify.v1`) with warm-cache consistency; `interactionMap` is deleted; `assistant.annotation.classifier.*` keys are fail-loud. |

### 5.4 Mobile Chat & Cross-Surface Parity (inherited from spec 073)

| Scenario | One-line behavior |
|---|---|
| SCN-073-A02 | Shared mobile client uses generated types from the golden schema (iOS + Android). |
| SCN-073-A03 | Transient network failure retries with the same `transport_message_id` (mobile). |
| SCN-073-A04 | Disambiguation prompt renders and round-trips identically on web + mobile. |
| SCN-073-A05 | Confirm card renders identically and round-trips. |
| SCN-073-A06 | Capture-as-fallback acknowledgement renders identically across transports. |
| SCN-073-A07 | No client-side scenario branching exists; all rendering driven by `render-descriptor-v1`. |
| SCN-073-A10 | Shared mobile client meets VoiceOver + TalkBack accessibility floor. |
| SCN-073-A11 | Missing backend base URL fails loud at build/start time (mobile). |

### 5.5 Capture-As-Fallback Provenance, Dedup, and Telemetry (inherited from spec 074)

| Scenario | One-line behavior |
|---|---|
| SCN-074-A02 | Explicit user-initiated capture is provenance-distinct from policy-driven fallback capture. |
| SCN-074-A03 | Same-user same-normalized-text within the dedup window dedupes. |
| SCN-074-A04 | Same-user same-text outside the dedup window does NOT dedup. |
| SCN-074-A05 | Cross-user dedup is forbidden (per-user isolation invariant). |
| SCN-074-A07 | `smackerel_capture_as_fallback_total` counter and IntentTrace carry the capture link. |
| SCN-074-A11 | Capture-fallback acknowledgement shape is identical across PWA, Telegram, WhatsApp, and mobile. |

### 5.6 Legacy-Retirement Safety, Telemetry & Lifecycle (inherited from spec 075)

| Scenario | One-line behavior |
|---|---|
| SCN-075-A01 | First retired-command invocation shows one notice and still serves the NL intent. |
| SCN-075-A02 | Second invocation of the same retired command does not re-notify. |
| SCN-075-A03 | A different retired command produces its own one-time notice. |
| SCN-075-A04 | Residual telemetry counts invocations per `(command, user_bucket)`; dashboard renders rolling 7-day report. |
| SCN-075-A05 | Rollback threshold pauses the window automatically after N consecutive days over budget. |
| SCN-075-A06 | Resuming the window resets the consecutive-day counter. |
| SCN-075-A07 | Window-closed response is the canonical unknown-command response with `/help` pointer. |
| SCN-075-A08 | Post-window observation report confirms zero legacy-handler invocations before final deletion. |
| SCN-075-A09 | Dedup ledger survives across sessions and transports (keyed on `user_id`, not on transport). |

---

## 6. Non-Goals

- Re-executing scenarios that the predecessor specs already executed
  and certified (SCN-064-A01, SCN-066-A01/A04..A07/A09, SCN-073-A01/A08/A09,
  SCN-074-A01/A06/A08..A10/A12, SCN-075-A10..A14).
- Adding net-new behaviors not already planned in a predecessor spec.
- Adding native UI surfaces beyond what spec 073 scope 3 already planned.
- Deleting legacy slash-command handler code (gated on SCN-075-A08
  observation evidence; the deletion itself is a follow-up PR, not a
  scope here).

---

## 7. UI Scenario Matrix (Mobile + Cross-Surface)

| Scenario | Actor | Entry Point | Steps | Expected Outcome | Surface(s) |
|---|---|---|---|---|---|
| SCN-073-A02 | Human user (mobile) | iOS / Android shared-mobile build | Open app, sign in, send a turn | Response renders from generated render-descriptor types; no hand-rolled JSON parsing | iOS, Android |
| SCN-073-A03 | Human user (mobile) | iOS / Android shared-mobile build | Send a turn, force transient 5xx, retry | Same `transport_message_id` reused; no duplicate Idea, no duplicate user message | iOS, Android |
| SCN-073-A04 | Human user | All surfaces | Trigger ambiguous intent | Disambiguation chips render identically; selection round-trips | Web, iOS, Android, Telegram, WhatsApp |
| SCN-073-A05 | Human user | All surfaces | Trigger confirm-required action | Confirm card renders identically; confirm/cancel round-trips | Web, iOS, Android, Telegram, WhatsApp |
| SCN-073-A06 | Human user | All surfaces | Trigger fallback-eligible turn | Capture-as-fallback ack renders identically | Web, iOS, Android, Telegram, WhatsApp |
| SCN-073-A07 | Engineering audit | Static scan of mobile + web client code | grep for client-side scenario branches | Zero hits | Web, iOS, Android |
| SCN-073-A10 | Human user (assistive tech) | iOS VoiceOver / Android TalkBack | Navigate a conversation | Roles, labels, and focus order pass the a11y floor harness | iOS, Android |
| SCN-073-A11 | Build pipeline | Mobile build / startup | Build or start with `SMACKEREL_API_BASE_URL` unset | Build/start fails loud naming the missing key | iOS, Android |
| SCN-075-A01..A03 / A09 | Human user (returning) | Any transport | Send a retired slash command | One-time notice + NL answer; dedup carries across transports | Web, iOS, Android, Telegram, WhatsApp |
| SCN-074-A11 | Human user | Any transport | Trigger fallback capture | Acknowledgement copy + structure identical | Web, iOS, Android, Telegram, WhatsApp |

---

## 8. Non-Functional Requirements

- **Performance:** Tool dispatch + render path MUST hold the existing
  spec 061 facade p95 SLA covered by `tests/stress/assistant_facade_p95_test.go`.
- **Accessibility:** Mobile chat MUST clear the spec 073 a11y floor on
  VoiceOver + TalkBack.
- **Privacy:** Residual-usage and capture-fallback telemetry MUST use
  HMAC user buckets; no raw user ids or turn text in labels.
- **Compliance:** All new persistence (tool traces, notice ledger,
  pause state, observation reports) inherits the existing backup +
  retention policy of `assistant_conversations` and the spec 064
  artifact table.

---

## 9. Acceptance Criteria

- Each SCN listed in §5 has at least one executed test entry in this
  spec's `scenario-manifest.json` (status `executed` with linked test).
- Each SCN appears as a `replaces`/`inheritsFrom` link from the
  predecessor scenario entry so the predecessor's manifest remains
  traceable.
- Every config key introduced or inherited by this spec is validated
  by a fail-loud config unit test.
- Live-stack regression coverage exists for every "Yes" live-system row
  in `scopes.md` Test Plan.
- `bash .github/bubbles/scripts/artifact-lint.sh specs/076-assistant-completion-rescope`
  passes.
