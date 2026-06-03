# Smackerel — Architecture

This document holds short, focused architecture notes that complement
[`docs/smackerel.md`](smackerel.md) (the full design document) and
[`docs/Deployment.md`](Deployment.md) (operator workflows). It is the
landing page for trust-boundary diagrams, security perimeters, and
cross-cutting architectural contracts that span both the runtime and
the deployment surface.

For the full system architecture (data flow, modules, storage, NATS
topology, ML sidecar boundaries), see [`docs/smackerel.md`](smackerel.md)
sections 3 (System Architecture), 8 (Storage), 17 (Trust), 18 (Privacy),
and 23 (Implementation reality).

## Current System Shape

Smackerel runs as a Go core service plus a Python ML sidecar on Docker
Compose. PostgreSQL stores canonical runtime state, NATS JetStream carries
asynchronous work, and Ollama or another configured LLM provider powers ML
tasks through the sidecar. The Go core owns HTTP routing, connector polling,
notification intelligence, storage writes, web UI routes, and operator APIs.

Notification intelligence is split into two layers:

- `internal/notification` is the source-neutral spec 054 core. It owns source
  contracts, source health, raw event persistence, normalized notifications,
  classification, correlation, incidents, decisions, suppressions, approvals,
  redaction, and output attempts.
- `internal/notification/source/ntfy` is the concrete spec 055 ntfy source
  adapter. It owns ntfy config validation, stream/webhook transport, payload
  parsing, topic state, reconnect/lag status, dead-letter records, replay, and
  adapter boundary tests.

## Major Components

| Component | Runtime surface | Responsibility |
|-----------|-----------------|----------------|
| Go core | `cmd/core`, `internal/api`, `internal/web` | HTTP API, HTMX web UI, startup wiring, auth, notification routes, connector orchestration. |
| Notification core | `internal/notification` | Source-neutral raw-before-normalized notification pipeline and output decisioning. |
| ntfy source adapter | `internal/notification/source/ntfy` | Concrete ntfy source intake and adapter-owned operations; no output dispatch. |
| Operational storage | `internal/db/migrations/036_notification_intelligence.sql`, `038_notification_ntfy_source_adapter.sql` | Notification core tables plus ntfy topic/dead-letter/replay tables. |
| Config pipeline | `config/smackerel.yaml`, `scripts/commands/config.sh`, `internal/config/config.go` | SST config generation and fail-loud runtime loading, including `NTFY_SOURCES_JSON`. |

## Data And Control Flows

For ntfy intake, startup reads `NTFY_SOURCES_JSON`, registers enabled source
instances, starts adapters, and wires the webhook receiver into
`NotificationHandlers`. Webhook mode accepts authenticated requests at
`/api/notifications/sources/{source_instance_id}/ntfy/webhook`; stream mode
opens topic streams through the adapter transport. Message-like ntfy events are
parsed, redacted where they become operator-visible, mapped to
`SourceEventEnvelope`, and submitted through `SourceEventSink`.

The source sink stores raw JSON before creating a normalized notification. The
spec 054 core then classifies, correlates, decides, suppresses, approves, or
queues output according to source-neutral policy. ntfy lifecycle events update
topic/source health only. Malformed, unsupported, unconfigured-topic, sink
unavailable, and sink-rejected records go to adapter-owned dead letters.
The first accepted replay reconstructs an eligible source envelope and submits
it through the same source sink. Later replay requests for an already-replayed
dead letter return the existing accepted attempt and do not repeat the sink side
effect.

## Integration Boundaries

| Boundary | Rule |
|----------|------|
| ntfy adapter to notification core | The adapter imports source-neutral notification interfaces and submits `SourceEventEnvelope`; the core must not import the ntfy adapter or branch on ntfy-only fields. |
| ntfy adapter to output delivery | The adapter never dispatches output. Output attempts are created only by the notification core decision/output layer. |
| ntfy config to secrets | Config stores secret reference names only. Credential values stay in the secret-management path and must not appear in status, logs, payload previews, dead letters, or API responses. |
| ntfy webhook to source identity | The route requires a registered ntfy source instance and rejects non-ntfy or non-webhook source forms. |
| replay to core pipeline | The first accepted replay requires `replay_through_source_sink` confirmation and calls `SourceEventSink`; repeated replay is idempotent and returns the existing accepted attempt without another sink submission. Replay does not bypass raw persistence or output policy. |

## Authoritative References

