# Bug: BUG-037-002 — DoD scenario fidelity gap (spec 037 traceability-guard)

## Classification

- **Type:** Artifact-only documentation/traceability bug
- **Severity:** MEDIUM (governance gate failure on an in-progress feature; no runtime impact)
- **Parent Spec:** 037 — LLM Scenario Agent & Tool Registry
- **Workflow Mode:** bugfix-fastlane
- **Status:** Fixed (artifact-only)

## Problem Statement

Bubbles traceability-guard reported `RESULT: FAILED (16 failures, 0 warnings)` against `specs/037-llm-agent-tools` covering 33 Gherkin scenarios (SCN-037-001..SCN-037-033). All failures were artifact-level — every scenario's behavior is delivered in production code (`internal/agent/`, `cmd/scenario-lint/`, `cmd/core/`, `internal/web/`, `internal/api/`, `internal/telegram/`, `internal/scheduler/`, `internal/pipeline/`, `ml/app/agent.py`, db migrations, Go + Python tests). The gap is purely in trace-tag fidelity between Gherkin scenarios, Test Plan rows, the missing scenario-manifest.json, and report.md cross-references.

## Failure Categories (pre-fix baseline)

| Category | Count | Description |
|----------|------:|-------------|
| Missing scenario-manifest.json (G057/G059) | 1 | `scenario-manifest.json` not present despite 33 SCN-037-* scenarios resolved |
| Test Plan row references non-existent file path (Scope 1, 5) | 4 | SCN-037-001/002 cite `internal/config/agent_test.go` / `internal/nats/contract_agent_test.go` / `tests/integration/config/{agent_env,sst_guard_agent}_test.go` (none exist); SCN-037-017 cites `tests/integration/agent/executor_bs021_test.go` (actual test is `tests/integration/agent/loop_test.go::TestExecutor_BS021_LLMTimeout`) |
| Test Plan row matched header row instead of body (Scope 3) | 1 | SCN-037-006 fuzzy-matches the column header `\| Layer \| Scenario \| File \| Type \|` because its title contains both `scenario` and `file`; header has no test path → "no concrete test file path" |
| Report.md missing literal evidence path (`grep -Fq`) | 11 | Test Plan rows for SCN-037-003/004/005/007/008/009/010/011/012/031/032 reference `registry_test.go`, `registry_schema_test.go`, `loader_bs009/010/011_test.go`, `router_similarity_test.go` (×2), `router_floor_test.go`, `scheduler_bridge_test.go`, `ci_forbidden_pattern_test.go` — these files exist on disk but their full repo-relative paths are not literal substrings in `specs/037-llm-agent-tools/report.md` (the guard uses `grep -F` so brace-expansion `{a,b}_test.go` in the report doesn't match `tests/integration/agent/a_test.go`) |
| **Total** | **16** | |

## Reproduction (Pre-fix)

```
$ bash .github/bubbles/scripts/traceability-guard.sh specs/037-llm-agent-tools 2>&1 | tail -1
RESULT: FAILED (16 failures, 0 warnings)
```

## Gap Analysis (per category)

For every flagged scenario, the bug investigator confirmed the underlying behavior is **delivered-and-tested** in production code. Examples:

| Scenario | Delivered? | Tested? | Concrete test file (actual) | Concrete source |
|----------|-----------|---------|-----------------------------|-----------------|
| SCN-037-001 Agent SST block | Yes — `agent:` block in `config/smackerel.yaml`, generated env files emit `AGENT_*`, no hardcoded defaults | Yes — `internal/agent/config_test.go`, `internal/agent/sst_guard_test.go` | `internal/agent/config_test.go` (Test Plan row pointed at non-existent `internal/config/agent_test.go`) | `internal/agent/config.go`, `config/smackerel.yaml` |
| SCN-037-002 AGENT NATS contract | Yes — AGENT stream in `config/nats_contract.json`, Go + Python contract tests | Yes — `internal/nats/contract_test.go::TestSCN002054_*`, `ml/tests/test_nats_contract.py` | `internal/nats/contract_test.go` (Test Plan row pointed at non-existent `internal/nats/contract_agent_test.go`) | `internal/nats/`, `ml/app/`, `config/nats_contract.json` |
| SCN-037-006 Loader §2.2 rules | Yes — `internal/agent/loader.go::parseScenario` + linter | Yes — `internal/agent/loader_rules_test.go::TestLoader_Rule_RejectsBadInputs` (14 rejection paths) | `internal/agent/loader_rules_test.go` (header collision blocked match) | `internal/agent/loader.go` |
| SCN-037-017 Provider timeout | Yes — `internal/agent/executor.go` honors per-invocation `context.WithTimeout` | Yes — `tests/integration/agent/loop_test.go::TestExecutor_BS021_LLMTimeout` | `tests/integration/agent/loop_test.go` (Test Plan row pointed at non-existent `tests/integration/agent/executor_bs021_test.go`) | `internal/agent/executor.go` |
| 11 × report-evidence | All Yes | All Yes | exists on disk, only path-string missing in `report.md` | various |

**Disposition:** All 33 scenarios are **delivered-but-trace-mislabeled** — artifact-only fix.

## Acceptance Criteria

- [x] `specs/037-llm-agent-tools/scenario-manifest.json` exists and registers all 33 SCN-037-* scenarios with linked test files and DoD pointers
- [x] Scope 1 Test Plan rows for SCN-037-001 / SCN-037-002 reference existing test files (`internal/agent/config_test.go`, `internal/nats/contract_test.go`, `internal/agent/sst_guard_test.go`) and carry the SCN-037-NNN tag
- [x] Scope 3 Test Plan column header renamed `| Layer | Behavior | Location | Type |` to eliminate the SCN-037-006 ↔ header fuzzy collision; the SCN-037-006 trace ID is now embedded as the row prefix
- [x] Scope 5 Test Plan row for SCN-037-017 references the existing `tests/integration/agent/loop_test.go` and carries the SCN-037-017 tag
- [x] `specs/037-llm-agent-tools/report.md` ends with an "Appendix: Traceability Evidence Index (BUG-037-002)" listing the literal repo-relative path tokens for the 12 mapped scenarios whose test files were previously absent from `report.md` (SCN-037-003/004/005/006/007/008/009/010/011/012/031/032). Pre-existing report content is unchanged.
- [x] `bash .github/bubbles/scripts/artifact-lint.sh specs/037-llm-agent-tools` PASSES
- [x] `bash .github/bubbles/scripts/artifact-lint.sh specs/037-llm-agent-tools/bugs/BUG-037-002-dod-scenario-fidelity-gap` PASSES
- [x] `bash .github/bubbles/scripts/traceability-guard.sh specs/037-llm-agent-tools` PASSES with 0 failures
- [x] No production code changed (boundary preserved — only `specs/037-llm-agent-tools/scopes.md`, `specs/037-llm-agent-tools/scenario-manifest.json`, `specs/037-llm-agent-tools/report.md` appendix, and the new bug folder)
- [x] Spec 037 implementation status, ceiling, scope DoD semantics, scenario count, and Gherkin Given/When/Then bodies are unchanged
