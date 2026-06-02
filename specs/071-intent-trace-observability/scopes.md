# Scopes: 071 IntentTrace Observability Surface

## Execution Outline

### Phase Order
1. **Scope 1: Trace Contract Foundation** — Define the versioned `IntentTrace`/sampled-out contract, required SST validation, and schema pinning before any consumers depend on it.
2. **Scope 2: Redaction, Sampling, Persistence, And Retention** — Implement the one-record-per-turn storage path, source-policy redaction, sampled-out accounting, and TTL sweep.
3. **Scope 3: Replay And Policy-Guard Integration** — Add read-only replay and the spec 067 bypass-guard trace contract using the persisted trace record.
4. **Scope 4: Dashboard, Refusal Join, And Operator Panels** — Project traces into metrics/dashboard panels and join refusal/capture counters without adding a second telemetry source.

### New Types & Signatures
- `IntentTraceRecorder.Record(ctx, TurnTraceInput) (IntentTraceResult, error)`
- `IntentTraceRedactor.Redact(SourcePolicy, CompilerPayload) (RedactedPayload, SlotsRedactionSummary, error)`
- `IntentTraceStore.Put(ctx, IntentTraceRow) error`
- `IntentTraceStore.Get(ctx, traceID string) (IntentTraceRow, error)`
- `IntentTraceStore.SweepExpired(ctx, now time.Time) (SweepResult, error)`
- `IntentTraceExporter.Export(ctx, IntentTraceRow) error`
- `IntentTraceReplay.Run(ctx, traceID string) (ReplayComparison, error)`
- Table: `assistant_intent_traces(trace_id, schema_version, turn_id, user_id_hash, transport, transport_message_id, sampled, action_class, side_effect_class, route_decision, tool_calls, final_response_status, compiler_invoked, refusal_cause, capture_cause, idea_artifact_id, slots_redaction_summary, redacted_payload, emitted_at, expires_at)`
- CLI: `./smackerel.sh assistant replay-intent <trace_id>`

### Validation Checkpoints
- After Scope 1, schema/config/golden-contract tests must fail loud on missing SST keys and schema drift before persistence work starts.
- After Scope 2, integration tests must prove full traces, sampled-out envelopes, redaction, and retention operate through the same store/export path.
- After Scope 3, E2E replay and bypass-guard tests must prove traces can drive read-only diagnostics and policy checks without side effects.
- After Scope 4, monitoring integration and dashboard query tests must prove operators can inspect action distribution, refusal joins, capture fallback, and retention outcomes.

## Overview

This plan is sequential and gated. Scope 1 creates the reusable `IntentTraceObservability` foundation required by all later replay, policy, and dashboard overlays. Scopes 2-4 depend on that foundation and add one vertical runtime outcome each.

| Scope | Name | Surfaces | Scenario IDs | Status |
|-------|------|----------|--------------|--------|
| SCOPE-071-01 | Trace Contract Foundation | backend, config, DB schema, contract tests | SCN-071-A01, SCN-071-A05, SCN-071-A10 | In Progress (tests partial) |
| SCOPE-071-02 | Redaction, Sampling, Persistence, And Retention | backend, DB, metrics, retention job | SCN-071-A02, SCN-071-A03, SCN-071-A09 | In Progress (tests partial) |
| SCOPE-071-03 | Replay And Policy-Guard Integration | CLI, assistant router, policy guard, trace store | SCN-071-A04, SCN-071-A08 | In Progress (SCN-071-A08 incomplete) |
| SCOPE-071-04 | Dashboard, Refusal Join, And Operator Panels | monitoring, metrics, dashboard inventory | SCN-071-A06, SCN-071-A07 | In Progress (tests partial) |

## Scope 1: Trace Contract Foundation

**Status:** Not Started  
**Scope-Kind:** runtime-behavior  
**Tags:** foundation:true  
**Depends On:** none  
**Scenario IDs:** SCN-071-A01, SCN-071-A05, SCN-071-A10

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-071-A01 — Exactly one IntentTrace per compiled turn
  Given the compiler is enabled and sampling_ratio = 1.0
  When a user sends a natural-language turn
  Then exactly one IntentTrace record is emitted with schema_version = "v1"
  And the record carries all required identity, routing, confidence, tool, and response-status fields

