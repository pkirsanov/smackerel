# BUG-064-003 — User Validation

> Items default to CHECKED `[x]`: each records a behaviour that was validated in
> this bug's session at the tier named on the line. The operator unchecks `[ ]`
> to report that a behaviour is broken again.
>
> **This is a test-tier + SST fix. No product source file changed**, so there is
> nothing new to observe on the running self-hosted bot and no redeploy is
> required to confirm any item below. Every item is validated by the
> repository's own lanes — `~/s064-integration.log` (8218 lines) and
> `~/s064-unit.log` (564 lines) — read at the line numbers shown. Neither lane
> was re-run in the pass that authored this file.

## Checklist

### The routing test measures routing, not embedder cold start

- [x] `TestOpenKnowledgeRouting_FallbackToOpenKnowledge` reaches and evaluates all three SCOPE-12 routing assertions instead of dying inside the warm-up loop — validated at the `integration` tier: PASS in `11.99s` (`~/s064-integration.log:3561`) with all three subtests PASS (lines 3562–3564). Pre-fix it aborted at ~32 s with `NewRouter: embed scenario … context deadline exceeded`.
- [x] The embedder warm-up gate runs BEFORE the timed region, so cold-start cost is paid outside it — validated at the `integration` tier: `embedder warm-up gate passed: probes=1 latencies=[#1=464ms] qualifying=464ms elapsed=464ms` (line 3552), and at the `unit` tier by `TestBUG064003_WarmupGateIsLoadBearing` and `TestBUG064003_ZeroWarmResultIsStructurallyRefused` (a fabricated zero-value warm result is structurally refused).
- [x] The router-construction budget is derived from the amount of work rather than a fixed wall clock — validated at the `integration` tier: `router construction budget: embed_calls=79 per_call_budget=2s (AGENT_ROUTING_BUILD_PER_CALL_BUDGET_MS) derived_build_budget=2m38s` (line 3554), and at the `unit` tier by `TestBUG064003_DerivedBudgetScalesWithTheWork`.
- [x] No fixed wall-clock literal survives around router construction, and a reintroduced one is mechanically rejected — validated at the `unit` tier by `TestBUG064003_RoutingTestCarriesNoWallClockLiteral`. Both literals named in the fix plan are gone: the `30*time.Second` router wrapper and the `5*time.Second` per-call ceiling.
- [x] An ML sidecar that never becomes ready fails with an explicit *embedder readiness* verdict distinguishable from a routing failure — **now validated at the declared `integration` tier**, 2026-08-28, by the fault-injected run this item itself named as a discharge route. Zeroing the warm-up budget at `routerwarmup.go:304` produced, at `openknowledge_routing_test.go:125`: `integration: embedder readiness gate failed (this is an ML sidecar readiness verdict, NOT a routing failure): routerwarmup: ML sidecar embedder did not reach warm latency within 1m0s ... probes=0 latencies=[] qualifying=0s elapsed=0s`. The wrapped cause is `ErrEmbedderNotWarm`, NOT `ErrEmbedderUnreachable`, so the `t.Skipf` arm was not taken and no `--- SKIP` appears in the run — the distinction this item is about is the one that fired. Tree restored afterwards: `dirty=0`, `diff_vs_HEAD=0`. The prior `unit`-tier proof (`TestBUG064003_UnreadyEmbedderReportsReadinessNotRouting`) still stands and is now corroboration across two tiers rather than a substitute for one.

## Human Acceptance Record

- **acceptedBy:** pkirsanov (repository owner)
- **acceptedAt:** 2026-08-28
- **method:** external-record — standing written directive in the operating session, not a per-behavior interview
- **record (verbatim):** "authorized, approved, update all user validations as approved"

**What this record claims.** The owner granted advance acceptance of the behaviors listed in this checklist, and each listed behavior additionally carries executed evidence recorded against it above.

**What this record does NOT claim, stated so the acceptance is not read as broader than it is.** It is a STANDING grant, not a demonstration that the owner individually exercised each behavior and reported back. It does not convert the packet's remaining pipeline gaps into satisfied ones: eight specialist phases (`implement`, `test`, `regression`, `simplify`, `stabilize`, `security`, `validate`, `audit`) have no execution record, and Gates G057, G053, G068, G093 and G094 remain unmet. Those are recorded in `state.json` and are unaffected by this acceptance. This packet therefore remains `in_progress`; the record discharges the human-acceptance question only.

### SST routing values are fail-loud

- [x] Absent, empty, or unparseable `AGENT_ROUTING_CONFIDENCE_FLOOR` / `AGENT_ROUTING_CONSIDER_TOP_N`, or an absent routing fallback scenario id, fail loud instead of silently substituting the old `0.65` / `5` constants — validated at the `unit` tier by `TestBUG064003_RoutingValuesFailLoudWithoutFallback` (8 subtests, `~/s064-unit.log:96-104`).
- [x] The local `parseFloatEnv` / `parseIntEnv` fallback helpers are gone from the routing test — call sites and definitions both removed; the only surviving textual hit is a comment naming the deletion. Recorded as an executed source scan under the corresponding DoD item in [scopes.md](scopes.md).
- [x] The per-call embed timeout the test passes is not stricter than the SST-declared `assistant.routing.embed_timeout_ms` — validated at the `unit` tier by `TestBUG064003_PerCallEmbedTimeoutIsNotStricterThanSST` (6 subtests, `~/s064-unit.log:112-118`), including the adversarial `build_unit_above_the_sst_per_call_ceiling_is_rejected`.
- [x] The warm-up contract is published by SST rather than hardcoded — `warmup_target_latency_ms`, `warmup_budget_ms`, and `build_per_call_budget_ms` added to `config/smackerel.yaml`; validated at the `unit` tier by `TestBUG064003_SSTPublishesTheWarmupContract` and `TestBUG064003_SSTDefaultsFitTheLaneBudget`.

### Blast radius and collateral

- [x] No product source file changed — the change surface is `tests/integration/agent/openknowledge_routing_test.go` (modified), `internal/agent/bug064003_router_warmup_contract_test.go` (new `_test.go`, excluded from the shipped binary), `tests/integration/agent/routerwarmup/` (new test-support package under `tests/`), and `config/smackerel.yaml` (3 new SST keys). Nothing under `cmd/` or `ml/`, and nothing under `internal/` other than a `_test.go`.
- [x] The full integration lane is green with no collateral regressions — `INTEGRATION_EXIT=0` (`~/s064-integration.log:8218`), zero `--- FAIL` / `FAIL github` lines across all 8218 lines, the previously-failing package now `ok … tests/integration/agent 20.238s` (line 3594), and the assistant acceptance gate still fires exactly once with `executed_assertions=210` (line 7925).

## How to report a regression

Uncheck the affected line and say which lane surfaced it. The two behaviours
most likely to regress silently are the derived budget (adding
`intent_examples` entries raises the embed-call count, which the budget must
absorb) and the fail-loud SST reads (a reintroduced `parseFloatEnv`-shaped
helper would restore the silent `0.65` / `5` substitution). Both have
contract guards, so a regression should surface as a `unit`-tier failure
before it reaches the integration lane.
