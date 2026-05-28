# Report: [BUG-020-008] `parseIntEnv` silent SST defaults

## Summary
Code-review finding H-1 (P0 SST NO-DEFAULTS violation). `internal/config/config.go` defined `parseIntEnv(key, defaultVal int) int` (L1777–1789) which silently substituted `defaultVal` on empty OR unparseable env input. 8 call-sites passed `0` as the silent fallback for SST-required int values. Fix: replaced with fail-loud `mustParseIntEnv(key) (int, error)`; the 8 call-sites are populated after the cfg literal via an error-accumulating loop; the accumulated errors are folded into the consolidated `Validate()` missing-keys error (same pattern as `requiredVars()`).

## Completion Statement
Implementation complete. Workflow mode `bugfix-fastlane` (ceiling `done`; TDD required). RED captured, GREEN confirmed, full `./internal/...` test suite passes. Change boundary: `internal/config/config.go` + `internal/config/mustparseintenv_test.go` (new) + `internal/config/validate_test.go` (reconciled to new contract) + bug folder artifacts only.

## Bug Reproduction — Before Fix
```
$ sed -n '1777,1789p' internal/config/config.go
// parseIntEnv reads an env var as an int, returning defaultVal when empty or unparseable.
func parseIntEnv(key string, defaultVal int) int {
	s := os.Getenv(key)
	if s == "" {
		return defaultVal
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return defaultVal
	}
	return v
}

$ grep -nE 'parseIntEnv\("[A-Z_]+", 0\)' internal/config/config.go
475:		BookmarksMinURLLength:                        parseIntEnv("BOOKMARKS_MIN_URL_LENGTH", 0),
481:		BrowserHistoryInitialLookbackDays:            parseIntEnv("BROWSER_HISTORY_INITIAL_LOOKBACK_DAYS", 0),
486:		BrowserHistoryRepeatVisitThreshold:           parseIntEnv("BROWSER_HISTORY_REPEAT_VISIT_THRESHOLD", 0),
488:		BrowserHistoryContentFetchConcurrency:        parseIntEnv("BROWSER_HISTORY_CONTENT_FETCH_CONCURRENCY", 0),
568:		QFDecisionsPacketVersion:                     parseIntEnv("QF_DECISIONS_PACKET_VERSION", 0),
569:		QFDecisionsPageSize:                          parseIntEnv("QF_DECISIONS_PAGE_SIZE", 0),
576:		HospitableInitialLookbackDays:                parseIntEnv("HOSPITABLE_INITIAL_LOOKBACK_DAYS", 0),
577:		HospitablePageSize:                           parseIntEnv("HOSPITABLE_PAGE_SIZE", 0),
```
Helper present and 8 call-sites passed `0` as silent fallback for required SST int values.

**Claim Source:** interpreted — original line numbers/content transcribed from `read_file` of `internal/config/config.go` at the pre-fix HEAD.

## Test Evidence

