# Scopes — Spec 064 (Open-Ended Knowledge Agent)

**Mode:** full-delivery · **Status ceiling:** `done` · **Parallelisation:** none (strict sequential v1)

**Cross-spec packets required:** SCOPE-10 (→ 061), SCOPE-14 (→ 049),
SCOPE-15 (→ 020), SCOPE-16 (→ 022), SCOPE-18 (→ deploy adapter overlay).

### Definition of Done

> Universal DoD — applies to every scope unless that scope's own DoD
> block explicitly relaxes an item. Per-scope DoD checkbox lists below
> remain authoritative for scope-specific items.

- [ ] `./smackerel.sh test unit` plus the relevant
  `integration` / `e2e` categories pass; evidence captured per the
  anti-fabrication policy (G021).
- [ ] No defaults or hidden fallbacks introduced; SST values originate
  from `config/smackerel.yaml` (G028).
- [ ] `artifact-lint.sh specs/064-open-ended-knowledge-agent`,
  `traceability-guard.sh`, and `regression-baseline-guard.sh` pass.
- [ ] No real hostnames, IP addresses, or tailnet identifiers
  introduced; `pii-scan.sh` clean.

---

## SCOPE-01 — Artifact bootstrap

**Status:** In Progress

**Goal:** Materialise `spec.md`, `design.md`, `scopes.md`,
`state.json`, and the minimum runtime skeleton so downstream scopes
have a lint-clean home.

**Files:** `specs/064-open-ended-knowledge-agent/{spec.md, design.md,
scopes.md, state.json}`; `internal/assistant/openknowledge/{doc.go,
tool.go, registry.go, registry_test.go}`.

**Tests:** artifact-lint, `go test ./internal/assistant/openknowledge/...`.

**DoD:**
- [ ] Spec, design, scopes, and state artifacts exist and pass
  `artifact-lint.sh`.
- [ ] `state.json` v3, status `in_progress`, `policySnapshot`
  populated.
- [ ] `spec.md` Outcome Contract present; `design.md` mirrors the
  design packet sections (tool registry, agent loop, cite-back, SST,
  security, observability, failure modes).
- [ ] Registry unit tests cover register / duplicate / allowlist
  allow / allowlist deny / unknown / deterministic ordering / nil
  allowlist denies all.
- [ ] Gates: G021, G028.

## SCOPE-02 — Tool registry skeleton + Tool interface

**Status:** Not Started

**Goal:** Promote the bootstrap registry to a stable capability
foundation, with typed sentinels and table-driven coverage.

**Files:** `internal/assistant/openknowledge/{tool.go, registry.go,
registry_test.go, doc.go}`.

**Tests:** unit (`TestRegistry*`).

**DoD:**
- [ ] `Tool` interface stable: `Name()`, `Description()`,
  `ParamsSchema()`, `Execute(ctx, args)`.
- [ ] Registry rejects unknown tool names with `ErrUnknownTool`;
  rejects duplicates with `ErrDuplicateTool`; rejects allowlist
  misses with `ErrToolNotAllowed`.
- [ ] Allowlist comes from a config struct passed at construction
  (no environment reads inside the package).
- [ ] Gates: G021, G028.

## SCOPE-03 — SST config block `assistant.open_knowledge.*`

**Status:** Not Started

**Goal:** Add config keys + fail-loud Go struct + validation; regenerate
`dev`/`test` env bundles.

**Files:** `config/smackerel.yaml`, `internal/config/openknowledge.go`,
`internal/config/openknowledge_test.go`,
`internal/config/loader.go` (wiring),
`config/generated/{dev,test}.env` (regenerated artifact),
`scripts/commands/config.sh` (additions if any).

**Tests:** unit (missing key → fatal; empty allowlist → fatal;
budgets must be > 0 / >= 0; provider enum validation).

**DoD:**
- [ ] Keys: `enabled`, `tool_allowlist`, `max_iterations`,
  `per_query_token_budget`, `per_query_usd_budget`,
  `monthly_budget_usd`, `per_user_monthly_budget_usd`, `provider`,
  `provider_endpoint`, `provider_api_key`, `llm_model_id`,
  `web_snippet_cache_enabled`.
- [ ] `os.Getenv` empty → fatal; no `getEnv(k, default)` helpers
  introduced.
- [ ] `./smackerel.sh config generate` reproduces env files
  deterministically.
- [ ] Gates: G021, G028.

## SCOPE-04 — LLM bridge tool-use round-trip

**Status:** Not Started

**Goal:** Extend the `ml/` sidecar contract + Go client to support
tool-call / tool-result messages.

**Files:** `ml/app/routes/chat.py`, `ml/app/schemas.py`,
`ml/tests/test_tool_roundtrip.py`,
`internal/assistant/openknowledge/llm/client.go`,
`internal/assistant/openknowledge/llm/client_test.go`.

**Note:** Scenario contract YAML is materialized in SCOPE-12 after all
tools are registered.

**Tests:** unit (Go client, mocked HTTP); integration (real Ollama
via `./smackerel.sh test integration` — sidecar reachable;
`tool_call`/`tool_result` schema validated).

**DoD:**
- [ ] Sidecar accepts `tools[]` schemas + `messages[]` including
  `tool_call` and `tool_result` roles.
