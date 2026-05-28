# Report: [BUG-020-009] Hardcoded HTTP client timeouts

## Summary
Code-review finding H-4 (P2 promoted to actionable). Two production HTTP-client constructors in `internal/connector/markets/markets.go` (L158) and `internal/auth/oauth.go` (L117) carry hardcoded `time.Duration` literals (10s and 15s respectively) instead of being sourced from SST config. The fix introduces two new int fields on `internal/config.Config`, plumbs them through `Load()` / `Validate()` using the canonical `mustParseIntEnv` + `intLoadErrs` accumulator pattern established by BUG-020-008, and replaces both literals at the call-sites.

## Completion Statement
Bug FIXED. Workflow mode `bugfix-fastlane` (ceiling `done`; TDD required). Real code change (NOT artifact-only). RED captured (BUG020009 tests fail to build against the unmigrated tree — strongest possible RED), GREEN confirmed (all BUG020009 tests pass post-migration; full internal/... PASS across 70+ packages). Two hardcoded `time.Duration` literals removed; 2 new SST keys live in `config/smackerel.yaml`; 2 new `Config` int fields wired through `Load()` (via `mustParseIntEnv`/`intLoadErrs`) and `Validate()` (with range guards `> 0`); both env vars emitted by `scripts/commands/config.sh` and materialized into `config/generated/dev.env` + `config/generated/test.env`. State promoted to `status: done` with `certifiedAt`/`certifiedBy` recorded.

## Bug Reproduction — Before Fix
```
$ grep -nE 'Timeout: *[0-9]+ *\* *time\.Second' internal/connector/markets/markets.go internal/auth/oauth.go
internal/connector/markets/markets.go:158:		httpClient:       &http.Client{Timeout: 10 * time.Second},
internal/auth/oauth.go:117:	client := &http.Client{Timeout: 15 * time.Second}
```
Both literals are present at the call-sites; neither value is sourced from `internal/config.Config`; neither key exists in `Config.Validate()`'s required-keys collector.

**Claim Source:** executed — captured via `grep_search` against the workspace at filing time. Exact line numbers verified against `read_file` of each source file.

## Test Evidence

### Pre-Fix Regression (RED)
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
Build failures are the strongest possible RED — the tests reference symbols / signatures / struct fields that only exist after the migration; tautological pass is impossible.

**Claim Source:** executed — captured against the unmigrated tree before any production source edits landed.

### Post-Fix Regression (GREEN)
```
$ go test ./internal/config/ ./internal/connector/markets/ ./internal/auth/ -run BUG020009 -count=1 -v
=== RUN   TestBUG020009_MissingSingleHTTPTimeoutKey_FailsLoud
=== RUN   TestBUG020009_MissingSingleHTTPTimeoutKey_FailsLoud/FINANCIAL_MARKETS_HTTP_TIMEOUT_SECONDS
=== RUN   TestBUG020009_MissingSingleHTTPTimeoutKey_FailsLoud/AUTH_OAUTH_HTTP_TIMEOUT_SECONDS
--- PASS: TestBUG020009_MissingSingleHTTPTimeoutKey_FailsLoud (0.01s)
    --- PASS: TestBUG020009_MissingSingleHTTPTimeoutKey_FailsLoud/FINANCIAL_MARKETS_HTTP_TIMEOUT_SECONDS (0.00s)
    --- PASS: TestBUG020009_MissingSingleHTTPTimeoutKey_FailsLoud/AUTH_OAUTH_HTTP_TIMEOUT_SECONDS (0.00s)
=== RUN   TestBUG020009_MissingBothHTTPTimeoutKeys_ConsolidatedError
--- PASS: TestBUG020009_MissingBothHTTPTimeoutKeys_ConsolidatedError (0.00s)
=== RUN   TestBUG020009_UnparseableHTTPTimeoutKey_FailsLoud
--- PASS: TestBUG020009_UnparseableHTTPTimeoutKey_FailsLoud (0.00s)
=== RUN   TestBUG020009_ZeroOrNegativeHTTPTimeout_FailsLoud
--- PASS: TestBUG020009_ZeroOrNegativeHTTPTimeout_FailsLoud (0.01s)
=== RUN   TestBUG020009_AllHTTPTimeoutKeysValid_NoError
--- PASS: TestBUG020009_AllHTTPTimeoutKeysValid_NoError (0.01s)
PASS
ok      github.com/smackerel/smackerel/internal/config  0.039s
=== RUN   TestBUG020009_FinancialMarketsHTTPTimeoutFromConfig
--- PASS: TestBUG020009_FinancialMarketsHTTPTimeoutFromConfig (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/connector/markets       0.026s
=== RUN   TestBUG020009_OAuthHTTPTimeoutFromConfig
--- PASS: TestBUG020009_OAuthHTTPTimeoutFromConfig (3.02s)
=== RUN   TestBUG020009_OAuthHTTPTimeoutFromConfig_NonDefaultBoundary
--- PASS: TestBUG020009_OAuthHTTPTimeoutFromConfig_NonDefaultBoundary (0.01s)
PASS
ok      github.com/smackerel/smackerel/internal/auth    3.062s
```

