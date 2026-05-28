# Design: 059 — Google Keep Live Sync (gkeepapi production hardening)

> **Author:** bubbles.design
> **Date:** May 28, 2026
> **Status:** Draft (post-clarify; awaiting plan)
> **Spec:** [spec.md](spec.md)
> **Workflow Mode:** `full-delivery` (per spec NC-1 resolution)

---

## Design Brief

**Current State.** `internal/connector/keep/keep.go` declares `SyncModeGkeepapi` and `SyncModeHybrid`, parses `gkeep_enabled` / `warning_acknowledged` / `poll_interval` config, and defines a Go `GkeepNote` struct that already matches the JSON the Python sidecar emits. `Sync()` dispatches to `syncGkeepapi()` which currently returns `"gkeepapi bridge not connected"`. The sidecar half — `ml/app/keep_bridge.py` — already implements `authenticate()`, `serialize_note()`, and `handle_sync_request()` with session caching, expiry, retry, and cursor filtering, and it reads `KEEP_GOOGLE_EMAIL` + `KEEP_GOOGLE_APP_PASSWORD` from the process environment. The wiring between the two halves does not exist: there is no NATS subscription in the sidecar entrypoint, no request publisher in the Go connector, no secret-manifest entry for the app password, and no drift circuit breaker.

**Target State.** A NATS request/reply bridge connects Go core (`internal/connector/keep`) to the ML sidecar (`ml/app/keep_bridge.py`). The Go connector publishes one `keep.sync.request` per poll cycle and consumes the matching `keep.sync.response` with the existing `GkeepNote` schema. `KEEP_GOOGLE_APP_PASSWORD` is registered in the Bucket-2 three-mirror secret manifest established by specs 051/052; `KEEP_GOOGLE_EMAIL` rides the standard env contract. A circuit-breaker state machine replaces the one-shot `warning_acknowledged` gate as the drift-recovery signal, driven by a new `drift_ack_token` config field. Operator workflow stays config-file-and-restart only; no new CLI verbs.

**Patterns to Follow.**

- **Secret manifest (three-mirror).** Add `KEEP_GOOGLE_APP_PASSWORD` to all three mirrors used by `TELEGRAM_BOT_TOKEN` today: [internal/config/secret_keys.go](../../internal/config/secret_keys.go) `secretKeys`, `config/smackerel.yaml` `infrastructure.secret_keys`, and `scripts/commands/config.sh`. The contract test [internal/config/secret_keys_test.go](../../internal/config/secret_keys_test.go) `TestSecretKeys_MirrorsYAMLManifest` already enforces parity and will fail loud on drift.
- **NATS request/reply.** Reuse the request/reply pattern already used by other Go → sidecar bridges (Drive, Photos, YouTube — see existing handlers in `ml/app/`). Subject naming follows the existing `<domain>.sync.request` shape (`keep.sync.request`, `keep.sync.response`).
- **Config parsing.** Extend `parseKeepConfig` in [internal/connector/keep/keep.go](../../internal/connector/keep/keep.go) for the new `drift_ack_token` field using the same `sc[...].(type)` pattern already in place for `warning_acknowledged` / `poll_interval`.
- **Fail-loud SST.** Sidecar `authenticate()` already raises on missing `KEEP_GOOGLE_EMAIL` / `KEEP_GOOGLE_APP_PASSWORD`; the Go side adds an equivalent startup check when `sync_mode ∈ {gkeepapi, hybrid}` AND `gkeep_enabled: true`. No fallback to `SyncModeTakeout`.
- **Metrics + structured log.** Add `keep_protocol_drift_detected` counter and structured `slog` event using the same shape as existing connector health counters in `internal/metrics`.

**Patterns to Avoid.**

- **Do NOT reuse `warning_acknowledged` as the drift-recovery toggle.** It is a one-shot risk gate that is `true` at steady state; flipping it to `false` for drift recovery would re-prompt the operator on every restart and conflate two distinct decisions (initial risk acceptance vs. post-drift investigation).
- **Do NOT add a `./smackerel.sh keep ack-drift` CLI verb.** The runtime-mutation surface is unnecessary for what is fundamentally a config-bump + restart; it also bypasses SST.
- **Do NOT shell out to `python3` from Go.** Keep the sidecar boundary clean; all `gkeepapi` calls live in the sidecar over NATS.
- **Do NOT log or emit metrics carrying `KEEP_GOOGLE_APP_PASSWORD` or any portion of it.** The sidecar's existing `authenticate()` deliberately raises generic errors; preserve that.
- **Do NOT silently retry past the drift window.** The breaker MUST trip and stay tripped until `drift_ack_token` rotates.