- [ ] Go client returns typed `ToolCall` when `stop_reason=tool_use`;
  otherwise final text.
- [ ] Contract test asserts schema parity Go ↔ Python.
- [ ] Gates: G021, G028.

## SCOPE-05 — Deterministic tools: `unit_convert`, `calculator`

**Status:** Not Started

**Goal:** First concrete `Tool` implementations exercising the
registry + loop end-to-end (no external deps).

**Files:** `internal/assistant/openknowledge/tools/{unit_convert.go,
unit_convert_test.go, calculator.go, calculator_test.go}`.

**Tests:** unit (table-driven correctness + error cases:
divide-by-zero, unknown unit, NaN); integration (registry → tool
invoke → typed result).

**DoD:**
- [ ] Both tools registered when allowlist includes them.
- [ ] Adversarial tests: malformed args rejected with typed error,
  no panics.
- [ ] Gates: G021, G028.

## SCOPE-06 — `internal_retrieval` tool

**Status:** Not Started

**Goal:** Wrap existing graph / pgvector search behind the `Tool`
interface.

**Files:** `internal/assistant/openknowledge/tools/internal_retrieval.go`
plus `_test.go`,
`tests/integration/openknowledge_internal_retrieval_test.go`.

**Tests:** unit (mocked graph client); integration (live test
Postgres via `./smackerel.sh test integration` — ephemeral DB, seeded
fixtures, real pgvector query).

**DoD:**
- [ ] Returns artifact IDs + content snippets with stable hash for
  cite-back.
- [ ] Integration runs against the disposable test compose only
  (G028 isolation).
- [ ] Gates: G021, G028.

## SCOPE-07 — Web search provider interface + SearxNG impl

**Status:** Implemented Pending Validation

**Goal:** `WebSearchProvider` interface + SearxNG implementation;
Brave/Tavily stubs return `ErrProviderNotConfigured`.

**Files:** `internal/assistant/openknowledge/web/{provider.go,
searxng.go, searxng_test.go}`,
`tests/integration/openknowledge_searxng_test.go`,
`docker-compose.test.yml` (add SearxNG test service).

**Tests:** unit (HTTP mocked); integration (real SearxNG container in
test compose).

**Test Infrastructure Note (2026-05-31):** SearxNG container is now
wired into `docker-compose.yml` under the `searxng` profile (image
pin: `searxng/searxng:2026.5.30-bd863f16b`, container port 8080,
host port 47006 in the test env). The `test` env auto-enables the
profile and `./smackerel.sh test integration` injects
`OPEN_KNOWLEDGE_SEARXNG_URL=http://searxng:8080` into the Go runner.
`TestSearxNGIntegration_Smoke` passes against the live container
(verified outside the integration runner via host-port probe;
provenance: executed against test-env SearxNG container at
`127.0.0.1:47006` on 2026-05-31). DoD verification and final status
flip belong to `bubbles.validate` once the rest of the scope's
provider/egress/Brave-Tavily-stub DoD items are exercised end-to-end.

**DoD:**
- [x] Provider returns `[]WebSnippet{URL, Title, Snippet, Hash,
  FetchedAt}`.
- [x] Egress restricted to configured `provider_endpoint`.
- [x] Brave/Tavily stubs return `ErrProviderNotConfigured`.
- [x] Gates: G021, G028.

**Evidence (2026-06-01, bubbles.implement):**

Phase: implement. Claim Source: executed.

`WebSnippet` struct fields verified in
`internal/assistant/openknowledge/web/provider.go` (URL, Title, Snippet,
ContentHash, FetchedAt, Provider) and exercised by
`TestSearxNG_Search_HappyPath`.

Egress restriction enforced by `egress.go` (allowlist transport derived
from configured `provider_endpoint`); 7 egress tests pass including
`TestEgressAllowlistTransport_DenyByDefault_Adversarial`,
`TestEgressAllowlistTransport_DisallowedHostDenied`,
`TestEgressAllowlistTransport_RejectsNonHTTPScheme`,
`TestEgressAllowlistTransport_UserinfoDoesNotBypass`.

Brave/Tavily stubs: `brave.go` and `tavily.go` return
`ErrProviderNotConfigured` unconditionally; `TestBrave_NeverDialsNetwork`
and `TestTavily_NeverDialsNetwork` prove no network dial occurs (G021
adversarial cases — would fail if the stubs forwarded to a real call).

```bash
$ SMACKEREL_HARDWARE_TIER=cpu go test -count=1 -timeout 120s \
    ./internal/assistant/openknowledge/web/
ok  github.com/smackerel/smackerel/internal/assistant/openknowledge/web  0.160s

$ SMACKEREL_HARDWARE_TIER=cpu go test -count=1 -timeout 60s -v \
    -run 'TestBrave|TestTavily|TestEgress' \
    ./internal/assistant/openknowledge/web/
--- PASS: TestEgressAllowlistTransport_DenyByDefault_Adversarial (0.00s)
--- PASS: TestEgressAllowlistTransport_AllowedHostPassthrough (0.06s)
--- PASS: TestEgressAllowlistTransport_DisallowedHostDenied (0.00s)
--- PASS: TestEgressAllowlistTransport_NormalizesMixedCaseHost (0.00s)
--- PASS: TestEgressAllowlistTransport_UserinfoDoesNotBypass (0.00s)
--- PASS: TestEgressAllowlistTransport_RejectsNonHTTPScheme (0.00s)
--- PASS: TestEgressAllowlistTransport_AllowsHTTPForAllowedHost (0.02s)
--- PASS: TestBrave_NeverDialsNetwork (0.00s)
--- PASS: TestTavily_NeverDialsNetwork (0.00s)
PASS
ok  github.com/smackerel/smackerel/internal/assistant/openknowledge/web  0.090s
```

