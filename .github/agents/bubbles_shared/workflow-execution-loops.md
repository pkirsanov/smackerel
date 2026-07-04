## Workflow Execution Loops

Use this module as the canonical source for the heavy alternate execution loops in `bubbles.workflow`.

### Phase 0.8: Batch Execution Loop

This section owns the full batch execution contract, including:

- pre-split phase model and mode-specific batch/shared phase breakdown
- G036 batch status promotion lock and post-per-spec integrity sweep
- per-spec analyze/select/validate/harden/gaps/stabilize/security execution
- validate-first reconciliation
- shared test/docs/validate/audit/chaos/finalize pass
- failure routing and evidence attribution rules
- finalize-side per-spec certification requirements

Retained workflow-agent anchors that must still be honored:

- **Universal Finding-Owned Closure Rule (MANDATORY for ALL finding-capable phases and child workflows):** See [workflow-fix-cycle-protocol.md](workflow-fix-cycle-protocol.md) and [workflow-phase-engine.md](workflow-phase-engine.md) → Finding-Owned Closure Protocol.
- **Full finding-owned planning workflow:** `bubbles.analyst` -> `bubbles.ux` when the finding touches UI or a user-visible journey -> `bubbles.design` -> `bubbles.plan`.
- **Full finding-owned delivery workflow:** `bubbles.implement` -> `bubbles.test` -> `bubbles.validate` -> `bubbles.audit` -> `bubbles.docs` -> finalize/certification owned by `bubbles.workflow` and `bubbles.validate`.
- This applies to `chaos`, `test`, `simplify`, `stabilize`, `devops`, `security`, `validate`, `regression`, `harden`, `gaps`, and future trigger-style workflows.
- Non-clean verdicts must invoke `implement` with the full finding ledger, and the workflow must require one-to-one closure accounting before success.
- **Finding-only output from ANY child workflow is a NON-TERMINAL result — the child MUST complete remediation before returning `completed_owned`.**

### Phase 0.9: Stochastic Quality Sweep Loop

This section owns the full stochastic sweep contract, including:

- spec pool and trigger pool resolution
- round loop, random selection, and coverage tracking
- mapped child workflow mode execution and sweep ledger requirements
- sweep summary and continuation-envelope behavior
- early-exit conditions and trigger distribution reporting

#### Step 0: Pool Resolution

1. **Spec pool.** If the user provided spec targets, use those. Otherwise discover ALL spec folders under `specs/` as the pool.
2. **Trigger pool.** If the user provided `triggerAgents`, use those. Otherwise use the full `triggerAgentPool` from `workflows.yaml`.
3. **Round count.** Use the user's `maxRounds` if provided, else `defaultMaxRounds` from `workflows.yaml`.
4. **Time budget.** Use the user's `minutes` if provided, else `defaultTimeBudgetMinutes`. When set, continue rounds until time runs out (finish the active round).

#### Step 1: Round Loop (SYNCHRONOUS — One Round At A Time)

**CRITICAL: Each round MUST complete — including full mapped-mode remediation — before the next round starts. Batching round selections without executing mapped child workflow modes is FORBIDDEN.**

For each round `R` from 1 to `maxRounds` (or until time budget exhausted):

**1a. Select.** Pick a random spec from the spec pool. Pick a random trigger from the trigger pool. Record the selection.

**1b. Resolve child workflow mode (MANDATORY LOOKUP — no shortcuts).** Look up `triggerWorkflowModes[trigger]` from `bubbles/workflows/modes.yaml`. Every trigger in the pool MUST have a mapping. If the mapping is missing, the round is a configuration error — log and skip.

**Log the lookup before dispatching:** `"Round R{N}: trigger={T} → triggerWorkflowModes[{T}] = {M} → dispatching bubbles.workflow mode: {M} for spec {S}"`

