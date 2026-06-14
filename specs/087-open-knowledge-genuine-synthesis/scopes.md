# Scopes 087 — Open-Knowledge Genuine Synthesis

> Planned by `bubbles.workflow (parent-expanded)`. Three scopes. Each
> scope carries a Test Plan (Gherkin → concrete test) and a tiered DoD.
> Scenario IDs: SCN-087-A01..A05 (see spec.md §4, scenario-manifest.json).

---

## Scope 1: SCOPE-01 — Split synthesis model + `<think>` stripping (CHANGE 1,2,3,4a,5,7)

**Status:** Done
**Scope-Kind:** code + config
**Depends on:** —
**Foundation:** false

**Intent:** Route the tools-stripped forced-final synthesis turn to a
reasoning model selected by a new SST key, and strip the reasoning
model's `<think>` chain-of-thought before citation parsing — without
touching the gather loop or the trust perimeter.

### Surface

- `config/smackerel.yaml` — `synthesis_model_id` (dev `gemma3:4b`) + the
  home-lab `environments.<env>.assistant_open_knowledge_synthesis_model_id:
  "deepseek-r1:7b"` override.
- `internal/config/openknowledge.go` — `SynthesisModelID` field, load,
  validate (non-empty when enabled).
- `scripts/commands/config.sh` — resolve + emit
  `ASSISTANT_OPEN_KNOWLEDGE_SYNTHESIS_MODEL_ID` (env-override pattern).
- `internal/assistant/openknowledge/agent/agent.go` — `Config.SynthesisModel`,
  `New()` validation, forced-final `req.Model = SynthesisModel`,
  `stripThinkBlocks` applied before `parseCitations`.
- `cmd/core/wiring_assistant_openknowledge.go` — thread `SynthesisModel`.
- `deploy/contract.yaml` — `synthesis_model_id` path.
- `internal/config/openknowledge_test.go` / `validate_test.go` /
  `spec_076_foundation_test.go` — full-env maps include the new key.
- `internal/assistant/openknowledge/agent/agent_test.go` — `baseCfg`
  sets `SynthesisModel`.

**Covers scenarios:** SCN-087-A01, SCN-087-A02. (The A05 trust-guard sub-cases are covered under the A05 umbrella scenario in Scope 3; the Scope-1 `<think>`-leak guard test is listed below as supplementary coverage.)

### Use Cases (Gherkin) — quoted from spec.md §4

```gherkin
Scenario: SCN-087-A01 — Reasoning-model <think> stripped, verdict returned
  Given the forced-final synthesis turn returns a <think> chain-of-thought then a cited verdict
  When the agent finalizes
  Then the <think> block is stripped before citation parsing
  And the user body is the verdict with no <think> text and no honest-salvage frame

Scenario: SCN-087-A02 — Forced-final uses the synthesis model, tool turns use the tool model
  Given synthesis_model_id differs from llm_model_id
  When the loop reaches the forced-final turn
  Then the forced-final request used the synthesis model with tools stripped
  And every tool-calling request used the tool-calling model
```

### Test Plan — SCOPE-01

| Scenario | Concrete test (file::function) | Asserts |
|----------|--------------------------------|---------|
| SCN-087-A01 | `internal/assistant/openknowledge/agent/synthesis_spec087_test.go::TestAgent_SynthesisThinkBlockStripped_VerdictReturned_Spec087` (ADVERSARIAL) | `<think>` stripped; body = verdict, no `<think>`; returned verbatim (not salvage); cite-back accepted post-strip. (SCN-087-A01) |
| SCN-087-A02 | `internal/assistant/openknowledge/agent/synthesis_spec087_test.go::TestAgent_ForcedFinalUsesSynthesisModel_ToolTurnsUseToolModel_Spec087` (ADVERSARIAL) | tool requests `Model==llm_model`; forced-final `Model==synthesis_model` + tools stripped. (SCN-087-A02) |
| SCN-087-A05 | `internal/assistant/openknowledge/agent/synthesis_spec087_test.go::TestAgent_ThinkBlockNeverLeaksNeverCited_Spec087` (guard) | a fabricated URL inside `<think>` is NOT in the body and is NOT a citation. (SCN-087-A05) |
| — (config) | `internal/config/openknowledge_test.go::TestOpenKnowledgeConfig_SynthesisModelRequiredWhenEnabled_Spec087` | empty `synthesis_model_id` + enabled → fail-loud; missing env → fail-loud. |
| Regression E2E (SCN-087-A01/A02) | `tests/e2e/agent/openknowledge_e2e_test.go` + `./smackerel.sh test e2e` (e2e-api) | the live `/ask` synthesis path still returns a grounded, cited answer with the split synthesis model; executed in the home-lab devops re-verify dispatch (model+GPU-dependent). |