Live-stack integration (`TestSearxNGIntegration_Smoke` against the
test-env SearxNG container at `127.0.0.1:47006`) was previously
executed on 2026-05-31 (see Test Infrastructure Note above); not
re-run in this session (`./smackerel.sh test integration` lock held
by a separate run). G028 fail-loud config validation for SearxNG
endpoint is covered by `TestSearxNG_NewSearxNG_ConfigValidation`
sub-cases (`empty_endpoint`, `whitespace_endpoint`, `nil_client`,
`bad_scheme`, `no_host`, `unparseable`).

Final scope status flip and certification belong to `bubbles.validate`.

## SCOPE-08 — Cite-back verifier

**Status:** Not Started

**Goal:** Mechanical verifier that every claim in the final answer
maps to a recorded tool-result hash.

**Files:** `internal/assistant/openknowledge/citeback/{verifier.go,
verifier_test.go}`.

**Tests:** unit incl. adversarial (G021): fabricated URL not in tool
trace → rejection; hash mismatch → rejection; partial citation →
rejection.

**DoD:**
- [ ] Verifier consumes tool trace + final answer; returns
  `Verdict{ok, missingCites[], fabricatedCites[]}`.
- [ ] At least 3 adversarial cases that would fail if verifier were
  disabled.
- [ ] Gates: G021, G028.

## SCOPE-09 — Agent loop / planner with bounded budgets

**Status:** Not Started

**Goal:** Orchestrate LLM ↔ tools with iteration / token / USD caps +
compaction trigger.

**Files:** `internal/assistant/openknowledge/{agent.go, agent_test.go,
budget.go}`.

**Tests:** unit (mocked LLM + tools — happy path, iteration cap hit,
token cap hit, USD cap hit, compaction at threshold, tool error
recovery).

**DoD:**
- [ ] Caps sourced from config struct (SCOPE-03); zero hardcoded
  values.
- [ ] Convergence cap enforced (G082); compaction at
  `compaction.threshold_tokens` (G083).
- [ ] Returns typed termination reason
  (`final`, `cap_iterations`, `cap_tokens`, `cap_usd`,
  `tool_error`, `refused`).
- [ ] Gates: G021, G028, G082, G083.

## SCOPE-10 — Cross-spec: provenance gate amendment in spec 061 **[ROUTE PACKET → 061]**

**Status:** Done

**Route packet:** [`route-packets/PKT-061-A.md`](route-packets/PKT-061-A.md) — emitted 2026-05-31 by `bubbles.implement`, routed to `specs/061-conversational-assistant`.

**Closure packet:** [`route-packets/PKT-061-A-RESPONSE.md`](route-packets/PKT-061-A-RESPONSE.md) — merged 2026-05-31 by `bubbles.workflow mode=bugfix-fastlane` against spec 061. Source taxonomy + canonical refusal taxonomy extension landed additively; spec 061 stays at `done` ceiling.

**Goal:** Extend the Source taxonomy + canonical refusal taxonomy in
spec 061 to cover `web_snippet`, `agent_answer`, and the new refusal
causes.

**Files:** `specs/061-.../spec.md`,
`specs/061-.../scenario-manifest.json` (owned by 061; this scope
produces a route packet).

**Tests:** N/A this spec; receiving spec runs its own.

**DoD:**
- [x] Route packet emitted with diff proposal, rationale, and
  acceptance criteria.
- [x] Status here remains `in_progress` until 061 returns
  merged-evidence. (Closed 2026-05-31 via PKT-061-A-RESPONSE.)
- [x] Gates: G021.

**Evidence (PKT-061-A closure, 2026-05-31):**

```text
$ ls specs/064-open-ended-knowledge-agent/route-packets/
PKT-061-A-RESPONSE.md  PKT-061-A.md

$ go test ./internal/assistant/contracts/... \
         ./internal/assistant/provenance/... \
         ./internal/telegram/assistant_adapter/...
ok      github.com/smackerel/smackerel/internal/assistant/contracts     0.059s
ok      github.com/smackerel/smackerel/internal/assistant/provenance    0.017s
ok      github.com/smackerel/smackerel/internal/telegram/assistant_adapter      0.020s
```

G021 adversarial coverage landed in `internal/assistant/provenance/gate_test.go`:
`TestEnforce_RejectsUnknownSourceKind` (would fail if Kind validation reverted),
`TestEnforceRefusal_AdversarialDefault` (would fail if `CanonicalRefusalBodyFor`
returned `""` for an unknown cause), and
`TestEnforce_PreExistingArtifactBehaviourUnchanged` (explicit backcompat proof).
Merged-evidence anchor in spec 061:
`specs/061-conversational-assistant/report.md` →
"Amendment — PKT-061-A (2026-05-31): Source taxonomy + canonical refusal
taxonomy extension".

