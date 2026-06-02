# Design: 076 Assistant Completion & Rescope Follow-Up

## Overview

This design composes — it does not invent. Every component named below
already exists on disk as the engineering core shipped by one of the six
predecessor specs. The work in this spec wires the existing seams into
the user-visible behaviors that were rescoped out of those predecessors.

The design is grouped by the six capability areas in `spec.md` §5, plus
a cross-cutting section covering the shared seams (facade composition,
render-descriptor-v1, scenario-manifest invariants, SST validation,
telemetry privacy).

## Architecture

### Shared Seams (Cross-Cutting)

| Seam | Owner spec | Where it lives | Role in this spec |
|---|---|---|---|
| `assistant.Facade.Handle` | 061 | `internal/assistant/facade.go` | Single composition point: legacy-retirement Policy → tool dispatch → capture-as-fallback hook → render |
| `assistant.AssistantResponse` + `render-descriptor-v1` | 073 | `internal/assistant/response.go`, `config/assistant/render-descriptor-v1.json` | Wire schema all transports render from; additive-only changes in this spec |
| `legacyretirement.Policy` | 075 | `internal/assistant/legacyretirement/` | Pre-routing dispatcher; this spec uses it as-is, adds notice-renderer wiring for inherited transports |
| `microtools.Envelope` + `microtools.Registry` | 065 | `internal/agent/tools/microtools/` | Typed envelope this spec extends with `location_normalize`, `entity_resolve`, expanded `unit_convert`/`calculator` |
| `openknowledge.Agent` | 064 | `internal/assistant/openknowledge/agent/` | Bounded tool loop; this spec adds typed-registry sentinels, budget enforcement at agent-loop boundary, cite-back verifier hook |
| `capturefallback.Policy` | 074 | `internal/assistant/capturefallback/` | Eligibility gate + hook; this spec adds provenance tag, dedup store, IntentTrace link, telemetry |
| `assistant_conversations` row family | 061 | `internal/db/migrations/...` | Shared row family for notice ledger, capture dedup, tool-trace lifecycle |

### Capability Area 1 — Open-Knowledge Agent Hardening (064 scopes 02–06, 08, 09, 11)

**Goal:** Take the typed-registry + agent-loop core shipped by 064/01/07/09/12/13/18 to behavior-complete state.

- **Registry typed sentinels** (064 scope 02): exported error types
  `ErrToolNotRegistered`, `ErrToolDisabled`, `ErrBudgetExhausted` on
  `openknowledge.Registry`. Agent loop pattern-matches sentinels instead
  of comparing strings.
- **SST struct fail-loud validation** (064 scope 03): single
  `OpenKnowledgeConfig` struct loaded from `assistant.openknowledge.*`;
  every field validated at startup; missing field → named-key fail-loud.
- **LLM bridge contract test** (064 scope 04): contract test pinning the
  request/response shape between Go agent and `ml/app/routes/chat.py`
  (the dispatch path already shipped via 064 scope 12).
- **Calculator / unit_convert adversarial cases** (064 scope 05): extends
  the deterministic-tool implementations shipped under spec 065 scope 1
  with the adversarial test surface 064 originally planned. SCN-064-A02
  re-executed against the expanded surface.
- **Internal-retrieval as a Tool interface** (064 scope 06): wraps the
  spec 064 scope 07 web-search Tool with a sibling
  `internal_retrieval.Tool` whose envelope mirrors `microtools.Envelope`.
  Drives SCN-064-A03 (hybrid).
- **Cite-back verifier** (064 scope 08): standalone package
  `internal/assistant/openknowledge/citeback/` that verifies every
  source URL referenced in an answer appears in the recorded tool trace.
  Mismatch flips the agent's decision to refusal-with-capture (SCN-064-A06).
  Complements the runtime provenance gate already in place under PKT-061-A.
- **Agent-loop budgets** (064 scope 09): enforces per-turn step budget
  and per-user monthly token budget at the agent boundary; circuit-breaker
  already integrated via SCOPE-14/16 is reused. Drives SCN-064-A04 + A08.
