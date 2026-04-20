# Bug: [BUG-006] Test Auth Token Provisioning

## Summary
All live-stack tests (integration, E2E, stress) fail because the Go core requires `SMACKEREL_AUTH_TOKEN` (min 16 chars, non-placeholder) but `config/smackerel.yaml` has `auth_token: ""` by SST policy, and the config generator propagates this empty value to `test.env`.

## Severity
- [ ] Critical - System unusable, data loss
- [x] High - Major feature broken, no workaround
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
1. Ensure `config/smackerel.yaml` has `auth_token: ""` (SST default)
2. Run `./smackerel.sh config generate`
3. Observe `config/generated/test.env` contains `SMACKEREL_AUTH_TOKEN=`
4. Run `./smackerel.sh test integration` (or e2e, or stress)
5. Observe `smackerel-core` container crashes on startup

## Expected Behavior
The test environment should auto-generate a disposable test-only auth token when the SST value is empty, allowing live-stack tests to run without manual token configuration.

## Actual Behavior
The config generator treats dev and test environments identically for `auth_token`. When the SST value is empty, the generated `test.env` contains an empty `SMACKEREL_AUTH_TOKEN`, causing the Go core to crash with a fatal startup error.

## Environment
- Service: smackerel-core (Go)
- Version: current HEAD
- Platform: Linux / Docker Compose

## Error Output
```
ERROR fatal startup error error="configuration error: missing required configuration: SMACKEREL_AUTH_TOKEN"
```

## Root Cause (filled after analysis)
The config generation script (`scripts/commands/config.sh`) does not differentiate between dev and test environments when handling `auth_token`. For test environments, when the SST value is empty, the generator should auto-produce a disposable random token. This is SST-compliant because:
- The SST value remains the canonical source for dev/prod
- The test environment is documented as disposable
- The generated token only goes into `config/generated/test.env` (gitignored)
- Dev environment still requires manual token configuration (fail-loud)

## Related
- Feature: `specs/023-engineering-quality/`
- Category: test-infrastructure
- Blocks: ALL live-stack testing (integration, E2E, stress)
