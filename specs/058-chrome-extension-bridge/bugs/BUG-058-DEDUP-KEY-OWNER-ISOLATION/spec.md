# Spec: BUG-058-DEDUP-KEY-OWNER-ISOLATION — dedup key must namespace owners

## Expected Behavior

The server-authoritative dedup contract MUST isolate owners. Two different
authenticated users that emit the same logical capture
`(url, content_type, source_device_id, bucket)` MUST resolve to **separate**
artifacts. One user's capture MUST NOT be suppressed, mis-attributed, or
have its `artifact_id` disclosed to another user because of a shared
`source_device_id` value.

## Actual Behavior

`ComputeDedupKey` omits `owner_user_id`, and `raw_ingest_dedup.dedup_key` is a
global `PRIMARY KEY`. The first owner to write a given tuple wins; a second
owner with the same tuple is collapsed onto the first owner's row — the second
owner's `publish` is skipped and the first owner's `artifact_id` is returned.
See `bug.md` → "Mechanism" + "Reproduction".

## Acceptance Criteria

1. **AC-1 (key namespaces owner):** The dedup key preimage includes
   `owner_user_id` such that two owners with identical
   `(url, content_type, source_device_id, bucket)` produce **different**
   `dedup_key` values. (Design §2.3 amended by `bubbles.design`.)
2. **AC-2 (no cross-owner collapse — unit):** A unit test in
   `internal/connector/ingest/dedup_test.go` asserts
   `ComputeDedupKey(ownerA, …) != ComputeDedupKey(ownerB, …)` for otherwise
   identical tuples, and a sibling positive test asserts same-owner
   determinism is preserved.
3. **AC-3 (no cross-owner collapse — integration):** A live-Postgres test
   (alongside `tests/integration/extension_dedup_race_test.go`) proves two
   owners with the same tuple yield two distinct `raw_ingest_dedup` rows and
   two distinct `artifact_id`s, with neither `publish` skipped.
4. **AC-4 (scenario + DoD):** A new scenario (e.g. `SCN-058-022`) is added to
   `scenario-manifest.json` and the Scope 2 DoD / Test Plan in `scopes.md`.
5. **AC-5 (no regression to single-owner-multi-device):** The existing
   `TestComputeDedupKey_VariesByDevice` ("Chrome Sync") behavior is preserved
   — one owner's distinct devices still map to distinct keys.

## Out of Scope

- The Round-18 `source_device_id` charset/length hardening
  (`internal/api/connectors/extension/ingest.go`) — already shipped this round;
  it narrows but does not resolve this bug.
- Schema migration of `raw_ingest_dedup` — none is required (the column stays
  `BYTEA PRIMARY KEY`; only the preimage changes; no production data exists
  behind this blocked spec).
- The four external-infrastructure gaps tracked in
  `../BUG-058-EXTERNAL-INFRA-MISSING/`.

## Cross-References

- Bug detail + recommended fix + open questions: `bug.md`
- Parent design (contract to amend): `../../design.md` §2.3
- Parent report (chaos evidence): `../../report.md` → `## Chaos Sweep — Round 18 (2026-06-06)`
