---
name: bubbles-fix-cycle-protocol
description: Run a Bubbles fix-cycle correctly: pick up findings, route them to owners, ensure full finding-set closure, and avoid cherry-picking. Use during fix-cycle rounds, when handling rework packets, when the workflow agent reports unresolved findings, or when a trigger-style specialist hands findings back to the parent.
---

# Bubbles Fix-Cycle Protocol

## Goal
Close every finding raised in a round. No cherry-picking. No silent disappearance. Every finding has a deterministic next owner.

## When to use
- Workflow agent dispatches a fix cycle round
- Handling a `route_required` packet from a specialist
- Inheriting a rework packet from a previous round
- Resolving validate/audit findings that the implement agent must address

## Non-negotiables
1. **Full finding-set closure.** Every finding in the round MUST end the round in `addressedFindings` (with evidence), `unresolvedFindings` (with owner), or a structured `followUps[]` entry under `done_with_concerns`.
2. **No selective remediation.** Implement may not cherry-pick the easy findings. The agent's envelope MUST account for every finding individually.
3. **Trigger-owned closure workflow.** When a trigger maps to a child workflow mode, that child workflow owns closeout. The parent waits.
4. **No bespoke parent-side fix cycles.** The shared protocol governs all fix cycles; parents do not invent custom resolution loops.
5. **Cherry-pick detection.** The state-transition guard rejects status promotion when previously-reported findings disappear between rounds without a recorded resolution.

## Round flow
1. Read all open findings carried into the round (from prior round's `unresolvedFindings`, validate/audit certification.* outputs, rework packets).
2. Group findings by owner (the specialist whose artifact requires the fix).
3. Dispatch each owner with the finding ledger. Wait for terminal envelope per owner.
4. Process every returned envelope: addressedFindings + unresolvedFindings must collectively re-cover the round's input ledger.
5. If unresolvedFindings remain, schedule the next round on the appropriate owner.
6. The round terminates `completed_owned` only when no unresolved findings remain across all owners.

## Authoritative modules
- `agents/bubbles_shared/workflow-fix-cycle-protocol.md` — full protocol
- `agents/bubbles_shared/workflow-execution-loops.md` — per-round synchronous execution
- `agents/bubbles_shared/completion-governance.md` — sequential completion + deferral blocking
- `agents/bubbles_shared/critical-requirements.md` — anti cherry-pick rule (e.g., timing-attack/JWT example)
- `bubbles/scripts/state-transition-guard.sh` — cherry-pick detection
