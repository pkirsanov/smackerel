# Scopes: 059 Google Keep Live Sync (gkeepapi production hardening)

## Execution Outline

### Phase Order

1. **Secret Manifest Wiring** — Add `KEEP_GOOGLE_APP_PASSWORD` to the three-mirror Bucket-2 manifest (Go `secretKeys`, `config/smackerel.yaml infrastructure.secret_keys`, `scripts/commands/config.sh`); surface `KEEP_GOOGLE_EMAIL` through the standard env contract; wire both env vars into `smackerel-ml` and `KEEP_GOOGLE_EMAIL` into `smackerel-core` compose env injection.
2. **`drift_ack_token` Config Field + Fail-Loud Connector Validation** — Extend `parseKeepConfig` for `drift_ack_token`; add the Connect-time fail-loud check that `KEEP_GOOGLE_EMAIL` is non-empty when `sync_mode ∈ {gkeepapi, hybrid}` and `gkeep_enabled: true`; resolve OQ-059-01 to a single floor in code. (Empty-`KEEP_GOOGLE_APP_PASSWORD` validation is owned by the sidecar handshake in Scope 3 because the Scope 1 boundary test `TestKeepAppPasswordReadOnlyFromSidecarNotCore` forbids any Go-core reference to that key.)
3. **NATS Request/Reply Bridge + Sidecar Connect Handshake** — Implement `syncGkeepapi` publish of `keep.sync.request` with request_id + cursor; add a `Connect()`-time handshake call on `keep.sidecar.handshake` (5 s timeout) whose sidecar handler validates non-empty `KEEP_GOOGLE_APP_PASSWORD` and replies fail-loud when empty (Go core surfaces the sidecar error verbatim); subscribe the sidecar (`ml/app/main.py` wiring `keep_bridge.register_nats_handler`) on both subjects and reply with `KeepSyncResponse` / `KeepHandshakeResponse`; constant 120 s sync timeout (OQ-059-02 resolution).
4. **Schema Validation + Drift Circuit Breaker** — Add `validateGkeepResponse` (per-field validation, `schema_version == 1` check); implement in-memory breaker state machine (`closed → tripping → open`) with reset on `drift_ack_token` rotation; classify sidecar auth-error as Connect-fail (not drift), all other error/validation failures as drift counts.
5. **Observability (metrics + structured logs)** — Add Prometheus counter `smackerel_keep_protocol_drift_detected_total`, histogram `smackerel_keep_gkeep_sync_duration_seconds`, counter `smackerel_keep_gkeep_notes_returned_total`; structured `slog` events for `keep_protocol_drift_detected` and request/response correlation by `request_id`; force `HealthError` while breaker OPEN.
6. **Operator Documentation** — Add the "Google Keep live sync" section to `docs/Operations.md` covering App Password runbook, initial enablement steps (5 step list), breaker recovery procedure (rotate `drift_ack_token` + restart), and token rotation; cross-reference specs 051/052 for the secret bundle path.

### New Types and Signatures

- `internal/config/secret_keys.go` — append `"KEEP_GOOGLE_APP_PASSWORD"` to `secretKeys`.
- `internal/connector/keep/keep.go` — `KeepConfig` gains `DriftAckToken string`; `parseKeepConfig` parses `drift_ack_token`; new `driftBreaker` struct fields on Connector (`driftFailures int`, `breakerState breakerState`, `lastAckToken string`).
- `internal/connector/keep/keep.go` — `KeepSyncRequest{Cursor, RequestID string}`, `KeepSyncResponse{Status string, Notes []GkeepNote, Cursor string, Error *string, SchemaVersion int}`, `validateGkeepResponse(*KeepSyncResponse) error`.
- `internal/connector/keep/keep.go` — constants `gkeepRequestSubject = "keep.sync.request"`, `gkeepRequestTimeout = 120 * time.Second`, `gkeepDriftFailureThreshold = 3`.
- `ml/app/keep_bridge.py` — new module-level `register_nats_handler(nc)` registering subscription on `keep.sync.request` calling `handle_sync_request` and `msg.respond(payload)`.
- `ml/app/main.py` — call `keep_bridge.register_nats_handler(nc)` during startup wiring (next to existing sidecar subscribers).
- `internal/metrics` — new `KeepProtocolDriftDetected prometheus.CounterVec`, `KeepGkeepSyncDuration prometheus.Histogram`, `KeepGkeepNotesReturned prometheus.Counter`.
- `config/smackerel.yaml` — `infrastructure.secret_keys` appended `KEEP_GOOGLE_APP_PASSWORD`; `connectors.google-keep.drift_ack_token: ""` placeholder.
- `scripts/commands/config.sh` — shell mirror gains `KEEP_GOOGLE_APP_PASSWORD`.
- `docs/Operations.md` — new `### Google Keep live sync` subsection under the Connectors section.

### Validation Checkpoints

- After Scope 1, `go test ./internal/config -run TestSecretKeys_MirrorsYAMLManifest` proves three-mirror parity; compose-up env smoke confirms both env vars are injected into the right containers.
- After Scope 2, `go test ./internal/connector/keep -run TestParseKeepConfig` proves the new `drift_ack_token` field round-trips; the Connect-time fail-loud unit test proves a missing `KEEP_GOOGLE_EMAIL` aborts startup before any NATS publish; Scope 1's boundary test `TestKeepAppPasswordReadOnlyFromSidecarNotCore` remains green (proving the Go core still has zero references to `KEEP_GOOGLE_APP_PASSWORD`).
- After Scope 3, integration test against the live test stack proves Go `Sync()` publishes on `keep.sync.request`, sidecar (with stubbed `gkeepapi` session) replies, and `RawArtifact`s appear in the canonical store.
- After Scope 4, unit tests prove every breaker state transition (`closed → tripping → open → closed` via token rotation); an integration test injects a malformed sidecar response and asserts breaker trips after the 4th failure and `HealthError` surfaces.
- After Scope 5, `curl <metrics endpoint>` against the live stack proves all three new metrics are exposed; structured-log assertions confirm `keep_protocol_drift_detected` event fires exactly once per state entry.
- After Scope 6, `bash .github/bubbles/scripts/regression-baseline-guard.sh specs/059-google-keep-live-mode --verbose` proves docs change is registered; manual review of the runbook confirms operator can complete enablement using only documented `./smackerel.sh` commands.

## Planning Assumptions