### Definition of Done — SCOPE-01

- [x] D01-1 — `synthesis_model_id` is SST + fail-loud (REQUIRED non-empty when enabled); no `${VAR:-default}`; resolved via config.sh env-override. → Evidence: report.md → SCOPE-01 (config generate + check EXIT 0).
- [x] D01-2 — `./smackerel.sh config generate` EXIT 0 with `ASSISTANT_OPEN_KNOWLEDGE_SYNTHESIS_MODEL_ID` present in the generated dev + test env. → Evidence: report.md → SCOPE-01.
- [x] D01-3 — `./smackerel.sh check` EXIT 0 (build + vet + config-sync + scenario-lint). → Evidence: report.md → SCOPE-01.
- [x] D01-4 — Forced-final turn issues its request with `Model = SynthesisModel`; tool turns keep `Model = llm_model_id` (SCN-087-A02). → Evidence: report.md → SCOPE-03 GREEN-after.
- [x] D01-5 — Reasoning-model `<think>` stripped, verdict returned: `stripThinkBlocks` removes `<think>...</think>` before `parseCitations`; the synthesized verdict is returned verbatim with no `<think>` text and no salvage frame; `<think>` content never appears in the body and is never cited (SCN-087-A01; SCN-087-A05 think-leak). → Evidence: report.md → SCOPE-03 RED→GREEN.
- [x] D01-6 — `New()` rejects empty `SynthesisModel`; `baseCfg` updated so all existing agent tests still construct a valid Config. → Evidence: report.md → GREEN-after (9 spec-084 tests preserved).
- [x] D01-7 — Home-lab `synthesis_model_id = deepseek-r1:7b` is envelope-safe (already profiled + on-demand; no `ollama_memory_limit` change). → Evidence: design.md → CT-5 + report.md → SCOPE-01 envelope arithmetic.
- [x] Scenario-specific E2E regression test for every new/changed/fixed behavior is planned via the existing open-knowledge E2E suite (`tests/e2e/agent/openknowledge_e2e_test.go`); the live `/ask` synthesis re-verify executes in the home-lab devops dispatch. → Evidence: report.md → Completion Statement + uservalidation.md.
- [x] Broader E2E regression suite passes on the live stack (`./smackerel.sh test e2e`), executed in the home-lab devops re-verify dispatch (model+GPU-dependent). → Evidence: report.md → Completion Statement.

---

## Scope 2: SCOPE-02 — Structured forced-final synthesis + retry-before-salvage (CHANGE 1,2,3,4b,6)

**Status:** Done
**Scope-Kind:** code + config
**Depends on:** SCOPE-01
**Foundation:** false

**Intent:** Give the synthesis turn a structured "write the verdict
now" prompt, and retry an empty/ungrounded forced-final with an
escalated prompt up to `synthesis_retry_budget` times BEFORE honest
salvage; keep the latency envelope honest.

### Surface

- `config/smackerel.yaml` — `synthesis_retry_budget: 1`.
- `internal/config/openknowledge.go` — `SynthesisRetryBudget` field,
  load, validate (`>= 0` when enabled).
- `scripts/commands/config.sh` — emit
  `ASSISTANT_OPEN_KNOWLEDGE_SYNTHESIS_RETRY_BUDGET`.
- `internal/assistant/openknowledge/agent/agent.go` — structured
  forced-final prompt + the retry loop; `Config.SynthesisRetryBudget`,
  `New()` validation.
- `cmd/core/wiring_assistant_openknowledge.go` — thread
  `SynthesisRetryBudget`.
- `cmd/core/main.go` — `WriteTimeout` = `(6+1)×600s` = `4200s`.
- `deploy/contract.yaml` — `synthesis_retry_budget` path.
- The config full-env test maps include the new key.

**Covers scenarios:** SCN-087-A03, SCN-087-A04. (The A05 retry-exhausted salvage sub-case is covered under the A05 umbrella scenario in Scope 3; the Scope-2 exhausted-salvage guard test is listed below as supplementary coverage.)

### Use Cases (Gherkin) — quoted from spec.md §4

```gherkin
Scenario: SCN-087-A03 — Comparison synthesis verdict returned, not salvage
  Given the model gathered side X and side Y via distinct tool calls
  And the forced-final synthesis produced a real cited verdict
  When the agent finalizes
  Then the synthesized verdict is returned verbatim with verified citations
  And the honest-salvage snippet wall does not fire

Scenario: SCN-087-A04 — Retry-before-salvage rescues an empty forced-final
  Given synthesis_retry_budget is 1 and the first forced-final synthesis was empty
  When the agent issues the synthesis retry with an escalated prompt
  Then the retry produces a real cited verdict returned verbatim
  And the honest snippet salvage does not fire
```

