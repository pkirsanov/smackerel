# Scopes: BUG-064-003 — router warm-up exceeds a fixed deadline in the SCOPE-12 routing test

**Bug:** [bug.md](bug.md) · **Spec:** [spec.md](spec.md) · **Design:** [design.md](design.md)
**Workflow mode:** `bugfix-fastlane`

> Implementation has landed and the fix is verified at the integration and unit
> tiers. **10 of 15** Definition-of-Done checkboxes below are checked, each with
> its own inline raw-output evidence. The remaining **5** are unchecked and each
> states its reason inline — none is checked on inference. This scope is **not**
> marked Done and `state.json` is **not** promoted: certification is owned by
> `bubbles.validate`.
>
> Note for the reviewer: [report.md](report.md) still describes the pre-fix
> artifacts-only session and is now stale with respect to this file. It was
> outside this pass's permitted edit set and is flagged rather than silently
> left to disagree.

---

## Scope 1: Make the SCOPE-12 routing test's verdict independent of embedder cold start

**Status:** In Progress

**foundation: true** — this scope establishes the `routerwarmup.BuildRouter`
seam described under `## Capability Foundation` in [design.md](design.md). It is
tagged foundation because it is the scope that CREATES the seam, not because a
family of implementations is planned: [design.md](design.md) records exactly one
concrete implementation and [spec.md](spec.md) records why one is correct under
`### Single-Capability Justification`. There are therefore no overlay scopes, and
none is owed a `Depends On` back-reference.

**Depends On:** nothing — this IS the foundation scope, so no overlay scope declares a Depends On against the foundation scope in this packet.
**Depends On:** none

### Gherkin Scenarios (regression contract)

```gherkin
Feature: BUG-064-003 — SCOPE-12 routing test measures routing, not warm-up

  Scenario: SCN-064-003-01 cold sidecar no longer fails the routing assertions
    Given a freshly created test stack whose ML sidecar embedder is cold
    And the scenario set declares 79 intent_examples across all scenarios
    When TestOpenKnowledgeRouting_FallbackToOpenKnowledge runs
    Then the test reaches all three routing assertions
    And it does not fail with "NewRouter: embed scenario ... context deadline exceeded"

  Scenario: SCN-064-003-02 an unready embedder reports itself as such
    Given an ML sidecar that never becomes ready
    When TestOpenKnowledgeRouting_FallbackToOpenKnowledge runs
    Then the failure message names embedder readiness as the cause
    And the message is distinguishable from a routing-assertion failure

  Scenario: SCN-064-003-03 adversarial — a reintroduced fixed wall-clock literal is rejected
    Given a maintainer wraps agent.NewRouter in a literal context.WithTimeout again
    When the BUG-064-003 contract guard runs
    Then the guard FAILS and names the offending file and line

  Scenario: SCN-064-003-04 adversarial — a missing SST routing value fails loud
    Given AGENT_ROUTING_CONFIDENCE_FLOOR is absent from the environment
    When the routing test resolves its RoutingConfig
    Then it fails loud naming the missing variable
    And it does NOT silently substitute the constant 0.65

  Scenario: SCN-064-003-05 the per-call embed timeout is not stricter than SST
    Given assistant.routing.embed_timeout_ms is 30000
    When the routing test constructs its sidecar embedder
    Then the per-call timeout it passes is not below that SST value
```

### Implementation Plan

1. Replace the single probe embed (lines 92–96) with a bounded embedder-readiness
   gate that returns an explicit, distinguishable "embedder not ready" error.
2. Start the router-construction deadline only after readiness is established,
   and derive its magnitude from the intent-example count rather than the
   `30*time.Second` literal at line 125.
3. Raise the `sidecar.New` per-call timeout (line 89) so it is not stricter than
   `ASSISTANT_ROUTING_EMBED_TIMEOUT_MS`.
4. Read `AGENT_ROUTING_CONFIDENCE_FLOOR` and `AGENT_ROUTING_CONSIDER_TOP_N`
   fail-loud, mirroring `internal/agent/config.go:155-156`; delete the local
   `parseFloatEnv` / `parseIntEnv` fallback helpers (lines 217, 229).
5. Add the BUG-064-003 contract guard that mechanically rejects reintroduction of
   the fixed literal and of the env-fallback helper shape.

Ordering note: step 5 must land RED against the pre-fix tree, otherwise it is
tautological and cannot detect regression.

### Test Plan

Five rows. The "Test DoD items" group below contains four checkboxes mapping
1:1 to rows T1-T4 in order; T5 is a planning row whose DoD items sit in the
"Regression E2E coverage" group and are deliberately still open.

| # | Test Type | Category | File / Location | Description | Command | Live System |
|---|-----------|----------|-----------------|-------------|---------|-------------|
| T1 | Integration | `integration` | `tests/integration/agent/openknowledge_routing_test.go` | On a cold test stack the test reaches and evaluates all three SCOPE-12 routing assertions (SCN-064-003-01) | `./smackerel.sh test integration --go-run 'TestOpenKnowledgeRouting'` | Yes |
| T2 | Integration | `integration` | `tests/integration/agent/openknowledge_routing_test.go` | An embedder that never becomes ready produces an explicit readiness failure, not a routing failure (SCN-064-003-02) | `./smackerel.sh test integration --go-run 'TestOpenKnowledgeRouting'` | Yes |
| T3 | Unit (adversarial contract guard) | `unit` | new `*_bug064003_test.go` under `internal/agent/` | Source-shape guard: FAILS if a fixed wall-clock literal wraps `agent.NewRouter` in the SCOPE-12 integration test, or if a local env-fallback helper is reintroduced (SCN-064-003-03) | `./smackerel.sh test unit --go --go-run BUG064003` | No |
| T4 | Unit (adversarial) | `unit` | new `*_bug064003_test.go` under `internal/agent/` | Absent/unparseable `AGENT_ROUTING_CONFIDENCE_FLOOR` or `AGENT_ROUTING_CONSIDER_TOP_N` fails loud rather than substituting `0.65` / `5`; per-call embed timeout is not stricter than the SST value (SCN-064-003-04, SCN-064-003-05) | `./smackerel.sh test unit --go --go-run BUG064003` | No |
| T5 | Regression E2E | `e2e-api` | NOT YET AUTHORED - see the "Regression E2E coverage" DoD group | Regression: a full-stack run re-proves SCN-064-003-01 and SCN-064-003-02 end to end, so the warm-up contract is protected above the integration tier as well | `./smackerel.sh test e2e` | Yes |
| T6 | Stress | `stress` | NOT YET AUTHORED - see the "SLA / stress coverage" DoD group | This scope declares latency budgets (`warmup_target_latency_ms`, `warmup_budget_ms`, `build_per_call_budget_ms`), so it owes a stress run proving the derived build budget still absorbs the embed-call count under a loaded sidecar rather than only on an idle one | `./smackerel.sh test stress` | Yes |

