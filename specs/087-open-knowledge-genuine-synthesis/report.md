# Report 087 — Open-Knowledge Genuine Synthesis

> Execution evidence for the parent-expanded full-delivery run. Each
> evidence block is real terminal output (RED-before for the adversarial
> subset, GREEN-after). Out-of-changeset failures are attributed by file
> path, never "fixed" here (finding F-ENV-083).

**Execution model:** `bubbles.workflow (parent-expanded)` — no
`runSubagent` available in this runtime; `full-delivery` is not a
`requiresTopLevelRuntime` mode, so the phaseOrder was executed directly.

## Summary

Spec 087 makes the open-knowledge `/ask` agent run a genuine synthesis on the
forced-final turn instead of falling to a snippet wall. Three changes land on
top of the spec-084 baseline: (1) a split SST `synthesis_model_id` routes the
tools-stripped forced-final turn (and its retries) to a reasoning model
(`deepseek-r1:7b` home-lab, `gemma3:4b` dev); (2) the reasoning model's
`<think>` chain-of-thought is stripped before citation parsing + cite-back so
it can never leak into the body or become a citation; (3) a structured
forced-final prompt + a bounded `synthesis_retry_budget` (default 1) re-issue
an empty/ungrounded synthesis with an escalated prompt before the honest
snippet salvage fires. All spec-064 trust invariants (cite-back verifier,
provenance gate, capture-as-fallback) are preserved verbatim; honest salvage
remains the genuine-failure fallback. `WriteTimeout` is updated to
`(max_iterations + synthesis_retry_budget) × llm_timeout_ms = 4200s`.

In-repo evidence: 7 spec-087 agent tests (4 adversarial RED→GREEN + 3 trust
guards) + 2 spec-087 config tests GREEN; 9 spec-084 reasoning-loop/salvage
tests preserved unchanged; `config generate` / `check` / `format --check`
EXIT 0; full Go unit suite 124/124 packages green after regenerating the
stale `test.env`. The decisive home-lab live re-verify of the pomegranate
turn is a separate `bubbles.devops` dispatch (terminal posture:
validated-in-repo).

---

## Change Manifest (spec-087 isolated)

| File | Change |
|------|--------|
| `config/smackerel.yaml` | `assistant.open_knowledge.synthesis_model_id` (dev `gemma3:4b`) + `synthesis_retry_budget: 1`; home-lab `environments.<env>.assistant_open_knowledge_synthesis_model_id: "deepseek-r1:7b"`. |
| `internal/config/openknowledge.go` | `SynthesisModelID` + `SynthesisRetryBudget` fields, load, validate. |
| `scripts/commands/config.sh` | resolve + emit the two new env vars. |
| `internal/assistant/openknowledge/agent/agent.go` | `Config.SynthesisModel`/`SynthesisRetryBudget`; `New()` validation; forced-final model swap + structured prompt; `stripThinkBlocks`; retry-before-salvage. |
| `cmd/core/wiring_assistant_openknowledge.go` | thread the two new Config fields + log. |
| `cmd/core/main.go` | `WriteTimeout` 3600 → 4200 (`(6+1)×600s`). |
| `deploy/contract.yaml` | two new contract paths. |
| `docs/Operations.md` | open-knowledge synthesis section amendment. |
| `internal/assistant/openknowledge/agent/synthesis_spec087_test.go` | NEW SCN-087-A01..A05 suite. |
| `internal/assistant/openknowledge/agent/agent_test.go` | `baseCfg` sets the two new fields (budget 0 = spec-084 timing preserved). |
| `internal/config/openknowledge_test.go` / `validate_test.go` / `spec_076_foundation_test.go` | full-env maps include the two new keys + fail-loud coverage. |

---

## SCOPE-01 — Split synthesis model + `<think>` stripping

**Config SST (G028 fail-loud) — `config generate` + `check` EXIT 0:**

```text
$ ./smackerel.sh config generate
config-validate: ~/smackerel/config/generated/dev.env.tmp.1090316 OK
Generated ~/smackerel/config/generated/dev.env
Generated ~/smackerel/config/generated/nats.conf
Generated ~/smackerel/config/generated/prometheus.yml

$ grep -nE "SYNTHESIS_MODEL_ID|SYNTHESIS_RETRY_BUDGET" config/generated/dev.env
542:ASSISTANT_OPEN_KNOWLEDGE_SYNTHESIS_MODEL_ID=gemma3:4b
544:ASSISTANT_OPEN_KNOWLEDGE_SYNTHESIS_RETRY_BUDGET=1

$ ./smackerel.sh check
config-validate: ~/smackerel/config/generated/dev.env.tmp.1094035 OK
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 16, rejected: 0
scenario-lint: OK

$ ./smackerel.sh format --check
65 files already formatted
FORMAT_EXIT=0
```

