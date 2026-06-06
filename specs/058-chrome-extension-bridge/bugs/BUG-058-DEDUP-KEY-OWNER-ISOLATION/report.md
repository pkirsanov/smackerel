# Report: BUG-058-DEDUP-KEY-OWNER-ISOLATION

## Summary

Filed 2026-06-06 by Chaos Sweep Round 18 (parent `stochastic-quality-sweep`,
trigger `chaos`, mapped child mode `chaos-hardening`, parent-expanded). A chaos
probe of the spec 058 ingest/dedup surface surfaced a multi-tenant
data-isolation gap: the dedup key omits `owner_user_id` and
`raw_ingest_dedup.dedup_key` is a global `PRIMARY KEY`, so two authenticated
owners that share a `source_device_id` value collapse onto one dedup row.

This packet is **discovery + routing only**. No implementation, test, or
validation phase has run on it. The fix changes planning truth (design §2.3
dedup key contract) and is therefore routed to `bubbles.design`, NOT applied
unilaterally in the chaos round.

## Completion Statement

Discovery and documentation are complete: the finding is verified by code
reading at repo HEAD, a confirming keyer probe was executed, and the full
six-artifact bug packet is filed and routed to `bubbles.design`. The fix itself
is NOT done — it is a planning-truth (design §2.3) change owned by
`bubbles.design` and is intentionally out of scope for the chaos round. This
bug remains `open` until the owner chain in `scopes.md` Scope 1 completes. The
Round-18 `source_device_id` charset hardening (parent report F1) narrows but
does not resolve this isolation gap.

## Discovery Evidence

All evidence is from code reading at repo HEAD (the live-Postgres reproduction
requires the integration harness — BLOCKER-2, resolved 2026-06-05 — to execute
end-to-end):

- `internal/connector/ingest/dedup.go:36` —
  `func ComputeDedupKey(url, contentType, deviceID string, bucket int64) []byte`;
  preimage has no `owner_user_id`.
- `internal/db/migrations/040_raw_ingest_dedup.sql:13` —
  `dedup_key BYTEA PRIMARY KEY` (global uniqueness).
- `internal/connector/ingest/dedup.go` — `ResolveOrPublish` UPDATE/INSERT both
  key on `dedup_key` alone (no `owner_user_id` predicate / conflict component).
- `internal/api/connectors/extension/ingest.go` — `processItem` stores
  `DedupRow.OwnerUserID` in the row but passes an owner-independent key to
  `ResolveOrPublish`.

The Round-18 chaos suite also added a CONFIRMING probe
(`TestComputeDedupKey_SeparatorInjectionResistance`, PASS) proving the keyer's
null-byte separator hygiene is intact — so this bug is specifically about the
**missing owner component**, not a separator weakness.

## Test Evidence

This is a discovery/routing packet; no fix code was written, so there is no
fix-validation run. The evidence below is the chaos-probe and code-inspection
evidence that established the finding.

### Confirming probe (executed this round, under -race)

```
$ go test -race -count=1 -run TestComputeDedupKey_SeparatorInjectionResistance ./internal/connector/ingest/
--- PASS: TestComputeDedupKey_SeparatorInjectionResistance (0.00s)
ok      github.com/smackerel/smackerel/internal/connector/ingest        1.025s
```

The keyer's null-byte separator hygiene is intact, so the collision in this bug
is NOT a separator weakness — it is the missing `owner_user_id` component.

### Cross-owner collision (logical reproduction — execution pending harness)

The cross-owner collapse is established by code inspection (see Discovery
Evidence): `ComputeDedupKey` has no owner input, so for any fixed
`(url, content_type, source_device_id, bucket)` it returns an identical key for
every owner, and `ResolveOrPublish` resolves it with no owner predicate. The
end-to-end live-Postgres reproduction (two owners, same tuple, asserting two
distinct rows) is the AC-3 gate and runs once the fix and integration assertion
land (harness available per BLOCKER-2, resolved 2026-06-05). It is honestly NOT
executed in this round.

## Why This Is Routed (not fixed in-round)

- The dedup key tuple is declared planning truth in `design.md` §2.3
  ("Resolved Decisions"). Per `bubbles-artifact-ownership-routing`, a change to
  another owner's contract is routed, not made unilaterally.
- The sibling packet `../BUG-058-EXTERNAL-INFRA-MISSING/spec.md` explicitly
  lists "the dedup keyer" as a shipped/stable contract (its "Out of Scope").
- Changing `ComputeDedupKey`'s signature touches the live-Postgres integration
  harness (`tests/integration/extension_dedup_race_test.go`) which cannot be
  executed from this chaos-round environment — so unilateral change could not
  be validated end-to-end here.

## Routing

Status: `open`. Severity: `medium`.
Resolution chain (owner-routed): `bubbles.design` (amend design §2.3) →
`bubbles.plan` (scenario + DoD) → `bubbles.implement` (prepend `owner_user_id`
to the `ComputeDedupKey` preimage; update caller + keyer unit tests) →
`bubbles.test` (cross-owner unit twin + live-Postgres integration re-run) →
`bubbles.validate` (recertify Scope 2). The appropriate workflow mode is
`product-to-delivery` (a contract amendment that needs analyze/design/plan/
implement/test/validate), not a fastlane.

## Next Required Owner

`bubbles.design` — confirm intent (OQ-2: was global dedup intentional?) and, if
not, amend the design §2.3 dedup key contract to namespace `owner_user_id`,
then hand off down the chain above.
