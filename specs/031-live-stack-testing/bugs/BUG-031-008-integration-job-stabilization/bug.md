# Bug: BUG-031-008 CI integration job stabilization (6 clusters)

## Summary
The smackerel `CI` workflow `integration` job has been RED on `origin/main`
for 2+ days. The integration job (`.github/workflows/ci.yml` → job
`integration`, gated `if: github.ref == 'refs/heads/main'`, `needs: build`)
brings up the full ephemeral test stack via `./smackerel.sh --env test up`
and then runs `./smackerel.sh test integration`. Six independent failure
clusters keep that job red. Clusters C1–C5 were not *caused* by recent work —
they are pre-existing latent defects + stale tests that were *surfaced* when the
integration gate began running on main (after spec-021 B2 and BUG-073-003
unblocked the gate). Cluster C6 is this-session fallout: BUG-064-002 added a new
REQUIRED `agent.Config.SourcesMax` field with fail-loud G028 validation and updated
the production wiring (`cmd/core/wiring_assistant_openknowledge.go`) + the unit-test
helper (`internal/assistant/openknowledge/agent/agent_test.go` baseCfg), but missed
the integration package's shared `defaultCfg()` helper. C6 was masked by C1–C5 and
only revealed once they were fixed and merged at `75ee520d` — the same
unblock-reveals-next-layer pattern.

This is a NEW bug. The two prior integration bugs under spec 031
(`BUG-031-001-integration-stack-volume-and-migration-hang` and
`specs/045-deploy-resource-filesystem-hardening/bugs/BUG-045-002-ci-integration-failure-persists`)
are both `done` and address stack-volume/migration-hang and CI topology/timeout
respectively — neither covers the four test clusters below.

## Severity
- [ ] Critical - System unusable, data loss
- [x] High - The CI `integration` gate on `main` is red, blocking the merge/promotion signal for every downstream spec
- [ ] Medium - Feature broken, workaround exists
- [ ] Low - Minor issue, cosmetic

## Status
- [x] Reported
- [x] Confirmed
- [x] In Progress
- [x] Fixed
- [ ] Verified
- [ ] Closed

## Affected Tests (6 clusters; reproduction surfaced C4 as separate from C3b; C6 surfaced after the C1–C5 fix merged at `75ee520d`)

| Cluster | Failing test(s) | File | REPRODUCED root cause → fix |
|---------|-----------------|------|----------------|
| C1 | `TestCLIAuthPassthrough_NoArgsExitsTwo`, `_UnknownSubcommandExitsTwo` | `tests/integration/cli_auth_passthrough_test.go` | `docker is required` (exit 1) — containerized go-integration runner has no docker → honest skip-when-no-docker. (NOT `-T`; the wrapper + in-container CLI are correct.) |
| C2 | `TestAssistantTransportHint_WebAndMobileAreAcceptedAsTelemetry`, `_UnknownHintRejectedBeforeFacade` | `tests/integration/api/assistant_transport_hint_test.go` | `CORE_EXTERNAL_URL=127.0.0.1:PORT` (host mapping) unreachable from the compose-network runner → `smackerel.sh` runner in-network override `http://smackerel-core:8080`. (NOT cold-start; core was healthy; reproduces locally.) |
| C3a | `TestMicroToolRegistryCanary_ExistingScenarioToolsStillValidate` (subtest) | `tests/integration/assistant/microtools_registry_canary_test.go` | Stale spec-065 SCOPE-1 assertion → assert location_normalize+entity_resolve register at import, unit_convert+calculator register lazily. |
| C3b | `TestWeatherPromptUsesLocationNormalizeAndShrinksByFortyPercent` (subtest `allowed_tools_lists_location_normalize`) | `tests/integration/assistant/microtools_prompt_contract_test.go` | `location_normalize` missing from weather `allowed_tools` → restore it (config). |
| C4 | `TestLocationNormalizeIntegration_OpenMeteoCanonicalLocations` | `tests/integration/assistant/microtools_location_test.go` | SEPARATE: test-stack geocode stub returns "Reykjavík" for all inputs (F-065-LOCATION-STUB); orphaned superseded spec-065 SCOPE-2 test → skip-when-stub (real-provider coverage owned by spec-076). |
| C6 | `TestOpenKnowledge_HybridInternalAndWeb`, `TestAgent_PerUserMonthlyBudgetExceeded`, `TestAgent_ToolFailureRefusesWithCapture`, `TestAgent_WebSearchDisabledFallsBack` | `tests/integration/openknowledge/{hybrid_answer,monthly_budget,tool_failure,web_search_disabled}_test.go` (all construct via `helpers_test.go` `defaultCfg()`) | All 4 fail at `agent.New`: `Config.SourcesMax must be > 0 (G028 — no silent default)`. BUG-064-002 added the required `SourcesMax` field + updated prod wiring + unit `baseCfg` but missed the integration `defaultCfg()`; revealed after the C1–C5 fix merged at `75ee520d` → add `SourcesMax: 5` to `tests/integration/openknowledge/helpers_test.go` `defaultCfg()` (mirrors `config/smackerel.yaml:899` `assistant.sources_max: 5` + the unit `baseCfg`). The fail-loud `agent.New` G028 check is correct and is NOT weakened. |