### Definition of Done

#### Test DoD items (exactly 4 — 1:1 with Test Plan rows T1–T4)

- [x] **T1** integration: cold-stack run evaluates all three routing assertions
  - **Claim Source:** executed · **Tree:** WORKING TREE, HEAD=3af96a02 · **Exit code:** `0`
  - **Command:** `./smackerel.sh test integration` — preserved at `~/s064-integration.log` (8218 lines), read this session at the line numbers shown; the lane was not re-run.
  - Raw output evidence (inline, ≥10 lines, no references or summaries):
    ```
    ### V1 - the readiness gate ran first, then the budget was derived from the work
    3552:    openknowledge_routing_test.go:127: embedder warm-up gate passed: probes=1 latencies=[#1=464ms] qualifying=464ms elapsed=464ms
    3554:    openknowledge_routing_test.go:149: router construction budget: embed_calls=79 per_call_budget=2s (AGENT_ROUTING_BUILD_PER_CALL_BUDGET_MS) derived_build_budget=2m38s

    ### V2 - all three SCOPE-12 routing assertions were REACHED and evaluated
    3556:    openknowledge_routing_test.go:186: query="weather in paris today" → weather_query (top_score=1.000, reason=similarity_match)
    3558:    openknowledge_routing_test.go:186: query="explain quantum entanglement briefly" → open_knowledge (top_score=0.248, reason=fallback_clarify)
    3560:    openknowledge_routing_test.go:186: query="what is 10F in C" → open_knowledge (top_score=0.763, reason=similarity_match)

    ### V3 - verdict: parent and all three subtests PASS; package green
    3561:--- PASS: TestOpenKnowledgeRouting_FallbackToOpenKnowledge (11.99s)
    3562:    --- PASS: TestOpenKnowledgeRouting_FallbackToOpenKnowledge/weather-domain-query-does-not-route-to-open-knowledge (0.08s)
    3563:    --- PASS: TestOpenKnowledgeRouting_FallbackToOpenKnowledge/open-ended-knowledge-question-routes-to-open-knowledge (0.03s)
    3564:    --- PASS: TestOpenKnowledgeRouting_FallbackToOpenKnowledge/deterministic-tool-question-routes-to-open-knowledge (0.04s)
    3594:ok      github.com/smackerel/smackerel/tests/integration/agent  20.238s
    ```
  - SCN-064-003-01 is discharged: the gate qualified on probe #1 at 464 ms, construction then ran under a 2m38s budget derived from `embed_calls=79`, and the test spent 11.99 s reaching a routing verdict rather than dying at ~32 s inside the warm-up loop.
- [x] **T2** integration: unready embedder produces an explicit readiness failure
  - **Claim Source:** executed 2026-08-28 via **fault-injected integration run** — the first of the two discharge routes this row itself named. The row is discharged at its DECLARED `integration` tier; it was NOT reclassified to `unit`.
  - **Why a green run could never discharge this.** The branch is a guard clause, not a subtest: it fires only when the embedder fails to warm. The healthy lane passes the gate (`probes=1 ... qualifying=464ms`), so it structurally cannot exercise the branch. Fault injection is the only way to reach it.
  - **Fault injected, and chosen to hit the right branch.** `tests/integration/agent/routerwarmup/routerwarmup.go:304` `deadline := started.Add(t.WarmupBudget)` → `deadline := started`, zeroing the warm-up budget. This matters for correctness of the proof: the switch at `openknowledge_routing_test.go:122-126` has TWO arms, and only one is T2 — `errors.Is(err, ErrEmbedderUnreachable)` takes a `t.Skipf`, while any other error takes the `t.Fatalf` readiness verdict. Pointing at a dead URL would have hit the SKIP arm and proven nothing. A zero budget yields `ErrEmbedderNotWarm` — reachable, but never warm — which is precisely the condition the row describes.
  - **Command:** `./smackerel.sh test integration --go-run TestOpenKnowledgeRouting_FallbackToOpenKnowledge` · **Exit Code:** 1

    ```
    === RUN   TestOpenKnowledgeRouting_FallbackToOpenKnowledge
        openknowledge_routing_test.go:125: integration: embedder readiness gate failed
        (this is an ML sidecar readiness verdict, NOT a routing failure): routerwarmup:
        ML sidecar embedder did not reach warm latency within 1m0s (1s target from
        AGENT_ROUTING_WARMUP_TARGET_LATENCY_MS, budget from AGENT_ROUTING_WARMUP_BUDGET_MS):
        probes=0 latencies=[] qualifying=0s elapsed=0s: routerwarmup: ML sidecar
        embedder did not reach warm latency
    --- FAIL: TestOpenKnowledgeRouting_FallbackToOpenKnowledge (0.00s)
    FAIL
    FAIL    github.com/smackerel/smackerel/tests/integration/agent  0.195s
    T2_FAULT_EXIT=1
    ```

  - **Four independent details confirm this is T2 and not something adjacent.** (1) The failure originates at `openknowledge_routing_test.go:125`, the exact assertion line the row cites. (2) The verdict text is the readiness-versus-routing sentence verbatim, so the distinction the row is about is the one that fired. (3) The wrapped cause is `ML sidecar embedder did not reach warm latency` — `ErrEmbedderNotWarm`, NOT `ErrEmbedderUnreachable` — so the SKIP arm was not taken; there is no `--- SKIP` anywhere in the run. (4) `probes=0 ... elapsed=0s` shows the zeroed budget took effect, and the message names its SST sources (`AGENT_ROUTING_WARMUP_TARGET_LATENCY_MS`, `AGENT_ROUTING_WARMUP_BUDGET_MS`), confirming the timings are SST-derived rather than literals.
  - **Isolation verified.** `dirty_before=0`; after `git restore`, `dirty=0` and `diff_vs_HEAD=0`. The injection never entered a commit.
  - The `unit`-tier corroboration still stands and is now the second of two tiers rather than a substitute: `TestBUG064003_UnreadyEmbedderReportsReadinessNotRouting` PASS, subtests `responds_but_never_reaches_warm_latency` and `every_probe_times_out`.
