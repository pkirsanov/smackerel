# <img src="../../icons/cathy-trail.svg" width="28"> Guided Journey

> *"Come on, I'll walk you through it — and tell you straight what's broken."*

Use this when the feature is built and running, and you want to walk the LIVE product toward a real goal WITH the user — capturing friction at each step and routing the fixes to the right owner.

## Problem

The code is done and the tests are green, but nobody has actually walked the finished product toward a concrete user goal. You want to drive the real UI/API step by step, see where it is unclear, inconvenient, missing, or broken, and turn that friction into planned refinements — without hand-editing specs, designs, or scopes.

## Solution

Start the cooperative-guided walkthrough with an explicit goal and a success signal:

```text
/bubbles.journey  Goal: rebalance my portfolio  (specs/NNN-portfolio-rebalance)
```

`bubbles.journey` (Cathy Curtis) drives the live dev/validate stack via the project browser-automation stack + direct API, and records each step's outcome as one of `works | unclear | inconvenient | missing | broken` with captured UI/API/telemetry evidence.

When you want the walkthrough to feed straight into planning, run it as a workflow mode:

```text
/bubbles.workflow  specs/NNN-portfolio-rebalance mode: journey-refinement
```

The mode structures `uservalidation.md` (acceptance stays human-owned — G057), appends discovered issues to `report.md`, and routes each refinement to its owner.

## Examples

- **QuantitativeFinance** — `Goal: rebalance my portfolio`. Cathy drives the dashboard from the holdings view through the rebalance proposal to the confirmation, and flags that the "apply rebalance" affordance is unclear (friction: `unclear`) → routes a scenario refinement to `bubbles.analyst`.
- **Smackerel** — `Goal: see this month's expenses in QuickBooks`. Cathy walks the connector-linked expense view, notices the month selector defaults to last month (friction: `inconvenient`) and the export-to-QuickBooks button is missing (friction: `missing`) → files the missing-feature refinement to `bubbles.plan` and the defect to `bubbles.bug`.

## When It Helps Most

- The feature is built but never walked toward a real goal
- You want usability friction captured with evidence, not opinions
- You need refinements routed to the right owner instead of patched inline
- You want the human to accept the experience, not automation

## Good Follow-Ups

- `/bubbles.analyst <feature>` when a friction finding is really a missing requirement
- `/bubbles.plan <feature>` when a refinement needs new scopes, tests, and DoD
- `/bubbles.bug <feature>` when a step exposed an actual defect with a reproduction
- `/bubbles.workflow <feature> mode: readiness-review` when you want a ship/no-ship synthesis after refinements land
