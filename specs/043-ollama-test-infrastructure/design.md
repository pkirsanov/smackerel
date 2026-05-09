# Design 043 — Ollama Test Infrastructure (close MIT-037-OLLAMA-001)

Links: [spec.md](spec.md) | [state.json](state.json)

## Design Brief

**Current State.** [`docker-compose.yml`](../../docker-compose.yml) lines 168–189
declare an `ollama` service gated by `profiles: [ollama]`. The runtime helper
[`scripts/lib/runtime.sh`](../../scripts/lib/runtime.sh) lines 58–84 already
reads `ENABLE_OLLAMA` from the per-env generated env file and appends
`--profile ollama` to every `docker compose` invocation when truthy. SST today
exposes one global toggle (`infrastructure.ollama.enabled: false` in
[`config/smackerel.yaml`](../../config/smackerel.yaml) line 64) and one per-env
slot for `ollama_host_port` / `ollama_volume_name` (lines 651–665). The current
test stack therefore never starts Ollama, and
[`tests/e2e/agent/`](../../tests/e2e/agent/) tests universally substitute a
`scriptedDriver` for the LLM (helpers_test.go line 86, bs020_prompt_injection_test.go
header) — there is no `tests/e2e/agent/happy_path_test.go` exercising real
Ollama.

**Target State.** `./smackerel.sh test e2e` — invoked from a clean checkout
with no extra flags — brings up the existing test stack with Ollama added to
the `smackerel-test` Compose project, pulls the deterministic small model
`qwen2.5:0.5b-instruct` into a test-isolated volume on first run (cached on
subsequent runs), and runs a new `tests/e2e/agent/happy_path_test.go` that
drives the production NATS request/response contract +
`ml/app/agent.py handle_invoke` path against the live Ollama, asserts the
recorded `agent_traces` row carries the full plan → tool-call → synthesis loop,
and fails loudly (no `t.Skip`, no fallback driver) when Ollama is missing.

**Patterns to Follow.**
- Per-env override slot pattern from `qf_decisions_*` overrides
  ([`config/smackerel.yaml`](../../config/smackerel.yaml) lines 657–660;
  resolution in [`scripts/commands/config.sh`](../../scripts/commands/config.sh)
  lines 568–573 via `env_override_value`). Used here to add
  `environments.<env>.ollama_enabled` so test=`true` while dev stays `false`.
- Profile-aware compose invocation already implemented in
  [`scripts/lib/runtime.sh`](../../scripts/lib/runtime.sh) lines 58–84. Reuse,
  do not parallel.
- Live-stack test gating + skip pattern from
  [`tests/e2e/agent/helpers_test.go`](../../tests/e2e/agent/helpers_test.go)
  lines 41–80 — `liveDB(t)` + `liveNATS(t)` reach the live stack via env vars
  exported by `./smackerel.sh test e2e` (smackerel.sh lines 1112–1122). The new
  happy-path test extends this with `liveOllama(t)` that **fails** the test on
  missing infra rather than skipping (per FR-OLLAMA-005).
- Build-tag-gated diagnostic tool registration from
  [`cmd/core/agent_e2e_tools.go`](../../cmd/core/agent_e2e_tools.go) — the
  test binary is compiled with `-tags=e2e,e2e_agent_tools` so the
  `scope6_e2e_echo` tool is registered in the production-shape registry.
  Resolves OQ-4 below.
- Test-stack lifecycle owned by `./smackerel.sh test e2e` trap discipline
  (smackerel.sh lines 880–920) — no per-test compose-up; the runner brings up
  the stack once, the test asserts against it, the runner tears it down.

**Patterns to Avoid.**
- Scripted-driver substitution (`scriptedDriver` in helpers_test.go line 86;
  `scriptedRunner` in api_invoke_test.go line 31). These are the correct
  pattern for outcome-class unit/e2e coverage of executor behavior, but the
  spec **explicitly forbids** this for the happy-path test (Hard Constraint
  in spec.md: "MUST NOT use the scripted-driver substitution"). The happy-path
  test goes through the real `ml/app/agent.py handle_invoke` + litellm + Ollama
  path.
- Bailout `t.Skip("ollama unavailable")`. Existing tests skip on missing infra
  (helpers_test.go lines 47, 70). The happy-path test instead **fails** with a
  precondition error (per FR-OLLAMA-005, AC-6, and the Adversarial Regression
  Tests for Bug Fixes rule in
  [`.github/copilot-instructions.md`](../../.github/copilot-instructions.md)).
- Hardcoded literal `ollama/ollama:0.6` in
  [`docker-compose.yml`](../../docker-compose.yml) line 169. This is a small
  pre-existing SST gap that this spec MUST close because it adds a second
  image identity (test image with pinned digest) — extracting both to SST is
  cheaper than maintaining two compose files.
- Init-container model pull. Spec OQ-2 lists this as an option; it is not
  selected because (a) it forces an extra service into compose for one
  one-shot side effect, (b) compose `depends_on: condition: service_completed`
  would re-run on every `up`, and (c) the runner already has a clean hook
  point between `health_ok` and the Go-test invocation (smackerel.sh
  line 1112). The runner shells out to the Ollama HTTP `/api/pull` endpoint
  directly (per design §4 below).
- Hardcoded model strings in `internal/`, `cmd/`, `ml/`, `tests/`, `scripts/`.
  Per FR-OLLAMA-006 + AC-3, every reference resolves from a SST-derived env
  var.

**Resolved Decisions.**
- **D1.** Single compose file. Extend `docker-compose.yml` minimally
  (`image: ${OLLAMA_IMAGE}`); do NOT add `docker-compose.test.yml`. Resolves
  OQ-1.
- **D2.** Per-env `ollama_enabled` override. Add
  `environments.<env>.ollama_enabled` slot and have
  [`scripts/commands/config.sh`](../../scripts/commands/config.sh) line 366
  resolve `ENABLE_OLLAMA` via `env_override_value`. `dev` and `home-lab` stay
  `false`; `test` becomes `true`. Resolves OQ-1 + FR-OLLAMA-007.