- [x] **T3** unit: contract guard rejects a reintroduced fixed wall-clock literal
  - **Claim Source:** executed · **Tree:** WORKING TREE, HEAD=3af96a02 · **Terminal marker:** `[go-unit] go test ./... finished OK`
  - **Command:** `./smackerel.sh test unit --go --go-run 'BUG064003|RouterWarmup|Warmup' --verbose` — preserved at `~/s064-unit.log` (564 lines), read this session.
  - Raw output evidence (inline, ≥10 lines, no references or summaries):
    ```
    ### V1 - selector and the exact go invocation
     51:[go-unit] applying -run selector: BUG064003|RouterWarmup|Warmup
     53:+ go test -v -run 'BUG064003|RouterWarmup|Warmup' -count=1 ./...

    ### V2 - the source-shape guard for the reintroduced fixed literal (SCN-064-003-03)
    119:=== RUN   TestBUG064003_RoutingTestCarriesNoWallClockLiteral
    120:--- PASS: TestBUG064003_RoutingTestCarriesNoWallClockLiteral (0.00s)

    ### V3 - companion guards that make the literal unnecessary rather than merely banned
    121:=== RUN   TestBUG064003_SSTPublishesTheWarmupContract
    122:--- PASS: TestBUG064003_SSTPublishesTheWarmupContract (0.00s)
    123:=== RUN   TestBUG064003_DerivedBudgetScalesWithTheWork
    124:--- PASS: TestBUG064003_DerivedBudgetScalesWithTheWork (0.00s)
    125:=== RUN   TestBUG064003_SSTDefaultsFitTheLaneBudget
    126:--- PASS: TestBUG064003_SSTDefaultsFitTheLaneBudget (0.02s)

    ### V4 - package verdict and lane terminator
    128:ok      github.com/smackerel/smackerel/internal/agent   1.682s
    564:[go-unit] go test ./... finished OK
    ```
  - **Note on the exit marker.** `~/s064-unit.log` contains **no** `UNIT_EXIT=` line; that marker was not written by this runner. The success evidence quoted is the runner's own terminator at line 564 plus the `ok` package verdict at line 128. No `--- FAIL` / `FAIL github` line appears anywhere in the file.
- [x] **T4** unit: missing SST routing values fail loud; per-call timeout ≥ SST value
  - **Claim Source:** executed · **Tree:** WORKING TREE, HEAD=3af96a02 · **Terminal marker:** `[go-unit] go test ./... finished OK`
  - **Command:** `./smackerel.sh test unit --go --go-run 'BUG064003|RouterWarmup|Warmup' --verbose` — preserved at `~/s064-unit.log`, read this session.
  - Raw output evidence (inline, ≥10 lines, no references or summaries):
    ```
    ### V1 - SCN-064-003-04: absent / empty / unparseable SST routing values fail loud (8 subtests)
     96:--- PASS: TestBUG064003_RoutingValuesFailLoudWithoutFallback (0.00s)
     97:    --- PASS: TestBUG064003_RoutingValuesFailLoudWithoutFallback/complete_environment_resolves (0.00s)
     98:    --- PASS: TestBUG064003_RoutingValuesFailLoudWithoutFallback/confidence_floor_absent (0.00s)
     99:    --- PASS: TestBUG064003_RoutingValuesFailLoudWithoutFallback/confidence_floor_empty (0.00s)
    100:    --- PASS: TestBUG064003_RoutingValuesFailLoudWithoutFallback/confidence_floor_unparseable (0.00s)
    101:    --- PASS: TestBUG064003_RoutingValuesFailLoudWithoutFallback/consider_top_n_absent (0.00s)
    102:    --- PASS: TestBUG064003_RoutingValuesFailLoudWithoutFallback/consider_top_n_empty (0.00s)
    103:    --- PASS: TestBUG064003_RoutingValuesFailLoudWithoutFallback/consider_top_n_unparseable (0.00s)
    104:    --- PASS: TestBUG064003_RoutingValuesFailLoudWithoutFallback/fallback_scenario_id_absent (0.00s)

    ### V2 - SCN-064-003-05: the per-call embed timeout is not stricter than the SST value (6 subtests)
    112:--- PASS: TestBUG064003_PerCallEmbedTimeoutIsNotStricterThanSST (0.00s)
    113:    --- PASS: TestBUG064003_PerCallEmbedTimeoutIsNotStricterThanSST/build_unit_above_the_sst_per_call_ceiling_is_rejected (0.00s)
    114:    --- PASS: TestBUG064003_PerCallEmbedTimeoutIsNotStricterThanSST/build_unit_below_the_proven_warm_latency_is_rejected (0.00s)
    115:    --- PASS: TestBUG064003_PerCallEmbedTimeoutIsNotStricterThanSST/absent_ASSISTANT_ROUTING_EMBED_TIMEOUT_MS (0.00s)
    116:    --- PASS: TestBUG064003_PerCallEmbedTimeoutIsNotStricterThanSST/absent_AGENT_ROUTING_WARMUP_TARGET_LATENCY_MS (0.00s)
    117:    --- PASS: TestBUG064003_PerCallEmbedTimeoutIsNotStricterThanSST/absent_AGENT_ROUTING_WARMUP_BUDGET_MS (0.00s)
    118:    --- PASS: TestBUG064003_PerCallEmbedTimeoutIsNotStricterThanSST/absent_AGENT_ROUTING_BUILD_PER_CALL_BUDGET_MS (0.00s)
    ```
  - Both halves of the row are covered: the `*_absent` / `*_empty` / `*_unparseable` subtests are the adversarial cases that fail if the deleted `0.65` / `5` fallbacks are ever reintroduced, and `build_unit_above_the_sst_per_call_ceiling_is_rejected` is the adversarial case for a per-call unit exceeding `embed_timeout_ms`.

