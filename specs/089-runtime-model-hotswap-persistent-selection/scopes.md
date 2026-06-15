# Scopes 089 — Runtime Model Hot-Swap & Persistent Selection

> Planned by `bubbles.plan (parent-expanded full-delivery)`. Four
> sequential, scope-gated scopes. Each carries a Test Plan (Gherkin →
> concrete test) and a tiered DoD (Tier-1 universal + Tier-2
> role-specific). Scenario IDs: SCN-089-A01..A13 (see
> [spec.md](spec.md) §5 and [scenario-manifest.json](scenario-manifest.json)).
>
> **Extends, does NOT amend** spec 088. Every spec-064/084/087/088 trust
> and synthesis behaviour is preserved verbatim and made
> model-/selection-agnostic (design.md → Trust-Invariant Preservation
> Matrix). The design's **24-item change manifest**
> ([design.md](design.md) → Change Manifest) is partitioned across the
> four scopes below; the change numbers are cited per scope.
>
> **Terminal posture (C9).** All four scopes are validated-in-repo only.
> Dev has no GPU/Ollama daemon, so in-repo proof is mechanism-level
> (fake-LLM agent traces, pure-validator tables, store/handler/adapter
> tests) — exactly the spec-087/088 precedent. The decisive **live 32b
> standing-default** behaviour (the persist-48G + 32b default +
> pull-on-deploy + ensure-32b-resident deploy) was already quality-proven
> by the home-lab A/B
> ([`docs/experiments/open-knowledge-synthesis-model-ab.md`](../../docs/experiments/open-knowledge-synthesis-model-ab.md));
> the live re-verify is a SEPARATE downstream `bubbles.devops` dispatch.
> **No commit/push here.** `nextRequiredOwner` after plan = `bubbles.implement`;
> the ultimate downstream owner after in-repo implement+test+validate is
> `bubbles.devops` for the live deploy.

---

## Execution Outline

A reviewer can read this ~50-line outline and catch a wrong scope order
or a missing validation checkpoint BEFORE the full plan is implemented.

### Phase Order

1. **SCOPE-01 — SST: persistent 32b default + envelope guard + gather-capability set (foundation).**
   The committed-SST edits (home-lab `synthesis_model_id` 7b→32b,
   `ollama_memory_limit` 28G→48G, `switchable_models` += 32b, the NEW
   `tool_capable_gather_models` set), the config load/validate, the
   fail-loud **standing-default** co-residence guard (the CT-6 gap close),
   and the deploy contract. Pure config + Go config-layer; NO selection
   wiring yet. *(design CHANGE 1,2,3,4,21)*
2. **SCOPE-02 — Per-user sticky preference store (claim-bound).**
   Migration `059_user_model_preferences.sql` (actor-keyed PK) + the new
   `modelpref` leaf store (`Get`/`Set`/`Clear`). Claim-bound persistence
   primitive; NO surface, NO resolver yet. *(design CHANGE 8,9)*
3. **SCOPE-03 — Precedence resolver + gather override + attribution (agent/facade/api core spine).**
   The `modelswitch` extension (`Override{+GatherModel}`, `Effective`,
   `ResolveEffective`, `ResolveGather`, `model_not_tool_capable`), the
   `Agent.WithModelOverride` gather clone + `TurnResult.GatherModel` +
   `stripContractScaffolding`, the envelope source/gather fields, and the
   facade + HTTP `/ask` fast-path spine (read sticky → resolve precedence
   → validate → clone+run → attribute). Proven with fake-LLM traces +
   pure-validator tables + handler tests. *(design CHANGE 5,6,7,10,11,15,16,20a,22)*
4. **SCOPE-04 — Multi-surface affordances + parity + docs (concrete carriers).**
   The thin per-surface carriers — Telegram `/model` set/show/reset +
   `--gather-model=` parse + the source-tagged dual footer; HTTP
   `GET/PUT/DELETE /v1/agent/model` + the `gather_model` request field +
   rejection envelopes — plus the wiring of the `/model` command + the
   `AgentModelHandler`, and the operator hot-swap runbook. SCN-089-A11
   proves both surfaces run the SAME validator/store and render the SAME
   result. *(design CHANGE 12,13,14,17,18,19,20b,23)*

### New Types & Signatures (the C-header view)

- **`internal/assistant/openknowledge/modelswitch`** (EXTENDED):
  - `type Override struct { SynthesisModel string; GatherModel string }` (Fork C); `func (Override) IsZero() bool`
  - `const ( SourceDefault = "default"; SourceSticky = "sticky"; SourcePerRequest = "per_request" )`
  - `type Effective struct { SynthesisModel, SynthesisSource, GatherModel, GatherSource string }`; `func (Effective) Override() Override`
  - `func (a *Allowlist) ResolveEffective(perReqSynth, perReqGather, stickySynth string) (Effective, *Rejection)` — precedence per-request > sticky > default; per-winner validate; source classify; orphaned-sticky→default
  - `func (a *Allowlist) ResolveGather(raw string) (string, *Rejection)` — tool-capability membership check
  - `const ReasonNotToolCapable = "model_not_tool_capable"` + its message template; `Rejection` gains `RejectedTurn` (`synthesis|gather`)
  - `func NewAllowlist(switchable, profiles, envelopeMiB, gatherModel, defaultModel, toolCapableGather …)` — NEW `toolCapableGather` param
- **`internal/assistant/openknowledge/modelpref`** (NEW leaf pkg): `type Preference struct { SynthesisModel string; UpdatedAt time.Time }`; `type Store interface { Get(ctx, userID) (Preference, bool, error); Set(ctx, userID, synthesisModel) error; Clear(ctx, userID) error }`; `PostgresStore`
- **`agent.TurnResult.GatherModel string`** (NEW field); **`WithModelOverride(Override)`** now also sets `clone.cfg.Model` when `o.GatherModel != ""`; **`func stripContractScaffolding(text string) string`** (residual `<CITATIONS>` / contract-marker strip on the salvage arms)
- **`agenttool`**: `outputEnvelope` += `ModelSource`/`GatherModel`/`GatherModelSource`; `MapTurnResult` sets `Model`/`GatherModel`; `func WithSelection(env, eff) outputEnvelope` stamps the source fields
- **`contracts.AssistantMessage.GatherModelOverride string`** (NEW typed, untrusted); **`ModelAttribution`** += `SynthesisSource`/`GatherModel`/`GatherSource`/`GatherOverridden`
- **`api.AgentInvokeRequest.GatherModel string`** (NEW field); rejection envelope += `rejected_turn`; success envelope += `model_source`/`gather_model`/`gather_model_source`; NEW `GET/PUT/DELETE /v1/agent/model` claim-bound handlers
- **SQL** `internal/db/migrations/059_user_model_preferences.sql` — `actor_user_id TEXT PRIMARY KEY, synthesis_model TEXT NOT NULL, gather_model TEXT (reserved), updated_at TIMESTAMPTZ NOT NULL`
- **SST** `assistant.open_knowledge.tool_capable_gather_models: []string` (REQUIRED non-empty when enabled, `llm_model_id` MUST be a member) → env `ASSISTANT_OPEN_KNOWLEDGE_TOOL_CAPABLE_GATHER_MODELS`; home-lab `assistant_open_knowledge_synthesis_model_id: deepseek-r1:32b`, `ollama_memory_limit: "48G"`, `switchable_models += deepseek-r1:32b`, `tool_capable_gather_models: [gemma4:26b, llama3.1:8b]`

### Validation Checkpoints (where breakage is caught before the next scope)

- **After SCOPE-01** — `validateModelEnvelopes` standing-default over-envelope test + tool-capable-set load/validate tests GREEN; `./smackerel.sh config generate` (all envs) + `./smackerel.sh check` EXIT 0; the generated env carries `ASSISTANT_OPEN_KNOWLEDGE_TOOL_CAPABLE_GATHER_MODELS` and the home-lab `…SYNTHESIS_MODEL_ID=deepseek-r1:32b` + `OLLAMA_MEMORY_LIMIT=48G`. Catches a wrong co-residence arithmetic or an un-fail-loud SST key BEFORE any store/resolver code.
- **After SCOPE-02** — `modelpref` store tests (persist / claim-bound two-user isolation / idempotent clear) GREEN; `./smackerel.sh check` EXIT 0. Catches a leaky key or a non-idempotent reset BEFORE the resolver reads it on the hot path.
- **After SCOPE-03** — `modelswitch` resolver tables (precedence/source/gather/orphaned-sticky) + fake-LLM agent traces (gather clone / scaffolding strip / forced-final retry / trust-under-any-selection / latency-envelope) + substrate_tool + facade + agent_invoke tests GREEN; `./smackerel.sh check` + `./smackerel.sh format --check` EXIT 0. Catches a singleton mutation, precedence drift, attribution gap, or trust regression BEFORE surface wiring.
- **After SCOPE-04** — telegram `/model` + `--gather-model=` + footer + api `/v1/agent/model` + cross-surface parity tests GREEN; `./smackerel.sh check` + `format --check` EXIT 0; full `./smackerel.sh test unit --go` regression GREEN except the out-of-changeset spec-083/073 reds (attributed by file path). Catches a cross-surface divergence BEFORE the downstream live deploy.

---

## Scope Table

| # | Scope | Surfaces | Covers SCN (primary) | Reinforces | Tests (categories) | DoD items | Status |
|---|-------|----------|----------------------|------------|--------------------|-----------|--------|
| 1 | SCOPE-01 — SST default + envelope guard + gather-capability set (foundation) | config (yaml + Go) + deploy contract | A06 | A01, A07 | unit (config, envelope) + config-gen | 13 | Done |
| 2 | SCOPE-02 — Per-user sticky preference store (claim-bound) | migration + new `modelpref` leaf pkg | A02, A04 | A03 | unit (store, adversarial) | 12 | Done |
| 3 | SCOPE-03 — Precedence resolver + gather override + attribution (core spine) | modelswitch + agent + agenttool + contracts + facade + api `/ask` + wiring | A01, A05, A07, A08, A09, A10, A12 | A02, A04, A11 | unit (validator tables, fake-LLM agent) + functional (facade, handler) | 15 | Done |
| 4 | SCOPE-04 — Multi-surface affordances + parity + docs (concrete carriers) | telegram adapter + HTTP api + wiring + docs | A03, A11, A13 | A02, A04, A08, A12 | functional (adapter, handler) + unit (parity) + doc | 14 | Done |
| 5 | SCOPE-05 — Telegram `/model` numbered-picker selection (reply-with-number) | telegram adapter (new picker store + reply resolver) | A14 | A03, A04, A11 | unit (renderer, store, handler, adversarial) | 12 | Done |

