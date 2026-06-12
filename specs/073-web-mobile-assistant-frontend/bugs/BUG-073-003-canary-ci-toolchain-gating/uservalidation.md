# User Validation: BUG-073-003 — Cross-language canary CI toolchain gating

These items are CHECKED `[x]` by default (validated via the bug's own evidence). Uncheck an
item `[ ]` to report that the behavior is broken.

## Checklist

- [x] The `CI` `lint-and-test` job is green (the canary skips in the go-only Go unit container).
- [x] The cross-language renderer canary still RUNS in CI (dedicated `cross-language-canary` job with node + Flutter/dart).
- [x] Cross-language drift is still caught: with toolchains present, a corrupted golden makes the canary fail loud.
- [x] A contract test fails the build if the dedicated canary job is removed or unwired (canary cannot silently stop running in CI).
- [x] Local developer runs without node/dart degrade gracefully (canary skips, suite stays green).
