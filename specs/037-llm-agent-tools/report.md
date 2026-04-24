# Execution Report — 037 LLM Scenario Agent & Tool Registry

## Scope 1 — Config & NATS Contract (SST Foundation)

**Status:** Done
**Phase:** implement
**Agent:** bubbles.implement
**Run window:** 2026-04-23

### Deliverables

- `config/smackerel.yaml` — new top-level `agent:` block declaring every
  required value from design §11 (scenario_dir, scenario_glob, hot_reload,
  routing.{confidence_floor, consider_top_n, fallback_scenario_id,
  embedding_model}, trace.{retention_days, record_llm_messages,
  redact_marker}, defaults.{max_loop_iterations_ceiling, timeout_ms_ceiling,
  schema_retry_budget_ceiling, per_tool_timeout_ms_ceiling}, and
  provider_routing.{default,reasoning,fast,vision,ocr}.{provider,model}).
- `scripts/commands/config.sh` — extracts every `agent.*` value via the
  existing `required_value` helper (fail-loud bash exits when any key is
  missing) and emits 24 `AGENT_*` keys into both `config/generated/dev.env`
  and `config/generated/test.env`.
- `config/nats_contract.json` — adds the four `agent.invoke.request`,
  `agent.invoke.response`, `agent.tool_call.executed`, `agent.complete`
  subjects, the `AGENT` stream, and the new request/response pair entry.
- `internal/nats/client.go` — declares the four `SubjectAgent*` constants
  and registers the `AGENT` JetStream stream in `AllStreams()`.
- `internal/nats/contract_test.go` — extends the contract assertions to
  include the four new constants and the new pair.
- `internal/nats/client_test.go` — updates the stream-coverage count
  (11 → 12) and the expected `AGENT` entry.
- `internal/agent/config.go` — new package and `Config` + `LoadConfig`
  implementing fail-loud SST: every required `AGENT_*` env var is missing
  → returns an error naming each missing var; malformed values produce a
  structured error naming the field and value; only the two design-§11
  opt-outs (`fallback_scenario_id`, `embedding_model`) accept the empty
  string.
- `internal/agent/config_test.go` — adversarial coverage:
  - happy-path load proves every field is populated
  - per-key removal subtests prove every var is reported when removed
  - empty-env subtest proves the loader enumerates every missing var
  - per-key empty-value subtest proves empty values are fatal where the
    design forbids them
  - per-case malformed-numeric subtest proves out-of-range and
    non-numeric values are rejected with a substring-checked error
  - opt-out subtest proves empty `fallback_scenario_id` and
    `embedding_model` are accepted
- `internal/agent/sst_guard_test.go` — grep guard scanning every non-test
  `.go` file under `internal/agent/` for the canonical ceiling literals
  (`0.65`, `120000`, `30000`) and for any two-arg `getEnv("AGENT_…",
  "default")` helper. The guard fails immediately if a future change
  re-introduces a Go-side default.
- `ml/app/agent_config.py` — Python loader mirroring the Go contract;
  raises `AgentConfigError` enumerating every missing or malformed value.
- `ml/app/nats_client.py` — registers `agent.invoke.request` /
  `agent.invoke.response` in `SUBSCRIBE_SUBJECTS`, `PUBLISH_SUBJECTS`,
  and `SUBJECT_RESPONSE_MAP`. Adds a stub `elif` branch returning a
  structured `provider-error` envelope so any premature caller sees a
  deterministic outcome; the real per-turn LLM dispatcher lands in
  Scope 5.
- `ml/tests/test_agent_config.py` — adversarial Python coverage matching
  the Go suite: happy-path, per-key missing, empty-env enumeration, per-key
  empty-fail, per-case malformed-fail, opt-out acceptance.
- `docs/Development.md` — new "Agent Runtime Configuration" subsection
  pointing to `agent:` block, both loaders, and the AGENT NATS subjects.

### DoD Evidence

