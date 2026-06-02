# Scopes: 076 Assistant Completion & Rescope Follow-Up

Single-file mode (`scopeLayout: single-file`).

Links: [spec.md](spec.md) | [design.md](design.md) | [uservalidation.md](uservalidation.md) | [report.md](report.md) | [scenario-manifest.json](scenario-manifest.json)

## Execution Outline

### Phase Order

1. **Scope 1 — Inherited-Behavior Manifest Bootstrap and Foundation Wiring (foundation):** stand up `scenario-manifest.json` with `inheritsFrom`/`replaces` links to every predecessor scenario, register fail-loud SST keys (`assistant.tools.location_normalize.*`, `assistant.tools.entity_resolve.*`, `assistant.annotation.classifier.*`, `assistant.capture_fallback.dedup_window`, `assistant.openknowledge.budgets.*`, `openknowledge.citeback.enforcement_mode`), and add the `assistant_tool_traces` + `assistant_capture_dedup` migrations + `ideas.provenance` column.
2. **Scope 2 — Open-Knowledge Agent Hardening:** ship 064 scopes 02/03/04/05/06/08/09/11 behavior under SCN-064-A02..A08.
3. **Scope 3 — Generic Micro-Tool Overlays:** ship 065 scopes 02/03/04 behavior under SCN-065-A01..A06.
4. **Scope 4 — NL Replacements and Annotation Classifier Swap:** ship 066 scopes 03/05 behavior under SCN-066-A02, SCN-066-A03, SCN-066-A08.
5. **Scope 5 — Capture Provenance, Dedup, Telemetry, and Acknowledgement Parity:** ship 074 scopes 02/03/05 behavior under SCN-074-A02..A05, A07, A11.
6. **Scope 6 — Legacy-Retirement Window Wiring and Lifecycle:** ship 075 scopes 01..05 behavior under SCN-075-A01..A09 across PWA + mobile + WhatsApp + Telegram + the spec 049 dashboard + the post-window observation cron.
7. **Scope 7 — Shared Mobile Vertical Slice and Cross-Surface Parity:** ship 073 scopes 03/04 behavior under SCN-073-A02..A07, A10, A11 (depends on Scopes 4, 5, 6 wire-schema additions).

### Validation Checkpoints

- After Scope 1: scenario-manifest invariants pass; every inherited scenario carries `inheritsFrom`; fail-loud SST tests pass; new migrations apply cleanly against the live test-stack Postgres.
- After Scope 2: SCN-064-A02..A08 executed; cite-back verifier shadow-mode logs reviewed before enforce.
- After Scope 3: SCN-065-A01..A06 executed; tool-registry canary remains green.
- After Scope 4: SCN-066-A02, A03, A08 executed; `interactionMap` deletion gated on dual-write shadow run for one release.
- After Scope 5: SCN-074-A02..A05, A07, A11 executed; cross-user dedup adversarial regression row passes.
- After Scope 6: SCN-075-A01..A09 executed; Grafana dashboard panel + alert rules deployed to the spec 049 stack; post-window cron + observation report gating active.
- After Scope 7: SCN-073-A02..A07, A10, A11 executed; cross-surface parity golden fixtures pass on web + iOS + Android + Telegram + WhatsApp.

### Planning Notes

- `.github/bubbles-project.yaml` has no `testImpact` or `traceContracts` entries — all rows are planned without impact-aware narrowing.
- Scope 1 is `foundation:true`; it ships only seams and migrations consumed by every later scope.
- Every Scope 2–7 entry preserves the predecessor's canonical Gherkin text via `scenario-manifest.json` `inheritsFrom`.

## Scope Inventory

