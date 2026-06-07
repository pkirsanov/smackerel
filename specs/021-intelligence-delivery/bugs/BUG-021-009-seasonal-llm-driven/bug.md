# BUG-021-009: seasonal volume significance must be LLM-driven, not a hardcoded YoY ratio threshold

**Status:** Resolved (LLM-driven seasonal significance via bugfix-fastlane — see report.md)
**Severity:** Medium
**Reported:** 2026-06-07
**Resolved:** 2026-06-07
**Reporter:** Owner directive — "there should be NO const limits; all should be decided by LLM depending on situation" (continuation of BUG-021-008)
**Owner:** `bubbles.workflow` (parent-expanded bugfix-fastlane; the active runtime lacks `runSubagent`)
**Affected feature:** `specs/021-intelligence-delivery/`
**Affected surface:** `internal/intelligence/monthly.go` (`DetectSeasonalPatterns`), `ml/app/intelligence.py` (`handle_seasonal_analyze`), `config/prompt_contracts` (n/a — uses the existing `seasonal.analyze` ML/NATS path)

## Summary

After BUG-021-008 made expertise tier/growth LLM-driven, the monthly report's
seasonal pattern detection (`DetectSeasonalPatterns`, R-508) still decided "is
this year-over-year volume change a meaningful seasonal pattern?" with hardcoded
ratio thresholds:

- **Volume drop**: `ratio < 0.7` → emit a `volume_drop` pattern.
- **Volume spike**: `ratio > 1.5` → emit a `volume_spike` pattern.
- **Topic seasonal**: `HAVING COUNT(*) >= 5` this month → emit a `topic_seasonal`
  pattern claiming the topic "tends to spike."

These answer the same domain question the product architecture says must be
LLM-driven (docs/smackerel.md §3.6). Whether a 30% drop or a 50% spike is
*seasonally meaningful* depends entirely on the situation — base volume,
month, the user's history. A fixed `0.7`/`1.5` cutoff cannot capture that, and
"≥5 captures this month" is not evidence a topic is *seasonal*.

## Mechanism (the old, hardcoded path) + a pre-existing dead integration

`DetectSeasonalPatterns` computed the YoY ratio and applied the `0.7`/`1.5`
thresholds directly in Go, then appended a `topic_seasonal` pattern for any
topic with ≥5 same-month captures. It THEN attempted to enrich via the ML
sidecar `seasonal.analyze` — but that path was **dead due to a contract
mismatch on both ends**:

- Request: Go sent `{current_month, data_days, local_patterns}`; Python read
  `data.get("patterns")` / `data.get("month")` → always received empty patterns.
- Response: Python returned `{patterns: [...]}`; Go read `resp.Observations`
  (`{observations: [...]}`) → never bound anything.

So the ML enrichment had been silently inert since BUG-003, and the only live
behavior was the hardcoded Go thresholds.

## Fix (delivered — LLM judges significance via the existing ML path; Option A)

1. **`DetectSeasonalPatterns` reworked** to gather raw SIGNALS only — this-month
   vs last-year-same-month counts, and candidate topics above an operational
   floor — and send them to the ML sidecar `seasonal.analyze`. No `0.7`/`1.5`
   ratio decision and no "≥5 = seasonal" claim remain in Go.
2. **`handle_seasonal_analyze` reworked** (`ml/app/intelligence.py`) to JUDGE
   significance from the raw signals: it decides whether the YoY change is a
   meaningful seasonal drop/spike and which candidate topics are genuinely
   seasonal, returning structured `observations`. This also FIXES the
   request/response contract mismatch (one coherent contract on both ends).
3. **No hardcoded fallback**: when the operational config is not wired, the ML
   sidecar is unavailable, or no LLM is configured, seasonal detection is
   SKIPPED (the monthly report omits the section) — there is no magic-number
   ratio fallback. Consistent with BUG-021-005/006/007/008.
4. **Operational bounds → SST** (fail-loud): `intelligence.seasonal.{min_data_days,
   topic_min_captures, topic_candidate_limit, max_observations}` — a
   data-sufficiency floor (6+ months, R-508), a topic-candidate floor + cap, and
   an observation cap. None of these decide significance; the LLM does.

## Operational vs business boundary

Per docs/smackerel.md §3.6 + constitution C8: **business reasoning → LLM**;
**operational limits → SST config (fail-loud)**. The "is this seasonally
meaningful?" JUDGMENT is the LLM's (in the ML sidecar). The remaining numbers
(maturity floor, topic candidate floor/cap, observation cap) bound the job —
they do not decide significance.

## Why the ML/NATS path (not a new agent.Bridge scenario)

Seasonal already had an ML sidecar touchpoint (`seasonal.analyze`). Extending
that existing path to judge significance keeps the feature's design coherent and
avoids a redundant second LLM integration for the same concern — this is Option
A as discussed with the owner. The four prior conversions
(BUG-021-005/006/007/008) used the `agent.Bridge` scenario loader because those
producers had no existing ML path; seasonal does.

## Relationship to BUG-021-005/006/007/008

Same directive, same operational/business boundary. This is the fifth
hardcoded-business-threshold conversion in the sweep, and the first delivered
through the ML/NATS path rather than `agent.Bridge`.

## Cross-References

- Detection: `internal/intelligence/monthly.go` (`DetectSeasonalPatterns`)
- ML judge: `ml/app/intelligence.py` (`handle_seasonal_analyze`)
- Wiring: `cmd/core/wiring_cooling.go` (`wireSeasonalConfig`)
- SST loader: `internal/config/seasonal.go`
- Sibling (expertise): `../BUG-021-008-expertise-llm-driven/`
- Architecture: `docs/smackerel.md` §3.6