**Resolved Decisions.**

- Live path uses NATS request/reply between Go connector and Python sidecar (spec NC-2).
- Auth uses Google App Password (not master_token / gpsoauth) (spec NC-3); only the password is Bucket-2-managed; email is non-secret.
- Drift recovery uses a new `drift_ack_token` config field plus restart, NOT a CLI verb (spec NC-4).
- `warning_acknowledged` is retained as the one-shot initial risk gate and will be retired only when a future official-API spec lands (spec NC-5).
- Min poll-interval floor stays at 15 min (current `parseKeepConfig` enforcement); resolved to 15 min in spec/scopes (OQ-059-01).

**Open Questions.**

- OQ-059-01 — RESOLVED: min poll-interval floor is 15 min (matches current `parseKeepConfig` enforcement); spec.md updated.
- OQ-059-02 — Response timeout: spec mentions 120 s in code comment; should this become a configurable `gkeep_response_timeout` field, or stay a constant? Default to a constant unless the operator surface demands tuning.
- OQ-059-03 — Drift-detection window: spec says "> 3 consecutive polls" returning unexpected shapes trips the breaker. Confirm whether HTTP non-2xx and schema-validation failures share one counter or are tracked separately. Design proposes one shared counter; plan can split if needed.

---

## Purpose & Scope

Implement the `gkeepapi`-backed live sync path for the Keep connector so that, with valid Google credentials and operator acknowledgment, new and edited Keep notes flow into Smackerel within one poll cycle, with first-class drift detection and a clean operator recovery path. Out-of-scope items remain as listed in [spec.md § Out of Scope](spec.md).

---

## Architecture Overview

```
              ┌──────────────────────────────────────────────────────┐
              │             smackerel-core (Go)                      │
              │  internal/connector/keep                             │
              │   ┌──────────────────────────────────────────────┐  │
              │   │ Connector.Sync(ctx, cursor)                  │  │
              │   │   case SyncModeGkeepapi / SyncModeHybrid:    │  │
              │   │     syncGkeepapi(ctx, cursor)                │  │
              │   │       ├─ publish  keep.sync.request          │  │
              │   │       ├─ await    keep.sync.response         │  │
              │   │       ├─ validate GkeepNote schema           │  │
              │   │       ├─ drift state machine ─ on trip ───┐  │  │
              │   │       └─ normalizer.Normalize() per note   │  │  │
              │   └────────────────────────────────────────────┘  │  │
              │   ┌────────────────────────────────────────────┐  │  │
              │   │ drift breaker (in-memory, per-connector)   │<─┘  │
              │   │  states: closed → tripping → open          │     │
              │   │  reset on drift_ack_token rotation         │     │
              │   └────────────────────────────────────────────┘     │
              └────────────────────────┬─────────────────────────────┘
                                       │ NATS req/reply
              ┌────────────────────────┴─────────────────────────────┐
              │             smackerel-ml (Python sidecar)            │
              │  ml/app/keep_bridge.py                               │
              │   ┌──────────────────────────────────────────────┐   │
              │   │ subscribe("keep.sync.request")               │   │
              │   │   → handle_sync_request(data)                │   │
              │   │       authenticate()   # cached session       │   │
              │   │       keep.sync()                             │   │
              │   │       serialize_note() per gnote              │   │
              │   │   → reply on inbox subject                   │   │
              │   └──────────────────────────────────────────────┘   │
              └──────────────────────────────────────────────────────┘
```

The Go connector is the only `RawArtifact` producer; the sidecar produces JSON `GkeepNote` payloads only. Hybrid mode runs Takeout sync first and treats `gkeepapi` results as supplemental; dedup happens in the normalizer by Keep `note_id`.

---

## (a) NATS Subjects + `GkeepNote` Schema

### Subjects

| Subject | Direction | Payload type | Reply pattern |
|---|---|---|---|
| `keep.sync.request` | Go → sidecar | `KeepSyncRequest` JSON | NATS request/reply (inbox) |
| `keep.sync.response` | sidecar → Go (reply inbox) | `KeepSyncResponse` JSON | inbox |