#### Scenario traceability items (one per Gherkin scenario)

Each item cites its scenario id so the mapping is explicit rather than inferred
from word overlap.

- [x] **SCN-064-003-01** cold sidecar no longer fails the routing assertions - the cold-stack run reaches and evaluates all three SCOPE-12 routing assertions. Evidence: [report.md](report.md) `### Test Evidence` - T1, `ok github.com/smackerel/smackerel/tests/integration/agent 1.664s`, 3 tests + 6 subtests, 0 FAIL / 0 SKIP.
- [x] **SCN-064-003-02** an unready embedder reports itself as such - proven at the `integration` tier by fault injection, not merely asserted. Zeroing the warm-up budget at `routerwarmup.go:304` produced the explicit readiness verdict at `openknowledge_routing_test.go:125` via the `ErrEmbedderNotWarm` branch, with no `--- SKIP`. Tree restored, `diff_vs_HEAD=0`. Evidence: [report.md](report.md) `### Self-Correction` and `## Automation Readiness` in [uservalidation.md](uservalidation.md).
- [x] **SCN-064-003-03** adversarial - a reintroduced fixed wall-clock literal is rejected. Evidence: [report.md](report.md) `### RED → GREEN ordering` - T3 mutation kill. Injecting `agent.NewRouter(` caused the guard to fail with its own message, *"calls agent.NewRouter directly; it must go through routerwarmup.BuildRouter"*. The token was asserted ABSENT before injection, so a vacuous survival was impossible.
- [x] **SCN-064-003-04** adversarial - a missing SST routing value fails loud. Evidence: [report.md](report.md) `### RED → GREEN ordering` - T4 mutation kill, and the kill was DISCRIMINATING: replacing the `EnvConfidenceFloor` miss-append with `floor = 0.65` failed exactly `confidence_floor_absent` and `confidence_floor_empty` (2 of 8) while `confidence_floor_unparseable`, every `consider_top_n_*` and `fallback_scenario_id_absent` still passed.
- [x] **SCN-064-003-05** the per-call embed timeout is not stricter than SST - evidence: [report.md](report.md) `### Test Evidence` - T4, `TestBUG064003_PerCallEmbedTimeoutIsNotStricterThanSST`, 6 subtests including the adversarial `build_unit_above_the_sst_per_call_ceiling_is_rejected`.

#### Regression E2E coverage

Both items are deliberately UNCHECKED. They are real outstanding work, not a
formality, and they are the only reason this scope is not Done.

- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior - T5 is not authored yet. Honest position: this fix changed NO product source (only an integration test, a new `_test.go` contract guard, a `tests/`-scoped support package, and 3 SST keys), so the integration tier is currently the highest tier that exercises it. That is an argument for E2E being low-value here, NOT evidence that it exists, so the box stays open rather than being talked shut.
- [ ] Broader E2E regression suite passes - not runnable as a clean signal today. The e2e lane has two recorded defects of its own: the ollama model pull runs AFTER the block that needs it (R-010) and `go-e2e-stack-start` was SIGKILLed at exit 137 (R-011), both in `.specify/memory/open-work.md`. Claiming a green broader suite before those are fixed would be a false green.

#### SLA / stress coverage

This scope is SLA-sensitive: it introduces three latency budgets into SST, so it
owes stress evidence rather than only idle-path evidence.

- [ ] Stress coverage for the declared latency SLAs - T6 is not authored yet. Honest position: every timing figure recorded in this packet (`464ms` qualifying warm latency, `2s` per-call budget, `2m38s` derived build budget for 79 embed calls) was measured on an otherwise IDLE sidecar. That says nothing about behaviour under concurrent load, which is exactly when a derived budget is most likely to be wrong. The gap is recorded rather than closed by assertion.

#### Regression-integrity items

- [x] Root cause confirmed against the running system (79 sequential embeds under one 30 s deadline)
  - **Claim Source:** executed · **Tree:** WORKING TREE, HEAD=3af96a02
  - **Command:** read the post-fix census from `~/s064-integration.log` and the two pre-fix aborts from `~/bug011-integration-a9.log` and `~/bug011-flake-check.log`; all three logs read this session at the line numbers shown.
  - Raw output evidence (inline, ≥10 lines, no references or summaries):
    ```
    ### V1 - the 79-embed census, measured on the live stack (post-fix run)
    3554:    openknowledge_routing_test.go:149: router construction budget: embed_calls=79 per_call_budget=2s (AGENT_ROUTING_BUILD_PER_CALL_BUDGET_MS) derived_build_budget=2m38s

    ### V2 - pre-fix run A: aborted at scenario "recipe_search" under the fixed deadline
    3964:=== RUN   TestOpenKnowledgeRouting_FallbackToOpenKnowledge
    3965:    openknowledge_routing_test.go:128: build router: agent: NewRouter: embed scenario "recipe_search" intent_examples[4]: sidecar.Embed: POST /embed: Post "http://smackerel-ml:8081/embed": context deadline exceeded
    3966:--- FAIL: TestOpenKnowledgeRouting_FallbackToOpenKnowledge (32.14s)

    ### V3 - pre-fix run B: same failure mode, DIFFERENT abort point in the same loop
    293:=== RUN   TestOpenKnowledgeRouting_FallbackToOpenKnowledge
    294:    openknowledge_routing_test.go:128: build router: agent: NewRouter: embed scenario "hospitality_concern_evaluate" intent_examples[1]: sidecar.Embed: POST /embed: Post "http://smackerel-ml:8081/embed": context deadline exceeded
    295:--- FAIL: TestOpenKnowledgeRouting_FallbackToOpenKnowledge (33.40s)

    ### V4 - the SST value the single fixed deadline contradicted
    config/smackerel.yaml:1628:    embed_timeout_ms: 30000 # REQUIRED: per-call timeout for the sidecar /embed roundtrip. Spec 064 SCOPE-17 raised from 500ms to accommodate cold sentence-transformer load on first startup; subsequent calls return in <50ms.
    ```
  - The 79-call count is measured, not inferred (V1). Two consecutive pre-fix runs aborted mid-loop at **different** scenarios (V2, V3) — that divergence is what makes the cause the shared deadline rather than any one slow scenario. V4 is the contradiction: production budgets 30 s for **one** cold call; the test budgeted 30 s for all 79.
