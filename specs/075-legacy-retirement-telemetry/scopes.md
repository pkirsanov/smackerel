# Scopes: 075 Legacy-Surface Deprecation Telemetry & User Comms

## Execution Outline

### Phase Order

1. **Scope 1 — Retirement Safety Foundation, Config, And Privacy:** create the finite retired-command catalog seam, fail-loud SST validation, server-side notice ledger shape, effective window state resolver, and HMAC user bucket policy.
2. **Scope 2 — Open-Window Notice Dedup And Intent Serving:** show one notice per `(user, retired_command, window_id)`, serve the mapped NL intent when confident, and persist dedup across sessions/transports.
3. **Scope 3 — Residual Usage Telemetry And Dashboard:** emit privacy-preserving residual usage metrics and build the rolling 7-day dashboard/report query.
4. **Scope 4 — Automatic Pause And Resume:** evaluate SST-defined rollback thresholds, enter paused state automatically, suppress new notices, and reset the counter on resume.
5. **Scope 5 — Closed-Window Response And Observation Gate:** return canonical unknown-command responses after close, block legacy handler invocation, and gate final deletion on zero-invocation observation.
6. **Scope 6 — Facade Policy Dispatch Rollout And Telegram Coexistence:** wire `legacyretirement.Policy` into the assistant facade as a pre-routing dispatcher, attach structured notice metadata to `AssistantResponse`, roll out per-transport renderers (PWA/WhatsApp/Mobile), short-circuit the legacy Telegram alias interceptor when the facade Policy is upstream, and execute the live-stack TP rows that Scopes 2/4/5 produced.

### New Types & Signatures

- `legacyretirement.Policy.Handle(ctx, AssistantTurn) (RetirementDecision, error)`
- `type WindowState string` values: `open`, `paused`, `closed`
- `type RetiredCommand{Command, ReplacementExample, NoticeCopy, Spec066ID}`
- `NoticeLedger.MarkShown(ctx, userID, windowID, command) error`
- `WindowStateResolver.Resolve(ctx) (WindowState, StateReason, error)`
- `ResidualTelemetry.Record(command, userBucket, outcome)`
- `ObservationReport.Generate(windowID) (retired_handler_invocations int, eligible_for_final_deletion bool, error)`
- Tables/columns: `assistant_conversations.legacy_retirement_notices`, `assistant_legacy_retirement_state`, `assistant_legacy_retirement_observations`.
- `assistant.FacadeConfig.Policy legacyretirement.Policy` — pre-routing dispatcher injected at facade construction.
- `assistant.AssistantResponse.LegacyRetirementNotice *NoticePayload` — structured notice metadata (command, replacement_example, copy_key, window_id) rendered by each transport.
- `type NoticePayload struct { Command, ReplacementExample, CopyKey, WindowID string }`
- Telegram `legacy_alias_intercept` short-circuit guard: when the request arrives with `ctx.Value(assistantFacadeUpstream) == true`, the interceptor returns `next(...)` immediately without rewriting the command (option 2 below).

### Validation Checkpoints

- After Scope 1, config/privacy/ledger schema tests must pass before user-facing notices are shown.
- After Scope 2, integration/e2e rows must prove one-time notices and cross-transport dedup while still serving mapped NL intent.
- After Scope 3, monitoring rows must prove residual usage and user buckets are queryable without raw ids/text.
- After Scope 4, threshold rows must prove automatic pause and resume counter reset.
- After Scope 5, closed-window rows must prove no legacy handler invocation and observation report gating before deletion.
- After Scope 6, facade Policy dispatch unit test must cover all five branches (open-notice, dedup-suppress, paused, closed, no-match passthrough); wire-schema notice propagation (sub-scope 6.2b) must prove the optional `notice` field round-trips through the JSON wire contract, generated PWA TypeScript bindings, and Flutter shared-core bindings without bumping `schema_version` (additive, v1-compatible); transport renderer rows must prove parity across PWA/WhatsApp/Mobile; Telegram interceptor short-circuit row must prove no double-dispatch when facade Policy is upstream; live-stack TP rows from Scopes 2/4/5 must execute against the real stack with evidence captured.

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
| 6 | Facade Policy Dispatch Rollout And Telegram Coexistence | assistant facade, FacadeConfig.Policy, transport renderers (PWA/WhatsApp/Mobile), Telegram interceptor short-circuit, live-stack execution | SCN-075-A12, SCN-075-A13 (plus re-runs of SCN-075-A01..A09 via TPs) | Not Started |

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

---

## Scope 6: Facade Policy Dispatch Rollout And Telegram Coexistence

**Status:** Not Started
**Depends On:** Scope 2 (foundation contracts + ledger from Scopes 1–2 must already exist; renderers also exercise Scope 4 paused / Scope 5 closed branches)
**Scope-Kind:** runtime-behavior

### Decomposition Rationale

