# Recipe: TDD First Execution

> *"Red first. Green after. No greasy shortcuts."*

---

## The Situation

You know the feature or bug you want to work on, but you do not want the workflow to drift into implementation-first behavior. You want Bubbles to keep the loop test-first: fail the right test, make it pass, then prove it stayed clean.

This recipe does **not** turn on spec/design/plan readiness, Gherkin-first planning, or scenario-to-test mapping. Those are already mandatory baseline workflow rules.

## The Command

```
/bubbles.workflow  <feature-or-bug> mode: product-to-delivery tdd: true
```

For existing code or bugs:

```
/bubbles.workflow  <feature-or-bug> mode: improve-existing tdd: true
/bubbles.workflow  <feature-or-bug> mode: bugfix-fastlane tdd: true
```

If the direction itself still feels shaky, combine it with a pressure pass first:

```
/bubbles.workflow  <feature-or-bug> mode: product-to-delivery grillMode: required-on-ambiguity tdd: true
```

## What The Tag Changes

With `tdd: true`, the workflow should bias toward:
- reproducing the missing or broken behavior before writing the implementation
- adding or tightening the smallest failing test that proves the gap
- implementing only enough to turn that test green
- keeping regression evidence attached to the same scope or bug path

What it does **not** change:
- it does not relax or replace the requirement for coherent spec/design/plan artifacts
- it does not replace Gherkin-first planning
- it does not replace the requirement for scenario-specific tests to be identified before coding

## Good Fits

Use this when:
- the behavior is crisp enough to express in a failing test
- the team keeps slipping into code-first work
- you are fixing regressions and want proof before touching the implementation
- you want smaller implementation loops with clearer evidence

## Pair It With

- `/bubbles.grill ...` when the direction still needs sharper questions
- `grillMode: required-on-ambiguity` when you want workflow-level pressure before planning or implementation
- `backlogExport: tasks|issues` when planning should also emit copy-ready execution items