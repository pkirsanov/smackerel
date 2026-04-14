# User Validation: 013 — GuestHost Connector & Hospitality Intelligence

Links: [spec.md](spec.md) | [design.md](design.md) | [scopes.md](scopes.md) | [report.md](report.md)

---

## Validation Checklist

Items are checked `[x]` by default (validated via audit). Uncheck `[ ]` to report broken behavior.

### Scope 01: GH Connector — API Client, Types & Config
- [x] GH API client authenticates correctly with tenant API key
- [x] Activity feed pagination follows hasMore flag
- [x] Rate limit (429) triggers retry with backoff
- [x] Config validation rejects missing api_key and base_url when enabled

### Scope 02: GH Connector — Implementation & Normalizer
- [x] Connector lifecycle works: Connect → Sync → Health → Close
- [x] All 11 GH event types map to correct content types and titles
- [x] Metadata includes property_id, guest_email, booking_id, revenue, booking_source
- [x] Cursor advances to last event timestamp
- [x] Event type filter restricts synced events

### Scope 03: Hospitality Graph Nodes & Linker
- [x] Guest nodes created/updated from booking, review, and message artifacts
- [x] Property nodes created/updated from property-tagged artifacts
- [x] STAYED_AT, REVIEWED, ISSUE_AT, DURING_STAY edges created correctly
- [x] Returning guests tagged when total_stays > 1
- [x] Hospitality topics seeded on first sync
- [x] Multi-source guest nodes merge correctly (source="both")

### Scope 04: Hospitality Digest
- [x] Digest includes today's arrivals with returning-guest detection
- [x] Digest includes today's departures
- [x] Digest includes pending tasks across properties
- [ ] Revenue snapshot shows 24h/7d/30d breakdown by channel and property *(Reconciliation 2026-04-12: only 7d/30d implemented; no 24h window; no per-channel breakdown)*
- [x] Guest and property alerts surface in digest
- [x] Empty day omits hospitality sections (not shown empty)
- [x] No hospitality connectors active → standard digest only

### Scope 05: Context Enrichment API
- [x] POST /api/context-for returns full guest context with history, sentiment, hints
- [x] POST /api/context-for returns full property context with performance, topics, hints
- [x] Unknown guest/property returns 404 (not empty 200)
- [ ] Communication hints are rule-based and deterministic *(Reconciliation 2026-04-12: repeat_guest/vip/positive_reviewer hints work; early-checkin and direct-booking-% rules not implemented)*
- [x] API key authentication enforced
- [x] Include parameter controls response sections
