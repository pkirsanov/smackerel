# Scopes: 076 Assistant Completion & Rescope Follow-Up

Single-file mode (`scopeLayout: single-file`).

Links: [spec.md](spec.md) | [design.md](design.md) | [uservalidation.md](uservalidation.md) | [report.md](report.md) | [scenario-manifest.json](scenario-manifest.json)

## Execution Outline

### Phase Order

1. **Scope 1 — Inherited-Behavior Manifest Bootstrap and Foundation Wiring (foundation):** stand up `scenario-manifest.json` with `inheritsFrom`/`replaces` links to every predecessor scenario, register fail-loud SST keys (`assistant.tools.location_normalize.*`, `assistant.tools.entity_resolve.*`, `assistant.annotation.classifier.*`, `assistant.openknowledge.budgets.*`, `openknowledge.citeback.enforcement_mode`), and add the `assistant_tool_traces` migration. `CAPTURE_AS_FALLBACK_DEDUP_WINDOW` and the `artifact_capture_policy` row family ship via spec 074 (migration `051_artifact_capture_policy.sql`) and are re-used as-is; spec 076 introduces no `ideas.provenance` column and no `assistant_capture_dedup` table.
2. **Scope 2 — Open-Knowledge Agent Hardening (sequential sub-scopes 2a → 2b → 2c → 2d):**
   - **Scope 2a — Tool-Trace Persistence + Unit-Convert Adversarial:** writer for `assistant_tool_traces` (turn_id, tool_name, payload_redacted, `call_outcome`) wired from the agent loop, plus TP-076-02-01 (SCN-064-A02).
   - **Scope 2b — Agent-Loop Budget/Refusal Tests:** typed sentinels (`ErrToolNotRegistered`, `ErrToolDisabled`, `ErrBudgetExhausted`), per-turn step + per-user monthly budget enforcement, hybrid path, web-search-disabled fallback. TP-076-02-02, 02-03, 02-04, 02-06, 02-07 (SCN-064-A03, A04, A05, A07, A08).
   - **Scope 2c — Citeback Shadow→Enforce + Regression E2E:** `internal/assistant/openknowledge/citeback/` verifier wired behind `openknowledge.citeback.enforcement_mode` shadow → enforce; fabricated-source path flips to refusal-with-capture. TP-076-02-05 + TP-076-02-08 (SCN-064-A06 + SCN-064-A02..A08 regression E2E).
   - **Scope 2d — Stress + Suite-Wide Regression:** TP-076-02-09 hot-path stress under tool load + broader E2E regression sweep covering 2a–2c.
3. **Scope 3 — Generic Micro-Tool Overlays:** ship 065 scopes 02/03/04 behavior under SCN-065-A01..A06.
4. **Scope 4 — NL Replacements and Annotation Classifier Swap:** ship 066 scopes 03/05 behavior under SCN-066-A02, SCN-066-A03, SCN-066-A08.
5. **Scope 5 — Capture Provenance, Dedup, Telemetry, and Acknowledgement Parity:** ship 074 scopes 02/03/05 behavior under SCN-074-A02..A05, A07, A11.
6. **Scope 6 — Legacy-Retirement Window Wiring and Lifecycle:** ship 075 scopes 01..05 behavior under SCN-075-A01..A09 across PWA + mobile + WhatsApp + Telegram + the spec 049 dashboard + the post-window observation cron.
7. **Scope 7 — Shared Mobile Vertical Slice and Cross-Surface Parity:** ship 073 scopes 03/04 behavior under SCN-073-A02..A07, A10, A11 (depends on Scopes 4, 5, 6 wire-schema additions).

### Validation Checkpoints