- Spec depends on shipped specs 051 (secret auth contract) and 052 (bundle secret injection contract); the three-mirror manifest pattern and the `__SECRET_PLACEHOLDER__*__` marker are pre-existing infrastructure.
- The `ml/app/keep_bridge.py` `authenticate()`, `serialize_note()`, and `handle_sync_request()` functions already exist with correct fail-loud env handling (verified 2026-05-28: `handle_sync_request` is defined at module scope and implements the retry-with-reauth path); the only sidecar additions are the new `register_nats_handler(nc)` function in `keep_bridge.py` and a single registration call from `ml/app/main.py` (which currently has zero `keep` references).
- Existing `internal/connector/keep/keep.go` already declares `SyncModeGkeepapi`, `SyncModeHybrid`, `KeepConfig`, `GkeepNote`, `parseKeepConfig`, `syncGkeepapi` stub, and `warning_acknowledged` gate — this plan extends them; it does not redesign.
- The NATS client is the existing per-process JetStream-capable connection used by Drive/Photos/YouTube bridges; no new credentials, streams, or topology.
- `gkeepRequestTimeout` and the drift threshold are constants (not config fields) per OQ-059-02 / OQ-059-03 design proposal; the plan codifies this and resolves both open questions in Scope 3 / Scope 4.
- OQ-059-01 is resolved to **15 min** floor (matching current `parseKeepConfig` enforcement); the spec narrative's "10 min" is treated as a documentation drift to be corrected during Scope 2.
- OQ-059-03 is resolved to **single shared counter** `smackerel_keep_protocol_drift_detected_total` per the design proposal; HTTP non-2xx and schema-validation failures both increment it.

## Scope Inventory

| Scope | Name | Surfaces | Primary Tests | DoD Summary | Status |
|-------|------|----------|---------------|-------------|--------|
| 1 | Secret Manifest Wiring | `internal/config`, `config/smackerel.yaml`, `scripts/commands/config.sh`, compose env | Unit (Go contract), integration (compose env injection) | `KEEP_GOOGLE_APP_PASSWORD` in all three mirrors; `KEEP_GOOGLE_EMAIL` flows to both containers; secret-key mirror test green | Not started |
| 2 | `drift_ack_token` Config + Fail-Loud Connector | `internal/connector/keep/keep.go` | Unit (parseKeepConfig, Connect EMAIL validation, boundary regression) | New field parses; missing EMAIL Connect fails loud; Go core never references `KEEP_GOOGLE_APP_PASSWORD`; OQ-059-01 resolved with single floor | Done |
| 3 | NATS Request/Reply Bridge + Sidecar Connect Handshake | `internal/connector/keep/keep.go`, `ml/app/keep_bridge.py`, `ml/app/main.py` | Unit (publisher + handler + handshake), integration (live stack + handshake fail-loud) | Go publishes and parses sync reply; sidecar subscribes and responds on both subjects; sidecar handshake validates non-empty `KEEP_GOOGLE_APP_PASSWORD` and Go core surfaces fail-loud error verbatim; canonical artifact flow proven end-to-end | Not started |
| 4 | Schema Validation + Drift Breaker | `internal/connector/keep/keep.go` (+ test fixtures) | Unit (validation + state machine), integration (drift injection) | All 4 transitions covered; HealthError surfaces; auth errors NOT counted as drift | Not started |
| 5 | Observability | `internal/metrics`, `internal/connector/keep/keep.go` | Unit (metric increments), integration (live `/metrics` scrape + log assertions) | Three new metrics exposed; `keep_protocol_drift_detected` slog emitted once per state entry | Not started |
| 6 | Operator Documentation | `docs/Operations.md` | regression-baseline-guard; manual review | Runbook complete (enablement + recovery + rotation); only `./smackerel.sh` commands referenced; no env-specific values | Not started |

## Scope 1: Secret Manifest Wiring

**Status:** In Progress (implement complete, awaiting test/validate)
**Depends On:** None
**Surfaces:** `internal/config/secret_keys.go`, `internal/config/secret_keys_test.go`, `config/smackerel.yaml`, `scripts/commands/config.sh`, `docker-compose.yml`, `deploy/compose.deploy.yml`

### Use Cases

#### SCN-059-001: Three-Mirror Manifest Carries `KEEP_GOOGLE_APP_PASSWORD`

```gherkin
Given the operator wants to enable Google Keep live sync
When `bash .github/bubbles/scripts/artifact-lint.sh specs/059-google-keep-live-mode` and `go test ./internal/config -run TestSecretKeys_MirrorsYAMLManifest` run
Then `KEEP_GOOGLE_APP_PASSWORD` is present in `internal/config/secret_keys.go` `secretKeys`
And `KEEP_GOOGLE_APP_PASSWORD` is present in `config/smackerel.yaml` under `infrastructure.secret_keys`
And `KEEP_GOOGLE_APP_PASSWORD` is present in the shell mirror in `scripts/commands/config.sh`
And the mirror-parity contract test exits 0
```

#### SCN-059-002: Compose Injects `KEEP_GOOGLE_EMAIL` and `KEEP_GOOGLE_APP_PASSWORD` Into Sidecar

```gherkin
Given `KEEP_GOOGLE_EMAIL` and `KEEP_GOOGLE_APP_PASSWORD` are present in the resolved `app.env`
When `./smackerel.sh up` boots the stack against the disposable test compose project
Then `docker exec smackerel-test-ml env` shows both variables set
And `docker exec smackerel-test-core env` shows `KEEP_GOOGLE_EMAIL` set
And the Go core process does not read `KEEP_GOOGLE_APP_PASSWORD` from its environment (sidecar boundary preserved at the application layer)
```

> **Compose env-file note (resolves planning B1, option c):** both `smackerel-core` and `smackerel-ml` share the same resolved `app.env` via `env_file: ${SMACKEREL_ENV_FILE}`, so the password variable IS materially present in the core container's process environment — this matches the existing precedent (`TELEGRAM_BOT_TOKEN` is also visible in the core env but only the ML sidecar reads it). The boundary is enforced at the application layer (only the ML sidecar's `keep_bridge.py` references `os.environ.get("KEEP_GOOGLE_APP_PASSWORD")`), not at the compose env-injection layer. Splitting into per-service env files (option a) and `environment: KEY: ""` overrides (option b) were considered and rejected as unjustified architectural churn for a single secret.

### Implementation Plan

- Append `"KEEP_GOOGLE_APP_PASSWORD"` to `var secretKeys` in `internal/config/secret_keys.go` (alphabetical or appended end — match existing convention; verify with the test).
- Append the same key under `infrastructure.secret_keys` in `config/smackerel.yaml` in the same ordinal slot.
- Update the shell array in `scripts/commands/config.sh` (the existing `KNOWN_SECRET_KEYS` or equivalent variable used by `config generate`).
- Update env emission so `smackerel-ml` receives both `KEEP_GOOGLE_EMAIL` and `KEEP_GOOGLE_APP_PASSWORD`, and `smackerel-core` receives `KEEP_GOOGLE_EMAIL` only (mirror `TELEGRAM_BOT_TOKEN` env injection for the sidecar; mirror existing non-secret connector env emission for the email).
- Add `KEEP_GOOGLE_EMAIL` to `config/smackerel.yaml` under `connectors.google-keep.email` (or equivalent non-secret slot) with empty-string placeholder, OR document it as a pure env contract value if the existing config codepath does not surface it as YAML — match the pattern used by other connector emails (e.g., the gmail or hospitable connector).
- No code in this scope reads the password; this scope only adds the manifest entry and the env-injection plumbing.
- **Change Boundary:** allowed file families = `internal/config/secret_keys.go`, `internal/config/secret_keys_test.go`, `config/smackerel.yaml`, `scripts/commands/config.sh`, `docker-compose.yml`, `deploy/compose.deploy.yml`, `internal/connector/keep/keep.go` (only if YAML config emission requires a parse hook). Excluded: `ml/app/**`, normalizer, sync code, tests under `tests/e2e/**` outside the env-injection smoke.
- **Shared Infrastructure Impact Sweep:** the three-mirror manifest is a shared protected fixture; the mirror-parity contract test is the canary; rollback is removing the new key from all three mirrors in a single revert commit.
- **Consumer Impact Sweep:** N/A (no renames/removals).

