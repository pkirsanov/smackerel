# Trigger Workflow Dispatch And Finding Closure Protocol

Use this module when `bubbles.workflow` runs a stochastic or other trigger-owned round after a trigger was selected.

## Core Contract

- Parent workflows select the target spec and trigger, then resolve `triggerWorkflowModes[trigger]` from `bubbles/workflows/modes.yaml`.
- Parent workflows MUST execute the mapped child workflow mode instead of pre-running the trigger or hand-building a bespoke fix cycle when a mapping exists.
- The mapped child workflow mode owns trigger execution and the full finding-owned closure chain for that spec.
- Runtime compatibility: if a nested `bubbles.workflow` child lacks the `agent`/`runSubagent` tool, the parent orchestrator MUST execute the resolved child mode in parent-expanded form from the current runtime. This is not a direct-trigger shortcut; it is the same mapped mode executed without recursive tool dependency.
- Every trigger in an active trigger pool MUST have a mapped delivery-capable child workflow.
- Do not accept narrative-only success. Mapped child workflow modes must return concrete evidence and a `## RESULT-ENVELOPE`.

## Finding-Owned Closure Workflow

- Any specialist or child workflow that discovers a legitimate bug, regression, design gap, operational gap, or improvement MUST start a finding-owned closure workflow before returning `completed_owned` to its parent.
- Full finding-owned planning workflow: `bubbles.analyst` → `bubbles.ux` when the finding touches UI or a user-visible journey → `bubbles.design` → `bubbles.plan`.
- Full finding-owned delivery workflow: `bubbles.implement` → `bubbles.test` → `bubbles.validate` → `bubbles.audit` → `bubbles.docs` → finalize/certification owned by `bubbles.workflow` and `bubbles.validate`.
- This applies to `chaos`, `test`, `simplify`, `stabilize`, `devops`, `security`, `validate`, `regression`, `harden`, `gaps`, and future trigger-style workflows.
- When the current workflow already owns some downstream phases later in the same mode, those phases remain workflow-owned, but the planning chain MUST still run before code changes continue and the workflow MUST NOT report terminal success until the downstream closure phases finish.

## Parent Responsibilities

- Pick the round target and trigger.
- Execute the resolved mode for the selected spec, either by invoking `bubbles.workflow` as a nested child when nested delegation is available, or by parent-expanding the resolved mode when nested delegation is unavailable.
- Pass through policy/options context that affects execution (`socratic`, `tdd`, `gitIsolation`, `autoCommit`, `specReview`, and similar workflow tags).
- Preserve any `route_required` or `blocked` outcome from the mapped mode. Parent workflows must not downgrade those outcomes into a clean round summary.
- Parent workflows MUST wait for the mapped finding-owned workflow mode to reach a terminal `## RESULT-ENVELOPE`, then report that envelope upward instead of narrating partially-closed work.
- **In round-based loops (stochastic sweep, iterate):** the parent MUST complete steps execute -> wait -> record for one round before starting the next. Selecting multiple rounds without executing mapped child workflow modes is a batch-then-summarize violation.

## Child Workflow Responsibilities

- Own the trigger phase for the selected spec.
- Own any required finding-owned planning refresh, bug packet creation, implementation, testing, validation, audit, docs, and finalize work for that trigger mode.
- Return a concrete `## RESULT-ENVELOPE` outcome.
- Keep workflow-owned continuation intact if the selected spec remains non-terminal.

When executed in parent-expanded form, these child workflow responsibilities are carried by the current `bubbles.workflow` runtime; do not spawn another `bubbles.workflow` child just to satisfy the naming convention.

## Reject Malformed Success

Treat the child result as incomplete when any of these are true:

- Verification is claimed without execution evidence
- The mapped mode omits the result envelope
- The mapped mode asks the user to continue manually instead of routing or completing the work itself
- The parent ran the trigger directly as the entire round even though a mapped child workflow mode existed
- A nested child reported missing `runSubagent` and the parent stopped instead of parent-expanding the mapped mode while the parent still had `runSubagent`
- The mapped mode reports `completed_owned` before the full finding-owned planning and delivery closure workflow reached a terminal outcome
- **The mapped mode returns a findings list, summary table, or narrative recommendations without having executed the finding-owned planning chain (analyst → ux → design → plan) and delivery chain (implement → test → validate → audit → docs) for each finding — this is a finding-only result, not a completed workflow**
- **The mapped mode ran its trigger phase (harden/gaps/security/chaos/stabilize/simplify/regression/etc.) but did NOT invoke `bubbles.implement` or any delivery specialist — the trigger-only probe is necessary but insufficient**

When a malformed result is detected, the parent MUST either:
1. Re-execute the same mapped child workflow mode with explicit instruction to complete the finding-owned closure chain, OR
2. Mark the round as `NON_TERMINAL` and include the unresolved findings in the continuation envelope

## Round Ledger Requirement

Every trigger-owned round must emit a ledger line that includes:

- `spec`
- `trigger`
- `triggerWorkflowMode`
- `childOutcome`
- `agents_invoked=[bubbles.workflow(<mode>)]` for nested execution OR `agents_invoked=[parent-expanded:<mode>, <phase agents...>]` for parent-expanded execution
- `executionModel=nested-child-workflow|parent-expanded-child-mode`
- `duration`

The ledger must prove which trigger-owned workflow actually ran. A round without `triggerWorkflowMode` is malformed.

## Summary And Continuation Contract

- Mapped child workflow modes own docs/finalize/certification for touched specs. Parent workflows must not rerun a bespoke docs/finalize tail after the mapped mode returns.
- If any round remains `route_required`, `blocked`, or otherwise non-terminal, the parent workflow must preserve a workflow-owned continuation packet instead of ending in summary-only output.
- Summary-only output is invalid while any trigger-owned round remains non-terminal.