No JetStream stream is created for these subjects; the exchange is synchronous request/reply per poll cycle. The Go publisher uses `nats.Request(ctx, subject, payload, timeout)`; the sidecar uses `subscribe + msg.respond`.

**Timeout.** Constant `gkeepRequestTimeout = 120 * time.Second` in Go (matches the spec narrative and existing comment in `syncGkeepapi`). See OQ-059-02.

### Request payload (`KeepSyncRequest`)

```json
{
  "cursor": "2026-05-28T14:33:21.000000Z",
  "request_id": "k-1716910101-3f9c2a"
}
```

| Field | Type | Required | Notes |
|---|---|---|---|
| `cursor` | string (RFC3339Nano UTC, `Z` suffix) | yes | Empty string ⇒ full sync. Matches the sidecar's existing cursor handling. |
| `request_id` | string | yes | Correlation handle for logs/traces; format `k-<unix>-<rand6hex>`. |

### Response payload (`KeepSyncResponse`)

```json
{
  "status": "ok",
  "notes": [ { /* GkeepNote */ } ],
  "cursor": "2026-05-28T15:01:07.000000Z",
  "error": null,
  "schema_version": 1
}
```

| Field | Type | Required | Notes |
|---|---|---|---|
| `status` | enum `ok` \| `error` | yes | On `error`, `notes` MUST be `[]` and `error` MUST be non-empty. |
| `notes` | `GkeepNote[]` | yes | Each element validated against the schema below. |
| `cursor` | string | yes | New cursor (RFC3339Nano UTC) or echoed input on error / no-progress. |
| `error` | string \| null | conditional | Generic; MUST NOT carry credential material. |
| `schema_version` | integer | yes | Hard-coded to `1` initially; bumped only on schema-breaking change. |

### `GkeepNote` schema (already declared in Go)

| Field | JSON type | Required | Validation |
|---|---|---|---|
| `note_id` | string | yes | non-empty; used as primary dedup key |
| `title` | string | yes | may be empty |
| `text_content` | string | yes | may be empty |
| `is_pinned` | bool | yes | — |
| `is_archived` | bool | yes | — |
| `is_trashed` | bool | yes | — |
| `color` | string | yes | known palette OR `"DEFAULT"` |
| `labels` | string[] | yes | empty array allowed |
| `collaborators` | string[] | yes | empty array allowed |
| `list_items` | object[] (`text: string`, `is_checked: bool`) | yes | empty array allowed |
| `modified_usec` | int64 | yes | ≥ 0; epoch-microseconds UTC |
| `created_usec` | int64 | yes | ≥ 0 |

**Validation rule (Go side).** Any unmarshal failure, unexpected JSON type, missing required field, or `schema_version != 1` is treated as a drift signal and feeds the circuit breaker (see (c)). Validation lives in a new `validateGkeepResponse` helper colocated with `syncGkeepapi`.

**Encoding.** UTF-8 JSON, no envelope wrapping beyond the payload itself. Subjects are unencrypted on the bus; NATS lives on the internal Docker network only.

---

## (b) Auth via `KEEP_GOOGLE_EMAIL` + `KEEP_GOOGLE_APP_PASSWORD`

### Classification

| Env var | Bucket | Source | Where it ends up |
|---|---|---|---|
| `KEEP_GOOGLE_EMAIL` | non-secret config | standard env contract via `config generate` | resolved `app.env`; surfaced in both core and ml containers |
| `KEEP_GOOGLE_APP_PASSWORD` | Bucket-2 managed secret (specs 051/052) | operator deploy-overlay sops-encrypted bundle | injected at container start; in-memory only in the sidecar |

### Three-mirror manifest updates (REQUIRED)

To add `KEEP_GOOGLE_APP_PASSWORD` as a managed secret, all three mirrors MUST be updated in the same change set, matching the existing `TELEGRAM_BOT_TOKEN` pattern:

1. [internal/config/secret_keys.go](../../internal/config/secret_keys.go) — append `"KEEP_GOOGLE_APP_PASSWORD"` to `var secretKeys`.
2. `config/smackerel.yaml` — append the same key under `infrastructure.secret_keys` in the same ordinal position.
3. `scripts/commands/config.sh` — append the same key to the shell-side mirror array.

