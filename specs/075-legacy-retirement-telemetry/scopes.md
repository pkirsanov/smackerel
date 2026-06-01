# Scopes: 075 Legacy-Surface Deprecation Telemetry & User Comms

## Execution Outline

### Phase Order

1. **Scope 1 — Retirement Safety Foundation, Config, And Privacy:** create the finite retired-command catalog seam, fail-loud SST validation, server-side notice ledger shape, effective window state resolver, and HMAC user bucket policy.
2. **Scope 2 — Open-Window Notice Dedup And Intent Serving:** show one notice per `(user, retired_command, window_id)`, serve the mapped NL intent when confident, and persist dedup across sessions/transports.
3. **Scope 3 — Residual Usage Telemetry And Dashboard:** emit privacy-preserving residual usage metrics and build the rolling 7-day dashboard/report query.
4. **Scope 4 — Automatic Pause And Resume:** evaluate SST-defined rollback thresholds, enter paused state automatically, suppress new notices, and reset the counter on resume.
5. **Scope 5 — Closed-Window Response And Observation Gate:** return canonical unknown-command responses after close, block legacy handler invocation, and gate final deletion on zero-invocation observation.

### New Types & Signatures

- `legacyretirement.Policy.Handle(ctx, AssistantTurn) (RetirementDecision, error)`
- `type WindowState string` values: `open`, `paused`, `closed`
- `type RetiredCommand{Command, ReplacementExample, NoticeCopy, Spec066ID}`
- `NoticeLedger.MarkShown(ctx, userID, windowID, command) error`
- `WindowStateResolver.Resolve(ctx) (WindowState, StateReason, error)`
- `ResidualTelemetry.Record(command, userBucket, outcome)`
- `ObservationReport.Generate(windowID) (retired_handler_invocations int, eligible_for_final_deletion bool, error)`
- Tables/columns: `assistant_conversations.legacy_retirement_notices`, `assistant_legacy_retirement_state`, `assistant_legacy_retirement_observations`.

### Validation Checkpoints

- After Scope 1, config/privacy/ledger schema tests must pass before user-facing notices are shown.
- After Scope 2, integration/e2e rows must prove one-time notices and cross-transport dedup while still serving mapped NL intent.
- After Scope 3, monitoring rows must prove residual usage and user buckets are queryable without raw ids/text.
- After Scope 4, threshold rows must prove automatic pause and resume counter reset.
- After Scope 5, closed-window rows must prove no legacy handler invocation and observation report gating before deletion.

### Planning Notes

- `.github/bubbles-project.yaml` has no `testImpact` or `traceContracts` entries.
- Scope 1 is `foundation:true` because `LegacyRetirementSafety` provides reusable catalog, ledger, state, telemetry, and observation contracts consumed by later scopes.
- This plan does not remove legacy handlers; it plans the measurable safety layer that gates spec 066 removal work.

## Scope Inventory

| Scope | Name | Surfaces | Scenarios | Status |
|---|---|---|---|---|
| 1 | Retirement Safety Foundation, Config, And Privacy | policy module, config, ledger schema, HMAC buckets | SCN-075-A10, SCN-075-A11 | Not Started |
| 2 | Open-Window Notice Dedup And Intent Serving | facade, notice renderer, ledger, cross-transport state | SCN-075-A01, SCN-075-A02, SCN-075-A03, SCN-075-A09 | Not Started |
| 3 | Residual Usage Telemetry And Dashboard | metrics, dashboard query, rolling report | SCN-075-A04 | Not Started |
| 4 | Automatic Pause And Resume | threshold evaluator, runtime pause state, alerts | SCN-075-A05, SCN-075-A06 | Not Started |
| 5 | Closed-Window Response And Observation Gate | closed response, legacy handler guard, observation report | SCN-075-A07, SCN-075-A08 | Not Started |

---

## Scope 1: Retirement Safety Foundation, Config, And Privacy

**Status:** Not Started  
**Depends On:** —  
**Scope-Kind:** runtime-behavior  
**foundation:** true

### Gherkin Scenarios