## Reproduction Steps
1. Check out `origin/main` (`28851e7a`).
2. Bring up + run the affected tests exactly as the CI integration job does:
   `./smackerel.sh test integration --go-run 'TestCLIAuthPassthrough|TestAssistantTransportHint|TestMicroToolRegistryCanary|TestWeatherPromptUsesLocationNormalize|TestLocationNormalizeIntegration'`
3. Observe the four clusters fail (full evidence captured per-cluster in `report.md`).

## Expected Behavior
The `integration` job is GREEN on `origin/main`: `./smackerel.sh test integration`
exits 0, every cluster's tests pass against the real ephemeral test stack with no
mocks/interception.

## Actual Behavior
- **C1:** `./smackerel.sh --env test auth [<bad-subcmd>]` returns a non-2 exit
  code and no `usage: smackerel auth` / `unknown subcommand` banner, because the
  `auth)`/`connector)` arms of `smackerel.sh` invoke `docker compose exec`
  WITHOUT `-T`; in the non-interactive Go-test / CI context compose aborts with
  `the input device is not a TTY` (exit 1) before the in-container CLI runs.
- **C2:** the two transport-hint tests fail at ~30.02s — `waitLiveStackHealthy(t, stack, 30*time.Second)`
  never sees `/api/health` return 200 in CI (CI log: "ML sidecar readiness timeout"
  + model-envelope validation). Local-vs-CI behavior captured in `report.md`.
- **C3a:** the canary subtest `microtools_foundation_did_not_register_any_tool`
  fails because `location_normalize`/`unit_convert`/`calculator`/`entity_resolve`
  ARE now registered via per-tool `init()` → `agent.RegisterTool`, contradicting
  the frozen spec-065 SCOPE-1 "envelope-only foundation" assumption.
- **C3b:** the subtest `allowed_tools_lists_location_normalize` fails because
  `config/prompt_contracts/weather-query-v1.yaml` `allowed_tools` lists only
  `weather_lookup`; `location_normalize` is missing.

## Environment
- Service: smackerel `CI` workflow `integration` job; `./smackerel.sh test integration`; spec-077 dedicated ephemeral test stack
- Parent owner: `specs/031-live-stack-testing/`
- Baseline: `origin/main` @ `28851e7a`
- Platform: Linux, Docker-backed ephemeral test stack; CI = `ubuntu-latest`, `timeout-minutes: 30`

## Error Output (reproduced this session @ origin/main 28851e7a — full transcripts in report.md)
```text
# C1 — docker absent in the containerized go-integration runner (NOT a -T issue)
cli_auth_passthrough_test.go:104: expected exit code 2 for `auth` with no subcommand, got 1
    output:
    docker is required
--- FAIL: TestCLIAuthPassthrough_NoArgsExitsTwo (0.02s)

# C2 — host-mapped CORE_EXTERNAL_URL unreachable from the compose-network runner (core WAS healthy)
assistant_transport_hint_test.go:112: integration: core not healthy after 30s at http://127.0.0.1:30001
--- FAIL: TestAssistantTransportHint_WebAndMobileAreAcceptedAsTelemetry (30.03s)

# C3a — concrete micro-tools that register at import (stale "must not register" assertion)
microtools_registry_canary_test.go:88: SCOPE-1 must not register "location_normalize"; concrete tools belong to later scopes
microtools_registry_canary_test.go:88: SCOPE-1 must not register "entity_resolve"; concrete tools belong to later scopes

# C3b — location_normalize missing from weather allowed_tools
microtools_prompt_contract_test.go:79: weather scenario allowed_tools = [weather_lookup]; want to include "location_normalize"

# C4 — test-stack geocode stub returns Reykjavík for all inputs (separate from C3b)
microtools_location_test.go:87: name = "Reykjavík", want to contain "Palm Springs"
microtools_location_test.go:90: admin1 = "", want "California"

# C6 — openknowledge integration tests fail at agent.New (SourcesMax unset in integration defaultCfg)
#      verbatim from the CI integration-test-log @ 75ee520d (CI run 27400228490, integration job)
hybrid_answer_test.go:59: agent.New: openknowledge/agent: invalid config: Config.SourcesMax must be > 0 (G028 — no silent default; load from assistant.sources_max)
--- FAIL: TestOpenKnowledge_HybridInternalAndWeb (0.01s)
monthly_budget_test.go:48: agent.New: openknowledge/agent: invalid config: Config.SourcesMax must be > 0 (G028 — no silent default; load from assistant.sources_max)
--- FAIL: TestAgent_PerUserMonthlyBudgetExceeded (0.01s)
tool_failure_test.go:42: agent.New: openknowledge/agent: invalid config: Config.SourcesMax must be > 0 (G028 — no silent default; load from assistant.sources_max)
--- FAIL: TestAgent_ToolFailureRefusesWithCapture (0.01s)
web_search_disabled_test.go:60: agent.New: openknowledge/agent: invalid config: Config.SourcesMax must be > 0 (G028 — no silent default; load from assistant.sources_max)
--- FAIL: TestAgent_WebSearchDisabledFallsBack (0.01s)
FAIL    github.com/smackerel/smackerel/tests/integration/openknowledge  0.055s
```
The verbatim, session-captured reproduction + post-fix transcripts (≥10 lines each) are in `report.md`.
