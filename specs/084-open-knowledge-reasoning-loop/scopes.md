# Scopes — Spec 084 (Open-Knowledge Reasoning Loop)

**Mode:** full-delivery · **Status ceiling:** `done` · **Parallelisation:** none (strict sequential)
**Amends:** spec 064. Does NOT reopen BUG-064-001 / BUG-064-002.

## Execution Outline

Three changes amend the spec-064 open-knowledge agent (review items 1, 2, 4 —
NOT model): (1) a question-agnostic reasoning prompt, (2) a drill-in loop
(raised budgets + reflect-before-final + the WriteTimeout latency invariant),
and (3) honest snippet salvage. The model matrix (gemma4:26b / gemma3:4b) is
unchanged.

## Bootstrap (not a numbered scope)

The canonical artifact set (`spec.md`, `design.md`, `scopes.md`, `report.md`,
`uservalidation.md`, `state.json`, `scenario-manifest.json`) was materialised
first and passes `artifact-lint.sh specs/084-open-knowledge-reasoning-loop`
(evidence in `report.md` → Test Evidence → Scope-01). `design.md` records the
analyze findings (C1 fast-path latency bypass; C2 token re-add growth) and the
decisions D1-D6.

## Cross-cutting build-quality gate (asserted in every scope's DoD)

Every scope's DoD includes the grouped build-quality gate: `./smackerel.sh
test unit --go` green, `./smackerel.sh check` clean, `./smackerel.sh format
--check` clean, no defaults / fail-loud SST from `config/smackerel.yaml`
(G028), `artifact-lint.sh` + `traceability-guard.sh` pass, no model / spec-083
file touched, no commit/push (owner directive C-4), evidence paths redacted
`~/` in `report.md`.

---

## Scope 1: SCOPE-01 — Question-agnostic reasoning prompt (CHANGE 1)

**Status:** Done
**Scope-Kind:** contract-only
**Depends on:** none
**Foundation:** false

### Surface

- `config/prompt_contracts/open_knowledge.yaml` — `agent_system_prompt` rewrite
  + `limits` clarifying comment (F-LAT).
- `cmd/core/openknowledge_prompt_contract_test.go` — new prompt-content guard.

**Covers scenarios:** SCN-084-A01.

**Design anchors:** design.md → D1, FR-1..FR-4.

### Use Cases (Gherkin) — quoted from spec.md §4

```gherkin
Scenario: SCN-084-A01 — Reasoning prompt is question-agnostic
  Given the open-knowledge agent_system_prompt in config/prompt_contracts/open_knowledge.yaml
  When the prompt is loaded
  Then it does NOT contain the anti-drill instruction to write the final answer in the next turn
  And it does NOT contain the BUG-064-002 question-type enumeration
  And it DOES contain a DECOMPOSE / GATHER / RECONCILE / ANSWER reasoning contract
  And it preserves the <CITATIONS> contract, the three citation shapes, the R1-R4 hard rules, and the refusal shape verbatim