```gherkin
Scenario: SCN-075-A10 — Missing SST keys fail loud
  Given legacy_retirement.rollback_threshold_percent_active_users is unset
  When the core process starts
  Then startup fails with a NO-DEFAULTS error naming the missing key
  And the deprecation window cannot be opened

Scenario: SCN-075-A11 — Telemetry contains no raw user identifiers
  Given the legacy-retirement dashboard is open
  When the operator inspects residual usage
  Then user_bucket is a privacy-preserving hash, not a raw user id
  And no raw text from user turns appears in the residual telemetry
```

### Implementation Plan

- Add `internal/assistant/legacyretirement` with finite catalog interface, notice ledger, window state resolver, residual telemetry, and observation report contracts.
- Add fail-loud config validation for every `legacy_retirement.*` key, including window id/state, thresholds, copy maps, user-bucket HMAC key, and active-user denominator window.
- Add `assistant_conversations.legacy_retirement_notices` JSONB migration plus runtime initialization that does not rely on fallback JSON values.
- Add runtime pause/observation tables and HMAC user bucket helper with no raw id/text metric labels.
- Add finite retired-command catalog integration point owned by spec 066 without copying or expanding the retired-command list here.

### Shared Infrastructure Impact Sweep

| Shared Surface | Downstream Contract | Canary Validation |
|---|---|---|
| `assistant_conversations` row family | Notice ledger must share facade truth, not parallel storage | TP-075-02 ledger schema row |
| Config loader | Window cannot open with missing threshold/copy/HMAC keys | TP-075-01 config row |
| Telemetry privacy | User buckets are HMAC values and raw text is excluded | TP-075-03 privacy row |

### Change Boundary

- **Allowed file families:** `internal/assistant/legacyretirement/**`, `internal/config/**`, assistant conversation migration/store code, targeted privacy/config tests.
- **Excluded surfaces:** actual legacy handler deletion, spec 066 retired-command list edits, transport renderer copy beyond structured notice metadata, docs updates.
- **Containment rule:** no raw user id, raw user text, or environment-specific operator values may be added to telemetry, config, or tests.

### Impact-Aware Validation

No project impact map is configured. This foundation touches shared assistant conversation state and telemetry privacy, so canary tests must run before user-facing notice behavior.

### Test Plan

| Row | Scenario | Category | File/Location | Planned test title | Command | Live System |
|---|---|---|---|---|---|---|
| TP-075-01 | SCN-075-A10 | unit | `internal/config/legacy_retirement_test.go` | Planned: missing rollback threshold key fails loud and blocks window open | `./smackerel.sh test unit` | No |
| TP-075-02 | SCN-075-A10 | integration | `tests/integration/assistant/legacy_retirement_foundation_test.go` | Planned: notice ledger column initializes without runtime fallback JSON | `./smackerel.sh test integration` | Yes |
| TP-075-03 | SCN-075-A11 | unit | `internal/assistant/legacyretirement/privacy_test.go` | Planned: user bucket is HMAC and telemetry labels reject raw ids/text | `./smackerel.sh test unit` | No |
| TP-075-04 | SCN-075-A11 | e2e-api | `tests/e2e/assistant/legacy_privacy_e2e_test.go` | Planned regression: live residual telemetry exposes buckets only | `./smackerel.sh test e2e` | Yes |

### Definition of Done — Tiered Validation

- [ ] Foundation contracts, config validation, ledger schema, pause/observation tables, HMAC bucket helper, and catalog seam satisfy SCN-075-A10 and SCN-075-A11.
- [ ] TP-075-01 through TP-075-04 pass with evidence.
- [ ] Build Quality Gate passes: `./smackerel.sh check`, `./smackerel.sh lint`, `./smackerel.sh format --check`, and artifact lint for this spec.

**Uncertainty Declaration:** This planning pass did not run implementation, build, lint, or test commands. Each unchecked item requires current-session execution evidence before completion.

---

## Scope 2: Open-Window Notice Dedup And Intent Serving

**Status:** Not Started  
**Depends On:** Scope 1  
**Scope-Kind:** runtime-behavior

### Gherkin Scenarios

