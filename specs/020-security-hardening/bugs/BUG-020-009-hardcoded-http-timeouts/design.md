# Bug Fix Design: [BUG-020-009] Hardcoded HTTP client timeouts

> **STATUS:** Initial design authored by `bubbles.bug` during Phase 2 documentation. The implementing agent (`bubbles.design` via `runSubagent` in Phase 3) MUST review, finalize the exact yaml/field/env names, and refine before code edits begin.

## Root Cause Analysis

### Investigation Summary
Code-review pass surfaced finding H-4 (P2 promoted to actionable). Two production HTTP-client constructors carry hardcoded `time.Duration` literals:

```go
// internal/connector/markets/markets.go L158
httpClient: &http.Client{Timeout: 10 * time.Second},

// internal/auth/oauth.go L117
client := &http.Client{Timeout: 15 * time.Second}
```

Neither value is sourced from `internal/config.Config`. Neither key is in `Config.Validate()`'s required-keys collector. Both directly violate the SST Zero-Defaults regime in `.github/copilot-instructions.md` and `.github/instructions/smackerel-no-defaults.instructions.md`: "ALL configuration values MUST originate from `config/smackerel.yaml`. Zero hardcoded ports, URLs, hostnames, or fallback defaults anywhere in the codebase."

The neighboring SST surface is correct: the financial-markets connector already reads its API keys, watchlists, and toggles from `connectors.financial-markets.*`; the auth surface already reads PASETO signing keys, hashing keys, telemetry flags, and the bootstrap token from `auth.*`. Only the per-call HTTP timeouts were missed.

### Root Cause
Sins of omission, not commission. Both literals predate the strict NO-DEFAULTS enforcement that was hardened by BUG-020-008 and the sibling SST scopes under spec 020. They were never migrated because no test or guard previously enforced the SST contract for `time.Duration` literals in client constructors.

### Impact Analysis
- **Affected components:** `internal/connector/markets/markets.go`, `internal/auth/oauth.go`, `internal/config/config.go`, `config/smackerel.yaml`.
- **Affected runtime:** any operator-tunable HTTP latency budget. A misbehaving upstream (Finnhub / FRED / CoinGecko / OAuth provider) cannot be triaged by tightening the timeout from config; a slow upstream that is borderline-acceptable cannot be loosened from config.
- **Affected operators:** anyone running Smackerel against an upstream with non-default latency characteristics (different region, slow ISP, sandbox endpoint, recovery mode).
- **Affected users:** none directly today, but the values become a load-bearing tunable for incident response — they SHOULD be config-driven so they can be changed without a rebuild.

## Fix Design

### Solution Approach
Four coordinated edits, mirroring the canonical pattern proven by BUG-020-008:

1. **Decide exact yaml/field/env names** (BLOCKING design step BEFORE code edits). The implementing agent MUST:
   - Inspect `config/smackerel.yaml` L452 (`connectors.financial-markets:`) and decide whether to add `http_timeout_seconds: 10` directly under that section. The user request named the path as `connectors.markets.http_timeout_seconds` but the existing section is `financial-markets`; document the chosen path in this design.md before edits.
   - Inspect `config/smackerel.yaml` L566 (`auth:`) and decide whether to add a new `oauth:` sub-section carrying `http_timeout_seconds: 15`, or attach the key directly under `auth:` as `auth.oauth_http_timeout_seconds`. The user request named the path as `auth.oauth.http_timeout_seconds` so a new `oauth:` sub-section is the literal match; document the choice.
   - Choose env var names that match the existing pattern (uppercased path components joined by `_`).
   - Choose Config field names that match the existing Go pattern (PascalCase from the yaml path).

2. **Add two int fields to `internal/config.Config`** with the chosen names. Populate them via the same fail-loud `mustParseIntEnv` helper introduced by BUG-020-008. Fold the load errors into the same `intLoadErrs` accumulator so a missing/unparseable value surfaces through the existing consolidated `Validate()` error.

3. **Add range validation in `Validate()`**. Each timeout MUST be `> 0`. A `0` or negative value MUST produce a clear range error naming the key. (A `0`-second HTTP timeout is meaningless; a negative value is a Go runtime trap.)

4. **Thread the resolved field into both call-sites**:
   - `internal/connector/markets/markets.go`: the `Connector` already takes a config struct (or accepts the value at construction time). Add the new field, plumb it through, replace `Timeout: 10 * time.Second` with `Timeout: time.Duration(cfg.FinancialMarketsHTTPTimeoutSeconds) * time.Second`.
   - `internal/auth/oauth.go`: the `GenericOAuth2.Config` (or equivalent struct on `GenericOAuth2`) already carries `ClientID`, `ClientSecret`, `TokenEndpoint`. Add the new timeout field, replace the literal in `tokenRequest` with `Timeout: time.Duration(g.Config.AuthOAuthHTTPTimeoutSeconds) * time.Second`.

The implementing agent MUST NOT take shortcuts:
- DO NOT add `if cfg.X == 0 { cfg.X = 10 }` defaulting logic anywhere. That re-introduces the silent-default anti-pattern the NO-DEFAULTS regime exists to ban.
- DO NOT add `os.Getenv("KEY", "10")` fallbacks. Use the fail-loud `mustParseIntEnv` already in the package.
- DO NOT define the timeout as a `string` or `time.Duration` yaml field to avoid the range guard. Use `int` seconds; the call-site multiplies by `time.Second`. (This matches the existing `*_seconds` SST naming pattern across the yaml.)
- DO NOT change the actual numeric values (10s, 15s) in this fix. Pure literal-to-config migration; behavior must be unchanged on a default config.

