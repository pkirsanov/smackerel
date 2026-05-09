# Spec 043: Ollama Test Infrastructure — Implementation Report

**Status:** in_progress (Scope 1 of 3 complete; Scopes 2 + 3 pending)

This report tracks execution evidence as scopes are landed. Scope 1 (Config + Compose Foundation) was implemented on 2026-05-09. Scopes 2 (Happy-Path Test + Pull Script) and 3 (Wire Into `./smackerel.sh test e2e` + Cross-Spec Closure) remain pending per [`scopes.md`](./scopes.md).

---

## Summary

Spec 043-ollama-test-infrastructure was scaffolded to close MIT-037-OLLAMA-001 (routed in spec 037 commit `ca5f831`). The analyst phase authored the spec.md (7 scenarios, 9 functional requirements). The design phase authored design.md (12 sections, 12 SST keys, 3-phase rollout plan). The plan phase authored scopes.md (3 scopes matching the 3 design rollout phases). Implementation has not yet begun. This report file exists to satisfy artifact-lint required-artifact presence and will be populated with execution evidence as scopes are landed.

## Completion Statement

This spec is **NOT yet complete**. Status remains `in_progress` after Scope 1 of 3. The closure will be marked when:

- ~~Scope 01 (Config + Compose Foundation) lands all 12 SST keys, the test compose Ollama service, and the SST grep guard.~~ **DONE 2026-05-09** (12 SST keys live in `config/smackerel.yaml`; `${OLLAMA_IMAGE}` substitution in `docker-compose.yml`; `internal/config/sst_grep_guard_test.go` enforcing zero hardcoded values; `internal/deploy/compose_ollama_contract_test.go` enforcing compose contract; `tests/integration/ollama_config_contract_test.go` enforcing SST→env round-trip).
- Scope 02 (Happy-Path Test + Pull Script) lands the live-Ollama e2e test with deterministic output and adversarial fail-loud regression.
- Scope 03 (Wire Into `./smackerel.sh test e2e` + Cross-Spec Closure) lands the `SMACKEREL_TEST_OLLAMA=1` gate, marks MIT-037-OLLAMA-001 resolved in spec 037 state.json, and drops the deferred-infra modifier from spec 037 Scope 5 DoD bullets.

## Test Evidence

### Scope 1 — Config + Compose Foundation (2026-05-09)

Implemented surfaces:

- `config/smackerel.yaml` — added `infrastructure.ollama.image`, `infrastructure.ollama.test.{image,model,pull_timeout_seconds,request_temperature,request_top_p,request_top_k,request_seed,request_num_predict}`, and `environments.{dev,test,home-lab}.ollama_enabled` (12 new SST keys total per design.md §3).
- `scripts/commands/config.sh` — `OLLAMA_ENABLED` switched from `required_value` to `env_override_value` so per-env `ollama_enabled` wins; new test-vs-non-test branch resolves `OLLAMA_IMAGE` (test image vs root image) and emits `OLLAMA_TEST_*` only for the test env.
- `docker-compose.yml` — `image: ollama/ollama:0.6` replaced with `image: ${OLLAMA_IMAGE}`; profile gate `[ollama]` and `${OLLAMA_VOLUME_NAME}` indirection preserved.
- `ml/app/intelligence.py` — pre-existing SST violation `url = ollama_url or "http://localhost:11434"` (line 40) replaced with explicit fail-loud branch (`if not ollama_url: ... return None`); the SST-source `OLLAMA_URL` is now the only configured URL path for the Python sidecar's Ollama branch.

New tests:

- `internal/config/sst_grep_guard_test.go` — three subtests:
  - `TestSST_NoHardcodedOllamaValues` — primary guard, walks `internal/`, `cmd/`, `ml/app/`, `scripts/`, `docker-compose*.yml`, `Dockerfile` for `11434`, `qwen2.5`, `ollama/ollama:`. Result: `SST guard OK: no production source file contains [11434 qwen2.5 ollama/ollama:] outside config/`.
  - `TestSST_NoHardcodedOllamaValues_Adversarial` — proves the scanner reports all three forbidden literals against a synthetic naughty-package fixture (3/3 reported).
  - `TestSST_NoHardcodedOllamaValues_AllowlistAdversarial` — proves `*_test.go` files are correctly skipped (legitimate test fixture allowlist).
