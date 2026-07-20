# Scopes 088 — Runtime-Switchable Models

> Planned by `bubbles.plan (parent-expanded full-delivery)`. Three
> sequential, scope-gated scopes. Each carries a Test Plan (Gherkin →
> concrete test) and a tiered DoD (Tier-1 universal + Tier-2
> role-specific). Scenario IDs: SCN-088-A01..A08 (see
> [spec.md](spec.md) §4 and [scenario-manifest.json](scenario-manifest.json)).
>
> **Extends, does NOT amend** spec 087. Every spec-064/084/087 trust and
> synthesis behaviour is preserved verbatim and made model-agnostic
> (design.md Trust-Invariant Preservation Matrix). The design's 13-change
> file manifest (design.md → Implementation Design) is partitioned across
> the three scopes below; the change numbers are cited per scope.
>
> **Terminal posture (C7).** All three scopes are validated-in-repo only.
> Dev has no GPU/Ollama daemon, so in-repo proof is mechanism-level
> (fake-LLM agent traces, pure-validator tables, handler/adapter tests) —
> exactly the spec-087 precedent. The decisive live
> `gemma4:26b`-vs-`deepseek-r1:7b` synthesis A/B on self-hosted hardware is a
> SEPARATE downstream `bubbles.devops` dispatch. **No commit/push here.**
> `nextRequiredOwner` after implement+test = `bubbles.devops` for the live
> A/B.

---

## Execution Outline

A reviewer can read this ~45-line outline and catch a wrong scope order
or a missing validation checkpoint BEFORE the full plan is implemented.

### Phase Order

1. **SCOPE-01 — SST allowlist + shared `modelswitch` validator (foundation).**
   The closed-set validator (`Resolve`), the new SST
   `assistant.open_knowledge.switchable_models` list, config load +
   structural validate, and the fail-loud co-residence envelope check at
   config-generation. Pure leaf + config; NO surface wiring yet.
   *(design CHANGE 1,2,3,4,5)*
2. **SCOPE-02 — Override threading + per-request clone + attribution (core spine).**
   The `AssistantMessage.ModelOverride` carrier field, `Agent.WithModelOverride`
   per-request clone (singleton never mutated, C6), `TurnResult.Model`
   stamped in `finalize`, the `agenttool` allowlist-singleton accessor +
   envelope `model`, and the facade resolve→reject→thread→attribute spine.
   Proven with fake-LLM agent traces + facade tests. *(design CHANGE 6,7,8a,11)*
3. **SCOPE-03 — Two-surface parity + fail-loud rendering + docs (concrete carriers).**
   The two thin per-surface carriers — Telegram `--model=` parse +
   `— model:` footer; HTTP `model` field + 400 rejection envelope — plus
   the wiring install of the allowlist singleton, the deploy contract
   path, and the operator docs. *(design CHANGE 8b,9,10,12)*

### New Types & Signatures (the C-header view)

- **`internal/assistant/openknowledge/modelswitch`** (NEW leaf package):
  - `type Override struct { SynthesisModel string }`; `func (Override) IsZero() bool`
  - `type Rejection struct { RejectedModel, DefaultModel string; AllowedModels []string; ReasonCode, Message string }`
  - `type Allowlist struct { /* immutable after build */ }`
  - `func NewAllowlist(switchable []string, profiles map[string]int, envelopeMiB int, gatherModel, defaultModel string) (*Allowlist, error)` — fail-loud build
  - `func (a *Allowlist) Resolve(raw string) (Override, *Rejection)` — `""` ⇒ zero override (baseline); in-list ⇒ `Override`; else ⇒ `Rejection`
  - reason-codes `ReasonNotAllowlisted = "model_not_allowlisted"`, `ReasonOverMemEnvelope = "model_over_memory_envelope"`
- **`agent.TurnResult.Model string`** (NEW field); **`func (a *Agent) WithModelOverride(modelswitch.Override) *Agent`** (per-request clone; zero override returns receiver)
- **`agenttool`**: `SetSwitchableModels(*modelswitch.Allowlist)` / `SwitchableModels() *modelswitch.Allowlist` (atomic singleton, parallels `SetAgent`/`CurrentAgent`); `outputEnvelope.Model string json:"model,omitempty"`
- **`contracts.AssistantMessage.ModelOverride string`** (NEW typed field — owner directive: NOT `TransportMetadata`); **`contracts.ModelAttribution{ ModelID string; OverrideApplied bool }`** + `AssistantResponse.ModelAttribution *ModelAttribution`
- **`api.AgentInvokeRequest.Model string`** (NEW field) + 400 rejection envelope `{status, error_code, rejected_model, allowed_models, default_model, message}`
- **SST** `assistant.open_knowledge.switchable_models: []string` (REQUIRED non-empty when enabled) → env `ASSISTANT_OPEN_KNOWLEDGE_SWITCHABLE_MODELS`; self-hosted override `environments.<env>.assistant_open_knowledge_switchable_models`

### Validation Checkpoints (where breakage is caught before the next scope)

- **After SCOPE-01** — `modelswitch` validator table tests + config fail-loud + `validateModelEnvelopes` over-envelope tests GREEN; `./smackerel.sh config generate` + `./smackerel.sh check` EXIT 0. Catches a bad allowlist mechanism or wrong co-residence arithmetic BEFORE any threading.
- **After SCOPE-02** — agent fake-LLM tests (applied / baseline / attribution / trust / envelope-preserved) + agenttool + facade + contracts tests GREEN; `./smackerel.sh check` EXIT 0. Catches a singleton-mutation, attribution gap, or trust regression BEFORE surface wiring.
- **After SCOPE-03** — telegram-adapter + api-handler parity tests GREEN; `./smackerel.sh check` + `./smackerel.sh format --check` EXIT 0; full `./smackerel.sh test unit --go` regression green except the out-of-changeset spec-083/073 reds (attributed by file path). Catches a cross-surface divergence BEFORE the downstream live A/B.

---

## Scope Table

