# Features — Smackerel MVP

## Carried Forward From Prior Phases (Phase 1–5)

All capabilities below were certified `done` (or equivalent terminal-for-mode) before this MVP gate. They are carried forward into MVP as-is and are NOT re-implemented by this packet. Sources: [`docs/INVESTOR_OVERVIEW.md`](../../INVESTOR_OVERVIEW.md) Phase Overview, [`specs/_spec-review-report.md`](../../../specs/_spec-review-report.md) (65 CURRENT specs).

| Capability | Origin spec(s) | Status |
|------------|----------------|--------|
| Go core + Docker Compose stack + `./smackerel.sh` runtime | 001, 002 | delivered |
| PostgreSQL + pgvector + pg_trgm | 002 | delivered |
| NATS JetStream bus + production hardening | 002, 046 | delivered |
| Python ML sidecar (FastAPI + Ollama) | 002, 050 | delivered |
| Universal capture/processing pipeline | 002, 003 | delivered |
| Knowledge graph + topic lifecycle | 003, 063 | delivered |
| Semantic search (vague-in / precise-out) | 003 + 068 + 063 | delivered |
| Daily/weekly digest + Telegram delivery | 021 (subset) | delivered |
| Pre-meeting briefs + contextual alerts (baseline) | 021, 025 | delivered |
| Maps timeline + trip dossier + people intelligence | 005 + 011, 005 + 015, 005 + IMAP/CalDAV | delivered |
| Expertise mapping, content fuel, learning paths, subscription tracking, serendipity | 006 family | delivered |
| **Connector roster (LOCKED in MVP)** — see table below | 007–018, 038, 040, 041, 056, 059, 072, 074, ingest | delivered |
| Annotations (baseline) | 027 | delivered |
| Actionable lists | 028 | delivered |
| DevOps pipeline + observability + monitoring | 029, 030, 047, 049, 053 | delivered |
| Live-stack testing + ML sidecar health isolation | 031, 043, 050 | delivered |
| Documentation freshness + spec-banner sweep | 032, _ops/OPS-001 | delivered |
| Mobile capture (PWA) | 033 | delivered |
| Expense tracking, recipe enhancements, meal planning | 034, 035, 036 | delivered |
| LLM agent tools + structured intent compiler + intent-driven policy enforcement | 037, 068, 067 | delivered |
| Cloud drives + cloud photo libraries + QF companion connector | 038, 040, 041 | delivered |
| Tailnet-edge bind pattern | 042 | delivered |
| Per-user bearer auth + bearer auth scope claim + deployment secret/auth contracts | 044, 060, 051, 052 | delivered |
| Deploy resource filesystem hardening | 045 | delivered |
| Backup/restore automation | 048 | delivered |
| Notification intelligence handler (baseline) | 054 | delivered |
| Notification source ntfy adapter | 055 | delivered |
| Twitter API connector | 056 | delivered |
| Browser login redirect | 057 | delivered (`done_with_concerns` — see [`actions.md`](actions.md)) |
| Chrome extension bridge | 058 | delivered (`done_with_concerns`) |
| Google Keep live mode | 059 | delivered |
| Conversational assistant + knowledge AI enrichment + open-ended knowledge agent | 061, 063, 064 | delivered |
| Domain extraction pipeline | 026 | delivered (`MAJOR_DRIFT` on scenario manifest — fix dispatched, see [`actions.md`](actions.md)) |
| Assistant HTTP transport + intent-trace observability | 069, 071 | delivered |
| Web username/password login + web/mobile assistant frontend | 070, 073 | delivered |
| WhatsApp Business transport | 072 | delivered |
| Capture-as-fallback policy | 074 | delivered |
| Phase roadmap specs (intentionally high-level) | 001, 002, 003, 004, 005, 006 | delivered |

### Connector roster (LOCKED at MVP — no additions in MVP)

Source-of-truth: [`internal/connector/`](../../../internal/connector/).

| Connector | Source spec(s) |
|-----------|----------------|
| `alerts` (gov alerts) | 017 |
| `bookmarks` | 009 |
| `browser` (history) | 010 |
| `caldav` (calendar) | — (Phase 2 ingestion family) |
| `discord` | 014 |
| `guesthost` | 013 |
| `hospitable` | 012 |
| `imap` (Gmail/IMAP baseline) | — (Phase 2 ingestion family) |
| `ingest` (universal active capture) | 002, 003 |
| `keep` (Google Keep) | 007, 059 |
| `maps` | 011 |
| `markets` (financial markets) | 018 |
| `photos` (cloud photo libraries) | 040 |
| `qfdecisions` (QF companion) | 041 |
| `rss` | — (Phase 2/4 family) |
| `twitter` | 015, 056 |
| `weather` | 016 |
| `youtube` | — (Phase 2 ingestion family) |
| `cloud-drives` | 038 |
| `telegram-share-capture` | 008 |
| `whatsapp-business-transport` | 072 |

Any new connector after this MVP gate is RELEASE-V1 scope.

## New In This Phase (MVP — operator decisions 2026-06-03)