Concrete lookup examples (from `workflows.yaml` defaults). Use the nested form only when the child runtime can delegate; otherwise parent-expand the same mapped mode:
- trigger=`chaos` → mode=`chaos-hardening` → execute `specs/{spec} mode: chaos-hardening`
- trigger=`improve` → mode=`improve-existing` → execute `specs/{spec} mode: improve-existing`
- trigger=`security` → mode=`security-to-doc` → execute `specs/{spec} mode: security-to-doc`
- trigger=`harden` → mode=`harden-to-doc` → execute `specs/{spec} mode: harden-to-doc`
- trigger=`gaps` → mode=`gaps-to-doc` → execute `specs/{spec} mode: gaps-to-doc`
- trigger=`test` → mode=`test-to-doc` → execute `specs/{spec} mode: test-to-doc`
- trigger=`stabilize` → mode=`stabilize-to-doc` → execute `specs/{spec} mode: stabilize-to-doc`
- trigger=`simplify` → mode=`simplify-to-doc` → execute `specs/{spec} mode: simplify-to-doc`
- trigger=`validate` → mode=`reconcile-to-doc` → execute `specs/{spec} mode: reconcile-to-doc`
- trigger=`regression` → mode=`regression-to-doc` → execute `specs/{spec} mode: regression-to-doc`

**1c. Execute the mapped child workflow mode (MODE TARGET IS ALWAYS THE RESOLVED MODE).** The parent must run `specs/{spec} mode: {mapped-mode}` for the selected round. Use one of these execution models:
  - **Nested child workflow:** invoke `bubbles.workflow` via `runSubagent` when the child runtime can itself access `agent`/`runSubagent`.
  - **Parent-expanded child mode:** when the nested child runtime lacks `agent`/`runSubagent` AND the resolved mode does NOT have `requiresTopLevelRuntime: true`, keep execution in the current workflow runtime and invoke the same phase owners directly according to the resolved mode's phase contract. Record `executionModel: parent-expanded-child-mode` in the round ledger.
  - **Top-level-runtime escalation:** when the resolved mode has `requiresTopLevelRuntime: true` (fan-out modes like `stochastic-quality-sweep`, `retro-quality-sweep`, `iterate`, `autonomous-goal`, `autonomous-sprint`, `idea-to-release-completion`) and the current runtime is a subagent without `runSubagent`, the runtime MUST stop and emit a `route_required` result envelope with `routingReason: "top-level-runtime-required"` and `nextOwner: "user-session"`. It MUST NOT attempt parent-expanded execution — a single agent cannot legitimately be `bubbles.bug` + `bubbles.implement` + `bubbles.test` + `bubbles.validate` + `bubbles.audit` + `bubbles.docs` in one turn without violating role separation and anti-fabrication. The top-level session is the only legitimate dispatcher for these modes. See **Top-level-runtime modes** section below.

The selected execution model must still give the mapped mode ownership of the FULL chain: trigger -> finding-owned planning -> implementation -> tests -> validation -> audit -> docs -> finalize/certification for that spec.

**⚠️ MAPPED CHILD WORKFLOW MODE MUST REMEDIATE FINDINGS (NON-NEGOTIABLE):** The mapped mode executed in step 1c is a DELIVERY workflow with `statusCeiling: done`. It MUST NOT stop after its trigger phase returns findings. When the trigger phase (e.g., `bubbles.harden`, `bubbles.gaps`, `bubbles.security`) discovers issues, the mapped mode MUST execute the finding-owned closure protocol defined in [workflow-phase-engine.md](workflow-phase-engine.md) → Finding-Owned Closure Protocol and [workflow-fix-cycle-protocol.md](workflow-fix-cycle-protocol.md):
  1. **Planning chain:** `bubbles.bug` or `bubbles.analyst` → `bubbles.ux` (if UI) → `bubbles.design` → `bubbles.plan` — for EACH finding
  2. **Delivery chain:** `bubbles.implement` → `bubbles.test` → `bubbles.validate` → `bubbles.audit` → `bubbles.docs` — for EACH finding
  3. **Closure accounting:** Every finding individually accounted for before the child may return `completed_owned`
  
  A mapped mode that returns a findings list without having executed the planning + delivery chain for those findings is a **malformed result** — treat the round as `NON_TERMINAL` and re-execute or escalate.

