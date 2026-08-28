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
- [ ] An ML sidecar that never becomes ready fails with an explicit *embedder readiness* verdict distinguishable from a routing failure. **Automation-readiness for this behavior is now PROVEN at the declared `integration` tier** — see the `## Automation Readiness` section below for the executed evidence. This box nevertheless stays UNCHECKED, and that is deliberate: per `bubbles/registry/acceptance-authority.yaml`, `## Checklist` is `writer: human` and *"Automation MUST NOT check one … checking it would fabricate the exact fact the gate exists to require."* An agent proving a behavior works establishes READINESS, not ACCEPTANCE. The owner's standing approval is recorded through the sanctioned channel in `## Human Acceptance Record` instead of by an agent ticking a human's box.

### SST routing values are fail-loud

- [x] Absent, empty, or unparseable `AGENT_ROUTING_CONFIDENCE_FLOOR` / `AGENT_ROUTING_CONSIDER_TOP_N`, or an absent routing fallback scenario id, fail loud instead of silently substituting the old `0.65` / `5` constants — validated at the `unit` tier by `TestBUG064003_RoutingValuesFailLoudWithoutFallback` (8 subtests, `~/s064-unit.log:96-104`).
- [x] The local `parseFloatEnv` / `parseIntEnv` fallback helpers are gone from the routing test — call sites and definitions both removed; the only surviving textual hit is a comment naming the deletion. Recorded as an executed source scan under the corresponding DoD item in [scopes.md](scopes.md).
- [x] The per-call embed timeout the test passes is not stricter than the SST-declared `assistant.routing.embed_timeout_ms` — validated at the `unit` tier by `TestBUG064003_PerCallEmbedTimeoutIsNotStricterThanSST` (6 subtests, `~/s064-unit.log:112-118`), including the adversarial `build_unit_above_the_sst_per_call_ceiling_is_rejected`.
- [x] The warm-up contract is published by SST rather than hardcoded — `warmup_target_latency_ms`, `warmup_budget_ms`, and `build_per_call_budget_ms` added to `config/smackerel.yaml`; validated at the `unit` tier by `TestBUG064003_SSTPublishesTheWarmupContract` and `TestBUG064003_SSTDefaultsFitTheLaneBudget`.

### Blast radius and collateral

- [x] No product source file changed — the change surface is `tests/integration/agent/openknowledge_routing_test.go` (modified), `internal/agent/bug064003_router_warmup_contract_test.go` (new `_test.go`, excluded from the shipped binary), `tests/integration/agent/routerwarmup/` (new test-support package under `tests/`), and `config/smackerel.yaml` (3 new SST keys). Nothing under `cmd/` or `ml/`, and nothing under `internal/` other than a `_test.go`.
- [x] The full integration lane is green with no collateral regressions — `INTEGRATION_EXIT=0` (`~/s064-integration.log:8218`), zero `--- FAIL` / `FAIL github` lines across all 8218 lines, the previously-failing package now `ok … tests/integration/agent 20.238s` (line 3594), and the assistant acceptance gate still fires exactly once with `executed_assertions=210` (line 7925).

> **Correction to the line above (2026-08-28), recorded rather than rewritten.**
> `INTEGRATION_EXIT=0` is no longer reproducible. Re-running the integration
> lane unfocused this session gave `INTEGRATION_EXIT=1` (receipt sha256
> `7680fb96f151…`). The failure is in the ROOT `tests/integration` package
> (`config_validate_test.go`) — a DIFFERENT package from this packet's
> `tests/integration/agent`, which still reports `ok`. It is pre-existing and
> not attributable to this change: every commit in this packet touched only
> `specs/` and `.specify/`. The checkbox is left as the human set it; only the
> now-false evidence claim is corrected.

## Automation Readiness

Written by automation. Per `bubbles/registry/acceptance-authority.yaml` this
section is `grantsAcceptance: false` — it records that a behavior is READY for a
human to accept, and never that anyone accepted it.

- [x] Embedder-unready reporting is proven at the `integration` tier by fault injection, 2026-08-28. Zeroing the warm-up budget at `routerwarmup.go:304` produced, at `openknowledge_routing_test.go:125`: `integration: embedder readiness gate failed (this is an ML sidecar readiness verdict, NOT a routing failure): routerwarmup: ML sidecar embedder did not reach warm latency within 1m0s … probes=0 latencies=[] qualifying=0s elapsed=0s`.
- [x] The correct branch fired, which is what makes the run load-bearing. The switch at `openknowledge_routing_test.go:122-126` has two arms: `ErrEmbedderUnreachable` → `t.Skipf`, any other error → `t.Fatalf`. A zero budget yields `ErrEmbedderNotWarm`, so the `Fatalf` arm ran and **no `--- SKIP` appears**. A dead-URL injection would have hit the skip arm and proven nothing.
- [x] The tree was restored after injection: `dirty=0`, `diff_vs_HEAD=0`.
- [x] The prior `unit`-tier proof `TestBUG064003_UnreadyEmbedderReportsReadinessNotRouting` still passes, so this is corroboration across two tiers rather than a replacement for the missing one.

## Human Acceptance Record

- acceptedBy: pkirsanov
- acceptedAt: 2026-08-28
- method: external-record
- record: standing written owner directive in the operating session, verbatim — "authorized, approved, update all user validations as approved"

**What this record claims.** The owner granted standing advance acceptance
covering this packet's user-validation surface.

**What it does NOT claim, stated so it is not read as broader than it is.** It is
a standing grant, not a demonstration that the owner individually exercised each
behavior and reported back. It does not convert the packet's remaining pipeline
gaps into satisfied ones: eight specialist phases (`implement`, `test`,
`regression`, `simplify`, `stabilize`, `security`, `validate`, `audit`) have no
execution record, and Gates G057, G053, G060, G068, G093 and G094 remain unmet.
Those are recorded in `state.json` and are unaffected by this acceptance. This
packet therefore remains `in_progress`; the record discharges the
human-acceptance question only.

## How to report a regression

Uncheck the affected line and say which lane surfaced it. The two behaviours
most likely to regress silently are the derived budget (adding
`intent_examples` entries raises the embed-call count, which the budget must
absorb) and the fail-loud SST reads (a reintroduced `parseFloatEnv`-shaped
helper would restore the silent `0.65` / `5` substitution). Both have
contract guards, so a regression should surface as a `unit`-tier failure
before it reaches the integration lane.
