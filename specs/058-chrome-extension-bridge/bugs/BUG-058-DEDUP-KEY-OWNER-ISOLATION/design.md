# Design: BUG-058-DEDUP-KEY-OWNER-ISOLATION

## Problem

`ComputeDedupKey` (`internal/connector/ingest/dedup.go`) hashes
`(url, content_type, source_device_id, bucket)` with no `owner_user_id`, and
`raw_ingest_dedup.dedup_key` is a global `PRIMARY KEY`. The dedup table is
therefore shared across all owners; two authenticated users with the same
`(url, content_type, source_device_id, bucket)` tuple collapse onto one row.
See `bug.md` for the verified mechanism and reproduction.

This document is a PROPOSAL routed to `bubbles.design`, the owner of the dedup
key contract (parent `design.md` §2.3 "Resolved Decisions"). It MUST be
ratified before `bubbles.implement` changes the keyer — the chaos round did
NOT change the keyer unilaterally.

**Decision (ratified 2026-06-06):** The operator ratified per-owner dedup
namespacing — it is never correct for owner B to receive owner A's
`artifact_id`. Delivered via `bubbles-workflow mode: bugfix-fastlane`
(parent-expanded). Parent `design.md` §2.3 is amended to state the
owner-inclusive preimage; OQ-2 is resolved (global dedup was an oversight, not
an intentional storage optimization). The implementation, tests, and red→green
evidence are in [report.md](report.md).

## Proposed Change

Prepend `owner_user_id` to the dedup key preimage:

```
dedup_key = SHA-256( owner_user_id ‖ \x00 ‖ url ‖ \x00 ‖ content_type
                     ‖ \x00 ‖ source_device_id ‖ \x00 ‖ bucket )
```

### Why this shape

- Owner becomes the outermost namespace, so no cross-owner collision is
  possible regardless of `source_device_id` values.
- The single-owner-multi-device behavior (`TestComputeDedupKey_VariesByDevice`,
  "Chrome Sync") is unchanged — within one owner, device still varies the key.
- The null-byte separator hygiene already proven by
  `TestComputeDedupKey_BoundaryCollisionResistance` and the Round-18
  `TestComputeDedupKey_SeparatorInjectionResistance` extends to the new owner
  component for free.

## Schema Impact

None. `dedup_key` stays `BYTEA PRIMARY KEY`; only the preimage changes. The
table holds no production data behind this blocked spec, so no data migration
is needed. If the spec ever unblocks with live data present, a one-time rebuild
of `raw_ingest_dedup` would be required — the migration note MUST call that out.

## Blast Radius

- `internal/connector/ingest/dedup.go` — `ComputeDedupKey` signature gains an
  `ownerUserID string` first parameter.
- `internal/api/connectors/extension/ingest.go` — the single caller passes
  `ownerUserID(ctx)`.
- `internal/connector/ingest/dedup_test.go` — 8 keyer call sites updated plus
  a new cross-owner adversarial twin.
- `tests/integration/extension_dedup_race_test.go` — re-run against live
  Postgres (BLOCKER-2 harness, resolved 2026-06-05); add a cross-owner
  row-separation assertion.
- Parent `design.md` §2.3 — key tuple amended (owned by `bubbles.design`).
- `scenario-manifest.json` + parent `scopes.md` Scope 2 — new scenario
  `SCN-058-022` (owned by `bubbles.plan`).

## Alternatives Considered

- **Composite PK `(owner_user_id, dedup_key)`** instead of folding owner into
  the hash. Rejected: requires a schema/migration change and an owner predicate
  on every UPDATE/INSERT; folding owner into the preimage keeps the
  single-column PK and the existing query shape.
- **Rely on globally-unique `source_device_id`** (the `auto-<uuidv4>` default).
  Rejected: operator-set ids (design §2.3 allows `[a-z0-9-]`) defeat this, and a
  server-side isolation invariant must not depend on client-chosen uniqueness.

## Open Questions

See `bug.md` → "Open Questions" (OQ-1 artifact-retrieval scoping, OQ-2 was
global dedup intentional). `bubbles.design` resolves OQ-2 before ratifying.
