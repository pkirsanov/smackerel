---
name: bubbles-workflow-execution-loops
description: Run per-round synchronous workflow execution loops correctly. Use when orchestrating a multi-round workflow, dispatching specialist sub-agents, handling round-based remediation, or interpreting a workflow mode's execution policy. Covers authorized direct execution, dispatch-and-wait, batch-then-summarize prohibition, and mapped-mode execution.
---

# Bubbles Workflow Execution Loops

## Goal
Execute workflow modes round-by-round with synchronous dispatch-and-wait, no report-only sweep completion, and no batch-then-summarize shortcuts.

## When to use
- Orchestrating any workflow mode that runs rounds (full-delivery, stochastic sweeps, fix-cycle, finding-driven loops)
- Dispatching specialist sub-agents and waiting for their RESULT-ENVELOPE
- Executing a mapped mode directly from the active authorized runner
- Reading a workflow mode definition to understand its loop policy

## Non-negotiables
1. **Synchronous dispatch-and-wait per round.** The orchestrator dispatches one (or a small batch of) specialist agents, waits for each terminal envelope, processes findings, and then decides the next round. Batch-then-summarize is forbidden.
2. **Mapped-mode execution.** When a trigger has a mapped workflow mode, the active runner resolves and executes that contract directly. Do not run only the trigger phase.
3. **Wait for mapped-mode terminal envelope.** Before the runner advances, every mapped mode phase owner's terminal result must be in run state.
4. **No report-only sweep completion.** A sweep that emits only a summary without round-level mapped-mode execution is incomplete; the workflow agent rejects this shape.
5. **No runner nesting.** Workflow execution occurs only in the authorized top-level runner. It invokes specialist phase owners directly and records `executionModel: direct-authorized-runner`.

## Authoritative modules
- `agents/bubbles_shared/workflow-execution-loops.md` — full per-round protocol
- `agents/bubbles_shared/workflow-fix-cycle-protocol.md` — fix-cycle round semantics, finding-set closure
- `agents/bubbles_shared/workflow-orchestration-core.md` — dispatcher contract
- `agents/bubbles_shared/workflow-delegation-core.md` — natural-language routing + work selection ownership
- `bubbles/workflows/modes.yaml` — per-mode loop policy (requiredGates, phaseRelevance, triggerWorkflowModes)
- `bubbles/workflows.yaml` — execution options + gate/phase wiring (resolver composes both files)
