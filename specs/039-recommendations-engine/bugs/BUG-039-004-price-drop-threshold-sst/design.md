# Design: BUG-039-004

## Problem

`gatherPriceDropCandidates` resolved the price-drop threshold with a hardcoded
`0.10` final fallback — a binding NO-DEFAULTS / smackerel-no-defaults violation.
See `bug.md`.

## Architecture

```
EvaluateWatch(price_check) → gatherPriceDropCandidates(watch, trigger)
  → resolvePriceDropThreshold(trigger, watch, e.defaultPriceDropThresholdPct)
      → trigger.Context["threshold_pct"]   (override)
      → watch.Filters["threshold_pct"]      (user preference)
      → defaultPct                          (OPERATIONAL SST default)
  → compare each product's current vs baseline against the resolved threshold
```

The resolution precedence is unchanged; only the source of the final fallback
moves from a Go literal to fail-loud SST.

### Components

1. **`internal/config/recommendations.go`** — `RecommendationWatchesConfig`
   gains `DefaultPriceDropThresholdPct float64`, loaded via the existing
   `parseUnitFloat("RECOMMENDATIONS_WATCHES_DEFAULT_PRICE_DROP_THRESHOLD_PCT")`
   with an added `> 0` guard (the existing helper allows [0,1]; a 0 threshold is
   degenerate for price-drop, so the loader rejects it).

2. **`config/smackerel.yaml`** — `recommendations.watches.default_price_drop_threshold_pct: 0.10`
   (the operator-tunable default; the prior behavioural value preserved).

3. **`scripts/commands/config.sh`** — propagates the key into the generated env
   (assignment via `required_value` + heredoc emit), reaching `dev.env`
   (`config generate`) and `test.env` (`config generate --env test`, run by the
   test runner).

4. **`internal/recommendation/watch/evaluator.go`** —
   `Options.DefaultPriceDropThresholdPct` → `Evaluator.defaultPriceDropThresholdPct`
   (threaded in `NewEvaluator`); the new `resolvePriceDropThreshold` helper
   replaces the inline literal; `gatherPriceDropCandidates` calls it.

5. **`cmd/core/wiring_recommendation_watches.go`** — passes
   `cfg.Recommendations.Watches.DefaultPriceDropThresholdPct` into `watch.Options`.

## Operational vs business boundary

The price-drop threshold is a USER-config value with a trigger override; the
fallback is operational. SST (not LLM) is the correct boundary — consistent with
every operational floor/cap/window in the BUG-021-005..011 sweep. Re-architecting
the scalar-threshold model into an LLM significance judgment would override the
user-config-first design and over-engineer a degenerate fallback path; rejected.

## Test Strategy

- **`internal/recommendation/watch/threshold_test.go`** (white-box, no DB):
  `resolvePriceDropThreshold` precedence (trigger > filter > default; zero and
  absent treated as unset) + a configured-default passthrough proving the
  fallback is the SST value, not a literal.
- **`internal/config/validate_test.go`** — `setRequiredEnv` sets the new key so
  the existing config-load tests stay green; the loader's `> 0` guard + fail-loud
  behaviour is covered by the existing required-env assertions (missing key →
  load error).
- The price-drop DB path is covered by the live-stack integration tier
  (`recommendation_price_watches_test.go`, which always supplies an explicit
  threshold and is unaffected).

## Blast Radius

- Modified: `internal/config/recommendations.go` (field + loader + guard),
  `internal/config/validate_test.go` (env), `internal/recommendation/watch/evaluator.go`
  (Options/struct/NewEvaluator + helper + call site), `cmd/core/wiring_recommendation_watches.go`
  (wiring), `config/smackerel.yaml`, `scripts/commands/config.sh`.
- New: `internal/recommendation/watch/threshold_test.go`.
- No schema migration. `config/generated/` is gitignored (regenerated per run).

## Alternatives Considered

- **LLM-judge price-drop significance.** Rejected: the threshold is user-config;
  the `0.10` is only the missing-input fallback. An LLM call would re-architect a
  certified scalar-threshold feature and override the user's explicit preference
  — over-engineering for a fallback. SST matches the sweep's operational boundary.
- **Keep `0.10`, document it.** Rejected: it is a binding NO-DEFAULTS violation;
  documentation does not remove the hardcoded runtime fallback.
- **Fail loud at evaluation time (no default at all).** Rejected: a user who
  creates a price-drop watch without a threshold should still get a sensible
  operator-tunable default rather than a runtime error; SST default is the
  proportionate, fail-loud-at-config-load fix.