### Pre-Fix Regression (RED)
```
$ go test ./internal/config/ -run 'TestBUG020008' -count=1 -v
=== RUN   TestBUG020008_MissingSingleIntKey_FailsLoud
=== RUN   TestBUG020008_MissingSingleIntKey_FailsLoud/BOOKMARKS_MIN_URL_LENGTH
    mustparseintenv_test.go:42: expected error for missing BOOKMARKS_MIN_URL_LENGTH, got nil (silent default to 0 is the bug)
=== RUN   TestBUG020008_MissingSingleIntKey_FailsLoud/BROWSER_HISTORY_INITIAL_LOOKBACK_DAYS
    mustparseintenv_test.go:42: expected error for missing BROWSER_HISTORY_INITIAL_LOOKBACK_DAYS, got nil (silent default to 0 is the bug)
=== RUN   TestBUG020008_MissingSingleIntKey_FailsLoud/BROWSER_HISTORY_REPEAT_VISIT_THRESHOLD
    mustparseintenv_test.go:42: expected error for missing BROWSER_HISTORY_REPEAT_VISIT_THRESHOLD, got nil (silent default to 0 is the bug)
=== RUN   TestBUG020008_MissingSingleIntKey_FailsLoud/BROWSER_HISTORY_CONTENT_FETCH_CONCURRENCY
    mustparseintenv_test.go:42: expected error for missing BROWSER_HISTORY_CONTENT_FETCH_CONCURRENCY, got nil (silent default to 0 is the bug)
=== RUN   TestBUG020008_MissingSingleIntKey_FailsLoud/QF_DECISIONS_PACKET_VERSION
    mustparseintenv_test.go:42: expected error for missing QF_DECISIONS_PACKET_VERSION, got nil (silent default to 0 is the bug)
=== RUN   TestBUG020008_MissingSingleIntKey_FailsLoud/QF_DECISIONS_PAGE_SIZE
    mustparseintenv_test.go:42: expected error for missing QF_DECISIONS_PAGE_SIZE, got nil (silent default to 0 is the bug)
=== RUN   TestBUG020008_MissingSingleIntKey_FailsLoud/HOSPITABLE_INITIAL_LOOKBACK_DAYS
    mustparseintenv_test.go:42: expected error for missing HOSPITABLE_INITIAL_LOOKBACK_DAYS, got nil (silent default to 0 is the bug)
=== RUN   TestBUG020008_MissingSingleIntKey_FailsLoud/HOSPITABLE_PAGE_SIZE
    mustparseintenv_test.go:42: expected error for missing HOSPITABLE_PAGE_SIZE, got nil (silent default to 0 is the bug)
--- FAIL: TestBUG020008_MissingSingleIntKey_FailsLoud (0.02s)
=== RUN   TestBUG020008_MissingAllIntKeys_ConsolidatedError
    mustparseintenv_test.go:61: expected consolidated error for all 8 missing int keys, got nil
--- FAIL: TestBUG020008_MissingAllIntKeys_ConsolidatedError (0.00s)
=== RUN   TestBUG020008_UnparseableIntKey_FailsLoud
    [8/8 sub-tests FAIL with: expected error for unparseable <KEY>=abc, got nil]
--- FAIL: TestBUG020008_UnparseableIntKey_FailsLoud (0.02s)
=== RUN   TestBUG020008_AllIntKeysValid_NoError
--- PASS: TestBUG020008_AllIntKeysValid_NoError (0.00s)
FAIL
FAIL    github.com/smackerel/smackerel/internal/config  0.050s
FAIL
```
**Claim Source:** executed — captured from `go test ./internal/config/ -run 'TestBUG020008' -count=1 -v` against the pre-fix tree (test file present, helper migration NOT yet applied). 17 of 18 sub-tests FAIL as expected; only the AllValid sanity case passes (the 8 keys were seeded valid in `setRequiredEnv` so parseIntEnv silently returned the seeded values). This is the adversarial RED proof: every assertion would re-fail if the helper migration were reverted.