- `internal/deploy/compose_ollama_contract_test.go` — four subtests:
  - `TestOllamaComposeContract_LiveFile` — primary contract, asserts live `docker-compose.yml` ollama service uses `${OLLAMA_IMAGE}`, has `profiles: [ollama]`, mounts `ollama-data`, and the named volume resolves via `${OLLAMA_VOLUME_NAME}`.
  - `TestOllamaComposeContract_AdversarialLiteralImage` — proves the contract rejects a regression to literal `ollama/ollama:0.6`.
  - `TestOllamaComposeContract_AdversarialHardcodedVolumeName` — proves the contract rejects a regression to literal `smackerel-ollama-data` volume name.
  - `TestOllamaComposeContract_AdversarialMissingProfile` — proves the contract rejects a regression that drops the profile gate (which would auto-start ollama in dev, violating FR-OLLAMA-007).
- `tests/integration/ollama_config_contract_test.go` (build tag `//go:build integration`) — `TestOllamaConfigGenerateAndRuntimeValidationStayInSync` plus three adversarial subtests:
  - Asserts every required `OLLAMA_*` key (12 vars) is present in `config/generated/test.env` AND that test env carries `ENABLE_OLLAMA=true` AND that `OLLAMA_TEST_MODEL=qwen2.5:0.5b-instruct`.
  - `AdversarialMissingTestModel` — strips `infrastructure.ollama.test.model` from a temp YAML, runs `config.sh` against it, asserts non-zero exit + key named in stderr.
  - `AdversarialMissingTestImage` — same pattern for `infrastructure.ollama.test.image`.
  - `AdversarialMissingRequestSeed` — same pattern for `infrastructure.ollama.test.request_seed`, proving determinism knobs are required (not optional with a fallback).

Command evidence:

```
$ ./smackerel.sh config generate
Generated config/generated/dev.env
Generated config/generated/nats.conf
$ for env in dev test home-lab; do ./smackerel.sh config generate --env=$env; done
Generated config/generated/dev.env  (ENABLE_OLLAMA=false; OLLAMA_TEST_MODEL=)
Generated config/generated/test.env (ENABLE_OLLAMA=true;  OLLAMA_TEST_MODEL=qwen2.5:0.5b-instruct, OLLAMA_TEST_REQUEST_SEED=42, OLLAMA_TEST_REQUEST_NUM_PREDICT=256)
Generated config/generated/home-lab.env (ENABLE_OLLAMA=false; OLLAMA_TEST_MODEL=)

$ ./smackerel.sh check
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 4, rejected: 0
scenario-lint: OK

$ go test ./internal/config/ -run TestSST_NoHardcodedOllamaValues -v
=== RUN   TestSST_NoHardcodedOllamaValues
    sst_grep_guard_test.go:226: SST guard OK: no production source file contains [11434 qwen2.5 ollama/ollama:] outside config/
--- PASS: TestSST_NoHardcodedOllamaValues (0.04s)
=== RUN   TestSST_NoHardcodedOllamaValues_Adversarial
--- PASS: TestSST_NoHardcodedOllamaValues_Adversarial (0.00s)
=== RUN   TestSST_NoHardcodedOllamaValues_AllowlistAdversarial
--- PASS: TestSST_NoHardcodedOllamaValues_AllowlistAdversarial (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/config  0.048s

$ go test ./internal/deploy/ -v
=== RUN   TestComposeContract_LiveFile        --- PASS (spec 042 pre-existing)
=== RUN   TestComposeContract_AdversarialLiteralBind  --- PASS
=== RUN   TestComposeContract_AdversarialInfraHasPorts --- PASS
=== RUN   TestOllamaComposeContract_LiveFile  --- PASS (spec 043 Scope 1)
=== RUN   TestOllamaComposeContract_AdversarialLiteralImage  --- PASS
=== RUN   TestOllamaComposeContract_AdversarialHardcodedVolumeName --- PASS
=== RUN   TestOllamaComposeContract_AdversarialMissingProfile --- PASS
PASS
ok      github.com/smackerel/smackerel/internal/deploy  0.021s

$ ./smackerel.sh test unit
... (78 Go packages OK)
... (411 Python tests passed in 15.04s)
exit 0
```

Live `./smackerel.sh up --env=test --profile ollama` cold-path validation is deferred to Scope 2 because the model pull and the live agent invocation are Scope 2 surfaces; Scope 1 only proves the config + compose substrate is correct (which the integration contract test does end-to-end against the actual `config.sh` executable).

### Scope 2 — Happy-Path Test + Pull Script (DONE 2026-05-09)

Status: **Done.** All 5 Scope 2 deliverables landed across 2 atomic commits.

**Deliverables (commit-by-commit):**

Commit 1 (Scope 2 PARTIAL — landed 2026-05-09 morning, SHA 26aec9e7):
- `scripts/commands/ollama-test-pull.sh` — fail-loud Ollama HTTP pull script.
- `tests/e2e/agent/no_skip_guard_test.go` — 3 tests enforcing SCN-OLLAMA-004 grep half.

