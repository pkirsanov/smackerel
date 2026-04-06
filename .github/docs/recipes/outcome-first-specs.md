# Recipe: Outcome-First Specs

> *"What's it supposed to DO, boys?"*

Define the outcome before the process. Every spec starts with an **Outcome Contract** — what the user should be able to do, how you'll prove it, and what must never break.

## When to Use

- Starting any new feature
- Reviewing a spec that only has process requirements but no clear success definition
- When validation passes but the feature doesn't actually solve the user's problem

## The Outcome Contract

Every `spec.md` must include this section (Gate G070):

```markdown
## Outcome Contract
**Intent:** [1-3 sentences: what outcome should be achieved from the user/system perspective]
**Success Signal:** [Observable, testable proof that the outcome was achieved — not "tests pass" but "user can do X and sees Y"]
**Hard Constraints:** [Business invariants that must hold regardless of implementation approach — these survive model upgrades]
**Failure Condition:** [What would make this feature a failure even if all tests pass]
```

## How It Works

1. **bubbles.analyst** writes the Outcome Contract during analysis (Phase 8)
2. **bubbles.plan** traces every scope back to the declared Intent
3. **bubbles.validate** checks the Success Signal is demonstrated in evidence (Step 0)
4. **bubbles.validate** verifies Hard Constraints are preserved
5. A feature that passes all process gates but fails its Outcome Contract is **NOT done**

## Example

```markdown
## Outcome Contract
**Intent:** Hosts can customize their public booking page layout with a visual drag-drop editor
**Success Signal:** A host logs in, rearranges sections on the page builder, publishes, and a guest visiting the public page sees the new layout
**Hard Constraints:** Published config must be served to guests within 5 seconds of publish; draft changes must never be visible to guests
**Failure Condition:** Host completes the editor flow but guest page shows default layout instead of customized one
```

## Two Types of Rules

The constitution now separates:

- **Business Invariants** — rules that survive model upgrades ("never double-charge," "PII encrypted at rest")
- **Model Compensations** — workarounds for current AI limitations ("evidence ≥10 lines," "batch-checking forbidden")

Review model compensations when models improve. Business invariants stay forever.

## Related

- [New Feature](new-feature.md) — full delivery pipeline
- [Brainstorm an Idea](brainstorm-idea.md) — explore before building
- [Plan Only](plan-only.md) — plan and scope without implementing
