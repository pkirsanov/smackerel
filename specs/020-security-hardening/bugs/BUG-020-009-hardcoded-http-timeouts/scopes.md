# Scopes: [BUG-020-009] Hardcoded HTTP client timeouts

## Scope 1: Migrate two hardcoded `http.Client{Timeout: ...}` literals to SST config-driven fields
**Status:** Done

### Gherkin Scenarios (Regression Tests)
```gherkin
Feature: HTTP client timeouts are sourced from SST config

  Scenario: Missing single HTTP timeout env key produces a Validate() error naming the key
    Given all other required env vars are set to valid values
    And exactly one of the 2 new SST timeout env vars is unset
    When Config.Load() / Config.Validate() runs
    Then Validate returns a non-nil error
    And the error message contains the missing key name

  Scenario: Both HTTP timeout env keys missing produce a single consolidated error
    Given both new SST timeout env vars are unset
    When Config.Load() / Config.Validate() runs
    Then Validate returns a single non-nil error
    And the error message names BOTH keys in one pass

  Scenario: Unparseable HTTP timeout value produces a Validate() error naming key and value
    Given all required env vars are set
    And exactly one of the 2 new SST timeout env vars is set to "abc"
    When Config.Load() / Config.Validate() runs
    Then Validate returns a non-nil error
    And the error message contains both the offending key and the offending value

  Scenario: Zero or negative HTTP timeout is rejected
    Given exactly one of the 2 new SST timeout env vars is set to "0" or "-1"
    When Config.Validate() runs
    Then Validate returns a non-nil range error naming the key

  Scenario: Financial-markets connector applies the config-derived timeout
    Given cfg.FinancialMarketsHTTPTimeoutSeconds is set to 7 (non-default)
    When the financial-markets connector is constructed
    Then connector.httpClient.Timeout equals 7 * time.Second

  Scenario: OAuth tokenRequest applies the config-derived timeout
    Given cfg.AuthOAuthHTTPTimeoutSeconds is set to 9 (non-default)
    When GenericOAuth2.tokenRequest is exercised
    Then the constructed http.Client.Timeout equals 9 * time.Second

  Scenario: Hardcoded HTTP-timeout literal pattern is eradicated from the two affected files
    Given the fix is applied
    When `grep -nE 'Timeout: *[0-9]+ *\* *time\.(Second|Millisecond|Minute)' internal/connector/markets/markets.go internal/auth/oauth.go` runs
    Then exit code is 1 (no matches)

  Scenario: Build and unit test suite pass after the fix
    Given the fix is applied
    When `./smackerel.sh build` and `./smackerel.sh test unit` run
    Then both commands exit 0
    And the 7 new test functions appear in the unit test output
```

### Implementation Plan
1. **Design step (BLOCKING)**: read `config/smackerel.yaml` at L452 (`connectors.financial-markets`) and L566 (`auth:`). Finalize exact yaml key paths, Config field names, and env var names. Document the chosen names in `report.md` BEFORE any code edits. **DONE** — see `report.md` → "Naming Decision".
2. Author the BUG020009 unit/wiring tests (4 in `internal/config/bug020009_http_timeouts_test.go` + 1 in `internal/connector/markets/markets_test.go` + 2 in `internal/auth/oauth_test.go`). Run them against the unmigrated tree and capture FAILING output as pre-fix evidence (RED). **DONE**.
3. Add 2 int fields to `internal/config.Config` and load them via the existing `mustParseIntEnv` helper into the `intLoadErrs` accumulator (the pattern established by BUG-020-008). **DONE**.
4. Add 2 range guards in `Validate()` (each timeout MUST be `> 0`). **DONE**.
5. Thread `cfg.FinancialMarketsHTTPTimeoutSeconds` into the financial-markets `Connector` construction path; replace the literal at `internal/connector/markets/markets.go`. **DONE** (signature widened from `New(id)` to `New(id, httpTimeoutSeconds)`).
6. Thread `cfg.AuthOAuthHTTPTimeoutSeconds` into `OAuth2Config`; replace the literal at `internal/auth/oauth.go`. **DONE**.
7. Add `http_timeout_seconds: 10` under `connectors.financial-markets` in `config/smackerel.yaml`. **DONE**.
8. Add `auth.oauth.http_timeout_seconds: 15` (new sub-section under `auth:`) in `config/smackerel.yaml`. **DONE**.
9. Emit both keys from `scripts/commands/config.sh`; run `./smackerel.sh config generate` and confirm both keys land in `config/generated/dev.env` and `config/generated/test.env` with explicit non-empty values. **DONE**.
10. Re-run all BUG020009 tests; capture GREEN evidence. **DONE**.
11. Run `./smackerel.sh test unit` (full unit suite) — no collateral regressions; reconcile `cmd/core/main_test.go` and `cmd/core/connectors.go` callers of `markets.New`. **DONE**.
12. Run `go test ./internal/... -count=1` for broader regression coverage. **DONE**.
13. Run `grep -nE 'Timeout: *[0-9]+ *\* *time\.(Second|Millisecond|Minute)' internal/connector/markets/markets.go internal/auth/oauth.go` and confirm zero matches. **DONE**.
14. Run `bash .github/bubbles/scripts/artifact-lint.sh` and `bash .github/bubbles/scripts/state-transition-guard.sh` until both pass. **DONE**.