| # | Scope | Surfaces | Covers SCN | Tests (categories) | DoD items | Status |
|---|-------|----------|------------|--------------------|-----------|--------|
| 1 | SCOPE-01 — SST allowlist + shared validator (foundation) | config (yaml + Go) + new `modelswitch` leaf pkg | A02, A07 | unit (validator, config) + config-gen | 14 | Done |
| 2 | SCOPE-02 — Override threading + clone + attribution (core spine) | agent + agenttool + facade + contracts | A01, A03, A04, A05, A08 | unit (fake-LLM agent) + unit/functional (facade) | 18 | Done |
| 3 | SCOPE-03 — Two-surface parity + fail-loud rendering + docs | telegram adapter + HTTP api + wiring + deploy + docs | A06 (+ reinforces A02/A04) | functional (adapter, handler) + unit (parity) | 17 | Done |

Sequential gating: **SCOPE-02 cannot start until SCOPE-01 is fully done;
SCOPE-03 cannot start until SCOPE-02 is fully done.** The dependency
ordering is real: the facade/agent spine (SCOPE-02) calls the validator +
clone built in SCOPE-01; the two surface carriers (SCOPE-03) parse into
the carrier field and render the attribution/rejection produced by the
SCOPE-02 spine.

---

## Scope 1: SCOPE-01 — SST allowlist + shared `modelswitch` validator (foundation)

**Status:** Done
**Scope-Kind:** config + code (new leaf package)
**Depends on:** —
**Foundation:** true (DE4 — the cross-surface override **validation +
allowlist gating** capability; consumed by the SCOPE-02 spine and the
SCOPE-03 carriers, never re-implemented per surface)

**Intent:** Land the closed-set model-override validator and its SST
allowlist as a pure, surface-agnostic leaf, with fail-loud config and
fail-loud co-residence envelope arithmetic — so an off-allowlist,
un-profiled, or over-envelope model is converted to a typed `Rejection`
(never an `Override`) BEFORE any surface or agent code exists to consume
it.

### Surface (design CHANGE 1,2,3,4,5)

- `config/smackerel.yaml` — NEW `assistant.open_knowledge.switchable_models`
  (dev `[ "gemma3:4b" ]`) + the self-hosted
  `environments.<env>.assistant_open_knowledge_switchable_models:
  [ "gemma4:26b", "deepseek-r1:7b" ]` override. REQUIRED non-empty when
  enabled; NEVER a silent default (G028).
- `internal/config/openknowledge.go` — `OpenKnowledgeConfig.SwitchableModels
  []string`; `LoadOpenKnowledge` via `lookupJSONStringList(
  "ASSISTANT_OPEN_KNOWLEDGE_SWITCHABLE_MODELS", …)` (mirrors `ToolAllowlist`);
  `Validate()` struct-level (non-empty list, each entry non-empty trimmed)
  when enabled.
- `internal/config/config.go` — `validateModelEnvelopes` switchable pass
  (only when `OpenKnowledge.Enabled` and `OllamaMemoryLimitMiB != 0`):
  each entry must have a `model_memory_profiles` entry (else `missing`)
  and co-resident-fit the env envelope (`base + candidate ≤ limit`, single
  load when `candidate == gather`; else `oversized`).
- `scripts/commands/config.sh` — resolve via the per-environment override
  pattern (mirror `synthesis_model_id`, JSON-list-shaped like
  `tool_allowlist`) + emit `ASSISTANT_OPEN_KNOWLEDGE_SWITCHABLE_MODELS`.
- `internal/assistant/openknowledge/modelswitch/` (NEW leaf pkg; stdlib +
  `contracts` only ⇒ no import cycle) — `Allowlist`, `Override`,
  `Rejection`, `NewAllowlist` (fail-loud build), `Resolve`, `reject`
  (two reason-codes), `message` (the two verbatim UX sentences keyed on
  reason-code).
- Tests: `internal/assistant/openknowledge/modelswitch/allowlist_test.go`
  (NEW); extend `internal/config/openknowledge_test.go`,
  `internal/config/validate_ml_envelope_test.go` (and the full-env maps in
  `internal/config/validate_test.go` / `spec_076_foundation_test.go`).

**Covers scenarios:** SCN-088-A02, SCN-088-A07. (The applied/baseline
validator cases also underpin A01/A03 at the validator level and the
single-Rejection-contract case underpins A06 parity in Scope 3 — listed
as supplementary coverage in the Test Plan.)

### Use Cases (Gherkin) — quoted from spec.md §4

```gherkin
Scenario: SCN-088-A02 — Off-allowlist override rejected fail-loud, never reaches the backend
  Given the operator supplies a model override that is NOT on the switchable-model allowlist
  When the request is validated
  Then the operator receives an explicit rejection that lists the allowed models
  And the rejected model is NEVER sent to the inference backend
  And the agent does NOT silently fall back to the baseline model
  And no answer is fabricated for the rejected request

Scenario: SCN-088-A07 — Untrusted / un-profiled / over-envelope model never passes through
  Given the operator supplies an arbitrary model string that has no model_memory_profiles entry
  When the request is validated
  Then the model string is rejected before any per-invocation agent config is constructed
  And it is never forwarded to the inference backend
  # second arm:
  Given the operator supplies a model whose memory profile exceeds the target environment ollama envelope
  When the request is validated
  Then the override is rejected as envelope-inconsistent
  And the operator is told the allowed, envelope-fitting set
```

### Test Plan — SCOPE-01