### Post-Fix Regression (GREEN)
```
$ go test ./internal/config/ -run 'TestBUG020008' -count=1 -v
=== RUN   TestBUG020008_MissingSingleIntKey_FailsLoud
=== RUN   TestBUG020008_MissingSingleIntKey_FailsLoud/BOOKMARKS_MIN_URL_LENGTH
=== RUN   TestBUG020008_MissingSingleIntKey_FailsLoud/BROWSER_HISTORY_INITIAL_LOOKBACK_DAYS
=== RUN   TestBUG020008_MissingSingleIntKey_FailsLoud/BROWSER_HISTORY_REPEAT_VISIT_THRESHOLD
=== RUN   TestBUG020008_MissingSingleIntKey_FailsLoud/BROWSER_HISTORY_CONTENT_FETCH_CONCURRENCY
=== RUN   TestBUG020008_MissingSingleIntKey_FailsLoud/QF_DECISIONS_PACKET_VERSION
=== RUN   TestBUG020008_MissingSingleIntKey_FailsLoud/QF_DECISIONS_PAGE_SIZE
=== RUN   TestBUG020008_MissingSingleIntKey_FailsLoud/HOSPITABLE_INITIAL_LOOKBACK_DAYS
=== RUN   TestBUG020008_MissingSingleIntKey_FailsLoud/HOSPITABLE_PAGE_SIZE
--- PASS: TestBUG020008_MissingSingleIntKey_FailsLoud (0.02s)
=== RUN   TestBUG020008_MissingAllIntKeys_ConsolidatedError
--- PASS: TestBUG020008_MissingAllIntKeys_ConsolidatedError (0.00s)
=== RUN   TestBUG020008_UnparseableIntKey_FailsLoud
=== RUN   TestBUG020008_UnparseableIntKey_FailsLoud/BOOKMARKS_MIN_URL_LENGTH
=== RUN   TestBUG020008_UnparseableIntKey_FailsLoud/BROWSER_HISTORY_INITIAL_LOOKBACK_DAYS
=== RUN   TestBUG020008_UnparseableIntKey_FailsLoud/BROWSER_HISTORY_REPEAT_VISIT_THRESHOLD
=== RUN   TestBUG020008_UnparseableIntKey_FailsLoud/BROWSER_HISTORY_CONTENT_FETCH_CONCURRENCY
=== RUN   TestBUG020008_UnparseableIntKey_FailsLoud/QF_DECISIONS_PACKET_VERSION
=== RUN   TestBUG020008_UnparseableIntKey_FailsLoud/QF_DECISIONS_PAGE_SIZE
=== RUN   TestBUG020008_UnparseableIntKey_FailsLoud/HOSPITABLE_INITIAL_LOOKBACK_DAYS
=== RUN   TestBUG020008_UnparseableIntKey_FailsLoud/HOSPITABLE_PAGE_SIZE
--- PASS: TestBUG020008_UnparseableIntKey_FailsLoud (0.02s)
=== RUN   TestBUG020008_AllIntKeysValid_NoError
--- PASS: TestBUG020008_AllIntKeysValid_NoError (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/config  0.042s
```
**Claim Source:** executed — captured from `go test ./internal/config/ -run 'TestBUG020008' -count=1 -v` after the helper migration. All 18 sub-tests PASS.

### Full Config Package Suite (regression — race detector)
```
$ go test ./internal/config/ -race -count=1
ok      github.com/smackerel/smackerel/internal/config  28.929s
$ echo "race-detector exit code: $?"
race-detector exit code: 0
```
The race-detector run covers the full `internal/config/` package including:
- the new `TestBUG020008_*` adversarial regression tests (18 sub-tests)
- the 5 pre-existing tests reconciled to the new fail-loud contract
- every other test in the config package (Validate, Drive, Photos, Recommendations, SST loader, secret-keys, runtime sentinel-marker check, etc.)
**Claim Source:** executed — captured after reconciling 5 pre-existing tests (`TestValidate_QFDecisionsDisabledAllowsEmptyValues`, `TestValidate_QFDecisionsEnabledRequiresExplicitValues`, `TestLoad_BookmarksMinURLLength_MissingEnv`, `TestLoad_HospitableEnabled_MissingLookbackDays_Fails`, `TestLoad_HospitableEnabled_MissingPageSize_Fails`) to the new fail-loud contract. The 5 reconciled tests previously encoded silent-default behavior (which WAS the bug); their new bodies assert the fail-loud contract. Race detector exercised the loader concurrency.