`bubbles.implement` returned `route_required` for the original monolithic
"facade rollout + transport renderers + Telegram coexistence + live-stack execution"
work item because it spanned ≥3 surfaces, mixed a design decision (Telegram
coexistence) with implementation work, and bundled live-stack execution behind
unvalidated facade plumbing. This scope decomposes that single work item into
five tractable sub-scopes that are sequentially gated: the facade contract
(6.1) must be unit-proven before construction wiring (6.2), construction
wiring must be in before any transport renderer (6.3/6.4) can be exercised,
and only after all renderers and the Telegram short-circuit land can the
live-stack TP rows (6.5) execute meaningfully.

### Telegram Coexistence Decision (resolved)

**Question.** Facade-level `Policy` dispatch (Scope 6.1) executes BEFORE the
existing `internal/telegram/legacy_alias_intercept.go` interceptor that
already rewrites legacy aliases. Without coordination, both layers would
attach notices and rewrite commands, producing double-dispatch and duplicate
notices for the same `(user, command, window)`.

**Options considered.**
1. Remove `legacy_alias_intercept.go` entirely once facade Policy ships.
2. Telegram interceptor short-circuits when the facade Policy is upstream
   (request carries `assistantFacadeUpstream=true` in context).
3. Move all Telegram-specific dispatch into the facade and delete the
   interceptor package.

**Chosen: option 2.** Lowest risk: preserves existing interceptor test
coverage, keeps the Telegram-only alias rewriting code path available for any
non-facade ingress (legacy webhook paths, future bot deployments without the
facade), and the short-circuit is a one-line guard plus one integration test.
Options 1 and 3 require deleting / migrating a tested integration surface
before the facade Policy has burned in on the live stack.

**Implementation contract.**
- The assistant facade attaches `ctx = context.WithValue(ctx, assistantFacadeUpstreamKey{}, true)` before invoking the Telegram transport.
- `legacy_alias_intercept.go` checks that key first and, when set, calls `next(...)` unchanged. The existing interceptor tests continue to exercise the non-upstream path.
- A new integration test (TP-075-23) exercises both branches and asserts that the upstream-facade path produces exactly one notice and no double rewrite.

### Sub-Scope Inventory

| Sub-Scope | Name | Surfaces | Tests | Live System |
|---|---|---|---|---|
| 6.1 | Facade Policy Dispatch Contract (no transport changes) | `internal/assistant/facade.go`, `FacadeConfig.Policy`, `AssistantResponse.LegacyRetirementNotice` | TP-075-19 | No |
| 6.2 | Facade Construction Wiring | `cmd/core/wiring_assistant_facade.go`, `NewMultiResidualTelemetry(prom, sql)` | TP-075-20 | No |
| 6.2b | Wire-Schema Notice Propagation (PWA + Flutter shared-core codegen) | `internal/assistant/schema/assistant_turn_v1.json` (+ `NoticePayload` sub-def), `internal/assistant/schema/types.go` (`TurnResponse.Notice`), `internal/assistant/schema/testdata/response_v1.json` golden, `internal/assistant/httpadapter/{schema.go,adapter.go RenderJSON,middleware.go}`, `web/pwa/generated/*` (regen via `cmd/web-assistant-codegen`), `clients/mobile/assistant/lib/shared_core/generated/*` (Flutter regen) | TP-075-25, TP-075-26, TP-075-27 | No (codegen + contract); Yes (renderer rows in 6.3/6.4 execute the live propagation) |
| 6.3 | PWA Notice Renderer + Live Go E2E | `web/pwa/src/assistant/*` (renderer), `tests/e2e/assistant/legacy_retirement_notice_test.go` (Go e2e, photos_capability_banner-equivalent pattern) | TP-075-09 (re-targeted to Go) | Yes |
| 6.4 | WhatsApp + Mobile Renderers + Telegram Interceptor Short-Circuit | WhatsApp transport, mobile transport, `internal/telegram/legacy_alias_intercept.go` | TP-075-21, TP-075-22, TP-075-23 | Yes |
| 6.5 | Live-Stack Execution Of Scope 2/4/5 TPs | live integration + e2e harness | Re-runs of TP-075-05/06/07/08, TP-075-13/14/15, TP-075-16/17; aggregated as TP-075-24 | Yes |

### Gherkin Scenarios

