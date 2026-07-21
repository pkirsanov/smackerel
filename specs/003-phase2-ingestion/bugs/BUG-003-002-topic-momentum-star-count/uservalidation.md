# User Validation Checklist: BUG-003-002

## Checklist

- [x] The packet preserves the user-visible R-208 contract: explicit stars increase only the momentum of topics containing those starred artifacts.
- [x] The packet preserves honest scheduled-job behavior: a database query failure is logged as a failure, not reported as a successful momentum update.
- [x] The packet excludes migration, deployment, release-train, secret, manifest, and health-feature changes.

Unchecked checklist entries are reserved for user-reported regressions.

## Goal

- Goal: Restore automatic topic momentum and lifecycle updates from canonical persisted user signals.
- Success signal: Zero-star and multi-star topics receive the correct persisted momentum after the scheduled lifecycle query runs.

## Journey Steps

| Step | User Intent | Observed | Evidence | Friction |
|---|---|---|---|---|
| 1 | Let Smackerel evolve topics automatically | Live red-team observed the lifecycle SQL failing on a missing column | [report.md](report.md#bug-reproduction---live-red-team-observation) | broken |

## Open Refinements

None found - the repair is bounded to the canonical lifecycle query and its regression coverage.