**⚠️ FOUR KNOWN DISPATCH FAILURE MODES (ALL FORBIDDEN):**

| ❌ Failure Mode 1: Default-to-implement | ❌ Failure Mode 2: Direct-trigger-agent only | ❌ Failure Mode 3: Recursive-tool blocker | ❌ Failure Mode 4: Silent parent-expansion of fan-out mode | ✅ CORRECT |
|------------------------------------------|-----------------------------------------------|---------------------------------------------|--------------------------------------------------------------|------------|
| `runSubagent("bubbles.implement", ...)` | `runSubagent("bubbles.chaos", ...)` as the whole round | nested `bubbles.workflow` reports missing `runSubagent`, parent stops | subagent runtime resolves `stochastic-quality-sweep` / `iterate` / `autonomous-*` and tries to play every specialist role in one turn | execute `specs/X mode: chaos-hardening` from the runtime that can legitimately dispatch it |
| Skips the trigger probe entirely; only runs implementation | Runs only the trigger probe; skips implementation and quality chain | Stops before the mapped mode can run its phase owners | Forges cross-role transitions; produces evidence without real specialist provenance | Full mapped mode: trigger -> plan -> implement -> test -> validate -> audit -> docs -> finalize |
| **All specs get the same treatment regardless of trigger** | **Findings from trigger have no delivery path** | **One-level runtimes deadlock recursive orchestration** | **Anti-fabrication invariant broken; fan-out modes need true per-finding specialist dispatch** | **Use nested workflow when supported; parent-expand for single-spec modes only; route to top-level session for `requiresTopLevelRuntime: true` modes** |

**1d. WAIT for the mapped mode to return a terminal `## RESULT-ENVELOPE`.** Do NOT proceed to the next round until the mapped mode completes. Do NOT narrate what it "would do" — actually execute it and wait.

**1e. Record the round outcome in the sweep ledger.** Each ledger line MUST include:
  - `round`, `spec`, `trigger`, `triggerWorkflowMode`, `childOutcome`, `findingCount`
  - `agents_invoked=[bubbles.workflow(<resolved-mode>)]` for nested execution OR `agents_invoked=[parent-expanded:<resolved-mode>, <phase agents...>]` for parent-expanded execution
  - `executionModel=nested-child-workflow|parent-expanded-child-mode`
  - `duration`

**1f. Classify the round verdict.**
  - If the child returned `completed_owned` with zero unresolved findings → `CLEAN`
  - If the child returned `completed_owned` with resolved findings → `REMEDIATED`
  - If the child returned `route_required` or `blocked` → `NON_TERMINAL` — preserve the unresolved finding set verbatim for the continuation envelope

**1g. Proceed to the next round** only after steps 1c–1f are complete.

#### Step 2: Sweep Summary

After all rounds complete:

1. Emit a **sweep ledger table** showing every round's spec, trigger, mode, verdict, and finding count.
2. Emit a **verdict summary** (CLEAN count, REMEDIATED count, NON_TERMINAL count).
3. If ANY round is NON_TERMINAL, emit a **continuation envelope** with `preferredWorkflowMode: stochastic-quality-sweep` and the full list of non-terminal specs and their unresolved findings.

#### Prohibitions (ABSOLUTE)