| Scope | Name | Surfaces | Inherited Scenarios | Status |
|---|---|---|---|---|
| 1 | Inherited-Behavior Manifest Bootstrap and Foundation Wiring | scenario-manifest, SST validation, migrations (`assistant_tool_traces`, `assistant_capture_dedup`, `ideas.provenance`) | — (foundation) | Not Started |
| 2 | Open-Knowledge Agent Hardening | `openknowledge.Registry` sentinels, struct config, LLM bridge contract, cite-back verifier, agent-loop budgets, persistence with lifecycle | SCN-064-A02, A03, A04, A05, A06, A07, A08 | Not Started |
| 3 | Generic Micro-Tool Overlays | `location_normalize`, `entity_resolve`, expanded `unit_convert`/`calculator` | SCN-065-A01, A02, A03, A04, A05, A06 | Not Started |
| 4 | NL Replacements and Annotation Classifier Swap | facade routing for NL `/find` and `/rate`, `annotation.classify.v1` compiled-intent + warm-cache, `interactionMap` deletion | SCN-066-A02, A03, A08 | Not Started |
| 5 | Capture Provenance, Dedup, Telemetry, and Acknowledgement Parity | `ideas.provenance`, `assistant_capture_dedup`, IntentTrace join, cross-transport ack render | SCN-074-A02, A03, A04, A05, A07, A11 | Not Started |
| 6 | Legacy-Retirement Window Wiring and Lifecycle | PWA/mobile/WhatsApp notice renderers, dashboard panel + rolling 7-day query, threshold evaluator + alert rules, post-window observation cron | SCN-075-A01, A02, A03, A04, A05, A06, A07, A08, A09 | Not Started |
| 7 | Shared Mobile Vertical Slice and Cross-Surface Parity | iOS adapter, Android adapter, mobile retry, VoiceOver/TalkBack, parity golden fixtures (web + iOS + Android + Telegram + WhatsApp) | SCN-073-A02, A03, A04, A05, A06, A07, A10, A11 | Not Started |

---

## Scope 1: Inherited-Behavior Manifest Bootstrap and Foundation Wiring

**Status:** Not Started
**Priority:** P0
**Depends On:** None
**Scope-Kind:** runtime-behavior
**foundation:** true

### Gherkin Scenarios

```gherkin
Scenario: SCN-076-F01 — Inherited scenario manifest is complete and linked
  Given spec 076 inherits scenarios from specs 064, 065, 066, 073, 074, 075
  When `scenario-manifest.json` is validated
  Then every scenario listed in spec.md §5 has an entry with `inheritsFrom` set to the predecessor scenarioId
  And every entry carries the predecessor's canonical Gherkin text byte-for-byte

Scenario: SCN-076-F02 — Foundation SST keys fail loud
  Given any of `assistant.tools.location_normalize.*`, `assistant.tools.entity_resolve.*`, `assistant.annotation.classifier.*`, `assistant.capture_fallback.dedup_window`, `assistant.openknowledge.budgets.*`, or `openknowledge.citeback.enforcement_mode` is unset
  When the core process starts
  Then startup fails with a NO-DEFAULTS error naming the missing key

Scenario: SCN-076-F03 — Foundation migrations apply cleanly
  Given a fresh disposable test-stack Postgres
  When the migration runner applies `assistant_tool_traces`, `assistant_capture_dedup`, and `ideas.provenance`
  Then the tables and column exist with NOT NULL constraints and the documented check constraint on `ideas.provenance`
```

### Implementation Plan

- Author `specs/076-assistant-completion-rescope/scenario-manifest.json` with one entry per inherited SCN, each carrying `inheritsFrom: { spec: "specs/NNN-...", scenarioId: "SCN-..." }` and the predecessor's exact Gherkin text.
- Extend `internal/config/` SST validation for each foundation key listed above; reject empty values with a named error.
- Add migrations under `internal/db/migrations/`:
  - `0NN_assistant_tool_traces.sql` (turn_id, tool_name, payload_redacted, lifecycle_state, created_at)
  - `0NN_assistant_capture_dedup.sql` (user_id, normalized_text_hash, time_bucket, created_at; UNIQUE on full key)
  - `0NN_ideas_provenance.sql` (ALTER TABLE ideas ADD COLUMN provenance TEXT NOT NULL CHECK (provenance IN ('explicit','fallback')))
- No code in `internal/assistant/legacyretirement/`, `internal/assistant/openknowledge/`, or transport renderers is modified in this scope.

### Shared Infrastructure Impact Sweep

| Shared Surface | Downstream Contract | Canary Validation |
|---|---|---|
| `assistant_conversations` row family | New sibling tables must not change row-family semantics | TP-076-01-03 migration canary |
| Config loader | Foundation keys must fail loud uniformly with existing keys | TP-076-01-02 fail-loud unit |
| Scenario-manifest schema | `inheritsFrom`/`replaces` must be additive and v1-compatible | TP-076-01-01 manifest lint |

### Change Boundary

- **Allowed file families:** `specs/076-assistant-completion-rescope/**`, `internal/config/**`, `internal/db/migrations/**`, focused unit + integration tests for the above.
- **Excluded surfaces:** transport renderers, `internal/assistant/openknowledge/`, `internal/assistant/legacyretirement/`, `internal/agent/tools/microtools/`, web/mobile clients, docs.

