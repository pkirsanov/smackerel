# Scopes: BUG-006-006 — Learning Path Video Duration Never Parsed

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md)

## Scope 01: Parse ISO 8601 Video Duration in Learning Path Estimation

**Status:** Done
**Priority:** P2
**Depends On:** [BUG-004](../BUG-004-learning-path-time-estimation/spec.md) (added the estimator this scope corrects)

### Change Boundary

This is a focused runtime fix confined to spec 006, Scope 02.

- **Allowed files:**
  - `internal/intelligence/learning.go` — duration parsing only.
  - `internal/intelligence/learning_test.go` — regression coverage.
  - `specs/006-phase5-advanced/bugs/BUG-006-006-video-duration-iso8601/**` — this bug packet.
- **Excluded surfaces (must remain untouched):**
  - `internal/connector/youtube/**` — the connector format is correct; do not change it.
  - All other `internal/**`, `cmd/**`, `ml/**`, `web/**`, `config/**`, `.github/**`, `.specify/**`.

### Test Plan

| # | Test | Type | File | Scenario |
|---|------|------|------|----------|
| 1 | ISO 8601 durations (`PT45M`, `PT1H30M`, `PT2H5M30S`, …) yield the true minutes, never the 15-min default | Unit (adversarial) | internal/intelligence/learning_test.go::TestEstimateReadingTime_YouTubeISO8601 | SCN-006-006c |
| 2 | Bare integer seconds (`"600"`) still yields 10 minutes | Unit | internal/intelligence/learning_test.go::TestEstimateReadingTime_YouTubePlainSecondsStillWorks | SCN-006-006d |
| 3 | Empty / `"PT"` / junk duration falls back to the 15-min default | Unit | internal/intelligence/learning_test.go::TestEstimateReadingTime_YouTubeUnparseableDuration | SCN-006-006e |
| 4 | Pre-existing estimator cases (Article/PDF/YouTube-no-duration/rounds-up) remain green | Unit (regression) | internal/intelligence/learning_test.go::TestEstimateReadingTime_* | — |

This scope is **scenario-first TDD compliant** (Gate G060): the adversarial
test was written first and captured RED against the unfixed code, then GREEN
after the fix — both captures are in [report.md](report.md).

### Definition of Done

- [x] Root cause confirmed: `estimateReadingTime` parsed duration as bare seconds while the YouTube connector stores ISO 8601
  > Evidence: producer `internal/connector/youtube/youtube.go:136` (`"duration": vid.Duration`) + fixtures `internal/connector/youtube/youtube_test.go:43,310,377` (`"PT45M"`/`"PT10M"`/`"PT5M"`); consumer pre-fix branch reproduced RED in [report.md](report.md) "Before Fix".
- [x] ISO 8601 durations are parsed to the correct minute count (no silent 15-min default)
  > Evidence: `internal/intelligence/learning.go::parseVideoDurationSeconds` (ISO 8601 regexp `videoDurationISO8601RE`) wired into the YouTube branch of `estimateReadingTime`; `TestEstimateReadingTime_YouTubeISO8601` PASS in [report.md](report.md) "After Fix".
- [x] Bare integer-seconds form still supported (no regression of the legacy path)
  > Evidence: `parseVideoDurationSeconds` `strconv.Atoi` fallback; `TestEstimateReadingTime_YouTubePlainSecondsStillWorks` PASS and the pre-existing `TestEstimateReadingTime_YouTube` ("600") still PASS in [report.md](report.md).
- [x] Malformed/empty duration falls back to the 15-minute default without panic or zero-length estimate
  > Evidence: `parseVideoDurationSeconds` returns `(0,false)` for `"PT"`/`"garbage"`/`"P1Y"`/`"--"`/`"PTM"`; `TestEstimateReadingTime_YouTubeUnparseableDuration` PASS in [report.md](report.md).
- [x] Adversarial regression test present that fails if the bug is reintroduced
  > Evidence: `TestEstimateReadingTime_YouTubeISO8601` asserts `PT45M`→45 etc.; captured RED before fix (all cases returned 15) and GREEN after — see [report.md](report.md) Before/After Fix.
- [x] SCN-006-006c / SCN-006-006d / SCN-006-006e covered by unit tests
  > Evidence: Test Plan rows 1–3 above; all PASS in [report.md](report.md) "After Fix".
- [x] No neighboring intelligence test regressed
  > Evidence: full `internal/intelligence` package PASS (`ok … internal/intelligence 0.094s`) and full repo `./smackerel.sh test unit --go` finished OK — see [report.md](report.md) Regression Evidence.
- [x] Spec-scoped artifact-lint and traceability-guard clean for the delta
  > Evidence: [report.md](report.md) Validation Evidence section.