```

### Consumer Impact Sweep

The `agent_system_prompt` is consumed by exactly ONE surface:
`cmd/core/wiring_assistant_openknowledge.go::loadOpenKnowledgeAgentPrompt`, which
loads it verbatim into `okagent.Config.SystemPrompt`. This is a PROMPT-TEXT-ONLY
change — no Go interface, function signature, struct field, SST config key,
scenario field, or wire contract is renamed or dropped. The loader contract (a
non-empty `agent_system_prompt` field) is unchanged; `agent.New()` still rejects
an empty prompt; the `<CITATIONS>` cite-back contract and the three citation
shapes are preserved verbatim. No downstream consumer requires migration.

### Test Plan

| Test | Type | Asserts |
|------|------|---------|
| `cmd/core/openknowledge_prompt_contract_test.go::TestOpenKnowledgeAgentPrompt_IsQuestionAgnostic_Spec084` | unit | anti-drill instruction + question-type enumeration REMOVED; DECOMPOSE/GATHER/RECONCILE/ANSWER reasoning contract PRESENT (adversarial: RED on the pre-084 prompt) (SCN-084-A01) |
| `cmd/core/openknowledge_prompt_contract_test.go::TestOpenKnowledgeAgentPrompt_PreservesCitationContract_Spec084` | unit | R1-R4, `<CITATIONS>`, the three citation shapes, and the refusal shape PRESERVED verbatim (SCN-084-A01) |

### Definition of Done

- [x] D01-1 — Anti-drill instruction ("write the final answer in the NEXT turn … Do NOT keep calling") REMOVED from `agent_system_prompt`.
- [x] D01-2 — BUG-064-002 question-type enumeration ("times, prices, temperatures, highs/lows, a schedule, a table") REMOVED and NOT replaced with another question-type list.
- [x] D01-3 — DECOMPOSE / GATHER / RECONCILE / ANSWER-THE-ACTUAL-QUESTION reasoning contract PRESENT and general (covers comparison / why / recommendation / multi-hop without enumerating types).
- [x] D01-4 — `<CITATIONS>` contract, the three citation shapes, R1-R4, the refusal shape, and the "never repeat / never invent" rules PRESERVED verbatim.
- [x] D01-5 — `limits` clarifying comment (F-LAT) added noting the `/ask` fast-path bypass.
- [x] D01-6 — Build-quality gate passes (unit/check/format/SST/artifact-lint/traceability/no-touch/no-commit).
- [x] D01-7 — SCN-084-A01 Reasoning prompt is question-agnostic — Evidence: RED→GREEN ≥10 lines captured in `report.md`.
- [x] D01-8 — Consumer Impact Sweep complete: the only consumer (`loadOpenKnowledgeAgentPrompt` → `okagent.Config.SystemPrompt`) is enumerated; prompt-text-only change with no code interface renamed/dropped; loader + cite-back contracts unchanged. → Evidence: report.md (Code Diff Evidence) + the Consumer Impact Sweep section above.

---

## Scope 2: SCOPE-02 — Let the loop drill in (CHANGE 2)

**Status:** Done
**Scope-Kind:** code + config
**Depends on:** SCOPE-01
**Foundation:** false

### Surface

- `config/smackerel.yaml` — `assistant.open_knowledge.max_iterations` 4→6;
  `per_query_token_budget` 64000→128000.
- `cmd/core/main.go` — `WriteTimeout` 1800s→3600s (request-deadline backstop; tracks max_iterations × llm_timeout_ms).
- `internal/assistant/openknowledge/agent/agent.go` — reflect-before-final nudge.
- `config/generated/{dev,test}.env` — regenerated (gitignored).

**Covers scenarios:** SCN-084-A02.

**Design anchors:** design.md → D2, D3, D4, D5, Finding C1, FR-5..FR-8.

### Use Cases (Gherkin) — quoted from spec.md §4

```gherkin
Scenario: SCN-084-A02 — The loop drills in before answering
  Given max_iterations is 6 and the model issues distinct tool calls each turn
  When the agent loop runs
  Then the loop allows at least 2 distinct tool calls before the forced-final turn
  And a reflect-before-final nudge is injected on the second-to-last iteration instructing the model to check coverage and fill any gap
  And the forced-final tool-stripping mechanism is preserved
