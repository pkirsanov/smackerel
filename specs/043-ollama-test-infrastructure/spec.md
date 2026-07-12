# Spec 043 — Ollama Test Infrastructure (close MIT-037-OLLAMA-001)

**Status:** Done (certified per state.json)

> **Successor Notice (added 2026-05-31, analyst).**
> [spec 069 — Assistant HTTP Transport](../069-assistant-http-transport/spec.md)
> re-targets the real-LLM happy-path E2E surface introduced here from
> Telegram-only test files to the assistant HTTP transport. Ollama
> orchestration, deterministic model pull, isolated test volume, and
> test-stack lifecycle stay owned by this spec. New assistant E2E
> tests should use the HTTP route so they can run in CI without a
> Telegram account while still exercising the real Go ↔ Python ↔
> Ollama loop.

## Problem Statement

Spec [037 LLM Scenario Agent & Tool Registry](../037-llm-agent-tools/spec.md)
shipped the executor loop, intent router, tool registry, allowlist enforcement,
trace persistence, and the Go ↔ Python NATS request/response contract. Every
adversarial behavior the spec promised (BS-003 allowlist, BS-004 arg-schema,
BS-005 tool-error, BS-006 hallucinated tool, BS-007 schema-retry, BS-008
loop-limit, BS-014 never-invent, BS-015 return-schema, BS-018 concurrency,
BS-019 hot-reload, BS-020 prompt-injection, BS-021 LLM timeout) is green at
unit / integration / adversarial-e2e levels against the live test stack.

What is **not** verified at HEAD `46bdd28` is the *single* path the executor
exists to drive: a real LLM, hosted by a real Ollama daemon, planning a real
tool call, returning a real answer. Today's compose layout (committed at
`46bdd28`) places `ollama` under an opt-in `profiles: [ollama]` gate in both
[`docker-compose.yml`](../../docker-compose.yml) and
[`deploy/compose.deploy.yml`](../../deploy/compose.deploy.yml). The default
`./smackerel.sh --env test up` does NOT start it; `./smackerel.sh test e2e`
does NOT auto-orchestrate it; no model is pulled into a test-isolated volume;
and there is no `tests/e2e/agent/happy_path_test.go` exercising the full
plan → tool → synthesis loop end to end.

Spec 037 Scope 5 acknowledged this as a *verification* gap, not an
implementation gap, and routed it through backlog item **MIT-037-OLLAMA-001**
(see commit `ca5f831` and
[`specs/037-llm-agent-tools/state.json`](../037-llm-agent-tools/state.json)).
The closing trigger documented there is "test-stack capacity supports an
Ollama service container and a deterministic-model pull strategy is approved
(likely a NEW SPEC for 'Ollama test infrastructure')." This spec is that new
spec. It also closes the same dependency that blocks
`tests/e2e/agent/telegram_replies_test.go` from running fully live (spec 037
Scope 9 cross-spec impact noted in MIT-037-OLLAMA-001).

This spec deliberately limits itself to the **test** boundary. It does not
choose, change, or deploy the production / self-hosted Ollama model. It does not
enable the `ollama` profile in `docker-compose.yml` for the dev stack. It
adds a deterministic, small, isolated Ollama runtime for the test stack and
exactly one canonical happy-path live-stack agent test on top of it.

## Outcome Contract

**Intent:** Close the live-Ollama verification gap routed via
MIT-037-OLLAMA-001 by giving the test stack a deterministic, isolated Ollama
runtime that `./smackerel.sh test e2e` orchestrates automatically, plus one
canonical happy-path live-stack agent test that exercises the full
plan → tool-call → synthesis loop documented in
[`docs/smackerel.md` §3.6](../../docs/smackerel.md) against that runtime.

**Success Signal:** From a clean checkout, a developer runs
`./smackerel.sh test e2e` (no extra flags). The runner brings up the test
stack, brings up an Ollama service that is part of the test compose project
only, ensures the deterministic small model `qwen2.5:0.5b-instruct` is
present in a test-isolated volume (pulling it on first run, reusing the cache
on subsequent runs), and runs `tests/e2e/agent/happy_path_test.go` against
the live agent path. The test calls a real allowlisted tool, receives the
tool result, lets the LLM synthesize a final answer, and asserts that the
recorded `agent_traces` row contains the scenario id, the tool-call sequence
with validated arguments and result, and a non-empty final output — with no
mock, no NATS-driver substitution, no scripted-driver injection, and no
bailout `t.Skip` on missing infra.