- **Persistence migration with lifecycle + outcome columns** (064 scope 11): new
  table `assistant_tool_traces` with `(turn_id, tool_name, payload_redacted, lifecycle_state, call_outcome, created_at)`.
  `lifecycle_state ∈ {active, cooling, pruned}` participates in the existing
  artifact-prune lifecycle (migration 053). `call_outcome ∈ {running, succeeded,
  failed, refused}` is the per-tool-call dispatcher outcome (migration 054).
  The two columns are intentionally separate — earlier drafts conflated them
  under one `lifecycle_state` name; the split is documented in migration 054.

### Capability Area 2 — Generic Micro-Tool Overlays (065 scopes 02–04)

**Goal:** Ship the user-visible micro-tools on top of the foundation
already at `internal/agent/tools/microtools/`.

- **`location_normalize`** (065 scope 02): new tool registered in the
  facade tool registry with overlay support (`assistant.tools.location_normalize.overlays.*`
  SST). Returns canonical place OR disambiguation list. Drives
  SCN-065-A01..A03.
- **`unit_convert` + `calculator` extensions** (065 scope 03): adds the
  adversarial cases (locale separators, mixed units, precedence, overflow,
  divide-by-zero). Shares test surface with 064 scope 05. Drives
  SCN-065-A04, A05.
- **`entity_resolve`** (065 scope 04): graph-backed entity resolver that
  consults `entities` / `connections` tables and returns either an entity
  or a disambiguation list. Drives SCN-065-A06.

### Capability Area 3 — NL Replacements & Annotation Classifier (066 scopes 03, 05)

- **NL `/find` equivalent** (066 scope 03 → SCN-066-A02): adds a
  scenario-manifest entry that maps "find me notes about X" through the
  spec 061 facade to the existing internal-retrieval path. No new tool;
  just a facade routing rule + e2e regression.
- **NL `/rate` disambiguation** (066 scope 03 → SCN-066-A03): "rate this"
  routes into spec 061's disambiguation flow instead of erroring.
- **Annotation classifier replacement** (066 scope 05 → SCN-066-A08):
  swap `annotation.interactionMap` for the `annotation.classify.v1`
  compiled-intent scenario; warm-cache consistency wired into the
  facade's cache layer; new SST keys `assistant.annotation.classifier.*`
  are fail-loud; deletes `internal/annotation/interaction_map.go`.

### Capability Area 4 — Mobile Chat & Cross-Surface Parity (073 scopes 03, 04)

- **Shared mobile vertical slice** (073 scope 03): wires the Dart shared
  core (already shipped under 073 scope 01) into an iOS adapter and an
  Android adapter; both use the generated render-descriptor types;
  retry layer uses the same `transport_message_id` contract as web.
  Drives SCN-073-A02, A03, A10, A11.
- **Cross-surface response controls** (073 scope 04): adds renderers for
  disambiguation, confirm card, capture-ack, source-detail citations on
  iOS + Android, asserting parity with PWA via fixture-based golden
  tests. Closed-vocabulary `transport_hint` (SCN-073-A08) already covered
  by 073 scope 01 and re-asserted here under live-stack regression.

### Capability Area 5 — Capture Provenance, Dedup, Telemetry (074 scopes 02, 03, 05)

This capability area composes the shipped `artifact_capture_policy` row
family (migration `051_artifact_capture_policy.sql`, spec 074 SCOPE-2)
plus the shipped `capturefallback` package — it does NOT introduce a
new `ideas.provenance` column or a new `assistant_capture_dedup` table.
The closed provenance vocabulary is the shipped one:
`('capture-as-fallback', 'capture-explicit')`. Spec 076 must NOT
re-define it as `('explicit','fallback')`; the SST keys, persistence,
ack payloads, and inherited SCN-074-A02..A05 scenarios all bind to the
shipped tokens.

- **Provenance separation** (074 scope 02 → SCN-074-A02): wires the
  "explicit capture amendment seam" so an explicit capture
  (`provenance='capture-explicit'`, NULL `dedup_bucket_start`)
  supersedes a prior fallback Idea
  (`provenance='capture-as-fallback'`) without losing the original
  IntentTrace. No schema change — the row family already distinguishes
  the two via the shipped CHECK constraint on
  `artifact_capture_policy.provenance` and the partial UNIQUE index
  `idx_capture_fallback_dedup` that only applies to
  `provenance='capture-as-fallback'`.