```gherkin
Scenario: SCN-075-A12 — Facade Policy dispatch covers all five branches before transport routing
  Given the assistant facade is configured with a legacyretirement.Policy
  When Facade.Handle receives a turn matching one of the five Policy branches:
    | branch                | precondition                                            | expected outcome                                                  |
    | open + notice         | window=open, no ledger entry                            | response carries NoticePayload, ledger.MarkShown called           |
    | open + dedup-suppress | window=open, ledger entry exists                        | response has no NoticePayload, normal NL response                 |
    | paused                | window=paused                                           | response has no NoticePayload, legacy serving mode preserved      |
    | closed                | window=closed                                           | response is canonical unknown-command, no legacy handler invoked  |
    | no-match passthrough  | turn does not match any retired command                 | facade routes to normal transport pipeline unchanged              |
  Then the unit test asserts the expected outcome for each branch
  And no transport (PWA/WhatsApp/Mobile/Telegram) is invoked from the unit test

Scenario: SCN-075-A13 — Telegram interceptor short-circuits when facade Policy is upstream
  Given the facade has dispatched a retired-command turn and attached assistantFacadeUpstream=true to the context
  When the Telegram transport reaches legacy_alias_intercept
  Then the interceptor returns next(ctx, turn) without rewriting the command
  And only one NoticePayload is attached to the final AssistantResponse
  And the notice ledger records exactly one entry for (user, retired_command, window_id)

Scenario: SCN-075-A14 — Wire-schema notice propagates through HTTP + generated client bindings (v1-compatible additive field)
  Given the facade attaches a NoticePayload{command, replacement_example, copy_key, window_id} to AssistantResponse
  When the HTTP adapter renders the response via RenderJSON
  Then the JSON body contains an optional top-level "notice" object matching the v1 sub-def in internal/assistant/schema/assistant_turn_v1.json
  And schema_version remains "v1" (the notice field is additive and OPTIONAL; no bump required)
  And the golden fixture internal/assistant/schema/testdata/response_v1.json round-trips notice presence and absence
  And cmd/web-assistant-codegen regenerates web/pwa/generated/* with a typed optional Notice field consumed by the PWA renderer
  And the Flutter shared-core regen produces a typed optional Notice field consumed by the mobile renderer
  And a response WITHOUT a notice (no retired-command match) decodes cleanly on every client (back-compat guard)
```

### Implementation Plan (per sub-scope)

**6.1 Facade Policy Dispatch Contract**
- Extend `assistant.FacadeConfig` with `Policy legacyretirement.Policy` (nil-safe: nil Policy means no-op passthrough).
- In `Facade.Handle`, call `cfg.Policy.Handle(ctx, turn)` BEFORE the existing routing pipeline. The decision determines: attach `LegacyRetirementNotice` payload, short-circuit to canonical closed response, or fall through to normal routing.
- Add `LegacyRetirementNotice *NoticePayload` field to `AssistantResponse`.
- Unit test exercises the five branches with a stub Policy.
- NO transport changes in this sub-scope.

**6.2 Facade Construction Wiring**
- Update `cmd/core/wiring_assistant_facade.go` to construct `Policy` via `legacyretirement.NewMultiResidualTelemetry(prom, sql)` (and the Scope 1 ledger/state/resolver dependencies) and pass it through `FacadeConfig.Policy`.
- Unit test asserts the construction site wires a non-nil Policy when SST config is present and fails loud when required keys are missing (covered via Scope 1 config validation).

**6.2b Wire-Schema Notice Propagation (PWA + Flutter shared-core codegen)**

Goal: surface the structured `LegacyRetirementNotice` payload on the live HTTP wire contract and through every generated client binding (PWA TypeScript + Flutter shared-core) so the transport renderers in 6.3/6.4 have a typed `notice` field to render. The field is OPTIONAL and ADDITIVE — `schema_version` stays at `"v1"`; a response without a notice (no retired-command match, paused window, etc.) MUST decode cleanly on every client. v1-compatibility is documented in `design.md` (added under sub-scope 6.2b's design follow-up; routed to bubbles.design).

- Add `NoticePayload` sub-definition to `internal/assistant/schema/assistant_turn_v1.json` with required fields `command`, `replacement_example`, `copy_key`, `window_id` (all strings, non-empty when notice is present). Mark the top-level `notice` property optional (`additionalProperties: false` preserved; not added to `required`).
- Add `Notice *NoticePayload \`json:"notice,omitempty"\`` to `internal/assistant/schema/types.go::TurnResponse` mirroring the existing `LegacyRetirementNotice` shape on `AssistantResponse` (Scope 6.1).
- Add/extend golden fixture `internal/assistant/schema/testdata/response_v1.json` to include BOTH a notice-present case and the existing notice-absent case; the golden contract test (`internal/assistant/httpadapter/golden_contract_test.go::TestHTTPAssistantTurnGoldenContractV1`) asserts byte-exact equality for both.
- Update `internal/assistant/httpadapter/schema.go` to validate the optional `notice` sub-object against the v1 schema (pre-marshal validation if any exists; otherwise validation rides the existing schema validator path).
- Update `internal/assistant/httpadapter/adapter.go::RenderJSON` to copy `AssistantResponse.LegacyRetirementNotice` into `TurnResponse.Notice` (nil-safe: when nil, the field is omitted from the JSON body via `omitempty`).
- Update `internal/assistant/httpadapter/middleware.go` ONLY if the middleware chain inspects or rewrites the response body in a way that must learn about the new field (otherwise no change — keep the change boundary minimal).
- Regenerate PWA bindings via `go run ./cmd/web-assistant-codegen` so `web/pwa/generated/*` exposes a typed optional `Notice` field; check the regen output into the repo. The existing `web/pwa/tests/assistant_codegen_drift_test.go` must pass against the regenerated artifacts.
- Regenerate Flutter shared-core bindings under `clients/mobile/assistant/lib/shared_core/generated/*` so the mobile renderer in 6.4 consumes a typed optional `Notice` field. The existing `clients/mobile/assistant/test/codegen_drift_test.dart` must pass against the regenerated artifacts.
- v1-compatibility decision record: because the `notice` field is OPTIONAL and not in the schema's `required` list, every existing v1 client decodes a notice-bearing response by ignoring unknown-to-them keys (or by populating the new optional field when regenerated). No `schema_version` bump is needed. Document this decision in `design.md` (routed to bubbles.design under sub-scope 6.2b's design follow-up).
- **Containment rule:** sub-scope 6.2b changes the schema, the Go response struct, the golden fixture, the adapter's `RenderJSON`, and the two generated-binding directories ONLY. It does NOT add renderer code (PWA renderer ships in 6.3; WhatsApp/Mobile renderers ship in 6.4). It does NOT modify `schema_version`. It does NOT change any other top-level response field. It does NOT touch Telegram rendering paths.
- **Sequencing:** 6.2b runs AFTER 6.2 (Policy is wired into the facade so a real `LegacyRetirementNotice` can be produced) and BEFORE 6.3 (PWA renderer needs the regenerated TypeScript bindings before it can consume the typed notice). 6.4 also depends on 6.2b because the Flutter shared-core regen lands here.