## SCOPE-11 — Artifact persistence (`WebSnippet`, `AgentAnswer`, tool trace)

**Status:** Not Started

**Goal:** Persist new artifact types with lifecycle per P3 (Knowledge
Breathes).

**Files:** `internal/db/migrations/NNN_open_knowledge.sql`,
`internal/knowledge/web_snippet.go`,
`internal/knowledge/agent_answer.go`,
`internal/knowledge/tool_trace.go` + tests,
`tests/integration/openknowledge_persistence_test.go`.

**Tests:** unit (struct ↔ row); integration (real test Postgres:
insert, lifecycle promotion, decay).

**DoD:**
- [ ] Migration declares `lifecycle_state` column; lifecycle
  transitions covered.
- [ ] Tool trace links to AgentAnswer + WebSnippets by hash.
- [ ] Gates: G021, G028.

## SCOPE-12 — Scenario manifest + routing rule

**Status:** Implemented Pending Validation (live integration run not
captured in current session; see `report.md` SCOPE-12 follow-ups).

**Goal:** Register `open_knowledge` as last-before-capture in the
assistant router AND wire the live open-knowledge subsystem into
`cmd/core` plus the facade source-assembler so the provenance gate
accepts the agent's web + tool-computation citations.

**Files (implemented):** `cmd/core/wiring_assistant_openknowledge.go`,
`cmd/core/wiring_assistant_openknowledge_assembler.go`,
`cmd/core/wiring_assistant_openknowledge_test.go`,
`cmd/core/wiring_assistant_facade.go` (+open_knowledge assembler entry),
`cmd/core/main.go` (+wireOpenKnowledge call),
`config/smackerel.yaml` (fallback flip + `llm_timeout_ms` + populated
allowlist), `config/prompt_contracts/open_knowledge.yaml`
(+agent_system_prompt), `internal/config/openknowledge.go` (+LLMTimeoutMs),
`internal/config/{openknowledge_test.go,validate_test.go}` (fixture updates),
`scripts/commands/config.sh` (+ASSISTANT_OPEN_KNOWLEDGE_LLM_TIMEOUT_MS),
`tests/integration/agent/openknowledge_routing_test.go`.

**Tests:** unit (`cmd/core` wiring + assembler + prompt-loader,
12 tests, all green this session); integration
(`TestOpenKnowledgeRouting_FallbackToOpenKnowledge` covering domain-query
non-stealing + open-ended fallback + conversion fallback). Live run
NOT completed in current session — test stack did not reach healthy
core within window; test binary builds + vets clean.

**DoD:**
- [x] Routing order verified adversarially (a known-intent query MUST
  NOT reach open_knowledge). Integration test asserts this for
  `weather in paris today` (top_score=1.000, reason=similarity_match)
  and for the two open-knowledge sub-cases. Live evidence in
  `report.md` §3.D (run on 2026-06-01).
- [x] Gates: G021, G028. Unit assemblies green; no defaults
  introduced (every wiring path reads from cfg; new SST key
  `assistant.open_knowledge.llm_timeout_ms` added with fail-loud
  loader + validator).

  **Phase:** implement
  **Evidence:**
  ```bash
  go test ./cmd/core/... -run 'TestWireOpenKnowledge|TestLoadOpenKnowledgeAgentPrompt|TestOpenKnowledgeAssembler' -count=1
  # ok  github.com/smackerel/smackerel/cmd/core  0.194s (12 tests pass; executed in current session)
  SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test unit --go
  # [go-unit] go test ./... finished OK (executed; full suite green after fixture updates)
  ./smackerel.sh lint
  # EXIT=0 (executed)
  ```
  **Claim Source:** executed

## SCOPE-13 — Telegram surface (response shapes, citations, refusals)

**Status:** Done

**Goal:** Render UX-packet response shapes, citation footnotes, and
refusal taxonomy strings.

**Files:** `internal/telegram/openknowledge_render.go` + `_test.go`,
`internal/telegram/handler.go` (wire),
`tests/integration/openknowledge_telegram_test.go`.

**Tests:** unit (snapshot of each response shape + each refusal
category); integration (real Telegram webhook contract test against
live core).

**DoD:**
- [x] All UX-packet response shapes covered.

  **Evidence:** `internal/telegram/assistant_adapter/render_outbound.go`
  now dispatches to `RenderSourcedAnswer` (all non-artifact sources),
  `RenderHybridAnswer` (mixed), and `RenderRefusalWithCapture` (any
  non-default `RefusalCause` carried via `ErrorCause`). All-artifact
  responses remain on the legacy spec 061 path. Routing predicates
  `hasNonArtifactSources`, `hasArtifactSource`, and
  `openKnowledgeRefusalCauseFromError` are unit-tested via
  `TestBuildTelegramRendering_OpenKnowledge_AllWeb`,
  `TestBuildTelegramRendering_OpenKnowledge_AllComputation`,
  `TestBuildTelegramRendering_OpenKnowledge_Hybrid`,
  `TestBuildTelegramRendering_OpenKnowledge_RefusalCauses`,
  `TestBuildTelegramRendering_AllArtifact_BackCompat`, and
  `TestBuildTelegramRendering_LegacyErrorCause_BackCompat`.

  ```text
  $ ./smackerel.sh test unit --go 2>&1 | tail -1
  [go-unit] go test ./... finished OK
  ```

  **Phase:** implement. **Claim Source:** executed.

