# BUG-073-001: Cross-language renderer canary flakes under parallel unit lane

**Status:** Fixed
**Severity:** Medium (intermittent CI noise; no production impact)
**Reported:** 2026-06-02
**Fixed:** 2026-06-02
**Reporter:** Owner via bubbles.bug
**Affected feature:** `specs/073-web-mobile-assistant-frontend/` — TP-073-03 cross-language renderer canary
**Affected file:** `tests/unit/clients/render_descriptor_canary_test.go`

## Summary

`TestRenderDescriptorV1_CrossLanguageCanary` (and its 7 fixture subtests) intermittently
fails when run as part of the full `./smackerel.sh test unit --go` lane, while passing
deterministically when run in isolation.

## Reproduction Steps

1. Standalone (PASSES, ~5s):
   ```
   go test -count=1 -run TestRenderDescriptorV1_CrossLanguageCanary -v ./tests/unit/clients/
   ```
   All 7 subtests (`text_only`, `with_sources`, `disambiguation`, `confirm_accept_decline`,
   `capture_acknowledgement`, `error_retry`, `unknown_shape`) PASS.

2. Under parallel load (FLAKES):
   ```
   ./smackerel.sh test unit --go
   ```
   `go test ./...` schedules many test binaries concurrently; the canary intermittently
   fails on the `dart run tool/render_descriptor_v1_cli.dart` subprocess invocation.

## Expected vs Actual

- **Expected:** Test is deterministic across both invocations.
- **Actual:** Test flakes (intermittent failure) only when CPU/IO is contested by the rest
  of the unit lane.

## Suspected Root Cause

The test invokes `dart run tool/render_descriptor_v1_cli.dart` for each of 7 fixtures.
Each `dart run` performs JIT compile + pub-cache lookups + kernel snapshot resolution
against the shared `clients/mobile/assistant/.dart_tool/` directory. Under heavy parallel
`go test ./...` load, that startup sequence is sensitive to CPU contention and shared
filesystem state on `.dart_tool/`.

Detailed analysis: see `design.md`.

## Impact

- False-positive CI failures on the unit lane block pushes and erode trust in the canary.
- The canary itself is correct (TP-073-03 contract is sound); the flake is purely in test
  infrastructure (subprocess invocation pattern).

## Severity Justification

Medium, not High: no production code is affected, no spec 073 acceptance behavior is at
risk. But every unit-lane run is a potential false-positive, and pre-push validation
depends on this lane.