### Test Plan

| Row | Scenario | Category | File/Location | Planned test title | Command | Live System |
|---|---|---|---|---|---|---|
| TP-076-01-01 | SCN-076-F01 | unit | `internal/manifest/scenario_manifest_test.go` | `TestScenario076Manifest_InheritsFromLinksComplete` | `./smackerel.sh test unit` | No |
| TP-076-01-02 | SCN-076-F02 | unit | `internal/config/spec_076_foundation_test.go` | `TestSpec076FoundationKeysFailLoud` | `./smackerel.sh test unit` | No |
| TP-076-01-03 | SCN-076-F03 | integration | `tests/integration/db/spec_076_migrations_test.go` | `TestSpec076FoundationMigrationsApplyCleanly` | `./smackerel.sh test integration` | Yes |
| TP-076-01-03R | SCN-076-F03 | Regression E2E | `tests/e2e/foundation/spec_076_migrations_e2e_test.go` | `Regression E2E: TestSpec076MigrationsSurviveFreshStack` | `./smackerel.sh test e2e` | Yes |

### Definition of Done

- [ ] SCN-076-F01 — scenario-manifest is complete and `inheritsFrom`-linked for every spec.md §5 scenario.
- [ ] SCN-076-F02 — every foundation SST key fails loud at startup when unset.
- [ ] SCN-076-F03 — `assistant_tool_traces`, `assistant_capture_dedup`, and `ideas.provenance` apply cleanly against a fresh disposable Postgres.
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior (TP-076-01-03R).
- [ ] Broader E2E regression suite passes.
- [ ] Independent canary suite for shared fixture/bootstrap contracts passes before broad suite reruns (TP-076-01-03).
- [ ] Rollback or restore path for shared infrastructure changes is documented and verified.
- [ ] Change Boundary is respected and zero excluded file families were changed.
- [ ] Build Quality Gate: lint, format, artifact-lint, traceability-guard all clean.

---

## Scope 2: Open-Knowledge Agent Hardening

**Status:** Not Started
**Priority:** P0
**Depends On:** Scope 1
**Scope-Kind:** runtime-behavior

### Gherkin Scenarios

Inherits canonical text from spec 064 via `scenario-manifest.json`:

- SCN-064-A02 — Unit/math conversion via deterministic tool
- SCN-064-A03 — Hybrid internal-graph + web answer
- SCN-064-A04 — Refusal-with-capture on per-turn budget exhaustion
- SCN-064-A05 — Refusal-with-capture on tool failure
- SCN-064-A06 — Refusal on fabricated source
- SCN-064-A07 — Operator disables `web_search`
- SCN-064-A08 — Per-user monthly budget exceeded

### UI Scenario Matrix

| Scenario | Preconditions | Steps | Expected | Test Type | Evidence |
|---|---|---|---|---|---|
| SCN-064-A02 | unit_convert registered | ask "what is 12 miles in km" | cited tool answer | e2e-api | report.md#scope-2 |
| SCN-064-A06 | source fabricator harness | force LLM to cite missing URL | response flips to refusal-with-capture | e2e-api | report.md#scope-2 |

### Implementation Plan

- Add typed sentinels (`ErrToolNotRegistered`, `ErrToolDisabled`, `ErrBudgetExhausted`) on `internal/assistant/openknowledge/registry/`.
- Replace ad-hoc env reads with `OpenKnowledgeConfig` struct loaded under `assistant.openknowledge.*`; struct fail-loud on missing field.
- Add LLM bridge contract test pinning Go ↔ `ml/app/routes/chat.py` request/response shape.
- Extend `calculator` and `unit_convert` with the adversarial cases planned in 064 scope 05 (locale separators, mixed units, precedence, overflow, divide-by-zero).
- Add `internal/assistant/openknowledge/internalretrieval/tool.go` implementing the `microtools.Envelope` shape.
- Add `internal/assistant/openknowledge/citeback/` package; agent loop calls verifier before emitting answer; mismatch → refusal-with-capture.
- Enforce per-turn step budget + per-user monthly token budget at the agent boundary.
- Persist tool traces to `assistant_tool_traces` (created in Scope 1) with `lifecycle_state`.

### Test Plan