The existing contract test `TestSecretKeys_MirrorsYAMLManifest` ([internal/config/secret_keys_test.go](../../internal/config/secret_keys_test.go)) fails loud on any drift between mirrors 1 and 2; the build pipeline already enforces parity to mirror 3. No new contract test is required.

### Resolution flow

```
operator deploy-overlay (sops-encrypted)
        │  (deploy-target apply)
        ▼
config generate --env <env> --bundle
        │
        ▼
app.env  ──────────────────────────────────► smackerel-ml container
        │                                        os.environ["KEEP_GOOGLE_APP_PASSWORD"]
        │                                        consumed by ml/app/keep_bridge.py::authenticate()
        │
        └──────────────────────────────────► smackerel-core container
                                                 KEEP_GOOGLE_EMAIL only (informational + startup check)
```

### Connector-side validation (fail-loud SST)

In `Connect()`, when `sync_mode ∈ {gkeepapi, hybrid}` AND `gkeep_enabled: true`:

- If `os.Getenv("KEEP_GOOGLE_EMAIL") == ""` → `connectError("KEEP_GOOGLE_EMAIL is required when sync_mode=%s and gkeep_enabled=true", mode)`.
- If `os.Getenv("KEEP_GOOGLE_APP_PASSWORD") == ""` → `connectError("KEEP_GOOGLE_APP_PASSWORD is required when sync_mode=%s and gkeep_enabled=true", mode)`.

These checks run BEFORE the existing `warning_acknowledged` gate so a missing secret never falls back to Takeout.

### Sidecar-side validation

`ml/app/keep_bridge.py::authenticate()` already raises `ValueError("KEEP_GOOGLE_EMAIL and KEEP_GOOGLE_APP_PASSWORD must be set for gkeepapi")` when either env var is missing. No change required.

### Forbidden patterns

- `${KEEP_GOOGLE_APP_PASSWORD:-...}` substitution anywhere — fail-loud `${VAR:?...}` only if it surfaces in Compose.
- Hand-edited entries in `config/generated/*.env`.
- Plaintext secret values in committed bundles, fixtures, or test data.
- Any log line or metric label containing the raw password value.

---

## (c) `drift_ack_token` + Circuit-Breaker State Machine

`warning_acknowledged: true` remains the one-shot initial risk gate (operator says "I understand this is an unofficial API"). It MUST stay `true` at steady state and is NOT toggled to recover from drift. A second, distinct field — `drift_ack_token` — drives drift recovery.

### New config field

```yaml
connectors:
  google-keep:
    sync_mode: gkeepapi
    gkeep_enabled: true
    warning_acknowledged: true       # one-shot risk gate (unchanged)
    drift_ack_token: "2026-05-28"    # rotate to ANY new value to clear a tripped breaker
```

| Field | Type | Default | Validation | Persistence |
|---|---|---|---|---|
| `drift_ack_token` | string | `""` (treated as "never acknowledged") | non-empty when breaker tripped; otherwise any value | source-of-truth in `config/smackerel.yaml`; flows through SST |

Parsing extends `parseKeepConfig` and stores the value on `KeepConfig`. The connector captures the last-seen token in memory; rotation is detected by comparing the in-memory snapshot to the freshly parsed value at `Connect()` time (which happens on container restart).

### State machine

```
                ┌───────────────────────────────────────────────┐
                │                  CLOSED                       │
                │  drift_failures = 0                            │
                │  every Sync() runs gkeepapi path normally     │
                └──────────────┬────────────────────────────────┘
                               │ validation failure OR HTTP non-2xx
                               ▼
                ┌───────────────────────────────────────────────┐
                │                  TRIPPING                     │
                │  drift_failures ∈ [1, 3]                       │
                │  each Sync() still attempts; failure → ++     │
                │  success → reset to CLOSED                    │
                └──────────────┬────────────────────────────────┘
                               │ drift_failures > 3
                               ▼
                ┌───────────────────────────────────────────────┐
                │                    OPEN                       │
                │  syncGkeepapi() returns early, no NATS call   │
                │  emit keep_protocol_drift_detected (counter)  │
                │  emit structured slog at WARN, once per state │
                │  health = HealthError                          │
                │  prior artifacts remain served                │
                └──────────────┬────────────────────────────────┘
                               │ Connect() observes a NEW drift_ack_token
                               ▼
                            CLOSED
```

