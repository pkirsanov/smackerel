# <img src="../../icons/donny-ducttape.svg" width="28"> Safe Shared-Infrastructure Refactor

> *"One little change in the wrong shared helper and the whole park starts burning down."*

Use this when the refactor target is a shared fixture, harness, global setup, auth bootstrap, session bootstrap, storage injection path, or any other helper surface with a wide downstream blast radius.

## Problem

You already know the risky surface. The danger is not finding it. The danger is breaking ten downstream assumptions while cleaning up one file.

## Solution

Run a constrained workflow with isolation, small scope sizing, and validated milestone commits:

```text
/bubbles.workflow  <feature> mode: simplify-to-doc gitIsolation: true autoCommit: scope grillMode: required-on-ambiguity tdd: true maxScopeMinutes: 60 maxDodMinutes: 30
```

If retro already identified the hotspot and you want the same protections:

```text
/bubbles.workflow  <feature> mode: retro-to-simplify gitIsolation: true autoCommit: scope grillMode: required-on-ambiguity tdd: true maxScopeMinutes: 60 maxDodMinutes: 30
```

## What Must Exist In Planning

- A `Shared Infrastructure Impact Sweep` that lists the downstream contract surfaces and likely blast radius
- An independent `Canary:` test row that proves the changed helper works for real consumers before the broader suite reruns
- A rollback or restore path for the shared helper change
- A `Change Boundary` listing allowed file families and excluded surfaces that must stay untouched

## Why This Is Different From Normal Simplification

- Shared helpers are treated as protected infrastructure, not casual cleanup targets
- The workflow now blocks completion if canary, rollback, or change-boundary controls are missing
- Narrow repair loops are expected to stay narrow; unrelated cleanup has to be split out or explicitly re-planned
- `autoCommit: scope` gives you a validated rollback point instead of one giant refactor diff

## Use It For

- Refactoring shared auth or login fixtures
- Cleaning up global Playwright or bootstrap helpers
- Reworking tenant/session/bootstrap state injection code used by many tests
- Tightening shared test harness behavior without dragging unrelated handlers or mocks into the same pass

## Do Not Use It For

- Pure feature implementation where the surface is not shared infrastructure
- Broad redesigns that intentionally span many file families
- Fast bugfixes where the risky helper is not being refactored

## Related Recipes

- [Simplify Existing Code](simplify-existing-code.md) — normal simplification when the surface is not high blast radius
- [Data-Driven Simplification](retro-driven-simplify.md) — let retro choose the targets first
- [Structured Commits](structured-commits.md) — why validated milestone commits matter for risky refactors