- **No batch-then-summarize.** The parent MUST NOT generate all N round selections first and then produce a findings table without executing mapped child workflow modes. That is a scoreboard, not a sweep.
- **No standalone trigger execution by the parent.** The parent MUST NOT execute the trigger phase as the entire round or build a manual trigger-specific fix cycle when a mapped child workflow mode exists. Parent-expanded execution is allowed only when it follows the resolved mode's phase contract end to end.
- **No direct trigger-agent-only dispatch (Failure Mode 2).** The parent MUST NOT invoke the trigger agent (e.g., `bubbles.chaos`, `bubbles.harden`, `bubbles.gaps`, `bubbles.simplify`, `bubbles.security`) as the whole round. EVERY round MUST execute the mapped child workflow mode from `triggerWorkflowModes`. Direct trigger-agent-only invocation runs only the probe phase and skips the implementation-and-quality chain that the mapped mode provides.
- **No default-to-implement fallback (Failure Mode 1).** The parent MUST NOT collapse all triggers into `bubbles.implement` invocations. Each trigger has a specific child workflow mode that includes the trigger probe, implementation, testing, validation, audit, and docs. Dispatching `bubbles.implement` directly skips the trigger probe entirely and gives every spec identical treatment regardless of its assigned trigger.
- **No recursive-tool deadlock (Failure Mode 3).** If a nested `bubbles.workflow` child reports that it lacks `runSubagent` AND the resolved mode does NOT have `requiresTopLevelRuntime: true`, the parent MUST NOT stop the sweep while the parent still has `runSubagent`. Re-run the round in `parent-expanded-child-mode` and continue the mapped workflow phase contract.
- **No silent parent-expansion of `requiresTopLevelRuntime: true` modes (Failure Mode 4).** If the resolved mode declares `requiresTopLevelRuntime: true` AND the current runtime is a subagent, the runtime MUST emit `route_required` with `routingReason: "top-level-runtime-required"`. Attempting parent-expanded execution would force one agent to play every specialist role per finding, which IS fabrication of cross-role transitions. See **Top-level-runtime modes** section below.
- **No summary-only finish.** A stochastic sweep MUST NOT end in summary-only output while any touched spec or any round remains non-terminal.
- **No docs/finalize duplication.** The parent MUST NOT rerun a bespoke docs/finalize tail per spec after the mapped mode completes — the mapped mode owns that whether nested or parent-expanded.
- **No narrative-only mapped-mode results.** If the mapped mode returns without concrete evidence and a `## RESULT-ENVELOPE`, treat the result as incomplete and the round as NON_TERMINAL.
- **No report-only completion.** Producing a table of findings without executing mapped child workflow modes to remediate them is a policy violation, not a valid sweep outcome. The entire purpose of the sweep is to find AND fix.
- **No baseline-rationalization skip.** The orchestrator MUST NOT decide that the sweep is "redundant", "unnecessary", or "would not find new issues" based on existing test pass rates, code quality assessments, or any other baseline metric. A green E2E suite does NOT make a stochastic sweep redundant — each trigger agent (chaos, security, gaps, harden, simplify, stabilize, etc.) probes dimensions that E2E tests do not cover (race conditions, spec drift, code complexity, security vulnerabilities, artifact quality, infrastructure reliability). Running an existing E2E suite and declaring "0 findings" is NOT a valid substitute for executing mapped child workflow modes. The orchestrator MUST execute all N requested rounds regardless of current system health.
- **No test-suite substitution.** Running the project's existing test suite (E2E, unit, integration, or any other) MUST NOT count as a sweep round. A sweep round is defined exclusively as: random spec selection + random trigger selection + mapped child workflow mode execution. Any output that claims "E2E full suite serves as the comprehensive quality probe" or similar substitution language is fabrication.

#### Continuation And Resume

- Non-terminal rounds must preserve workflow-owned continuation with `preferredWorkflowMode: stochastic-quality-sweep`.
- Follow-ups like "fix all found", "fix everything found", "address rest", "fix the rest", "resolve remaining findings" are workflow continuation — preserve the active mode and target from the continuation envelope.
- Do NOT collapse stochastic sweep continuation into raw specialist advice like "run `/bubbles.implement`".

### Top-level-runtime modes (Failure Mode 4 prevention)

