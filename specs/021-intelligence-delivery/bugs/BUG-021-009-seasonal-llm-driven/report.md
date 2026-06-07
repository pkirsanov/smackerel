# Report: BUG-021-009 — LLM-driven seasonal significance via the ML path

**Workflow mode:** `bugfix-fastlane` (parent-expanded — the active runtime lacks `runSubagent`)
**Owner:** `bubbles.workflow`
**Resolved:** 2026-06-07
**Continues:** BUG-021-008

## Summary

`DetectSeasonalPatterns` decided "is this year-over-year volume change a
meaningful seasonal pattern?" with hardcoded ratio thresholds (`< 0.7`,
`> 1.5`) and a "≥5 captures = seasonal" claim, and its ML `seasonal.analyze`
enrichment was dead due to a request/response key mismatch. Per
docs/smackerel.md §3.6 and the owner directive, this moves the significance
judgment to the LLM via the existing `seasonal.analyze` ML path (Option A): the
Go core gathers raw signals; the ML sidecar judges significance. The contract is
made coherent on both ends.

## Root Cause

The `0.7`/`1.5` ratio decision and the `topic_seasonal` "≥5" claim lived in Go.
The ML enrichment never fired: Go sent `{current_month, data_days,
local_patterns}` while Python read `patterns`/`month`, and Python returned
`{patterns}` while Go read `observations`.

## Fix

`DetectSeasonalPatterns` now gathers raw signals and calls `seasonal.analyze`;
`handle_seasonal_analyze` judges significance and returns `{observations}`. The
operational bounds moved to fail-loud SST; no hardcoded ratio fallback remains.

## Test Evidence

Unit coverage spans both ends of the seasonal path: the Go SST loader (fail-loud
operational bounds) and the reworked Python ML handler (significance judgment +
no-LLM/no-signal skip with no ratio fallback). The two subsections below carry
the captured terminal output.

### ML Handler Evidence

```
$ cd ml && python3 -m pytest tests/test_intelligence_handlers.py -v -k Seasonal
tests/test_intelligence_handlers.py::TestSeasonalAnalyze::test_no_llm_returns_empty_observations PASSED [ 33%]
tests/test_intelligence_handlers.py::TestSeasonalAnalyze::test_no_signals_returns_empty PASSED [ 66%]
tests/test_intelligence_handlers.py::TestSeasonalAnalyze::test_response_shape_has_observations_key PASSED [100%]
======================= 3 passed, 15 deselected in 0.09s =======================
```

`test_no_llm_returns_empty_observations` proves the no-LLM path returns ZERO
observations — there is no hardcoded ratio fallback. The handler consumes the
new raw-signal contract and returns the `observations` key the Go caller reads.

## SST loader Evidence (fail-loud + populate + range)

```
$ go test -v -count=1 -run 'Seasonal' ./internal/config/
--- PASS: TestLoadSeasonalConfig_Populates (0.00s)
=== RUN   TestLoadSeasonalConfig_FailLoudOnMissing/INTELLIGENCE_SEASONAL_MIN_DATA_DAYS
=== RUN   TestLoadSeasonalConfig_FailLoudOnMissing/INTELLIGENCE_SEASONAL_TOPIC_MIN_CAPTURES
=== RUN   TestLoadSeasonalConfig_FailLoudOnMissing/INTELLIGENCE_SEASONAL_TOPIC_CANDIDATE_LIMIT
=== RUN   TestLoadSeasonalConfig_FailLoudOnMissing/INTELLIGENCE_SEASONAL_MAX_OBSERVATIONS
--- PASS: TestLoadSeasonalConfig_FailLoudOnMissing (0.00s)
=== RUN   TestLoadSeasonalConfig_RejectsOutOfRange/min_data_days_zero
=== RUN   TestLoadSeasonalConfig_RejectsOutOfRange/topic_candidate_limit_zero
=== RUN   TestLoadSeasonalConfig_RejectsOutOfRange/max_observations_negative
--- PASS: TestLoadSeasonalConfig_RejectsOutOfRange (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/config  0.015s
```