- **D3.** Runner shells out to Ollama HTTP `/api/pull` and `/api/tags` after
  the test stack health check, before the Go E2E `docker run`. Resolves OQ-2
  + FR-OLLAMA-002 + NFR-OLLAMA-002.
- **D4.** Determinism = `temperature=0`, `top_p=1.0`, `top_k=1`, `seed=42`,
  `num_predict=256`, fixed system prompt, fixed user prompt, fixed
  `qwen2.5:0.5b-instruct` digest. Generation params live in
  `infrastructure.ollama.test.request_*` SST keys. Resolves OQ-3.
- **D5.** Happy-path scenario allowlists `scope6_e2e_echo` only. The test
  binary is built with `-tags=e2e,e2e_agent_tools`. Resolves OQ-4.
- **D6.** Hook point: between the existing `test_runtime_health.sh`
  invocation and the Go E2E `docker run` (smackerel.sh line 1112), the runner
  invokes a new `scripts/runtime/ollama-test-pull.sh` that polls
  `/api/tags`, pulls if missing, and waits for the model to appear. Resolves
  OQ-5.
- **D7.** Port reuse: the test Ollama service uses the **already-allocated**
  `environments.test.ollama_host_port: 47004` (smackerel.yaml line 662). The
  user brief mentioned `47003`, which is the existing
  `nats_monitor_host_port` for the test env (smackerel.yaml line 660) — the
  brief value would conflict; this design uses the correct existing slot.

**Open Questions** (deferred to Scope DoD, not blocking design completion).
- **OQ-D1.** Should the pinned-digest Ollama image be set in this spec
  (operator picks a sha256 at scope-execution time) or deferred to a follow-up
  hardening pass? Answer recorded in `report.md` once the operator pins it.
- **OQ-D2.** Should `infrastructure.ollama.image` (the non-test image) also
  move to a pinned digest? Out of scope for this spec — flagged for a future
  dev-Ollama pinning spec.

---

## 1. System Context

### Dev compose (`smackerel` project)

| Value                                   | Source                                                     |
|-----------------------------------------|------------------------------------------------------------|
| Compose project                         | `smackerel` (smackerel.yaml line 644)                       |
| Compose file                            | [`docker-compose.yml`](../../docker-compose.yml)            |
| Ollama service profile gate             | `profiles: [ollama]` (line 178)                             |
| `ENABLE_OLLAMA` resolution              | After D2 → `environments.dev.ollama_enabled` (= `false`)    |
| Effective behavior                      | `--profile ollama` NOT appended → ollama service NOT started |
| Dev model selection                     | `gemma4:26b` etc. (smackerel.yaml lines 54–58, unchanged)   |

This satisfies FR-OLLAMA-007: dev gate is preserved.

### Test compose (`smackerel-test` project)

| Value                                   | Source                                                          |
|-----------------------------------------|-----------------------------------------------------------------|
| Compose project                         | `smackerel-test` (smackerel.yaml line 656)                       |
| Compose file                            | [`docker-compose.yml`](../../docker-compose.yml) (same)          |
| Ollama service profile gate             | `profiles: [ollama]` (same line 178)                             |
| `ENABLE_OLLAMA` resolution              | After D2 → `environments.test.ollama_enabled` (= `true`)         |
| Effective behavior                      | `--profile ollama` appended → `smackerel-test-ollama` starts     |
| Image                                   | `infrastructure.ollama.test.image` (pinned digest) via `OLLAMA_IMAGE`|
| Host port                               | `environments.test.ollama_host_port: 47004`                      |
| Container port                          | `infrastructure.ollama.container_port: 11434`                    |
| Volume                                  | `environments.test.ollama_volume_name: smackerel-test-ollama-data`|
| Test model                              | `infrastructure.ollama.test.model: qwen2.5:0.5b-instruct`        |

This satisfies FR-OLLAMA-001 (auto-start on test profile), FR-OLLAMA-002
(deterministic small model in test-isolated volume), and FR-OLLAMA-008
(teardown disposes only test state). Same image/service definition with
SST-driven differentiation — single compose file, no parallel
`docker-compose.test.yml`.

### Home-lab compose (`smackerel-home-lab` project)

| Value                                   | Source                                                          |
|-----------------------------------------|-----------------------------------------------------------------|
| Compose project                         | `smackerel-home-lab` (smackerel.yaml line 666)                   |
| Compose file                            | [`deploy/compose.deploy.yml`](../../deploy/compose.deploy.yml)   |
| Ollama service profile gate             | `profiles: [ollama]` (unchanged by this spec)                    |
| `ENABLE_OLLAMA` resolution              | After D2 → `environments.home-lab.ollama_enabled` (= `false`)    |
| Effective behavior                      | Unchanged — operator opts in per home-lab deployment workflow    |

Out of scope for this spec — Non-Goal "Production / home-lab Ollama".

## 2. Component Diagram (Test Stack)

