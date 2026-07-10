# Mapped Workflow Execution And Finding Closure Protocol

Use this module when `bubbles.workflow` runs a stochastic or other trigger-owned round after a trigger was selected.

## Core Contract

- The active authorized runner selects the target spec and trigger, then resolves `triggerWorkflowModes[trigger]` from `bubbles/workflows/modes.yaml`.
- The runner MUST execute the mapped workflow contract instead of pre-running the trigger or hand-building a bespoke fix cycle when a mapping exists.
- The mapped mode owns trigger execution and the full finding-owned closure chain for that spec.
- The runner remains in the top-level runtime, invokes each phase owner directly, and records `executionModel: direct-authorized-runner`. It never dispatches another workflow-running orchestrator.
- Every trigger in an active trigger pool MUST have a mapped delivery-capable workflow.
- Do not accept narrative-only success. Mapped modes must return concrete evidence and a `## RESULT-ENVELOPE`.

## Finding-Owned Closure Workflow

- Any specialist or mapped mode that discovers a legitimate bug, regression, design gap, operational gap, or improvement MUST complete a finding-owned closure workflow before returning `completed_owned` to its runner.
- Full finding-owned planning workflow: `bubbles.analyst` → `bubbles.ux` when the finding touches UI or a user-visible journey → `bubbles.design` → `bubbles.plan`.
- Full finding-owned delivery workflow: `bubbles.implement` → `bubbles.test` → `bubbles.validate` → `bubbles.audit` → `bubbles.docs` → finalize owned by the active workflow runner and certification owned by `bubbles.validate`.
- This applies to `chaos`, `test`, `simplify`, `stabilize`, `devops`, `security`, `validate`, `regression`, `harden`, `gaps`, and future trigger-style workflows.
- When the current workflow already owns some downstream phases later in the same mode, those phases remain workflow-owned, but the planning chain MUST still run before code changes continue and the workflow MUST NOT report terminal success until the downstream closure phases finish.

## Runner Responsibilities

- Pick the round target and trigger.
- Verify the current agent is granted the resolved mode, then execute it directly by invoking each phase owner.
- Pass through policy/options context that affects execution (`socratic`, `tdd`, `gitIsolation`, `autoCommit`, `specReview`, and similar workflow tags).
- Preserve any `route_required` or `blocked` outcome from the mapped mode. The runner must not downgrade those outcomes into a clean round summary.
- Wait for every mapped phase owner to return a terminal `## RESULT-ENVELOPE`, then aggregate those envelopes instead of narrating partially-closed work.
- **In round-based loops (stochastic sweep, iterate):** complete execute -> wait -> record for one round before starting the next. Selecting multiple rounds without executing mapped modes is a batch-then-summarize violation.

## Mapped Mode Responsibilities

- Own the trigger phase for the selected spec.
- Own any required finding-owned planning refresh, bug packet creation, implementation, testing, validation, audit, docs, and finalize work for that trigger mode.
- Return a concrete `## RESULT-ENVELOPE` outcome.
- Keep workflow-owned continuation intact if the selected spec remains non-terminal.

These responsibilities are carried by the active authorized runner. The mode name identifies the contract; it does not require a second workflow-agent runtime.

## Reject Malformed Success

Treat the child result as incomplete when any of these are true:

- Verification is claimed without execution evidence
- The mapped mode omits the result envelope
- The mapped mode asks the user to continue manually instead of routing or completing the work itself
- The runner ran the trigger directly as the entire round even though a mapped workflow mode existed
- The runner attempted to dispatch another workflow-running orchestrator instead of invoking phase owners directly
- The mapped mode reports `completed_owned` before the full finding-owned planning and delivery closure workflow reached a terminal outcome
- **The mapped mode returns a findings list, summary table, or narrative recommendations without having executed the finding-owned planning chain (analyst → ux → design → plan) and delivery chain (implement → test → validate → audit → docs) for each finding — this is a finding-only result, not a completed workflow**
- **The mapped mode ran its trigger phase (harden/gaps/security/chaos/stabilize/simplify/regression/etc.) but did NOT invoke `bubbles.implement` or any delivery specialist — the trigger-only probe is necessary but insufficient**

When a malformed result is detected, the parent MUST either:
1. Re-execute the same mapped workflow contract in the active runner with explicit instruction to complete the finding-owned closure chain, OR
2. Mark the round as `NON_TERMINAL` and include the unresolved findings in the continuation envelope

## Round Ledger Requirement

Every trigger-owned round must emit a ledger line that includes:

- `spec`
- `trigger`
- `triggerWorkflowMode`
- `mappedOutcome`
- `agents_invoked=[active-runner:<mode>, <phase agents...>]`
- `executionModel=direct-authorized-runner`
- `duration`

The ledger must prove which trigger-owned workflow actually ran. A round without `triggerWorkflowMode` is malformed.

## Summary And Continuation Contract

- Mapped workflow modes own docs/finalize/certification for touched specs. The runner must not add a bespoke duplicate docs/finalize tail after the mapped mode returns.
- If any round remains `route_required`, `blocked`, or otherwise non-terminal, the parent workflow must preserve a workflow-owned continuation packet instead of ending in summary-only output.
- Summary-only output is invalid while any trigger-owned round remains non-terminal.