Scenario: SCN-071-A05 — SST keys are required and fail loud
  Given assistant.intent_trace.sampling_ratio is unset
  When the core process starts
  Then startup fails with a NO-DEFAULTS error naming the missing key
  And no IntentTrace records are emitted because the process never reaches steady state

Scenario: SCN-071-A10 — Schema is pinned by a golden contract test
  Given the IntentTrace schema declared in this spec
  When the contract test runs
  Then any change to field names, types, or required fields fails unless schema_version is bumped
```

### Implementation Plan

- Add `internal/assistant/intenttrace` contract types for full trace and sampled-out envelope with `schema_version = "v1"`.
- Add fail-loud config parsing for `assistant.intent_trace.sampling_ratio`, `retention_days`, `export_targets`, `replay_enabled`, and `retention_sweep_interval`.
- Add the `assistant_intent_traces` migration and schema validation before persistence/export.
- Wire the recorder call into the spec 068 compiler/facade boundary at the point before side-effect execution.
- Add golden schema fixtures that cover required fields, closed vocabularies, and a schema-version bump requirement.

### Shared Infrastructure Impact Sweep

| Shared Surface | Contract Risk | Canary Validation |
|----------------|---------------|-------------------|
| `internal/assistant/tracing/` | Existing assistant OTel attributes must keep working while adding the `assistant.intent.*` family. | `tests/integration/assistant/intent_trace_test.go::TBD emits one v1 trace without breaking existing assistant spans` |
| Config loader | Missing keys must fail loud without introducing fallbacks. | `internal/config/assistant_intent_trace_test.go::TBD missing sampling ratio names key` |
| DB migrations | New table must not alter `agent_traces` executor behavior. | `tests/integration/assistant/intent_trace_test.go::TBD compiler turn writes intent trace without agent trace requirement` |

### Change Boundary

Allowed file families: `internal/assistant/intenttrace/**`, `internal/assistant/tracing/**`, `internal/config/**`, `internal/db/migrations/**`, `cmd/core/**` wiring for the recorder only, and planned tests under `internal/**` and `tests/**`.  
Excluded surfaces: transport adapters, assistant scenario definitions, ML sidecar, dashboard JSON, and legacy-command handlers.

### Test Plan

| Scenario | Category | Planned File | Planned Test Title | Command | Live System | Notes |
|----------|----------|--------------|--------------------|---------|-------------|-------|
| SCN-071-A01 | integration | `tests/integration/assistant/intent_trace_test.go` | `TBD: emits exactly one v1 IntentTrace per compiled turn` | `./smackerel.sh test integration` | Yes | Regression row for one-record-per-turn invariant. |
| SCN-071-A05 | unit | `internal/config/assistant_intent_trace_test.go` | `TBD: missing assistant.intent_trace.sampling_ratio fails loud` | `./smackerel.sh test unit --go` | No | NO-DEFAULTS guard for required SST. |
| SCN-071-A10 | unit | `internal/assistant/intenttrace/golden_contract_test.go` | `TBD: v1 schema drift fails without version bump` | `./smackerel.sh test unit --go` | No | Golden contract pin. |
| SCN-071-A01 | e2e-api | `tests/e2e/assistant/intent_trace_contract_e2e_test.go` | `TBD: live compiled turn exposes v1 trace contract` | `./smackerel.sh test e2e` | Yes | Persistent E2E regression for contract visibility. |

### Impact-Aware Validation

`.github/bubbles-project.yaml` does not define `testImpact` or `traceContracts`; no generated impact map or trace-contract guard is available for this planning pass. Implementation must rerun planning if those project config sections are added before execution.

### Definition of Done — Tiered Validation

- [x] Contract, config, migration, and recorder wiring satisfy SCN-071-A01, SCN-071-A05, and SCN-071-A10.
  Evidence: report.md → Test Evidence (Unit: TestSchemaVersionV1IsPinned, TestGoldenV1PayloadHashPinned, TestClosedVocabulariesPinned, TestStoreRecorder_RecordsSampledRow, TestStoreRecorder_ValidationFailures; Unit-config: TestIntentTraceConfigRequiresEverySSTKey; Integration live-stack: TestIntentTracePersistsExactlyOneV1RowPerRecordCall, TestIntentTraceRecordsCompileValidateRouteToolResponseSequence). Claim Source: executed 2026-06-01T21:11Z–21:20Z.
- [ ] Test rows listed above are implemented with the planned titles or an equivalent title that preserves the scenario ID, and all pass with current-session evidence.
  Uncertainty Declaration: Integration + unit rows pass with current-session evidence (see report.md). Planned e2e row `tests/e2e/assistant/intent_trace_contract_e2e_test.go::TestIntentTraceContractE2E_LiveCompiledTurnExposesV1Contract` **is present on disk** (correcting the stale prior note). It could not execute in this pass because `./smackerel.sh test e2e` aborts at config-validate with missing `ASSISTANT_TRANSPORTS_HTTP_*` SST keys (spec 069 SCOPE-2 generator gap). Route to `bubbles.implement` on spec 069 to unblock e2e.
- [ ] Build Quality Gate passes: `./smackerel.sh format --check`, `./smackerel.sh lint`, `./smackerel.sh test unit --go`, applicable integration/e2e commands, and artifact lint for this spec.
  Uncertainty Declaration: Not run in this session — only the filtered integration suite and the targeted unit packages were executed. Format/lint/e2e/artifact-lint remain owed.

## Scope 2: Redaction, Sampling, Persistence, And Retention

**Status:** Not Started  
**Scope-Kind:** runtime-behavior  
**Depends On:** SCOPE-071-01  
**Scenario IDs:** SCN-071-A02, SCN-071-A03, SCN-071-A09

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-071-A02 — Sampled-out turns still emit a minimal envelope
  Given sampling_ratio = 0.1 and this turn is not sampled
  When the turn is compiled and executed
  Then a minimal IntentTraceSampledOut envelope is emitted
  And total-turn counters match sampled plus sampled-out envelopes

Scenario: SCN-071-A03 — Sensitive slot values are redacted per source policy
  Given a turn whose source_policy.persist_raw_text = false
  When the IntentTrace is emitted
  Then raw_text is absent and slots_redaction_summary is present
  And no sensitive slot value appears anywhere in the exported record

Scenario: SCN-071-A09 — Retention TTL is enforced from SST
  Given assistant.intent_trace.retention_days = 14
  When 15 days pass since a record was emitted
  Then the record is no longer queryable from the configured export targets
  And the retention sweep itself is observable via a structured log entry
```

### Implementation Plan

- Implement central source-policy redaction before persistence, logging, metrics, or OTel export.
- Implement deterministic sampling that writes either full trace or sampled-out envelope and increments total-turn accounting for both.
- Persist redacted trace rows in Postgres and export derived structured logs, OTel attributes, and metrics from the validated row.
- Add TTL sweep using `expires_at` and emit structured retention-sweep logs with counts only.

### Shared Infrastructure Impact Sweep

| Shared Surface | Contract Risk | Canary Validation |
|----------------|---------------|-------------------|
| Redaction helper | A leak would affect every assistant transport and dashboard. | `internal/assistant/intenttrace/redaction_test.go::TBD source policy removes raw text and sensitive slots` |
| Metrics exporter | Sampling must not under-count total turns. | `tests/integration/assistant/intent_trace_test.go::TBD sampled-out envelope contributes to total count` |
| Retention sweep | Sweep must delete only expired trace rows. | `tests/integration/assistant/intent_trace_retention_test.go::TBD expired rows swept while fresh rows remain` |

### Change Boundary

Allowed file families: `internal/assistant/intenttrace/**`, `internal/metrics/**` only for new metric registration, retention scheduler wiring, DB migration tests.  
Excluded surfaces: replay CLI, dashboard provisioning, transport adapter renderers, and spec 067 guard behavior.

### Test Plan

| Scenario | Category | Planned File | Planned Test Title | Command | Live System | Notes |
|----------|----------|--------------|--------------------|---------|-------------|-------|
| SCN-071-A02 | unit | `internal/assistant/intenttrace/sampling_test.go` | `TBD: sampled-out decision emits minimal envelope shape` | `./smackerel.sh test unit --go` | No | Unit contract for sampling branch. |
| SCN-071-A02 | integration | `tests/integration/assistant/intent_trace_test.go` | `TBD: sampled-out envelope preserves total-turn accounting` | `./smackerel.sh test integration` | Yes | Live accounting regression. |
| SCN-071-A03 | unit | `internal/assistant/intenttrace/redaction_test.go` | `TBD: source policy redacts raw text and sensitive slots` | `./smackerel.sh test unit --go` | No | Adversarial redaction fixtures. |
| SCN-071-A09 | integration | `tests/integration/assistant/intent_trace_retention_test.go` | `TBD: retention sweep removes expired traces and logs count` | `./smackerel.sh test integration` | Yes | TTL enforcement. |
| SCN-071-A03 | e2e-api | `tests/e2e/assistant/intent_trace_privacy_e2e_test.go` | `TBD: live exported trace contains redaction summary without raw slot values` | `./smackerel.sh test e2e` | Yes | Persistent privacy regression. |

### Impact-Aware Validation

No project `testImpact` or `traceContracts` map is configured. The scope still requires unit, integration, and live E2E coverage because it touches shared trace export and persistence surfaces.

### Definition of Done — Tiered Validation

- [x] Sampling, redaction, persistence, export, and retention satisfy SCN-071-A02, SCN-071-A03, and SCN-071-A09.
  Evidence: report.md → Test Evidence (Unit: TestRatioSampler_*, TestStoreRecorder_SampledOutEnvelope, TestDefaultRedactor_*; Integration live-stack: TestIntentTraceSampledOutPreservesTotalTurnAccounting, TestIntentTraceRedactionLeavesNoRawSlotValueInPayload, TestIntentTraceRetentionSweepRemovesExpiredAndKeepsFresh). Claim Source: executed 2026-06-01T21:11Z–21:20Z.
- [x] Canary rows for redaction, sampled-out accounting, and TTL sweep pass before broader suite execution.
  Evidence: The three integration canaries above ran first in the filtered live-stack run; all PASS.
- [ ] Build Quality Gate passes with artifact lint for this spec and no NO-DEFAULTS fallback syntax in touched config/code/docs.
  Uncertainty Declaration: Format, lint, and artifact-lint were not executed in this session. Planned e2e row `tests/e2e/assistant/intent_trace_privacy_e2e_test.go::TestIntentTracePrivacyE2E_StoredTraceCarriesRedactionSummaryWithoutRawSlotValues` **is present on disk** (correcting the stale prior note) but the e2e stack is blocked by the spec 069 SCOPE-2 config-generator gap (missing `ASSISTANT_TRANSPORTS_HTTP_*` keys). NO-DEFAULTS audit on touched code is also pending.

## Scope 3: Replay And Policy-Guard Integration

**Status:** Not Started  
**Scope-Kind:** runtime-behavior  
**Depends On:** SCOPE-071-02  
**Scenario IDs:** SCN-071-A04, SCN-071-A08

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-071-A04 — Replay reproduces the routing decision
  Given a stored IntentTrace with action_class = "weather.lookup" and route_decision = "scenarios/weather"
  When `./smackerel.sh assistant replay-intent <trace_id>` runs
  Then the compiler/router rehydrates the trace in dry-run mode
  And produced route_decision and tool_calls match the original without side effects or state mutation

Scenario: SCN-071-A08 — Spec 067 bypass guard reads IntentTrace fields
  Given a tool call is observed via OpenTelemetry
  When the spec 067 bypass guard inspects the surrounding trace
  Then it finds compiler_invoked = true and a matching route_decision
  And a synthetic raw-route bypass triggers the guard
```

### Implementation Plan

- Add `assistant replay-intent <trace_id>` dispatch through `./smackerel.sh` and `cmd/core` without bypassing the repo CLI surface.
- Load one persisted trace row, validate schema, run compiler/router dry-run, and compare original vs dry-run decisions.
- Add hard side-effect blocking around replay and verify no conversation/artifact/tool write happens.
- Wire spec 067 bypass guard to read `compiler_invoked`, `route_decision`, and `tool_calls[].name` from trace ancestry.

### Shared Infrastructure Impact Sweep

| Shared Surface | Contract Risk | Canary Validation |
|----------------|---------------|-------------------|
| Runtime command aliasing | `./smackerel.sh` must remain the only sanctioned runtime entrypoint. | `tests/e2e/assistant/intent_replay_test.go::TBD replay runs through smackerel CLI and returns JSON` |
| Router dry-run path | Replay must use real compiler/router logic without side effects. | `tests/e2e/assistant/intent_replay_test.go::TBD replay invokes no side-effect tools and no state writes` |
| Spec 067 guard | Guard must reject bypasses without requiring raw trace payloads. | `tests/integration/policy/intent_bypass_guard_test.go::TBD raw route without IntentTrace ancestor triggers guard` |

### Change Boundary

Allowed file families: `cmd/core/**` for CLI dispatch, `scripts/commands/**` and `smackerel.sh` only for the sanctioned replay command, `internal/assistant/intenttrace/**`, `internal/assistant/**` dry-run seams, `tests/e2e/assistant/**`, `tests/integration/policy/**`.  
Excluded surfaces: scenario definitions, transport renderers, DB schema beyond trace fetch indexes already introduced, and dashboard provisioning.

### Test Plan

| Scenario | Category | Planned File | Planned Test Title | Command | Live System | Notes |
|----------|----------|--------------|--------------------|---------|-------------|-------|
| SCN-071-A04 | e2e-api | `tests/e2e/assistant/intent_replay_test.go` | `TBD: replay reproduces route and tool calls without side effects` | `./smackerel.sh test e2e` | Yes | Live replay regression. |
| SCN-071-A04 | integration | `tests/integration/assistant/intent_replay_store_test.go` | `TBD: replay loads one stored redacted trace by trace id` | `./smackerel.sh test integration` | Yes | Store lookup and dry-run comparison. |
| SCN-071-A08 | integration | `tests/integration/policy/intent_bypass_guard_test.go` | `TestIntentBypassGuardReportsRouterRouteWithoutCompiledIntent` | `./smackerel.sh test integration` | Yes | Shared with spec 068 SCN-068-A08; guard behavior is identical and satisfies SCN-071-A08's spec 067 integration requirement. |
| SCN-071-A08 | e2e-api | `tests/e2e/assistant/intent_bypass_guard_e2e_test.go` | `TestIntentBypassGuardE2E_SyntheticRawRouteBypassIsRejected` | `./smackerel.sh test e2e` | Yes | Persistent bypass regression. |

### Impact-Aware Validation

No configured impact/trace map exists. Because this scope touches runtime command aliasing and policy guard behavior, the canary rows above must execute before broad suite validation.

- [x] Replay and policy-guard behavior satisfy SCN-071-A04 and SCN-071-A08 without side effects (integration tier only).
  Evidence: SCN-071-A04 — unit (TestStoreReplay_HappyPath_PayloadDryRunner, TestStoreReplay_SampledOutRejected, TestStoreReplay_SchemaInvalidRejected, TestStoreReplay_DryRunnerSideEffectIsBlocked, TestStoreReplay_MatchSummaryReportsDivergence) and live-stack integration (TestIntentReplayLoadsOneStoredRedactedTraceByTraceID, TestIntentReplayRefusesSampledOutEnvelope, TestIntentReplayReportsNotFoundForUnknownTraceID) executed 2026-06-01T21:11Z–21:20Z. SCN-071-A08 — live-stack integration `tests/integration/policy/intent_bypass_guard_test.go::TestIntentBypassGuardReportsRouterRouteWithoutCompiledIntent` executed 2026-06-01T23:36Z–23:41Z via `./smackerel.sh test integration --go-run '^TestIntentBypassGuardReportsRouterRouteWithoutCompiledIntent$'` → `--- PASS: TestIntentBypassGuardReportsRouterRouteWithoutCompiledIntent (0.04s)`, `ok github.com/smackerel/smackerel/tests/integration/policy 0.071s`. See report.md → Test Evidence (Round 2: 2026-06-01 23:36Z). Claim Source: executed.
- [ ] CLI and policy tests listed above pass with e2e evidence from the sanctioned repo commands.
  Uncertainty Declaration: e2e files `tests/e2e/assistant/intent_replay_test.go` (TestIntentReplayE2E_ReproducesRouteAndToolCallsWithoutSideEffects, TestIntentReplayE2E_UnknownTraceIDExits2) and `tests/e2e/assistant/intent_bypass_guard_e2e_test.go` (TestIntentBypassGuardE2E_SyntheticRawRouteBypassIsRejected) **are present on disk** but could not execute: the e2e test stack `./smackerel.sh test e2e` aborts at config-validate with `[F061-SST-MISSING] missing or invalid required assistant configuration: ASSISTANT_TRANSPORTS_HTTP_ENABLED, ASSISTANT_TRANSPORTS_HTTP_SCHEMA_VERSION, ASSISTANT_TRANSPORTS_HTTP_BODY_SIZE_MAX_BYTES, ASSISTANT_TRANSPORTS_HTTP_RATE_LIMIT_PER_USER_PER_MINUTE, ASSISTANT_TRANSPORTS_HTTP_CONVERSATION_TTL_SECONDS, ASSISTANT_TRANSPORTS_HTTP_REQUIRED_SCOPE, ASSISTANT_TRANSPORTS_HTTP_CORS_ALLOWED_ORIGINS, ASSISTANT_TRANSPORTS_HTTP_TRANSPORT_HINT_ALLOWLIST` because `scripts/commands/config.sh` does not yet emit the spec 069 SCOPE-2 HTTP transport keys (yaml block exists at `config/smackerel.yaml` lines 800–814 but the bash generator stops at WhatsApp). This is a spec 069 SCOPE-2 implementation gap — route to `bubbles.implement` on spec 069 before this item can be marked.
- [ ] Change boundary is respected: no transport renderer, scenario, or dashboard files change in this scope.
  Uncertainty Declaration: Diff review against the change boundary was not performed in this pass.

## Scope 4: Dashboard, Refusal Join, And Operator Panels

**Status:** Not Started  
**Scope-Kind:** runtime-behavior  
**Depends On:** SCOPE-071-03  
**Scenario IDs:** SCN-071-A06, SCN-071-A07

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-071-A06 — Dashboard surfaces top action_class distribution
  Given the IntentTrace export target is the monitoring stack from spec 049
  When the operator opens the "Assistant Intents" dashboard
  Then top action_class, clarification rate, refusal cause, compiler error, and capture-as-fallback panels render from real trace samples

Scenario: SCN-071-A07 — Refusal counters join to IntentTrace by cause label
  Given spec 064 refusal counters emit a `cause` label
  When a refusal occurs
  Then IntentTrace.refusal_cause equals the counter's cause label exactly
  And a dashboard join by cause label returns matching rows in both data sources
```

### Implementation Plan

- Add closed-label metrics and dashboard query fields for action class, clarification, refusal cause, compiler error, capture cause, and recent samples.
- Add refusal-cause vocabulary validation shared by spec 064 counters and `IntentTrace.refusal_cause`.
- Add fail-loud dashboard error behavior when export targets are unavailable rather than rendering zeroes.
- Add dashboard inventory/amendment hooks through the spec 049 monitoring stack owner during implementation.

### Shared Infrastructure Impact Sweep

| Shared Surface | Contract Risk | Canary Validation |
|----------------|---------------|-------------------|
| Monitoring dashboard inventory | Panel queries must not invent a second trace source. | `tests/integration/monitoring/assistant_intents_dashboard_test.go::TBD dashboard panels read trace metrics` |
| Refusal counters | Counter labels and trace labels must match exactly. | `tests/integration/assistant/refusal_trace_join_test.go::TBD refusal cause label joins counter and trace row` |
| Capture-as-fallback telemetry | Dashboard must include capture rate without owning capture policy. | `tests/integration/monitoring/assistant_intents_dashboard_test.go::TBD capture-as-fallback panel reads trace capture_cause` |

### Change Boundary

Allowed file families: monitoring inventory/query definitions, assistant metrics registration, refusal-label validation tests, and dashboard integration tests.  
Excluded surfaces: capture policy implementation, replay CLI, transport renderers, and raw log payload shape outside `IntentTrace` export fields.

### Test Plan

| Scenario | Category | Planned File | Planned Test Title | Command | Live System | Notes |
|----------|----------|--------------|--------------------|---------|-------------|-------|
| SCN-071-A06 | integration | `tests/integration/monitoring/assistant_intents_dashboard_test.go` | `TBD: Assistant Intents panels render from trace metrics` | `./smackerel.sh test integration` | Yes | Dashboard query canary. |
| SCN-071-A06 | e2e-ui | `web/pwa/tests/assistant_intents_dashboard.spec.ts` | `TBD: operator dashboard shows action distribution and trace samples` | `./smackerel.sh test e2e` | Yes | Operator UX regression if a UI/dashboard harness exists. |
| SCN-071-A07 | integration | `tests/integration/assistant/refusal_trace_join_test.go` | `TBD: refusal counter cause equals IntentTrace refusal_cause` | `./smackerel.sh test integration` | Yes | Join correctness. |
| SCN-071-A07 | e2e-api | `tests/e2e/assistant/intent_refusal_join_e2e_test.go` | `TBD: refusal event is queryable through trace and counter data` | `./smackerel.sh test e2e` | Yes | Persistent join regression. |

### Impact-Aware Validation

No `testImpact` or `traceContracts` config is present. Monitoring changes still require dashboard integration coverage and live-stack E2E/API proof because the scenarios are operator-visible.

### Definition of Done — Tiered Validation

- [x] Dashboard panels and refusal joins satisfy SCN-071-A06 and SCN-071-A07.
  Evidence: report.md → Test Evidence (Integration live-stack: TestAssistantIntentsDashboardHasRequiredPanels, TestAssistantIntentsDashboardQueriesCanonicalMetrics, TestAssistantIntentsDashboardRefusalPanelJoinsBothSources, TestRefusalCauseVocabularyMatchesIntentTraceColumn, TestRefusalCounterAndIntentTraceJoinByCauseLabel). Claim Source: executed 2026-06-01T21:20Z.
- [ ] Scenario-specific integration and E2E rows pass with current-session evidence.
  Uncertainty Declaration: Integration rows PASS as recorded. E2E rows `tests/e2e/assistant/intent_refusal_join_e2e_test.go` (TestIntentRefusalJoinE2E_LiveCoreExposesJoinKeyOnBothMetrics) and the e2e-ui row `web/pwa/tests/assistant_intents_dashboard.spec.ts` **are both present on disk** (correcting the stale prior note). They could not execute in this pass because the e2e stack aborts at config-validate with the missing `ASSISTANT_TRANSPORTS_HTTP_*` SST keys (spec 069 SCOPE-2 generator gap). Route to `bubbles.implement` on spec 069 to unblock e2e.
- [ ] Build Quality Gate passes and dashboard errors fail loud when export targets are unavailable.
  Uncertainty Declaration: Format/lint/e2e/artifact-lint not executed in this session; fail-loud audit of dashboard error paths not performed.

## Superseded Scopes (Do Not Execute)

None. This is the first planning artifact for this feature.