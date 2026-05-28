# User Validation: [BUG-020-009] Hardcoded HTTP client timeouts

## Checklist

### [Bug Fix] [BUG-020-009] HTTP client timeouts are SST config-driven
- [x] **What:** `internal/connector/markets/markets.go` (L158) and `internal/auth/oauth.go` (L117) no longer carry hardcoded `time.Duration` literals; both timeouts are now sourced from new SST config fields on `internal/config.Config` and surfaced through the consolidated fail-loud `Validate()` error (same pattern as BUG-020-008).
  - **Steps:**
    1. Inspect `internal/connector/markets/markets.go` and `internal/auth/oauth.go` and confirm neither file contains a `Timeout: <N> * time.Second` literal.
    2. Run `grep -nE 'Timeout: *[0-9]+ *\* *time\.(Second|Millisecond|Minute)' internal/connector/markets/markets.go internal/auth/oauth.go` and confirm zero matches (exit 1).
    3. Inspect the 6 new unit tests across `internal/config/`, `internal/connector/markets/`, and `internal/auth/`: `TestBUG020009_MissingHTTPTimeoutKey_FailsLoud`, `TestBUG020009_UnparseableHTTPTimeoutKey_FailsLoud`, `TestBUG020009_ZeroOrNegativeHTTPTimeout_FailsLoud`, `TestBUG020009_MissingBothHTTPTimeoutKeys_ConsolidatedError`, `TestBUG020009_FinancialMarketsHTTPTimeoutFromConfig`, `TestBUG020009_OAuthHTTPTimeoutFromConfig`.
    4. Run `./smackerel.sh test unit` and confirm exit 0 with the 6 new test functions in the output.
    5. Run `./smackerel.sh build` and confirm exit 0.
    6. Run `./smackerel.sh config generate` and confirm both new keys land in `config/generated/dev.env` and `config/generated/test.env` with explicit non-empty values.
    7. As an adversarial check, unset `FINANCIAL_MARKETS_HTTP_TIMEOUT_SECONDS` (or the finalized env name) in your dev env and boot the core service; confirm it ABORTS at boot with a clear error naming the missing key.
  - **Expected:** A missing or typo'd SST timeout env var aborts boot with a consolidated `Validate()` error; the runtime never starts with a hardcoded fallback. The wiring tests in markets/auth use NON-DEFAULT values (7 and 9, not 10 and 15) so they would fail if the migration were reverted.
  - **Verify:** Step 2 is the structural guard; step 4 is the regression guard; step 7 is the live-stack guard.
  - **Evidence:** report.md → Post-Fix Regression Test (PENDING — populate during Phase 5/6 implementation)
  - **Notes:** Code-review finding H-4 (P2 promoted to actionable). Spec association: `specs/020-security-hardening/`. Workflow mode: `bugfix-fastlane` (NOT tdd.exempt — this IS a code change with real Red→Green tests). Sibling reference: BUG-020-008 (same SST NO-DEFAULTS regime; same `mustParseIntEnv` + `intLoadErrs` pattern).
