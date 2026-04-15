# Bug: [BUG-004] main.go god-wirer — 724 LOC with 15 connector imports

## Summary
`cmd/core/main.go` is a god-wirer with 724 LOC that imports all 15 connector packages, has inline config parsing for each connector, and constructs all services. It's the #2 churn hotspot (22 changes in 30 days) co-changing with scheduler.go (15), router.go (13), and config.sh (11).

## Severity
- [ ] Critical
- [ ] High
- [x] Medium - Application works, but every new connector or service requires modifying the 724-LOC main
- [ ] Low

## Status
- [x] Reported
- [x] Confirmed (reproduced via retro hotspot analysis)
- [ ] In Progress
- [ ] Fixed
- [ ] Verified
- [ ] Closed

## Reproduction Steps
1. Run `wc -l cmd/core/main.go` → 724 LOC
2. Count connector import aliases: 15 (`alertsConnector`, `bookmarksConnector`, `browserConnector`, etc.)
3. Observe inline connector config parsing + registration blocks for each connector (each ~20-40 LOC)
4. Observe service construction (DB, NATS, pipeline, scheduler, web handler, etc.) mixed into run()

## Expected Behavior
`main.go` should contain only `run()`, signal handling, and server lifecycle. Connector wiring and service construction should be in separate files within `cmd/core/`.

## Actual Behavior
`main.go` contains everything: config loading, DB/NATS connection, connector instantiation + config parsing + registration for 15 connectors, service construction, route setup, and server lifecycle — all in one function.

## Environment
- Service: smackerel-core
- File: `cmd/core/main.go` (724 LOC, 15 connector imports)
- Evidence: retro 2026-04-15 hotspot analysis

## Root Cause
Organic growth — connectors were added incrementally during specs 007-018, each adding ~20-40 LOC of wiring directly to `main.go`. No extraction was done because each individual addition was small.

## Related
- Feature: `specs/023-engineering-quality/`
- Retro: `.specify/memory/retros/2026-04-15-hotspots.md`
- Co-hotspot: BUG-002 (scheduler.go), BUG-003 (engine.go)