### Test Plan

| Test Type | Category | Scenario Mapping | File/Location | Expected Test Title | Command | Live System |
|-----------|----------|------------------|---------------|---------------------|---------|-------------|
| Mirror parity | `unit` | SCN-059-001 | `internal/config/secret_keys_test.go` | `TestSecretKeys_MirrorsYAMLManifest` (existing — must continue to pass) | `./smackerel.sh test unit` | No |
| Env injection | `integration` | SCN-059-002 | `internal/deploy/compose_contract_test.go` (extend) | `TestComposeEnvFileSharedAcrossCoreAndMlServices` (asserts both services declare `env_file: ${SMACKEREL_ENV_FILE}` and neither service introduces a per-service override that masks the secret) | `./smackerel.sh test integration` | No (compose file parse) |
| Application-layer boundary | `unit` | SCN-059-002 | `internal/connector/keep/keep_test.go` | `TestKeepAppPasswordReadOnlyFromSidecarNotCore` (greps `internal/` for `KEEP_GOOGLE_APP_PASSWORD` reads and asserts the only consumers are sidecar-facing config wiring, not Go runtime code paths) | `./smackerel.sh test unit` | No |
| Env injection live | `integration` | SCN-059-002 | `tests/integration/keep_secret_env_test.go` | `TestKeepSecretEnvReachesMlContainer` (asserts `smackerel-test-ml` env has both vars; core-container assertion is sidecar-boundary informational only) | `./smackerel.sh test integration` | Yes |
| Regression | `unit` | SCN-059-001 | `internal/config/secret_keys_test.go` | `TestSecretKeys_KeepAppPasswordRegistered` | `./smackerel.sh test unit` | No |

### Definition of Done

