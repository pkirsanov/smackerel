# User Validation Checklist

## Checklist

- [x] Baseline checklist initialized for this feature

## Phase-Spec User Validation Policy

> Closes finding **GAP-005-G2** (stochastic-quality-sweep round 12,
> disposition: close-as-policy). See
> `specs/_ops/sweep-round-012-gap-005/routing.md` and the matching
> entry in `state.json` `discoveredIssues`.

Phase-rollup specs (such as this one) **defer per-feature user
validation to their constituent feature specs and to the release
packets under `docs/releases/`**. Per-scope user acceptance evidence
is not back-filled into the phase aggregator, because doing so would
duplicate evidence and create a second source of truth that drifts
from the constituent specs.

### Where Phase 4 user validation actually lives

**Constituent connector specs** (verified against
`specs/005-phase4-expansion/spec.md` §Goals R-401 / R-402 and the
phase-4 surface area):

- [`specs/010-browser-history-connector`](../010-browser-history-connector/spec.md)
  — opt-in browser history connector (R-402).
- [`specs/011-maps-connector`](../011-maps-connector/spec.md) —
  Google Maps Timeline connector (R-401).

In-phase capabilities (trip dossier assembly, trail/route journals,
people intelligence) do not have standalone connector specs; their
user acceptance is captured in the release packets below alongside
the constituent connectors.

**Release packets** (phase-level acceptance):

- [`docs/releases/mvp/`](../../../docs/releases/mvp/) — MVP release
  packet (vision, features, deployment, ops/scalability).
- [`docs/releases/v1/`](../../../docs/releases/v1/) — v1 release
  packet covering the post-MVP expansion surface area.

### Why this policy exists

Phase specs are aggregators. The MVP map in
[`specs/001-smackerel-mvp/spec.md`](../001-smackerel-mvp/spec.md)
(row "Phase 4 — Expansion") points to this phase as a planning
container; the actual user-facing acceptance evidence lives with the
constituent feature specs and the release packets that ship them.
This `uservalidation.md` therefore intentionally remains a
bootstrap-stub checklist and defers to the artifacts above.