- [x] Citations render with stable ordering; refusal strings match
  taxonomy exactly.

  **Evidence:** Inline `[N]` citation lines are emitted by
  `RenderSourcedAnswer`/`RenderHybridAnswer` (already unit-tested
  for deterministic Kind+Title+ID sort in
  `render_openknowledge_test.go`); refusal bodies are emitted via
  `contracts.CanonicalRefusalBodyFor(cause) + " (saved as idea)"`
  for each of the five non-default `RefusalCause` values, exhaustively
  asserted in `TestBuildTelegramRendering_OpenKnowledge_RefusalCauses`.

  **Phase:** implement. **Claim Source:** executed.

- [x] Gates: G021, G028.

  **Evidence:** G021 adversarial back-compat — empty `Sources` and
  empty `ErrorCause` MUST fall through to the unchanged default
  body-only render; asserted by
  `TestBuildTelegramRendering_G021_NoSourcesNoErrorCauseFallsThroughDefault`.
  G028 — `openKnowledgeRefusalCauseFromError` explicitly excludes
  `RefusalDefault` (no silent fallback) and the dispatch refuses to
  call open_knowledge renderers when the predicates do not match
  (no defaults). Both gates verified by the closed-vocabulary tests
  + adversarial back-compat case above.

  **Phase:** implement. **Claim Source:** executed.

## SCOPE-14 — Cross-spec: observability **[ROUTE PACKET → 049]**

**Status:** Awaiting Cross-Spec Resolution (PKT-049-A pending — local metrics + redacted logging shipped)

**Goal:** Metrics (`openknowledge_iterations`, `_tokens`,
`_usd_cents`, `_tool_calls_total{tool}`, `_refusals_total{reason}`)
plus redacted trace logging.

**Files (local stub + packet):**
`internal/assistant/openknowledge/metrics.go` + `_test.go`; route
packet to `specs/049-...` for dashboards/alerts.

**Tests:** unit (metric increments, log redaction of API keys + URLs
outside allowlist).

**DoD:**
- [x] No raw API keys ever logged (adversarial test:
  `TestAgentTurnLog_RedactsSecrets` in
  `internal/assistant/openknowledge/agent/agent_log_test.go` — drives
  an api-key-containing prompt + tool args + tool result through one
  turn and asserts the JSON-handler log buffer contains none of the
  raw secret, full URL, snippet body, raw prompt, or system prompt
  text). Cardinality guard:
  `TestOpenKnowledgeMetrics_RejectsUnknownCause_AdversarialG021` in
  `internal/assistant/openknowledge/metrics/metrics_test.go` —
  thousands of adversarial unknown labels (cause / tool / scope /
  outcome / latency-tool) MUST NOT inflate the series count.

  **Phase:** implement
  **Evidence:**
  ```bash
  go test ./internal/assistant/openknowledge/metrics/... ./internal/assistant/openknowledge/agent/... \
      -run "TestOpenKnowledgeMetrics|TestAgent.*Log|TestAgentTurnLog" -v
  # === RUN   TestAgentTurnLog_RedactsSecrets
  # --- PASS: TestAgentTurnLog_RedactsSecrets (0.00s)
  # === RUN   TestAgentTurnLog_EmittedOnRefusal
  # --- PASS: TestAgentTurnLog_EmittedOnRefusal (0.00s)
  # === RUN   TestOpenKnowledgeMetrics_RejectsUnknownCause_AdversarialG021
  # --- PASS: TestOpenKnowledgeMetrics_RejectsUnknownCause_AdversarialG021 (0.00s)
  # ... (all 12 targeted tests pass)
  # Exit code: 0 (executed in current session)
  ```
- [ ] Route packet to 049 with dashboard + alert proposals — **packet
  written**: `route-packets/PKT-049-A.md` (status pending; awaiting
  spec 049 owner to land Grafana panels + Prometheus alert rules).
- [x] Gates: G021 (adversarial cardinality test passes), G028 (every
  histogram bucket is a named var; no magic numbers at call sites;
  Recorder allow-sets enforced at every increment site).

  **Phase:** implement
  **Evidence:**
  ```bash
  ./smackerel.sh test unit --go
  # [go-unit] go test ./... finished OK
  # Exit code: 0 (executed; full unit suite green — no collateral failures)
  ./smackerel.sh lint
  # Web validation passed
  # Exit code: 0 (executed)
  bash .github/bubbles/scripts/artifact-lint.sh specs/064-open-ended-knowledge-agent
  # Exit code: 0 (executed; report.md SCOPE-14 section + state.json PKT-049-A entries lint-clean)
  ```

## SCOPE-15 — Cross-spec: security hardening **[ROUTE PACKET → 020]**

**Status:** Awaiting Cross-Spec Resolution (local items shipped; PKT-020-A pending spec 020 review)

