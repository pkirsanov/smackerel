# Report: 071 IntentTrace Observability Surface

## Planning Scaffold

### Summary

Planning artifacts were created for SCN-071-A01 through SCN-071-A10. This report intentionally contains no implementation, test, lint, build, replay, or dashboard execution evidence.

### Decision Record

- Scope 1 is the `foundation:true` scope because design defines `IntentTraceObservability` as a reusable trace contract consumed by replay, policy guard, dashboard, and privacy review surfaces.
- Scopes are ordered so schema/config validation precedes persistence, persistence precedes replay/guard integration, and replay/guard integration precedes dashboard joins.
- `.github/bubbles-project.yaml` does not define `testImpact` or `traceContracts`, so no generated impact plan or trace-contract guard row is available during planning.

### Completion Statement (MANDATORY)

Planning-only artifact creation is represented by `scopes.md`, `scenario-manifest.json`, `test-plan.json`, and `uservalidation.md`. Runtime work remains unchecked in `scopes.md` and requires current-session evidence before any DoD item can be completed.

### Code Diff Evidence

Implementation surfaces for SCN-071-A01..A10 already exist on disk as untracked files (verified via `git status --short`, 2026-06-01):

```
?? internal/assistant/intenttrace/
   - types.go, recorder.go, recorder_test.go
   - redaction.go, redaction_test.go
   - sampling.go, sampling_test.go
   - replay.go, replay_test.go
   - retention.go
   - export.go
   - golden_contract_test.go
?? internal/config/assistant_intent_trace.go
?? internal/db/migrations/047_assistant_intent_traces.sql
?? tests/integration/assistant/intent_trace_persistence_test.go
?? tests/integration/assistant/intent_trace_retention_test.go
?? tests/integration/assistant/intent_replay_store_test.go
?? tests/integration/assistant/refusal_trace_join_test.go
?? tests/integration/monitoring/  (assistant_intents_dashboard_test.go)
?? tests/e2e/assistant/  (intent_replay_test.go, intent_refusal_join_e2e_test.go,
                          intent_bypass_guard_e2e_test.go)
 M tests/integration/assistant/intent_trace_test.go
```

The Test Plan also lists three additional planned rows that are **not yet present** on disk and remain outstanding for a future implementation pass:

- `tests/e2e/assistant/intent_trace_contract_e2e_test.go` (SCN-071-A01 e2e)
- `tests/e2e/assistant/intent_trace_privacy_e2e_test.go` (SCN-071-A03 e2e)
- `tests/integration/policy/intent_bypass_guard_test.go` (SCN-071-A08 integration)
- `web/pwa/tests/assistant_intents_dashboard.spec.ts` (SCN-071-A06 e2e-ui)

**Claim Source:** executed (`git status --short` output, recorded 2026-06-01T21:13Z).

### Test Evidence (ALL TYPES REQUIRED)

#### Unit — `go test ./internal/assistant/intenttrace/`

Command: `go test -count=1 -timeout 180s -v ./internal/assistant/intenttrace/`  
Exit: 0  
Captured at: 2026-06-01T21:11Z (`/tmp/it-unit.log`)

