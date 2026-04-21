# <img src="../../icons/tyrone-chain.svg" width="28"> Recipe: Autonomous Goal

> *"I handle things, that's what I do."* — Tyrone

## Situation

You have a single goal — a feature to implement, a bug to fix, an ops or stabilization problem to close, or a system improvement to make — and you want the agent to handle everything autonomously from start to finish. No hand-holding. No intermediate check-ins. Just the goal and the result.

## Command

```
/bubbles.goal  <your goal in plain English>
```

## Examples

```
/bubbles.goal  Implement the security deposit hold/release feature for guestHost

/bubbles.goal  Fix all broken E2E tests and make chaos scenarios pass

/bubbles.goal  Add webhook notification system to the booking flow

/bubbles.goal  Refactor the calendar module to eliminate all lint warnings and increase test coverage to 100%

/bubbles.goal  Stabilize the deployment pipeline, close config drift, and don't stop until validation and docs are clean
```

## What Happens

1. Tyrone parses your goal, searches the codebase for existing work
2. Creates spec/design/scopes if they don't exist
3. Implements all scopes sequentially
4. Runs full verify suite: unit + integration + E2E + chaos + validate + audit
5. Remediates ALL findings (searches web/docs if stuck)
6. Loops until convergence (zero findings + all gates pass) or max 10 iterations
7. Produces a result envelope with completion status

## When To Use

- You trust the agent to make good decisions autonomously
- The goal is well-defined enough to be decomposed into scopes
- You want end-to-end delivery including E2E, chaos, and audit
- The work is feature, bug, ops, hardening, or stabilization oriented and should run to full convergence

## When NOT To Use

- You want to review decisions at each step → use `full-delivery` instead
- The goal is vague and needs brainstorming first → use `bubbles.grill` or `product-to-planning`
- You have multiple goals → use `bubbles.sprint` instead

## Tips

- Be specific about what "done" looks like in your goal description
- The more context you give, the better the initial plan will be
- If you want TDD, add it: `/bubbles.goal mode: autonomous-goal tdd: true <goal>`

---

*"Peace. Tyrone got this."*
