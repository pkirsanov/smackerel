# Scopes 096 — Multi-Provider AI Model Connections

> Planned by `bubbles.plan (parent-expanded full-delivery)`. Seven
> sequential, scope-gated scopes (SCOPE-01..07). Each carries a Test Plan
> (scenario → concrete manifest test) and a tiered DoD (Tier-1 universal +
> Tier-2 role-specific).
>
> **Authoritative mapping.** The scenario → scope → test mapping is owned
> by [scenario-manifest.json](scenario-manifest.json) (20 scenarios,
> SCN-096-A01..G06, assigned SCOPE-01..07). This plan MUST NOT contradict
> the manifest; every `linkedTests` entry named below is copied verbatim
> from it. Full Given/When/Then for each scenario lives in
> [spec.md](spec.md) §4; the manifest scenario `title` is reproduced here
> as the behavioural statement so a reviewer need not cross-reference.
>
> **This is the full-delivery plan.** All seven scopes (SCOPE-01..07) are
> authored in this file — SCOPE-01..04 and SCOPE-05..07 were written across
> a two-part split to keep each planning turn bounded; `state.json`
> `certification.scopeProgress` covers all seven. The plan is executable in
> strict sequential order (see the [Sequential Gate
> Summary](#sequential-gate-summary-all-seven-scopes) at the foot of the
> file).
>
> **Extends, does NOT amend** specs 088
> ([088-runtime-switchable-models](../088-runtime-switchable-models/spec.md))
> and 089
> ([089-runtime-model-hotswap-persistent-selection](../089-runtime-model-hotswap-persistent-selection/spec.md)).
> Every 088/089 parity, precedence, fork, attribution, and no-override
> behaviour is preserved verbatim and made provider-agnostic
> ([design.md](design.md) §14). The owner-confirmed posture
> (operator-global connections, reversible encrypted-at-rest vault under an
> operator master key the Go core never lets out) is binding (design.md
> Design Brief → Resolved Decisions).
>
> **Terminal posture (C7).** In-repo proof is mechanism-level: pure
> validator tables, config fail-loud, the `connvault` AEAD round-trip,
> the `_dispatch_live` byte-for-byte parity + secret-scrub tests, the
> CatalogAggregator graceful-degradation table, and ephemeral-Postgres
> integration with **synthetic** secrets only. The live hosted-provider
> `e2e-api` legs (real Anthropic/OpenAI reachability, live Ollama
> `/api/tags`, GPU-dependent answers) are a SEPARATE downstream
> `bubbles.devops` dispatch on home-lab hardware — `nextRequiredOwner`
> after implement+test. **No commit/push from the planning phase.**

---

## Execution Outline

A reviewer can read this outline and catch a wrong scope order or a
missing validation checkpoint BEFORE the full plan is implemented. Phase
Order lists all seven scopes for plan-shape review; SCOPE-01..04 are
elaborated in this file, SCOPE-05..07 in the continuation.

### Phase Order

1. **SCOPE-01 — Provider-connection registry + config SST schema (foundation).**
   The closed-set, fail-loud `llm.connections[]` registry (id, kind,
   non-secret params, `secret_ref`), plus `llm.discovery` and
   `llm.model_costs` SST, loaded NO-DEFAULTS. Pure config + loader; NO
   vault, NO dispatch, NO discovery wiring yet. *(design §5.1, §7)*
2. **SCOPE-02 — Encrypted credential vault + master-key lifecycle.** The
   AES-256-GCM `connvault.SecretVault`, migration 061
   `model_provider_connections`, and the fail-loud master-key load +
   rotation. Reversible, authenticated, never-plaintext. *(design §5.2,
   §3 foundation #2)*
3. **SCOPE-03 — Provider-aware `/ask` dispatch (credential seam).** The
   additive `ChatRequest` extension (Go + Python `extra=forbid`) and the
   `_dispatch_live` ollama-vs-hosted fork carrying the per-request
   decrypted key, with provider-qualified attribution and secret-scrubbed
   logs/errors. *(design §6.2, §2.4, §2.5)*
4. **SCOPE-04 — Discovery + unified catalog + identifier canonicalization.**
   Per-kind discovery adapters + the `CatalogAggregator` (SST-bounded,
   graceful-degradation, typed status) feeding the **existing**
   `modelswitch` validator as an injected admissible set, with
   `<kind>/<backend-id>` canonicalization at the agenttool boundary.
   *(design §6.3, §10, §3 foundation #3)*
5. **SCOPE-05 — Model-aware CostFn + load-bearing USD budget enforcement.**
   Replace the zero-cost CostFn with a model-aware closure over the SST
   `llm.model_costs` rate table (ollama=$0 non-consuming; a billable model
   with no rate = fail-loud typed refusal, never a silent $0) + an
   append-only `model_usage_ledger`, making the EXISTING 064/087 per-user +
   global USD budget pre-flight load-bearing before any billable dispatch.
   *(design §12, §5.3; SCN-096-G03)*
6. **SCOPE-06 — Operator-gated web admin connection surface (wire/test/enable).**
   The operator-gated `/v1/admin/model-connections*` surface (write-only
   credential, truthful typed test, 409-guarded enable, disable) + an
   operator middleware + the PWA triad, on the runtime plane, feeding the
   SCOPE-04 catalog via the effective-enabled predicate. *(design §6.1,
   §5.1, §11.4; SCN-096-W01..W04)*
7. **SCOPE-07 — Combined catalog selection across Telegram + web, 088/089 parity.**
   Render ONE provider-grouped, provider-qualified, cost-hinted catalog
   across the Telegram `/model` picker + `/v1/agent/model` through the
   EXISTING 088/089 validator + store; additive `capabilities[]` /
   `cost_class` enrichment (`allowed_models[]` byte-for-byte), non-tool-
   capable gather shown-disabled + rejected. *(design §6.4, §8, §14;
   SCN-096-D02/D03/D05/G06)*

### New Types & Signatures (the C-header view — SCOPE-01..04)

- **`internal/config`** (registry + SST): `ModelConnection{ ID, Kind
  string; Params map[string]any; SecretRef SecretRef; Models ModelSpec }`;
  `SecretRef{ Mode string /* db | env | "" */; EnvKey string }`;
  `DiscoveryConfig{ CacheTTLms, PerProviderTimeoutMs int }` (both REQUIRED
  `> 0`, fail-loud); `ModelCost{ Model string; InputUSDPer1k,
  OutputUSDPer1k float64 }`; closed-set `Validate()` over the kind
  vocabulary + per-kind required params + env-mode-secret-in-`secret_keys`
  + paid-model-has-cost checks. NEW managed secret
  `LLM_PROVIDER_SECRET_MASTER_KEY` in `infrastructure.secret_keys`.
- **`internal/assistant/openknowledge/connvault`** (NEW pkg):
  `SecretVault` with `Encrypt(connID, kind string, keyVersion int, bundle
  map[string]string) (ciphertext, nonce []byte, redaction string, err
  error)` and `Decrypt(rec) (map[string]string, error)` — AES-256-GCM,
  per-record 96-bit nonce, `AAD = connID|kind|keyVersion`, last-4
  redaction; fail-loud master-key load; never returns plaintext to a
  surface, never one-way hashes.
- **`internal/assistant/openknowledge/llm.ChatRequest`** (additive):
  `Provider string`; `APIBase *string`; `APIKey *string`;
  `ProviderParams map[string]any` — mirrored in Python
  `ml/app/schemas.py::ChatRequest` (`extra="forbid"`).
- **`internal/assistant/openknowledge/llm.DispatchResolver`** (NEW):
  resolves `(connection + decrypted key + backend model)`; refuses a
  not-effective-enabled target with a typed reason — NEVER a silent Ollama
  fallback.
- **`internal/assistant/openknowledge/catalog`** (NEW pkg):
  `ModelDescriptor{ ID, ConnectionID, Kind string; ToolCapable, Vision
  bool; ContextWindow int }`; `ProviderDiscoveryStatus{ ConnectionID,
  Kind, State, Detail string; ModelCount int }`;
  `CatalogAggregator.GetCatalog(ctx) (ModelCatalog,
  []ProviderDiscoveryStatus)`; `Canonicalize(raw, installed) string`
  (split on FIRST `/`). The `modelswitch` leaf stays import-pure: the
  catalog is INJECTED as the admissible set at construction.

### Validation Checkpoints (where breakage is caught before the next scope)

- **After SCOPE-01** — `internal/config/model_connections_test.go` table
  GREEN (multi-connection, unknown-kind, per-kind params, missing param,
  non-positive TTL, env-secret-not-in-`secret_keys`, no-defaults);
  `./smackerel.sh config generate` + `./smackerel.sh check` EXIT 0.
  Catches a bad registry shape or a leaked default BEFORE any vault.
- **After SCOPE-02** — `connvault/vault_test.go` (round-trip,
  never-plaintext, AAD-tamper, wrong-key) + the ephemeral-Postgres
  `tests/integration/model_connections_vault_test.go` GREEN;
  `./smackerel.sh check` EXIT 0. Catches an AEAD or master-key-lifecycle
  defect BEFORE the dispatch seam consumes a decrypted key.
- **After SCOPE-03** — the Python parity + secret-scrub + hosted-dispatch
  suites and the Go `client_provider` / `dispatch_resolver` /
  `attribution_provider` suites GREEN; `./smackerel.sh check` +
  `./smackerel.sh test unit` EXIT 0. The live hosted `e2e-api` leg is
  deferred to the home-lab dispatch (C7). Catches an Ollama-path
  regression, a secret leak, or a silent fallback BEFORE discovery.
- **After SCOPE-04** — `catalog/aggregator_test.go` +
  `catalog/canonical_test.go` + the ephemeral `model_discovery_test.go`
  GREEN; a leaf-purity check proves zero project imports into
  `modelswitch`; `./smackerel.sh check` EXIT 0. Catches a
  silently-dropped provider or a leaked import BEFORE selection wiring.
- **After SCOPE-05** — `agent/costfn_modelaware_test.go` +
  `agent/budget_preflight_modelcost_test.go` + the ephemeral
  `model_budget_enforcement_test.go` GREEN; the EXISTING 064/087 USD-budget
  tests re-run GREEN (R4 blast-radius); `./smackerel.sh check` EXIT 0.
  Catches a silent-$0 paid-model budget bypass or a 064/087 budget
  regression BEFORE the admin surface.
- **After SCOPE-06** — `api/model_connections_admin_test.go` +
  `api/model_connections_operator_gate_test.go` + the ephemeral
  `model_connections_enable_disable_test.go` GREEN; `./smackerel.sh check`
  EXIT 0. Catches an echoed secret, a false-success test, an unverified
  enable, or a non-operator reaching the surface BEFORE combined selection
  wiring. (Live hosted-provider `e2e-api` legs deferred to home-lab — C7.)
- **After SCOPE-07** — `telegram/model_command_multiprovider_test.go` +
  `api/agent_model_multiprovider_test.go` +
  `api/agent_model_parity_spec096_test.go` GREEN; `./smackerel.sh check` +
  `./smackerel.sh test unit` EXIT 0. Catches a second validator/store, a
  broken `allowed_models[]` byte-parity (R2), or an accepted
  non-tool-capable gather selection (R3) — the final in-repo gate before
  the home-lab `e2e-api` dispatch (C7).

---

## Scope Table (SCOPE-01..07)

| # | Scope | Surfaces | Covers SCN | Tests (categories) | DoD items | Status |
|---|-------|----------|------------|--------------------|-----------|--------|
| 1 | SCOPE-01 — Provider-connection registry + config SST schema (foundation) | config (yaml + `internal/config`) | A01, A04, G02 | unit (registry/validate) + config-gen | 12 | [~] In Progress (10/12 DoD; T1-1 + T1-3 environmental residuals) |
| 2 | SCOPE-02 — Encrypted credential vault + master-key lifecycle | new `connvault` pkg + migration 061 + secret-keys path | W05 | unit (AEAD) + integration (ephemeral PG) | 12 | [~] In Progress (7/12 DoD; integration leg + T1-1/T1-3 deferred/foreign residuals) |
| 3 | SCOPE-03 — Provider-aware `/ask` dispatch (credential seam) | Go `llm` + Python `chat.py`/`schemas.py` + agent attribution | A02, A03, G01, G04, G05 | unit + integration + e2e-api (deferred) | 14 | [~] In progress |
| 4 | SCOPE-04 — Discovery + unified catalog + identifier canonicalization | new `catalog` pkg + agenttool resolver boundary | D01, D04 | unit + integration + e2e-api (deferred) | 13 | [~] Unit complete; live legs deferred |
| 5 | SCOPE-05 — Model-aware CostFn + load-bearing USD budget enforcement | agent budget seam + `llm.model_costs` SST + migration 062 `model_usage_ledger` | G03 | unit + integration | 12 | [~] Unit complete; live legs deferred |
| 6 | SCOPE-06 — Operator-gated web admin connection surface (wire/test/enable) | `/v1/admin/model-connections*` + operator middleware + PWA triad | W01, W02, W03, W04 | unit + integration + e2e-api (deferred) | 13 | [~] In progress (backend + PWA triad) |
| 7 | SCOPE-07 — Combined catalog selection across Telegram + web, 088/089 parity | Telegram `/model` + `/v1/agent/model` over the existing validator/store | D02, D03, D05, G06 | unit + e2e-api (deferred) | 13 | [~] In progress (unit-GREEN; closeout + e2e-api deferred) |

Sequential gating: **SCOPE-02 cannot start until SCOPE-01 is fully done;
SCOPE-03 until SCOPE-02; SCOPE-04 until SCOPE-01 (registry) AND its
dispatch seam (SCOPE-03) exist; SCOPE-05 until SCOPE-04; SCOPE-06 until
SCOPE-02 (vault) AND SCOPE-04 (enable→catalog); SCOPE-07 until SCOPE-04
(catalog) AND SCOPE-06 (enable state).** The dependencies are real: the
vault (SCOPE-02) encrypts/decrypts credentials keyed to the SCOPE-01
registry ids; the dispatch seam (SCOPE-03) replays the SCOPE-02-decrypted
key; the catalog (SCOPE-04) aggregates the SCOPE-01-declared connections
and feeds the existing validator the dispatch seam routes through; the
model-aware CostFn (SCOPE-05) prices the SCOPE-04-resolved model over the
SCOPE-01 rate table and gates the SCOPE-03 dispatch; the admin surface
(SCOPE-06) wires the SCOPE-02 vault credential + truthful test into the
SCOPE-04 catalog; and combined selection (SCOPE-07) renders the SCOPE-04
catalog + SCOPE-06 enable state across both surfaces through the existing
088/089 validator/store. The full per-scope gate table is the [Sequential
Gate Summary](#sequential-gate-summary-all-seven-scopes) at the foot of the
file.

---

## Scope 1: SCOPE-01 — Provider-connection registry + config SST schema

**Status:** [~] In Progress (10 of 12 DoD met + evidenced; T1-1 and T1-3 are environmental/foreign residuals — see report.md → SCOPE-01)
**Scope-Kind:** config (yaml + Go loader/validator)
**Depends on:** —
**Foundation:** true (design §3 foundation #1 — the SST-declared closed
set of admissible operator-global connections; consumed by the SCOPE-02
vault, the SCOPE-03 dispatch seam, and the SCOPE-04 catalog, never
re-declared per surface)

**Intent:** Land the operator-global connection registry as
SST-source-of-truth in `config/smackerel.yaml` `llm.connections[]` — each
connection an independent slot (`id`, `kind`, non-secret `params`,
`secret_ref`, curated `models`) with **no `actor_user_id`** (single
shared graph) — plus the `llm.discovery` and `llm.model_costs` SST, and
make the registry build closed-set fail-loud, so an unknown kind, a
missing required per-kind param, a non-positive discovery bound, an
env-mode secret absent from `infrastructure.secret_keys`, or an enabled
non-ollama model with no cost rate aborts loudly with a NAMED error and
**zero** substituted default — BEFORE any vault, dispatch, or discovery
code consumes the registry.

### Surface (design §5.1, §7)

- `config/smackerel.yaml` — NEW `llm.connections[]` (each: `id`, `kind`,
  non-secret `params`, optional `secret_ref.{mode,env_key}`, curated
  `models`), `llm.discovery.{cache_ttl_ms, per_provider_timeout_ms}`
  (each REQUIRED `> 0`, NO default — G028), `llm.model_costs[]`
  (provider-qualified USD rates; REQUIRED for every enabled non-ollama
  billable model), and `infrastructure.secret_keys += LLM_PROVIDER_SECRET_MASTER_KEY`.
  Secret material is NEVER inline plaintext — db-mode secrets ride the
  vault (SCOPE-02), env-mode secrets ride the managed-secret
  `infrastructure.secret_keys` path
  ([`internal/config/secret_keys.go`](../../internal/config/secret_keys.go)
  precedent).
- `internal/config` — NEW registry loader + closed-set `Validate()`
  (kind vocabulary `ollama|anthropic|openai|azure-foundry|google|bedrock`;
  per-kind required non-secret params per design §4; env-mode
  `secret_ref.env_key ∈ secret_keys`; paid-model-has-`model_costs`;
  discovery bounds `> 0`), carried generically by `params` so the
  contract never grows a field per kind. Exercised by the NEW
  [`internal/config/model_connections_test.go`](../../internal/config/model_connections_test.go).
- No surface wiring, no DB, no network in this scope — registry shape +
  fail-loud load only.

**Covers scenarios:** SCN-096-A01, SCN-096-A04, SCN-096-G02.

### Use Cases (Gherkin) — quoted from spec.md §4

```gherkin
Scenario: SCN-096-A01 — Operator configures multiple provider connections
  Given a deployment with only a local Ollama connection
  When the operator adds an Anthropic and an OpenAI connection
  Then each connection is recorded as an independent operator-global provider connection
  And each carries the connection parameters its provider kind requires

Scenario: SCN-096-A04 — A connection carries provider-specific parameters
  Given the operator adds a Microsoft Foundry connection
  When the connection is saved
  Then it records the endpoint, API version, and deployment that the provider kind requires
  And a different provider kind records its own required parameters instead

Scenario: SCN-096-G02 — Missing required connection config aborts startup loudly
  Given the single config source omits a required provider-connection value
  When the runtime starts and config validation runs
  Then startup aborts non-zero with a named missing-config error
  And no fallback-default value is substituted
```

### Test Plan — SCOPE-01

All tests are `unit` (closed-set registry/validate; no DB, no network).
Every entry is copied verbatim from
[scenario-manifest.json](scenario-manifest.json).

**Category: unit**

| Scenario | Concrete test (file::function) | Asserts |
|----------|--------------------------------|---------|
| SCN-096-A01 | `internal/config/model_connections_test.go::TestModelConnections_MultipleOperatorGlobalConnections_Spec096` | N independent slots load with distinct `id`/`kind`/`params` and NO `actor_user_id`; the registry is operator-global + single shared graph. |
| SCN-096-A01 | `internal/config/model_connections_test.go::TestModelConnections_UnknownKindRejectedFailLoud_Spec096` (ADVERSARIAL) | An out-of-vocabulary `kind` aborts the registry build with a named error; fails if an unknown kind is ever silently accepted or defaulted. |
| SCN-096-A04 | `internal/config/model_connections_test.go::TestModelConnections_PerKindParams_AzureFoundryRichest_Spec096` | The richest per-kind param set (azure-foundry `endpoint`+`api_version`+`deployment`) loads via the generic `params` map; a kind records its own params, not another kind's. |
| SCN-096-A04 | `internal/config/model_connections_test.go::TestModelConnections_MissingRequiredPerKindParam_FailsLoud_Spec096` (ADVERSARIAL) | A connection missing a required per-kind param fails loud NAMING the offending connection + param; fails if a missing param is tolerated. |
| SCN-096-G02 | `internal/config/model_connections_test.go::TestModelConnections_DiscoveryTtlNonPositive_AbortsNamed_Spec096` (ADVERSARIAL) | A `cache_ttl_ms` or `per_provider_timeout_ms` `<= 0` aborts with a NAMED error and NO substituted default. |
| SCN-096-G02 | `internal/config/model_connections_test.go::TestModelConnections_EnvModeSecretNotInSecretKeys_AbortsNamed_Spec096` (ADVERSARIAL) | An `env`-mode `secret_ref.env_key` absent from `infrastructure.secret_keys` aborts naming the missing key; fails if an undeclared env secret is tolerated. |
| SCN-096-G02 | `internal/config/model_connections_test.go::TestModelConnections_NoDefaultsFailLoud_Spec096` (ADVERSARIAL) | An enabled non-ollama model with no `llm.model_costs` entry (and any other required-but-absent registry field) aborts fail-loud; no `${VAR:-default}` / `getenv(k,default)` / `unwrap_or` path substitutes a value. |

> Live-stack note: none of SCOPE-01's tests touch a live system; they are
> pure config-load/validate unit tests with NO request interception.

### Definition of Done — SCOPE-01 (all unchecked — implementation pending)

**Tier-1 (universal):**

- [ ] D01-T1-1 — `bash .github/bubbles/scripts/artifact-lint.sh specs/096-multi-provider-model-connections` clean. → Evidence: [report.md](report.md) → SCOPE-01. **RESIDUAL (not a code gap):** artifact-lint's only failure is the missing foreign `uservalidation.md` (owned by `bubbles.plan`, not `bubbles.implement`); all implement-owned lint + anti-fabrication checks pass (E7). Routed to `bubbles.plan`.
- [x] D01-T1-2 — `./smackerel.sh check` EXIT 0 (build + vet + config-sync + scenario-lint). → Evidence: report.md → SCOPE-01.
- [ ] D01-T1-3 — `./smackerel.sh format --check` EXIT 0. → Evidence: report.md → SCOPE-01. **RESIDUAL (not a code gap):** every changed file is gofmt-clean (`gofmt -l` empty, E6); the global command is blocked solely by a pre-existing untracked FOREIGN file (`internal/connector/qfdecisions/chaos_hardening_test.go`, another session's WIP) that must not be modified.
- [x] D01-T1-4 — Every evidence block in report.md → SCOPE-01 is REAL terminal output (anti-fabrication); no synthesized results. → Evidence: report.md → SCOPE-01.
- [x] D01-T1-5 — 088/089 do-not-amend boundary respected: zero behavioural change to `modelswitch`/`modelpref`, the Telegram `/model` picker, or `/v1/agent/model`; this scope only adds the SST registry. → Evidence: report.md → Change Manifest (isolated diff).

**Tier-2 (role-specific: config + fail-loud SST / G028):**

- [x] D01-T2-1 — `llm.connections[]` is SST source-of-truth: N operator-global slots (`id`, `kind`, non-secret `params`, `secret_ref`, curated `models`), NO `actor_user_id`; `./smackerel.sh config generate` EXIT 0 with the registry + `LLM_PROVIDER_SECRET_MASTER_KEY` present in the generated dev + test env. → Evidence: report.md → SCOPE-01.
- [x] D01-T2-2 — **NO-DEFAULTS / fail-loud (G028, `smackerel-no-defaults`):** `grep` proves no `${VAR:-default}` / `${VAR-default}` / `getenv(k, default)` / `os.getenv(k, default)` / `unwrap_or` introduced for any registry/discovery/cost value; `discovery.cache_ttl_ms` + `per_provider_timeout_ms` are REQUIRED `> 0`; a missing required value aborts named. → Evidence: report.md → SCOPE-01 (config generate + grep).
- [x] D01-T2-3 — Closed-set kind vocabulary (`ollama|anthropic|openai|azure-foundry|google|bedrock`) is enforced; an unknown kind aborts (`TestModelConnections_UnknownKindRejectedFailLoud_Spec096`). → Evidence: report.md → SCOPE-01.
- [x] D01-T2-4 — Per-kind required non-secret params (design §4) are validated generically through `params`; a missing required param fails loud naming the connection + param (`TestModelConnections_MissingRequiredPerKindParam_FailsLoud_Spec096`); the contract does not grow a typed field per kind. → Evidence: report.md → SCOPE-01.
- [x] D01-T2-5 — `env`-mode `secret_ref.env_key` must exist in `infrastructure.secret_keys` (`TestModelConnections_EnvModeSecretNotInSecretKeys_AbortsNamed_Spec096`); db-mode and ollama slots carry NO inline plaintext secret. → Evidence: report.md → SCOPE-01.
- [x] D01-T2-6 — Each SCN-096-G02 adversarial test is non-tautological: it fails if the loader ever substitutes a default or tolerates the missing/invalid value (no bailout early-returns). → Evidence: report.md → SCOPE-01 RED-before (neutralised validator).
- [x] D01-T2-7 — All seven SCOPE-01 unit tests pass with NO skips/ignores; evidence is the real `./smackerel.sh test unit --go` (or `--go-run` for the spec-096 set) output. → Evidence: report.md → SCOPE-01 (test run).

---

## Scope 2: SCOPE-02 — Encrypted credential vault + master-key lifecycle

**Status:** in_progress (7 of 12 DoD items met + evidenced; the residuals are the integration/migrate leg [T2-1/T2-6/T2-7, deferred to a clean-stack run] + two foreign/closeout items [T1-1 `uservalidation.md`, T1-3 a foreign untracked file] — see DoD)
**Scope-Kind:** code (new `connvault` pkg) + DB (migration 061)
**Depends on:** SCOPE-01
**Foundation:** false (the design §3 foundation #2 reversible secret store
— keyed 1:1 to the SCOPE-01 registry db-mode ids; consumed by the SCOPE-03
dispatch seam and the SCOPE-06 admin surface)

**Intent:** Land the reversible, authenticated, encrypted-at-rest
credential vault: migration 061 `model_provider_connections` (operator-
global, NO `actor_user_id`) and a `connvault.SecretVault` that
AES-256-GCM-encrypts the secret bundle under the in-core master key with a
per-record 96-bit nonce + `AAD(connection_id|kind|key_version)`, persists
only ciphertext + nonce + `key_version` + last-4 redaction, decrypts
in-core only, and NEVER returns plaintext to any surface — with a
fail-loud master-key load (required iff a db-mode hosted connection is
declared) and a documented re-encrypt-all rotation. The credential is
**reversible** (replayable to `Authorization: Bearer <key>`), NEVER
one-way hashed.

### Surface (design §5.2, §3 foundation #2)

- `internal/db` migration **061** `model_provider_connections` (design
  §5.2): `connection_id` PK (1:1 to the SST registry id),
  `provider_kind`, `enabled` (app-written, NO DB-side default — G028),
  `secret_ciphertext`/`secret_nonce`/`secret_key_version` (NULL for
  ollama; non-NULL when an enabled hosted db-mode row),
  `secret_redaction` (last-4 display hint), `last_tested_at` /
  `last_test_outcome` / `last_test_detail` (typed, NEVER the secret),
  `created_at` / `updated_at` (app-written).
- `internal/assistant/openknowledge/connvault` (NEW pkg):
  `SecretVault.Encrypt(...)` / `Decrypt(...)` — AES-256-GCM AEAD,
  per-record random 96-bit nonce, `AAD = connection_id|kind|key_version`,
  last-4 redaction computed at save time; in-core master-key load
  (fail-loud, required iff a db-mode hosted connection is declared in the
  SCOPE-01 registry; managed-secret `LLM_PROVIDER_SECRET_MASTER_KEY` via
  the `infrastructure.secret_keys` three-mirror path); documented
  rotation = bump `key_version` + re-encrypt-all. The master key NEVER
  leaves the Go core; the bundle is the kind's secret fields serialized
  and encrypted as ONE blob (design §5.2 invariants).
- Reversible-secret class (design Design Brief): the precedent is
  `CARD_REWARDS_GCAL_CREDENTIALS`
  ([`internal/cardrewards/gcal_client.go`](../../internal/cardrewards/gcal_client.go)),
  NOT the verifier class `AUTH_AT_REST_HASHING_KEY` — the vault MUST be
  decryptable, never argon2id-hashed.

**Covers scenarios:** SCN-096-W05.

### Use Cases (Gherkin) — quoted from spec.md §4

```gherkin
Scenario: SCN-096-W05 — Credentials are stored protected and never returned in plaintext
  Given the operator has saved a provider connection with a credential
  When the operator re-opens the connection in the web UI
  Then the credential is shown only as a redacted/managed value, never in plaintext
  And the stored credential is protected at rest
```

### Test Plan — SCOPE-02

`unit` proves the AEAD contract in isolation; `integration` proves the
persist/round-trip against an **ephemeral** Postgres with a **synthetic**
test master key. Entries are copied verbatim from
[scenario-manifest.json](scenario-manifest.json).

**Category: unit**

| Scenario | Concrete test (file::function) | Asserts |
|----------|--------------------------------|---------|
| SCN-096-W05 | `internal/assistant/openknowledge/connvault/vault_test.go::TestSecretVault_EncryptDecrypt_RoundTrip_Spec096` | Encrypt → Decrypt under the same master key + AAD returns the original bundle byte-for-byte; the credential is reversible (not hashed). |
| SCN-096-W05 | `internal/assistant/openknowledge/connvault/vault_test.go::TestSecretVault_NeverReturnsPlaintext_RedactionLast4_Spec096` (ADVERSARIAL) | The persisted/returned record exposes only `secret_present` + last-4 redaction + `last_tested_*`; the cleartext secret never appears in any returned struct/field. Fails if plaintext is ever surfaced. |
| SCN-096-W05 | `internal/assistant/openknowledge/connvault/vault_test.go::TestSecretVault_AADTamperRejected_Spec096` (ADVERSARIAL) | A decrypt with a tampered `AAD(connection_id\|kind\|key_version)` is rejected (authenticated AEAD); fails if a tampered AAD ever decrypts. |
| SCN-096-W05 | `internal/assistant/openknowledge/connvault/vault_test.go::TestSecretVault_WrongKeyRejected_Spec096` (ADVERSARIAL) | A decrypt under the wrong master key is rejected; fails if a wrong-key ciphertext ever yields plaintext. |

**Category: integration (ephemeral Postgres + synthetic master key)**

| Scenario | Concrete test (file::function) | Asserts |
|----------|--------------------------------|---------|
| SCN-096-W05 | `tests/integration/model_connections_vault_test.go::TestVault_PersistRoundTripTestMasterKey_Spec096` | Against a live ephemeral Postgres, Encrypt → persist to `model_provider_connections` → re-read → Decrypt returns the original bundle; the row stores only ciphertext+nonce+`key_version`+redaction; uses a synthetic test master key, NEVER a real provider secret. Hits the REAL DB — NO query interception/mocking. |

> Live-stack note: the integration test runs against the disposable test
> stack's ephemeral Postgres with synthetic secrets only (test isolation +
> env-pollution policy); it MUST NOT touch the persistent dev store or any
> real credential.

### Definition of Done — SCOPE-02 (all unchecked — implementation pending)

**Tier-1 (universal):**

- [ ] D02-T1-1 — `bash .github/bubbles/scripts/artifact-lint.sh specs/096-multi-provider-model-connections` clean. → Evidence: report.md → SCOPE-02. **DEFERRED (not a code gap):** blocked by the absent `uservalidation.md` — a user-acceptance/closeout artifact authored at validation time (owned by `bubbles.plan`/validation, not `bubbles.implement`).
- [x] D02-T1-2 — `./smackerel.sh check` EXIT 0. → Evidence: report.md → SCOPE-02.
- [ ] D02-T1-3 — `./smackerel.sh format --check` EXIT 0. → Evidence: report.md → SCOPE-02. **DEFERRED (not a code gap):** the global format gate fails only on a pre-existing UNTRACKED FOREIGN file from a concurrent session (`internal/connector/qfdecisions/chaos_hardening_test.go`); every SCOPE-02 file is gofmt-clean.
- [x] D02-T1-4 — Every evidence block in report.md → SCOPE-02 is REAL terminal output (anti-fabrication). → Evidence: report.md → SCOPE-02.
- [x] D02-T1-5 — 088/089 do-not-amend boundary respected: the vault is additive; no `modelswitch`/`modelpref`/picker behaviour changes. → Evidence: report.md → Change Manifest.

**Tier-2 (role-specific: migration + secret-safety + G028):**

- [ ] D02-T2-1 — **Migration discipline:** migration 061 `model_provider_connections` applies cleanly forward, is operator-global (NO `actor_user_id`), uses NO DB-side defaults for `enabled`/`updated_at` (app-written, G028), and `./smackerel.sh` DB-migrate path is green on the ephemeral test DB. → Evidence: report.md → SCOPE-02 (migrate output). **DEFERRED:** the integration/migrate leg was not run this turn to avoid disrupting concurrent sessions on the shared test stack (OOM/contention risk); `061_model_provider_connections.sql` exists and is idempotently applied by the integration test on a clean stack.
- [x] D02-T2-2 — **Secret-safety (binding):** no plaintext credential is ever logged, traced, or returned; `secret_redaction` is last-4 only; the master key NEVER leaves the Go core (the sidecar receives nothing in this scope). → Evidence: report.md → SCOPE-02 (grep for log/return of the bundle + `TestSecretVault_NeverReturnsPlaintext_RedactionLast4_Spec096`).
- [x] D02-T2-3 — **Reversible, not hashed:** the vault is AES-256-GCM (decryptable); a grep proves no argon2id/one-way hashing of the credential bundle; the round-trip test recovers the exact secret. → Evidence: report.md → SCOPE-02.
- [x] D02-T2-4 — **Authenticated AEAD adversarial:** `TestSecretVault_AADTamperRejected_Spec096` + `TestSecretVault_WrongKeyRejected_Spec096` are non-tautological — each fails if a tampered-AAD or wrong-key ciphertext ever decrypts. → Evidence: report.md → SCOPE-02.
- [x] D02-T2-5 — **Master-key fail-loud (G028):** `LLM_PROVIDER_SECRET_MASTER_KEY` is a managed secret loaded fail-loud when a db-mode hosted connection is declared; an absent/empty master key aborts named (no default key, no skip-encryption fallback); rotation (bump `key_version` + re-encrypt-all) is documented. → Evidence: report.md → SCOPE-02.
- [ ] D02-T2-6 — **Test isolation:** unit + integration tests use synthetic secrets and the ephemeral test Postgres only; no real provider credential and no persistent-store write. → Evidence: report.md → SCOPE-02. **PARTIAL:** the unit leg uses synthetic secrets only (met); the integration leg — correctly isolated (unique timestamped `connection_id`, `t.Cleanup` row delete, synthetic 32-byte key) — is DEFERRED to a clean-stack run.
- [ ] D02-T2-7 — All five SCOPE-02 tests pass with NO skips/ignores; evidence is the real unit + integration run output. → Evidence: report.md → SCOPE-02 (test run). **PARTIAL:** 6/6 SCOPE-02 unit tests pass (real run, report.md → SCOPE-02); the 1 integration test (`TestVault_PersistRoundTripTestMasterKey_Spec096`) is DEFERRED to a clean-stack run (it skips without `DATABASE_URL` by design).

---

## Scope 3: SCOPE-03 — Provider-aware `/ask` dispatch (credential seam)

**Status:** [~] In progress (unit + resolver + attribution shipped & GREEN; live-stack integration/e2e-api deferred — see report.md → SCOPE-03)
**Scope-Kind:** code (Go `llm` contract + Python `chat.py`/`schemas.py` + agent attribution)
**Depends on:** SCOPE-02
**Foundation:** false (the credential seam that replays the SCOPE-02
vault's decrypted key to the sidecar; preserves the 088/089 fork +
precedence + attribution provider-agnostically)

**Intent:** Make the open-knowledge `/ask` dispatch provider-aware while
keeping the no-override / Ollama-only path **byte-for-byte** today's
behaviour. Extend the Go `llm.ChatRequest` and Python `ChatRequest`
(`extra="forbid"`) additively with `provider`/`api_base`/`api_key`/
`provider_params`; redesign `_dispatch_live` to fork ollama (today's
kwargs exactly, NO `api_key`) vs hosted (compose `<kind>/<backend-id>` +
`api_base` + the per-request cleartext key the Go core decrypted); emit a
typed `llm_misconfigured` on a missing key with NO Ollama substitution;
carry the provider-qualified model into the 089 `ModelAttribution`; and
scrub any `api_key` substring out of every log line, span, and error
body.

### Surface (design §6.2, §2.4, §2.5)

- [`internal/assistant/openknowledge/llm/client.go`](../../internal/assistant/openknowledge/llm/client.go)
  — `ChatRequest` extended additively: `Provider string`, `APIBase
  *string`, `APIKey *string`, `ProviderParams map[string]any` (the Ollama
  no-override path leaves all four zero/nil). Exercised by NEW
  [`internal/assistant/openknowledge/llm/client_provider_test.go`](../../internal/assistant/openknowledge/llm/client_provider_test.go).
- [`ml/app/schemas.py`](../../ml/app/schemas.py) `ChatRequest`
  (`extra="forbid"`) — the same four fields declared so the additive
  request validates.
- [`ml/app/routes/chat.py`](../../ml/app/routes/chat.py) `_dispatch_live`
  — redesigned to the `synthesis.py` provider fork shape (design §6.2):
  `if req.provider == "ollama"` builds `ollama_chat/<model>` + `api_base`
  with NO `api_key` (litellm kwargs IDENTICAL to today); `else` composes
  `<kind>/<backend-id>` + `api_base` + `api_key` + `provider_params`. A
  missing required `api_key` on the hosted branch returns a typed
  `llm_misconfigured` (NO Ollama substitution). The error `detail` that
  crosses the wire is built from `type(e).__name__` + provider + model
  with any `api_key` substring scrubbed; `_dispatch_live` MUST NOT log
  `api_key`.
- NEW `internal/assistant/openknowledge/llm/dispatch_resolver.go` —
  `DispatchResolver` refuses a target whose connection is not
  effective-enabled with a typed reason rather than a silent local
  fallback. Exercised by NEW
  [`internal/assistant/openknowledge/llm/dispatch_resolver_test.go`](../../internal/assistant/openknowledge/llm/dispatch_resolver_test.go).
- `internal/assistant/openknowledge/agent` — the dispatch path carries
  the provider-qualified model (e.g. `anthropic/claude-3-5-sonnet`) +
  selection source into the EXISTING 089 `ModelAttribution` /
  `TurnResult.Model` contract (provider-qualified, never coerced to a bare
  or Ollama name). Exercised by NEW
  [`internal/assistant/openknowledge/agent/attribution_provider_test.go`](../../internal/assistant/openknowledge/agent/attribution_provider_test.go).

**Covers scenarios:** SCN-096-A02, SCN-096-A03, SCN-096-G01, SCN-096-G04,
SCN-096-G05.

### Use Cases (Gherkin) — quoted from spec.md §4

```gherkin
Scenario: SCN-096-A02 — The /ask agent dispatches to the selected model's provider
  Given a user has selected a hosted model from a connected provider
  When the open-knowledge agent serves the user's question
  Then the request is dispatched to that model's provider using that provider's credentials
  And the answer is produced by the selected hosted model, not by Ollama

Scenario: SCN-096-A03 — No selection keeps today's Ollama behavior byte-for-byte
  Given a deployment with only the local Ollama connection and no model override
  When the open-knowledge agent serves a question
  Then dispatch follows the existing Ollama path unchanged
  And no provider credential or routing change is observable

Scenario: SCN-096-G01 — A misconfigured connection fails loud and never falls back to Ollama
  Given a provider connection with an invalid or missing required value
  When the runtime starts or the connection is exercised
  Then it fails loud with a named error identifying the connection
  And no request is silently re-routed to Ollama in its place

Scenario: SCN-096-G04 — Every answer is attributed to the model that produced it
  Given a user asks a question answered by a selected hosted model
  When the answer is returned
  Then it is attributed to the provider and model that generated it

Scenario: SCN-096-G05 — Provider secrets never leak to users
  Given a user uses the model picker and reads an attributed answer
  When the user inspects the available surfaces
  Then no provider credential is exposed in the catalog, selection, attribution, or logs
```

### Test Plan — SCOPE-03

`unit` proves the parity, additive-contract, typed-error, attribution, and
secret-scrub mechanisms; `integration` proves the Go→sidecar hosted seam;
the live hosted answer is an `e2e-api` deferred to the home-lab dispatch
(C7). Entries are copied verbatim from
[scenario-manifest.json](scenario-manifest.json).

**Category: unit**

| Scenario | Concrete test (file::function) | Asserts |
|----------|--------------------------------|---------|
| SCN-096-A03 | `ml/tests/test_chat_dispatch_parity_spec096.py::test_dispatch_live_ollama_kwargs_byte_for_byte` (ADVERSARIAL) | For a fixed Ollama request the `litellm.acompletion(**kwargs)` dict equals today's `_dispatch_live` byte-for-byte; fails if any provider field leaks into the Ollama path (the 088 `IsZero()` baseline guarantee). |
| SCN-096-A03 | `ml/tests/test_chat_dispatch_parity_spec096.py::test_ollama_branch_carries_no_api_key` (ADVERSARIAL) | The ollama branch carries NO `api_key`; fails if a key is ever attached to an Ollama dispatch. |
| SCN-096-A02 | `ml/tests/test_chat_dispatch_hosted_spec096.py::test_dispatch_live_hosted_composes_model_and_api_key` | The hosted branch composes `<kind>/<backend-id>` + `api_base` + `api_key` + `provider_params` and routes to the selected hosted model. |
| SCN-096-A02 | `ml/tests/test_chat_dispatch_hosted_spec096.py::test_chatrequest_extra_forbid_still_holds` (ADVERSARIAL) | The Python `ChatRequest` stays `extra="forbid"`: the four new fields validate, an undeclared field still 422s. |
| SCN-096-A02 | `internal/assistant/openknowledge/llm/client_provider_test.go::TestChatRequest_ProviderFieldsAdditive_Spec096` | The Go `ChatRequest` gains `Provider`/`APIBase`/`APIKey`/`ProviderParams` additively; a zero-value request serializes byte-for-byte the pre-096 wire shape. |
| SCN-096-G01 | `ml/tests/test_chat_dispatch_hosted_spec096.py::test_hosted_missing_api_key_typed_error_no_ollama_substitution` (ADVERSARIAL) | A hosted dispatch with an absent required `api_key` returns a typed `llm_misconfigured` and NEVER substitutes Ollama; fails if the path falls back to a local model. |
| SCN-096-G01 | `internal/assistant/openknowledge/llm/dispatch_resolver_test.go::TestDispatchResolver_MisconfiguredConnection_NeverFallsBackToOllama_Spec096` (ADVERSARIAL) | The resolver refuses a not-effective-enabled target with a typed reason; fails if it ever silently resolves to Ollama. |
| SCN-096-G04 | `internal/assistant/openknowledge/agent/attribution_provider_test.go::TestAttribution_ProviderQualified_Spec096` | `ModelAttribution`/`TurnResult.Model` is provider-qualified (`anthropic/claude-3-5-sonnet`) and never coerced to a bare or Ollama name; two providers' answers are distinguishable by attribution. |
| SCN-096-G05 | `ml/tests/test_chat_dispatch_secret_scrub_spec096.py::test_api_key_never_logged` (ADVERSARIAL) | No log line emitted by `_dispatch_live` contains the cleartext `api_key`; fails if the key appears in any log. |
| SCN-096-G05 | `ml/tests/test_chat_dispatch_secret_scrub_spec096.py::test_error_detail_scrubs_api_key_substring` (ADVERSARIAL) | A litellm exception that embeds the key in a URL/header is scrubbed: the wire `detail` is `type(e).__name__` + provider + model with the key substring removed. |
| SCN-096-G05 | `internal/assistant/openknowledge/llm/dispatch_resolver_test.go::TestDispatch_SecretNeverInLogsOrErrors_Spec096` (ADVERSARIAL) | The Go side never places the decrypted key into a log, span, or error body. |

**Category: integration (ephemeral stack)**

| Scenario | Concrete test (file::function) | Asserts |
|----------|--------------------------------|---------|
| SCN-096-A02 | `tests/integration/openknowledge_hosted_dispatch_test.go::TestAsk_HostedConnection_ProviderAware_Spec096` | Against the live ephemeral Go core + sidecar, a hosted-connection `/ask` takes the provider-aware seam end-to-end (Go decrypts → `/llm/chat` carries provider fields → sidecar hosted branch); hits the REAL services — NO request interception. |

**Category: e2e-api (deferred to home-lab `bubbles.devops` dispatch — C7)**

| Scenario | Concrete test (file::function) | Asserts |
|----------|--------------------------------|---------|
| SCN-096-G04 | `tests/e2e/agent/openknowledge_e2e_test.go::TestAsk_HostedAnswer_AttributedToProviderModel_Spec096` | The live `/ask` against a real hosted provider returns a grounded answer attributed to the provider-qualified model; runs in the home-lab dispatch (real credentials + reachability), NOT in-repo. |

> Live-stack note: every `integration`/`e2e-api` row hits the REAL running
> system; the Python parity/scrub tests construct the kwargs/error from the
> real `_dispatch_live` code path WITHOUT intercepting litellm's network
> call (they assert the composed arguments, not a mocked response) — they
> are correctly classified `unit`.

### Definition of Done — SCOPE-03 (implementation in progress — see report.md → SCOPE-03)

**Tier-1 (universal):**

- [ ] D03-T1-1 — `bash .github/bubbles/scripts/artifact-lint.sh specs/096-multi-provider-model-connections` clean. → Evidence: report.md → SCOPE-03. _(Deferred: blocked by absent foreign `uservalidation.md`, owned by `bubbles.plan` — same closeout caveat as SCOPE-01/02; not a SCOPE-03 code gap.)_
- [ ] D03-T1-2 — `./smackerel.sh check` EXIT 0. → Evidence: report.md → SCOPE-03. _(Deferred: the orchestrator runs `check` post-implementation to bound this turn; SCOPE-03 Go builds + Python imports are clean.)_
- [ ] D03-T1-3 — `./smackerel.sh format --check` EXIT 0. → Evidence: report.md → SCOPE-03. _(Deferred: global gate run at closeout; same foreign-untracked-file caveat as SCOPE-01/02.)_
- [x] D03-T1-4 — Every evidence block in report.md → SCOPE-03 is REAL terminal output (anti-fabrication). → Evidence: report.md → SCOPE-03.

**Tier-2 (role-specific: 088/089 parity + secret-safety + fail-loud):**

- [x] D03-T2-1 — **088/089 PARITY (binding):** the no-override Ollama dispatch is byte-for-byte today's `_dispatch_live` — `test_dispatch_live_ollama_kwargs_byte_for_byte` + `test_ollama_branch_carries_no_api_key` are ADVERSARIAL and fail if any provider field leaks into the Ollama path; the 088/089 gather-vs-synthesis fork + per-request>sticky>default precedence are preserved (no behavioural change to the existing path). → Evidence: report.md → SCOPE-03 (parity test output).
- [x] D03-T2-2 — **Additive contract:** the Go + Python `ChatRequest` gain only the four new fields; Python stays `extra="forbid"` (`test_chatrequest_extra_forbid_still_holds`); a zero-value Go request is wire-identical to pre-096 (`TestChatRequest_ProviderFieldsAdditive_Spec096`). → Evidence: report.md → SCOPE-03.
- [x] D03-T2-3 — **Secret-safety adversarial (binding):** the cleartext `api_key` never appears in any log line, span, or HTTP error body — proven by `test_api_key_never_logged`, `test_error_detail_scrubs_api_key_substring`, and `TestDispatch_SecretNeverInLogsOrErrors_Spec096`. → Evidence: report.md → SCOPE-03 (scrub test output).
- [x] D03-T2-4 — **Fail-loud, never-fallback-to-Ollama adversarial:** a missing required key returns typed `llm_misconfigured` (`test_hosted_missing_api_key_typed_error_no_ollama_substitution`) and the resolver refuses a not-effective-enabled target (`TestDispatchResolver_MisconfiguredConnection_NeverFallsBackToOllama_Spec096`) — both fail if Ollama is ever silently substituted. → Evidence: report.md → SCOPE-03.
- [x] D03-T2-5 — **Provider-qualified attribution:** `ModelAttribution`/`TurnResult.Model` is the provider-qualified id (`TestAttribution_ProviderQualified_Spec096`); two providers' answers are distinguishable. → Evidence: report.md → SCOPE-03.
- [x] D03-T2-6 — Each adversarial test is non-tautological with a captured RED-before (e.g. a build that leaks a provider field into the Ollama kwargs, or that logs the key, would fail it); no bailout early-returns. → Evidence: report.md → SCOPE-03 RED-before.
- [ ] D03-T2-7 — All `unit` + `integration` SCOPE-03 tests pass with NO skips; the live hosted `e2e-api` row (`TestAsk_HostedAnswer_AttributedToProviderModel_Spec096`) is explicitly handed to the home-lab `bubbles.devops` dispatch (C7) and NOT marked passing from dev. → Evidence: report.md → SCOPE-03 (test run + deferral note). _(Unit leg done with NO skips — E1–E3; the `integration` (`TestAsk_HostedConnection_ProviderAware_Spec096`) + `e2e-api` rows are DEFERRED to a clean-stack `bubbles.devops` dispatch, not marked passing from dev.)_

---

## Scope 4: SCOPE-04 — Discovery + unified catalog + identifier canonicalization

**Status:** [~] In progress — unit leg complete (7 of 11 DoD met + evidenced, T2-7 partial); live `integration`/`e2e-api` legs + closeout gates (`check`/`format`/`artifact-lint`) deferred. See report.md → SCOPE-04.
**Scope-Kind:** code (new `catalog` pkg + per-kind discovery adapters + agenttool resolver boundary)
**Depends on:** SCOPE-01
**Foundation:** false (design §3 foundation #3 — aggregates the SCOPE-01
registry into the provider-qualified catalog injected as the EXISTING
`modelswitch` validator's admissible set; the dispatch seam it feeds is
SCOPE-03)

**Intent:** Aggregate every effective-enabled connection's models into
ONE provider-qualified `ModelCatalog` and make it the injected admissible
set for the EXISTING `modelswitch` validator + `modelpref` store
(one-validator/one-store, 088/089 invariant). Per-kind discovery adapters
(Ollama live `GET /api/tags`; hosted SST-curated) run in parallel bounded
by the SST `cache_ttl_ms` + `per_provider_timeout_ms`; a slow/unreachable/
auth-failed provider degrades gracefully (its models absent) and ALWAYS
emits a typed `ProviderDiscoveryStatus` — the reachable subset is always
served and a provider is NEVER silently dropped. `<kind>/<backend-id>`
canonicalization (split on the FIRST `/`) happens at the agenttool
resolver boundary so the `modelswitch` leaf stays a PURE stdlib leaf
(admissible set INJECTED at construction, not imported); an off-catalog id
yields the SAME `modelswitch.Rejection` shape with NO store write and NO
dispatch.

### Surface (design §6.3, §10, §3 foundation #3)

- `internal/assistant/openknowledge/catalog` (NEW pkg):
  - `aggregator.go` — `CatalogAggregator.GetCatalog(ctx) → (ModelCatalog,
    []ProviderDiscoveryStatus)`; in-memory last-good catalog rebuilt on
    `cache_ttl_ms` expiry (stale-while-revalidate); each provider's
    discovery bounded by `per_provider_timeout_ms`, run in parallel;
    `ModelDescriptor{ id (<kind>/<backend-id>), connection_id, kind,
    tool_capable, vision, context_window }`; typed
    `ProviderDiscoveryStatus{ connection_id, kind, state
    (ok|unreachable|timeout|auth_failed|disabled), model_count, detail }`
    ALWAYS one per effective-enabled connection (design §6.3). Exercised
    by NEW
    [`internal/assistant/openknowledge/catalog/aggregator_test.go`](../../internal/assistant/openknowledge/catalog/aggregator_test.go).
  - `canonical.go` — `Canonicalize(raw, installed)` splits on the FIRST
    `/`, normalizes a bare Ollama id to `ollama/<id>` iff installed, then
    the validator string-compares against the provider-qualified catalog;
    an off-catalog id → the same `modelswitch.Rejection` (no store write,
    no dispatch). Exercised by NEW
    [`internal/assistant/openknowledge/catalog/canonical_test.go`](../../internal/assistant/openknowledge/catalog/canonical_test.go).
- Per-kind discovery adapters (the foundation §3 adapter contract
  `Discover`): Ollama live `GET <base_url>/api/tags`; hosted kinds serve
  the SST-curated `models[]` from the SCOPE-01 registry.
- The agenttool resolver boundary
  ([`internal/assistant/openknowledge/agenttool`](../../internal/assistant/openknowledge/agenttool))
  injects the catalog set into the EXISTING `modelswitch.Allowlist` —
  [`modelswitch`](../../internal/assistant/openknowledge/modelswitch)
  remains import-pure (NO project imports), preserving 088's leaf-purity
  invariant.

**Covers scenarios:** SCN-096-D01, SCN-096-D04.

### Use Cases (Gherkin) — quoted from spec.md §4

```gherkin
Scenario: SCN-096-D01 — Discovery aggregates local and hosted models into one list
  Given Ollama-installed models and one or more enabled hosted connections
  When the model catalog is built
  Then it contains every Ollama-installed model and every enabled provider's models
  And each entry has a provider-qualified identifier and its capabilities

Scenario: SCN-096-D04 — Selecting a model absent from the catalog is refused
  Given a user attempts to select a model not present in the current catalog
  When the selection is submitted on any surface
  Then it is refused with a typed reason
  And no dispatch occurs against an unknown model
```

### Test Plan — SCOPE-04

`unit` proves aggregation, graceful degradation, canonicalization, and
off-catalog rejection in isolation; `integration` proves one-provider-down
against the live ephemeral stack; the live multi-provider catalog is an
`e2e-api` deferred to the home-lab dispatch (C7). Entries are copied
verbatim from [scenario-manifest.json](scenario-manifest.json).

**Category: unit**

| Scenario | Concrete test (file::function) | Asserts |
|----------|--------------------------------|---------|
| SCN-096-D01 | `internal/assistant/openknowledge/catalog/aggregator_test.go::TestCatalogAggregator_AggregatesOllamaAndHostedProviderQualified_Spec096` | Ollama-installed + hosted-curated models merge into ONE provider-qualified catalog; each descriptor carries `id`/`connection_id`/`kind`/capabilities. |
| SCN-096-D01 | `internal/assistant/openknowledge/catalog/aggregator_test.go::TestCatalogAggregator_GracefulDegradation_TypedStatusNeverDropped_Spec096` (ADVERSARIAL) | A timed-out/unreachable/auth-failed provider yields a typed `ProviderDiscoveryStatus` and its models are absent while the reachable subset still serves; fails if a provider is ever silently dropped with no status. |
| SCN-096-D04 | `internal/assistant/openknowledge/catalog/canonical_test.go::TestCanonicalize_SplitOnFirstSlash_RoundTrip_Spec096` | `<kind>/<backend-id>` splits on the FIRST `/` and round-trips (a backend id containing `/` is preserved). |
| SCN-096-D04 | `internal/assistant/openknowledge/catalog/canonical_test.go::TestCanonicalize_BareOllamaIdNormalized_Spec096` | A bare Ollama id normalizes to `ollama/<id>` iff installed, then validates against the provider-qualified catalog. |
| SCN-096-D04 | `internal/assistant/openknowledge/catalog/canonical_test.go::TestValidate_OffCatalogRefused_TypedRejection_Spec096` (ADVERSARIAL) | An off-catalog id yields the SAME `modelswitch.Rejection` shape with NO store write and NO dispatch; fails if an off-catalog id is ever accepted or written. |

**Category: integration (ephemeral stack)**

| Scenario | Concrete test (file::function) | Asserts |
|----------|--------------------------------|---------|
| SCN-096-D01 | `tests/integration/model_discovery_test.go::TestDiscovery_OneProviderDown_CatalogStillServes_Spec096` | Against the live ephemeral stack, with one provider deliberately down, the catalog still serves the reachable subset and the down provider emits a typed status; hits the REAL aggregator/services — NO request interception. |

**Category: e2e-api (deferred to home-lab `bubbles.devops` dispatch — C7)**

> SCN-096-D04's `requiredTestType` includes `e2e-api`; the live
> off-catalog rejection over the real `/ask` selection path runs in the
> home-lab dispatch (model/Ollama-dependent), surfaced through the SCOPE-07
> selection-surface e2e (`tests/e2e/agent/openknowledge_e2e_test.go`). It
> is NOT marked passing from dev (C7).

> Live-stack note: the `integration` row hits the REAL aggregator +
> services; the unit rows construct catalogs/descriptors directly and
> assert the pure canonicalization/rejection logic WITHOUT network
> interception — correctly classified `unit`.

### Definition of Done — SCOPE-04 (all unchecked — implementation pending)

**Tier-1 (universal):**

- [ ] D04-T1-1 — `bash .github/bubbles/scripts/artifact-lint.sh specs/096-multi-provider-model-connections` clean. → Evidence: report.md → SCOPE-04. _(DEFERRED — blocked by absent `uservalidation.md`, a closeout artifact owned by `bubbles.plan`; not a SCOPE-04 code gap. Same caveat as SCOPE-01/02/03.)_
- [ ] D04-T1-2 — `./smackerel.sh check` EXIT 0. → Evidence: report.md → SCOPE-04. _(DEFERRED to the orchestrator post-implementation; SCOPE-04 compiles clean under `go test ./...` — E1 `finished OK`.)_
- [ ] D04-T1-3 — `./smackerel.sh format --check` EXIT 0. → Evidence: report.md → SCOPE-04. _(DEFERRED — global gate run at closeout by the orchestrator; same foreign-untracked-file caveat as SCOPE-01/02/03.)_
- [x] D04-T1-4 — Every evidence block in report.md → SCOPE-04 is REAL terminal output (anti-fabrication). → Evidence: report.md → SCOPE-04. _(E1 GREEN `UNIT_EXIT=0`, E2 RED-before `RED_EXIT=1`, E3 import greps — all captured.)_

**Tier-2 (role-specific: graceful degradation + leaf purity + G028 + canonicalization):**

- [x] D04-T2-1 — **Graceful-degradation adversarial (NFR-1):** `TestCatalogAggregator_GracefulDegradation_TypedStatusNeverDropped_Spec096` + the integration `TestDiscovery_OneProviderDown_CatalogStillServes_Spec096` prove one provider down → typed `ProviderDiscoveryStatus`, catalog still served, provider NEVER silently dropped; the Ollama path's availability is independent of any hosted provider (NFR-2). → Evidence: report.md → SCOPE-04. _(Unit adversarial GREEN across unreachable/auth_failed/timeout + captured RED-before — E1/E2; the `integration` test is the live confirmation, DEFERRED to a clean-stack dispatch under T2-7.)_
- [x] D04-T2-2 — **Leaf-purity checkitem (088 invariant):** a grep/`go list` proves zero project imports into `internal/assistant/openknowledge/modelswitch` — the catalog is INJECTED as the admissible set at construction, never imported into the leaf. → Evidence: report.md → SCOPE-04 (import check). _(modelswitch imports only `fmt`/`log/slog`/`strings` — E3.)_
- [x] D04-T2-3 — **One-validator/one-store (088/089 invariant):** discovery feeds the EXISTING `modelswitch.Allowlist` + `modelpref.Store`; NO second validator/store/picker is introduced. → Evidence: report.md → SCOPE-04. _(E3 — `CatalogResolver.Validate` delegates to `modelswitch.Allowlist.Resolve`; `Select` persists via `modelpref.Store`.)_
- [x] D04-T2-4 — **Fail-loud discovery SST (G028):** `cache_ttl_ms` + `per_provider_timeout_ms` come from SST `> 0` (SCOPE-01); no hardcoded TTL/timeout default appears in the aggregator. → Evidence: report.md → SCOPE-04 (grep). _(E1 — `TestCatalogAggregator_FailLoudOnNonPositiveSSTBounds_Spec096`: zero/negative ttl + timeout all fail loud at construction.)_
- [x] D04-T2-5 — **Canonicalization round-trip + off-catalog rejection:** split-on-first-`/` round-trips (`TestCanonicalize_SplitOnFirstSlash_RoundTrip_Spec096`), a bare Ollama id normalizes iff installed (`TestCanonicalize_BareOllamaIdNormalized_Spec096`), and an off-catalog id yields the same `modelswitch.Rejection` with NO store write / NO dispatch (`TestValidate_OffCatalogRefused_TypedRejection_Spec096`, ADVERSARIAL). → Evidence: report.md → SCOPE-04. _(E1 — the bare `gemma3:4b` control validates, so 089 bare-Ollama selections keep working.)_
- [x] D04-T2-6 — Each adversarial test is non-tautological with a captured RED-before (a build that silently drops a down provider, or that accepts an off-catalog id, would fail it); no bailout early-returns. → Evidence: report.md → SCOPE-04 RED-before. _(E2 — both adversarial tests fail with the injected silent-drop / accept-any regressions, GREEN after revert.)_
- [ ] D04-T2-7 — All `unit` + `integration` SCOPE-04 tests pass with NO skips; the SCN-096-D04 `e2e-api` leg is handed to the home-lab dispatch (C7) and NOT marked passing from dev. → Evidence: report.md → SCOPE-04 (test run + deferral note). _(PARTIAL — the `unit` leg passes with NO skips (E1); the `integration` `TestDiscovery_OneProviderDown_CatalogStillServes_Spec096` + the `e2e-api` leg are live-stack, DEFERRED to a clean-stack `bubbles.devops` dispatch, NOT marked passing from dev.)_

---

## Scope 5: SCOPE-05 — Model-aware CostFn + load-bearing USD budget enforcement

**Status:** [ ] Not started
**Scope-Kind:** code (model-aware CostFn closure + budget pre-flight wiring + migration 062 companion table)
**Depends on:** SCOPE-04
**Foundation:** false (the cost/budget seam that makes the EXISTING
064/087 USD budget pre-flight load-bearing for paid providers; consumes
the SCOPE-04-resolved provider-qualified model + the SCOPE-01
`llm.model_costs` SST + the SCOPE-03 dispatch seam it gates)

**Intent:** Make spend **model-aware** and the existing per-user + global
USD budgets **load-bearing** for paid providers while keeping Ollama
free. Today the open-knowledge agent wires a zero-cost CostFn
(`okagent.CostFn(func(int) float64 { return 0 })` in
[`cmd/core/wiring_assistant_openknowledge.go`](../../cmd/core/wiring_assistant_openknowledge.go)),
so the per-query / monthly / per-user USD pre-flight already present in
[`internal/assistant/openknowledge/agent/agent.go`](../../internal/assistant/openknowledge/agent/agent.go)
(the `ErrCapUSDPerUserMonth` / `ErrCapUSDMonthly` sentinels) **never
binds**. This scope replaces that closure with a model-aware CostFn over
the SST `llm.model_costs` rate table — `ollama` returns `$0`
deterministically (budget **not** consumed), a paid provider-qualified
model returns its SST rate, and **a billable model with NO declared rate
is a fail-loud typed refusal BEFORE the call — NEVER a silent `$0`** — and
adds the append-only `model_usage_ledger` (migration 062 companion table)
so the pre-flight reads month-to-date per-user + global spend and refuses
an exhausted ceiling **before** any billable provider call, appending the
actual cost only after a successful dispatch.

### Surface (design §12, §5.3, §10)

- [`cmd/core/wiring_assistant_openknowledge.go`](../../cmd/core/wiring_assistant_openknowledge.go)
  — replace the zero-cost `okagent.CostFn(func(int) float64 { return 0 })`
  wiring with a model-aware closure built over the SST rate table; the
  closure is invoked with the effective (provider-qualified) model the
  agent already resolves (`Effective`), so the cost seam knows
  `(provider, model, tokens)`.
- NEW `internal/assistant/openknowledge/agent/costfn_modelaware.go` —
  the model-aware CostFn: `provider == ollama → $0` (deterministic, free
  local inference, budget not consumed — NFR-2); else look up the SST
  `llm.model_costs` rate for the provider-qualified model; a missing rate
  for a billable model returns a typed fail-loud refusal (NEVER a silent
  `$0`). Exercised by NEW
  [`internal/assistant/openknowledge/agent/costfn_modelaware_test.go`](../../internal/assistant/openknowledge/agent/costfn_modelaware_test.go).
- [`internal/assistant/openknowledge/agent/agent.go`](../../internal/assistant/openknowledge/agent/agent.go)
  — the EXISTING budget pre-flight is left structurally intact; this scope
  wires the model-aware cost estimate (prompt-token estimate × rate) into
  it and makes it read month-to-date spend from `model_usage_ledger` for
  the caller (`actor_user_id`) and globally, refusing with the existing
  `ErrCapUSDPerUserMonth` / `ErrCapUSDMonthly` sentinels when either
  ceiling would be breached — **before** any provider call; the actual
  cost is appended after a successful dispatch. Exercised by NEW
  [`internal/assistant/openknowledge/agent/budget_preflight_modelcost_test.go`](../../internal/assistant/openknowledge/agent/budget_preflight_modelcost_test.go).
- `internal/db` migration **062** companion table `model_usage_ledger`
  (design §5.3): append-only `id`, `actor_user_id` (per-user **spend**
  dimension — allowed; this is per-user budget, NOT a per-user key),
  `connection_id`, `model` (provider-qualified), `tokens`, `usd_cost`
  (`0` for ollama), `created_at` (app-written monthly-window key). No
  edits/deletes — audit-clean.
- `config/smackerel.yaml` `llm.model_costs[]` (SCOPE-01 SST) is the rate
  source; SCOPE-01 already validates that every enabled non-ollama model
  has a rate (the config-time backstop), and this scope is the **runtime**
  defensive refusal if a rate is ever absent at dispatch.

**Covers scenarios:** SCN-096-G03.

### Use Cases (Gherkin) — quoted from spec.md §4

```gherkin
Scenario: SCN-096-G03 — Selecting a paid model enforces the USD budgets before dispatch
  Given a user has selected a paid hosted model
  And the user's monthly USD budget is exhausted
  When the user asks a question
  Then the request is refused before any billable provider call
  And the refusal states that the budget is exhausted
```

### Test Plan — SCOPE-05

`unit` proves the model-aware cost mapping (ollama→$0, paid→rate,
missing-rate→refusal) and the pre-flight refusal in isolation;
`integration` proves the exhausted-budget refusal fires **before** the
provider call against the live ephemeral stack with **synthetic** rates.
Entries are copied verbatim from
[scenario-manifest.json](scenario-manifest.json).

**Category: unit**

| Scenario | Concrete test (file::function) | Asserts |
|----------|--------------------------------|---------|
| SCN-096-G03 | `internal/assistant/openknowledge/agent/costfn_modelaware_test.go::TestCostFn_OllamaZero_PaidUsesRate_Spec096` | `provider == ollama` → `$0` (budget not consumed); a paid provider-qualified model → its SST `llm.model_costs` rate; the cost seam is model-aware (knows `(provider, model, tokens)`). |
| SCN-096-G03 | `internal/assistant/openknowledge/agent/costfn_modelaware_test.go::TestCostFn_PaidModelMissingRate_RefusesFailLoud_Spec096` (ADVERSARIAL) | A billable (non-ollama) model with NO `llm.model_costs` rate yields a typed fail-loud refusal; fails if the path ever returns a silent `$0` for a paid model (the NO-DEFAULTS budget-bypass guard). |
| SCN-096-G03 | `internal/assistant/openknowledge/agent/budget_preflight_modelcost_test.go::TestBudgetPreflight_PaidOverBudget_RefusesBeforeDispatch_Spec096` (ADVERSARIAL) | A paid model whose estimated cost would breach the per-user or global month-to-date ceiling is refused with `ErrCapUSDPerUserMonth` / `ErrCapUSDMonthly` **before** any dispatch; fails if the provider call is reached before the ceiling check. |

**Category: integration (ephemeral stack + synthetic rates/ledger)**

| Scenario | Concrete test (file::function) | Asserts |
|----------|--------------------------------|---------|
| SCN-096-G03 | `tests/integration/model_budget_enforcement_test.go::TestAsk_PaidModelExhaustedBudget_RefusedBeforeProviderCall_Spec096` | Against the live ephemeral Go core, an `/ask` with a paid model and an exhausted `model_usage_ledger` month-to-date spend is refused before the billable provider call (no `/llm/chat` dispatch fires); uses synthetic rates + a synthetic ledger, NEVER a real provider/credential. Hits the REAL agent/budget path — NO interception. |

> Live-stack note: the integration row hits the REAL agent + budget
> pre-flight against the ephemeral test stack with synthetic rates and a
> synthetic ledger (test-isolation + env-pollution policy); it MUST NOT
> touch the persistent dev store, a real provider, or a real credential.
> The unit rows construct the CostFn/pre-flight directly and assert the
> cost mapping + refusal WITHOUT network interception — correctly
> classified `unit`.

### Definition of Done — SCOPE-05 (7 of 12 met — implementation in progress)

**Tier-1 (universal):**

- [ ] D05-T1-1 — `bash .github/bubbles/scripts/artifact-lint.sh specs/096-multi-provider-model-connections` clean. → Evidence: report.md → SCOPE-05. _(DEFERRED — blocked by absent `uservalidation.md`, a closeout artifact owned by `bubbles.plan`; not a SCOPE-05 code gap. Same caveat as SCOPE-01..04.)_
- [ ] D05-T1-2 — `./smackerel.sh check` EXIT 0. → Evidence: report.md → SCOPE-05. _(DEFERRED to the orchestrator post-implementation; SCOPE-05 compiles clean under `go test ./...` — E1 `finished OK`, `cmd/core` built.)_
- [ ] D05-T1-3 — `./smackerel.sh format --check` EXIT 0. → Evidence: report.md → SCOPE-05. _(DEFERRED to the orchestrator post-implementation; global gate.)_
- [x] D05-T1-4 — Every evidence block in report.md → SCOPE-05 is REAL terminal output (anti-fabrication). → Evidence: report.md → SCOPE-05 (E1–E4, captured).
- [x] D05-T1-5 — 088/089 do-not-amend boundary respected: the budget/cost seam is additive; no `modelswitch`/`modelpref`/picker behaviour changes. → Evidence: report.md → SCOPE-05 Change Manifest (no modelswitch/modelpref/picker file edited).

**Tier-2 (role-specific: model-aware cost + load-bearing budget + G028 + R4 blast-radius):**

- [x] D05-T2-1 — **G028 fail-loud on missing/zero rate (binding):** a billable (non-ollama) model with NO `llm.model_costs` rate is a typed fail-loud refusal at dispatch — proven non-tautologically by `TestCostFn_PaidModelMissingRate_RefusesFailLoud_Spec096`; a grep proves no `${VAR:-default}` / `getenv(k,default)` / `unwrap_or` / silent-`0` path substitutes a zero cost for a paid model. → Evidence: report.md → SCOPE-05 (E1 test + E2 RED-before + E4 grep).
- [x] D05-T2-2 — **Ollama stays free (NFR-2):** `provider == ollama → $0` deterministically and the budget is NOT consumed (`TestCostFn_OllamaZero_PaidUsesRate_Spec096`); the local path adds zero cost and no budget read on the hot path (estCost=$0 ⇒ no ledger read; two in-process map lookups, no I/O). → Evidence: report.md → SCOPE-05 (E1/E3).
- [x] D05-T2-3 — **Budget refused BEFORE dispatch (adversarial):** `TestBudgetPreflight_PaidOverBudget_RefusesBeforeDispatch_Spec096` proves an exhausted per-user/global ceiling refuses with `ErrCapUSDPerUserMonth` / `ErrCapUSDMonthly` before any billable provider call (the fake LLM `t.Fatalf`s on any dispatch); demonstrably fails under the captured RED-before. The live integration `TestAsk_PaidModelExhaustedBudget_RefusedBeforeProviderCall_Spec096` corroboration is DEFERRED (T2-7). → Evidence: report.md → SCOPE-05 (E1/E3 + E2 RED-before).
- [ ] D05-T2-4 — **Append-only spend ledger:** migration 062 `model_usage_ledger` applies cleanly forward, is append-only (no edits/deletes), records per-user `actor_user_id` + provider-qualified `model` + `usd_cost` (`0` for ollama) with an app-written `created_at` month-window key, and the actual cost is appended only after a successful dispatch. → Evidence: report.md → SCOPE-05 (migrate output). _(PARTIAL — migration 062 authored (append-only, no DB-side defaults, CHECKs + indexes) + embedded; the `migrate`-output gate needs a live DB, DEFERRED with the integration leg. Note: design said "060"; 060/061 were taken, so 062 is the next free slot.)_
- [x] D05-T2-5 — **R4 — model-aware CostFn blast-radius (binding):** the CostFn signature change from the zero-cost `func(int) float64` closure to the model-aware `(provider, model, tokens)` seam is enumerated against its blast radius on the EXISTING 064/087 USD-budget tests; every pre-existing budget test still passes (re-run) or is updated only to supply the new model-aware seam without weakening its assertion; zero 064/087 budget behaviour silently regresses. → Evidence: report.md → SCOPE-05 (R4 table: 10 sites + whole-tree compile; 064/087 budget tests re-ran GREEN in E1).
- [x] D05-T2-6 — **Test isolation:** unit + integration tests use synthetic rates + a synthetic ledger on the ephemeral test Postgres only; no real provider, credential, or persistent-store write. → Evidence: report.md → SCOPE-05 (unit rows use a fake `SpendLedger` + synthetic rates, NO DB/provider/credential; the deferred integration row is the only DB-touching leg, ephemeral-only by design).
- [ ] D05-T2-7 — All `unit` + `integration` SCOPE-05 tests pass with NO skips/ignores; evidence is the real `./smackerel.sh test unit --go` + integration run output. → Evidence: report.md → SCOPE-05 (test run). _(PARTIAL — the `unit` leg passes with NO skips (E1/E3); the `integration` `TestAsk_PaidModelExhaustedBudget_RefusedBeforeProviderCall_Spec096` is live-stack, DEFERRED to a clean-stack `bubbles.devops` dispatch, NOT marked passing from dev.)_

---

## Scope 6: SCOPE-06 — Operator-gated web admin connection surface (wire/test/enable)

**Status:** [~] In progress (backend half + frontend PWA triad delivered — 7 of 13 DoD met + evidenced; the live `e2e-ui` triad walk + the `integration`/`e2e-api` legs + the `check`/`format`/`artifact-lint` closeout gates are deferred — see report.md → SCOPE-06 and → SCOPE-06 (frontend))
**Scope-Kind:** code (operator-gated admin API + operator middleware) + web (PWA triad)
**Depends on:** SCOPE-02, SCOPE-04
**Foundation:** false (the operator runtime-plane surface that wires the
SCOPE-02 vault credential + truthful test + enable/disable into the
SCOPE-04 catalog via the effective-enabled predicate)

**Intent:** Land the operator-gated web admin surface that lets the
operator wire, test, enable, and disable each SST-declared db-mode
connection slot on the **runtime plane** (design §5.1) — without ever
echoing a stored secret and without ever reporting a false test success.
`PUT …/credential` is write-only (returns a redacted view, secret never
echoed); `POST …/test` runs a live per-kind reachability + credential
probe and reports a **TRUTHFUL** typed pass/fail (`auth_failed` |
`unreachable` | `timeout`), NEVER a false `ok` and NEVER an Ollama
substitute; `enable` is 409-guarded (only when a credential is present
**AND** the last test = `ok`); `disable` removes the connection from the
catalog. The **effective-enabled** predicate (registry-declared **AND** DB
`enabled` **AND** `last_test_outcome = ok` **AND** credential present) is
the SINGLE gate discovery consults (design §5.1). The operator-gated PWA
triad (`model-connections.html`, `model-connection-add.html`,
`model-connection-detail.html`) follows the existing connectors
convention (`connectors.html` → `connectors-add.html` →
`connector-detail.html`).

**R1 — operator-gate mechanism RESOLVED in this scope.** Design §11.4
left the exact operator-gate mechanism as a residual decision "to confirm
against the live auth model in plan". Resolved here: the operator gate is
an SST `infrastructure.operator_user_ids` allowlist (fail-loud,
NO-DEFAULTS — G028) checked by an operator middleware that reads the
authenticated bearer/session **subject** from the existing
[`internal/auth/`](../../internal/auth) middleware (the spec-044
claim-bound `actor_user_id`), layered over the existing
`webAuthMiddleware`. **Fail-loud rule:** if `operator_user_ids` is empty
**while** any connection-mutating endpoint (`/credential`, `/test`,
`/enable`, `/disable`) is reachable, startup aborts named (no
open-by-default operator surface — today's shared-token "operator" read
in [`internal/web/invites.go`](../../internal/web/invites.go#L196-L201)
is NOT sufficient for a credential-mutating surface). A non-operator
authenticated subject is `403`; an anonymous caller is `401`. This is
pinned by `TestAdminModelConnections_OperatorGate_403NonOperator_401Anonymous_Spec096`.

### Surface (design §6.1, §5.1, §11.4, §2.2)

- NEW `internal/api/model_connections_admin.go` — the operator-gated
  surface (design §6.1), mounted beside the existing web admin routes:
  `GET /v1/admin/model-connections` (list slots + status, no secrets),
  `GET …/{id}` (one slot; `404` unknown-slot fail-loud), `PUT
  …/{id}/credential` (write-only secret store → redacted view; `404`
  non-db-mode slot; `422` missing required secret field for the kind),
  `POST …/{id}/test` (live per-kind probe → `{outcome:"ok"|"failed",
  detail, tested_at}`), `POST …/{id}/enable` (`409` if untested /
  no-credential), `POST …/{id}/disable`. **Every response omits plaintext
  credentials** (reads expose only `secret_present` + `secret_redaction`
  last-4 + `last_tested_*`). Exercised by NEW
  [`internal/api/model_connections_admin_test.go`](../../internal/api/model_connections_admin_test.go).
- NEW `internal/api/model_connections_operator_gate.go` — the operator
  middleware (R1): SST `infrastructure.operator_user_ids` allowlist
  checked against the existing `internal/auth` bearer/session subject;
  fail-loud when empty while a mutating endpoint is reachable; `403`
  non-operator / `401` anonymous. Exercised by NEW
  [`internal/api/model_connections_operator_gate_test.go`](../../internal/api/model_connections_operator_gate_test.go).
- The per-kind **test probe** drives the SCOPE-04 foundation `Discover` /
  reachability adapter against the SCOPE-02-decrypted credential and
  records `last_test_outcome` / `last_test_detail` (typed, NEVER the
  secret) to the SCOPE-02 `model_provider_connections` row; a failed
  probe persists `failed` and the slot cannot be enabled (the `409`
  guard).
- The **effective-enabled** predicate (design §5.1) is the single gate
  shared with SCOPE-04 discovery: enable adds the provider's models to the
  combined catalog, disable removes them.
- NEW operator-gated PWA triad under the web UI:
  `model-connections.html` (list: every SST-declared slot + kind,
  enabled/disabled, last-tested pass/fail + when, secret presence + last-4,
  model count), `model-connection-add.html` (pick a declared-but-
  unconfigured db-mode slot → enter that kind's write-only secret fields,
  non-secret params shown read-only from the SST registry — a brand-new
  KIND is an SST topology edit, not a UI invention), and
  `model-connection-detail.html` (test/enable/disable/rotate one slot),
  following the existing `connectors.html` → `connectors-add.html` →
  `connector-detail.html` convention.

**Covers scenarios:** SCN-096-W01, SCN-096-W02, SCN-096-W03, SCN-096-W04.

### Use Cases (Gherkin) — quoted from spec.md §4

```gherkin
Scenario: SCN-096-W01 — Operator adds an Anthropic connection and tests it
  Given the operator opens the model-connections page in the web UI
  When the operator enters Anthropic connection details and runs "test connection"
  Then the system reports a truthful pass or fail for that connection
  And on pass the connection becomes available for model discovery

Scenario: SCN-096-W02 — Operator adds OpenAI, Foundry, Google, and Bedrock connections
  Given the operator is on the model-connections page
  When the operator adds an OpenAI, a Microsoft Foundry, a Google, and an Amazon Bedrock connection
  Then each is saved with its provider-specific parameters
  And each can be tested, enabled, and disabled independently

Scenario: SCN-096-W03 — Operator enables and disables a connection
  Given a configured, tested provider connection
  When the operator disables it
  Then its models are removed from the combined catalog
  And re-enabling it restores its models to the catalog

Scenario: SCN-096-W04 — A failed test connection reports a typed, actionable error
  Given the operator enters an invalid key or unreachable endpoint
  When the operator runs "test connection"
  Then the system reports a typed failure naming the connection and the failure reason
  And the system never reports a false success and never substitutes Ollama
```

### Test Plan — SCOPE-06

`unit` proves the write-only-credential redaction, the truthful
pass/fail, the `409` enable guard, the `404` unknown-slot, the per-kind
secret fields, and the operator gate in isolation; `integration` proves
enable/disable catalog membership against the live ephemeral stack; the
live `e2e-api` legs (real hosted-provider reachability) are **deferred to
the home-lab `bubbles.devops` dispatch (C7)**. Entries are copied verbatim
from [scenario-manifest.json](scenario-manifest.json).

**Category: unit**

| Scenario | Concrete test (file::function) | Asserts |
|----------|--------------------------------|---------|
| SCN-096-W01 | `internal/api/model_connections_admin_test.go::TestAdminModelConnections_PutCredentialWriteOnly_RedactedView_Spec096` (ADVERSARIAL) | `PUT …/credential` stores write-only and returns a redacted view; the cleartext secret is never echoed in the response/body. Fails if the secret is ever returned. |
| SCN-096-W01 | `internal/api/model_connections_admin_test.go::TestAdminModelConnections_TestConnection_TruthfulOutcome_Spec096` | `POST …/test` reports the TRUTHFUL probe outcome (`ok` on a reachable+authenticated probe); `last_test_outcome` is persisted. |
| SCN-096-W02 | `internal/api/model_connections_admin_test.go::TestAdminModelConnections_PerKindSecretFields_OpenAIFoundryGoogleBedrock_Spec096` | Each kind saves its provider-specific write-only secret fields (OpenAI / Azure-Foundry / Google / Bedrock) with non-secret params read-only from the SST registry. |
| SCN-096-W02 | `internal/api/model_connections_admin_test.go::TestAdminModelConnections_UnknownSlotRejected404_Spec096` (ADVERSARIAL) | An `id` not in the SST registry is `404` (closed-set fail-loud); fails if a UI-invented slot is ever accepted. |
| SCN-096-W03 | `internal/api/model_connections_admin_test.go::TestAdminModelConnections_EnableUntested_Blocked409_Spec096` (ADVERSARIAL) | Enabling a slot with no credential or `last_test_outcome != ok` is `409`; fails if an unverified connection is ever enabled into the catalog. |
| SCN-096-W04 | `internal/api/model_connections_admin_test.go::TestAdminModelConnections_FailedTest_TypedError_NeverFalseSuccess_Spec096` (ADVERSARIAL) | A failed probe yields `outcome:failed` with a typed `detail` (`auth_failed` \| `unreachable` \| `timeout`) and persists `failed` (never the secret); fails if a failed probe is ever reported `ok` or substitutes Ollama. |
| SCN-096-W04 | `internal/api/model_connections_operator_gate_test.go::TestAdminModelConnections_OperatorGate_403NonOperator_401Anonymous_Spec096` (ADVERSARIAL) | The operator gate (R1) admits only an `operator_user_ids` subject: a non-operator authenticated subject is `403`, an anonymous caller is `401`; fails if a non-operator ever reaches a connection-mutating endpoint. |

**Category: integration (ephemeral stack)**

| Scenario | Concrete test (file::function) | Asserts |
|----------|--------------------------------|---------|
| SCN-096-W03 | `tests/integration/model_connections_enable_disable_test.go::TestEnableDisable_CatalogMembershipFollows_Spec096` | Against the live ephemeral stack, enabling a credentialed+tested slot adds its models to the combined catalog and disabling removes them (the effective-enabled predicate is the single gate); hits the REAL aggregator/DB — NO interception. |

**Category: e2e-api (deferred to home-lab `bubbles.devops` dispatch — C7)**

| Scenario | Concrete test (file::function) | Asserts |
|----------|--------------------------------|---------|
| SCN-096-W01 | `tests/e2e/admin/model_connections_e2e_test.go::TestAdmin_WireTestEnableAnthropic_Spec096` | The live operator wire → truthful test → enable flow against a REAL Anthropic connection; runs in the home-lab dispatch (real credential + reachability), NOT in-repo. |
| SCN-096-W02 | `tests/e2e/admin/model_connections_e2e_test.go::TestAdmin_AddFourHostedProviders_Independent_Spec096` | Adding OpenAI / Azure-Foundry / Google / Bedrock as four independent live connections; home-lab dispatch (real credentials). |
| SCN-096-W04 | `tests/e2e/admin/model_connections_e2e_test.go::TestAdmin_BadCredential_FailsTruthfully_Spec096` | A bad credential against a REAL provider endpoint fails truthfully (`outcome:failed`, typed detail); home-lab dispatch (real endpoint reachability). |

> Live-stack note: the `integration` row hits the REAL aggregator + DB on
> the ephemeral test stack (no request interception). The `e2e-api` rows
> require real hosted-provider reachability + real credentials and are
> handed to the home-lab `bubbles.devops` dispatch (C7); they are NOT
> marked passing from dev. The `unit` rows exercise the real handler +
> operator middleware with synthetic credentials and a stubbed probe
> seam, asserting the redaction / typed-outcome / gate logic — correctly
> classified `unit`.

### Definition of Done — SCOPE-06 (backend half + frontend PWA triad implemented — 7 of 13 met + evidenced; live-stack legs — integration/e2e-api/e2e-ui — deferred, see report.md → SCOPE-06 and → SCOPE-06 (frontend))

**Tier-1 (universal):**

- [ ] D06-T1-1 — `bash .github/bubbles/scripts/artifact-lint.sh specs/096-multi-provider-model-connections` clean. → Evidence: report.md → SCOPE-06.
- [ ] D06-T1-2 — `./smackerel.sh check` EXIT 0. → Evidence: report.md → SCOPE-06.
- [ ] D06-T1-3 — `./smackerel.sh format --check` EXIT 0. → Evidence: report.md → SCOPE-06.
- [x] D06-T1-4 — Every evidence block in report.md → SCOPE-06 is REAL terminal output (anti-fabrication). → Evidence: report.md → SCOPE-06.
- [x] D06-T1-5 — **No env-specific content in repo:** the admin surface + PWA triad carry only generic placeholders (no real hostnames/IPs/tailnet identifiers/operator usernames/secret values); `infrastructure.operator_user_ids` is an abstract SST allowlist, real values live in the deploy adapter. → Evidence: report.md → SCOPE-06 (pii-scan / grep).

**Tier-2 (role-specific: operator gate (R1) + write-only secret + truthful test + live-stack):**

- [x] D06-T2-1 — **R1 operator gate (binding, adversarial):** the operator-only boundary is enforced by the `infrastructure.operator_user_ids` SST allowlist over the existing `internal/auth` subject; `TestAdminModelConnections_OperatorGate_403NonOperator_401Anonymous_Spec096` proves `403` non-operator / `401` anonymous; an empty `operator_user_ids` while a connection-mutating endpoint is reachable aborts fail-loud at startup (NO-DEFAULTS, G028). → Evidence: report.md → SCOPE-06 (gate test + fail-loud grep).
- [x] D06-T2-2 — **Write-only secret (binding):** the stored credential is NEVER echoed, returned, or logged; `PUT …/credential` returns only a redacted view; reads expose only `secret_present` + `secret_redaction` (last-4) + `last_tested_*` — proven by `TestAdminModelConnections_PutCredentialWriteOnly_RedactedView_Spec096` + a grep that no handler logs/returns the bundle. → Evidence: report.md → SCOPE-06.
- [x] D06-T2-3 — **Truthful test, never false success, never Ollama (adversarial):** `TestAdminModelConnections_FailedTest_TypedError_NeverFalseSuccess_Spec096` proves a failed probe reports `failed` with a typed `detail` and persists `failed` (never the secret) and never substitutes Ollama; non-tautological (fails if a failed probe is ever reported `ok`). → Evidence: report.md → SCOPE-06.
- [ ] D06-T2-4 — **Enable 409-guard + effective-enabled single gate:** enable is refused `409` unless a credential is present AND `last_test_outcome = ok` (`TestAdminModelConnections_EnableUntested_Blocked409_Spec096`), and the effective-enabled predicate (registry-declared AND DB `enabled` AND `last_test_outcome = ok` AND credential present) is the single gate discovery consults (`TestEnableDisable_CatalogMembershipFollows_Spec096`). → Evidence: report.md → SCOPE-06.
- [x] D06-T2-5 — **Closed-set slot + per-kind secret fields:** an `id` not in the SST registry is `404` (`TestAdminModelConnections_UnknownSlotRejected404_Spec096`); each kind saves its provider-specific write-only secret fields with non-secret params read-only from the registry (`TestAdminModelConnections_PerKindSecretFields_OpenAIFoundryGoogleBedrock_Spec096`); a brand-new kind is an SST topology edit, not a UI invention. → Evidence: report.md → SCOPE-06.
- [ ] D06-T2-6 — **Live-stack authenticity + C7 deferral:** the `integration` row hits the REAL aggregator/DB with no request interception; each `e2e-api` row (`TestAdmin_WireTestEnableAnthropic_Spec096`, `TestAdmin_AddFourHostedProviders_Independent_Spec096`, `TestAdmin_BadCredential_FailsTruthfully_Spec096`) is explicitly handed to the home-lab `bubbles.devops` dispatch (C7) and NOT marked passing from dev. → Evidence: report.md → SCOPE-06 (test run + deferral note).
- [x] D06-T2-7 — Each adversarial test is non-tautological with a captured RED-before (a build that echoes the secret, reports a failed probe as `ok`, enables an untested slot, or lets a non-operator through would fail it); no bailout early-returns. → Evidence: report.md → SCOPE-06 RED-before.
- [ ] D06-T2-8 — All `unit` + `integration` SCOPE-06 tests pass with NO skips; the live `e2e-api` legs are deferred to the home-lab dispatch (C7). → Evidence: report.md → SCOPE-06 (test run).

---

## Scope 7: SCOPE-07 — Combined catalog selection across Telegram + web, 088/089 parity

**Status:** [~] In progress (unified selection surfaces + 088/089 parity implemented and unit-GREEN, 9 of 13 DoD met + evidenced; the `check`/`format`/`artifact-lint` closeout gates + the live `e2e-api` parity legs are deferred — see report.md → SCOPE-07)
**Scope-Kind:** code (Telegram `/model` picker + `/v1/agent/model` enrichment over the existing validator/store)
**Depends on:** SCOPE-04, SCOPE-06
**Foundation:** false (the unified selection surface that renders the
SCOPE-04 catalog + SCOPE-06 enable state across both surfaces through the
EXISTING 088/089 validator + store, preserving every 088/089 parity,
fork, and precedence invariant provider-agnostically)

**Intent:** Render ONE provider-grouped, provider-qualified, cost-hinted
combined catalog across the Telegram `/model` numbered picker and the web
`/v1/agent/model` surface, resolving through the **same** `modelswitch`
validator and the **same** `modelpref` store (one-validator/one-store —
the 088/089 invariant, design §14). The list groups by provider
(Ollama/local first); only **reachable** models are numbered so a numbered
reply always maps to a dispatchable model; an unreachable provider is
**shown-but-disabled** with its typed `ProviderDiscoveryStatus` (NEVER
dropped). `GET /v1/agent/model` gains additive per-entry `capabilities[]`
+ `cost_class` (+ optional month-to-date `budget`) while `allowed_models[]`
is preserved **BYTE-FOR-BYTE** for existing 089 clients (**R2**).
Selection persists via the EXISTING claim-bound `modelpref` store; the
per-request > sticky > default precedence + the gather-vs-synthesis fork
behave EXACTLY as 088/089 define, provider-agnostically. A non-tool-capable
GATHER selection is **shown-but-disabled** and **rejected fail-loud**
(**R3**).

**R2 — additive enrichment preserves 089 clients (RESOLVED here).** The
`GET /v1/agent/model` response gains `capabilities[]` + `cost_class` (+
optional MTD `budget`) as **additive sibling fields**; the existing
`allowed_models[]` array is preserved byte-for-byte (same ordering, same
provider-qualified strings an 089 client already reads), so an 089-era
consumer that reads only `allowed_models` / `effective_model` /
`sticky_model` / `source` sees no breaking change. Pinned by
`TestAgentModel_GetEnrichedCatalogAdditive_AllowedModelsPreserved_Spec096`.

**R3 — non-tool-capable GATHER selection (RESOLVED here).** A model that
is not `tool_capable` is invalid as a **gather** selection (the 088/089
gather turn requires tool-calling). Consistent with the **unreachable**
decision (shown-but-disabled, never silently dropped — design §6.3 /
UX OQ-5), a non-tool-capable model is **shown-but-disabled** in the gather
context and a direct attempt to select it for gather is **rejected
fail-loud** (the same `modelswitch.Rejection` shape), NEVER silently
coerced or accepted. **Disable-vs-filter justification:** the entry is
*shown* (so the user understands why a known model is unavailable for
gather — Principle 8 transparency) but *disabled/rejected* (so a numbered
reply never maps to a non-dispatchable gather selection), exactly
mirroring the unreachable-provider treatment. Pinned by
`TestParity_NonToolCapableGatherShownDisabled_RejectedFailLoud_Spec096`.

### Surface (design §6.4, §8, §14, §2.4)

- [`internal/telegram/model_command.go`](../../internal/telegram/model_command.go)
  — the existing `/model` numbered picker renders the combined
  provider-grouped, provider-qualified, cost-hinted list (Ollama/local
  group first); only reachable models are numbered; an unreachable
  provider is shown-but-disabled with its typed status; selecting a hosted
  model persists via the existing claim-bound `modelpref` store. Exercised
  by NEW
  [`internal/telegram/model_command_multiprovider_test.go`](../../internal/telegram/model_command_multiprovider_test.go).
- [`internal/api/agent_model.go`](../../internal/api/agent_model.go) —
  `GET /v1/agent/model` `agentModelView` gains additive per-entry
  `capabilities[]` + `cost_class` (+ optional MTD `budget`); `allowed_models[]`
  preserved byte-for-byte (R2); `effective_model` / `sticky_model` /
  `system_default` / `source` become provider-qualified strings; `PUT`
  accepts the same selection Telegram accepts. Exercised by NEW
  [`internal/api/agent_model_multiprovider_test.go`](../../internal/api/agent_model_multiprovider_test.go).
- Both surfaces resolve through the SAME `agenttool` singletons — the
  EXISTING [`modelswitch`](../../internal/assistant/openknowledge/modelswitch)
  validator (over the SCOPE-04 injected catalog) and the EXISTING
  [`modelpref`](../../internal/assistant/openknowledge/modelpref) store;
  per-request > sticky > default precedence + the gather-vs-synthesis fork
  (`Override{SynthesisModel, GatherModel}`) are unchanged (design §14
  invariants #1, #3, #4); only the admissible ids are now
  provider-qualified. The 088/089 parity is pinned by NEW
  [`internal/api/agent_model_parity_spec096_test.go`](../../internal/api/agent_model_parity_spec096_test.go).
- The SCOPE-06 enable state feeds the SCOPE-04 catalog the picker renders;
  no second validator/store/picker is introduced (design §14 invariant
  #1).

**Covers scenarios:** SCN-096-D02, SCN-096-D03, SCN-096-D05, SCN-096-G06.

### Use Cases (Gherkin) — quoted from spec.md §4

```gherkin
Scenario: SCN-096-D02 — A user lists the combined models in Telegram and picks a hosted model
  Given a user opens the model picker in Telegram
  When the picker lists the available models
  Then it shows Ollama-installed and connected hosted models in one numbered, provider-qualified list
  And selecting a hosted model sets it as the user's model for subsequent questions

Scenario: SCN-096-D03 — The same list and selection are available over HTTP/web (parity)
  Given the same catalog and the same user identity
  When the user reads and sets their model through the HTTP/web model surface
  Then it returns the same combined catalog and accepts the same selection as Telegram
  And both surfaces resolve through the same validator and the same store

Scenario: SCN-096-D05 — A user's selection persists across turns
  Given a user has selected a model
  When the user asks another question later without re-selecting
  Then the system uses the user's persisted selection
  And per-request > sticky > default precedence is honored

Scenario: SCN-096-G06 — Specs 088/089 fork, precedence, and parity are preserved provider-agnostically
  Given a user switches the gather model and the synthesis model independently
  When the open-knowledge agent runs
  Then the gather-vs-synthesis turn fork, the precedence order, and one-validator/one-store parity behave exactly as specs 088/089 define
  And the only change is that the selected model may now belong to any connected provider
```

### Test Plan — SCOPE-07

`unit` proves the combined provider-grouped rendering, the
unreachable-shown-disabled / only-reachable-numbered rule, the additive
enrichment (allowed_models byte-for-byte — R2), the sticky+precedence
contract, and the one-validator/one-store + non-tool-capable-gather parity
(R3) in isolation; the live cross-surface `e2e-api` parity legs (real
hosted models) are **deferred to the home-lab `bubbles.devops` dispatch
(C7)**. Entries are copied verbatim from
[scenario-manifest.json](scenario-manifest.json).

**Category: unit**

| Scenario | Concrete test (file::function) | Asserts |
|----------|--------------------------------|---------|
| SCN-096-D02 | `internal/telegram/model_command_multiprovider_test.go::TestModelPicker_TelegramCombinedProviderGroupedList_Spec096` | The `/model` picker renders ONE provider-grouped, provider-qualified, cost-hinted list (Ollama/local group first) over the combined catalog. |
| SCN-096-D02 | `internal/telegram/model_command_multiprovider_test.go::TestModelPicker_UnreachableShownDisabledOnlyReachableNumbered_Spec096` (ADVERSARIAL) | An unreachable provider is shown-but-disabled with its typed status and only reachable models are numbered (a numbered reply always maps to a dispatchable model); fails if an unreachable provider is dropped or numbered. |
| SCN-096-D03 | `internal/api/agent_model_multiprovider_test.go::TestAgentModel_GetEnrichedCatalogAdditive_AllowedModelsPreserved_Spec096` (ADVERSARIAL — R2) | `GET /v1/agent/model` gains additive per-entry `capabilities[]` + `cost_class`; `allowed_models[]` is preserved byte-for-byte (ordering + provider-qualified strings) for 089 clients; fails if the additive enrichment renames/reorders/removes any `allowed_models` entry. |
| SCN-096-D03 | `internal/api/agent_model_multiprovider_test.go::TestAgentModel_TelegramWebParity_SameValidatorSameStore_Spec096` | Web `PUT` accepts the same selection Telegram accepts and both resolve through the SAME `modelswitch` validator + `modelpref` store (one-validator/one-store). |
| SCN-096-D05 | `internal/api/agent_model_multiprovider_test.go::TestAgentModel_StickyHostedPersistsPrecedence_Spec096` | A sticky hosted selection persists across turns and a per-request override beats the sticky for one invocation only; the precedence ordering + store reads are byte-for-byte the 089 contract (provider-qualified ids only). |
| SCN-096-G06 | `internal/api/agent_model_parity_spec096_test.go::TestParity_OneValidatorOneStore_ForkPrecedence_ProviderAgnostic_Spec096` (ADVERSARIAL) | The gather-vs-synthesis fork + per-request > sticky > default precedence + one-validator/one-store behave EXACTLY as 088/089 define provider-agnostically; fails if a second validator/store is introduced. |
| SCN-096-G06 | `internal/api/agent_model_parity_spec096_test.go::TestParity_NonToolCapableGatherShownDisabled_RejectedFailLoud_Spec096` (ADVERSARIAL — R3) | A non-tool-capable model is shown-but-disabled for gather and a direct gather selection of it is rejected fail-loud (the same `modelswitch.Rejection`); fails if a non-tool-capable gather selection is ever accepted. |

**Category: e2e-api (deferred to home-lab `bubbles.devops` dispatch — C7)**

| Scenario | Concrete test (file::function) | Asserts |
|----------|--------------------------------|---------|
| SCN-096-D02 | `tests/e2e/agent/openknowledge_e2e_test.go::TestTelegram_PickHostedModelPersists_Spec096` | The live Telegram pick → persist → dispatch of a REAL hosted model; home-lab dispatch (real reachability). |
| SCN-096-D03 | `tests/e2e/agent/openknowledge_e2e_test.go::TestWebTelegramModelParity_Spec096` | The live web and Telegram surfaces return the SAME selection result over the real combined catalog; home-lab dispatch. |
| SCN-096-D05 | `tests/e2e/agent/openknowledge_e2e_test.go::TestAsk_StickyHostedSelectionPersistsAcrossTurns_Spec096` | A sticky hosted selection persists across live `/ask` turns; home-lab dispatch (real hosted model). |
| SCN-096-G06 | `tests/e2e/agent/openknowledge_e2e_test.go::TestForkPrecedenceParity_MultiProvider_Spec096` | The live gather-vs-synthesis fork + precedence parity across providers; home-lab dispatch. |

> Live-stack note: every `e2e-api` row hits the REAL running system with
> real hosted models and is handed to the home-lab `bubbles.devops`
> dispatch (C7) — NOT marked passing from dev. The `unit` rows construct
> the picker/view + validator/store directly and assert the rendering,
> additive-enrichment, precedence, and parity logic WITHOUT request
> interception — correctly classified `unit` (the one-validator/one-store
> + non-tool-capable-gather parity is a pure-mechanism guarantee provable
> in-repo).

### Definition of Done — SCOPE-07 (9 of 13 met + evidenced; the `check`/`format`/`artifact-lint` closeout gates + the live `e2e-api` parity legs are deferred — see report.md → SCOPE-07)

**Tier-1 (universal):**

- [ ] D07-T1-1 — `bash .github/bubbles/scripts/artifact-lint.sh specs/096-multi-provider-model-connections` clean. → Evidence: report.md → SCOPE-07.
- [ ] D07-T1-2 — `./smackerel.sh check` EXIT 0. → Evidence: report.md → SCOPE-07.
- [ ] D07-T1-3 — `./smackerel.sh format --check` EXIT 0. → Evidence: report.md → SCOPE-07.
- [x] D07-T1-4 — Every evidence block in report.md → SCOPE-07 is REAL terminal output (anti-fabrication). → Evidence: report.md → SCOPE-07.
- [x] D07-T1-5 — 088/089 do-not-amend boundary respected: selection extends the EXISTING validator/store/picker; NO second validator/store/picker is introduced. → Evidence: report.md → Change Manifest.

**Tier-2 (role-specific: 088/089 parity + additive enrichment (R2) + non-tool-capable gather (R3)):**

- [x] D07-T2-1 — **088/089 parity checkitem (binding, adversarial):** one-validator/one-store, the gather-vs-synthesis fork, and per-request > sticky > default precedence are preserved provider-agnostically — `TestParity_OneValidatorOneStore_ForkPrecedence_ProviderAgnostic_Spec096` FAILS if a second validator/store is introduced; `TestAgentModel_TelegramWebParity_SameValidatorSameStore_Spec096` proves both surfaces resolve through the same singletons. → Evidence: report.md → SCOPE-07 (parity test output).
- [x] D07-T2-2 — **R2 additive-enrichment preserves 089 clients (binding, adversarial):** `GET /v1/agent/model` gains `capabilities[]` + `cost_class` (+ optional MTD `budget`) additively while `allowed_models[]` is byte-for-byte preserved (ordering + provider-qualified strings) — `TestAgentModel_GetEnrichedCatalogAdditive_AllowedModelsPreserved_Spec096` fails if any `allowed_models` entry is renamed/reordered/removed. → Evidence: report.md → SCOPE-07.
- [x] D07-T2-3 — **R3 non-tool-capable gather (binding, adversarial):** a non-tool-capable model is shown-but-disabled for gather and a direct gather selection of it is rejected fail-loud (same `modelswitch.Rejection`) — `TestParity_NonToolCapableGatherShownDisabled_RejectedFailLoud_Spec096` fails if a non-tool-capable gather selection is ever accepted; the disable-vs-filter choice is justified (shown for transparency, disabled/rejected so a numbered reply never maps to a non-dispatchable gather selection), consistent with the unreachable-provider treatment. → Evidence: report.md → SCOPE-07.
- [x] D07-T2-4 — **Combined provider-grouped catalog:** the Telegram `/model` picker renders one provider-grouped, provider-qualified, cost-hinted list (Ollama/local first) with only reachable models numbered and an unreachable provider shown-but-disabled with its typed status (`TestModelPicker_TelegramCombinedProviderGroupedList_Spec096` + `TestModelPicker_UnreachableShownDisabledOnlyReachableNumbered_Spec096`); a numbered reply always maps to a dispatchable model. → Evidence: report.md → SCOPE-07.
- [x] D07-T2-5 — **Selection persistence + precedence (089 contract):** a hosted selection persists via the EXISTING claim-bound `modelpref` store and per-request > sticky > default precedence is byte-for-byte the 089 contract with provider-qualified ids only (`TestAgentModel_StickyHostedPersistsPrecedence_Spec096`). → Evidence: report.md → SCOPE-07.
- [ ] D07-T2-6 — **Live-stack parity + C7 deferral:** each `e2e-api` row (`TestTelegram_PickHostedModelPersists_Spec096`, `TestWebTelegramModelParity_Spec096`, `TestAsk_StickyHostedSelectionPersistsAcrossTurns_Spec096`, `TestForkPrecedenceParity_MultiProvider_Spec096`) hits the REAL system (Telegram↔web same result) and is handed to the home-lab `bubbles.devops` dispatch (C7), NOT marked passing from dev. → Evidence: report.md → SCOPE-07 (test run + deferral note).
- [x] D07-T2-7 — Each adversarial test is non-tautological with a captured RED-before (a build that introduces a second validator/store, breaks `allowed_models` byte-parity, or accepts a non-tool-capable gather selection would fail it); no bailout early-returns. → Evidence: report.md → SCOPE-07 RED-before.
- [x] D07-T2-8 — All `unit` SCOPE-07 tests pass with NO skips; the live cross-surface `e2e-api` parity legs are deferred to the home-lab dispatch (C7). → Evidence: report.md → SCOPE-07 (test run).

---

## Sequential Gate Summary (all seven scopes)

The plan is executable in strict order — scope N cannot start until scope
N-1 is fully Done (the lowest-numbered eligible scope is picked):

| Order | Scope | Depends on | Terminal gate before next |
|-------|-------|------------|---------------------------|
| 1 | SCOPE-01 — registry + SST | — | config-load/validate table GREEN |
| 2 | SCOPE-02 — credential vault | SCOPE-01 | AEAD + ephemeral-PG round-trip GREEN |
| 3 | SCOPE-03 — provider-aware dispatch | SCOPE-02 | parity + scrub + hosted-seam GREEN |
| 4 | SCOPE-04 — discovery + catalog | SCOPE-01 (+ SCOPE-03 seam) | aggregator + canonical + leaf-purity GREEN |
| 5 | SCOPE-05 — model-aware CostFn + budget | SCOPE-04 | cost-mapping + budget-refusal GREEN; 064/087 budget re-run |
| 6 | SCOPE-06 — operator admin surface | SCOPE-02, SCOPE-04 | operator-gate + write-only + truthful-test GREEN |
| 7 | SCOPE-07 — combined selection + parity | SCOPE-04, SCOPE-06 | one-validator/one-store + R2/R3 parity GREEN |

The plan is NOT executable and MUST NOT be transitioned to a terminal
status until all seven scopes above are Done and `state.json`
`certification.scopeProgress` reflects them. The live hosted-provider
`e2e-api` legs across SCOPE-03/04/06/07 are a SEPARATE downstream
`bubbles.devops` home-lab dispatch (C7) — `nextRequiredOwner` after
implement + test.