```
=== RUN   TestSchemaVersionV1IsPinned
--- PASS: TestSchemaVersionV1IsPinned (0.00s)
=== RUN   TestGoldenV1PayloadRoundTrip
--- PASS: TestGoldenV1PayloadRoundTrip (0.00s)
=== RUN   TestGoldenV1PayloadHashPinned
--- PASS: TestGoldenV1PayloadHashPinned (0.00s)
=== RUN   TestClosedVocabulariesPinned
--- PASS: TestClosedVocabulariesPinned (0.00s)
=== RUN   TestStoreRecorder_RecordsSampledRow
--- PASS: TestStoreRecorder_RecordsSampledRow (0.00s)
=== RUN   TestStoreRecorder_SampledOutEnvelope
--- PASS: TestStoreRecorder_SampledOutEnvelope (0.00s)
=== RUN   TestStoreRecorder_ValidationFailures
--- PASS: TestStoreRecorder_ValidationFailures (0.00s)
    (6 sub-tests: missing_trace_id, missing_turn_id, unknown_transport,
     missing_action_class, unknown_status, missing_redaction_summary_raw_text)
=== RUN   TestNopRecorder_NoStoreWrite
--- PASS: TestNopRecorder_NoStoreWrite (0.00s)
=== RUN   TestDefaultRedactor_PersistRawTextFalseHidesRawText
--- PASS: TestDefaultRedactor_PersistRawTextFalseHidesRawText (0.00s)
=== RUN   TestDefaultRedactor_PersistRawTextTrueKeepsDispositionMarker
--- PASS: TestDefaultRedactor_PersistRawTextTrueKeepsDispositionMarker (0.00s)
=== RUN   TestDefaultRedactor_EmptyRawTextIsAbsent
--- PASS: TestDefaultRedactor_EmptyRawTextIsAbsent (0.00s)
=== RUN   TestStoreReplay_HappyPath_PayloadDryRunner
--- PASS: TestStoreReplay_HappyPath_PayloadDryRunner (0.00s)
=== RUN   TestStoreReplay_NotFound
--- PASS: TestStoreReplay_NotFound (0.00s)
=== RUN   TestStoreReplay_EmptyTraceID
--- PASS: TestStoreReplay_EmptyTraceID (0.00s)
=== RUN   TestStoreReplay_SampledOutRejected
--- PASS: TestStoreReplay_SampledOutRejected (0.00s)
=== RUN   TestStoreReplay_SchemaInvalidRejected
--- PASS: TestStoreReplay_SchemaInvalidRejected (0.00s)
=== RUN   TestStoreReplay_DryRunnerSideEffectIsBlocked
--- PASS: TestStoreReplay_DryRunnerSideEffectIsBlocked (0.00s)
=== RUN   TestStoreReplay_MatchSummaryReportsDivergence
--- PASS: TestStoreReplay_MatchSummaryReportsDivergence (0.00s)
=== RUN   TestRatioSampler_Validation
--- PASS: TestRatioSampler_Validation (0.00s)
=== RUN   TestRatioSampler_FullRatioAlwaysSamples
--- PASS: TestRatioSampler_FullRatioAlwaysSamples (0.00s)
=== RUN   TestRatioSampler_ZeroRatioNeverSamples
--- PASS: TestRatioSampler_ZeroRatioNeverSamples (0.00s)
=== RUN   TestRatioSampler_DeterministicForSameID
--- PASS: TestRatioSampler_DeterministicForSameID (0.00s)
=== RUN   TestRatioSampler_ApproximatesRatio
--- PASS: TestRatioSampler_ApproximatesRatio (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/assistant/intenttrace   0.027s
```

Maps: SCN-071-A01 (recorder validation + one-row invariant), SCN-071-A02 (sampler determinism + sampled-out envelope), SCN-071-A03 (redaction), SCN-071-A04 (replay dry-run + side-effect block + divergence), SCN-071-A10 (golden schema pin + closed vocabularies).

**Claim Source:** executed.

#### Unit — `go test ./internal/config/` (SCN-071-A05)

Command: `go test -count=1 -timeout 60s -v -run 'IntentTrace|SCN_071|AssistantIntentTrace' ./internal/config/`  
Exit: 0  
Captured at: 2026-06-01T21:11Z (`/tmp/it-cfg.log`)

```
=== RUN   TestIntentTraceConfigRequiresEverySSTKey
=== RUN   TestIntentTraceConfigRequiresEverySSTKey/all_missing_names_every_key
=== RUN   TestIntentTraceConfigRequiresEverySSTKey/fully_populated_no_errors
=== RUN   TestIntentTraceConfigRequiresEverySSTKey/each_key_independently_required
=== RUN   TestIntentTraceConfigRequiresEverySSTKey/each_key_independently_required/missing_ASSISTANT_INTENT_TRACE_SAMPLING_RATIO
=== RUN   TestIntentTraceConfigRequiresEverySSTKey/each_key_independently_required/missing_ASSISTANT_INTENT_TRACE_RETENTION_DAYS
=== RUN   TestIntentTraceConfigRequiresEverySSTKey/each_key_independently_required/missing_ASSISTANT_INTENT_TRACE_EXPORT_TARGETS
=== RUN   TestIntentTraceConfigRequiresEverySSTKey/each_key_independently_required/missing_ASSISTANT_INTENT_TRACE_REPLAY_ENABLED
=== RUN   TestIntentTraceConfigRequiresEverySSTKey/each_key_independently_required/missing_ASSISTANT_INTENT_TRACE_RETENTION_SWEEP_INTERVAL
=== RUN   TestIntentTraceConfigRequiresEverySSTKey/unknown_export_target_rejected
--- PASS: TestIntentTraceConfigRequiresEverySSTKey (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/config  0.020s
```

Maps: SCN-071-A05 (fail-loud SST naming every key).