| Row | Scenario | Category | File/Location | Planned test title | Command | Live System |
|---|---|---|---|---|---|---|
| TP-076-02-01 | SCN-064-A02 | unit | `internal/agent/tools/microtools/unit_convert_adversarial_test.go` | `TestUnitConvert_AdversarialCases` | `./smackerel.sh test unit` | No |
| TP-076-02-02 | SCN-064-A03 | integration | `tests/integration/openknowledge/hybrid_answer_test.go` | `TestOpenKnowledge_HybridInternalAndWeb` | `./smackerel.sh test integration` | Yes |
| TP-076-02-03 | SCN-064-A04 | unit | `internal/assistant/openknowledge/agent/budget_test.go` | `TestAgent_PerTurnBudgetExhaustionRefusesWithCapture` | `./smackerel.sh test unit` | No |
| TP-076-02-04 | SCN-064-A05 | integration | `tests/integration/openknowledge/tool_failure_test.go` | `TestAgent_ToolFailureRefusesWithCapture` | `./smackerel.sh test integration` | Yes |
| TP-076-02-05 | SCN-064-A06 | unit | `internal/assistant/openknowledge/citeback/verifier_test.go` | `TestCiteback_FabricatedSourceFlipsToRefusal` | `./smackerel.sh test unit` | No |
| TP-076-02-06 | SCN-064-A07 | integration | `tests/integration/openknowledge/web_search_disabled_test.go` | `TestAgent_WebSearchDisabledFallsBack` | `./smackerel.sh test integration` | Yes |
| TP-076-02-07 | SCN-064-A08 | integration | `tests/integration/openknowledge/monthly_budget_test.go` | `TestAgent_PerUserMonthlyBudgetExceeded` | `./smackerel.sh test integration` | Yes |
| TP-076-02-08 | SCN-064-A02..A08 | Regression E2E | `tests/e2e/openknowledge/open_knowledge_e2e_test.go` | `Regression E2E: TestOpenKnowledgeAgent_FullScenarioMatrix` | `./smackerel.sh test e2e` | Yes |
| TP-076-02-09 | hot path | stress | `tests/stress/openknowledge_p95_test.go` | `TestOpenKnowledge_P95SLAUnderToolLoad` | `./smackerel.sh test stress` | Yes |

### Definition of Done

- [ ] SCN-064-A02..A08 each executed against their planned test row.
- [ ] Cite-back verifier shipped behind `openknowledge.citeback.enforcement_mode` shadow → enforce.
- [ ] Tool traces persisted to `assistant_tool_traces` with `lifecycle_state` set.
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior (TP-076-02-08).
- [ ] Broader E2E regression suite passes.
- [ ] Build Quality Gate: lint, format, artifact-lint clean.

---

## Scope 3: Generic Micro-Tool Overlays

**Status:** Not Started
**Priority:** P1
**Depends On:** Scope 1
**Scope-Kind:** runtime-behavior

### Gherkin Scenarios

Inherits from spec 065:

- SCN-065-A01 — `location_normalize` returns canonical place for unambiguous phrase
- SCN-065-A02 — `location_normalize` returns disambiguation list for ambiguous phrase
- SCN-065-A03 — `location_normalize` overlay rules apply project aliases without leaking PII
- SCN-065-A04 — `unit_convert` covers adversarial cases (shared with TP-076-02-01)
- SCN-065-A05 — `calculator` covers adversarial cases
- SCN-065-A06 — `entity_resolve` returns entity or disambiguation list

### Implementation Plan

- Implement `location_normalize` in `internal/agent/tools/microtools/location/`; SST keys `assistant.tools.location_normalize.provider`, `assistant.tools.location_normalize.overlays.*`.
- Extend `calculator` adversarial test surface (shares targets with Scope 2 TP-076-02-01).
- Implement `entity_resolve` in `internal/agent/tools/microtools/entity/`; queries `entities`/`connections` tables.
- Register all three with the facade tool registry; canary test ensures existing scenario tools still validate.

### Test Plan