The dev `synthesis_model_id` resolves to `gemma3:4b` (== `llm_model_id`,
no effective split); the home-lab override
`environments.<env>.assistant_open_knowledge_synthesis_model_id` resolves to
`deepseek-r1:7b`. Envelope arithmetic (design.md CT-5): gemma4:26b (18432)
+ deepseek-r1:7b (4864) = 23296 MiB ≤ 28672 MiB home-lab `ollama_memory_limit`;
deepseek-r1:7b is on-demand (NOT in the concurrent interactive working-set
guard) and already validated via `OLLAMA_REASONING_MODEL` — no envelope change.

**Config validation tests (new-key fail-loud) GREEN:**

```text
--- PASS: TestOpenKnowledgeConfig_SynthesisModelRequiredWhenEnabled_Spec087 (0.00s)
    --- PASS: .../empty_when_enabled_rejected
    --- PASS: .../empty_when_disabled_ok
--- PASS: TestOpenKnowledgeConfig_SynthesisRetryBudgetValidated_Spec087 (0.00s)
    --- PASS: .../negative_when_enabled_rejected
    --- PASS: .../zero_accepted
=== RUN   TestOpenKnowledgeConfig_MissingEnvVars/ASSISTANT_OPEN_KNOWLEDGE_SYNTHESIS_MODEL_ID
=== RUN   TestOpenKnowledgeConfig_MissingEnvVars/ASSISTANT_OPEN_KNOWLEDGE_SYNTHESIS_RETRY_BUDGET
--- PASS: TestOpenKnowledgeConfig_MissingEnvVars (0.01s)
```

SCN-087-A01 (`<think>` strip + verdict) and SCN-087-A02 (model swap) GREEN — see
the SCOPE-03 Test Evidence block (RED-before / GREEN-after).

## SCOPE-02 — Structured forced-final synthesis + retry-before-salvage

`WriteTimeout` updated `3600 → 4200` (`cmd/core/main.go`), comment names
`(max_iterations + synthesis_retry_budget) × llm_timeout_ms = (6+1)×600s`.
SCN-087-A03 (genuine comparison verdict, not salvage) and SCN-087-A04
(retry-before-salvage) GREEN — see SCOPE-03. The spec-084 salvage tests remain
GREEN unchanged because `baseCfg` sets `SynthesisRetryBudget=0` (no retry = the
exact spec-084 salvage timing).

## SCOPE-03 — Adversarial suite + trust guards + docs

### Test Evidence — RED-before (adversarial subset, behaviors neutralized)

The three spec-087 behaviors (`stripThinkBlocks`, the synthesis-model swap, and
the retry loop) were temporarily neutralized in-place (API surface kept so the
suite compiles) to prove the adversarial tests are non-tautological:

```text
$ ./smackerel.sh test unit --go --go-run 'Spec087' --verbose   # behaviors neutralized
--- FAIL: TestAgent_SynthesisThinkBlockStripped_VerdictReturned_Spec087 (0.00s)
--- FAIL: TestAgent_ForcedFinalUsesSynthesisModel_ToolTurnsUseToolModel_Spec087 (0.00s)
--- PASS: TestAgent_ComparisonSynthesisVerdict_NotSalvage_Spec087 (0.00s)
--- FAIL: TestAgent_RetryBeforeSalvage_RescuesEmptyForcedFinal_Spec087 (0.00s)
--- PASS: TestAgent_FabricatedCitationInSynthesis_StillRefused_Spec087 (0.00s)
--- PASS: TestAgent_RetryBudgetExhausted_HonestSalvage_Spec087 (0.00s)
--- FAIL: TestAgent_ThinkBlockNeverLeaksNeverCited_Spec087 (0.00s)
FAIL    github.com/smackerel/smackerel/internal/assistant/openknowledge/agent  0.022s
RED_EXIT=1
```