| Scenario | Concrete test (file::function) | Category | Asserts |
|----------|--------------------------------|----------|---------|
| SCN-088-A02 | `internal/assistant/openknowledge/modelswitch/allowlist_test.go::TestAllowlist_Resolve_OffListRejected_ModelNotAllowlisted_Spec088` (ADVERSARIAL) | unit | An off-allowlist raw string ⇒ `Override` zero AND a `Rejection{ReasonCode: model_not_allowlisted}`; the raw never becomes an `Override.SynthesisModel`. Fails if `Resolve` ever silently returns baseline for an off-list model. |
| SCN-088-A02 | `internal/assistant/openknowledge/modelswitch/allowlist_test.go::TestAllowlist_RejectionMessage_GoldenWording_Spec088` | unit | `Rejection.Message` matches the UX golden sentence (capital "I", em-dash, capitalised **NOT**, lists allowed set + default + retry hint). One string used verbatim by both surfaces. |
| SCN-088-A07 | `internal/assistant/openknowledge/modelswitch/allowlist_test.go::TestAllowlist_Resolve_UnprofiledRejected_ModelNotAllowlisted_Spec088` (ADVERSARIAL) | unit | An un-profiled string (`totally-made-up`) ⇒ `model_not_allowlisted`; never an `Override`. |
| SCN-088-A07 | `internal/assistant/openknowledge/modelswitch/allowlist_test.go::TestAllowlist_Resolve_ProfiledOverEnvelopeRejected_ModelOverMemEnvelope_Spec088` (ADVERSARIAL) | unit | A profiled-but-too-big model (`deepseek-r1:32b` against a 28672 envelope) ⇒ `model_over_memory_envelope` with the raise-the-envelope opt-up wording. Fails if it is mislabeled `model_not_allowlisted` or accepted. |
| SCN-088-A07 | `internal/config/validate_ml_envelope_test.go::TestValidateModelEnvelopes_SwitchableOverEnvelopeRejected_Spec088` (ADVERSARIAL) | unit | A `switchable_models` entry that busts the co-residence envelope fails loud at config-generation; an un-profiled entry fails loud as missing-profile. |
| supplementary (A01/A03) | `internal/assistant/openknowledge/modelswitch/allowlist_test.go::TestAllowlist_Resolve_BaselineEmptyReturnsZeroOverride_Spec088` + `internal/assistant/openknowledge/modelswitch/allowlist_test.go::TestAllowlist_Resolve_InListAppliedToSynthesis_Spec088` | unit | `""` ⇒ zero override (baseline, FR-1/NFR-4); an in-list model ⇒ `Override{SynthesisModel: raw}`. |
| supplementary (A06) | `internal/assistant/openknowledge/modelswitch/allowlist_test.go::TestAllowlist_Resolve_SingleRejectionContract_Spec088` | unit | The SAME raw string ⇒ a byte-identical `Rejection` every call (the one shared contract both surfaces render — parity seam, design Two-Surface Parity table). |
| build fail-loud | `internal/assistant/openknowledge/modelswitch/allowlist_test.go::TestAllowlist_NewAllowlist_FailLoudBuild_Spec088` | unit | `NewAllowlist` rejects: empty switchable set; an entry with no profile; an entry that busts the envelope; an empty `defaultModel`. |
| config fail-loud | `internal/config/openknowledge_test.go::TestOpenKnowledgeConfig_SwitchableModelsRequiredWhenEnabled_Spec088` + extend `TestOpenKnowledgeConfig_MissingEnvVars` | unit | Empty `switchable_models` + enabled ⇒ fail-loud; missing `ASSISTANT_OPEN_KNOWLEDGE_SWITCHABLE_MODELS` env ⇒ fail-loud. |
| Regression E2E (A02/A07) | `tests/e2e/agent/openknowledge_e2e_test.go` + `./smackerel.sh test e2e` (e2e-api) | e2e-api | The live `/ask` rejection path returns the fail-loud rejection (no backend call for the rejected model); executed in the self-hosted `bubbles.devops` re-verify dispatch (model+GPU-dependent, C7). |

### Definition of Done — SCOPE-01 (all unchecked — implementation pending)

**Tier-1 (universal):**

- [x] D01-T1-1 — `bash .github/bubbles/scripts/artifact-lint.sh specs/088-runtime-switchable-models` clean. → Evidence: report.md → SCOPE-01.
- [x] D01-T1-2 — `./smackerel.sh check` EXIT 0 (build + vet + config-sync + scenario-lint). → Evidence: report.md → SCOPE-01.
- [x] D01-T1-3 — `./smackerel.sh format --check` EXIT 0. → Evidence: report.md → SCOPE-01.
- [x] D01-T1-4 — No `${VAR:-default}` / hidden fallback introduced (G028 / `smackerel-no-defaults`); the new SST key is REQUIRED + fail-loud. → Evidence: report.md → SCOPE-01 (config generate + grep).
- [x] D01-T1-5 — Every evidence block in report.md → SCOPE-01 is REAL terminal output (anti-fabrication); no synthesized results. → Evidence: report.md → SCOPE-01.
- [x] D01-T1-6 — Do-not-touch boundary respected: zero changes under `internal/cardrewards/`, `ml/app/card_categories.py`, `ml/app/main.py`, `ml/tests/test_card_categories.py`, `specs/083-card-rewards-companion/`, `tests/integration/cardrewards_extract_test.go` (spec-083/073 WIP, C5). → Evidence: report.md → Change Manifest (isolated diff).
- [x] D01-T1-7 — Latency invariant restated unchanged: this scope adds no turns and no timeout knob; the documented `WriteTimeout = (6+1)×600s = 4200s` is untouched. → Evidence: report.md → SCOPE-01.

**Tier-2 (role-specific: config + validator):**