**Hard Constraints:**
- The test Ollama container, its model cache volume, and any host port it
  publishes MUST belong to the `smackerel-test` Compose project and MUST
  NOT share state with the `smackerel` (dev) or `smackerel-self-hosted`
  Compose projects.
- The model used by the happy-path test MUST be deterministic in the sense
  required by [`docs/Testing.md`](../../docs/Testing.md): the same input
  envelope produces the same tool-call sequence on a freshly pulled model
  on the same model digest, given the test's generation parameters
  (temperature, top-p, seed).
- The happy-path test MUST NOT use the scripted-driver substitution that
  Scope 5 / Scope 7 used for unit and BS-020 e2e tests. It MUST go through
  the production NATS driver and the production Python sidecar
  `handle_invoke` path.
- The happy-path test MUST fail loudly when the Ollama service or the
  declared model is unavailable. It MUST NOT contain a bailout
  `t.Skip("ollama unavailable")` or equivalent escape hatch (per the
  Adversarial Regression Tests for Bug Fixes rule in
  [`.github/copilot-instructions.md`](../../.github/copilot-instructions.md)).
- All Ollama-related configuration values (model name, base URL, pull
  policy, generation parameters) MUST originate from
  [`config/smackerel.yaml`](../../config/smackerel.yaml) and flow through
  the existing SST pipeline. Zero hardcoded model strings or hardcoded
  Ollama URLs anywhere in `internal/`, `cmd/`, `ml/`, `tests/`, or
  `scripts/`.
- The pulled model digest MUST be cached across test runs. A clean second
  invocation of `./smackerel.sh test e2e` MUST NOT re-download the model.
- Ollama in the dev compose project (`smackerel`) MUST remain opt-in via
  the existing `profiles: [ollama]` gate. This spec only changes the test
  compose project's behavior.

**Failure Condition:** If `./smackerel.sh test e2e` from a clean checkout
either (a) does not start an Ollama service in the test compose project, or
(b) does not exercise the agent loop end-to-end against that real Ollama, or
(c) silently skips when Ollama or the model is missing, the spec has failed.
If closing this spec does not close MIT-037-OLLAMA-001 and does not unblock
spec 037 Scope 9's `tests/e2e/agent/telegram_replies_test.go` from running
the same live path, the spec has failed.

## Goals

- G1: Add a test-stack-only Ollama service to the bootstrap path used by
  `./smackerel.sh test e2e`, with a deterministic small model
  (`qwen2.5:0.5b-instruct`) pulled into a test-isolated volume.
- G2: Author exactly one canonical live-stack happy-path agent test
  (`tests/e2e/agent/happy_path_test.go`) that exercises the full
  plan → tool-call → synthesis loop against the real Ollama runtime.
- G3: Close MIT-037-OLLAMA-001 and unblock spec 037 Scope 9
  `tests/e2e/agent/telegram_replies_test.go` from full live verification.
- G4: Preserve test environment isolation: no leakage between dev, test,
  and self-hosted Ollama state.
- G5: Preserve SST: every Ollama-related runtime value flows through
  `config/smackerel.yaml` and `./smackerel.sh config generate`.

## Non-Goals

- **Production / self-hosted Ollama**. This spec does NOT pick a production
  model, change the self-hosted Ollama deployment, or modify
  `deploy/compose.deploy.yml`'s Ollama profile gate.
- **Dev model selection**. The dev `gemma4:26b` (and supporting models in
  `config/smackerel.yaml` lines 50–58) stay as-is. This spec does not
  change the model the developer uses for interactive work.
- **Multi-model coverage**. Only `qwen2.5:0.5b-instruct` is pulled and used.
  Coverage of other models, vision models, OCR models, or reasoning models
  is out of scope.
- **Performance benchmarking**. The latency budget below is a *correctness*
  budget (does the test finish before the runner times out). Throughput
  benchmarking, p99 latency, or model-quality benchmarking is out of scope.