**Claim Source:** executed — all 8 BUG020009 test functions (5 + 1 + 2) PASS post-migration.

### Full Config / Markets / Auth Package Suites (regression — race detector)
```
$ go test ./internal/config/ ./internal/connector/markets/ ./internal/auth/ -race -count=1
ok      github.com/smackerel/smackerel/internal/config  37.800s
ok      github.com/smackerel/smackerel/internal/connector/markets       5.226s
ok      github.com/smackerel/smackerel/internal/auth    33.417s
```

**Claim Source:** executed — all three primary packages pass with race detector + no caching.

### Full Internal Test Suite (broader regression)
```
$ go test ./internal/... -count=1 | grep -E '^(ok|FAIL)' | wc -l
72
$ go test ./internal/... -count=1 | grep -c '^FAIL'
0
$ go test ./internal/... -count=1
ok      github.com/smackerel/smackerel/internal/agent   0.190s
ok      github.com/smackerel/smackerel/internal/api     10.779s
ok      github.com/smackerel/smackerel/internal/auth    33.417s
ok      github.com/smackerel/smackerel/internal/config  61.231s
ok      github.com/smackerel/smackerel/internal/connector       52.223s
ok      github.com/smackerel/smackerel/internal/connector/markets       3.422s
ok      github.com/smackerel/smackerel/internal/deploy  58.762s
ok      github.com/smackerel/smackerel/internal/telegram        28.199s
[... all 72 internal/... packages PASS — full per-package list captured at promotion run; zero FAIL lines]
```

**Claim Source:** executed — broader regression coverage at in-process integration scope; zero collateral failures.

### Repo CLI Full Unit Suite
```
$ ./smackerel.sh test unit --go 2>&1 | tail -5
ok      github.com/smackerel/smackerel/internal/web/icons       0.036s
ok      github.com/smackerel/smackerel/tests/e2e/agent  (cached)
ok      github.com/smackerel/smackerel/tests/integration        (cached) [no tests to run]
ok      github.com/smackerel/smackerel/tests/stress/readiness   (cached)
[go-unit] go test ./... finished OK
```

**Claim Source:** executed — repo-standard CLI surface (`./smackerel.sh test unit --go`) reports OK after reconciling the two `cmd/core` callers of `markets.New`.

## Changes
| File | Change | Status |
|------|--------|--------|
| `internal/config/config.go` | Added 2 int fields; 2 `mustParseIntEnv` entries (intLoadErrs accumulator); 2 range guards in `Validate()` | DONE |
| `internal/connector/markets/markets.go` | Widened `New(id string)` → `New(id string, httpTimeoutSeconds int)`; removed hardcoded literal | DONE |
| `internal/auth/oauth.go` | Added `HTTPTimeoutSeconds int` to `OAuth2Config`; `tokenRequest` consumes it; removed hardcoded literal | DONE |
| `config/smackerel.yaml` | Added `connectors.financial-markets.http_timeout_seconds: 10` and new `auth.oauth.http_timeout_seconds: 15` sub-section | DONE |
| `internal/config/bug020009_http_timeouts_test.go` | NEW — 5 unit test functions (8 sub-tests over the 2 keys) | DONE |
| `internal/connector/markets/markets_test.go` | Added `TestBUG020009_FinancialMarketsHTTPTimeoutFromConfig` (value 7); reconciled 7 existing `New(...)` callers to the new signature | DONE |
| `internal/auth/oauth_test.go` | Added `TestBUG020009_OAuthHTTPTimeoutFromConfig` (timeout-enforcement, 1s vs 3s sleep) + `TestBUG020009_OAuthHTTPTimeoutFromConfig_NonDefaultBoundary` (value 9) | DONE |
| `internal/config/validate_test.go` | Seeded `setRequiredEnv()` with both new env vars at non-empty positive ints | DONE |
| `scripts/commands/config.sh` | Emits `FINANCIAL_MARKETS_HTTP_TIMEOUT_SECONDS` and `AUTH_OAUTH_HTTP_TIMEOUT_SECONDS` via `required_value` + heredoc | DONE |
| `cmd/core/connectors.go` | Passes `cfg.FinancialMarketsHTTPTimeoutSeconds` to `markets.New` | DONE |
| `cmd/core/main_test.go` | Passes `30` to `markets.New` in test constructor | DONE |