```
                       smackerel-test_default network (driver=bridge)
   ┌─────────────────────────────────────────────────────────────────────────────┐
   │                                                                             │
   │  ┌─────────────────┐    NATS request/response contract                      │
   │  │ smackerel-test- │    subjects: agent.invoke.{request,response}           │
   │  │     core        │   ◄─────────────────────────────────────┐              │
   │  │  (Go runtime)   │                                         │              │
   │  │  build tag:     │       ┌───────────────────┐             │              │
   │  │  e2e_agent_     │──────►│ smackerel-test-   │             │              │
   │  │  tools          │       │      nats        │◄────────────┤              │
   │  │  registers      │       │  (jetstream)     │             │              │
   │  │  scope6_e2e_    │◄──────│                  │             │              │
   │  │  echo tool      │       └───────────────────┘             │              │
   │  └────────┬────────┘                                         │              │
   │           │                                                  │              │
   │           │ writes agent_traces row                          │              │
   │           ▼                                                  │              │
   │  ┌─────────────────┐                              ┌──────────┴───────────┐  │
   │  │ smackerel-test- │                              │ smackerel-test-      │  │
   │  │   postgres      │                              │       ml             │  │
   │  │ (pgvector pg16) │                              │ (Python FastAPI +    │  │
   │  │ tmpfs volume    │                              │  ml/app/agent.py     │  │
   │  │ smackerel-test- │                              │  handle_invoke)      │  │
   │  │ postgres-data   │                              │                      │  │
   │  └─────────────────┘                              │  litellm.api_base ─┐ │  │
   │                                                   └─────────────────┬──┘ │  │
   │                                                                      │   │  │
   │                                                                      │   │  │
   │                                       OLLAMA_URL=http://ollama:11434 │   │  │
   │                                                                      ▼   │  │
   │                                                       ┌──────────────────┐│  │
   │                                                       │ smackerel-test-  ││  │
   │                                                       │     ollama       ││  │
   │                                                       │  (image pinned   ││  │
   │                                                       │   by digest)     ││  │
   │                                                       │                  ││  │
   │                                                       │ Model store      ││  │
   │                                                       │ ─────────────────││  │
   │                                                       │ qwen2.5:0.5b-    ││  │
   │                                                       │ instruct         ││  │
   │                                                       │ (~ 397 MB Q4)    ││  │
   │                                                       │                  ││  │
   │                                                       │ /root/.ollama →  ││  │
   │                                                       │ smackerel-test-  ││  │
   │                                                       │ ollama-data      ││  │
   │                                                       │ (named volume,   ││  │
   │                                                       │  test-isolated)  ││  │
   │                                                       └──────────────────┘│  │
   │                                                                           │  │
   └───────────────────────────────────────────────────────────────────────────┘  │
                                       ▲                                          │
                                       │ host port 47004 (loopback)               │
                                       │ ./smackerel.sh runner uses this for      │
                                       │ /api/tags + /api/pull during pre-test    │
                                       │ orchestration                            │
                                       └──────────────────────────────────────────┘
```

**Network:** Compose's default `<project>_default` network — for the test
project this is `smackerel-test_default`. All four services
(`postgres`, `nats`, `smackerel-core`, `smackerel-ml`, `ollama`) attach to it.
Service-to-service communication uses Compose DNS (`http://ollama:11434`,
`nats://nats:4222`).

**Volume:** `smackerel-test-ollama-data` — a named, **persistent-across-runs**
volume mounted at `/root/.ollama` inside the container. This is the explicit
design tradeoff for SCN-OLLAMA-002 (cached model across runs) and
NFR-OLLAMA-002 (subsequent cached runs add ≤ 30 s). It is named under the
`smackerel-test-` prefix per NFR-OLLAMA-005, so it is unambiguously test
property and any operator running `docker volume rm smackerel-test-ollama-data`
removes only test state. It satisfies the
[`bubbles-test-environment-isolation`](../../.github/skills/bubbles-test-environment-isolation/SKILL.md)
test-data identifiability rule (test prefix per category-scoped project) and
the Pre-Spec Hard Constraint that the model cache survives across `./smackerel.sh test e2e`
invocations.

## 3. Configuration Plan (SST Compliance)

### New SST keys (additions to `config/smackerel.yaml`)

| Key                                              | Type    | Value (proposed)                                | Justification |
|--------------------------------------------------|---------|--------------------------------------------------|---------------|
| `infrastructure.ollama.image`                    | string  | `ollama/ollama:0.6`                              | Extract literal currently in `docker-compose.yml` line 169. Default for non-test environments. SST-pure. |
| `infrastructure.ollama.test.image`               | string  | `ollama/ollama:0.4.0@sha256:<digest>` (operator-pinned at scope time; see OQ-D1) | Pinned-digest test image. Reproducibility across CI/dev hosts (FR-OLLAMA-002, NFR-OLLAMA-002). |
| `infrastructure.ollama.test.model`               | string  | `qwen2.5:0.5b-instruct`                          | Per spec.md G1 + FR-OLLAMA-002. ~ 397 MB Q4 — within NFR-OLLAMA-001 (≤ 1 GB) budget. |
| `infrastructure.ollama.test.pull_timeout_seconds`| int     | `300`                                            | Cold-cache pull budget (FR-OLLAMA-002, NFR-OLLAMA-002). 5 min < 15 min `./smackerel.sh test e2e` ceiling. |
| `infrastructure.ollama.test.request_temperature` | float   | `0.0`                                            | Determinism (FR-OLLAMA-006, OQ-3 resolution D4). |
| `infrastructure.ollama.test.request_top_p`       | float   | `1.0`                                            | Determinism (D4). |
| `infrastructure.ollama.test.request_top_k`       | int     | `1`                                              | Determinism (D4). |
| `infrastructure.ollama.test.request_seed`        | int     | `42`                                             | Determinism (D4). Same input + same seed + same digest → same output. |
| `infrastructure.ollama.test.request_num_predict` | int     | `256`                                            | Determinism + bounded latency (D4, NFR-OLLAMA-003). |
| `environments.dev.ollama_enabled`                | bool    | `false`                                          | Per-env override (D2). Preserves dev's opt-in behavior (FR-OLLAMA-007). |
| `environments.test.ollama_enabled`               | bool    | `true`                                           | Per-env override (D2). Auto-starts Ollama in test compose (FR-OLLAMA-001). |
| `environments.home-lab.ollama_enabled`           | bool    | `false`                                          | Per-env override (D2). Preserves opt-in for home-lab. |

### Existing SST keys reused (NO change)