- [x] `KEEP_GOOGLE_APP_PASSWORD` present in all three mirrors and `TestSecretKeys_MirrorsYAMLManifest` exits 0. Evidence: `report.md#scope-1`
- [x] `KEEP_GOOGLE_EMAIL` flows through the standard non-secret env contract to both containers; `KEEP_GOOGLE_APP_PASSWORD` is delivered via the shared `app.env` (per existing `TELEGRAM_BOT_TOKEN` precedent — both core and ml read the same env_file) AND the application-layer boundary is enforced: zero references to `KEEP_GOOGLE_APP_PASSWORD` in any Go source under `internal/` or `cmd/` (only the Python sidecar reads it). Evidence: `report.md#scope-1`
- [x] No language-level fallback default introduced for either env var (`grep -E '(KEEP_GOOGLE_(EMAIL|APP_PASSWORD)).*:-' returns empty). Evidence: `report.md#scope-1`
- [x] `./smackerel.sh config generate --env dev` succeeds and the resolved `config/generated/dev.env` carries the secret placeholder marker for `KEEP_GOOGLE_APP_PASSWORD` per spec-052. Evidence: `report.md#scope-1`
- [x] Adversarial regression: removing `KEEP_GOOGLE_APP_PASSWORD` from the YAML mirror fails the contract test. Evidence: `report.md#scope-1`
- [x] Change Boundary respected; zero unrelated file families changed. Evidence: `report.md#scope-1`

## Scope 2: `drift_ack_token` Config Field + Fail-Loud Connector Validation

**Status:** Done (2026-05-28; boundary-test conflict resolved via Scope 3 sidecar handshake — see SCN-059-019; Scope 2 narrows to EMAIL-only fail-loud at Go-core `Connect()`, KEEP_GOOGLE_APP_PASSWORD validation owned by sidecar handshake)
**Depends On:** Scope 1
**Surfaces:** `internal/connector/keep/keep.go`, `internal/connector/keep/keep_test.go`, `config/smackerel.yaml`

### Use Cases

#### SCN-059-003: `drift_ack_token` Round-Trips Through `parseKeepConfig`

```gherkin
Given a `KeepConfig` YAML block containing `drift_ack_token: "2026-05-28"`
When `parseKeepConfig` runs
Then the returned `KeepConfig.DriftAckToken` equals `"2026-05-28"`
And omitting the field yields an empty string (not an error)
And a non-string value yields a parse error referencing the field name
```

#### SCN-059-004: Missing `KEEP_GOOGLE_EMAIL` Fails Loud at Go-Core `Connect()` When Live Mode Is Enabled

```gherkin
Given `sync_mode: gkeepapi`, `gkeep_enabled: true`, `warning_acknowledged: true`
And `KEEP_GOOGLE_EMAIL` is unset in the Go-core process environment
When the connector `Connect()` runs
Then it returns an error containing `"KEEP_GOOGLE_EMAIL is required"`
And no NATS publish has occurred

Notes:
- The Go core MUST NOT reference the string literal `KEEP_GOOGLE_APP_PASSWORD` anywhere; that key is owned exclusively by the ML sidecar (boundary test `TestKeepAppPasswordReadOnlyFromSidecarNotCore` enforces this).
- Empty-`KEEP_GOOGLE_APP_PASSWORD` validation is performed by the ML sidecar and surfaced to the Go core via the Scope 3 NATS handshake (`keep.sidecar.handshake` reply with `status: "error"`, `error: "KEEP_GOOGLE_APP_PASSWORD is required"`), which the Go core's `Connect()` surfaces as a fail-loud error. See SCN-059-019.
```

#### SCN-059-005: Poll-Interval Floor Is A Single Value

```gherkin
Given `gkeep_poll_interval: 5m` (below the floor)
When `parseKeepConfig` runs
Then it returns an error referencing the documented minimum
And the documented minimum value is consistent between code, `config/smackerel.yaml` comments, and spec.md
```

### Implementation Plan

- Extend the existing `parseKeepConfig` switch in `internal/connector/keep/keep.go` with a `drift_ack_token` case using the same `sc[...].(type)` pattern already used for `warning_acknowledged`.
- Add `DriftAckToken string` to the `KeepConfig` struct adjacent to `WarningAcknowledged`.
- Add a `Connect()`-time precondition block: when `sync_mode ∈ {SyncModeGkeepapi, SyncModeHybrid}` AND `gkeep_enabled: true`, `os.Getenv("KEEP_GOOGLE_EMAIL")` MUST be non-empty; if not, return an error using field-name-only language (no value, no length, no hash).
- The Go core MUST NOT read or reference `KEEP_GOOGLE_APP_PASSWORD` in any form (string literal, helper, or constant). The boundary test `TestKeepAppPasswordReadOnlyFromSidecarNotCore` (Scope 1) enforces this — any attempt to add an `os.Getenv("KEEP_GOOGLE_APP_PASSWORD")` check to the Go core would fail Scope 1's boundary test. Password emptiness is validated by the ML sidecar's `keep_bridge` handshake handler and surfaced to the Go core via Scope 3's NATS reply.
- The Go-core EMAIL precondition runs BEFORE the existing `warning_acknowledged` gate so a missing email never silently falls back.
- Resolve OQ-059-01 by selecting the 15-min floor (matching existing `parseKeepConfig` enforcement); update the spec narrative through a follow-up note in `report.md` and adjust any in-code constant naming for clarity (`gkeepPollIntervalFloor = 15 * time.Minute`).
- Add `drift_ack_token: ""` placeholder under `connectors.google-keep` in `config/smackerel.yaml`.
- **Change Boundary:** allowed = `internal/connector/keep/keep.go`, `internal/connector/keep/keep_test.go`, `config/smackerel.yaml`. Excluded: `ml/app/**`, NATS bridge code (Scope 3), metrics, docs.
- **Consumer Impact Sweep:** N/A (no renames; new field is additive).

### Test Plan

| Test Type | Category | Scenario Mapping | File/Location | Expected Test Title | Command | Live System |
|-----------|----------|------------------|---------------|---------------------|---------|-------------|
| Config parse | `unit` | SCN-059-003 | `internal/connector/keep/keep_test.go` | `TestParseKeepConfigParsesDriftAckToken` | `./smackerel.sh test unit` | No |
| Config parse negative | `unit` | SCN-059-003 | `internal/connector/keep/keep_test.go` | `TestParseKeepConfigRejectsNonStringDriftAckToken` | `./smackerel.sh test unit` | No |
| Connect fail-loud (EMAIL) | `unit` | SCN-059-004 | `internal/connector/keep/keep_test.go` | `TestConnectFailsLoudWhenKeepEmailMissingInLiveMode` | `./smackerel.sh test unit` | No |
| Poll floor | `unit` | SCN-059-005 | `internal/connector/keep/keep_test.go` | `TestParseKeepConfigEnforcesPollIntervalFloor` | `./smackerel.sh test unit` | No |
| Boundary regression | `unit` | SCN-059-004 | `internal/connector/keep/keep_test.go` | `TestKeepAppPasswordReadOnlyFromSidecarNotCore` (Scope 1 test — must continue to pass after Scope 2; proves Go core still has zero references to `KEEP_GOOGLE_APP_PASSWORD`) | `./smackerel.sh test unit` | No |

### Definition of Done

- [x] `DriftAckToken` parses correctly; both positive and negative tests pass. Evidence: `report.md#scope-2`
- [x] `Connect()` fails loud for missing `KEEP_GOOGLE_EMAIL` when live mode is enabled. Empty-`KEEP_GOOGLE_APP_PASSWORD` validation is performed by the ML sidecar (Scope 3 handshake — see SCN-059-019) and surfaced to the Go core as a fail-loud `Connect()` error; the Go core itself MUST NOT reference `KEEP_GOOGLE_APP_PASSWORD`, and the Scope 1 boundary test `TestKeepAppPasswordReadOnlyFromSidecarNotCore` MUST continue to pass after Scope 2 lands. Evidence: `report.md#scope-2`
- [x] Poll-interval floor is a single named constant; spec-narrative drift recorded in `report.md` for follow-up. Evidence: `report.md#scope-2`
- [x] No language-level fallback default (`os.Getenv("KEEP_..."); if v == "" { v = "..." }`) anywhere in the connector. Evidence: `report.md#scope-2`
- [x] Adversarial regression: Scope 1 boundary test `TestKeepAppPasswordReadOnlyFromSidecarNotCore` re-runs green after Scope 2 changes (proves no Go-core reference to `KEEP_GOOGLE_APP_PASSWORD` was introduced). Evidence: `report.md#scope-2`
- [x] Change Boundary respected. Evidence: `report.md#scope-2`

## Scope 3: NATS Request/Reply Bridge

**Status:** Not started
**Depends On:** Scope 2
**Surfaces:** `internal/connector/keep/keep.go`, `ml/app/keep_bridge.py`, `ml/app/main.py`, `tests/integration/keep_*_test.go`, `ml/tests/test_keep_bridge.py`

### Use Cases

#### SCN-059-006: Go Connector Publishes `keep.sync.request` And Parses Reply

```gherkin
Given the connector is in `sync_mode: gkeepapi`, breaker CLOSED, cursor non-empty
When `Sync()` runs
Then it publishes a `KeepSyncRequest{cursor, request_id}` on subject `keep.sync.request` using `nats.Request` with timeout `gkeepRequestTimeout`
And it consumes a single reply on the inbox, JSON-decoded as `KeepSyncResponse`
And on `status: "ok"` each `GkeepNote` is normalized into a `RawArtifact` via `normalizer.Normalize`
And the connector advances its cursor to `KeepSyncResponse.cursor`
```

#### SCN-059-007: Sidecar Subscribes And Replies With Schema-Conformant Payload

```gherkin
Given the ML sidecar boots with `KEEP_GOOGLE_EMAIL` and `KEEP_GOOGLE_APP_PASSWORD` set and `gkeepapi.Keep().login` is stubbed
When a `keep.sync.request` arrives
Then `keep_bridge.handle_sync_request` is invoked with the decoded payload
And `msg.respond` carries a `KeepSyncResponse` with `status: "ok"`, `schema_version: 1`, `notes: [...]`, and a fresh `cursor`
And the request_id from the request is echoed in the sidecar structured log
```

#### SCN-059-008: Hybrid Mode Dedups By `note_id` Across Takeout And gkeepapi

```gherkin
Given an existing artifact whose source identifier is the Keep `note_id` "abc"
And a `gkeepapi` sync returns a `GkeepNote` with `note_id: "abc"`
When the normalizer processes the response
Then the existing artifact is versioned (not duplicated)
And the dedup key resolves to the same canonical artifact key as the Takeout origin
```

#### SCN-059-019: Sidecar Handshake Rejects Empty `KEEP_GOOGLE_APP_PASSWORD` And Surfaces As Fail-Loud `Connect()` Error

```gherkin
Given `sync_mode: gkeepapi`, `gkeep_enabled: true`, `warning_acknowledged: true`
And `KEEP_GOOGLE_EMAIL` is set in the Go-core process environment
And `KEEP_GOOGLE_APP_PASSWORD` is empty/unset in the ML sidecar process environment
When the Go-core connector `Connect()` runs
Then it publishes a `KeepHandshakeRequest{request_id}` on subject `keep.sidecar.handshake` using `nats.Request` with a short timeout (5 s)
And the sidecar handler decodes the request, checks `os.environ.get("KEEP_GOOGLE_APP_PASSWORD", "")`, and replies with `KeepHandshakeResponse{status: "error", error: "KEEP_GOOGLE_APP_PASSWORD is required", schema_version: 1}` when it is empty
And the Go core surfaces the sidecar error verbatim as a `Connect()` error (no NATS publish on `keep.sync.request` ever occurs)
And the same flow with a non-empty `KEEP_GOOGLE_APP_PASSWORD` replies `status: "ok"` and `Connect()` proceeds
And the sidecar reply never echoes the password value, length, or hash
And the Go core does NOT reference the string literal `KEEP_GOOGLE_APP_PASSWORD` anywhere (boundary test `TestKeepAppPasswordReadOnlyFromSidecarNotCore` remains green)
```

### Implementation Plan

- Add `KeepSyncRequest` and `KeepSyncResponse` Go types alongside `GkeepNote` in `internal/connector/keep/keep.go` with explicit JSON tags matching the design schema.
- Add `KeepHandshakeRequest{RequestID string}` and `KeepHandshakeResponse{Status string, Error *string, SchemaVersion int}` Go types alongside the sync envelope.
- Replace the `"gkeepapi bridge not connected"` stub in `syncGkeepapi` with: build `KeepSyncRequest{Cursor: cursor.String(), RequestID: newRequestID()}`, call `nc.Request(ctx, gkeepRequestSubject, payload, gkeepRequestTimeout)`, decode response, hand notes to existing normalizer.
- Add a `Connect()`-time sidecar handshake step (after the local EMAIL precondition from Scope 2): build `KeepHandshakeRequest{RequestID: newRequestID()}`, call `nc.Request(ctx, gkeepHandshakeSubject, payload, gkeepHandshakeTimeout)`, decode `KeepHandshakeResponse`; on `status: "error"` return the sidecar `error` field verbatim as the Go-core `Connect()` error; on `status: "ok"` proceed. The Go core MUST NOT reference the string literal `KEEP_GOOGLE_APP_PASSWORD` anywhere — boundary test `TestKeepAppPasswordReadOnlyFromSidecarNotCore` (Scope 1) MUST remain green.
- Define a `newRequestID()` helper producing `k-<unix>-<6 hex>` strings.
- Constants: `gkeepRequestSubject = "keep.sync.request"`, `gkeepRequestTimeout = 120 * time.Second`, `gkeepHandshakeSubject = "keep.sidecar.handshake"`, `gkeepHandshakeTimeout = 5 * time.Second`, `gkeepDriftFailureThreshold = 3`.
- Add `register_nats_handler(nc)` in `ml/app/keep_bridge.py`: subscribe to BOTH `keep.sync.request` (calls `handle_sync_request`) AND `keep.sidecar.handshake` (calls a new `handle_handshake_request(payload)` that returns `{"status": "ok", "schema_version": 1}` when `os.environ.get("KEEP_GOOGLE_APP_PASSWORD", "")` is non-empty, else `{"status": "error", "error": "KEEP_GOOGLE_APP_PASSWORD is required", "schema_version": 1}`). Both subscriptions call `await msg.respond(payload)` with the JSON-encoded reply. The handshake handler MUST NOT log the password value, length, or hash.
- Wire the registration call in `ml/app/main.py` alongside the existing sidecar subscribers (search for `register_nats_handler` or `subscribe(...)` calls used by Drive/Photos/YouTube and insert next to them).
- Wrap `handle_sync_request` callsite so any unhandled exception becomes a `KeepSyncResponse{status: "error", notes: [], cursor: <echo>, error: <generic class name>, schema_version: 1}` reply rather than a NATS-level failure.
- The Go connector treats `status: "error"` as drift (Scope 4 handles the breaker), except when `error` starts with the stable prefix `"gkeepapi authentication failed"` which causes a Connect-style fatal (NOT a drift count). Handshake errors are surfaced at `Connect()` time and do NOT count as drift.
- **Change Boundary:** allowed = `internal/connector/keep/keep.go`, `internal/connector/keep/keep_test.go`, `ml/app/keep_bridge.py`, `ml/app/main.py`, `ml/tests/test_keep_bridge.py`, new test file under `tests/integration/`. Excluded: metrics, docs, normalizer changes beyond what dedup already supports.
- **Shared Infrastructure Impact Sweep:** `ml/app/main.py` is a shared sidecar entrypoint; the canary is a sidecar boot integration test that asserts ALL existing subscribers still register after the new one is added; rollback is reverting the single registration line.

### Test Plan

| Test Type | Category | Scenario Mapping | File/Location | Expected Test Title | Command | Live System |
|-----------|----------|------------------|---------------|---------------------|---------|-------------|
| Publisher | `unit` | SCN-059-006 | `internal/connector/keep/keep_test.go` | `TestSyncGkeepapiPublishesRequestAndDecodesResponse` (uses test NATS double) | `./smackerel.sh test unit` | No |
| Request ID | `unit` | SCN-059-006 | `internal/connector/keep/keep_test.go` | `TestNewRequestIDMatchesPattern` | `./smackerel.sh test unit` | No |
| Sidecar handler | `unit` | SCN-059-007 | `ml/tests/test_keep_bridge.py` | `test_handle_sync_request_returns_ok_envelope` | `./smackerel.sh test unit` | No |
| Sidecar handler error | `unit` | SCN-059-007 | `ml/tests/test_keep_bridge.py` | `test_handle_sync_request_wraps_exception_as_error_envelope` | `./smackerel.sh test unit` | No |
| Bridge live | `integration` | SCN-059-006, SCN-059-007 | `tests/integration/keep_bridge_test.go` | `TestKeepBridgeRoundTripsRequestReplyAgainstLiveSidecar` | `./smackerel.sh test integration` | Yes |
| Dedup | `integration` | SCN-059-008 | `tests/integration/keep_bridge_test.go` | `TestKeepHybridModeDedupsNoteIdAcrossTakeoutAndGkeepapi` | `./smackerel.sh test integration` | Yes |
| Sidecar boot regression | `integration` | SCN-059-007 | `tests/integration/ml_sidecar_boot_test.go` | `TestMlSidecarRegistersAllExistingSubscribersIncludingKeep` | `./smackerel.sh test integration` | Yes |
| Handshake reject empty pw | `unit` | SCN-059-019 | `ml/tests/test_keep_bridge.py` | `test_handle_handshake_request_rejects_empty_app_password` | `./smackerel.sh test unit` | No |
| Handshake accept set pw | `unit` | SCN-059-019 | `ml/tests/test_keep_bridge.py` | `test_handle_handshake_request_accepts_non_empty_app_password` | `./smackerel.sh test unit` | No |
| Handshake Go publisher | `unit` | SCN-059-019 | `internal/connector/keep/keep_test.go` | `TestConnectPublishesHandshakeAndSurfacesSidecarErrorVerbatim` (uses test NATS double) | `./smackerel.sh test unit` | No |
| Handshake live | `integration` | SCN-059-019 | `tests/integration/keep_bridge_test.go` | `TestKeepConnectHandshakeFailsLoudWhenSidecarAppPasswordEmpty` (boots sidecar with empty `KEEP_GOOGLE_APP_PASSWORD` and asserts Go `Connect()` returns the sidecar error verbatim with zero `keep.sync.request` publishes) | `./smackerel.sh test integration` | Yes |
| Boundary regression (post-Scope-3) | `unit` | SCN-059-019 | `internal/connector/keep/keep_test.go` | `TestKeepAppPasswordReadOnlyFromSidecarNotCore` (Scope 1 test — must remain green after Scope 3 handshake wiring) | `./smackerel.sh test unit` | No |
| Regression | `integration` | SCN-059-006 | `tests/integration/keep_bridge_test.go` | `TestSyncGkeepapiReturnsLoudErrorOnNatsTimeout` (timeout = drift signal — counts toward breaker in Scope 4) | `./smackerel.sh test integration` | Yes |

### Definition of Done

- [ ] Go connector publishes and parses replies on the new subjects with the 120 s timeout. Evidence: `report.md#scope-3`
- [ ] Sidecar subscribes via `register_nats_handler` and replies with schema-conformant `KeepSyncResponse` envelopes for both success and exception paths. Evidence: `report.md#scope-3`
- [ ] Sidecar handshake handler is subscribed on `keep.sidecar.handshake` and replies fail-loud (`status: "error"`, `error: "KEEP_GOOGLE_APP_PASSWORD is required"`) when the sidecar env var is empty, success when set; reply never echoes value/length/hash. Evidence: `report.md#scope-3`
- [ ] Go-core `Connect()` performs the handshake after the local EMAIL precondition, surfaces sidecar error verbatim, and emits zero `keep.sync.request` publishes when the handshake errors. Evidence: `report.md#scope-3`
- [ ] Scope 1 boundary test `TestKeepAppPasswordReadOnlyFromSidecarNotCore` remains green after Scope 3 (Go core still has zero references to `KEEP_GOOGLE_APP_PASSWORD`). Evidence: `report.md#scope-3`
- [ ] Live-stack integration proves end-to-end `keep.sync.request → KeepSyncResponse → RawArtifact`. Evidence: `report.md#scope-3`
- [ ] Hybrid-mode dedup by `note_id` proven against the live store; zero duplicate artifacts. Evidence: `report.md#scope-3`
- [ ] Sidecar-boot canary proves existing subscribers (Drive, Photos, YouTube) still register after the new one is wired. Evidence: `report.md#scope-3`
- [ ] No subprocess shellout, no `python3` invocation from Go (`grep -RE 'exec\.Command.*python' internal/connector/keep` returns empty). Evidence: `report.md#scope-3`
- [ ] No write-intent `gkeep_*` symbol in either codebase (`grep -RE 'gkeep.*\.(add|edit|archive|trash|delete|save)' internal/connector/keep ml/app` returns empty). Evidence: `report.md#scope-3`
- [ ] Change Boundary respected. Evidence: `report.md#scope-3`

## Scope 4: Schema Validation + Drift Circuit Breaker

**Status:** Not started
**Depends On:** Scope 3
**Surfaces:** `internal/connector/keep/keep.go`, `internal/connector/keep/keep_test.go`

### Use Cases

#### SCN-059-009: `validateGkeepResponse` Catches All Schema Drift Classes

```gherkin
Given a canonical `KeepSyncResponse` fixture
When each field is mutated (missing, wrong type, wrong schema_version, `status: "error"` with empty error, etc.)
Then `validateGkeepResponse` returns a non-nil error for each mutation
And the canonical fixture returns nil
```

#### SCN-059-010: Breaker Transitions `closed → tripping → open → closed`

```gherkin
Given the breaker starts in CLOSED with `driftFailures = 0`
When the first failure is recorded
Then state is TRIPPING and `driftFailures = 1`
When a 2nd then 3rd failure are recorded
Then state remains TRIPPING and `driftFailures = 3`
When a 4th consecutive failure is recorded
Then state transitions to OPEN
And subsequent `Sync()` calls return early without publishing on NATS
When `Connect()` runs with a new `drift_ack_token`
Then state resets to CLOSED and `driftFailures = 0`
```

#### SCN-059-011: Sidecar Auth Errors Do NOT Count As Drift

```gherkin
Given the sidecar replies with `status: "error"`, `error: "gkeepapi authentication failed"`
When `Sync()` runs
Then the connector returns a Connect-style fatal error (operator-config issue)
And `driftFailures` is NOT incremented
And the breaker state does not advance
```

#### SCN-059-012: Successful Sync Resets `driftFailures` In TRIPPING

```gherkin
Given the breaker is TRIPPING with `driftFailures = 2`
When the next `Sync()` succeeds
Then `driftFailures = 0`
And state is CLOSED
```

### Implementation Plan

- Add `validateGkeepResponse(*KeepSyncResponse) error` checking: `schema_version == 1`; `status ∈ {"ok","error"}`; on `"error"` ⇒ `error != nil && *error != ""` AND `notes == []`; on `"ok"` ⇒ every `GkeepNote` field present with correct type and `note_id != ""`.
- Add `type breakerState int` with constants `breakerClosed`, `breakerTripping`, `breakerOpen`; embed `driftFailures int`, `breakerState breakerState`, `lastAckToken string` on the Connector.
- Drive the transitions in `syncGkeepapi`: on validation failure or `status: "error"` (non-auth), call `b.recordFailure()`; on success, `b.recordSuccess()`; on `Connect()`, compare the freshly parsed `DriftAckToken` to `lastAckToken` and reset if changed (and store the new value).
- Classify auth errors by stable prefix matching on the sidecar `error` field (`strings.HasPrefix(*resp.Error, "gkeepapi authentication failed")`) — these return a distinct error class that the caller surfaces as a Connect-style failure.
- When state is OPEN, `syncGkeepapi` returns immediately with a stable sentinel error; no NATS publish.
- **Change Boundary:** allowed = `internal/connector/keep/keep.go`, `internal/connector/keep/keep_test.go`, plus a new fixture file. Excluded: metrics (Scope 5), docs (Scope 6).
- **Consumer Impact Sweep:** N/A.

### Test Plan

| Test Type | Category | Scenario Mapping | File/Location | Expected Test Title | Command | Live System |
|-----------|----------|------------------|---------------|---------------------|---------|-------------|
| Validation matrix | `unit` | SCN-059-009 | `internal/connector/keep/keep_test.go` | `TestValidateGkeepResponseAcceptsCanonicalFixtureAndRejectsEveryMutation` | `./smackerel.sh test unit` | No |
| Breaker | `unit` | SCN-059-010 | `internal/connector/keep/keep_test.go` | `TestDriftBreakerTransitionsClosedTrippingOpenAndResetsOnTokenRotation` | `./smackerel.sh test unit` | No |
| Auth classification | `unit` | SCN-059-011 | `internal/connector/keep/keep_test.go` | `TestSidecarAuthErrorDoesNotIncrementDriftFailures` | `./smackerel.sh test unit` | No |
| Recovery | `unit` | SCN-059-012 | `internal/connector/keep/keep_test.go` | `TestDriftBreakerResetsOnSuccessFromTripping` | `./smackerel.sh test unit` | No |
| Live drift | `integration` | SCN-059-010 | `tests/integration/keep_bridge_test.go` | `TestKeepBridgeBreakerTripsAfterFourConsecutiveMalformedResponses` (uses sidecar fixture mode that returns invalid envelopes) | `./smackerel.sh test integration` | Yes |
| Regression | `unit` | SCN-059-010 | `internal/connector/keep/keep_test.go` | `TestOpenBreakerSkipsNatsPublish` | `./smackerel.sh test unit` | No |

### Definition of Done

- [ ] `validateGkeepResponse` catches every defined drift class with a per-mutation test row. Evidence: `report.md#scope-4`
- [ ] All four breaker transitions covered by unit tests. Evidence: `report.md#scope-4`
- [ ] Sidecar auth errors classified as Connect-fail (NOT drift) by unit test. Evidence: `report.md#scope-4`
- [ ] Live integration proves a real malformed-response stream trips the breaker after the 4th failure. Evidence: `report.md#scope-4`
- [ ] OPEN-state breaker skips all NATS publishes (adversarial: a publish during OPEN would be detected by a test NATS double). Evidence: `report.md#scope-4`
- [ ] No persistence of breaker state across container restarts; restart with same token does NOT clear the breaker (it re-trips on first failure). Evidence: `report.md#scope-4`
- [ ] Change Boundary respected. Evidence: `report.md#scope-4`

## Scope 5: Observability (Metrics + Structured Logs)

**Status:** Not started
**Depends On:** Scope 4
**Surfaces:** `internal/metrics/*.go`, `internal/connector/keep/keep.go`

### Use Cases

#### SCN-059-013: Drift Counter Increments Exactly Once Per State Entry Into OPEN

```gherkin
Given the breaker transitions CLOSED → TRIPPING → ... → OPEN
When the OPEN state is entered
Then `smackerel_keep_protocol_drift_detected_total{connector_id=...}` increments by exactly 1
And subsequent `Sync()` calls while still OPEN do NOT increment the counter
And a token rotation followed by a fresh trip increments the counter by 1 again
```

#### SCN-059-014: Health Reports `HealthError` While Breaker Is OPEN

```gherkin
Given the breaker is OPEN
When `Health()` is called
Then it returns `HealthError`
And on token rotation + successful Sync the next `Health()` call returns `HealthHealthy`
```

#### SCN-059-015: Sync Duration Histogram And Notes Counter Populated On Success

```gherkin
Given a successful `Sync()` returns N notes
When `/metrics` is scraped from the live stack
Then `smackerel_keep_gkeep_sync_duration_seconds_count` increments by 1
And `smackerel_keep_gkeep_notes_returned_total` increments by N
And neither metric carries any label value containing email or password material
```

### Implementation Plan

- Add `KeepProtocolDriftDetected = prometheus.NewCounterVec(..., []string{"connector_id"})`, `KeepGkeepSyncDuration = prometheus.NewHistogram(...)`, `KeepGkeepNotesReturned = prometheus.NewCounter(...)` in the existing `internal/metrics` package matching the existing connector-metric registration pattern.
- Increment the drift counter at the exact CLOSED/TRIPPING → OPEN transition (once per entry), NOT on every failure in OPEN.
- Record duration via `time.Now()` / `Observe(time.Since(start).Seconds())` around the NATS request + decode block.
- Add `slog.Error("keep_protocol_drift_detected", "connector_id", id, "consecutive_failures", n, "last_request_id", reqID)` at the OPEN entry; add `slog.Info("keep_sync_request"|"keep_sync_response", "request_id", reqID, ...)` at the request/response boundaries (no payload bodies, no email, no password).
- Force `connectorHealth = HealthError` while the breaker is OPEN; ensure the existing `Health()` reads this transition without races (protect with the existing connector mutex).
- **Change Boundary:** allowed = `internal/metrics/*.go`, `internal/connector/keep/keep.go`, `internal/connector/keep/keep_test.go`. Excluded: docs (Scope 6), bridge code beyond instrumentation hooks.
- **Consumer Impact Sweep:** N/A.

### Test Plan

| Test Type | Category | Scenario Mapping | File/Location | Expected Test Title | Command | Live System |
|-----------|----------|------------------|---------------|---------------------|---------|-------------|
| Counter once | `unit` | SCN-059-013 | `internal/connector/keep/keep_test.go` | `TestDriftCounterIncrementsExactlyOncePerOpenEntry` | `./smackerel.sh test unit` | No |
| Health | `unit` | SCN-059-014 | `internal/connector/keep/keep_test.go` | `TestHealthReportsErrorWhileBreakerOpenAndRecoversAfterTokenRotation` | `./smackerel.sh test unit` | No |
| Metrics live | `integration` | SCN-059-015 | `tests/integration/keep_metrics_test.go` | `TestKeepGkeepMetricsExposedViaPrometheusEndpoint` | `./smackerel.sh test integration` | Yes |
| Log redaction | `unit` | SCN-059-015 | `internal/connector/keep/keep_test.go` | `TestKeepStructuredLogsDoNotContainEmailOrPassword` (asserts neither value appears in captured log handler output for any code path) | `./smackerel.sh test unit` | No |
| Regression | `integration` | SCN-059-013 | `tests/integration/keep_metrics_test.go` | `TestKeepDriftCounterStableLabelCardinality` (label set never grows with per-request values) | `./smackerel.sh test integration` | Yes |

### Definition of Done

- [ ] Three new metrics registered and exposed on the live `/metrics` endpoint. Evidence: `report.md#scope-5`
- [ ] Drift counter increments exactly once per OPEN entry (adversarial: repeated `Sync()` in OPEN does not advance it). Evidence: `report.md#scope-5`
- [ ] `Health()` returns `HealthError` while OPEN and recovers on token rotation. Evidence: `report.md#scope-5`
- [ ] No log line or metric label carries `KEEP_GOOGLE_EMAIL` or `KEEP_GOOGLE_APP_PASSWORD` values (`grep` over captured test logs returns empty). Evidence: `report.md#scope-5`
- [ ] Histogram and notes counter populated on success path. Evidence: `report.md#scope-5`
- [ ] Change Boundary respected. Evidence: `report.md#scope-5`

## Scope 6: Operator Documentation

**Status:** Not started
**Depends On:** Scope 5
**Surfaces:** `docs/Operations.md`

### Use Cases

#### SCN-059-016: Operator Can Complete Enablement Using Only Documented `./smackerel.sh` Commands

```gherkin
Given the operator has a Google account with 2-Step Verification enabled
When they follow the "Google Keep live sync" subsection of `docs/Operations.md`
Then they can generate a Google App Password
And add `KEEP_GOOGLE_APP_PASSWORD` to the deploy-overlay sops-encrypted secret bundle
And set `KEEP_GOOGLE_EMAIL` plus the `connectors.google-keep` block in `config/smackerel.yaml`
And run only `./smackerel.sh config generate`, `./smackerel.sh deploy-target <target> apply`, `./smackerel.sh status`, and `./smackerel.sh logs`
And no ad-hoc `docker compose`, `pytest`, `go test`, or `python3` invocations appear in the runbook
```

#### SCN-059-017: Operator Can Recover From A Tripped Breaker From The Runbook Alone

```gherkin
Given the breaker is OPEN and `smackerel_keep_protocol_drift_detected_total` has incremented
When the operator follows the "Recovering from a tripped breaker" subsection
Then the steps cover: inspect logs, optionally bump `gkeepapi` pin in `ml/requirements.txt`, rotate `drift_ack_token`, regenerate config bundle, redeploy, verify
And the runbook explicitly states no CLI verb or HTTP endpoint exists for ack
And the runbook references `docs/Deployment.md` for the bundle redeploy mechanics rather than duplicating them
```

#### SCN-059-018: Runbook Carries No Env-Specific Values

```gherkin
Given the new subsection is committed
When `bash .github/bubbles/scripts/pii-scan.sh` runs against the staged diff
Then it exits 0
And no real email, hostname, IP, or tailnet identifier appears
And every example uses generic placeholders (`<operator-email>`, `<any-non-empty-string>`)
```

### Implementation Plan

- Add a new `### Google Keep live sync` subsection under the existing Connectors section of `docs/Operations.md`.
- Subsection structure (mirroring existing connector subsections):
  1. **Overview** — what live sync is, the gkeepapi fragility caveat, and a pointer back to spec 059.
  2. **Prerequisites** — 2-Step Verification, App Password generation steps (linked to Google's published help if a stable URL exists; otherwise descriptive only).
  3. **Initial enablement** — the 5-step list from `design.md § (d) Operator Workflow > Initial enablement`, using only `./smackerel.sh` commands and placeholder values.
  4. **Recovering from a tripped breaker** — the 6-step recovery list from `design.md § (d) > Recovering from a tripped breaker`.
  5. **Rotating the App Password** — short reference to initial enablement steps 2 and 5–7.
  6. **What you must NOT do** — no ad-hoc `docker compose`; no manual edits to `config/generated/*.env`; no language-level fallback values; no plaintext password commits.
  7. **Cross-references** — link to specs 051, 052, 054 (notification escalation), and `docs/Deployment.md`.
- Use only generic placeholders; never inline a real email or hostname.
- **Change Boundary:** allowed = `docs/Operations.md` only. Excluded: source code, tests, config.
- **Consumer Impact Sweep:** N/A.

### Test Plan

| Test Type | Category | Scenario Mapping | File/Location | Expected Test Title | Command | Live System |
|-----------|----------|------------------|---------------|---------------------|---------|-------------|
| Regression baseline | `artifact` | SCN-059-016, SCN-059-017 | `specs/059-google-keep-live-mode` (baseline registry) | `regression-baseline-guard.sh` exit 0 | `timeout 600 bash .github/bubbles/scripts/regression-baseline-guard.sh specs/059-google-keep-live-mode --verbose` | No |
| PII scan | `artifact` | SCN-059-018 | staged diff | `pii-scan.sh` exit 0 | `bash .github/bubbles/scripts/pii-scan.sh` | No |
| Runbook command discipline | `artifact` | SCN-059-016 | `docs/Operations.md` Google Keep subsection | `grep -E '(docker compose|pytest|python3|go test)' docs/Operations.md` (within the new subsection) returns empty | `./smackerel.sh check` (extend if a doc-lint hook exists) | No |
| Cross-reference completeness | `artifact` | SCN-059-017 | `docs/Operations.md` | `grep` confirms the new subsection mentions specs 051, 052, 054, and `docs/Deployment.md` | manual + `grep` | No |

### Definition of Done

- [ ] New subsection exists in `docs/Operations.md` with all seven structural pieces. Evidence: `report.md#scope-6`
- [ ] Runbook references only `./smackerel.sh` commands; no ad-hoc invocations leak in. Evidence: `report.md#scope-6`
- [ ] No env-specific values (`pii-scan.sh` exit 0). Evidence: `report.md#scope-6`
- [ ] Cross-references to specs 051, 052, 054 and `docs/Deployment.md` present. Evidence: `report.md#scope-6`
- [ ] Recovery procedure explicitly states no CLI verb or HTTP endpoint exists for drift ack. Evidence: `report.md#scope-6`
- [ ] `regression-baseline-guard.sh` exits 0 after baseline refresh. Evidence: `report.md#scope-6`
- [ ] Change Boundary respected (only `docs/Operations.md` changed). Evidence: `report.md#scope-6`

## Cross-Scope Certification Gates

The Keep live sync spans the shared three-mirror secret manifest (Scope 1), the shared `ml/app/main.py` sidecar bootstrap (Scope 3), and the connector contract surface that other connectors will follow as a pattern. The following gates remain unchecked until validation records direct evidence in `report.md`.

### Cross-Scope Canary Test Plan

| Test Type | Category | Scenario Mapping | File/Location | Expected Test Title | Command | Live System |
|-----------|----------|------------------|---------------|---------------------|---------|-------------|
| Secret manifest canary | `unit` | SCN-059-001 | `internal/config/secret_keys_test.go` | `TestSecretKeys_MirrorsYAMLManifest` | `./smackerel.sh test unit` | No |
| Sidecar boot canary | `integration` | SCN-059-007 | `tests/integration/ml_sidecar_boot_test.go` | `TestMlSidecarRegistersAllExistingSubscribersIncludingKeep` | `./smackerel.sh test integration` | Yes |
| Full vertical canary | `integration` | SCN-059-006, SCN-059-008, SCN-059-010 | `tests/integration/keep_bridge_test.go` | `TestKeepBridgeRoundTripsRequestReplyAgainstLiveSidecar` plus `TestKeepBridgeBreakerTripsAfterFourConsecutiveMalformedResponses` | `./smackerel.sh test integration` | Yes |

### Cross-Scope Certification DoD

- [ ] Independent canary suite for the shared secret manifest and sidecar bootstrap passes before broad suite reruns. Evidence: `report.md#cross-scope-certification-gates`
- [ ] Rollback for shared infrastructure (three-mirror manifest, sidecar subscriber registration) is a single-revert per change set and documented in `report.md`. Evidence: `report.md#cross-scope-certification-gates`
- [ ] Change Boundary respected across all six scopes (no foreign file family touched). Evidence: `report.md#cross-scope-certification-gates`