- [x] Pre-fix state: T3 and T4 FAIL against the unmodified tree (proving they are not tautological)
  - **Claim Source:** executed 2026-08-28. **Mechanism differs from the one this item originally proposed — stated here rather than glossed.** The item asked for the guards run against a checkout without the fix. What was executed instead is a MUTATION proof: each guard was run against a tree carrying a deliberately reintroduced defect, and each guard killed its mutant with a matching failure signature. The item's stated purpose — *proving they are not tautological* — is what a mutant kill establishes directly. A reviewer who requires the pre-fix-checkout form specifically should treat this as adjacent evidence, not identical evidence.
  - **T3 — `TestBUG064003_RoutingTestCarriesNoWallClockLiteral` · mutant KILLED**

    ```
    $ python3 -c "... assert 'agent.NewRouter(' not in s; append '// MUTATION-PROBE agent.NewRouter('"
    mutation applied
    dirty_after_mutation=1
    $ ./smackerel.sh test unit --go --go-run TestBUG064003_RoutingTestCarriesNoWallClockLiteral --verbose
    openknowledge_routing_test.go calls agent.NewRouter directly; it must go through
    routerwarmup.BuildRouter so the derived budget and the warm-up precondition ...
    --- FAIL: TestBUG064003_RoutingTestCarriesNoWallClockLiteral (0.00s)
    FAIL    github.com/smackerel/smackerel/internal/agent   0.091s
    T3_MUTANT_EXIT=1
    ```

    The mutation asserted the token was ABSENT before injecting it, so a no-op mutation (which would have produced a vacuous survival) was structurally impossible.
  - **T4 — `TestBUG064003_RoutingValuesFailLoudWithoutFallback` · mutant KILLED, and killed precisely.** The mutation replaced the fail-loud `missing = append(missing, EnvConfidenceFloor)` in `tests/integration/agent/routerwarmup/routerwarmup.go:204` with a silent `floor = 0.65` — the exact defect this guard exists to forbid.

    ```
    $ ./smackerel.sh test unit --go --go-run TestBUG064003_RoutingValuesFailLoudWithoutFallback --verbose
    bug064003_router_warmup_contract_test.go:317: confidence_floor_absent did not ...
    bug064003_router_warmup_contract_test.go:317: confidence_floor_empty did not ...
    --- FAIL: TestBUG064003_RoutingValuesFailLoudWithoutFallback (0.00s)
        --- FAIL: .../confidence_floor_absent
        --- FAIL: .../confidence_floor_empty
    FAIL    github.com/smackerel/smackerel/internal/agent   0.033s
    T4_MUTANT_EXIT=1
    ```

    The kill is discriminating, not blanket: `confidence_floor_unparseable` still PASSED, because `"not-a-float"` fails parsing regardless of the substituted default. Exactly the two subtests the mutation could affect failed, and no others. A guard that failed everything would not have demonstrated this.
  - **Isolation verified.** `dirty_before=0` in both cases; after `git restore`, `dirty_after_restore=0`, `diff_vs_HEAD=0`, and the injected token count returned to `0`. No mutant escaped into the committed tree.
  - Why this was not produced as a mechanical receipt: `.github/bubbles/scripts/mutation-receipt.sh` is present and was invoked first, but returned `adapter=none  receipts=skipped  records=0` — the default-OFF switch. Earning machine-checkable receipts needs a project-owned `mutationExecution.command` executable, which is a new capability outside this bug's Change Boundary rather than a config flip.
- [x] Post-fix state: T1–T4 PASS
  - **Claim Source:** executed 2026-08-28. **4 of 4**, not 3 of 4 — the composite is checked only because T2 was discharged at its declared tier, which the previous revision of this entry set as the precondition.
  - **T1** — `ok github.com/smackerel/smackerel/tests/integration/agent 1.664s`. Three tests, six subtests, 0 FAIL / 0 SKIP: `TestOpenKnowledgeRouting_FallbackToOpenKnowledge` (1.44s; weather-domain-query, open-ended-knowledge, deterministic-tool-query), `_ScenarioHealthProbe` (0.02s), `_RelativeAGENT_SCENARIO_DIRResolvesAgainstRepoRoot` (3 subtests).
  - **T2** — discharged by fault-injected integration run; see its own entry above for the readiness-verdict evidence and the four confirmations that the correct branch fired.
  - **T3 / T4** — part of `27 === RUN / 9 --- PASS / 0 FAIL / 0 SKIP` across the nine `TestBUG064003_*` contract tests, `ok github.com/smackerel/smackerel/internal/agent 1.666s`. Both additionally carry mutation kills with matching signatures (see the pre-fix entry).
  - **Execution was verified, not inferred from a green exit code.** `go test -run` exits 0 when its selector matches nothing, so an exit code alone cannot distinguish "passed" from "never ran". The distinguishing evidence is that the two packages carrying these tests report a real duration with NO `[no tests to run]` suffix, while every other package in the same run carries that suffix. That asymmetry is what makes these results real.