**6.3 PWA Notice Renderer + Live Go E2E**
- Implement PWA-side renderer for `LegacyRetirementNotice` as a one-line addendum (non-blocking, dismissible).
- Re-target existing `TP-075-09` from the prior Playwright `web/pwa/tests/legacy_retirement_notice.spec.ts` plan to a Go end-to-end test at `tests/e2e/assistant/legacy_retirement_notice_test.go`. Pattern after `tests/e2e/photos_capability_test.go` (the Go counterpart to `web/pwa/tests/photos_capability_banner.spec.ts`): drive the live HTTP transport with a real bearer turn, assert the `notice` field is present in the schema-v1 response body, and assert the rendered PWA payload surfaces the addendum text without blocking the primary response. Command stays `./smackerel.sh test e2e` (Go e2e harness). The Playwright path is removed from the planning artifacts to keep one execution surface.

**6.4 WhatsApp + Mobile Renderers + Telegram Interceptor Short-Circuit**
- Implement WhatsApp renderer that appends the notice as a short message addendum (TP-075-21).
- Implement Mobile renderer that surfaces the notice in the chat thread without modal interruption (TP-075-22).
- Add `assistantFacadeUpstreamKey` context key and short-circuit guard in `internal/telegram/legacy_alias_intercept.go`; preserve existing interceptor tests for the non-upstream path (TP-075-23).

**6.5 Live-Stack Execution Of Scope 2/4/5 TPs**
- Run TP-075-05/06/07/08, TP-075-13/14/15, TP-075-16/17 against the live stack with `./smackerel.sh up` then `./smackerel.sh test integration` / `./smackerel.sh test e2e`.
- Capture raw outputs in `report.md` evidence blocks (redact home paths to `~/`).
- Aggregated as TP-075-24 in the test plan.

### Shared Infrastructure Impact Sweep

| Shared Surface | Downstream Contract | Canary Validation |
|---|---|---|
| Assistant facade `Handle` | Pre-routing Policy dispatch is nil-safe and never blocks normal turns | TP-075-19 unit |
| `AssistantResponse` contract | New optional `LegacyRetirementNotice` field is backward-compatible (omitempty) | TP-075-19 unit |
| Facade construction site (`cmd/core/wiring_assistant_facade.go`) | Real Policy with prom+sql telemetry wired in production | TP-075-20 unit |
| Telegram `legacy_alias_intercept` | No double-dispatch when facade is upstream; legacy path preserved for non-upstream ingress | TP-075-23 integration |
| Transport renderers (PWA/WhatsApp/Mobile) | Notice metadata renders consistently as a one-line addendum | TP-075-09, TP-075-21, TP-075-22 |

### Change Boundary

- **Allowed file families:** `internal/assistant/facade.go`, `internal/assistant/types.go` (or wherever `AssistantResponse` lives), `cmd/core/wiring_assistant_facade.go`, `internal/telegram/legacy_alias_intercept.go` (short-circuit guard only), per-transport renderer code (`web/pwa/**` for 6.3, WhatsApp transport for 6.4, mobile transport for 6.4), corresponding tests under `internal/assistant/`, `tests/integration/assistant/`, `tests/e2e/assistant/`, `web/pwa/tests/`.
- **Excluded surfaces:** retired-command catalog (owned by spec 066), Scope 1 ledger/state/telemetry contracts (already shipped), other transports beyond PWA/WhatsApp/Mobile/Telegram, deletion of legacy handlers.
- **Containment rule:** facade Policy dispatch MUST be nil-safe; the existing facade tests must keep passing when `FacadeConfig.Policy == nil`.

### Consumer Impact Sweep