- **GPU acceleration**. The test runs on whatever the CI / dev host
  provides. CPU is sufficient for `qwen2.5:0.5b-instruct`.
- **Replacing the existing scripted-driver-based BS-020 / BS-021 tests**.
  Those tests stay (they exercise allowlist enforcement and timeout
  behavior in isolation from LLM behavior). The happy-path test in this
  spec is *additional* coverage, not a replacement.
- **Provider-agnostic happy path**. Future work may add a hosted-LLM happy
  path (OpenAI / Anthropic). This spec is Ollama-only.

## User Scenarios (Gherkin)

### SCN-OLLAMA-001 — Ollama starts deterministically on the test profile

```gherkin
Given a clean checkout at HEAD with no Ollama container running
And the dev Ollama state (volume, container) is absent
When a developer runs `./smackerel.sh test e2e`
Then the test runner brings up an Ollama service inside the
   `smackerel-test` Compose project
And that Ollama service uses a test-only volume distinct from any dev
   or self-hosted Ollama volume
And the test Ollama service becomes healthy before the agent happy-path
   test starts
```

### SCN-OLLAMA-002 — Deterministic small model present before the agent test runs

```gherkin
Given the test Ollama service is healthy
And the model `qwen2.5:0.5b-instruct` is the SST-declared test model
When the test runner is about to execute `tests/e2e/agent/happy_path_test.go`
Then the model `qwen2.5:0.5b-instruct` is present in the test Ollama
   service's model store
And the model is pulled on the first run if absent
And the pulled model is cached for subsequent runs in the test Ollama
   volume so a second invocation of `./smackerel.sh test e2e` does not
   re-download it
```

### SCN-OLLAMA-003 — Happy-path test exercises plan → tool → synthesis through real Ollama

```gherkin
Given the test stack is up with a healthy Ollama service hosting
   `qwen2.5:0.5b-instruct`
And an allowlisted read-only tool is registered in the agent tool registry
   for the happy-path scenario
When `tests/e2e/agent/happy_path_test.go` runs
Then the test sends a real intent envelope through the production NATS
   request/response contract `agent.invoke.{request,response}`
And the Python sidecar `handle_invoke` path drives Ollama via the
   provider-route resolved from SST
And Ollama emits at least one tool call against the allowlisted tool
And the executor validates the tool's input arguments against the tool's
   declared schema, executes the tool, validates the tool's return value
   against the tool's declared schema, and feeds the result back to Ollama
And Ollama produces a non-empty final synthesis
And the test asserts the recorded `agent_traces` row contains the
   scenario id, the tool-call sequence with validated arguments and
   result, and the non-empty final output
```

### SCN-OLLAMA-004 — Test fails loudly when Ollama or model is unavailable

```gherkin
Given the test runner reaches the happy-path test
When the test Ollama service is unreachable OR the declared model is
   missing from the test model store
Then the test FAILS with a clear error message identifying the missing
   precondition
And the test does NOT skip via `t.Skip` or equivalent bailout
And the test does NOT silently degrade to a scripted-driver substitution
```

### SCN-OLLAMA-005 — Per-environment Ollama storage volumes remain isolated

```gherkin
Given the developer has a running dev Ollama container with a different
   model loaded
When `./smackerel.sh test e2e` runs
Then the test Ollama container, network, and volume are all scoped to the
   `smackerel-test` Compose project
And the dev Ollama state is untouched after the test run
And tearing down the test stack does not delete the dev Ollama volume
```

### SCN-OLLAMA-006 — Configuration flows through SST, no hardcoded model strings

```gherkin
Given the model name `qwen2.5:0.5b-instruct` is the agreed test model
When a maintainer searches for that string in `internal/`, `cmd/`, `ml/`,
   `tests/`, and `scripts/`
Then the only occurrences are read-side references that resolve the value
   from a SST-derived environment variable
And `config/smackerel.yaml` is the single source declaring the test model
And `./smackerel.sh config generate` propagates the value into the
   generated `test.env` (and only `test.env` for test-scoped overrides)
```

### SCN-OLLAMA-007 — Closing this spec closes MIT-037-OLLAMA-001 and unblocks Scope 9

