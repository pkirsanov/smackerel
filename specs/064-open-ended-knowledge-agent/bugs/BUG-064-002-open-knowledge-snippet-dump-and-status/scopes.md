# BUG-064-002 — Scopes

## SCOPE-01 — Synthesized answer, no duplication, terminal status, capped sources

- **Status:** Done
- **Depends On:** none

### Gherkin scenarios (from spec.md)

- SCN-064-002-A01 — synthesized answer, not a snippet dump (DEFECT 1)
- SCN-064-002-A02 — no triplicate duplication in salvage (DEFECT 2)
- SCN-064-002-A03 — terminal status, not "thinking…" (DEFECT 3a)
- SCN-064-002-A04 — source set capped and deduplicated (DEFECT 3b)
- SCN-064-002-A05 — fail-loud source cap (FR-5)

### Implementation plan

1. De-duplicate `synthesizeFromSnippets` in
   `internal/assistant/openknowledge/agent/agent.go` (normalized dedup key;
   one unique snippet per tool call).
2. Add REQUIRED `Config.SourcesMax` (validated `> 0` in `New()`); add a
   `cappedTraceSources` helper; use it in the salvage source-attach sites.
3. Wire `cfg.Assistant.SourcesMax` → `okagent.Config.SourcesMax` in
   `cmd/core/wiring_assistant_openknowledge.go`; set `SourcesMax` in the
   `baseCfg` test helper.
4. Add terminal `StatusAnswered` token to `internal/assistant/contracts/response.go`
   (+ `AllStatusTokens`); map `open_knowledge` `OutcomeOK` → `StatusAnswered`
   in `facade.go::translateOutcomeToStatus`; render no prefix in
   `render_outbound.go::statusPrefix`.
5. Redesign `config/prompt_contracts/open_knowledge.yaml::agent_system_prompt`
   final-answer shape to extract-then-synthesize and forbid raw-snippet
   passthrough (preserve the `<CITATIONS>` contract verbatim).

### Test Plan

| # | Test Type | Category | File | Description | Command | Live |
|---|-----------|----------|------|-------------|---------|------|
| T1 | Unit | `unit` | `internal/assistant/openknowledge/agent/snippet_dedup_bug064002_test.go` | `synthesizeFromSnippets` dedups identical lead snippets → appears once (SCN-A02) | `./smackerel.sh test unit` | No |
| T2 | Unit | `unit` | `internal/assistant/openknowledge/agent/snippet_dedup_bug064002_test.go` | end-to-end `Run`: empty forced-final salvage body is deduped, not 3× passthrough; real cited synthesis body == synthesis, not snippets (SCN-A01) | `./smackerel.sh test unit` | No |
| T3 | Unit | `unit` | `internal/assistant/facade_open_knowledge_status_bug064002_test.go` | `translateOutcomeToStatus(OutcomeOK,"open_knowledge")` is terminal answered, not thinking (SCN-A03) | `./smackerel.sh test unit` | No |
| T3b | Unit | `unit` | `internal/telegram/assistant_adapter/render_outbound_test.go` | `statusPrefix(StatusAnswered)` renders no "thinking…" (SCN-A03) | `./smackerel.sh test unit` | No |
| T4 | Unit | `unit` | `internal/assistant/openknowledge/agent/snippet_dedup_bug064002_test.go` | salvage source set capped to `SourcesMax`, deduped (SCN-A04) | `./smackerel.sh test unit` | No |
| T5 | Unit | `unit` | `internal/assistant/openknowledge/agent/snippet_dedup_bug064002_test.go` | `New()` rejects `SourcesMax <= 0` (SCN-A05) | `./smackerel.sh test unit` | No |
| T6 | Integration | `integration` | `internal/assistant/facade_open_knowledge_status_bug064002_test.go` | facade open_knowledge OutcomeOK path: assembled `AssistantResponse.Status` is terminal answered (un-redacted assembly proof) | `./smackerel.sh test unit` | No |
| T7 | Regression | `unit` | (existing) `agent_test.go`, `render_outbound_test.go`, `wiring_assistant_openknowledge_test.go` | existing salvage / status / assembler tests still pass | `./smackerel.sh test unit` | No |

> The open-knowledge agent + facade run in-process with fakes (LLM, web tool,
> registry); these are real-code-path Go tests asserting the assembled,
> un-redacted `TurnResult.FinalText` / `AssistantResponse.Body`, satisfying the
> "get the real final body" investigation requirement without the redacted prod
> log. A live self-hosted E2E (real Telegram + searxng + GPU) is the redeploy
> verification step, owned by `bubbles.devops` (see report.md).

### Definition of Done

Core items:
- [x] DEFECT 2 fixed: `synthesizeFromSnippets` de-duplicates; identical lead snippets across tool calls collapse to one block → Evidence: [report.md#t1]
- [x] DEFECT 1 fixed: salvage body is not a verbatim N× snippet passthrough; a real cited synthesis is preserved as the body; prompt contract instructs extract-then-synthesize and forbids raw-snippet passthrough → Evidence: [report.md#t2] [report.md#prompt]
- [x] DEFECT 3a fixed: completed `open_knowledge` answer carries terminal `StatusAnswered`; Telegram renders no `thinking…` header → Evidence: [report.md#t3] [report.md#t3b]
- [x] DEFECT 3b fixed: agent caps + dedups the salvaged source set to `assistant.sources_max` → Evidence: [report.md#t4]
- [x] FR-5 fail-loud: `New()` rejects non-positive `SourcesMax`; cap sourced from `config/smackerel.yaml` (no `${VAR:-default}`) → Evidence: [report.md#t5]
- [x] Reproduction gate: the snippet-dump + triplicate behavior reproduced RED before the fix and GREEN after, asserted on the un-redacted assembled body → Evidence: [report.md#reproduction]
- [x] Adversarial regression tests added (T1–T6), non-tautological, RED→GREEN → Evidence: [report.md#red] [report.md#green]
- [x] Existing open-knowledge / status / assembler tests still pass (no weakened assertions) → Evidence: [report.md#t7]

Build Quality Gate (grouped):
- [x] `./smackerel.sh test unit` (Go) passes; `./smackerel.sh build` clean; `gofmt`/`go vet` clean; zero warnings; zero deferrals; artifact lint clean; docs aligned → Evidence: [report.md#build] [report.md#lint]