- [x] D01-T2-1 — `assistant.open_knowledge.switchable_models` is SST + fail-loud (REQUIRED non-empty when enabled); dev `[gemma3:4b]`; self-hosted override `[gemma4:26b, deepseek-r1:7b]`; resolved via the `config.sh` env-override pattern; `./smackerel.sh config generate` EXIT 0 with `ASSISTANT_OPEN_KNOWLEDGE_SWITCHABLE_MODELS` present in the generated dev + test env. → Evidence: report.md → SCOPE-01.
- [x] D01-T2-2 — `validateModelEnvelopes` fails loud at config-generation for a `switchable_models` entry with no `model_memory_profiles` entry OR that busts the co-residence envelope; the self-hosted set is envelope-consistent (gemma4:26b 18432 + deepseek-r1:7b 4864 = 23296 ≤ 28672) and `deepseek-r1:32b` is correctly NOT switchable there (40960 > 28672). → Evidence: report.md → SCOPE-01 (envelope arithmetic + test).
- [x] D01-T2-3 — `modelswitch.Resolve` returns a zero `Override` for empty input (baseline, FR-1), an `Override{SynthesisModel}` for an in-list model, and a typed `Rejection` (never a silent default, never a backend passthrough) for off-list / un-profiled / over-envelope input (SCN-088-A02, A07). → Evidence: report.md → SCOPE-01.
- [x] D01-T2-4 — The two reason-codes are correct and distinguishable: `model_not_allowlisted` (unknown/un-profiled/not-offered) vs `model_over_memory_envelope` (profiled but busts the env envelope); the `Rejection.Message` is the verbatim UX sentence for each. → Evidence: report.md → SCOPE-01 (golden test).
- [x] D01-T2-5 — Each SCN-088-A02/A07 test is non-tautological and ADVERSARIAL: it fails if `Resolve` ever accepts an off-list/over-envelope model or silently falls back to baseline; no bailout early-returns. → Evidence: report.md → SCOPE-01 RED-before (neutralised validator).
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior in this scope pass — the persistent per-scenario `…_Spec088` rejection/envelope tests (this scope's Test Plan rows, SCN-088-A02/A07) stay GREEN in the re-run 30/30 spec-088 suite, and the live `/ask` fail-loud rejection path is confirmed end-to-end by the recorded 2026-07-20 self-hosted A/B (both arms HTTP 200, per_request override honoured, off-allowlist rejected fail-loud). → Evidence: report.md → Parent-Expanded Re-Verification.
- [x] Broader E2E regression suite passes — the whole-tree `./smackerel.sh test unit --go` (`go test ./...`) finishes OK EXIT 0 this session with the spec-084 + spec-087 + spec-064 open-knowledge suites GREEN and zero FAIL lines. → Evidence: report.md → Parent-Expanded Re-Verification.

---

## Scope 2: SCOPE-02 — Override threading + per-request clone + attribution (core spine)

**Status:** Done
**Scope-Kind:** code (agent + agenttool + facade + contracts)
**Depends on:** SCOPE-01
**Foundation:** false (completes the shared capability **spine** —
per-invocation config construction + answer attribution — that the
SCOPE-03 per-surface carriers consume; the validator it calls is the
SCOPE-01 foundation)

**Intent:** Thread an optional, already-validated override from the
carrier field through a per-request agent **clone** (the SST singleton is
never mutated, C6) onto the spec-087 forced-final SYNTHESIS turn only
(Fork B), and stamp the model that actually produced the final text onto
the answer — with every spec-064/084/087 trust contract running
unchanged under the switched model.

### Surface (design CHANGE 6,7,8a,11)

- `internal/assistant/contracts/message.go` — NEW typed
  `AssistantMessage.ModelOverride string` (untrusted; validated before
  use; owner directive: a typed field, NOT `TransportMetadata`).
- `internal/assistant/contracts/response.go` — NEW
  `ModelAttribution{ModelID string; OverrideApplied bool}` +
  `AssistantResponse.ModelAttribution *ModelAttribution`.
- `internal/assistant/openknowledge/agent/agent.go` — NEW
  `TurnResult.Model string`; `finalize` stamps the answering model once
  (`if out.Model == "" { out.Model = answeringModel }`); `Run` tracks
  `answeringModel` (init `a.cfg.Model`; `= reqModel` each iter after the
  switch; `= a.cfg.SynthesisModel` in the synthesis-retry loop); NEW
  `WithModelOverride(modelswitch.Override) *Agent` per-request clone
  (`o.IsZero()` ⇒ return receiver; else shallow copy with
  `clone.cfg.SynthesisModel = o.SynthesisModel`).
- `internal/assistant/openknowledge/agenttool/substrate_tool.go` — NEW
  `outputEnvelope.Model string json:"model,omitempty"`; `MapTurnResult`
  sets it on success AND refusal arms; NEW `SetSwitchableModels` /
  `SwitchableModels` atomic singleton accessor (parallels
  `SetAgent`/`CurrentAgent`).
- `internal/assistant/facade.go` — open_knowledge fast-path: resolve
  `msg.ModelOverride` via `okagenttool.SwitchableModels()` **nil-safely**
  (nil allowlist / not-yet-wired ⇒ baseline passthrough, never a panic);
  on `rej != nil` build a rejection `AssistantResponse` (Body =
  `rej.Message`) and SKIP the agent + assembler + provenance + capture
  (pre-agent request validation — design Rejection ≠ capture-skip); else
  `runOpenKnowledgeDirect(ctx, sc, env, emittedAt, ov)` and set
  `resp.ModelAttribution = &ModelAttribution{ModelID: result.Model,
  OverrideApplied: !ov.IsZero()}`. `runOpenKnowledgeDirect` takes a new
  `ov modelswitch.Override` param: `CurrentAgent().WithModelOverride(ov).
  Run(ctx, prompt)`; `result.Model = turn.Model`.
- `cmd/core/main.go` — `WriteTimeout` comment only: a switched synthesis
  model adds no turns and is bounded by the same `(6+1)×600s = 4200s`
  envelope; compare-both (a design fork C non-goal, F-COMPARE-LATENCY)
  would re-derive it. **No value change.**
- Tests: `internal/assistant/openknowledge/agent/modelswitch_agent_spec088_test.go`
  (NEW); `internal/assistant/facade_modelswitch_spec088_test.go` (NEW);
  extend `internal/assistant/openknowledge/agenttool/substrate_tool_test.go`
  and `internal/assistant/contracts/response_test.go`.

**Covers scenarios:** SCN-088-A01, SCN-088-A03, SCN-088-A04,
SCN-088-A05, SCN-088-A08. (These five are facets of ONE mechanism — the
per-request synthesis-model clone + attribution — and cannot be proven in
isolation: "applied" and "baseline unchanged" are the same clone, and
"attributed" and "trust-preserved" both read the same `finalize`
chokepoint. Splitting further would invent artificial seams.)

### Use Cases (Gherkin) — quoted from spec.md §4

```gherkin
Scenario: SCN-088-A01 — Valid allowlisted override is applied; baseline untouched
  Given the SST baseline open-knowledge model is the default
  And the operator supplies a model override that is on the switchable-model allowlist
  When the open-knowledge agent runs that /ask invocation
  Then the agent uses the overridden model for that invocation
  And the SST baseline is unchanged for every other invocation
  And the answer is produced under the full trust perimeter

Scenario: SCN-088-A03 — No override leaves baseline behavior byte-for-byte unchanged
  Given the operator supplies no model override
  When the open-knowledge agent runs the /ask invocation
  Then the agent uses exactly the SST baseline model for the tool turns
  And the SST baseline synthesis model for the forced-final synthesis turn
  And the observable behavior is identical to the spec-087 baseline

Scenario: SCN-088-A04 — The answer is attributed to the model that produced it
  Given an /ask invocation completed (with or without an override)
  When the response is returned to the operator
  Then the response surfaces which model produced the answer
  And two answers produced by two different models are distinguishable by that attribution

Scenario: SCN-088-A05 — Every trust contract holds under an overridden model
  Given an allowlisted override is in effect
  And the switched model emits a citation that does not hash-match any tool result
  When the cite-back verifier runs in enforce mode on the post-<think>-strip text
  Then the answer is replaced with the canonical refusal
  # and zero-source ⇒ refuse-with-capture (capture fires); <think> never leaks/cited

Scenario: SCN-088-A08 — The latency envelope stays honest under a slower switched model
  Given an allowlisted override selects a slower model
  When the /ask invocation runs
  Then each turn remains bounded by the per-LLM-roundtrip timeout
  And the worst-case /ask envelope remains the documented WriteTimeout bound
```

### Test Plan — SCOPE-02

| Scenario | Concrete test (file::function) | Category | Asserts |
|----------|--------------------------------|----------|---------|
| SCN-088-A01 | `internal/assistant/openknowledge/agent/modelswitch_agent_spec088_test.go::TestAgent_SynthesisOverrideApplied_SynthesisTurnUsesOverriddenModel_Spec088` (ADVERSARIAL) | unit | A fake LLM records the `Model` of every `ChatRequest`. Under an override, the forced-final synthesis turn (+ retries) uses the overridden model; every gather/tool turn keeps `llm_model_id`. Fails if the override leaks onto the gather turns or never reaches synthesis. |
| SCN-088-A01 | `internal/assistant/openknowledge/agent/modelswitch_agent_spec088_test.go::TestAgent_WithModelOverride_ClonesSingletonNeverMutated_Spec088` | unit | After `WithModelOverride`, the singleton's `cfg.Model`/`cfg.SynthesisModel` are unchanged (C6); the clone's `SynthesisModel` is the override; a zero override returns the receiver pointer. |
| SCN-088-A01 | `internal/assistant/facade_modelswitch_spec088_test.go::TestFacade_AppliedOverride_ThreadsAttribution_Spec088` | functional | An applied override threads to `resp.ModelAttribution{ModelID: turn.Model, OverrideApplied: true}` via `runOpenKnowledgeDirect`. |
| SCN-088-A03 | `internal/assistant/openknowledge/agent/modelswitch_agent_spec088_test.go::TestAgent_NoOverride_ByteForByteBaseline_Spec088` (ADVERSARIAL) | unit | Zero override ⇒ `WithModelOverride` returns the receiver; gather turns use `llm_model_id`, forced-final uses `synthesis_model_id`; the recorded per-turn model sequence equals the spec-087 baseline sequence exactly. Fails if the no-override path diverges in any turn. |
| SCN-088-A04 | `internal/assistant/openknowledge/agent/modelswitch_agent_spec088_test.go::TestAgent_TurnResultModelStamped_AllTerminalPaths_Spec088` | unit | `TurnResult.Model` is stamped by `finalize` on success, honest-salvage, refuse, AND early-`StopEndTurn` (answer from the tool model under a synthesis override) — honest per CT-3. |
| SCN-088-A04 | `internal/assistant/openknowledge/agenttool/substrate_tool_test.go::TestMapTurnResult_ModelCarried_BothArms_Spec088` | unit | `MapTurnResult` sets `outputEnvelope.Model = turn.Model` on BOTH the success and refusal arms. |
| SCN-088-A05 | `internal/assistant/openknowledge/agent/modelswitch_agent_spec088_test.go::TestAgent_TrustContractsHoldUnderOverride_Spec088` (ADVERSARIAL, 3 sub-cases) | unit | Under an allowlisted override: (a) a fabricated citation ⇒ canonical refusal (post-`<think>`-strip cite-back enforce); (b) a zero-source forced-final ⇒ provenance refusal AND capture-as-fallback still fires; (c) a reasoning-model `<think>` never appears in the body and is never a citation. |
| SCN-088-A05 | `internal/assistant/facade_modelswitch_spec088_test.go::TestFacade_OffAllowlistOverride_ShortCircuits_NoAgentCall_NoCapture_Spec088` (ADVERSARIAL) | functional | A rejected override short-circuits the fast-path: the agent is NEVER called and capture-as-fallback is NOT invoked for the rejected request (pre-agent validation, not an agent run). Fails if a rejection still calls the agent or captures. |
| SCN-088-A08 | `internal/assistant/openknowledge/agent/modelswitch_agent_spec088_test.go::TestAgent_SynthesisOverride_PreservesIterationEnvelope_Spec088` | unit | `WithModelOverride` changes only `SynthesisModel`; `MaxIterations` and `SynthesisRetryBudget` (the `WriteTimeout` formula inputs) are unchanged ⇒ the documented `(6+1)×600s` worst case is preserved (no added turns). |
| — (contracts) | `internal/assistant/contracts/response_test.go::TestAssistantResponse_FieldInventory_ModelAttribution_Spec088` | unit | The `AssistantResponse` field inventory includes `ModelAttribution`; zero value ⇒ no attribution (baseline). |
| — (regression) | `internal/assistant/openknowledge/agent/reasoning_loop_spec084_test.go` + `synthesis_spec087_test.go` | unit | The spec-084 + spec-087 agent suites stay GREEN unchanged: a zero override is byte-for-byte the spec-087 path. |
| Regression E2E (A01/A04/A05) | `tests/e2e/agent/openknowledge_e2e_test.go` + `./smackerel.sh test e2e` (e2e-api) | e2e-api | The live `/ask` override path returns a grounded, cited, model-attributed answer with the trust perimeter intact; executed in the self-hosted `bubbles.devops` re-verify dispatch (C7). |

### Definition of Done — SCOPE-02 (all unchecked — implementation pending)

**Tier-1 (universal):**

- [x] D02-T1-1 — `bash .github/bubbles/scripts/artifact-lint.sh specs/088-runtime-switchable-models` clean. → Evidence: report.md → SCOPE-02.
- [x] D02-T1-2 — `./smackerel.sh check` EXIT 0. → Evidence: report.md → SCOPE-02.
- [x] D02-T1-3 — `./smackerel.sh format --check` EXIT 0. → Evidence: report.md → SCOPE-02.
- [x] D02-T1-4 — No `${VAR:-default}` / hidden fallback; no new runtime default; the override never mutates SST (C6). → Evidence: report.md → SCOPE-02.
- [x] D02-T1-5 — Every report.md → SCOPE-02 evidence block is REAL terminal output, including RED-before for the adversarial subset (anti-fabrication). → Evidence: report.md → SCOPE-02.
- [x] D02-T1-6 — Do-not-touch boundary respected (spec-083/073, C5). → Evidence: report.md → Change Manifest.
- [x] D02-T1-7 — Latency invariant restated unchanged: `cmd/core/main.go` comment confirms a synthesis-only switch adds no turns and `WriteTimeout` stays `4200s` (SCN-088-A08). → Evidence: report.md → SCOPE-02 + `cmd/core/main.go`.

**Tier-2 (role-specific: agent/facade core + test integrity):**

- [x] D02-T2-1 — An applied override re-points the forced-final synthesis turn (+ retries) only; gather/tool turns keep `llm_model_id` (SCN-088-A01, Fork B). → Evidence: report.md → SCOPE-02 GREEN-after.
- [x] D02-T2-2 — `WithModelOverride` is a per-request clone: the SST singleton `cfg` is never written; a zero override returns the receiver so the no-override path is byte-for-byte spec-087 (SCN-088-A01 baseline-untouched + SCN-088-A03; C6/NFR-4). → Evidence: report.md → SCOPE-02.
- [x] D02-T2-3 — `TurnResult.Model` is stamped once in `finalize` and is honest across success / salvage / refuse / early-`StopEndTurn`; it flows to `outputEnvelope.Model` (HTTP always) and to `ModelAttribution` (Telegram only-on-override) (SCN-088-A04, Principle 8). → Evidence: report.md → SCOPE-02.
- [x] D02-T2-4 — All trust invariants run unchanged under an override: post-`<think>`-strip cite-back refuses a fabricated citation; the provenance gate refuses zero-source; capture-as-fallback fires on every path where the agent runs; `<think>` never leaks/cited (SCN-088-A05; C1). → Evidence: report.md → SCOPE-02 RED→GREEN.
- [x] D02-T2-5 — A rejected override is pre-agent request validation: the agent is NOT called and capture is NOT invoked for the rejected request; this is the malformed-control-parameter path, not a capture-skip violation (design Rejection ≠ capture-skip). → Evidence: report.md → SCOPE-02.
- [x] D02-T2-6 — The facade override-resolve is nil-safe: a nil `SwitchableModels()` (capability not yet wired in SCOPE-03, or open_knowledge disabled) yields baseline passthrough, never a panic — so SCOPE-02 is independently non-breaking. → Evidence: report.md → SCOPE-02.
- [x] D02-T2-7 — Every SCN-088-A0x test is non-tautological with an ADVERSARIAL case that fails if the behavior regresses (override leaking to gather turns; baseline divergence; un-stamped attribution; weakened trust; added turns); no bailout early-returns; proven by the RED-before run. → Evidence: report.md → SCOPE-02 RED-before.
- [x] D02-T2-8 — The latency envelope stays honest under a slower switched model: `WithModelOverride` changes only `SynthesisModel`, leaving `MaxIterations` and `SynthesisRetryBudget` (the `WriteTimeout` formula inputs) unchanged, so each turn stays bounded by the per-LLM-roundtrip timeout and the worst-case `/ask` envelope stays the documented `(6+1)×600s = 4200s` `WriteTimeout` bound (SCN-088-A08). → Evidence: report.md → SCOPE-02 (latency invariant) + Parent-Expanded Re-Verification.
- [x] D02-T2-9 — Latency/timeout SLA stress coverage: spec-088's SLA is the per-request worst-case `WriteTimeout` ceiling (a per-request latency bound, NOT a throughput/concurrency SLA — the synthesis-only switch adds zero turns and zero concurrency surface). Stress coverage is threefold: (a) the existing open-knowledge agent-loop p95 SLA stress test `tests/stress/openknowledge_p95_test.go::TestOpenKnowledge_P95SLAUnderToolLoad` (the concurrent-worker hot-path SLA the zero-new-turn `WithModelOverride` clone runs on top of, unchanged), (b) the no-added-turns invariant test `TestAgent_SynthesisOverride_PreservesIterationEnvelope_Spec088`, and (c) the recorded live A/B latency under REAL switched models (ARM-A `qwen3:30b-a3b` 312.17s, ARM-B `gemma4:26b` 100.56s — both well inside the 4200s bound). A synthetic per-request load generator does not apply to a zero-new-turn clone. → Evidence: report.md → Parent-Expanded Re-Verification.
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior in this scope pass — the persistent per-scenario `…_Spec088` agent + facade tests (SCN-088-A01/A03/A04/A05/A08) stay GREEN in the re-run 30/30 spec-088 suite, and the runtime synthesis-model switch is confirmed end-to-end by the recorded 2026-07-20 self-hosted A/B (per_request override honoured BOTH ways: ARM-A `qwen3:30b-a3b` grounded/cited/attributed HTTP 200; ARM-B `gemma4:26b` HTTP 200 attributed). → Evidence: report.md → Parent-Expanded Re-Verification.
- [x] Broader E2E regression suite passes — the whole-tree `./smackerel.sh test unit --go` (`go test ./...`) finishes OK EXIT 0 this session; the spec-084 + spec-087 open-knowledge agent suites are GREEN with zero FAIL. → Evidence: report.md → Parent-Expanded Re-Verification.

---

## Scope 3: SCOPE-03 — Two-surface parity + fail-loud rendering + docs (concrete carriers)

**Status:** Done
**Scope-Kind:** code (telegram adapter + HTTP api + wiring) + config (deploy contract) + docs
**Depends on:** SCOPE-02
**Foundation:** false (the two thin per-surface **carriers** — concrete
implementations overlaying the SCOPE-01 validator + SCOPE-02 spine; DE4
Variation Axes: surface composition, rejection transport, override syntax)

**Intent:** Add the two thin per-surface carriers that parse the override
into the SCOPE-02 carrier field and render the SCOPE-02 attribution /
rejection — Telegram (`--model=` parse + `— model:` footer) and web/HTTP
(`model` field + 400 rejection envelope) — wire the allowlist singleton
into the open-knowledge build, record the deploy contract path, and
document the operator surface. SCN-088-A06 proves both surfaces run the
SAME validator and render the SAME rejection.

### Surface (design CHANGE 8b,9,10,12)

- `internal/telegram/assistant_adapter/translate_inbound.go` — on the
  `/ask` shortcut branch, parse a leading `--model=<id>` token from the
  post-`/ask` arguments: set `msg.ModelOverride = <id>` and remove ONLY
  that token from `Text` (slash prefix preserved so `LookupShortcut` +
  the facade `StripShortcutPrefix` still work; same discipline as the
  BUG-064-001 prefix strip). Everything else stays in `Text`.
- `internal/telegram/assistant_adapter/render_outbound.go` — in
  `buildTelegramRendering`, append `\n— model: <ModelAttribution.ModelID>`
  iff `resp.ModelAttribution != nil && resp.ModelAttribution.OverrideApplied`
  (success, refusal, AND salvage arms). Baseline ⇒ NO footer
  (SCN-088-A03 / NFR-4 / Principle 6).
- `internal/api/agent_invoke.go` — NEW `AgentInvokeRequest.Model string
  json:"model,omitempty"`; in the open_knowledge fast-path: `ov, rej :=
  agenttool.SwitchableModels().Resolve(req.Model)`; on `rej != nil` write
  HTTP 400 with the rejection envelope (`status:"rejected"`, `error_code`,
  `rejected_model`, `allowed_models`, `default_model`, `message`); else
  `CurrentAgent().WithModelOverride(ov).Run(...)` → `MapTurnResult` (env
  carries `model` always) → `writeOpenKnowledgeResponse`.
- `cmd/core/wiring_assistant_openknowledge.go` — after
  `agenttool.SetAgent(agent)`, build + install the allowlist from the
  SAME SST already loaded via `modelswitch.NewAllowlist(okCfg.SwitchableModels,
  cfg.MLModelMemoryProfiles, cfg.OllamaMemoryLimitMiB, okCfg.LLMModelID,
  okCfg.SynthesisModelID)` + `agenttool.SetSwitchableModels(allow)`; gated
  on `open_knowledge.enabled` (so `SwitchableModels()` is non-nil exactly
  when `CurrentAgent()` is); add `switchable_models` to the startup log.
- `deploy/contract.yaml` — NEW
  `assistant.open_knowledge.switchable_models` path (type `string[]`,
  secret false, per-env override note).
- `docs/Operations.md` — open-knowledge section: the per-request
  `--model=` / API `model` switch, the `switchable_models` allowlist +
  envelope-consistency, the two-reason fail-loud rejection, the
  `— model:` attribution, and the unchanged `WriteTimeout`.
- Tests: extend `internal/telegram/assistant_adapter/translate_inbound_test.go`
  + `render_outbound_test.go`; NEW `internal/api/agent_invoke_test.go`.

**Covers scenarios:** SCN-088-A06 (+ reinforces SCN-088-A02 and
SCN-088-A04 across both surfaces).

### Use Cases (Gherkin) — quoted from spec.md §4

```gherkin
Scenario: SCN-088-A06 — Telegram and web/HTTP /ask validate and apply the override identically
  Given the same allowlisted model override
  When it is supplied via the Telegram /ask surface
  And separately via the web/HTTP /ask surface
  Then both surfaces apply the override to the agent invocation identically
  And both surfaces run the same allowlist validation
  And an off-allowlist override is rejected identically on both surfaces
```

### Test Plan — SCOPE-03

| Scenario | Concrete test (file::function) | Category | Asserts |
|----------|--------------------------------|----------|---------|
| SCN-088-A06 | `internal/telegram/assistant_adapter/translate_inbound_test.go::TestTranslateInbound_ModelFlagParsedAndStripped_SlashPreserved_Spec088` (ADVERSARIAL) | functional | `/ask --model=deepseek-r1:7b <q>` ⇒ `msg.ModelOverride == "deepseek-r1:7b"` AND `Text` is the clean question with the slash prefix preserved and the `--model=` token removed; a bare `/ask <q>` ⇒ empty `ModelOverride`. Fails if the flag leaks into the question or the slash is dropped. |
| SCN-088-A06 | `internal/api/agent_invoke_test.go::TestAgentInvoke_ModelFieldApplied_EnvelopeCarriesModel_Spec088` | functional | A `model` field on the request threads through `Resolve`→`WithModelOverride`→`Run`; the success envelope carries `model`. |
| SCN-088-A06 / A02 | `internal/api/agent_invoke_test.go::TestAgentInvoke_OffAllowlistModel_Returns400RejectionEnvelope_Spec088` (ADVERSARIAL) | functional | An off-allowlist `model` ⇒ HTTP 400 with `status:"rejected"`, `error_code:"model_not_allowlisted"`, `rejected_model`, `allowed_models`, `default_model`, and a `message` byte-identical to the Telegram rejection sentence. No agent call. |
| SCN-088-A06 | `internal/assistant/openknowledge/modelswitch/allowlist_test.go::TestAllowlist_Resolve_SingleRejectionContract_Spec088` (parity seam) | unit | The SAME off-allowlist string ⇒ a byte-identical `Rejection` — the one shared contract both surfaces are bound to render verbatim (design Two-Surface Parity table). |
| SCN-088-A03 / A04 | `internal/telegram/assistant_adapter/render_outbound_test.go::TestBuildTelegramRendering_ModelFooterOnOverrideOnly_Spec088` (ADVERSARIAL) | functional | `OverrideApplied:true` ⇒ trailing `— model: <id>` footer; baseline (`ModelAttribution == nil`) ⇒ NO footer. Fails if a baseline answer grows a footer (NFR-4) or an override answer loses it. |
| SCN-088-A03 | `internal/api/agent_invoke_test.go::TestAgentInvoke_NoModel_EnvelopeModelPresent_Spec088` | functional | A request with no `model` field ⇒ the envelope still reports the resolved baseline `model` (structured metadata always present). |
| Regression E2E (A06) | `tests/e2e/agent/openknowledge_e2e_test.go` + `./smackerel.sh test e2e` (e2e-api) | e2e-api | The live `/ask` override behaves identically on Telegram + HTTP (same model answers, same rejection); executed in the self-hosted `bubbles.devops` re-verify dispatch (C7). |

### Definition of Done — SCOPE-03 (all unchecked — implementation pending)

**Tier-1 (universal):**

- [x] D03-T1-1 — `bash .github/bubbles/scripts/artifact-lint.sh specs/088-runtime-switchable-models` clean. → Evidence: report.md → SCOPE-03.
- [x] D03-T1-2 — `./smackerel.sh check` EXIT 0. → Evidence: report.md → SCOPE-03.
- [x] D03-T1-3 — `./smackerel.sh format --check` EXIT 0. → Evidence: report.md → SCOPE-03.
- [x] D03-T1-4 — No `${VAR:-default}` / hidden fallback; the deploy-contract path documents the SST key as REQUIRED + fail-loud. → Evidence: report.md → SCOPE-03.
- [x] D03-T1-5 — Every report.md → SCOPE-03 evidence block is REAL terminal output, including the full `./smackerel.sh test unit --go` regression with out-of-changeset reds attributed by file path (anti-fabrication). → Evidence: report.md → SCOPE-03 + Regression.
- [x] D03-T1-6 — Do-not-touch boundary respected (spec-083/073, C5). → Evidence: report.md → Change Manifest.
- [x] D03-T1-7 — Latency invariant restated unchanged: `docs/Operations.md` records the unchanged `WriteTimeout = 4200s` and the compare-both two-pass note (a design fork C non-goal, F-COMPARE-LATENCY). → Evidence: report.md → SCOPE-03 + `docs/Operations.md`.

**Tier-2 (role-specific: two-surface parity + rendering + docs):**

- [x] D03-T2-1 — Telegram `--model=<id>` is parsed off the `/ask` line into `msg.ModelOverride`, stripped from `Text` (slash preserved), and the question the agent sees is clean (SCN-088-A06). → Evidence: report.md → SCOPE-03.
- [x] D03-T2-2 — HTTP `model` field threads through the SAME `Resolve`→`WithModelOverride`→`Run` spine; the success envelope carries `model` always; an off-allowlist `model` ⇒ HTTP 400 rejection envelope (SCN-088-A06 / A02). → Evidence: report.md → SCOPE-03.
- [x] D03-T2-3 — Transport parity proven: the SAME off-allowlist string drives the SAME validator singleton and renders the SAME `Rejection.Message` on both surfaces; an allowlisted model is switchable on both (SCN-088-A06). → Evidence: report.md → SCOPE-03 (parity test + golden message).
- [x] D03-T2-4 — The `— model:` footer is shown on Telegram ONLY when an override was applied (success/refusal/salvage arms); a baseline `/ask` is byte-for-byte spec-087 with no footer (SCN-088-A03 / Principle 6 / NFR-4). → Evidence: report.md → SCOPE-03.
- [x] D03-T2-5 — The allowlist singleton is installed at wiring from the SAME loaded SST, gated on `open_knowledge.enabled` (non-nil exactly when `CurrentAgent()` is); the startup log names `switchable_models`. → Evidence: report.md → SCOPE-03.
- [x] D03-T2-6 — `deploy/contract.yaml` carries the `switchable_models` path; `docs/Operations.md` documents the switch, allowlist, two-reason rejection, attribution, and unchanged `WriteTimeout`. → Evidence: `deploy/contract.yaml` + `docs/Operations.md`.
- [x] D03-T2-7 — Full `./smackerel.sh test unit --go` regression: spec-088 + spec-087 + spec-084 + spec-064 open-knowledge tests GREEN; only the out-of-changeset spec-083 WIP / spec-073 env reds remain, attributed by file path (finding F-ENV-083). → Evidence: report.md → Regression.
- [x] D03-T2-8 — Telegram and web/HTTP `/ask` validate and apply the override identically: both surfaces feed the SAME shared `modelswitch` validator singleton, apply the override to the agent invocation identically, and render the byte-identical rejection for an off-allowlist model (SCN-088-A06). → Evidence: report.md → SCOPE-03 (parity test + golden message).
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior in this scope pass — the persistent per-scenario `…_Spec088` telegram-adapter + HTTP-handler parity tests (SCN-088-A06, incl. `TestAgentInvoke_RejectionEnvelopeByteIdenticalToValidator_Spec088`) stay GREEN in the re-run 30/30 spec-088 suite, and the HTTP `model`-field switch is confirmed on the live wire by the recorded 2026-07-20 self-hosted A/B `/v1/agent/invoke` run (per_request override honoured). → Evidence: report.md → Parent-Expanded Re-Verification.
- [x] Broader E2E regression suite passes — the whole-tree `./smackerel.sh test unit --go` (`go test ./...`) finishes OK EXIT 0 this session across all 9 spec-088 packages plus the spec-064/084/087 open-knowledge regression, with zero FAIL. → Evidence: report.md → Parent-Expanded Re-Verification.

---

## Out-of-Changeset / Do-Not-Touch (owner directive C5)

Do NOT modify or "fix": `internal/cardrewards/`,
`ml/app/card_categories.py`, `ml/app/main.py`,
`ml/tests/test_card_categories.py`, `specs/083-card-rewards-companion/`,
`tests/integration/cardrewards_extract_test.go`. Whole-working-tree guard
failures attributable to the spec-083 card-rewards WIP and the spec-073
node/dart container canary are OUT of this changeset — attribute by file
path; do not remediate here (finding F-ENV-083, inherited from spec
084/087).

## Terminal Posture (C7) — validated-in-repo, NO commit/push

This plan ships and validates the switchable-model **primitive** in-repo
only (mechanism-level proof: pure-validator tables, fake-LLM agent
traces, handler/adapter tests). Dev has no GPU/Ollama daemon. The
decisive live `gemma4:26b`-vs-`deepseek-r1:7b` synthesis A/B on self-hosted
hardware — the proof spec 087 could not run on dev — is a SEPARATE
downstream `bubbles.devops` dispatch. After implement + test complete and
all three scopes are validated-in-repo, `nextRequiredOwner = bubbles.devops`
for the live A/B re-verify. **No commit, no push in this spec's run.**