**Trip threshold.** `> 3` consecutive failures (matches spec). Drift state is per-connector in-memory; it does not persist across restarts, which is the entire point — restart with a rotated `drift_ack_token` is the recovery action.

**What counts as a drift failure.** Initial design: any `KeepSyncResponse` that fails Go-side schema validation OR carries `status: "error"` from an HTTP non-2xx (sidecar propagates this in the `error` field with a stable prefix). See OQ-059-03.

**Outputs on trip.**

- Prometheus counter `smackerel_keep_protocol_drift_detected_total` (label: `connector_id`).
- Structured log: `slog.Error("keep_protocol_drift_detected", "connector_id", id, "consecutive_failures", n, "last_request_id", reqID)`.
- Health transitions to `HealthError`; the existing `Health()` accessor is the operator's read path.
- Notification routing: the existing notification pipeline (spec 054) already escalates `HealthError` transitions; no new route is wired here.

**No new endpoints.** There is no HTTP ack endpoint, no CLI verb, no NATS control subject. Acknowledgment is exclusively `drift_ack_token` rotation + restart.

### Relationship to `warning_acknowledged`

| Condition | Behavior |
|---|---|
| `warning_acknowledged: false` | Connect fails (existing behavior); `drift_ack_token` is irrelevant. |
| `warning_acknowledged: true`, breaker CLOSED | Normal sync. |
| `warning_acknowledged: true`, breaker OPEN, restart with same `drift_ack_token` | Connect succeeds, but first Sync re-trips immediately (no token rotation). |
| `warning_acknowledged: true`, breaker OPEN, restart with NEW `drift_ack_token` | Breaker resets to CLOSED; Sync resumes. |

---

## (d) Operator Workflow

The operator surface is config-file edits plus `./smackerel.sh` restart. **No new CLI verb.**

### Initial enablement

1. Generate a Google App Password (Google Account → Security → 2-Step Verification → App passwords).
2. Add `KEEP_GOOGLE_APP_PASSWORD` to the deploy-overlay sops-encrypted secret store (operator-side; never in this repo).
3. Set `KEEP_GOOGLE_EMAIL=<address>` in the standard env contract for the target.
4. Edit `config/smackerel.yaml`:

   ```yaml
   connectors:
     google-keep:
       sync_mode: gkeepapi          # or hybrid
       gkeep_enabled: true
       warning_acknowledged: true
       drift_ack_token: "<any-non-empty-string>"
   ```

5. Run `./smackerel.sh config generate --env <env> --bundle --source-sha <sha>`.
6. Run `./smackerel.sh deploy-target <target> apply ...` (or local `./smackerel.sh up`).
7. Verify with `./smackerel.sh status` and `./smackerel.sh logs` (see Observability below).

### Recovering from a tripped breaker

1. `./smackerel.sh logs` to inspect the `keep_protocol_drift_detected` event(s) and identify what changed (e.g., `gkeepapi` version, response shape).
2. If a library upgrade is needed, bump the `gkeepapi` pin in `ml/requirements.txt` and rebuild.
3. Edit `config/smackerel.yaml` and change `drift_ack_token` to ANY new value (timestamp, git SHA, free-form string — any change from the prior value counts).
4. `./smackerel.sh config generate --env <env> --bundle --source-sha <new-sha>`.
5. `./smackerel.sh deploy-target <target> apply ...` (or local `./smackerel.sh down && ./smackerel.sh up`).
6. Verify breaker is CLOSED by checking `./smackerel.sh status` and the absence of new drift events.

### Token rotation (operator regenerates Google App Password)

Same as initial enablement steps 2 and 5–7. No code change. No `drift_ack_token` rotation required unless the breaker was tripped.

### Honored repo policies

- **Terminal discipline.** All commands above are `./smackerel.sh` runtime commands; no ad-hoc `docker compose` or `pytest` invocations.
- **SST zero-defaults.** Every new field (`drift_ack_token`) and every new env var (`KEEP_GOOGLE_EMAIL`, `KEEP_GOOGLE_APP_PASSWORD`) is sourced from `config/smackerel.yaml` / the three-mirror manifest. No language-level fallback defaults.
- **No env-specific values in this repo.** Email addresses, real app-password values, and operator deploy hostnames live exclusively in the deploy-overlay; spec/design/code carry placeholders only.