- **Per-user dedup** (074 scope 03 → SCN-074-A03..A05): consumes the
  shipped partial UNIQUE index
  `(user_id, provenance, normalized_text_hash, dedup_bucket_start)
   WHERE provenance = 'capture-as-fallback'`
  and the shipped fail-loud SST `CAPTURE_AS_FALLBACK_DEDUP_WINDOW`
  (loaded by `internal/config.LoadCaptureFallback`). Cross-user dedup
  remains structurally impossible because `user_id` is part of the
  unique key. No new dedup store is introduced; a second store with a
  non-overlapping key shape is explicitly rejected as duplicate state.
- **Telemetry + IntentTrace + cross-transport ack** (074 scope 05 →
  SCN-074-A07, A11): the `smackerel_capture_as_fallback_total` counter
  shipped under spec 074 SCOPE-04A/B/C is joined to the IntentTrace via
  `intent_trace_id`; the capture-ack `AssistantResponse` is rendered
  identically across PWA, Telegram, WhatsApp, and the new mobile build.

### Capability Area 6 — Legacy-Retirement Safety, Telemetry, Lifecycle (075 scopes 01–05)

The foundation code (`internal/assistant/legacyretirement/`, migration
046, HMAC user-bucket hasher, SQL stores for notice ledger / pause state
/ observation report) is already on disk and is exercised today by spec
075's SCOPE-06 facade Policy dispatch + live integration TPs. This spec
adds the user-visible + operator-visible wiring rescoped out of 075:

- **Foundation behavioral closure** (075 scope 01 → SCN-075-A10, A11):
  re-execute the fail-loud SST and privacy assertions against this
  spec's harness so the closure lives in one place.
- **Open-window notice dedup + intent serving** (075 scope 02 →
  SCN-075-A01..A03, A09): wires the `SQLNoticeLedger.MarkShown` /
  `Dedup` contracts through the facade for PWA + mobile + WhatsApp
  (Telegram already shipped under spec 075 SCOPE-06.4); cross-transport
  ledger survival validated against live test-stack Postgres.
- **Residual telemetry + dashboard** (075 scope 03 → SCN-075-A04):
  ships the Grafana dashboard panel + the rolling 7-day query; the
  counter is already emitted by `NewMultiResidualTelemetry(prom, sql)`.
- **Automatic pause + resume** (075 scope 04 → SCN-075-A05, A06):
  threshold evaluator + alert rules wired into spec 049's monitoring
  stack; `SQLPauseStateStore.Pause/Resume` is already proven by
  spec 075 TP-075-13/14.
- **Closed-window response + observation gate** (075 scope 05 →
  SCN-075-A07, A08): closed-window dispatch branch already in the
  facade Policy; this spec adds the post-window cron that runs
  `SQLObservationReport.Generate` and gates legacy-handler-deletion
  PRs on a zero-invocation report.

## Data Model

| Table / Column | Owner | Change in this spec |
|---|---|---|
| `assistant_conversations.legacy_retirement_notices` | spec 075 | None (re-used) |
| `assistant_legacy_retirement_state` | spec 075 | None (re-used) |
| `assistant_legacy_retirement_observations` | spec 075 | None (re-used) |
| `assistant_tool_traces` | this spec (064 scope 11) | **New** table `(turn_id, tool_name, payload_redacted, lifecycle_state, call_outcome, created_at)`. `lifecycle_state ∈ {active, cooling, pruned}` (migration 053) participates in the existing prune lifecycle. `call_outcome ∈ {running, succeeded, failed, refused}` (migration 054) is the per-tool-call dispatcher outcome. The two columns are distinct concepts and MUST NOT be conflated. |
| `artifact_capture_policy` | spec 074 SCOPE-2 (migration `051_artifact_capture_policy.sql`) | **Re-used as-is.** Provides `provenance TEXT NOT NULL CHECK (provenance IN ('capture-as-fallback','capture-explicit'))`, partial UNIQUE index `(user_id, provenance, normalized_text_hash, dedup_bucket_start) WHERE provenance = 'capture-as-fallback'`, and `intent_trace_id` link. Spec 076 introduces no new dedup table and no `ideas.provenance` column. |

