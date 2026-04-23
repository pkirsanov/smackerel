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
- child workflow dispatch and sweep ledger requirements
- sweep summary and continuation-envelope behavior
- early-exit conditions and trigger distribution reporting

#### Step 0: Pool Resolution

1. **Spec pool.** If the user provided spec targets, use those. Otherwise discover ALL spec folders under `specs/` as the pool.
2. **Trigger pool.** If the user provided `triggerAgents`, use those. Otherwise use the full `triggerAgentPool` from `workflows.yaml`.
3. **Round count.** Use the user's `maxRounds` if provided, else `defaultMaxRounds` from `workflows.yaml`.
4. **Time budget.** Use the user's `minutes` if provided, else `defaultTimeBudgetMinutes`. When set, continue rounds until time runs out (finish the active round).

#### Step 1: Round Loop (SYNCHRONOUS — One Round At A Time)

**CRITICAL: Each round MUST complete — including full child workflow remediation — before the next round starts. Batching round selections without executing child workflows is FORBIDDEN.**

For each round `R` from 1 to `maxRounds` (or until time budget exhausted):

**1a. Select.** Pick a random spec from the spec pool. Pick a random trigger from the trigger pool. Record the selection.

**1b. Resolve child workflow mode (MANDATORY LOOKUP — no shortcuts).** Look up `triggerWorkflowModes[trigger]` from `workflows.yaml`. Every trigger in the pool MUST have a mapping. If the mapping is missing, the round is a configuration error — log and skip.

**Log the lookup before dispatching:** `"Round R{N}: trigger={T} → triggerWorkflowModes[{T}] = {M} → dispatching bubbles.workflow mode: {M} for spec {S}"`

Concrete lookup examples (from `workflows.yaml` defaults):
- trigger=`chaos` → mode=`chaos-hardening` → `runSubagent("bubbles.workflow", "specs/{spec} mode: chaos-hardening")`
- trigger=`improve` → mode=`improve-existing` → `runSubagent("bubbles.workflow", "specs/{spec} mode: improve-existing")`
- trigger=`security` → mode=`security-to-doc` → `runSubagent("bubbles.workflow", "specs/{spec} mode: security-to-doc")`
- trigger=`harden` → mode=`harden-to-doc` → `runSubagent("bubbles.workflow", "specs/{spec} mode: harden-to-doc")`
- trigger=`gaps` → mode=`gaps-to-doc` → `runSubagent("bubbles.workflow", "specs/{spec} mode: gaps-to-doc")`
- trigger=`test` → mode=`test-to-doc` → `runSubagent("bubbles.workflow", "specs/{spec} mode: test-to-doc")`
- trigger=`stabilize` → mode=`stabilize-to-doc` → `runSubagent("bubbles.workflow", "specs/{spec} mode: stabilize-to-doc")`
- trigger=`simplify` → mode=`simplify-to-doc` → `runSubagent("bubbles.workflow", "specs/{spec} mode: simplify-to-doc")`
- trigger=`validate` → mode=`reconcile-to-doc` → `runSubagent("bubbles.workflow", "specs/{spec} mode: reconcile-to-doc")`
- trigger=`regression` → mode=`regression-to-doc` → `runSubagent("bubbles.workflow", "specs/{spec} mode: regression-to-doc")`

**1c. Dispatch child workflow via `runSubagent` (DISPATCH TARGET IS ALWAYS `bubbles.workflow`).** Invoke `bubbles.workflow` as a child workflow with:
  - The resolved child workflow mode from step 1b (e.g. `harden-to-doc`, `chaos-hardening`, `improve-existing`)
  - The selected spec as the target
  - Instruction that the child workflow owns the FULL chain: trigger → finding-owned planning → implementation → tests → validation → audit → docs → finalize/certification for that spec

**⚠️ CHILD WORKFLOW MUST REMEDIATE FINDINGS (NON-NEGOTIABLE):** The child workflow dispatched in step 1c is a DELIVERY workflow with `statusCeiling: done`. It MUST NOT stop after its trigger phase returns findings. When the trigger phase (e.g., `bubbles.harden`, `bubbles.gaps`, `bubbles.security`) discovers issues, the child workflow MUST execute the finding-owned closure protocol defined in [workflow-phase-engine.md](workflow-phase-engine.md) → Finding-Owned Closure Protocol and [workflow-fix-cycle-protocol.md](workflow-fix-cycle-protocol.md):
  1. **Planning chain:** `bubbles.bug` or `bubbles.analyst` → `bubbles.ux` (if UI) → `bubbles.design` → `bubbles.plan` — for EACH finding
  2. **Delivery chain:** `bubbles.implement` → `bubbles.test` → `bubbles.validate` → `bubbles.audit` → `bubbles.docs` — for EACH finding
  3. **Closure accounting:** Every finding individually accounted for before the child may return `completed_owned`
  
  A child workflow that returns a findings list without having executed the planning + delivery chain for those findings is a **malformed result** — treat the round as `NON_TERMINAL` and re-dispatch or escalate.