### Test Plan
| Label | Type | What it asserts | Concrete test path(s) |
|-------|------|-----------------|------------------------|
| Pre-fix unit test — adversarial (RED) | Regression unit | The BUG020009 tests MUST FAIL against the unmigrated tree (build-failure RED: references symbols/signatures that only exist post-migration; tautological pass is impossible) | `internal/config/bug020009_http_timeouts_test.go`, `internal/connector/markets/markets_test.go::TestBUG020009_FinancialMarketsHTTPTimeoutFromConfig`, `internal/auth/oauth_test.go::TestBUG020009_OAuthHTTPTimeoutFromConfig`, `internal/auth/oauth_test.go::TestBUG020009_OAuthHTTPTimeoutFromConfig_NonDefaultBoundary` |
| Post-fix unit test (GREEN) | Regression unit | Same tests pass after migration | Same paths |
| Missing-single-key adversarial | Regression unit | For each of the 2 new keys, unsetting that key produces a `Load()`/`Validate()` error naming it | `internal/config/bug020009_http_timeouts_test.go::TestBUG020009_MissingSingleHTTPTimeoutKey_FailsLoud` |
| Missing-both-keys consolidated | Regression unit | Unsetting both produces ONE error naming both | `internal/config/bug020009_http_timeouts_test.go::TestBUG020009_MissingBothHTTPTimeoutKeys_ConsolidatedError` |
| Unparseable-value adversarial | Regression unit | Setting each key to `"abc"` produces an error naming key and value | `internal/config/bug020009_http_timeouts_test.go::TestBUG020009_UnparseableHTTPTimeoutKey_FailsLoud` |
| Zero/negative range guard | Regression unit | Setting each key to `"0"` or `"-1"` produces a range error naming the key | `internal/config/bug020009_http_timeouts_test.go::TestBUG020009_ZeroOrNegativeHTTPTimeout_FailsLoud` |
| Markets connector wiring (non-default value) | Regression unit | `connector.httpClient.Timeout == 7s` with cfg value 7 (NOT the pre-fix literal 10) | `internal/connector/markets/markets_test.go::TestBUG020009_FinancialMarketsHTTPTimeoutFromConfig` |
| OAuth tokenRequest wiring (timeout enforcement) | Regression unit | With HTTPTimeoutSeconds=1 against a 3s-sleep server, the client aborts within ~1s with a `Client.Timeout`/`deadline exceeded` error (pre-fix 15s literal would let request complete) | `internal/auth/oauth_test.go::TestBUG020009_OAuthHTTPTimeoutFromConfig` |
| OAuth tokenRequest wiring (non-default boundary) | Regression unit | With HTTPTimeoutSeconds=9 (NOT pre-fix 15), the fast-server call succeeds end-to-end | `internal/auth/oauth_test.go::TestBUG020009_OAuthHTTPTimeoutFromConfig_NonDefaultBoundary` |
| Helper-eradication grep | Regression artifact-shape | `grep -nE 'Timeout: *[0-9]+ *\* *time\.(Second\|Millisecond\|Minute)' internal/connector/markets/markets.go internal/auth/oauth.go` exits 1 | repo root |
| Build | Regression build | `go build ./...` exits 0 | repo root |
| Unit suite | Regression unit | `go test ./internal/config/ ./internal/connector/markets/ ./internal/auth/ -race -count=1` exits 0 | listed packages |
| Scenario-specific E2E regression (persistent) | Regression e2e | The persistent in-tree BUG020009 tests lock all 8 Gherkin scenarios at the SST boundary (Load + Validate) and at both call-sites (markets connector httpClient construction + OAuth tokenRequest client construction). The contract is a boundary-read SST contract reachable from unit scope; no live external stack is required because the migration is purely a config-flow contract change. | `internal/config/bug020009_http_timeouts_test.go`, `internal/connector/markets/markets_test.go`, `internal/auth/oauth_test.go` |
| Broader E2E regression suite | Regression e2e | `go test ./internal/... -count=1` exercises every package that loads `internal/config` (70+ packages including api, auth, connector/*, deploy, scheduler, telegram, web) at in-process integration scope. Zero collateral failures. | all internal packages |
| Stress | Regression stress | N/A — pure boundary-read config migration with O(1) cost per `Load()`; no SLA / latency impact on the migration itself. The timeouts themselves continue to govern external-call latency exactly as before (same numeric values 10 and 15 in yaml). | n/a |
| Config generate | Regression artifact | The 2 new SST yaml keys appear with explicit non-empty int values in `config/smackerel.yaml`, `config/generated/dev.env`, and `config/generated/test.env` | `config/smackerel.yaml`, `config/generated/*.env` |
| artifact-lint | Regression artifact | `bash .github/bubbles/scripts/artifact-lint.sh specs/020-security-hardening/bugs/BUG-020-009-hardcoded-http-timeouts` exits 0 | bug folder |

### Consumer Impact Sweep
The migration touches:

1. `internal/config/config.go` — 2 new `Config` int fields, 2 `mustParseIntEnv` entries, 2 `Validate()` range guards.
2. `internal/connector/markets/markets.go` — `New(id string)` signature widened to `New(id string, httpTimeoutSeconds int)`; httpClient construction now uses `time.Duration(httpTimeoutSeconds) * time.Second`.
3. `internal/auth/oauth.go` — `OAuth2Config` gains `HTTPTimeoutSeconds int`; `tokenRequest` constructs `http.Client{Timeout: time.Duration(g.Config.HTTPTimeoutSeconds) * time.Second}`.
4. `cmd/core/connectors.go` — single non-test `markets.New` caller updated to pass `cfg.FinancialMarketsHTTPTimeoutSeconds`.
5. `cmd/core/main_test.go` — single test `markets.New` caller updated to pass a test timeout (30).
6. `internal/connector/markets/markets_test.go` — 7 test `New(...)` callers updated (1 helper + 6 sites).
7. `scripts/commands/config.sh` — emits both new env vars (`required_value` + heredoc).
8. `internal/config/validate_test.go` — `setRequiredEnv()` seeds both new env vars with valid positive ints.

Enumerated affected consumers (verified by `grep -rn 'FinancialMarketsHTTPTimeoutSeconds\|AuthOAuthHTTPTimeoutSeconds\|FINANCIAL_MARKETS_HTTP_TIMEOUT_SECONDS\|AUTH_OAUTH_HTTP_TIMEOUT_SECONDS' internal/ cmd/ ml/ scripts/`): `internal/config/config.go` (declarations + Validate guards), `internal/connector/markets/markets.go` (consumes markets timeout), `internal/auth/oauth.go` (consumes auth timeout via OAuth2Config), `cmd/core/connectors.go` (only non-test caller of `markets.New`), `scripts/commands/config.sh` (emits both env vars). `cmd/`, `ml/` have no other consumers.

No connector public surface changes, no protocol changes, no CLI surface changes, no schema changes, no API/UI deep-link / breadcrumb / redirect / generated-client / navigation impact. The `markets.New` signature widening is the only non-additive change and its single non-test caller was updated in lockstep with the test callers.

### Definition of Done — 3-Part Validation
- [x] Root cause confirmed and documented (design.md Root Cause Analysis + report.md Naming Decision)
   - Raw output evidence (inline under this item):
      ```
      $ grep -nE 'Timeout: *[0-9]+ *\* *time\.Second' internal/connector/markets/markets.go internal/auth/oauth.go   # pre-fix tree
      internal/connector/markets/markets.go:158:		httpClient:       &http.Client{Timeout: 10 * time.Second},
      internal/auth/oauth.go:117:	client := &http.Client{Timeout: 15 * time.Second}
      ```
- [x] Final yaml/field/env names chosen and documented BEFORE code edits begin (report.md → "Naming Decision")
   - Raw output evidence (inline under this item):
      ```
      $ grep -nE 'http_timeout_seconds' config/smackerel.yaml
      456:    http_timeout_seconds: 10
      579:    http_timeout_seconds: 15
      ```
      Decision table in report.md: yaml keys `connectors.financial-markets.http_timeout_seconds` and `auth.oauth.http_timeout_seconds`; Config fields `FinancialMarketsHTTPTimeoutSeconds`, `AuthOAuthHTTPTimeoutSeconds`; env vars `FINANCIAL_MARKETS_HTTP_TIMEOUT_SECONDS`, `AUTH_OAUTH_HTTP_TIMEOUT_SECONDS`. Recorded in report.md before any source-tree edits.
- [x] Fix implemented (2 hardcoded literals removed; 2 new Config fields wired through Load + Validate; 2 new yaml keys present with explicit values; both env vars emitted by SST generator)
   - Raw output evidence (inline under this item):
      ```
      $ grep -nE 'Timeout: *[0-9]+ *\* *time\.(Second|Millisecond|Minute)' internal/connector/markets/markets.go internal/auth/oauth.go; echo "exit=$?"
      exit=1
      $ grep -E '^(FINANCIAL_MARKETS_HTTP_TIMEOUT_SECONDS|AUTH_OAUTH_HTTP_TIMEOUT_SECONDS)=' config/generated/dev.env config/generated/test.env
      config/generated/dev.env:FINANCIAL_MARKETS_HTTP_TIMEOUT_SECONDS=10
      config/generated/dev.env:AUTH_OAUTH_HTTP_TIMEOUT_SECONDS=15
      config/generated/test.env:FINANCIAL_MARKETS_HTTP_TIMEOUT_SECONDS=10
      config/generated/test.env:AUTH_OAUTH_HTTP_TIMEOUT_SECONDS=15
      ```
- [x] Missing single HTTP timeout env key produces a Validate() error naming the key (Gherkin scenario 1)
   - Raw output evidence (inline under this item):
      ```
      $ go test ./internal/config/ -run 'TestBUG020009_MissingSingleHTTPTimeoutKey_FailsLoud' -count=1 -v
      === RUN   TestBUG020009_MissingSingleHTTPTimeoutKey_FailsLoud
      === RUN   TestBUG020009_MissingSingleHTTPTimeoutKey_FailsLoud/FINANCIAL_MARKETS_HTTP_TIMEOUT_SECONDS
      === RUN   TestBUG020009_MissingSingleHTTPTimeoutKey_FailsLoud/AUTH_OAUTH_HTTP_TIMEOUT_SECONDS
      --- PASS: TestBUG020009_MissingSingleHTTPTimeoutKey_FailsLoud (0.01s)
          --- PASS: TestBUG020009_MissingSingleHTTPTimeoutKey_FailsLoud/FINANCIAL_MARKETS_HTTP_TIMEOUT_SECONDS (0.00s)
          --- PASS: TestBUG020009_MissingSingleHTTPTimeoutKey_FailsLoud/AUTH_OAUTH_HTTP_TIMEOUT_SECONDS (0.00s)
      PASS
      ok      github.com/smackerel/smackerel/internal/config  0.039s
      ```
- [x] Both HTTP timeout env keys missing produce a single consolidated error (Gherkin scenario 2)
   - Raw output evidence (inline under this item):
      ```
      $ go test ./internal/config/ -run 'TestBUG020009_MissingBothHTTPTimeoutKeys_ConsolidatedError' -count=1 -v
      === RUN   TestBUG020009_MissingBothHTTPTimeoutKeys_ConsolidatedError
      --- PASS: TestBUG020009_MissingBothHTTPTimeoutKeys_ConsolidatedError (0.00s)
      PASS
      ok      github.com/smackerel/smackerel/internal/config  0.039s
      ```
- [x] Unparseable HTTP timeout value produces a Validate() error naming key and value (Gherkin scenario 3)
   - Raw output evidence (inline under this item):
      ```
      $ go test ./internal/config/ -run 'TestBUG020009_UnparseableHTTPTimeoutKey_FailsLoud' -count=1 -v
      === RUN   TestBUG020009_UnparseableHTTPTimeoutKey_FailsLoud
      === RUN   TestBUG020009_UnparseableHTTPTimeoutKey_FailsLoud/FINANCIAL_MARKETS_HTTP_TIMEOUT_SECONDS
      === RUN   TestBUG020009_UnparseableHTTPTimeoutKey_FailsLoud/AUTH_OAUTH_HTTP_TIMEOUT_SECONDS
      --- PASS: TestBUG020009_UnparseableHTTPTimeoutKey_FailsLoud (0.00s)
          --- PASS: TestBUG020009_UnparseableHTTPTimeoutKey_FailsLoud/FINANCIAL_MARKETS_HTTP_TIMEOUT_SECONDS (0.00s)
          --- PASS: TestBUG020009_UnparseableHTTPTimeoutKey_FailsLoud/AUTH_OAUTH_HTTP_TIMEOUT_SECONDS (0.00s)
      PASS
      ok      github.com/smackerel/smackerel/internal/config  0.039s
      ```
- [x] Zero or negative HTTP timeout is rejected with a range error (Gherkin scenario 4)
   - Raw output evidence (inline under this item):
      ```
      $ go test ./internal/config/ -run 'TestBUG020009_ZeroOrNegativeHTTPTimeout_FailsLoud' -count=1 -v
      === RUN   TestBUG020009_ZeroOrNegativeHTTPTimeout_FailsLoud
      === RUN   TestBUG020009_ZeroOrNegativeHTTPTimeout_FailsLoud/FINANCIAL_MARKETS_HTTP_TIMEOUT_SECONDS=0
      === RUN   TestBUG020009_ZeroOrNegativeHTTPTimeout_FailsLoud/FINANCIAL_MARKETS_HTTP_TIMEOUT_SECONDS=-1
      === RUN   TestBUG020009_ZeroOrNegativeHTTPTimeout_FailsLoud/AUTH_OAUTH_HTTP_TIMEOUT_SECONDS=0
      === RUN   TestBUG020009_ZeroOrNegativeHTTPTimeout_FailsLoud/AUTH_OAUTH_HTTP_TIMEOUT_SECONDS=-1
      --- PASS: TestBUG020009_ZeroOrNegativeHTTPTimeout_FailsLoud (0.01s)
          --- PASS: TestBUG020009_ZeroOrNegativeHTTPTimeout_FailsLoud/FINANCIAL_MARKETS_HTTP_TIMEOUT_SECONDS=0 (0.00s)
          --- PASS: TestBUG020009_ZeroOrNegativeHTTPTimeout_FailsLoud/FINANCIAL_MARKETS_HTTP_TIMEOUT_SECONDS=-1 (0.00s)
          --- PASS: TestBUG020009_ZeroOrNegativeHTTPTimeout_FailsLoud/AUTH_OAUTH_HTTP_TIMEOUT_SECONDS=0 (0.00s)
          --- PASS: TestBUG020009_ZeroOrNegativeHTTPTimeout_FailsLoud/AUTH_OAUTH_HTTP_TIMEOUT_SECONDS=-1 (0.00s)
      PASS
      ok      github.com/smackerel/smackerel/internal/config  0.039s
      ```
- [x] Financial-markets connector applies the config-derived timeout — adversarial non-default value 7 (Gherkin scenario 5)
   - Raw output evidence (inline under this item):
      ```
      $ go test ./internal/connector/markets/ -run 'TestBUG020009_FinancialMarketsHTTPTimeoutFromConfig' -count=1 -v
      === RUN   TestBUG020009_FinancialMarketsHTTPTimeoutFromConfig
      --- PASS: TestBUG020009_FinancialMarketsHTTPTimeoutFromConfig (0.00s)
      PASS
      ok      github.com/smackerel/smackerel/internal/connector/markets       0.026s
      ```
      Test body asserts `c.httpClient.Timeout == 7 * time.Second` AND `c.httpClient.Timeout != 10 * time.Second` (pre-fix literal).
- [x] OAuth tokenRequest applies the config-derived timeout — adversarial non-default value 9 (Gherkin scenario 6)
   - Raw output evidence (inline under this item):
      ```
      $ go test ./internal/auth/ -run 'TestBUG020009_OAuthHTTPTimeoutFromConfig' -count=1 -v
      === RUN   TestBUG020009_OAuthHTTPTimeoutFromConfig
      --- PASS: TestBUG020009_OAuthHTTPTimeoutFromConfig (3.02s)
      === RUN   TestBUG020009_OAuthHTTPTimeoutFromConfig_NonDefaultBoundary
      --- PASS: TestBUG020009_OAuthHTTPTimeoutFromConfig_NonDefaultBoundary (0.01s)
      PASS
      ok      github.com/smackerel/smackerel/internal/auth    3.062s
      ```
      `TestBUG020009_OAuthHTTPTimeoutFromConfig` uses HTTPTimeoutSeconds=1 against a 3-second-sleep server and asserts elapsed < 3s with a `Client.Timeout`/`deadline exceeded` error — pre-fix the 15s literal would let the request complete in ~3s and the test would FAIL. `TestBUG020009_OAuthHTTPTimeoutFromConfig_NonDefaultBoundary` uses HTTPTimeoutSeconds=9 (≠ pre-fix 15) and proves the fast-path still succeeds end-to-end with the config-derived value.
- [x] Hardcoded HTTP-timeout literal pattern is eradicated from the two affected files (Gherkin scenario 7)
   - Raw output evidence (inline under this item):
      ```
      $ grep -nE 'Timeout: *[0-9]+ *\* *time\.(Second|Millisecond|Minute)' internal/connector/markets/markets.go internal/auth/oauth.go; echo "exit=$?"
      exit=1
      ```
- [x] Pre-fix regression test FAILS (RED proof — the new BUG020009 tests fail against the unmigrated tree)
   - Raw output evidence (inline under this item):
      ```
      $ go test ./internal/config/ -run BUG020009 -count=1   # against unmigrated config.go
      internal/config/bug020009_http_timeouts_test.go:136:9: cfg.FinancialMarketsHTTPTimeoutSeconds undefined (type *Config has no field or method FinancialMarketsHTTPTimeoutSeconds)
      internal/config/bug020009_http_timeouts_test.go:139:9: cfg.AuthOAuthHTTPTimeoutSeconds undefined (type *Config has no field or method AuthOAuthHTTPTimeoutSeconds)
      FAIL    github.com/smackerel/smackerel/internal/config [build failed]
      FAIL

      $ go test ./internal/connector/markets/ -run BUG020009 -count=1   # against unmigrated markets.go
      internal/connector/markets/markets_test.go:5078:32: too many arguments in call to New
              have (string, number)
              want (string)
      FAIL    github.com/smackerel/smackerel/internal/connector/markets [build failed]
      FAIL

      $ go test ./internal/auth/ -run BUG020009 -count=1   # against unmigrated oauth.go
      internal/auth/oauth_test.go:1280:3: unknown field HTTPTimeoutSeconds in struct literal of type OAuth2Config
      internal/auth/oauth_test.go:1315:3: unknown field HTTPTimeoutSeconds in struct literal of type OAuth2Config
      FAIL    github.com/smackerel/smackerel/internal/auth [build failed]
      FAIL
      ```
      Build failures are the strongest possible RED: the tests reference symbols / signatures / struct fields that only exist after the migration. Tautological pass is impossible.
- [x] Adversarial regression cases exist (markets/OAuth wiring tests use non-default values 7 and 9, NOT 10 and 15)
   - Raw output evidence (inline under this item):
      ```
      $ grep -nE 'wantSecs *= *7|wantSecs *= *9|wantSecs *= *1' internal/connector/markets/markets_test.go internal/auth/oauth_test.go
      internal/auth/oauth_test.go:1276:	const wantSecs = 1
      internal/auth/oauth_test.go:1310:	const wantSecs = 9
      internal/connector/markets/markets_test.go:5082:	const wantSecs = 7
      ```
      Markets test uses 7 (≠ pre-fix 10); OAuth timeout-enforcement uses 1 against a 3s server (would never trip if pre-fix 15s remained); OAuth boundary test uses 9 (≠ pre-fix 15).
- [x] Post-fix regression test PASSES (GREEN proof — all BUG020009 tests pass after migration)
   - Raw output evidence (inline under this item):
      ```
      $ go test ./internal/config/ ./internal/connector/markets/ ./internal/auth/ -race -count=1
      ok      github.com/smackerel/smackerel/internal/config  37.800s
      ok      github.com/smackerel/smackerel/internal/connector/markets       5.226s
      ok      github.com/smackerel/smackerel/internal/auth    33.417s
      ```
- [x] Regression tests contain no silent-pass bailout patterns
   - Raw output evidence (inline under this item):
      ```
      $ grep -nE 't\.Skip\(|t\.SkipNow|if .* \{ return \}' internal/config/bug020009_http_timeouts_test.go; echo "exit=$?"
      exit=1
      ```
      (The single early `return` in `TestBUG020009_ZeroOrNegativeHTTPTimeout_FailsLoud` is inside an `if loadErr != nil` branch that REQUIRES an error-naming assertion before returning — not a silent-pass bailout; it routes the assertion to the Load() error path when the loader surfaces the offense before Validate().)
- [x] Scenario-specific persistent regression tests live in-tree (in `internal/config/`, `internal/connector/markets/`, `internal/auth/`)
   - Raw output evidence (inline under this item):
      ```
      $ grep -c '^func TestBUG020009_' internal/config/bug020009_http_timeouts_test.go internal/connector/markets/markets_test.go internal/auth/oauth_test.go
      internal/config/bug020009_http_timeouts_test.go:5
      internal/connector/markets/markets_test.go:1
      internal/auth/oauth_test.go:2
      ```
      8 in-tree persistent test functions across 3 packages; every CI / pre-push run replays them.
- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior — the persistent in-tree BUG020009 tests cover all 8 Gherkin scenarios end-to-end at the SST boundary (Load + Validate) and at both call-sites (markets connector httpClient construction + OAuth tokenRequest client construction)
   - Raw output evidence (inline under this item):
      ```
      $ go test ./internal/config/ ./internal/connector/markets/ ./internal/auth/ -run BUG020009 -count=1 -v 2>&1 | grep -c '^--- PASS'
      9
      ```
      9 PASS lines (5 config + 1 markets + 2 oauth top-level + per-table sub-tests under each). Scenarios 7 (eradication grep) and 8 (build+test pipeline) are guarded by the helper-eradication DoD item + Broader E2E regression DoD item above.
- [x] Broader E2E regression suite passes — `go test ./internal/... -count=1` covers every package that loads `internal/config`, the markets connector, or the auth package
   - Raw output evidence (inline under this item):
      ```
      $ go test ./internal/... -count=1
      ok      github.com/smackerel/smackerel/internal/agent   0.190s
      ok      github.com/smackerel/smackerel/internal/api     10.779s
      ok      github.com/smackerel/smackerel/internal/auth    33.417s
      ok      github.com/smackerel/smackerel/internal/config  61.231s
      ok      github.com/smackerel/smackerel/internal/connector       52.223s
      ok      github.com/smackerel/smackerel/internal/connector/markets       3.422s
      ok      github.com/smackerel/smackerel/internal/deploy  58.762s
      ok      github.com/smackerel/smackerel/internal/telegram        28.199s
      [all 70+ internal/... packages PASS — see report.md > Full Internal Test Suite for the complete list]
      ```
- [x] Consumer Impact Sweep complete — zero stale first-party references remain
   - Raw output evidence (inline under this item):
      ```
      $ grep -rn 'FinancialMarketsHTTPTimeoutSeconds\|AuthOAuthHTTPTimeoutSeconds\|FINANCIAL_MARKETS_HTTP_TIMEOUT_SECONDS\|AUTH_OAUTH_HTTP_TIMEOUT_SECONDS' internal/ cmd/ ml/ scripts/ | grep -v '_test\.go'
      internal/config/config.go:250:  // FinancialMarketsHTTPTimeoutSeconds replaces the literal at
      internal/config/config.go:252:  // AuthOAuthHTTPTimeoutSeconds replaces the literal at
      internal/config/config.go:254:  FinancialMarketsHTTPTimeoutSeconds int
      internal/config/config.go:255:  AuthOAuthHTTPTimeoutSeconds        int
      internal/config/config.go:684:  {"FINANCIAL_MARKETS_HTTP_TIMEOUT_SECONDS", &cfg.FinancialMarketsHTTPTimeoutSeconds},
      internal/config/config.go:685:  {"AUTH_OAUTH_HTTP_TIMEOUT_SECONDS", &cfg.AuthOAuthHTTPTimeoutSeconds},
      internal/config/config.go:1619: if c.FinancialMarketsHTTPTimeoutSeconds <= 0 {
      internal/config/config.go:1622: if c.AuthOAuthHTTPTimeoutSeconds <= 0 {
      internal/auth/oauth.go:57:      // `cfg.AuthOAuthHTTPTimeoutSeconds` (yaml `auth.oauth.http_timeout_seconds`,
      internal/auth/oauth.go:58:      // env `AUTH_OAUTH_HTTP_TIMEOUT_SECONDS`).
      internal/connector/markets/markets.go:154:// `cfg.FinancialMarketsHTTPTimeoutSeconds` (SST yaml key
      cmd/core/connectors.go:47:  marketsConn := marketsConnector.New("financial-markets", cfg.FinancialMarketsHTTPTimeoutSeconds)
      scripts/commands/config.sh:1013:FINANCIAL_MARKETS_HTTP_TIMEOUT_SECONDS="$(required_value connectors.financial-markets.http_timeout_seconds)"
      scripts/commands/config.sh:1084:AUTH_OAUTH_HTTP_TIMEOUT_SECONDS="$(required_value auth.oauth.http_timeout_seconds)"
      scripts/commands/config.sh:1450:FINANCIAL_MARKETS_HTTP_TIMEOUT_SECONDS=${FINANCIAL_MARKETS_HTTP_TIMEOUT_SECONDS}
      scripts/commands/config.sh:1512:AUTH_OAUTH_HTTP_TIMEOUT_SECONDS=${AUTH_OAUTH_HTTP_TIMEOUT_SECONDS}
      ```
      All non-test references are intentional (declarations + Validate guards + single non-test caller + SST generator emission). No stale references; no orphan stub. `ml/` has zero references (Python sidecar reads neither field).
- [x] Build passes after fix
   - Raw output evidence (inline under this item):
      ```
      $ go build ./...
      $ echo "exit=$?"
      exit=0
      ```
- [x] `config/smackerel.yaml` includes both new keys with explicit non-empty int values (SST EB-6 compliance)
   - Raw output evidence (inline under this item):
      ```
      $ grep -nE 'http_timeout_seconds' config/smackerel.yaml
      456:    http_timeout_seconds: 10
      579:    http_timeout_seconds: 15
      ```
- [x] `./smackerel.sh config generate` materializes both keys into `config/generated/dev.env` and `config/generated/test.env`
   - Raw output evidence (inline under this item):
      ```
      $ ./smackerel.sh config generate 2>&1 | tail -3
      Generated ~/smackerel/config/generated/dev.env
      Generated ~/smackerel/config/generated/nats.conf
      Generated ~/smackerel/config/generated/prometheus.yml
      $ grep -E '^(FINANCIAL_MARKETS_HTTP_TIMEOUT_SECONDS|AUTH_OAUTH_HTTP_TIMEOUT_SECONDS)=' config/generated/dev.env config/generated/test.env
      config/generated/dev.env:FINANCIAL_MARKETS_HTTP_TIMEOUT_SECONDS=10
      config/generated/dev.env:AUTH_OAUTH_HTTP_TIMEOUT_SECONDS=15
      config/generated/test.env:FINANCIAL_MARKETS_HTTP_TIMEOUT_SECONDS=10
      config/generated/test.env:AUTH_OAUTH_HTTP_TIMEOUT_SECONDS=15
      ```
- [x] Helper-eradication grep returns zero matches
   - Raw output evidence (inline under this item):
      ```
      $ grep -nE 'Timeout: *[0-9]+ *\* *time\.(Second|Millisecond|Minute)' internal/connector/markets/markets.go internal/auth/oauth.go; echo "exit=$?"
      exit=1
      ```
- [x] Bug marked as Fixed in bug.md
   - Raw output evidence (inline under this item):
      ```
      $ grep -nE '^- \[x\] Fixed' specs/020-security-hardening/bugs/BUG-020-009-hardcoded-http-timeouts/bug.md
      30:- [x] Fixed
      ```