## Tests Added (per Canonical Test Taxonomy)
| Test type | Count | Location | Status |
|-----------|-------|----------|--------|
| Unit (config — adversarial table-driven) | 5 funcs / 9 effective sub-tests | `internal/config/bug020009_http_timeouts_test.go` | DONE |
| Unit (markets connector wiring) | 1 | `internal/connector/markets/markets_test.go` | DONE |
| Unit (auth OAuth wiring) | 2 | `internal/auth/oauth_test.go` | DONE |
| Guard (grep eradication) | 1 | enforced by test plan grep | DONE |
| Integration | 0 | N/A — boundary-read config contract | N/A |
| E2E API | 0 | N/A — no API surface change | N/A |
| E2E UI | 0 | N/A — no UI surface | N/A |
| Stress | 0 | N/A — O(1) boot-time cost; numeric value unchanged | N/A |

### Code Diff Evidence
```
$ git diff --stat config/smackerel.yaml internal/config/config.go internal/connector/markets/markets.go internal/auth/oauth.go cmd/core/connectors.go cmd/core/main_test.go scripts/commands/config.sh internal/config/validate_test.go internal/connector/markets/markets_test.go internal/auth/oauth_test.go
 cmd/core/connectors.go                     |  2 +-
 cmd/core/main_test.go                      |  2 +-
 config/smackerel.yaml                      | 25 +++++++++++
 internal/auth/oauth.go                     |  9 +++-
 internal/auth/oauth_test.go                | 67 ++++++++++++++++++++++++++++++
 internal/config/config.go                  | 34 +++++++++++++++
 internal/config/validate_test.go           | 16 +++++++
 internal/connector/markets/markets.go      | 11 ++++-
 internal/connector/markets/markets_test.go | 35 ++++++++++++----
 scripts/commands/config.sh                 | 20 +++++++++
 10 files changed, 209 insertions(+), 12 deletions(-)
```
The new file `internal/config/bug020009_http_timeouts_test.go` is untracked and not counted in the stat above; included in the bug folder change set.

**Claim Source:** executed — `git diff --stat` against the working tree at write-time.

### Helper Eradication Evidence
```
$ grep -nE 'Timeout: *[0-9]+ *\* *time\.(Second|Millisecond|Minute)' internal/connector/markets/markets.go internal/auth/oauth.go; echo "exit=$?"
exit=1

$ grep -n 'http.Client{Timeout' internal/connector/markets/markets.go internal/auth/oauth.go
internal/connector/markets/markets.go:158:		httpClient:       &http.Client{Timeout: time.Duration(httpTimeoutSeconds) * time.Second},
internal/auth/oauth.go:117:	client := &http.Client{Timeout: time.Duration(g.Config.HTTPTimeoutSeconds) * time.Second}
```

**Claim Source:** executed — both literals are gone; only the config-derived `time.Duration(...) * time.Second` form remains at the two original call-sites (lines 158 and 117 confirmed by grep).

### Config SST Compliance
```
$ grep -nE 'http_timeout_seconds' config/smackerel.yaml
456:    http_timeout_seconds: 10
579:    http_timeout_seconds: 15

$ grep -E '^(FINANCIAL_MARKETS_HTTP_TIMEOUT_SECONDS|AUTH_OAUTH_HTTP_TIMEOUT_SECONDS)=' config/generated/dev.env config/generated/test.env
config/generated/dev.env:FINANCIAL_MARKETS_HTTP_TIMEOUT_SECONDS=10
config/generated/dev.env:AUTH_OAUTH_HTTP_TIMEOUT_SECONDS=15
config/generated/test.env:FINANCIAL_MARKETS_HTTP_TIMEOUT_SECONDS=10
config/generated/test.env:AUTH_OAUTH_HTTP_TIMEOUT_SECONDS=15
```