| ID | Capability | Owning existing spec(s) | Adjustment shape | New spec required? | Status |
|----|------------|-------------------------|------------------|---------------------|--------|
| **M1a** | Global user-interruption-budget controller ("Next Smackerel" prioritizer) — owns budget across digest + push + Telegram + web + ntfy + email-out, with measurable SLOs (≤ N nudges/day, target acted-on-rate, false-positive ceiling) | [`021-intelligence-delivery`](../../../specs/021-intelligence-delivery/) | Add **scope** "global-interruption-budget-controller"; add **design.md section** "Unified surfacing prioritizer + SLO contract"; add **scenarios** for budget-respect across all 6 channels | **No** — 021 is the credible host; it already owns delivery prioritization | planned |
| **M1b** | Calendar-triggered briefs (lead-time scheduled producers tied to CalDAV upcoming events) | [`025-knowledge-synthesis-layer`](../../../specs/025-knowledge-synthesis-layer/) (primary), scheduler in `internal/scheduler/` | Add **scope** "calendar-triggered-brief-producer"; add **design.md section** "Lead-time scheduled brief contract"; add **scenarios** for brief production at T-15m / T-1h / T-1d before event | **No** — 025 is the synthesis layer; calendar-trigger is a producer type | planned |
| **M1c** | Reminder / promise engine (user-stated future-intent reminders) — scheduler-backed | [`054-notification-intelligence-handler`](../../../specs/054-notification-intelligence-handler/) (primary), scheduler in `internal/scheduler/` | Add **scope** "reminder-promise-engine"; add **design.md section** "User-stated future-intent contract + conditional fires"; add **scenarios** for time-based ("ping me at Y"), condition-based ("ping me if X hasn't happened by Y"), and arrival-based ("remind me when Z arrives") | **No** — 054 already owns notification dispatch logic | planned |
| **M2** | Wiki / graph-browse UI surface in web — views by topic / person / place / time; rendered cross-links; editable annotations beyond 027 baseline | [`073-web-mobile-assistant-frontend`](../../../specs/073-web-mobile-assistant-frontend/) (primary), [`027-user-annotations`](../../../specs/027-user-annotations/) (annotation extension) | 073: Add **scope** "graph-browse-views"; add **design.md section** "Pivot views (topic/person/place/time) + cross-link rendering". 027: Add **scope** "wiki-surface-annotation-extensions"; add **design.md section** "Inline editable annotations on wiki surface" | **No** — 073 is the web/mobile assistant front-end; it is the credible browse-surface host | planned |
| **M3** | Ratify [`docs/Product-Principles.md`](../../Product-Principles.md) 1–10 — flip [`.github/instructions/product-principles.instructions.md`](../../../.github/instructions/product-principles.instructions.md) from advisory to BLOCKING | `.github/instructions/product-principles.instructions.md` | Single-file edit per [`actions.md`](actions.md) "Next Dispatches" | **No** — single instruction-file edit | planned |
| **M4** | Spec 026 MAJOR_DRIFT fix — rewire `scenario-manifest.json` away from deleted `internal/api/domain_intent*.go` | [`026-domain-extraction`](../../../specs/026-domain-extraction/) | `improve-existing` mode revision per [`actions.md`](actions.md) "Next Dispatches" | **No** — same spec | planned |
| **M5a** | Spec 039 MINOR_DRIFT — re-point evidence cell | [`039-recommendations-engine`](../../../specs/039-recommendations-engine/) | `improve-existing` cosmetic | **No** | planned |
| **M5b** | Spec 058 MINOR_DRIFT — resolve `done_with_concerns` | [`058-chrome-extension-bridge`](../../../specs/058-chrome-extension-bridge/) | `improve-existing` reconciliation | **No** | planned |
| **M5c** | Spec 067 MINOR_DRIFT — past-tense inventory entry | [`067-intent-driven-policy-enforcement`](../../../specs/067-intent-driven-policy-enforcement/) | `improve-existing` cosmetic | **No** | planned |
| **M5d** | `_ops/OPS-001` EB-7 idempotence verification | [`_ops/OPS-001-spec-banner-sweep`](../../../specs/_ops/OPS-001-spec-banner-sweep/) | `validate-only` mode grep verification | **No** | planned |

**All MVP items are hostable within existing specs.** No new spec is required for MVP. This is intentional — operator constraint excluded any item requiring a new spec from MVP.

## Plan-to-Release Traceability

| MVP item | Target spec dispatch | Dispatch mode |
|----------|----------------------|---------------|
| M1a | `specs/021-intelligence-delivery` | `improve-existing` |
| M1b | `specs/025-knowledge-synthesis-layer` | `improve-existing` |
| M1c | `specs/054-notification-intelligence-handler` | `improve-existing` |
| M2  | `specs/073-web-mobile-assistant-frontend` + `specs/027-user-annotations` | `improve-existing` (each) |
| M3  | `.github/instructions/product-principles.instructions.md` | `bubbles.docs` single-file edit |
| M4  | `specs/026-domain-extraction` | `improve-existing` |
| M5a | `specs/039-recommendations-engine` | `improve-existing` |
| M5b | `specs/058-chrome-extension-bridge` | `improve-existing` |
| M5c | `specs/067-intent-driven-policy-enforcement` | `improve-existing` |
| M5d | `specs/_ops/OPS-001-spec-banner-sweep` | `validate-only` |

## Capability evidence trace

Every "delivered" claim in the carry-forward table traces to a spec folder under [`specs/`](../../../specs/) whose `state.json` was certified at `done` (or terminal-for-mode equivalent) per [`specs/_spec-review-report.md`](../../../specs/_spec-review-report.md) audit on 2026-06-02. No capability is claimed delivered without a spec reference. No capability is fabricated.

Every M-item planned claim traces to a real owning spec; no M-item is implemented by this packet.

## Deprecations in MVP

None. All carry-forward capabilities remain in scope. The 026 MAJOR_DRIFT fix is a manifest reconciliation, not a capability removal — the domain-extraction pipeline itself is delivered and in MVP.