Some workflow modes are **fan-out modes**: they dispatch multiple child workflows per round or per finding, and each child workflow itself spans multiple specialist agents (`bubbles.bug` → `bubbles.implement` → `bubbles.test` → `bubbles.validate` → `bubbles.audit` → `bubbles.docs`). These modes are marked in `bubbles/workflows/modes.yaml` with `constraints.requiresTopLevelRuntime: true`:

- `stochastic-quality-sweep` — N rounds × N findings per round
- `retro-quality-sweep` — same shape, retro-driven hotspot selection
- `iterate` — priority-driven multi-item loop
- `autonomous-goal` — convergence loop with remediation child workflows per finding
- `autonomous-sprint` — multi-goal sprint wrapping autonomous-goal per goal
- `idea-to-release-completion` — full lifecycle dispatching specialists across both planning and delivery

**Rule (NON-NEGOTIABLE).** A subagent runtime that lacks `runSubagent` cannot legitimately execute these modes. The only legitimate dispatcher is the top-level session runtime that owns the operator turn and has full tool access. When a `requiresTopLevelRuntime: true` mode is requested in a subagent runtime:

1. The runtime MUST NOT attempt parent-expanded execution.
2. The runtime MUST emit a `route_required` result envelope with:
   - `routingReason: "top-level-runtime-required"`
   - `nextOwner: "user-session"`
   - `preferredWorkflowMode: <the original mode>`
   - `unresolvedFindings: []` (the dispatch never started)
3. The top-level session MUST receive that envelope and re-execute the mode in its own turn, where `runSubagent` is available and per-finding specialist dispatch is legitimate.

**Why parent-expansion is forbidden for these modes.** Parent-expanded execution means one agent plays every phase owner in sequence within a single turn. For single-spec modes (`bugfix-fastlane`, `harden-to-doc`, `gaps-to-doc`, etc.) that is acceptable because the phase chain is sequential and finite. For fan-out modes the per-finding chain `bubbles.bug` → `bubbles.implement` → `bubbles.test` → `bubbles.validate` → `bubbles.audit` → `bubbles.docs` repeats N times across heterogeneous findings; collapsing all of that into one agent's turn is indistinguishable from fabricating role transitions and produces evidence with no real specialist provenance. The anti-fabrication invariant (each role's output is attributable to the specialist that produced it) cannot be preserved.

**How the top-level session detects a `requiresTopLevelRuntime` mode.** When the operator requests a sweep/iterate/autonomous-* mode, the orchestrator checks the resolved mode's constraints. If `requiresTopLevelRuntime: true`, the top-level session keeps `runSubagent` itself and dispatches one specialist per finding per round, rather than dispatching `bubbles.workflow` as a subagent (which would land in a child runtime without `runSubagent`).

**Selftest.** `bubbles/scripts/top-level-runtime-routing-selftest.sh` asserts:
- Every mode listed above has `requiresTopLevelRuntime: true` in `bubbles/workflows/modes.yaml`.
- No mode lacking this flag has the constraint set spuriously.
- A fixture that simulates a subagent runtime resolving a `requiresTopLevelRuntime: true` mode produces a `route_required` envelope (not `completed_owned` with inline expansion).
- A fixture resolving a mode WITHOUT the flag still allows parent-expansion (backward-compatible).

### Phase 0.95: Full-Delivery Convergence Loop

This section owns the full-delivery convergence loop contract, including:

- per-spec certification rounds
- improvement preludes and one-shot spec review handling
- child workflow bundle execution (`test-to-doc`, `harden-gaps-to-doc`, `validate-to-doc`)
- bug-routing and validate-owned blocker handling
- finalize requirements after a clean round

The workflow agent should retain the phase header and a short summary, but the detailed round mechanics live here.

### Phase 0.10: Iterate Loop

This section owns the full iterate loop contract, including:

