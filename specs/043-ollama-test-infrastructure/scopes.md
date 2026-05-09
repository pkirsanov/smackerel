# Spec 043: Ollama Test Infrastructure — Scopes

**Workflow Mode:** full-delivery
**Source spec:** [`spec.md`](./spec.md) — 7 SCN-OLLAMA-001..007 + 9 FR-OLLAMA-001..009
**Source design:** [`design.md`](./design.md) — 12 sections, 12 SST keys, 3-phase rollout plan
**Closes:** MIT-037-OLLAMA-001 (routed in spec 037 commit `ca5f831`); VAL-FINDING-037-G041 live-Ollama coverage gap; spec 037 Scope 5 deferred-infra modifier; spec 037 Scope 9 e2e LLM-agent path.

---

## Scope Strategy

The 3 scopes match the 3 sequential rollout phases from design.md §11:

1. **Scope 01 — Config + Compose Foundation** — SST keys, env generation, test-stack Ollama service. Sole sources of Ollama runtime values pinned by SST. No tests run yet.
2. **Scope 02 — Happy-Path Test + Pull Script** — Author the e2e test + pull script. Hand-runnable end-to-end against compose with Ollama profile.
3. **Scope 03 — Wire Into `./smackerel.sh test e2e` + Cross-Spec Closure** — Default e2e gate behind `SMACKEREL_TEST_OLLAMA=1`; close MIT-037-OLLAMA-001; drop deferred-infra modifier from spec 037 Scope 5.

Each scope ends with a working state. Test plan rows must reference real test files. DoD bullets carry `Scenario "<SCN-OLLAMA-NNN ...>": ` trace prefix per Gate G068.

---

## Scope Table

| # | Name | Surfaces | Tests | DoD Summary | Status |
|---|------|----------|-------|-------------|--------|
| 1 | Config + Compose Foundation | `config/smackerel.yaml`, `scripts/commands/config.sh`, `docker-compose.yml` | unit, integration | 12 SST keys live; ollama service uses `${OLLAMA_IMAGE}`; zero hardcoded values | [x] Done |
| 2 | Happy-Path Test + Pull Script | `tests/e2e/agent/`, `scripts/commands/ollama-test-pull.sh` | e2e, adversarial | `TestAgentHappyPath_PlanToolSynthesis` runs against live Ollama; deterministic output; fail-loud on unavailable | [ ] Not started |
| 3 | Wire Into `./smackerel.sh test e2e` + Cross-Spec Closure | `scripts/commands/test.sh`, `specs/037-llm-agent-tools/state.json`, `specs/037-llm-agent-tools/scopes.md` | e2e, smoke | `SMACKEREL_TEST_OLLAMA=1` gate; spec 037 deferred-infra modifier dropped; MIT-037-OLLAMA-001 marked resolved | [ ] Not started |

---

## Scope Validation Strategy

- After **Scope 1**: `./smackerel.sh check` confirms config in sync; `./smackerel.sh up --env=test --profile ollama` brings up Ollama container with healthy `/api/tags` endpoint.
- After **Scope 2**: hand-run `tests/e2e/agent/happy_path_test.go` against test compose with Ollama profile; deterministic output reproduces across 3 runs; adversarial `TestOllamaUnreachable_FailsLoudly` fails-loud (not skip).
- After **Scope 3**: `SMACKEREL_TEST_OLLAMA=1 ./smackerel.sh test e2e` runs the full e2e suite including agent happy-path; spec 037 trace-guard remains PASSED with Scope 5 deferred-infra modifier removed.

---

## Scope 1: Config + Compose Foundation