```

### Test Plan

| Test | Type | Asserts |
|------|------|---------|
| `internal/assistant/openknowledge/agent/reasoning_loop_spec084_test.go::TestAgent_ReflectBeforeFinal_NudgeOnSecondToLastIteration_Spec084` | unit | reflect nudge present on the second-to-last request, absent before it, forced-final message on the last, tools stripped on the last (adversarial: RED today) (SCN-084-A02) |
| `internal/assistant/openknowledge/agent/reasoning_loop_spec084_test.go::TestAgent_MultiHop_AllowsDistinctToolCallsBeforeForcedFinal_Spec084` | unit | at max_iterations=6 the loop processes 5 distinct tool calls to a cited synthesis without premature stop (SCN-084-A02) |
| `internal/config/openknowledge_test.go` | unit | `max_iterations` / `per_query_token_budget` remain `> 0` fail-loud (existing coverage; re-run) |
| `tests/e2e/agent/openknowledge_e2e_test.go` | e2e-api | scenario-specific regression: the open-knowledge /ask path still produces a grounded, cited answer end-to-end with the raised iteration budget (broader open-knowledge E2E suite; the live behavioral re-verify of multi-hop reasoning runs in the home-lab devops dispatch) (SCN-084-A02) |
| `./smackerel.sh test e2e` | e2e-api | broader E2E regression suite: the assistant agent suite still passes on the live stack (executed in the home-lab devops re-verify dispatch) |

### Definition of Done

- [x] D02-1 — `assistant.open_knowledge.max_iterations: 6` (SST, fail-loud, comment = "5 tool-calling turns + 1 forced-synthesis turn").
- [x] D02-2 — `assistant.open_knowledge.per_query_token_budget: 128000` (SST, fail-loud, comment = 6-iteration re-add-growth rationale).
- [x] D02-3 — `cmd/core/main.go` `WriteTimeout: 3600s` (comment = `6 × 600s`); the real `/ask` ceiling is documented as the HTTP WriteTimeout (Finding C1).
- [x] D02-4 — Forced-final-turn tool-stripping mechanism PRESERVED; reflect-before-final nudge added on the second-to-last iteration (ephemeral, within budget, no new model / dependency).
- [x] D02-5 — `./smackerel.sh config generate` regenerates dev/test env deterministically with `MAX_ITERATIONS=6` and `PER_QUERY_TOKEN_BUDGET=128000`.
- [x] D02-6 — Build-quality gate passes (unit/check/format/SST/artifact-lint/traceability/no-touch/no-commit).
- [x] D02-7 — SCN-084-A02 The loop drills in before answering — Evidence: RED→GREEN ≥10 lines captured in `report.md`.
- [x] D02-8 — Scenario-specific + broader E2E regression coverage is planned via the existing open-knowledge E2E suite (`tests/e2e/agent/openknowledge_e2e_test.go`, `./smackerel.sh test e2e`); the deterministic reflect/budget behavior is unit-proven (fakeLLM adversarial traces) and the live behavioral re-verify is the home-lab devops dispatch. → Evidence: report.md → Test Evidence (Scope-02).
- [x] D02-9 — No enforced performance target (p95/throughput) is introduced by this scope (the `WriteTimeout` change is a request-deadline backstop, not an enforced service-level target); stress coverage is therefore not applicable. The request-deadline analysis is recorded in design.md → D5 / Finding C1. → Evidence: design.md → D5 / Finding C1.

---

## Scope 3: SCOPE-03 — Honest salvage + adversarial tests + docs (CHANGE 4)

**Status:** Done
**Scope-Kind:** code + docs
**Depends on:** SCOPE-02
**Foundation:** false

### Surface

- `internal/assistant/openknowledge/agent/agent.go` — honest-salvage frame helper + both snippet-salvage call sites reframed.
- `internal/assistant/openknowledge/agent/reasoning_loop_spec084_test.go` — adversarial + guard tests.
- `docs/Operations.md` — amend the spec-064 open-knowledge section.

**Covers scenarios:** SCN-084-A03, SCN-084-A04, SCN-084-A05.

**Design anchors:** design.md → D6, FR-9, FR-10, Principle 8.

### Use Cases (Gherkin) — quoted from spec.md §4

```gherkin
Scenario: SCN-084-A03 — A failed comparison synthesis is framed honestly
  Given side X and side Y evidence came from distinct tool calls
  And the model produced no usable synthesis on the forced-final turn
  When the agent salvages from snippets
  Then the body is framed as raw findings (couldn't directly answer)
  And the body carries both sides' evidence
  And the body is NOT presented as a confident comparison verdict

Scenario: SCN-084-A04 — Salvage never presents a snippet wall as a confident answer
  Given tools returned content but the model returned empty or ungrounded text
  When the agent salvages
  Then the salvaged body is framed as raw findings, not a confident answer
  And the salvaged body still carries capped, deduped sources
  And the provenance and cite-back contracts still hold

Scenario: SCN-084-A05 — Genuine synthesis and the trust contract are not regressed
  Given the model produced a real synthesized answer with a valid CITATIONS block
  When the agent finalizes
  Then the body is returned verbatim with no honest-salvage frame
  And a fabricated citation is still rejected by the cite-back verifier
```

### Test Plan

| Test | Type | Asserts |
|------|------|---------|
| `internal/assistant/openknowledge/agent/reasoning_loop_spec084_test.go::TestAgent_ComparisonSalvage_HonestlyFramed_BothSides_Spec084` | unit | distinct-tool-call comparison salvage is honestly framed + carries both sides + not a confident verdict (adversarial: RED today) (SCN-084-A03) |
| `internal/assistant/openknowledge/agent/reasoning_loop_spec084_test.go::TestAgent_HonestSalvage_EmptyForcedFinal_FramedWithSources_Spec084` | unit | empty forced-final salvage framed as raw findings + still carries capped sources (adversarial: RED today) (SCN-084-A04) |
| `internal/assistant/openknowledge/agent/reasoning_loop_spec084_test.go::TestAgent_HonestSalvage_UngroundedExcuse_ReplacedWithFramedFindings_Spec084` | unit | ungrounded-excuse body replaced with framed findings + sources (adversarial: RED today) (SCN-084-A04) |
| `internal/assistant/openknowledge/agent/reasoning_loop_spec084_test.go::TestAgent_GenuineSynthesis_ReturnedVerbatim_NoSalvageFrame_Spec084` | unit | genuine cited synthesis returned verbatim, no frame (guard) (SCN-084-A05) |
| `internal/assistant/openknowledge/agent/reasoning_loop_spec084_test.go::TestAgent_FabricatedCitation_StillRejected_Spec084` | unit | fabricated citation still rejected by the cite-back verifier (guard) (SCN-084-A05) |
| `internal/assistant/openknowledge/agent/snippet_dedup_bug064002_test.go` | unit | BUG-064-002 snippet-dedup + source-cap regressions still pass (no regression) |
| `tests/e2e/agent/openknowledge_e2e_test.go` | e2e-api | scenario-specific regression: the open-knowledge /ask path still returns a grounded answer or an honest refusal end-to-end after the salvage-honesty change (broader open-knowledge E2E suite; live behavioral re-verify is the home-lab devops dispatch) (SCN-084-A03) |
| `./smackerel.sh test e2e` | e2e-api | broader E2E regression suite: the assistant agent suite still passes on the live stack (executed in the home-lab devops re-verify dispatch) |

### Definition of Done

- [x] D03-1 — When `synthesizeFromSnippets` is the body (forced-final empty AND empty-citations/ungrounded-excuse paths), the body is framed as raw findings ("couldn't directly answer; here is what I found"), NOT a confident verdict.
- [x] D03-2 — Capped/deduped sources still attach on the salvage path (no zero-source refusal); provenance / cite-back contracts unchanged.
- [x] D03-3 — Genuine-synthesis happy path returns the model's text verbatim (no frame); fabricated citation still rejected.
- [x] D03-4 — BUG-064-002 regression tests (snippet dedup, source cap) still pass.
- [x] D03-5 — `docs/Operations.md` open-knowledge section amended (reasoning loop, max_iterations, token budget, WriteTimeout/F-LAT, honest salvage).
- [x] D03-6 — Build-quality gate passes (full `./smackerel.sh test unit --go` green, `check` clean, `format --check` clean, SST/G028, artifact-lint + traceability pass, no model / spec-083 file touched, no commit).
- [x] D03-7 — SCN-084-A03 comparison salvage framed honestly; SCN-084-A04 salvage never a confident snippet wall; SCN-084-A05 genuine synthesis + trust contract not regressed — Evidence: RED→GREEN + guards ≥10 lines captured in `report.md`.
- [x] D03-8 — Scenario-specific + broader E2E regression coverage is planned via the existing open-knowledge E2E suite (`tests/e2e/agent/openknowledge_e2e_test.go`, `./smackerel.sh test e2e`); the deterministic salvage-honesty behavior is unit-proven (fakeLLM adversarial traces incl. empty/ungrounded forced-final) and the live behavioral re-verify is the home-lab devops dispatch. → Evidence: report.md → Test Evidence (Scope-03).