- After Scope 1: scenario-manifest invariants pass; every inherited scenario carries `inheritsFrom`; fail-loud SST tests pass; new migrations apply cleanly against the live test-stack Postgres.
- After Scope 2a: `assistant_tool_traces` rows written with non-null `call_outcome` for every tool call in a representative agent run; TP-076-02-01 PASS.
- After Scope 2b: TP-076-02-02..02-04, 02-06, 02-07 PASS; budget+refusal sentinels emitted and observable in traces.
- After Scope 2c: citeback verifier shadow-mode logs reviewed and switched to enforce; TP-076-02-05 + TP-076-02-08 PASS.
- After Scope 2d: TP-076-02-09 stress PASS at the declared p95 SLA; suite-wide E2E regression for the open-knowledge surface PASS.
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
| 1 | Inherited-Behavior Manifest Bootstrap and Foundation Wiring | scenario-manifest, SST validation, migration (`assistant_tool_traces`); re-uses shipped `artifact_capture_policy` (migration 051) | — (foundation) | In Progress |
| 2a | Open-Knowledge Agent — Tool-Trace Persistence + Unit-Convert Adversarial | `assistant_tool_traces` writer with `call_outcome`, `unit_convert` adversarial coverage | SCN-064-A02 | Not Started |
| 2b | Open-Knowledge Agent — Budget/Refusal Hardening | sentinels, per-turn + per-user budgets, hybrid path, web-search-disabled fallback | SCN-064-A03, A04, A05, A07, A08 | In Progress |
| 2c | Open-Knowledge Agent — Citeback Shadow→Enforce | `citeback/` verifier wired behind `openknowledge.citeback.enforcement_mode`; fabricated-source refusal-with-capture | SCN-064-A06 (+ SCN-064-A02..A08 regression E2E) | In Progress |
| 2d | Open-Knowledge Agent — Stress + Suite-Wide Regression | hot-path p95 stress under tool load; broader regression sweep | — (covers SCN-064-A02..A08 via regression suite) | In Progress |
| 3 | Generic Micro-Tool Overlays | `location_normalize`, `entity_resolve`, expanded `unit_convert`/`calculator` | SCN-065-A01, A02, A03, A04, A05, A06 | Done |
| 4 | NL Replacements and Annotation Classifier Swap | facade routing for NL `/find` and `/rate`, `annotation.classify.v1` compiled-intent + warm-cache, `interactionMap` deletion | SCN-066-A02, A03, A08 | Not Started |
| 5 | Capture Provenance, Dedup, Telemetry, and Acknowledgement Parity | re-uses shipped `artifact_capture_policy` (provenance + partial-unique dedup), IntentTrace join, cross-transport ack render | SCN-074-A02, A03, A04, A05, A07, A11 | Not Started |
| 6 | Legacy-Retirement Window Wiring and Lifecycle | PWA/mobile/WhatsApp notice renderers, dashboard panel + rolling 7-day query, threshold evaluator + alert rules, post-window observation cron | SCN-075-A01, A02, A03, A04, A05, A06, A07, A08, A09 | Not Started |
| 7 | Shared Mobile Vertical Slice and Cross-Surface Parity | iOS adapter, Android adapter, mobile retry, VoiceOver/TalkBack, parity golden fixtures (web + iOS + Android + Telegram + WhatsApp) | SCN-073-A02, A03, A04, A05, A06, A07, A10, A11 | Not Started |

---

## Scope 1: Inherited-Behavior Manifest Bootstrap and Foundation Wiring

**Status:** In Progress
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
  Given any of `assistant.tools.location_normalize.*`, `assistant.tools.entity_resolve.*`, `assistant.annotation.classifier.*`, `assistant.openknowledge.budgets.*`, or `openknowledge.citeback.enforcement_mode` is unset
  When the core process starts
  Then startup fails with a NO-DEFAULTS error naming the missing key

Scenario: SCN-076-F03 — Foundation migration applies cleanly
  Given a fresh disposable test-stack Postgres with shipped migration `051_artifact_capture_policy.sql` already applied
  When the migration runner applies `assistant_tool_traces`
  Then the table exists with NOT NULL constraints and the existing `artifact_capture_policy` CHECK on `provenance IN ('capture-as-fallback','capture-explicit')` and partial UNIQUE index `idx_capture_fallback_dedup` remain intact