| Key                                             | Value (existing)                              | Why reused |
|-------------------------------------------------|------------------------------------------------|------------|
| `infrastructure.ollama.container_port`          | `11434` (smackerel.yaml line 65)               | Same Ollama daemon port across all envs. |
| `environments.test.ollama_host_port`            | `47004` (smackerel.yaml line 662)              | Already SST-allocated for the test compose project. **Brief mentioned `47003` — that slot is already `nats_monitor_host_port` for test (line 660); using it would collide. This design uses the correct existing slot.** |
| `environments.test.ollama_volume_name`          | `smackerel-test-ollama-data` (line 665)        | Already test-isolated, already SST. |
| `environments.test.compose_project`             | `smackerel-test` (line 656)                    | Existing project name. |

### Generator changes (`scripts/commands/config.sh`)

| Existing line                                                         | Change |
|------------------------------------------------------------------------|--------|
| Line 366: `OLLAMA_ENABLED="$(required_value infrastructure.ollama.enabled)"` | Replace with `OLLAMA_ENABLED="$(env_override_value ollama_enabled infrastructure.ollama.enabled)"` so per-env value wins. |
| Line 367 (no change): `OLLAMA_CONTAINER_PORT`                          | Reused as-is. |
| Line 558 (no change): `OLLAMA_HOST_PORT` from per-env                  | Reused as-is. |
| Line 561 (no change): `OLLAMA_VOLUME_NAME` from per-env                | Reused as-is. |
| **NEW** before line 794 in the `OUTPUT_FILE` heredoc                   | Resolve `OLLAMA_IMAGE`: when `$TARGET_ENV == test` → `infrastructure.ollama.test.image`; else → `infrastructure.ollama.image`. Emit `OLLAMA_IMAGE=<value>` line. |
| **NEW** for env=test only                                              | Emit `OLLAMA_TEST_MODEL`, `OLLAMA_TEST_PULL_TIMEOUT_SECONDS`, `OLLAMA_TEST_REQUEST_TEMPERATURE`, `OLLAMA_TEST_REQUEST_TOP_P`, `OLLAMA_TEST_REQUEST_TOP_K`, `OLLAMA_TEST_REQUEST_SEED`, `OLLAMA_TEST_REQUEST_NUM_PREDICT`. (For dev/home-lab these keys are absent — they are test-only.) |

### Compose changes (`docker-compose.yml`)

| Line(s) | Change |
|---------|--------|
| 169     | `image: ollama/ollama:0.6` → `image: ${OLLAMA_IMAGE}` |
| 168–189 | No structural change. Profile gate, ports, volume, healthcheck, deploy block all unchanged. |

### Existing source-side reads (NO change)

[`ml/app/agent.py`](../../ml/app/agent.py) line 211 already reads
`os.environ.get("OLLAMA_URL")` to set `litellm.api_base`. The agent provider
routing in `config/smackerel.yaml` lines 477–489 maps scenario
`model_preference` → `(provider, model)`. The happy-path test resolves the
model via the SST → env → Python sidecar chain — no new code paths in
`internal/`, `cmd/`, or `ml/` need to learn about the test model directly.

### SST audit grep (the test in AC-3)

```bash
# Must show ONLY one occurrence — in config/smackerel.yaml — outside the new
# scripts/runtime/ollama-test-pull.sh which reads it from env.
grep -rE 'qwen2\.5:0\.5b-instruct' internal/ cmd/ ml/ tests/ scripts/
```

Expected: zero occurrences in `internal/`, `cmd/`, `ml/`, `tests/`. The single
read site is `scripts/runtime/ollama-test-pull.sh` reading
`$OLLAMA_TEST_MODEL` from the env file already resolved by the runner.

## 4. Test-Stack Lifecycle

`./smackerel.sh test e2e` already owns a stable lifecycle (smackerel.sh
lines 661–1142). This spec adds exactly one new step inside the existing
Go E2E block, between `test_runtime_health.sh` and the Go `docker run`.

```
./smackerel.sh test e2e
  │
  ├─ smackerel_acquire_e2e_suite_lock test         (smackerel.sh line 770)
  │
  ├─ trap e2e_cleanup_trap EXIT                    (smackerel.sh line 884)
  │
  ├─ Shell E2E block (lifecycle + shared scripts)  (smackerel.sh lines 974–1107)
  │
  └─ Go E2E block                                   (smackerel.sh lines 1108–1142)
        │
        ├─ smackerel_generate_config test           (regenerates env, includes new OLLAMA_TEST_*)
        │
        ├─ test_runtime_health.sh                   (brings up test stack incl. ollama via --profile ollama
        │                                            because ENABLE_OLLAMA=true after D2)
        │
        ├─ ★ NEW: scripts/runtime/ollama-test-pull.sh   ← inserted here per D6
        │     │
        │     ├─ Resolve OLLAMA_HOST_PORT, OLLAMA_TEST_MODEL,
        │     │   OLLAMA_TEST_PULL_TIMEOUT_SECONDS from $env_file
        │     │
        │     ├─ Probe http://127.0.0.1:$OLLAMA_HOST_PORT/api/tags
        │     │   ├─ unreachable after retry budget → exit non-zero (fail loudly)
        │     │   └─ reachable → continue
        │     │
        │     ├─ If $OLLAMA_TEST_MODEL listed in /api/tags response → SKIP pull (warm cache)
        │     │
        │     ├─ Else POST /api/pull {"name":"$OLLAMA_TEST_MODEL"}
        │     │   ├─ Stream NDJSON progress lines (full output, no truncation per
        │     │   │   .github/instructions/terminal-discipline.instructions.md)
        │     │   ├─ Bounded by $OLLAMA_TEST_PULL_TIMEOUT_SECONDS
        │     │   └─ Final {"status":"success"} → continue; otherwise exit non-zero
        │     │
        │     └─ Re-probe /api/tags to confirm model is now listed → exit 0
        │
        ├─ docker run … golang:1.25.10-bookworm bash scripts/runtime/go-e2e.sh
        │     │ (test container has CORE_EXTERNAL_URL, DATABASE_URL, NATS_URL,
        │     │  OLLAMA_HOST_PORT, OLLAMA_TEST_* in its env via -e flags;
        │     │  go-e2e.sh runs `go test -tags e2e,e2e_agent_tools …` so the
        │     │  scope6_e2e_echo tool is registered for happy_path_test.go)
        │     │
        │     └─ tests/e2e/agent/happy_path_test.go runs (per §5)
        │
        └─ EXIT trap → e2e_cleanup → e2e_down_test_stack
              │
              └─ smackerel_compose test down --timeout 30 --remove-orphans
                  (NOTE: e2e_down_test_stack passes --volumes per smackerel.sh
                   line 893; this DOES delete smackerel-test-ollama-data on
                   teardown. To preserve cross-run cache, this spec changes
                   the test-cleanup contract in smackerel.sh: see §4.1 below.)
```

