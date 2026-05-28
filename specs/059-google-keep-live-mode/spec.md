# Feature: 059 — Google Keep Live Sync (gkeepapi production hardening)

> **Author:** bubbles.analyst (draft scaffold)
> **Date:** May 28, 2026
> **Status:** Draft (intent only — `bubbles.specify` / `bubbles.clarify` should harden before `bubbles.plan`)
> **Workflow Mode:** TBD (likely `full-delivery` if accepted; alternatively `docs-only` if owner decides fragility is too high)
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

**Success Signal:** Operator follows the documented token-extraction runbook (Android device + `gpsoauth` one-time script — see Clarifications), sets `connectors.google-keep.sync_mode: gkeepapi` and `gkeep_enabled: true`, provides the master_token via SST-managed secret injection, runs `./smackerel.sh up`, and:

1. Within the first poll interval, all existing Keep notes are ingested as `RawArtifact`s of kind `note` with provenance `google-keep`.
2. A new note created on phone or web appears in smackerel within ≤ 1 poll interval + 30 s.
3. Editing an existing note updates the corresponding artifact (versioned, not duplicated; dedup by Keep note ID).
4. Deleted/archived notes are soft-deleted in smackerel with the archival timestamp recorded.
5. If Google rotates the protocol such that `gkeepapi` calls start returning unexpected shapes, the connector emits a structured `keep_protocol_drift_detected` log + Prometheus counter and disables further sync attempts until the operator acknowledges (failing loud, not silently degrading).
6. Token rotation (operator regenerates master_token) is a single sops-encrypted secret update + restart — no code change.

**Hard Constraints:**

- **master_token treated as a Bucket-2 managed secret** (per [specs/051-deployment-secret-auth-contract](../051-deployment-secret-auth-contract)): added to the 3-mirror secret manifest (`config/smackerel.yaml` `infrastructure.secret_keys`, `internal/config/secret_keys.go`, `scripts/commands/config.sh`), value stored sops-encrypted in [knb/smackerel/secrets/home-lab.enc.env](https://github.com/pkirsanov/knb) under the operator-issued key name (e.g. `GOOGLE_KEEP_MASTER_TOKEN`), never in plaintext anywhere
- **master_token MUST NEVER appear in logs, metrics, traces, error messages, or fixture data**
- **Read-only — never write to Keep.** No note creation, no edit, no archive, no delete. The connector observes; the operator owns the data on Google's side.
- **No fall-back path** — if `sync_mode: gkeepapi` is set and master_token is missing/invalid, the connector fails loudly at startup. NO silent fall-back to Takeout mode (NO-DEFAULTS policy).
- **Protocol drift detection is a first-class feature** — schema validation on every `gkeepapi` response; any unexpected field type, missing required field, or HTTP non-2xx for > 3 consecutive polls trips the `keep_protocol_drift_detected` circuit breaker, which:
  - Disables further polls until operator acknowledgment endpoint is hit
  - Surfaces an alert through the existing notification-intelligence pipeline (spec 054)
  - Continues to serve already-ingested data; does NOT delete prior artifacts
- **Rate-limit conservative** — minimum `gkeep_poll_interval` floor of 10 min in code (config minimum); Google does not publish quotas for the unofficial endpoints, so we err on the side of caution
- **Test isolation** — `gkeepapi` is a Python library; if we import it into smackerel-core (Go), we need a bridge. Options: (a) extend the existing ml-sidecar (Python) with a Keep-fetch endpoint that core polls over NATS; (b) shell out to a Python subprocess; (c) reimplement `gkeepapi` in Go. Recommend (a) — leverages existing Python sidecar, no shellouts, no rewrite. Clarify before design.
- **Dedup by Keep note ID across Takeout and gkeepapi origins** so users running `sync_mode: hybrid` don't double-ingest

---

## Out of Scope (v1)

- Creating, editing, archiving, or deleting Keep notes from smackerel
- Real-time push from Google (Keep has no webhook / push channel)
- Sharing Keep notes via Smackerel
- Multi-account (only one Google account / master_token per smackerel deployment in v1)
- Migrating away from `gkeepapi` to a hypothetical official API (would be a new spec when/if Google ships one)

---

## Open Questions for `bubbles.clarify`

- **NC-1:** Decision between option A (build live) and option B (delete dead config + close as wontfix). The default proposal is A; owner may prefer B given the fragility risk.
- **NC-2:** Python sidecar vs Go subprocess vs Go reimplementation of `gkeepapi`. Recommend Python sidecar (lowest risk, leverages existing process).
- **NC-3:** master_token extraction runbook — does the operator do this once on their personal Android device with `gpsoauth`, or do we ship a helper container that runs the extraction interactively? Recommend operator-side one-time procedure, documented in `docs/Operations.md`.
- **NC-4:** Drift acknowledgment endpoint shape — admin HTTP endpoint? CLI command (`./smackerel.sh keep ack-drift`)? Recommend CLI for operator simplicity.
- **NC-5:** Does this spec need to coexist with a future spec that integrates official Google APIs if Keep ever opens up? Recommend leaving an explicit "deprecation path" section in design.md.

---

## Anti-Requirements

- This feature MUST NOT use any non-master_token authentication path (e.g. OAuth attempts against the unofficial endpoints, which Google blocks).
- This feature MUST NOT silently retry indefinitely on protocol drift — must trip the circuit breaker after the configured failure window.
- This feature MUST NOT scrape `keep.google.com` HTML as a fallback if `gkeepapi` fails — that is a separate, even-more-fragile path that the spec explicitly forbids.
- This feature MUST NOT cache the master_token in any unencrypted location (in-memory only during connector lifetime; sourced from sops on each container restart).
- This feature MUST NOT enable itself by default — operator must explicitly set `gkeep_enabled: true` AND provide a real master_token, otherwise the connector stays in takeout-only mode.
