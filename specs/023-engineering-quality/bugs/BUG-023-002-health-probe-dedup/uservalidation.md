# User Validation: BUG-023-002

Links: [spec.md](spec.md) | [report.md](report.md)

---

## Checklist

- [x] `internal/api/health.go` now has a single `probeHTTPGet` helper that owns the bounded HTTP-GET + body-drain pattern shared by the two probe wrappers
- [x] `checkMLSidecar` and `checkOllama` are reduced to short wrappers (empty-URL guard + one helper call + ServiceStatus translation)
- [x] All 8 pre-existing probe-wrapper unit tests (`TestCheckMLSidecar_EmptyURL/_HealthyResponse/_UnhealthyResponse/_ConnectionRefused`, `TestCheckOllama_Healthy/_Down/_NotConfigured/_Unreachable`) continue to PASS without modification
- [x] `go test -count=1 -race ./internal/api/` exit 0 — no race introduced; SCN-023-01 race-detector regression remains green
- [x] `go build ./...` exit 0
- [x] No changes to parent `specs/023-engineering-quality/spec.md`, `scopes.md`, or `state.json` — the parent spec stays certified `done`
- [x] Boundary: only `internal/api/health.go` was modified in production code; all other edits are confined to `specs/023-engineering-quality/bugs/BUG-023-002-health-probe-dedup/`

## Notes

Simplify refactor spawned by stochastic-quality-sweep round 18 (`sweep-2026-05-23-r30`, trigger: simplify, mapped mode: simplify-to-doc). The refactor preserves all observable behaviour and the four-branch contract per wrapper; the existing tests are the regression suite.