| Consumer | Search Surface | Validation |
|---|---|---|
| All `FacadeConfig{...}` construction sites (test + production) | `grep -r 'FacadeConfig{' tests/ internal/ cmd/` | New `Policy` field is optional; nil-safe; no stale references | TP-075-19 unit + `./smackerel.sh test unit` regression |
| All `AssistantResponse` consumers | `grep -r 'AssistantResponse' internal/ tests/ web/` | New optional field doesn't break existing renderers | TP-075-19, TP-075-09/21/22 |
| Telegram interceptor callers | `grep -r 'legacy_alias_intercept' internal/ tests/` | Short-circuit path covered; non-upstream path unchanged | TP-075-23 |

### Impact-Aware Validation

No project impact map is configured. This scope touches the assistant facade (shared by every transport) and a tested Telegram interceptor, so canary unit + integration coverage MUST land before any live-stack execution.

### Test Plan

| Row | Scenario | Category | File/Location | Planned test title | Command | Live System |
|---|---|---|---|---|---|---|
| TP-075-19 | SCN-075-A12 | unit | `internal/assistant/facade_legacy_retirement_dispatch_test.go` | Planned: Facade.Handle pre-routing Policy dispatch covers all five branches (open-notice, dedup-suppress, paused, closed, no-match) | `./smackerel.sh test unit` | No |
| TP-075-20 | SCN-075-A12 | unit | `cmd/core/wiring_assistant_facade_test.go` | Planned: construction site wires NewMultiResidualTelemetry(prom, sql) into FacadeConfig.Policy with no nil dependencies | `./smackerel.sh test unit` | No |
| TP-075-09 (re-targeted to Go e2e) | SCN-075-A01 | e2e-api | `tests/e2e/assistant/legacy_retirement_notice_test.go` | Planned regression: live HTTP turn returns schema-v1 response with optional `notice` field populated and the PWA renderer surfaces it as a one-line addendum without blocking the primary response (pattern: `tests/e2e/photos_capability_test.go`) | `./smackerel.sh test e2e` | Yes |
| TP-075-25 | SCN-075-A14 | unit | `internal/assistant/httpadapter/golden_contract_test.go` (extend) | Planned: golden fixture `internal/assistant/schema/testdata/response_v1.json` round-trips both notice-present and notice-absent responses; schema_version stays `"v1"`; v1-compatibility holds (additive optional field) | `./smackerel.sh test unit` | No |
| TP-075-26 | SCN-075-A14 | unit | `web/pwa/tests/assistant_codegen_drift_test.go` (regenerate then re-run) | Planned: regenerated PWA TypeScript bindings expose a typed optional `Notice` field; codegen-drift test passes against the regen | `./smackerel.sh test unit` | No |
| TP-075-27 | SCN-075-A14 | unit | `clients/mobile/assistant/test/codegen_drift_test.dart` (regenerate then re-run) | Planned: regenerated Flutter shared-core bindings expose a typed optional `Notice` field; codegen-drift test passes against the regen | `flutter test clients/mobile/assistant/test/codegen_drift_test.dart` (Flutter harness) | No |
| TP-075-21 | SCN-075-A01 | integration | `tests/integration/assistant/legacy_retirement_whatsapp_renderer_test.go` | Planned: WhatsApp transport appends notice payload as a short message addendum | `./smackerel.sh test integration` | Yes |
| TP-075-22 | SCN-075-A01 | integration | `tests/integration/assistant/legacy_retirement_mobile_renderer_test.go` | Planned: Mobile transport surfaces notice payload in chat thread without modal interruption | `./smackerel.sh test integration` | Yes |
| TP-075-23 | SCN-075-A13 | integration | `tests/integration/assistant/legacy_telegram_short_circuit_test.go` | Planned: Telegram legacy_alias_intercept short-circuits when assistantFacadeUpstream=true; exactly one notice attached | `./smackerel.sh test integration` | Yes |
| TP-075-24 | re-runs of SCN-075-A01..A09 | integration/e2e-api/e2e | `./smackerel.sh test integration && ./smackerel.sh test e2e` (live stack) | Planned: live-stack execution of TP-075-05/06/07/08, TP-075-13/14/15, TP-075-16/17 with raw outputs captured in report.md | `./smackerel.sh up && ./smackerel.sh test integration && ./smackerel.sh test e2e` | Yes |

### Definition of Done — Tiered Validation

