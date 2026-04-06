# <img src="../../icons/private-dancer-lamp.svg" width="28"> Grill an Idea

> *"Let's get it under the light and see if it survives."*

Use this when the idea sounds promising but you do not trust the first version of the story yet.

## Problem

You have a feature idea, design direction, or workflow plan, but you want hard questions before anyone starts building.

## Solution

Start with the dedicated pressure-test agent:

```text
/bubbles.grill  <describe the idea, design, or plan>
```

If you already know you want a workflow after the grilling pass, carry the pressure-test into the workflow itself:

```text
/bubbles.workflow  <feature> mode: product-discovery grillMode: required-on-ambiguity
```

Or for direct delivery:

```text
/bubbles.workflow  <feature> mode: full-delivery grillMode: required-on-ambiguity
```

## When It Helps Most

- The idea is still vague
- The design feels too easy
- The rollout or migration risk is unclear
- You suspect missing consumers, tests, or observability
- You want the workflow mode and tags challenged before execution starts

## Good Follow-Ups

- `/bubbles.analyst <feature>` when product framing is the problem
- `/bubbles.design <feature>` when the technical shape is weak
- `/bubbles.plan <feature> backlogExport: tasks` when the concept is solid enough to break into execution work