# User Validation: 010 — Browser History Connector

> **Status:** In Progress

---

## Checklist

- [x] Browser history connector implements Connector interface (ID, Connect, Sync, Health, Close)
- [x] ParseChromeHistorySince added for cursor-based incremental sync
- [x] GoTimeToChrome and ChromeTimeToGo time conversion functions exported
- [x] Copy-then-read strategy with temp file cleanup and retry-once on copy failure
- [x] Dwell-time tiering assigns full/standard/light/metadata tiers based on visit duration
- [x] Configurable dwell thresholds via config (DwellFullMin, DwellStandardMin, DwellLightMin)
- [x] Skip rules filter non-content URLs (chrome://, localhost, about:blank, file://)
- [x] Social media visits aggregated at domain level per day (browsing/social-aggregate)
- [x] High-dwell social media reads get individual processing (≥ SocialMediaIndividualThreshold)
- [x] Repeat visit detection with tier escalation for frequently visited URLs
- [x] Privacy gate: metadata-tier entries produce no individual artifacts
- [x] Content fetch failures produce metadata-only artifacts with content_fetch_failed flag
- [x] Config section in smackerel.yaml with SST pipeline integration (disabled by default)
- [x] Connector registered and auto-started in main.go
- [x] All 67 unit tests pass across connector_test.go (49) and browser_test.go (18)
- [x] ./smackerel.sh lint passes
- [x] ./smackerel.sh check passes
- [ ] Integration tests pass against real SQLite fixture (blocked: F002/R003 — SQLite driver not in go.mod)
- [ ] E2E tests pass against live stack (blocked: no live stack available)
