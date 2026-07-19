# Specification: BUG-075-002 Assistant renderer Node toolchain

## Expected Behavior

Node-dependent assistant E2E tests MUST run through `./smackerel.sh` inside a repository-sanctioned Docker environment that provides Node. Host Node installation and direct host `node` or `npm` commands are forbidden. Missing Node MUST fail the harness before tests can be reported green; broken renderer execution MUST remain a test failure.

## Acceptance Criteria

1. The assistant-package E2E bootstrap ensures `node` exists inside the Go tooling container.
2. Tool installation is idempotent, bounded to the E2E wrapper, and fail-loud.
3. Both notice renderer tests execute the real JS CLI and pass their body/addendum assertions.
4. Removing the bootstrap call or Node package makes a contract test or live renderer test fail.
5. No `t.Skip`, bailout return, host Node install, `npm` invocation, request interception, or sleep is added.
6. The full assistant package runs without invoking every E2E package.

## Release Train

This bug targets the `mvp` train and introduces no feature flag.

## Test Isolation

The live assistant requests use the disposable stack. Node executes only the checked-in pure renderer CLI against the live response body.

## Deployment Boundary

No production image, deployment adapter, host, manifest, release-train, or secret surface is changed.
