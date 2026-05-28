# Feature: 059 — Google Keep Live Sync (gkeepapi production hardening)

> **Author:** bubbles.analyst (draft scaffold)
> **Date:** May 28, 2026
> **Status:** Clarified (NC-1..NC-5 resolved 2026-05-28 by `bubbles.clarify` against repo evidence — ready for `bubbles.design`)
> **Workflow Mode:** `full-delivery` (NC-1 resolved to option A; see Clarifications)
> **Design Doc:** [docs/smackerel.md](../../docs/smackerel.md) — Section 6.2 Capture Input Types (Notes); [Connector_Development.md](../../docs/Connector_Development.md)

---

## Related

- **Augments:** [internal/connector/keep](../../internal/connector/keep) — existing `keep.go` already declares `SyncMode = takeout | gkeepapi | hybrid`. Takeout mode is fully implemented; gkeepapi mode is a placeholder (config keys exist; runtime path is stubbed).
- **Predecessor:** Whatever historical spec stood up the `SyncModeTakeout` path (likely an early-numbered keep spec under `specs/`).

---

## Problem Statement

Google has NEVER released a public API for Keep. The only ways to programmatically access a user's Keep notes are:

1. **Google Takeout** (current `SyncModeTakeout`) — operator periodically requests Takeout, downloads a zip, drops it in `import_dir`. Latency: days. Operator effort: high. Result: stale notes.
2. **`gkeepapi` Python library** — reverse-engineered access to the Keep app's internal HTTP endpoints. Requires a `master_token` extracted from a Google account on a real Android device (via `gpsoauth`). The library is community-maintained, unofficial, and Google may change the protocol at any release.

The connector code already names `gkeepapi` mode but does not implement it. This means operators who want live note ingestion (per the "Knowledge Breathes" product principle) have NO path today — Takeout is too slow to support real workflows, and the live mode is dead config.

Two choices exist for resolution:

- **(A) Build the live path**, accept the fragility, document a token-rotation runbook, and add health checks that detect when Google changes the protocol so the connector fails loudly instead of silently.
- **(B) Remove the dead `gkeepapi` mode** from config, document that Keep is Takeout-only, and treat this as a closed-bug.

This spec defaults to **(A)** but explicitly surfaces (B) as a clarification for the owner.

---

## Outcome Contract (assuming option A)

**Intent:** Implement the `gkeepapi`-backed live mode for the Keep connector so that an operator who has provisioned a `master_token` sees new and edited Keep notes appear in smackerel within the configured `gkeep_poll_interval` (default 60 min) without manual export.

**Success Signal:** Operator follows the documented app-password runbook (Google Account → Security → App passwords — see Clarifications NC-3), sets `connectors.google-keep.sync_mode: gkeepapi`, `gkeep_enabled: true`, and `warning_acknowledged: true`, provides `KEEP_GOOGLE_EMAIL` + `KEEP_GOOGLE_APP_PASSWORD` via SST-managed secret injection, runs `./smackerel.sh up`, and:

1. Within the first poll interval, all existing Keep notes are ingested as `RawArtifact`s of kind `note` with provenance `google-keep`.
2. A new note created on phone or web appears in smackerel within ≤ 1 poll interval + 30 s.
3. Editing an existing note updates the corresponding artifact (versioned, not duplicated; dedup by Keep note ID).
4. Deleted/archived notes are soft-deleted in smackerel with the archival timestamp recorded.
5. If Google rotates the protocol such that `gkeepapi` calls start returning unexpected shapes, the connector emits a structured `keep_protocol_drift_detected` log + Prometheus counter and disables further sync attempts until the operator acknowledges (failing loud, not silently degrading).
6. Token rotation (operator regenerates master_token) is a single sops-encrypted secret update + restart — no code change.

**Hard Constraints:**

- **`KEEP_GOOGLE_APP_PASSWORD` treated as a Bucket-2 managed secret** (per [specs/051-deployment-secret-auth-contract](../051-deployment-secret-auth-contract)): added to the 3-mirror secret manifest (`config/smackerel.yaml` `infrastructure.secret_keys`, `internal/config/secret_keys.go`, `scripts/commands/config.sh`), value stored sops-encrypted in the operator deploy-overlay secret store, never in plaintext anywhere. `KEEP_GOOGLE_EMAIL` is non-secret config and lives in the standard env contract.
- **`KEEP_GOOGLE_APP_PASSWORD` MUST NEVER appear in logs, metrics, traces, error messages, or fixture data**
- **Read-only — never write to Keep.** No note creation, no edit, no archive, no delete. The connector observes; the operator owns the data on Google's side.
- **No fall-back path** — if `sync_mode: gkeepapi` is set and `KEEP_GOOGLE_EMAIL`/`KEEP_GOOGLE_APP_PASSWORD` are missing or rejected by Google, the connector fails loudly at startup. NO silent fall-back to Takeout mode (NO-DEFAULTS policy).
- **Protocol drift detection is a first-class feature** — schema validation on every `gkeepapi` response; any unexpected field type, missing required field, or HTTP non-2xx for > 3 consecutive polls trips the `keep_protocol_drift_detected` circuit breaker, which:
  - Disables further polls until operator acknowledgment endpoint is hit
  - Surfaces an alert through the existing notification-intelligence pipeline (spec 054)
  - Continues to serve already-ingested data; does NOT delete prior artifacts