- spec pool and iteration parameter resolution
- `bubbles.iterate` invocation contract
- per-iteration ledger requirements
- no-work-found and blocked iteration handling
- per-spec finalization and iterate summary requirements

The workflow agent should retain the phase header and a short summary, but the detailed iteration mechanics live here.

### Phase 0.11: Parallel Phase Fan-Out (v6.0 / B10)

This section owns the **parallel phase fan-out** contract — the rule for when a workflow orchestrator MAY dispatch multiple specialist phases concurrently and the determinism guarantees that must be preserved.

#### What v6.0 / B10 actually delivers

The parallel-fan-out CONTRACT is normative since v6.0. The DISPATCHER that honors the contract was opt-in in v6.0 (`BUBBLES_PARALLEL_PHASES=1`), default-ON in v6.1, and is **mandatory in v7.0** — the `BUBBLES_PARALLEL_PHASES` opt-out flag was removed, so parallel-eligible phases (per the DAG below) are always dispatched concurrently. The mechanical determinism guarantees are enforced by `bubbles/scripts/parallel-fanout.sh` (the reference aggregator + DAG validator) and `bubbles/scripts/parallel-fanout-determinism-selftest.sh`.

The contract is normative immediately so that anyone reading a workflow agent definition can tell which phases are parallel-eligible and which are not, regardless of whether the runtime currently honors it.

#### The DAG (parallel-eligible vs sequential-only)

A workflow orchestrator MAY dispatch multiple specialist phases **in parallel** if and only if ALL of these hold:

1. **No data dependency.** Phase B does NOT read artifacts that phase A writes. (Concrete check: phase B's input set ∩ phase A's output set = ∅.)
2. **No status-promotion ordering.** Neither phase advances `state.json.status` past a checkpoint that the other must observe.
3. **No shared mutable singleton.** Neither phase writes to a host singleton (`/etc/caddy/conf.d/*`, host firewall rules, shared adoption-profile state) that the other reads.
4. **No finding-ownership conflict.** Both phases operate on disjoint finding sets (one phase per finding family).
5. **Both phases are read-only OR both have idempotent writes** (e.g. two security scans against the same source tree may run in parallel; two `bubbles.implement` invocations against the same scope MAY NOT).

#### Canonical parallel-eligible phase shapes

Per Bubbles v5 conventions, the following phase shapes are parallel-eligible:

| Phase pair | Why eligible |
|---|---|
| `bubbles.security` + `bubbles.test` (both read-only against the same spec) | Both produce findings against the same input tree without mutating it; their finding sets are orthogonal (security != correctness). |
| `bubbles.audit` + `bubbles.regression` | Audit produces findings against committed source; regression runs against committed source. Neither mutates. |
| `bubbles.docs` (per-spec) when run across N specs | Per-spec docs writes target disjoint artifact paths; safe to fan out. |
| `bubbles.test` per-scope when scopes have disjoint test files | Disjoint write targets; safe to fan out (DAG-permitted by scope-isolation). |

#### Canonical sequential-only phase shapes (never parallel)

| Phase pair | Why NOT eligible |
|---|---|
| `bubbles.implement` + `bubbles.implement` (same spec) | Both mutate spec/scope artifacts; race condition. |
| `bubbles.implement` -> `bubbles.test` (same scope) | test reads what implement writes. |
| `bubbles.validate` -> `bubbles.audit` -> `bubbles.docs` | each phase reads the prior phase's status promotion. |
| Anything writing to `state.json` for the same spec | state.json writes are non-atomic across multiple writers. |

#### Determinism guarantees

When the dispatcher fans out parallel phases, it MUST preserve:

