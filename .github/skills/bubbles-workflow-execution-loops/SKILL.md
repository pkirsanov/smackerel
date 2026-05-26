---
name: bubbles-workflow-execution-loops
description: Run per-round synchronous workflow execution loops correctly. Use when orchestrating a multi-round workflow, dispatching specialist sub-agents, handling round-based remediation, or interpreting a workflow mode's execution policy. Covers dispatch-and-wait, batch-then-summarize prohibition, mapped-mode execution, and the parent-expanded child workflow fallback.
---

# Bubbles Workflow Execution Loops

## Goal
Execute workflow modes round-by-round with synchronous dispatch-and-wait, no report-only sweep completion, and no batch-then-summarize shortcuts.

## When to use
- Orchestrating any workflow mode that runs rounds (full-delivery, stochastic sweeps, fix-cycle, finding-driven loops)
- Dispatching specialist sub-agents and waiting for their RESULT-ENVELOPE
- Deciding whether to spawn a child workflow or run a parent-expanded fallback
- Reading a workflow mode definition to understand its loop policy

## Non-negotiables
1. **Synchronous dispatch-and-wait per round.** The orchestrator dispatches one (or a small batch of) specialist agents, waits for each terminal envelope, processes findings, and then decides the next round. Batch-then-summarize is forbidden.
2. **Mapped-mode execution.** When a trigger has a mapped child workflow mode, run that mode. Do not run a stochastic parent that fires only trigger-rounds when a mapped child workflow exists.
3. **Wait for mapped-mode terminal envelope.** Before the parent workflow returns, the mapped child workflow's terminal result must be in the parent's run state.
4. **No report-only sweep completion.** A sweep that emits only a summary without round-level mapped-mode execution is incomplete; the workflow agent rejects this shape.
5. **Parent-expanded child workflow fallback.** If the runtime cannot nest sub-agent calls, the parent must expand the child workflow's planning chain inline and execute the same sequence.

## Authoritative modules
- `agents/bubbles_shared/workflow-execution-loops.md` — full per-round protocol
- `agents/bubbles_shared/workflow-fix-cycle-protocol.md` — fix-cycle round semantics, finding-set closure
- `agents/bubbles_shared/workflow-orchestration-core.md` — dispatcher contract
- `agents/bubbles_shared/workflow-delegation-core.md` — natural-language routing + work selection ownership
- `bubbles/workflows.yaml` — per-mode loop policy, execution options, gate wiring