| Row | Scenario | Category | File/Location | Planned test title | Command | Live System |
|---|---|---|---|---|---|---|
| TP-076-03-01 | SCN-065-A01 | unit | `internal/agent/tools/microtools/location/normalize_test.go` | `TestLocationNormalize_CanonicalCase` | `./smackerel.sh test unit` | No |
| TP-076-03-02 | SCN-065-A02 | unit | `internal/agent/tools/microtools/location/normalize_test.go` | `TestLocationNormalize_AmbiguousReturnsList` | `./smackerel.sh test unit` | No |
| TP-076-03-03 | SCN-065-A03 | integration | `tests/integration/microtools/location_overlay_test.go` | `TestLocationOverlay_AppliesAliasWithoutPIILeakage` | `./smackerel.sh test integration` | Yes |
| TP-076-03-04 | SCN-065-A05 | unit | `internal/agent/tools/microtools/calculator/adversarial_test.go` | `TestCalculator_AdversarialCases` | `./smackerel.sh test unit` | No |
| TP-076-03-05 | SCN-065-A06 | integration | `tests/integration/microtools/entity_resolve_test.go` | `TestEntityResolve_GraphBackedResolution` | `./smackerel.sh test integration` | Yes |
| TP-076-03-06 | SCN-065-A01..A06 | Regression E2E | `tests/e2e/microtools/overlays_e2e_test.go` | `Regression E2E: TestMicroToolOverlays_FullMatrix` | `./smackerel.sh test e2e` | Yes |

### Definition of Done

- [ ] SCN-065-A01..A06 each executed.
- [ ] Tool-registry canary remains green.
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior (TP-076-03-06).
- [ ] Broader E2E regression suite passes.
- [ ] Build Quality Gate: lint, format, artifact-lint clean.

---

## Scope 4: NL Replacements and Annotation Classifier Swap

**Status:** Not Started
**Priority:** P1
**Depends On:** Scope 1
**Scope-Kind:** runtime-behavior

### Gherkin Scenarios

Inherits from spec 066:

- SCN-066-A02 — NL replaces `/find`
- SCN-066-A03 — NL replaces `/rate` via disambiguation
- SCN-066-A08 — annotation classification uses LLM extraction

### Consumer Impact Sweep (annotation classifier swap)

| Consumer | Touched? | Regression Row |
|---|---|---|
| `internal/annotation/interaction_map.go` | DELETED | TP-076-04-04 stale-reference scan |
| `internal/annotation/` callers | Repointed to `annotation.classify.v1` | TP-076-04-03 |
| Docs referencing `interactionMap` | Updated | TP-076-04-04 |
| ML eval fixtures | Updated | TP-076-04-03 |

### Implementation Plan

- Add facade routing rule mapping NL "find me notes about X" to the existing internal-retrieval path; live regression row.
- Add facade routing rule mapping NL "rate this" (ambiguous) into spec 061 disambiguation flow.
- Swap `annotation.interactionMap` for the `annotation.classify.v1` compiled-intent scenario; warm-cache consistency wired into facade cache; fail-loud `assistant.annotation.classifier.*` SST keys (already added in Scope 1).
- Dual-write shadow run for one release before deleting `interaction_map.go`.
- Stale-reference scan ensures zero first-party `interactionMap` references remain.

### Test Plan

| Row | Scenario | Category | File/Location | Planned test title | Command | Live System |
|---|---|---|---|---|---|---|
| TP-076-04-01 | SCN-066-A02 | e2e-api | `tests/e2e/assistant/nl_find_replacement_test.go` | `TestNLReplaceFind_LiveSameAsLegacyFind` | `./smackerel.sh test e2e` | Yes |
| TP-076-04-02 | SCN-066-A03 | e2e-api | `tests/e2e/assistant/nl_rate_disambig_test.go` | `TestNLReplaceRate_EntersDisambiguation` | `./smackerel.sh test e2e` | Yes |
| TP-076-04-03 | SCN-066-A08 | integration | `tests/integration/annotation/classify_v1_test.go` | `TestAnnotationClassifyV1_WarmCacheConsistency` | `./smackerel.sh test integration` | Yes |
| TP-076-04-04 | SCN-066-A08 | unit | `internal/annotation/stale_reference_test.go` | `TestNoStaleInteractionMapReferences` | `./smackerel.sh test unit` | No |
| TP-076-04-05 | SCN-066-A02, A03, A08 | Regression E2E | `tests/e2e/assistant/nl_replacement_e2e_test.go` | `Regression E2E: TestNLReplacementsAndAnnotationClassifier` | `./smackerel.sh test e2e` | Yes |

### Definition of Done

- [ ] SCN-066-A02, A03, A08 each executed.
- [ ] `internal/annotation/interaction_map.go` deleted; stale-reference scan green.
- [ ] Dual-write shadow run recorded for one release before deletion.
- [ ] Consumer impact sweep complete (docs + ML eval fixtures updated).
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior (TP-076-04-05).
- [ ] Broader E2E regression suite passes.
- [ ] Build Quality Gate: lint, format, artifact-lint clean.

---