- [x] Regression tests contain no silent-pass bailout patterns
  - **Claim Source:** executed · **Tree:** WORKING TREE, HEAD=3af96a02 · **Exit codes:** `0` (both modes)
  - **Command:** `bash .github/bubbles/scripts/regression-quality-guard.sh <files>` and the same with `--bugfix`, run live this session against both regression files.
  - Raw output evidence (inline, ≥10 lines, no references or summaries):
    ```
    ### V1 - default mode: bailout / silent-pass scan
    $ bash .github/bubbles/scripts/regression-quality-guard.sh tests/integration/agent/openknowledge_routing_test.go internal/agent/bug064003_router_warmup_contract_test.go
    ℹ️  Scanning tests/integration/agent/openknowledge_routing_test.go
    ℹ️  Scanning internal/agent/bug064003_router_warmup_contract_test.go
      REGRESSION QUALITY RESULT: 0 violation(s), 0 warning(s)
      Files scanned: 2
    GUARD_EXIT=0

    ### V2 - bugfix mode: adversarial-signal requirement
    $ bash .github/bubbles/scripts/regression-quality-guard.sh --bugfix tests/integration/agent/openknowledge_routing_test.go internal/agent/bug064003_router_warmup_contract_test.go
      Bugfix mode: true
    ✅ Adversarial signal detected in tests/integration/agent/openknowledge_routing_test.go
    ✅ Adversarial signal detected in internal/agent/bug064003_router_warmup_contract_test.go
      REGRESSION QUALITY RESULT: 0 violation(s), 0 warning(s)
      Files scanned: 2
      Files with adversarial signals: 2
    GUARD_BUGFIX_EXIT=0
    ```
  - **What this does not assert.** The guard proves the absence of bailout *shapes*; it does not prove the guards would have failed pre-fix. That is the separate unchecked item above. One `t.Skipf` does remain in the routing test at `openknowledge_routing_test.go:124` and the guard passed it: it fires only on `routerwarmup.ErrEmbedderUnreachable` (no live stack at all), and is a different branch from the `t.Fatalf` readiness verdict at line 125.
- [x] Full integration lane passes with no collateral regressions
  - **Claim Source:** executed · **Tree:** WORKING TREE, HEAD=3af96a02 · **Exit code:** `0`
  - **Command:** `./smackerel.sh test integration` — preserved at `~/s064-integration.log` (8218 lines), read this session; the lane was not re-run.
  - Raw output evidence (inline, ≥10 lines, no references or summaries):
    ```
    ### V1 - lane terminator
    8218:INTEGRATION_EXIT=0

    ### V2 - zero failures anywhere across all 8218 lines
    $ grep -cE '^(--- FAIL|FAIL[[:space:]]+github|FAIL:)' ~/s064-integration.log
    0

    ### V3 - the previously-failing package is now green
    3594:ok      github.com/smackerel/smackerel/tests/integration/agent  20.238s

    ### V4 - no collateral damage downstream; the eval gate still fires exactly once
    7925:ASSISTANT_ACCEPTANCE_GATE_V1 executed_assertions=210 rows=150 capture_expected=60 routing_accuracy=1.0000 capture_fallback_rate=1.0000
    7940:--- PASS: TestAcceptanceGate_RoutingAccuracyAndCaptureFallback (0.00s)
    8028:ok      github.com/smackerel/smackerel/tests/eval/assistant     0.040s
    8029:go-integration: acceptance gate executed 210 assertions.

    ### V5 - ephemeral test storage torn down, no residue
    8214: Volume smackerel-test-postgres-data  Removed
    8215: Volume smackerel-test-ollama-data  Removed
    8216: Volume smackerel-test-nats-data  Removed
    ```
  - This tick also unblocks the Stage 1 exit-criterion item in `specs/061-conversational-assistant/bugs/BUG-061-011-eval-gate-runs-in-no-automated-lane/scopes.md`, which was held on this lane exiting `0`.

#### Implementation items

- [x] Embedder-readiness gate precedes the timed region
  - **Claim Source:** executed · **Tree:** WORKING TREE, HEAD=3af96a02
  - **Command:** read the phase split from the source this session, then confirm the ordering actually observed on the live stack in `~/s064-integration.log` and the gate's load-bearing proof in `~/s064-unit.log`.
  - Raw output evidence (inline, ≥10 lines, no references or summaries):
    ```
    ### V1 - source: Phase 1 (untimed readiness) strictly precedes Phase 2 (timed build)
    tests/integration/agent/openknowledge_routing_test.go:117:  // Phase 1 — pay the cold start OUTSIDE the measured region. This must
    tests/integration/agent/openknowledge_routing_test.go:118:  // precede router construction; routerwarmup.BuildRouter refuses a
    tests/integration/agent/openknowledge_routing_test.go:120:  warm, err := routerwarmup.WaitForWarmEmbedder(context.Background(), embedder, timings, "probe")
    tests/integration/agent/openknowledge_routing_test.go:125:      t.Fatalf("integration: embedder readiness gate failed (this is an ML sidecar readiness verdict, NOT a routing failure): %v", err)
    tests/integration/agent/openknowledge_routing_test.go:147:  // Phase 2 — measure only router construction, under a budget derived from
    tests/integration/agent/openknowledge_routing_test.go:150:  router, err := routerwarmup.BuildRouter(context.Background(), warm, cfg, registered, embedder, timings)

    ### V2 - that ordering observed on the live stack (gate line precedes budget line)
    3552:    openknowledge_routing_test.go:127: embedder warm-up gate passed: probes=1 latencies=[#1=464ms] qualifying=464ms elapsed=464ms
    3554:    openknowledge_routing_test.go:149: router construction budget: embed_calls=79 per_call_budget=2s (AGENT_ROUTING_BUILD_PER_CALL_BUDGET_MS) derived_build_budget=2m38s

    ### V3 - the gate is load-bearing, and cannot be bypassed with a fabricated warm result
     76:--- PASS: TestBUG064003_WarmupGateIsLoadBearing (0.81s)
     77:    --- PASS: TestBUG064003_WarmupGateIsLoadBearing/without_the_gate_the_derived_budget_cannot_absorb_cold_start (0.20s)
     78:    --- PASS: TestBUG064003_WarmupGateIsLoadBearing/with_the_gate_the_same_budget_and_embedder_succeed (0.61s)
     80:--- PASS: TestBUG064003_ZeroWarmResultIsStructurallyRefused (0.00s)
    ```
  - The ordering is structural, not conventional: `BuildRouter` takes a `warm` value that only `WaitForWarmEmbedder` can produce, and `TestBUG064003_ZeroWarmResultIsStructurallyRefused` proves a zero-value substitute is rejected. The 464 ms probe was paid outside the measured region.