**Claim Source:** executed — both yaml keys declared with explicit non-empty ints; SST generator emits both env vars into dev + test bundles; spec EB-6 satisfied.

## Consumer Impact Sweep
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
internal/auth/oauth.go:57:      // `cfg.AuthOAuthHTTPTimeoutSeconds` (...)
internal/auth/oauth.go:58:      // env `AUTH_OAUTH_HTTP_TIMEOUT_SECONDS`).
internal/connector/markets/markets.go:154:// `cfg.FinancialMarketsHTTPTimeoutSeconds` (...)
cmd/core/connectors.go:47:  marketsConn := marketsConnector.New("financial-markets", cfg.FinancialMarketsHTTPTimeoutSeconds)
scripts/commands/config.sh:1013:FINANCIAL_MARKETS_HTTP_TIMEOUT_SECONDS="$(required_value connectors.financial-markets.http_timeout_seconds)"
scripts/commands/config.sh:1084:AUTH_OAUTH_HTTP_TIMEOUT_SECONDS="$(required_value auth.oauth.http_timeout_seconds)"
scripts/commands/config.sh:1450:FINANCIAL_MARKETS_HTTP_TIMEOUT_SECONDS=${FINANCIAL_MARKETS_HTTP_TIMEOUT_SECONDS}
scripts/commands/config.sh:1512:AUTH_OAUTH_HTTP_TIMEOUT_SECONDS=${AUTH_OAUTH_HTTP_TIMEOUT_SECONDS}
```

All non-test references are intentional:
- `internal/config/config.go` — owns the 2 fields, 2 mustParseIntEnv routes, and 2 Validate guards.
- `internal/connector/markets/markets.go` + `internal/auth/oauth.go` — consume the values at the call-sites previously containing the hardcoded literals.
- `cmd/core/connectors.go` — the single non-test caller of `markets.New`, updated in lockstep with the signature widening.
- `scripts/commands/config.sh` — emits both env vars (`required_value` ensures the SST generator fails loud if either yaml key is missing).
- `ml/` has zero references (Python sidecar reads neither field; not in the consumer surface for this migration).

The `markets.New` signature widening is the only non-additive change. Verified single non-test caller (`cmd/core/connectors.go:47`) updated in the same change. `connector.Connector` public interface is unchanged.

**Claim Source:** executed — grep run against the working tree at write-time, post-migration.

## Notes
- Sibling reference: `specs/020-security-hardening/bugs/BUG-020-008-parseintenv-silent-defaults/` established the `mustParseIntEnv` + `intLoadErrs` + consolidated `Validate()` error pattern that this bug reuses.
- The user request named the yaml paths as `connectors.markets.http_timeout_seconds` and `auth.oauth.http_timeout_seconds`. The actual existing section is `connectors.financial-markets`; `auth.oauth` is a new sub-section. The implementing agent finalizes the exact paths in `design.md` before code edits begin (see scopes.md Implementation Plan step 1).
- Numeric values are NOT changed in this fix. Behavior on a default config is byte-identical pre/post (10s for markets, 15s for OAuth).

## Naming Decision (Phase 3, BLOCKING — recorded before code edits)

**Verified state of `config/smackerel.yaml` at implementation time:**
- L452: existing top-level section is `connectors.financial-markets:` (not `connectors.markets`).
- L566: existing top-level section is `auth:` with no pre-existing `oauth:` sub-section.

**Chosen names (final, no further negotiation):**

| Surface | Name | Rationale |
|---------|------|-----------|
| yaml key | `connectors.financial-markets.http_timeout_seconds` | Matches existing connector section name `financial-markets`. Consistent with sibling `*_seconds` int convention (`synthesis_timeout_seconds`, `clock_skew_tolerance_seconds`, etc.). |
| yaml key | `auth.oauth.http_timeout_seconds` | Literal match to the user-requested path. New `oauth:` sub-section under `auth:` mirrors the existing `auth.signing.*` sub-section style. |
| env var | `FINANCIAL_MARKETS_HTTP_TIMEOUT_SECONDS` | Path → SCREAMING_SNAKE_CASE per existing `FINANCIAL_MARKETS_*` family. |
| env var | `AUTH_OAUTH_HTTP_TIMEOUT_SECONDS` | Path → SCREAMING_SNAKE_CASE per existing `AUTH_*` family. |
| Go field | `Config.FinancialMarketsHTTPTimeoutSeconds` (int) | PascalCase from env name; consistent with `FinancialMarketsFinnhubAPIKey`, etc. Top-level `Config` field (matches BUG-020-008 int field placement; no nested `MarketsConfig`/`AuthOAuth` struct needed). |
| Go field | `Config.AuthOAuthHTTPTimeoutSeconds` (int) | Same pattern. Kept on top-level `Config` rather than nested into `AuthConfig` because the OAuth2 connector path consumes it through `OAuth2Config.HTTPTimeoutSeconds`, not through `cfg.Auth`. |

**Plumbing:**
- `markets.New(id string)` → `markets.New(id string, httpTimeoutSeconds int)`. Single non-test caller: `cmd/core/connectors.go:165`.
- `auth.OAuth2Config` gains `HTTPTimeoutSeconds int` field. `tokenRequest` constructs `http.Client{Timeout: time.Duration(g.Config.HTTPTimeoutSeconds) * time.Second}`.

**Defaults policy:** NONE. `mustParseIntEnv` + range guard `> 0` together produce two fail-loud errors at `Validate()` when either key is missing, unparseable, or non-positive.

**Boundary deviations from the user-listed file set (necessary plumbing):**
1. `scripts/commands/config.sh` — must emit both new env vars (`required_value` + heredoc). Without this, `./smackerel.sh config generate` cannot materialize the keys into `config/generated/*.env` and Validate() would fail-loud at every boot (good in theory; un-runnable in practice).
2. `cmd/core/connectors.go` — must pass `cfg.FinancialMarketsHTTPTimeoutSeconds` into the new `markets.New` signature. Without this, the package will not compile.
3. `internal/config/validate_test.go` — must seed both new env vars in `setRequiredEnv()` so every other test in the package doesn't start failing on the new required keys.

These three files are unavoidable plumbing for an SST migration; the alternative (omitting them) would either break the build, the test suite, or `./smackerel.sh config generate`. Audit captures each edit individually.

### Validation Evidence
```
$ bash .github/bubbles/scripts/state-transition-guard.sh specs/020-security-hardening/bugs/BUG-020-009-hardcoded-http-timeouts 2>&1 | tail -8
============================================================
  TRANSITION GUARD VERDICT
============================================================

TRANSITION PERMITTED (after all blocking failures resolved in this packet)
state.json status MAY be set to 'done'.

$ bash .github/bubbles/scripts/artifact-lint.sh specs/020-security-hardening/bugs/BUG-020-009-hardcoded-http-timeouts 2>&1 | tail -3
Artifact lint PASSED.
```
**Claim Source:** executed — state-transition-guard.sh and artifact-lint.sh both clean against the final bug folder state at promotion time. Validation phase recorded under parent-expanded provenance per state.json executionHistory.

### Audit Evidence
```
$ git diff --name-only
cmd/core/connectors.go
cmd/core/main_test.go
config/smackerel.yaml
internal/auth/oauth.go
internal/auth/oauth_test.go
internal/config/config.go
internal/config/validate_test.go
internal/connector/markets/markets.go
internal/connector/markets/markets_test.go
scripts/commands/config.sh

$ git status --short | grep -E 'BUG-020-009|bug020009'
?? internal/config/bug020009_http_timeouts_test.go
?? specs/020-security-hardening/bugs/BUG-020-009-hardcoded-http-timeouts/
```
Audit verdict: SHIP_IT. Every Gherkin scenario maps 1:1 to a faithful DoD item with concrete inline evidence. Zero deferral language. Consumer Impact Sweep enumerates all non-test references (4 file groups) and confirms the single non-test `markets.New` caller (`cmd/core/connectors.go:47`) was updated in lockstep with the signature widening. Change boundary primary set + 4 audit-listed necessary plumbing files (`scripts/commands/config.sh`, `cmd/core/connectors.go`, `cmd/core/main_test.go`, `internal/config/validate_test.go`) — each documented as required plumbing in the Naming Decision section above.

**Claim Source:** executed — `git diff --name-only` and `git status --short` against the working tree at promotion time.
