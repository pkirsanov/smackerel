# Design: 096 Multi-Provider AI Model Connections

> Design authored by `bubbles.design` from the analyst-phase
> [spec.md](spec.md) (FR/NFR/SCN, Outcome Contract, Domain Capability
> Model) and the committed code it cites. **Depth:** contract-grade —
> the spec carries `## Actors & Personas` + `## Domain Capability
> Model`, so this design elaborates API/data/security contracts to the
> schema level. UI **pixels/flows** are deferred to a subsequent
> `bubbles.ux` pass (OQ-5/OQ-6); the web surface **data contract**
> (endpoints, request/response shapes, the never-return-plaintext rule)
> is design-owned and specified here.
>
> **Extends, does NOT amend** specs 088
> ([088-runtime-switchable-models](../088-runtime-switchable-models/spec.md))
> and 089
> ([089-runtime-model-hotswap-persistent-selection](../089-runtime-model-hotswap-persistent-selection/spec.md)).
> Every 088/089 parity, precedence, fork, attribution, and
> no-override behaviour is preserved verbatim and made
> provider-agnostic. See [§14](#14-builds-on-088089--invariants-preserved).
>
> **Owner-confirmed (2026-06-18):** operator-global connections (NOT
> per-user BYOK); OQ-1 → reversible encrypted-at-rest secret store under
> an operator master key; the Go core decrypts and passes the credential
> to the sidecar per-request (the master key never leaves the Go core).

---

## Design Brief

A code owner can review this ~50-line brief and catch a wrong pattern or
direction BEFORE the full design + plan are generated.

**Current State.** Smackerel talks to exactly one backend LLM at a time.
The open-knowledge `/ask` agent path is **hardcoded to Ollama**:
[`ml/app/routes/chat.py`](../../ml/app/routes/chat.py) `_dispatch_live`
builds `model=f"ollama_chat/{req.model}"`, `api_base=OLLAMA_URL`, **no
`api_key`, no provider branch**. The single global `llm:` block
([`config/smackerel.yaml`](../../config/smackerel.yaml) L80-88) exposes
one `provider`/`model`/`api_key`/`ollama_url`. Runtime model switching
exists but is Ollama-only: specs 088/089 ship a pure-leaf validator
([`modelswitch/allowlist.go`](../../internal/assistant/openknowledge/modelswitch/allowlist.go)),
a claim-bound per-user store
([`modelpref/store.go`](../../internal/assistant/openknowledge/modelpref/store.go),
migration 059), the Telegram `/model` picker
([`internal/telegram/model_command.go`](../../internal/telegram/model_command.go)),
and the HTTP `/v1/agent/model` surface
([`internal/api/agent_model.go`](../../internal/api/agent_model.go)) —
all sharing ONE validator + ONE store with per-request > sticky > default
precedence. There is no model discovery (`/api/tags` is used only in
healthchecks), no operator UI for connection setup, and **no reversible
encrypted-at-rest secret vault**.

**Target State.** N **operator-global** provider connections (local
Ollama + hosted Anthropic / OpenAI / Microsoft Foundry-Azure / Google /
Amazon Bedrock); the operator wires + tests each from the web UI;
discovery aggregates every effective-enabled provider's models into ONE
provider-qualified catalog; every user picks any model from the existing
Telegram + HTTP pickers; the `/ask` dispatch becomes **provider-aware**.
With **no** hosted connection configured the runtime is **byte-for-byte**
today's Ollama path.

**Patterns to Follow.**
- The `synthesis.py` provider fork (`if provider == "ollama": … else:
  litellm.acompletion(… api_key=api_key …)`,
  [`ml/app/synthesis.py`](../../ml/app/synthesis.py#L183-L196)) — the
  redesigned `_dispatch_live` adopts the SAME shape, closing the
  "inconsistent provider-awareness" gap by making `chat.py` consistent
  with `synthesis.py`.
- The 088/089 singletons reached by BOTH surfaces via the `agenttool`
  boundary — the catalog + canonicalization install the SAME way (one
  instance, one validator, one store).
- The managed-secret path (`infrastructure.secret_keys` →
  `__SECRET_PLACEHOLDER__<KEY>__` → deploy-adapter `<target>.enc.env`,
  three-mirror contract,
  [`internal/config/secret_keys.go`](../../internal/config/secret_keys.go))
  — the **master key** rides this exact path; `gcal_credentials` /
  `telegram.bot_token` are the precedent.
- Reversible-secret class (`CARD_REWARDS_GCAL_CREDENTIALS` parse +
  fail-loud, [`internal/cardrewards/gcal_client.go`](../../internal/cardrewards/gcal_client.go#L69-L91)),
  NOT the verifier class (`AUTH_AT_REST_HASHING_KEY`).

**Patterns to Avoid.**
- Do NOT add a second validator/store/picker per provider or per surface
  (violates the 088/089 one-validator/one-store invariant).
- Do NOT import provider/catalog data INTO the pure `modelswitch` leaf —
  inject the (already provider-qualified) admissible set at construction
  (stdlib-only purity preserved).
- Do NOT hash provider credentials (argon2id) — they must be **replayed**
  to `Authorization: Bearer <key>`; a one-way hash is structurally wrong
  for this data class.
- Do NOT introduce per-user provider keys, or any `user_id` on the
  connection/credential surfaces (operator-global; the graph stays
  single + shared).
- Do NOT let a missing/misconfigured connection, a missing master key, or
  a missing cost rate silently fall back to Ollama or to `$0` — every gap
  is a **named fail-loud** abort or a typed refusal (G028).
- Do NOT design the connections-page pixels (UX-owned, OQ-5/OQ-6).

**Resolved Decisions.** Connections = operator-global (no BYOK). Secret
store = single-key **AES-256-GCM** AEAD, per-record nonce + AAD, in
Postgres (migration 060), `key_version` for rotation. Master key =
`LLM_PROVIDER_SECRET_MASTER_KEY` managed secret, loaded fail-loud, never
leaves the Go core. Go→sidecar = `/llm/chat` `ChatRequest` extended with
`provider` / `api_base` / `api_key` / `provider_params` (additive;
Ollama dispatch byte-for-byte). Discovery = adapter-per-kind, Ollama
live `/api/tags` + hosted SST-curated lists, SST TTL + per-provider
timeout, typed per-provider status, graceful degradation. Budget =
model-aware CostFn from an SST rate table + a monthly USD spend ledger;
Ollama = `$0`, missing rate for a paid model = fail-loud refusal.
Identifier grammar = `<kind>/<backend-model-id>` split on the FIRST `/`,
at-most-one-enabled-connection-per-kind, canonicalized at the resolver
boundary (leaf stays pure).

**Open Questions (residual; see [§18](#18-risks--open-questions)).** The
operator-vs-user gate mechanism for the admin surface (proposed: SST
operator allowlist); the connections-page visual/flow + picker
capability-surfacing wording (UX-owned, OQ-5/OQ-6).

---

## 1. Overview

This feature adds **operator-global multi-provider model connectivity**
in three capability layers, all preserving the 088/089 selection
primitive:

- **Layer A — multi-connection abstraction + provider-aware `/ask`.** A
  connection registry (SST-declared) + an encrypted credential vault
  (runtime, web-UI-entered) + a provider-aware dispatch that routes each
  request to the selected model's provider with that provider's
  credential, decrypted in the Go core and passed to the sidecar
  per-request.
- **Layer B — web operator surface + at-rest secret protection.** An
  operator-only set of admin endpoints to enter/test/enable/disable each
  connection; credentials encrypted at rest (AES-256-GCM), never returned
  in plaintext.
- **Layer C — discovery + unified selection.** A catalog aggregator that
  merges Ollama-installed and each effective-enabled hosted connection's
  models into ONE provider-qualified catalog, which becomes the source
  set for the **existing** 088/089 validator and is presented by the
  **existing** Telegram + HTTP pickers.

**Ownership boundary (constitution C2).** Every authority decision — the
connection registry, the master key, decryption, the catalog, the
validator/store, the CostFn, and budget enforcement — lives in the **Go
core**. The Python ML sidecar receives only the per-request cleartext
credential for the `litellm` call and holds no static provider secret and
never the master key.

**Out of scope (per spec §4):** per-user BYOK; graph partitioning;
amending 088/089; new data connectors; the live self-hosted A/B run;
replacing the model gateway; unifying the already-provider-aware
`synthesis.py` / `drive_classify.py` config into `connections[]` (those
paths are not redesigned here).

---

## 2. Architecture

### 2.1 Component view

```
                        config/smackerel.yaml (SST)
        ┌──────────── llm.connections[] (registry: id, kind, params, secret_ref) ───────────┐
        │             llm.discovery.{cache_ttl_ms, per_provider_timeout_ms}                  │
        │             llm.model_costs[] (provider-qualified USD rates)                        │
        │             infrastructure.secret_keys += LLM_PROVIDER_SECRET_MASTER_KEY            │
        └────────────────────────────────────────────────────────────────────────────────────┘
                                          │ (config generate, fail-loud)
                                          ▼
  ┌──────────────────────────────────── GO CORE (authoritative) ─────────────────────────────────┐
  │                                                                                                │
  │  ConnectionRegistry ──── SecretVault (AES-256-GCM) ◄── master key (env, never leaves core)     │
  │      │  (SST topology)        │  (Postgres: model_provider_connections, migration 060)         │
  │      │                        │                                                                │
  │      ▼                        ▼                                                                │
  │  CatalogAggregator ──► provider-qualified ModelCatalog ──► modelswitch.Allowlist (PURE LEAF,   │
  │      │  per-kind discovery adapters         (injected admissible set)   one validator)         │
  │      │  (Ollama /api/tags; hosted curated)                              modelpref.Store        │
  │      │                                                                  (one store, 059)        │
  │      ▼                                                                        ▲  ▲              │
  │  DispatchResolver ── resolves (connection + decrypted key + backend model) ──┘  │              │
  │      │  + CostFn(provider,model,tokens) + budget pre-flight (spend ledger)        │ (selection) │
  │      ▼                                                                            │             │
  │  okagent.Run ──per-request──► llm.Client.Chat(ChatRequest{provider,api_base,api_key,…})         │
  └──────────┬──────────────────────────────────────────────────────────┬──────────┬──────────────┘
             │ POST /llm/chat (internal, AuthToken)                       │          │
             ▼                                                            │          │
  ┌── PYTHON ML SIDECAR ──┐                                  Telegram /model    HTTP /v1/agent/model
  │  _dispatch_live:      │                                  + operator web surface (admin)
  │   if provider==ollama │                                  GET/PUT/POST /v1/admin/model-connections
  │     ollama_chat/<m>   │  ── litellm ──► Ollama / Anthropic / OpenAI /
  │   else                │                 Azure-Foundry / Google / Bedrock
  │     <kind>/<m>,api_key │  (cleartext key used transiently, never logged/persisted)
  └───────────────────────┘
```

### 2.2 Sequence — operator adds + tests a connection

```
Operator → Web (operator-gated) : PUT /v1/admin/model-connections/{id}/credential {secret,…}
Web → ConnectionRegistry        : assert {id} is an SST-declared db-mode slot (else 404, fail-loud)
Web → SecretVault               : AES-256-GCM(master key, nonce, AAD={id|kind|key_version}) → ciphertext
SecretVault → Postgres          : UPSERT model_provider_connections (ciphertext, nonce, key_version,
                                  secret_redaction=last4, enabled UNCHANGED)
Web → Operator                  : 200 redacted view (secret_present=true, secret_redaction, NEVER plaintext)

Operator → Web                  : POST /v1/admin/model-connections/{id}/test
Web → SecretVault               : decrypt → cleartext (in-core only)
Web → Provider (live probe)     : minimal auth+reachability call via the per-kind adapter
Provider → Web                  : ok | typed failure (auth_failed | unreachable | timeout)
Web → Postgres                  : persist last_tested_at, last_test_outcome, last_test_detail (never the secret)
Web → Operator                  : 200 {outcome, detail, tested_at}   # failure NEVER reports success,
                                  #                                     NEVER substitutes Ollama
```

### 2.3 Sequence — discovery aggregation

```
Surface (picker) → CatalogAggregator : GetCatalog()
CatalogAggregator                     : if cached && age < cache_ttl_ms → return last-good catalog + statuses
  for each effective-enabled connection (parallel, each bounded by per_provider_timeout_ms):
    ollama  → GET <base_url>/api/tags          → installed models (live)
    hosted  → SST-curated connection.models[]  → available models (+ optional live validation at test time)
  on timeout/unreachable/auth_failed for a provider:
    → that provider contributes its last-good entries (if any) OR none
    → record ProviderDiscoveryStatus{connection_id, kind, state, model_count, detail}  # never silently dropped
CatalogAggregator → Surface           : (ModelCatalog provider-qualified, []ProviderDiscoveryStatus)
                                        # reachable subset always served; one slow/down provider never blocks
```

### 2.4 Sequence — user picks a hosted model and asks

```
User → Telegram/HTTP : set model = "anthropic/claude-3-5-sonnet"  (PUT /v1/agent/model or /model <id>)
Surface → boundary   : canonicalize → "anthropic/claude-3-5-sonnet"; modelswitch.Resolve against catalog set
  off-catalog        → typed Rejection (same modelswitch.Rejection shape) — NO store write, NO dispatch
  in-catalog         → modelpref.Set(actor_user_id, "anthropic/claude-3-5-sonnet")   # per-user selection

User → /ask          : open-knowledge agent run
DispatchResolver     : ResolveEffective (per-request > sticky > default) → effective model (provider-qualified)
                     : map "anthropic/claude-3-5-sonnet" → (anthropic connection, "claude-3-5-sonnet")
                     : CostFn(anthropic, "claude-3-5-sonnet", est_tokens) → est USD
                     : budget pre-flight (per-user + global remaining vs spend ledger) → ok | typed refusal
                     : SecretVault.decrypt(anthropic connection) → cleartext key (in-core only)
okagent.Run → sidecar: POST /llm/chat {provider:"anthropic", model:"claude-3-5-sonnet",
                                        api_base:…, api_key:<cleartext>, provider_params:…}
sidecar _dispatch_live: litellm.acompletion(model="anthropic/claude-3-5-sonnet", api_key=…, …)
sidecar → core        : ChatResponse (text/tool_calls); key never logged
core → spend ledger   : append {actor_user_id, connection_id, model, tokens, usd_cost, ts}
core → User           : answer + attribution {provider:"anthropic", model:"claude-3-5-sonnet", source}
```

### 2.5 Sequence — byte-for-byte Ollama no-override path

```
User → /ask (no selection; no hosted connection effective-enabled)
DispatchResolver : catalog == Ollama installed set only → ResolveEffective → Override.IsZero()==true
                 : WithModelOverride returns the receiver UNCHANGED (spec 087/088 baseline)
okagent.Run → sidecar : POST /llm/chat {provider:"ollama", model:"gemma3:4b", api_base:<OLLAMA_URL>,
                                         api_key: null, provider_params: null}
sidecar _dispatch_live: provider=="ollama" → model="ollama_chat/gemma3:4b", api_base, NO api_key
                        → litellm kwargs IDENTICAL to today (parity test asserts byte-for-byte)
```

---

## 3. Capability Foundation

The foundation (per `capability-foundation.md`; proportionality triggers
apply — six provider kinds, an adapter pattern, a surface shared across
Telegram + web) is **three cooperating singletons in the Go core**, each
provider-/surface-neutral:

1. **`ConnectionRegistry`** — the SST-declared closed set of admissible
   operator-global connections (`id`, `kind`, non-secret `params`,
   `secret_ref`). It answers "which connections may this deployment
   use, and what is each one's non-secret shape?" Fail-loud on an unknown
   kind or a missing required per-kind param.

2. **`SecretVault`** — the reversible AES-256-GCM credential store
   (Postgres) keyed 1:1 to db-mode registry ids. It encrypts on write,
   decrypts in-core on dispatch/test, and NEVER returns plaintext to any
   surface. It owns the master-key load (fail-loud) and rotation.

3. **`CatalogAggregator` + per-kind discovery/dispatch adapters** — the
   provider-/surface-neutral aggregation of effective-enabled
   connections into the provider-qualified `ModelCatalog`, which is
   injected as the admissible set into the **existing** `modelswitch`
   validator and resolved by the **existing** `modelpref` store. One
   validator, one store (088/089 invariant).

The foundation defines the per-kind **adapter contract** (the extension
point); concrete providers are adapters layered on top.

**Adapter contract (illustrative — design owns final Go signatures):**

```text
type ProviderAdapter interface {
    Kind() ProviderKind
    // Non-secret param validation at registry build (fail-loud).
    ValidateParams(params map[string]any) error
    // Required secret field names for this kind ([] for ollama).
    SecretFields() []string
    // Live reachability + credential probe for the test-connection action.
    Test(ctx, params, cred) (TestOutcome, error)
    // Models this connection offers (Ollama: live /api/tags; hosted: curated list).
    Discover(ctx, params, cred) ([]ModelDescriptor, error)
    // Per-request dispatch routing fields handed to the sidecar
    // (provider, api_base, provider_params); api_key is supplied by the
    // vault, never stored in the adapter.
    DispatchRouting(params) (apiBase string, providerParams map[string]any)
}
```

---

## 4. Concrete Implementations

| Kind | Non-secret `params` | Secret field(s) (vault) | Discover strategy | litellm routing (`model`) |
|------|---------------------|-------------------------|-------------------|---------------------------|
| `ollama` | `base_url` | **none** (local) | live `GET <base_url>/api/tags` | `ollama_chat/<model>`, `api_base`, no key |
| `anthropic` | — | `api_key` | SST-curated `models[]` | `anthropic/<model>`, `api_key` |
| `openai` | `org?`, `base_url?` | `api_key` | live `GET <base_url>/v1/models` or curated | `<model>` (or `openai/<model>`), `api_key`, `api_base?`, `organization?` |
| `azure-foundry` | `endpoint`, `api_version`, `deployment` | `api_key` | SST-curated `models[]` (≈deployments) | `azure/<deployment>`, `api_base=endpoint`, `api_version`, `api_key` |
| `google` | Vertex: `project`, `location`; or Gemini: (none) | Vertex: `service_account` (JSON); or Gemini: `api_key` | SST-curated `models[]` | Vertex `vertex_ai/<model>` (+project/location); Gemini `gemini/<model>` (+key) |
| `bedrock` | `region` | `aws_access_key_id` + `aws_secret_access_key` (or assumed-role params) | SST-curated `models[]` | `bedrock/<model-id>` (+region, +AWS creds) |

Each adapter is the ONLY place that knows its provider's quirks. Adding a
seventh kind is a new adapter + a registry-kind vocabulary entry, no
change to the foundation or the validator/store.

### Variation Axes

1. **Provider kind / protocol** — six kinds above; Ollama (local, no
   key) vs hosted (key) vs cloud-IAM (Bedrock AWS creds, Vertex service
   account) vs deployment-routed (Azure).
2. **Connection-parameter shape** — per-kind non-secret param sets +
   per-kind secret-field sets (table above), carried generically by
   `params` (plaintext) + the vault bundle (encrypted) so the contract
   never grows a field per kind.
3. **Secret provisioning mode** — `secret_ref.mode`: `db` (web-UI-entered
   → encrypted vault) vs `env` (managed-secret env var via the
   `infrastructure.secret_keys` path) vs none (Ollama).
4. **Discovery strategy** — live list API (Ollama `/api/tags`, OpenAI
   `/v1/models`) vs SST-curated `models[]` (hosted default).
5. **Dispatch turn** — gather (tool-calling) vs synthesis (forced-final)
   keep the 088/089 fork, now provider-agnostic.
6. **Selection surface** — Telegram numbered picker + HTTP/web
   `/v1/agent/model` resolve through the SAME validator + store
   (parity). The operator **connections** surface is a SEPARATE concern
   (credential management) and adds NO second validator/store.

---

## 5. Data Model

### 5.1 Two-plane model (resolves the SST-vs-runtime tension)

| Plane | Source of truth | Holds | Mutated by |
|-------|-----------------|-------|------------|
| **Registry (SST)** | `config/smackerel.yaml` `llm.connections[]` | which connection slots may exist; `id`, `kind`, non-secret `params`, `secret_ref.mode`, curated `models[]`, capability flags | operator editing SST + `config generate` (declarative/GitOps; topology change) |
| **Runtime (DB)** | Postgres `model_provider_connections` (migration 060) | per-db-mode-slot: encrypted credential, `enabled` toggle, `last_tested_*` | operator via the web admin surface (credential + test + enable/disable) |

The web admin surface operates on the **runtime plane** for slots the
registry declares with `secret_ref.mode: db`; it MUST refuse an `id` not
in the registry (closed-set, fail-loud — the exact philosophy 088/089
applied to `switchable_models`). "Add a brand-new provider kind/slot" =
an SST topology edit; "wire/test/enable a declared slot" = the web UI.
This honors Config-SST (topology from SST), fail-loud (unknown slot
rejected), and the owner's `llm.connections[]` block shape.

**Effective-enabled** (the predicate discovery + dispatch use):
- db-mode: registry-declared **AND** DB `enabled = true` **AND**
  `last_test_outcome = 'ok'` **AND** a credential present.
- env-mode: registry-declared **AND** the managed-secret env var present.
- ollama: registry-declared **AND** reachable at discovery time.

A connection that is not effective-enabled is simply **absent from the
catalog** (graceful degradation) and is NEVER a silent Ollama fallback.

### 5.2 Encrypted credential store — migration 060 (shape, not SQL)

`model_provider_connections` — operator-global (**no `actor_user_id`**,
consistent with the single shared graph + operator-global connectors):

| Column | Type | Notes |
|--------|------|-------|
| `connection_id` | `TEXT PRIMARY KEY` | 1:1 to the SST registry `id` (db-mode slots) |
| `provider_kind` | `TEXT NOT NULL` | closed-set; app-validated fail-loud (mirrors registry) |
| `enabled` | `BOOLEAN NOT NULL` | runtime toggle; app-written (no DB-side default — G028) |
| `secret_ciphertext` | `BYTEA` | AES-256-GCM ciphertext **+ tag** of the secret bundle (NULL for ollama) |
| `secret_nonce` | `BYTEA` | per-record random 96-bit GCM nonce (NULL for ollama) |
| `secret_key_version` | `INT` | master-key epoch used (rotation tracking; NULL for ollama) |
| `secret_redaction` | `TEXT` | non-secret display hint (e.g. `…wxyz` last-4), written at save time (NULL for ollama) |
| `last_tested_at` | `TIMESTAMPTZ` | nullable |
| `last_test_outcome` | `TEXT` | nullable typed: `ok` \| `failed` |
| `last_test_detail` | `TEXT` | nullable typed reason — **never** the secret |
| `created_at` | `TIMESTAMPTZ NOT NULL` | app-written |
| `updated_at` | `TIMESTAMPTZ NOT NULL` | app-written (no DB-side default — G028) |

**Invariants (app-layer, fail-loud; optionally CHECK-reinforced):**
- ollama rows: secret columns MUST be NULL.
- hosted db-mode rows: when `enabled = true`, `secret_ciphertext` /
  `secret_nonce` / `secret_key_version` MUST be non-NULL.
- The secret **bundle** is a serialized map of the kind's secret fields
  (e.g. Bedrock `{aws_access_key_id, aws_secret_access_key}`) encrypted
  as ONE blob; non-secret params live in the SST registry, not here.

### 5.3 Monthly USD spend ledger — migration 060 (companion table)

Required to make budgets **load-bearing** (today's zero-cost CostFn never
exercises them). `model_usage_ledger` (append-only):

| Column | Type | Notes |
|--------|------|-------|
| `id` | `BIGSERIAL PRIMARY KEY` | |
| `actor_user_id` | `TEXT NOT NULL` | per-user **spend** dimension (allowed; this is per-user budget, NOT a per-user key) |
| `connection_id` | `TEXT NOT NULL` | |
| `model` | `TEXT NOT NULL` | provider-qualified |
| `tokens` | `INT NOT NULL` | |
| `usd_cost` | `NUMERIC(12,6) NOT NULL` | `0` for ollama |
| `created_at` | `TIMESTAMPTZ NOT NULL` | app-written; the monthly window key |

The budget pre-flight reads `SUM(usd_cost)` for the current month, per
`actor_user_id` (per-user ceiling) and global (all users), and compares
against the SST ceilings before each billable dispatch. Append-only — no
edits (audit-clean). Per-user rows are spend accounting, not credentials.

---

## 6. API / Contracts

### 6.1 Web operator surface (data contract; pixels deferred to UX)

Operator-gated (see [§11.4](#114-authorization-matrix)). Mounted beside
the existing web admin routes. **Every response omits plaintext
credentials.**

| Method + path | Request | Response (200) | Errors |
|---------------|---------|----------------|--------|
| `GET /v1/admin/model-connections` | — | `[{connection_id, kind, display_name, enabled, params, secret_present, secret_redaction, last_tested_at, last_test_outcome, model_count}]` | 401/403 (not operator) |
| `GET /v1/admin/model-connections/{id}` | — | one object as above | 404 (unknown slot, fail-loud), 403 |
| `PUT /v1/admin/model-connections/{id}/credential` | `{secret_fields:{…cleartext…}}` | redacted view (no secret echoed) | 404 (not a db-mode slot), 422 (missing required secret field for kind), 403 |
| `POST /v1/admin/model-connections/{id}/test` | — | `{outcome:"ok"\|"failed", detail, tested_at}` | 404, 403 |
| `POST /v1/admin/model-connections/{id}/enable` | — | redacted view (`enabled:true`) | 409 (untested/no-credential → cannot enable), 404, 403 |
| `POST /v1/admin/model-connections/{id}/disable` | — | redacted view (`enabled:false`) | 404, 403 |

**Never-return-plaintext rule (binding).** No endpoint returns a stored
credential. `PUT …/credential` is write-only; reads expose only
`secret_present` + `secret_redaction` (last-4) + `last_tested_*`. A
failed test reports `outcome:"failed"` with a typed `detail` and NEVER a
false `ok`; it NEVER substitutes Ollama. Enable is refused (409) unless
the slot has a credential and a passing test (no enabling an unverified
connection into the catalog).

### 6.2 Go → sidecar `/llm/chat` per-request credential contract

The single redesign that closes the primary gap. `ChatRequest` is
extended **additively** on both sides (Go
[`llm.ChatRequest`](../../internal/assistant/openknowledge/llm/client.go#L93-L99)
and Python
[`ChatRequest`](../../ml/app/schemas.py#L126-L133), which is
`extra="forbid"` so the new fields must be declared):

| New field | Type | Semantics |
|-----------|------|-----------|
| `provider` | `str` (closed set) | routing discriminant: `ollama` \| `anthropic` \| `openai` \| `azure-foundry` \| `google` \| `bedrock` |
| `api_base` | `str \| None` | provider endpoint/base URL (carries today's `OLLAMA_URL` for ollama; Azure `endpoint`; OpenAI `base_url`; etc.) |
| `api_key` | `str \| None` | **cleartext** credential, decrypted by the Go core, supplied per request; `null` for ollama; used transiently, never logged/persisted |
| `provider_params` | `dict \| None` | non-secret per-kind routing extras (Azure `api_version`+`deployment`, OpenAI `org`, Vertex `project`+`location`, Bedrock `region`, Gemini-vs-Vertex discriminator) |

`model` continues to carry the **backend** model id (the Go core strips
the provider qualifier; see [§10](#10-identifier-grammar-oq-4)).

**Redesigned `_dispatch_live` (adopts the `synthesis.py` fork):**

```text
if req.provider == "ollama":
    model    = f"ollama_chat/{req.model}"     # byte-for-byte today
    api_base = req.api_base                    # today's OLLAMA_URL value
    # NO api_key  → litellm kwargs IDENTICAL to the current code path
else:
    model    = compose_litellm_model(req.provider, req.model, req.provider_params)
    api_base = req.api_base
    api_key  = req.api_key                      # required; absent → typed 500 llm_misconfigured
    # + provider_params (api_version, deployment, region, org, project, location)
```

**Byte-for-byte guarantee (parity test).** For an `ollama` dispatch the
`litellm.acompletion(**kwargs)` arguments are identical to the current
`_dispatch_live`. A contract test asserts the kwargs dict for a fixed
Ollama request is unchanged.

**Secrets out of the sidecar's mouth (binding).** `_dispatch_live` MUST
NOT log `api_key`. The existing error handlers echo
`f"{type(e).__name__}: {e}"` — the design mandates the error `detail`
that crosses the wire is built from `type(e).__name__` + `provider` +
`model` only, with any `api_key` substring scrubbed (litellm exceptions
can embed the key in a URL/header). The credential rides the request body
over the **internal, AuthToken-protected** sidecar boundary (the same
boundary today's `/llm/chat` uses); per-request transmission means the
sidecar holds no static provider secret (owner decision 3).

### 6.3 Discovery contract (OQ-2)

`CatalogAggregator.GetCatalog(ctx) → (ModelCatalog, []ProviderDiscoveryStatus)`:

```text
ModelDescriptor {
    id            string   // provider-qualified, e.g. "anthropic/claude-3-5-sonnet"
    connection_id string
    kind          string
    tool_capable  bool
    vision        bool
    context_window int
}
ProviderDiscoveryStatus {           // ALWAYS one per effective-enabled connection
    connection_id string
    kind          string
    state         string  // "ok" | "unreachable" | "timeout" | "auth_failed" | "disabled"
    model_count   int
    detail        string  // typed, never a secret
}
```

- **Caching + TTL (SST, fail-loud).** `llm.discovery.cache_ttl_ms`
  (REQUIRED `> 0` when ≥1 connection declared; no hardcoded default).
  In-memory last-good catalog in the Go core; rebuilt on expiry.
  **Stale-while-revalidate:** serve last-good while a bounded background
  refresh runs, so a picker render never blocks on a slow provider.
- **Per-provider timeout (SST, fail-loud).**
  `llm.discovery.per_provider_timeout_ms` (REQUIRED `> 0`). Each
  provider's discovery call is independently bounded; providers run in
  parallel.
- **Graceful degradation (NFR-1).** A timed-out / unreachable /
  auth-failed provider contributes its last-good entries (if any) or
  none, and ALWAYS emits a typed `ProviderDiscoveryStatus` — **never
  silently dropped with no signal.** The reachable subset + all other
  providers still render. The Ollama path's availability is independent
  of any hosted provider (NFR-2). The picker surfaces the statuses
  (wording is UX-owned, OQ-6).

### 6.4 Selection surfaces (provider-qualified catalog, parity preserved)

The existing surfaces extend, not multiply:
- `GET /v1/agent/model` ([`agent_model.go`](../../internal/api/agent_model.go))
  `agentModelView.allowed_models` becomes the provider-qualified catalog
  (optionally enriched with per-entry capability metadata as a sibling
  field, additive). `effective_model` / `sticky_model` / `system_default`
  / `source` are provider-qualified strings.
- Telegram `/model` ([`model_command.go`](../../internal/telegram/model_command.go))
  renders the same combined numbered list.
- Both resolve through the **same** `agenttool` singletons (validator +
  store). Selecting an off-catalog id → the **same** `modelswitch.Rejection`
  envelope, now over the provider-qualified catalog.

---

## 7. Configuration (SST additions, fail-loud)

New SST under `llm:` (illustrative; the design owns final keys, the
NO-DEFAULTS / fail-loud shape is binding — no `${VAR:-default}`, no
`getenv(k, default)`, no `unwrap_or`):

```yaml
llm:
  # ── existing single-provider block retained (feeds synthesis.py /
  #    drive_classify.py / healthchecks; NOT redesigned by spec 096) ──
  provider: "ollama"
  # … existing fields …

  # ── NEW: operator-global connection registry (SST source of truth) ──
  connections:
    - id: "local-ollama"            # stable slug → the "ollama/…" qualifier
      kind: "ollama"
      params: { base_url: "http://ollama:11434" }   # non-secret
      models: { strategy: "live" }                   # /api/tags
      # ollama: NO secret_ref (local, no credential)
    - id: "anthropic-primary"
      kind: "anthropic"
      secret_ref: { mode: "db" }    # operator enters via web UI → encrypted vault
      models:
        strategy: "curated"
        list:
          - { id: "claude-3-5-sonnet", tool_capable: true, vision: true, context_window: 200000 }
    # openai / azure-foundry / google / bedrock similarly, each with its
    # per-kind non-secret params + secret_ref (mode: db | env) + curated models
  discovery:
    cache_ttl_ms: 60000             # REQUIRED > 0 when ≥1 connection (no default)
    per_provider_timeout_ms: 4000   # REQUIRED > 0 (no default)
  model_costs:                      # REQUIRED entry for every non-ollama billable model
    - { model: "anthropic/claude-3-5-sonnet", input_usd_per_1k: 0.003, output_usd_per_1k: 0.015 }
    # a paid model with NO entry here → fail-loud refusal at dispatch (NEVER $0)

infrastructure:
  secret_keys:
    # … existing keys …
    - LLM_PROVIDER_SECRET_MASTER_KEY   # NEW managed secret (+ Go + shell mirrors)
```

**Fail-loud validation (`internal/config/config.go` `Validate`,
extending the existing `validateModelEnvelopes` envelope guard):**
- unknown `kind`; a missing required per-kind non-secret param; a
  `secret_ref.mode: env` whose `env_key` is not in
  `infrastructure.secret_keys`; a `discovery.*` value `≤ 0`; a
  non-ollama model offered by an enabled connection with no
  `model_costs` entry — each aborts config-generation / startup with a
  **named** error. No silent fallback to Ollama anywhere.
- The master key is REQUIRED when **any** db-mode hosted connection is
  declared (see [§11.2](#112-master-key-load-fail-loud-g028)); an
  Ollama-only deployment requires no new secret (local-first default
  unchanged).

**Coexistence with `switchable_models` / `tool_capable_gather_models`.**
These 088/089 SST lists keep their meaning (operator-curated admissible
synthesis / tool-capable gather sets) but their **entries become
provider-qualified**, and the value handed to `NewAllowlist` is computed
by the `CatalogAggregator` (the Ollama-side curated subset ∪ the
effective-enabled hosted models). The memory-envelope / profile
co-residence check applies **only to `ollama/`-kind entries** (hosted
models do not load into the local Ollama envelope); this is a
provider-agnostic extension of the validator's construction, not a spec
088/089 amendment (see [§14](#14-builds-on-088089--invariants-preserved)).

---

## 8. Identifier Grammar (OQ-4)

**Canonical form:** `<provider-kind>/<backend-model-id>`, where
`<provider-kind>` is a member of the closed registry vocabulary and
`<backend-model-id>` is the provider's native id.

**Parsing rule:** split on the **FIRST** `/` only.
`kind = substring before first '/'`; `backend-model-id = everything
after` (may itself contain `/` and `:`). This handles Ollama namespaced
ids (`ollama/library/llama3:8b` → kind `ollama`, model
`library/llama3:8b`) and Bedrock ids
(`bedrock/anthropic.claude-3-5-sonnet-20241022-v2:0`). Unambiguous
because `kind` never contains `/`.

**Deterministic mapping:** with the **at-most-one-effective-enabled
connection per kind** invariant, `(kind, model) → (connection,
backend-model-id)` is a function. `azure/<deployment>`,
`google/<model>`, `bedrock/<model-id>` all resolve cleanly.

**Round-trip through the existing validator + store (no parity break):**
- `modelpref.Store` is string-keyed (`synthesis_model TEXT`) → a
  provider-qualified string stores losslessly.
- `modelswitch.Allowlist.isSwitchable` does string equality → it
  compares provider-qualified strings; the admissible set is the
  injected catalog (the leaf stays pure, stdlib-only).

**Bare-id coexistence / migration (preserves 088/089 callers verbatim).**
Today's SST + 089-era callers use bare Ollama ids (`gemma3:4b`).
**Canonicalization at the resolver boundary (in `agenttool`, OUTSIDE the
pure leaf):** a candidate or admissible entry with no `<kind>/` prefix is
normalized to `ollama/<id>` **iff** `<id>` is in the Ollama installed
set, before the validator's string compare. Thus a bare `gemma3:4b` from
a 089-era surface still validates and dispatches identically, and the
no-override Ollama-only path is byte-for-byte unchanged. The `modelswitch`
leaf is **not modified** for this — canonicalization is applied to its
inputs at the boundary that already owns the catalog.

---

## 9. Architecture decision: why the Go core holds the seam

| Decision | Rationale |
|----------|-----------|
| Master key + decryption + CostFn + budget + catalog + validator in the **Go core** | Constitution C2 (Go is the authoritative orchestrator; Python is the ML sidecar). Owner decision 3: the master key never leaves the Go core. |
| Sidecar receives **per-request** cleartext key | Mirrors the established `synthesis.py(api_key=…)` pattern; no static provider secret on the sidecar; the sidecar cannot decrypt anything (holds no master key). |
| Connection **topology** in SST, **credential + activation** in DB | Honors Config-SST (topology single source of truth) AND the owner's "keys entered in the web UI, stored encrypted in DB" — the two-plane split is the only design that satisfies both without a config-SST violation. |

---

## 10. Data flow summary (selection → dispatch → attribution)

```
SST registry ─┐
              ├─► CatalogAggregator ─► provider-qualified catalog ─► modelswitch.Allowlist (injected set)
DB enabled ───┘            │                                              │
                           │                              ResolveEffective (per-req > sticky > default)
                           │                                              │
                           ▼                                              ▼
                 ProviderDiscoveryStatus[]                    effective (provider-qualified) model
                 (typed, surfaced)                                        │
                                                   canonicalize → map (kind,model) → (connection, backend id)
                                                                          │
                                              CostFn(provider,model,tokens) + budget pre-flight (ledger)
                                                                          │  (paid + over budget → typed refusal)
                                              SecretVault.decrypt(connection) → cleartext (in-core)
                                                                          │
                                              POST /llm/chat {provider, api_base, api_key, provider_params}
                                                                          │
                                              ChatResponse → spend-ledger append → attribution {provider, model, source}
```

---

## 11. Security / Compliance

### 11.1 AEAD primitive + justification

**AES-256-GCM** (Go stdlib `crypto/aes` + `crypto/cipher.NewGCM`),
single operator master key, **per-record random 96-bit nonce**, 128-bit
auth tag, **AAD** = a canonical context binding (`connection_id` + `:` +
`provider_kind` + `:` + `secret_key_version`) so a ciphertext cannot be
relocated to another record or replayed under a different key epoch.

- **Authenticated (AEAD).** Tamper or wrong-key → decryption fails
  loudly (the tag/AAD check), never silent garbage.
- **Reversible — required.** The credential is **replayed** to
  `Authorization: Bearer <key>` (litellm) at call time, so it MUST be
  recoverable. **One-way hashing (argon2id) is structurally wrong for
  this data and is forbidden here** — argon2id verifies a *presented*
  secret; it cannot *recover* one. This credential is the **reversible
  managed-secret class** (`CARD_REWARDS_GCAL_CREDENTIALS`,
  `telegram.bot_token`), explicitly NOT the verifier class
  (`AUTH_AT_REST_HASHING_KEY`, which one-way-hashes bearer tokens). The
  design states this distinction so a future agent does not "harden" the
  vault into a hash and break dispatch.
- **Single-key (not full KEK/DEK envelope) — justified.** At
  single-tenant scale (≤ a handful of connections) the rotation
  re-encrypt-all cost is trivial, so the KEK/DEK indirection is premature
  (Complexity Tracking). `secret_key_version` is stored so rotation is
  auditable and a future envelope move is additive.
- **At-rest protection (NFR-4).** A Postgres/repo leak yields only
  ciphertext + nonce + key-version; without the env-held master key it is
  unusable. The master key is never committed (managed-secret path).

### 11.2 Master-key load (fail-loud, G028)

- **Name:** `LLM_PROVIDER_SECRET_MASTER_KEY`, a managed secret added to
  all **three** mirrors (`config/smackerel.yaml` `infrastructure.secret_keys`,
  [`internal/config/secret_keys.go`](../../internal/config/secret_keys.go),
  `scripts/commands/config.sh`) — the existing drift contract test
  enforces parity. Provisioned via the deploy adapter's
  `<target>.enc.env`; **never committed**; 32 bytes (256-bit),
  base64-encoded in env.
- **Fail-loud predicate.** When **any ENABLED** `llm.connections[]` entry
  declares `secret_ref.mode: db` (i.e. the deployment intends to use the
  vault), the master key MUST be present and decode to exactly 32 bytes
  at startup, else **abort non-zero with a named error**, e.g.
  `llm: LLM_PROVIDER_SECRET_MASTER_KEY is required and must be a base64
  32-byte key when one or more ENABLED llm.connections declare secret_ref.mode=db`.
  Declared-but-disabled db-mode hosted slots (the default-shipped
  anthropic/openai/azure-foundry/google/bedrock templates) do NOT require
  the key at boot — the admin surface is still mounted + operator-gated for
  them, and the key becomes mandatory once such a slot is enabled (which
  requires a credential, which itself requires the configured vault).
  When NO db-mode hosted connection exists (Ollama-only), the key is not
  required — the local-first default adds no new required secret.
- **Confinement.** Loaded once into the Go core's `SecretVault`; never
  passed to the sidecar, never logged (length/presence only, per
  terminal-discipline secret rules).

### 11.3 Master-key rotation procedure (documented)

1. Provision the new key as `LLM_PROVIDER_SECRET_MASTER_KEY` (epoch
   N+1); keep the prior key available as
   `LLM_PROVIDER_SECRET_MASTER_KEY_PREVIOUS` (epoch N) for the window.
2. Run a one-shot, operator-invoked re-encrypt routine (a Go core
   subcommand; design specifies the contract, plan/implement build it):
   for each row, decrypt with the `secret_key_version` it names (N via
   the previous key), re-encrypt with epoch N+1 (new nonce + AAD bound to
   N+1), bump `secret_key_version` to N+1 — transactional per row.
3. When every row is at N+1, remove `…_PREVIOUS`.
4. The routine NEVER logs key bytes or plaintext; it reports counts only.

### 11.4 Authorization matrix

| Surface | Operator | Authenticated user | Public |
|---------|----------|--------------------|--------|
| `GET/PUT/POST /v1/admin/model-connections*` (wire/test/enable/disable, credentials) | ✅ | ❌ 403 | ❌ 401 |
| `GET/PUT/DELETE /v1/agent/model` (own selection) | ✅ | ✅ (own `actor_user_id` only) | ❌ 401 |
| Telegram `/model` (own selection) | ✅ | ✅ (own subject only) | n/a |
| Discovery catalog (read, no secrets) | ✅ | ✅ | ❌ 401 |
| Decrypted credential | core-internal only | ❌ never | ❌ never |

**Operator gate (FR-B4).** The admin surface admits only the operator
identity; ordinary users are 403. Today the web surface has no hard
operator role (shared-token sessions read as "operator",
[`internal/web/invites.go`](../../internal/web/invites.go#L196-L201)).
**Proposed:** an SST `infrastructure.operator_user_ids` allowlist
(fail-loud, NO-DEFAULTS) checked by an operator middleware, reusing the
`webAuthMiddleware` session/bearer subject. The exact mechanism is a
residual design decision to confirm against the live auth model in plan
([§18](#18-risks--open-questions)).

### 11.5 Secrets never leak (FR-X4 / SCN-096-G05)

- No API response returns a plaintext credential (write-only PUT; reads
  expose presence + last-4 + test state only).
- No log/span/error (Go core or sidecar) contains a credential or the
  master key; sidecar error `detail` is built from exception type +
  provider + model with `api_key` scrubbed.
- The provider-qualified catalog, selection, and attribution surfaces
  carry provider+model identity only — never a credential.
- Tests assert "secret never appears in logs/responses" as an explicit
  negative check.

---

## 12. Budget CostFn (OQ-3)

**Today.** `costFn := okagent.CostFn(func(int) float64 { return 0 })`
([`wiring_assistant_openknowledge.go`](../../cmd/core/wiring_assistant_openknowledge.go))
— zero-cost; the per-query / monthly / per-user USD pre-flight in
[`agent.go`](../../internal/assistant/openknowledge/agent/agent.go) never
binds.

**Design.** Make cost **model-aware** and budgets **load-bearing** for
paid providers while keeping Ollama free:

- **Model-aware CostFn.** Extend the cost seam so the per-call estimate
  knows the effective (provider-qualified) model — the agent already
  resolves it (`Effective`). The CostFn becomes a closure over an SST
  **rate table** invoked with `(provider, model, tokens)`:
  - `provider == ollama` → **`$0`**, deterministically (free local
    inference), and the budget is **not consumed** (NFR-2: zero cost +
    zero added latency on the local path).
  - else → look up the SST `llm.model_costs` rate for the
    provider-qualified model. **A missing rate for a billable model is a
    fail-loud typed refusal BEFORE the call — NEVER a silent `$0`** (the
    critical NO-DEFAULTS point; a silent zero would let a paid call
    bypass the budget).
- **Rate source (SST, fail-loud).** `llm.model_costs[]`
  (`input_usd_per_1k` + `output_usd_per_1k` per provider-qualified
  model). Config validation requires a rate for every non-ollama model
  any enabled connection offers; otherwise config-generation / startup
  aborts with a named error (so the gap is caught before runtime, and
  the runtime refusal is the defensive backstop).
- **Pre-flight (existing gate, now exercised).** Before a billable
  dispatch the resolver estimates cost (prompt-token estimate × rate),
  reads the month-to-date spend from the `model_usage_ledger`
  ([§5.3](#53-monthly-usd-spend-ledger--migration-060-companion-table))
  for the caller (`actor_user_id`) and globally, and refuses with a
  typed "budget exhausted" reason (the existing
  `ErrCapUSDPerUserMonth` / `ErrCapUSDMonthly` sentinels) when either
  ceiling would be breached — **before** any provider call (FR-X2 /
  SCN-096-G03). After a successful dispatch the actual cost is appended
  to the ledger.

---

## 13. Observability

If the repo's `traceContracts` are wired for this workflow, the design's
runtime contract is:

- **Spans.** `model.dispatch` (attrs: `provider`, `connection.id`,
  `model` [provider-qualified], `model.source` [`per_request` \|
  `sticky` \| `default`], `turn` [`gather` \| `synthesis`],
  `cost.usd`) → child `llm.chat` to the sidecar. `model.discovery` with
  one child span per provider (`connection.id`, `kind`, `state`,
  `model_count`, `latency_ms`). `model.connection.test`
  (`connection.id`, `kind`, `outcome`).
- **Metrics.** per-provider discovery latency + reachability; per-provider
  dispatch count / tokens / USD; budget-refusal count; test-connection
  outcomes; vault decrypt failures.
- **Invariants / red flags (alarming).** a credential or the master key in
  ANY span/log/error; a dispatch whose `provider != ollama` but whose
  actual backend is ollama with no typed refusal (silent fallback); a
  `cost.usd == 0` on a `provider != ollama` dispatch (missing-rate silent
  zero); a `ProviderDiscoveryStatus` absent for an effective-enabled
  connection (silent drop).

### Trace Topology (planned; MUST-when-wired)

```
ask.openknowledge                          # root = workflow
└─ model.resolve                            # attrs: model.source, turn, provider, connection.id
   ├─ budget.preflight                       # attrs: cost.usd(est), invariant: paid+over → refusal, no dispatch
   ├─ vault.decrypt                          # invariant: never logs ciphertext/plaintext/key
   └─ llm.chat                               # attrs: provider, model; invariant: api_key never in span
model.discovery                             # root (separate workflow)
└─ provider.discover (×N, parallel)          # attrs: connection.id, kind, state, model_count, latency_ms
                                             # invariant: exactly one status per effective-enabled connection
```

OMIT if the scope is not wired / not service-bearing (the plan decides
per-scope).

---

## 14. Builds On 088/089 — Invariants Preserved

| # | 088/089 invariant | How this design upholds it |
|---|-------------------|----------------------------|
| 1 | **One validator / one store** | `modelswitch.Allowlist` + `modelpref.Store` stay singular, reached by both surfaces via the `agenttool` singletons. The catalog becomes the validator's **injected** admissible set; the web **connections** surface manages credentials only and adds NO second validator/store (SCN-096-D03, G06). |
| 2 | **Pure leaf `modelswitch`** | The leaf stays stdlib-only; provider data is **injected at construction** (catalog → `NewAllowlist` switchable slice) and canonicalization is applied to its **inputs** at the `agenttool` boundary — `modelswitch` imports nothing new. |
| 3 | **Synthesis-vs-gather fork** | `Override{SynthesisModel, GatherModel}` unchanged; each turn re-points independently; only the admissible ids are now provider-qualified and dispatch is provider-aware (G06). |
| 4 | **per-request > sticky > default precedence** | `ResolveEffective` unchanged; canonicalization happens before the compare; precedence ordering untouched (SCN-096-D05). |
| 5 | **Byte-for-byte no-override path** | No hosted connection effective-enabled ⇒ catalog == Ollama set ⇒ `Override.IsZero()` ⇒ `WithModelOverride` returns the receiver unchanged ⇒ identical `ollama_chat/<model>` + `OLLAMA_URL` + no `api_key` litellm kwargs (parity test) (SCN-096-A03, FR-A4). |
| 6 | **Closed-set validation, never silent default/passthrough** | Off-catalog selection → the **same** `modelswitch.Rejection` shape, over the provider-qualified catalog; a misconfigured/unreachable connection → absent from the catalog + a typed discovery status, NEVER a silent Ollama substitution (SCN-096-D04, G01). |
| 7 | **Attribution contract** | The existing `ModelAttribution` / `TurnResult.Model` + source classification is reused; attribution strings become provider-qualified (`anthropic/claude-3-5-sonnet`), preserving "which model answered" (FR-X3, G04). |
| 8 | **Envelope/profile co-residence guard** | `NewAllowlist`'s profile + envelope check is **scoped to `ollama/`-kind entries** (hosted models don't load the local Ollama envelope) — a provider-agnostic extension of construction, not a behavioural amendment of 088/089. |

The net-new surface beyond 088/089 — N connections (Layer A), the web
wire/test surface (Layer B), the cross-provider discovery feeding the
existing validator (Layer C) — is exactly what 088/089 scoped out.

---

## 15. Testing Strategy

| Test type | Stack | Coverage (→ scenarios) |
|-----------|-------|------------------------|
| **Unit (Go)** | core | canonicalization + provider-qualified validation (leaf purity preserved); identifier split-on-first-`/` + round-trip; AES-256-GCM encrypt/decrypt round-trip + **AAD-tamper rejection** + **wrong-key rejection**; master-key load fail-loud (present-when-db-mode / absent-allowed-when-ollama-only); registry SST validation fail-loud; model-aware CostFn (ollama→$0; missing rate→refusal); two-plane effective-enabled predicate (A01, A04, D04, G01, G02, X4) |
| **Unit (Python)** | sidecar | `_dispatch_live` ollama branch **byte-for-byte parity** (kwargs unchanged); hosted branch composes litellm model + `api_key`; `ChatRequest` `extra="forbid"` still holds; `api_key` never logged + error `detail` scrubbed (A02, A03, G05) |
| **Integration** | Go+sidecar+PG+Ollama (+ stub hosted endpoint declared as a connection) | provider-aware `/ask` end-to-end to a hosted connection; discovery aggregation Ollama + hosted; **graceful degradation** (one provider down → typed status, catalog still serves); budget pre-flight refuses an exhausted paid model **before** dispatch; vault persist + round-trip via a real test master key (A02, D01, D02, G03, NFR-1) |
| **E2E-api** | live stack | operator wire → **truthful** test pass/fail → enable; user picks hosted via `/v1/agent/model` + Telegram parity; off-catalog refusal; attribution present (W01, W02, W04, D03, G04, G06) |
| **E2E-UI** | — | **deferred to UX** (connections page); noted ux-owned |
| **Stress** | core | discovery under many connections + slow-provider timeout bound; Ollama dispatch latency parity (NFR-2) |

**Mandatory negative/error coverage:** bad-credential test (W04),
missing-config abort (G02), exhausted-budget refusal (G03),
unknown-model refusal (D04), missing-master-key abort, missing-cost-rate
refusal, AEAD-tamper rejection, secret-never-in-logs, byte-for-byte
Ollama parity.

**Isolation (binding).** Live categories hit the real stack (no
request-interception of the Go↔sidecar boundary — that boundary is real;
a stub *provider endpoint* declared as a connection is a real network
peer, not interception). Test credentials + the test master key are
**test-scoped / ephemeral** (`env=test`), never prod surfaces
(env-pollution-isolation); the vault + ledger use the disposable test DB
(test-environment-isolation). Synthetic connections are uniquely
identifiable and torn down.

---

## 16. Complexity Tracking

| Decision | Simpler alternative considered | Why rejected |
|----------|-------------------------------|--------------|
| Single-key AES-256-GCM + `key_version` column | Full KEK/DEK envelope encryption | At single-tenant scale rotation re-encrypts a handful of rows; KEK/DEK indirection is premature. `key_version` keeps a future envelope move additive. |
| New `model_usage_ledger` (monthly USD spend) | Reuse the existing config USD ceilings only | The zero-cost CostFn never exercised the ceilings; FR-X2 requires budgets to **bind** for paid providers, which needs month-to-date spend accounting. |
| Two-plane (SST registry + DB credential/activation) | Connections fully in SST, or fully in DB | Fully-SST can't accept web-UI-entered keys + runtime enable/disable; fully-DB violates Config-SST topology. The split is the only design honoring both. |
| `<kind>/<model>` + at-most-one-enabled-connection-per-kind | `<connection-id>/<model>` (always) | The owner's stated providers are one-per-kind; kind-qualified matches the owner's grammar examples and is simpler. Connection-id qualification is an additive extension if a second same-kind connection is ever needed. |
| Hosted discovery via SST-curated `models[]` (default) + live-list adapter capability | Live list API for every provider on every discovery | Per-provider list-API variance + latency + failure surface; curated lists are fail-loud SST and graceful. Live-list stays available (Ollama uses it; OpenAI may). |
| Provider-aware extension of `NewAllowlist` envelope check (ollama-scoped) | A parallel multi-provider validator | A second validator violates the 088/089 one-validator invariant. Scoping the existing check to `ollama/` entries is the minimal provider-agnostic extension. |

---

## 17. Migrations & Rollout

- **Migration 060** (single file, two tables): `model_provider_connections`
  (encrypted vault) + `model_usage_ledger` (spend). Forward-only with a
  documented manual rollback (`DROP TABLE …`), mirroring 059's style.
- **Rollout / no flag.** Per spec §11, no feature flag: an Ollama-only
  deployment (no db-mode hosted connection) is byte-for-byte today. The
  capability activates purely by the operator declaring connections in
  SST + wiring credentials in the UI. If plan later concludes a
  controlled-rollout flag is warranted, it MUST be declared in
  `state.json.flagsIntroduced`, default-ON only in the owning `next`
  train (release-train policy).
- **Train:** `next` (per spec §11).

---

## 18. Risks & Open Questions

**Residual — routed to UX (OQ-5/OQ-6):**
- **OQ-5 (ux-owned).** The model-connections page visual + flow
  (add/test/enable/disable layout, redacted-credential affordance,
  per-provider status presentation). Design provides the **data
  contract** ([§6.1](#61-web-operator-surface-data-contract-pixels-deferred-to-ux)); UX owns the pixels.
- **OQ-6 (ux-owned).** Picker capability-surfacing wording (how
  provider-qualified ids + capability flags + per-provider discovery
  status are phrased in Telegram + web). Design provides the catalog +
  status data shapes; UX owns the copy.

**Residual — for plan:**
- **Operator gate mechanism.** Design specifies the operator-only
  requirement + proposes an SST `infrastructure.operator_user_ids`
  allowlist; the exact wiring against the live `webAuthMiddleware`
  session/bearer model is a plan-phase confirmation
  ([§11.4](#114-authorization-matrix)).
- **Add-connection ergonomics.** The two-plane model means "add a new
  provider slot" is an SST edit + `config generate`, while "wire/test/
  enable" is the UI. Plan should confirm this operator workflow reads
  cleanly in the runbook.
- **CostFn signature change.** Threading the effective model into the
  cost seam is a surgical extension of the existing `okagent` cost
  accounting; plan sizes the exact `agent.go` change.

**Upstream correction (not owned by design — `bubbles.analyst` owns
`state.json`).** `state.json.specDependsOn` carries two inaccurate
directory pointers: `specs/088-open-knowledge-model-switching` →
`specs/088-runtime-switchable-models`, and
`specs/089-gather-turn-model-switching` →
`specs/089-runtime-model-hotswap-persistent-selection`. The `spec.md`
body already cites the correct paths. Routed to `bubbles.analyst` in the
result envelope.

---

Links: [spec.md](spec.md) | [state.json](state.json)