- [x] Router-construction budget derives from the intent-example count, not a literal
  - **Claim Source:** executed · **Tree:** WORKING TREE, HEAD=3af96a02
  - **Command:** read the derived budget from `~/s064-integration.log`, the SST keys from the working-tree diff of `config/smackerel.yaml`, the guard verdicts from `~/s064-unit.log`, and scan the source for surviving wall-clock literals.
  - Raw output evidence (inline, ≥10 lines, no references or summaries):
    ```
    ### V1 - the budget is a function of the work, and the arithmetic checks out
    3554:    openknowledge_routing_test.go:149: router construction budget: embed_calls=79 per_call_budget=2s (AGENT_ROUTING_BUILD_PER_CALL_BUDGET_MS) derived_build_budget=2m38s
         79 embeds x 2s per call = 158s = 2m38s  (vs the old flat 30s for all 79)

    ### V2 - the per-call unit is SST-published, not hardcoded (working-tree additions)
    $ git diff HEAD -- config/smackerel.yaml
    +    warmup_target_latency_ms: 1000 # REQUIRED: > 0; probe latency at/below which the embedder counts as warm
    +    warmup_budget_ms: 60000 # REQUIRED: > 0; ceiling on the readiness wait before an embedder-not-ready failure
    +    build_per_call_budget_ms: 2000 # REQUIRED: > 0; per-embed unit the router-construction budget is derived from

    ### V3 - guards: the budget scales with the work, the SST publishes the contract, defaults fit the lane
    120:--- PASS: TestBUG064003_RoutingTestCarriesNoWallClockLiteral (0.00s)
    122:--- PASS: TestBUG064003_SSTPublishesTheWarmupContract (0.00s)
    124:--- PASS: TestBUG064003_DerivedBudgetScalesWithTheWork (0.00s)
    126:--- PASS: TestBUG064003_SSTDefaultsFitTheLaneBudget (0.02s)

    ### V4 - source scan: no wall-clock literal survives (absence IS the pass condition)
    $ grep -nE 'context\.WithTimeout|[0-9]+\s*\*\s*time\.(Second|Minute)' tests/integration/agent/openknowledge_routing_test.go
    GREP_EXIT=1 (1 = no matches)
    ```
  - Both literals named in the Implementation Plan are gone: the `30*time.Second` router wrapper and the `5*time.Second` per-call ceiling. The per-call value now flows from SST via `timings.EmbedCallTimeout` at `openknowledge_routing_test.go:112`.
- [x] Local `parseFloatEnv` / `parseIntEnv` fallback helpers removed; env reads are fail-loud
  - **Claim Source:** executed · **Tree:** WORKING TREE, HEAD=3af96a02
  - **Command:** scan the source for surviving call sites and helper definitions, read the replacement fail-loud reads, and confirm the adversarial unit coverage in `~/s064-unit.log`.
  - Raw output evidence (inline, ≥10 lines, no references or summaries):
    ```
    ### V1 - no call site and no definition survives; the ONLY hit is a comment naming the deletion
    $ grep -nE 'parseFloatEnv|parseIntEnv' tests/integration/agent/openknowledge_routing_test.go
    94:     // parseFloatEnv/parseIntEnv helpers used to swallow exactly that and keep
         (report.md recorded 4 hits pre-fix: lines 119, 120 call sites and 217, 229 definitions — all gone)

    ### V2 - source: SST reads are now fatal-on-missing, mirroring internal/agent/config.go:155-156
    tests/integration/agent/openknowledge_routing_test.go:96:   timings, err := routerwarmup.LoadTimings(os.LookupEnv)
    tests/integration/agent/openknowledge_routing_test.go:100:  cfg, err := routerwarmup.LoadRoutingConfig(os.LookupEnv)
         each followed immediately by t.Fatalf("integration: %v", err) — no substituted value on the error path

    ### V3 - adversarial unit proof: the deleted 0.65 / 5 fallbacks cannot silently return
     98:    --- PASS: TestBUG064003_RoutingValuesFailLoudWithoutFallback/confidence_floor_absent (0.00s)
     99:    --- PASS: TestBUG064003_RoutingValuesFailLoudWithoutFallback/confidence_floor_empty (0.00s)
    100:    --- PASS: TestBUG064003_RoutingValuesFailLoudWithoutFallback/confidence_floor_unparseable (0.00s)
    101:    --- PASS: TestBUG064003_RoutingValuesFailLoudWithoutFallback/consider_top_n_absent (0.00s)
    104:    --- PASS: TestBUG064003_RoutingValuesFailLoudWithoutFallback/fallback_scenario_id_absent (0.00s)
    ```
  - This closes the drift finding recorded in [design.md](design.md) § 2 by removing the forbidden shape outright, rather than relying on the SST guard's `_test.go` / `tests/` exclusion. It does **not** resolve whether that exclusion is itself correct — see the unchecked owner-decision item below.
- [x] No product source file changed (test + SST config only)
  - **Claim Source:** executed · **Tree:** WORKING TREE, HEAD=3af96a02
  - **Command:** `git diff --stat HEAD` and `git status --porcelain`, **scoped to this bug's paths** — the shared working tree also carries several concurrent agents' unrelated changes (for example `internal/auth/scopes.go`, `specs/108-corpus-grant-enforcement/**`, `config/release-trains.yaml`), which are not this bug's and are excluded from the claim.
  - Raw output evidence (inline, ≥10 lines, no references or summaries):
    ```
    ### V1 - modified files in this bug's change surface
    $ git diff --stat HEAD -- tests/integration/agent/openknowledge_routing_test.go config/smackerel.yaml
     config/smackerel.yaml                              | 32 ++++++++
     .../agent/openknowledge_routing_test.go            | 86 +++++++++++-----------
     2 files changed, 76 insertions(+), 42 deletions(-)

    ### V2 - new files added by this bug
    $ git status --porcelain -- internal/agent/bug064003_router_warmup_contract_test.go tests/integration/agent/routerwarmup/
    ?? internal/agent/bug064003_router_warmup_contract_test.go
    ?? tests/integration/agent/routerwarmup/

    ### V3 - classification of all four
    tests/integration/agent/openknowledge_routing_test.go     test           (modified)
    internal/agent/bug064003_router_warmup_contract_test.go   test           (new, _test.go — excluded from the binary)
    tests/integration/agent/routerwarmup/routerwarmup.go      test-support   (new, under tests/)
    config/smackerel.yaml                                     SST config     (modified: 3 new keys, V2 of the budget item)
    ```
  - No file under `internal/` other than a `_test.go`, and no file under `cmd/` or `ml/`, is in this bug's surface. `routerwarmup.go` is a non-`_test.go` file, so it is called out explicitly rather than absorbed: it lives under `tests/` and is imported only by the integration test.