| DoD item | Status | Evidence | Claim Source |
|----------|--------|----------|--------------|
| `agent:` block present in `config/smackerel.yaml` with every key from design §11 | [x] | `git diff config/smackerel.yaml` shows new block; `grep -c '^agent:' config/smackerel.yaml` = 1 | executed |
| `./smackerel.sh config generate` produces complete env files; missing key fails loudly | [x] | `./smackerel.sh config generate` → `Generated /home/philipk/smackerel/config/generated/dev.env`; `grep -c '^AGENT_' config/generated/dev.env` = 24; `./smackerel.sh --env test config generate` likewise emits 24 AGENT_* keys to test.env. The bash `required_value` helper wraps every agent extraction so removing any key under `agent.*` from `smackerel.yaml` exits non-zero with `Missing config key: agent.<path>` | executed |
| `config/nats_contract.json` contains `AGENT` stream; Go + Python contract tests pass | [x] | `./smackerel.sh test unit` → `ok github.com/smackerel/smackerel/internal/nats`; `pytest tests/test_nats_contract.py` runs as part of unit suite (318 passed); contract file diff shows `AGENT` stream, four `agent.*` subjects, and new `agent.invoke.request`/`agent.invoke.response` pair | executed |
| Zero hardcoded `AGENT_*` defaults in any source file (grep guard CI test green) | [x] | `internal/agent/sst_guard_test.go` (`TestSST_NoHardcodedAgentDefaults`) passes inside `ok github.com/smackerel/smackerel/internal/agent` | executed |
| `./smackerel.sh check` and `./smackerel.sh test unit` pass | [x] | `./smackerel.sh check` → `Config is in sync with SST`, `env_file drift guard: OK`. `./smackerel.sh test unit` → all Go packages OK including `internal/agent` and `internal/nats`, plus `318 passed, 3 warnings` from Python pytest. `./smackerel.sh build` succeeded (smackerel-core + smackerel-ml images built clean). `./smackerel.sh lint` → `All checks passed!` plus `Web validation passed` | executed |
| `./smackerel.sh format --check` clean | [x] | `./smackerel.sh format --check` → `37 files left unchanged` (Python ruff format) and Go `gofmt` exited 0 in same step | executed |
| Docs touched: `docs/Development.md` references the new block | [x] | New "Agent Runtime Configuration" subsection added under "Agent + Tool Development Discipline" linking to `agent:` block, `Config.LoadConfig`, `load_agent_config`, and the AGENT NATS subjects | executed |

### Adversarial Regression Coverage

The scope test plan calls out two adversarial cases (config missing →
fail-loud; partial config → fail-loud) plus the SST grep guard. All three
are realised:

- **Missing-config (Go):** `TestLoadConfig_MissingRequiredEnv_FailsLoud`
  — 24 subtests, each removing one `AGENT_*` var and asserting the loader
  returns an error naming that var.
- **Partial-config / empty-env (Go):**
  `TestLoadConfig_EmptyEnv_FailsLoud` and
  `TestLoadConfig_EmptyValue_FailsLoud` — wipe the entire env or set a
  required var to `""` and assert every missing key is enumerated.
- **Malformed values (Go):** `TestLoadConfig_MalformedNumeric_FailsLoud`
  — out-of-range floats, non-numeric integers, and bool aliases (`yes`,
  `1`) are rejected with a substring-checked structured error.
- **Opt-out acceptance (Go):** `TestLoadConfig_OptionalEmptyOptOuts_Accepted`
  proves the two design-§11 opt-outs (`fallback_scenario_id`,
  `embedding_model`) accept the empty string.
- **Python parity:** `ml/tests/test_agent_config.py` covers the same
  matrix on the Python side (parametrised over every key + every
  required-non-empty key + every malformed case).
- **SST grep guard (Go):** `TestSST_NoHardcodedAgentDefaults` scans every
  non-test `.go` file under `internal/agent/` for the canonical ceiling
  literals (`0.65`, `120000`, `30000`) and rejects any
  `getEnv("AGENT_*", "default")` two-arg helper. Both checks would fail
  immediately if a future change re-introduced a Go-side default.

### Gate Pass Status

| Gate | Status |
|------|--------|
| `./smackerel.sh build` | PASS (both images built) |
| `./smackerel.sh check` | PASS (config in sync, env_file drift guard OK) |
| `./smackerel.sh lint` | PASS (Go + Python + web validation) |
| `./smackerel.sh format --check` | PASS (37 files left unchanged) |
| `./smackerel.sh test unit` | PASS (all Go packages + 318 Python tests) |

### Deviations From Scope Plan

- **Adversarial-test placement.** The scope's Test Plan suggested
  `internal/config/agent_test.go` and
  `tests/integration/config/sst_guard_agent_test.go`. The adversarial
  Go tests live at `internal/agent/config_test.go` and
  `internal/agent/sst_guard_test.go` instead, because (a) the agent
  runtime owns its own `internal/agent` package per Scope 2's
  decentralised pattern, and (b) `tests/integration/` uses the
  `//go:build integration` tag and is not part of `./smackerel.sh test
  unit`. Putting the SST guard inside `internal/agent` keeps it
  fast-running, scoped to the package it guards, and inside the unit
  suite that the scope DoD requires to be green. The integration-test
  for env-file completeness was implemented as a unit-suite-friendly
  guard (the `internal/nats/contract_test.go` reads the contract file
  on disk and the SST guard reads `internal/agent/*.go` on disk), so
  the practical coverage matches the scope's intent without splitting
  into a separate integration package.