```gherkin
Given this spec is fully implemented and tests are green
When `bash .github/bubbles/scripts/artifact-lint.sh
   specs/037-llm-agent-tools` is run after MIT-037-OLLAMA-001 is marked
   resolved
Then no new lint findings appear that point to a residual Ollama
   verification gap on spec 037 Scope 5
And `tests/e2e/agent/telegram_replies_test.go` (spec 037 Scope 9) can be
   refactored in a follow-up PR to use the same live Ollama path without
   needing additional infrastructure work
```

## Functional Requirements

- **FR-OLLAMA-001** — `./smackerel.sh test e2e` MUST auto-start an Ollama
  service in the `smackerel-test` Compose project before running the agent
  e2e tests, without any extra CLI flag from the operator.
- **FR-OLLAMA-002** — The test Ollama service MUST host the deterministic
  small model `qwen2.5:0.5b-instruct`. The model MUST be pulled on first
  run if absent and cached for subsequent runs in a test-isolated volume.
- **FR-OLLAMA-003** — A new file `tests/e2e/agent/happy_path_test.go` MUST
  exist and exercise the full agent plan → tool-call → synthesis loop
  against the real test Ollama service through the production NATS driver
  and the production Python sidecar `handle_invoke` path.
- **FR-OLLAMA-004** — The happy-path test MUST register at least one
  allowlisted read-only tool whose handler returns a deterministic value,
  send a real intent envelope, and assert that the recorded `agent_traces`
  row contains the scenario id, the tool-call sequence with validated
  arguments and result, and a non-empty final output.
- **FR-OLLAMA-005** — The happy-path test MUST fail loudly (non-zero exit,
  clear error message naming the missing precondition) when Ollama is
  unreachable or the declared model is missing. It MUST NOT contain a
  bailout `t.Skip` or equivalent escape hatch on missing infra.
- **FR-OLLAMA-006** — All Ollama-related runtime values used by the test
  path (model name, base URL, generation parameters that govern
  determinism) MUST originate from `config/smackerel.yaml` and propagate
  via `./smackerel.sh config generate`. No hardcoded model name strings
  or hardcoded Ollama URLs in `internal/`, `cmd/`, `ml/`, `tests/`, or
  `scripts/`.
- **FR-OLLAMA-007** — The dev Compose project's Ollama service MUST
  remain opt-in (gated behind `profiles: [ollama]` in
  `docker-compose.yml`). This spec MUST NOT change the dev gate or the
  dev model selection in `config/smackerel.yaml` lines 50–58.
- **FR-OLLAMA-008** — Tearing down the test stack (`./smackerel.sh down`
  scoped to the test project, or the runner's automatic teardown after
  `./smackerel.sh test e2e`) MUST tear down the test Ollama service and
  test-only volumes without touching the dev or self-hosted Ollama volumes.
- **FR-OLLAMA-009** — Closing this spec MUST mark MIT-037-OLLAMA-001
  resolved in [`specs/037-llm-agent-tools/state.json`](../037-llm-agent-tools/state.json)
  with a cross-reference back to this spec, AND remove (or update) the
  "Done modulo deferred infra" modifier on spec 037 Scope 5 status text
  in [`specs/037-llm-agent-tools/scopes.md`](../037-llm-agent-tools/scopes.md).

## Non-Functional Requirements

- **NFR-OLLAMA-001** — Model size budget: the chosen test model
  (`qwen2.5:0.5b-instruct`) MUST be ≤ 1 GB on disk. (`qwen2.5:0.5b-instruct`
  is ~ 397 MB at Q4 quantization.)
- **NFR-OLLAMA-002** — First-run pull latency: pulling the model on a
  clean test volume MUST complete inside the existing
  `./smackerel.sh test e2e` timeout (15 min, per
  [`.github/copilot-instructions.md`](../../.github/copilot-instructions.md)
  Commands table). Subsequent cached runs MUST add ≤ 30 s of
  Ollama-related setup time to the e2e run.
- **NFR-OLLAMA-003** — Per-test latency budget: the happy-path test
  itself (excluding stack startup and model pull) MUST complete in
  ≤ 60 s on CPU on a developer-class host.