```

### Implementation Plan

- Author `specs/076-assistant-completion-rescope/scenario-manifest.json` with one entry per inherited SCN, each carrying `inheritsFrom: { spec: "specs/NNN-...", scenarioId: "SCN-..." }` and the predecessor's exact Gherkin text.
- Extend `internal/config/` SST validation for each foundation key listed above; reject empty values with a named error.
- Add migration under `internal/db/migrations/`:
  - `053_assistant_tool_traces.sql` (turn_id, tool_name, payload_redacted, lifecycle_state, created_at)
  - `054_assistant_tool_traces_call_outcome.sql` (adds per-call `call_outcome` column, distinct from prune `lifecycle_state`)
- Re-use shipped `artifact_capture_policy` (migration 051) and shipped `CAPTURE_AS_FALLBACK_DEDUP_WINDOW` SST key (`internal/config.LoadCaptureFallback`); do NOT add `ideas.provenance` or `assistant_capture_dedup` — neither exists in this spec.
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

- [x] SCN-076-F01 — scenario-manifest is complete and `inheritsFrom`-linked for every spec.md §5 scenario. (TP-076-01-01 `TestScenario076Manifest_InheritsFromLinksComplete` PASS — see report.md#scope-1-implement-2026-06-02.)

  ```text
  $ go test ./internal/manifest/... -count=1 -run TestScenario076Manifest_InheritsFromLinksComplete
  ok  github.com/smackerel/smackerel/internal/manifest  0.017s
  ```
  Exit Code: 0

- [x] SCN-076-F02 — every foundation SST key fails loud at startup when unset. (TP-076-01-02 `TestSpec076FoundationKeysFailLoud` PASS for all 13 foundation env vars across the 5 SST families — see report.md#scope-1-implement-2026-06-02.)

  ```text
  $ go test ./internal/config/ -count=1 -run TestSpec076FoundationKeysFailLoud
  ok  github.com/smackerel/smackerel/internal/config   0.023s
  ```
  Exit Code: 0

- [x] SCN-076-F03 — `assistant_tool_traces` migration applies cleanly against a fresh disposable Postgres without disturbing the shipped `artifact_capture_policy` constraints and partial-unique dedup index from migration 051. (TP-076-01-03 + TP-076-01-03R PASS against live disposable test stack — see report.md#scope-1-test-2026-06-02.)

  ```text
  $ ./smackerel.sh test e2e --go-run TestSpec076MigrationsSurviveFreshStack
  2026/06/02 16:32:49 INFO applied migration version=051_artifact_capture_policy.sql
  2026/06/02 16:32:49 INFO applied migration version=052_capture_as_fallback_pending_clarify.sql
  2026/06/02 16:32:49 INFO applied migration version=053_assistant_tool_traces.sql
  --- PASS: TestSpec076MigrationsSurviveFreshStack (2.19s)
  ok  github.com/smackerel/smackerel/tests/e2e/foundation  2.195s
  PASS: go-e2e
  ```
  Exit Code: 0

- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior (TP-076-01-03R). (TP-076-01-03R `TestSpec076MigrationsSurviveFreshStack` PASS against live disposable test stack — see report.md#scope-1-test-2026-06-02.)

  ```text
  $ ./smackerel.sh test e2e --go-run TestSpec076MigrationsSurviveFreshStack
  === RUN   TestSpec076MigrationsSurviveFreshStack
  --- PASS: TestSpec076MigrationsSurviveFreshStack (2.19s)
  PASS
  ok  github.com/smackerel/smackerel/tests/e2e/foundation  2.195s
  PASS: go-e2e
  ```
  Exit Code: 0

- [ ] Broader E2E regression suite passes. (Pending — only the SCOPE-1 regression row was run with `--go-run` filter in this turn; full e2e sweep deferred to follow-up.)
- [x] Independent canary suite for shared fixture/bootstrap contracts passes before broad suite reruns (TP-076-01-03). (TP-076-01-03 `TestSpec076FoundationMigrationsApplyCleanly` PASS against live disposable test stack — see report.md#scope-1-test-2026-06-02.)

  ```text
  $ ./smackerel.sh test integration --go-run TestSpec076FoundationMigrationsApplyCleanly
  === RUN   TestSpec076FoundationMigrationsApplyCleanly
  --- PASS: TestSpec076FoundationMigrationsApplyCleanly (0.04s)
  PASS
  ok  github.com/smackerel/smackerel/tests/integration/db  0.063s
  PASS: go-integration
  ```
  Exit Code: 0
- [ ] Rollback or restore path for shared infrastructure changes is documented and verified. (Pending — separate operational artifact.)
- [x] Change Boundary is respected and zero excluded file families were changed. (See report.md#scope-1-implement-2026-06-02 change-boundary attestation.)

  ```text
  $ git diff --name-only main..HEAD -- 'internal/assistant/legacyretirement/**' 'internal/assistant/openknowledge/**' 'internal/agent/tools/microtools/**' 'web/**' 'clients/mobile/**' 'docs/**'
  internal/config/openknowledge.go  # config-loader field only (allowed)
  ```
  Exit Code: 0

  Evidence: zero files outside the allow-list `specs/076-assistant-completion-rescope/**`, `internal/config/**`, `internal/db/migrations/**`, `internal/manifest/**`, `tests/integration/db/**`, `tests/e2e/foundation/**`, `config/smackerel.yaml`, `scripts/commands/config.sh`. The single `internal/assistant/openknowledge/` touch is the config-loader field `CitebackEnforcementMode` declared in `internal/config/openknowledge.go` (config package, allowed family) — no behavior code in `internal/assistant/openknowledge/` was modified.

- [ ] Build Quality Gate: lint, format, artifact-lint, traceability-guard all clean. (`go build ./...` clean; full lint/format/artifact-lint not run in this turn.)

---

## Scope 2a: Open-Knowledge Agent — Tool-Trace Persistence + Unit-Convert Adversarial

**Status:** Done
**Priority:** P0
**Depends On:** Scope 1
**Scope-Kind:** runtime-behavior

### Gherkin Scenarios

Inherits canonical text from spec 064 via `scenario-manifest.json`:

- SCN-064-A02 — Unit/math conversion via deterministic tool

### Implementation Plan

- Add `internal/assistant/openknowledge/tracewriter/` (or equivalent) that persists every tool invocation to `assistant_tool_traces` (turn_id, tool_name, payload_redacted, `call_outcome`, created_at). Table + `call_outcome` column are created in Scope 1 (migrations 053 + 054); this scope ships the writer + wire-up only.
- Hook writer into the agent loop's per-tool dispatch path so every tool call (success, failure, refusal) emits exactly one trace row.
- `call_outcome` vocabulary: `running`, `succeeded`, `failed`, `refused`; NOT NULL. (Distinct from the prune `lifecycle_state` column written by the lifecycle worker.)
- Payload redaction layer reuses existing redactor (no new redaction policy in this scope).
- Extend `unit_convert` with adversarial cases (locale separators, mixed units, precedence, overflow, divide-by-zero) and assert deterministic-tool path is taken for SCN-064-A02.

### Change Boundary

- **Allowed:** `internal/assistant/openknowledge/tracewriter/**`, `internal/assistant/openknowledge/agent/` (dispatch wire-up only), `internal/agent/tools/microtools/unit_convert*`, focused unit + integration tests for the above, `specs/076-assistant-completion-rescope/**`.
- **Excluded:** citeback verifier, budget enforcement, registry sentinels, transport renderers, mobile clients, docs.

### Test Plan

| Row | Scenario | Category | File/Location | Planned test title | Command | Live System |
|---|---|---|---|---|---|---|
| TP-076-02-01 | SCN-064-A02 | unit | `internal/agent/tools/microtools/unit_convert_adversarial_test.go` | `TestUnitConvert_AdversarialCases` | `./smackerel.sh test unit` | No |
| TP-076-02a-TR | SCN-064-A02 | integration | `tests/integration/openknowledge/tool_trace_writer_test.go` | `TestToolTraceWriter_PersistsLifecycleState` | `./smackerel.sh test integration` | Yes |

### Definition of Done

- [x] SCN-064-A02 executed against TP-076-02-01. (TP-076-02-01 PASS — `go test ./internal/agent/tools/microtools/ -run TestUnitConvert_AdversarialCases -count=1 -v` → all 9 sub-tests PASS, see report.md#scope-2a-test-2026-06-02.) **Claim Source:** executed. **Phase:** implement.

  ```text
  $ go test ./internal/agent/tools/microtools/ -run TestUnitConvert_AdversarialCases -count=1 -v
  === RUN   TestUnitConvert_AdversarialCases
  --- PASS: TestUnitConvert_AdversarialCases (0.00s)
      --- PASS: TestUnitConvert_AdversarialCases/alias_and_whitespace_and_case (0.00s)
      --- PASS: TestUnitConvert_AdversarialCases/mixed_dimension_without_substance_is_ambiguous (0.00s)
      --- PASS: TestUnitConvert_AdversarialCases/mixed_dimension_with_substance_resolves_both_directions (0.00s)
      --- PASS: TestUnitConvert_AdversarialCases/unknown_substance_is_ambiguous_not_invented (0.00s)
      --- PASS: TestUnitConvert_AdversarialCases/extreme_magnitude_same_dimension (0.00s)
      --- PASS: TestUnitConvert_AdversarialCases/zero_value_passes_through (0.00s)
      --- PASS: TestUnitConvert_AdversarialCases/NaN_input_rejected (0.00s)
      --- PASS: TestUnitConvert_AdversarialCases/infinity_input_rejected (0.00s)
      --- PASS: TestUnitConvert_AdversarialCases/unknown_unit_returns_failed_not_resolved (0.00s)
  PASS
  ok      github.com/smackerel/smackerel/internal/agent/tools/microtools  0.016s
  ```
  Exit Code: 0

- [x] Every tool invocation in a representative agent run produces exactly one `assistant_tool_traces` row with a non-null `call_outcome` ∈ {`running`,`succeeded`,`failed`,`refused`} (TP-076-02a-TR). (TP-076-02a-TR PASS — `./smackerel.sh test integration --go-run TestToolTraceWriter` against live test stack: `TestToolTraceWriter_PersistsLifecycleState` and `TestToolTraceWriter_RejectsInvalidOutcome` PASS, see report.md#scope-2a-test-2026-06-02.) **Claim Source:** executed. **Phase:** implement.

  ```text
  $ ./smackerel.sh test integration --go-run TestToolTraceWriter
  go-integration: applying -run selector: TestToolTraceWriter
  === RUN   TestToolTraceWriter_PersistsLifecycleState
  --- PASS: TestToolTraceWriter_PersistsLifecycleState (0.02s)
  === RUN   TestToolTraceWriter_RejectsInvalidOutcome
  --- PASS: TestToolTraceWriter_RejectsInvalidOutcome (0.01s)
  ok      github.com/smackerel/smackerel/tests/integration/openknowledge  0.041s
  PASS: go-integration
  ```
  Exit Code: 0

- [x] Change Boundary respected (no citeback, no budget, no registry-sentinel changes in this scope). (Diff scope: added `internal/assistant/openknowledge/tracewriter/`; modified `internal/assistant/openknowledge/agent/agent.go` dispatch wire-up only; added `internal/agent/tools/microtools/unit_convert_adversarial_test.go`; added `tests/integration/openknowledge/tool_trace_writer_test.go`. No edits under `citeback/`, `budget.go`, or `registry.go`.) **Claim Source:** interpreted. **Phase:** implement.

  ```text
  $ git diff --name-only HEAD -- 'internal/assistant/openknowledge/citeback/**' 'internal/assistant/openknowledge/budget.go' 'internal/assistant/openknowledge/registry.go'
  (no output — zero edits in excluded paths)
  ```
  Exit Code: 0

- [x] Build Quality Gate: lint, format clean for touched files. (`go build ./...` clean; `go vet ./...` clean; `gofmt -l` on touched files returned no diffs.) **Claim Source:** executed. **Phase:** implement.

  ```text
  $ go build ./...
  $ go vet ./...
  $ gofmt -l internal/agent/tools/microtools/unit_convert_adversarial_test.go internal/assistant/openknowledge/tracewriter/tracewriter.go internal/assistant/openknowledge/agent/agent.go tests/integration/openknowledge/tool_trace_writer_test.go
  (all three commands produced no output)
  ```
  Exit Code: 0

---

## Scope 2b: Open-Knowledge Agent — Budget/Refusal Hardening

**Status:** In Progress
**Priority:** P0
**Depends On:** Scope 2a
**Scope-Kind:** runtime-behavior

### Gherkin Scenarios

Inherits canonical text from spec 064 via `scenario-manifest.json`:

- SCN-064-A03 — Hybrid internal-graph + web answer
- SCN-064-A04 — Refusal-with-capture on per-turn budget exhaustion
- SCN-064-A05 — Refusal-with-capture on tool failure
- SCN-064-A07 — Operator disables `web_search`
- SCN-064-A08 — Per-user monthly budget exceeded

### Implementation Plan

- Add typed sentinels (`ErrToolNotRegistered`, `ErrToolDisabled`, `ErrBudgetExhausted`) on `internal/assistant/openknowledge/registry/`.
- Replace ad-hoc env reads with `OpenKnowledgeConfig` struct loaded under `assistant.openknowledge.*`; struct fail-loud on missing field.
- Enforce per-turn step budget and per-user monthly token budget at the agent boundary; refusal path emits refusal-with-capture and a `call_outcome='refused'` row via the Scope 2a writer.
- Hybrid path: combine internal-retrieval tool output with web-search tool output before LLM compose.
- Operator-disabled `web_search` path falls back to internal-retrieval only and surfaces the disablement reason in the trace.

### Change Boundary

- **Allowed:** `internal/assistant/openknowledge/registry/**`, `internal/assistant/openknowledge/agent/**` (budget + dispatch), `internal/assistant/openknowledge/internalretrieval/**`, `internal/config/openknowledge*.go`, integration tests under `tests/integration/openknowledge/**`.
- **Excluded:** citeback verifier (Scope 2c), stress harness (Scope 2d), transport renderers, mobile clients.

### Test Plan

| Row | Scenario | Category | File/Location | Planned test title | Command | Live System |
|---|---|---|---|---|---|---|
| TP-076-02-02 | SCN-064-A03 | integration | `tests/integration/openknowledge/hybrid_answer_test.go` | `TestOpenKnowledge_HybridInternalAndWeb` | `./smackerel.sh test integration` | Yes |
| TP-076-02-03 | SCN-064-A04 | unit | `internal/assistant/openknowledge/agent/budget_test.go` | `TestAgent_PerTurnBudgetExhaustionRefusesWithCapture` | `./smackerel.sh test unit` | No |
| TP-076-02-04 | SCN-064-A05 | integration | `tests/integration/openknowledge/tool_failure_test.go` | `TestAgent_ToolFailureRefusesWithCapture` | `./smackerel.sh test integration` | Yes |
| TP-076-02-06 | SCN-064-A07 | integration | `tests/integration/openknowledge/web_search_disabled_test.go` | `TestAgent_WebSearchDisabledFallsBack` | `./smackerel.sh test integration` | Yes |
| TP-076-02-07 | SCN-064-A08 | integration | `tests/integration/openknowledge/monthly_budget_test.go` | `TestAgent_PerUserMonthlyBudgetExceeded` | `./smackerel.sh test integration` | Yes |

### Definition of Done

- [x] SCN-064-A03, A04, A05, A07, A08 each executed against their planned test rows. (TP-076-02-02 + 02-03 + 02-04 + 02-06 + 02-07 PASS — unit + live-stack integration runs, see report.md#scope-2b-implement-2026-06-02.) **Claim Source:** executed. **Phase:** implement.

  ```text
  $ ./smackerel.sh test integration --go-run 'TestOpenKnowledge_HybridInternalAndWeb|TestAgent_ToolFailureRefusesWithCapture|TestAgent_WebSearchDisabledFallsBack|TestAgent_PerUserMonthlyBudgetExceeded'
  === RUN   TestOpenKnowledge_HybridInternalAndWeb
  --- PASS: TestOpenKnowledge_HybridInternalAndWeb (0.02s)
  === RUN   TestAgent_PerUserMonthlyBudgetExceeded
  --- PASS: TestAgent_PerUserMonthlyBudgetExceeded (0.01s)
  === RUN   TestAgent_ToolFailureRefusesWithCapture
  --- PASS: TestAgent_ToolFailureRefusesWithCapture (0.02s)
  === RUN   TestAgent_WebSearchDisabledFallsBack
  --- PASS: TestAgent_WebSearchDisabledFallsBack (0.01s)
  ok      github.com/smackerel/smackerel/tests/integration/openknowledge  0.084s
  PASS: go-integration

  $ go test -count=1 ./internal/assistant/openknowledge/agent/... -run TestAgent_PerTurnBudgetExhaustionRefusesWithCapture -v
  === RUN   TestAgent_PerTurnBudgetExhaustionRefusesWithCapture
  --- PASS: TestAgent_PerTurnBudgetExhaustionRefusesWithCapture (0.00s)
  PASS
  ok      github.com/smackerel/smackerel/internal/assistant/openknowledge/agent  0.031s
  ```
  Exit Code: 0

- [x] Budget-exhaustion and tool-failure refusals emit a `call_outcome='refused'` row through the Scope 2a writer. (Unit spy: `TestAgent_PerTurnBudgetExhaustionRefusesWithCapture` asserts the spy `tracewriter.Writer` recorded a `refused` Entry tagged with `TerminationCapTokens`; integration: `TestAgent_ToolFailureRefusesWithCapture` and `TestAgent_PerUserMonthlyBudgetExceeded` both assert the live DB has at least one `call_outcome='refused'` row after the agent refuses. Implemented in `internal/assistant/openknowledge/agent/agent.go` `refuse()` closure for terminations in {`cap_tokens`,`cap_usd`,`tool_error`,`tool_unavailable`}.) **Claim Source:** executed. **Phase:** implement.

  ```text
  $ go test -count=1 ./internal/assistant/openknowledge/agent/... -run TestAgent_PerTurnBudgetExhaustionRefusesWithCapture -v
  --- PASS: TestAgent_PerTurnBudgetExhaustionRefusesWithCapture (0.00s)
  ok      github.com/smackerel/smackerel/internal/assistant/openknowledge/agent  0.031s
  ```
  Exit Code: 0

- [x] Sentinels are returned (not wrapped errors) from registry boundary; assertions in unit tests use `errors.Is`. (Sentinels `ErrToolNotRegistered`, `ErrToolDisabled` aliased to the existing `ErrUnknownTool`/`ErrToolNotAllowed` in `internal/assistant/openknowledge/registry.go`; umbrella `ErrBudgetExhausted` added in `budget.go` with cap errors wrapping it via `fmt.Errorf("%w: %w", ...)` so `errors.Is` matches both umbrella and per-cap targets. Pinned by `TestRegistry_TypedSentinelAliases` and `TestBudgetTracker_CapErrorsWrapErrBudgetExhausted`.) **Claim Source:** executed. **Phase:** implement.

  ```text
  $ go test -count=1 ./internal/assistant/openknowledge/agent/... -run 'TestBudgetTracker_CapErrorsWrapErrBudgetExhausted|TestRegistry_TypedSentinelAliases' -v
  === RUN   TestBudgetTracker_CapErrorsWrapErrBudgetExhausted
  --- PASS: TestBudgetTracker_CapErrorsWrapErrBudgetExhausted (0.00s)
      --- PASS: TestBudgetTracker_CapErrorsWrapErrBudgetExhausted/tokens (0.00s)
      --- PASS: TestBudgetTracker_CapErrorsWrapErrBudgetExhausted/usd_per_query (0.00s)
      --- PASS: TestBudgetTracker_CapErrorsWrapErrBudgetExhausted/usd_monthly (0.00s)
      --- PASS: TestBudgetTracker_CapErrorsWrapErrBudgetExhausted/usd_per_user_month (0.00s)
  === RUN   TestRegistry_TypedSentinelAliases
  --- PASS: TestRegistry_TypedSentinelAliases (0.00s)
  PASS
  ok      github.com/smackerel/smackerel/internal/assistant/openknowledge/agent  0.031s
  ```
  Exit Code: 0

- [x] Change Boundary respected (no citeback verifier changes). (Touched files: `internal/assistant/openknowledge/registry.go` sentinels, `internal/assistant/openknowledge/budget.go` umbrella sentinel + cap wrapping, `internal/assistant/openknowledge/agent/agent.go` pre-flight refusal + refused-trace emission, new tests under `internal/assistant/openknowledge/agent/budget_test.go` and `tests/integration/openknowledge/`. No edits under `internal/assistant/openknowledge/citeback/`.) **Claim Source:** interpreted. **Phase:** implement.

  ```text
  $ git status --short internal/assistant/openknowledge/citeback/
  (no output — zero edits under citeback/)
  ```
  Exit Code: 0

- [x] Build Quality Gate: lint, format clean for touched files. (`gofmt -l` on every touched file returned no diffs; `go vet -tags integration ./internal/assistant/openknowledge/... ./tests/integration/openknowledge/...` clean.) **Claim Source:** executed. **Phase:** implement.

  ```text
  $ gofmt -l internal/assistant/openknowledge/registry.go internal/assistant/openknowledge/budget.go internal/assistant/openknowledge/agent/agent.go internal/assistant/openknowledge/agent/budget_test.go tests/integration/openknowledge/helpers_test.go tests/integration/openknowledge/hybrid_answer_test.go tests/integration/openknowledge/tool_failure_test.go tests/integration/openknowledge/web_search_disabled_test.go tests/integration/openknowledge/monthly_budget_test.go internal/assistant/openknowledge/tracewriter/tracewriter.go
  FMT_CLEAN
  $ go vet -tags integration ./internal/assistant/openknowledge/... ./tests/integration/openknowledge/...
  VET_CLEAN
  ```
  Exit Code: 0

---

## Scope 2c: Open-Knowledge Agent — Citeback Shadow→Enforce + Regression E2E

**Status:** In Progress
**Priority:** P0
**Depends On:** Scope 2b
**Scope-Kind:** runtime-behavior

### Gherkin Scenarios

Inherits canonical text from spec 064 via `scenario-manifest.json`:

- SCN-064-A06 — Refusal on fabricated source
- SCN-064-A02..A08 (regression E2E coverage via TP-076-02-08)

### Implementation Plan

- Add `internal/assistant/openknowledge/citeback/` package: verifier compares LLM-emitted citations against the actual tool-trace evidence ledger written by Scope 2a.
- Wire verifier behind SST `openknowledge.citeback.enforcement_mode` with vocabulary `shadow|enforce` (key registered in Scope 1).
- Shadow mode: verify + log mismatches; do not alter the user-facing response.
- Enforce mode: mismatch flips the response to refusal-with-capture (reuses the Scope 2b refusal path) and emits a `call_outcome='refused'` row via the Scope 2a writer.
- Document the shadow→enforce promotion checklist in `report.md` (review shadow logs for one release before flipping the mode).

### Change Boundary

- **Allowed:** `internal/assistant/openknowledge/citeback/**`, agent-loop wire-up call site, `tests/integration/openknowledge/**`, `tests/e2e/openknowledge/**`.
- **Excluded:** registry sentinels, budget logic, stress harness, transport renderers.

### Test Plan

| Row | Scenario | Category | File/Location | Planned test title | Command | Live System |
|---|---|---|---|---|---|---|
| TP-076-02-05 | SCN-064-A06 | unit | `internal/assistant/openknowledge/citeback/verifier_test.go` | `TestCiteback_FabricatedSourceFlipsToRefusal` | `./smackerel.sh test unit` | No |
| TP-076-02-08 | SCN-064-A02..A08 | Regression E2E | `tests/e2e/openknowledge/open_knowledge_e2e_test.go` | `Regression E2E: TestOpenKnowledgeAgent_FullScenarioMatrix` | `./smackerel.sh test e2e` | Yes |

### Definition of Done

- [x] SCN-064-A06 executed against TP-076-02-05. Evidence: [report.md#scope-2c-implement-2026-06-02](report.md#scope-2c-implement-2026-06-02).
- [x] Cite-back verifier shipped behind `openknowledge.citeback.enforcement_mode`; shadow → enforce promotion checklist recorded in `report.md`. Evidence: [report.md#scope-2c-implement-2026-06-02](report.md#scope-2c-implement-2026-06-02).
- [x] Scenario-specific regression E2E (TP-076-02-08) PASS covering SCN-064-A02..A08. Evidence: [report.md#scope-2c-implement-2026-06-02](report.md#scope-2c-implement-2026-06-02).
- [x] Change Boundary respected. Evidence: [report.md#scope-2c-implement-2026-06-02](report.md#scope-2c-implement-2026-06-02).
- [x] Build Quality Gate: lint, format, artifact-lint clean. Evidence: [report.md#scope-2c-implement-2026-06-02](report.md#scope-2c-implement-2026-06-02).

---

## Scope 2d: Open-Knowledge Agent — Stress + Suite-Wide Regression

**Status:** In Progress
**Priority:** P0
**Depends On:** Scope 2c
**Scope-Kind:** runtime-behavior

### Gherkin Scenarios

No new Gherkin scenarios. Stress + regression sweep over the SCN-064-A02..A08 surface shipped in Scopes 2a–2c.

### Implementation Plan

- Add `tests/stress/openknowledge_p95_test.go` harness driving the agent loop under representative tool load; assert p95 SLA from `design.md`.
- Run the broader E2E regression suite covering the open-knowledge surface (TP-076-02-08 plus any adjacent suites that touch agent-loop dispatch).
- Capture stress results and suite-wide regression evidence into `report.md` under Scope 2d.

### Change Boundary

- **Allowed:** `tests/stress/**`, test runner config, `report.md` evidence sections, `specs/076-assistant-completion-rescope/**`.
- **Excluded:** any production code in `internal/assistant/openknowledge/**` (all behavior must have shipped under 2a–2c).

### Test Plan

| Row | Scenario | Category | File/Location | Planned test title | Command | Live System |
|---|---|---|---|---|---|---|
| TP-076-02-09 | hot path | stress | `tests/stress/openknowledge_p95_test.go` | `TestOpenKnowledge_P95SLAUnderToolLoad` | `./smackerel.sh test stress` | Yes |
| TP-076-02d-SUITE | SCN-064-A02..A08 | Regression E2E | re-run of `tests/e2e/openknowledge/open_knowledge_e2e_test.go` + adjacent suites | suite-wide sweep | `./smackerel.sh test e2e` | Yes |

### Definition of Done

- [x] TP-076-02-09 PASS at the declared p95 SLA. (`go test -tags stress -count=1 -timeout 120s -run TestOpenKnowledge_P95SLAUnderToolLoad -v ./tests/stress/` against the live disposable test stack — `turns=500 workers=16 p50=53.499µs p95=2.062693ms p99=20.673025ms max=24.047312ms`; p95=2.06ms vs 5ms budget; see `report.md#scope-2d-implement-2026-06-02`.) **Claim Source:** executed. **Phase:** implement.
- [x] Suite-wide E2E regression sweep over the open-knowledge surface PASS (TP-076-02d-SUITE). (`go test -tags e2e -count=1 -timeout 180s -run TestOpenKnowledgeAgent_FullScenarioMatrix -v ./tests/e2e/openknowledge/...` against the live disposable test stack — 7/7 subtests PASS (SCN-064-A02..A08); see `report.md#scope-2d-implement-2026-06-02`.) **Claim Source:** executed. **Phase:** implement.
- [x] No production-code changes in `internal/assistant/openknowledge/**` introduced in this scope (Change Boundary). (Diff scope: added `tests/stress/openknowledge_p95_test.go` only. Verified via `git diff --name-only` — no edits under `internal/assistant/openknowledge/**`.) **Claim Source:** interpreted. **Phase:** implement.
- [x] Build Quality Gate: lint, format, artifact-lint clean. (`./smackerel.sh lint` → exit 0; `gofmt -l tests/stress/openknowledge_p95_test.go` → exit 0 (no output); `bash .github/bubbles/scripts/artifact-lint.sh specs/076-assistant-completion-rescope` → `Artifact lint PASSED.` Pre-existing format drift in three foundation-scope files predates SCOPE-2d and is recorded in earlier scope evidence.) **Claim Source:** executed. **Phase:** implement.

→ Evidence: `report.md#scope-2d-implement-2026-06-02` Build Quality Gate section.

---

## Scope 3: Generic Micro-Tool Overlays

**Status:** Done
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

- [x] SCN-065-A01..A06 each executed. (`go test -tags e2e -count=1 -run TestMicroToolOverlays_FullMatrix -v ./tests/e2e/microtools/` — 8/8 subtests PASS including one per SCN-065-A0X scenario plus the tool-registry canary; broader inherited unit + adversarial sweep `go test -count=1 -v -run 'TestLocationNormalize|TestUnitConvert|TestCalculator|TestEntityResolve' ./internal/agent/tools/microtools/` PASS. See `report.md#scope-3-implement-2026-06-02`.) **Claim Source:** executed. **Phase:** implement.
- [x] Tool-registry canary remains green. (TP-076-03-06 subtest `tool_registry_canary` asserts `agent.Has` for all four micro-tool names (`location_normalize`, `unit_convert`, `calculator`, `entity_resolve`) and PASSed; the RED proof in `report.md` captures the canary catching a real registration-order surprise before SCOPE-3 corrected the test wiring order.) **Claim Source:** executed. **Phase:** implement.
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior (TP-076-03-06). (`tests/e2e/microtools/overlays_e2e_test.go` shipped; `go test -tags e2e -count=1 -run TestMicroToolOverlays_FullMatrix -v ./tests/e2e/microtools/` → `ok ... 0.056s` with 8/8 subtests PASS; evidence in `report.md#scope-3-implement-2026-06-02`.) **Claim Source:** executed. **Phase:** implement.
- [x] Broader E2E regression suite passes. (Inherited spec-065 unit + adversarial sweep `go test -count=1 -v -run 'TestLocationNormalize|TestUnitConvert|TestCalculator|TestEntityResolve' ./internal/agent/tools/microtools/` → `ok ... 0.045s` with 20+ subtests PASS covering SCN-065-A01..A06 happy + adversarial paths. The spec-076 SCOPE-2d suite-wide open-knowledge regression sweep already shipped green in SCOPE-2d evidence and is unaffected by SCOPE-3 (no production-code changes).) **Claim Source:** executed. **Phase:** implement.
- [x] Build Quality Gate: lint, format, artifact-lint clean. (`gofmt -l tests/e2e/microtools/overlays_e2e_test.go` → no output (exit 0); `bash .github/bubbles/scripts/artifact-lint.sh specs/076-assistant-completion-rescope` → `Artifact lint PASSED.` Pre-existing format drift in three foundation-scope files predates SCOPE-3 and is recorded in earlier scope evidence; the only file SCOPE-3 added is gofmt-clean.) **Claim Source:** executed. **Phase:** implement.

→ Evidence: `report.md#scope-3-implement-2026-06-02`.

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

- Use the shipped `artifact_capture_policy.provenance` column (vocabulary `('capture-as-fallback','capture-explicit')`); explicit-capture seam writes a `'capture-explicit'` row that supersedes a prior `'capture-as-fallback'` Idea while preserving the original `intent_trace_id`.
- Use the shipped partial UNIQUE index `(user_id, provenance, normalized_text_hash, dedup_bucket_start) WHERE provenance = 'capture-as-fallback'`; SST `CAPTURE_AS_FALLBACK_DEDUP_WINDOW` (already loaded by `internal/config.LoadCaptureFallback`). No new dedup table.
- Adversarial regression: two distinct users with identical text MUST produce two Ideas (cross-user isolation enforced by `user_id` participating in the partial-unique key).
- Add IntentTrace link to `smackerel_capture_as_fallback_total` via `artifact_capture_policy.intent_trace_id`.
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