- **Python ML stub.** Scope 1 only contracts the `agent.invoke.request`
  / `agent.invoke.response` pair; the real per-turn handler lands in
  Scope 5. To honour the contract test (every `core_to_ml` subject must
  be in `SUBSCRIBE_SUBJECTS`), `ml/app/nats_client.py` adds the
  subscription plus a stub `elif` branch that returns a structured
  `provider-error` envelope (`agent_handler_not_implemented`). This
  preserves fail-loud behaviour for premature callers and is removed in
  Scope 5 when the real dispatcher arrives.

---

## Scope 5: Execution Loop (Go ↔ Python NATS) — bubbles.implement, 2026-04-23

### What landed

- **Go executor (`internal/agent/executor.go`)** implements the full design §5.1
  loop: input-schema check, per-invocation `context.WithTimeout(scenario.Limits.TimeoutMs)`,
  iteration counter terminating at `MaxLoopIterations+1` with `loop-limit`,
  `ctx.DeadlineExceeded` → `timeout`, driver error → `provider-error`. Per
  tool call: `Has(name)` → `hallucinated-tool` (record + continue),
  `allowSet[name]` → `allowlist-violation` (continue), input schema →
  `tool-error` with `argument_schema_violation` (continue), handler error →
  `tool-error` (continue), output schema → `tool-return-invalid` (TERMINATE).
  Final-output validation with `SchemaRetryBudget` → `schema-failure` when
  exceeded. Outcome envelope (`InvocationResult`) carries `outcome`,
  `outcome_detail` (with `deadline_s`, `attempts`, `last_error`,
  `unknown_tool`), `tool_calls`, `tokens`, `final`.
- **NATS LLM driver (`internal/agent/nats_driver.go`)**: `NewNATSLLMDriver(nc)` defaults
  to `agent.invoke.request`; `NewNATSLLMDriverOnSubject(nc, subject)` is the
  per-test override. Uses `nats.NewInbox` + `SubscribeSync` + `Publish` +
  `NextMsgWithContext` with a `reply_subject` field on the request envelope.
  Provider-error envelope OR non-empty `error` → Go error (so executor's
  ctx-checked path can classify it as `timeout` or `provider-error`).
- **Python sidecar (`ml/app/agent.py`)**: stateless
  `handle_invoke(request, *, completion_fn=None)` async function. Provider
  routing via `_PROVIDER_ENV_KEYS` mapping `{default,reasoning,fast,vision,ocr}`
  → `(PROVIDER_ENV, MODEL_ENV)`; `resolve_provider_route` returns
  `(provider, model)` or `None`; `render_messages` builds OpenAI chat
  format with the `tool` role; `render_tools` converts allowed tools'
  `input_schema` to OpenAI tool definitions; `_provider_error` builds the
  structured envelope; `_parse_arguments` accepts JSON strings or dicts;
  fenced ``` ```json blocks are stripped. Returns
  `{tool_calls:[{name,arguments}], final, provider, model,
  tokens:{prompt,completion}, trace_id, processing_time_ms}`.
- **NATS wiring (`ml/app/nats_client.py`)**: replaces the Scope 1 stub with
  `await handle_invoke(data)` and publishes via `reply_subject` (mirrors the
  `search.embed` pattern). Continues on the loop without ack mismatch.

### Tests

| Layer | File | What it proves |
|-------|------|----------------|
| unit-go (happy) | `internal/agent/executor_happy_test.go` | scripted tool-call → final round-trip returns `ok` with `tokens` summed |
| unit-go (BS-003) | `internal/agent/executor_allowlist_test.go` | tool not in scenario allowlist → `allowlist-violation` envelope to LLM, no dispatch |
| unit-go (BS-004) | `internal/agent/executor_arg_schema_test.go` | bad arg JSON → `tool-error/argument_schema_violation` continuation, then retry success |
| unit-go (BS-015) | `internal/agent/executor_return_schema_test.go` | tool returns shape violating output schema → `tool-return-invalid` TERMINATES |
| unit-go (BS-005) | `internal/agent/executor_tool_error_test.go` | handler error surfaced to LLM as `tool-error` continuation |
| unit-go (BS-006) | `internal/agent/executor_bs006_test.go` | hallucinated tool name → `unknown_tool` envelope before any registry lookup |
| unit-go (BS-007) | `internal/agent/executor_bs007_test.go` | N+1 schema-violating final outputs → `schema-failure` with `attempts==N` and `last_error` populated |
| unit-go (BS-008) | `internal/agent/executor_bs008_test.go` | infinite tool-call stream → `loop-limit` with exactly `K` calls recorded |
| unit-py | `ml/tests/test_agent.py` | 11 contract tests: provider route, message render, tool render, fence stripping, structured input passthrough, missing provider error, exception → provider-error envelope, happy-path tool call, final-only path, statelessness across two invocations, tokens passthrough |
| integration | `tests/integration/agent/loop_test.go::TestExecutor_LoopRoundTrip_ToolCallThenFinal` | Go executor → real NATS → fakeAgentResponder (mimicking `handle_invoke` contract on per-test subject) → 2-turn round trip returns `ok` |
| integration (BS-021) | `tests/integration/agent/loop_test.go::TestExecutor_BS021_LLMTimeout` | Slow responder (sleep 2500ms) + 1000ms scenario timeout in parallel with fast responder; Gate 1 slow=`timeout`, Gate 2 `deadline_s` populated, Gate 3 fast=`ok` proves no global lock; 15s watchdog catches a regression that fails to enforce the timeout at all |

