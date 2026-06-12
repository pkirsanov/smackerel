# Bug: BUG-031-008 CI integration job stabilization (4 clusters)

## Summary
The smackerel `CI` workflow `integration` job has been RED on `origin/main`
for 2+ days. The integration job (`.github/workflows/ci.yml` → job
`integration`, gated `if: github.ref == 'refs/heads/main'`, `needs: build`)
brings up the full ephemeral test stack via `./smackerel.sh --env test up`
and then runs `./smackerel.sh test integration`. Four independent failure
clusters keep that job red. None were *caused* by recent work — they are
pre-existing latent defects + stale tests that were *surfaced* when the
integration gate began running on main (after spec-021 B2 and BUG-073-003
unblocked the gate).

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

## Affected Tests (reproduced — 5 clusters; the reproduction surfaced C4 as separate from C3b)

| Cluster | Failing test(s) | File | REPRODUCED root cause → fix |
|---------|-----------------|------|----------------|
| C1 | `TestCLIAuthPassthrough_NoArgsExitsTwo`, `_UnknownSubcommandExitsTwo` | `tests/integration/cli_auth_passthrough_test.go` | `docker is required` (exit 1) — containerized go-integration runner has no docker → honest skip-when-no-docker. (NOT `-T`; the wrapper + in-container CLI are correct.) |
| C2 | `TestAssistantTransportHint_WebAndMobileAreAcceptedAsTelemetry`, `_UnknownHintRejectedBeforeFacade` | `tests/integration/api/assistant_transport_hint_test.go` | `CORE_EXTERNAL_URL=127.0.0.1:PORT` (host mapping) unreachable from the compose-network runner → `smackerel.sh` runner in-network override `http://smackerel-core:8080`. (NOT cold-start; core was healthy; reproduces locally.) |
| C3a | `TestMicroToolRegistryCanary_ExistingScenarioToolsStillValidate` (subtest) | `tests/integration/assistant/microtools_registry_canary_test.go` | Stale spec-065 SCOPE-1 assertion → assert location_normalize+entity_resolve register at import, unit_convert+calculator register lazily. |
| C3b | `TestWeatherPromptUsesLocationNormalizeAndShrinksByFortyPercent` (subtest `allowed_tools_lists_location_normalize`) | `tests/integration/assistant/microtools_prompt_contract_test.go` | `location_normalize` missing from weather `allowed_tools` → restore it (config). |
| C4 | `TestLocationNormalizeIntegration_OpenMeteoCanonicalLocations` | `tests/integration/assistant/microtools_location_test.go` | SEPARATE: test-stack geocode stub returns "Reykjavík" for all inputs (F-065-LOCATION-STUB); orphaned superseded spec-065 SCOPE-2 test → skip-when-stub (real-provider coverage owned by spec-076). |

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
```
The verbatim, session-captured reproduction + post-fix transcripts (≥10 lines each) are in `report.md`.