### 4.1 Cleanup contract change

Today `e2e_down_test_stack` calls `./smackerel.sh --env test down --volumes`
(smackerel.sh line 893). With `--volumes`, `docker compose down` removes named
volumes — including `smackerel-test-ollama-data`. That defeats the warm-cache
contract (SCN-OLLAMA-002 + NFR-OLLAMA-002).

**Decision:** This spec preserves the existing teardown semantics for
`postgres-data` and `nats-data` (which MUST be ephemeral per
[`bubbles-test-environment-isolation`](../../.github/skills/bubbles-test-environment-isolation/SKILL.md))
and adds a labeled exemption for the Ollama model-cache volume. Two
implementation options surface in scopes:

- **Option A (preferred):** Replace `--volumes` with explicit
  `docker volume rm smackerel-test-postgres-data smackerel-test-nats-data` so
  `smackerel-test-ollama-data` is not auto-removed. This makes the keep-list
  explicit.
- **Option B:** Add an env flag `SMACKEREL_TEST_KEEP_OLLAMA_VOLUME=1` (default
  on for `test e2e`, off for explicit `clean full`). Adds a knob; less clean.

The DoD picks Option A. The scope MUST also add a `./smackerel.sh clean full`
path that DOES delete `smackerel-test-ollama-data` so a developer can force a
cold-cache run for re-pull testing (AC-2 evidence), per
[`bubbles-docker-lifecycle-governance`](../../.github/skills/bubbles-docker-lifecycle-governance/SKILL.md)
"prove freshness" guidance.

## 5. Happy-Path Test Anatomy (`tests/e2e/agent/happy_path_test.go`)

### 5.1 Build tags

```go
//go:build e2e && e2e_agent_tools
```

Justification: The `e2e_agent_tools` tag is the existing mechanism for
registering the `scope6_e2e_echo` diagnostic tool into the production-shape
agent registry (cmd/core/agent_e2e_tools.go). The runner already passes both
tags to `go-e2e.sh` (this spec's scope changes go-e2e.sh to add
`-tags e2e,e2e_agent_tools` instead of just `-tags e2e`).

### 5.2 Preconditions (live, fail-loud — FR-OLLAMA-005, AC-6)

```
liveDB(t)        — existing helper; skips if DATABASE_URL unset (kept as-is for
                   the e2e_agent_tools build outside the runner; the runner
                   ALWAYS exports DATABASE_URL so the runner-driven path always
                   passes)
liveNATS(t)      — existing helper; same posture
liveOllama(t)    — NEW helper, fails (not skips) when:
                   (a) OLLAMA_HOST_PORT env var is unset/empty
                   (b) OLLAMA_TEST_MODEL env var is unset/empty
                   (c) GET http://127.0.0.1:${OLLAMA_HOST_PORT}/api/tags returns
                       non-200 within a 5 s timeout
                   (d) the listed models do not include OLLAMA_TEST_MODEL
                   t.Fatal() with a precondition message naming the exact
                   missing piece (no t.Skip — this is the spec's hard line).
```

### 5.3 Test wiring

```
1. Register happy-path scenario in temp scenario dir, allowed_tools = ["scope6_e2e_echo"]
   (real scenario YAML loaded by the production loader — same shape as
   replay tests in tests/e2e/agent/replay_pass_test.go).

2. Build IntentEnvelope with:
     scenario_id        = "happy_path_e2e_echo"
     trace_id           = uuid
     model_preference   = ""   (resolves to provider_routing.default = ollama/gemma4:26b)
     temperature        = $OLLAMA_TEST_REQUEST_TEMPERATURE (= 0)
     seed               = $OLLAMA_TEST_REQUEST_SEED       (= 42)

   ★ The model_preference for THIS test overrides default to test model:
     since provider_routing.default.model is gemma4:26b (the dev model), the
     test happy-path scenario YAML declares model_preference = "test" and the
     scope plan adds a NEW provider_routing entry:
       provider_routing.test.provider = ollama
       provider_routing.test.model    = $OLLAMA_TEST_MODEL (qwen2.5:0.5b-instruct)
     This keeps dev provider routing untouched while routing the happy-path
     scenario to the test model. SST-pure.

3. Publish via the production NATS subject `agent.invoke.request` to the
   live test NATS (NATS_URL exported by the runner).

4. Subscribe to `agent.invoke.response.<trace_id>` with a bounded deadline
   (NFR-OLLAMA-003: ≤ 60 s).

5. ml/app/nats_client.py picks up the request, dispatches to
   ml/app/agent.py handle_invoke, which:
     - resolves provider_route → (ollama, qwen2.5:0.5b-instruct)
     - sets litellm.api_base = http://ollama:11434 (compose DNS)
     - calls litellm.acompletion with tools=[scope6_e2e_echo schema]
     - LLM responds with a tool_call to scope6_e2e_echo with arguments
       matching its declared input schema {"q": "<some echo string>"}
     - executor (Go core) validates args, invokes the tool's handler,
       echoes back the args
     - second LLM turn synthesizes a final response containing the echo
6. Test asserts:
     a. response envelope.outcome == "ok"
     b. response.final is non-empty string
     c. agent_traces row exists in postgres for trace_id
        SELECT scenario_id, tool_calls, final_output FROM agent_traces
        WHERE trace_id = $1
        — scenario_id == "happy_path_e2e_echo"
        — len(tool_calls) >= 1 with name == "scope6_e2e_echo"
        — tool_calls[0].arguments validates against the tool's input schema
        — tool_calls[0].result validates against the tool's output schema
        — final_output non-empty

7. Cleanup: t.Cleanup() removes the temp scenario YAML; the agent_traces
   row is left for the next test isolation cycle (the postgres volume is
   ephemeral per §4.1 Option A so the row goes away on stack teardown).
```