- [`specs/054-notification-intelligence-handler/`](../specs/054-notification-intelligence-handler/) — source-neutral notification core contract.
- [`specs/055-notification-source-ntfy-adapter/`](../specs/055-notification-source-ntfy-adapter/) — concrete ntfy adapter execution packet.
- [`docs/API.md`](API.md#ntfy-source-adapter) — authenticated ntfy source endpoints.
- [`docs/Operations.md`](Operations.md#notification-intelligence-operations-spec-054) — operator runbook for source health, reconnect, dead letters, and replay.
- [`docs/Development.md`](Development.md#ntfy-source-adapter-sst-spec-055) — SST shape and implementation references.

---

## Cross-Surface Surfacing Controller (spec 078)

`internal/intelligence/surfacing/` is the single decision point that
intelligence producers consult before dispatching a nudge to any user-facing
channel. It enforces a unified pipeline across all 7 producers (alerts,
digest, resurfacing, weekly synthesis, monthly report, pre-meeting briefs,
frequent lookups) and all 5 channels (telegram, web push, ntfy, email out,
digest).

| Aspect | Detail |
|--------|--------|
| Package | `internal/intelligence/surfacing/` (controller.go, budget.go, dedupe.go, suppression.go, types.go) |
| Entrypoint | `Controller.Propose(ctx, SurfacingCandidate) (SurfacingDecision, error)` — synchronous call site for every producer |
| Pipeline order | `dedupe → suppress → budget → escalate` (mandated; see `controller.go::Propose`) |
| Decision vocabulary | `permit`, `deduped`, `suppressed`, `deferred-budget-exhausted`, `escalated` |
| SST config block | `config/smackerel.yaml::surfacing:` → `daily_nudge_budget`, `suppression_window_hours`, `dedupe_window_hours`, `urgent_escalation_enabled`. Loader `internal/config/surfacing.go`; env emit `SURFACING_*` via `scripts/commands/config.sh`. Fail-loud — missing env aborts startup (NO-DEFAULTS SST policy). |
| Metrics sink | `internal/metrics/surfacing.go` exposes 8 `smackerel_surfacing_*` families (see [Operations.md → Surfacing Metrics](Operations.md#surfacing-metrics-spec-078)) |
| Construction | Exactly one `Controller` per process, shared across all producers so budget/dedupe/suppression state is unified. |

Adding a new producer or channel is a deliberate code change — both enums in
`types.go` are bounded so cardinality on the metric labels stays finite.

**Authoritative references:**

- [`specs/078-cross-surface-surfacing-prioritizer/`](../specs/078-cross-surface-surfacing-prioritizer/) — controller adoption spec, design, scopes.
- [`docs/Operations.md`](Operations.md#surfacing-metrics-spec-078) — operator-facing metric families and alerting guidance.

---

## Secret Boundary (spec 052)

Smackerel's secret pipeline crosses three trust boundaries between three
distinct hosts running in three distinct security postures. The contract is
defined in
[`specs/052-bundle-secret-injection-contract/`](../specs/052-bundle-secret-injection-contract/);
this section is the operator-facing summary.

```
┌──────────────────────────────────────────────────────────────────────┐
│ L1: SST LOADER (build time, in CI or operator workstation)            │
│ scripts/commands/config.sh + internal/config/secret_keys.go (mirror)  │
│                                                                       │
│   for KEY in infrastructure.secret_keys:                              │
│     if TARGET_ENV in infrastructure.production_class_targets:         │
│       app.env: KEY=__SECRET_PLACEHOLDER__<KEY>__                      │
│       (skip FR-051-005 dev-default check for this key)                │
│     else:                                                             │
│       app.env: KEY=<literal yaml value>                               │
│       (FR-051-005 dev-default check still fires for actual literals)  │
│                                                                       │
│   bundle ships sibling: secret-keys.yaml (enumerates declared keys)   │
└─────────────────────────────────┬────────────────────────────────────┘
                                  │ tar.gz, deterministic
                                  │ cosign-signed, sha256-pinned
                                  ▼
┌──────────────────────────────────────────────────────────────────────┐
│ L2: KNB ADAPTER (apply time, on target host with sops + age key)      │
│ <knb-repo>/smackerel/home-lab/apply.sh                                │
│                                                                       │
│   1. Verify bundle cosign signature (existing)                        │
│   2. Verify bundle sha256 against build manifest (existing)           │
│   3. Extract bundle → COMPOSE_DIR (existing)                          │
│   4. NEW: parse secret-keys.yaml from extracted bundle                │
│   5. NEW: assert every declared key has placeholder in app.env        │
│   6. sops -d secrets file → tmpfile chmod 0600 (existing)             │
│   7. NEW: assert every declared key has real value in tmpfile         │
│           (non-empty AND not equal to its placeholder marker)         │
│   8. docker compose --env-file app.env --env-file tmpfile up          │
│      (existing — Compose's "later wins" override does the substitution)│
│   9. NEW: docker compose --env-file ... config | grep __SECRET_       │
│           → MUST find zero placeholder markers in resolved env        │
│  10. Audit log: secrets_substituted=N placeholders_remaining=0 (NEW)  │
└─────────────────────────────────┬────────────────────────────────────┘
                                  │ docker compose up -d
                                  ▼
┌──────────────────────────────────────────────────────────────────────┐
│ L3: GO RUNTIME (startup time, inside smackerel-core container)        │
│ internal/config/config.go::Validate()                                 │
│ internal/auth/startup.go::ValidateRuntimeAuthStartup()                │
│                                                                       │
│   for KEY in internal/config/secret_keys.go::SecretKeys():            │
│     if env[KEY] == __SECRET_PLACEHOLDER__<KEY>__:                     │
│       return fmt.Errorf("KEY still equals placeholder marker          │
│                          (spec 052 FR-052-007)")                      │
│       (FR-051-007 redaction: name KEY, never echo placeholder/value)  │
└──────────────────────────────────────────────────────────────────────┘
```

### Trust boundaries

| Layer | Host | Privilege | Secret access |
|-------|------|-----------|---------------|
| L1 SST loader | CI runner OR operator workstation | Build-time only; runs `./smackerel.sh config generate --env <env> --bundle` | **None** for production-class targets — emits placeholder marker only |
| L2 knb adapter | Target host (e.g. home-lab box) | Operator-trusted; runs `<knb-repo>/smackerel/<target>/apply.sh` | sops + age private key (`/etc/sops/age/keys.txt` or operator-mounted) |
| L3 Go runtime | Inside `smackerel-core` container on target host | Container-scoped; runs Smackerel process | Process env vars only — no key material on disk inside the container |

### Defense-in-depth invariants

Each layer assumes the layer below it may be compromised. Each layer fails
loud independently. **Compromising any one layer does not leak production
secrets nor allow a placeholder-as-credential boot:**

- L1 compromise (e.g. malicious CI step) → still cannot exfiltrate secrets
  because L1 has no secret access; the bundle ships placeholders.
- L2 compromise (e.g. operator machine compromise) → leaks the four
  bootstrap secrets in `secrets/<target>.enc.env`, which can be rotated
  without code changes; L3 still rejects any process started without
  substitution.
- L3 compromise (e.g. container escape) → process env vars are reachable,
  but the bundle and the operator secret store are not on the container
  filesystem.

The contract is enforced by the canonical secret-key manifest
(`config/smackerel.yaml::infrastructure.secret_keys`, mirrored in
`internal/config/secret_keys.go::secretKeys` and
`scripts/commands/config.sh`; drift caught by
`internal/deploy/bundle_secret_contract_test.go`).

For the full operator workflow (adding a new managed secret, rotating a
secret, auditor inspection), see:

- [`docs/Deployment.md` → Bundle Secret Injection (spec 052)](Deployment.md#bundle-secret-injection-spec-052)
- [`docs/Operations.md` → Bundle Secret Substitution (spec 052)](Operations.md#bundle-secret-substitution-spec-052)
- [`README.md` → Managed Secrets & Bundle Substitution (spec 052) — 3-Layer Defense](../README.md#managed-secrets--bundle-substitution-spec-052--3-layer-defense)

---

## Intent-Driven Assistant (Specs 061, 064–069)

The conversational assistant is a transport-agnostic capability that turns
free-text user messages into either an answered turn (retrieval, weather,
notifications, open-knowledge) or a capture-as-fallback. It is layered on
the spec 037 LLM Scenario Agent substrate and exposes a single
`internal/assistant/facade.go` boundary to every transport.

### Module Boundaries

| Boundary | Owning spec | Rule |
|----------|-------------|------|
| Facade ↔ transports | [061](../specs/061-conversational-assistant/) | Facade defines `contracts.TransportAdapter`; transport adapters translate I/O and call the facade. Facade, scenarios, and executor code MUST NOT branch on `AssistantMessage.Transport` — only adapter and audit layers may inspect that field. |
| NL ↔ router | [068](../specs/068-structured-intent-compiler/) | Every user turn is compiled into a normalized, schema-bound `CompiledIntent` BEFORE scenario routing, tool selection, external calls, or response synthesis. The compiler runs inside [`internal/assistant/facade.go`](../internal/assistant/facade.go) at Step 3.5; Step 3.55 short-circuits `clarify`/`capture_only` (SCN-068-A05) and Step 3.6 gates `write`/`external_write` side-effect classes (SCN-068-A03/A04/A09) BEFORE `Router.Route` runs. The compiler ships as `intent.LLMCompiler` with an injectable `intent.Transport` (the ML-sidecar HTTP call), and the operational-command bypass set is the closed list `/help, /status, /reset, /digest, /recent, /done` owned by [`internal/assistant/intent/bypass.go`](../internal/assistant/intent/bypass.go). |
| Terminal scenario | [064](../specs/064-open-ended-knowledge-agent/) | An open-ended knowledge agent is the terminal scenario absorbing any NL turn that no deterministic scenario claims, BEFORE capture-as-fallback fires. It runs the spec 037 LLM ↔ tool loop with internal retrieval + bounded web search + calculator + unit-convert. |
| Cross-scenario primitives | [065](../specs/065-generic-micro-tools/) | `location_normalize`, `unit_convert`, `entity_resolve`, `calculator` are scenario-agnostic micro-tools in the spec 037 registry. Scenarios consume these instead of forking per-API normalization logic into their system prompts or scenario-local Go. |
| Legacy keyword surfaces | [066](../specs/066-legacy-keyword-surface-retirement/) | Keyword-driven competitors to the NL pipeline are retired: the Telegram slash-command surface is reduced to a small operational set, `internal/api/domain_intent.go` regex parser is replaced by `entity_resolve`, and the annotation keyword map is dropped. A configurable alias window keeps retired commands as NL aliases during the cutover. |
| CI policy enforcement | [067](../specs/067-intent-driven-policy-enforcement/) | Mechanical guards enforce: per-scenario prompt-length cap, mandatory `principleAlignment` block per scenario YAML, broadened NO-DEFAULTS check, forbidden-keyword guard against retired surfaces, and compiler-bypass detection (no user-facing NL path may call `Router.Route` without a validated `CompiledIntent` trace record). |
| HTTP transport | [069](../specs/069-assistant-http-transport/) | A second concrete `TransportAdapter` registered under `Transport="web"` exposes `POST /api/assistant/turn` under the per-user bearer policy. This is the canonical programmatic conversational surface used by E2E tests and by every future frontend (web chat, Android in-app, WhatsApp Business webhook, devtools). Telegram is one of many transports, not the privileged path. |
| WhatsApp Business transport | [072](../specs/072-whatsapp-business-transport/) | A third concrete `TransportAdapter` registered under `Transport="whatsapp"` mounts the Meta WhatsApp Business Cloud API webhook at the configured `assistant.transports.whatsapp.webhook_path` (default `/v1/assistant/transports/whatsapp/webhook`). GET handles Meta hub-mode verification; POST verifies the `X-Hub-Signature-256` HMAC against `WHATSAPP_APP_SECRET` BEFORE invoking the facade, hashes inbound E.164 phones with `WHATSAPP_IDENTITY_HASH_KEY` into the `assistant_transport_identities` table, deduplicates by `TransportMessageID`, and renders `AssistantResponse` shapes through the documented WhatsApp message-type table (text, interactive buttons for disambig/confirm, list for `>3` choices, text fallback for unknown shapes). The transport disables independently of Telegram and HTTP via `ASSISTANT_TRANSPORTS_WHATSAPP_ENABLED=false`. |

### Authoritative References

- [`specs/061-conversational-assistant/`](../specs/061-conversational-assistant/) — facade, `TransportAdapter` contract, router/post-processor, confirm/disambig lifecycle, observability substrate.
- [`specs/064-open-ended-knowledge-agent/`](../specs/064-open-ended-knowledge-agent/) — terminal open-knowledge scenario, v1 tool set, provenance gate amendments.
- [`specs/065-generic-micro-tools/`](../specs/065-generic-micro-tools/) — cross-scenario micro-tool registry.
- [`specs/066-legacy-keyword-surface-retirement/`](../specs/066-legacy-keyword-surface-retirement/) — slash-command + `domain_intent.go` + annotation keyword-map retirement and alias-window plan.
- [`specs/067-intent-driven-policy-enforcement/`](../specs/067-intent-driven-policy-enforcement/) — CI guards keeping the intent-driven architecture from silently regressing.
- [`specs/068-structured-intent-compiler/`](../specs/068-structured-intent-compiler/) — NL → `CompiledIntent` → route runtime contract.
- [`specs/069-assistant-http-transport/`](../specs/069-assistant-http-transport/) — `POST /api/assistant/turn` HTTP transport adapter for E2E and frontends.
- [`specs/072-whatsapp-business-transport/`](../specs/072-whatsapp-business-transport/) — WhatsApp Business Cloud API webhook transport adapter, signature verification, transport-identity hashing, and render-table for interactive replies.
- [`docs/Operations.md` → Assistant Capability (Spec 061)](Operations.md#assistant-capability-spec-061) — operator runbook, metrics, recovery actions, HTTP-route notes.
- [`docs/Development.md`](Development.md) — scenario authoring, forbidden patterns, agent + tool discipline.