---

## Data Model & Storage

No new tables. The existing artifact pipeline persists `RawArtifact`s produced by `normalizer.Normalize`. Dedup uses Keep `note_id` as the artifact's source identifier; hybrid-mode collisions between Takeout and `gkeepapi` collapse onto the same artifact key, satisfying the spec's "dedup across origins" hard constraint.

In-memory state (per connector, not persisted):

| Field | Type | Reset on |
|---|---|---|
| `driftFailures` | int | success OR `drift_ack_token` rotation |
| `breakerState` | enum (`closed`, `tripping`, `open`) | success → closed; threshold breach → open; token rotation → closed |
| `lastAckToken` | string | container start |

---

## Security & Compliance

- `KEEP_GOOGLE_APP_PASSWORD` is Bucket-2; lifecycle is owned by specs 051/052. This spec ONLY adds the key to the three mirrors and consumes the injected value.
- Auth path: app password only. No OAuth, no master_token, no HTML scraping. Anti-requirements enforced.
- Logs / metrics MUST NOT carry the password. The sidecar already raises generic errors; the Go validation messages above use field-name references only.
- Read-only: the Go connector never publishes a write-intent subject; the sidecar never invokes any `gkeepapi` mutation method. A code-review rule (covered in scopes during planning) will assert no `gkeep_*write*` symbols appear in either codebase.
- Rate-limit floor enforced at config-parse time (see OQ-059-01).

---

## Configuration & Migrations

### `config/smackerel.yaml`

- Add `infrastructure.secret_keys` entry `KEEP_GOOGLE_APP_PASSWORD`.
- Add `connectors.google-keep.drift_ack_token` with empty-string placeholder.
- `KEEP_GOOGLE_EMAIL` flows through the standard env contract; surface it in the env emission code path the same way other non-secret connector envs are surfaced.

### Generated env files

- `config/generated/dev.env` (and per-env equivalents) gain `KEEP_GOOGLE_EMAIL` plus the placeholder marker `__SECRET_PLACEHOLDER__KEEP_GOOGLE_APP_PASSWORD__` for production-class targets per the spec-052 contract.

### Compose

- `smackerel-ml` service env injection passes both `KEEP_GOOGLE_EMAIL` and `KEEP_GOOGLE_APP_PASSWORD`.
- `smackerel-core` receives `KEEP_GOOGLE_EMAIL` only (needed for the startup validation message and parity with the connector config).
- No bind-address or port changes.

### Migration / data-model

None. Existing Keep artifacts (Takeout-mode) remain valid; first `gkeepapi` sync of an existing note ID resolves to the same artifact key.

---

## Observability & Failure Handling

| Signal | Type | Source | Used for |
|---|---|---|---|
| `smackerel_keep_protocol_drift_detected_total` | Prometheus counter | Go connector | alerting on breaker trip |
| `smackerel_keep_gkeep_sync_duration_seconds` | Prometheus histogram | Go connector (reuse existing connector histograms shape) | latency of NATS round-trip |
| `smackerel_keep_gkeep_notes_returned_total` | Prometheus counter | Go connector | volume tracking |
| `keep_protocol_drift_detected` | structured log (slog ERROR) | Go connector | operator inspection |
| `keep_sync_request` / `keep_sync_response` | structured log (slog INFO at DEBUG verbosity) | both sides | trace correlation via `request_id` |

Health transitions reuse the existing `HealthSyncing` / `HealthHealthy` / `HealthDegraded` / `HealthError` ladder; the breaker forces `HealthError` while OPEN.

Failure modes:

- Missing env vars → connector fails Connect.
- NATS request timeout (120 s) → counted as one drift failure.
- Sidecar `status: "error"` with `error: "gkeepapi authentication failed"` → escalates to Connect-style failure (NOT a drift failure; auth errors are operator-config issues, not protocol drift).
- Sidecar `status: "error"` with any other `error` → drift failure.
- Go-side schema validation failure → drift failure.

---

## Testing & Validation Strategy