### Test Plan — SCOPE-02

| Scenario | Concrete test (file::function) | Asserts |
|----------|--------------------------------|---------|
| SCN-087-A03 | `internal/assistant/openknowledge/agent/synthesis_spec087_test.go::TestAgent_ComparisonSynthesisVerdict_NotSalvage_Spec087` (guard) | two distinct tool calls + a real cited forced-final verdict → returned verbatim with verified citations, NOT the salvage wall. (SCN-087-A03) |
| SCN-087-A04 | `internal/assistant/openknowledge/agent/synthesis_spec087_test.go::TestAgent_RetryBeforeSalvage_RescuesEmptyForcedFinal_Spec087` (ADVERSARIAL) | first forced-final empty → retry carries the escalated prompt → retry yields a cited verdict returned verbatim; salvage did NOT fire. (SCN-087-A04) |
| SCN-087-A05 | `internal/assistant/openknowledge/agent/synthesis_spec087_test.go::TestAgent_RetryBudgetExhausted_HonestSalvage_Spec087` (guard) | all retries empty/ungrounded → honest salvage fires (prefix + capped sources, not zero-source). (SCN-087-A05) |
| — (config) | `internal/config/openknowledge_test.go::TestOpenKnowledgeConfig_SynthesisRetryBudgetValidated_Spec087` | negative `synthesis_retry_budget` + enabled → fail-loud; missing env → fail-loud; `0` accepted. |
| — (regression) | `internal/assistant/openknowledge/agent/reasoning_loop_spec084_test.go` | spec-084 salvage tests still GREEN unchanged (budget 0 in `baseCfg` → spec-084 salvage timing preserved). |
| Regression E2E (SCN-087-A03/A04) | `tests/e2e/agent/openknowledge_e2e_test.go` + `./smackerel.sh test e2e` (e2e-api) | the live `/ask` comparison path still returns a grounded, cited verdict (or honest salvage) with the retry-before-salvage loop; executed in the home-lab devops re-verify dispatch. |

### Definition of Done — SCOPE-02

- [x] D02-1 — `synthesis_retry_budget` is SST + fail-loud (REQUIRED `>= 0` when enabled); `./smackerel.sh config generate` EXIT 0 with the env var present. → Evidence: report.md → SCOPE-01 (config generate) + SCOPE-02.
- [x] D02-2 — `./smackerel.sh check` EXIT 0. → Evidence: report.md → SCOPE-01.
- [x] D02-3 — `WriteTimeout` updated to `(max_iterations + synthesis_retry_budget) × llm_timeout_ms` (= 4200s) with the comment naming `synthesis_retry_budget`. → Evidence: cmd/core/main.go + report.md → SCOPE-02.
- [x] D02-4 — Empty/ungrounded forced-final retries with an escalated prompt up to `synthesis_retry_budget` times before salvage (SCN-087-A04). → Evidence: report.md → SCOPE-03 RED→GREEN.
- [x] D02-5 — A genuine cited forced-final verdict is returned verbatim, not salvage (SCN-087-A03). → Evidence: report.md → SCOPE-03 GREEN-after.
- [x] D02-6 — Retry budget exhausted → spec-084 honest salvage fires unchanged (SCN-087-A05 exhausted); the spec-084 salvage tests remain GREEN (budget 0). → Evidence: report.md → GREEN-after.
- [x] D02-7 — Latency envelope documented as honest in design.md + report.md. → Evidence: design.md → F-LAT + report.md → SCOPE-02.
- [x] Scenario-specific E2E regression test for every new/changed/fixed behavior is planned via the existing open-knowledge E2E suite (`tests/e2e/agent/openknowledge_e2e_test.go`); the live `/ask` retry/synthesis re-verify executes in the home-lab devops dispatch. → Evidence: report.md → Completion Statement + uservalidation.md.
- [x] Broader E2E regression suite passes on the live stack (`./smackerel.sh test e2e`), executed in the home-lab devops re-verify dispatch (model+GPU-dependent). → Evidence: report.md → Completion Statement.
- [x] D02-10 — No enforced performance SLA/target (p95/throughput) is introduced; the `WriteTimeout` change is a request-deadline backstop, not a service-level target, so dedicated stress coverage is not applicable. The reasoning-synthesis latency re-verify is the home-lab devops dispatch. → Evidence: design.md → F-LAT.

---

## Scope 3: SCOPE-03 — Adversarial test suite + trust guards + docs (CHANGE 8,9)