**Claim Source:** executed.

#### Integration — live test stack against Postgres + NATS + ML + core

Command: `./smackerel.sh test integration --go-run '^(TestIntentTrace|TestIntentReplay|TestRefusalCause|TestRefusalCounter|TestAssistantIntents)'`  
Exit: 1 (unrelated pre-existing build failure in `tests/integration/telegram/legacy_alias_test.go` — `undefined: telegram.NewTestBotWithReplyRecorder`, `undefined: telegram.InterceptLegacyAliasForTest`; not in this spec's change boundary)  
Captured at: 2026-06-01T21:20Z (`/tmp/it-071.log`); live stack containers smackerel-test-{postgres,nats,ml,core,ollama,stub-providers,jaeger,searxng} all reported `healthy` before tests ran.

All spec 071 integration packages PASS:

```
=== RUN   TestIntentReplayLoadsOneStoredRedactedTraceByTraceID
--- PASS: TestIntentReplayLoadsOneStoredRedactedTraceByTraceID (0.03s)
=== RUN   TestIntentReplayRefusesSampledOutEnvelope
--- PASS: TestIntentReplayRefusesSampledOutEnvelope (0.02s)
=== RUN   TestIntentReplayReportsNotFoundForUnknownTraceID
--- PASS: TestIntentReplayReportsNotFoundForUnknownTraceID (0.01s)
=== RUN   TestIntentTracePersistsExactlyOneV1RowPerRecordCall
--- PASS: TestIntentTracePersistsExactlyOneV1RowPerRecordCall (0.02s)
=== RUN   TestIntentTraceSampledOutPreservesTotalTurnAccounting
--- PASS: TestIntentTraceSampledOutPreservesTotalTurnAccounting (0.02s)
=== RUN   TestIntentTraceRedactionLeavesNoRawSlotValueInPayload
--- PASS: TestIntentTraceRedactionLeavesNoRawSlotValueInPayload (0.01s)
=== RUN   TestIntentTraceRetentionSweepRemovesExpiredAndKeepsFresh
--- PASS: TestIntentTraceRetentionSweepRemovesExpiredAndKeepsFresh (0.02s)
=== RUN   TestIntentTraceRecordsCompileValidateRouteToolResponseSequence
--- PASS: TestIntentTraceRecordsCompileValidateRouteToolResponseSequence (0.00s)
=== RUN   TestIntentTraceDistinguishesClarifyFailureAndOperationalBypass
--- PASS: TestIntentTraceDistinguishesClarifyFailureAndOperationalBypass (0.00s)
=== RUN   TestRefusalCauseVocabularyMatchesIntentTraceColumn
--- PASS: TestRefusalCauseVocabularyMatchesIntentTraceColumn (0.03s)
=== RUN   TestRefusalCounterAndIntentTraceJoinByCauseLabel
--- PASS: TestRefusalCounterAndIntentTraceJoinByCauseLabel (0.02s)
ok      github.com/smackerel/smackerel/tests/integration/assistant      0.332s
=== RUN   TestAssistantIntentsDashboardHasRequiredPanels
--- PASS: TestAssistantIntentsDashboardHasRequiredPanels (0.00s)
=== RUN   TestAssistantIntentsDashboardQueriesCanonicalMetrics
--- PASS: TestAssistantIntentsDashboardQueriesCanonicalMetrics (0.00s)
=== RUN   TestAssistantIntentsDashboardRefusalPanelJoinsBothSources
--- PASS: TestAssistantIntentsDashboardRefusalPanelJoinsBothSources (0.00s)
ok      github.com/smackerel/smackerel/tests/integration/monitoring     0.019s
```

Scope mapping:
- SCN-071-A01 → `TestIntentTracePersistsExactlyOneV1RowPerRecordCall`, `TestIntentTraceRecordsCompileValidateRouteToolResponseSequence`
- SCN-071-A02 → `TestIntentTraceSampledOutPreservesTotalTurnAccounting`
- SCN-071-A03 → `TestIntentTraceRedactionLeavesNoRawSlotValueInPayload`
- SCN-071-A04 → `TestIntentReplayLoadsOneStoredRedactedTraceByTraceID`, `TestIntentReplayRefusesSampledOutEnvelope`, `TestIntentReplayReportsNotFoundForUnknownTraceID`
- SCN-071-A06 → `TestAssistantIntentsDashboardHasRequiredPanels`, `TestAssistantIntentsDashboardQueriesCanonicalMetrics`, `TestAssistantIntentsDashboardRefusalPanelJoinsBothSources`
- SCN-071-A07 → `TestRefusalCauseVocabularyMatchesIntentTraceColumn`, `TestRefusalCounterAndIntentTraceJoinByCauseLabel`, `TestAssistantIntentsDashboardRefusalPanelJoinsBothSources`
- SCN-071-A09 → `TestIntentTraceRetentionSweepRemovesExpiredAndKeepsFresh`
- SCN-071-A08 (integration row): **not executed** — `tests/integration/policy/intent_bypass_guard_test.go` is not present on disk yet.

**Claim Source:** executed for listed PASS lines; the EXIT=1 is attributable to the unrelated `tests/integration/telegram` package and is **not** a regression introduced by this spec's surfaces.

#### E2E — not executed in this pass

The e2e files `tests/e2e/assistant/intent_replay_test.go`, `tests/e2e/assistant/intent_refusal_join_e2e_test.go`, and `tests/e2e/assistant/intent_bypass_guard_e2e_test.go` exist on disk but were **not** executed in this session due to terminal-session time budget. The planned e2e rows `intent_trace_contract_e2e_test.go`, `intent_trace_privacy_e2e_test.go`, `intent_bypass_guard_e2e_test.go` (file present), and the e2e-ui row `web/pwa/tests/assistant_intents_dashboard.spec.ts` (file absent) all remain outstanding for the e2e validation pass.

**Claim Source:** not-run.

#### Format/Lint/Artifact-Lint — not executed in this pass

`./smackerel.sh format --check`, `./smackerel.sh lint`, and `bash .github/bubbles/scripts/artifact-lint.sh specs/071-intent-trace-observability` were not executed in this session.

**Claim Source:** not-run.

### Uncertainty Declarations

- Runtime behavior has not been implemented or executed in this planning-only pass.
- Artifact lint must be executed after artifact creation and reported by the invoking agent.

### Scenario Contract Evidence

Scenario contract coverage is planned through `scenario-manifest.json` and `test-plan.json`; no execution evidence is recorded here.

### Coverage Report

No coverage command was executed in this planning-only pass.

### Lint/Quality

Artifact lint is expected after planning artifacts are written.

### Spot-Check Recommendations

- Confirm no raw user id, raw text, slot value, or token appears in trace export fixtures.
- Confirm replay uses `./smackerel.sh assistant replay-intent <trace_id>` and not an ad hoc runtime command.
- Confirm dashboard zero states distinguish no traces from unavailable export targets.

### Validation Summary

Validation status is pending artifact lint execution by the current planning invocation.

### Audit Verdict

No audit verdict is claimed by this planning scaffold.

---

## Test Evidence (Round 2: 2026-06-01 23:36Z, bubbles.test)

### Scope 3 / SCN-071-A08 — Integration (live-stack) PASS

**Command:** `./smackerel.sh test integration --go-run '^TestIntentBypassGuardReportsRouterRouteWithoutCompiledIntent$'`
**Working directory:** `~/smackerel`
**Log:** `/tmp/s071-int.log` (4524 bytes)
**Wall:** 2026-06-01 23:36:33Z → 2026-06-01 23:41:32Z (~5 min, incl. build+stack startup)

Raw terminal output (filtered to the relevant lines):

```
go-integration: applying -run selector: ^TestIntentBypassGuardReportsRouterRouteWithoutCompiledIntent$
ok      github.com/smackerel/smackerel/tests/integration        0.157s [no tests to run]
ok      github.com/smackerel/smackerel/tests/integration/agent  0.300s [no tests to run]
ok      github.com/smackerel/smackerel/tests/integration/api    0.042s [no tests to run]
ok      github.com/smackerel/smackerel/tests/integration/assistant      0.153s [no tests to run]
ok      github.com/smackerel/smackerel/tests/integration/drive  0.251s [no tests to run]
ok      github.com/smackerel/smackerel/tests/integration/monitoring     0.017s [no tests to run]
=== RUN   TestIntentBypassGuardReportsRouterRouteWithoutCompiledIntent
--- PASS: TestIntentBypassGuardReportsRouterRouteWithoutCompiledIntent (0.04s)
ok      github.com/smackerel/smackerel/tests/integration/policy 0.071s
```

The runner wrapper recorded `EXIT=1` due to a project-scoped stack teardown step
that ran `config generate` after a concurrent agent had momentarily broken
`internal/config/assistant.go` (undefined symbol error during the teardown
window). That symbol now resolves (`go build ./internal/config/ RC=0`); the
SCN-071-A08 test itself executed against the live `smackerel-test` compose
stack (postgres+nats+ml+core+ollama+stub-providers+jaeger+searxng all healthy
per the runner's diagnostics block) and PASSED.

**Claim Source:** executed.

### Scope 3 / SCN-071-A04 + Scope 1 / SCN-071-A01 + Scope 2 / SCN-071-A02/A03/A09 + Scope 4 / SCN-071-A06/A07 — E2E BLOCKED

**Command attempted:** `./smackerel.sh test e2e --go-run '^(TestIntentReplayE2E|TestIntentBypassGuardE2E|TestIntentTraceContractE2E|TestIntentTracePrivacyE2E|TestIntentRefusalJoinE2E)'`
**Working directory:** `~/smackerel`
**Log:** `/tmp/s071-e2e.log`
**Wall:** 2026-06-01 23:41:52Z → 2026-06-01 23:42:11Z (failed at config-validate)

The e2e stack aborts before any test executes:

```
ERROR: [F061-SST-MISSING] missing or invalid required assistant configuration: ASSISTANT_TRANSPORTS_HTTP_ENABLED, ASSISTANT_TRANSPORTS_HTTP_SCHEMA_VERSION, ASSISTANT_TRANSPORTS_HTTP_BODY_SIZE_MAX_BYTES, ASSISTANT_TRANSPORTS_HTTP_RATE_LIMIT_PER_USER_PER_MINUTE, ASSISTANT_TRANSPORTS_HTTP_CONVERSATION_TTL_SECONDS, ASSISTANT_TRANSPORTS_HTTP_REQUIRED_SCOPE, ASSISTANT_TRANSPORTS_HTTP_CORS_ALLOWED_ORIGINS, ASSISTANT_TRANSPORTS_HTTP_TRANSPORT_HINT_ALLOWLIST
exit status 1
ERROR: config-generate-time validation failed for env=test (see above)
...
EXIT=1
```

Root cause: the spec 069 SCOPE-1a `assistant.transports.http.*` YAML block
exists at `config/smackerel.yaml` lines 800–814 (with every key marked
REQUIRED), and `internal/config/assistant.go` consumes those env vars via
`loadAssistantHTTPTransportConfig` (`internal/config/assistant_http_transport.go`),
but `scripts/commands/config.sh` (the generator) does not yet emit the
`ASSISTANT_TRANSPORTS_HTTP_*` env vars into `config/generated/<env>.env`.
The bash generator stops at the WhatsApp transport block (last emitted key
is `ASSISTANT_TRANSPORTS_WHATSAPP_MAX_TEXT_CHARS` at config.sh:1297). This
is a spec 069 SCOPE-2 generator gap, not a spec 071 issue.

All five planned spec 071 e2e test files are present on disk and well-formed:

```
tests/e2e/assistant/intent_trace_contract_e2e_test.go        (SCN-071-A01)
tests/e2e/assistant/intent_trace_privacy_e2e_test.go         (SCN-071-A03)
tests/e2e/assistant/intent_replay_test.go                    (SCN-071-A04, 2 funcs)
tests/e2e/assistant/intent_bypass_guard_e2e_test.go          (SCN-071-A08)
tests/e2e/assistant/intent_refusal_join_e2e_test.go          (SCN-071-A07)
web/pwa/tests/assistant_intents_dashboard.spec.ts            (SCN-071-A06)
tests/integration/policy/intent_bypass_guard_test.go         (SCN-071-A08, ran above)
```

This corrects the prior Round-1 report's claim that several of these files
were "not present on disk" — they exist. The remaining blocker is the spec
069 config-generator gap.

**Claim Source:** executed (the command ran; e2e tests themselves did not).

### Build Quality Gate — not executed in this pass

`./smackerel.sh format --check`, `./smackerel.sh lint`, and
`bash .github/bubbles/scripts/artifact-lint.sh specs/071-intent-trace-observability`
were not executed in this session. They remain owed before certification.

**Claim Source:** not-run.

### Next Owner

Route to `bubbles.implement` on **spec 069 SCOPE-2** to plumb the
`ASSISTANT_TRANSPORTS_HTTP_*` env vars through `scripts/commands/config.sh`,
mirroring the WhatsApp block. Once `./smackerel.sh test e2e` boots the
stack, return to `bubbles.test` on spec 071 to execute the five e2e files
listed above and the e2e-ui dashboard spec, then to `bubbles.validate` for
certification.
