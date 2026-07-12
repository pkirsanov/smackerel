# Report 088 — Runtime-Switchable Models

> **SKELETON — authored by `bubbles.plan` at the planning phase, BEFORE
> any implementation.** Every scope section below is **PENDING**: it
> carries NO evidence yet. The `bubbles.implement` / `bubbles.test`
> phases fill each section with REAL terminal output (RED-before for the
> adversarial subset, GREEN-after; `config generate` / `check` /
> `format --check` / `test unit --go` transcripts) as the scope is built.
>
> **Anti-fabrication (NON-NEGOTIABLE).** Nothing here is claimed as
> already-passing. No `Exit Code: 0`, no `--- PASS`, no "all tests pass"
> may be written until the command was actually run and its real output
> pasted. Out-of-changeset failures (spec-083 card-rewards WIP / spec-073
> container canary) are attributed by file path, never "fixed" here
> (finding F-ENV-083). Home-path PII in any captured transcript MUST be
> redacted to `~/` before it is written into this file.
>
> **Execution model:** `bubbles.workflow (parent-expanded)` — no
> `runSubagent` available in this runtime; `full-delivery` is not a
> `requiresTopLevelRuntime` mode, so the phaseOrder is executed directly
> (same precedent as specs 084 / 087).
>
> **Terminal posture (C7):** validated-in-repo only. The live
> `gemma4:26b`-vs-`deepseek-r1:7b` synthesis A/B on self-hosted hardware is a
> SEPARATE downstream `bubbles.devops` dispatch. No commit/push in this
> run.

## Summary

**PENDING (filled at completion).** Spec 088 adds an OPTIONAL,
allowlist-validated, per-invocation model override that re-points the
spec-087 forced-final SYNTHESIS turn for one `/ask`, on BOTH the Telegram
and web/HTTP surfaces, with the answering model attributed back —
enabling a live, no-redeploy `gemma4:26b`-vs-`deepseek-r1:7b` synthesis
A/B. No override ⇒ byte-for-byte the spec-087 baseline. Off-allowlist /
un-profiled / over-envelope models are rejected fail-loud (two
reason-codes) and never reach the inference backend. All spec-064/084/087
trust invariants (cite-back, provenance, capture-as-fallback,
`<think>`-strip + retry-before-salvage) run unchanged under any selected
model. `WriteTimeout` is unchanged at `4200s` (synthesis-only switch adds
no turns; compare-both deferred, F-COMPARE-LATENCY).

In-repo evidence target (to be populated): the `modelswitch` validator
table tests + config fail-loud + over-envelope tests (SCOPE-01); the
fake-LLM agent tests (applied / baseline / attribution / trust /
envelope-preserved) + agenttool + facade + contracts tests (SCOPE-02);
the telegram-adapter + api-handler parity tests (SCOPE-03);
`config generate` / `check` / `format --check` EXIT 0; the full Go unit
suite green except the out-of-changeset spec-083/073 reds.

---

## Change Manifest (spec-088 isolated — design 13-change map → scope)

| # | File | Change | Scope |
|---|------|--------|-------|
| 1 | `config/smackerel.yaml` | NEW `assistant.open_knowledge.switchable_models` (dev `[gemma3:4b]`) + self-hosted override `[gemma4:26b, deepseek-r1:7b]`. | SCOPE-01 |
| 2 | `internal/config/openknowledge.go` | `SwitchableModels []string` field, load, struct-level validate. | SCOPE-01 |
| 3 | `internal/config/config.go` | `validateModelEnvelopes` switchable co-residence pass (fail-loud). | SCOPE-01 |
| 4 | `scripts/commands/config.sh` | resolve (per-env override) + emit `ASSISTANT_OPEN_KNOWLEDGE_SWITCHABLE_MODELS`. | SCOPE-01 |
| 5 | `internal/assistant/openknowledge/modelswitch/` | NEW leaf pkg: `Allowlist` / `Override` / `Rejection` / `NewAllowlist` / `Resolve` (two reason-codes). | SCOPE-01 |
| 6 | `internal/assistant/openknowledge/agent/agent.go` | `TurnResult.Model`; `finalize` stamp; `answeringModel` tracking; `WithModelOverride` clone. | SCOPE-02 |
| 7 | `internal/assistant/openknowledge/agenttool/substrate_tool.go` | `outputEnvelope.Model`; `MapTurnResult` stamp; `Set/SwitchableModels` singleton. | SCOPE-02 |
| 8a | `internal/assistant/contracts/message.go`, `response.go`, `internal/assistant/facade.go` | `AssistantMessage.ModelOverride`; `ModelAttribution`; facade resolve→reject→thread→attribute (nil-safe). | SCOPE-02 |
| 8b | `internal/telegram/assistant_adapter/translate_inbound.go`, `render_outbound.go`, `internal/api/agent_invoke.go` | Telegram `--model=` parse + footer; HTTP `model` field + 400 rejection envelope. | SCOPE-03 |
| 9 | `cmd/core/wiring_assistant_openknowledge.go` | build + install the allowlist singleton from SST; startup log. | SCOPE-03 |
| 10 | `deploy/contract.yaml` | NEW `switchable_models` contract path. | SCOPE-03 |
| 11 | `cmd/core/main.go` | `WriteTimeout` comment only (unchanged `4200s`; compare-both deferred). | SCOPE-02 |
| 12 | `docs/Operations.md` | open-knowledge switch / allowlist / rejection / attribution / WriteTimeout. | SCOPE-03 |
| 13 | tests (per scope) | `modelswitch/allowlist_test.go`, `agent/modelswitch_agent_spec088_test.go`, `facade_modelswitch_spec088_test.go`, `agenttool/substrate_tool_test.go`, `contracts/response_test.go`, `config/*_test.go`, `telegram/assistant_adapter/*_test.go`, `internal/api/agent_invoke_test.go`. | all |

---

## Environment Constraint (test-execution surface)

The repo-standard `./smackerel.sh test unit --go` runs `go test ./...`
inside the `golang:1.25.10-bookworm` container (`run_go_tooling` in
`smackerel.sh`). In this offline sandbox the container's
`_ensure_envsubst.sh` bootstrap (`apt-get install gettext-base`) fails —
`deb.debian.org` is unreachable — so the dockerized unit runner aborts
before any test executes (`E: Package 'gettext-base' has no installation
candidate`). This is a documented environment limitation of the sandbox,
exactly parallel to the spec-087 "dev has no GPU/Ollama" constraint.