### 5.4 Hard prohibitions in this test (per spec.md Hard Constraints)

- **No `scriptedDriver` / `scriptedRunner`.** Search the file source for
  `scripted` — must return zero matches inside `happy_path_test.go`.
- **No `t.Skip`.** Search the file source for `t.Skip` — must return zero
  matches.
- **No `httptest.NewServer`.** This is not an in-process HTTP test; the path
  is NATS request/response against a live sidecar.
- **No mocking of `internal/agent` interfaces.** The executor under test is
  the real, production-wired `agent.Runner`.

## 6. Failure Modes (SCN-OLLAMA-004 + AC-6)

| Trigger                                                                    | Detected by                                                       | Surface                                                                                              |
|----------------------------------------------------------------------------|--------------------------------------------------------------------|------------------------------------------------------------------------------------------------------|
| Ollama service down (`docker stop smackerel-test-ollama`)                  | `liveOllama(t)` — `/api/tags` HTTP timeout                         | `t.Fatal("happy_path_test: ollama unreachable at http://127.0.0.1:%d (got %v)", OLLAMA_HOST_PORT, err)` |
| Model not present in cache + pull fails                                    | `scripts/runtime/ollama-test-pull.sh` exit 1                        | `./smackerel.sh test e2e` exits non-zero **before** the Go test runs; clear log line names the model |
| Pull exceeds `OLLAMA_TEST_PULL_TIMEOUT_SECONDS`                            | bash `timeout` wrapper around the curl POST                         | Same — runner-level fail-loud                                                                        |
| Model present but agent loop yields no tool call (LLM hallucinates final)  | Test assertion `len(tool_calls) >= 1`                              | `t.Fatalf("happy_path_test: agent_traces.tool_calls = %d, want ≥ 1", n)`                            |
| Tool call argument validation fails                                        | Existing executor returns `outcome=arg-validation-error`            | Test assertion `outcome == "ok"` fails                                                              |
| Tool result schema validation fails                                        | Existing executor returns `outcome=tool-return-validation-error`    | Test assertion `outcome == "ok"` fails                                                              |
| `OLLAMA_HOST_PORT` env var missing in the test container                   | `liveOllama(t)`                                                     | `t.Fatal("happy_path_test: OLLAMA_HOST_PORT env unset — runner contract broken")`                   |
| `OLLAMA_TEST_MODEL` env var missing in the test container                  | `liveOllama(t)`                                                     | `t.Fatal("happy_path_test: OLLAMA_TEST_MODEL env unset — runner contract broken")`                  |

### 6.1 Adversarial regression test (AC-6)

A second test in the same file:

```go
func TestHappyPath_FailsLoudWhenOllamaUnavailable(t *testing.T) {
    // Adversarial: temporarily corrupts OLLAMA_HOST_PORT in the test process
    // env, then asserts that a fresh liveOllama(t) call WOULD fail-loud.
    // Implementation: spawn liveOllama in a sub-test with t.Setenv("OLLAMA_HOST_PORT", "1")
    // and capture that t.Fatal was invoked (using a fake testing.TB recorder).
    // This proves the test would catch a regression where someone replaces
    // t.Fatal with t.Skip.
}
```

This satisfies the
[`.github/copilot-instructions.md`](../../.github/copilot-instructions.md)
"Adversarial Regression Tests for Bug Fixes" rule even though this spec is
forward-looking — the regression target is "future authors must not soften
the live-Ollama precondition into a t.Skip."

## 7. Performance Budget

| Phase                                              | Budget                                | Source                                  |
|----------------------------------------------------|----------------------------------------|------------------------------------------|
| Test stack startup (postgres + nats + core + ml + ollama) | ≤ 60 s wall-clock                | Existing test e2e budget (300 s shell timeout, smackerel.sh line 1112) — adds ~ 5–10 s for ollama healthcheck |
| Cold-cache model pull (`qwen2.5:0.5b-instruct` first run) | ≤ 60 s on a 100 Mb/s connection (~ 397 MB Q4 = ~ 32 s @ 100 Mb/s + decode) | NFR-OLLAMA-002 ("inside the existing 15 min timeout") + design tightening |
| Warm-cache model probe (cached run)                | ≤ 2 s wall-clock (single GET /api/tags) | NFR-OLLAMA-002 ("≤ 30 s of Ollama-related setup time") — design tightens to ≤ 2 s for the probe alone |
| `tests/e2e/agent/happy_path_test.go` body          | ≤ 60 s wall-clock on CPU              | NFR-OLLAMA-003                          |
| Total `./smackerel.sh test e2e` cold path Δ        | ≤ 90 s additional wall-clock vs. today | Sum of stack startup Δ + cold pull + test body |
| Total `./smackerel.sh test e2e` warm path Δ        | ≤ 30 s additional wall-clock vs. today | Sum of stack startup Δ + warm probe + test body |

The 15-min `./smackerel.sh test e2e` ceiling per
[`.github/copilot-instructions.md`](../../.github/copilot-instructions.md)
Commands table has ample headroom.

## 8. Test Isolation (per `bubbles-test-environment-isolation` skill)