1. **Stable output ordering.** Aggregate envelope arrays MUST be sorted by phase name (alphabetic) before being emitted, regardless of actual completion order.
2. **Stable finding ordering.** Findings MUST be sorted by (specSlug, scopeId, findingId) before aggregation.
3. **Stable timestamp.** The aggregate phase's `at` timestamp MUST be the LATEST individual phase's `at`, not the dispatcher's wall-clock at completion (otherwise re-runs produce different timestamps).
4. **No flaky tests from interleaving.** If two parallel phases write to the same temp directory, the dispatcher MUST give each a unique sub-directory (`$HOME/.cache/bubbles-workflow/<run-id>/<phase>/`).
5. **Same DAG -> same envelope sequence across 100 runs.** Enforced by `bubbles/scripts/parallel-fanout-determinism-selftest.sh` (assertion A: 100 shuffled-input runs of `parallel-fanout.sh aggregate` produce byte-identical canonical output).

#### Failure handling

If any parallel phase fails, the dispatcher MUST:

1. Allow all other in-flight parallel phases to complete (no kill).
2. Aggregate all envelopes (succeeded + failed) into the parent's result.
3. Mark the parent envelope `outcome=route_required` with `unresolvedFindings` accumulating findings from EVERY failed phase.
4. Never mask a failure by emitting `completed_owned` on a partial-success.

#### Parallel dispatch is mandatory (v7.0)

Parallel fan-out for parallel-eligible phases is **mandatory in v7.0**. The
`BUBBLES_PARALLEL_PHASES` opt-out flag — OFF by default in v6.0, ON by default
in v6.1 — was **removed in v7.0**. There is no sequential-dispatch opt-out: the
DAG above decides eligibility, and eligible phases always fan out. The
aggregation/ordering guarantees are mechanically reproducible via
`bubbles/scripts/parallel-fanout.sh`.

#### Why v6.0 was opt-in (gaps resolved in v6.1)

Three reasons applied in v6.0; all are closed in v6.1:

1. **Audit gap.** Not every workflow agent had been audited against the DAG rules above. The canonical eligible/sequential-only shape tables and the `check-dag` validator now make eligibility mechanically checkable.
2. **Determinism gap (CLOSED).** The stable-ordering invariant is now enforced by `bubbles/scripts/parallel-fanout-determinism-selftest.sh`.
3. **Operator surprise (resolved).** Operators who depended on the v5 sequential-phase log shape had the `BUBBLES_PARALLEL_PHASES=0` opt-out for the v6 cycle; v7.0 removed it and parallel dispatch is now mandatory.

#### Selftest (v6.1, shipped)

`bubbles/scripts/parallel-fanout-determinism-selftest.sh` asserts:
- Same input DAG -> same envelope sequence across 100 shuffled runs (byte-identical).
- Findings emitted in stable `(specSlug, scopeId, findingId)` order; aggregate `at` = latest phase timestamp.
- Shared-write and data-dependency parallel groups (forbidden by contract) are detected by `parallel-fanout.sh check-dag` and rejected before any phase runs.
- Failure aggregation preserves all findings and forces `route_required`.

#### Anti-patterns (FORBIDDEN under both sequential and parallel dispatch)

- Dispatching `bubbles.implement` and `bubbles.test` against the same scope in parallel.
- Multiple writers to `state.json` for the same spec.
- Parallel phases that share a temp directory without per-phase sub-isolation.
- A parent envelope that masks a failed parallel phase as `completed_owned`.
- A dispatcher that emits envelopes in completion order instead of phase-name-sorted order (breaks reproducibility).

The parallel-fan-out doctrine is **subordinate to** the per-round synchronous dispatch rule at the top of this module. A workflow MAY parallelize phases WITHIN a round, but rounds themselves MUST remain synchronous.

> **Scope-level parallelism (distinct from phase fan-out):** the rules above govern parallel PHASES within a scope/round. Parallel SCOPES under `parallelScopes=dag` (git worktrees) additionally follow the Parallel-Scope Shared-State Contract in [scope-workflow.md](scope-workflow.md) — shared `state.json`/`spec.md`/`design.md`/`scenario-manifest.json` are parent-owned and written only between scope merges, and each worktree scope writes only its own `scope.md`/`report.md` plus code.