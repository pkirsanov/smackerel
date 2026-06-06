# BUG-058-DEDUP-KEY-OWNER-ISOLATION: dedup_key omits owner_user_id → cross-tenant collapse

**Status:** Open (routed to design owner — contract decision required)
**Severity:** Medium
**Reported:** 2026-06-06
**Reporter:** Chaos Sweep Round 18 (parent: stochastic-quality-sweep) — `chaos-hardening`, parent-expanded
**Owner:** `bubbles.design` (dedup-key contract is planning truth — design §2.3 "Resolved Decisions")
**Affected feature:** `specs/058-chrome-extension-bridge/`
**Affected surface:** `internal/connector/ingest/dedup.go` (`ComputeDedupKey`, `PostgresDedupStore.ResolveOrPublish`), `internal/db/migrations/040_raw_ingest_dedup.sql`

## Summary

The server-authoritative dedup key is computed as
`sha256(url, content_type, source_device_id, bucket)` — `owner_user_id`
is **not** part of the preimage — and the persisted column
`raw_ingest_dedup.dedup_key` is a **global** `PRIMARY KEY`. As a result the
dedup table is shared across **all** owners: two different authenticated
users who emit the same `(url, content_type, source_device_id, bucket)`
tuple collide on a single row. The first writer wins; the second user's
artifact is silently **not published**, and the second user receives the
first user's `artifact_id`.

This is a multi-tenant data-isolation gap. It is distinct from the
single-user-multi-device case the design's dedup threat model targets
(`TestComputeDedupKey_VariesByDevice` / "Chrome Sync"): that case correctly
keeps per-device rows for one owner. The design simply never extended the
key to namespace owners, even though `owner_user_id` is stored in the row
and the admin devices view is owner-scoped.

## Mechanism (verified by code reading at repo HEAD)

1. `internal/connector/ingest/dedup.go` →
   `func ComputeDedupKey(url, contentType, deviceID string, bucket int64) []byte`
   — preimage = `url ‖ \x00 ‖ contentType ‖ \x00 ‖ deviceID ‖ \x00 ‖ bucket`.
   No `owner_user_id`.
2. `internal/db/migrations/040_raw_ingest_dedup.sql` →
   `dedup_key BYTEA PRIMARY KEY` — globally unique across owners.
3. `PostgresDedupStore.ResolveOrPublish`:
   - `UPDATE raw_ingest_dedup SET visit_count = visit_count + 1, last_seen_at = $2 WHERE dedup_key = $1 RETURNING artifact_id`
     — **no** `owner_user_id` predicate.
   - `INSERT ... ON CONFLICT (dedup_key) DO UPDATE ...` — conflict target is
     `dedup_key` alone.
4. `internal/api/connectors/extension/ingest.go` → `processItem` populates
   `DedupRow.OwnerUserID = ownerUserID(ctx)` (stored in the row) but the
   key passed to `ResolveOrPublish` is owner-independent.

## Reproduction (logical — needs the deferred live-Postgres harness to run)

1. User A (session owner `u-alice`) POSTs a bookmark of `https://github.com`
   with `metadata.source_device_id = "laptop"`. A row is inserted with
   `dedup_key = K`, `owner_user_id = u-alice`, `artifact_id = art-A`.
2. User B (session owner `u-bob`) POSTs a bookmark of `https://github.com`
   with `metadata.source_device_id = "laptop"`. `ComputeDedupKey` produces
   the **same** `K`. The UPDATE-first path matches User A's row, bumps
   `visit_count` → 2, and returns `(art-A, deduped=true)`.
3. **Outcome:** User B's `publish` callback is **skipped** — User B's
   bookmark is never created. User B's per-item outcome carries
   `artifact_id = art-A` (User A's id). User A's `visit_count` is inflated
   by User B's traffic.

`source_device_id = "laptop"` is a natural operator-set value (design §2.3
allows operator-set `[a-z0-9-]` ids); the collision needs no out-of-band
knowledge when two users pick the same common device name and capture the
same popular URL.

## Impact / Severity rationale (Medium)

- **Data suppression (primary harm):** a second owner's genuine capture is
  silently dropped. A multi-tenant isolation/correctness defect.
- **Cross-owner `artifact_id` disclosure:** the second owner receives the
  first owner's `artifact_id`. Impact is bounded **iff** artifact retrieval
  is owner-scoped (not verified in this round — flagged as an open question).
- **Counter poisoning:** `visit_count` / `last_seen_at` on the first owner's
  device row are inflated by another owner's traffic (visible in the admin
  devices view, which attributes the row to the first owner).
- **Mitigating factors:** the **default** `source_device_id` is
  `auto-<uuidv4>` (globally unique) → no accidental collision for default
  users; the collision requires two authenticated accounts; the surface is
  not remote-unauthenticated.

## Relationship to Round-18 chaos fix (finding F1)

Round 18 hardened the server `source_device_id` charset/length validation
(`internal/api/connectors/extension/ingest.go`, `sourceDeviceIDRe`). That fix
**narrows** the surface (no null-byte / control-char / unbounded device ids
can enter the key preimage) but does **not** resolve this bug — two **valid**
`[a-z0-9-]` device names (e.g. both `"laptop"`) still collide across owners.

## Recommended fix (design-owned — DO NOT change the keyer unilaterally)

1. **`bubbles.design`** — amend design §2.3 "Resolved Decisions": dedup key
   tuple becomes `(owner_user_id, url, content_type, source_device_id, bucket)`.
2. **`bubbles.plan`** — add a scenario (e.g. `SCN-058-022`): two owners with
   identical `(url, content_type, source_device_id, bucket)` MUST resolve to
   **separate** artifacts (no cross-owner collapse); extend the Scope 2
   DoD / Test Plan.
3. **`bubbles.implement`** — prepend `owner_user_id` to the `ComputeDedupKey`
   preimage. **No schema change** is required (`dedup_key` stays `BYTEA
   PRIMARY KEY`; only the preimage changes; the table holds no production
   data behind this blocked spec). Update the single caller in
   `internal/api/connectors/extension/ingest.go` and the keyer's unit call
   sites in `internal/connector/ingest/dedup_test.go`.
4. **`bubbles.test`** — add the cross-owner adversarial twin (unit) and
   re-run `tests/integration/extension_dedup_race_test.go` against the live
   Postgres harness (BLOCKER-2 — resolved 2026-06-05).
5. **`bubbles.validate`** — recertify Scope 2.

## Open Questions

- **OQ-1:** Is downstream artifact retrieval owner-scoped? If yes, the
  `artifact_id` disclosure is a leak of an opaque identifier only; if no, the
  severity rises. Route to the artifact-store owner to confirm.
- **OQ-2:** Was global (cross-owner) dedup ever an intentional storage-saving
  decision? Design §2.3 does not say. If intentional, this bug is `wontfix`
  with a documented rationale; the evidence suggests it was an oversight
  (owner is stored in the row + the admin view is owner-scoped).

## Cross-References

- Parent spec: [`../../spec.md`](../../spec.md), design: [`../../design.md`](../../design.md) §2.3, scopes: [`../../scopes.md`](../../scopes.md)
- Parent report: [`../../report.md`](../../report.md) → `## Chaos Sweep — Round 18 (2026-06-06)`
- Keyer: `internal/connector/ingest/dedup.go`
- Migration: `internal/db/migrations/040_raw_ingest_dedup.sql`
- Round-18 hardening (F1): `internal/api/connectors/extension/ingest.go` (`sourceDeviceIDRe`)