**Status:** Done (2026-05-09)
**Phase:** implement
**Agent:** bubbles.implement
**Goal:** Land all 12 Ollama-related SST keys in `config/smackerel.yaml`, emit them via `./smackerel.sh config generate`, and add the `smackerel-test-ollama` service to test compose. After this scope, `./smackerel.sh up --env=test --profile ollama` brings up a healthy Ollama container.
**FR coverage:** FR-OLLAMA-001 (Ollama starts on test profile), FR-OLLAMA-002 (qwen2.5:0.5b-instruct cached), FR-OLLAMA-006 (SST source), FR-OLLAMA-007 (dev opt-in profile preserved), FR-OLLAMA-008 (test isolation).
**Dependencies:** None.

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-OLLAMA-001 Ollama starts deterministically with qwen2.5:0.5b-instruct on test profile
  Given config/smackerel.yaml declares the infrastructure.ollama and environments.test.ollama_* blocks
  And docker-compose.test.yml defines smackerel-test-ollama service gated on profile ollama
  When ./smackerel.sh up --env=test --profile ollama runs
  Then the smackerel-test-ollama container reports healthy
  And the Ollama HTTP API at the configured host_port returns 200 on /api/tags within pull_timeout_seconds

Scenario: SCN-OLLAMA-005 Per-environment Ollama storage volumes remain isolated
  Given dev compose uses volume smackerel-ollama-data and test compose uses smackerel-test-ollama-data
  When ./smackerel.sh down --volumes --env=test runs
  Then volume smackerel-test-ollama-data is removed
  And volume smackerel-ollama-data is preserved (dev cache untouched)

Scenario: SCN-OLLAMA-006 Configuration flows through SST, no hardcoded model strings
  Given the source tree contains internal/, cmd/, ml/, scripts/, Dockerfile, docker-compose*.yml
  When grep -rn '11434\|qwen2.5\|ollama:0.4' --include='*.go' --include='*.py' --include='Dockerfile' --include='docker-compose*.yml' --include='*.sh' . runs
  Then zero matches are reported outside config/smackerel.yaml and config/generated/