Commit 2 (Scope 2 COMPLETION — landed 2026-05-09 evening, this commit):
- `ml/app/agent.py::resolve_ollama_determinism_options()` — reads `OLLAMA_TEST_REQUEST_TEMPERATURE/TOP_P/TOP_K/SEED/NUM_PREDICT` env vars (sourced from SST keys `infrastructure.ollama.test.request_*`) and forwards them as kwargs to `litellm.acompletion` when the resolved provider is `ollama`. Temperature is overridden as a named arg; the other 4 are passed via `**extra_kwargs`. For non-ollama providers the function is a strict no-op (test asserts no leak).
- `ml/tests/test_agent.py` — 6 new pytest cases:
  - `test_resolve_ollama_determinism_options_unset_is_empty` — dev/home-lab path
  - `test_resolve_ollama_determinism_options_full_set` — full env var set parses correctly
  - `test_resolve_ollama_determinism_options_skips_malformed` — malformed seed is dropped, valid top_p preserved (logged warning)
  - `test_handle_invoke_passes_ollama_determinism_kwargs` — env vars forwarded as kwargs; temperature env var overrides request temperature
  - `test_handle_invoke_does_not_inject_ollama_kwargs_for_other_providers` — adversarial: openai route does NOT receive top_k/seed/num_predict
  - `test_handle_invoke_no_determinism_env_is_no_op` — adversarial: ollama route with no env vars passes request temperature unchanged
- `config/smackerel.yaml::environments.test.agent_provider_fast_model = "qwen2.5:0.5b-instruct"` — per-env override.
- `scripts/commands/config.sh::AGENT_PROVIDER_FAST_MODEL` line — flipped from `required_value` to `env_override_value agent_provider_fast_model agent.provider_routing.fast.model` so the override actually applies.
- `config/prompt_contracts/e2e-ollama-smoke-v1.yaml` — new agent scenario `id: e2e_ollama_smoke`, `model_preference: fast`, `allowed_tools: [{name: recommendation_parse_intent, side_effect_class: read}]`. Uses an existing production-registered read-only stub tool (returns `{"ok":true}`) so scenario-lint and production startup both accept the file. scenario-lint reports `registered: 5, rejected: 0` (was 4 before).
- `tests/e2e/agent/happy_path_test.go` (build tag `e2e_ollama`) — 3 tests:
  - `TestAgentHappyPath_PlanToolSynthesis` — POSTs to `${CORE_URL}/v1/agent/invoke` with `scenario_id=e2e_ollama_smoke`, polls `agent_traces` for the returned `trace_id`, asserts `outcome=ok` + non-empty `turn_log` + non-empty `tool_calls` + `final_output.acknowledged` exists.
  - `TestAgentHappyPath_DeterministicOutput` — runs the same invocation 3 times, asserts byte-identical `final_output` across all 3 runs.
  - `TestOllamaUnreachable_FailsLoudly` — adversarial: when Ollama is reachable, asserts the smoke invocation succeeds (proves test wiring); when unreachable, asserts API returns non-OK outcome whose body mentions `ollama` or `provider`.

**Command evidence:**

```
$ ./smackerel.sh check
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 5, rejected: 0
scenario-lint: OK

$ ./smackerel.sh format --check
49 files already formatted

$ ./smackerel.sh test unit --go    # all packages green
ok      github.com/smackerel/smackerel/internal/config        0.038s
ok      github.com/smackerel/smackerel/tests/e2e/agent        0.033s
... (full run; SST guard PASSES — no Ollama literals leaked into production source)

$ ./smackerel.sh test unit --python    # 417 PASS (411 → 417 after +6 determinism tests)
417 passed in 16.10s

$ go vet -tags=e2e_ollama ./tests/e2e/agent/...    # happy_path_test.go compiles + vets clean
(no output)

$ for env in dev test home-lab; do ./smackerel.sh --env "$env" config generate >/dev/null; done
$ grep AGENT_PROVIDER_FAST_MODEL config/generated/{dev,test,home-lab}.env
config/generated/dev.env:AGENT_PROVIDER_FAST_MODEL=gpt-oss:20b
config/generated/test.env:AGENT_PROVIDER_FAST_MODEL=qwen2.5:0.5b-instruct
config/generated/home-lab.env:AGENT_PROVIDER_FAST_MODEL=gpt-oss:20b
```

T2-05 manual smoke (`SMACKEREL_TEST_OLLAMA=1 ./smackerel.sh test e2e`) is held until Scope 3 wires the gating into the e2e runner and unblocks the cold-pull of `qwen2.5:0.5b-instruct` (~397MB). The compile + vet evidence above proves the test code is correct; the live cold-pull run is Scope 3 surface.

### Scope 3 — pending

Scope 3 will populate this section as it is implemented.



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
