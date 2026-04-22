# Bug: MAPS_ENABLED Flag Ignored by Auto-Start Guard

**Severity:** Medium
**Found by:** bubbles.devops (devops-to-doc trigger, stochastic sweep)
**Date:** April 22, 2026
**Spec:** 011-maps-connector

## Description

The `config/smackerel.yaml` SST pipeline generates `MAPS_ENABLED=false` in the env file, but `internal/config/config.go` has no `MapsEnabled` field to consume it. The auto-start guard in `cmd/core/connectors.go` checks only `cfg.MapsImportDir != ""` instead of `cfg.MapsEnabled && cfg.MapsImportDir != ""`.

This is inconsistent with bookmarks (`cfg.BookmarksEnabled && cfg.BookmarksImportDir != ""`) and browser-history (`cfg.BrowserHistoryEnabled && cfg.BrowserHistoryPath != ""`), both of which respect their `Enabled` flags.

## Impact

If a user sets `enabled: false` in `smackerel.yaml` but provides a non-empty `import_dir`, the maps connector auto-starts anyway, silently ignoring the explicit disable. Maps processes location data requiring R-401 opt-in consent, so ignoring the `enabled` flag compounds privacy risk.

## Reproduction

1. Set `google-maps-timeline.enabled: false` in `config/smackerel.yaml`
2. Set `google-maps-timeline.import_dir: /data/maps-import`
3. Run `./smackerel.sh config generate` → `MAPS_ENABLED=false` + `MAPS_IMPORT_DIR=/data/maps-import`
4. Start the stack → maps connector starts despite `enabled: false`

## Fix

1. Add `MapsEnabled bool` field to `config.Config`
2. Load from `MAPS_ENABLED` env var
3. Add `cfg.MapsEnabled &&` guard in `connectors.go` auto-start block
