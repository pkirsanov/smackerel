---
applyTo: "**"
---

# bubbles-propagation — non-negotiable cross-train propagation rules

Loaded automatically in every workspace that has `agents/bubbles.propagate.agent.md`.

## Authority

- `bubbles.propagate` (J-Roc) is the SOLE writer of `propagation-policy.yaml` and `propagation-ledger.yaml`.
- `bubbles.train` (DVS) is the SOLE writer of `config/release-trains.yaml` and `config/feature-flags.<train>.yaml`.
- `bubbles.devops` (Tommy Bean) is the SOLE executor of cherry-pick / merge operations on disk.

These three boundaries are enforced and MUST NOT be crossed.

## NO inline cherry-picks

`bubbles.propagate` MUST NOT run `git cherry-pick` itself. It emits `route_required` with target `bubbles.devops` and the cherry-pick payload. Tommy executes, returns success/failure, and J-Roc proceeds to validation + ledger.

Reason: separation of decision (J-Roc) from execution (Tommy) keeps the audit trail clean and lets either be reverted independently.

## NO bypassing receiving-train validation

Every `forward` or `backport` operation MUST route the receiving train through the validation mode declared in `propagation-policy.yaml`. The only exception is `receivingTrainValidationMode: none`, which requires `validationSkipReason` to be non-empty in the policy.

Reason: "I'll skip validation just this once" is how prod incidents start. The skip must be policy-declared and documented.

## NO backport without approval

When `backportRequiresApproval: true` (default for any policy shipping a `prod` train), `bubbles.propagate` MUST emit `route_required` with `action: human-approval` and refuse to proceed until invoked with `--approval-token=<sha>`. The token is recorded in the ledger.

Reason: backporting from prod to experimental is high-risk — it can re-introduce bugs the trunk has already moved past. Explicit human approval is the speed bump.

## NO ledger rewrites

`propagation-ledger.yaml` is append-only JSONL. Editing past entries fails G123 (propagation-ledger-recorded) and breaks the audit chain. If a ledger entry is wrong, append a corrective entry (`operation: correction`, referencing the original entry's timestamp).

## Failure semantics

If receiving-train validation fails after cherry-pick, J-Roc MUST:
1. Emit `route_required` to `bubbles.devops` with action `revert-cherry-pick`.
2. Wait for revert completion.
3. Append a ledger entry with `validationOutcome: failed`.
4. End with verdict `blocked` and a remediation message.

Silent rollback or partial-success claims are forbidden.

## Cross-references

- Skill: `bubbles-propagation-policy` — schema and authoring guide
- Recipe: `docs/recipes/propagate-changes.md` — NL-first operator guide
- Gates: G121, G122, G123
- Owning agent: `bubbles.propagate`
