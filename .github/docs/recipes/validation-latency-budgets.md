# Validation Latency Budgets

## Why

Validation latency is a framework health signal. A passing gate that takes twice as long as the previous run still changes operator behavior: it slows repair loops, increases context pressure, and makes evidence gathering harder to trust. SCOPE-6 adds a lightweight report so maintainers can see phase duration trends before they become quality problems.

## Sample Invocation

Run the direct report when you only need latency data:

```bash
bash bubbles/scripts/validation-latency-report.sh --since 7 --group phase
```

Run it through the trajectory inspector when you want the session trajectory and latency table in one operator view:

```bash
bash bubbles/scripts/trajectory-inspector.sh --latency --since 7
```

For a single spec, keep the same path shape used by `state.json`:

```bash
bash bubbles/scripts/validation-latency-report.sh --since 7 --spec specs/NNN-feature-name
```

## Sample Output

```markdown
# Validation Latency Report

Session file: .specify/memory/bubbles.session.json
Window: last 7 day(s)
Spec filter: specs/NNN-feature-name
Group: phase

| Phase | Agent | Spec | Count | P50 | P95 | Max | Budget | Within? |
|---|---|---|---:|---:|---:|---:|---:|---|
| implement | all | specs/NNN-feature-name | 3 | 18m0s | 29m0s | 29m0s | 30m0s | yes |
| test | all | specs/NNN-feature-name | 3 | 7m0s | 13m0s | 13m0s | 15m0s | yes |
| validate | all | specs/NNN-feature-name | 2 | 5m0s | 8m0s | 8m0s | 10m0s | yes |
| audit | all | specs/NNN-feature-name | 1 | 4m0s | 4m0s | 4m0s | 10m0s | yes |

Scanned records: 9
Valid durations: 9
Skipped records: 0
```

## Per-Phase Budgets

| Phase | P95 Budget | Why This Budget Exists |
|---|---:|---|
| implement | 30m | Implementation can include the narrow red-to-green loop, but sustained p95 above this usually means scopes are too broad or failure containment is weak. |
| test | 15m | Tests should prove the changed behavior without turning every local loop into a full release rehearsal. |
| validate | 10m | Validation should certify evidence and route concrete gaps, not rediscover the whole feature. |
| audit | 10m | Audit should inspect known artifacts and findings with bounded scope. |
| regression | 15m | Regression sweeps may be broader than unit checks, but p95 above this needs explicit blast-radius review. |
| docs | 10m | Managed docs updates should be bounded once implementation evidence exists. |

## Retro Cross-Link

SCOPE-7 retro convergence health consumes latency trends as supporting context for loop efficiency. When p95 phase time exceeds a budget, the retro should explain whether the cause was scope size, repeated repair loops, missing evidence, or external validation blockers.

## Trajectory Cross-Link

SCOPE-8 trajectory health mode is the broader session view. The `trajectory-inspector.sh --latency` bridge added with SCOPE-6 lets an operator inspect the current trajectory and append the same latency table without learning a second command surface.

## Bubbles Source Repository Note

The Bubbles source repository itself does not keep persistent `specs/` packets. Use generic or downstream spec paths when documenting examples, and use framework validation/selftests plus release manifest checks for source-repo evidence.