```gherkin
Scenario: SCN-075-A01 — First retired-command invocation shows one notice and serves the intent
  Given the deprecation window is open and user U has never invoked /weather since the window opened
  When U sends "/weather barcelona"
  Then the response contains the canonical NL alternative as a one-line addendum
  And the user's weather intent is served via the NL path
  And the dedup ledger records (U, "/weather") as notified

Scenario: SCN-075-A02 — Second invocation of the same retired command does not re-notify
  Given the dedup ledger already records (U, "/weather") as notified
  When U sends "/weather barcelona" again
  Then the response is the normal NL-driven weather response
  And no deprecation notice is included

Scenario: SCN-075-A03 — Different retired command produces its own one-time notice
  Given the dedup ledger records (U, "/weather") as notified but NOT (U, "/remind")
  When U sends "/remind tomorrow at 9"
  Then the deprecation notice for /remind is shown exactly once
  And the dedup ledger records (U, "/remind") as notified

Scenario: SCN-075-A09 — Dedup ledger survives across sessions and devices
  Given user U received the /weather deprecation notice on Telegram
  When U invokes /weather later from the web client (spec 073)
  Then no deprecation notice is shown because the ledger is keyed on (user_id, retired_command), not on transport
```

### UI Scenario Matrix

| Scenario | Surface | Preconditions | User Action | Expected User-Visible Assertion | Test Row |
|---|---|---|---|---|---|
| SCN-075-A01 | Transport-neutral assistant response | open window, no ledger entry | send `/weather barcelona` | primary weather answer plus one short replacement notice | TP-075-05 |
| SCN-075-A02 | Transport-neutral assistant response | ledger entry exists | send `/weather barcelona` again | normal NL response with no notice | TP-075-06 |
| SCN-075-A03 | Transport-neutral assistant response | only `/weather` ledger entry exists | send `/remind tomorrow at 9` | `/remind` notice appears once and ledger records it | TP-075-07 |
| SCN-075-A09 | Telegram then web | notice shown on Telegram | send same retired command from web | web response suppresses notice via server ledger | TP-075-08 |

### Implementation Plan

- Add retired-command classifier based on finite spec 066 catalog and run it before the normal assistant facade path.
- When window is open and a command has no notice ledger entry, render structured deprecation metadata as an addendum while preserving the primary NL response when mapping is confident.
- Persist `(user_id, retired_command, window_id)` notice state in `assistant_conversations.legacy_retirement_notices`.
- Suppress duplicate notices for the same command/user/window across transports and sessions.
- Preserve help guidance when mapping is not confident without guessing execution.

### Shared Infrastructure Impact Sweep

| Shared Surface | Downstream Contract | Canary Validation |
|---|---|---|
| Assistant facade | Notice must not block confidently mapped NL result | TP-075-05 integration row |
| Server-side ledger | Notice dedup survives sessions/transports | TP-075-08 cross-transport row |
| Transport renderers | Structured notice metadata renders consistently | TP-075-09 e2e-ui row |

### Change Boundary

- **Allowed file families:** `internal/assistant/legacyretirement/**`, facade integration seam, assistant conversation ledger access, renderer metadata tests.
- **Excluded surfaces:** deleting legacy handlers, broad command parser rewrites, retired-command catalog edits owned by spec 066, unrelated assistant scenarios.
- **Containment rule:** user-facing notice is informational and cannot become a blocking interstitial.

### Impact-Aware Validation

No project impact map is configured. User-facing retirement messaging requires integration plus e2e-api/e2e-ui validation across at least two transports.

### Test Plan

| Row | Scenario | Category | File/Location | Planned test title | Command | Live System |
|---|---|---|---|---|---|---|
| TP-075-05 | SCN-075-A01 | integration/e2e-api | `tests/integration/assistant/legacy_retirement_notice_test.go` | Planned: first retired-command invocation shows notice and serves mapped NL intent | `./smackerel.sh test integration` | Yes |
| TP-075-06 | SCN-075-A02 | integration | `tests/integration/assistant/legacy_retirement_notice_test.go` | Planned: second same command suppresses notice and serves normal NL response | `./smackerel.sh test integration` | Yes |
| TP-075-07 | SCN-075-A03 | integration | `tests/integration/assistant/legacy_retirement_notice_test.go` | Planned: different retired command has independent one-time notice | `./smackerel.sh test integration` | Yes |
| TP-075-08 | SCN-075-A09 | e2e-api | `tests/e2e/assistant/legacy_cross_transport_dedup_e2e_test.go` | Planned regression: notice shown on one transport is suppressed on another | `./smackerel.sh test e2e` | Yes |
| TP-075-09 | SCN-075-A01 | e2e-ui | `web/pwa/tests/legacy_retirement_notice.spec.ts` | Planned regression: structured notice renders as one short addendum without blocking primary response | `./smackerel.sh test e2e` | Yes |

