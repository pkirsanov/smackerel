# Scopes: BUG-001 — MAPS_ENABLED Flag Ignored

## Scope 01: Add MapsEnabled Config Field and Auto-Start Guard

**Status:** Done
**Priority:** P1

### Description

Add `MapsEnabled bool` to `config.Config`, load from `MAPS_ENABLED` env var, and add the guard check in the maps connector auto-start block.

### Scenarios

```gherkin
Scenario: Maps connector respects enabled flag
  Given config/smackerel.yaml has google-maps-timeline.enabled = false
  And google-maps-timeline.import_dir = "/data/maps-import"
  When config generate runs
  Then MAPS_ENABLED=false and MAPS_IMPORT_DIR=/data/maps-import in dev.env
  And the maps connector does NOT auto-start

Scenario: Maps connector starts when enabled with valid dir
  Given config/smackerel.yaml has google-maps-timeline.enabled = true
  And google-maps-timeline.import_dir = "/data/maps-import"
  When the core service starts
  Then the maps connector auto-starts

Scenario: Maps connector does not start when enabled but no dir
  Given config/smackerel.yaml has google-maps-timeline.enabled = true
  And google-maps-timeline.import_dir = ""
  When the core service starts
  Then the maps connector does NOT auto-start
```

### Test Plan

- Unit test: `MapsEnabled` parsed as `true` when env var is `"true"`
- Unit test: `MapsEnabled` parsed as `false` when env var is `"false"` or empty
- Existing maps connector tests must pass (no behavioral change to connector itself)

### Definition of Done

- [x] `config.Config` has `MapsEnabled bool` field
  Added at line ~77 in `internal/config/config.go`, between `BrowserHistorySocialMediaIndividualThreshold` and `MapsImportDir`.
- [x] `MapsEnabled` loaded from `MAPS_ENABLED` env var with `== "true"` pattern
  Added `MapsEnabled: os.Getenv("MAPS_ENABLED") == "true"` at line ~269 in `internal/config/config.go`.
- [x] `connectors.go` maps auto-start guard checks `cfg.MapsEnabled && cfg.MapsImportDir != ""`
  Changed guard at line ~120 in `cmd/core/connectors.go` from `if cfg.MapsImportDir != ""` to `if cfg.MapsEnabled && cfg.MapsImportDir != ""`.
- [x] Unit tests verify `MapsEnabled` parsing
  Extended `TestLoad_ConnectorPathFields` to set `MAPS_ENABLED=true` and assert `cfg.MapsEnabled == true`.
  Extended `TestLoad_ConnectorPathFieldsOptional` to assert `cfg.MapsEnabled == false` when env var is not set.
- [x] All existing unit tests pass
  ```
  ok  github.com/smackerel/smackerel/cmd/core 0.564s
  ok  github.com/smackerel/smackerel/internal/config  0.227s
  ok  github.com/smackerel/smackerel/internal/connector/maps  (cached)
  (42 packages total, all pass)
  ```
- [x] `./smackerel.sh check` passes
  ```
  Config is in sync with SST
  env_file drift guard: OK
  ```