- **Rate-limit conservative** — minimum `gkeep_poll_interval` floor of 15 min in code (config minimum); Google does not publish quotas for the unofficial endpoints, so we err on the side of caution
- **Test isolation** — `gkeepapi` calls live in the Python ml-sidecar (`ml/app/keep_bridge.py`, already scaffolded). Go core dispatches `keep.sync.request` NATS messages and consumes structured `GkeepNote` payloads on the response subject. No subprocess shellouts, no Go reimplementation. (See Clarifications NC-2.)
- **Dedup by Keep note ID across Takeout and gkeepapi origins** so users running `sync_mode: hybrid` don't double-ingest

---

## Out of Scope (v1)

- Creating, editing, archiving, or deleting Keep notes from smackerel
- Real-time push from Google (Keep has no webhook / push channel)
- Sharing Keep notes via Smackerel
- Multi-account (only one Google account / master_token per smackerel deployment in v1)
- Migrating away from `gkeepapi` to a hypothetical official API (would be a new spec when/if Google ships one)

---

## Clarifications (Resolved 2026-05-28)

Resolved by `bubbles.clarify` against repo evidence (`ml/app/keep_bridge.py`, `internal/connector/keep/keep.go`, `specs/051-deployment-secret-auth-contract`, `specs/052-bundle-secret-injection-contract`). Each NC below is a binding decision for `bubbles.design`.

- **NC-1 — Build vs delete (RESOLVED: A — build live).** Evidence: the connector already exposes `SyncModeGkeepapi` + `SyncModeHybrid` in `internal/connector/keep/keep.go`, the ml-sidecar already contains `ml/app/keep_bridge.py`, and the product principle "Knowledge Breathes" cannot be satisfied with Takeout-only latency (days). Removing the live mode would orphan committed scaffolding and leave the principle unmet. The fragility risk is mitigated by the drift-detection circuit breaker (Hard Constraints) and the `warning_acknowledged` config gate already in `keep.go`. Option B is explicitly rejected.

- **NC-2 — Bridge architecture (RESOLVED: Python ml-sidecar over NATS).** Evidence: `ml/app/keep_bridge.py` already implements `authenticate()` and `serialize_note()`; the sidecar already participates in NATS request/response flows for other domains (Drive, Photos, YouTube). Go core publishes `keep.sync.request` and consumes `GkeepNote` payloads on the response subject (Go type `GkeepNote` already declared in `keep.go`). Subprocess shellouts and Go reimplementation of `gkeepapi` are rejected: shellouts violate the sidecar boundary; reimplementation duplicates a fast-moving reverse-engineered protocol surface.

- **NC-3 — Credential extraction runbook (RESOLVED: Google App Password, operator-side, one-time, no helper container).** Evidence: `ml/app/keep_bridge.py` already reads `KEEP_GOOGLE_EMAIL` + `KEEP_GOOGLE_APP_PASSWORD` and calls `gkeepapi.Keep().login(email, password)` — the master_token + `gpsoauth` path is NOT what the codebase uses. The operator generates a Google App Password (Google Account → Security → 2-Step Verification → App passwords), stores it via the deploy-adapter secret bundle, and the value is injected at container start. A helper container is rejected as unnecessary attack surface for a one-time procedure. Runbook lives in `docs/Operations.md` under the Keep connector section (design.md to specify exact wording).

- **NC-4 — Drift acknowledgment mechanism (RESOLVED: config-flag bump + restart, no new CLI verb).** Evidence: `keep.go` already gates startup on a `warning_acknowledged: true` config field; reusing that gate keeps the operator surface uniform with the existing risk-acknowledgment pattern. When the breaker trips it (a) emits `keep_protocol_drift_detected` log + Prometheus counter, (b) sets a sidecar/connector status to `drift_paused`, (c) requires the operator to inspect logs, decide whether to upgrade `gkeepapi`, and bump a dedicated `drift_ack_token` config field (free-form string, e.g. a timestamp or git SHA the operator picks) to a NEW value before restart. Reusing `warning_acknowledged` alone is insufficient because it is already `true` at steady state; the `drift_ack_token` rotation is the explicit "I have looked" signal. A new CLI verb (`./smackerel.sh keep ack-drift`) is rejected: it adds a runtime mutation surface for a procedure that is fundamentally a config + restart event.

- **NC-5 — Future-official-API coexistence (RESOLVED: design.md MUST include a Deprecation Path section).** When/if Google ships an official Keep API, a successor spec replaces `SyncModeGkeepapi` with `SyncModeOfficial`; this spec's deprecation path defines (a) how `master`/`hybrid` mode interacts with a new official path, (b) the artifact-ID continuity requirement (Keep note ID remains the dedup key across modes), (c) the warning-acknowledgment field can be removed once on the official API. Owning agent: `bubbles.design` adds the section in `design.md`.

---

## Anti-Requirements

- This feature MUST NOT use any non-master_token authentication path (e.g. OAuth attempts against the unofficial endpoints, which Google blocks).
- This feature MUST NOT silently retry indefinitely on protocol drift — must trip the circuit breaker after the configured failure window.
- This feature MUST NOT scrape `keep.google.com` HTML as a fallback if `gkeepapi` fails — that is a separate, even-more-fragile path that the spec explicitly forbids.
- This feature MUST NOT cache `KEEP_GOOGLE_APP_PASSWORD` in any unencrypted on-disk location (in-memory only during sidecar lifetime; sourced from the SST-managed secret bundle on each container restart). The cached `gkeepapi` session object in `ml/app/keep_bridge.py` is in-process state only and MUST NOT be serialized.
- This feature MUST NOT enable itself by default — operator must explicitly set `gkeep_enabled: true`, `warning_acknowledged: true`, AND provide real `KEEP_GOOGLE_EMAIL` + `KEEP_GOOGLE_APP_PASSWORD` values, otherwise the connector stays in takeout-only mode.