### Full Internal Test Suite (broader regression)
```
$ go test ./internal/... -count=1
ok      github.com/smackerel/smackerel/internal/agent   0.146s
ok      github.com/smackerel/smackerel/internal/agent/render    0.063s
ok      github.com/smackerel/smackerel/internal/agent/userreply 0.035s
ok      github.com/smackerel/smackerel/internal/annotation      0.152s
ok      github.com/smackerel/smackerel/internal/api     10.619s
ok      github.com/smackerel/smackerel/internal/auth    15.369s
ok      github.com/smackerel/smackerel/internal/auth/revocation 0.016s
ok      github.com/smackerel/smackerel/internal/backup  0.030s
ok      github.com/smackerel/smackerel/internal/config  37.309s
ok      github.com/smackerel/smackerel/internal/connector       47.750s
ok      github.com/smackerel/smackerel/internal/connector/alerts        3.179s
ok      github.com/smackerel/smackerel/internal/connector/bookmarks     0.136s
ok      github.com/smackerel/smackerel/internal/connector/browser       0.096s
ok      github.com/smackerel/smackerel/internal/connector/caldav        0.049s
ok      github.com/smackerel/smackerel/internal/connector/discord       9.888s
ok      github.com/smackerel/smackerel/internal/connector/guesthost     0.494s
ok      github.com/smackerel/smackerel/internal/connector/hospitable    14.469s
ok      github.com/smackerel/smackerel/internal/connector/imap  0.123s
ok      github.com/smackerel/smackerel/internal/connector/keep  0.250s
ok      github.com/smackerel/smackerel/internal/connector/maps  0.288s
ok      github.com/smackerel/smackerel/internal/connector/markets       2.640s
ok      github.com/smackerel/smackerel/internal/connector/photos        0.027s
ok      github.com/smackerel/smackerel/internal/connector/photos/adapters/immich        0.118s
ok      github.com/smackerel/smackerel/internal/connector/photos/adapters/photoprism    0.141s
ok      github.com/smackerel/smackerel/internal/connector/qfdecisions   1.694s
ok      github.com/smackerel/smackerel/internal/connector/rss   0.338s
ok      github.com/smackerel/smackerel/internal/connector/twitter       8.154s
ok      github.com/smackerel/smackerel/internal/connector/weather       36.647s
ok      github.com/smackerel/smackerel/internal/connector/youtube       0.011s
ok      github.com/smackerel/smackerel/internal/db      0.057s
ok      github.com/smackerel/smackerel/internal/deploy  33.800s
ok      github.com/smackerel/smackerel/internal/digest  1.340s
ok      github.com/smackerel/smackerel/internal/domain  0.058s
ok      github.com/smackerel/smackerel/internal/drive   0.020s
ok      github.com/smackerel/smackerel/internal/drive/confirm   0.010s
ok      github.com/smackerel/smackerel/internal/drive/consumers 0.027s
ok      github.com/smackerel/smackerel/internal/drive/google    0.413s
ok      github.com/smackerel/smackerel/internal/drive/health    0.017s
ok      github.com/smackerel/smackerel/internal/drive/monitor   0.015s
ok      github.com/smackerel/smackerel/internal/drive/policy    0.014s
ok      github.com/smackerel/smackerel/internal/drive/retrieve  0.021s
ok      github.com/smackerel/smackerel/internal/drive/rules     0.023s
ok      github.com/smackerel/smackerel/internal/drive/save      0.018s
ok      github.com/smackerel/smackerel/internal/drive/scan      0.029s
ok      github.com/smackerel/smackerel/internal/drive/tools     0.054s
ok      github.com/smackerel/smackerel/internal/extract 0.129s
ok      github.com/smackerel/smackerel/internal/graph   0.014s
ok      github.com/smackerel/smackerel/internal/intelligence    0.042s
ok      github.com/smackerel/smackerel/internal/knowledge       0.037s
ok      github.com/smackerel/smackerel/internal/list    0.032s
ok      github.com/smackerel/smackerel/internal/mealplan        0.068s
ok      github.com/smackerel/smackerel/internal/metrics 0.047s
ok      github.com/smackerel/smackerel/internal/nats    4.026s
ok      github.com/smackerel/smackerel/internal/notification    0.023s
ok      github.com/smackerel/smackerel/internal/notification/source/ntfy        1.058s
ok      github.com/smackerel/smackerel/internal/pipeline        0.383s
ok      github.com/smackerel/smackerel/internal/recipe  0.008s
ok      github.com/smackerel/smackerel/internal/recommendation/location 0.010s
ok      github.com/smackerel/smackerel/internal/recommendation/policy   0.015s
ok      github.com/smackerel/smackerel/internal/recommendation/provider 0.007s
ok      github.com/smackerel/smackerel/internal/recommendation/quality  0.008s
ok      github.com/smackerel/smackerel/internal/recommendation/rank     0.010s
ok      github.com/smackerel/smackerel/internal/recommendation/store    0.015s
ok      github.com/smackerel/smackerel/internal/recommendation/tools    0.062s
ok      github.com/smackerel/smackerel/internal/scheduler       5.046s
ok      github.com/smackerel/smackerel/internal/stringutil      0.007s
ok      github.com/smackerel/smackerel/internal/telegram        27.929s
ok      github.com/smackerel/smackerel/internal/telegram/render 0.040s
ok      github.com/smackerel/smackerel/internal/topics  0.009s
ok      github.com/smackerel/smackerel/internal/web     0.108s
ok      github.com/smackerel/smackerel/internal/web/icons       0.006s
```
**Claim Source:** executed — every `internal/...` test package green. No collateral regressions from the helper migration or test reconciliation.

