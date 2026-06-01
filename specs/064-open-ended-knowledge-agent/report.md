# Report — Spec 064 (Open-Ended Knowledge Agent)

> Initial. Evidence is appended by downstream agents as scopes execute.
> Anti-fabrication policy (G021) applies: every block below cites only
> commands actually run or files actually present in the working tree at
> the time of capture.

## Summary

Implementation phase in progress. SCOPE-01 (artifact bootstrap) was completed in
a prior session. SCOPE-02 through SCOPE-08 were implemented in the current
session, producing the tool registry foundation, the SST config block, the
LLM tool-use bridge (Go client + Python sidecar contract + roundtrip test),
the deterministic `unit_convert` and `calculator` tools, the
`internal_retrieval` tool, the `WebSearchProvider` interface with the SearxNG
implementation plus Brave/Tavily `ErrProviderNotConfigured` stubs, and the
mechanical cite-back verifier with adversarial coverage.

Scopes 02-08 are **Implemented Pending Validation**: code and tests exist on
disk, the canonical test invocations succeeded in-session, but the spec has
not yet been routed through `bubbles.validate` / `bubbles.audit` /
`bubbles.chaos`. Scopes 09-18 are Not Started.

Pre-existing issues encountered and resolved in this session:

- **Architecture violation:** the LLM client was initially placed at
  `internal/assistant/openknowledge/client.go` next to the registry. Moved
  to `internal/assistant/openknowledge/llm/client.go` so the registry
  foundation does not depend on a concrete LLM transport.
- **Premature scenario contract:** a scenario-manifest YAML referencing
  not-yet-registered tools was authored during SCOPE-04 planning notes and
  removed. The contract is correctly deferred to SCOPE-12 per `scopes.md`.

## Test Evidence

Per-scope evidence blocks below cite the canonical test command and the
file inventory verified at capture time. Output transcripts are not pasted
verbatim because they exceeded the terminal scrollback at the time of
report assembly; the command + exit code is the auditable record.

### SCOPE-01 — Artifact bootstrap

**Status:** Implemented Pending Validation

**Files (verified via `find internal/assistant/openknowledge -type f` and
`ls specs/064-open-ended-knowledge-agent/`):**

- `specs/064-open-ended-knowledge-agent/spec.md` — 276 lines
- `specs/064-open-ended-knowledge-agent/design.md` — 593 lines
- `specs/064-open-ended-knowledge-agent/scopes.md` — 421 lines
- `specs/064-open-ended-knowledge-agent/state.json` — 90 lines
- `internal/assistant/openknowledge/doc.go` — 15 lines
- `internal/assistant/openknowledge/tool.go` — 129 lines
- `internal/assistant/openknowledge/registry.go` — 106 lines
- `internal/assistant/openknowledge/registry_test.go` — 148 lines

**Test command:**

```bash
SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test unit --go --go-run TestRegistry
```

Exit code: 0 (claim source: executed in this session).

### SCOPE-02 — Tool registry skeleton + Tool interface

**Status:** Implemented Pending Validation

**Files:**

- `internal/assistant/openknowledge/tool.go` — 129 lines (Tool interface,
  typed sentinels `ErrUnknownTool`, `ErrDuplicateTool`, `ErrToolNotAllowed`)
- `internal/assistant/openknowledge/registry.go` — 106 lines
- `internal/assistant/openknowledge/registry_test.go` — 148 lines (covers
  register / duplicate / allowlist allow / allowlist deny / unknown /
  deterministic ordering / nil allowlist denies all)

**Test command:**

```bash
SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test unit --go --go-run TestRegistry
```

Exit code: 0 (claim source: executed in this session).

Lint: `./smackerel.sh lint` not re-run for this scope individually; will be
re-run at the validation pass.

### SCOPE-03 — SST config block `assistant.open_knowledge.*`

**Status:** Implemented Pending Validation

**Files:**

- `internal/config/openknowledge.go` — 220 lines (fail-loud struct,
  validation for all keys listed in scopes.md DoD)
- `internal/config/openknowledge_test.go` — 288 lines (missing key fatal,
  empty allowlist fatal, budgets > 0 / >= 0, provider enum)
- `config/smackerel.yaml` — `assistant.open_knowledge.*` block added
- `config/generated/dev.env`, `config/generated/test.env` — regenerated
  via `./smackerel.sh config generate`

**Test command:**

```bash
SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test unit --go --go-run TestOpenKnowledgeConfig
```

Exit code: 0 (claim source: executed in this session).

Note: SCOPE-03 source files (`internal/config/openknowledge.go` + test,
`config/smackerel.yaml` block, `scripts/commands/config.sh` additions,
regenerated env files) were authored in a prior run as recorded in
`state.json.certification.notes`; this session re-verified them and they
remain consistent with the SCOPE-03 DoD.

### SCOPE-04 — LLM bridge tool-use round-trip

**Status:** Implemented Pending Validation

**Files:**

- `ml/app/schemas.py` — 162 lines (`tools[]`, `tool_call`, `tool_result`
  Pydantic models)
- `ml/app/routes/chat.py` — created in this session (route accepting
  tools/messages, returning `stop_reason=tool_use` or final text)
- `ml/tests/test_tool_roundtrip.py` — 167 lines (sidecar contract +
  parity assertions)
- `internal/assistant/openknowledge/llm/client.go` — 229 lines (typed
  `ToolCall` return path)
- `internal/assistant/openknowledge/llm/client_test.go` — 189 lines
  (mocked HTTP, happy path + tool-use branch)
- `internal/assistant/openknowledge/llm/testdata/chat_fixture.json` —
  fixture for parity test

**Resolution of pre-existing issue:** initial draft placed `client.go` at
`internal/assistant/openknowledge/client.go`, which violated the
capability-foundation boundary (the registry would have depended on a
concrete LLM transport). Moved into `llm/` subpackage so the registry
remains transport-agnostic.

**Test commands:**

```bash
SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test unit --go --go-run TestOpenKnowledgeLLM
SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test unit --py -k test_tool_roundtrip
```

Exit code: 0 for both (claim source: executed in this session).

Premature scenario contract (referencing tools not yet registered) was
removed; contract creation correctly deferred to SCOPE-12.

### SCOPE-05 — Deterministic tools: `unit_convert`, `calculator`

**Status:** Implemented Pending Validation

**Files:**

- `internal/assistant/openknowledge/tools/unit_convert.go` — 215 lines
- `internal/assistant/openknowledge/tools/unit_convert_test.go` — 145 lines
  (table-driven correctness, malformed args, unknown unit)
- `internal/assistant/openknowledge/tools/calculator.go` — 427 lines
- `internal/assistant/openknowledge/tools/calculator_test.go` — 126 lines
  (divide-by-zero, NaN, malformed expression)
- `internal/assistant/openknowledge/tools/registry_integration_test.go` —
  85 lines (registry → tool invoke end-to-end)

**Test command:**

```bash
SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test unit --go --go-run TestOpenKnowledgeTools
```

Exit code: 0 (claim source: executed in this session).

### SCOPE-06 — `internal_retrieval` tool

**Status:** Implemented Pending Validation

**Files:**

- `internal/assistant/openknowledge/tools/internal_retrieval.go` —
  231 lines
- `internal/assistant/openknowledge/tools/internal_retrieval_test.go` —
  168 lines (mocked graph client)
- `tests/integration/openknowledge_internal_retrieval_test.go` — 96 lines
  (live test Postgres via `./smackerel.sh test integration`, ephemeral DB)

**Test commands:**

```bash
SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test unit --go --go-run TestOpenKnowledge
./smackerel.sh test integration --go-run TestOpenKnowledgeInternalRetrieval
```

Exit code: 0 for both (claim source: executed in this session).

### SCOPE-07 — Web search provider interface + SearxNG impl

**Status:** Implemented Pending Validation

**Files:**

- `internal/assistant/openknowledge/web/provider.go` — 87 lines
  (`WebSearchProvider`, `WebSnippet`, `ErrProviderNotConfigured`)
- `internal/assistant/openknowledge/web/provider_test.go` — 81 lines
- `internal/assistant/openknowledge/web/searxng.go` — 181 lines
- `internal/assistant/openknowledge/web/searxng_test.go` — 230 lines
  (HTTP mocked, egress restricted to configured endpoint)
- `internal/assistant/openknowledge/web/brave.go` — 23 lines (stub returns
  `ErrProviderNotConfigured`)
- `internal/assistant/openknowledge/web/tavily.go` — 19 lines (same stub
  contract)
- `tests/integration/openknowledge_searxng_test.go` — 72 lines (real
  SearxNG container in test compose)

**Test commands:**

```bash
SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test unit --go --go-run TestOpenKnowledgeWeb
./smackerel.sh test integration --go-run TestOpenKnowledgeSearxNG
```

Exit code: 0 for both (claim source: executed in this session).

### SCOPE-08 — Cite-back verifier

**Status:** Implemented Pending Validation

**Files:**

- `internal/assistant/openknowledge/citeback/doc.go` — 60 lines
- `internal/assistant/openknowledge/citeback/verifier.go` — 377 lines
- `internal/assistant/openknowledge/citeback/verifier_test.go` — 308 lines
  (adversarial: fabricated URL not in tool trace → rejection; hash
  mismatch → rejection; partial citation → rejection; ≥ 3 adversarial
  cases per DoD)

**Test command:**

```bash
SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test unit --go --go-run TestCitebackVerifier
```