Sequential gating: **SCOPE-02 cannot start until SCOPE-01 is fully done;
SCOPE-03 until SCOPE-02; SCOPE-04 until SCOPE-03; SCOPE-05 until SCOPE-04.**
The dependency ordering is real: the standing-default + tool-capable SST set
(SCOPE-01) is what `NewAllowlist`/`ResolveGather` validate against (SCOPE-03);
the sticky store (SCOPE-02) is what the precedence resolver reads on the
`/ask` hot path (SCOPE-03); the two surface carriers (SCOPE-04) parse into
the carrier fields and render the attribution/rejection produced by the
SCOPE-03 spine, and the `/model` CRUD writes the SCOPE-02 store; the numbered
picker (SCOPE-05) overlays the SCOPE-04 `/model` command, re-using the SAME
SCOPE-02 store + SCOPE-03 validator (one store, one validator) and never a
new security surface.

---

## Scope 1: SCOPE-01 — SST: persistent 32b default + envelope guard + gather-capability set (foundation)

**Status:** [x] Done
**Scope-Kind:** config + code (config layer)
**Depends on:** —
**Foundation:** true (DE4 — the cross-surface **allowlist + tool-capability
SST data + envelope arithmetic** the SCOPE-03 resolver and the SCOPE-04
carriers consume, never re-derive)

**Intent:** Land the committed-SST persistent-default promotion
(`deepseek-r1:32b`) with the raised envelope (48G), add `deepseek-r1:32b`
to the home-lab switchable set, introduce the NEW operator-curated
`tool_capable_gather_models` SST set, and close the CT-6 gap with an
explicit **standing-default** co-residence guard in
`validateModelEnvelopes` — all fail-loud (G028), so an over-envelope
standing default or an un-profiled/empty tool-capable set is rejected at
config-generation BEFORE any selection code exists to consume it.

### Surface (design CHANGE 1,2,3,4,21)

- `config/smackerel.yaml` — home-lab `environments.home-lab`:
  `ollama_memory_limit` `28G → 48G` (FR-2; gather 18432 + 32b 22528 =
  40960 ≤ 49152; A/B-verified 82/26 GiB no pressure);
  `assistant_open_knowledge_synthesis_model_id` `deepseek-r1:7b →
  deepseek-r1:32b` (FR-1; quality-first standing default);
  `assistant_open_knowledge_switchable_models` += `deepseek-r1:32b` (now
  envelope-fits at 48G; 7b stays the speed escape hatch); NEW
  `assistant_open_knowledge_tool_capable_gather_models:
  [ "gemma4:26b", "llama3.1:8b" ]`. Base `assistant.open_knowledge`: NEW
  `tool_capable_gather_models: [ "gemma3:4b" ]` (dev = baseline gather;
  testable no-op). REQUIRED non-empty when enabled; NEVER a silent
  default (G028).
- `internal/config/openknowledge.go` —
  `OpenKnowledgeConfig.ToolCapableGatherModels []string`; load via
  `lookupJSONStringList("ASSISTANT_OPEN_KNOWLEDGE_TOOL_CAPABLE_GATHER_MODELS")`
  (mirrors `SwitchableModels`); `Validate()` struct-level: non-empty list
  + each entry non-empty trimmed + `llm_model_id ∈ set` when enabled.
- `internal/config/config.go` — `validateModelEnvelopes`: (a) NEW
  **standing-default** co-residence guard — resolve the
  `synthesis_model_id` profile + the gather `llm_model_id` profile ≤
  `OllamaMemoryLimitMiB`, the same arithmetic the spec-088 switchable
  pass uses (closes the CT-6 gap: today the every-query default is the
  ONE large selection NOT envelope-checked); (b) each
  `tool_capable_gather_models` entry must have a `model_memory_profiles`
  entry (sanity). Fail-loud, naming the offending model + the envelope.
- `scripts/commands/config.sh` — resolve + emit
  `ASSISTANT_OPEN_KNOWLEDGE_TOOL_CAPABLE_GATHER_MODELS` via the per-env
  override pattern (mirror `switchable_models`).
- `deploy/contract.yaml` — NEW
  `assistant.open_knowledge.tool_capable_gather_models` path (type
  `string[]`, secret false, per-env override note).
- Tests: extend `internal/config/openknowledge_test.go`,
  `internal/config/validate_ml_envelope_test.go` (and the full-env maps in
  `internal/config/validate_test.go` / `spec_076_foundation_test.go` if
  the new key participates in those fixtures).

**Covers scenarios:** SCN-089-A06 (primary). Reinforces SCN-089-A01 (the
SST default is now `deepseek-r1:32b`, emitted by `config generate`) and
SCN-089-A07 (the `tool_capable_gather_models` set the gather guard
validates against is loaded + fail-loud here). Listed as supplementary
coverage in the Test Plan.

### Use Cases (Gherkin) — quoted from spec.md §5

```gherkin
Scenario: SCN-089-A06 — The standing default is envelope- and footprint-checked
  Given the persistent default synthesis model is a large reasoning model
  When the configuration is generated for the target environment
  Then the ollama memory envelope is raised to fit the gather model plus the standing synthesis model co-resident
  And the standing default is rejected fail-loud if it busts the declared envelope
  And the real-footprint headroom at the pipeline's context budget is verified safe co-resident with the ingestion pipeline

Scenario: SCN-089-A06 — An over-envelope standing default is refused at config generation
  Given a persistent default whose co-resident profile exceeds the environment ollama envelope
  When the configuration is generated
  Then config generation fails loud and names the offending model and the envelope
```

### Test Plan — SCOPE-01

| Scenario | Concrete test (file::function) | Category | Asserts |
|----------|--------------------------------|----------|---------|
| SCN-089-A06 | `internal/config/validate_ml_envelope_test.go::TestValidateModelEnvelopes_StandingDefaultOverEnvelopeRejected_Spec089` (ADVERSARIAL) | unit | A `synthesis_model_id` whose co-resident profile (default + gather) busts `OllamaMemoryLimitMiB` (e.g. `deepseek-r1:32b` 22528 + `gemma4:26b` 18432 = 40960 against a 28672 envelope) fails loud at config-generation, naming the offending model + the envelope. Fails if the standing default is silently accepted (the exact CT-6 gap). |
| SCN-089-A06 | `internal/config/validate_ml_envelope_test.go::TestValidateModelEnvelopes_StandingDefaultCoResidenceFits_Spec089` | unit | At `ollama_memory_limit: 48G` (49152) the same `deepseek-r1:32b` + `gemma4:26b` sum (40960 ≤ 49152) passes; the guard is not over-tight. |
| SCN-089-A07 (suppl.) | `internal/config/validate_ml_envelope_test.go::TestValidateModelEnvelopes_ToolCapableGatherEntryUnprofiledRejected_Spec089` (ADVERSARIAL) | unit | A `tool_capable_gather_models` entry with no `model_memory_profiles` entry fails loud. |
| SCN-089-A07 (suppl.) | `internal/config/openknowledge_test.go::TestOpenKnowledgeConfig_ToolCapableGatherModels_BaselineMemberRequired_Spec089` (ADVERSARIAL) | unit | An enabled config whose `llm_model_id` is NOT a member of `tool_capable_gather_models` fails loud (the no-override gather path must always pass); the home-lab `[gemma4:26b, llama3.1:8b]` with gather `gemma4:26b` passes. |
| SCN-089-A07 (suppl.) | `internal/config/openknowledge_test.go::TestOpenKnowledgeConfig_ToolCapableGatherModels_RequiredWhenEnabled_Spec089` | unit | Empty `tool_capable_gather_models` + enabled ⇒ fail-loud; missing `ASSISTANT_OPEN_KNOWLEDGE_TOOL_CAPABLE_GATHER_MODELS` env ⇒ fail-loud (mirrors the spec-088 `switchable_models` cases). |
| SCN-089-A01 (suppl.) | `internal/config/openknowledge_test.go::TestOpenKnowledgeConfig_HomeLabSynthesisDefaultIs32b_Spec089` | unit | The home-lab resolved `synthesis_model_id` is `deepseek-r1:32b` and `switchable_models` contains it; dev/test resolve the base values unchanged (NFR-4). |
| Regression E2E (A06) | `tests/e2e/agent/openknowledge_e2e_test.go` + `./smackerel.sh test e2e` (e2e-api) | e2e-api | The live `config generate --env home-lab` + boot path accepts the 48G/32b standing default and refuses an over-envelope edit; executed in the home-lab `bubbles.devops` re-verify dispatch (model+GPU-dependent, C9). |

### Definition of Done — SCOPE-01 (all unchecked — implementation pending)

**Tier-1 (universal):**

- [x] D01-T1-1 — `bash .github/bubbles/scripts/artifact-lint.sh specs/089-runtime-model-hotswap-persistent-selection` clean. → Evidence: report.md → SCOPE-01.
- [x] D01-T1-2 — `./smackerel.sh check` EXIT 0 (build + vet + config-sync + scenario-lint). → Evidence: report.md → SCOPE-01.
- [x] D01-T1-3 — `./smackerel.sh format --check` EXIT 0. → Evidence: report.md → SCOPE-01.
- [x] D01-T1-4 — No `${VAR:-default}` / hidden fallback introduced (G028 / `smackerel-no-defaults`); the new `tool_capable_gather_models` SST key + the promoted default + the raised envelope are REQUIRED + fail-loud; `config generate` is the sole emitter. → Evidence: report.md → SCOPE-01 (config generate + grep).
- [x] D01-T1-5 — Every evidence block in report.md → SCOPE-01 is REAL terminal output (anti-fabrication), including RED-before for the adversarial over-envelope case; no synthesized results; home-path PII redacted to `~/`. → Evidence: report.md → SCOPE-01.
- [x] D01-T1-6 — Do-not-touch boundary respected: zero changes under `internal/cardrewards/`, `ml/app/card_categories.py`, `ml/app/main.py`, `ml/tests/test_card_categories.py`, `specs/083-card-rewards-companion/`, `tests/integration/cardrewards_extract_test.go` (spec-083/073 WIP, C7). → Evidence: report.md → Change Manifest (isolated diff).
- [x] D01-T1-7 — Latency invariant restated unchanged: this scope adds no turns and no timeout knob; the documented `WriteTimeout = (max_iterations + synthesis_retry_budget) × llm_timeout_ms = (6+1)×600s = 4200s` is untouched (the 32b default changes typical, not max, latency — NFR-2). → Evidence: report.md → SCOPE-01.