- [x] 6.1 Facade Policy dispatch contract: `FacadeConfig.Policy` and `AssistantResponse.LegacyRetirementNotice` land; TP-075-19 passes covering all five branches; nil-Policy path leaves existing facade tests green.

  **Phase:** implement
  **Claim Source:** executed
  **Evidence:**

  ```text
  $ cd ~/smackerel && go test -count=1 -timeout 90s -run TestFacadeLegacyRetirement ./internal/assistant/
  ok      github.com/smackerel/smackerel/internal/assistant       0.494s
  EXIT=0
  ```

  Regression sweep (assistant + telegram packages, both directly and transitively touching `FacadeConfig`, `AssistantResponse`, and the existing `legacyretirement.Policy` consumers):

  ```text
  $ cd ~/smackerel && go test -count=1 -timeout 180s ./internal/assistant/... ./internal/telegram/...
  ok  github.com/smackerel/smackerel/internal/assistant                          0.866s
  ok  github.com/smackerel/smackerel/internal/assistant/legacyretirement         0.122s
  ok  github.com/smackerel/smackerel/internal/telegram                          28.370s
  ok  github.com/smackerel/smackerel/internal/telegram/assistant_adapter         0.071s
  ok  github.com/smackerel/smackerel/internal/telegram/render                    0.082s
  (… 22 other assistant subpackages all `ok` …)
  ```

  Files changed in this sub-scope:
  - `internal/assistant/contracts/legacy_retirement_notice.go` (new) — `NoticePayload{Command, ReplacementExample, CopyKey, WindowID}`.
  - `internal/assistant/contracts/response.go` — `AssistantResponse.LegacyRetirementNotice *NoticePayload` field added.
  - `internal/assistant/facade.go` — `FacadeConfig.Policy legacyretirement.Policy` (nil-safe) and pre-routing Step 1.6 dispatch covering all five SCN-075-A12 branches.
  - `internal/assistant/legacyretirement/policy.go`, `policyimpl.go` — `RetirementDecision.WindowID` field exposed so the facade can populate `NoticePayload.WindowID` without depending on the concrete `policyImpl`.
  - `internal/assistant/facade_legacy_retirement_dispatch_test.go` (new) — TP-075-19: 5 branch tests + nil-Policy containment test, all PASS.

  No transport (PWA/WhatsApp/Mobile/Telegram) code was modified — confirms "no transport changes in 6.1" boundary.
- [x] 6.2 Construction wiring: `cmd/core/wiring_assistant_facade.go` constructs Policy with `NewMultiResidualTelemetry(prom, sql)`; TP-075-20 passes; fail-loud on missing SST keys per Scope 1.

  **Phase:** implement
  **Claim Source:** executed
  **Evidence:**

  Helper `buildLegacyRetirementPolicy` added in `cmd/core/wiring_assistant_facade.go` and invoked from `wireAssistantFacade` to populate `FacadeConfig.Policy` before `NewFacade`. It composes:

  - `legacyretirement.NewConfigCatalog` from `cfg.LegacyRetirement.NoticeCopyPerCommand` + `PostWindowUnknownResponseCopy`
  - `legacyretirement.NewSQLNoticeLedger(svc.pg.Pool)`
  - `legacyretirement.NewWindowStateResolver` over `SSTStateConfig{WindowID, WindowState}` + `NewStaticPauseStateReader(false)` (Scope 4 swaps in the threshold-driven writer)
  - `legacyretirement.NewUserBucketHasher(cfg.LegacyRetirement.UserBucketHMACKey)`
  - `legacyretirement.NewMultiResidualTelemetry(NewPrometheusResidualTelemetry(), NewSQLResidualStore(...))`
  - `legacyretirement.NewPolicy(PolicyConfig{Catalog, Ledger, StateResolver, Telemetry, BucketHasher, WindowID, Clock: time.Now})`

  Every dependency is fail-loud (G028/G029): nil config, nil pool, nil clock, empty WindowID, empty HMAC key, empty notice-copy map, or invalid window state each cause `buildLegacyRetirementPolicy` to return an error that bubbles up through `wireAssistantFacade`.

  TP-075-20 unit coverage (8 sub-tests covering the happy path + each fail-loud branch):

  ```text
  $ cd ~/smackerel && go test -count=1 -timeout 90s -run '^TestBuildLegacyRetirementPolicy_' ./cmd/core/
  ok      github.com/smackerel/smackerel/cmd/core 0.276s
  EXIT=0
  ```

  cmd/core package regression (proves the new wiring call did not break neighbouring tests):

  ```text
  $ cd ~/smackerel && go test -count=1 -timeout 120s ./cmd/core/
  ok      github.com/smackerel/smackerel/cmd/core 0.761s
  EXIT=0
  ```

  Files changed in this sub-scope:
  - `cmd/core/wiring_assistant_facade.go` — added `pgxpool` + `legacyretirement` imports, populated `facadeCfg.Policy` via `buildLegacyRetirementPolicy(cfg, svc.pg.Pool, time.Now)`, and added the helper itself.
  - `cmd/core/wiring_assistant_facade_test.go` (new) — TP-075-20: 8 sub-tests (`Valid…NonNilPolicy`, `NilConfigErrors`, `NilPoolErrors`, `NilClockErrors`, `EmptyWindowIDErrors`, `EmptyHMACKeyErrors`, `EmptyNoticeCopyErrors`, `InvalidWindowStateErrors`), all PASS.

  No other transport, facade, or telegram code was modified — confirms the "construction wiring only" boundary for sub-scope 6.2.
