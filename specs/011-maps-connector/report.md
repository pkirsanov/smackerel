# Report: 011 — Maps Timeline Connector

> **Status:** Validated — unit-complete, integration/e2e gated on live stack
> **Last validated:** 2026-04-09

---

## Execution Evidence

### Test Results

| Category | Count | Result | Command |
|----------|-------|--------|---------|
| Go unit (maps package) | 78 tests (22 connector + 15 maps + 17 normalizer + 24 patterns) | PASS | `./smackerel.sh test unit` |
| Go unit (all 26 packages) | All green | PASS | `./smackerel.sh test unit` |
| Python unit (ML sidecar) | 31 tests | PASS | `./smackerel.sh test unit` |
| Lint | All checks | PASS | `./smackerel.sh lint` |
| Format | All files | PASS | `./smackerel.sh format --check` |
| Build/check | Config in sync | PASS | `./smackerel.sh check` |

### Files Implemented

| File | Lines | Purpose |
|------|-------|---------|
| `internal/connector/maps/connector.go` | ~460 | Connector interface implementation (Connect, Sync, PostSync, Health, Close, SetPool, InsertLocationCluster, archiveFile) |
| `internal/connector/maps/normalizer.go` | ~140 | Activity→RawArtifact normalizer (NormalizeActivity, buildContent, buildMetadata, computeDedupHash, assignTier, roundToGrid) |
| `internal/connector/maps/patterns.go` | ~550 | PatternDetector (DetectCommutes, DetectTrips, LinkTemporalSpatial, InferHome, classifyCommutes, classifyTrips, determineLinkType, normalization) |
| `internal/db/migrations/009_maps.sql` | 24 | location_clusters table + 3 indexes |

### Test Files

| File | Tests | Purpose |
|------|-------|---------|
| `internal/connector/maps/connector_test.go` | 22 | Connector lifecycle, sync, cursor, archiving, PostSync, config |
| `internal/connector/maps/maps_test.go` | 15 | Parsing, classification, trail qualification, GeoJSON, Haversine |
| `internal/connector/maps/normalizer_test.go` | 17 | Normalization, metadata, dedup hash, tier assignment, GeoJSON route storage |
| `internal/connector/maps/patterns_test.go` | 24 | Commute detection, trip detection, link type, tier downgrade/upgrade, commute/trip normalization |

### Scope Completion Summary

**Scope 1 — Connector Implementation & Normalizer:** Done (16/16 DoD items checked)
- Connector interface compiles (`var _ connector.Connector = (*Connector)(nil)`)
- Registered in `cmd/core/main.go`
- Config validated: import_dir, min thresholds, all optional fields with defaults
- All 6 activity types produce correct ContentType
- Title format, metadata (17 fields), tier assignment, cursor management all tested

**Scope 2 — Trail Journal, Dedup & Migration:** Done (10/10 DoD items checked)
- `computeDedupHash()`: SHA-256 of date + ~500m-grid-rounded coords, 16-char hex prefix
- Trail-qualified enrichment: route_geojson LineString, distance_km, duration_min, trail_qualified flag
- `009_maps.sql`: location_clusters table with 12 columns, 3 indexes
- File archiving: move to `{import_dir}/archive/` when enabled
- `ON CONFLICT DO NOTHING` for idempotent location_clusters insertion

**Scope 3 — Commute/Trip Detection & Temporal-Spatial Linking:** Implemented, unit-validated
- `PatternDetector` with pure logic functions (`classifyCommutes`, `classifyTrips`, `determineLinkType`) separated for testability
- 15 unit tests PASS covering commute thresholds, weekday filtering, trip overnight detection, link type determination, tier upgrades/downgrades, PostSync continuation on failure
- DB-dependent items (InferHome query, LinkTemporalSpatial edge insertion, integration/e2e) require live stack — code verified correct via audit

### Quality Chain Evidence

| Phase | Result | Details |
|-------|--------|---------|
| test | PASS | 78 maps tests, 26 Go packages green, 31 Python tests green |
| regression | PASS | edges schema mismatch fixed during hardening |
| simplify+gaps+harden | PASS | 6 findings fixed: PostSync returns, ctx cancellation, file size warning |
| stabilize | PASS | 2 findings fixed: pool exhaustion (rows collected before close), file size limit (200MB hard cap) |
| security | PASS | 2 findings fixed: symlink follow (EvalSymlinks + entry.Type check), path TOCTOU (canonical path resolution) |
| lint | PASS | "All checks passed!" |
| format | PASS | All files clean |
| validate | PASS | All unit-testable DoD items verified, integration items infrastructure-gated |

### Security Audit

| Finding | Mitigation |
|---------|-----------|
| Symlink follow in import directory | `filepath.EvalSymlinks()` resolves canonical path in `Connect()`. `findNewFiles()` skips entries with `os.ModeSymlink` type |
| TOCTOU on import directory path | Symlink resolved once at Connect time, stored as canonical path for all subsequent Sync calls |
| File size DoS | 200MB hard limit enforced before `os.ReadFile()`, 50MB warning threshold logged |
| Pool exhaustion in LinkTemporalSpatial | Rows collected into `[]artifactMatch` slice before `rows.Close()`, then edges inserted outside row iteration |
| SQL injection | All queries use parameterized `$N` placeholders, zero string concatenation |
| Context cancellation | `ctx.Err()` checked between files in Sync and between activities in LinkTemporalSpatial |
| Idempotent operations | `ON CONFLICT DO NOTHING` on location_clusters insert and edges insert |

### Unchecked DoD Items (Infrastructure-Gated)

These Scope 3 items are implemented in code and audited as correct, but cannot be verified with passing tests until the live database stack is available:

1. `InferHome()` integration test — queries location_clusters, code reviewed correct
2. `LinkTemporalSpatial()` integration tests — creates CAPTURED_DURING edges, code reviewed correct with ON CONFLICT DO NOTHING
3. `CAPTURED_DURING` edges ON CONFLICT DO NOTHING verification — SQL audited correct
4. Full integration + e2e test count (6 integration + 2 e2e) — requires PostgreSQL + full stack

### Config Registration

Maps connector config section added to `config/smackerel.yaml` at line 81 with fields: `import_dir`, `watch_interval`, `archive_processed`, `min_distance_m`, `min_duration_min`, `location_radius_m`, `home_detection`, `commute_min_occurrences`, `commute_window_days`, `commute_weekdays_only`, `trip_min_distance_km`, `trip_min_overnight_hours`, `link_time_extend_min`, `link_proximity_radius_m`. Connector registered in `cmd/core/main.go`.