```

### Implementation Plan (no code)

- Add to `config/smackerel.yaml`:
  - `infrastructure.ollama.image` (root key for Ollama image identifier; per design.md §3 OQ-D1, pinned to digest)
  - `infrastructure.ollama.container_port` (11434, reused existing slot)
  - `infrastructure.ollama.test.image` (image for test stack — initially same as root, allows future divergence)
  - `infrastructure.ollama.test.model` (`qwen2.5:0.5b-instruct`)
  - `infrastructure.ollama.test.pull_timeout_seconds` (300)
  - `infrastructure.ollama.test.request_temperature` (0)
  - `infrastructure.ollama.test.request_top_p` (1)
  - `infrastructure.ollama.test.request_top_k` (1)
  - `infrastructure.ollama.test.request_seed` (42)
  - `infrastructure.ollama.test.request_num_predict` (512)
  - `environments.dev.ollama_enabled` (default true with profile gate)
  - `environments.test.ollama_enabled` (default false; test runner sets to true via `SMACKEREL_TEST_OLLAMA=1`)
  - `environments.home-lab.ollama_enabled` (default false)
  - Reuse existing `environments.test.ollama_host_port` (47004 — distinct from postgres:47001, nats:47002, nats_monitor:47003 per design.md §3 correction)
  - Reuse existing `environments.test.ollama_volume_name` (`smackerel-test-ollama-data`)
- Update `scripts/commands/config.sh` to emit all `OLLAMA_*` keys to `config/generated/test.env` with required-value validation.
- Update `docker-compose.test.yml` to add `smackerel-test-ollama` service with `profiles: [ollama]`, healthcheck on `/api/tags`, volume mount for model cache.
- Update `internal/deploy/compose_contract_test.go` to assert `smackerel-test-ollama` service contract when `ollama_enabled=true`.
- Forbidden patterns: `os.Getenv("OLLAMA_*", "fallback")`, hardcoded `:11434`, hardcoded `ollama/ollama:` tags, hardcoded `qwen2.5` model strings.

### Test Plan

| ID | Test Type | Location | Trace ID | Assertion |
|----|-----------|----------|----------|-----------|
| T1-01 | integration | `tests/integration/ollama_config_contract_test.go` | SCN-OLLAMA-006 | `TestOllamaConfigGenerateAndRuntimeValidationStayInSync/AdversarialMissingTestModel` and `AdversarialMissingTestImage` strip the SST key from a temp YAML and assert `config.sh` exits non-zero with the missing key named in stderr (fail-loud at the SST→env-file boundary; covers the implementation-equivalent of `TestValidate_OllamaConfig_FailsLoudOnEmpty`) |
| T1-02 | integration | `tests/integration/ollama_config_contract_test.go` | SCN-OLLAMA-006 | `TestOllamaConfigGenerateAndRuntimeValidationStayInSync/AdversarialMissingRequestSeed` strips a determinism knob and asserts `config.sh` fails-loud (covers fail-loud determinism-knob requirement; the host-port validity-range check from the planning row was redirected here because `OLLAMA_HOST_PORT` is a docker-compose substitution var with no Go runtime consumer to validate against, and the SST-emission fail-loud is the single source of truth for missing-key validation) |
| T1-03 | unit | `internal/deploy/compose_ollama_contract_test.go` | SCN-OLLAMA-001 | `TestOllamaComposeContract_LiveFile` asserts `services.ollama` exists in `docker-compose.yml` with `image: ${OLLAMA_IMAGE}` and `profiles: [ollama]` (planning row referenced `docker-compose.test.yml` + `smackerel-test-ollama`; design.md §3 OQ-D1 collapsed to a single compose file with profile-gating + per-env env-file substitution, so the test contract reflects that single-file shape) |
| T1-04 | unit | `internal/deploy/compose_ollama_contract_test.go` | SCN-OLLAMA-005 | `TestOllamaComposeContract_LiveFile` + `TestOllamaComposeContract_AdversarialHardcodedVolumeName` assert the named volume `ollama-data` resolves to `${OLLAMA_VOLUME_NAME}` (not a hardcoded string), which is the indirection that keeps `smackerel-ollama-data` (dev) and `smackerel-test-ollama-data` (test) on distinct named volumes |
| T1-05 | integration | `tests/integration/ollama_config_contract_test.go` | SCN-OLLAMA-001 | `TestOllamaConfigGenerateAndRuntimeValidationStayInSync` (primary path) asserts every required `OLLAMA_*` key is emitted to `config/generated/test.env` AND `ENABLE_OLLAMA=true` AND `OLLAMA_TEST_MODEL=qwen2.5:0.5b-instruct` (proves SST→env round-trip; live HTTP `/api/tags` health is deferred to Scope 2 where the model pull lives) |
| T1-06 | adversarial-grep | `internal/config/sst_grep_guard_test.go` | SCN-OLLAMA-006 | `TestSST_NoHardcodedOllamaValues` walks `internal/`, `cmd/`, `ml/app/`, `scripts/`, `Dockerfile`, `docker-compose*.yml` for forbidden literals (`11434`, `qwen2.5`, `ollama/ollama:`); fails if any match outside `config/`. Adversarial: `TestSST_NoHardcodedOllamaValues_Adversarial` proves the scanner reports all three literals against a synthetic naughty-package fixture; `TestSST_NoHardcodedOllamaValues_AllowlistAdversarial` proves the `*_test.go` allowlist works |

### Definition of Done

- [x] Scenario "SCN-OLLAMA-001 Ollama starts deterministically with qwen2.5:0.5b-instruct on test profile": 12 SST keys added to `config/smackerel.yaml`; `./smackerel.sh config generate --env=test` emits all `OLLAMA_*` keys to `config/generated/test.env`. Live `./smackerel.sh up --env=test --profile ollama` deferred to Scope 2 (model pull is Scope 2 surface; Scope 1 only proves config + compose substrate correctness via the integration contract test).
  - Evidence: `config/smackerel.yaml` lines 637-665 (root + test sub-block) and lines 671-715 (per-env `ollama_enabled`); `scripts/commands/config.sh` lines 366-396 (per-env override + test/non-test branching) and lines 823-830 (heredoc emission); `tests/integration/ollama_config_contract_test.go::TestOllamaConfigGenerateAndRuntimeValidationStayInSync` asserts every required `OLLAMA_*` key present in `config/generated/test.env` AND adversarial generation against a YAML missing `infrastructure.ollama.test.{model,image,request_seed}` exits non-zero with the missing key named in stderr.
- [x] Scenario "SCN-OLLAMA-005 Per-environment Ollama storage volumes remain isolated": Dev `OLLAMA_VOLUME_NAME=smackerel-ollama-data` (line 681 of `config/smackerel.yaml`) and test `OLLAMA_VOLUME_NAME=smackerel-test-ollama-data` (line 695) resolve to distinct named volumes via `${OLLAMA_VOLUME_NAME}` in `docker-compose.yml` line 200; per-env Compose project name (`smackerel` vs `smackerel-test`) provides the second isolation axis.
  - Evidence: `internal/deploy/compose_ollama_contract_test.go::TestOllamaComposeContract_LiveFile` asserts `volumes.ollama-data.name == "${OLLAMA_VOLUME_NAME}"`; `TestOllamaComposeContract_AdversarialHardcodedVolumeName` proves the contract function rejects a literal `smackerel-ollama-data` volume name.
- [x] Scenario "SCN-OLLAMA-006 Configuration flows through SST, no hardcoded model strings": `grep -rnE '11434|qwen2\.5|ollama/ollama:'` over `internal/`, `cmd/`, `ml/app/`, `scripts/`, `Dockerfile`, `docker-compose*.yml` returns ZERO matches outside `config/`. Pre-existing fallback `url = ollama_url or "http://localhost:11434"` at `ml/app/intelligence.py:40` was an SST violation rooted in spec 037 plumbing; replaced with explicit fail-loud `if not ollama_url: ... return None` so the SST-source `OLLAMA_URL` is the only configured URL path.
  - Evidence: `internal/config/sst_grep_guard_test.go::TestSST_NoHardcodedOllamaValues` walks the production source tree and asserts zero findings; `TestSST_NoHardcodedOllamaValues_Adversarial` proves the scanner reports all three forbidden literals against a synthetic naughty-package fixture; `TestSST_NoHardcodedOllamaValues_AllowlistAdversarial` proves `*_test.go` files are correctly skipped.
- [x] All unit + integration tests pass: `./smackerel.sh test unit` returns exit 0 (78 Go packages PASS, 411 Python tests PASS); the Scope 1 integration test `tests/integration/ollama_config_contract_test.go` is build-tag-gated `//go:build integration` and is exercised by `./smackerel.sh test integration`.
  - Evidence: `./smackerel.sh test unit` invocation on 2026-05-09 — Go portion exited 0 with `ok` for every package including `internal/config`, `internal/deploy`, `internal/intelligence`; Python portion exited 0 with `411 passed in 15.04s`; `go test ./internal/deploy/ -v` showed all 4 `TestOllamaComposeContract*` subtests + the 3 pre-existing spec-042 contract subtests PASSED.