Exit code: 0 (claim source: executed in this session).

### SCOPE-09 through SCOPE-18

**Status:** SCOPE-12 in progress (this session); SCOPE-09–11, 13–18 Not Started.

### SCOPE-12 — Live wiring + facade source-assembler + fallback flip

**Status:** Implemented Pending Validation (this session)

**Findings closed (from prior implement run):**

- **F4 — openknowledge subsystem not wired into cmd/core.** Closed: new
  `cmd/core/wiring_assistant_openknowledge.go` (218 lines) constructs
  the LLM client, web provider, GraphSearcher, registry, agent, and
  installs it via `agenttool.SetAgent`. Wired into `cmd/core/main.go`
  right after `wireAssistantSkillServices`. No-op when
  `assistant.open_knowledge.enabled=false`; fails loud when enabled
  but any required dep is missing (G028).
- **F5 — facade source-assembler missing for open_knowledge.** Closed:
  new `cmd/core/wiring_assistant_openknowledge_assembler.go` (208
  lines) parses the substrate Handler envelope and maps each
  `sources[]` entry into the matching `contracts.Source{Kind, Ref}`
  per PKT-061-A taxonomy. Registered in `buildAssistantSourceAssemblers`
  (`wiring_assistant_facade.go`).
- **F6 — `agent.routing.fallback_scenario_id` was `""`.** Closed:
  flipped to `"open_knowledge"` in `config/smackerel.yaml` and
  regenerated `config/generated/{dev,test}.env` via
  `./smackerel.sh config generate`. Verified:
  `grep AGENT_ROUTING_FALLBACK_SCENARIO_ID config/generated/dev.env`
  → `open_knowledge`; same for `test.env`.
- **F7 — integration test for routing.** Code closed:
  `tests/integration/agent/openknowledge_routing_test.go` (~180 lines)
  with three sub-cases plus a scenario-health probe. Live-stack
  execution: started `./smackerel.sh test integration --go-run
  TestOpenKnowledgeRouting` in this session but the live stack did not
  reach `healthy` for `smackerel-test-smackerel-core-1` within the
  session window (ML sidecar was reporting `unhealthy` after 2+ min);
  the test command was left running. Routing-layer behaviour itself
  is exercised by `agent.NewRouter` + `Route` calls in the test
  driver, which is the SCOPE-12-owned surface. End-to-end
  POST `/assistant` coverage (SCN-064-A01..A08) is owned by SCOPE-17
  per scopes.md; SCOPE-12's contract is the routing-layer fallback
  flip, not the full LLM tool loop.

**Files (this session):**

- `cmd/core/wiring_assistant_openknowledge.go` — 218 lines (new)
- `cmd/core/wiring_assistant_openknowledge_assembler.go` — 208 lines (new)
- `cmd/core/wiring_assistant_openknowledge_test.go` — 245 lines (new;
  12 unit tests covering wire construction guards,
  `agent_system_prompt` loader, and envelope→Source mapping incl.
  adversarial malformed-source dropout per G021)
- `cmd/core/wiring_assistant_facade.go` — 1-line `+open_knowledge`
  entry in `buildAssistantSourceAssemblers`
- `cmd/core/main.go` — 7-line `wireOpenKnowledge` call insertion
- `config/smackerel.yaml` — `fallback_scenario_id: "open_knowledge"`;
  `tool_allowlist` populated with the v1 policy snapshot tools;
  `llm_timeout_ms: 30000` added (new REQUIRED SST key)
- `config/prompt_contracts/open_knowledge.yaml` — `agent_system_prompt`
  top-level field added (full `<CITATIONS>` protocol prompt for the
  open-knowledge AGENT loop, distinct from the substrate planner's
  `system_prompt` block)
- `internal/config/openknowledge.go` — `LLMTimeoutMs` field +
  `lookupInt` + Validate range check
- `internal/config/openknowledge_test.go` — fixtures updated for new
  env var
- `internal/config/validate_test.go` — `setRequiredEnv` fixture
  updated for new env var
- `scripts/commands/config.sh` — `ASSISTANT_OPEN_KNOWLEDGE_LLM_TIMEOUT_MS`
  read + emit
- `tests/integration/agent/openknowledge_routing_test.go` — 192 lines
  (new; build tag `integration`, package `agent_integration`)

**Test commands executed in this session:**

```bash
./smackerel.sh config generate                         # exit 0 (executed)
./smackerel.sh --env test config generate              # exit 0 (executed)
go build ./...                                          # exit 0 (executed; clean)
go test ./cmd/core/... -run 'TestWireOpenKnowledge|TestLoadOpenKnowledgeAgentPrompt|TestOpenKnowledgeAssembler' -count=1
                                                       # exit 0 (executed; all 12 pass)
SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test unit --go
                                                       # exit 0 (executed; full unit suite green)
./smackerel.sh lint                                    # exit 0 (executed)
go build -tags=integration ./tests/integration/agent/...
                                                       # exit 0 (executed; integration test compiles)
go vet -tags=integration ./tests/integration/agent/... # exit 0 (executed)
./smackerel.sh test integration --go-run TestOpenKnowledgeRouting
                                                       # NOT COMPLETED in session — stack
                                                       # startup did not reach healthy core
                                                       # within available window. Test code
                                                       # itself builds + vets clean.
```

**Follow-ups routed (open findings):**

- **CostFn is zero-stub.** The agent's `CostFn` installed by wiring
  returns 0 USD per token. Token caps + iteration caps still bind,
  but the per-query USD budget is not exercised by LLM round-trips
  until a provider-priced rate table is added to
  `OpenKnowledgeConfig`. Route packet to a future SCOPE adding
  `assistant.open_knowledge.llm_usd_per_1k_tokens` (and/or per-model
  rates) + a CostFn that multiplies. Until then, USD budget
  enforcement is documented as "not yet engaged" — not silently
  defaulted (G028 honesty).
- **GraphSearcher is text-similarity (`PgxGraphSearcher`).** SCOPE-06
  already records this finding; SCOPE-12 inherits the same adapter.
  An embedding-backed adapter that re-uses the SearchEngine NATS/ML
  pipeline is a follow-up scope.
- **Live integration run.** The full
  `TestOpenKnowledgeRouting_FallbackToOpenKnowledge` execution against
  the live test compose was started but did not complete. To unblock
  SCOPE-12 final closure, the next session should: (a) ensure the
  test stack reaches `healthy` (specifically
  `smackerel-test-smackerel-core-1` and ML sidecar
  `/embed` reachability), (b) run
  `./smackerel.sh test integration --go-run TestOpenKnowledgeRouting`,
  (c) paste the per-case decision log lines into this report under
  "Routing test evidence", and (d) flip the SCOPE-12 DoD checkbox.
- **SCOPE-17 unblocked.** Now that the live wiring is in place and the
  fallback fires, the full SCN-064-A01..A08 end-to-end coverage (live
  Ollama tool loop + SearxNG + citation paths) can proceed under
  SCOPE-17 ownership.

**Test command:**

```bash
./smackerel.sh test integration --go-run TestOpenKnowledgeRouting
```

Exit code: NOT CAPTURED in this session (see above).

#### Routing test evidence (post-PKT-064-SCOPE12-A YAML application — BLOCKED on foreign config-emission gap)

**Packet:** PKT-064-SCOPE12-A (see
`specs/064-open-ended-knowledge-agent/route-packets/PKT-064-SCOPE12-A-RESPONSE.md`).

**§3.A — YAML diff applied:** `config/prompt_contracts/weather-query-v1.yaml`
`intent_examples` extended additively per the packet. The original
four entries are preserved verbatim (lines 7-10); 15 adversarial
coverage variations are appended (lines 12-26). Verified by
`git diff config/prompt_contracts/weather-query-v1.yaml`:

```text
+# Existing — keep verbatim, do not remove:
 - "weather in Seattle today"
 - "is it going to rain in Reykjavík tomorrow?"
 - "what's the forecast for Portland this weekend?"
 - "temperature in London right now"
+# Added by PKT-064-SCOPE12-A — adversarial-coverage variations:
+- "weather in Paris today"
+- "weather in Tokyo today"
+- "weather in New York today"
+- "weather in Berlin tomorrow"
+- "what's the weather in San Francisco"
+- "what's the weather like in Madrid today"
+- "weather today in Chicago"
+- "current weather in Dublin"
+- "how's the weather in Lisbon today"
+- "weather forecast for Amsterdam today"
+- "is it raining in Vancouver right now"
+- "how hot is it in Phoenix today"
+- "what's the temperature in Oslo today"
+- "weather in 90210"
+- "forecast in 10115 tomorrow"
```

**Claim Source:** executed (string-replace edit in IDE; verified by
`grep -c '^- ' config/prompt_contracts/weather-query-v1.yaml` over
the `intent_examples` block — 19 entries, matching 4 originals + 15
additions). No other field touched. No SST default altered. No
router/test/scenario change.

**§3.B — Live integration re-run: BLOCKED.** The packet requires:

```bash
./smackerel.sh test integration --go-run '^TestOpenKnowledgeRouting_FallbackToOpenKnowledge$'
```

This command FAILED at the config-generation gate (before any test
container started) with the following captured output from
`/tmp/scope12-pkt-a.log`:

```text
ERROR: [F074-SST-MISSING] missing or invalid required capture_as_fallback configuration: \
  CAPTURE_AS_FALLBACK_DEDUP_WINDOW (env var not set), \
  CAPTURE_AS_FALLBACK_CLARIFY_ABANDON_TIMEOUT (env var not set), \
  CAPTURE_AS_FALLBACK_NORMALIZATION_POLICY (env var not set), \
  CAPTURE_AS_FALLBACK_DEDUP_HASH_KEY (env var not set), \
  CAPTURE_AS_FALLBACK_RETENTION_AUDIT_DAYS (env var not set)
exit status 1
ERROR: config-generate-time validation failed for env=test (see above)
```

**Claim Source:** executed (`./smackerel.sh test integration --go-run
'^TestOpenKnowledgeRouting_FallbackToOpenKnowledge$'` ran to completion
and exited non-zero at the `smackerel_generate_config test` step in
`smackerel.sh:872`, which invokes `cmd/config-validate` against the
freshly-generated `config/generated/test.env.tmp`).

**Root cause (foreign-owned):** spec 074 SCOPE-1 added
`internal/config/capture_fallback.go` (`LoadCaptureFallback`) and
wired it into `internal/config/config.go` line 958 such that
`config.Load()` fails-loud when any `CAPTURE_AS_FALLBACK_*` env var
is unset. The corresponding emission step in
`scripts/commands/config.sh` was NOT landed in the same commit: a
`grep -n CAPTURE_AS_FALLBACK scripts/commands/config.sh` returns
zero matches. The persisted `config/generated/test.env` lines
513-517 contain the values from a stale prior run, but every fresh
`config generate` writes a new `.tmp` file that omits them, and the
pre-emit `cmd/config-validate` binary correctly rejects it.

This gap blocks ALL `./smackerel.sh test integration` invocations —
not just this packet's. It is owned by spec 074
(`specs/074-capture-as-fallback-policy/`), not spec 064.

**Per implement-mode artifact-ownership policy** (do NOT repair
undocumented foreign work ad hoc; route to the planning owner
instead), the SCOPE-12 DoD checkbox "Routing order verified
adversarially" CANNOT be flipped from this implement pass. The YAML
change (§3.A) is landed and ready for the live re-run as soon as
spec 074's config-emission gap is closed by its owning agent.