## Scope 5: Capture Provenance, Dedup, Telemetry, and Acknowledgement Parity

**Status:** Not Started
**Priority:** P1
**Depends On:** Scope 1
**Scope-Kind:** runtime-behavior

### Gherkin Scenarios

Inherits from spec 074:

- SCN-074-A02 — Explicit capture is provenance-distinct
- SCN-074-A03 — Same-user same-text within dedup window dedupes
- SCN-074-A04 — Same-user same-text outside dedup window does not dedup
- SCN-074-A05 — Cross-user dedup is forbidden
- SCN-074-A07 — Counter and IntentTrace carry the capture link
- SCN-074-A11 — Acknowledgement shape is identical across transports

### Implementation Plan

- Use `ideas.provenance` column from Scope 1; explicit-capture seam supersedes prior fallback Idea while preserving trace.
- Use `assistant_capture_dedup` from Scope 1; key `(user_id, normalized_text_hash, time_bucket)`; SST `assistant.capture_fallback.dedup_window`.
- Adversarial regression: two distinct users with identical text MUST produce two Ideas (cross-user isolation by schema).
- Add IntentTrace link to `smackerel_capture_as_fallback_total` via `intent_trace_id`.
- Implement capture-ack renderer on PWA + mobile + WhatsApp using the same render-descriptor payload as Telegram.

### Test Plan

| Row | Scenario | Category | File/Location | Planned test title | Command | Live System |
|---|---|---|---|---|---|---|
| TP-076-05-01 | SCN-074-A02 | integration | `tests/integration/capture/provenance_test.go` | `TestCapture_ExplicitVsFallbackProvenance` | `./smackerel.sh test integration` | Yes |
| TP-076-05-02 | SCN-074-A03 | integration | `tests/integration/capture/dedup_window_test.go` | `TestCaptureDedup_WithinWindowDedupes` | `./smackerel.sh test integration` | Yes |
| TP-076-05-03 | SCN-074-A04 | integration | `tests/integration/capture/dedup_window_test.go` | `TestCaptureDedup_OutsideWindowDoesNotDedup` | `./smackerel.sh test integration` | Yes |
| TP-076-05-04 | SCN-074-A05 | integration | `tests/integration/capture/cross_user_isolation_test.go` | `TestCaptureDedup_CrossUserNeverDedupes_Adversarial` | `./smackerel.sh test integration` | Yes |
| TP-076-05-05 | SCN-074-A07 | unit | `internal/assistant/metrics/capture_fallback_intent_trace_test.go` | `TestCaptureFallback_IntentTraceLinkPresent` | `./smackerel.sh test unit` | No |
| TP-076-05-06 | SCN-074-A11 | e2e-ui | `tests/e2e/transports/capture_ack_parity_test.go` | `TestCaptureAckParity_AcrossAllTransports` | `./smackerel.sh test e2e` | Yes |
| TP-076-05-07 | SCN-074-A02..A05, A07, A11 | Regression E2E | `tests/e2e/capture/capture_fallback_e2e_test.go` | `Regression E2E: TestCaptureFallback_FullScenarioMatrix` | `./smackerel.sh test e2e` | Yes |

### Definition of Done

- [ ] SCN-074-A02..A05, A07, A11 each executed.
- [ ] Adversarial cross-user regression (TP-076-05-04) passes — two users with identical text produce two Ideas.
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior (TP-076-05-07).
- [ ] Broader E2E regression suite passes.
- [ ] Build Quality Gate: lint, format, artifact-lint clean.

---

## Scope 6: Legacy-Retirement Window Wiring and Lifecycle

**Status:** Not Started
**Priority:** P1
**Depends On:** Scope 1
**Scope-Kind:** runtime-behavior

### Gherkin Scenarios

Inherits from spec 075:

- SCN-075-A01 — First retired-command invocation shows one notice and serves the intent
- SCN-075-A02 — Second invocation does not re-notify
- SCN-075-A03 — Different retired command produces its own one-time notice
- SCN-075-A04 — Residual telemetry counts invocations per (command, user_bucket)
- SCN-075-A05 — Rollback threshold pauses the window automatically
- SCN-075-A06 — Resuming the window resets the consecutive-day counter
- SCN-075-A07 — Window-closed response is canonical unknown-command response
- SCN-075-A08 — Post-window observation confirms zero legacy-handler invocations
- SCN-075-A09 — Dedup ledger survives across sessions and devices

### Implementation Plan