> **Superseded Design Decisions.** Earlier drafts of this design named a
> new `ideas.provenance` column and a new `assistant_capture_dedup`
> table. Both are withdrawn: there is no `ideas` table in the schema
> (only `artifacts.key_ideas JSONB`), and migration 051 already
> persists per-user fallback dedup against `artifact_capture_policy`.
> Adding a second dedup store with a different key shape would create
> divergent dedup state and is rejected.

The one new table this spec introduces (`assistant_tool_traces`) gets
fail-loud `NOT NULL` constraints and a named migration; no defaults.

## API / Contracts

- `render-descriptor-v1.json` is extended **additively**: new optional
  field `notice` (already shipped via 075 SCOPE-06.2b) is re-asserted;
  new optional field `provenance` on capture-ack payloads is added under
  v1-compatible rules (no `schema_version` bump).
- `openknowledge.Registry` adds exported sentinels (064 scope 02); the
  agent-loop call signature is unchanged.
- `microtools.Registry` registrations for `location_normalize` and
  `entity_resolve` follow the existing envelope contract.
- `assistant.AssistantResponse.LegacyRetirementNotice` (already shipped)
  is consumed by the new mobile renderers without schema change.

## UI / UX

All UX is composed from existing render-descriptor-v1 primitives. No
new UI atoms are introduced; cross-surface parity is enforced by
fixture-based golden tests across PWA + iOS + Android + Telegram +
WhatsApp.

## Security / Compliance

- HMAC user bucket key remains in `legacy_retirement.user_bucket_hmac_key`
  (fail-loud).
- `assistant_capture_dedup.normalized_text_hash` is HMAC-keyed so the
  dedup store cannot be reversed to original text.
- `assistant_tool_traces.payload_redacted` is the redacted INFO log path
  already shipped under 064 SCOPE-14.

## Observability

- `traceContracts`: this project does not yet define `traceContracts` in
  `.github/bubbles-project.yaml`. Trace assertions for this spec are
  implicit in the facade unit tests + the existing IntentTrace
  contract from spec 030.
- Metrics: re-uses `smackerel_capture_as_fallback_total{cause,user_bucket}`,
  `legacy_command_residual_total{command,user_bucket}`,
  `smackerel_openknowledge_tool_calls_total{tool,outcome}`.
- Dashboards: extends the spec 049 dashboard with the rolling 7-day
  residual-usage panel and a capture-fallback per-cause panel.

## Testing Strategy

Tests are mapped 1-to-1 to inherited scenarios in
`scenario-manifest.json` and laid out by capability area in
`scopes.md`. Each capability area carries:

- **unit** rows for sentinel / envelope / config / fail-loud paths
- **integration** rows for ledger / dedup / tool-registry against the
  live test-stack Postgres
- **e2e-api** rows for facade composition end-to-end
- **e2e-ui** rows for cross-surface parity (web + mobile + Telegram +
  WhatsApp golden fixtures)
- **stress** row re-asserting the existing facade p95 SLA continues to
  hold with the new tool surface + cite-back verifier
- **Regression E2E** persistent rows for every new/changed/fixed
  behavior, satisfying the transition-guard requirement

`testImpact` is not configured for this project, so all rows are
planned without impact-aware narrowing.

## Risks & Open Questions

| Risk | Mitigation |
|---|---|
| Six capability areas in one spec inflates blast radius. | Capability areas are independent scopes with explicit `dependsOn` only on shared seams; each scope ships independently. |
| Cross-surface parity tests on iOS + Android require device farm time. | Parity tests use the existing Dart shared-core golden fixture harness; native device runs are gated by a separate `mobile-device` test category. |
| Cite-back verifier may flag legitimate sources during initial roll-out. | Verifier ships behind `openknowledge.citeback.enforcement_mode` SST (`shadow` → `enforce`); shadow mode runs first and records mismatches without flipping the answer. |
| Annotation classifier swap might regress existing annotation flows. | Warm-cache consistency test + dual-write shadow run for one release before `interactionMap` deletion. |
| Predecessor `replaces` links risk drift between this spec's manifest and predecessors'. | Artifact-lint + traceability-guard run on every push; `inheritsFrom` is mandatory for every scenario in §5. |