**Tier-2 (role-specific: config + envelope guard):**

- [x] D01-T2-1 — The home-lab SST edits are correct + fail-loud: `synthesis_model_id = deepseek-r1:32b`, `ollama_memory_limit = 48G`, `switchable_models` contains `deepseek-r1:32b`, `tool_capable_gather_models = [gemma4:26b, llama3.1:8b]`; `./smackerel.sh config generate` (dev/test/home-lab) EXIT 0 with `ASSISTANT_OPEN_KNOWLEDGE_TOOL_CAPABLE_GATHER_MODELS` present in every generated env and the home-lab env carrying `…SYNTHESIS_MODEL_ID=deepseek-r1:32b` + `OLLAMA_MEMORY_LIMIT=48G`. → Evidence: report.md → SCOPE-01.
- [x] D01-T2-2 — The NEW standing-default co-residence guard in `validateModelEnvelopes` fails loud when the resolved `synthesis_model_id` + the gather `llm_model_id` profile sum busts `OllamaMemoryLimitMiB`, naming the model + the envelope; it passes at 48G for `deepseek-r1:32b` + `gemma4:26b` (40960 ≤ 49152) — closing the CT-6 gap (the every-query default is now envelope-checked, not only the switchable entries). → Evidence: report.md → SCOPE-01 (arithmetic + adversarial test).
- [x] D01-T2-3 — `tool_capable_gather_models` is SST + fail-loud: REQUIRED non-empty when enabled, each entry profiled, and the baseline gather `llm_model_id` MUST be a member (so the no-override gather path always passes); resolved via the `config.sh` per-env override pattern. → Evidence: report.md → SCOPE-01.
- [x] D01-T2-4 — `deploy/contract.yaml` carries the `assistant.open_knowledge.tool_capable_gather_models` path (type `string[]`, per-env override note); the contract stays generic (no real hostnames/IPs/secrets, C5). → Evidence: `deploy/contract.yaml` + report.md → SCOPE-01.
- [x] D01-T2-5 — The §2 footprint-headroom decision is recorded in the SST comment + carried to docs (SCOPE-04): the Docker `OLLAMA_MEMORY_LIMIT` cgroup cap is the real-KV bound (A/B-verified 82/26 GiB), the profile is NOT bumped, and F-FOOTPRINT (explicit `num_ctx` bound) is the deferred follow-up — so the standing 32b default cannot silently OOM the host at the pipeline's `per_query_token_budget` (128000). → Evidence: report.md → SCOPE-01 (comment + design cite).
- [x] D01-T2-6 — Each SCN-089-A06 test is non-tautological and ADVERSARIAL: it fails if `validateModelEnvelopes` ever accepts an over-envelope standing default or an un-profiled tool-capable entry; no bailout early-returns; proven by the RED-before run (guard temporarily neutralised). → Evidence: report.md → SCOPE-01 RED-before.

---

## Scope 2: SCOPE-02 — Per-user sticky preference store (claim-bound)

**Status:** [x] Done
**Scope-Kind:** code (migration + new `modelpref` leaf package)
**Depends on:** SCOPE-01
**Foundation:** false (the one NEW **persistence seam** — a claim-bound
per-user sticky store — consumed by the SCOPE-03 resolver hot-path read
and the SCOPE-04 `/model` CRUD, never re-implemented per surface)

**Intent:** Land the per-user sticky preference as genuinely new
per-user state (CT-10: no general per-user store exists today), keyed
ONLY on the authenticated `actor_user_id` (spec 044), with a cheap
single-PK read for the `/ask` hot path and an idempotent reset — so a
user's sticky model is private, persistent, and resettable, and a second
user can never read or write it.

### Surface (design CHANGE 8,9)

- `internal/db/migrations/059_user_model_preferences.sql` — NEW table
  (latest on disk is 058 ⇒ this is 059), actor-keyed PK, no DB-side
  defaults, with a ROLLBACK comment:
  ```sql
  CREATE TABLE IF NOT EXISTS user_model_preferences (
      actor_user_id   TEXT PRIMARY KEY,        -- claim-bound principal (spec 044); one row per user
      synthesis_model TEXT NOT NULL,           -- the sticky /ask synthesis model id
      gather_model    TEXT,                    -- RESERVED (nullable) for F-STICKY-GATHER; unread today
      updated_at      TIMESTAMPTZ NOT NULL     -- written by app code (no DB-side default)
  );
  ```
