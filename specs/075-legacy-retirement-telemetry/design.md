# Design: 075 Legacy-Surface Deprecation Telemetry & User Comms

Owner: `bubbles.design`
Workflow mode: `product-to-planning`
Status ceiling for this pass: `specs_hardened`
Source requirements: [spec.md](spec.md)

## Design Brief

**Current State.** Spec 066 retires legacy command handlers and defines an alias window, but its safety net is limited to alias notice storage and handler-removal contracts. There is no residual-usage dashboard, automatic pause state, post-window zero-invocation report, or cross-transport one-notice ledger.

**Target State.** Add a retirement safety layer in the assistant facade: retired-command detection, one-time-per-user-per-command notices, residual usage counters with privacy-preserving user buckets, SST-defined threshold evaluation, automatic paused state, and a post-window observation report that gates final handler deletion.

**Patterns to Follow.** Keep user intent served through the natural-language facade when confident, keep command retirement metadata finite and explicit, store notice state in the `assistant_conversations` row family, and wire dashboards/alerts through spec 049. Use fail-loud SST validation for all window, copy, and threshold values.

**Patterns to Avoid.** Do not keep retired handlers as the normal path after window close, re-notify the same user for the same command, put raw user ids or raw turn text in telemetry, make thresholds code constants, or make the notice a blocking interstitial.

**Resolved Decisions.** The dedup ledger is a JSONB map column on `assistant_conversations`. Effective window state is explicit SST plus durable runtime pause state: `closed` in SST wins, otherwise an active pause row makes the window paused, otherwise it is open. `user_bucket` is HMAC-SHA256 with required `legacy_retirement.user_bucket_hmac_key`. Deprecation notice is structured response metadata.

**Open Questions.** Planning must coordinate the exact retired-command list with spec 066. This design recommends the operator explicitly set 5 percent, 3 consecutive days, and 14 observation days; these are recommendations for config authors, not runtime defaults.

## Overview

This design makes legacy command retirement measurable and humane. During an open window, users get one short notice per retired command and still receive the natural-language result when mapping is confident. Operators get residual usage and rollback signals before final handler deletion.

| State | Source | Behavior |
|-------|--------|----------|
| open | SST says open and no active pause | notices allowed, NL mapping attempted |
| paused | active runtime pause row while SST is open | new notices suppressed, legacy handlers remain active |
| closed | SST says closed | unknown-command response, no legacy handler invocation |

## Architecture

```text
Inbound assistant turn / legacy command token
  -> retired command classifier from spec 066 finite list
  -> effective window state resolver
     -> open: dedup ledger check, optional notice, NL mapping
     -> paused: no new notice, legacy path remains available per spec 066 safety mode
     -> closed: canonical unknown-command response, no legacy handler
  -> residual usage counter + user_bucket
  -> threshold evaluator updates runtime pause state
  -> dashboard/report queries metrics and state
```

| Component | Location | Responsibility |
|-----------|----------|----------------|
| Retirement policy | `internal/assistant/legacyretirement` | window state, notices, telemetry, response metadata |
| Command catalog | spec 066 owned finite list | retired token to NL example/copy |
| Notice ledger | `assistant_conversations.legacy_retirement_notices` | one notice per user/command/window |
| Runtime state | `assistant_legacy_retirement_state` | automatic pause/resume counters |
| Metrics/dashboard | spec 049 monitoring stack | residual usage and alerting |
| Observation report | CLI/admin diagnostic | zero-handler-invocation proof |

## Capability Foundation

The reusable capability is `LegacyRetirementSafety`: a policy and telemetry layer for finite legacy surfaces without creating a second command grammar.

| Contract | Responsibility | Consumers |
|----------|----------------|-----------|
| `RetiredCommandCatalog` | finite command metadata from spec 066 | classifier, notice renderer |
| `NoticeLedger` | durable one-time notice map | facade across transports |
| `WindowStateResolver` | combine SST and runtime pause state | facade, dashboard |
| `ResidualTelemetry` | counters and user bucket hashing | monitoring stack |
| `ObservationReport` | prove zero retired-handler invocation after close | deletion gate |

Foundation-owned behavior:

- At most one notice per `(user_id, retired_command, window_id)`.
- Notice never blocks a confidently mapped NL result.
- Telemetry uses HMAC buckets, not raw ids.
- Automatic pause suppresses new notices and alerts operators.
- Closed state rejects retired handlers before invocation.

### Variation Axes

