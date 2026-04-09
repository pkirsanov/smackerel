# User Validation: 016 — Weather Connector

> **Feature:** [specs/016-weather-connector](.)
> **Status:** Not Started

## Checklist

- [ ] Baseline checklist initialized for this feature
- [ ] Spec reviewed and approved
- [ ] Design reviewed and approved
- [ ] Scopes planned (5 scopes)
- [ ] Open-Meteo client fetches current, forecast, and historical weather
- [ ] Cache prevents excessive API calls with TTL-based expiration
- [ ] Coordinates rounded for privacy before API calls
- [ ] Connector implements standard Connector interface
- [ ] Config schema follows smackerel.yaml conventions
- [ ] NWS alerts fetched and classified by CAP severity
- [ ] High-severity alerts routed to proactive notification
- [ ] Historical weather enrichment serves NATS request/response
- [ ] Daily digest can include weather data
- [ ] Trip dossiers can include destination forecasts