| Test type | Location | Asserts |
|---|---|---|
| Go unit | `internal/connector/keep/*_test.go` | `validateGkeepResponse` accepts the canonical fixture and rejects each per-field mutation (missing key, wrong type, wrong `schema_version`). |
| Go unit | `internal/connector/keep/*_test.go` | Circuit-breaker transitions: closed → tripping (1, 2, 3 failures), tripping → open (4th failure), open → closed via `drift_ack_token` rotation. |
| Go unit | `internal/connector/keep/*_test.go` | `parseKeepConfig` handles `drift_ack_token` field presence/absence/empty string. |
| Go contract | `internal/config/secret_keys_test.go` | Existing test catches drift between the three secret-key mirrors after `KEEP_GOOGLE_APP_PASSWORD` is added. |
| Python unit | `ml/tests/test_keep_bridge.py` | `handle_sync_request` returns shape-correct `KeepSyncResponse`; error branches set `notes=[]` and non-empty `error`. |
| Integration | `tests/integration/...` (live stack) | Go connector publishes `keep.sync.request`, sidecar (stubbed gkeepapi) replies, Go normalizes and emits `RawArtifact`. |
| E2E | `tests/e2e/...` (live stack) | With sidecar fixtured to inject a malformed response, the breaker trips, `HealthError` surfaces, and a `drift_ack_token` rotation resets it on restart. |

All live-stack tests run against the disposable test stack per repo policy; persistent dev state is never touched. Fixtures use placeholder credentials only.

---

## Alternatives & Tradeoffs

| Alternative | Why rejected |
|---|---|
| Go reimplementation of `gkeepapi` | Duplicates a fast-moving reverse-engineered protocol; sidecar boundary already exists. |
| `python3` subprocess shellout from Go | Violates sidecar boundary; harder to test; loses NATS-based observability. |
| Reusing `warning_acknowledged` as the drift-recovery toggle | Conflates initial risk acceptance with post-drift investigation; would force operators to re-acknowledge risk on every restart. |
| New CLI verb `./smackerel.sh keep ack-drift` | Adds a runtime mutation surface for what is fundamentally a config + restart event; bypasses SST. |
| Persisting breaker state to disk | Restart is the recovery path; persistence would defeat the operator-must-look-at-this signal. |
| HTTP ack endpoint | Same objection as the CLI verb; also widens the public attack surface. |
| JetStream stream for `keep.sync.*` | Synchronous request/reply is the natural fit; durable streams add lag and no replay value here. |

---

## Deprecation Path (per spec NC-5)

When/if Google ships an official Keep API, a successor spec introduces `SyncModeOfficial` and this design retires in three stages:

1. **Coexistence stage.** Successor spec adds `SyncModeOfficial`. Hybrid mode definition expands to mean "Takeout + (official OR gkeepapi)". Operators set `sync_mode: official`; `gkeepapi` mode remains valid for a deprecation window. The Go connector dispatches by `sync_mode`; no breaking change to existing operators.
2. **Dedup-key continuity.** The Keep note ID MUST remain the dedup key across all modes. The successor spec is REQUIRED to extract the same `note_id` from the official-API response; if Google chooses a different primary identifier, the successor spec MUST publish a deterministic mapping from the new identifier back to the historical Keep `note_id` so existing artifacts collapse correctly. No artifact rewrites or backfills are needed if this mapping holds.
3. **`warning_acknowledged` retirement.** Once `SyncModeGkeepapi` is removed:
   - The `warning_acknowledged` field is removed from `KeepConfig` and `parseKeepConfig`.
   - The `drift_ack_token` field is removed (no protocol drift on a stable official API).
   - The `keep_protocol_drift_detected` metric is retained (zero-valued) for one release for dashboard continuity, then dropped in the release after.
   - `KEEP_GOOGLE_APP_PASSWORD` is removed from the three-mirror manifest in the same change set that removes `SyncModeGkeepapi`; the official-API spec wires its own auth keys (likely OAuth client credentials).

This deprecation path is intentionally documented now so that any future migration is a pure replacement of the `gkeepapi`-mode plumbing, with zero impact on already-ingested artifacts.

---

## Open Questions

- OQ-059-01 — RESOLVED to 15 min (current code enforcement); spec narrative updated.
- OQ-059-02 — Should `gkeep_response_timeout` be a config field or stay a `120 s` constant?
- OQ-059-03 — Drift counter: single shared counter for HTTP non-2xx + schema-validation failures, or two distinct counters? Design proposes single; plan may split.
