# User Validation: BUG-021-009

**Reported by:** Owner directive — "there should be NO const limits; all should be decided by LLM depending on situation"
**Validated:** 2026-06-07

## Acceptance

- [x] AC-1 — `DetectSeasonalPatterns` sends raw signals to the `seasonal.analyze` ML path which decides significance; no hardcoded volume-ratio threshold or "≥N captures = seasonal" claim remains in Go.
- [x] AC-2 — Go sends `{current_month, data_days, this_month_count, last_year_same_month_count, topic_candidates}`; the only Go-side numbers are operational ($min_data_days, $topic_min_captures, $topic_candidate_limit, $max_observations).
- [x] AC-3 — `handle_seasonal_analyze` consumes the raw signals and returns `{observations:[...]}`; the request/response contract matches the Go caller on both ends (fixing the prior dead mismatch).
- [x] AC-4 — nil config / nil NATS / no-LLM ⇒ seasonal detection skipped (no hardcoded ratio runs).
- [x] AC-5 — `intelligence.seasonal.*` are fail-loud SST keys (missing/invalid rejected naming the key).

## Notes

Continues the directive from BUG-021-005/006/007/008. This is the fifth
hardcoded-business-threshold conversion in the sweep, and the first delivered
through the existing `seasonal.analyze` ML/NATS path rather than a new
`agent.Bridge` scenario — chosen (Option A) because seasonal already had an ML
sidecar touchpoint, so reusing it keeps the feature coherent and avoids a
redundant second LLM integration for the same concern.

A pre-existing latent defect was also fixed: the `seasonal.analyze` request and
response payloads had mismatched keys on both ends (Go ↔ Python), so the ML
enrichment had been silently inert since BUG-003. The rework makes the contract
coherent.

The "is this seasonally meaningful?" JUDGMENT is now the LLM's. The
operational/business boundary is honored: business reasoning → LLM; operational
limits (data-sufficiency floor, topic-candidate floor/cap, observation cap) →
fail-loud SST config.
