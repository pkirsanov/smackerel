# Bug Fix Design: [BUG-006] Test Auth Token Provisioning

## Root Cause Analysis

### Investigation Summary
Investigated why `smackerel-core` crashes during live-stack tests with "missing required configuration: SMACKEREL_AUTH_TOKEN". Traced the value flow from `config/smackerel.yaml` through `scripts/commands/config.sh` to `config/generated/test.env`.

### Root Cause
The config generation script (`scripts/commands/config.sh`) treats dev and test environments identically when processing `auth_token`. The SST file (`config/smackerel.yaml`) correctly has `auth_token: ""` as an empty placeholder per SST policy. The generator reads this value and writes it verbatim to both `dev.env` and `test.env`. Since the Go core validates that `SMACKEREL_AUTH_TOKEN` is non-empty and at least 16 characters at startup, the empty value causes a fatal crash in the test environment.

### Impact Analysis
- Affected components: `scripts/commands/config.sh` (config generator), all test harnesses
- Affected data: None (test environment only)
- Affected users: All developers — no live-stack tests can run

## Fix Design

### Solution Approach
In `scripts/commands/config.sh`, add a conditional block for the test environment that detects when `SMACKEREL_AUTH_TOKEN` would be empty and auto-generates a random 48-character hex token using `openssl rand -hex 24`. This approach is SST-compliant because:

1. The SST value in `config/smackerel.yaml` remains the canonical source — it is read first
2. The auto-generation only triggers when `TARGET_ENV=test` AND the SST value is empty
3. The generated token only goes into `config/generated/test.env` (gitignored, never committed)
4. Dev environment continues to propagate the empty value (fail-loud preserved)
5. Each `config generate` run produces a fresh token (no persistence)

Implementation:
- After reading `auth_token` from the SST YAML
- Check if `TARGET_ENV` is `test` and the value is empty
- If so, generate: `SMACKEREL_AUTH_TOKEN=$(openssl rand -hex 24 2>/dev/null || head -c 24 /dev/urandom | xxd -p)`
- Write the generated value to `test.env`
- Log that a test token was auto-generated (for debuggability)

### Alternative Approaches Considered
1. **Set a default test token in smackerel.yaml** — Rejected: violates SST policy (secrets must not be committed), and a static token defeats the disposable test environment principle
2. **Set SMACKEREL_AUTH_TOKEN in test harness scripts** — Rejected: would bypass the SST config pipeline and scatter config across multiple scripts
3. **Lower the Go core's validation requirement for test mode** — Rejected: the core should not have environment-aware validation relaxation; the config layer should provide valid values
4. **Use a .env.test.local override file** — Rejected: adds another config surface outside the SST pipeline

### Affected Files
- `scripts/commands/config.sh` — Add test-env auto-generation logic for auth_token

### Regression Test Design
- Pre-fix: Run `./smackerel.sh config generate` and verify `test.env` has empty `SMACKEREL_AUTH_TOKEN`
- Post-fix: Run `./smackerel.sh config generate` and verify `test.env` has ≥48 hex char `SMACKEREL_AUTH_TOKEN`
- Adversarial: Verify `dev.env` still has empty `SMACKEREL_AUTH_TOKEN` (dev fail-loud preserved)
- Integration: Verify `./smackerel.sh test integration` starts without auth crash