#### Build Quality Gate (grouped)

- [x] `./smackerel.sh check`, `lint`, and `format --check` clean; zero warnings; zero deferrals; artifact lint clean; docs aligned
  - **Claim Source:** executed 2026-08-28 against this tree state. The earlier note below is superseded and left standing as the historical record.
  - `bash .github/bubbles/scripts/evidence-capture.sh --label "BUG-064-003 check + lint + format --check" -- bash -lc './smackerel.sh check && ./smackerel.sh lint && ./smackerel.sh format --check'` → **exit 0**, 299 lines, sha256 `769a440c8916a474cf29ca238cf9e8d512eb37af35006f81937fe1fd31b568e6`. The chain is `&&`, so exit 0 requires all three to pass. Terminal line: `78 files already formatted`. Head shows `config-validate: ... OK`, `Config is in sync with SST`, `env_file drift guard: OK`, `scenario-lint: OK` (17 scenarios registered, 0 rejected).
  - `bash .github/bubbles/scripts/artifact-lint.sh <this packet>` → **exit 0**, `Artifact lint PASSED.`, including the anti-fabrication block: all checked DoD items carry evidence, no unfilled template placeholders in `scopes.md` or `report.md`, no repo-CLI bypass in report command evidence.
  - Docs aligned: the three stale claims this item previously blocked on were corrected in [report.md](report.md) in the same pass — the DoD count (10→11 of 15), the Artifacts-Created row that still read "Scope 1 Not Started, all DoD unchecked", and uncertainty item 4 which declared the no-defaults question unresolved. Each is marked SUPERSEDED rather than deleted, so the historical record survives.
  - Disk note, recorded because it changes how the receipts should be read: these lanes ran with `DISK_PREFLIGHT_OVERRIDE=1`. It was NOT a blind bypass. The clean reclaim was attempted first — `docker-safe-prune.sh --apply` removed 42 dangling volumes (67→25, 176.6→140.3 GB) and 3.12 GB of images while never touching the 21 protected persistent stores. That freed space INSIDE ext4 (524 GB free, against a 15 GB requirement) but did not shrink the `ext4.vhdx` high-water mark on C:, which is what the gate measures and which only an operator-side `wsl --shutdown` + elevated `Optimize-VHD` can reclaim. The guard's own text scopes the override to the "you KNOW it fits" case; the reclaim is what made that knowable.
  - **Claim Source:** not-run · **Uncertainty Declaration.** SUPERSEDED by the executed evidence above; retained verbatim as the historical record.
  - None of `check`, `lint`, `format --check`, or `artifact-lint.sh` was executed against this tree state, and no preserved log for them exists for this bug. `ls ~/*.log` this session shows `bug011-lint.log` and `bug011-format.log`, but those were captured for BUG-061-011 at an earlier tree state that predates `routerwarmup/` and `bug064003_router_warmup_contract_test.go` entirely; citing them here would be evidence laundering.
  - "Docs aligned" is additionally **not** satisfied: [report.md](report.md) still states "No fix was implemented" and "every Definition-of-Done checkbox in scopes.md is unchecked", both of which this file now contradicts. `report.md` was outside this pass's permitted edit set.
- [x] Owner decision recorded for design.md § "Open questions" item 1 (does the no-defaults policy bind test code?)
  - **Claim Source:** executed 2026-08-28 (artifact inspection) under explicit operator delegation ("MAKE YOUR DECISION, YOU APPROVED TO TAKE BEST PATH FORWARD").
  - **Command:** `grep -c 'DECISION (recorded 2026-08-28, owner-delegated): YES' design.md` then `grep -n ...` · **Exit Code:** 0

    ```
    $ grep -c 'DECISION (recorded 2026-08-28, owner-delegated): YES' design.md
    1
    $ grep -n 'DECISION (recorded 2026-08-28, owner-delegated)' design.md
    345:   **DECISION (recorded 2026-08-28, owner-delegated): YES — the policy binds
    GREP_EXIT=0
    ```

    The item asks that a decision be RECORDED. The evidence above is that it is
    recorded, at [design.md](design.md) line 345, exactly once. The decision's
    correctness is argued in design.md and summarised below; this block proves
    only presence, which is what the DoD item requires.
  - **Decision: YES — the policy binds test code, but what it forbids is the fallback FORM, not explicit test values.** Recorded in full at [design.md](design.md) § 5 item 1, which previously read *"This bug does not presume the answer."*
  - The operative distinction: `os.Setenv("AGENT_ROUTING_CONFIDENCE_FLOOR", "0.7")` is an EXPLICIT value and always was permitted; a helper shaped like `getenvOr("AGENT_ROUTING_CONFIDENCE_FLOOR", 0.65)` inside test code is a SILENT SUBSTITUTION and is forbidden, because it lets a test pass under a configuration production would reject.
  - Grounded in the guard's own words rather than inference. `internal/config/sst_grep_guard_test.go:13` justifies the exemption as *"test fixtures with explicit intent to use deterministic test values"* — explicit is the operative word. Its MECHANISM is broader than that rationale: line 75 skips whole files via `strings.HasSuffix(path, "_test.go")`, and the meta-test at lines 285-310 pins that file-level skip deliberately.
  - Why the ruling matters to THIS bug even though the forbidden helper was deleted: T4 asserts those two knobs fail loud rather than substituting `0.65` / `5`. If test code may substitute defaults, a future helper could satisfy T4's setup while quietly restoring the behaviour T4 forbids — the same false-green class already filed as BUG-069-005.
  - Consequence filed, not silently absorbed: the `_test.go` exclusion IS a gap, recorded as residue **R-012** in [.specify/memory/open-work.md](../../../../.specify/memory/open-work.md) with `bubbles.design` as next owner. NOT fixed here — it is the guard owner's surface and narrowing a file-level skip to a form-level one has blast radius across every existing test. NOT claimed: that any current test exploits the gap; no audit for live violations was run.