- **NFR-OLLAMA-004** — Memory ceiling: the test Ollama container MUST
  fit within the test stack's existing memory envelope. A `mem_limit`
  of 2 GB on the test Ollama service is the upper bound.
- **NFR-OLLAMA-005** — Disposable storage: the test Ollama volume MUST be
  named under the `smackerel-test-` prefix (mirroring
  `smackerel-test-postgres-data`, `smackerel-test-nats-data` in
  `config/smackerel.yaml` lines 660–665) and MUST be safe to delete
  between runs without affecting dev or self-hosted.

## Acceptance Criteria

- **AC-1** — From a clean checkout with no test Ollama state present,
  `./smackerel.sh test e2e` exits 0 and includes a passing run of
  `tests/e2e/agent/happy_path_test.go`.
- **AC-2** — A second invocation of `./smackerel.sh test e2e` immediately
  after AC-1 also exits 0 AND does not re-download the test model
  (verifiable from Ollama logs / inspecting the cached volume).
- **AC-3** — `grep -rE 'qwen2\.5:0\.5b-instruct' internal/ cmd/ ml/ tests/ scripts/`
  shows only read-side references that resolve from a SST-derived env var.
  No string-literal hardcoded test model name in business logic.
- **AC-4** — `bash .github/bubbles/scripts/artifact-lint.sh
  specs/043-ollama-test-infrastructure` exits 0.
- **AC-5** — After spec close, `specs/037-llm-agent-tools/state.json`
  carries an entry that marks MIT-037-OLLAMA-001 resolved with a
  back-reference to spec 043, AND `specs/037-llm-agent-tools/scopes.md`
  Scope 5 status text drops the "Done modulo deferred infra" modifier.
- **AC-6** — Forcing the test Ollama service down mid-run (e.g.,
  `docker stop smackerel-test-ollama`) and re-running
  `tests/e2e/agent/happy_path_test.go` produces a clear failure naming
  Ollama unreachability — not a silent skip.
- **AC-7** — `./smackerel.sh up` (dev) after a full `./smackerel.sh down`
  teardown of the test stack still does not start the dev Ollama service
  unless the dev `ENABLE_OLLAMA` flag is truthy in `config/smackerel.yaml`.

## Product Principle Alignment

> Per [`.github/instructions/product-principles.instructions.md`](../../.github/instructions/product-principles.instructions.md),
> every new feature spec touching a principle area MUST declare its
> alignment. Principles 1–10 in
> [`docs/Product-Principles.md`](../../docs/Product-Principles.md) are
> currently *Surfaced for owner approval — not yet ratified*; this section
> is therefore advisory until ratification, but the alignment is recorded
> for the future binding pass.

- **Principle 8 — Trust Through Transparency.** This spec exists to make
  the agent loop's transparency contract *demonstrably* end-to-end. Every
  scenario / tool / final output the production agent path produces is
  recorded in `agent_traces` (per spec 037 Scope 6). Today that
  transparency is verified only against scripted drivers and the BS-020
  fixture. SCN-OLLAMA-003 + AC-1 close that gap by proving the same
  trace-completeness contract holds when a real LLM is choosing the tool
  call. This is the live-stack expression of Constitution Core Principle 4
  (Explainable Synthesis) and the Model Compensations table's
  source-link / schema-validation discipline.

- **Principle 6 — Invisible By Default, Felt Not Heard.** This spec is
  pure infrastructure. It does not add user-visible features, status
  prompts, or notifications. The end-user surface area changes by zero.
  The only behavior change is that maintainers can prove agent
  end-to-end correctness from a clean checkout.

- **Principle 9 — Design For Restart, Not Perfection** (indirectly).
  Closing this verification gap reduces the friction of returning to
  agent work after a hiatus: a developer who picks up agent-related work
  weeks later can run one command and see end-to-end correctness, instead
  of having to manually stand up Ollama, pick a model, pull it, and write
  a one-off live test.

- **Constitution C3 (Live-stack tests) and C4 (No mocks in
  integration / e2e).** This spec is the canonical live-stack-tests
  expression for the agent loop. The happy-path test explicitly forbids
  scripted-driver substitution (Hard Constraint above) so it cannot
  silently degrade into a unit-class test.

