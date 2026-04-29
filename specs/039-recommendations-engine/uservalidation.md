# User Validation Checklist: 039 Recommendations Engine

Links: [scopes.md](scopes.md) | [report.md](report.md) | [scenario-manifest.json](scenario-manifest.json)

User acceptance checklist. Items default to `[x]` (no regression observed against the planned outcome). Uncheck any item where implementation reveals a user-level deviation from [spec.md](spec.md) or [design.md](design.md).

## Checklist

### Reactive recommendations

- [x] Reactive ramen-style query returns sourced top-3 with personal-graph rationale (BS-001)
- [x] Coffee-style query with no graph signals returns candidates labeled "no personal signals" (BS-002)
- [x] Single-provider outage degrades gracefully without blocking the response (BS-006)
- [x] Mobile query never leaks raw GPS to provider (BS-008)
- [x] No-providers configuration returns explicit `no_providers` outcome with no fabricated candidates (BS-011)
- [x] Adding a new provider participates in scenarios via registry only (BS-013)
- [x] Hallucinated provider candidate rejected before delivery (BS-014)
- [x] Ambiguous query asks one clarification with zero provider calls (BS-015)
- [x] Conflicting opening-hours facts both rendered with `source_conflict=true` (BS-016)
- [x] Provider attribution badge + link rendered and persisted (BS-019)
- [x] Hard vegetarian constraint outranks raw popularity (BS-020)
- [x] No silent relaxation of hard constraints (BS-029)
- [x] Travel-effort basis labeled honestly (BS-030)
- [x] Low-confidence candidate disclosed without overstated rationale (BS-032)

### Feedback, suppression, and why

- [x] `Why?` answers with zero provider calls (BS-010)
- [x] Not-interested suppression scoped to originating watch (BS-005)
- [x] Disliked suppression crosses watches and queries (BS-012)
- [x] Preference correction stops the inferred signal and is cited later (BS-024)

### Watches and scheduler

- [x] Dwell-trigger location watch fires once per rate window (BS-003)
- [x] Rate limit withholds surplus matches (BS-004)
- [x] Price-drop alert only on real threshold crossing (BS-007)
- [x] Trip-context watch attaches grouped recommendations to trip dossier (BS-009)
- [x] Stale source data cannot proactively alert (BS-017)
- [x] Quiet hours withhold delivery and audit decision (BS-018)
- [x] No proactive watch is created from passive behavior (BS-021)
- [x] Watch scope cannot broaden silently — new consent revision required (BS-022)
- [x] Repeat cooldown suppresses unchanged alert (BS-028)

### Policy, quality, and operator

- [x] Sponsored does not buy rank above stronger organic (BS-023)
- [x] Restricted-category candidate withheld with category-level reason (BS-025)
- [x] Recalled product does not send ordinary deal alert (BS-026)
- [x] Near-duplicate diversity enforced by default (BS-027)
- [x] Total-cost transparency: unknown shipping/return facts visible (BS-031)
- [x] Operator can filter `/admin/agent/traces` by `recommendation-*` scenarios

### Non-functional

- [x] Latency NFR holds under 50 concurrent warm reactive requests for 5 minutes
- [x] Metrics emit with bounded labels; per-watch visibility via audit-table join
- [x] Logs and traces never leak provider keys, raw payloads, exact GPS, or sensitive graph text

Unchecked items indicate a user-reported regression that must be remediated before certification.