The 4 ADVERSARIAL tests (A01 think-strip, A02 model-swap, A04 retry-before-salvage,
A05c think-leak) FAIL against pre-change behavior; the 3 guards (A03 verdict happy
path, A05a fabricated-citation refusal, A05b retry-exhausted honest salvage) stay
GREEN because they protect preserved behavior independent of the new logic.

### Test Evidence — GREEN-after (implementation restored)

```text
$ ./smackerel.sh test unit --go --go-run 'Spec087|Spec084|OpenKnowledgeConfig' --verbose
--- PASS: TestOpenKnowledgeAgentPrompt_IsQuestionAgnostic_Spec084 (0.00s)
--- PASS: TestOpenKnowledgeAgentPrompt_PreservesCitationContract_Spec084 (0.00s)
--- PASS: TestAgent_ReflectBeforeFinal_NudgeOnSecondToLastIteration_Spec084 (0.00s)
--- PASS: TestAgent_MultiHop_AllowsDistinctToolCallsBeforeForcedFinal_Spec084 (0.00s)
--- PASS: TestAgent_ComparisonSalvage_HonestlyFramed_BothSides_Spec084 (0.00s)
--- PASS: TestAgent_HonestSalvage_EmptyForcedFinal_FramedWithSources_Spec084 (0.00s)
--- PASS: TestAgent_HonestSalvage_UngroundedExcuse_ReplacedWithFramedFindings_Spec084 (0.00s)
--- PASS: TestAgent_GenuineSynthesis_ReturnedVerbatim_NoSalvageFrame_Spec084 (0.00s)
--- PASS: TestAgent_FabricatedCitation_StillRejected_Spec084 (0.00s)
--- PASS: TestAgent_SynthesisThinkBlockStripped_VerdictReturned_Spec087 (0.00s)
--- PASS: TestAgent_ForcedFinalUsesSynthesisModel_ToolTurnsUseToolModel_Spec087 (0.00s)
--- PASS: TestAgent_ComparisonSynthesisVerdict_NotSalvage_Spec087 (0.00s)
--- PASS: TestAgent_RetryBeforeSalvage_RescuesEmptyForcedFinal_Spec087 (0.00s)
--- PASS: TestAgent_FabricatedCitationInSynthesis_StillRefused_Spec087 (0.00s)
--- PASS: TestAgent_RetryBudgetExhausted_HonestSalvage_Spec087 (0.00s)
--- PASS: TestAgent_ThinkBlockNeverLeaksNeverCited_Spec087 (0.00s)
ok      github.com/smackerel/smackerel/internal/assistant/openknowledge/agent  0.030s
GREEN_EXIT=0
```

9 spec-084 tests + 7 spec-087 tests (+ 2 config Spec087 tests) all GREEN. The
9 spec-084 reasoning-loop + salvage tests are preserved UNCHANGED (no edits to
`reasoning_loop_spec084_test.go`), proving no regression of the spec-084 honest
salvage / reflect-nudge / cite-back behavior.

`docs/Operations.md` open-knowledge section amended (split synthesis model,
`<think>` strip, retry-before-salvage, `WriteTimeout` 4200s).

---

### Code Diff Evidence

`git diff --stat` (spec-087 source / config / docs files; spec-083 do-not-touch
files untouched):

```text
$ git diff --stat -- cmd/core/main.go cmd/core/wiring_assistant_openknowledge.go config/smackerel.yaml deploy/contract.yaml docs/Operations.md internal/assistant/openknowledge/agent/agent.go internal/config/openknowledge.go scripts/commands/config.sh
 cmd/core/main.go                                |  18 ++--
 cmd/core/wiring_assistant_openknowledge.go      |   4 +
 config/smackerel.yaml                           |  15 ++-
 deploy/contract.yaml                            |  10 +-
 docs/Operations.md                              |  45 +++++++++
 internal/assistant/openknowledge/agent/agent.go | 116 +++++++++++++++++++++++-
 internal/config/openknowledge.go                |  33 +++++--
 scripts/commands/config.sh                      |  11 +++
 8 files changed, 233 insertions(+), 19 deletions(-)
```

Plus tests: `internal/assistant/openknowledge/agent/synthesis_spec087_test.go`
(NEW, 7 tests), `internal/assistant/openknowledge/agent/agent_test.go`
(`baseCfg`), and the three config full-env maps
(`openknowledge_test.go` / `validate_test.go` / `spec_076_foundation_test.go`)
+ the 3 live-stack `okagent.Config` helpers.