### Build
```
$ go build ./...
$ echo "go build exit code: $?"
go build exit code: 0
$ go vet ./internal/config/
$ echo "go vet exit code: $?"
go vet exit code: 0
```
**Claim Source:** executed - full Go build clean; vet clean on the modified package.

### Helper-Eradication Grep
```
$ grep -nE 'parseIntEnv\(.*,\s*0\)' internal/config/config.go; echo "exit=$?"
exit=1
$ grep -nE 'parseIntEnv\(' internal/config/config.go; echo "exit=$?"
exit=1
$ grep -n 'func parseIntEnv' internal/config/config.go; echo "exit=$?"
exit=1
```
**Claim Source:** executed - zero matches for the silent-default helper call pattern, zero matches for any remaining `parseIntEnv(` call, and zero matches for the `func parseIntEnv` declaration. The old helper and every call-site are gone.

### Stale References (comments only)
```
$ grep -rn 'parseIntEnv' internal/ cmd/
internal/config/mustparseintenv_test.go:4:// vars previously routed through parseIntEnv(key, 0). Each MUST cause
internal/config/mustparseintenv_test.go:6:// unset or unparseable. Today (pre-fix) parseIntEnv silently substitutes
internal/config/mustparseintenv_test.go:72:// AND the offending value. Pre-fix: parseIntEnv silently returns 0 on
internal/config/validate_test.go:873:	// through the silent-default parseIntEnv helper. Defaults mirror
internal/config/config.go:1817:// previous silent-default parseIntEnv helper with this fail-loud variant
```
**Claim Source:** executed — only comment references remain (the new `mustParseIntEnv` doc-comment and a setRequiredEnv comment explaining the prior helper). No call-site references survive.

### Code Diff Evidence
```
$ git diff --stat internal/config/config.go internal/config/validate_test.go internal/config/mustparseintenv_test.go
 internal/config/config.go        | 94 +++++++++++++++++++++++++++++-----------
 internal/config/validate_test.go | 52 +++++++++++++++-------
 2 files changed, 105 insertions(+), 41 deletions(-)
```
(The new file `internal/config/mustparseintenv_test.go` is untracked and not counted in this stat; see `git status --short` evidence below.)
```
$ git status --short
 M internal/config/config.go
 M internal/config/validate_test.go
?? internal/config/mustparseintenv_test.go
?? specs/020-security-hardening/bugs/BUG-020-008-parseintenv-silent-defaults/
```
Three production-relevant files in the bug change boundary: (1) `internal/config/config.go` — helper replacement + 8 call-site migration + intLoadErrs Validate() folding. (2) `internal/config/mustparseintenv_test.go` (NEW, 4 unit tests, 8-key table-driven) — the regression contract. (3) `internal/config/validate_test.go` — 8-key seeding in `setRequiredEnv` + 5 reconciled pre-existing tests. No other production code touched.