- `internal/assistant/openknowledge/modelpref/` — NEW leaf store package:
  `Preference{SynthesisModel string; UpdatedAt time.Time}`; `Store`
  interface (`Get` PK-lookup, `ok=false` ⇒ inherit default; `Set` upsert
  via `ON CONFLICT (actor_user_id) DO UPDATE`; `Clear` idempotent
  `DELETE`); `PostgresStore` over the shared `*sql.DB`. No cache (the read
  is one indexed row; don't over-engineer).
- Tests: NEW `internal/assistant/openknowledge/modelpref/store_test.go`
  (table-driven over a test DB / sqlmock per the repo's store-test
  precedent for migration-022 actor-keyed stores).

**Covers scenarios:** SCN-089-A02 (primary — the store persists the sticky
across reads), SCN-089-A04 (primary — the store is claim-bound; a second
user never reads/writes the first user's row). Reinforces SCN-089-A03 (the
`Clear` reset primitive surfaced by `/model default` in SCOPE-04).

### Use Cases (Gherkin) — quoted from spec.md §5

```gherkin
Scenario: SCN-089-A02 — A sticky /model <id> set persists across subsequent turns
  Given the user issues /model with an allowlisted model id
  When the user later runs two /ask invocations with no per-request override
  Then both invocations use the sticky model for the synthesis turn
  And the user is not required to repeat the selection on each turn
  And the SST persistent default is unchanged for every other user

Scenario: SCN-089-A04 — A sticky preference is claim-bound and never leaks across users
  Given user A has set a sticky model and user B has not
  When user B runs an /ask invocation
  Then user B's invocation uses the SST persistent default, not user A's sticky model
  # spoofed-actor arm proven at the HTTP handler in SCOPE-04
```

### Test Plan — SCOPE-02

| Scenario | Concrete test (file::function) | Category | Asserts |
|----------|--------------------------------|----------|---------|
| SCN-089-A02 | `internal/assistant/openknowledge/modelpref/store_test.go::TestModelPrefStore_GetAfterSet_PersistsAcrossReads_Spec089` | unit | `Set(userA, deepseek-r1:7b)` then two separate `Get(userA)` calls both return `{SynthesisModel: deepseek-r1:7b}, ok=true` — the sticky persists across reads with no re-set (the "no flag repeated" guarantee at the store level). |
| SCN-089-A02 | `internal/assistant/openknowledge/modelpref/store_test.go::TestModelPrefStore_Set_UpsertOnConflict_Spec089` | unit | A second `Set(userA, gemma4:26b)` overwrites the row (one row per user, `ON CONFLICT (actor_user_id) DO UPDATE`); `Get` returns the latest; `updated_at` advances. |
| SCN-089-A04 | `internal/assistant/openknowledge/modelpref/store_test.go::TestModelPrefStore_ClaimBound_UserBNeverReadsUserA_Spec089` (ADVERSARIAL) | unit | After `Set(userA, deepseek-r1:7b)`, `Get(userB)` returns `ok=false` (inherits default); `Set(userB, …)` never mutates userA's row. Fails if the store ever returns A's preference for B (a leaked key). |
| SCN-089-A03 (suppl.) | `internal/assistant/openknowledge/modelpref/store_test.go::TestModelPrefStore_Clear_IdempotentDelete_Spec089` | unit | `Clear(userA)` deletes the row; a subsequent `Get(userA)` ⇒ `ok=false` (default); a second `Clear(userA)` on an absent row is a no-op (no error) — the `/model default` reset primitive. |
| SCN-089-A04 | `internal/assistant/openknowledge/modelpref/store_test.go::TestModelPrefStore_GatherModelColumnReservedUnread_Spec089` | unit | The `gather_model` column exists (migration shape) but `Get` does not surface it (reserved for F-STICKY-GATHER; the resolver does not read it). Fails if a sticky gather is silently activated. |
| Regression E2E (A02/A04) | `tests/e2e/agent/openknowledge_e2e_test.go` + `./smackerel.sh test e2e` (e2e-api) | e2e-api | The live `/ask` sticky persists per user across turns and never leaks across users; executed in the home-lab `bubbles.devops` re-verify dispatch (C9). |

### Definition of Done — SCOPE-02 (all unchecked — implementation pending)

**Tier-1 (universal):**

- [x] D02-T1-1 — `bash .github/bubbles/scripts/artifact-lint.sh specs/089-runtime-model-hotswap-persistent-selection` clean. → Evidence: report.md → SCOPE-02.
- [x] D02-T1-2 — `./smackerel.sh check` EXIT 0. → Evidence: report.md → SCOPE-02.
- [x] D02-T1-3 — `./smackerel.sh format --check` EXIT 0. → Evidence: report.md → SCOPE-02.
- [x] D02-T1-4 — No `${VAR:-default}` / hidden fallback; the migration has no DB-side default (`updated_at` written by app code); the store never invents a model id (a missing row ⇒ `ok=false` ⇒ inherit SST default, never a hardcoded fallback). → Evidence: report.md → SCOPE-02.
- [x] D02-T1-5 — Every report.md → SCOPE-02 evidence block is REAL terminal output, including RED-before for the claim-bound adversarial case; home-path PII redacted to `~/`. → Evidence: report.md → SCOPE-02.
- [x] D02-T1-6 — Do-not-touch boundary respected (spec-083/073, C7). → Evidence: report.md → Change Manifest.
- [x] D02-T1-7 — Latency invariant restated unchanged: the store adds one indexed PK read per `/ask`, no turns, no timeout knob; `WriteTimeout` stays `4200s` (the PK read is negligible against a multi-minute `/ask`, NFR-2). → Evidence: report.md → SCOPE-02.

**Tier-2 (role-specific: store + claim-binding + test integrity):**

- [x] D02-T2-1 — Migration `059_user_model_preferences.sql` is the next sequential migration (058 is latest on disk), actor-keyed PK (`actor_user_id TEXT PRIMARY KEY`), `synthesis_model TEXT NOT NULL`, `gather_model TEXT` reserved nullable, `updated_at TIMESTAMPTZ NOT NULL`, ROLLBACK comment present; `./smackerel.sh` migrate path applies + reverts cleanly. → Evidence: report.md → SCOPE-02.
- [x] D02-T2-2 — `modelpref.Store` is a single shared claim-bound capability: `Get` is one PK lookup (`ok=false` ⇒ inherit default), `Set` upserts (one row per user), `Clear` is an idempotent delete; keyed ONLY on the passed `actor_user_id` (the claim-bound principal threaded by the surfaces, never a body field). → Evidence: report.md → SCOPE-02.
- [x] D02-T2-3 — Claim-binding is provable at the store layer (FR-5 / spec 044 / OWASP A01): `Get(userB)` never returns `userA`'s preference; `Set(userB,…)` never mutates `userA`'s row (SCN-089-A04 store arm; the spoofed-body arm lands at the HTTP handler in SCOPE-04). → Evidence: report.md → SCOPE-02 RED→GREEN.
- [x] D02-T2-4 — The reserved `gather_model` column is present but unread (F-STICKY-GATHER forward-compat); no sticky gather is activated by this scope; the resolver (SCOPE-03) does not read it. → Evidence: report.md → SCOPE-02.
- [x] D02-T2-5 — Each SCN-089-A02/A04 test is non-tautological with an ADVERSARIAL case that fails if the behavior regresses (a leaked key returning A's pref for B; a non-idempotent clear; a silently-activated sticky gather); no bailout early-returns; proven by the RED-before run. → Evidence: report.md → SCOPE-02 RED-before.

---

## Scope 3: SCOPE-03 — Precedence resolver + gather override + attribution (agent/facade/api core spine)

**Status:** [x] Done
**Scope-Kind:** code (modelswitch + agent + agenttool + contracts + facade + api + wiring)
**Depends on:** SCOPE-02
**Foundation:** false (completes the shared capability **spine** —
precedence resolution + per-invocation config construction + answer
attribution — that the SCOPE-04 per-surface carriers consume; the SST
allowlist it validates against is the SCOPE-01 foundation, the sticky
store it reads is the SCOPE-02 seam)

**Intent:** Build the single precedence resolver (`ResolveEffective`:
per-request > sticky > SST default) + the gather tool-capability guard
(`ResolveGather`) in the pure `modelswitch` leaf, extend the per-request
clone to re-point BOTH gather and synthesis turns (singleton never
mutated, C6), stamp model + gather-model + source attribution, strengthen
the `<CITATIONS>` scaffolding strip on the salvage arms, and wire the
read→resolve→validate→clone→run→attribute spine into BOTH `/ask`
fast-paths (facade + HTTP) — with every spec-064/084/087/088 trust
contract running unchanged under any selection.

### Surface (design CHANGE 5,6,7,10,11,15,16,20a,22)

- `internal/assistant/openknowledge/modelswitch/allowlist.go` —
  `Override{+GatherModel}` (Fork C; `IsZero` updated); `Effective` +
  `Source{Default,Sticky,PerRequest}` consts + `Effective.Override()`;
  `ResolveEffective(perReqSynth, perReqGather, stickySynth)` (precedence;
  per-winner validate; source classify; a synthesis reject does NOT fall
  through to sticky/default; an orphaned sticky — operator-retired —
  resolves to default + a structured log, never breaking every `/ask`);
  `ResolveGather(raw)` (membership in `tool_capable_gather_models`);
  `ReasonNotToolCapable = "model_not_tool_capable"` + its message
  template; `Rejection.RejectedTurn` (`synthesis|gather`); `toolCapableGather`
  field + the NEW `NewAllowlist` param. Pure leaf (no store, no backend).
- `internal/assistant/openknowledge/agent/agent.go` — `TurnResult.GatherModel
  string` (the gather model that ran, `= a.cfg.Model` at finalize, stamped
  once beside the existing `Model`); `WithModelOverride(o)` also sets
  `clone.cfg.Model = o.GatherModel` when non-empty (Fork C; singleton
  `Model`/`SynthesisModel` never mutated, C6); NEW
  `stripContractScaffolding(text)` applied on the salvage body arms (the
  missing-CITATIONS salvage + the empty-citations/honest-salvage bodies)
  BEFORE `finalize`, removing any residual `<CITATIONS>…</CITATIONS>`, a
  stray unterminated `<CITATIONS>`, and the `<one synthesized answer…>`
  marker (FR-13; `<think>` already stripped pre-parse, CT-8 — unchanged).
- `internal/assistant/openknowledge/agenttool/substrate_tool.go` —
  `outputEnvelope` += `ModelSource`/`GatherModel`/`GatherModelSource`
  (`json:",omitempty"` except `model`/`model_source` which the HTTP
  envelope ALWAYS carries); `MapTurnResult` sets `Model`/`GatherModel`
  from the turn; NEW `WithSelection(env, eff)` stamps the source fields
  (the caller knows the `Effective` it resolved — source is a resolver
  concept, not a turn concept).
- `internal/assistant/contracts/message.go` — NEW typed
  `AssistantMessage.GatherModelOverride string` (untrusted; validated
  before use; beside the spec-088 `ModelOverride`).
- `internal/assistant/contracts/response.go` — `ModelAttribution` +=
  `SynthesisSource`/`GatherModel`/`GatherSource`/`GatherOverridden`;
  update `response_test.go` field inventory.
- `internal/assistant/facade.go` — open_knowledge fast-path: read
  `sticky, ok, _ := f.modelPref.Get(ctx, msg.UserID)` (claim-bound by
  CT-3; nil store ⇒ `stickySynth=""` ⇒ default path, never a panic),
  `eff, rej := allow.ResolveEffective(msg.ModelOverride,
  msg.GatherModelOverride, sticky)`; on `rej != nil` build a rejection
  `AssistantResponse` and SKIP the agent + assembler + provenance +
  capture (pre-agent request validation — Rejection ≠ capture-skip); else
  `runOpenKnowledgeDirect(ctx, sc, env, emittedAt, eff.Override())` and
  stamp the extended `ModelAttribution` from `turn.Model`/`turn.GatherModel`
  + the `eff` sources. `runOpenKnowledgeDirect` takes the full `Override`
  (synthesis + gather).
- `internal/api/agent_invoke.go` — NEW `AgentInvokeRequest.GatherModel
  string json:"gather_model,omitempty"`; fast-path reads `subject :=
  auth.UserIDFromContext(r.Context())` (claim-bound by CT-2) + `sticky :=
  h.ModelPref.Get(...)`, calls `ResolveEffective`, clone+run; the
  rejection envelope += `rejected_turn`; the success envelope +=
  `model_source`/`gather_model`/`gather_model_source` (always present
  since gather always runs).
- `cmd/core/wiring_assistant_openknowledge.go` + api wiring (CHANGE 20a) —
  pass `okCfg.ToolCapableGatherModels` to `modelswitch.NewAllowlist`;
  construct + inject the `modelpref` store into the facade + the api
  Dependencies (so the `/ask` hot-path read works); add the
  `tool_capable_gather_models` + store-wired lines to the boot log
  (`open-knowledge subsystem wired … synthesis_model=<id>`).
- `cmd/core/main.go` (CHANGE 22) — comment-only: `WriteTimeout` stays
  `4200s`; the 32b default + a gather override add no turns (NFR-2
  honest). No value change.
- Tests: NEW `internal/assistant/openknowledge/agent/modelswitch_agent_spec089_test.go`,
  `internal/assistant/facade_modelswitch_spec089_test.go`; extend
  `internal/assistant/openknowledge/modelswitch/allowlist_test.go`,
  `internal/assistant/openknowledge/agenttool/substrate_tool_test.go`,
  `internal/assistant/contracts/response_test.go`,
  `internal/api/agent_invoke_test.go`.

**Covers scenarios:** SCN-089-A01, A05, A07, A08, A09, A10, A12 (primary).
Reinforces SCN-089-A02 / A04 (the resolver consumes the SCOPE-02 sticky)
and SCN-089-A11 (the shared resolver both SCOPE-04 surfaces apply).

### Use Cases (Gherkin) — quoted from spec.md §5

```gherkin
Scenario: SCN-089-A05 — A per-request override beats a sticky preference; sticky beats the SST default
  Given the user has a sticky model preference
  And the user supplies a different allowlisted per-request override on one /ask
  When that invocation runs
  Then the per-request override wins for that invocation only
  And the user's sticky preference is unchanged for the next invocation
  # and: sticky beats default when no per-request override is supplied

Scenario: SCN-089-A07 — The gather model is overridable but must stay tool-calling-capable
  Given the user selects an allowlisted, tool-calling-capable gather model
  When the open-knowledge agent runs the gather turns
  Then the gather turns use the selected gather model
  And a non-tool-capable gather selection is rejected before any gather turn runs

Scenario: SCN-089-A01 — The committed-SST persistent default is applied with no selection
  Given no sticky and no per-request override
  When the open-knowledge agent runs the /ask invocation
  Then the agent uses the committed-SST persistent default synthesis model
  And the answer is attributed to the persistent-default model with selection source "default"

Scenario: SCN-089-A09 — No <think> / <CITATIONS> scaffolding leaks into the user body under any model
  Given any selected synthesis model emits <think> reasoning and/or <CITATIONS> scaffolding
  When the agent finalizes the answer
  Then the <think> content is stripped before citation parsing
  And no <think> or <CITATIONS> scaffolding appears in the user-visible body

Scenario: SCN-089-A10 — A blank forced-final synthesis is rescued by retry-before-salvage
  Given the persistent default (or selected) synthesis model returns an empty forced-final answer
  When the agent finalizes
  Then the escalated retry is issued up to synthesis_retry_budget times
  And only if every retry is still empty does the honest snippet salvage fire
  And the user never receives a silently-empty answer
```

### Test Plan — SCOPE-03

| Scenario | Concrete test (file::function) | Category | Asserts |
|----------|--------------------------------|----------|---------|
| SCN-089-A05 | `internal/assistant/openknowledge/modelswitch/allowlist_test.go::TestAllowlist_ResolveEffective_PrecedencePerRequestOverStickyOverDefault_Spec089` (ADVERSARIAL) | unit | `ResolveEffective(perReq, sticky, default)` returns the per-request synthesis when supplied (source `per_request`); the sticky when only sticky supplied (source `sticky`); the SST default otherwise (source `default`). Fails if precedence inverts or a source is mis-tagged. |
| SCN-089-A05 | `internal/assistant/openknowledge/modelswitch/allowlist_test.go::TestAllowlist_ResolveEffective_OrphanedStickyFallsToDefault_Spec089` | unit | A sticky model the operator has retired from `switchable_models` resolves to the SST default (source `default`) — never breaks every `/ask` for that user; a per-request reject does NOT fall through (explicit refusal). |
| SCN-089-A07 | `internal/assistant/openknowledge/modelswitch/allowlist_test.go::TestAllowlist_ResolveGather_ToolCapableApplied_NonCapableRejected_Spec089` (ADVERSARIAL) | unit | A tool-capable gather (`gemma4:26b`/`llama3.1:8b`) resolves; a non-tool-capable gather (`deepseek-r1:7b`) ⇒ `Rejection{ReasonCode: model_not_tool_capable, RejectedTurn: gather}` naming the tool-capable set. Fails if a weak-tool model is accepted for gather. |
| SCN-089-A08 | `internal/assistant/openknowledge/modelswitch/allowlist_test.go::TestAllowlist_ResolveEffective_OffAllowlistByteIdenticalContract_Spec089` (parity seam) | unit | The SAME off-allowlist string ⇒ a byte-identical `Rejection` every call — the one shared contract both SCOPE-04 surfaces render verbatim (design Multi-Surface Parity table). |
| SCN-089-A01 | `internal/assistant/openknowledge/agent/modelswitch_agent_spec089_test.go::TestAgent_NoSelection_UsesSstDefaultSynthesis_ByteForByteBaseline_Spec089` (ADVERSARIAL) | unit | A fake LLM records every turn's model; a zero `Effective.Override()` ⇒ `WithModelOverride` returns the receiver; gather turns use `llm_model_id`, forced-final uses the SST `synthesis_model_id`; the per-turn sequence equals the spec-087/088 baseline exactly (now pointing at the 32b default). Fails if the default path diverges. |
| SCN-089-A07 | `internal/assistant/openknowledge/agent/modelswitch_agent_spec089_test.go::TestAgent_WithModelOverride_GatherClonePointsCfgModel_SingletonUnmutated_Spec089` (ADVERSARIAL) | unit | Under a gather override the gather turns use the overridden `cfg.Model` and the synthesis turn resolves by precedence; after the clone the singleton `cfg.Model`/`cfg.SynthesisModel` are byte-for-byte unchanged (C6). Fails if the override leaks onto the singleton or the wrong turn. |
| SCN-089-A12 | `internal/assistant/openknowledge/agent/modelswitch_agent_spec089_test.go::TestAgent_TurnResultModelAndGatherModelStamped_AllTerminalPaths_Spec089` | unit | `TurnResult.Model` (answering model) + `TurnResult.GatherModel` are stamped once in `finalize` across success / honest-salvage / refuse / early-`StopEndTurn` (honest per CT-4). |
| SCN-089-A09 | `internal/assistant/openknowledge/agent/modelswitch_agent_spec089_test.go::TestAgent_StripContractScaffolding_NoCitationsLeakInSalvageBody_Spec089` (ADVERSARIAL) | unit | A salvage-arm body containing a residual `<CITATIONS>` fragment / `<one synthesized answer…>` marker is stripped before `finalize`; neither `<think>` nor `<CITATIONS>` reaches the body and neither becomes a citation, under any model. Fails if the scaffolding survives into the user body. |
| SCN-089-A10 | `internal/assistant/openknowledge/agent/modelswitch_agent_spec089_test.go::TestAgent_ForcedFinalEmpty_EscalatedRetryThenHonestSalvage_Spec089` (REGRESSION / ADVERSARIAL) | unit | The 32b-Q6 shape: an empty forced-final ⇒ exactly one escalated retry (`synthesis_retry_budget=1`) ⇒ still-empty ⇒ honest salvage WITH sources; never a silently-empty body. Fails if the retry is skipped or a blank surfaces. |
| SCN-089-A05/A08 (trust) | `internal/assistant/openknowledge/agent/modelswitch_agent_spec089_test.go::TestAgent_TrustContractsHoldUnderAnySelection_Spec089` (ADVERSARIAL, 3 sub-cases) | unit | Under default / sticky / per-request: (a) a fabricated citation ⇒ canonical refusal (post-`<think>`-strip cite-back enforce); (b) zero-source ⇒ provenance refusal + capture-as-fallback fires; (c) `<think>` never in body / never cited. |
| SCN-089-A08 | `internal/assistant/facade_modelswitch_spec089_test.go::TestFacade_OffAllowlistSelection_ShortCircuits_NoAgentCall_NoCapture_Spec089` (ADVERSARIAL) | functional | A rejected selection (synthesis OR gather, per-request OR sticky-set) short-circuits: the agent is NEVER called and capture is NOT invoked (pre-agent validation). Fails if a rejection still runs the agent or captures. |
| SCN-089-A01/A12 | `internal/assistant/facade_modelswitch_spec089_test.go::TestFacade_BareDefault_NoFooter_AttributesModelSourceDefault_Spec089` | functional | A bare `/ask` (no sticky, no override) ⇒ `runOpenKnowledgeDirect` with a zero `Override`; `ModelAttribution` reports the 32b default with source `default`; no footer is implied (NFR-4 / Principle 6). |
| SCN-089-A12 | `internal/assistant/openknowledge/agenttool/substrate_tool_test.go::TestMapTurnResult_ModelAndGatherCarried_WithSelectionStampsSources_Spec089` | unit | `MapTurnResult` sets `outputEnvelope.Model`/`GatherModel` on success AND refusal arms; `WithSelection(env, eff)` stamps `model_source`/`gather_model_source`; `model`+`model_source` are always present. |
| SCN-089-A07/A08 | `internal/api/agent_invoke_test.go::TestAgentInvoke_GatherModelField_EnvelopeCarriesGatherSource_AndNonCapableRejected_Spec089` (ADVERSARIAL) | functional | The `gather_model` field threads through `ResolveGather`→clone→run and the envelope carries `gather_model`/`gather_model_source`; a non-tool-capable `gather_model` ⇒ HTTP 400 with `error_code:"model_not_tool_capable"`, `rejected_turn:"gather"`, no agent call. |
| SCN-089-A01 | `internal/api/agent_invoke_test.go::TestAgentInvoke_BareDefault_EnvelopeModel32bSourceDefault_Spec089` | functional | A bare invoke ⇒ envelope `model:"deepseek-r1:32b"`, `model_source:"default"`, `gather_model:"gemma4:26b"`, `gather_model_source:"default"`. |
| — (contracts) | `internal/assistant/contracts/response_test.go::TestModelAttribution_FieldInventory_GatherSource_Spec089` | unit | The `ModelAttribution` inventory includes `SynthesisSource`/`GatherModel`/`GatherSource`/`GatherOverridden`; a zero value ⇒ baseline (no attribution surfaced). |
| — (latency) | `internal/assistant/openknowledge/agent/modelswitch_agent_spec089_test.go::TestAgent_AnySelection_PreservesIterationEnvelope_Spec089` | unit | `WithModelOverride` changes only `Model`/`SynthesisModel`; `MaxIterations` and `SynthesisRetryBudget` (the `WriteTimeout` inputs) are unchanged ⇒ the documented `(6+1)×600s = 4200s` is preserved (SCN-089 NFR-2). |
| — (regression) | `internal/assistant/openknowledge/agent/reasoning_loop_spec084_test.go` + `synthesis_spec087_test.go` + the spec-088 suite | unit | The 9 spec-084 + 7 spec-087 agent tests + the spec-088 suite stay GREEN unchanged: a zero selection is byte-for-byte the spec-088 path (C8). |
| Regression E2E (A01/A05/A07/A09/A10) | `tests/e2e/agent/openknowledge_e2e_test.go` + `./smackerel.sh test e2e` (e2e-api) | e2e-api | The live `/ask` precedence + gather override + scaffolding-clean + retry-rescued answer holds the trust perimeter under any selection; executed in the home-lab `bubbles.devops` re-verify dispatch (C9). |

### Definition of Done — SCOPE-03 (all unchecked — implementation pending)

**Tier-1 (universal):**

- [x] D03-T1-1 — `bash .github/bubbles/scripts/artifact-lint.sh specs/089-runtime-model-hotswap-persistent-selection` clean. → Evidence: report.md → SCOPE-03.
- [x] D03-T1-2 — `./smackerel.sh check` EXIT 0. → Evidence: report.md → SCOPE-03.
- [x] D03-T1-3 — `./smackerel.sh format --check` EXIT 0. → Evidence: report.md → SCOPE-03.
- [x] D03-T1-4 — No `${VAR:-default}` / hidden fallback; no new runtime default; `WithModelOverride` clones and never mutates the SST singleton (C6 / FR-15); a nil store / nil allowlist ⇒ baseline passthrough, never a panic. → Evidence: report.md → SCOPE-03.
- [x] D03-T1-5 — Every report.md → SCOPE-03 evidence block is REAL terminal output, including RED-before for the adversarial subset (precedence inversion, gather leak, scaffolding survival, forced-final blank, trust weakening); home-path PII redacted to `~/`. → Evidence: report.md → SCOPE-03.
- [x] D03-T1-6 — Do-not-touch boundary respected (spec-083/073, C7). → Evidence: report.md → Change Manifest.
- [x] D03-T1-7 — Latency invariant restated unchanged: `cmd/core/main.go` comment confirms the 32b default + a gather override add no turns and `WriteTimeout` stays `4200s`; the 32b default changes typical, not max, latency (SCN-089 NFR-2 / C4). → Evidence: report.md → SCOPE-03 + `cmd/core/main.go`.

**Tier-2 (role-specific: resolver + agent/facade/api core + trust + test integrity):**

- [x] D03-T2-1 — `ResolveEffective` applies precedence deterministically (per-request > sticky > SST default), validates each winning model, classifies the source (`default`/`sticky`/`per_request`), and never falls a per-request reject through to sticky/default; an orphaned sticky resolves to default + a structured log (SCN-089-A05). → Evidence: report.md → SCOPE-03 GREEN-after.
- [x] D03-T2-2 — `ResolveGather` rejects a non-tool-capable gather (`model_not_tool_capable`, `rejected_turn: gather`) BEFORE any gather turn runs; a tool-capable gather is applied; the baseline `llm_model_id` (a member, validated in SCOPE-01) always passes (SCN-089-A07 / FR-8). → Evidence: report.md → SCOPE-03.
- [x] D03-T2-3 — `WithModelOverride` is a per-request clone re-pointing BOTH gather (`cfg.Model`) and synthesis (`cfg.SynthesisModel`) only when supplied; the SST singleton is never written; a zero override returns the receiver so the no-selection path is byte-for-byte spec-088 (SCN-089-A01 / C6 / FR-15 / NFR-4). → Evidence: report.md → SCOPE-03.
- [x] D03-T2-4 — Attribution is honest + complete: `TurnResult.Model`/`GatherModel` stamped once in `finalize` across all terminal paths; the envelope always carries `model`+`model_source` and (gather always runs) `gather_model`+`gather_model_source`; `ModelAttribution` carries the source tags + the dual-gather flag (SCN-089-A12 / FR-11 / Principle 8). → Evidence: report.md → SCOPE-03.
- [x] D03-T2-5 — Trust invariants hold under EVERY selection + source: post-`<think>`-strip cite-back refuses a fabricated citation; the provenance gate refuses zero-source; capture-as-fallback fires on every path where the agent runs; a selection rejection is pre-agent validation (no agent run, no capture) — Rejection ≠ capture-skip (SCN-089-A08 sibling / C1). → Evidence: report.md → SCOPE-03 RED→GREEN.
- [x] D03-T2-6 — Output hygiene: `stripContractScaffolding` removes any residual `<CITATIONS>` / contract marker from the salvage arms under any model; the happy cited-synthesis path is unchanged; `<think>` stays stripped pre-parse (SCN-089-A09 / FR-13). → Evidence: report.md → SCOPE-03.
- [x] D03-T2-7 — Forced-final reliability verified: an empty forced-final fires exactly `synthesis_retry_budget` escalated retries before the honest snippet salvage; never a silently-empty body (SCN-089-A10 / FR-14; the 32b-Q6 shape). → Evidence: report.md → SCOPE-03.
- [x] D03-T2-8 — The facade + HTTP fast-paths read the sticky claim-bound (`msg.UserID` / `auth.UserIDFromContext`), resolve via the SAME `ResolveEffective`, and are nil-safe (nil store ⇒ default path); the wiring injects the store + passes `ToolCapableGatherModels` to `NewAllowlist` + names them in the boot log. Agent-level tests use fake LLMs and are `unit`; the live 32b-default quality is the downstream `bubbles.devops` re-verify (cite the A/B evidence doc, not claimed here). Each SCN test is non-tautological + ADVERSARIAL with a RED-before; no bailout early-returns. → Evidence: report.md → SCOPE-03 RED-before + A/B cite.

---

## Scope 4: SCOPE-04 — Multi-surface affordances + parity + docs (concrete carriers)

**Status:** [x] Done
**Scope-Kind:** code (telegram adapter + HTTP api + wiring) + docs
**Depends on:** SCOPE-03
**Foundation:** false (the thin per-surface **carriers** — concrete
implementations overlaying the SCOPE-01 SST, the SCOPE-02 store, and the
SCOPE-03 spine; DE4 Variation Axes: surface composition, selection
persistence, turn target, rejection transport)

**Intent:** Add the two thin per-surface carriers that expose the full
selection capability identically — Telegram (`/model` set/show/reset +
`--gather-model=` parse + the source-tagged dual footer) and web/HTTP
(`GET/PUT/DELETE /v1/agent/model` + the `gather_model` request field +
the rejection envelopes) — wire the `/model` command + the
`AgentModelHandler`, and document the operator hot-swap runbook.
SCN-089-A11 proves both surfaces run the SAME validator + the SAME store
and render the SAME `Effective`/`Rejection`.

### Surface (design CHANGE 12,13,14,17,18,19,20b,23)

- `internal/telegram/assistant_adapter/translate_inbound.go` —
  `parseModelFlag` ALSO consumes a leading `--gather-model=<id>` token
  (order-independent with `--model=`; slash preserved); set
  `msg.GatherModelOverride`. ollama tags have no whitespace ⇒ a single
  token is the whole value.
- `internal/telegram/assistant_adapter/render_outbound.go` —
  `appendModelFooter`: source tags (`your default` / `this question`);
  the single form `— model: <id> (<source>)` when only the synthesis
  selection is non-default; the dual form `— gather: <g> (<gsrc>) · synth:
  <s> (<ssrc>)` whenever a gather override is active; NO footer on a pure
  system-default answer (Principle 6 / NFR-4 — byte-for-byte baseline).
- `internal/telegram/bot.go` + NEW `internal/telegram/model_command.go` —
  `case "model"` dispatch (beside `case "ask"`) → resolve
  `resolveActorUserID(chatID)` → claim-bound `/model` set/show/reset via
  the shared `modelpref.Store` + the shared `modelswitch` validator + a
  shared discovery/confirm/reject renderer (NOT an agent run).
- `internal/api/agent_model.go` — NEW `GET/PUT/DELETE /v1/agent/model`
  claim-bound handlers (mirror `annotation_list.go::subject :=
  auth.UserIDFromContext(...)`); `GET` shows effective+allowed+default;
  `PUT {model}` sets (off-allowlist ⇒ 400, preference UNCHANGED); `DELETE`
  resets. The body NEVER carries a user id.
- `internal/api/router.go` — mount `/agent/model` GET/PUT/DELETE in the
  SAME bearer-auth `/v1` group as `/agent/invoke` (CT-2).
- `internal/api/health.go` (`Dependencies`) — add the `modelpref.Store` +
  the `AgentModelHandler` deps (the store is shared with the SCOPE-03
  `/ask` read; the handler is new).
- `cmd/core/wiring_*` (CHANGE 20b) — wire the telegram `/model` command +
  the api `AgentModelHandler` to the SAME store installed in SCOPE-03.
- `docs/Operations.md` — the hot-swap runbook (Fork D ~15s core-recreate:
  edit SST → `config generate --env home-lab` → operator overlay recreates
  core `--no-deps` → verify via the boot log `synthesis_model=<new>` + a
  live `/ask` envelope `model_source: default`), the `/model` sticky
  affordance, the `--gather-model=` override, the precedence+source table,
  and the standing-default footprint note (A/B headroom + the cgroup-cap
  real-KV bound).
- Tests: extend `internal/telegram/assistant_adapter/translate_inbound_test.go`
  + `render_outbound_test.go`; NEW `internal/telegram/model_command_test.go`,
  `internal/api/agent_model_test.go`; a cross-surface parity test.

**Covers scenarios:** SCN-089-A03, A11, A13 (primary). Reinforces
SCN-089-A02 (sticky two-turn end-to-end on a surface), A04 (the spoofed-body
HTTP arm), A08 (surface rejection parity), A12 (the footer source tags).

### Use Cases (Gherkin) — quoted from spec.md §5

```gherkin
Scenario: SCN-089-A03 — /model shows current + allowed; /model default resets
  Given the user has a sticky model selection
  When the user issues /model with no argument
  Then the reply shows the user's current selection and the allowed switchable set and the SST default
  # reset arm: /model default clears the sticky preference; next /ask uses the SST default again

Scenario: SCN-089-A11 — Telegram + web/HTTP expose sticky + per-request + gather identically
  Given the same allowlisted sticky selection, per-request override, and gather selection
  When each is exercised on the Telegram surface and on the web/HTTP surface
  Then every surface resolves them through the same modelswitch validator
  And the applied model(s), the rejection shape, and the attribution are identical across surfaces

Scenario: SCN-089-A13 — Hot-swap the standing model in prod via config
  Given a new standing default synthesis model that is envelope- and footprint-verified
  When the operator follows the documented hot-swap procedure
  Then the standing default changes for subsequent /ask invocations
  And the trust perimeter and every other behavior are unchanged
  And the procedure's downtime characteristic is the one documented for the chosen mechanism
```

### Test Plan — SCOPE-04

| Scenario | Concrete test (file::function) | Category | Asserts |
|----------|--------------------------------|----------|---------|
| SCN-089-A03 | `internal/telegram/model_command_test.go::TestModelCommand_ShowListsEffectiveAllowedAndDefault_Spec089` | functional | `/model` (no arg) replies with the caller's effective model marked active, the full switchable set, the system default, and whether the choice is sticky-set (`your default`) or inherited (`system default`). |
| SCN-089-A03 | `internal/telegram/model_command_test.go::TestModelCommand_ResetClearsStickyAndConfirms_Spec089` | functional | `/model default` (or `/model reset`) calls `modelpref.Clear`, confirms the revert; a subsequent bare `/ask` resolves the SST default. |
| SCN-089-A03 | `internal/api/agent_model_test.go::TestAgentModel_GetShowsEffective_DeleteResets_Spec089` | functional | `GET /v1/agent/model` returns `{effective_model, source, sticky_model, system_default, allowed_models}`; `DELETE` clears and returns `source:"default"`. |
| SCN-089-A04 | `internal/api/agent_model_test.go::TestAgentModel_Put_BodyUserIdIgnored_ClaimBoundToSubject_Spec089` (ADVERSARIAL) | functional | A `PUT /v1/agent/model` whose body attempts a different user id sets/reads ONLY the PASETO subject's preference; the body id is ignored (OWASP A01). Fails if a spoofed body id reaches the store key. |
| SCN-089-A11 | `internal/api/agent_model_test.go::TestParity_SameStickyAndOffAllowlist_IdenticalAcrossSurfaces_Spec089` (ADVERSARIAL) | unit | The SAME allowlisted sticky + the SAME off-allowlist string drive the SAME `modelswitch` validator + the SAME store on Telegram and HTTP; the applied `Effective` + the `Rejection.Message` are byte-identical across surfaces. Fails if either surface diverges. |
| SCN-089-A11 | `internal/telegram/assistant_adapter/translate_inbound_test.go::TestTranslateInbound_GatherModelFlagParsedAndStripped_SlashPreserved_Spec089` (ADVERSARIAL) | functional | `/ask --gather-model=llama3.1:8b --model=deepseek-r1:7b <q>` ⇒ `msg.GatherModelOverride == "llama3.1:8b"`, `msg.ModelOverride == "deepseek-r1:7b"`, `Text` is the clean question (slash preserved, both tokens removed, order-independent); a bare `/ask` ⇒ both empty. Fails if a flag leaks into the question. |
| SCN-089-A12 | `internal/telegram/assistant_adapter/render_outbound_test.go::TestRenderOutbound_FooterSourceTagsAndDualGatherForm_Spec089` (ADVERSARIAL) | functional | Non-default synthesis ⇒ `— model: <id> (<source>)`; an active gather override ⇒ the dual `— gather: … · synth: …` form; a pure system-default answer ⇒ NO footer. Fails if a baseline answer grows a footer (NFR-4) or an override answer loses it. |
| SCN-089-A08 | `internal/api/agent_model_test.go::TestAgentModel_Put_OffAllowlist_400_PreferenceUnchanged_Spec089` (ADVERSARIAL) | functional | A `PUT` with an off-allowlist model ⇒ HTTP 400 with the rejection envelope (same `Rejection.Message` as Telegram), and the caller's existing sticky preference is UNCHANGED (the failed set is a no-op). |
| SCN-089-A13 | `cmd/core/wiring_assistant_openknowledge_test.go::TestWiring_BootLogNamesSynthesisModelAndToolCapableSet_Spec089` | unit | The open-knowledge wiring boot log names the resolved `synthesis_model=<id>` + the `tool_capable_gather_models` + the store-wired line — the operator's hot-swap verification hook (the runbook reads this log). |
| Regression E2E (A03/A11/A13) | `tests/e2e/agent/openknowledge_e2e_test.go` + `./smackerel.sh test e2e` (e2e-api) | e2e-api | The live `/model` CRUD + `--gather-model=` + the hot-swap recreate behave identically on Telegram + HTTP and the boot log names the new default; executed in the home-lab `bubbles.devops` re-verify dispatch (C9). |

### Definition of Done — SCOPE-04 (all unchecked — implementation pending)

**Tier-1 (universal):**

- [x] D04-T1-1 — `bash .github/bubbles/scripts/artifact-lint.sh specs/089-runtime-model-hotswap-persistent-selection` clean. → Evidence: report.md → SCOPE-04.
- [x] D04-T1-2 — `./smackerel.sh check` EXIT 0. → Evidence: report.md → SCOPE-04.
- [x] D04-T1-3 — `./smackerel.sh format --check` EXIT 0. → Evidence: report.md → SCOPE-04.
- [x] D04-T1-4 — No `${VAR:-default}` / hidden fallback; the `/model` + `/v1/agent/model` surfaces never invent a model id (an off-allowlist set is a no-op rejection, the existing preference is preserved). → Evidence: report.md → SCOPE-04.
- [x] D04-T1-5 — Every report.md → SCOPE-04 evidence block is REAL terminal output, including the full `./smackerel.sh test unit --go` regression with out-of-changeset spec-083/073 reds attributed by file path (anti-fabrication); home-path PII redacted to `~/`. → Evidence: report.md → SCOPE-04 + Regression.
- [x] D04-T1-6 — Do-not-touch boundary respected (spec-083/073, C7). → Evidence: report.md → Change Manifest.
- [x] D04-T1-7 — Latency invariant restated unchanged: `docs/Operations.md` records the unchanged `WriteTimeout = 4200s`, the 32b-default ~1.9× typical-latency expectation (not a max change), and the deferred `synthesis_retry_budget` 1→2 operator knob (F-RETRYBUDGET). → Evidence: report.md → SCOPE-04 + `docs/Operations.md`.

**Tier-2 (role-specific: multi-surface parity + rendering + claim-binding + docs):**

- [x] D04-T2-1 — Telegram `/model` set/show/reset is a claim-bound CRUD (resolves `resolveActorUserID`, NOT an agent run): show lists effective+allowed+default with the sticky/inherited tag; set validates via `modelswitch` (off-allowlist ⇒ rejection, preference unchanged); reset calls `Clear` (SCN-089-A03). → Evidence: report.md → SCOPE-04.
- [x] D04-T2-2 — HTTP `GET/PUT/DELETE /v1/agent/model` mirror the Telegram CRUD, mounted in the bearer-auth `/v1` group, keyed on `auth.UserIDFromContext` (the body NEVER carries a user id); a spoofed body id is ignored (SCN-089-A04 HTTP arm / OWASP A01). → Evidence: report.md → SCOPE-04 RED→GREEN.
- [x] D04-T2-3 — `--gather-model=` is parsed off the `/ask` line into `msg.GatherModelOverride` (order-independent with `--model=`, slash preserved, clean question); the HTTP `gather_model` field threads through `ResolveGather` (the SCOPE-03 spine) (SCN-089-A11). → Evidence: report.md → SCOPE-04.
- [x] D04-T2-4 — The footer renders the source tags + the dual gather form ONLY when a synthesis OR gather selection is non-default; a pure system-default `/ask` is byte-for-byte spec-087/088 with NO footer (SCN-089-A12 / Principle 6 / NFR-4). → Evidence: report.md → SCOPE-04.
- [x] D04-T2-5 — Multi-surface parity proven: the SAME sticky + the SAME off-allowlist string drive the SAME validator + the SAME store and render the SAME `Effective`/`Rejection.Message` on both surfaces (SCN-089-A11 / A08 / FR-10). → Evidence: report.md → SCOPE-04 (parity test + golden message).
- [x] D04-T2-6 — `docs/Operations.md` documents the Fork-D ~15s core-recreate hot-swap (with the boot-log + live-envelope verification), the `/model` sticky + `--gather-model=` affordances, the precedence+source table, and the standing-default footprint note (cgroup-cap real-KV bound, A/B headroom); the boot-log assertion test pins the verification hook (SCN-089-A13). → Evidence: `docs/Operations.md` + report.md → SCOPE-04.
- [x] D04-T2-7 — Full `./smackerel.sh test unit --go` regression: spec-089 + spec-088 + spec-087 + spec-084 + spec-064 open-knowledge tests GREEN; only the out-of-changeset spec-083 WIP / spec-073 env reds remain, attributed by file path (finding F-ENV-083). Each SCN-089 surface test is non-tautological + ADVERSARIAL (spoofed body id; flag leak; baseline footer growth; cross-surface divergence) with a RED-before; no bailout early-returns. → Evidence: report.md → Regression + SCOPE-04 RED-before.

---

## Scope 5: SCOPE-05 — Telegram `/model` numbered-picker selection (reply-with-number)

**Status:** [x] Done
**Scope-Kind:** code (telegram adapter — new per-chat picker store + reply resolver)
**Depends on:** SCOPE-04
**Foundation:** false (a thin **carrier overlay** on the SCOPE-04 `/model`
command; re-uses the SCOPE-02 store + the SCOPE-03 validator unchanged — one
store, one validator, no new security surface; DE4 Variation Axes: selection
affordance shape — numbered pick vs explicit id)

**Intent:** Make `/model` (no arg) render the switchable set as a 1-indexed
NUMBERED list and arm a per-chat pending selection; a subsequent bare-number
reply selects that model and writes the sticky synthesis preference — re-
validated against the SAME `modelswitch` allowlist and written through the
SAME claim-bound `modelpref` store the SCOPE-04 `/model <id>` path uses
(SCN-089-A11 parity). The reply resolver mirrors `handleDisambiguationReply`
and the pending store mirrors `disambiguationStore` (per-chat, `sync.Mutex`,
TTL). Claim-binding is preserved exactly (the write keys ONLY on
`resolveActorUserID(chatID)`); the existing `/model <id>` set and
`/model default`/`reset` behavior is unchanged.

### Surface

- `internal/telegram/model_command.go` — NEW `modelPickerReply(ctx, allow,
  store, userID) (string, []string)` pure renderer (numbered list marking
  current + system default, returns the ordered id list); `handleModelCommand`
  no-arg branch now renders the picker AND arms the pending selection. The
  explicit-id set + `default`/`reset` paths (`modelCommandReply`) are unchanged.
- NEW `internal/telegram/model_selection.go` — `pendingModelSelection` +
  `modelSelectionStore` (thread-safe per-chat store, `set`/`get`/`clear`, TTL,
  mirrors `disambiguationStore`); `handleModelSelectionReply(ctx, msg) bool`
  (catch a bare-number reply, bounds-check, re-validate via the shared
  allowlist, claim-bound `Set` via `resolveActorUserID`, clear, confirm).
- `internal/telegram/bot.go` — NEW `modelSelections *modelSelectionStore`
  field, wired in `NewBot` (reusing `cfg.DisambiguationTimeoutSeconds`, no new
  SST key); a "Priority 2.6" routing block calls `handleModelSelectionReply`
  AFTER the annotation/cook disambiguation resolvers and BEFORE the
  cook-nav/servings triggers, so a non-model-pending numeric reply still
  reaches the other flows.
- Tests: NEW `internal/telegram/model_selection_test.go`.

**Covers scenarios:** SCN-089-A14 (primary). Reinforces SCN-089-A03 (the
`/model` discovery surface), A04 (claim-bound sticky write), A11 (one store +
one validator across affordances).

### Use Cases (Gherkin) — quoted from spec.md §5

```gherkin
Scenario: SCN-089-A14 — /model shows a NUMBERED list; a reply-with-number selects, claim-bound
  Given the user issues /model with no argument on the Telegram surface
  When the reply is rendered
  Then the switchable models are shown as a 1-indexed numbered list in a stable order
  And the user's current effective model and the system default are marked
  And a per-chat pending selection holding the ordered list shown is armed
  # select arm: an in-range number reply re-validates the picked id against the
  #   shared allowlist and sets the sticky pref claim-bound to the resolved actor
  # don't-hijack: a number with NO armed picker is not consumed (falls through)
  # change-nothing: out-of-range re-prompts; an off-allowlist (stale) pick is
  #   rejected fail-loud; neither changes the stored preference
```

### Test Plan — SCOPE-05

| Scenario | Concrete test (file::function) | Category | Asserts |
|----------|--------------------------------|----------|---------|
| SCN-089-A14 | `internal/telegram/model_selection_test.go::TestModelPickerReply_NumberedListMarksCurrentAndDefault_Spec089` | unit | `/model` (no arg) renders a 1-indexed numbered list in the stable switchable order, tags the caller's effective model "current" and the SST default "system default" (both on a model that is both), and returns the ordered id list so number N → models[N-1]. |
| SCN-089-A14 | `internal/telegram/model_selection_test.go::TestModelSelectionStore_SetGetClear_Spec089` + `…_Expiry_Spec089` | unit | The per-chat pending store sets/gets/clears the armed ordered list and expires it after the TTL (mirrors `disambiguationStore`). |
| SCN-089-A14 | `internal/telegram/model_selection_test.go::TestHandleModelSelectionReply_ValidPickSetsStickyForResolvedActor_Spec089` | unit | An in-range number selects `models[N-1]` and SETS the sticky pref for the resolved actor, confirms, and clears the picker. |
| SCN-089-A14 / A04 | `internal/telegram/model_selection_test.go::TestHandleModelSelectionReply_ClaimBoundToResolvedActor_Spec089` (ADVERSARIAL) | unit | The selection binds to `resolveActorUserID(chatID)`, NEVER to the message `From.ID`; a reply whose `From.ID` differs sets ONLY the resolved actor's pref (OWASP A01). Fails if `From.ID` becomes the key. |
| SCN-089-A14 | `internal/telegram/model_selection_test.go::TestHandleModelSelectionReply_OffAllowlistStalePending_RejectsPrefUnchanged_Spec089` (ADVERSARIAL) | unit | A stale armed list whose entry is off the current allowlist is re-validated and refused fail-loud; the existing pref is UNCHANGED, nothing reaches the backend. Fails if an off-allowlist id is set. |
| SCN-089-A14 | `internal/telegram/model_selection_test.go::TestHandleModelSelectionReply_OutOfRange_RepromptsPrefUnchanged_Spec089` (ADVERSARIAL) | unit | An out-of-range number re-prompts with the valid range, sets NO pref, and keeps the picker armed. Fails if an out-of-range index is dereferenced or a pref is written. |
| SCN-089-A14 | `internal/telegram/model_selection_test.go::TestHandleModelSelectionReply_NoArmedPicker_FallsThrough_Spec089` (ADVERSARIAL) | unit | A bare number with NO armed picker falls through (returns false), proving it never hijacks a number meant for the annotation/cook disambiguation or servings flows. |
| SCN-089-A14 | `internal/telegram/model_selection_test.go::TestHandleModelSelectionReply_NonNumberReply_FallsThrough_Spec089` + `…_ExpiredPending_FallsThrough_Spec089` | unit | A non-number reply, or a bare number against an EXPIRED pending selection, falls through (returns false) and is never resolved as a selection. |
| Regression (A03/A14) | `internal/telegram/model_command_test.go::TestModelCommand_*_Spec089` + `assistant_adapter/*_test.go::Test*Model*_Spec088` + full `./smackerel.sh test unit --go` | unit | The existing `/model` show/set/reset, the spec-088 footer/flag tests, and the full unit suite stay GREEN (no regression). |

### Definition of Done — SCOPE-05

**Tier-1 (universal):**

- [x] D05-T1-1 — `bash .github/bubbles/scripts/artifact-lint.sh specs/089-runtime-model-hotswap-persistent-selection` clean. → Evidence: report.md → SCOPE-05.
- [x] D05-T1-2 — `./smackerel.sh check` EXIT 0. → Evidence: report.md → SCOPE-05.
- [x] D05-T1-3 — `./smackerel.sh format --check` EXIT 0. → Evidence: report.md → SCOPE-05.
- [x] D05-T1-4 — No `${VAR:-default}` / hidden fallback; the pending picker is in-memory per-chat runtime state (NO new SST key, reuses `cfg.DisambiguationTimeoutSeconds`); the picker never invents a model id (an off-allowlist pick is a fail-loud rejection, the existing preference is preserved). → Evidence: report.md → SCOPE-05.
- [x] D05-T1-5 — Every report.md → SCOPE-05 evidence block is REAL terminal output (RED-before for the adversarial subset, GREEN-after), including the full `./smackerel.sh test unit --go` regression; home-path PII redacted to `~/`. → Evidence: report.md → SCOPE-05 + Regression.
- [x] D05-T1-6 — Do-not-touch boundary respected (spec-083/073, C7) — `git status --porcelain` on the forbidden paths is EMPTY. → Evidence: report.md → SCOPE-05.
- [x] D05-T1-7 — Latency invariant unchanged: the picker adds NO agent turns (it is a per-user CRUD affordance, not an `/ask` run); `WriteTimeout = 4200s` is untouched. → Evidence: report.md → SCOPE-05.

**Tier-2 (role-specific: numbered picker + claim-binding + don't-hijack):**

- [x] D05-T2-1 — `/model` (no arg) renders a 1-indexed numbered list marking the caller's current effective model + the system default, AND arms a per-chat pending selection holding the ORDERED id list shown (so number N → models[N-1]); the explicit `/model <id>` set + `/model default`/`reset` paths are unchanged (SCN-089-A14 show arm). → Evidence: report.md → SCOPE-05.
- [x] D05-T2-2 — A bare in-range number reply selects `models[N-1]`, RE-VALIDATES it against the shared `modelswitch` allowlist (defense in depth), and SETS the sticky synthesis preference CLAIM-BOUND to `resolveActorUserID(chatID)` — NEVER a message/text-supplied id; proven by an ADVERSARIAL claim-binding test with a RED-before (keyed on `From.ID` → breach) (SCN-089-A14 select arm / SCN-089-A04 / OWASP A01). → Evidence: report.md → SCOPE-05 RED→GREEN.
- [x] D05-T2-3 — `handleModelSelectionReply` returns false (falls through) UNLESS THIS chat has an armed, unexpired pending selection AND the text is a number; a bare number with no armed picker is NOT swallowed (don't-hijack); it is routed AFTER the annotation/cook disambiguation resolvers so a non-model-pending numeric reply still reaches the other flows (SCN-089-A14 don't-hijack). → Evidence: report.md → SCOPE-05.
- [x] D05-T2-4 — An out-of-range number re-prompts with the valid range and KEEPS the picker armed; an off-allowlist (stale armed list) pick renders the verbatim fail-loud rejection; an expired pending selection falls through — each with the stored preference UNCHANGED and nothing sent to the backend; proven non-tautological by RED-before on the re-validation + bounds guards (SCN-089-A14 change-nothing). → Evidence: report.md → SCOPE-05 RED→GREEN.
- [x] D05-T2-5 — No regression: the existing spec-089 `/model` show/set/reset tests, the spec-088 footer/flag tests, and the full `./smackerel.sh test unit --go` suite stay GREEN; the numbered picker re-uses the SAME `agenttool.SwitchableModels()` validator + `agenttool.ModelPref()` store as the `/model <id>` path and the HTTP `/v1/agent/model` surface (one validator, one store, SCN-089-A11) (SCN-089-A14 / A03 / A11). → Evidence: report.md → SCOPE-05 + Regression.

---

## Out-of-Changeset / Do-Not-Touch (owner directive C7)

Do NOT modify or "fix": `internal/cardrewards/`,
`ml/app/card_categories.py`, `ml/app/main.py`,
`ml/tests/test_card_categories.py`, `specs/083-card-rewards-companion/`,
`tests/integration/cardrewards_extract_test.go`. Whole-working-tree guard
failures attributable to the spec-083 card-rewards WIP and the spec-073
node/dart container canary are OUT of this changeset — attribute by file
path; do not remediate here (finding F-ENV-083, inherited from spec
084/087/088).

## Terminal Posture (C9) — validated-in-repo, NO commit/push

This plan ships and validates the persistent-selection + sticky +
gather-override + hot-swap **primitive** in-repo only (mechanism-level
proof: pure-validator tables, claim-bound store tests, fake-LLM agent
traces, handler/adapter parity tests). Dev has no GPU/Ollama daemon. The
decisive live 32b standing-default quality was already proven by the
home-lab A/B
(`docs/experiments/open-knowledge-synthesis-model-ab.md`); the live deploy
(persist 48G + the 32b default + pull-on-deploy + ensure-32b-resident +
the live re-verify) is a SEPARATE downstream `bubbles.devops` dispatch.
After implement + test complete and all four scopes are validated-in-repo,
`nextRequiredOwner = bubbles.implement` for the next phase, and the
ultimate downstream owner after in-repo implement+test+validate is
`bubbles.devops` for the live deploy. **No commit, no push in this spec's
run.**