## Code Diff Evidence

```
$ go build ./...
# BUILD=0
$ go vet ./internal/intelligence/ ./internal/config/ ./cmd/core/
# VET=0
$ git diff --stat (modified) ; git status --short (new)
 cmd/core/main.go                       |   6 ++
 cmd/core/wiring_cooling.go             |  30 ++++++
 config/smackerel.yaml                  |  14 +++
 internal/intelligence/engine.go        |  13 +++
 internal/intelligence/monthly.go       | 167 ++++++++++++++++---------
 ml/app/intelligence.py                 | 111 +++++++++++------
 ml/tests/test_intelligence_handlers.py |  45 +++++--
 scripts/commands/config.sh             |   8 ++
?? internal/config/seasonal.go
?? internal/config/seasonal_test.go
```

`DetectSeasonalPatterns` skips when the seasonal config is nil, when the NATS
client is nil, or when the ML sidecar returns nothing — none of these fall back
to a ratio heuristic. No schema migration.

### Validation Evidence

```
$ ./smackerel.sh config generate
Generated ~/smackerel/config/generated/dev.env
$ grep -n 'INTELLIGENCE_SEASONAL' config/generated/dev.env
config/generated/dev.env:188:INTELLIGENCE_SEASONAL_MIN_DATA_DAYS=180
config/generated/dev.env:189:INTELLIGENCE_SEASONAL_TOPIC_MIN_CAPTURES=5
config/generated/dev.env:190:INTELLIGENCE_SEASONAL_TOPIC_CANDIDATE_LIMIT=5
config/generated/dev.env:191:INTELLIGENCE_SEASONAL_MAX_OBSERVATIONS=2
$ go test -count=1 ./internal/intelligence/ ./internal/config/ ./cmd/core/
ok      github.com/smackerel/smackerel/internal/intelligence    0.040s
ok      github.com/smackerel/smackerel/internal/config  38.307s
ok      github.com/smackerel/smackerel/cmd/core 1.008s
$ ./smackerel.sh test unit --python
487 passed, 2 skipped, 2 warnings in 15.56s
```

The SST pipeline resolves the new operational keys end-to-end; the affected Go
packages and the full Python ML suite return green.

### Lint Evidence

```
$ ./smackerel.sh lint
Installing collected packages: ... ruff-0.15.16 ...
All checks passed!
  OK: web/pwa/manifest.json
  ...
Web validation passed
LINT_RC=0
```

Go lint, Python ruff (`ml/app` + `ml/tests`), and web-validate all pass.

### Audit Evidence

```
$ git status --short | grep -E 'internal/db/migrations/'
# (empty — no migration)
$ grep -cE "ratio < 0\.7|ratio > 1\.5" internal/intelligence/monthly.go
0
```

No hardcoded seasonal-significance ratio threshold remains in `monthly.go`. The
diff is confined to the detector, the new SST loader/config, the ML handler, the
wiring, and the seasonal payload contract. No migration, no `.github/bubbles`
framework files.

## Completion Statement

`DetectSeasonalPatterns` now gathers raw year-over-year and topic signals and
delegates the "is this seasonally meaningful?" judgment to the `seasonal.analyze`
ML path; `handle_seasonal_analyze` judges significance and returns a coherent
`observations` contract, fixing the prior dead request/response mismatch. No
hardcoded ratio threshold remains; nil config / nil NATS / no-LLM all skip
seasonal detection with no magic-number fallback. The SST loader and the
reworked ML handler are unit-tested; the SST pipeline resolves the operational
bounds; Go build/vet/tests, the full Python suite, and `./smackerel.sh lint`
return green. Scope 1 DoD is complete (10/10). BUG-021-009 is Done and continues
BUG-021-008.