**Claim Source:** executed — `git diff --stat` and `git status --short` captured against the working tree at write-time.

### Config SST Compliance
The 8 keys are already declared with explicit non-empty int values in `config/smackerel.yaml` (verified pre-fix):
```
$ grep -nE 'min_url_length|initial_lookback_days|repeat_visit_threshold|content_fetch_concurrency|packet_version|page_size' config/smackerel.yaml
231:    min_url_length: 10
246:      initial_lookback_days: 30
251:      repeat_visit_threshold: 3
253:      content_fetch_concurrency: 5
308:    initial_lookback_days: 90 # How far back to sync on first run
309:    page_size: 100
331:    packet_version: 1
332:    page_size: 25
```
**Claim Source:** executed — all 8 SST yaml entries present with explicit non-empty integer values, satisfying spec EB-5. No yaml mutation required.

## Consumer Impact Sweep
The migration touches one private package-internal helper (`parseIntEnv` → `mustParseIntEnv`) and 8 call-sites all inside `internal/config/config.go`. No public API surface changes.

```
$ grep -rn 'parseIntEnv' internal/ cmd/ ml/
internal/config/mustparseintenv_test.go:4:// vars previously routed through parseIntEnv(key, 0). Each MUST cause
internal/config/mustparseintenv_test.go:6:// unset or unparseable. Today (pre-fix) parseIntEnv silently substitutes
internal/config/mustparseintenv_test.go:72:// AND the offending value. Pre-fix: parseIntEnv silently returns 0 on
internal/config/validate_test.go:873:	// through the silent-default parseIntEnv helper. Defaults mirror
internal/config/config.go:1817:// previous silent-default parseIntEnv helper with this fail-loud variant
```
Zero external callers — `parseIntEnv` was lower-case (unexported) and never re-exported. The only surviving references are descriptive comments in the new test file, the loader source, and `setRequiredEnv`. No connector code, no `cmd/`, no `ml/` consumer touches the helper. The renamed helper (`mustParseIntEnv`) is likewise unexported.

Behavioral consumer impact: every code path that previously received `0` from the silent-default now receives the genuine env-set value OR causes `Load()` to return an error at boot. The 8 affected struct fields are `BookmarksMinURLLength`, `BrowserHistoryInitialLookbackDays`, `BrowserHistoryRepeatVisitThreshold`, `BrowserHistoryContentFetchConcurrency`, `QFDecisionsPacketVersion`, `QFDecisionsPageSize`, `HospitableInitialLookbackDays`, `HospitablePageSize`. Each is now guaranteed to be non-zero in any boot that reaches post-Validate code; the affected consumers (`internal/connector/bookmarks`, `internal/connector/browser`, `internal/connector/hospitable`, `internal/connector/qfdecisions`) already validate `< 1` after load — their existing `< 1` guards are now defense-in-depth.

**Claim Source:** executed — grep run against the working tree after the helper migration.
### Validation Evidence
```
$ bash .github/bubbles/scripts/state-transition-guard.sh specs/020-security-hardening/bugs/BUG-020-008-parseintenv-silent-defaults 2>&1 | tail -8
============================================================
  TRANSITION GUARD VERDICT
============================================================

TRANSITION PERMITTED (after all blocking failures resolved in this packet)
state.json status MAY be set to 'done'.

$ bash .github/bubbles/scripts/artifact-lint.sh specs/020-security-hardening/bugs/BUG-020-008-parseintenv-silent-defaults 2>&1 | tail -3
Artifact lint PASSED.
```
**Claim Source:** executed - state-transition-guard.sh and artifact-lint.sh both clean against the final bug folder state at promotion time. Validation phase under parent-expanded provenance per executionHistory entry.

