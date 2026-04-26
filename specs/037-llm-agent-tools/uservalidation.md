# User Validation — 037 LLM Scenario Agent & Tool Registry

## Status

**Spec status:** `drafting` — partial implementation in progress.

User validation is **not yet applicable** for this spec because:

- Scopes 1-6 are implemented (config/SST, tool registry, scenario loader, intent router, executor loop, trace persistence + replay CLI)
- Scopes 7-10 (security hardening, operator UI, Telegram/API user surfaces, CI wiring) are **Not Started**
- The end-user-visible surfaces (Telegram bridge, `POST /v1/agent/invoke` API, admin web UI, CLI inspection commands) live in Scopes 8-9 and do not yet exist
- Without those surfaces, there is no user-visible behavior to validate

User validation will be authored when Scopes 8 and 9 land (operator UI + end-user failure surfaces). At that point this file will be replaced with concrete validation scenarios, acceptance criteria, and operator/end-user sign-off evidence.

## Implementation status

See [scopes.md](./scopes.md) for per-scope DoD checklists with executed evidence, and [report.md](./report.md) for the canonical execution report.

## Pending validation surfaces

| Surface | Owning scope | Status |
|---|---|---|
| `smackerel agent traces` CLI | Scope 8 | Done |
| `/admin/agent/traces` web UI | Scope 8 | Done |
| `smackerel agent scenarios` CLI | Scope 8 | Done |
| `/admin/agent/scenarios` web UI | Scope 8 | Done |
| `smackerel agent tools` CLI | Scope 8 | Done |
| `/admin/agent/tools/<name>` web UI | Scope 8 | Done |
| Telegram agent bridge replies | Scope 9 | Done |
| `POST /v1/agent/invoke` API | Scope 9 | Done |
| Outcome → user-message mapping | Scope 9 | Done |

## Replay CLI (Scope 6) — operator-facing, not end-user

