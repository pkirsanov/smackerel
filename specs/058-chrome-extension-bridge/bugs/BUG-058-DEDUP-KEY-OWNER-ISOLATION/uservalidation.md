# User Validation: BUG-058-DEDUP-KEY-OWNER-ISOLATION

This is a discovery + routing packet from Chaos Sweep Round 18. End-user
validation of the FIX is pending `bubbles.design` ratification and the
downstream delivery chain. The checked items below reflect what is genuinely
complete in Round 18 (discovery + documentation + the confirming probe); the
unchecked items are the pending fix-validation gates.

## Checklist

- [x] Finding mechanism verified by code reading at repo HEAD (`internal/connector/ingest/dedup.go`, `internal/db/migrations/040_raw_ingest_dedup.sql`, `internal/api/connectors/extension/ingest.go`)
- [x] Round-18 confirming probe `TestComputeDedupKey_SeparatorInjectionResistance` ran and passed (keyer null-byte separator hygiene intact)
- [x] Bug packet (bug.md, spec.md, design.md, scopes.md, report.md, state.json) filed and routed to bubbles.design
- [ ] Cross-owner isolation fix implemented (`owner_user_id` folded into the dedup-key preimage)
- [ ] Live-Postgres test proves two owners with the same `(url, content_type, source_device_id, bucket)` tuple resolve to separate artifacts
- [ ] design/operator confirms global dedup was not intended (OQ-2)