- [x] `./smackerel.sh check` passes: `Config is in sync with SST` + `env_file drift guard: OK` + `scenario-lint: OK` (4 scenarios registered, 0 rejected).
  - Evidence: `./smackerel.sh check` invocation on 2026-05-09 — exit 0 with the three OK lines above.

---

## Scope 2: Happy-Path Test + Pull Script

**Status:** Not started
**Phase:** implement
**Agent:** bubbles.implement
**Goal:** Author `tests/e2e/agent/happy_path_test.go` exercising the production NATS+sidecar+litellm+Ollama path with deterministic output. Author `scripts/commands/ollama-test-pull.sh` to wire model pull into the e2e test setup. Includes adversarial regression test asserting fail-loud (NOT skip) when Ollama or model is unavailable.
**FR coverage:** FR-OLLAMA-003 (happy-path test), FR-OLLAMA-004 (allowlisted read-only tool + agent_traces assertions), FR-OLLAMA-005 (fail-loud, no `t.Skip()` bailout).
**Dependencies:** Scope 1 (Config + Compose Foundation).

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-OLLAMA-002 Agent happy path test exercises plan→tool→synthesis loop with deterministic temperature=0
  Given test compose is up with the ollama profile and qwen2.5:0.5b-instruct cached
  And request_temperature=0, request_top_p=1, request_top_k=1, request_seed=42 are pinned via SST
  When tests/e2e/agent/happy_path_test.go::TestAgentHappyPath_DeterministicOutput runs 3 times
  Then all 3 runs produce byte-identical agent_traces.synthesis_response payloads

