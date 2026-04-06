# Recipe: Regression Check

> *"Something's prowlin' around in the code, boys."* — Steve French detects cross-feature interference.

---

## The Situation

You've implemented a feature or fixed a bug and want to make sure nothing else broke — especially in other features that share routes, tables, components, or APIs.

## Quick Check

```
/bubbles.regression  check for regressions after booking changes
```

**What happens:**
1. Runs the full test suite and captures a baseline snapshot
2. Scans changed files for cross-spec dependencies
3. Runs tests from affected specs (already-done features)
4. Checks for route/API/table collisions between specs
5. Verifies test coverage didn't decrease

**You'll get:** A verdict — `REGRESSION_FREE`, `REGRESSION_DETECTED`, or `CONFLICT_DETECTED` — with specific findings.

## Full Pipeline (Automatic — Built Into Delivery)

You don't need to run regression checks manually in most cases. Every delivery mode now includes the `regression` phase automatically:

```
implement → test → regression → simplify → stabilize → security → docs → ...
```

Steve French prowls automatically after every implementation.

## Targeted Regression Checks

```
# Check if a specific feature was affected
/bubbles.regression  did we break the page builder?

# Compare test baselines
/bubbles.regression  compare test counts before and after

# Check for spec conflicts
/bubbles.regression  check if new feature conflicts with existing ones

# Verify UI flows survive
/bubbles.regression  make sure UI flows still work after theme changes
```

## Stochastic Regression Sweeps

Use the stochastic quality sweep with regression as a trigger:

```
/bubbles.workflow  stochastic-quality-sweep triggerAgents: regression maxRounds: 5
```

Steve French prowls randomly across specs, looking for interference that targeted checks might miss.

## When Steve French Finds Something

If regressions or conflicts are detected:
1. The regression agent routes fixes to the right specialist:
   - Failing tests → `bubbles.implement` to fix
   - Coverage gaps → `bubbles.test` to add tests
   - Design conflicts → `bubbles.design` to resolve
   - UI flow breaks → `bubbles.ux` to verify
2. After fixes, the regression check re-runs automatically

Like a cougar guarding its territory — nothing gets past Steve French.
