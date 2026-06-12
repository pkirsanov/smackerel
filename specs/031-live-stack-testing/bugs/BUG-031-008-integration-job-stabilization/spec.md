# Spec: BUG-031-008 CI integration job stabilization

Links: [bug.md](bug.md) | [design.md](design.md) | [scopes.md](scopes.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md) | [state.json](state.json)

## Problem

The smackerel `CI` workflow `integration` job is RED on `origin/main` (`28851e7a`)
and has been for 2+ days. Four independent failure clusters keep it red. The job
is the live-stack quality gate owned by spec 031 (`specs/031-live-stack-testing/`);
keeping it green is that spec's charter.

## Goal

Drive the `integration` job to GREEN on `origin/main` by fixing the real root
cause of each cluster — never by skipping, intercepting, or papering over a
genuine failure.

## In Scope

- **C1 (Scope 1):** `smackerel.sh` `auth)` + `connector)` passthrough arms — add `-T`
  to `docker compose exec` so the in-container CLI exit code (2) and stderr banners
  propagate through the non-interactive Go-test / CI caller.
- **C2 (Scope 2):** `tests/integration/api/assistant_transport_hint_test.go` live-stack
  readiness — determine whether core genuinely reaches health (local vs CI) and fix the
  real cause (CI cold-start readiness budget vs a genuinely unhealthy core). No blind
  timeout bump over an unhealthy core.
- **C3a (Scope 3):** `tests/integration/assistant/microtools_registry_canary_test.go` —
  update the stale `microtools_foundation_did_not_register_any_tool` subtest to match
  shipped reality (the four concrete micro-tools are now registered).
- **C3b (Scope 4):** `config/prompt_contracts/weather-query-v1.yaml` — restore
  `location_normalize` to `allowed_tools` so the spec-065-SCOPE-4-derived contract test
  passes; keep the system_prompt shrunk.

## Out of Scope (HARD CONSTRAINT)

- **spec-083 (card-rewards companion) is committed on `origin/main` and MUST NOT be
  touched.** Specifically: `specs/083-card-rewards-companion/*`, `internal/cardrewards/*`,
  `ml/app/card_categories.py`, `ml/app/main.py`, `ml/tests/test_card_categories.py`,
  `tests/integration/cardrewards_extract_test.go`,
  `internal/deploy/docs_connector_count_contract_test.go`, `docs/Development.md`,
  `docs/smackerel.md`. None of the four clusters point there.
- Any production source change beyond the C1 wrapper arms.
- Re-litigating BUG-031-001 / BUG-045-002 (both `done`).
- spec-077 ephemeral-stack lifecycle internals (reused as-is).

## Acceptance

- `./smackerel.sh test integration` exits 0 for the four clusters' tests against the
  real ephemeral test stack (reproduced RED before fix, GREEN after fix, ≥10 lines each).
- C1: `./smackerel.sh --env test auth` and `... auth not-a-real-subcommand` each return
  exit 2 with the expected banner through the non-interactive caller.
- C3a: the canary asserts the four concrete micro-tools ARE registered and stays green.
- C3b: `config/prompt_contracts/weather-query-v1.yaml` `allowed_tools` includes both
  `weather_lookup` and `location_normalize`; the prompt remains ≥40% smaller than the
  pre-spec-065 baseline; `./smackerel.sh check` (scenario-lint) stays green.
- The integration job is GREEN on `origin/main` after the fast-forward push (or the
  honestly-handed-back remainder is documented with exact repro + next owner).
- `artifact-lint.sh` and `state-transition-guard.sh` pass on this bug packet.

## Capability Posture

### Single-Capability Justification

This bug is a single, contained stabilization of one capability — the CI
`integration` job owned by spec 031 — decomposed into five independent
correctness fixes. It introduces NO new capability, foundation, provider,
adapter, strategy, plugin, channel, driver, connector, or variant. The
"provider" / "connector" terms that appear in this packet refer to the
PRE-EXISTING open-meteo geocoder provider (owned by spec 065 / spec 076) and the
existing `smackerel.sh` connector passthrough arm — neither is created,
abstracted, nor given a second implementation here. Each cluster has exactly one
mechanism against an existing surface:

- C1: a docker-absence honest-skip guard in one existing integration test.
- C2: one in-network `CORE_EXTERNAL_URL` override in the existing runner.
- C3a: one corrected assertion in one existing canary test.
- C3b: one restored `allowed_tools` entry in one existing scenario contract.
- C4: one fallback-geocoder honest-skip guard in one existing test.

Because no N≥2 implementation/provider/component/variant set is introduced, a
capability-foundation + concrete-implementations split is not applicable; the
matching single-implementation justification is recorded in design.md.

## Cross-Spec / Cross-Product Impact

- C3a/C3b touch micro-tool / scenario surfaces owned historically by spec-065
  (SCOPE-2..4 superseded → spec-076, which is `done`). spec-076 Scope-3's own
  `tool_registry_canary` already asserts the four micro-tools are registered, so the
  C3a fix aligns this canary with the authoritative owner. No QF cross-product packet
  metadata is touched.
- No change to `config/release-trains.yaml` or feature-flag bundles.