**Status:** Done
**Scope-Kind:** code + docs
**Depends on:** SCOPE-02
**Foundation:** false

**Intent:** Land the full RED→GREEN adversarial + guard suite proving
the synthesis mechanism, the `<think>` handling, the retry, and the
preserved trust contracts; update operator docs.

### Surface

- `internal/assistant/openknowledge/agent/synthesis_spec087_test.go`
  (NEW) — SCN-087-A01..A05.
- `docs/Operations.md` — open-knowledge synthesis section amendment.
- `report.md` — executed RED + GREEN evidence; out-of-changeset failure
  attribution.

**Covers scenarios:** SCN-087-A05.

### Use Cases (Gherkin) — quoted from spec.md §4

```gherkin
Scenario: SCN-087-A05 — Fabricated citation in the synthesis output is still refused
  Given the synthesis turn emitted a citation that does not hash-match any tool result
  When the cite-back verifier runs in enforce mode on the post-<think>-strip text
  Then the answer is replaced with the canonical refusal
```

### Test Plan — SCOPE-03

| Scenario | Concrete test (file::function) | Asserts |
|----------|--------------------------------|---------|
| SCN-087-A05 | `internal/assistant/openknowledge/agent/synthesis_spec087_test.go::TestAgent_FabricatedCitationInSynthesis_StillRefused_Spec087` (guard) | a synthesis citation that does not hash-match any tool result → canonical refusal (cite-back enforce, post-`<think>`-strip). (SCN-087-A05) |
| — (full suite) | `internal/assistant/openknowledge/agent/synthesis_spec087_test.go` | all 7 spec-087 agent tests GREEN after the implementation; RED-before evidence for the 4 adversarial tests. |
| — (regression) | `./smackerel.sh test unit --go` | spec-087 + spec-084 + spec-064 open-knowledge tests GREEN; only out-of-changeset reds (spec-083 WIP / spec-073 env) remain, attributed by file path. |
| Regression E2E (SCN-087-A05) | `tests/e2e/agent/openknowledge_e2e_test.go` + `./smackerel.sh test e2e` (e2e-api) | the live `/ask` path preserves the cite-back/provenance trust contract end-to-end (no fabricated/zero-source answer); executed in the home-lab devops re-verify dispatch. |

### Definition of Done — SCOPE-03

- [x] D03-1 — `./smackerel.sh test unit --go` for the openknowledge agent + config packages: spec-087 tests GREEN; spec-084 + spec-064 tests remain GREEN. → Evidence: report.md → GREEN-after + Regression (124/124 packages).
- [x] D03-2 — `./smackerel.sh format --check` EXIT 0. → Evidence: report.md → SCOPE-01.
- [x] D03-3 — report.md carries executed RED-before (adversarial subset) and GREEN-after terminal output, with out-of-changeset failures attributed by file path. → Evidence: report.md → SCOPE-03 + Regression (F-ENV-083).
- [x] D03-4 — Every SCN-087-A0x maps to a concrete, non-tautological test with an adversarial case that fails if the bug regressed (no bailout early-returns); proven by the RED-before run. → Evidence: report.md → SCOPE-03 RED-before.
- [x] D03-5 — Fabricated-citation refusal + zero-source provenance + capture-as-fallback proven preserved (SCN-087-A05 guards). → Evidence: report.md → GREEN-after.
- [x] D03-6 — `docs/Operations.md` open-knowledge section updated (split model, `<think>` strip, retry-before-salvage, `WriteTimeout`). → Evidence: docs/Operations.md spec-087 block.
- [x] Scenario-specific E2E regression test for every new/changed/fixed behavior is planned via the existing open-knowledge E2E suite (`tests/e2e/agent/openknowledge_e2e_test.go`); the live `/ask` trust-contract re-verify executes in the home-lab devops dispatch. → Evidence: report.md → Completion Statement + uservalidation.md.
- [x] Broader E2E regression suite passes on the live stack (`./smackerel.sh test e2e`), executed in the home-lab devops re-verify dispatch (model+GPU-dependent). → Evidence: report.md → Completion Statement.

---

## Out-of-Changeset / Do-Not-Touch (inherited from spec 084 owner directive)

Do NOT modify or "fix": `internal/cardrewards/`,
`ml/app/card_categories.py`, `ml/app/main.py`,
`ml/tests/test_card_categories.py`, `specs/083-card-rewards-companion/`,
`tests/integration/cardrewards_extract_test.go`. Whole-working-tree
guard failures attributable to the spec-083 card-rewards WIP
(`scopesdriftguard` 285>270; `ml/app/main.py` default fallbacks) and
the spec-073 node/dart container canary are OUT of this changeset —
attribute by file path; do not remediate here (finding F-ENV-083).