### Affected Files
| File | Change |
|------|--------|
| `internal/config/config.go` | Add 2 int fields, 2 `mustParseIntEnv` calls into `intLoadErrs`, 2 range guards in `Validate()` |
| `internal/connector/markets/markets.go` | Plumb the new field into the `Connector` struct/constructor; replace the literal at L158 |
| `internal/auth/oauth.go` | Plumb the new field into `GenericOAuth2.Config` (or equivalent); replace the literal at L117 |
| `config/smackerel.yaml` | Add `connectors.financial-markets.http_timeout_seconds: 10` and `auth.oauth.http_timeout_seconds: 15` (or chosen variant paths) |
| `internal/config/mustparseintenv_test.go` (or sibling) | Extend table-driven adversarial tests with the 2 new keys, OR add a new sibling test file |
| `internal/connector/markets/*_test.go` | Add unit test asserting `httpClient.Timeout` equals the config-derived value |
| `internal/auth/oauth_test.go` (or sibling) | Add unit test asserting the tokenRequest `http.Client.Timeout` equals the config-derived value |

### Alternative Approaches Considered
1. **Define one shared `HTTPDefaults` block under `runtime:`**. Rejected — too coarse; each call-site has different latency characteristics (token endpoints are fast; market-data endpoints are slow). Sharing a single value would force both timeouts to move together for unrelated reasons.
2. **Use `time.Duration` (e.g., `"10s"`) yaml strings**. Rejected — inconsistent with the existing `*_seconds` int-second convention used throughout `config/smackerel.yaml` (see `synthesis_timeout_seconds`, `revocation_cache_refresh_interval_seconds`, `clock_skew_tolerance_seconds`, etc.). Stay consistent.
3. **Inject a pre-constructed `*http.Client` via DI**. Rejected as part of this fix — orthogonal refactor; do not expand the change boundary. The literal-to-config migration is the targeted policy fix.
4. **Add a global `runtime.http_default_timeout_seconds` and have each call-site fall back to it when its per-call value is empty**. Rejected — that IS the silent-default anti-pattern the NO-DEFAULTS regime bans, just hidden behind a yaml indirection.

### Regression Test Design
Mirror BUG-020-008's pattern. Reuse the existing `mustparseintenv_test.go` table where possible:

1. **`TestBUG020009_MissingHTTPTimeoutKey_FailsLoud`** — table-driven over the 2 new keys; for each key, `t.Setenv(key, "")` to force `os.Unsetenv`, call `Load()`, assert error contains the missing key name.
2. **`TestBUG020009_UnparseableHTTPTimeoutKey_FailsLoud`** — table-driven over the 2 new keys; set each to `"abc"`, call `Load()`, assert error names both the key and the offending value.
3. **`TestBUG020009_ZeroOrNegativeHTTPTimeout_FailsLoud`** — table-driven over the 2 new keys; set each to `"0"` then `"-1"`, call `Validate()`, assert range error naming the key.
4. **`TestBUG020009_MissingBothHTTPTimeoutKeys_ConsolidatedError`** — unset both simultaneously, call `Load()`, assert single returned error contains BOTH key names.
5. **`TestBUG020009_FinancialMarketsHTTPTimeoutFromConfig`** in `internal/connector/markets/`: construct the connector with `cfg.FinancialMarketsHTTPTimeoutSeconds = 7`, assert `connector.httpClient.Timeout == 7 * time.Second`. Use a non-default value (NOT 10) to prove the value actually flows from config and is not a coincidence.
6. **`TestBUG020009_OAuthHTTPTimeoutFromConfig`** in `internal/auth/`: construct `GenericOAuth2` with `cfg.AuthOAuthHTTPTimeoutSeconds = 9`, exercise `tokenRequest` against `httptest.NewServer`, assert the returned client's `Timeout == 9 * time.Second`. Use a non-default value (NOT 15) for the same reason.
7. **`TestBUG020009_HelperEradicationGrep`** — guard test: `exec.Command("grep", "-nE", "Timeout: *[0-9]+ *\\* *time\\.(Second|Millisecond|Minute)", "internal/connector/markets/markets.go", "internal/auth/oauth.go")` exits 1.

**Adversarial requirement (NON-NEGOTIABLE):** Tests 5 and 6 MUST use non-default config values (7 and 9, NOT 10 and 15). A test that uses the same value as the pre-fix literal would PASS even if the migration were reverted — tautological and forbidden.

**Pre-fix evidence:** Tests 1–4 and 7 above MUST be authored AND executed BEFORE the migration. They MUST FAIL against `main` because the 2 keys do not exist in `Config` and the literals are still present.

### Rollback Plan
Revert the four affected source files and the yaml addition. Behavior is unchanged on a default config because the chosen yaml values (10, 15) match the pre-fix literals exactly. No data migration; no schema change.

## Verification Plan
- `grep -nE 'Timeout: *[0-9]+ *\* *time\.(Second|Millisecond|Minute)' internal/connector/markets/markets.go internal/auth/oauth.go` returns zero matches.
- `./smackerel.sh build` passes.
- `./smackerel.sh test unit` passes; the 7 new test functions appear in the output.
- `./smackerel.sh config generate` produces dev/test env files containing both new keys with non-empty parseable int values.
- `bash .github/bubbles/scripts/artifact-lint.sh specs/020-security-hardening/bugs/BUG-020-009-hardcoded-http-timeouts` passes.