**⚠️ TWO KNOWN DISPATCH FAILURE MODES (BOTH FORBIDDEN):**

| ❌ Failure Mode 1: Default-to-implement | ❌ Failure Mode 2: Direct-trigger-agent | ✅ CORRECT |
|------------------------------------------|------------------------------------------|------------|
| `runSubagent("bubbles.implement", ...)` | `runSubagent("bubbles.chaos", ...)` | `runSubagent("bubbles.workflow", "specs/X mode: chaos-hardening")` |
| Skips the trigger probe entirely; only runs implementation | Runs only the trigger probe; skips implementation and quality chain | Full child workflow: trigger → plan → implement → test → validate → audit → docs → finalize |
| **All specs get the same treatment regardless of trigger** | **Findings from trigger have no delivery path** | **Each trigger gets its correct full workflow** |

**1d. WAIT for the child workflow to return a terminal `## RESULT-ENVELOPE`.** Do NOT proceed to the next round until the child workflow completes. Do NOT narrate what the child "would do" — actually invoke it and wait.

**1e. Record the round outcome in the sweep ledger.** Each ledger line MUST include:
  - `round`, `spec`, `trigger`, `triggerWorkflowMode`, `childOutcome`, `findingCount`
  - `agents_invoked=[bubbles.workflow(<resolved-mode>)]`
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

- **No batch-then-summarize.** The parent MUST NOT generate all N round selections first and then produce a findings table without dispatching child workflows. That is a scoreboard, not a sweep.
- **No trigger execution by the parent.** The parent MUST NOT execute the trigger phase directly or build a manual trigger-specific fix cycle when a mapped child workflow exists.
- **No direct trigger-agent dispatch (Failure Mode 2).** The parent MUST NOT invoke the trigger agent (e.g., `bubbles.chaos`, `bubbles.harden`, `bubbles.gaps`, `bubbles.simplify`, `bubbles.security`) directly via `runSubagent`. EVERY dispatch MUST go through `bubbles.workflow` with the mapped child workflow mode from `triggerWorkflowModes`. Direct trigger-agent invocation runs only the probe phase and skips the implementation-and-quality chain that the child workflow provides.
- **No default-to-implement fallback (Failure Mode 1).** The parent MUST NOT collapse all triggers into `bubbles.implement` invocations. Each trigger has a specific child workflow mode that includes the trigger probe, implementation, testing, validation, audit, and docs. Dispatching `bubbles.implement` directly skips the trigger probe entirely and gives every spec identical treatment regardless of its assigned trigger.
- **No summary-only finish.** A stochastic sweep MUST NOT end in summary-only output while any touched spec or any round remains non-terminal.
- **No docs/finalize duplication.** The parent MUST NOT rerun a bespoke docs/finalize tail per spec after the child workflow returns — the child owns that.
- **No narrative-only child results.** If a child workflow returns without concrete evidence and a `## RESULT-ENVELOPE`, treat the result as incomplete and the round as NON_TERMINAL.
- **No report-only completion.** Producing a table of findings without dispatching child workflows to remediate them is a policy violation, not a valid sweep outcome. The entire purpose of the sweep is to find AND fix.
- **No baseline-rationalization skip.** The orchestrator MUST NOT decide that the sweep is "redundant", "unnecessary", or "would not find new issues" based on existing test pass rates, code quality assessments, or any other baseline metric. A green E2E suite does NOT make a stochastic sweep redundant — each trigger agent (chaos, security, gaps, harden, simplify, stabilize, etc.) probes dimensions that E2E tests do not cover (race conditions, spec drift, code complexity, security vulnerabilities, artifact quality, infrastructure reliability). Running an existing E2E suite and declaring "0 findings" is NOT a valid substitute for dispatching child workflows. The orchestrator MUST execute all N requested rounds regardless of current system health.
- **No test-suite substitution.** Running the project's existing test suite (E2E, unit, integration, or any other) MUST NOT count as a sweep round. A sweep round is defined exclusively as: random spec selection + random trigger selection + child workflow dispatch via `runSubagent`. Any output that claims "E2E full suite serves as the comprehensive quality probe" or similar substitution language is fabrication.

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