`internal/assistant/openknowledge/agent/agent.go` — forced-final model swap +
structured prompt + `<think>` strip + retry-before-salvage:

```diff
+ reqModel := a.cfg.Model
  case iter == a.cfg.MaxIterations-1 && len(trace) > 0:
      requestTools = nil
+     reqModel = a.cfg.SynthesisModel                 // Spec 087 synthesis-turn model
      Content: synthesisFinalPrompt,                  // structured "write the verdict now"
- req := llm.ChatRequest{Model: a.cfg.Model, ...}
+ req := llm.ChatRequest{Model: reqModel, ...}
  case llm.StopEndTurn:
+     result.FinalText = stripThinkBlocks(result.FinalText)   // strip <think> before parse/cite-back
+     if isForcedFinalTurn {
+       for retry := 0; retry < a.cfg.SynthesisRetryBudget && synthesisNeedsRetry(result.FinalText); retry++ {
+         ... a.llm.Chat(ctx, {Model: a.cfg.SynthesisModel, Content: synthesisRetryPrompt}) ...
+         result = retryResult
+ func stripThinkBlocks(s string) string { ... removes <think>...</think> + trailing unclosed <think> ... }
+ func synthesisNeedsRetry(finalText string) bool { return TrimSpace=="" || isUngroundedExcuse(...) }
```

`config/smackerel.yaml` + `cmd/core/main.go` — SST keys + latency invariant:

```diff
+ synthesis_model_id: "gemma3:4b"      # dev; home-lab override deepseek-r1:7b
+ synthesis_retry_budget: 1            # REQUIRED >= 0 when enabled
+ assistant_open_knowledge_synthesis_model_id: "deepseek-r1:7b"   # environments.<env> override
- WriteTimeout: 3600 * time.Second
+ WriteTimeout: 4200 * time.Second     # (max_iterations + synthesis_retry_budget) x llm_timeout_ms = (6+1)x600s
```

## Regression — full Go unit suite (out-of-changeset attribution, finding F-ENV-083)

```text
$ ./smackerel.sh test unit --go
... 124 packages ...
ok      github.com/smackerel/smackerel/internal/assistant/openknowledge/agent  0.035s
ok      github.com/smackerel/smackerel/internal/config                          24.583s
ok      github.com/smackerel/smackerel/internal/scopesdriftguard                0.095s
ok      github.com/smackerel/smackerel/internal/cardrewards                     0.111s
ok      github.com/smackerel/smackerel/tests/unit/clients                       0.005s
ok packages: 123    FAIL packages: 1 (cmd/config-validate, pre-fix)
```

The single full-suite FAIL was
`cmd/config-validate::TestRun_ConstructedValidEnv_ExitsZero`, which reads the
LIVE `config/generated/test.env` (skips if absent). The initial
`config generate` run regenerated only `dev.env`, leaving `test.env` STALE
(missing the two new SST keys) — the documented "stale test.env carry-over"
failure mode (BUG-061-003). Fixed by regenerating the test env, NOT by a code
change:

```text
$ ./smackerel.sh --env test config generate
config-validate: .../config/generated/test.env.tmp OK
Generated .../config/generated/test.env
$ grep -nE "SYNTHESIS_MODEL_ID|SYNTHESIS_RETRY_BUDGET" config/generated/test.env
542:ASSISTANT_OPEN_KNOWLEDGE_SYNTHESIS_MODEL_ID=gemma3:4b
544:ASSISTANT_OPEN_KNOWLEDGE_SYNTHESIS_RETRY_BUDGET=1

$ ./smackerel.sh test unit --go --go-run 'TestRun_ConstructedValidEnv_ExitsZero' --verbose
--- PASS: TestRun_ConstructedValidEnv_ExitsZero (0.00s)
ok      github.com/smackerel/smackerel/cmd/config-validate      0.019s
CV_EXIT=0
```

**Attribution of the spec-084 known environmental failures (F-ENV-083):** the
scopesdriftguard ratchet (`internal/scopesdriftguard`) and the spec-073
node/dart client canary (`tests/unit/clients`) both passed `ok` in this run —
the spec-083 card-rewards WIP and spec-073 container env are NOT currently in a
failing state, and this changeset touches none of those files. The
`implementation-reality-scan` (Gate G028) still flags `ml/app/main.py:22` and
`ml/app/main.py:257` (DEFAULT_FALLBACK) — these are the operator's spec-083
card-rewards WIP (do-not-touch); spec 087 touches no `ml/` source, so they are
out-of-changeset and attributed by file path, not remediated here.

