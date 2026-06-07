# Spec: BUG-021-009 — seasonal volume significance must be LLM-driven

## Expected Behavior

Whether a year-over-year capture-volume change (or a candidate topic) represents
a meaningful seasonal pattern MUST be decided by the LLM per situation — not by
a hardcoded ratio threshold. The Go core (`DetectSeasonalPatterns`) gathers raw
signals only and sends them to the ML sidecar `seasonal.analyze`, which judges
significance. Only OPERATIONAL bounds (data-sufficiency floor, topic-candidate
floor + cap, observation cap) remain, SST-configured and fail-loud.

## Actual Behavior

`DetectSeasonalPatterns` applied `ratio < 0.7` (volume_drop) and `ratio > 1.5`
(volume_spike) directly in Go, and claimed `topic_seasonal` for any topic with
`COUNT(*) >= 5` this month. The ML `seasonal.analyze` enrichment was dead due to
a request/response key mismatch. See `bug.md`.

## Acceptance Criteria

1. **AC-1 (LLM judges):** `DetectSeasonalPatterns` sends raw signals to the
   `seasonal.analyze` ML path which decides significance; no Go code contains a
   hardcoded volume-ratio threshold or a "≥N captures = seasonal" claim.
2. **AC-2 (signals, not thresholds):** Go sends `{current_month, data_days,
   this_month_count, last_year_same_month_count, topic_candidates}`; the only
   Go-side numbers are operational ($min_data_days, $topic_min_captures,
   $topic_candidate_limit, $max_observations).
3. **AC-3 (ML judges + contract fixed):** `handle_seasonal_analyze` consumes the
   raw signals and returns `{observations:[{pattern, month, observation,
   actionable}]}`; the request/response contract matches the Go caller on both
   ends.
4. **AC-4 (no ratio fallback):** when the operational config is not wired, the
   ML sidecar is unavailable, or no LLM is configured, seasonal detection is
   skipped (no hardcoded ratio runs).
5. **AC-5 (operational bounds as SST):** `intelligence.seasonal.*` keys are
   fail-loud SST; missing/invalid values are rejected naming the key.

## Out of Scope

- Live-LLM behavioral validation (live-stack tier).
- The cooling / alert-timing / resurfacing / expertise producers (already
  converted in BUG-021-005/006/007/008).
- Re-deriving the monthly report's other sections.

## Cross-References

- Bug detail + the operational/business boundary + dead-contract finding: `bug.md`
- Sibling (expertise): `../BUG-021-008-expertise-llm-driven/`
- Architecture: `docs/smackerel.md` §3.6