- Wire `SQLNoticeLedger.MarkShown`/`Dedup` through the facade for PWA, mobile, WhatsApp (Telegram already shipped under spec 075 SCOPE-06.4).
- Ship the spec 049 dashboard panel + rolling 7-day query for `legacy_command_residual_total`.
- Wire the threshold evaluator + alert rules; on threshold breach, `SQLPauseStateStore.Pause()` is invoked automatically; resume resets the consecutive-day counter.
- Add the post-window cron that runs `SQLObservationReport.Generate(windowID)` and gates legacy-handler deletion on `retired_handler_invocations == 0`.

### Test Plan

| Row | Scenario | Category | File/Location | Planned test title | Command | Live System |
|---|---|---|---|---|---|---|
| TP-076-06-01 | SCN-075-A01 | e2e-api | `tests/e2e/legacy_retirement/notice_first_invocation_test.go` | `TestRetirement_FirstInvocationShowsOneNoticeAndServesIntent` | `./smackerel.sh test e2e` | Yes |
| TP-076-06-02 | SCN-075-A02 | integration | `tests/integration/legacy_retirement/dedup_test.go` | `TestRetirement_SecondInvocationDoesNotRenotify` | `./smackerel.sh test integration` | Yes |
| TP-076-06-03 | SCN-075-A03 | integration | `tests/integration/legacy_retirement/per_command_dedup_test.go` | `TestRetirement_DifferentCommandProducesOwnNotice` | `./smackerel.sh test integration` | Yes |
| TP-076-06-04 | SCN-075-A04 | integration | `tests/integration/legacy_retirement/telemetry_test.go` | `TestRetirement_ResidualTelemetryCountsPerCommandAndBucket` | `./smackerel.sh test integration` | Yes |
| TP-076-06-05 | SCN-075-A05 | integration | `tests/integration/legacy_retirement/auto_pause_test.go` | `TestRetirement_ThresholdAutoPausesWindow` | `./smackerel.sh test integration` | Yes |
| TP-076-06-06 | SCN-075-A06 | integration | `tests/integration/legacy_retirement/resume_test.go` | `TestRetirement_ResumeResetsConsecutiveDayCounter` | `./smackerel.sh test integration` | Yes |
| TP-076-06-07 | SCN-075-A07 | e2e-api | `tests/e2e/legacy_retirement/closed_window_test.go` | `TestRetirement_ClosedWindowReturnsCanonicalResponse` | `./smackerel.sh test e2e` | Yes |
| TP-076-06-08 | SCN-075-A08 | integration | `tests/integration/legacy_retirement/observation_report_test.go` | `TestRetirement_ZeroInvocationGateBlocksDeletion` | `./smackerel.sh test integration` | Yes |
| TP-076-06-09 | SCN-075-A09 | e2e-ui | `tests/e2e/transports/dedup_cross_transport_test.go` | `TestRetirement_DedupSurvivesAcrossTransports` | `./smackerel.sh test e2e` | Yes |
| TP-076-06-10 | SCN-075-A01..A09 | Regression E2E | `tests/e2e/legacy_retirement/retirement_e2e_test.go` | `Regression E2E: TestLegacyRetirement_FullScenarioMatrix` | `./smackerel.sh test e2e` | Yes |

### Definition of Done

- [ ] SCN-075-A01..A09 each executed.
- [ ] Spec 049 dashboard panel + alert rules deployed against the live monitoring stack.
- [ ] Post-window observation cron operational; final legacy-handler deletion gated on zero-invocation report.
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior (TP-076-06-10).
- [ ] Broader E2E regression suite passes.
- [ ] Build Quality Gate: lint, format, artifact-lint clean.

---

## Scope 7: Shared Mobile Vertical Slice and Cross-Surface Parity

**Status:** Not Started
**Priority:** P1
**Depends On:** Scopes 4, 5, 6 (consumes their render payloads for parity tests)
**Scope-Kind:** runtime-behavior

### Gherkin Scenarios

Inherits from spec 073:

- SCN-073-A02 — Shared mobile client uses generated types from golden schema
- SCN-073-A03 — Transient network failure retries with same `transport_message_id`
- SCN-073-A04 — Disambiguation prompt renders and round-trips on web and mobile
- SCN-073-A05 — Confirm card renders identically and round-trips
- SCN-073-A06 — Capture-as-fallback acknowledgement is identical to other transports
- SCN-073-A07 — No client-side scenario logic exists
- SCN-073-A10 — Shared mobile client meets VoiceOver and TalkBack accessibility floor
- SCN-073-A11 — Missing backend base URL fails loud at build/start time

