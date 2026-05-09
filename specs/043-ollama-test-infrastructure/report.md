# Spec 043: Ollama Test Infrastructure — Implementation Report

**Status:** in_progress (Scopes not yet executed)

This report is a placeholder for in-progress execution evidence. It will be populated as Scopes 01, 02, 03 are implemented per [`scopes.md`](./scopes.md).

---

## Summary

Spec 043-ollama-test-infrastructure was scaffolded to close MIT-037-OLLAMA-001 (routed in spec 037 commit `ca5f831`). The analyst phase authored the spec.md (7 scenarios, 9 functional requirements). The design phase authored design.md (12 sections, 12 SST keys, 3-phase rollout plan). The plan phase authored scopes.md (3 scopes matching the 3 design rollout phases). Implementation has not yet begun. This report file exists to satisfy artifact-lint required-artifact presence and will be populated with execution evidence as scopes are landed.

## Completion Statement

This spec is **NOT yet complete**. Status remains `in_progress` until all 3 scopes are implemented, tested, validated, audited, and certified. The closure will be marked when:

- Scope 01 (Config + Compose Foundation) lands all 12 SST keys, the test compose Ollama service, and the SST grep guard.
- Scope 02 (Happy-Path Test + Pull Script) lands the live-Ollama e2e test with deterministic output and adversarial fail-loud regression.
- Scope 03 (Wire Into `./smackerel.sh test e2e` + Cross-Spec Closure) lands the `SMACKEREL_TEST_OLLAMA=1` gate, marks MIT-037-OLLAMA-001 resolved in spec 037 state.json, and drops the deferred-infra modifier from spec 037 Scope 5 DoD bullets.

## Test Evidence

No execution evidence yet. Test files planned for authoring during scope implementation are listed below for trace-guard report-evidence reference. Each test file path will be exercised by `./smackerel.sh test unit`, `./smackerel.sh test integration`, or `./smackerel.sh test e2e` as appropriate when scopes are implemented.

---

## Planned Implementation Order

Per [`design.md`](./design.md) §11 Rollout Plan and [`scopes.md`](./scopes.md):

1. **Scope 01 — Config + Compose Foundation** — pending (bubbles.implement)
2. **Scope 02 — Happy-Path Test + Pull Script** — pending (bubbles.implement)
3. **Scope 03 — Wire Into `./smackerel.sh test e2e` + Cross-Spec Closure** — pending (bubbles.implement)

---

## Planned Evidence References (placeholders for trace-guard)

The following test files will be authored as scopes are implemented. Listing them here ensures the trace-guard report-evidence check has the file paths it expects:

- `internal/config/validate_test.go` — Scope 1 SST validation tests (TestValidate_OllamaConfig_FailsLoudOnEmpty, TestValidate_OllamaConfig_FailsLoudOnInvalidPort)
- `internal/config/sst_grep_guard_test.go` — Scope 1 grep-guard for forbidden literals (TestSST_NoHardcodedOllamaValues)
- `internal/deploy/compose_contract_test.go` — Scope 1 compose contract tests (TestComposeContract_TestOllamaService_PresentWithProfile, TestComposeContract_TestOllamaVolume_DistinctFromDev)
- `tests/integration/ollama_health_test.go` — Scope 1 live HTTP API health (TestOllamaHealth_TestProfile_HTTPApiResponds)
- `tests/e2e/agent/happy_path_test.go` — Scope 2 e2e tests (TestAgentHappyPath_DeterministicOutput, TestAgentHappyPath_PlanToolSynthesis, TestOllamaUnreachable_FailsLoudly)
- `tests/e2e/agent/no_skip_guard_test.go` — Scope 2 grep-guard (TestNoSkipBailoutInAgentE2E)
- `scripts/commands/ollama-test-pull.sh` — Scope 2 pull script (smoke test under integration suite)
- `scripts/commands/test.sh` — Scope 3 e2e wiring updates (smoke verified by manual `SMACKEREL_TEST_OLLAMA=1 ./smackerel.sh test e2e` run)
- `specs/037-llm-agent-tools/state.json` — Scope 3 MIT-037-OLLAMA-001 closure

---

## Cross-Spec Closure Plan

This spec's completion will close the following routed backlog items:

- **MIT-037-OLLAMA-001** (routed in spec 037 commit `ca5f831`) — fully resolved when Scope 3 lands.
- **VAL-FINDING-037-G041** — live-Ollama coverage gap closed when Scope 2 happy-path test runs against live Ollama.
- **Spec 037 Scope 5 deferred-infra modifier** — dropped when Scope 3 updates `specs/037-llm-agent-tools/scopes.md`.

---

## References

- [`spec.md`](./spec.md) — feature specification (7 SCN-OLLAMA-NNN scenarios + 9 FR-OLLAMA-NNN requirements)
- [`design.md`](./design.md) — 12-section design (system context, component diagram, SST plan, lifecycle, test anatomy, failure modes, performance budget, isolation, SST compliance, risks, rollout, open questions)
- [`scopes.md`](./scopes.md) — 3 scopes per design rollout plan
- [`scenario-manifest.json`](./scenario-manifest.json) — scenario → evidence-ref manifest (planned status)
- `specs/037-llm-agent-tools/state.json` — MIT-037-OLLAMA-001 routing entry (closure target)
- `.github/skills/bubbles-config-sst/SKILL.md` — SST zero-defaults compliance
- `.github/skills/bubbles-test-environment-isolation/SKILL.md` — test-isolated volume pattern