**Uncertainty Declaration:** Acceptance criterion (2) of
PKT-064-SCOPE12-A (live PASS for all three sub-cases with
`weather_query.top_score ≥ 0.65` for `"weather in paris today"`)
CANNOT be verified in this session because the live integration
runner exits non-zero before reaching the Go test binary. The DoD
checkbox at `scopes.md` line 406 ("Routing order verified
adversarially") therefore remains `[ ]` (unchecked). Flipping it
without live evidence would be fabrication.

**Routed:** PKT-064-SCOPE12-A-FOLLOWUP-SPEC074-CONFIG-EMISSION (see
RESULT-ENVELOPE) — request spec 074 implement owner to emit the five
`CAPTURE_AS_FALLBACK_*` env vars in `scripts/commands/config.sh` so
`./smackerel.sh test integration` is unblocked. Once landed, the
SCOPE-12 re-run can be a single command and a single evidence-block
update.

**§3.C — Post-config-gen-fix re-run (2026-06-01 20:30–20:53 UTC):
STILL BLOCKED — stack-up failure, not routing failure.**

Prior follow-up confirmed `./smackerel.sh config generate --env test`
now succeeds (the spec 074 emission gap was closed by its owner).
With `intent_examples` already extended per §3.A (including
`"weather in Paris today"`) and config-gen green, this packet
re-issued the exact command from PKT-064-SCOPE12-A:

```bash
./smackerel.sh test integration --go-run '^TestOpenKnowledgeRouting_FallbackToOpenKnowledge$'
```

Two consecutive attempts (`/tmp/scope12-run6.log`,
`/tmp/scope12-run7.log`) both exited non-zero BEFORE the Go test
binary executed. The failure mode is identical across attempts and
distinct from the §3.B blocker:

```text
 Container smackerel-test-smackerel-ml-1  Healthy
container smackerel-test-smackerel-core-1 is unhealthy
Test stack start failed once (exit 1); retrying after project-scoped teardown...
...
Running project-scoped integration test stack teardown (exit cleanup, timeout 180s)...
EXIT=1            # run7 (after one auto-retry that also failed at
                  # `Error response from daemon: No such container: ...`
                  # — a race with a concurrent compose teardown)
EXIT=124          # run6 (teardown exceeded its 180s budget under
                  # parallel-agent load)
```

Live container introspection while the first attempt was in flight
(`docker logs smackerel-test-smackerel-core-1` captured at
20:48:48 UTC) shows core blocks at:

```text
{"level":"INFO","msg":"waiting for ML sidecar readiness","timeout":60000000000,"url":"http://smackerel-ml:8081"}
```

Core's compose `healthcheck` window is `start_period:15s` +
`retries:3` × (`interval:10s` + `timeout:5s`) = ~60s, while ML
needed ≥60s to become healthy in this session
(`docker ps`: `smackerel-test-smackerel-ml-1 Up About a minute (healthy)`
appeared at 20:49:47 UTC; core was already marked unhealthy at
20:48:57 UTC, ~38s after its own start, having spent every second
inside `waiting for ML sidecar readiness`). ML startup itself was
slowed by a foreign issue surfaced in `docker logs smackerel-test-smackerel-ml-1`:

```text
litellm.llms.ollama.common_utils.OllamaError: {"error":"model 'qwen2.5:0.5b-instruct' not found"}
litellm.exceptions.APIConnectionError: litellm.APIConnectionError: OllamaException - {"error":"model 'qwen2.5:0.5b-instruct' not found"}
Subscribe to search.embed failed (attempt 1/30): nats: timeout — retrying in 1.0s
```

Both observations are foreign to spec 064 SCOPE-12 (which owns
routing only): the core↔ML readiness race lives in
`cmd/core/run()` / `docker-compose.yml` (no `depends_on` for
smackerel-ml on smackerel-core; healthcheck window too short for
sidecar cold-start), and the missing Ollama model is a separate
test-stack provisioning gap. Neither is the routing-layer surface
SCOPE-12 owns.

**Claim Source:** executed — full logs in `/tmp/scope12-run6.log`
(251 lines) and `/tmp/scope12-run7.log` (292 lines). Container
healthy/unhealthy transitions captured live via
`docker ps --filter name=smackerel-test`. ML root-cause traces
captured via `docker logs smackerel-test-smackerel-ml-1`.

**Uncertainty Declaration:** Acceptance criterion (2) of
PKT-064-SCOPE12-A (live PASS for all three sub-cases with
`weather_query.top_score ≥ 0.65` for `"weather in paris today"`)
STILL cannot be verified in this session because the live
integration runner exits non-zero before reaching the Go test
binary. The DoD checkbox at `scopes.md` line 406 ("Routing order
verified adversarially") therefore remains `[ ]` (unchecked).
Flipping it without live evidence would be fabrication. The
intent_examples addition from §3.A is necessary but not sufficient
to certify the contract — the live re-run is what proves the
embedding-cosine arithmetic actually clears the 0.65 floor under
the production ML sidecar.

**Routed:** PKT-064-SCOPE12-A-FOLLOWUP-CORE-ML-STARTUP — request
the core/compose owner to either (a) extend the smackerel-core
`healthcheck.start_period` to accommodate ML cold-start (≥120s),
(b) gate core's HTTP server start on ML readiness as a soft
dependency so `/api/health` is reachable before the ML wait
completes, or (c) add `smackerel-ml: condition: service_healthy`
to core's `depends_on` and accept the serial startup cost. Once
core can become healthy reliably in the test stack, the SCOPE-12
live re-run is a single command and a single evidence-block update.

### SCOPE-13 — Telegram surface (response shapes, citations, refusals)

**Status:** Done (this session)

**Finding closed (#1 — wire open_knowledge renderers into adapter dispatch):**
`internal/telegram/assistant_adapter/render_openknowledge.go` exposed
`RenderSourcedAnswer`, `RenderHybridAnswer`, and
`RenderRefusalWithCapture` from a prior scope but no caller invoked
them. `buildTelegramRendering` (`render_outbound.go`) now dispatches
to the open_knowledge renderers strictly via `AssistantResponse`
content composition — no change to the spec 061 `AssistantResponse`
shape, no change to `cmd/core` wiring:

- `ErrorCause` string matching any non-default
  `contracts.RefusalCause` → `RenderRefusalWithCapture`.
- `Sources` carrying at least one non-`SourceArtifact` kind →
  `RenderHybridAnswer` (mixed with `SourceArtifact`) or
  `RenderSourcedAnswer` (all non-artifact).
- All-`SourceArtifact` source sets and existing spec 061 `ErrorCause`
  values fall through to the unchanged renderer (backward
  compatibility).

**Files (this session):**

- `internal/telegram/assistant_adapter/render_outbound.go` — dispatch
  block + helpers `hasNonArtifactSources`, `hasArtifactSource`,
  `openKnowledgeRefusalCauseFromError`.
- `internal/telegram/assistant_adapter/render_outbound_test.go` —
  7 new table-driven tests:
  - `TestBuildTelegramRendering_AllArtifact_BackCompat`
  - `TestBuildTelegramRendering_OpenKnowledge_AllWeb`
  - `TestBuildTelegramRendering_OpenKnowledge_AllComputation`
  - `TestBuildTelegramRendering_OpenKnowledge_Hybrid`
  - `TestBuildTelegramRendering_OpenKnowledge_RefusalCauses`
    (5 sub-cases, one per non-default `RefusalCause`)
  - `TestBuildTelegramRendering_LegacyErrorCause_BackCompat`
  - `TestBuildTelegramRendering_G021_NoSourcesNoErrorCauseFallsThroughDefault`
    (adversarial G021 — guards that the dispatch does NOT engage on
    an uncategorised default response)

**Test commands executed in this session:**

```bash
go test ./internal/telegram/assistant_adapter/... -run 'TestBuildTelegramRendering' -count=1
# exit 0 (executed)
./smackerel.sh test unit --go
# exit 0 (executed; full unit suite green)
./smackerel.sh lint
# exit 0 (executed)
bash .github/bubbles/scripts/artifact-lint.sh specs/064-open-ended-knowledge-agent
# Artifact lint PASSED (executed)
```

**Gates:** G021 (adversarial back-compat case), G028 (closed-vocabulary
match excludes `RefusalDefault`; no silent fallback to open_knowledge
renderer for any unmatched response shape).

**Constraint honoured:** `contracts.AssistantResponse` unchanged;
`cmd/core/wiring_assistant_openknowledge_assembler.go` unchanged.
Dispatch is purely adapter-side classification on existing fields.

## SCOPE-14 — Observability (Metrics + Redacted Trace Logging)

**Phase:** implement

**Status:** local Prometheus surface + redacted INFO log line per turn
shipped; cross-spec dashboard/alert work routed via PKT-049-A to spec
049. Local DoD items satisfied; final scope completion gated on
PKT-049-A response.

**Files added:**

- `internal/assistant/openknowledge/metrics/metrics.go` (303 lines) —
  nine Prometheus collectors, closed-vocabulary allow-sets for `tool`,
  `cause`, and `scope` labels, named histogram-bucket vars (no magic
  numbers), explicit `(*Metrics).Register(prometheus.Registerer)`
  entrypoint, `Nop` recorder for tests.
- `internal/assistant/openknowledge/metrics/metrics_test.go` (210
  lines) — 10 test functions including the adversarial cardinality
  test `TestOpenKnowledgeMetrics_RejectsUnknownCause_AdversarialG021`.
- `internal/assistant/openknowledge/agent/agent_log_test.go` (137
  lines) — 2 test functions:
  `TestAgentTurnLog_RedactsSecrets` (drives an API-key-bearing prompt
  + tool args + tool result through one turn and asserts the JSON log
  contains none of the raw secret, full URL, snippet body, raw
  prompt, or system prompt text), and `TestAgentTurnLog_EmittedOnRefusal`.
- `specs/064-open-ended-knowledge-agent/route-packets/PKT-049-A.md` —
  cross-spec request to 049 (dashboard panels + alert rules + DoD
  for the responding spec).

**Files modified:**

- `internal/assistant/openknowledge/agent/agent.go` — added
  `Config.Recorder` (`okmetrics.Recorder`) and `Config.Logger`
  (`*slog.Logger`); both optional with safe defaults (`Nop{}` and
  `slog.Default()`). Instrumented `Run` to track `iterations`
  per-loop; added `finalize(out)` closure that emits one
  `recordTurnMetrics(iters, out)` call and one redacted
  `emitTurnLog(turnID, promptHash, iters, out)` call per terminal
  path (success, refusal-cap, refusal-fabricated-source, parse-error).
  Instrumented `invokeTool` to time each `tool.Execute` call and
  record `openknowledge_tool_calls_total{tool, outcome}` +
  `openknowledge_tool_latency_seconds{tool}`. Outcome derivation:
  `error` if either `execErr != nil` or `result.Error != nil`,
  otherwise `success`. Added helpers `terminationToRefusalCause`,
  `terminationToBudgetScope`, `recordTurnMetrics`, `emitTurnLog`,
  `newTurnID`, `sha256Hex`, `hostOnly`.
- `cmd/core/wiring_assistant_openknowledge.go` — added step 8b
  "Metrics" that derives `allowedToolNames` from `registry.Enabled()`
  (so the closed-vocabulary `tool` label exactly matches the running
  tool set — G021), constructs `okmetrics.New(allowedToolNames)`,
  registers it against `prometheus.DefaultRegisterer` so the existing
  `/metrics` scrape picks it up without any router change, and passes
  the recorder + `slog.Default()` into `okagent.Config`. Wiring fails
  loud on registration error.
- `specs/064-open-ended-knowledge-agent/state.json` — appended
  `PKT-049-A` to `transitionRequests` (status `pending`) and
  `PKT-049-A-await` to `reworkQueue`.
- `specs/064-open-ended-knowledge-agent/scopes.md` — SCOPE-14 status
  flipped to "Awaiting Cross-Spec Resolution (PKT-049-A pending —
  local metrics + redacted logging shipped)"; DoD items 1 + 3 marked
  `[x]` with inline evidence; DoD item 2 remains `[ ]` pending the
  spec 049 response.

**Design choice (recorded explicitly):** the user's directive listed
each tool source file as an instrumentation site. The implementation
instead instruments `agent.invokeTool` once, which is functionally
equivalent because every tool call passes through that single
function. The agent-level wrap (a) preserves the tool constructors
unchanged (so existing tool tests need no `Recorder` plumbing), (b)
gives consistent outcome classification via the same
`execErr/result.Error` predicate, and (c) keeps the closed-vocabulary
allow-set in one place (`registry.Enabled()` ⇄ `okmetrics.New`).

**Test evidence (claim source: executed):**

```text
$ go test ./internal/assistant/openknowledge/metrics/... ./internal/assistant/openknowledge/agent/... \
    -run "TestOpenKnowledgeMetrics|TestAgent.*Log|TestAgentTurnLog" -v
=== RUN   TestOpenKnowledgeMetrics_NamesPinned
--- PASS: TestOpenKnowledgeMetrics_NamesPinned (0.00s)
=== RUN   TestOpenKnowledgeMetrics_RegisterAndScrape
--- PASS: TestOpenKnowledgeMetrics_RegisterAndScrape (0.00s)
=== RUN   TestOpenKnowledgeMetrics_RejectsUnknownCause_AdversarialG021
--- PASS: TestOpenKnowledgeMetrics_RejectsUnknownCause_AdversarialG021 (0.00s)
=== RUN   TestOpenKnowledgeMetrics_RegisterRejectsNil
--- PASS: TestOpenKnowledgeMetrics_RegisterRejectsNil (0.00s)
=== RUN   TestOpenKnowledgeMetrics_DuplicateRegisterFails
--- PASS: TestOpenKnowledgeMetrics_DuplicateRegisterFails (0.00s)
=== RUN   TestOpenKnowledgeMetrics_NopSatisfiesRecorder
--- PASS: TestOpenKnowledgeMetrics_NopSatisfiesRecorder (0.00s)
=== RUN   TestOpenKnowledgeMetrics_LiveSatisfiesRecorder
--- PASS: TestOpenKnowledgeMetrics_LiveSatisfiesRecorder (0.00s)
=== RUN   TestOpenKnowledgeMetrics_AllowedToolsRoundTrips
--- PASS: TestOpenKnowledgeMetrics_AllowedToolsRoundTrips (0.00s)
=== RUN   TestOpenKnowledgeMetrics_AllRefusalCausesAccepted
--- PASS: TestOpenKnowledgeMetrics_AllRefusalCausesAccepted (0.00s)
=== RUN   TestOpenKnowledgeMetrics_AllBudgetScopesAccepted
--- PASS: TestOpenKnowledgeMetrics_AllBudgetScopesAccepted (0.00s)
PASS
ok  github.com/smackerel/smackerel/internal/assistant/openknowledge/metrics 0.018s
=== RUN   TestAgentTurnLog_RedactsSecrets
--- PASS: TestAgentTurnLog_RedactsSecrets (0.00s)
=== RUN   TestAgentTurnLog_EmittedOnRefusal
--- PASS: TestAgentTurnLog_EmittedOnRefusal (0.00s)
PASS
ok  github.com/smackerel/smackerel/internal/assistant/openknowledge/agent  0.014s
# Exit code: 0
```

**Full unit suite (no collateral failures):**

```text
$ ./smackerel.sh test unit --go
[go-unit] go test ./... finished OK
# Exit code: 0
```

**Lint (no collateral failures):**

```text
$ ./smackerel.sh lint
Web validation passed
# Exit code: 0
```

**Cross-spec routing:** route packet
[`PKT-049-A`](route-packets/PKT-049-A.md) emitted to
`specs/049-monitoring-stack` with full dashboard panel queries and
alert-rule expressions. SCOPE-14 close-out blocked on the response
packet from spec 049.

**Gates honoured:**

- **G021 (bounded cardinality):** every label allow-set is enumerated
  (tools from `registry.Enabled()`, causes from
  `contracts.AllRefusalCauses`, scopes from `AllBudgetScopes`,
  outcomes ∈ `{success, error}`). Adversarial test thousands of
  rogue values across cause, tool, scope, latency-tool, and outcome;
  series count stays at the legitimate value.
- **G028 (no defaults / no magic numbers):** every histogram bucket
  is a named `var` (`IterationBuckets`, `TokenBuckets`,
  `USDCentsBuckets`, `ToolLatencyBuckets`); Recorder is required
  (nil → safe `Nop{}` for tests but never silently widens runtime
  behaviour); `Register(nil)` returns a typed error; the wiring
  layer fails loud on registration error.
- **No PII / no secrets in logs:**
  `TestAgentTurnLog_RedactsSecrets` is the regression guard. The
  log payload contains only the SHA-256 of the user prompt,
  per-tool {name, outcome} entries (no args, no URL, no snippet
  body), and bounded sentinel strings for termination/refusal.

## Cross-Spec Route Packets

| Packet ID | Date emitted | Routed to | Status | Blocks (in spec 064) | Summary |
|-----------|--------------|-----------|--------|----------------------|---------|
| [PKT-061-A](route-packets/PKT-061-A.md) | 2026-05-31 | `specs/061-conversational-assistant` | pending | SCOPE-13 | Extend provenance gate Source taxonomy (`SourceWeb`, `SourceToolComputation`) + extend `CanonicalRefusalBody` with 5 spec 064 refusal causes. Additive; existing `SourceArtifact` behaviour unchanged. |
| [PKT-049-A](route-packets/PKT-049-A.md) | 2026-05-31 | `specs/049-monitoring-stack` | pending | SCOPE-14 close-out only | Add Grafana panels (iterations p95, USD spend rate, refusal rate by cause, fabricated-source rate, tool call rate by tool, per-tool latency p95, budget exhausted by scope, compaction-signal rate) + Prometheus alert rules (fabricated_source > 0, monthly/per-user budget hits, tool error rate > 0.1/s, refusal spike) for the new `openknowledge_*` series. No scrape-config change required (rides existing /metrics endpoint). |

SCOPE-10 status is `Awaiting Cross-Spec Resolution`; its DoD MUST NOT be
marked complete until spec 061 returns merged-evidence closing PKT-061-A.
State tracked under `state.json.transitionRequests[PKT-061-A]` and
`state.json.reworkQueue[PKT-061-A-await]`.

## Completion Statement

Implementation chain has produced runnable, test-backed code for SCOPE-01
through SCOPE-08. Status remains `in_progress` because no scope has yet
been promoted past implementation: validation, audit, and chaos phases
have not run, and scopes 09-18 are Not Started. No DoD checkboxes in
`scopes.md` are flipped to `[x]` yet because per-scope DoD verification is
the responsibility of `bubbles.validate` once the full chain is delivered.

## SCOPE-15 — Security Hardening (Egress Allowlist + Snippet Sanitisation + API-Key Audit)

**Phase:** implement · **Agent:** bubbles.implement · **Date:** 2026-05-31

### Files

| File | Purpose | LoC |
|------|---------|----:|
| [internal/assistant/openknowledge/web/egress.go](../../internal/assistant/openknowledge/web/egress.go) | `EgressAllowlistTransport` — deny-by-default `http.RoundTripper` enforcing exact-host allowlist, case-insensitive, userinfo-stripped, scheme-guarded. | 91 |
| [internal/assistant/openknowledge/web/egress_test.go](../../internal/assistant/openknowledge/web/egress_test.go) | Adversarial coverage: deny-by-default, mixed-case host, userinfo non-bypass, non-http(s) scheme rejection, malformed entries, nil inner. | 198 |
| [internal/assistant/openknowledge/web/sanitize.go](../../internal/assistant/openknowledge/web/sanitize.go) | `SanitizeSnippet` — control-char strip, UTF-8 repair, `MaxSnippetRunes=2000` truncation, 14 suspicious-injection patterns + `SuspiciousSnippetRecorder` boundary. | 157 |
| [internal/assistant/openknowledge/web/sanitize_test.go](../../internal/assistant/openknowledge/web/sanitize_test.go) | Adversarial: prompt-injection detected without content stripping, control chars stripped, invalid UTF-8 safely handled, truncation marker, no false-positive on benign text. | 117 |
| [internal/assistant/openknowledge/web/searxng.go](../../internal/assistant/openknowledge/web/searxng.go) | Sanitiser wired into every returned snippet; `ContentHash` computed over sanitised body; `WithSuspiciousSnippetRecorder` option. | +27 lines |
| [internal/assistant/openknowledge/web/searxng_test.go](../../internal/assistant/openknowledge/web/searxng_test.go) | `TestSearxNG_Search_AppliesSanitization` proves snippet → sanitised + ContentHash covers sanitised + recorder fired. | +47 lines |
| [internal/assistant/openknowledge/web/apikey_test.go](../../internal/assistant/openknowledge/web/apikey_test.go) | API-key non-leakage regression guards (`TestOpenKnowledgeAPIKey_NeverLoggedByWebProviders`, `TestOpenKnowledgeAPIKey_NeverInErrorMessages`). | 88 |
| [internal/assistant/openknowledge/metrics/metrics.go](../../internal/assistant/openknowledge/metrics/metrics.go) | New `openknowledge_suspicious_snippet_total{provider}` collector + `IncSuspiciousSnippet(provider)` method with bounded-cardinality provider allowlist. | +35 lines |
| [cmd/core/wiring_assistant_openknowledge.go](../../cmd/core/wiring_assistant_openknowledge.go) | `buildOpenKnowledgeWebProvider` wraps `http.Client.Transport` with `EgressAllowlistTransport`; effective allowlist = `provider_endpoint` host ∪ `AllowedEgressHosts`; installs metric recorder. | +38 lines |
| [internal/config/openknowledge.go](../../internal/config/openknowledge.go) | New `AllowedEgressHosts []string` SST field + format validation (no scheme, path, port, userinfo, whitespace). | +24 lines |
| [internal/config/openknowledge_test.go](../../internal/config/openknowledge_test.go) | `TestOpenKnowledgeConfig_AllowedEgressHosts_*` (happy path, rejects malformed entries, empty-allowed-when-enabled). | +63 lines |
| [internal/config/validate_test.go](../../internal/config/validate_test.go) | Add `ASSISTANT_OPEN_KNOWLEDGE_ALLOWED_EGRESS_HOSTS` to `setRequiredEnv` baseline. | +1 line |
| [config/smackerel.yaml](../../config/smackerel.yaml) | New `assistant.open_knowledge.allowed_egress_hosts: []` (deny-by-default, explicit empty list). | +9 lines |
| [scripts/commands/config.sh](../../scripts/commands/config.sh) | Emit `ASSISTANT_OPEN_KNOWLEDGE_ALLOWED_EGRESS_HOSTS` from `yaml_get_json` with `[]` fallback for empty. | +6 lines |
| [specs/064-open-ended-knowledge-agent/route-packets/PKT-020-A.md](route-packets/PKT-020-A.md) | Route packet to spec 020 — review allowlist policy, wildcard follow-up, network-layer egress firewall, SearxNG upstream-engines constraint. | 156 |

### Evidence

**Claim Source:** executed.

```text
$ ./smackerel.sh test unit --go --go-run "TestEgress|TestSanitize|TestOpenKnowledgeAPIKey"
[go-unit] go test ./... finished OK
exit=0
```

```text
$ ./smackerel.sh test unit --go
[go-unit] go test ./... finished OK
exit=0
```

```text
$ ./smackerel.sh lint
Web validation passed
exit=0
```

```text
$ go test -run "TestEgress|TestSanitize|TestOpenKnowledgeAPIKey|TestSearxNG_Search_AppliesSanitization|TestOpenKnowledgeConfig_AllowedEgressHosts" \
    ./internal/config/... ./internal/assistant/openknowledge/...
ok  github.com/smackerel/smackerel/internal/config
ok  github.com/smackerel/smackerel/internal/assistant/openknowledge/web
```

```text
$ ./smackerel.sh config generate
config-validate: ~/smackerel/config/generated/dev.env.tmp.* OK
Generated ~/smackerel/config/generated/dev.env
$ grep ALLOWED_EGRESS_HOSTS config/generated/dev.env
ASSISTANT_OPEN_KNOWLEDGE_ALLOWED_EGRESS_HOSTS=[]
```

### Adversarial Test Names (G021 coverage)

- `TestEgressAllowlistTransport_DenyByDefault_Adversarial` — empty allowlist denies every request; inner transport MUST NOT be called (proves no DNS/SYN leak on deny).
- `TestEgressAllowlistTransport_DisallowedHostDenied` — non-allowlisted host returns `ErrEgressDenied`; inner transport not called.
- `TestEgressAllowlistTransport_NormalizesMixedCaseHost` — `EXAMPLE.com` matches allowlist entry `Example.COM` (case-fold attack blocked).
- `TestEgressAllowlistTransport_UserinfoDoesNotBypass` — `https://allowed.example:pass@blocked.example/` denied (userinfo containing allowed-host string MUST NOT bypass the gate).
- `TestEgressAllowlistTransport_RejectsNonHTTPScheme` — `file://`, `ftp://`, `gopher://` all rejected (no protocol-handler exfiltration).
- `TestNewEgressAllowlistTransport_RejectsMalformedEntries` — scheme / path / port / userinfo / whitespace in allowlist entries fail loud at construction time (no silent allow-all from operator typo).
- `TestSanitizeSnippet_DetectsPromptInjection_Adversarial` — "IGNORE PREVIOUS INSTRUCTIONS" content passes through unmodified AND increments metric (LLM-side fencing primary; observability secondary).
- `TestSanitizeSnippet_DetectsLLMChatTokens` — `<|im_start|>` / `<|im_end|>` chat-template tokens detected.
- `TestSanitizeSnippet_NoFalsePositiveOnBenignText` — "cooking instructions" benign phrase does NOT trigger.
- `TestSanitizeSnippet_InvalidUTF8SafelyHandled` — `\xff`-bearing input does not panic; output is valid UTF-8.
- `TestOpenKnowledgeConfig_AllowedEgressHosts_RejectsMalformedEntries` — config-layer rejects scheme / path / port / userinfo / whitespace / empty entries with `F064-SST-INVALID`.
- `TestSearxNG_Search_AppliesSanitization` — BEL stripped from upstream snippet, suspicious-pattern metric fires once, `ContentHash` covers sanitised body (cite-back verifier canonical-form invariant).
- `TestOpenKnowledgeAPIKey_NeverLoggedByWebProviders` — sentinel API key never appears in `slog` output after provider construction + Search call.
- `TestOpenKnowledgeAPIKey_NeverInErrorMessages` — sentinel API key never appears in any returned error (regression guard against `fmt.Errorf("%v", config)`-style leaks).

### API-Key Audit Result

Static audit of `internal/assistant/openknowledge/{web,llm}/`:

| File | `slog`/`log.` calls | API key field touched | Logged? |
|------|---------------------|------------------------|---------|
| `internal/assistant/openknowledge/web/searxng.go` | none | none (SearxNG v1 contract takes no API key on the wire) | n/a |
| `internal/assistant/openknowledge/web/brave.go` | none | none (stub) | n/a |
| `internal/assistant/openknowledge/web/tavily.go` | none | none (stub) | n/a |
| `internal/assistant/openknowledge/llm/client.go` | none | `cfg.AuthToken` → `Authorization: Bearer …` header (write-only into request, never logged) | no |

No regression of the existing `Logs redact API keys` contract (design.md §Security) was introduced. The new `apikey_test.go` is a forward-looking guard against future regressions.

### Cross-Spec Route Packets

| Packet | Status | Routed To | Scope | Summary |
|--------|--------|-----------|-------|---------|
| [PKT-020-A](route-packets/PKT-020-A.md) | pending | `specs/020-security-hardening` | SCOPE-15 close-out only | Review (a) v1 exact-match allowlist policy, (b) wildcard follow-up question, (c) network-layer egress firewall / container egress policy as additive defence, (d) SearxNG upstream-engines (`engines:`) constraint at the deploy adapter layer. |

SCOPE-15 status is `Awaiting Cross-Spec Resolution`; local items are complete (egress allowlist transport, sanitiser, ContentHash-over-sanitised, API-key audit, SST + emission + validation). Spec 020 owns the network-layer review.

State tracked under `state.json.transitionRequests[PKT-020-A]` and `state.json.reworkQueue[PKT-020-A-await]`.

## SCOPE-16 — Resilience: Web-Provider Circuit Breaker + Budget-Exhaustion Degradation

**Status:** `Awaiting Cross-Spec Resolution` (local items complete; PKT-022-A pending against `specs/022-operational-resilience`).

### Implementation Summary

Added a concurrency-safe circuit breaker that wraps any `WebSearchProvider` (SearxNG today; Brave / Tavily stubs unchanged) and short-circuits subsequent calls once a configurable number of consecutive transport-class failures has been observed. Wired into `cmd/core` before tool registration; SST-bound thresholds; new termination reason for the agent loop; new metrics for the existing `/metrics` endpoint.

| Component | File | Notes |
|-----------|------|-------|
| Circuit breaker | `internal/assistant/openknowledge/web/circuit.go` | Three-state machine (Closed/HalfOpen/Open) with `sync.Mutex`-guarded transitions, exported `State()` accessor, `CircuitOption` seam for clock + recorder injection. |
| Circuit tests | `internal/assistant/openknowledge/web/circuit_test.go` | 12 tests; 3 adversarial G021 cases (Open does NOT leak through to inner provider; ErrInvalidQuery does NOT count toward threshold; non-counting errors do NOT reset the counter). |
| SST surface | `internal/config/openknowledge.go` | New `OpenKnowledgeCircuitBreakerConfig` sub-struct: `FailureThreshold`, `OpenWindowSeconds`, `HalfOpenAfterSeconds` — all required > 0; deep-validation skipped when `Enabled=false`. |
| YAML SST | `config/smackerel.yaml` | New `circuit_breaker:` sub-block under `open_knowledge:` with explicit values `5 / 60 / 30`. |
| Config script | `scripts/commands/config.sh` | Three new `required_value` lookups + three new env emissions. |
| Wiring | `cmd/core/wiring_assistant_openknowledge.go` | Wraps the built `WebSearchProvider` in `web.NewCircuitBreaker(...)` before `tools.RegisterAll`. |
| Agent termination | `internal/assistant/openknowledge/agent/agent.go` | New `TerminationToolUnavailable` reason; new `ToolErrorCodeCircuitOpen` constant; mid-loop check after `invokeTool` short-circuits the turn with empty FinalText so the Telegram surface applies capture-as-fallback. |
| Agent mapping | `internal/assistant/openknowledge/agenttool/substrate_tool.go` | `MapTerminationToRefusalCause` extended: `TerminationToolUnavailable → RefusalToolUnavailable`. |
| Web-search tool | `internal/assistant/openknowledge/tools/web_search.go` | New `ErrWebSearchCircuitOpen` sentinel (code `provider_circuit_open`); `classifyProviderError` branch on `web.ErrCircuitOpen`. |
| Metrics | `internal/assistant/openknowledge/metrics/metrics.go` | New `openknowledge_circuit_state{provider}` gauge (0/1/2) and `openknowledge_circuit_trips_total{provider}` counter. Same bounded `provider` allow-set as `suspicious_snippet`. |
| Agent tests | `internal/assistant/openknowledge/agent/agent_test.go` | `TestAgent_CircuitOpen_TerminatesToolUnavailable` (happy path); `TestAgent_CircuitOpen_DoesNotLeakUnrelatedErrorCodes_AdversarialG021` (proves the check is narrow — a calculator divide-by-zero stays recoverable). |
| Mapping tests | `internal/assistant/openknowledge/agenttool/substrate_tool_test.go` | Extended `TestMapTerminationToRefusalCause` + `TestMapTurnResult_Refused` tables to include the new termination reason. |
| Config tests | `internal/config/openknowledge_test.go` | Extended baseline env + 8 new tests: happy path, 6 reject-non-positive cases, disabled-skips-validation. |
| Shared test env | `internal/config/validate_test.go` | Added three `t.Setenv` lines for the new circuit env keys. |

### Failure Classification (G028)

| Provider error | Counts toward `FailureThreshold`? | Rationale |
|----------------|-----------------------------------|-----------|
| `ErrProviderUnreachable` | Yes | Transport-class outage. |
| `ErrQuotaExceeded` | Yes | Provider-side rate-limit / paid quota exhaustion. |
| `ErrInvalidQuery` | No | Caller-side bug, not a provider problem. |
| `ErrProviderNotConfigured` | No | Config bug, not a provider problem. |
| `ErrMalformedResponse` | No | Protocol bug — breaker can't help. |
| `ErrInvalidConfig` | No | Construction bug — breaker can't help. |
| Default arm (unknown error) | No | Conservative — a stray classification bug must not inflate the trip rate. |

A successful call always resets the consecutive-failure counter.

### Cross-Spec Route Packets

| Packet | Status | Routed To | Scope | Summary |
|--------|--------|-----------|-------|---------|
| [PKT-022-A](route-packets/PKT-022-A.md) | pending | `specs/022-operational-resilience` | SCOPE-16 close-out only | Review (a) whether v1 thresholds (5 / 60s / 30s) match the operational-resilience playbook (connector supervisor uses 5 panics — should a `runtime.resilience.default_*` SST block exist?), (b) whether an openknowledge health-check endpoint contribution is required for operator dashboards, (c) whether the budget-exhaustion refusal-with-capture handshake should be lifted into a cross-subsystem graceful-degradation pattern. |

SCOPE-16 status is `Awaiting Cross-Spec Resolution`; local items are complete (circuit breaker, SST keys + validation, wiring, agent termination + refusal mapping, metrics). Spec 022 owns the operational-playbook alignment + the cross-subsystem degradation-pattern question.

State tracked under `state.json.transitionRequests[PKT-022-A]` and `state.json.reworkQueue[PKT-022-A-await]`.

---

## SCOPE-17 — End-to-end live-stack scenarios (bubbles.implement, 2026-05-31)

**Status:** Blocked — see [PKT-WORKFLOW-A](route-packets/PKT-WORKFLOW-A.md).

**Phase:** implement
**Claim Source:** executed
**Agent:** bubbles.implement

### What shipped

- New file `tests/e2e/agent/openknowledge_e2e_test.go` (~520 lines, `//go:build e2e`):
  - `TestOpenKnowledgeE2E_A01_WebAnswerWithCitations` — UC-064-A01
  - `TestOpenKnowledgeE2E_A02_UnitConvert` — UC-064-A02
  - `TestOpenKnowledgeE2E_A03_HybridInternalPlusWeb` — UC-064-A03
  - `TestOpenKnowledgeE2E_A04_PerTurnBudgetExhausted` — UC-064-A04
  - `TestOpenKnowledgeE2E_A05_WebSearchDisabled` — UC-064-A05
  - `TestOpenKnowledgeE2E_A06_PerUserMonthlyBudgetExceeded` — UC-064-A06
  - `TestOpenKnowledgeE2E_A06_FabricatedSourceRejected` — adversarial G021
- Each test posts to the real `/v1/agent/invoke` endpoint with the
  `open_knowledge` scenario id, asserts the structured refusal /
  success envelope shape, and asserts source-kind composition per
  spec.md SCN-064-Axx. No mocks; uses `net/http.Client` direct.
- Each test polls a distinct env knob and `t.Skip(...)`s honestly with
  an explicit message naming the routed PKT-WORKFLOW-A finding when
  the corresponding infrastructure piece is not in place.

### Executed evidence

```text
$ go vet -tags e2e ./tests/e2e/agent/...
(exit 0, no output)

$ go test -tags e2e -run TestOpenKnowledgeE2E -count=1 -v ./tests/e2e/agent/...
=== RUN   TestOpenKnowledgeE2E_A01_WebAnswerWithCitations
    openknowledge_e2e_test.go:192: e2e: AGENT_INVOKE_URL not set — live stack not exposed to test runner. This is a routed finding for the e2e test-harness owner; SCOPE-17 cannot exercise the real POST /v1/agent/invoke path until the harness injects it.
--- SKIP: TestOpenKnowledgeE2E_A01_WebAnswerWithCitations (0.00s)
... (six more SKIP entries with the same explicit message) ...
PASS
ok      github.com/smackerel/smackerel/tests/e2e/agent  0.247s
```

(Per-test stderr varies by the first missing prerequisite — finding #6 for
the URL knob hits first when no harness env is present. With the URL knob
present, finding #1 fires for tests A01/A02/A03/A04, finding #3 fires for
the fabricated-source adversarial, etc.)

### Blocking infrastructure findings

| # | Finding | Owner |
|---|---------|-------|
| 1 | `ml/app/routes/chat.py` returns HTTP 501 without fixture header — no real Ollama dispatch path | spec 064 SCOPE-09 (or spec 045 / 061 LLM wiring) |
| 2 | `/v1/agent/invoke` does not run capture-as-fallback (Telegram facade only today) | spec 061 conversational-assistant |
| 3 | No `fixture-fabricated-cite` test mode in `chat.py` for adversarial G021 | spec 064 SCOPE-08 |
| 4 | No per-test per-query token budget override knob | spec 064 SCOPE-09 wiring |
| 5 | No per-test tool-allowlist override knob | spec 064 SCOPE-09 wiring |
| 6 | `smackerel.sh test e2e` does not export `AGENT_INVOKE_URL` | spec 023 / test harness owner |

All six routed via [PKT-WORKFLOW-A](route-packets/PKT-WORKFLOW-A.md) to
`bubbles.workflow` for dispatch. State tracked under
`state.json.transitionRequests[PKT-WORKFLOW-A]` and
`state.json.reworkQueue[PKT-WORKFLOW-A-await]`.

### Scope-isolation decision

Per `agent-common.md` artifact ownership: each blocking finding is
owned by an upstream scope/spec whose deliverable was incomplete.
SCOPE-17 is the consumer; repairing each finding here would either
duplicate work owned elsewhere (#1, #3, #4, #5) or modify a different
spec's facade (#2). The route-out is the correct disposition.

### Scope DoD status

| DoD item | Status | Note |
|----------|--------|------|
| All 8 scenarios green; ≥1 adversarial per scenario | `[ ]` | Blocked on findings #1, #3, #4, #5 |
| Fabricated-citation regression proves verifier blocks it | `[ ]` | Blocked on finding #3 |
| No `route()` / `intercept()` / `msw` in suite | `[x]` | `grep -rn 'route()\|intercept(\|msw\|nock' tests/e2e/agent/openknowledge_e2e_test.go` → 0 matches |
| Gates G021, G028, G082, G083 | `[ ]` | Verification deferred until tests execute |

### Honesty declaration

The seven test functions document precisely what SCOPE-17 needs to
prove. Each refuses to lie: rather than passing trivially in the
absence of infrastructure, they `t.Skip("…")` with the routed-finding
ID embedded in the message. When PKT-WORKFLOW-A findings land, the
existing tests activate without modification and the SCOPE-17 DoD can
be re-evaluated. SCOPE-17 is NOT marked done.

## SCOPE-18 — Docs + deploy adapter contract (bubbles.docs, 2026-05-31)

**Status:** Done.
**Phase:** docs
**Claim Source:** executed
**Agent:** bubbles.docs

### Summary

Published managed-doc updates and the deploy-adapter contract delta
for the open-knowledge agent. No new infrastructure work, no new
runtime code; purely documentation + the SST key catalog entry under
`deploy/contract.yaml`.

### What shipped

| File | Delta | Purpose |
|------|-------|---------|
| `docs/Operations.md` | +~200 lines, new section `## Open-Knowledge Assistant Agent (Spec 064)` | Operator surface: enable/disable, provider tradeoffs, budgets, allowlists, circuit breaker, refusal taxonomy, security posture, troubleshooting, privacy note. |
| `docs/Development.md` | +~50 lines, new subsection `### Spec 064 Open-Knowledge Agent` under "Agent + Tool Development Discipline" | Developer surface: local dev config, integration test invocation (`ENABLE_SEARXNG=true`), e2e test status (SCOPE-17 blocked on PKT-WORKFLOW-A). |
| `docs/Testing.md` | +1 paragraph appended to existing SCOPE-07 section | Documents the pending `fixture-fabricated-cite` test mode and which routed finding owns it. |
| `docs/Deployment.md` | +~75 lines, new section `## Spec 064 Deployment Notes (Open-Knowledge Agent)` | Operator-facing SST key inventory (with `secret: true|false`), egress implication, rollback path. |
| `deploy/contract.yaml` | +~110 lines, new `sstKeyCatalog:` block + spec 064 entry | Adapter-side contract: 21 `assistant.open_knowledge.*` keys + 3 `environments.<env>.*` keys, each annotated with `path`, `type`, `secret`, and (where applicable) fail-loud `notes`. |
| `deploy/README.md` | +10 lines, new section `## Per-Spec SST Key Catalogs` | Pointer to the new catalog and to `docs/Deployment.md` operator notes. |

### Test Evidence

```bash
$ python3 -c "import yaml; yaml.safe_load(open('deploy/contract.yaml')); print('YAML OK')"
YAML OK
# Exit code: 0 (executed)

$ grep -nE '([0-9]{1,3}\.){3}[0-9]{1,3}|\.ts\.net|tailnet' \
    docs/Operations.md docs/Development.md docs/Testing.md \
    docs/Deployment.md deploy/contract.yaml deploy/README.md \
    | grep -v "127.0.0.1\|0.0.0.0\|10.0\.\|192\.168"
# Only pre-existing generic placeholders remain
# (<host-tailnet-fqdn>, "tailnet IP / loopback").
# No new IPs, no real hostnames, no real tailnet ids introduced.

$ timeout 600 bash .github/bubbles/scripts/regression-baseline-guard.sh \
    specs/064-open-ended-knowledge-agent --verbose
🐾 Regression baseline guard: PASSED
   All 0 checks passed.
# Exit code: 0 (executed)

$ bash .github/bubbles/scripts/artifact-lint.sh \
    specs/064-open-ended-knowledge-agent
Artifact lint PASSED.
# Exit code: 0 (executed)

$ go test -count=1 -timeout 60s ./internal/deploy/...
ok      github.com/smackerel/smackerel/internal/deploy  50.729s
# Exit code: 0 (executed; compose_contract_test.go still green)
```

### Completion Statement

Documentation and contract obligations for spec 064 are published.
SCOPE-18 DoD checkboxes are all `[x]` with executed evidence above
and inline in `scopes.md` SCOPE-18. The PKT-WORKFLOW-A blockage
on SCOPE-17 is documented in the published docs so operators and
downstream agents see the active gap; marking SCOPE-18 done does
NOT advance the spec to `done` overall — spec-level closure still
gates on SCOPE-17.

### Honesty declaration

No drift fixes were required during this scope — the spec 064
managed-doc surface did not exist before this scope. The cross-
reference work focused on the existing committed code (`refusal.go`
taxonomy, `assistant.open_knowledge.*` SST block in
`config/smackerel.yaml`, `compose.deploy.yml` invariants) and on
existing route packets (PKT-WORKFLOW-A, PKT-020-A, PKT-022-A,
PKT-049-A) so the published docs name real, traceable follow-up
ownership rather than aspirational behavior.

---

## PKT-WORKFLOW-A Finding #1 — Closed (2026-05-31)

**Status:** Resolved. `ml/app/routes/chat.py` now dispatches a real LLM
turn via `litellm.acompletion` against the Ollama backend pointed to by
`OLLAMA_URL` whenever the `X-OpenKnowledge-Test-Mode` header is absent.
Both fixture modes (`fixture-final-text`, `fixture-tool-use`) are
preserved unchanged.

### Implementation

- `ml/app/routes/chat.py` — added `_translate_messages`,
  `_translate_tools`, `_parse_tool_call_arguments`, and `_dispatch_live`
  helpers. Header-absent branch now calls litellm with the request's
  `model` (prefixed `ollama/`) and `api_base=OLLAMA_URL`. Errors map to
  typed envelopes: `llm_misconfigured` (500, OLLAMA_URL unset, G028
  fail-loud), `llm_provider_unreachable` (502, transport/timeout/503),
  `llm_provider_error` (502, generic APIError),
  `llm_malformed_tool_call` (502, missing function.name),
  `llm_dispatch_failed` (500, defensive).
- `ml/tests/test_tool_roundtrip.py` —
  `test_route_no_test_mode_header_returns_501` replaced with
  `test_route_no_test_mode_header_dispatches_live` which asserts the
  G028 fail-loud `llm_misconfigured` 500 when `OLLAMA_URL` is unset
  (proving the route no longer silently 501s).
- `ml/tests/test_chat_live_ollama.py` — new opt-in
  `live_ollama`-marked smoke test that POSTs a minimal real request and
  skips when `OLLAMA_URL` / `LLM_MODEL` are unset or Ollama is
  unreachable.
- `ml/pyproject.toml` — registered `live_ollama` pytest marker.

### Evidence (Claim Source: executed)

```text
$ ./smackerel.sh test unit --python
+ pytest ml/tests -q
........................................................................ [ 15%]
..............s......................................................... [ 30%]
........................................................................ [ 45%]
........................................................................ [ 60%]
........................................................................ [ 75%]
........................................................................ [ 90%]
............................................                             [100%]
475 passed, 1 skipped, 1 warning in 17.95s
+ echo '[py-unit] pytest ml/tests finished OK'
Exit Code: 0
```

The 1 skip is the new `test_chat_live_ollama.py` (opt-in marker; no
`OLLAMA_URL` in unit test env). All previously passing tests still pass
including both fixture-mode tests.

Live end-to-end smoke against the running stack (gemma3:4b pulled in
the dev Ollama):

```text
$ ./smackerel.sh build && ./smackerel.sh down && ./smackerel.sh up
... (smackerel-core + smackerel-ml rebuilt, stack restarted, all
    containers healthy)

$ TOKEN=<REDACTED-DEV-TOKEN>
$ curl -sS -m 180 -X POST http://127.0.0.1:40002/llm/chat \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/json" \
    -d '{"model":"gemma3:4b","messages":[{"role":"user","content":"What is 10 degrees Fahrenheit in Celsius? Answer with one short sentence."}],"max_tokens":80,"temperature":0.0}'
{"stop_reason":"end_turn","text":"10 degrees Fahrenheit is equal to -17.78 degrees Celsius.","tool_calls":null,"tokens_used":47}
Exit Code: 0
```

Tool-use translation also verified live:

```text
$ curl -sS -m 180 -X POST http://127.0.0.1:40002/llm/chat \
    -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
    -d '{"model":"gemma3:4b","messages":[{"role":"user","content":"Convert 10F to C using the unit_convert tool."}],"tools":[{"name":"unit_convert","description":"Convert between units","parameters":{"type":"object","properties":{"value":{"type":"number"},"from":{"type":"string"},"to":{"type":"string"}},"required":["value","from","to"]}}],"max_tokens":120,"temperature":0.0}'
{"stop_reason":"tool_use","text":null,"tool_calls":[{"id":"call_da311507-f8be-42cd-b6ed-a668d8f5dcbc","name":"unit_convert","arguments":{"value":10,"from":"F","to":"C"}}],"tokens_used":180}
Exit Code: 0
```

### Honest scope note

The user-supplied curl against `POST /v1/agent/invoke` still returns
`{"message":"llm_driver_error","outcome":"provider-error"}` — but the
failure originates from a DIFFERENT code path (`internal/agent`
NATS-based LLM driver → `ml/app/processor.py`) that is unrelated to
`/llm/chat`. The ML log shows the underlying cause is that the
NATS-side `LLM_MODEL=qwen2.5:0.5b-instruct` is not pulled in the local
Ollama (only `gemma3:4b` and `nomic-embed-text` are). Wiring
`/v1/agent/invoke` to use the new `/llm/chat` real-dispatch path (or
pulling `qwen2.5:0.5b-instruct`) is out of scope for PKT-WORKFLOW-A
Finding #1 and is captured in the remaining five PKT-WORKFLOW-A
findings.

### PKT-WORKFLOW-A overall status

Finding #1 resolved. Findings #2–#6 remain pending — `reworkQueue`
entry `PKT-WORKFLOW-A-await` stays open with an updated note.

---

## §3.D — SCOPE-12 live integration re-run (2026-06-01 21:50 UTC): PASS

**Claim Source:** executed.

**Context.** The test-stack core healthcheck blocker
(`docker-compose.yml` `smackerel-core` `start_period` 15s→120s) is
landed. This re-run also closes three downstream blockers discovered
during execution and addressed in the same change set:

1. `smackerel.sh test integration` did not propagate
   `AGENT_SCENARIO_DIR`, `ML_SIDECAR_URL`, or
   `AGENT_ROUTING_FALLBACK_SCENARIO_ID` into the Go test container
   (test self-skipped with "AGENT_SCENARIO_DIR not set — live stack
   not available"). Fixed by reading those values from the generated
   test env file and passing them explicitly, plus anchoring the
   scenario-dir path under `/workspace` so the per-package Go test
   CWD resolves it. Also switched the test container to consume the
   generated env file via `--env-file` so SST-driven timeouts
   (`RECIPE_SEARCH_TIMEOUT_MS`, `RECIPE_SEARCH_PER_TOOL_TIMEOUT_MS`,
   `RETRIEVAL_QA_TIMEOUT_MS`, `RETRIEVAL_QA_PER_TOOL_TIMEOUT_MS`)
   reach the loader's `envsubst` pass instead of leaving
   `limits.timeout_ms: ${RECIPE_SEARCH_TIMEOUT_MS}` unresolved.
2. The `tests/integration/agent` package did not import the
   production tool packages, so `agent.DefaultLoader().Load(...)`
   rejected every scenario whose `allowed_tools[0].name` was missing
   from the registry (`weather_lookup`, `recipe_search`,
   `retrieval_search`, `recommendation_*`, `notification_propose`).
   Fixed by adding a build-tagged
   `tests/integration/agent/tool_imports_test.go` with blank imports
   mirroring `cmd/core/wiring_agent.go` (notification, recipesearch,
   retrieval, weather, microtools, recommendation/tools, plus the
   openknowledge substrate tool).

**Command.**

```text
$ ./smackerel.sh test integration --go-run \
    '^TestOpenKnowledgeRouting_FallbackToOpenKnowledge$'
```

(captured at `/tmp/s064-12e.log`)

**Result.**

```text
=== RUN   TestOpenKnowledgeRouting_FallbackToOpenKnowledge
=== RUN   TestOpenKnowledgeRouting_FallbackToOpenKnowledge/weather-domain-query-does-not-route-to-open-knowledge
    openknowledge_routing_test.go:128: query="weather in paris today" → weather_query (top_score=1.000, reason=similarity_match)
=== RUN   TestOpenKnowledgeRouting_FallbackToOpenKnowledge/open-ended-knowledge-question-routes-to-open-knowledge
    openknowledge_routing_test.go:128: query="explain quantum entanglement briefly" → open_knowledge (top_score=0.183, reason=fallback_clarify)
=== RUN   TestOpenKnowledgeRouting_FallbackToOpenKnowledge/deterministic-tool-question-routes-to-open-knowledge
    openknowledge_routing_test.go:128: query="what is 10F in C" → open_knowledge (top_score=0.763, reason=similarity_match)
--- PASS: TestOpenKnowledgeRouting_FallbackToOpenKnowledge (1.18s)
    --- PASS: TestOpenKnowledgeRouting_FallbackToOpenKnowledge/weather-domain-query-does-not-route-to-open-knowledge (0.04s)
    --- PASS: TestOpenKnowledgeRouting_FallbackToOpenKnowledge/open-ended-knowledge-question-routes-to-open-knowledge (0.02s)
    --- PASS: TestOpenKnowledgeRouting_FallbackToOpenKnowledge/deterministic-tool-question-routes-to-open-knowledge (0.01s)
PASS
ok      github.com/smackerel/smackerel/tests/integration/agent  1.313s
…
PASS: go-integration
EXIT=0
```

**Acceptance criteria check.**

| Criterion                                                  | Observed                                          | Result |
|------------------------------------------------------------|---------------------------------------------------|--------|
| weather query routes to `weather_query`, not open_knowledge | weather_query, reason=similarity_match            | PASS   |
| `weather_query.top_score ≥ 0.65` for `weather in paris today` | top_score=1.000                                   | PASS   |
| open-ended knowledge question routes to open_knowledge     | open_knowledge, reason=fallback_clarify (0.183)   | PASS   |
| deterministic-tool question routes to open_knowledge       | open_knowledge, reason=similarity_match (0.763)   | PASS   |
| overall test exit code                                     | `EXIT=0` (go-integration: PASS)                   | PASS   |

**DoD impact.** SCOPE-12 DoD item "Routing order verified
adversarially" is flipped to `[x]` in `scopes.md` line 406; live
evidence is the transcript above.

**Files touched (same change set).**

- `docker-compose.yml` — `smackerel-core` `start_period` 15s→120s
  (already landed; verified active).
- `smackerel.sh` — `test integration` case propagates
  `AGENT_SCENARIO_DIR` (anchored to `/workspace`), `ML_SIDECAR_URL`,
  `AGENT_ROUTING_FALLBACK_SCENARIO_ID`, and the full generated test
  env file (`--env-file "$env_file"`) into the Go test container.
- `tests/integration/agent/tool_imports_test.go` — new build-tagged
  blank-import file that bootstraps the production tool registry for
  scenario-loader validation.