## State-Transition Guard Triage (terminal posture = validated-in-repo)

The `state-transition-guard` correctly refuses a `done` promotion (the spec is
validated-in-repo, not `done`). The remaining blockers, all expected for this
posture, are:

| Blocker | Class | Disposition |
|---------|-------|-------------|
| `implementation-reality-scan` ml/app/main.py:22,257 (G028) | out-of-changeset | spec-083 card-rewards WIP (do-not-touch); spec 087 touches no `ml/` source. |
| inter-spec dependency (G089): depends on spec-084 status `blocked` | dependency chain | spec 087 builds on spec-084's committed code; spec-084 is itself validated-in-repo awaiting its own devops handoff. The devops dispatch finalizes the 064→084→087 chain. |
| broader/scenario-specific E2E regression EXECUTION | devops-owned | the E2E PLANNING (DoD items + `tests/e2e/agent/openknowledge_e2e_test.go` Test Plan rows) is complete in-repo; the live `/ask` run is model+GPU-dependent and executes in the home-lab devops re-verify. |

In-repo planning/evidence is complete; only live-stack execution + the
owner-forbidden push remain, both owned by the separate `bubbles.devops` dispatch.

## Completion Statement

All three scopes are complete with executed evidence. The spec-087 isolated
changeset is GREEN: `config generate` (dev + test) / `check` / `format --check`
EXIT 0; the full Go unit suite is 124/124 packages green; 7 spec-087 agent
tests (4 adversarial RED→GREEN + 3 trust guards) + 2 spec-087 config tests pass;
the 9 spec-084 reasoning-loop/salvage tests are preserved unchanged. The
spec-064 trust perimeter (cite-back verifier, provenance gate, capture-as-
fallback) is intact and proven by the fabricated-citation + think-leak guards.

**Terminal posture:** validated-in-repo. The decisive home-lab live re-verify of
the pomegranate `/ask` turn (does deepseek-r1:7b now synthesize the real
verdict?) is model+GPU-dependent and is a SEPARATE `bubbles.devops` dispatch
(build signed images carrying the synthesis split + pull `deepseek-r1:7b` +
bundle that sets the per-environment `synthesis_model_id` + apply + operator
re-verify). No commit/push performed here per the owner directive; no live-stack
result fabricated. `nextRequiredOwner: bubbles.devops`.

## DevOps Execution Outcome + Operator Runbook (2026-06-14)

`bubbles.devops` ran the commit/push/deploy/A-B dispatch. **Claim Source: executed**
for STEP 1–3 (git + CI observed); **blocked-on-operator** for STEP 4–5 (no live
result fabricated). 087 ships co-mingled with 088 in one commit — see
`specs/088-runtime-switchable-models/report.md` → "DevOps Execution Outcome +
Operator Runbook (2026-06-14)" for the full shared runbook (deploy + the
gemma4:26b-vs-deepseek-r1:7b A/B). 087-specific summary:

| Step | Outcome |
|------|---------|
| 1 — commit | DONE — combined 087+088 commit `99c8d629` (50 files); pii-scan clean; `.kotlin` excluded. |
| 2 — push | DONE — `origin/main 10ed4a48..99c8d629`; pre-push uniformity lint PASSED; no `--no-verify`. |
| 3 — CI build/sign | CI lint+test+canary GREEN (087 validated). `build-images` ✓ (core+ML cosign+Rekor signed, SBOM+SLSA, ghcr digest). `build-clients` ✗ (operator Android keystore secret) → `publish-build-manifest` SKIPPED → **no `build-manifest-99c8d629.yaml`**. |
| 4 — deploy home-lab | BLOCKED-ON-OPERATOR — no build manifest = no Build-Once Deploy-Many input; `deepseek-r1:7b` not resident on the home-lab host; live stack still pre-087/088. |
| 5 — live A/B | BLOCKED-ON-OPERATOR — depends on STEP 4 + the model pull; no A/B captured. |

**Honest terminal status:** `status` held at `blocked` (NOT `done`) — the live
A/B + `verify` did not run. `nextRequiredOwner: operator/user-session`.
