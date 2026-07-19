# Bug Fix Design: BUG-075-002

## Root Cause Analysis

### Investigation Summary

`smackerel.sh test e2e` launches `scripts/runtime/go-e2e.sh` in `golang:1.25.10-bookworm`. That wrapper ensures `envsubst` but not Node. Both failing tests call `node web/pwa/lib/render_descriptor_v1_cli.js` after successful live turns. BUG-073-003 intentionally keeps the shared Go unit lane Go-only and runs a separate CI canary, so it does not cover this live package.

### Root Cause

The E2E wrapper's declared tool prerequisites are incomplete for the assistant package it executes.

### Impact Analysis

- Affected components: Go E2E wrapper and two assistant renderer tests
- Affected behavior: cross-language PWA projection of legacy notices
- Production renderer defect: none observed because renderer execution never starts

## Fix Design

### Solution Approach

Add a shared, idempotent Node prerequisite helper for the Debian Go tooling container and invoke it from `go-e2e.sh`. The helper must require its log tag, use the container's trusted package source, verify `node` after installation, and return nonzero on failure. Add a static contract with adversarial mutations proving the bootstrap call cannot silently disappear. Keep `exec.LookPath("node")` fatal in both live tests.

### Alternative Approaches Considered

1. Install Node on the host - rejected by repository Docker-only policy.
2. Skip renderer assertions when Node is absent - rejected because it erases required E2E coverage.
3. Add Node to the production core image - rejected because renderer tooling is a test-only prerequisite.
4. Reclassify the tests as unit canaries - rejected because they also verify the live HTTP response and retirement policy.

## Complexity Tracking

| Decision | Simpler fix considered | Why rejected |
|---|---|---|
| Shared prerequisite helper | Inline package installation in `go-e2e.sh` | The existing envsubst precedent centralizes idempotent tooling setup and makes contract testing practical. |