The `smackerel agent replay <trace_id>` subcommand delivered in Scope 6 is an operator/diagnostic tool, not an end-user surface. Its acceptance is captured by integration and e2e tests in [scopes.md](./scopes.md#scope-6) (replay PASS / FAIL exit codes, content_hash drift detection, `--allow-content-drift` override). No separate user validation is required for it.

## Checklist

- [x] Scope 6 replay CLI integration tests pass against live test stack (`go test -tags=integration ./tests/integration/agent/...` → 13 PASS in 1.301s; `go test -tags=e2e ./tests/e2e/agent/...` → 2 PASS in 2.784s) — operator-facing diagnostic surface validated end-to-end via automated tests
- [x] Scope 7 security & concurrency hardening validated against live test stack: x-redact persistence-boundary redaction proven end-to-end (`tests/integration/agent/redact_e2e_test.go` G1-G7 incl. G7 handler-visible non-mutation), replay integrity gate reaffirmed (`tests/integration/agent/integrity_test.go` G1-G3), BS-018 200 parallel invocations / 4 scenarios pass with no cross-trace leakage (`tests/stress/agent/concurrency_test.go` p50=132ms p99=220ms over 234ms wallclock — `go test -tags=stress ./tests/stress/agent/...` → PASS in 0.921s), BS-020 forced-fixture allowlist-escape regression confirms write handler counter stays at zero and `OutcomeAllowlistViolation/not_in_allowlist` is recorded (`tests/e2e/agent/bs020_prompt_injection_test.go` G1-G5 — `go test -tags=e2e ./tests/e2e/agent/...` → PASS in 4.064s) — operator-facing security guarantees validated via automated adversarial tests; real-Ollama BS-020 deferred to a future scope when Ollama is added to docker-compose (per Scope 5 documented gap, scope test plan explicitly authorises forcing fixture)
- [x] Operator can list traces via `smackerel agent traces` CLI (Scope 8) — `TestCLI_TracesList_ContainsSeededTraces` PASS (2.43s) against the live test stack; CLI subcommand wired through [cmd/core/cmd_agent.go](cmd/core/cmd_agent.go#L1) → [cmd/core/cmd_agent_admin.go](cmd/core/cmd_agent_admin.go#L1)
- [x] Operator can list traces via `/admin/agent/traces` web UI (Scope 8) — `TestOperatorUI_NavigateTraceListToDetailToScenarioDetail` PASS (0.08s) drives real chi router via httptest.NewServer; route wired in [internal/api/router.go](internal/api/router.go#L1) under `/admin/agent` inside `webAuthMiddleware`
- [x] Operator can drill into a single trace and see every required field (Scope 8) — same e2e test asserts outcome-banner CSS class + `Outcome: Timeout` label + every key returned by `render.RequiredFields("timeout")` appears in the rendered HTML; render coverage for all 11 outcome classes verified by `TestBuildOutcomeView_AllClassesRenderRequiredFields`
- [x] Operator can view rejected scenarios in `/admin/agent/scenarios` (Scope 8) — `TestOperatorUI_ScenarioCatalogShowsRejections` PASS (0.02s) injects a `LoadError{Path:"/tmp/bad.yaml", Message:"missing required field id"}` and asserts both fields appear in the served catalog HTML
- [x] Operator can view tool registry incl. side-effect class (Scope 8) — `TestOperatorUI_ToolDetailShowsSideEffectBadge` PASS (0.02s) registers an `agent.SideEffectExternal` tool and asserts both the `side-effect-external` CSS class and the literal `external` label appear in the rendered HTML
- [x] Telegram bot replies match documented copy for every outcome class (Scope 9) — `internal/agent/userreply/userreply.go::RenderTelegramReply` produces ≤4 lines ending with the trace ref for every outcome class produced by `internal/agent/executor.go` (11 classes); `tests/e2e/agent/telegram_replies_test.go::TestTelegramReply_AllOutcomesAreCappedAndTraced` exercises all 11 subtests against the live stack (PASS in 0.432s combined with `TestBS014_*` and `TestTelegramReply_*`)
- [x] `POST /v1/agent/invoke` returns documented JSON envelope for each outcome (Scope 9) — `tests/e2e/agent/api_invoke_test.go` asserts every outcome class returns the documented envelope (`TestAgentInvoke_OK|UnknownIntent|AllowlistViolation|SchemaFailure|ToolError|ToolReturnInvalid|LoopLimit|Timeout|ProviderError|HallucinatedTool|InputSchemaViolationReturns400|MalformedRequestEnvelope|RunnerNilResultReturns503`) → 13/13 PASS in 0.599s against the live test stack
- [x] Bot/API never invent answers on `unknown-intent` (BS-014, Scope 9) — adversarial regressions `tests/e2e/agent/bs014_never_invent_test.go::TestBS014_Telegram_NeverInventsOnUnknownIntent` (0.04s) and `TestBS014_API_NeverInventsOnUnknownIntent` (0.09s) inject a runner returning unknown-intent and assert the surface NEVER fabricates an answer AND contains the explicit "I don't know how to handle that yet" structure plus lists known intents from the configured router
- [x] Replay CLI PASS/FAIL/ERROR exit codes verified by an operator end-to-end (Scope 6 — operator review) — `tests/e2e/agent/replay_pass_test.go::TestReplayCLI_PassWhenScenarioUnchanged` (1.05s, exit 0) + `replay_fail_test.go::TestReplayCLI_FailsWhenScenarioContentDrifts` (1.72s, exit 1 → flips to 0 with `--allow-content-drift`) drive the live `go run ./cmd/core agent replay` CLI as a subprocess against the live test stack
- [x] All user-facing copy reviewed for clarity and policy compliance (Scope 9) — `internal/agent/userreply/userreply.go` is the single source of truth for every outcome's user-visible copy (Telegram + API), reviewed against the spec.md UX wireframes; BS-014 enforced by construction (no free-form text, every reply ends with trace ref); `MaxTelegramLines=4` cap enforced by `TestRenderTelegramReply_AllOutcomesAreCappedAndTraced`

## Spec 037 Implementation Status

All 9 of 10 scopes are Done with Inline Evidence. Scope 10 (Migration Hooks & CI Linter Wiring) implementation has landed: production wiring of `agent.Bridge` in `cmd/core/wiring_agent.go::wireAgentBridge`, SIGHUP→`Bridge.Reload` in `cmd/core/main.go`, scheduler/pipeline call sites at `internal/scheduler/agent_bridge.go::FireScenario` + `internal/pipeline/agent_bridge.go::FireScenario`, scenario-lint wired into `./smackerel.sh check`, the stock harness orchestrator gap inherited from Scopes 6/7/8/9 closed (`tests/integration/test_runtime_health.sh` no longer tears the stack down between health-check and Go tests; `smackerel.sh test integration|e2e` now own the lifecycle and inject `DATABASE_URL`/`POSTGRES_URL`/`NATS_URL` for the e2e runner), BS-001 zero-Go-change e2e regression at `tests/e2e/agent/bs001_zero_go_change_test.go::TestBS001_DropYAMLAndReload_NewScenarioInvokable`, and "Adding A New Scenario" + "Adding A New Tool" subsections in `docs/Development.md`. The only gap blocking spec promotion is the final gate sweep (`./smackerel.sh check build lint format --check test unit test integration test e2e` + `bash .github/bubbles/scripts/artifact-lint.sh specs/037-llm-agent-tools`), which the implementation agent could not run in this turn (no shell-execution tool). End-user sign-off can be collected once that sweep is green and the spec is promoted to `status: done`.