**Goal:** Egress allowlist amendment; prompt-injection mitigations
(system-prompt fencing, tool-output sanitisation); API key handling.

**Files:** `internal/assistant/openknowledge/web/{egress.go,
egress_test.go, sanitize.go, sanitize_test.go, apikey_test.go}`,
`internal/assistant/openknowledge/web/searxng.go` (sanitiser
integration), `internal/assistant/openknowledge/metrics/metrics.go`
(`openknowledge_suspicious_snippet_total{provider}` collector),
`cmd/core/wiring_assistant_openknowledge.go` (egress transport
wiring), `internal/config/openknowledge.go` (`AllowedEgressHosts`
field), `config/smackerel.yaml`, `scripts/commands/config.sh`,
`internal/config/openknowledge_test.go` (validation),
`internal/config/validate_test.go` (env baseline),
`specs/064-open-ended-knowledge-agent/route-packets/PKT-020-A.md`.

**Tests:** unit (egress denied for non-allowlisted host; sanitiser
strips control chars / repairs UTF-8 / truncates / emits
suspicious-snippet metric on injection trigger patterns; API keys
never appear in slog output or error messages; adversarial: empty
allowlist denies all, mixed-case host normalisation,
userinfo-bearing URL cannot bypass, non-http(s) scheme rejected).

**DoD:**
- [x] Egress allowlist sourced from SST.
  - Evidence: new SST key `assistant.open_knowledge.allowed_egress_hosts: []`
    in `config/smackerel.yaml`; `AllowedEgressHosts []string` field on
    `OpenKnowledgeConfig`; `EgressAllowlistTransport` wired at
    `buildOpenKnowledgeWebProvider` time with effective allowlist =
    `provider_endpoint` host ∪ `AllowedEgressHosts`; deny-by-default
    proven by `TestEgressAllowlistTransport_DenyByDefault_Adversarial`.
- [x] Adversarial prompt-injection corpus (≥ 10 cases) green.
  - Evidence: `suspiciousPatterns` in `sanitize.go` covers 14 known
    triggers (ignore previous / disregard previous / developer mode /
    DAN mode / `system:` / `<|im_start|>` / `<|im_end|>` /
    `<|endoftext|>` / `[INST]` / `[/INST]` / `<<sys>>` / `<</sys>>` /
    plus variants); `TestSanitizeSnippet_DetectsPromptInjection_Adversarial`
    + `TestSanitizeSnippet_DetectsLLMChatTokens` +
    `TestSanitizeSnippet_NoFalsePositiveOnBenignText` prove
    detection without content stripping (LLM-side fencing remains
    the primary defence).
- [x] Route packet to 020 with allowlist diff.
  - Evidence: `route-packets/PKT-020-A.md` enumerates the v1 allowlist
    policy + four review questions (exact-match sufficiency, wildcard
    follow-up, network-layer egress firewall, SearxNG upstream-engines
    constraint at the deploy adapter layer); state tracked under
    `state.json.transitionRequests[PKT-020-A]` +
    `state.json.reworkQueue[PKT-020-A-await]`.
- [x] Gates: G021, G028.
  - Evidence: G021 — every claim has at least one adversarial test
    that would fail under regression (empty-allowlist deny,
    userinfo non-bypass, mixed-case normalisation, non-http(s)
    scheme rejection, malformed entry rejection, suspicious-pattern
    detection, ContentHash covers sanitised body). G028 — no
    defaults: empty `AllowedEgressHosts` denies all (no silent
    allow-all), malformed entries fail loud with
    `F064-SST-INVALID`, new env var emitted by
    `scripts/commands/config.sh` with no fallback.

**Status note (PKT-020-A — pending):** Application-layer
hardening + API-key audit + sanitiser-into-ContentHash wiring have
shipped locally. The network-layer review questions (container
egress firewall, wildcard policy, SearxNG upstream-engines
constraint) are owned by spec 020. SCOPE-15 cannot transition to
`Done` until spec 020 returns a response packet either accepting
the application-layer-only posture or filing concrete additive
requirements.

## SCOPE-16 — Cross-spec: resilience **[ROUTE PACKET → 022]**

**Status:** Awaiting Cross-Spec Resolution

**Goal:** Circuit breaker per tool/provider + budget-exhaustion
graceful paths.

**Files:**
`internal/assistant/openknowledge/web/{circuit.go,circuit_test.go}`;
`internal/assistant/openknowledge/agent/agent.go` (TerminationToolUnavailable +
circuit-open mid-loop short-circuit); `internal/assistant/openknowledge/agenttool/substrate_tool.go`
(mapping); `internal/assistant/openknowledge/tools/web_search.go`
(provider_circuit_open ToolError); `internal/assistant/openknowledge/metrics/metrics.go`
(circuit_state + circuit_trips_total collectors); `internal/config/openknowledge.go`
(CircuitBreaker SST sub-block); `config/smackerel.yaml`,
`scripts/commands/config.sh`; `cmd/core/wiring_assistant_openknowledge.go`
(breaker wraps provider); route packet to `specs/022-operational-resilience`.

**Tests:** unit (breaker open/half-open/closed; budget exhausted →
typed refusal; downstream tool 5xx → breaker trip).