| Axis | Values | Foundation-Owned? |
|------|--------|-------------------|
| Window state | open, paused, closed | Yes |
| Command outcome | notice_and_served, served_no_notice, paused, closed_unknown, mapping_not_confident | Yes |
| Transport | Telegram, web, iOS, Android, WhatsApp, HTTP | Ledger yes, layout no |
| User bucket | HMAC bucket, aggregate distinct count | Yes |
| Deletion gate | observation incomplete, zero invocation proven | Yes |

## Concrete Implementations

### Legacy Retirement Policy

Package: `internal/assistant/legacyretirement`.

```go
type WindowState string
const (
    WindowOpen WindowState = "open"
    WindowPaused WindowState = "paused"
    WindowClosed WindowState = "closed"
)

type RetiredCommand struct {
    Command string
    ReplacementExample string
    NoticeCopy string
    Spec066ID string
}
```

The policy runs only after a token is classified as a retired command by the finite catalog. It does not parse arbitrary free text and does not choose scenarios directly.

### Notice Renderer

Deprecation notice is structured metadata:

```json
{
  "deprecation_notice": {
    "schema_version": "v1",
    "retired_command": "/weather",
    "replacement_example": "weather in Barcelona tomorrow",
    "shown": true,
    "window_state": "open"
  }
}
```

## Data Model

### Assistant Conversations Amendment

```sql
ALTER TABLE assistant_conversations
    ADD COLUMN IF NOT EXISTS legacy_retirement_notices JSONB NOT NULL;
```

Implementation must explicitly populate `legacy_retirement_notices` for existing rows during migration or startup migration; it must not rely on a runtime fallback value.

Ledger JSON:

```json
{
  "schema_version": 1,
  "window_id": "2026-05-retirement",
  "commands": {
    "/weather": {
      "first_notified_at": "2026-05-31T20:10:07Z",
      "last_seen_at": "2026-05-31T20:10:07Z",
      "notice_count": 1
    }
  }
}
```

### Runtime Pause State

```sql
CREATE TABLE IF NOT EXISTS assistant_legacy_retirement_state (
    state_id                         TEXT PRIMARY KEY,
    window_id                        TEXT        NOT NULL,
    effective_state                  TEXT        NOT NULL CHECK (effective_state IN ('open', 'paused')),
    paused_reason                    TEXT,
    threshold_command                TEXT,
    threshold_started_on             DATE,
    consecutive_days_over_threshold  INTEGER     NOT NULL,
    updated_at                       TIMESTAMPTZ NOT NULL,
    updated_by                       TEXT        NOT NULL,
    schema_version                   INTEGER     NOT NULL
);
```

### Observation Snapshot

```sql
CREATE TABLE IF NOT EXISTS assistant_legacy_retirement_observations (
    report_id                       TEXT PRIMARY KEY,
    window_id                       TEXT        NOT NULL,
    observation_started_at          TIMESTAMPTZ NOT NULL,
    observation_ended_at            TIMESTAMPTZ NOT NULL,
    retired_handler_invocations     INTEGER     NOT NULL,
    generated_at                    TIMESTAMPTZ NOT NULL,
    schema_version                  INTEGER     NOT NULL
);
```

## API/Contracts

No public user endpoint is required.

Open window, first command:

```json
{
  "status": "thinking",
  "body": "primary NL-driven response",
  "deprecation_notice": {
    "schema_version": "v1",
    "retired_command": "/weather",
    "replacement_example": "weather in Barcelona tomorrow",
    "shown": true,
    "window_state": "open"
  }
}
```

Closed window:

```json
{
  "status": "unavailable",
  "error_cause": "retired_command_closed",
  "body": "I do not use /weather anymore. Ask in plain English instead, for example: weather in Barcelona tomorrow",
  "facade_invoked": false
}
```

CLI report:

```json
{
  "window_id": "2026-05-retirement",
  "observation_days": 14,
  "retired_handler_invocations": 0,
  "eligible_for_final_deletion": true
}
```

Authorization matrix:

| Surface | Human User | Operator |
|---------|------------|----------|
| Deprecation notice | own assistant turn only | no special access |
| Dashboard | no | read |
| Runtime pause/resume | no | admin only |
| Observation report | no | operator/admin |

## UI/UX

The notice is one short block attached to the primary response. It names the retired command and a plain-English replacement. It does not block the answer when mapping is confident. It is deduped server-side across transports.

Dashboard panels:

