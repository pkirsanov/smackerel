# Report: 023 Engineering Quality

## Summary

**Feature:** 023-engineering-quality
**Scopes:** 3
**Status:** Done

| Scope | Name | Status |
|-------|------|--------|
| 1 | mlClient Race + Typed Deps + Dead Code | Done |
| 2 | SST Connectors + writeJSON + Health Probes | Done |
| 3 | Health Logging + Sync Schedule | Done |

## Gaps Sweep (2026-04-11)

A stochastic quality sweep (trigger: gaps) identified 3 remaining runtime type assertions in `internal/api/capture.go` that violated R-ENG-009 and Scope 1 DoD ("All runtime type assertions in `router.go`, `capture.go`, `digest.go` replaced with direct interface calls"):

| Gap | Location | Assertion | Root Cause |
|-----|----------|-----------|------------|
| GAP-1 | `capture.go` RecentHandler | `d.SearchEngine.(*SearchEngine)` | Handler directly accessed `Pool` for DB queries via SearchEngine cast |
| GAP-2 | `capture.go` ArtifactDetailHandler | `d.SearchEngine.(*SearchEngine)` | Same pattern — direct pool access for artifact detail query |
| GAP-3 | `capture.go` ExportHandler | `d.DB.(querier)` local anonymous interface | Bypassed typed interface to reach `ExportArtifacts` method |

### Remediation

1. Added `RecentArtifacts()` and `GetArtifact()` methods to `db.Postgres` — proper data-access layer encapsulation
2. Added `ArtifactQuerier` typed interface in `internal/api/health.go` with `RecentArtifacts`, `GetArtifact`, `ExportArtifacts`
3. Added `ArtifactStore ArtifactQuerier` field to `Dependencies` struct
4. Rewrote `RecentHandler`, `ArtifactDetailHandler`, and `ExportHandler` to use `d.ArtifactStore` — zero type assertions
5. Wired `pg` (satisfies `ArtifactQuerier`) to `ArtifactStore` in `cmd/core/main.go`

### Verification

- `./smackerel.sh check` — passes (config SST + go vet/build)
- `./smackerel.sh test unit` — all packages pass, `internal/api` re-ran (not cached) at 2.446s
- `grep '\.(' internal/api/*.go` — zero runtime type assertions in api package
- All DoD items from scopes.md remain satisfied

## Test Evidence

### Scope 1: mlClient Race + Typed Deps + Dead Code

- `sync.Once` guards `mlClient()` — race detector clean
- 5 typed interfaces (`Pipeliner`, `Searcher`, `DigestGenerator`, `WebUI`, `OAuthFlow`) replace `interface{}`
- `ArtifactQuerier` interface eliminates remaining 3 type assertions (gaps sweep)
- Dead `checkAuth` removed — zero grep hits
- `go build ./...` succeeds; `go test -race ./internal/api/...` passes

### Scope 2: SST Connectors + writeJSON + Health Probes

- Connector paths (`BookmarksImportDir`, `BrowserHistoryPath`, `MapsImportDir`) flow through `config.Config`
- All 4 intelligence handlers use `writeJSON`/`writeError` — zero manual `json.NewEncoder` in intelligence.go
- Ollama probed live via `checkOllama()`; Telegram via `TelegramHealthChecker.Healthy()`

### Scope 3: Health Logging + Sync Schedule

- `/api/health` and `/ping` excluded from `structuredLogger`
- Connector supervisor uses `getSyncInterval()` with `parseSyncInterval()` — handles cron and duration formats
- Default fallback to 5 minutes when no schedule configured

## Reconciliation Pass (2026-04-11, validate trigger)

A stochastic quality sweep (trigger: validate, mode: reconcile-to-doc) verified every DoD claim against the live codebase. Method: code inspection of all touched files + `./smackerel.sh check` + `./smackerel.sh test unit`.

### Scope 1 Reconciliation

| DoD Claim | Verified | Evidence |
|-----------|----------|----------|
| `mlClient()` guarded by `sync.Once` | Yes | `health.go` — `d.mlClientOnce.Do(func(){...})` |
| 5 `interface{}` fields → typed interfaces | Yes | `Pipeliner`, `Searcher`, `DigestGenerator`, `WebUI`, `OAuthFlow` on `Dependencies` |
| All type assertions in router/capture/digest replaced | Yes | `grep` for type assertions in `internal/api/*.go` — zero hits |
| Dead `checkAuth` removed | Yes | `grep -rn "checkAuth" internal/api/` — zero hits |
| `go build ./...` succeeds | Yes | `./smackerel.sh check` — pass |
| No new `interface{}` fields | Yes | `grep "interface{}" internal/api/health.go` — zero hits |

### Scope 2 Reconciliation

| DoD Claim | Verified | Evidence |
|-----------|----------|----------|
| Zero `os.Getenv()` for connector paths in `cmd/` | Yes | `grep` for BOOKMARKS/BROWSER_HISTORY/MAPS_IMPORT os.Getenv in `cmd/` — zero hits |
| Paths read from `cfg.BookmarksImportDir` etc. | Yes | `config.go` fields + `main.go` reads `cfg.` fields |
| All 4 intelligence handlers use `writeJSON` | Yes | `grep "json.NewEncoder" internal/api/intelligence.go` — zero hits |
| Ollama probed live via `GET /api/tags` | Yes | `checkOllama()` in `health.go` — HTTP probe with 2s timeout |
| Telegram health from `Healthy()` method | Yes | `d.TelegramBot.Healthy()` check in HealthHandler |
| Health JSON shape backward-compatible | Yes | Same `HealthResponse`/`ServiceStatus` structs; only status values change |

### Scope 3 Reconciliation

| DoD Claim | Verified | Evidence |
|-----------|----------|----------|
| `/api/health` and `/ping` excluded from logging | Yes | `structuredLogger` early return for both paths in `router.go` |
| Supervisor uses `getSyncInterval()` | Yes | `supervisor.go` — `interval := s.getSyncInterval(id)` replaces hardcoded wait |
| No hardcoded `time.After(5 * time.Minute)` in sync loop | Yes | `grep` — zero hits |
| `parseSyncInterval()` handles cron + duration | Yes | Function in `supervisor.go` + 7 unit tests in `sync_interval_test.go` |
| Default 5m fallback | Yes | `defaultSyncInterval = 5 * time.Minute` |

### Cross-Cutting

| Check | Result |
|-------|--------|
| `./smackerel.sh check` | Pass |
| `./smackerel.sh test unit` (all packages) | Pass |
| `ArtifactQuerier` interface (gaps sweep fix) | Wired in `main.go`, used by `RecentHandler`, `ArtifactDetailHandler`, `ExportHandler` — zero type assertions |

### Drift Found

**None.** All 27 DoD items across 3 scopes match the implemented code. No claim-vs-reality drift detected.

## Completion Statement

Feature 023 is complete. All 3 scopes are done. A gaps sweep on 2026-04-11 found and fixed 3 remaining runtime type assertions in capture.go that had been missed by the original implementation. A reconciliation pass on 2026-04-11 verified all DoD claims against live code with zero drift. All unit tests pass.