| Isolation contract                                        | Mechanism                                                                                            |
|-----------------------------------------------------------|------------------------------------------------------------------------------------------------------|
| Compose project distinct from dev/home-lab                | `environments.test.compose_project: smackerel-test` (existing).                                       |
| Container name carries `-test-` infix                     | Compose default `<project>-<service>` → `smackerel-test-ollama`.                                      |
| Network namespace distinct                                 | Compose default `smackerel-test_default`.                                                              |
| Host port distinct from dev                               | `47004` (test) vs. `42004` (dev) — both inside the project's 40k–47k Smackerel block.                 |
| Volume name carries `-test-` prefix                       | `smackerel-test-ollama-data` (existing per smackerel.yaml line 665).                                  |
| Postgres + NATS volumes ephemeral per-run                 | `e2e_down_test_stack` explicitly removes `smackerel-test-postgres-data` and `smackerel-test-nats-data` (per §4.1 Option A). |
| Ollama model-cache volume preserved across runs           | Excluded from the explicit-rm list in §4.1 Option A. Justified by the warm-cache contract; sized at ≤ 1 GB per NFR-OLLAMA-001. |
| `./smackerel.sh clean full` path nukes everything         | New scope DoD ensures `clean full` removes the model-cache volume too — operator escape hatch.        |
| Dev Ollama state untouched by `./smackerel.sh test e2e`   | The runner only invokes `smackerel_compose test`; smackerel_compose for `test` resolves COMPOSE_PROJECT=smackerel-test, so all volume/container/network operations are project-scoped (SCN-OLLAMA-005). |

The model-cache volume is the **sole intentional exception** to the
"ephemeral by default" rule in
[`bubbles-test-environment-isolation`](../../.github/skills/bubbles-test-environment-isolation/SKILL.md).
Per the SKILL's "Test Data Identifiability" requirement (test prefix +
project-scoped name) the exception is contained: any operator can identify
and remove it deterministically. The exception is documented inline in
`config/smackerel.yaml` next to the new SST keys so future agents reading
the SKILL won't be surprised.

## 9. SST Compliance (per `bubbles-config-sst` skill)

| SST contract                                                            | Mechanism                                                                       |
|-------------------------------------------------------------------------|---------------------------------------------------------------------------------|
| Single source of truth for every value                                  | All new keys land in `config/smackerel.yaml` (§3 table).                         |
| Generated env files produced by the generator only                      | `scripts/commands/config.sh` extended (§3); generated files in `config/generated/*.env` carry the auto-generated header. |
| Fail-loud on missing required keys                                      | New keys use `required_value` (or `env_override_value` for the per-env override). |
| Zero hardcoded values in source                                         | `OLLAMA_IMAGE` replaces literal `ollama/ollama:0.6` in compose. Test model name appears nowhere in `internal/`, `cmd/`, `ml/`, `tests/` — only in `config/smackerel.yaml` (definition) and `scripts/runtime/ollama-test-pull.sh` (consumption from env). |
| Zero silent fallbacks                                                   | Per-env override resolved via `env_override_value` (which still calls `required_value` as a base). No `||` defaults. |
| Empty-string placeholders for secrets                                   | N/A — Ollama uses no API key. |
| Generated files in DO-NOT-EDIT manifest                                 | `config/generated/*.env` already in copilot-instructions.md DO-NOT-EDIT table. |
| Test contract (AC-3) enforces the rule mechanically                     | Documented `grep -rE 'qwen2\.5:0\.5b-instruct' internal/ cmd/ ml/ tests/ scripts/` returns zero hits except `scripts/runtime/ollama-test-pull.sh`. |

## 10. Risks & Mitigations

| Risk                                                                                                       | Likelihood | Impact | Mitigation |
|------------------------------------------------------------------------------------------------------------|-----------:|-------:|------------|
| **Disk usage:** model cache adds ~ 397 MB to disk per dev/CI host                                          | Cert        | Low    | NFR-OLLAMA-001 caps model size at 1 GB. `./smackerel.sh clean full` removes the volume. |
| **CI bandwidth:** cold-cache pull on every CI worker downloads ~ 397 MB                                    | Med         | Med    | The pull happens once per worker per cache eviction. CI hosts are typically warm-cached after first run. Documented in the scope's CI-tuning DoD bullet. |
| **Determinism flakiness:** small models can drift even with seed=42 if Ollama internals change           | Low         | High (test flakes) | Pin Ollama image by digest (`infrastructure.ollama.test.image`). Pin model digest implicitly via the named tag (`qwen2.5:0.5b-instruct` is a stable Ollama-published reference). Scope DoD includes a 50-run repeat to baseline observed flakiness. |
| **Tool-call fidelity:** `qwen2.5:0.5b` is small; it may produce malformed tool calls                    | Med         | Med    | The agent executor's existing schema-validation loop (BS-007) retries malformed tool calls within `schema_retry_budget`. The happy-path scenario's tool schema is intentionally trivial (`{"q": string}`) to maximize the chance of correct first-call generation. |
| **Pull endpoint streams indefinitely** if network stalls                                                   | Low         | Med    | `OLLAMA_TEST_PULL_TIMEOUT_SECONDS=300` wrapped in `timeout`. Bounded fail-loud per §6. |
| **Volume name collision** if two `./smackerel.sh test e2e` runs execute concurrently on the same host    | Low         | Low    | `smackerel_acquire_e2e_suite_lock test` (smackerel.sh line 770) already serializes test runs. Existing infrastructure. |
| **Operator deletes the cache volume by mistake**                                                          | Low         | Low    | First subsequent run re-pulls in ≤ 60 s (NFR-OLLAMA-002). No data loss — the volume is a derived artifact. |
| **`profile: ollama` accidentally removed from `docker-compose.yml`** for non-test envs                   | Low         | High (dev would auto-start ollama) | The per-env `ollama_enabled` override still gates the `--profile ollama` injection in `smackerel_compose`. The compose profile is defense-in-depth; removing it would still leave `ENABLE_OLLAMA=false` for dev preventing the profile from being added. Documented as redundancy. |

## 11. Rollout Plan

This spec is closed in three phases, each with its own scope:

### Phase 1 — Configuration + Compose (low risk)

