# Trigger Workflow Dispatch And Finding Closure Protocol

Use this module when `bubbles.workflow` runs a stochastic or other trigger-owned round after a trigger was selected.

## Core Contract

- Parent workflows select the target spec and trigger, then resolve `triggerWorkflowModes[trigger]` from `bubbles/workflows.yaml`.
- Parent workflows MUST dispatch the mapped child workflow mode instead of pre-running the trigger or hand-building a bespoke fix cycle when a mapping exists.
- The child workflow owns trigger execution and the full finding-owned closure chain for that spec.
- Every trigger in an active trigger pool MUST have a mapped delivery-capable child workflow.
- Do not accept narrative-only success. Child workflows must return concrete evidence and a `## RESULT-ENVELOPE`.

## Finding-Owned Closure Workflow

- Any specialist or child workflow that discovers a legitimate bug, regression, design gap, operational gap, or improvement MUST start a finding-owned closure workflow before returning `completed_owned` to its parent.
- Full finding-owned planning workflow: `bubbles.analyst` → `bubbles.ux` when the finding touches UI or a user-visible journey → `bubbles.design` → `bubbles.plan`.
- Full finding-owned delivery workflow: `bubbles.implement` → `bubbles.test` → `bubbles.validate` → `bubbles.audit` → `bubbles.docs` → finalize/certification owned by `bubbles.workflow` and `bubbles.validate`.
- This applies to `chaos`, `test`, `simplify`, `stabilize`, `devops`, `security`, `validate`, `regression`, `harden`, `gaps`, and future trigger-style workflows.
- When the current workflow already owns some downstream phases later in the same mode, those phases remain workflow-owned, but the planning chain MUST still run before code changes continue and the workflow MUST NOT report terminal success until the downstream closure phases finish.

## Parent Responsibilities

- Pick the round target and trigger.
- Invoke `bubbles.workflow` as a child workflow with the resolved mode and the selected spec target only.
- Pass through policy/options context that affects execution (`socratic`, `tdd`, `gitIsolation`, `autoCommit`, `specReview`, and similar workflow tags).
- Preserve any `route_required` or `blocked` outcome from the child workflow. Parent workflows must not downgrade those outcomes into a clean round summary.
- Parent workflows MUST wait for the child finding-owned workflow to reach a terminal `## RESULT-ENVELOPE`, then report that envelope upward instead of narrating partially-closed work.
- **In round-based loops (stochastic sweep, iterate):** the parent MUST complete steps dispatch → wait → record for one round before starting the next. Selecting multiple rounds without dispatching child workflows is a batch-then-summarize violation.

## Child Workflow Responsibilities

- Own the trigger phase for the selected spec.
- Own any required finding-owned planning refresh, bug packet creation, implementation, testing, validation, audit, docs, and finalize work for that trigger mode.
- Return a concrete `## RESULT-ENVELOPE` outcome.
- Keep workflow-owned continuation intact if the selected spec remains non-terminal.

## Reject Malformed Success

Treat the child result as incomplete when any of these are true:

- Verification is claimed without execution evidence
- The child workflow omits the result envelope
- The child workflow asks the user to continue manually instead of routing or completing the work itself
- The parent ran the trigger directly even though a mapped child workflow existed
- The child workflow reports `completed_owned` before the full finding-owned planning and delivery closure workflow reached a terminal outcome

## Round Ledger Requirement

Every trigger-owned round must emit a ledger line that includes:

- `spec`
- `trigger`
- `triggerWorkflowMode`
- `childOutcome`
- `agents_invoked=[bubbles.workflow(<mode>)]`
- `duration`

The ledger must prove which trigger-owned workflow actually ran. A round without `triggerWorkflowMode` is malformed.

## Summary And Continuation Contract

- Child workflows own docs/finalize/certification for touched specs. Parent workflows must not rerun a bespoke docs/finalize tail after the child returns.
- If any round remains `route_required`, `blocked`, or otherwise non-terminal, the parent workflow must preserve a workflow-owned continuation packet instead of ending in summary-only output.
- Summary-only output is invalid while any trigger-owned round remains non-terminal.