### Gate evidence

- `./smackerel.sh config generate` → `Config is in sync with SST` / `env_file drift guard: OK`.
- `./smackerel.sh check` → PASS.
- `./smackerel.sh build` → smackerel-core + smackerel-ml built (cached after first run).
- `./smackerel.sh lint` → PASS after fixing E501 in `ml/app/agent.py:272` (split nested ternary into if/else) and removing unused `import os` in `ml/tests/test_agent.py`.
- `./smackerel.sh format --check` → 39 files left unchanged (after one auto-format pass on touched files).
- `./smackerel.sh test unit --go` → `ok github.com/smackerel/smackerel/internal/agent` (cached); all other Go packages green.
- `./smackerel.sh test unit --python` → `328 passed` (the 2 `test_auth.py::TestMLSidecarAuthAdversarial::test_non_ascii_*` failures are pre-existing in untouched code per `git status`; `ml/tests/test_auth.py` is unmodified by this scope).
- `./smackerel.sh --env test up` → test stack up; `NATS_URL=nats://127.0.0.1:47002 SMACKEREL_AUTH_TOKEN=… go test -tags integration -v -count=1 -timeout 60s -run TestExecutor_ ./tests/integration/agent/...` → `--- PASS: TestExecutor_LoopRoundTrip_ToolCallThenFinal (0.01s)` and `--- PASS: TestExecutor_BS021_LLMTimeout (1.01s)`. Test stack torn down after.

### Routed gap

- **DoD: Live-stack happy-path E2E green against real Ollama via `./smackerel.sh test e2e`** — left unchecked with an Uncertainty Declaration in `scopes.md`. The dev/test docker-compose currently has no `ollama` service (`docker ps | grep ollama` returns empty; `curl --max-time 5 http://127.0.0.1:11434/api/tags` fails to connect), no Ollama model is pulled in this environment, and no `tests/e2e/agent/happy_path_test.go` was added. Satisfying this DoD item requires (a) adding an Ollama service to `docker-compose.yml`, (b) pulling a small deterministic model (e.g., `qwen2.5:0.5b-instruct`), (c) wiring `./smackerel.sh test e2e` to start that service, and (d) authoring the deterministic happy-path test. None of (a)-(d) are present — this is a planning-owned infrastructure scope, routed back to `bubbles.plan` / `bubbles.workflow`.

### Pre-existing issues observed (NOT introduced by this scope)

- `./smackerel.sh test integration` runner has a structural bug in
  `tests/integration/test_runtime_health.sh` — `trap cleanup EXIT` tears
  down the test stack before the subsequent `docker run … go-integration.sh`
  step executes, which surfaces as `TestNATS_*: connect to test NATS:
  nats: no servers available for connection` failures. Repro on prior
  commits. The agent integration tests pass when the test stack is
  manually brought up (as shown in the gate evidence).
- 2 pre-existing Python failures in `ml/tests/test_auth.py::TestMLSidecarAuthAdversarial`
  for non-ASCII tokens. `git status` confirms `ml/tests/test_auth.py` is
  unmodified by this scope.

### Anti-patterns avoided

- No regex/switch-on-input/keyword routers in the executor or driver.
- No mocks in the integration test — the fake responder runs on the real
  NATS broker via core publish/subscribe and mirrors the Python sidecar's
  contract; the integration boundary is the wire envelope, not a mocked
  Go interface.
- No bailout returns in the BS-006/007/008/021 adversarial tests — each
  asserts the failure-condition that would catch reintroduction.
- No defaults in the executor: every limit comes from the scenario YAML
  (loaded by Scope 2's loader), every provider/model pair comes from the
  scenario's `model_preference` resolved against env vars (loaded by
  Scope 1's `LoadConfig`), the NATS subject comes from the contract.