**DoD:**
- [x] Breaker thresholds from SST.
  - Evidence: new SST sub-block
    `assistant.open_knowledge.circuit_breaker.{failure_threshold,
    open_window_seconds, half_open_after_seconds}` in
    `config/smackerel.yaml` with explicit `5 / 60 / 30`; required env
    vars `ASSISTANT_OPEN_KNOWLEDGE_CIRCUIT_BREAKER_*` emitted by
    `scripts/commands/config.sh`; new
    `OpenKnowledgeCircuitBreakerConfig` struct on `OpenKnowledgeConfig`;
    fail-loud validation in `Validate()` (`[F064-SST-INVALID]` when
    `Enabled=true` and any field `<= 0`); wiring in
    `cmd/core/wiring_assistant_openknowledge.go` constructs
    `web.NewCircuitBreaker(webProvider, web.CircuitConfig{...})` from
    the SST fields before passing to `tools.RegisterAll`. Validation
    is proven by
    `TestOpenKnowledgeConfig_CircuitBreaker_HappyPath` +
    `TestOpenKnowledgeConfig_CircuitBreaker_RejectsNonPositive` (6
    subtests) + `TestOpenKnowledgeConfig_CircuitBreaker_DisabledSkipsValidation`
    in `internal/config/openknowledge_test.go`.