- [ ] 6.3 PWA renderer: TP-075-09 (Go e2e at `tests/e2e/assistant/legacy_retirement_notice_test.go`) passes against the live stack.
- [x] 6.2b Wire-schema notice propagation: TP-075-25 (golden contract round-trips notice present + absent at schema_version="v1"), TP-075-26 (regenerated PWA bindings + codegen-drift test), TP-075-27 (regenerated Flutter shared-core bindings + codegen-drift test) all pass; design.md records the v1-compatible additive-field decision (routed to bubbles.design).

  **Phase:** implement
  **Claim Source:** executed
  **Evidence:**

  Wire schema: `internal/assistant/schema/assistant_turn_v1.json` adds an OPTIONAL top-level `notice` property on `TurnResponse` plus a `NoticePayload` sub-definition with required `command`, `replacement_example`, `copy_key`, `window_id` strings. `additionalProperties: false` preserved. Top-level `required[]` for `TurnResponse` is unchanged so the field is additive — `schema_version` stays `"v1"`.

  Go types: `internal/assistant/schema/types.go` adds `TurnResponse.Notice *NoticePayload` with `json:"notice,omitempty"`, plus the `NoticePayload` struct mirroring the schema. `internal/assistant/httpadapter/schema.go` adds `TurnResponse.Notice *NoticeJSON` (also `omitempty`) plus `NoticeJSON`. `internal/assistant/httpadapter/adapter.go::RenderJSON` copies `AssistantResponse.LegacyRetirementNotice` into `TurnResponse.Notice` nil-safely.

  Golden contract helpers (`internal/assistant/schema/golden_contract_test.go`) relaxed: schema `properties` MAY be a superset of `required`; any schema property not in `required` MUST have `,omitempty` on the Go field, and fixture key sets must be a subset of `properties` and superset of `required`. The adversarial drift tests still flag (a) Go-vs-schema field-set drift and (b) unknown fixture keys.

  Goldens: `internal/assistant/schema/testdata/response_v1.json` retained (notice-absent baseline); new `internal/assistant/schema/testdata/response_v1_notice.json` and `internal/assistant/httpadapter/testdata/response_v1_notice.json` exercise the notice-present path. The httpadapter golden contract also asserts that a notice-absent `RenderJSON` output omits the `notice` key entirely (proves `omitempty` on the wire).

  TP-075-25 — schema + httpadapter golden contract round-trip both fixtures and pin `schema_version="v1"` on the notice-present path:

  ```text
  $ cd ~/smackerel && go test -count=1 -timeout 90s ./internal/assistant/schema/... ./internal/assistant/httpadapter/...
  ok      github.com/smackerel/smackerel/internal/assistant/schema                0.008s
  ?       github.com/smackerel/smackerel/internal/assistant/schema/webcodegen     [no test files]
  ok      github.com/smackerel/smackerel/internal/assistant/httpadapter           0.041s
  EXIT=0
  ```

  TP-075-26 — regenerated PWA bindings (`go run ./cmd/web-assistant-codegen`) drift-clean, regenerated bytes match the on-disk artifacts under `web/pwa/generated/`:

  ```text
  $ cd ~/smackerel && go run ./cmd/web-assistant-codegen
  wrote web/pwa/generated/assistant_turn_v1.d.ts
  wrote web/pwa/generated/assistant_turn_v1.js
  $ cd ~/smackerel && go test -count=1 -timeout 60s ./web/pwa/tests/
  ok      github.com/smackerel/smackerel/web/pwa/tests    0.013s
  ```

  Inspection of the regenerated TypeScript surface — `Notice` is exposed as a typed OPTIONAL field on `TurnResponse`, plus a `NoticePayload` interface and validator:

  ```text
  $ grep -nE 'Notice|notice' web/pwa/generated/assistant_turn_v1.d.ts
  45:  notice?: NoticePayload;
  50:export interface NoticePayload {
  56:export function validateNoticePayload(obj: unknown): NoticePayload;
  $ grep -nE 'Notice|notice' web/pwa/generated/assistant_turn_v1.js
  109:  if (Object.prototype.hasOwnProperty.call(obj, "notice") && obj["notice"] !== null && obj["notice"] !== undefined) {
  110:    validateNoticePayload(obj["notice"]);
  115:export function validateNoticePayload(obj) {
  116:  requireObject(obj, "NoticePayload");
  117:  requireFields(obj, "NoticePayload", NOTICE_PAYLOAD_FIELDS);
  118:  requireString(obj, "NoticePayload", "command");
  119:  requireString(obj, "NoticePayload", "replacement_example");
  120:  requireString(obj, "NoticePayload", "copy_key");
  121:  requireString(obj, "NoticePayload", "window_id");
  ```

  TP-075-27 — regenerated Flutter shared-core bindings (`dart run tool/gen_dart_models.dart`) drift-clean against the Flutter codegen-drift test:

  ```text
  $ cd ~/smackerel/clients/mobile/assistant && dart run tool/gen_dart_models.dart
  wrote ~/smackerel/clients/mobile/assistant/lib/core/generated/assistant_turn_v1.dart (12294 bytes)
  $ cd ~/smackerel/clients/mobile/assistant && flutter test test/codegen_drift_test.dart
  00:09 +1: TP-073-01 — Dart wire-schema codegen drift committed artifact matches regenerated bytes
  00:09 +2: TP-073-01 — Dart wire-schema codegen drift regeneration is deterministic across runs
  00:09 +3: TP-073-01 — Dart wire-schema codegen drift adversarial drift fails the comparison
  00:09 +3: All tests passed!
  EXIT=0
  ```

  Regression sweep — assistant + PWA packages clean (proves the relaxed golden helper, the codegen optional-field path, and the new `RenderJSON` copy did not break neighbouring tests):

  ```text
  $ cd ~/smackerel && go test -count=1 -timeout 180s ./internal/assistant/... ./web/pwa/tests/
  ok  github.com/smackerel/smackerel/internal/assistant                          0.599s
  ok  github.com/smackerel/smackerel/internal/assistant/contracts                0.137s
  ok  github.com/smackerel/smackerel/internal/assistant/httpadapter              0.149s
  ok  github.com/smackerel/smackerel/internal/assistant/legacyretirement         0.018s
  ok  github.com/smackerel/smackerel/internal/assistant/schema                   0.017s
  ok  github.com/smackerel/smackerel/web/pwa/tests                               0.012s
  (… 19 other assistant subpackages all `ok` …)
  EXIT=0
  ```

  Files changed in this sub-scope:
  - `internal/assistant/schema/assistant_turn_v1.json` — added optional `notice` property + `NoticePayload` sub-definition; updated docstring to reflect that additive OPTIONAL properties are v1-compatible (do not bump schema_version).
  - `internal/assistant/schema/types.go` — added `TurnResponse.Notice *NoticePayload` with `omitempty` and the `NoticePayload` struct.
  - `internal/assistant/schema/golden_contract_test.go` — relaxed helpers to permit `properties ⊇ required` with `omitempty` enforcement on Go optionals; added `NoticePayload_pins_Go_type` subtest and a new `response_v1_notice_fixture_round_trip` subtest. Existing adversarial drift checks still fire.
  - `internal/assistant/schema/testdata/response_v1_notice.json` (new) — notice-present golden fixture.
  - `internal/assistant/schema/webcodegen/generator.go` — added `NoticePayload` to `definitionOrder`; iterate optional properties separately; JS validator wraps optional checks in `hasOwnProperty && !== null`; DTS emits `field?: Type` for optional fields.
  - `internal/assistant/httpadapter/schema.go` — added `TurnResponse.Notice *NoticeJSON` with `omitempty` and `NoticeJSON` struct.
  - `internal/assistant/httpadapter/adapter.go` — `RenderJSON` copies `AssistantResponse.LegacyRetirementNotice` into the new `Notice` field (nil-safe).
  - `internal/assistant/httpadapter/golden_contract_test.go` — added the `response_v1_notice` subtest (TP-075-25) plus a notice-absent guard that the wire body omits the `notice` key entirely.
  - `internal/assistant/httpadapter/testdata/response_v1_notice.json` (new) — notice-present httpadapter golden.
  - `web/pwa/generated/assistant_turn_v1.d.ts`, `web/pwa/generated/assistant_turn_v1.js` — regenerated via `go run ./cmd/web-assistant-codegen`.
  - `clients/mobile/assistant/lib/src/codegen.dart` — added `NoticePayload` to `definitionOrder` and the optional-field iteration; emits `containsKey && != null` wrapper around optional validators.
  - `clients/mobile/assistant/lib/core/generated/assistant_turn_v1.dart` — regenerated via `dart run tool/gen_dart_models.dart`.

  **Containment proof:** no transport renderer code (PWA, WhatsApp, Mobile, Telegram), no `schema_version` change, no modification to any other top-level `TurnResponse` field, and no facade dispatch logic was touched in this sub-scope. The renderer work owned by 6.3 / 6.4 will consume the new typed optional `notice` field surfaced here.

  **Design-record handoff (still required, not owned by this agent):** the v1-compatibility decision record under `design.md` for SCOPE-075-06.2b must be authored by `bubbles.design`. This implementation agent did not edit `design.md`. Route packet emitted in the result envelope below.
- [ ] 6.4 WhatsApp + Mobile renderers + Telegram short-circuit: TP-075-21, TP-075-22, TP-075-23 all pass; existing `legacy_alias_intercept` tests still green for the non-upstream path.
- [ ] 6.5 Live-stack execution: TP-075-05/06/07/08, TP-075-13/14/15, TP-075-16/17 re-executed against the live stack; raw outputs (redacted) captured in `report.md` per `bubbles-evidence-capture`.
- [ ] Build Quality Gate passes: `./smackerel.sh check`, `./smackerel.sh lint`, `./smackerel.sh format --check`, artifact lint for this spec.
- [ ] Telegram Coexistence Decision Record (above) is recorded in `design.md` by `bubbles.design` before sub-scope 6.4 begins.

**Uncertainty Declaration:** This planning pass did not run implementation, build, lint, or test commands. The Telegram coexistence decision (option 2) is recommended; `bubbles.design` MUST ratify the decision in `design.md` before Scope 6.4 implementation starts. Each unchecked DoD item requires current-session execution evidence before completion.

**Uncertainty Declaration:** This planning pass did not execute runtime, report, or test commands.