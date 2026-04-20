# Feature: [BUG-006] Test Auth Token Provisioning

## Problem Statement
Live-stack tests cannot run because `smackerel-core` enforces non-empty `SMACKEREL_AUTH_TOKEN` at startup, but the SST-compliant empty placeholder in `config/smackerel.yaml` propagates verbatim to `config/generated/test.env`. This blocks all integration, E2E, and stress testing.

## Outcome Contract
**Intent:** Enable live-stack tests to run without manual auth token configuration while preserving SST fail-loud behavior for dev/prod environments.
**Success Signal:** `./smackerel.sh test integration` starts `smackerel-core` without auth token crash, and `./smackerel.sh test e2e` Compose stack starts successfully.
**Hard Constraints:** Dev environment must still fail-loud when `auth_token` is empty. No hardcoded tokens anywhere. SST remains the single source of truth.
**Failure Condition:** A hardcoded or persistent token leaks into source control, or dev environment silently accepts an empty token.

## Goals
- Config generator auto-provisions a disposable auth token for test environments
- Dev environment preserves fail-loud behavior for empty auth tokens
- Generated test tokens are cryptographically random and sufficiently long (≥48 hex chars)
- No tokens are hardcoded in source-controlled files

## Non-Goals
- Changing the SST policy for dev/prod auth tokens
- Implementing token rotation or expiry
- Modifying the Go core's auth token validation logic

## Requirements
- R1: When `TARGET_ENV=test` and `auth_token` is empty in SST, the config generator MUST auto-generate a random token
- R2: The generated token MUST be at least 48 hexadecimal characters
- R3: When `TARGET_ENV=dev` and `auth_token` is empty in SST, the config generator MUST propagate the empty value (fail-loud preserved)
- R4: The generated token MUST only appear in `config/generated/test.env` (gitignored)
- R5: Each `config generate` invocation MUST produce a fresh random token (no persistence)
- R6: The token generation MUST use a cryptographically secure source (`openssl rand` or `/dev/urandom` fallback)

## User Scenarios (Gherkin)

```gherkin
Scenario: Test environment auto-generates auth token when SST is empty
  Given config/smackerel.yaml has auth_token: ""
  And TARGET_ENV is "test"
  When the config generator runs
  Then config/generated/test.env contains SMACKEREL_AUTH_TOKEN with at least 48 hex characters

Scenario: Dev environment fails loud when auth token is empty
  Given config/smackerel.yaml has auth_token: ""
  And TARGET_ENV is "dev"
  When the config generator runs
  Then config/generated/dev.env contains SMACKEREL_AUTH_TOKEN= (empty value preserved)

Scenario: Integration tests succeed with auto-generated test token
  Given the config generator has produced test.env with an auto-generated token
  When ./smackerel.sh test integration runs
  Then smackerel-core starts without auth token crash

Scenario: No hardcoded tokens exist in source-controlled files
  Given the fix is applied
  When scanning all tracked files for token patterns
  Then no hardcoded auth tokens are found
```

## Acceptance Criteria
- AC1: `./smackerel.sh config generate` produces a non-empty `SMACKEREL_AUTH_TOKEN` in `test.env` when SST value is empty
- AC2: The generated token is at least 48 hex characters
- AC3: `config/generated/dev.env` still has an empty `SMACKEREL_AUTH_TOKEN` when SST value is empty
- AC4: `./smackerel.sh test integration` starts `smackerel-core` without crash
- AC5: `./smackerel.sh test e2e` Compose stack starts successfully
- AC6: No hardcoded tokens in any source-controlled file
- AC7: Each `config generate` produces a different token (not cached/persistent)
