# Bug: BUG-075-002 Assistant renderer E2E lacks Node toolchain

## Summary

Two live legacy-retirement renderer tests invoke the real PWA renderer with `node`, but the canonical Go E2E lane runs inside `golang:1.25.10-bookworm` without Node.

## Severity

- [ ] Critical - System unusable, data loss
- [x] High - Two required cross-language E2E scenarios cannot execute
- [ ] Medium - Feature broken, workaround exists
- [ ] Low - Minor issue, cosmetic

## Status

- [ ] Reported
- [x] Confirmed (reproduced)
- [x] In Progress
- [ ] Fixed
- [ ] Verified
- [ ] Closed

## Reproduction Steps

1. Confirm the host has no role in Node execution.
2. Run the two notice-renderer tests through `./smackerel.sh test e2e`.
3. Observe both live HTTP turns succeed and both tests fail at `exec.LookPath("node")` inside the Go tooling container.

## Expected Behavior

The repository CLI provides Node inside the sanctioned Docker E2E environment before the assistant package starts. The tests continue to fail loudly if Node or the renderer CLI disappears, and they execute the real renderer without host tooling.

## Actual Behavior

The Go E2E container has no Node, so both tests stop after the live response and before renderer assertions.

## Environment

- Service: assistant PWA renderer E2E
- Version: `7ca186217c007a24075b2273275a22434d89fc44`
- Platform: Linux, `golang:1.25.10-bookworm` repository tooling container

## Error Output

```text
node not on PATH; spec 075 SCOPE-075-06.3 e2e requires node to run the PWA renderer: exec: "node": executable file not found in $PATH
```

## Root Cause

The tests were added to a Go-only E2E lane without extending that lane's container bootstrap. The earlier BUG-073-003 repair covers a separate unit canary in dedicated CI and does not supply Node to live assistant E2E.

## Related

- Feature: `specs/075-legacy-retirement-telemetry/`
- Prior toolchain packet: `specs/073-web-mobile-assistant-frontend/bugs/BUG-073-003-canary-ci-toolchain-gating/`
- Companion packet: `BUG-075-001-residual-metric-order-independence`