| Panel | Data |
|-------|------|
| Window state | effective state, SST state, pause reason |
| Threshold summary | percent active users and consecutive-day rule |
| Residual usage by command | 7-day count and trend |
| Distinct user buckets | HMAC bucket distinct count |
| Alert state | ok, over threshold, paused |
| Post-window observation | retired handler invocations over configured period |

## Security/Compliance

- `user_bucket` is HMAC-SHA256 using `legacy_retirement.user_bucket_hmac_key`.
- Telemetry labels include retired command tokens but not raw user text or raw user ids.
- Notice copy is loaded from SST; missing copy fails startup/window open.
- Paused state suppresses new notices during unsafe rollout.
- Closed state rejects before legacy handler invocation.
- Dedup ledger lives server-side in `assistant_conversations`.

## Configuration And Migrations

Required SST keys:

| Key | Validation |
|-----|------------|
| `legacy_retirement.window_id` | non-empty stable id |
| `legacy_retirement.window_state` | `open` or `closed`; `paused` is runtime state |
| `legacy_retirement.rollback_threshold_percent_active_users` | float `> 0` and `<= 100` |
| `legacy_retirement.rollback_threshold_days_consecutive` | integer `>= 1` |
| `legacy_retirement.post_window_observation_days` | integer `>= 1` |
| `legacy_retirement.notice_copy_per_command` | non-empty map covering every retired command |
| `legacy_retirement.post_window_unknown_response_copy` | non-empty map or template |
| `legacy_retirement.user_bucket_hmac_key` | non-empty secret |
| `legacy_retirement.active_user_window_days` | integer `>= 1` for denominator calculation |

Missing keys abort startup. Recommended explicit starting values are 5 percent, 3 consecutive days, and 14 observation days.

Migrations add `legacy_retirement_notices`, `assistant_legacy_retirement_state`, and `assistant_legacy_retirement_observations`.

## Observability

| Metric | Labels | Meaning |
|--------|--------|---------|
| `smackerel_legacy_command_residual_total` | `command,user_bucket` | retired command invocations during window |
| `smackerel_legacy_retirement_notice_total` | `command,outcome` | shown, dedup_suppressed, paused_suppressed |
| `smackerel_legacy_retirement_window_state` | `state` | effective-state gauge |
| `smackerel_legacy_retired_handler_invocation_total` | `command` | handler invocations after close/safety checks |
| `smackerel_legacy_retirement_threshold_over_total` | `command` | daily threshold breach count |

Alert rule: if residual usage for any retired command exceeds configured percent for configured consecutive days, alert fires and runtime state changes to `paused`. Resumption resets `consecutive_days_over_threshold` to 0.

## Testing Strategy

| Scenario | Test Type | Test Location | Assertion |
|----------|-----------|---------------|-----------|
| SCN-075-A01 | integration/e2e-api | `tests/integration/assistant/legacy_retirement_notice_test.go` | first invocation notice plus NL-served result |
| SCN-075-A02 | integration | same | second same command no notice |
| SCN-075-A03 | integration | same | different command has independent notice |
| SCN-075-A04 | integration | `tests/integration/monitoring/legacy_retirement_metrics_test.go` | residual counter and 7-day query shape |
| SCN-075-A05 | integration | `tests/integration/assistant/legacy_retirement_threshold_test.go` | threshold auto-pauses window |
| SCN-075-A06 | unit/integration | same | resume resets consecutive-day counter |
| SCN-075-A07 | e2e-api | `tests/e2e/assistant/legacy_closed_response_test.go` | closed response and no handler invocation |
| SCN-075-A08 | integration/CLI | `tests/integration/assistant/legacy_observation_report_test.go` | zero-invocation report gates deletion |
| SCN-075-A09 | integration | `tests/integration/assistant/legacy_cross_transport_dedup_test.go` | notice dedup persists across transports |
| SCN-075-A10 | unit | `internal/config/legacy_retirement_test.go` | missing SST key fails startup |
| SCN-075-A11 | unit/integration | `tests/integration/monitoring/legacy_privacy_test.go` | user_bucket is HMAC, no raw ids/text |

## Risks & Open Questions

| Risk | Mitigation |
|------|------------|
| Pause state conflicts with SST | `closed` SST priority and runtime pause only while SST is open |
| Notice copy becomes noisy | one-time server ledger and short structured notice block |
| Residual usage denominator is unclear | required active-user window and dashboard denominator display |
| Final deletion happens without proof | observation report must show zero invocations over configured period |
| Raw ids leak through metrics | HMAC bucket tests and dashboard privacy guard |