Scenario: SCN-OLLAMA-003 Happy-path test exercises plan→tool→synthesis through real Ollama
  Given an allowlisted read-only tool (e.g., search_artifacts) is registered
  When the e2e test sends an agent invocation requiring the tool
  Then agent_traces records exactly 3 ordered steps: plan_step, tool_call_step, synthesis_step
  And each step's NATS-subject and tool-name fields are populated
  And the test runs to completion in less than the cold-path budget (≤ 90s including stack startup)

Scenario: SCN-OLLAMA-004 Test fails loudly when Ollama or model is unavailable (no t.Skip bailout)
  Given the e2e test framework is started but Ollama container is intentionally stopped
  When the e2e suite runs tests/e2e/agent/happy_path_test.go::TestOllamaUnreachable_FailsLoudly
  Then the test produces a Go test FAILURE (rc != 0)
  And the test does NOT call t.Skip(), t.SkipNow(), or any equivalent bailout
  And the failure message includes the unreachable URL and the SST-configured pull_timeout_seconds
```

### Implementation Plan (no code)

- Author `scripts/commands/ollama-test-pull.sh` (curl-based pull via Ollama HTTP `/api/pull`; respects `pull_timeout_seconds` SST key; emits structured progress; fail-loud on non-200).
- Author `tests/e2e/agent/happy_path_test.go` with build-tag gating (`// +build e2e_ollama` or runtime check on `SMACKEREL_TEST_OLLAMA=1`):
  - `TestAgentHappyPath_PlanToolSynthesis`: registers an allowlisted read-only tool, sends agent invocation, polls `agent_traces` table for 3-step trace, asserts `plan_step → tool_call_step → synthesis_step` order and field population.
  - `TestAgentHappyPath_DeterministicOutput`: runs the same invocation 3 times, asserts byte-identical synthesis output (temperature=0, fixed seed, fixed top_p/top_k).
  - `TestOllamaUnreachable_FailsLoudly`: adversarial regression — stops Ollama container mid-test (or points to nonexistent URL), asserts test FAILS not SKIPS.
- Forbidden patterns: `t.Skip()`, `t.SkipNow()`, `t.Skipf()` anywhere under `tests/e2e/agent/`. Per `.github/copilot-instructions.md`: every regression test must include at least one adversarial case that would fail if the bug were reintroduced.

### Test Plan

| ID | Test Type | Location | Trace ID | Assertion |
|----|-----------|----------|----------|-----------|
| T2-01 | e2e | `tests/e2e/agent/happy_path_test.go` | SCN-OLLAMA-002 | `TestAgentHappyPath_DeterministicOutput` runs the same invocation 3 times; asserts byte-identical agent_traces.synthesis_response payloads |
| T2-02 | e2e | `tests/e2e/agent/happy_path_test.go` | SCN-OLLAMA-003 | `TestAgentHappyPath_PlanToolSynthesis` records 3-step trace in agent_traces table (plan → tool_call → synthesis); each step has populated NATS-subject and tool-name fields |
| T2-03 | adversarial | `tests/e2e/agent/happy_path_test.go` | SCN-OLLAMA-004 | `TestOllamaUnreachable_FailsLoudly` produces Go test FAILURE (rc != 0) when Ollama is stopped; failure message includes unreachable URL and `pull_timeout_seconds` |
| T2-04 | grep-guard | `tests/e2e/agent/no_skip_guard_test.go` | SCN-OLLAMA-004 | `TestNoSkipBailoutInAgentE2E` greps `tests/e2e/agent/*.go` for `t.Skip\|t.SkipNow\|t.Skipf` outside the regression test that adversarially asserts fail-loud; returns ZERO unexpected matches |
| T2-05 | smoke | `scripts/commands/ollama-test-pull.sh` | SCN-OLLAMA-002 | Manual run with `SMACKEREL_TEST_OLLAMA=1`; asserts pull completes within `pull_timeout_seconds`; fail-loud on non-200 |

