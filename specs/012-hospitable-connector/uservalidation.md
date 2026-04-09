# User Validation: 012 — Hospitable Connector

> **Status:** Done

---

## Checklist

- [x] Hospitable connector implements Connector interface (ID, Connect, Sync, Health, Close)
- [x] PAT Bearer authentication validated on Connect() — 401/403 handled gracefully
- [x] Properties synced with paginated GET, correct struct mapping
- [x] Reservations synced with guest name, dates, financial summary, channel
- [x] Messages synced per-reservation with 3-way sender classification (guest/host/automated)
- [x] Reviews synced with fractional rating precision (4.5★) and host response
- [x] Incremental cursor sync with per-resource-type JSON timestamps
- [x] Initial lookback window applied on first sync (configurable days)
- [x] Rate limit (429) triggers exponential backoff with Retry-After header parsing
- [x] Disabled resource types skipped without cursor advancement
- [x] Knowledge graph edge hints: BELONGS_TO, PART_OF, REVIEW_OF, DURING_STAY window
- [x] Property name cache enriches reservation/review titles, persists across syncs
- [x] Partial failure isolation — one resource type failing does not block others
- [x] Active reservation message sync fetches messages for non-incremental reservations
- [x] Artifact URLs populated (listing URL for properties, dashboard URL for reservations)
- [x] Processing tiers: properties=light, reservations=standard, messages=full, reviews=full
- [x] Config section in smackerel.yaml with SST pipeline integration
- [x] Connector registered in main.go following Keep pattern
- [x] All 52 unit tests pass across connector_test.go and normalizer_test.go
- [x] ./smackerel.sh lint passes
- [x] ./smackerel.sh check passes
