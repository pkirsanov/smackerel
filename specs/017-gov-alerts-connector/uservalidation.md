# User Validation: 017 — Government Alerts Connector

> **Feature:** [specs/017-gov-alerts-connector](.)
> **Status:** Done

## Checklist

- [x] Baseline checklist initialized for this feature
- [x] Spec reviewed and approved
- [x] Design reviewed and approved
- [x] Scopes planned (6 scopes)
- [x] Proximity filter accurately calculates distances (Haversine)
  > Evidence: `haversineKm()` verified by TestHaversineKm, TestHaversineKm_ExtremeDistances (poles, date line, antipodal)
- [x] USGS earthquake source parses GeoJSON correctly
  > Evidence: `fetchUSGSEarthquakes()` decodes FeatureCollection; 20+ earthquake tests pass
- [x] NWS weather alerts source parses CAP/JSON-LD
  > Evidence: `fetchNWSAlerts()` parses JSON-LD with CAP fields; 17+ NWS tests pass
- [x] Connector implements standard Connector interface
  > Evidence: ID(), Connect(), Sync(), Health(), Close() implemented and tested
- [x] Config schema follows smackerel.yaml conventions
  > Evidence: gov-alerts section in `config/smackerel.yaml`, SST pipeline wired
- [x] Multi-source aggregation polls all enabled feeds
  > Evidence: Sync() iterates all 7 sources with per-source config toggles
- [x] Alert lifecycle tracks active/updated/expired states
  > Evidence: known map dedup with 7-day eviction; TestKnownMapEviction verifies
- [x] Additional sources (tsunami, volcano, wildfire, air quality, GDACS) work
  > Evidence: 5 additional sources with fetch+normalize pattern; 30+ tests for additional sources
- [x] High-severity alerts route to proactive notification
  > Evidence: maybeNotify() routes extreme/severe to AlertNotifier; TestSync_ExtremeEarthquake_NotifiesAlert verifies
- [x] Travel destination alerts match upcoming calendar events
  > Evidence: TravelLocationProvider interface; mergedLocations() doubles radius; TestSync_TravelLocation_ExpandedRadius verifies
- [x] Full certification pass completed — all 7 phases certified
  > Evidence: 166 test functions, 10 quality probes (chaos×2, simplify, test, reconcile, regression, security, harden, improve×2), all unit tests pass