### Definition of Done — Tiered Validation

- [ ] Open-window notice, dedup ledger, per-command independence, and cross-transport persistence satisfy SCN-075-A01, SCN-075-A02, SCN-075-A03, and SCN-075-A09.
- [ ] TP-075-05 through TP-075-09 pass with evidence.
- [ ] Shared Infrastructure Impact Sweep confirms no duplicate notice for the same `(user, command, window)` and no blocking interstitial.
- [ ] Build Quality Gate passes with artifact lint for this spec.

**Uncertainty Declaration:** This planning pass did not execute runtime or test commands.

---

## Scope 3: Residual Usage Telemetry And Dashboard

**Status:** Not Started  
**Depends On:** Scope 2  
**Scope-Kind:** runtime-behavior

### Gherkin Scenarios

```gherkin
Scenario: SCN-075-A04 — Residual telemetry counts invocations per command per user bucket
  Given the deprecation window is open
  When users invoke retired commands across the deprecation period
  Then legacy_command_residual_total{command,user_bucket} increments accordingly
  And the dashboard's rolling 7-day report renders per-command and per-day counts plus distinct user counts
```

### UI Scenario Matrix

| Scenario | Surface | Preconditions | Operator Action | Expected Operator Assertion | Test Row |
|---|---|---|---|---|---|
| SCN-075-A04 | Legacy Retirement Dashboard | residual usage exists | open rolling 7-day report | per-command, per-day, distinct user bucket counts render without raw ids/text | TP-075-10 |

### Implementation Plan

- Emit `smackerel_legacy_command_residual_total{command,user_bucket}` for retired-command invocations during the window.
- Add dashboard/query materialization for rolling 7-day per-command counts and distinct user bucket counts.
- Add export/report path that omits raw user ids and raw turn text.
- Add metric labels for notice outcomes and window state needed by Scope 4.

### Shared Infrastructure Impact Sweep

| Shared Surface | Downstream Contract | Canary Validation |
|---|---|---|
| Monitoring metrics | Residual usage drives rollback threshold decisions | TP-075-10 dashboard query row |
| Privacy labels | `user_bucket` must be HMAC and no raw text label exists | TP-075-11 privacy integration row |
| Runtime command/reporting surface | Rolling report must use repo CLI or approved admin diagnostic | TP-075-12 report row |

### Change Boundary

- **Allowed file families:** metrics registration, monitoring query/tests, legacy retirement report command if routed through `./smackerel.sh`, privacy tests.
- **Excluded surfaces:** legacy handler removal, notice renderer copy, operator-specific config values, real user ids or real command payloads beyond retired command tokens.
- **Containment rule:** dashboard/report cannot become the source of state changes; it is read-only.

### Impact-Aware Validation

No project impact map is configured. Telemetry changes require integration tests and privacy assertions before threshold evaluation uses the metrics.

### Test Plan

| Row | Scenario | Category | File/Location | Planned test title | Command | Live System |
|---|---|---|---|---|---|---|
| TP-075-10 | SCN-075-A04 | integration | `tests/integration/monitoring/legacy_retirement_metrics_test.go` | Planned: residual counter and rolling 7-day query render per-command/day counts | `./smackerel.sh test integration` | Yes |
| TP-075-11 | SCN-075-A04 | integration | `tests/integration/monitoring/legacy_privacy_test.go` | Planned: dashboard/report contains user buckets and no raw ids or raw text | `./smackerel.sh test integration` | Yes |
| TP-075-12 | SCN-075-A04 | e2e-api | `tests/e2e/assistant/legacy_retirement_report_e2e_test.go` | Planned regression: live rolling report returns residual counts and distinct bucket totals | `./smackerel.sh test e2e` | Yes |

### Definition of Done — Tiered Validation

- [ ] Residual usage metrics, rolling dashboard/report queries, and privacy constraints satisfy SCN-075-A04.
- [ ] TP-075-10 through TP-075-12 pass with evidence.
- [ ] Telemetry contains no raw user identifiers or raw turn text.
- [ ] Build Quality Gate passes with artifact lint for this spec.

**Uncertainty Declaration:** This planning pass did not execute telemetry or test commands.

---

## Scope 4: Automatic Pause And Resume

