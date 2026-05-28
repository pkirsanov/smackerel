# Spec: [BUG-020-009] HTTP client timeouts MUST be SST config-driven

## Expected Behavior

### EB-1: Hardcoded `time.Second` literals removed from both call-sites
After the fix, NEITHER `internal/connector/markets/markets.go` NOR `internal/auth/oauth.go` may contain a `time.Duration` literal of the form `<N> * time.Second` (or `<N> * time.Millisecond`, etc.) as the `Timeout` of an `http.Client{...}` constructor. The timeout MUST be read from a resolved field on `internal/config.Config`.

### EB-2: Two new SST keys exist and are routed through `Config.Validate()`
Two new keys MUST be added to `config/smackerel.yaml` and to `internal/config.Config`:

| # | Suggested env var | Suggested field | Suggested yaml path | Replaces literal at |
|---|-------------------|-----------------|---------------------|---------------------|
| 1 | `FINANCIAL_MARKETS_HTTP_TIMEOUT_SECONDS` | `FinancialMarketsHTTPTimeoutSeconds` (int) | `connectors.financial-markets.http_timeout_seconds: 10` | `internal/connector/markets/markets.go:158` |
| 2 | `AUTH_OAUTH_HTTP_TIMEOUT_SECONDS` | `AuthOAuthHTTPTimeoutSeconds` (int) | `auth.oauth.http_timeout_seconds: 15` (new sub-section under `auth:`) | `internal/auth/oauth.go:117` |

The implementing agent MAY choose different exact names if they fit the existing yaml/Config naming better, provided EB-1, EB-3, EB-4, EB-5 are all satisfied. The chosen names MUST be documented in `design.md` BEFORE code edits begin.

Both keys MUST be wired into whatever required-keys collector `Config.Validate()` uses, so a missing or unparseable value produces a consolidated fail-loud boot error (the canonical pattern is established by BUG-020-008's `intLoadErrs` accumulator).

### EB-3: Missing or unparseable values fail loud
- Missing env var (`os.Unsetenv`) → `Config.Validate()` returns an error naming the missing key.
- Unparseable env var (e.g., `"abc"`) → `Config.Validate()` returns an error naming both the key and the offending value.
- A value of `0` or a negative int MUST also be rejected with a clear range error — a `0`-second HTTP timeout is meaningless and a negative timeout is a Go runtime trap.

### EB-4: Validate() error is consolidated
When BOTH keys are missing/invalid, `Validate()` MUST surface BOTH in a single error (same shape as BUG-020-008). An operator MUST NOT need two reboots to discover both missing keys.

### EB-5: Call-sites read from the resolved Config field
- `internal/connector/markets/markets.go` constructor MUST receive the timeout via its existing `Config` (or equivalent) struct and apply `time.Duration(cfg.FinancialMarketsHTTPTimeoutSeconds) * time.Second` when constructing the `http.Client`.
- `internal/auth/oauth.go` `tokenRequest` MUST receive the timeout via the `GenericOAuth2.Config` (or equivalent) struct and apply the same shape.
- No `time.Second` literal may remain at either call-site.

### EB-6: SST contract — yaml MUST materialize both keys with explicit non-empty values
`config/smackerel.yaml` MUST carry explicit non-empty values for both new keys. The `./smackerel.sh config generate` pipeline MUST emit both keys into `config/generated/dev.env` and `config/generated/test.env` with the yaml values. No `${VAR:-default}`, no `os.Getenv("KEY", "default")`, no in-Go default.

## Acceptance Criteria
1. `grep -nE 'Timeout: *[0-9]+ *\* *time\.(Second|Millisecond|Minute)' internal/connector/markets/markets.go internal/auth/oauth.go` returns zero matches after the fix.
2. A unit test asserts that unsetting either new env var and calling `Config.Validate()` returns an error naming the missing key.
3. A unit test asserts that unsetting both simultaneously returns a single consolidated error that names both keys.
4. A unit test asserts that setting either to a non-numeric value (e.g., `"abc"`) returns a `Validate()` error naming both the offending key and the offending value.
5. A unit test asserts that a value of `0` or a negative int is rejected with a range error naming the key.
6. A unit test in `internal/connector/markets/` asserts the constructed `http.Client.Timeout` equals `time.Duration(cfg.FinancialMarketsHTTPTimeoutSeconds) * time.Second`.
7. A unit test in `internal/auth/` asserts the OAuth `tokenRequest` `http.Client.Timeout` equals `time.Duration(cfg.AuthOAuthHTTPTimeoutSeconds) * time.Second`.
8. `./smackerel.sh build` passes after the change.
9. `./smackerel.sh test unit` passes after the change.
10. `./smackerel.sh config generate` produces `dev.env` and `test.env` that include both keys with non-empty, parseable int values.
11. `bash .github/bubbles/scripts/artifact-lint.sh specs/020-security-hardening/bugs/BUG-020-009-hardcoded-http-timeouts` passes.