### Definition of Done

- [ ] Scenario "SCN-OLLAMA-002 Agent happy path test exercises plan→tool→synthesis loop with deterministic temperature=0": `tests/e2e/agent/happy_path_test.go` authored with `TestAgentHappyPath_DeterministicOutput`; 3-run comparison shows byte-identical agent_traces.synthesis_response.
- [ ] Scenario "SCN-OLLAMA-003 Happy-path test exercises plan→tool→synthesis through real Ollama": `TestAgentHappyPath_PlanToolSynthesis` passes against test compose with Ollama profile; agent_traces records 3 ordered steps; cold-path wall-clock ≤ 90s.
- [ ] Scenario "SCN-OLLAMA-004 Test fails loudly when Ollama or model is unavailable": adversarial test `TestOllamaUnreachable_FailsLoudly` exists; `grep -rnE 't\.Skip(\(|f\(|Now\()' tests/e2e/agent/` returns ZERO matches outside the explicitly-allowlisted regression test.
- [ ] `scripts/commands/ollama-test-pull.sh` authored; respects `pull_timeout_seconds` SST key; fail-loud on non-200 from Ollama HTTP API.
- [ ] All e2e tests still pass: `SMACKEREL_TEST_OLLAMA=1 ./smackerel.sh test e2e` (when manually run with profile gate).

---

## Scope 3: Wire Into `./smackerel.sh test e2e` + Cross-Spec Closure

**Status:** Not started
**Phase:** implement
**Agent:** bubbles.implement
**Goal:** Wire `./smackerel.sh test e2e` to gate Ollama profile startup + model pull on `SMACKEREL_TEST_OLLAMA=1` env-var. Update spec 037 state.json + scopes.md to mark MIT-037-OLLAMA-001 closed and drop "deferred infra" modifier from Scope 5 DoD bullets.
**FR coverage:** FR-OLLAMA-009 (Closing this spec resolves MIT-037-OLLAMA-001 + drops Scope 5 modifier).
**Dependencies:** Scope 1 (Config + Compose Foundation), Scope 2 (Happy-Path Test + Pull Script).

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-OLLAMA-007 Closing this spec closes MIT-037-OLLAMA-001 and unblocks Scope 9
  Given spec 043 Scopes 1 + 2 are Done
  And SMACKEREL_TEST_OLLAMA=1 gates Ollama profile in ./smackerel.sh test e2e
  When the e2e suite runs with SMACKEREL_TEST_OLLAMA=1
  Then tests/e2e/agent/happy_path_test.go runs and passes against live Ollama
  And specs/037-llm-agent-tools/state.json MIT-037-OLLAMA-001 entry status changes to "resolved" with a closure-link to spec 043
  And specs/037-llm-agent-tools/scopes.md Scope 5 DoD bullets drop their "deferred infra" modifier
