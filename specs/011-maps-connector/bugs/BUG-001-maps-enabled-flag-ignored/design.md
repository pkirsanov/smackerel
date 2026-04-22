# Design: BUG-001 — MAPS_ENABLED Flag Ignored

**Parent:** [011-maps-connector](../../design.md)

## Current Truth

- `internal/config/config.go` line ~76: no `MapsEnabled` field exists
- `internal/config/config.go` line ~268: `MapsImportDir` loaded from env, no `MapsEnabled` loaded
- `cmd/core/connectors.go` line ~122: guard is `if cfg.MapsImportDir != ""` (no Enabled check)
- `config/generated/dev.env` line 61: `MAPS_ENABLED=false` is generated but never consumed

## Fix Design

### Change 1: `internal/config/config.go`

Add `MapsEnabled bool` field after the existing `MapsImportDir` field (line ~76).
Load it from `MAPS_ENABLED` env var using the same `== "true"` pattern as bookmarks/browser-history.

### Change 2: `cmd/core/connectors.go`

Change the maps auto-start guard from:
```go
if cfg.MapsImportDir != "" {
```
to:
```go
if cfg.MapsEnabled && cfg.MapsImportDir != "" {
```

### Change 3: `internal/config/validate_test.go`

Add test coverage verifying `MapsEnabled` is parsed correctly from the env var.

## Patterns Followed

- Bookmarks: `BookmarksEnabled bool` + `os.Getenv("BOOKMARKS_ENABLED") == "true"` + `cfg.BookmarksEnabled && cfg.BookmarksImportDir != ""`
- Browser history: `BrowserHistoryEnabled bool` + `os.Getenv("BROWSER_HISTORY_ENABLED") == "true"` + `cfg.BrowserHistoryEnabled && cfg.BrowserHistoryPath != ""`