### Shared Infrastructure Impact Sweep

| Shared Surface | Downstream Contract | Canary Validation |
|---|---|---|
| `render-descriptor-v1` | Mobile renderers must consume same payload as PWA / WhatsApp / Telegram | TP-076-07-04 cross-surface parity fixture |
| `transport_message_id` | Mobile retry must reuse the existing idempotency contract | TP-076-07-02 |
| Mobile build pipeline | Missing `SMACKEREL_API_BASE_URL` must fail at build, not at runtime | TP-076-07-08 |

### Change Boundary

- **Allowed file families:** `clients/mobile/**`, mobile build pipeline configs, fixture-based parity tests under `tests/e2e/transports/`.
- **Excluded surfaces:** server-side facade code (already shipped); `internal/assistant/` server-side changes for parity.

### Implementation Plan

- Wire Dart shared core (shipped under 073 scope 01) into an iOS adapter and an Android adapter using generated render-descriptor types.
- Implement mobile retry layer that reuses the same `transport_message_id`.
- Add disambiguation + confirm + capture-ack + source-detail renderers on iOS + Android.
- Add static scan asserting zero client-side scenario branching.
- Wire VoiceOver + TalkBack a11y harness.
- Fail-loud build/start when `SMACKEREL_API_BASE_URL` is unset.

### Test Plan

| Row | Scenario | Category | File/Location | Planned test title | Command | Live System |
|---|---|---|---|---|---|---|
| TP-076-07-01 | SCN-073-A02 | unit | `clients/mobile/shared/test/render_descriptor_test.dart` | `RenderDescriptor_UsesGeneratedTypes` | `flutter test` | No |
| TP-076-07-02 | SCN-073-A03 | integration | `tests/integration/mobile/retry_idempotency_test.go` | `TestMobileRetry_ReusesTransportMessageId` | `./smackerel.sh test integration` | Yes |
| TP-076-07-03 | SCN-073-A04 | e2e-ui | `tests/e2e/transports/disambig_parity_test.go` | `TestDisambigParity_WebMobileTelegramWhatsApp` | `./smackerel.sh test e2e` | Yes |
| TP-076-07-04 | SCN-073-A05 | e2e-ui | `tests/e2e/transports/confirm_card_parity_test.go` | `TestConfirmCardParity_AcrossAllTransports` | `./smackerel.sh test e2e` | Yes |
| TP-076-07-05 | SCN-073-A06 | e2e-ui | `tests/e2e/transports/capture_ack_parity_test.go` | `TestCaptureAckParity_AcrossAllTransports` (shared with TP-076-05-06) | `./smackerel.sh test e2e` | Yes |
| TP-076-07-06 | SCN-073-A07 | unit | `clients/mobile/shared/test/no_client_scenario_branching_test.dart` | `NoClientScenarioBranches_StaticScan` | `flutter test` | No |
| TP-076-07-07 | SCN-073-A10 | e2e-ui | `tests/e2e/mobile/a11y_floor_test.go` | `TestMobileA11yFloor_VoiceOverAndTalkBack` | `./smackerel.sh test e2e` | Yes |
| TP-076-07-08 | SCN-073-A11 | unit | `clients/mobile/shared/test/config_fail_loud_test.dart` | `ConfigFailLoud_MissingBaseUrl` | `flutter test` | No |
| TP-076-07-09 | SCN-073-A02..A11 | Regression E2E | `tests/e2e/mobile/mobile_e2e_test.go` | `Regression E2E: TestMobileVerticalSlice_FullScenarioMatrix` | `./smackerel.sh test e2e` | Yes |

### Definition of Done

- [ ] SCN-073-A02..A07, A10, A11 each executed.
- [ ] Cross-surface parity golden fixtures pass on web + iOS + Android + Telegram + WhatsApp.
- [ ] Static scan confirms zero client-side scenario branching.
- [ ] Mobile build fails loud on missing backend base URL.
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior (TP-076-07-09).
- [ ] Broader E2E regression suite passes.
- [ ] Independent canary suite for shared fixture/bootstrap contracts passes before broad suite reruns (TP-076-07-03).
- [ ] Rollback or restore path for shared infrastructure changes documented and verified.
- [ ] Change Boundary respected and zero excluded file families changed.
- [ ] Build Quality Gate: lint, format, artifact-lint clean.