```

### Implementation Plan (no code)

- Update `scripts/commands/test.sh` (or wherever `./smackerel.sh test e2e` dispatches) to detect `SMACKEREL_TEST_OLLAMA=1` env-var and:
  - Add `--profile ollama` to the compose-up call.
  - Run `scripts/commands/ollama-test-pull.sh` after compose readiness probe.
  - Run the e2e suite including `tests/e2e/agent/`.
  - Tear down with `--volumes` to remove the test-isolated cache.
- When `SMACKEREL_TEST_OLLAMA` is unset or 0, e2e suite skips Ollama profile startup cleanly (no compose changes; the agent happy_path test is build-tag gated so it won't even compile into the test binary).
- Update `specs/037-llm-agent-tools/state.json` MIT-037-OLLAMA-001 entry to `status: resolved` with `closureCommit: <commit>`, `closureSpec: 043-ollama-test-infrastructure`.
- Update `specs/037-llm-agent-tools/scopes.md` Scope 5 DoD bullets to drop "(Done modulo deferred infra — see MIT-037-OLLAMA-001)" modifier text.
- Update `specs/037-llm-agent-tools/scopes.md` Scope 9 (if applicable per spec 037 state) to mark live-Ollama coverage as available.
- Verify spec 037 trace-guard remains PASSED after status edits.

### Test Plan

| ID | Test Type | Location | Trace ID | Assertion |
|----|-----------|----------|----------|-----------|
| T3-01 | e2e | `tests/e2e/agent/happy_path_test.go` | SCN-OLLAMA-007 | When `SMACKEREL_TEST_OLLAMA=1`, `./smackerel.sh test e2e` starts Ollama profile + runs `TestAgentHappyPath_PlanToolSynthesis` to completion |
| T3-02 | smoke | `scripts/commands/test.sh` | SCN-OLLAMA-007 | When `SMACKEREL_TEST_OLLAMA` is unset, e2e suite runs without Ollama profile (no startup attempt; agent happy_path test compiled out via build tag) |
| T3-03 | spec-state | `specs/037-llm-agent-tools/state.json` | SCN-OLLAMA-007 | MIT-037-OLLAMA-001 entry has `status: resolved` and `closureSpec: 043-ollama-test-infrastructure` after Scope 3 closure commit |
| T3-04 | spec-trace | `specs/037-llm-agent-tools/scopes.md` | SCN-OLLAMA-007 | `bash .github/bubbles/scripts/traceability-guard.sh specs/037-llm-agent-tools` returns PASSED after deferred-infra modifier removal |

### Definition of Done

- [ ] Scenario "SCN-OLLAMA-007 Closing this spec closes MIT-037-OLLAMA-001 and unblocks Scope 9": `./smackerel.sh test e2e` gates Ollama startup on `SMACKEREL_TEST_OLLAMA=1`; happy-path test runs against live Ollama; spec 037 MIT-037-OLLAMA-001 marked resolved with closure-link to spec 043.
- [ ] `specs/037-llm-agent-tools/state.json` MIT-037-OLLAMA-001 entry has `status: resolved` and `closureSpec: 043-ollama-test-infrastructure`.
- [ ] `specs/037-llm-agent-tools/scopes.md` Scope 5 DoD bullets no longer carry "(Done modulo deferred infra)" modifier text.
- [ ] `bash .github/bubbles/scripts/traceability-guard.sh specs/037-llm-agent-tools` returns PASSED.
- [ ] All e2e + integration + unit tests still pass.

---

## Cross-Cutting Test & Validation Discipline

- All tests labeled `e2e` or `integration` in this spec MUST hit the live test stack with the Ollama profile up — no mocks, no `httptest.Server`, no NATS in-memory shims.
- The adversarial regression test in Scope 2 (`TestOllamaUnreachable_FailsLoudly`) is REQUIRED per `.github/copilot-instructions.md` Adversarial Regression Tests rule. Removing it would cause regression-baseline-guard to fail.
- All Ollama config values originate from `config/smackerel.yaml` and flow through `config/generated/test.env` per SST zero-defaults. The grep-guard test (`TestSST_NoHardcodedOllamaValues`) provides ongoing enforcement.
- Per design.md §10 risks, Scope 1 includes the `pull_timeout_seconds` ceiling to prevent unbounded pull stalls; Scope 2's adversarial test verifies fail-loud behavior when this timeout is exceeded.

---

## References

- `specs/043-ollama-test-infrastructure/spec.md` — feature specification
- `specs/043-ollama-test-infrastructure/design.md` — architecture + 12-section design
- `specs/037-llm-agent-tools/state.json` — MIT-037-OLLAMA-001 routing entry (commit `ca5f831`)
- `specs/037-llm-agent-tools/scopes.md` — Scope 5 deferred-infra modifier (to be dropped in Scope 3)
- `.github/skills/bubbles-config-sst/SKILL.md` — SST zero-defaults compliance
- `.github/skills/bubbles-test-environment-isolation/SKILL.md` — test-isolated volume pattern
- `.github/skills/bubbles-docker-port-standards/SKILL.md` — port allocation (47001 postgres, 47002 nats, 47003 nats_monitor, 47004 ollama)
- `.github/copilot-instructions.md` — Adversarial Regression Tests rule, SST zero-defaults non-negotiable, repo-CLI surface
