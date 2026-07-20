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

### Single-Capability Justification

- **Classification:** This extends the existing repository-sanctioned tooling-prerequisite convention with one Node-specific helper. Node is a second tool prerequisite alongside envsubst, but it is not a second provider of one business capability and does not introduce an open toolchain plugin surface.
- **Existing foundation and reuse path:** `scripts/runtime/go-e2e.sh` already performs fail-loud tooling bootstrap before running Go E2E packages. It reuses that ordering by sourcing `_ensure_node.sh` and calling `ensure_node "go-e2e"` before the closed assistant package selector, matching the established `_ensure_envsubst.sh` convention.
- **Consumer set:** The Go E2E wrapper, the assistant package selector, `legacy_retirement_notice_test.go`, and the checked-in `web/pwa/lib/render_descriptor_v1_cli.js` renderer consume the sanctioned Node executable. Other Go wrappers do not require Node.
- **Why no new abstraction or provider registry is needed:** Envsubst is shared across multiple Go wrappers, while Node is required only by the assistant renderer lane. A generic package/tool registry would erase those different requirement boundaries and add dynamic behavior where two explicit, idempotent helpers are sufficient.

## Release Train

This bug targets the `mvp` train and introduces no feature flag.

## Test Isolation

The live assistant requests use the disposable stack. Node executes only the checked-in pure renderer CLI against the live response body.

## Deployment Boundary

No production image, deployment adapter, host, manifest, release-train, or secret surface is changed.
