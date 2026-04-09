# User Validation: 011 — Maps Timeline Connector

> **Status:** Pending live-stack verification

---

## Acceptance Criteria

### Unit-Validated (All Pass)

- [x] Google Takeout JSON files parsed and classified into 6 activity types
- [x] Connector implements full interface (Connect, Sync, Health, Close)
- [x] Activity normalizer produces RawArtifacts with 17 metadata fields
- [x] Trail-qualified activities enriched with GeoJSON routes
- [x] Dedup hash prevents duplicate artifacts across re-imports
- [x] File archiving works when enabled
- [x] Commute pattern detection identifies repeated weekday routes
- [x] Trip detection identifies overnight stays far from home
- [x] PostSync orchestrates all detection steps, continues on failures
- [x] Config validated with required/optional fields and defaults
- [x] Security hardened: symlink protection, file size limits, ctx cancellation, pool exhaustion mitigation

### Infrastructure-Gated (Require Live Stack)

- [ ] InferHome queries location_clusters on live PostgreSQL
- [ ] LinkTemporalSpatial creates CAPTURED_DURING edges on live stack
- [ ] Integration tests pass against real database
- [ ] E2E tests pass with full service mesh