### Audit Evidence
```
$ git diff --name-only
internal/config/config.go
internal/config/validate_test.go

$ git status --short
 M internal/config/config.go
 M internal/config/validate_test.go
?? internal/config/mustparseintenv_test.go
?? specs/020-security-hardening/bugs/BUG-020-008-parseintenv-silent-defaults/

$ grep -rn 'parseIntEnv\|mustParseIntEnv' internal/ cmd/ ml/ | grep -v '_test\.go' | grep -v 'config\.go'
(no output - zero non-test, non-config callers exist anywhere in the tree)

$ grep -n '^func ' internal/config/mustparseintenv_test.go
33:func TestBUG020008_MissingSingleIntKey_FailsLoud(t *testing.T) {
56:func TestBUG020008_MissingAllIntKeys_ConsolidatedError(t *testing.T) {
75:func TestBUG020008_UnparseableIntKey_FailsLoud(t *testing.T) {
95:func TestBUG020008_AllIntKeysValid_NoError(t *testing.T) {
```
Audit verdict: SHIP_IT. Change boundary confirmed contained to 3 source files plus the bug folder. Every Gherkin scenario maps 1:1 to a faithful DoD item (G068). Zero G040 hits. Consumer Impact Sweep complete. Zero foreign-owned artifacts touched. The 4 new tests are persistent in-tree adversarial regressions that mechanically lock the SST NO-DEFAULTS boundary contract for the 8 affected int env vars.

**Claim Source:** executed - audit phase under parent-expanded provenance per executionHistory entry.
## Phase Provenance Ledger
| Phase | Agent | Status | Notes |
|-------|-------|--------|-------|
| discovery | bubbles.bug | done | Code-review finding H-1 surfaced by user |
| documentation | bubbles.bug | done | bug.md + spec.md authored at packet creation |
| design | bubbles.bug (initial) plus bubbles.implement (parent-expanded refinement) | done | Initial design.md authored at packet creation; runtime had no specialist dispatch capability so the implementing agent satisfied design refinement under parent-expanded provenance |
| plan | bubbles.bug (initial) plus bubbles.implement (parent-expanded refinement) | done | Initial scopes.md authored at packet creation; refinement under parent-expanded provenance added Consumer Impact Sweep + canonical scope status + DoD evidence |
| implement | bubbles.implement | done | Helper replaced; 8 call-sites migrated; intLoadErrs folded into Validate() |
| test | bubbles.implement (parent-expanded) | done | RED+GREEN captured for the 4 new tests; full config suite + full internal/... suite green; 5 pre-existing tests reconciled to new contract |
| regression | bubbles.implement (parent-expanded) | done | Race-detector run of full config package + full internal/... — zero collateral failures |
| simplify | bubbles.implement (parent-expanded) | done | No simplification opportunity surfaced — error-accumulator loop is the minimum-viable form mirroring `requiredVars()` pattern |
| stabilize | bubbles.implement (parent-expanded) | done | All tests deterministic; no time/random/concurrency factors |
| security | bubbles.implement (parent-expanded) | done | A05 Misconfiguration — STRENGTHENED. No new attack surface. NO-DEFAULTS contract restored at the boundary |
| validate | bubbles.implement (parent-expanded) | done | state-transition-guard.sh run iteratively until TRANSITION PERMITTED |
| audit | bubbles.implement (parent-expanded) | done | DoD-to-Gherkin fidelity preserved; zero G040 hits; change-boundary confined to 3 source files + bug folder |
| docs | bubbles.implement (parent-expanded) | done | No external doc updates required — the fix is a private-helper rename plus a contract-tightening that the existing copilot-instructions Secrets Management table already mandates. No operator workflow change, no Deployment.md change, no README change |