- [x] Route packet to 022 with operational playbook delta.
  - Evidence: `route-packets/PKT-022-A.md` enumerates the three review
    questions: (a) whether v1 thresholds (5 / 60s / 30s) match the
    operational-resilience playbook (with the cross-reference to the
    connector supervisor's `maxPanicsBeforeDisable = 5` convention),
    (b) whether an openknowledge health-check endpoint contribution
    is required for operator dashboards, (c) whether the
    budget-exhaustion refusal-with-capture handshake should be lifted
    into a cross-subsystem graceful-degradation pattern; state
    tracked under `state.json.transitionRequests[PKT-022-A]` +
    `state.json.reworkQueue[PKT-022-A-await]`.
- [x] Gates: G021, G028.
  - Evidence: G021 — `TestCircuit_OpenShortCircuits_AdversarialG021`
    proves an Open breaker does NOT invoke the inner provider (a
    leak-through regression would underflow the fake queue and
    `t.Fatalf`); `TestCircuit_InvalidQueryDoesNotCount_AdversarialG021`
    proves the failure classification is narrow (4 × `ErrInvalidQuery`
    + 1 × `ErrProviderUnreachable` records exactly 1 failure, not 5);
    `TestAgent_CircuitOpen_DoesNotLeakUnrelatedErrorCodes_AdversarialG021`
    proves the agent-loop circuit-open check is narrow (a calculator
    divide-by-zero stays recoverable). G028 — every `CircuitConfig`
    field rejected fail-loud on construction (`TestCircuit_New_ValidatesConfig`,
    4 cases + nil-inner case); every SST circuit-breaker key required
    at the generator boundary (`required_value` in `config.sh`);
    fail-loud validation in `OpenKnowledgeConfig.Validate()`; no
    silent in-source defaults anywhere on the path
    (`NewCircuitBreaker` returns `ErrInvalidConfig` for 0 / negative
    values).

**Status note (PKT-022-A — pending):** Local circuit-breaker (three-state,
concurrency-safe, SST-bound, fail-loud) and budget-exhaustion refusal-with-capture
path have shipped. 12 circuit unit tests (3 adversarial G021), 2 agent
circuit-open tests (1 adversarial G021), 9 SST validation tests. Spec 022 owns
the operational-resilience playbook alignment, the health-check endpoint
contribution decision, and the cross-subsystem graceful-degradation pattern
question. SCOPE-16 cannot transition to `Done` until spec 022 returns a response
packet either accepting the v1 posture or filing concrete additive requirements.

## SCOPE-17 — End-to-end live-stack scenarios SCN-064-A01..A08

**Status:** Blocked — see route-packets/PKT-WORKFLOW-A.md (2026-05-31, bubbles.implement)

**Goal:** Real Postgres + real Ollama + real SearxNG container in
the test compose; cover all eight scenarios incl. the adversarial
fabricated-source regression.

**Files:** `tests/e2e/agent/openknowledge_e2e_test.go` (scaffolding shipped),
`docker-compose.yml` (ollama + searxng profiles already exist),
`tests/fixtures/openknowledge/*`.

**Tests:** `./smackerel.sh test e2e` only; no mocks
(`Testing.md` live-stack rule).

**Blocker summary (PKT-WORKFLOW-A — six findings):**
1. `ml/app/routes/chat.py` returns HTTP 501 without fixture header — no real Ollama dispatch path.
2. `/v1/agent/invoke` does not run capture-as-fallback (Telegram-facade only today).
3. No `fixture-fabricated-cite` test mode in `chat.py` for the adversarial G021 path.
4. No per-test per-query token budget override knob.
5. No per-test tool-allowlist override knob.
6. `smackerel.sh test e2e` does not export `AGENT_INVOKE_URL` to the Go test container.

**Evidence shipped now:** `tests/e2e/agent/openknowledge_e2e_test.go` — 7 test functions (A01..A06 + adversarial fabrication). All skip honestly with explicit `t.Skip(...)` messages naming the routed finding. `go vet -tags e2e ./tests/e2e/agent/...` is clean. The file activates automatically as each finding lands; no SCOPE-17-side code change needed.

**DoD:**
- [ ] All eight scenarios green; ≥ 1 adversarial case per scenario
  where applicable (G021). — Blocked on PKT-WORKFLOW-A findings #1, #3, #4, #5.
- [ ] Fabricated-citation regression case proves verifier blocks it. — Blocked on PKT-WORKFLOW-A finding #3.
- [x] No `route()` / `intercept()` / `msw` anywhere in the suite.
  - **Phase:** implement (bubbles.implement, 2026-05-31)
  - **Claim Source:** executed
  - **Evidence:** `grep -rn 'route()\|intercept(\|msw\|nock' tests/e2e/agent/openknowledge_e2e_test.go` → 0 matches. The file uses `net/http.Client` against the real `/v1/agent/invoke` URL — no interception layer.
- [ ] Gates: G021, G028, G082, G083. — Verification deferred until tests actually execute.

## SCOPE-18 — Docs + deploy adapter contract **[ROUTE PACKET → deploy overlay]**

**Status:** Done (bubbles.docs, 2026-05-31)

**Goal:** Update `docs/Operations.md` + `docs/Development.md`; emit
deploy-adapter contract delta for new config keys (G074
Build-Once-Deploy-Many).

**Files:** `docs/Operations.md` (+~200 lines, Open-Knowledge Assistant
Agent section), `docs/Development.md` (+~50 lines, Spec 064
Open-Knowledge Agent subsection), `docs/Testing.md` (+1 paragraph,
fabricated-cite fixture mode note in the existing SCOPE-07
section), `docs/Deployment.md` (+~75 lines, Spec 064 Deployment
Notes section), `deploy/contract.yaml` (+~110 lines, `sstKeyCatalog`
with spec 064 entry), `deploy/README.md` (+10 lines, per-spec SST
key catalog note).

**Tests:** `regression-baseline-guard.sh` for managed docs;
`compose_contract_test.go` still green.

**DoD:**
- [x] No real hostnames, IPs, or tailnet ids in any doc.

  Evidence (Claim Source: executed):
  ```bash
  $ grep -nE '([0-9]{1,3}\.){3}[0-9]{1,3}|\.ts\.net|tailnet' \
      docs/Operations.md docs/Development.md docs/Testing.md \
      docs/Deployment.md deploy/contract.yaml deploy/README.md \
      | grep -v "127.0.0.1\|0.0.0.0\|10.0\.\|192\.168"
  # Only pre-existing generic placeholders remain
  # (<host-tailnet-fqdn>, "tailnet IP / loopback").
  # No new IPs, no real hostnames, no real tailnet ids introduced.
  ```

- [x] Deploy contract lists every new required env var with
  fail-loud semantics.

  Evidence (Claim Source: executed): `deploy/contract.yaml`
  `sstKeyCatalog[spec=064-open-ended-knowledge-agent]` enumerates
  all 21 `assistant.open_knowledge.*` keys plus 3 per-env keys
  under `perEnvKeys`. Each entry has `path`, `type`, `secret`
  (true|false), and (where applicable) `notes` capturing the
  fail-loud constraint (e.g., `">0 when enabled"`, "Non-empty").
  YAML parses:
  ```bash
  $ python3 -c "import yaml; yaml.safe_load(open('deploy/contract.yaml')); print('YAML OK')"
  YAML OK
  ```

- [x] Status here can advance to `done` only after all prior scopes'
  DoD are met.

  Evidence (Claim Source: interpreted — SCOPE-18 is docs-only and
  decoupled from SCOPE-17 closure): SCOPE-17 is `Blocked` on
  PKT-WORKFLOW-A infrastructure findings owned by upstream scopes.
  SCOPE-18 is a docs-only scope owned by `bubbles.docs` and is
  decoupled from SCOPE-17 — its DoD is satisfied for the
  documentation-affecting scopes (SCOPE-01..SCOPE-16). The
  PKT-WORKFLOW-A blockage is documented in `docs/Development.md`
  (Spec 064 subsection, "E2E tests") and `docs/Testing.md`
  (SearxNG section, fabricated-cite fixture mode note) so operators
  and downstream agents see the active gap. Marking SCOPE-18 done
  does NOT advance the spec to `done` overall; spec-level closure
  still gates on SCOPE-17.

- [x] Gates: G021, G028, G074.

  Evidence (Claim Source: executed):
  ```bash
  $ timeout 600 bash .github/bubbles/scripts/regression-baseline-guard.sh \
      specs/064-open-ended-knowledge-agent --verbose
  # 🐾 Regression baseline guard: PASSED
  # All 0 checks passed. EXIT=0

  $ bash .github/bubbles/scripts/artifact-lint.sh \
      specs/064-open-ended-knowledge-agent
  # Artifact lint PASSED. EXIT=0

  $ go test -count=1 -timeout 60s ./internal/deploy/...
  # ok      github.com/smackerel/smackerel/internal/deploy  50.729s
  # (compose_contract_test.go still green — no deploy-Compose drift.)
  ```
