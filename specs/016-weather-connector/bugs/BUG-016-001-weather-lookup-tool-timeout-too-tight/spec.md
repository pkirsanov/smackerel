# BUG-016-001 — Spec

## Expected behavior

The `weather_lookup` agent tool MUST complete successfully on cold-cache
invocations against open-meteo from a healthy home-lab network. The
per-call timeout MUST be sized to fit the **end-to-end** time of a
single lookup (geocoding + forecast = two sequential HTTPS round
trips, ~2-4s worst case), with reasonable headroom.

Specifically:

1. A cold-cache lookup for any reasonable location string (city name,
   ZIP code, "City, ST" form, etc.) MUST return a forecast within
   the tool budget when both open-meteo endpoints are responsive
   (HTTP 200 within ~2s each).
2. A warm-cache lookup (same location within `cache_ttl`) MUST be
   sub-100ms.
3. A genuine open-meteo outage (5xx for ≥`tool_budget` worth of
   wall-clock time) MUST surface as `OutcomeProviderError` with
   `OutcomeDetail.detail` carrying the upstream error text (see
   BUG-061-004 for the parallel observability fix).

## Out of scope

- Pre-warming a local geocoding cache for popular locations.
- Switching to a different weather provider (open-meteo is the
  project default per spec 016).
- Parallelizing geocode + forecast (forecast needs lat/lon from
  geocode, inherently sequential).
- Restructuring the scenario YAML / executor precedence so
  `prompt_contracts/weather-query-v1.yaml::per_tool_timeout_ms`
  wins over the tool registration default (separate scope).