## References

- **Routing origin:** Backlog item `MIT-037-OLLAMA-001` recorded in
  [`specs/037-llm-agent-tools/state.json`](../037-llm-agent-tools/state.json)
  (entry under `executionHistory.newBacklogItems`) and surfaced in
  commit `ca5f831`.
- **Closure target:** `VAL-FINDING-037-G041` — closure documented in
  [`specs/037-llm-agent-tools/report.md` § "VAL-FINDING-037-G041 Closure — 2026-05-08"](../037-llm-agent-tools/report.md).
- **Spec 037 Scope 5** (deferred infra modifier): see
  [`specs/037-llm-agent-tools/scopes.md`](../037-llm-agent-tools/scopes.md)
  Scope 5 "Status: Done (with documented deferred infra-dependent
  verification — see MIT-037-OLLAMA-001)" and the two checked-with-prefix
  DoD bullets.
- **Spec 037 Scope 9 cross-spec impact:** the same Ollama dependency
  blocks `tests/e2e/agent/telegram_replies_test.go` from full live
  verification. Closing this spec unblocks that test in a follow-up PR
  (see SCN-OLLAMA-007 + AC-5).
- **Architectural anchor:**
  [`docs/smackerel.md` §3.6 LLM Agent + Tools Pattern](../../docs/smackerel.md)
  — the production loop this spec exists to verify end-to-end.
- **Live-stack testing posture:**
  [`docs/Testing.md`](../../docs/Testing.md) and
  [`.github/copilot-instructions.md`](../../.github/copilot-instructions.md)
  "Live-Stack Test Authenticity" + "E2E And Validation Isolation" +
  "Adversarial Regression Tests for Bug Fixes".
- **Test environment isolation governance:**
  [`.github/instructions/bubbles-test-environment-isolation.instructions.md`](../../.github/instructions/bubbles-test-environment-isolation.instructions.md)
  and the `bubbles-test-environment-isolation` skill.
- **SST governance:**
  [`.github/instructions/bubbles-config-sst.instructions.md`](../../.github/instructions/bubbles-config-sst.instructions.md)
  and the `bubbles-config-sst` skill — applies to FR-OLLAMA-006 and AC-3.
- **Adjacent ratified spec:**
  [`specs/031-live-stack-testing/`](../031-live-stack-testing/) — sets
  the pattern for live-stack test orchestration this spec extends to the
  agent loop.

## Open Questions (to be resolved by `bubbles.design`)

These do not block spec acceptance — they are the design-owned decisions
the next phase will make.

- **OQ-1** — Where does the test Ollama service live? Options:
  (a) extend `docker-compose.yml`'s existing `ollama` service so the test
  Compose project (`smackerel-test`) starts it without a profile gate
  while dev still requires the gate; (b) add a separate
  `docker-compose.test.yml` overlay that contributes only the test
  Ollama service; (c) reuse the existing service and have the runner
  pass `--profile ollama` only when the env is `test`. This is a design
  decision for `bubbles.design`.
- **OQ-2** — How is the model pulled? Options: (a) one-shot
  init-container in compose that runs `ollama pull qwen2.5:0.5b-instruct`
  before the agent test phase; (b) the runner shells out to
  `docker exec smackerel-test-ollama ollama pull …` after health-check;
  (c) a tiny Python helper in `ml/` that uses Ollama's HTTP API. Design
  decision.
- **OQ-3** — Generation determinism: which exact set of generation
  parameters (temperature, top-p, top-k, seed, num_predict) does the
  happy-path test pin to make assertions stable across pulls of the
  same model digest? Design decision; goes into
  `config/smackerel.yaml` per FR-OLLAMA-006.
- **OQ-4** — Which exact tool does the happy-path scenario allowlist?
  (Likely the `scope6_e2e_echo` diagnostic tool already registered behind
  the `e2e_agent_tools` build tag in `cmd/core/agent_e2e_tools.go`, but
  this is a design call.)
- **OQ-5** — Where in the runner's e2e flow does Ollama orchestration
  hook in? Today `./smackerel.sh test e2e` already brings up the test
  stack. The Ollama bring-up + pull needs a documented hook point that
  honors test environment isolation. Design decision.