Test evidence below is therefore captured with the **host** Go toolchain
(`go1.25.10`, byte-identical to the container's Go version) plus the
host `envsubst` (`/usr/bin/envsubst`), which is what the config-shelling
tests need. `./smackerel.sh check`, `./smackerel.sh format --check`, and
`./smackerel.sh config generate` run in containers that do NOT need
`gettext-base`, so those gates run through the repo CLI unchanged and
their real exit codes are captured verbatim.

---

## SCOPE-01 — SST allowlist + shared `modelswitch` validator

**Status:** DONE (validated-in-repo).

### Config generation (CHANGE 1,2,3,4) — `ASSISTANT_OPEN_KNOWLEDGE_SWITCHABLE_MODELS` SST + envelope guard

`./smackerel.sh config generate` for all three envs, then the resolved key:

```text
config-validate: ~/smackerel/config/generated/dev.env.tmp.* OK
Generated ~/smackerel/config/generated/dev.env
CONFIG_GEN_EXIT=0
config-validate: ~/smackerel/config/generated/test.env.tmp.* OK
Generated ~/smackerel/config/generated/test.env
TEST_GEN_EXIT=0
config-validate: skipped for production-class target env=self-hosted (placeholder mode; runtime check enforces at container start)
Generated ~/smackerel/config/generated/self-hosted.env
SELFHOSTED_GEN_EXIT=0
=== SWITCHABLE across envs ===
config/generated/dev.env:ASSISTANT_OPEN_KNOWLEDGE_SWITCHABLE_MODELS=["gemma3:4b"]
config/generated/test.env:ASSISTANT_OPEN_KNOWLEDGE_SWITCHABLE_MODELS=["gemma3:4b"]
config/generated/self-hosted.env:ASSISTANT_OPEN_KNOWLEDGE_SWITCHABLE_MODELS=["gemma4:26b","deepseek-r1:7b"]
```

Dev/test correctly fall back to the base `[gemma3:4b]`; self-hosted resolves
the per-env override `[gemma4:26b, deepseek-r1:7b]`. The dev/test
`config-validate ... OK` lines are the Go `validateModelEnvelopes`
switchable pass accepting the envelope-consistent sets (dev gather
gemma3:4b 4096 ≤ 8192; the self-hosted arithmetic gemma4:26b 18432 +
deepseek-r1:7b 4864 = 23296 ≤ 28672 is pinned by the unit test below).

**Env-override boundary fix (finding F-088-CFGSH, fixed in-scope).** The
first `config generate` failed because the original `yaml_get_json`
env-path query bled across sibling `environments:` blocks and returned
the self-hosted switchable list for `env=dev` (validated against dev's 8 GiB
envelope → correct fail-loud "envelope exceeded"). Root cause: the shared
`yaml_get_json` tree-walk does not respect environment-block boundaries.
Fixed by resolving the env override through `yaml_get` (exact
flattened-key match), normalised to compact JSON. The first failure is
itself proof the envelope guard fires end-to-end at config-generation.

### `./smackerel.sh check` (build + vet + config-sync + scenario-lint) — EXIT 0

```text
config-validate: ~/smackerel/config/generated/dev.env.tmp.* OK
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 16, rejected: 0
scenario-lint: OK
CHECK_EXIT=0
```

### `./smackerel.sh format --check` — EXIT 0

```text
65 files already formatted
FORMAT_CHECK_EXIT=0
```

### RED-before (adversarial subset neutralised) — `modelswitch.Resolve` reject path, `validateModelEnvelopes` switchable pass, and the `switchable_models` non-empty guard all temporarily disabled

```text
--- FAIL: TestAllowlist_Resolve_OffListRejected_ModelNotAllowlisted_Spec088 (0.00s)
--- FAIL: TestAllowlist_RejectionMessage_GoldenWording_Spec088 (0.00s)
--- FAIL: TestAllowlist_Resolve_UnprofiledRejected_ModelNotAllowlisted_Spec088 (0.00s)
--- FAIL: TestAllowlist_Resolve_ProfiledOverEnvelopeRejected_ModelOverMemEnvelope_Spec088 (0.00s)
--- FAIL: TestAllowlist_Resolve_SingleRejectionContract_Spec088 (0.00s)
FAIL    github.com/smackerel/smackerel/internal/assistant/openknowledge/modelswitch     0.006s
--- FAIL: TestOpenKnowledgeConfig_SwitchableModelsRequiredWhenEnabled_Spec088 (0.00s)
--- FAIL: TestValidateModelEnvelopes_SwitchableOverEnvelopeRejected_Spec088 (0.00s)
FAIL    github.com/smackerel/smackerel/internal/config  0.017s
RED_DEMO_EXIT=1
```

The 7 adversarial tests FAIL when the behaviour is neutralised (silent
baseline fallback / disabled envelope guard / disabled non-empty guard),
proving they are non-tautological and would catch the regression. The
neutralisations were reverted verbatim before the GREEN run below.

### GREEN-after (CHANGE 5 validator + CHANGE 2/3 config) — restored implementation

```text
--- PASS: TestAllowlist_Resolve_OffListRejected_ModelNotAllowlisted_Spec088 (0.00s)
--- PASS: TestAllowlist_RejectionMessage_GoldenWording_Spec088 (0.00s)
--- PASS: TestAllowlist_Resolve_UnprofiledRejected_ModelNotAllowlisted_Spec088 (0.00s)
--- PASS: TestAllowlist_Resolve_ProfiledOverEnvelopeRejected_ModelOverMemEnvelope_Spec088 (0.00s)
--- PASS: TestAllowlist_Resolve_BaselineEmptyReturnsZeroOverride_Spec088 (0.00s)
--- PASS: TestAllowlist_Resolve_InListAppliedToSynthesis_Spec088 (0.00s)
--- PASS: TestAllowlist_Resolve_SingleRejectionContract_Spec088 (0.00s)
--- PASS: TestAllowlist_NewAllowlist_FailLoudBuild_Spec088 (0.00s)
--- PASS: TestAllowlist_NewAllowlist_DevEnvelopeSkipped_Spec088 (0.00s)
ok      github.com/smackerel/smackerel/internal/assistant/openknowledge/modelswitch     0.013s
--- PASS: TestOpenKnowledgeConfig_SwitchableModelsRequiredWhenEnabled_Spec088 (0.00s)
--- PASS: TestValidateModelEnvelopes_SwitchableOverEnvelopeRejected_Spec088 (0.00s)
ok      github.com/smackerel/smackerel/internal/config  0.039s
SCOPE01_SPEC088_GREEN_EXIT=0
```

Config-package regression (the new REQUIRED env var rippled into every
full-env map): the spec-064/076/087 config suites stay GREEN —
`go test ./internal/config/... -run 'TestOpenKnowledgeConfig|TestValidate_Rejects|TestValidate_Accepts|Spec076|Spec087'`
⇒ `ok github.com/smackerel/smackerel/internal/config` (EXIT 0).

### Files touched (SCOPE-01)

- `config/smackerel.yaml` — `assistant.open_knowledge.switchable_models` (dev `[gemma3:4b]`) + self-hosted override `[gemma4:26b, deepseek-r1:7b]`.
- `internal/config/openknowledge.go` — `SwitchableModels []string` field + load (`lookupJSONStringList`) + struct-level Validate (non-empty list + non-empty entries, enabled-gated).
- `internal/config/config.go` — `validateModelEnvelopes` switchable co-residence pass (missing-profile + over-envelope, gated on enabled + ollama envelope known).
- `scripts/commands/config.sh` — env-override resolve via `yaml_get` (boundary-safe) + compact-JSON normalise + emit `ASSISTANT_OPEN_KNOWLEDGE_SWITCHABLE_MODELS`.
- `internal/assistant/openknowledge/modelswitch/allowlist.go` — NEW leaf pkg: `Allowlist`/`Override`/`Rejection`/`NewAllowlist`/`Resolve`/`reject`/`message` (two reason-codes, golden wording).
- Tests: `internal/assistant/openknowledge/modelswitch/allowlist_test.go` (NEW); `internal/config/openknowledge_test.go`, `validate_ml_envelope_test.go`, `spec_076_foundation_test.go`, `validate_test.go` (extended full-env maps + new fail-loud/over-envelope cases).

---

## SCOPE-02 — Override threading + per-request clone + attribution

**Status:** DONE (validated-in-repo).

### Surface (CHANGE 6,7,8a,11)

- `internal/assistant/contracts/message.go` — typed `AssistantMessage.ModelOverride string` (untrusted).
- `internal/assistant/contracts/response.go` — `ModelAttribution{ModelID, OverrideApplied}` + `AssistantResponse.ModelAttribution *ModelAttribution`; NEW `ErrModelNotSwitchable` cause (fail-loud rejection discriminator).
- `internal/assistant/openknowledge/agent/agent.go` — `TurnResult.Model`; `answeringModel` tracked per turn; `finalize` stamps it once (all terminal paths); `WithModelOverride` per-request clone (singleton never mutated, C6).
- `internal/assistant/openknowledge/agenttool/substrate_tool.go` — `outputEnvelope.Model` + schema `model`; `MapTurnResult` stamps both arms; `SetSwitchableModels`/`SwitchableModels` atomic singleton.
- `internal/assistant/facade.go` — fast-path: nil-safe `SwitchableModels().Resolve(msg.ModelOverride)`; reject ⇒ rejection `AssistantResponse` (StatusUnavailable + `ErrModelNotSwitchable` + `rej.Message`) with a `break` that SKIPS agent + assembler + provenance + capture; accept ⇒ `runOpenKnowledgeDirect(..., ov)` (new `ov` param) + `resp.ModelAttribution`.
- `cmd/core/main.go` — `WriteTimeout` comment only; value unchanged at `4200s` (synthesis-only switch adds no turns; compare-both deferred).

### `./smackerel.sh check` EXIT 0 + `./smackerel.sh format --check` EXIT 0

```text
config-validate: ~/smackerel/config/generated/dev.env.tmp.* OK
Config is in sync with SST
env_file drift guard: OK
scenario-lint: OK
CHECK_EXIT=0
65 files already formatted
FORMAT_EXIT=0
```

### RED-before (adversarial subset neutralised) — `WithModelOverride` made a no-op AND the facade rejection `break` removed

```text
--- FAIL: TestFacade_OffAllowlistOverride_ShortCircuits_NoAgentCall_NoCapture_Spec088 (0.00s)
--- FAIL: TestFacade_AppliedOverride_ThreadsAttribution_Spec088 (0.00s)
FAIL    github.com/smackerel/smackerel/internal/assistant       0.207s
--- FAIL: TestAgent_SynthesisOverrideApplied_SynthesisTurnUsesOverriddenModel_Spec088 (0.00s)
    --- FAIL: …/forced_final_uses_override (0.00s)
    --- FAIL: …/synthesis_retry_uses_override (0.00s)
--- FAIL: TestAgent_WithModelOverride_ClonesSingletonNeverMutated_Spec088 (0.00s)
--- FAIL: TestAgent_TrustContractsHoldUnderOverride_Spec088/fabricated_citation_still_refused (0.00s)
--- FAIL: TestAgent_SynthesisOverride_PreservesIterationEnvelope_Spec088 (0.00s)
FAIL    github.com/smackerel/smackerel/internal/assistant/openknowledge/agent     0.036s
SCOPE02_RED_EXIT=1
```

When the override is never applied and the rejection no longer
short-circuits, the rejection test sees the agent run (≠ 0 LLM calls),
the applied-override test sees the wrong synthesis model, the clone test
sees `clone == base`, and the trust/envelope tests see the override
ignored. All reverted verbatim before the GREEN run.

### GREEN-after — restored implementation (agent fake-LLM traces + facade/agenttool/contracts)

```text
--- PASS: TestAgent_SynthesisOverrideApplied_SynthesisTurnUsesOverriddenModel_Spec088 (0.00s)
    --- PASS: …/forced_final_uses_override (0.00s)
    --- PASS: …/synthesis_retry_uses_override (0.00s)
--- PASS: TestAgent_WithModelOverride_ClonesSingletonNeverMutated_Spec088 (0.00s)
--- PASS: TestAgent_NoOverride_ByteForByteBaseline_Spec088 (0.00s)
--- PASS: TestAgent_TurnResultModelStamped_AllTerminalPaths_Spec088 (0.00s)
    --- PASS: …/success_forced_final  …/honest_salvage  …/refuse_fabricated  …/early_stop_under_override_reports_gather_model
--- PASS: TestAgent_TrustContractsHoldUnderOverride_Spec088 (0.00s)
    --- PASS: …/fabricated_citation_still_refused  …/think_never_leaks_never_cited
--- PASS: TestAgent_SynthesisOverride_PreservesIterationEnvelope_Spec088 (0.00s)
ok      github.com/smackerel/smackerel/internal/assistant/openknowledge/agent
--- PASS: TestFacade_OffAllowlistOverride_ShortCircuits_NoAgentCall_NoCapture_Spec088 (0.00s)
--- PASS: TestFacade_AppliedOverride_ThreadsAttribution_Spec088 (0.00s)
--- PASS: TestFacade_NoOverride_BaselineAttributionNotApplied_Spec088 (0.00s)
ok      github.com/smackerel/smackerel/internal/assistant
--- PASS: TestMapTurnResult_ModelCarried_BothArms_Spec088 (success_arm / refusal_arm / empty_model_omitted_in_json)
ok      github.com/smackerel/smackerel/internal/assistant/openknowledge/agenttool
--- PASS: TestAssistantResponse_FieldInventory_ModelAttribution_Spec088 (0.00s)
ok      github.com/smackerel/smackerel/internal/assistant/contracts
```

### Regression — the spec-084 + spec-087 suites stay GREEN unchanged

Full-package runs (a zero override is byte-for-byte the spec-087 path):

```text
ok      github.com/smackerel/smackerel/internal/assistant                          0.367s
ok      github.com/smackerel/smackerel/internal/assistant/openknowledge/agent      0.071s
ok      github.com/smackerel/smackerel/internal/assistant/openknowledge/agenttool  0.047s
ok      github.com/smackerel/smackerel/internal/assistant/contracts                0.130s
```

The 9 spec-084 + 7 spec-087 agent tests (`TestAgent_…_Spec084/Spec087`)
and the contracts golden/inventory suites pass unchanged; the new
`ErrModelNotSwitchable` cause got a generated golden fixture
(`testdata/golden/unavailable_model_not_switchable.json`) so the
exhaustive coverage + golden tests stay green.

### Latency invariant (CHANGE 11) — `cmd/core/main.go` `WriteTimeout` unchanged at `4200s`

The comment now records that a per-request synthesis-model switch adds
no turns and changes neither `max_iterations` nor `synthesis_retry_budget`,
so a switched (even slower) synthesis model is bounded by the SAME
`(6+1)×600s = 4200s` envelope (no value change); compare-both (deferred,
F-COMPARE-LATENCY) would double it and require re-derivation.
`TestAgent_SynthesisOverride_PreservesIterationEnvelope_Spec088` pins the
formula inputs.

### Files touched (SCOPE-02)

`internal/assistant/contracts/message.go`, `response.go`,
`response_test.go` (+ golden fixture); `internal/assistant/openknowledge/agent/agent.go`,
`modelswitch_agent_spec088_test.go` (NEW);
`internal/assistant/openknowledge/agenttool/substrate_tool.go`,
`substrate_tool_test.go`; `internal/assistant/facade.go`,
`facade_modelswitch_spec088_test.go` (NEW); `cmd/core/main.go` (comment only).

---

## SCOPE-03 — Two-surface parity + fail-loud rendering + docs

**Status:** DONE (validated-in-repo).

### Surface (CHANGE 8b,9,10,12)

- `internal/telegram/assistant_adapter/translate_inbound.go` — `--model=<id>` parsed off the `/ask` line into `msg.ModelOverride` (scoped to the `open_knowledge` shortcut; slash preserved; `parseModelFlag`).
- `internal/telegram/assistant_adapter/render_outbound.go` — `appendModelFooter` (`— model: <id>` only-on-override, applied to success/refusal/sourced/default render arms); fail-loud rejection-body intercept (`ErrModelNotSwitchable` ⇒ render `rej.Message` verbatim, NOT the generic error line).
- `internal/api/agent_invoke.go` — `AgentInvokeRequest.Model`; fast-path nil-safe `SwitchableModels().Resolve(req.Model)` → 400 rejection envelope (`status/error_code/rejected_model/allowed_models/default_model/message`) OR `WithModelOverride(ov).Run` → `MapTurnResult` (envelope carries `model` always) → `writeOpenKnowledgeResponse`.
- `cmd/core/wiring_assistant_openknowledge.go` — build + install the allowlist singleton from the SAME SST, gated on `open_knowledge.enabled` (non-nil exactly when `CurrentAgent()` is); `switchable_models` added to the startup log.
- `deploy/contract.yaml` — NEW `assistant.open_knowledge.switchable_models` contract path.
- `docs/Operations.md` — spec-088 block (the `--model=`/API switch, the allowlist + envelope-consistency, the two-reason fail-loud rejection, the `— model:` attribution, the unchanged `WriteTimeout`).

### `./smackerel.sh check` EXIT 0 + `./smackerel.sh format --check` EXIT 0

```text
config-validate: ~/smackerel/config/generated/dev.env.tmp.* OK
Config is in sync with SST
env_file drift guard: OK
scenario-lint: OK
CHECK_EXIT=0
65 files already formatted
FORMAT_EXIT=0
```

### RED-before (adversarial subset neutralised) — `parseModelFlag` no-op'd, the API 400 rejection ignored, `appendModelFooter` no-op'd, the render rejection-body intercept removed

```text
--- FAIL: TestBuildTelegramRendering_ModelFooterOnOverrideOnly_Spec088/override_applied_has_trailing_footer (0.00s)
--- FAIL: TestTranslateInbound_ModelFlagParsedAndStripped_SlashPreserved_Spec088/flag_parsed_and_stripped (0.00s)
--- FAIL: TestBuildTelegramRendering_ModelRejectionBodyRendered_Spec088 (0.00s)
FAIL    github.com/smackerel/smackerel/internal/telegram/assistant_adapter     0.033s
--- FAIL: TestAgentInvoke_OffAllowlistModel_Returns400RejectionEnvelope_Spec088 (0.00s)
FAIL    github.com/smackerel/smackerel/internal/api     0.137s
SCOPE03_RED_EXIT=1
```

The footer test sees no footer, the inbound test sees the flag leak into
the question, the rejection-render test sees the generic
`open_knowledge: model_not_switchable` error line, and the HTTP test sees
a 200 instead of a 400 (with the agent run). All reverted verbatim.

### GREEN-after — restored implementation (Telegram adapter + HTTP api)

```text
--- PASS: TestTranslateInbound_ModelFlagParsedAndStripped_SlashPreserved_Spec088 (0.00s)
    --- PASS: …/flag_parsed_and_stripped  …/bare_ask_no_override  …/model_flag_is_ask_only_not_other_shortcuts
--- PASS: TestBuildTelegramRendering_ModelFooterOnOverrideOnly_Spec088 (0.00s)
    --- PASS: …/override_applied_has_trailing_footer  …/baseline_no_footer  …/override_present_but_not_applied_no_footer
--- PASS: TestBuildTelegramRendering_ModelRejectionBodyRendered_Spec088 (0.00s)
ok      github.com/smackerel/smackerel/internal/telegram/assistant_adapter
--- PASS: TestAgentInvoke_ModelFieldApplied_EnvelopeCarriesModel_Spec088 (0.00s)
--- PASS: TestAgentInvoke_OffAllowlistModel_Returns400RejectionEnvelope_Spec088 (0.00s)
--- PASS: TestAgentInvoke_NoModel_EnvelopeModelPresent_Spec088 (0.00s)
ok      github.com/smackerel/smackerel/internal/api
```

### Parity (SCN-088-A06)

Both surfaces feed the SAME `agenttool.SwitchableModels()` singleton and
the SAME `modelswitch.Resolve`: the HTTP `TestAgentInvoke_OffAllowlistModel_…`
400 envelope `message` and the Telegram `TestBuildTelegramRendering_ModelRejectionBodyRendered_…`
body are both the verbatim `modelswitch.Rejection.Message`; the
`modelswitch/allowlist_test.go::TestAllowlist_Resolve_SingleRejectionContract_Spec088`
(SCOPE-01) pins that the SAME off-allowlist string yields a byte-identical
`Rejection` every call — the one shared contract both surfaces render.
Both apply an allowlisted model via the identical `Resolve →
WithModelOverride → Run` spine.

### Files touched (SCOPE-03)

`internal/telegram/assistant_adapter/translate_inbound.go` + `_test.go`,
`render_outbound.go` + `_test.go`; `internal/api/agent_invoke.go` +
`agent_invoke_test.go` (NEW); `cmd/core/wiring_assistant_openknowledge.go`;
`deploy/contract.yaml`; `docs/Operations.md`.

---

## Test Evidence — captured (host go1.25.10; see Environment Constraint)

Every `…_Spec088` adversarial test was run RED-before (behaviour
neutralised, expected FAIL) and GREEN-after (restored, PASS); the
neutralisations were reverted verbatim before each GREEN run. Per-scope
transcripts are in the SCOPE-01 / SCOPE-02 / SCOPE-03 sections above.

| Scenario | Owning scope | Primary adversarial test | RED→GREEN |
|----------|--------------|--------------------------|-----------|
| SCN-088-A01 | SCOPE-02 | `TestAgent_SynthesisOverrideApplied_SynthesisTurnUsesOverriddenModel_Spec088` | ✅ |
| SCN-088-A02 | SCOPE-01 | `TestAllowlist_Resolve_OffListRejected_ModelNotAllowlisted_Spec088` | ✅ |
| SCN-088-A03 | SCOPE-02 | `TestAgent_NoOverride_ByteForByteBaseline_Spec088` | ✅ |
| SCN-088-A04 | SCOPE-02 | `TestAgent_TurnResultModelStamped_AllTerminalPaths_Spec088` | ✅ |
| SCN-088-A05 | SCOPE-02 | `TestAgent_TrustContractsHoldUnderOverride_Spec088` + `TestFacade_OffAllowlistOverride_ShortCircuits…` | ✅ |
| SCN-088-A06 | SCOPE-03 | `TestAgentInvoke_OffAllowlistModel_Returns400RejectionEnvelope_Spec088` + `TestTranslateInbound_ModelFlagParsedAndStripped…` | ✅ |
| SCN-088-A07 | SCOPE-01 | `TestAllowlist_Resolve_ProfiledOverEnvelopeRejected_ModelOverMemEnvelope_Spec088` | ✅ |
| SCN-088-A08 | SCOPE-02 | `TestAgent_SynthesisOverride_PreservesIterationEnvelope_Spec088` | ✅ |

The live `gemma4:26b`-vs-`deepseek-r1:7b` synthesis A/B (the proof spec
087 could not run on dev) is a SEPARATE downstream `bubbles.devops`
dispatch (GPU/Ollama-dependent, C7) — NOT claimed here.

### Concrete test files implemented (traceability)

Every Test Plan row resolves to a real file under this changeset:

- `internal/assistant/openknowledge/modelswitch/allowlist_test.go` (SCOPE-01 validator + golden + fail-loud build)
- `internal/config/openknowledge_test.go`, `internal/config/validate_ml_envelope_test.go` (SCOPE-01 config fail-loud + over-envelope)
- `internal/assistant/openknowledge/agent/modelswitch_agent_spec088_test.go` (SCOPE-02 fake-LLM A01/A03/A04/A05/A08)
- `internal/assistant/openknowledge/agenttool/substrate_tool_test.go` (SCOPE-02 MapTurnResult both arms)
- `internal/assistant/facade_modelswitch_spec088_test.go` (SCOPE-02 facade rejection short-circuit / applied attribution / baseline / nil-safe)
- `internal/assistant/contracts/response_test.go` (SCOPE-02 field inventory + golden fixture)
- `internal/telegram/assistant_adapter/translate_inbound_test.go` (SCOPE-03 `--model=` parse)
- `internal/telegram/assistant_adapter/render_outbound_test.go` (SCOPE-03 footer-on-override + rejection-body render)
- `internal/api/agent_invoke_test.go` (SCOPE-03 model field + 400 rejection envelope + baseline)

---

## Regression (whole-working-tree)

Full host unit regression across `./internal/... ./cmd/...` (the same
scope `go-unit.sh` runs `go test ./...` over; tests/integration + web are
the live-stack / container tiers, out of this in-repo mechanism-level
proof per C7):

```text
FULL_REGRESS_EXIT=0
ok packages: 119
FAIL packages: 0
--- FAIL: NONE
```

**One spec-088 ripple found and fixed in-changeset (NOT deferred).** The
first regression run surfaced exactly one red:
`cmd/config-validate::TestRun_ConstructedValidEnv_ExitsZero` — the test's
hand-built "valid env" fixture overrides the ML profile map to a fixture
model set, so the live `switchable_models=[gemma3:4b]` no longer had a
profile entry and the NEW `validateModelEnvelopes` switchable pass
correctly failed it (`missing model memory profile(s):
…switchable_models entry "gemma3:4b"`). Fixed by pointing the fixture's
`ASSISTANT_OPEN_KNOWLEDGE_LLM_MODEL_ID` / `SYNTHESIS_MODEL_ID` /
`SWITCHABLE_MODELS` at the profiled `bug-045-fixture-llm-6gib` (single
6144 MiB load ≤ the 8G fixture envelope). Re-run after the fix:
`FAIL packages: 0`. This is the guard working as designed (it caught a
fixture that now under-specifies a required profile), not a spec-088
defect.

**No out-of-changeset spec-083 / spec-073 reds in this scope.** The
spec-083 card-rewards WIP red (`tests/integration/cardrewards_extract_test.go`)
and the spec-073 node/dart container canary live in the
`tests/integration` + web tiers, which are NOT part of this unit
regression scope (and are the live-stack tiers this in-repo run does not
exercise). The do-not-touch boundary is clean — `git status --porcelain`
over `internal/cardrewards/`, `ml/app/card_categories.py`,
`ml/app/main.py`, `ml/tests/test_card_categories.py`,
`specs/083-card-rewards-companion/`,
`tests/integration/cardrewards_extract_test.go` is EMPTY.

---

## Completion Statement

The switchable-model primitive is proven in-repo at the mechanism level
(pure-validator tables + fail-loud config-generation envelope arithmetic
+ fake-LLM agent traces + facade/agenttool/contracts spine tests +
handler/adapter parity tests). All three scopes are validated-in-repo:

- **SCOPE-01** (12/12 DoD) — SST `switchable_models` allowlist + shared
  `modelswitch` validator + fail-loud co-residence envelope guard.
- **SCOPE-02** (14/14 DoD) — `AssistantMessage.ModelOverride` carrier +
  `Agent.WithModelOverride` per-request clone (singleton never mutated,
  C6) + `TurnResult.Model` attribution + facade resolve→reject→thread→
  attribute spine.
- **SCOPE-03** (13/13 DoD) — Telegram `--model=` parse + `— model:`
  footer + fail-loud rejection render; HTTP `model` field + 400 rejection
  envelope; allowlist-singleton wiring; deploy contract; operator docs.

Every spec-064/084/087 trust invariant (cite-back, provenance /
no-zero-source, capture-as-fallback, `<think>`-strip + retry-before-
salvage) is preserved model-agnostically; the no-override path is
byte-for-byte spec-087 (`TestAgent_NoOverride_ByteForByteBaseline_Spec088`
+ the unchanged spec-084/087 suites); `WriteTimeout` is unchanged at
`4200s`. `./smackerel.sh check` + `format --check` + `config generate`
(dev/test/self-hosted) all EXIT 0; the full `internal/`+`cmd/` unit
regression is `119 ok / 0 FAIL`.

**Terminal posture (C7): validated-in-repo, no commit/push.** The
decisive live `gemma4:26b`-vs-`deepseek-r1:7b` synthesis A/B on self-hosted
GPU/Ollama hardware is the separate downstream owner.
`nextRequiredOwner = bubbles.test` (then `bubbles.devops` for the live
A/B re-verify).

---

## TEST PHASE VERIFICATION (`bubbles.test`)

**Status:** ✅ TESTED (validated-in-repo). Authoritative re-run of every
spec-088 test + spec-084/087 regression + the broader Go unit suite;
A06 parity gap closed (RED→GREEN). No commit/push (C7).
`nextRequiredOwner = bubbles.security`.

### Runner correction — the dockerized repo-CLI path is NOT blocked here

The implement-phase note recorded the dockerized `./smackerel.sh test
unit --go` runner as blocked (offline `apt-get install gettext-base`
unreachable). **In this test-phase sandbox the network IS available, so
that constraint does NOT apply** — the canonical dockerized
`golang:1.25.10-bookworm` runner (`run_go_tooling` →
`scripts/runtime/go-unit.sh`) installs `gettext-base` and runs the real
`go test ./...`. All evidence below is therefore from the **authoritative
repo CLI** (`./smackerel.sh test unit --go [...]`), **not** a host
toolchain fallback. The `ensure_envsubst` bootstrap succeeds verbatim:

```text
[go-unit] envsubst missing — installing gettext-base
apt-get install -y --no-install-recommends gettext-base
Get:1 http://deb.debian.org/debian bookworm/main amd64 gettext-base amd64 0.21-12 [160 kB]
Fetched 160 kB in 0s (1161 kB/s)
Setting up gettext-base (0.21-12) ...
[go-unit] gettext-base install OK
```

### Spec-088 — 30/30 GREEN (the 29 planned + 1 new A06 parity test)

`./smackerel.sh test unit --go --go-run 'Spec088|Spec087|Spec084' --verbose`
(authoritative dockerized runner) — every `…_Spec088` PASS, across all 9
spec-088 packages. Representative real output:

```text
+ go test -v -run 'Spec088|Spec087|Spec084' -count=1 ./...
--- PASS: TestAgentInvoke_ModelFieldApplied_EnvelopeCarriesModel_Spec088 (0.00s)
--- PASS: TestAgentInvoke_OffAllowlistModel_Returns400RejectionEnvelope_Spec088 (0.00s)
--- PASS: TestAgentInvoke_NoModel_EnvelopeModelPresent_Spec088 (0.00s)
--- PASS: TestAgentInvoke_RejectionEnvelopeByteIdenticalToValidator_Spec088 (0.00s)   # NEW (A06 parity)
ok      github.com/smackerel/smackerel/internal/api
--- PASS: TestFacade_OffAllowlistOverride_ShortCircuits_NoAgentCall_NoCapture_Spec088 (0.00s)   # strengthened (A06 parity)
--- PASS: TestFacade_AppliedOverride_ThreadsAttribution_Spec088 (0.00s)
--- PASS: TestFacade_NoOverride_BaselineAttributionNotApplied_Spec088 (0.00s)
--- PASS: TestFacade_NilAllowlist_BaselinePassthrough_NoPanic_Spec088 (0.00s)
ok      github.com/smackerel/smackerel/internal/assistant
--- PASS: TestAssistantResponse_FieldInventory_ModelAttribution_Spec088 (0.00s)
ok      github.com/smackerel/smackerel/internal/assistant/contracts
--- PASS: TestAgent_SynthesisOverrideApplied_SynthesisTurnUsesOverriddenModel_Spec088 (0.00s)
    --- PASS: …/forced_final_uses_override (0.00s)
    --- PASS: …/synthesis_retry_uses_override (0.00s)
--- PASS: TestAgent_WithModelOverride_ClonesSingletonNeverMutated_Spec088 (0.00s)
--- PASS: TestAgent_NoOverride_ByteForByteBaseline_Spec088 (0.00s)
--- PASS: TestAgent_TurnResultModelStamped_AllTerminalPaths_Spec088 (0.00s)
    --- PASS: …/success_forced_final  …/honest_salvage  …/refuse_fabricated  …/early_stop_under_override_reports_gather_model
--- PASS: TestAgent_TrustContractsHoldUnderOverride_Spec088 (0.00s)
    --- PASS: …/fabricated_citation_still_refused  …/think_never_leaks_never_cited
--- PASS: TestAgent_SynthesisOverride_PreservesIterationEnvelope_Spec088 (0.00s)
ok      github.com/smackerel/smackerel/internal/assistant/openknowledge/agent
--- PASS: TestMapTurnResult_ModelCarried_BothArms_Spec088 (success_arm / refusal_arm / empty_model_omitted_in_json)
ok      github.com/smackerel/smackerel/internal/assistant/openknowledge/agenttool
--- PASS: TestAllowlist_Resolve_OffListRejected_ModelNotAllowlisted_Spec088 (0.00s)
--- PASS: TestAllowlist_RejectionMessage_GoldenWording_Spec088 (0.00s)
--- PASS: TestAllowlist_Resolve_UnprofiledRejected_ModelNotAllowlisted_Spec088 (0.00s)
--- PASS: TestAllowlist_Resolve_ProfiledOverEnvelopeRejected_ModelOverMemEnvelope_Spec088 (0.00s)
--- PASS: TestAllowlist_Resolve_BaselineEmptyReturnsZeroOverride_Spec088 (0.00s)
--- PASS: TestAllowlist_Resolve_InListAppliedToSynthesis_Spec088 (0.00s)
--- PASS: TestAllowlist_Resolve_SingleRejectionContract_Spec088 (0.00s)
--- PASS: TestAllowlist_NewAllowlist_FailLoudBuild_Spec088 (empty set / unprofiled / over-envelope / empty default / blank entry)
--- PASS: TestAllowlist_NewAllowlist_DevEnvelopeSkipped_Spec088 (0.00s)
ok      github.com/smackerel/smackerel/internal/assistant/openknowledge/modelswitch
--- PASS: TestOpenKnowledgeConfig_SwitchableModelsRequiredWhenEnabled_Spec088 (empty / missing-env / empty-entry / disabled-ok)
--- PASS: TestValidateModelEnvelopes_SwitchableOverEnvelopeRejected_Spec088 (over-envelope / missing-profile / consistent-accepted / dev-skip)
ok      github.com/smackerel/smackerel/internal/config
--- PASS: TestBuildTelegramRendering_ModelFooterOnOverrideOnly_Spec088 (override_applied / baseline_no_footer / present_not_applied)
--- PASS: TestBuildTelegramRendering_ModelRejectionBodyRendered_Spec088 (0.00s)
--- PASS: TestTranslateInbound_ModelFlagParsedAndStripped_SlashPreserved_Spec088 (flag_parsed / bare_ask / ask_only)
ok      github.com/smackerel/smackerel/internal/telegram/assistant_adapter
[go-unit] go test ./... finished OK
```

### Regression — spec-084 (9) + spec-087 (7) GREEN, broader Go unit suite 0 FAIL

The SAME verbose run carried the regression suites (a zero override is
byte-for-byte the spec-087 path):

```text
--- PASS: TestOpenKnowledgeAgentPrompt_IsQuestionAgnostic_Spec084 (0.00s)          # cmd/core
--- PASS: TestOpenKnowledgeAgentPrompt_PreservesCitationContract_Spec084 (0.00s)   # cmd/core
--- PASS: TestAgent_ReflectBeforeFinal_NudgeOnSecondToLastIteration_Spec084 (0.00s)
--- PASS: TestAgent_MultiHop_AllowsDistinctToolCallsBeforeForcedFinal_Spec084 (0.00s)
--- PASS: TestAgent_ComparisonSalvage_HonestlyFramed_BothSides_Spec084 (0.00s)
--- PASS: TestAgent_HonestSalvage_EmptyForcedFinal_FramedWithSources_Spec084 (0.00s)
--- PASS: TestAgent_HonestSalvage_UngroundedExcuse_ReplacedWithFramedFindings_Spec084 (0.00s)
--- PASS: TestAgent_GenuineSynthesis_ReturnedVerbatim_NoSalvageFrame_Spec084 (0.00s)
--- PASS: TestAgent_FabricatedCitation_StillRejected_Spec084 (0.00s)               # 9 spec-084 total
--- PASS: TestAgent_SynthesisThinkBlockStripped_VerdictReturned_Spec087 (0.00s)
--- PASS: TestAgent_ForcedFinalUsesSynthesisModel_ToolTurnsUseToolModel_Spec087 (0.00s)
--- PASS: TestAgent_ComparisonSynthesisVerdict_NotSalvage_Spec087 (0.00s)
--- PASS: TestAgent_RetryBeforeSalvage_RescuesEmptyForcedFinal_Spec087 (0.00s)
--- PASS: TestAgent_FabricatedCitationInSynthesis_StillRefused_Spec087 (0.00s)
--- PASS: TestAgent_RetryBudgetExhausted_HonestSalvage_Spec087 (0.00s)
--- PASS: TestAgent_ThinkBlockNeverLeaksNeverCited_Spec087 (0.00s)                 # 7 spec-087 agent
```

Full unfiltered broader suite — `./smackerel.sh test unit --go` (whole
`go test ./...` tree, real execution, not cached-only):

```text
ok      github.com/smackerel/smackerel/cmd/core 1.401s
ok      github.com/smackerel/smackerel/internal/api     6.265s
ok      github.com/smackerel/smackerel/internal/assistant/openknowledge/agent
ok      github.com/smackerel/smackerel/internal/assistant/openknowledge/modelswitch     0.012s
ok      github.com/smackerel/smackerel/internal/config  21.304s
ok      github.com/smackerel/smackerel/internal/telegram        27.957s
ok      github.com/smackerel/smackerel/internal/telegram/assistant_adapter
ok      github.com/smackerel/smackerel/tests/integration        (cached) [no tests to run]
ok      github.com/smackerel/smackerel/web/pwa/tests    (cached)
[go-unit] go test ./... finished OK
```

Every package is `ok` (or `[no test files]` / `[no tests to run]`) —
**0 FAIL across the whole tree.** Out-of-changeset attribution:
`tests/integration/cardrewards_extract_test.go` (spec-083 card-rewards
WIP) is `//go:build integration` and is therefore **excluded from the
unit tier** (it shows `[no tests to run]`, never executed/failed in
`--go`); the spec-073 node/dart canary is a non-Go (web/client) tier not
exercised by `./smackerel.sh test unit --go`. The do-not-touch boundary
(`internal/cardrewards/`, `ml/app/card_categories.py`, `ml/app/main.py`,
`ml/tests/test_card_categories.py`, `specs/083-card-rewards-companion/`,
`tests/integration/cardrewards_extract_test.go`) is untouched by this
test phase.

### Test integrity — adversarial subset is non-tautological (spot-checked)

Read-through + the RED demos below confirm the adversarial tests are real
(no bailout early-returns, no self-validating asserts, genuinely
distinct inputs):

- Off-allowlist uses `gpt-4o` / `totally-made-up` (genuinely absent from
  the allowlist); over-envelope uses `deepseek-r1:32b` (profiled 22528 +
  gemma4:26b gather 18432 = 40960 MiB > 28672 envelope) — distinct
  `model_over_memory_envelope` reason, not mislabeled.
- `TestAgent_NoOverride_ByteForByteBaseline_Spec088` exercises the real
  no-override path against the singleton (`WithModelOverride(Override{})`
  MUST return the receiver) and pins the per-turn model sequence
  `[gather-model, synth-model]`.
- `TestAgent_TrustContractsHoldUnderOverride_Spec088` asserts
  `StatusRefused` + `TerminationFabricatedSource` AND `<think>` never
  leaks under an override — fails if cite-back/`<think>`-strip were
  bypassed.
- `TestFacade_OffAllowlistOverride_ShortCircuits…` uses an EMPTY fake-LLM
  queue (any Chat call fails) + a recording capture policy asserting
  `decide==0 && capture==0` — proves the rejection short-circuits before
  the agent and capture-as-fallback.
- A04 early-`StopEndTurn` (finding F-ATTR-EARLY) is covered:
  `…/early_stop_under_override_reports_gather_model` asserts the tool
  model (not the un-reached synthesis override) is attributed.

### A06 parity gap CLOSED (RED→GREEN) — both surfaces emit the validator's EXACT rejection

**Gap found:** the HTTP rejection test and the facade short-circuit test
only *substring*-checked the rejection message (`"is not a switchable
model"`), and the only byte-exact rejection assertion (the Telegram
render test) used a *hardcoded golden literal*, not the validator's
output. So nothing pinned the two surfaces' user-visible rejection text
byte-identical to `modelswitch.Resolve` for the same input — the precise
SCN-088-A06 parity claim the owner flagged.

**Closure (test-only):**
- NEW `internal/api/agent_invoke_test.go::TestAgentInvoke_RejectionEnvelopeByteIdenticalToValidator_Spec088`
  — drives the REAL HTTP handler and asserts the full 400 envelope
  (`message`, `error_code`, `rejected_model`, `default_model`,
  `allowed_models`) is **byte-identical** to
  `modelswitch.Resolve(sameAllowlist, "gpt-4o")`.
- STRENGTHENED `internal/assistant/facade_modelswitch_spec088_test.go::TestFacade_OffAllowlistOverride_ShortCircuits…`
  — adds a **byte-identical** `resp.Body == Resolve(sameAllowlist,"gpt-4o").Message`
  assertion (Telegram validation seam). With `modelswitch`'s
  determinism pin (`TestAllowlist_Resolve_SingleRejectionContract_Spec088`),
  both surfaces now provably render ONE validator's exact rejection.

**RED demo 1 (HTTP)** — temporarily reformatted the handler's rejection
message (`Message: rej.Message + " (please retry)"`). The NEW byte-exact
test FAILED while the OLD substring test still PASSED (proving the new
test catches a divergence the old one missed); reverted verbatim:

```text
--- PASS: TestAgentInvoke_OffAllowlistModel_Returns400RejectionEnvelope_Spec088 (0.00s)
    agent_invoke_test.go:260: HTTP message MUST be byte-identical to the validator.
         got: …Retry e.g. /ask --model=override-model <your question> (please retry)
        want: …Retry e.g. /ask --model=override-model <your question>
--- FAIL: TestAgentInvoke_RejectionEnvelopeByteIdenticalToValidator_Spec088 (0.00s)
FAIL    github.com/smackerel/smackerel/internal/api     0.247s
```

**RED demo 2 (Telegram facade)** — temporarily reformatted the facade
rejection body (`truncateBody(rej.Message+" (please retry)", …)`). The
strengthened byte-exact assertion FAILED; reverted verbatim:

```text
    facade_modelswitch_spec088_test.go:223: Telegram rejection body MUST be byte-identical to the shared validator.
         got: …Retry e.g. /ask --model=override-model <your question> (please retry)
        want: …Retry e.g. /ask --model=override-model <your question>
--- FAIL: TestFacade_OffAllowlistOverride_ShortCircuits_NoAgentCall_NoCapture_Spec088 (0.00s)
FAIL    github.com/smackerel/smackerel/internal/assistant       0.206s
```

Both production edits were reverted verbatim (net diff to
`internal/api/agent_invoke.go` and `internal/assistant/facade.go` is
ZERO — confirmed by `grep RED-DEMO` returning nothing in `internal/`) and
the final scoped GREEN run re-passes both:

```text
--- PASS: TestAgentInvoke_RejectionEnvelopeByteIdenticalToValidator_Spec088 (0.00s)
ok      github.com/smackerel/smackerel/internal/api
--- PASS: TestFacade_OffAllowlistOverride_ShortCircuits_NoAgentCall_NoCapture_Spec088 (0.00s)
ok      github.com/smackerel/smackerel/internal/assistant
```

### Gates — `check` + `format --check` clean

```text
# ./smackerel.sh check
config-validate: ~/smackerel/config/generated/dev.env.tmp.* OK
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 16, rejected: 0
scenario-lint: OK

# ./smackerel.sh format --check
65 files already formatted
```

### Test-phase verdict

✅ **TESTED.** Spec-088 = 30/30 GREEN (29 planned + 1 new A06 parity
test); spec-084 (9) + spec-087 (7) regression GREEN; full Go unit suite
0 FAIL; A06 parity gap closed RED→GREEN; `check` + `format --check`
clean. Terminal posture validated-in-repo (C7) — NO commit/push. The
live `gemma4:26b`-vs-`deepseek-r1:7b` synthesis A/B remains the separate
downstream `bubbles.devops` proof. `nextRequiredOwner = bubbles.security`.

---

## Security Review (`bubbles.security`, 2026-06-13)

Diagnostic phase of the parent-expanded full-delivery. Scope: the
per-request model override accepted from UNTRUSTED user input on two
surfaces (Telegram `/ask --model=<id>`, HTTP `model` field) and used to
select the Ollama synthesis model. Method: static code trace of the
override→validate→clone→run→attribute spine + the trust perimeter, plus
an executed scoped test run. NO new agents dispatched; NO commit/push.

**Claim Source:** `executed` for the test evidence (scoped `./smackerel.sh
test unit --go --go-run 'Spec088'` run, 0 FAIL across 126 packages);
`interpreted` for the code-path controls (cited by file:line below).

### Threat-model checklist

| # | Threat | Verdict | Key control (file:line evidence) |
|---|--------|---------|----------------------------------|
| 1 | No arbitrary model-string passthrough to the backend | ✅ PASS | `Resolve` trims + EXACT-matches the SST allowlist BEFORE any agent build — only an exact-match string becomes `Override.SynthesisModel` ([allowlist.go](../../internal/assistant/openknowledge/modelswitch/allowlist.go#L139-L149)). HTTP rejects pre-`Run` ([agent_invoke.go](../../internal/api/agent_invoke.go#L142-L160)); Telegram/facade rejects then `break`s the `switch` case → NO agent call, NO capture ([facade.go](../../internal/assistant/facade.go#L1063-L1090)). `WithModelOverride` only mutates a per-request CLONE ([agent.go](../../internal/assistant/openknowledge/agent/agent.go#L268-L275)). Proven by `TestFacade_OffAllowlistOverride_ShortCircuits_NoAgentCall_NoCapture_Spec088`. |
| 2 | Injection surface (model id → Ollama) | ✅ PASS | The id reaches Ollama ONLY as a JSON field value (`Model string json:"model"`) via `json.Marshal` → `POST <endpoint>/llm/chat` — NOT concatenated into the URL path/shell/template ([llm/client.go](../../internal/assistant/openknowledge/llm/client.go#L93-L168)). Exact-match allowlist makes injection doubly moot. Rejected raw string is `%q`-escaped in the message ([allowlist.go](../../internal/assistant/openknowledge/modelswitch/allowlist.go#L259-L279)), JSON-encoded on HTTP 400, and MarkdownV2-escaped on Telegram ([render_outbound.go](../../internal/telegram/assistant_adapter/render_outbound.go#L104-L110)). No secret/PII in the rejection (allowed models + default + the rejected token only). |
| 3 | SST / NO-DEFAULTS integrity | ✅ PASS | Override is a per-request PARAMETER; the SST singleton `Agent` is never mutated (shallow clone, [agent.go](../../internal/assistant/openknowledge/agent/agent.go#L268-L275)). Baseline `SynthesisModel` stays REQUIRED + fail-loud G028 ([agent.go](../../internal/assistant/openknowledge/agent/agent.go#L207)). Envelope-consistency fails loud at config-gen ([config.go](../../internal/config/config.go#L2288-L2310)) AND at wiring (`NewAllowlist`, [allowlist.go](../../internal/assistant/openknowledge/modelswitch/allowlist.go#L84-L120)); `deepseek-r1:32b` (40960 > 28672) is structurally excluded — no `${VAR:-default}` introduced. |
| 4 | Trust perimeter unchanged under override | ✅ PASS | `<think>`-strip runs UNCONDITIONALLY on every `StopEndTurn` and each synthesis retry, BEFORE parse/salvage/cite-back ([agent.go](../../internal/assistant/openknowledge/agent/agent.go#L431) + [L454](../../internal/assistant/openknowledge/agent/agent.go#L454)); cite-back verify + `Decide(enforce)` and the provenance gate are model-agnostic and SHARED by the clone (only `cfg.SynthesisModel` differs). Capture-as-fallback is the facade's downstream model-agnostic hook ([facade.go](../../internal/assistant/facade.go#L1190)); it is correctly skipped only for a REJECTED request (request validation, not a no-ground agent run). |
| 5 | DoS / resource | ✅ PASS | Override swaps the synthesis model on the EXISTING forced-final turn + retries — adds ZERO turns; `WriteTimeout = (max_iterations + synthesis_retry_budget) × llm_timeout_ms = 4200s` unchanged; per-query token/iteration/USD budgets bound every request; compare-both deferred ⇒ no 2× amplification path. Envelope guard blocks switching to a model that OOMs the shared host. |
| 6 | Secrets/PII + dependency/supply-chain | ✅ PASS | No new secret (override carries a model id only; LLM `AuthToken` unchanged). No new external egress (model selects among already-wired Ollama models on the same `/llm/chat` endpoint). No new dependency — `go.mod`/`go.sum` unchanged; `modelswitch` is stdlib-only (`fmt`, `strings`). |

### Additional bypass analysis (all fail-closed / fail-safe)

- **Nil allowlist is fail-SAFE, not fail-open:** when `SwitchableModels()`
  is nil the raw override is IGNORED and the baseline model runs — the raw
  string never reaches the backend (`TestFacade_NilAllowlist_BaselinePassthrough_NoPanic_Spec088`).
  In production the allowlist is installed under the same gate as the agent
  ([wiring](../../cmd/core/wiring_assistant_openknowledge.go#L256-L273)).
- **Scenario scoping:** `ModelOverride` is parsed only on the `/ask`
  (open_knowledge) line and consumed only in the open_knowledge fast-path;
  no other scenario reads it.
- **Whitespace/case evasion:** `TrimSpace` + EXACT match ⇒ a whitespace-only
  override is baseline (safe); a case-variant (`GEMMA4:26B`) is rejected
  (fail-closed). No normalization can make a crafted string match an entry
  it shouldn't.
- **Fork B isolation:** the override re-points the synthesis turn ONLY; a
  test guards against it leaking onto the gather turn
  (`TestAgent_SynthesisOverrideApplied_SynthesisTurnUsesOverriddenModel_Spec088`).

### Findings

- **SEC-088-01 — LOW / informational (DEFERRED, no fix required).** The
  HTTP 400 rejection envelope reflects the raw rejected `model` string
  (`rejected_model` + `message`) with no explicit length cap; a large
  `model` field (bounded by the existing 64 KiB body limit) is echoed back
  to the caller. This is NOT a vulnerability: the value is JSON-escaped
  (no XSS — `application/json`), `%q`-quoted, self-reflected only to the
  authenticated (spec-044 bearer-gated) caller, never stored, never logged,
  never cross-user, and bounded by the 64 KiB cap (~3× echo at worst). For
  the single-operator self-hosted threat model this is negligible. Optional
  cheap hardening (not blocking): cap the override length before
  reflection. Recorded for transparency; no owner routed.

### Confirmations

- **No secret added; no new egress; no new dependency** — confirmed
  (item 6; `go.mod`/`go.sum` diff empty).
- **No unsafe log echo** — no `slog`/`log` of `req.Model` / `msg.ModelOverride`
  / `RejectedModel` anywhere in the changeset (grep clean); the wiring
  startup log emits only the operator-curated SST `switchable_models` list.
- **Do-not-touch respected** — review was read-only over the spec-088
  changeset; `internal/cardrewards/`, `ml/app/*`, spec 083, and the
  cardrewards integration test were not inspected or modified.

### Executed evidence — scoped spec-088 unit run

```text
# ./smackerel.sh test unit --go --go-run 'Spec088' --verbose
FAIL count: 0          (126 ok packages with spec-088 tests; 0 FAIL)
--- PASS: TestAgentInvoke_OffAllowlistModel_Returns400RejectionEnvelope_Spec088
--- PASS: TestAgentInvoke_RejectionEnvelopeByteIdenticalToValidator_Spec088
--- PASS: TestFacade_OffAllowlistOverride_ShortCircuits_NoAgentCall_NoCapture_Spec088
--- PASS: TestFacade_AppliedOverride_ThreadsAttribution_Spec088
--- PASS: TestFacade_NoOverride_BaselineAttributionNotApplied_Spec088
--- PASS: TestFacade_NilAllowlist_BaselinePassthrough_NoPanic_Spec088
--- PASS: TestAgent_WithModelOverride_ClonesSingletonNeverMutated_Spec088
--- PASS: TestAllowlist_Resolve_OffListRejected_ModelNotAllowlisted_Spec088
--- PASS: TestAllowlist_Resolve_UnprofiledRejected_ModelNotAllowlisted_Spec088
```

### Security verdict

🔒 **SECURE.** All six threat-model items PASS with file:line + executed
test evidence. One LOW/informational observation (SEC-088-01) recorded,
non-blocking, no fix required for the single-operator threat model. The
core risk — an arbitrary/off-allowlist/over-envelope model string reaching
the Ollama backend — is structurally impossible: exact-match allowlist
validation runs before any agent build, rejection short-circuits with NO
agent call and NO capture, and the trust perimeter (cite-back, provenance,
capture-as-fallback, `<think>`-strip + retry-before-salvage) is
model-agnostic and shared by the per-request clone.
`nextRequiredOwner = bubbles.validate`.

---

## Validation Report (`bubbles.validate`)

**Status:** ✅ VALIDATED-IN-REPO → terminal status set to `blocked`
(validated-in-repo, owner-directed devops handoff). Deep claims-vs-reality
validation (mode = `certification-required`). Every gate below was
**executed** via `run_in_terminal` (execution-only, Gate G071 — no
predicted/file-read findings); transcripts captured verbatim. No agents
dispatched; no commit/push (C7).

### Gate results (executed — real exit codes)

| Gate | Command | Exit | Result |
|------|---------|------|--------|
| Artifact lint | `bash .github/bubbles/scripts/artifact-lint.sh specs/088-runtime-switchable-models` | 0 | ✅ PASSED (2 non-blocking ⚠️: legacy uservalidation layout, deprecated `scopeProgress` field) |
| Traceability guard | `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/088-runtime-switchable-models` | 0 | ✅ PASSED, 0 warnings — 8 SCN-088-A01..A08 → Test Plan rows → concrete test files → report evidence → DoD fidelity 8/8 mapped |
| Check (build+vet+config-sync+scenario-lint) | `./smackerel.sh check` | 0 | ✅ EXIT 0 |
| Format | `./smackerel.sh format --check` | 0 | ✅ EXIT 0 — 65 files already formatted |
| Spec-088 tests (dockerized repo CLI) | `./smackerel.sh test unit --go --go-run 'Spec088' --verbose` | 0 | ✅ 30/30 GREEN across all 9 spec-088 packages; `go test ./... finished OK` |
| Implementation reality scan (scoped) | `bash .github/bubbles/scripts/implementation-reality-scan.sh specs/088-runtime-switchable-models --verbose` | 0 | ✅ 0 violations across 19 resolved files (1 non-blocking ⚠️) |

**Runner-correction confirmed independently.** The dockerized
`golang:1.25.10-bookworm` runner is **NOT blocked** in this session:
`apt-get install gettext-base` succeeds (network available) and the real
`go test ./...` runs. This validates the test-phase RUNNER CORRECTION over
the implement-phase "offline apt" note — the spec-088 test evidence is from
the canonical repo CLI, not a host fallback.

### Claims-vs-reality verdict (per scope)

| Scope | DoD | Claim verified | Verdict |
|-------|-----|----------------|---------|
| SCOPE-01 (SST allowlist + `modelswitch` validator) | 12/12 | `modelswitch` validator table tests + config fail-loud + over-envelope tests GREEN (ran); validator source asserts exact-match allowlist, two reason-codes, byte-exact golden wording; adversarial inputs are real (`gpt-4o`, `totally-made-up`, `deepseek-r1:32b` 40960>28672) | ✅ REAL |
| SCOPE-02 (override threading + clone + attribution) | 14/14 | agent fake-LLM traces GREEN (ran); read tests confirm C6 singleton-never-mutated, byte-for-byte baseline turn sequence `[gather-model, synth-model]`, trust contracts hold under override (fabricated citation → `StatusRefused`+`TerminationFabricatedSource`; `<think>` never leaks into `FinalText`/never cited); facade short-circuit uses empty LLM queue asserting 0 agent calls + 0 capture | ✅ REAL |
| SCOPE-03 (two-surface parity + fail-loud + docs) | 14/14 | telegram adapter + HTTP api parity tests GREEN (ran); HTTP 400 rejection envelope byte-identical to `modelswitch.Resolve`; Telegram footer only-on-override; the same shared validator drives both surfaces | ✅ REAL |

**Anti-fabrication finding:** every evidence block I re-executed
(spec-088 tests, `check`, `format`, artifact-lint, traceability-guard,
reality-scan) is backed by **real** executed output that matches the
report's claims. Note: state-transition-guard Check 11 WARNs that 17/22
report evidence blocks are summarized (lack raw exit-code/PASS-count
signals); this is an evidence-shape note, not fabrication — the underlying
claims are corroborated by my independent re-execution (30/30 GREEN, all
gates pass).

### Trust-perimeter verification (preserved under override)

- **Cite-back enforce holds** — a fabricated citation under an allowlisted
  override yields `StatusRefused` + `TerminationFabricatedSource`; runtime
  log: `openknowledge_citeback_mismatch … mode=enforce rejected_count=1 refused=true`.
- **Provenance + capture-as-fallback** — facade fires capture only when the
  agent runs; a **rejected** override short-circuits with `0` agent calls
  and `0` capture calls (pre-agent request validation).
- **`<think>`-strip + retry-before-salvage** — `<think>` content never
  appears in `FinalText` and is never cited under override; the no-override
  path is **byte-for-byte spec-087**.
- **SST fail-loud / NO-DEFAULTS (G028)** — `./smackerel.sh check` config
  sync passes; no `${VAR:-default}`; `switchable_models` is REQUIRED + fail
  loud; the singleton is never mutated (per-request clone, C6).

### Out-of-changeset attribution (confirmed by FILE PATH)

The spec-088 isolated changeset is clean on its own. Whole-working-tree
guard noise is attributable to the do-not-touch spec-083/073 WIP:

- **G028 (whole-tree reality scan)** — `ml/app/main.py:22` and
  `ml/app/main.py:257` (`DEFAULT_FALLBACK`), the spec-083 card-rewards WIP
  (do-not-touch, C5). NOT a spec-088 file; `ml/` is absent from
  `git status --porcelain`. The spec-088-scoped reality scan is **0
  violations**. Identical to the spec-087 precedent finding.
- **G089 (inter-spec dependency)** — `088 → 087 (blocked) → 084 (blocked)`;
  the `064→084→087→088` chain is validated-in-repo awaiting the same devops
  handoff. Identical pattern to spec-087 (which depended on blocked 084).
- **spec-073 node/dart canary** — non-Go (web/client) tier, not exercised
  by `./smackerel.sh test unit --go`.
- **Do-not-touch boundary EMPTY** — `git status --porcelain` over
  `internal/cardrewards/`, `ml/app/card_categories.py`, `ml/app/main.py`,
  `ml/tests/test_card_categories.py`, `specs/083-card-rewards-companion/`,
  `tests/integration/cardrewards_extract_test.go` shows zero changes.

### Terminal status set (mirrors the spec-087 precedent)

`state.json` → `status: blocked`, `certification.status: blocked`,
`certifiedAt: null` (blocked is NOT a certified-done state),
`completedScopes: [SCOPE-01, SCOPE-02, SCOPE-03]`,
`certifiedCompletedPhases: [analyze, ux, design, plan, implement, test,
security, validate]`, `nextRequiredOwner: bubbles.devops`. The state-guard
**correctly refuses `done`** (28 BLOCKs at `blocked`); for a blocked spec
this is expected — the guard enumerates the unmet `done`-ceiling gates.

### Residual state-guard findings (NOT in-repo substance failures)

| Category | Checks | Disposition |
|----------|--------|-------------|
| Devops-owned `done` ceiling | commit/push (C7) + live GPU `gemma4:26b`-vs-`deepseek-r1:7b` A/B | Owned by the `bubbles.devops` dispatch — the substantive blocker |
| Dependency chain | Check 31 / G089 (`088→087→084` blocked) | Resolves when devops promotes the chain |
| Subsequent parent-expanded phases not yet run | Check 6 / G022 (`regression, simplify, gaps, harden, stabilize, audit, chaos, docs`) | `bubbles.audit` is the next phase; pipeline continues post-validate |
| Artifact-hygiene vs spec-087 precedent (plan/implement-owned; non-blocking for substance) | 4B/G041 (`[x] Done` → canonical `Done`), 18/G040 (`compare-both deferred` wording — a legitimately scoped-out design decision, fork C), 5A (stress DoD), 8A (E2E-regression DoD item), 13B/G053 (`### Code Diff Evidence` section), 22/G068 (A06/A08 DoD-Gherkin keyword fidelity) | Normalize at done-promotion (foreign-owned; not validate-fixable, no dispatch per owner directive) |

None of the residual findings touch the validated in-repo substance
(code, 30/30 tests, trust perimeter, do-not-touch boundary, security
SECURE). They are the gap between `blocked` and `done`, owned downstream.

### Ownership Routing Summary

| Finding | Owner | Reason | Re-validation |
|---------|-------|--------|---------------|
| Artifact-hygiene gaps (4B/18/5A/8A/13B/22) | `bubbles.plan` / `bubbles.implement` | scopes.md scope-status + DoD shape + report code-diff section are foreign-owned; surfaced for the done-promotion cleanup (no dispatch per owner directive C7) | at done-promotion |
| Done-ceiling (push + live A/B) + dependency chain | `bubbles.devops` | owner-directed handoff; not satisfiable in-repo | post-apply |

## ROUTE-REQUIRED

NONE (validated-in-repo; terminal status `blocked`; next parent-expanded
phase is `bubbles.audit`; the substantive done-ceiling owner is
`bubbles.devops`).

### Validation verdict

✅ **VALIDATED-IN-REPO.** The switchable-model primitive is proven in-repo
at the mechanism level with REAL executed evidence (independently
re-verified): 40/40 DoD, 30/30 spec-088 tests GREEN, trust perimeter
preserved model-agnostically, no-override path byte-for-byte spec-087, SST
fail-loud intact, do-not-touch boundary clean. Terminal status `blocked`
(validated-in-repo) — the `done` ceiling (owner-forbidden commit/push + the
GPU/self-hosted live A/B) is the separate downstream `bubbles.devops` dispatch.
This is NOT a `done` certification and NOT a clean-guard pass; the
state-guard correctly refuses `done`. `nextRequiredOwner = bubbles.devops`
(state.json terminal owner); next parent-expanded diagnostic phase =
`bubbles.audit`.

---

## Final Audit Report (`bubbles.audit`)

**Feature:** Runtime-Switchable Models (spec 088)
**Date:** 2026-06-14
**Phase:** parent-expanded full-delivery — final policy + ship-readiness
sign-off (diagnostic; read-only; NO commit/push, C7).
**Scope of this audit:** policy/ship-readiness sign-off on top of the
`bubbles.validate` claims-vs-reality pass — NOT a wholesale re-run of
validate. Independent anti-fabrication corroboration via the
state-transition-guard, artifact-lint, the do-not-touch git boundary, a
full changeset attribution sweep, and a code-level read of the
NO-DEFAULTS + trust-perimeter seams.

### Audit Results

| Category | Checks | Passed | Failed (substance) |
|----------|--------|--------|--------------------|
| NO-DEFAULTS / SST (G028) | 4 | 4 | 0 |
| Trust perimeter (model-agnostic) | 5 | 5 | 0 |
| Deployment-ownership boundary | 2 | 2 | 0 |
| Build-Once Deploy-Many (deploy contract) | 1 | 1 | 0 |
| Product Principle Alignment (P8) | 1 | 1 | 0 |
| Do-not-touch boundary (C5) | 1 | 1 | 0 |
| Terminal-status correctness | 1 | 1 | 0 |
| **Total** | **15** | **15** | **0** |

### Policy checklist — independently verified

1. **NO-DEFAULTS / SST (G028) — ✅ PASS.** `assistant.open_knowledge.switchable_models`
   is REQUIRED + fail-loud, NOT a hidden default. `OpenKnowledgeConfig.Validate()`
   returns early when `!Enabled` (openknowledge.go:176), so the non-empty
   guard (openknowledge.go:239 — `len(SwitchableModels)==0 ⇒ error`) fires
   exactly when the capability is enabled — the established `tool_allowlist`
   pattern. No `${VAR:-default}` / `getEnv(k,default)` introduced (grep
   `SWITCHABLE_MODELS:-` / `SWITCHABLE_MODELS-` returned 0). The `config.sh`
   `="[]"` terminal assignment is the same generation-time empty-list shape
   `tool_allowlist` uses (legal only when disabled; the Go layer fails loud
   when enabled), NOT a runtime-hiding shell fallback. Envelope-consistency
   is enforced fail-loud in `validateModelEnvelopes` (config.go:2288–2310 —
   per-entry `model_memory_profiles` lookup + co-residence arithmetic vs the
   ollama envelope). Grounded in real SST values: self-hosted
   `ollama_memory_limit: "28G"` (28672 MiB); `gemma4:26b` 18432 (gather,
   single load); `deepseek-r1:7b` 4864 (co-resident 18432+4864 = 23296 ≤
   28672); `deepseek-r1:32b` (18432+22528 = 40960 > 28672) is structurally
   excluded. `config generate self-hosted` EXIT 0 proves the fail-loud validator
   *accepts* the self-hosted set — no switchable entry can OOM the host.
2. **Trust perimeter (the product's core promise) — ✅ PASS.** The override
   changes WHICH model runs, never the grounding/citation enforcement.
   `Agent.WithModelOverride` (agent.go:268) is a shallow per-request clone
   that rewrites ONLY `clone.cfg.SynthesisModel`; the SST singleton receiver
   is never mutated (C6) and a zero override returns the receiver (no-override
   path byte-for-byte spec-087). The facade (facade.go:1063–1090) resolves the
   UNTRUSTED `msg.ModelOverride` BEFORE the agent runs; an off-allowlist /
   un-profiled / over-envelope model becomes a rejection `AssistantResponse`
   (`ErrModelNotSwitchable` + the validator's verbatim `rej.Message`) and
   `break`s — no agent call, no capture. cite-back, provenance/no-zero-source,
   capture-as-fallback (inviolable), the spec-087 `<think>`-strip, and
   retry-before-salvage are all downstream of `finalize`, shared by the clone,
   and model-agnostic. Corroborated by the security phase (file:line) +
   validate (executed) + this audit's code read.
3. **Deployment-ownership boundary (NON-NEGOTIABLE) — ✅ PASS.** No
   environment-specific content leaked. The self-hosted override
   `assistant_open_knowledge_switchable_models: [ "gemma4:26b", "deepseek-r1:7b" ]`
   (smackerel.yaml:2069) is a generic model-tag list expressed through the
   SAME `environments.<env>.assistant_open_knowledge_*` shape as spec-087's
   `assistant_open_knowledge_synthesis_model_id: "deepseek-r1:7b"`
   (smackerel.yaml:2059) — abstract per-env config, not operator-coupled
   topology. No real hostnames/IPs/tailnet IDs/secrets.
4. **Build-Once Deploy-Many / deploy contract — ✅ PASS.**
   `deploy/contract.yaml:187` adds the abstract contract path
   `assistant.open_knowledge.switchable_models` (`type: string[]`,
   `secret: false`, generic per-env-override note) — a contract-surface
   addition, not target-specific final config.
5. **Product Principle Alignment — ✅ PASS.** spec.md §10 declares the
   section; Principle 8 (Trust Through Transparency, PRIMARY) is genuinely
   *implemented*, not merely asserted: the answering model is attributed
   (`TurnResult.Model` stamped in `finalize` → `outputEnvelope.model` always
   on HTTP + the `— model:` footer only-on-override on Telegram), the switch
   is explicit (operator `--model=` / API `model`, never silent), and an
   invalid model is met with a fail-loud rejection that lists the allowed set
   (two reason-codes) — never a silent swap/default/fallback.
6. **Do-not-touch boundary (C5) — ✅ PASS.** `git status --porcelain` over
   `internal/cardrewards/`, `ml/app/card_categories.py`, `ml/app/main.py`,
   `ml/tests/test_card_categories.py`, `specs/083-card-rewards-companion/`,
   `tests/integration/cardrewards_extract_test.go` is **EMPTY**. The
   out-of-changeset guard failures are attributable by file path and are NOT
   spec-088-introduced: the whole-tree G028 `ml/app/main.py:22/:257`
   `DEFAULT_FALLBACK` is the spec-083 WIP (the spec-088-scoped reality scan —
   state-guard Check 16 — passes with **0 violations**; `ml/` is absent from
   `git status`); G089 is the `088→087→084` dependency chain (all
   validated-in-repo awaiting the same devops handoff); the spec-073 node/dart
   canary is a non-Go tier not exercised by `--go`.
7. **Terminal-status correctness — ✅ PASS.** `blocked` (validated-in-repo)
   is the correct terminal-for-session posture — NOT a forced `done`, NOT a
   fabricated live result. `certifiedAt` stays `null`; `blockedReason` is
   accurate; `nextRequiredOwner = bubbles.devops`. The state-transition-guard
   **correctly refuses `done`** (28 BLOCKs, exit 1) and every BLOCK is
   attributable to either the devops-owned `done` ceiling or known
   non-substance artifact-hygiene (table below) — **zero** BLOCKs are
   spec-088 substance failures. Mirrors the spec-087 precedent.

### State-transition-guard BLOCK attribution (28 BLOCKs — none are substance)

| Source | BLOCKs | Disposition |
|--------|--------|-------------|
| G022 phases not-yet-run (regression, simplify, gaps, harden, stabilize, **audit**, chaos, docs) | 9 | Expected — subsequent parent-expanded phases; `audit` runs now (this record resolves it); the rest continue post-audit |
| G089 inter-spec dependency (`088→087→084` blocked) | 1 | Resolves when devops promotes the chain |
| G041 `[x] Done` non-canonical scope status | 4 | Artifact-hygiene; plan-owned; done-promotion cleanup |
| 5A SLA stress-coverage DoD item | 1 | Artifact-hygiene; stress test exists (`tests/stress/openknowledge_p95_test.go`), DoD checkbox absent |
| 8A E2E-regression DoD items (scenario + broader) | 7 | Artifact-hygiene; Test Plan E2E rows exist; live E2E is the devops dispatch (C7) |
| G053 `### Code Diff Evidence` heading | 1 | Artifact-hygiene; delivery delta IS present (G093 passes; git diff captured by this audit) |
| G040 `compare-both deferred` wording | 3 | A legitimately scoped-out design decision (Fork C), not hidden incomplete work |
| G068 DoD-Gherkin keyword fidelity (A06/A08) | 2 | Behaviors ARE tested (A08 `…PreservesIterationEnvelope…`; A06 parity tests); strict keyword matcher only |

**Anti-fabrication corroboration (all PASS, independently re-run by this audit):**
state-guard Check 4 (40/40 DoD checked), Check 9 (all 40 have evidence
blocks), Check 10 (no template placeholders), Check 12 (no duplicate
evidence), Check 13 (artifact-lint exit 0 — also re-run standalone, exit 0),
Check 16/G028 (spec-088-scoped reality scan — 0 stub/fake/hardcoded), Check
20/G021 (no cloned evidence), Check 29B/G093 (delivery delta present). The
Check 11 WARN (17/22 evidence blocks summarized) is an evidence-*shape* note,
not fabrication — corroborated by validate's independent 30/30 GREEN re-run
and this audit's mechanical + code-level verification; consistent with the
spec-087 precedent.

### Audit-added findings (LOW — non-blocking; for the done-promotion cleanup)

- **AUD-088-01 (LOW) — DoD count inconsistency.** scopes.md Scope Table and
  the report Completion Statement say "SCOPE-03 = 13 / 13/13 DoD", but the
  actual SCOPE-03 checkbox count is **14** (7 Tier-1 + 7 Tier-2), matching
  `state.json.execution.scopeProgress` (14/14) and the state-guard Check 4
  total (40 = 12+14+14). The `13` is a stale miscount in two summary spots;
  all 14 items are genuinely checked + evidenced. No substance impact.
- **AUD-088-02 (LOW) — report file-inventory under-inclusive.** Three benign
  spec-087 ripple one-liners (a `SynthesisModel:` field add to the test
  fixtures in `tests/e2e/openknowledge/`, `tests/integration/openknowledge/`,
  `tests/stress/openknowledge_p95_test.go`) are in the working tree but not
  itemized in the Change Manifest / Files-touched. They are attributable to
  the co-resident, uncommitted spec-087 work (not spec-088, not do-not-touch),
  and are harmless (+1 line each). Note for devops: this working tree
  conflates the uncommitted **087 + 088** changes — isolating "the spec-088
  commit" requires sequencing 087 first (or together), consistent with the
  G089 dependency-chain handoff.

### Artifact-hygiene judgment (per the owner's explicit ask)

None of the validate-flagged artifact-hygiene deviations (G041 `[x] Done`,
G040 deferral wording, 5A/8A DoD shape, G053 heading, G068 keyword fidelity)
are **ship-blocking for the validated-in-repo milestone**. They are
cosmetic/mechanical, foreign-owned (plan/implement), and become relevant only
at the `done`-promotion — which is itself the downstream `bubbles.devops`
dispatch. They are NOT fabrication, NOT do-not-touch violations, NOT
security/policy issues. The audit-added items (AUD-088-01/02) are likewise
non-blocking.

### Devops handoff summary (what `bubbles.devops` must do for `done`)

1. Isolate the spec-088 changeset (sequencing the co-resident spec-087 work
   first/with it — G089 chain) and commit + push it through CI.
2. Build + sign the core/ML images (cosign keyless + Rekor + SBOM/SLSA) and
   generate the per-env config bundle that carries the
   `switchable_models` allowlist.
3. Ensure `deepseek-r1:7b` is present on the self-hosted Ollama host and the
   28 GiB envelope holds; apply the bundle (pointer-swap) to self-hosted.
4. Run the live `gemma4:26b`-vs-`deepseek-r1:7b` synthesis A/B on real
   GPU/Ollama hardware (the proof spec 087 could not run on dev) +
   the `tests/e2e/openknowledge` regression on the live stack.
5. On green, promote the `088→087→084` chain to `done`; at that
   promotion, normalize the artifact-hygiene items above (plan/implement).

### Spot-Check Recommendations (automation-bias mitigation)

1. **Evidence shape (17/22 summarized blocks).** The report uses
   representative/filtered test output, not full raw transcripts. Validate
   independently re-ran 30/30 GREEN; if you want first-hand confirmation,
   run `./smackerel.sh test unit --go --go-run 'Spec088' --verbose` once.
2. **The live A/B is the real remaining proof.** Every in-repo claim is
   mechanism-level (fake-LLM traces, validator tables). The decisive
   `gemma4:26b`-vs-`deepseek-r1:7b` synthesis quality comparison is the
   devops self-hosted run — verify it produces a grounded, cited, correctly
   attributed verdict on the real models before trusting the A/B result.
3. **Envelope headroom on self-hosted.** The 28 GiB envelope fits the
   switchable set arithmetically (23296 ≤ 28672); confirm real resident
   memory under the concurrent interactive working-set + the on-demand
   `deepseek-r1:7b` during the live run.
4. **DoD count (AUD-088-01).** Confirm SCOPE-03 is 14 items, not 13, when
   normalizing the summaries at done-promotion.

### Verdict

⚠️ **SHIP_WITH_NOTES — validated-in-repo milestone APPROVED; route to
`bubbles.devops` for the `done` ceiling.** The spec-088 in-repo deliverable
is sound: NO substance, policy (G028/SST, deployment-ownership,
Build-Once-Deploy-Many, Principle 8), security, or anti-fabrication blocker
was found. The trust perimeter is preserved model-agnostically; the
no-override path is byte-for-byte spec-087; the do-not-touch boundary is
clean; the terminal `blocked` status is honest and correct. This is **NOT**
a clearance to flip `state.json` to `done` — the state-guard correctly
refuses `done`, and the only work between `blocked` and `done` is the
owner-directed `bubbles.devops` handoff (commit/push + the live GPU A/B),
which is by design, not a defect. The artifact-hygiene notes (validate's +
AUD-088-01/02) are non-blocking and owned by plan/implement at
done-promotion. `nextRequiredOwner = bubbles.devops`.

## ROUTE-REQUIRED

Owner: `bubbles.devops` — finalize the `done` ceiling for the
`088→087→084` validated-in-repo chain: isolate + commit + push the spec-088
changeset, build/sign images, generate + apply the self-hosted config bundle
carrying the `switchable_models` allowlist (ensure `deepseek-r1:7b` resident
within the 28 GiB envelope), run the live `gemma4:26b`-vs-`deepseek-r1:7b`
synthesis A/B + `tests/e2e/openknowledge` regression, then promote the chain.
No spec-088 substance rework is required.

## DevOps Execution Outcome + Operator Runbook (2026-06-14)

`bubbles.devops` ran the commit/push/deploy/A-B dispatch for the
`064→084→087→088` chain. **Claim Source: executed** for STEP 1–3 (git + `gh run`
observed); **blocked-on-operator** for STEP 4–5 (no live result fabricated).

> **SHA reconciliation (2026-06-24):** the commit pushed below as `99c8d629` was rebased to **`9d0716b3`** during reconcile-20260612 (`f686b88d`); `9d0716b3` is the current on-main commit carrying the combined 087+088 work (verify: `git show 9d0716b3`). Every `99c8d629` reference below — the commit, the push range, the CI re-run target, `build-manifest-99c8d629.yaml`, and the `self-hosted-99c8d629` config bundle — should be read as `9d0716b3` / `build-manifest-9d0716b3.yaml` / `self-hosted-9d0716b3`. The historical STEP 1–2 text records the original push SHA and is left intact.

### What completed (executed)

| Step | Outcome |
|------|---------|
| 1 — commit | DONE — combined 087+088 commit `99c8d629` (50 files, 8690 insertions). The 088 `--model` override (`modelswitch` pkg) + 087 split-synthesis hunks co-mingle across `agent.go`/`facade.go`/config; a clean per-spec hunk-split was error-prone, so both ship intact (a clean combined commit beats a broken split). pii-scan clean; transient `clients/.../.kotlin` excluded. |
| 2 — push | DONE — `origin/main 10ed4a48..99c8d629`; pre-push uniformity lint PASSED; no `--no-verify`. Also carried the pre-existing unpushed spec-085, spec-086, framework-7.12.0 commits. |
| 3 — CI | CI workflow (lint-and-test + cross-language-canary + build) **GREEN** — 088 Go validated on origin/main (agent-invoke model-override path, `switchable_models` allowlist, facade two-surface parity). `build-images` ✓ (core+ML cosign keyless+Rekor signed, SBOM+SLSA attested, ghcr digest push), `build-bundles` (dev/test/self-hosted) ✓. |

### What is blocked-on-operator (no result fabricated)

- **STEP 3 gap — no build manifest.** `build-clients` ✗ (operator-private Android upload keystore secret missing) → `publish-build-manifest` was SKIPPED (`needs: build-clients`) → **`build-manifest-99c8d629.yaml` was NOT published**. Build-Once Deploy-Many therefore has no deploy input.
- **STEP 4 — deploy.** Reachability is fine (self-hosted reachable via tailscale root ssh; deploy-adapter overlay present; cosign installed), but (a) no build manifest, and (b) `deepseek-r1:7b` is NOT resident on the self-hosted ollama (`gemma4:26b` IS, 17 GB). The live stack still runs the pre-088 build, so the wire does not yet honor `--model`.
- **STEP 5 — the decisive A/B.** Requires the 088 code deployed + `deepseek-r1:7b` resident; cannot be run meaningfully against the pre-088 live stack.

### Findings (operator-private CI secrets — surfaced by the push; all in spec-085/086, NOT 087/088)

1. **`build-clients` ✗** — "Materialize Android upload keystore (operator-private secret)". Configure the Android upload keystore CI secret.
2. **`client-binary-conformance` ✗** — "Input required and not supplied: token" at the deploy-adapter-overlay checkout step. Configure the overlay checkout token CI secret.
3. **`Gitleaks` ✗** — FALSE POSITIVE on the test constant `lcbSecretSentinel = "s3cr3t-lcb-do-not-leak-DEADBEEF"` in `internal/deploy/local_client_build_test.go:142` (spec-086 commit `a243a465`, rule `generic-api-key`). Will NOT recur on the next push (gitleaks scans only the new commit range, which excludes `a243a465`). To durably baseline it, append this fingerprint to `.gitleaksignore`: `a243a4656c991696f6aa2cd3eb7d14d9fb6623bf:internal/deploy/local_client_build_test.go:generic-api-key:142`.

### Operator runbook — finish STEP 4–5

Substitute `<deploy-host>` / `<self-hosted-core-fqdn>` with your real (operator-private) values.

**A. Unblock the build manifest (CI secrets → re-run the build):**
1. Add the Android upload keystore + deploy-overlay checkout token CI secrets.
2. Re-run the `build` workflow on `99c8d629`; confirm `publish-build-manifest` writes `build-manifest-99c8d629.yaml`.

**B. Ensure both synthesis models resident on the self-hosted ollama (within the 28672 MiB envelope):**

```bash
tailscale ssh root@<deploy-host> -- 'docker exec smackerel-self-hosted-ollama-1 ollama pull deepseek-r1:7b'
tailscale ssh root@<deploy-host> -- 'docker exec smackerel-self-hosted-ollama-1 ollama list'   # expect deepseek-r1:7b + gemma4:26b
```

**C. Deploy (Build-Once Deploy-Many, from the deploy-adapter overlay):**

```bash
bash scripts/deploy/promote.sh --target self-hosted --build-manifest <path>/build-manifest-99c8d629.yaml
# promote resolves digests + the self-hosted-99c8d629 bundle and calls:
#   ./smackerel.sh deploy-target self-hosted apply \
#     --image-core=sha256:<core-digest> --image-ml=sha256:<ml-digest> \
#     --config-bundle=self-hosted-99c8d629 --config-bundle-sha=<sha256-hex>
./smackerel.sh deploy-target self-hosted verify
```

**D. Run the A/B (the decisive test) — two-town pomegranate-growing comparison:**

Question template: `what is a better place to grow pomegranate, <town-A> or <town-B>, <ST>?`

> The operator substitutes the real two-town comparison from the session. The
> literal towns are operator-private location data (flagged by the machine-local
> `pii-tokens.txt`) and are intentionally NOT committed to this generic repo.

Telegram surface:

```
/ask what is a better place to grow pomegranate, <town-A> or <town-B>, <ST>?
/ask --model=deepseek-r1:7b what is a better place to grow pomegranate, <town-A> or <town-B>, <ST>?
```

HTTP surface (`POST /v1/agent/invoke`; baseline omits `model`, override sets it; add the operator runtime auth header if the endpoint is gated):

```bash
# Baseline (configured synthesis model)
curl --max-time 300 -X POST https://<self-hosted-core-fqdn>/v1/agent/invoke \
  -H 'Content-Type: application/json' \
  -d '{"raw_input":"what is a better place to grow pomegranate, <town-A> or <town-B>, <ST>?"}'

# Override (deepseek-r1:7b)
curl --max-time 300 -X POST https://<self-hosted-core-fqdn>/v1/agent/invoke \
  -H 'Content-Type: application/json' \
  -d '{"raw_input":"what is a better place to grow pomegranate, <town-A> or <town-B>, <ST>?","model":"deepseek-r1:7b"}'
```

Capture BOTH full answers with their `— model:` attribution. **Verdict criterion:**
does `deepseek-r1:7b` produce a genuine synthesized COMPARISON VERDICT (reconciling
the two towns' climate / USDA zone vs pomegranate requirements) instead of the
honest-salvage snippet wall? Compare against `gemma4:26b` on the same question,
then run `tests/e2e/openknowledge` against the live stack.

**Honest terminal status:** `status` held at `blocked` (NOT `done`) — the live
A/B + `verify` did not run, so the chain is not certifiable from this session.
Once the operator captures a genuine A/B verdict and `verify` passes, the
`064→084→087→088` chain can be promoted to `done`. `nextRequiredOwner:
operator/user-session`.

---

## Stability Diagnostic (stabilize-to-doc, 2026-06-17)

> Round owner: `bubbles.workflow` parent-expanded child mode `stabilize-to-doc`
> (`executionModel: parent-expanded-child-mode`; this runtime lacks nested
> `runSubagent`, and `stabilize-to-doc` is NOT top-level-runtime-locked).
> Dispatched as ONE mapped child round of the top-level stochastic-quality-sweep.
> `statusCeiling: docs_updated` — spec 088 status is UNCHANGED (`blocked`,
> blocked-on-operator); this round neither promotes nor certifies. No commit /
> push / checkout / restore / stash. Surface touched: spec 088 owned artifacts
> only (this report). Sibling spec 089 untouched (its uncommitted deploy/state
> WIP was not modified or reverted).

### Probe scope

Stability probe of the runtime-switchable-models surface for performance,
infrastructure, configuration, deployment, reliability, and resource-usage
issues — specifically the model-memory-envelope validation, switchable-model
allowlist enforcement, fail-loud config, OOM-safety of the memory profiles, and
the per-request hot-path reliability.

### Evidence — spec 088 test surface GREEN

Command (dockerized `golang:1.25.x` repo CLI; `-run Spec088` scoped to avoid the
spec-094 weather-key fixture RED in the sibling `internal/config` full `Validate`
suite — see foreign attribution below):

```
./smackerel.sh test unit --go --go-run 'Spec088' --verbose
```

Result: `WRAPPER_EXIT=0`. **0 FAIL lines**, **30 top-level `--- PASS`**, **48
`Spec088` PASS lines** (incl. subtests) across all 8 spec-088 packages. The
`internal/config` package executed ONLY the two `*_Spec088` tests
(`...SwitchableModelsRequiredWhenEnabled_Spec088`,
`...ValidateModelEnvelopes_SwitchableOverEnvelopeRejected_Spec088`) and passed —
the spec-094-reddened full `Validate` tests were correctly excluded by the
selector and did NOT run:

```
ok  github.com/smackerel/smackerel/internal/assistant/openknowledge/modelswitch  0.034s
ok  github.com/smackerel/smackerel/internal/assistant/openknowledge/agent        0.135s
ok  github.com/smackerel/smackerel/internal/assistant/openknowledge/agenttool    0.052s
ok  github.com/smackerel/smackerel/internal/assistant                            0.432s
ok  github.com/smackerel/smackerel/internal/assistant/contracts                  0.069s
ok  github.com/smackerel/smackerel/internal/api                                  0.313s
ok  github.com/smackerel/smackerel/internal/config                               0.012s
ok  github.com/smackerel/smackerel/internal/telegram/assistant_adapter           (PASS)
```

### Stability verdict per dimension — STABLE (defense-in-depth confirmed)

1. **OOM-safety of the memory profiles** — the switchable co-residence rule
   (gather `llm_model_id` resident + candidate synthesis model ≤ ollama
   envelope) is enforced at THREE layers with identical arithmetic:
   config-generation (`config.go::validateModelEnvelopes` switchable pass,
   gated on `open_knowledge.enabled && OllamaMemoryLimitMiB != 0`), runtime
   construction (`modelswitch.NewAllowlist` at
   `cmd/core/wiring_assistant_openknowledge.go`, built from the SAME SST —
   verified: same switchable set, `MLModelMemoryProfiles`, `OllamaMemoryLimitMiB`,
   gather `LLMModelID`, default `SynthesisModelID`), and request-time
   (`Allowlist.reject` → `ReasonOverMemEnvelope`). An envelope-busting switchable
   list is refused fail-loud at generation AND at wiring AND at request time.
   The co-residence `baseMiB := profiles[gather]` is non-zero by construction:
   the baseline gather `llm_model_id` is transitively required to be profiled via
   spec 089's `tool_capable_gather_models` membership + per-entry profile contract
   (sibling surface — not modified). Proven by
   `TestValidateModelEnvelopes_SwitchableOverEnvelopeRejected_Spec088` (over-envelope
   / unprofiled / envelope-consistent / dev-skip subtests) and
   `TestAllowlist_Resolve_ProfiledOverEnvelopeRejected_ModelOverMemEnvelope_Spec088`.

2. **Switchable-model allowlist enforcement** — `Resolve` converts an untrusted
   model string into a validated `Override` or a typed fail-loud `Rejection`;
   never a silent default, never a backend passthrough; empty string is the
   byte-for-byte baseline. Proven by `...OffListRejected...`,
   `...UnprofiledRejected...`, `...SingleRejectionContract...`,
   `...BaselineEmptyReturnsZeroOverride...`.

3. **Fail-loud config (G028)** — `NewAllowlist` rejects empty/unprofiled/
   over-envelope/empty-default/blank-entry sets; `OpenKnowledgeConfig` requires a
   non-empty `switchable_models` when enabled. Proven by
   `TestAllowlist_NewAllowlist_FailLoudBuild_Spec088` (5 subtests) and
   `TestOpenKnowledgeConfig_SwitchableModelsRequiredWhenEnabled_Spec088` (4 subtests).

4. **Hot-path reliability / concurrency (C6 singleton-safety)** — `WithModelOverride`
   returns a per-request shallow clone with shared concurrency-safe deps; the SST
   singleton is never mutated; a zero override returns the receiver unchanged
   (no allocation, byte-for-byte baseline). No per-request heavy-resource
   allocation, no leak. Proven by `...ClonesSingletonNeverMutated_Spec088` and
   `...NoOverride_ByteForByteBaseline_Spec088`.

5. **Attribution honesty / trust invariants under override** — `TurnResult.Model`
   is stamped exactly once in `finalize` across every terminal path (success,
   honest-salvage, refuse, early-StopEndTurn); fabricated citations still refuse
   and `<think>` never leaks/cited under an override. Proven by
   `...TurnResultModelStamped_AllTerminalPaths_Spec088` (4 subtests) and
   `...TrustContractsHoldUnderOverride_Spec088`.

**No genuine stability finding on spec 088's owned surface.** `findingsTotal: 0`.
The `blocked` status is blocked-on-operator (CI build-manifest + live self-hosted
`gemma4:26b`-vs-`deepseek-r1:7b` A/B), NOT a stability defect — that operational
A/B is the separately-owned `operator/user-session` runbook above, not an
in-repo stabilize finding.

### Foreign attribution (not spec 088 — not fixed this round)

- **`internal/config/validate_test.go` sibling RED** — the spec-094 sweep's
  `setRequiredEnv` helper is missing four `ASSISTANT_SKILLS_WEATHER_*` keys, so
  the full `Validate` suite in `internal/config` is RED in the working tree. This
  is spec-094 fixture WIP, NOT a spec 088 regression; the `-run Spec088` selector
  scoped around it (config package ran only the two Spec088 tests, both GREEN).
  Owner: the spec-094 sweep round. Not modified here.
- **Uncommitted sibling sweep work** across many other specs (incl. spec 089's
  deploy/state in-flight WIP) was left untouched.

---

## SUPERSESSION NOTE — self-hosted model optimization (2026-06-20)

Record-only; this spec's status and history are unchanged. The self-hosted
switchable synthesis set this spec shipped
(`environments.self-hosted.assistant_open_knowledge_switchable_models: [gemma4:26b, deepseek-r1:7b]`)
has been superseded by the operator's optimized self-hosted model set: the
switchable set is now **`[gpt-oss:20b, gemma4:26b]`** — the only two models the
operator's self-hosted Ollama host pulls. `gpt-oss:20b` is the standing synthesis
default and `gemma4:26b` is the gather model; the deepseek switchable arms are
retired from the self-hosted active selection. The spec-088 runtime-switch
machinery, the `switchable_models` co-residence envelope guard
(`validateModelEnvelopes`), and the trust invariants are unchanged — only the
offered model set changed. See `docs/Operations.md` → "Model Envelope Sizing".
