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

**1b. Resolve child workflow mode (MANDATORY LOOKUP — no shortcuts).** Look up `triggerWorkflowModes[trigger]` from `workflows.yaml`. Every trigger in the pool MUST have a mapping. If the mapping is missing, the round is a configuration error — log and skip.

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
  - **Parent-expanded child mode:** when the nested child runtime lacks `agent`/`runSubagent`, keep execution in the current workflow runtime and invoke the same phase owners directly according to the resolved mode's phase contract. Record `executionModel: parent-expanded-child-mode` in the round ledger.

The selected execution model must still give the mapped mode ownership of the FULL chain: trigger -> finding-owned planning -> implementation -> tests -> validation -> audit -> docs -> finalize/certification for that spec.

**⚠️ MAPPED CHILD WORKFLOW MODE MUST REMEDIATE FINDINGS (NON-NEGOTIABLE):** The mapped mode executed in step 1c is a DELIVERY workflow with `statusCeiling: done`. It MUST NOT stop after its trigger phase returns findings. When the trigger phase (e.g., `bubbles.harden`, `bubbles.gaps`, `bubbles.security`) discovers issues, the mapped mode MUST execute the finding-owned closure protocol defined in [workflow-phase-engine.md](workflow-phase-engine.md) → Finding-Owned Closure Protocol and [workflow-fix-cycle-protocol.md](workflow-fix-cycle-protocol.md):
  1. **Planning chain:** `bubbles.bug` or `bubbles.analyst` → `bubbles.ux` (if UI) → `bubbles.design` → `bubbles.plan` — for EACH finding
  2. **Delivery chain:** `bubbles.implement` → `bubbles.test` → `bubbles.validate` → `bubbles.audit` → `bubbles.docs` — for EACH finding
  3. **Closure accounting:** Every finding individually accounted for before the child may return `completed_owned`
  
  A mapped mode that returns a findings list without having executed the planning + delivery chain for those findings is a **malformed result** — treat the round as `NON_TERMINAL` and re-execute or escalate.

**⚠️ TWO KNOWN DISPATCH FAILURE MODES (BOTH FORBIDDEN):**

| ❌ Failure Mode 1: Default-to-implement | ❌ Failure Mode 2: Direct-trigger-agent only | ❌ Failure Mode 3: Recursive-tool blocker | ✅ CORRECT |
|------------------------------------------|-----------------------------------------------|---------------------------------------------|------------|
| `runSubagent("bubbles.implement", ...)` | `runSubagent("bubbles.chaos", ...)` as the whole round | nested `bubbles.workflow` reports missing `runSubagent`, parent stops | execute `specs/X mode: chaos-hardening` |
| Skips the trigger probe entirely; only runs implementation | Runs only the trigger probe; skips implementation and quality chain | Stops before the mapped mode can run its phase owners | Full mapped mode: trigger -> plan -> implement -> test -> validate -> audit -> docs -> finalize |
| **All specs get the same treatment regardless of trigger** | **Findings from trigger have no delivery path** | **One-level runtimes deadlock recursive orchestration** | **Use nested workflow when supported, otherwise parent-expand the mapped mode** |

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
- **No recursive-tool deadlock (Failure Mode 3).** If a nested `bubbles.workflow` child reports that it lacks `runSubagent`, the parent MUST NOT stop the sweep while the parent still has `runSubagent`. Re-run the round in `parent-expanded-child-mode` and continue the mapped workflow phase contract.
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