**Status:** Not Started  
**Depends On:** Scope 3  
**Scope-Kind:** runtime-behavior

### Gherkin Scenarios

```gherkin
Scenario: SCN-075-A05 — Rollback threshold pauses the window automatically
  Given residual usage for /weather exceeds legacy_retirement.rollback_threshold_percent_active_users for legacy_retirement.rollback_threshold_days_consecutive consecutive days
  When the alerting evaluation runs
  Then an alert fires
  And the window enters PAUSED state: new notices are suppressed and legacy handlers continue serving requests until the operator decides

Scenario: SCN-075-A06 — Resuming the window resets the consecutive-day counter
  Given the window is in PAUSED state
  When the operator resumes the window after addressing the cause
  Then the consecutive-day counter resets to 0
  And residual telemetry continues unchanged
```

### UI Scenario Matrix

| Scenario | Surface | Preconditions | Operator/System Action | Expected Assertion | Test Row |
|---|---|---|---|---|---|
| SCN-075-A05 | Legacy Retirement Dashboard | threshold exceeded for configured days | alert evaluation runs | alert fires, effective state is paused, new notices suppressed | TP-075-13 |
| SCN-075-A06 | Legacy Retirement Dashboard/admin diagnostic | window paused | operator resumes | consecutive-day counter resets; residual telemetry remains queryable | TP-075-14 |

### Implementation Plan

- Implement threshold evaluator over residual usage and active-user denominator with explicit SST thresholds.
- Persist runtime pause state in `assistant_legacy_retirement_state` while SST `closed` remains highest priority.
- Suppress new notices in paused state while preserving legacy safety mode behavior defined by spec 066.
- Add resume admin diagnostic/command path that resets `consecutive_days_over_threshold` and records updater metadata.
- Emit window state gauge and threshold-over counters for dashboard/alerting.

### Shared Infrastructure Impact Sweep

| Shared Surface | Downstream Contract | Canary Validation |
|---|---|---|
| Threshold evaluator | Metrics drive pause state without hardcoded values | TP-075-13 integration row |
| Runtime state table | Pause state combines with SST state predictably | TP-075-15 state resolver row |
| Runtime command/admin diagnostic | Resume must be explicit and auditable | TP-075-14 row |

### Change Boundary

- **Allowed file families:** legacyretirement threshold evaluator, runtime pause state store, alert/metric wiring, approved admin diagnostic through repo CLI.
- **Excluded surfaces:** config defaults, legacy handler removal, retired-command catalog edits, deploy-specific operator scripts.
- **Containment rule:** pause/resume cannot edit SST files at runtime; SST remains the config source of truth.

### Impact-Aware Validation

No project impact map is configured. State-machine changes require integration tests for open/paused/closed precedence and e2e-api validation of notice suppression.

### Test Plan

| Row | Scenario | Category | File/Location | Planned test title | Command | Live System |
|---|---|---|---|---|---|---|
| TP-075-13 | SCN-075-A05 | integration | `tests/integration/assistant/legacy_retirement_threshold_test.go` | Planned: threshold breach fires alert and moves window to paused state | `./smackerel.sh test integration` | Yes |
| TP-075-14 | SCN-075-A06 | integration | `tests/integration/assistant/legacy_retirement_threshold_test.go` | Planned: operator resume resets consecutive-day counter and keeps telemetry | `./smackerel.sh test integration` | Yes |
| TP-075-15 | SCN-075-A05 | e2e-api | `tests/e2e/assistant/legacy_retirement_pause_e2e_test.go` | Planned regression: paused state suppresses new notices while preserving safe legacy serving mode | `./smackerel.sh test e2e` | Yes |

### Definition of Done — Tiered Validation

- [ ] Threshold evaluator, runtime pause state, alerting, notice suppression, and resume reset satisfy SCN-075-A05 and SCN-075-A06.
- [ ] TP-075-13 through TP-075-15 pass with evidence.
- [ ] Shared Infrastructure Impact Sweep confirms SST remains authoritative and no runtime code writes config files.
- [ ] Build Quality Gate passes with artifact lint for this spec.

**Uncertainty Declaration:** This planning pass did not execute runtime, alerting, or test commands.

---

## Scope 5: Closed-Window Response And Observation Gate