- Add the new SST keys (§3 first table) to `config/smackerel.yaml` with the
  inline comment block documenting the test-only intent.
- Update `scripts/commands/config.sh` to:
  - Replace `OLLAMA_ENABLED` resolution with `env_override_value`.
  - Resolve `OLLAMA_IMAGE` per env in the heredoc.
  - Emit `OLLAMA_TEST_*` lines for env=test only.
- Replace the `ollama/ollama:0.6` literal in `docker-compose.yml` with
  `${OLLAMA_IMAGE}`.
- `./smackerel.sh config generate` regenerates dev/test/home-lab env files;
  diff check confirms only the expected new keys appear.
- `./smackerel.sh test unit` (Go + Python) passes — no source changes.
- `./smackerel.sh up` (dev) still does NOT start Ollama (FR-OLLAMA-007 sanity
  check).
- `./smackerel.sh --env test up` DOES now start Ollama (`docker compose ps`
  shows `smackerel-test-ollama` Up).

### Phase 2 — Happy-path test + pull script (medium risk)

- Add `scripts/runtime/ollama-test-pull.sh` per §4 D6/D7.
- Add `tests/e2e/agent/happy_path_test.go` per §5.
- Update `scripts/runtime/go-e2e.sh` line 37 to use
  `-tags e2e,e2e_agent_tools`.
- Update `smackerel.sh` Go E2E block to:
  - Pass `OLLAMA_HOST_PORT`, `OLLAMA_TEST_MODEL`, `OLLAMA_TEST_*` via `-e`
    flags to the `golang:1.25.10-bookworm` `docker run`.
  - Insert the `ollama-test-pull.sh` invocation between `test_runtime_health.sh`
    and the Go `docker run`.
- Replace `e2e_down_test_stack` `--volumes` with explicit
  `docker volume rm smackerel-test-postgres-data smackerel-test-nats-data` per
  §4.1 Option A.
- Run `./smackerel.sh test e2e` from a clean state — full path passes (AC-1).
- Run `./smackerel.sh test e2e` again — second run is faster, no model
  re-pull (AC-2).

### Phase 3 — Cross-spec MIT closure + cleanup contract (low risk)

- Update `specs/037-llm-agent-tools/state.json` `executionHistory` with an
  entry marking `MIT-037-OLLAMA-001` resolved, back-referencing spec 043
  (FR-OLLAMA-009 + AC-5).
- Update `specs/037-llm-agent-tools/scopes.md` Scope 5 status text — drop the
  "Done modulo deferred infra" modifier (FR-OLLAMA-009 + AC-5).
- Update `./smackerel.sh clean full` path so it DOES remove
  `smackerel-test-ollama-data` (operator escape hatch per §8).
- Add `bash .github/bubbles/scripts/artifact-lint.sh
  specs/043-ollama-test-infrastructure` to the spec-author checklist; assert
  exit 0 (AC-4).

Phases are sequential. Phase 2 cannot start until Phase 1 is green; Phase 3
follows Phase 2's AC-1 evidence.

## 12. Open Questions (deferred to scope DoD)

- **OQ-D1 (refines spec OQ-3 + new):** Operator picks the exact Ollama
  image digest (`ollama/ollama:0.4.0@sha256:<digest>`) at scope-execution
  time. Recommended source: latest stable `0.4.x` Ollama release at the time
  of scope execution, with the digest captured from `docker buildx imagetools
  inspect` and pinned in `infrastructure.ollama.test.image`. Recorded in
  `report.md` as evidence.
- **OQ-D2 (new, deferred to a future spec):** Should
  `infrastructure.ollama.image` (the dev image) also move to a pinned
  digest, mirroring the test image discipline? Out of scope here; flagged
  for a future "Pin dev runtime images" hardening spec.
- **OQ-D3 (refines spec OQ-3):** If the 50-run determinism baseline (§10
  Risks row 3) reveals tool-call drift > 0% even with seed=42, scope DoD
  picks the next mitigation: (a) tighten generation params further,
  (b) replace `qwen2.5:0.5b-instruct` with a different ≤ 1 GB model with
  better tool-calling stability, or (c) accept a documented retry-on-flake
  budget. Decision deferred to scope evidence.
- **OQ-D4 (refines spec OQ-5):** Does the
  `scripts/runtime/ollama-test-pull.sh` step need a `set -x`-style
  verbose-mode flag for CI debugging, or is the default NDJSON stream
  output (full, unfiltered per terminal-discipline) sufficient? Default to
  the NDJSON stream; reconsider if CI runs surface unhelpful pull logs.

## References

- Spec: [spec.md](spec.md) — SCN-OLLAMA-001..007, FR-OLLAMA-001..009,
  NFR-OLLAMA-001..005, AC-1..7, OQ-1..5.
- State: [state.json](state.json).
- Adjacent design: [`specs/037-llm-agent-tools/`](../037-llm-agent-tools/) —
  the executor + tool registry under verification.
- Adjacent design: [`specs/031-live-stack-testing/`](../031-live-stack-testing/) —
  the live-stack orchestration pattern this spec extends.
- Skill: [`bubbles-test-environment-isolation`](../../.github/skills/bubbles-test-environment-isolation/SKILL.md).
- Skill: [`bubbles-config-sst`](../../.github/skills/bubbles-config-sst/SKILL.md).
- Skill: [`bubbles-docker-port-standards`](../../.github/skills/bubbles-docker-port-standards/SKILL.md).
- Skill: [`bubbles-docker-lifecycle-governance`](../../.github/skills/bubbles-docker-lifecycle-governance/SKILL.md).
- Instruction: [`.github/copilot-instructions.md`](../../.github/copilot-instructions.md) —
  Live-Stack Test Authenticity, E2E And Validation Isolation, Adversarial
  Regression Tests for Bug Fixes.
- Architecture: [`docs/smackerel.md` §3.6](../../docs/smackerel.md) — LLM
  Agent + Tools pattern, the loop this spec exists to verify end-to-end.
