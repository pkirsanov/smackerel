# User Validation: [BUG-020-010] QF-decisions callback keystore direct env-read

## Checklist

### [Bug Fix] [BUG-020-010] QF-decisions callback signing keystore ingested via Config SST
- [x] **What:** `internal/connector/qfdecisions/callback_keystore.go` no longer reads the `QF_DECISIONS_CALLBACK_SIGNING_KEYS_JSON` env var directly. The JSON string is now carried on a new `QFDecisionsCallbackSigningKeysJSON` field on `internal/config.Config`, populated by `Config.Load()` from the same env var, and validated at boot by `validateQFDecisionsConfig()` (malformed non-empty JSON aborts at `Validate()` time, naming the field). The single non-test call site in `Connect()` consumes the resolved Config field; the 3 existing integration-test call sites and the in-tree unit test are migrated to the new API.
  - **Steps:**
    1. Run `grep -nE 'os\.Getenv' internal/connector/qfdecisions/callback_keystore.go` and confirm zero matches (exit 1).
    2. Run `grep -nE 'QFDecisionsCallbackSigningKeysJSON' internal/config/config.go` and confirm at least 1 field declaration + 1 `Load()` populate + 1 `validateQFDecisionsConfig` guard.
    3. Inspect the 3 new BUG020010 unit tests: `TestBUG020010_KeystoreReadsFromConfigNotEnv`, `TestBUG020010_ValidateFailsLoudOnMalformedSigningKeysJSON`, `TestBUG020010_KeystoreEnvVarLiteralRemoved` (and the PERMISSIVE/STRICT policy test per the decision recorded in `report.md` → "Naming Decision").
    4. Run `./smackerel.sh test unit` and confirm exit 0 with the BUG020010 test functions in the output.
    5. Run `./smackerel.sh build` and confirm exit 0.
    6. Run `go test -tags=integration ./tests/integration/ -run TestQFCallback -count=1` and confirm exit 0 — proves the migrated integration-test call sites still cover the QF callback signing flow end-to-end against the live test stack.
    7. As an adversarial check, set `QF_DECISIONS_ENABLED=true`, set the other required QF-decisions env vars to valid values, set `QF_DECISIONS_CALLBACK_SIGNING_KEYS_JSON="not-valid-json"`, and boot the core service. Confirm it ABORTS at `Validate()` time (NOT at `Connect()` time) with a clear error naming the field — proves the fail-loud move is real.
    8. As a second adversarial check, deploy with `QF_DECISIONS_ENABLED=true` and `QF_DECISIONS_CALLBACK_SIGNING_KEYS_JSON` UNSET. If PERMISSIVE policy is in force (recommended), the service MUST start cleanly with no keystore configured; if STRICT policy is in force, the service MUST abort at `Validate()` time naming the missing field — confirm the behavior matches the recorded "Naming Decision".
  - **Expected:** A misconfigured callback signing JSON aborts boot via `Validate()` (consolidated fail-loud surface), not at `Connect()` time. The `qfdecisions` package no longer reads the process environment — `Config` is the single ingestion point for every connector secret in the codebase. The existing operator-facing env var name is unchanged (no deploy-adapter break).
  - **Verify:** Step 1 is the structural guard; step 4 is the regression guard; step 6 is the live-stack guard; steps 7 and 8 are the adversarial behavior guards.
  - **Evidence:** report.md → Post-Fix Regression Test (PENDING — populate during Phase 5/6 implementation)
  - **Notes:** Code-review finding SEC-2 (P2 SST/security). Spec association: `specs/020-security-hardening/` (SST regime owner). Workflow mode: `bugfix-fastlane`. TDD: required (real code change, not artifact-only). Sibling references: BUG-020-008 (canonical `mustParseIntEnv` + `intLoadErrs` pattern), BUG-020-009 (canonical config-thread-through-the-call-site migration). The fix preserves the env-var name `QF_DECISIONS_CALLBACK_SIGNING_KEYS_JSON` — only the runtime ingestion path moves into Config.