**Status:** Not Started  
**Depends On:** Scope 4  
**Scope-Kind:** runtime-behavior

### Gherkin Scenarios

```gherkin
Scenario: SCN-075-A07 — Window-closed response is the canonical unknown-command response
  Given the operator flips legacy_retirement.window_state to "closed"
  When user U invokes /weather
  Then the response is the canonical unknown-command response with a /help pointer
  And no legacy handler is invoked

Scenario: SCN-075-A08 — Post-window observation confirms zero legacy handler invocations
  Given the window has been closed for the SST-defined observation period
  When the observation report runs
  Then the report shows zero invocations of the retired handlers over the period
  And only then may final code deletion proceed (gated by the report)
```

### UI Scenario Matrix

| Scenario | Surface | Preconditions | User/Operator Action | Expected Assertion | Test Row |
|---|---|---|---|---|---|
| SCN-075-A07 | Window-Closed Command Response | SST state closed | user sends retired command | canonical unknown-command response with `/help`; handler not invoked | TP-075-16 |
| SCN-075-A08 | Observation report | closed for configured period | operator runs report | zero handler invocations gate final deletion eligibility | TP-075-17 |

### Implementation Plan

- Implement closed-state branch that rejects retired command tokens before legacy handler invocation and returns canonical unknown-command response copy from SST.
- Add runtime guard/counter for any retired handler invocation after close.
- Implement observation report over configured observation window and persist report snapshots.
- Require zero-invocation observation result before spec 066 final deletion work can proceed.
- Add stale-reference search plan for removed/renamed legacy command handlers, help entries, tests, dashboard rows, and docs owned by their respective agents.

### Shared Infrastructure Impact Sweep

| Shared Surface | Downstream Contract | Canary Validation |
|---|---|---|
| Legacy handler registry | Closed state must block handler invocation | TP-075-16 e2e row |
| Observation report command | Deletion gate uses observed zero invocations | TP-075-17 integration/CLI row |
| Consumer trace for deletion | Final handler deletion must update consumers together | TP-075-18 stale-reference row |

### Change Boundary

- **Allowed file families:** closed-state guard, observation report diagnostic, legacy handler invocation counter, tests.
- **Excluded surfaces:** actual final code deletion, help catalog edits outside canonical response, broad command parser rewrites, unrelated docs.
- **Containment rule:** any removal/rename of legacy handlers must be handled by spec 066 with consumer-trace coverage after this observation gate is satisfied.

### Consumer Impact Sweep

| Consumer | Search Surface | Validation |
|---|---|---|
| Legacy handler registry | retired command symbols and route/case labels | TP-075-18 |
| Help output | `/help` pointer and retired command examples | TP-075-16 response assertion |
| Metrics/dashboard/tests | handler invocation counters and retired command tokens | TP-075-17 and TP-075-18 |

### Impact-Aware Validation

No project impact map is configured. Closed-state behavior and deletion gate are high-risk retirement surfaces, so e2e-api and consumer-trace rows are mandatory.

### Test Plan

| Row | Scenario | Category | File/Location | Planned test title | Command | Live System |
|---|---|---|---|---|---|---|
| TP-075-16 | SCN-075-A07 | e2e-api | `tests/e2e/assistant/legacy_closed_response_test.go` | Planned regression: closed retired command returns unknown-command response and invokes no handler | `./smackerel.sh test e2e` | Yes |
| TP-075-17 | SCN-075-A08 | integration/CLI | `tests/integration/assistant/legacy_observation_report_test.go` | Planned: observation report proves zero retired-handler invocations over configured period | `./smackerel.sh test integration` | Yes |
| TP-075-18 | SCN-075-A08 | functional | `tests/integration/assistant/legacy_retirement_consumer_trace_test.go` | Planned: stale first-party references are found before final handler deletion proceeds | `./smackerel.sh test integration` | Yes |

### Definition of Done — Tiered Validation

- [ ] Closed-state response, no-handler guard, observation report, and deletion gate satisfy SCN-075-A07 and SCN-075-A08.
- [ ] TP-075-16 through TP-075-18 pass with evidence.
- [ ] Consumer Impact Sweep confirms final deletion remains gated by observed zero invocations and stale-reference checks.
- [ ] Build Quality Gate passes with artifact lint for this spec.

**Uncertainty Declaration:** This planning pass did not execute runtime, report, or test commands.