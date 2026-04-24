# User Validation: 016 — Weather Connector

> **Feature:** [specs/016-weather-connector](.)
> **Status:** In Progress (Scopes 1-3 implemented; Scopes 4-5 deferred)

## Checklist

- [x] Baseline checklist initialized for this feature
- [x] Spec reviewed and approved
- [x] Design reviewed and approved
- [x] Scopes planned (5 scopes)
- [x] Open-Meteo client fetches current, forecast, and historical weather
- [x] Cache prevents excessive API calls with TTL-based expiration
- [x] Coordinates rounded for privacy before API calls
- [x] Connector implements standard Connector interface
- [x] Config schema follows smackerel.yaml conventions
- [ ] NWS alerts fetched and classified by CAP severity
- [ ] High-severity alerts routed to proactive notification
- [ ] Historical weather enrichment serves NATS request/response
- [ ] Daily digest can include weather data
- [ ] Trip dossiers can include destination forecasts
