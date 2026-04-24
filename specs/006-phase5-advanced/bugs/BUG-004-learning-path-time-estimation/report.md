# Report: BUG-004 — Learning Path Missing Time Estimation

## Discovery
- **Found by:** `bubbles.gaps` during stochastic quality sweep
- **Date:** April 22, 2026
- **Method:** Compared design R-502 data flow step 5 against `learning.go` implementation

## Summary

Re-verified 2026-04-24 against committed code: `LearningResource` carries `EstimatedMinutes`, `LearningPath` carries `TotalMinutes`/`RemainingMinutes`, and `estimateReadingTime` handles articles (content length / 200 wpm), videos (duration metadata in seconds), and unknown content types (default fallback). The full set of `TestEstimateReadingTime_*` cases passes.

## Completion Statement

Status: done. R-502 step 5 is fully wired in `internal/intelligence/learning.go` and the focused regression run plus full repo-CLI unit run executed in this re-cert pass have been captured below.

## Test Evidence

Focused Go run captured 2026-04-24T07:29:44Z → 07:29:45Z:

```text
$ go test -count=1 -v -run "TestEstimateReadingTime|TestLearningPath" ./internal/intelligence/...
=== RUN   TestLearningPath_ResourcesSortedByDifficulty
--- PASS: TestLearningPath_ResourcesSortedByDifficulty (0.00s)
=== RUN   TestEstimateReadingTime_Article
--- PASS: TestEstimateReadingTime_Article (0.00s)
=== RUN   TestEstimateReadingTime_ShortArticle
--- PASS: TestEstimateReadingTime_ShortArticle (0.00s)
=== RUN   TestEstimateReadingTime_ZeroLength
--- PASS: TestEstimateReadingTime_ZeroLength (0.00s)
=== RUN   TestEstimateReadingTime_YouTube
--- PASS: TestEstimateReadingTime_YouTube (0.00s)
=== RUN   TestEstimateReadingTime_YouTubeNoDuration
--- PASS: TestEstimateReadingTime_YouTubeNoDuration (0.00s)
=== RUN   TestEstimateReadingTime_PDF
--- PASS: TestEstimateReadingTime_PDF (0.00s)
=== RUN   TestEstimateReadingTime_UnknownType
--- PASS: TestEstimateReadingTime_UnknownType (0.00s)
=== RUN   TestEstimateReadingTime_YouTubeRoundsUp
--- PASS: TestEstimateReadingTime_YouTubeRoundsUp (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/intelligence    0.022s
```

Full repo-CLI unit run captured 2026-04-24:

```text
$ ./smackerel.sh test unit
........................................................................ [ 21%]
........................................................................ [ 43%]
........................................................................ [ 65%]
........................................................................ [ 87%]
..........................................                               [100%]
330 passed, 2 warnings in 11.48s
```

### Validation Evidence

Field wiring + estimation entry points captured 2026-04-24:

```text
$ grep -nE "EstimatedMinutes|TotalMinutes|RemainingMinutes|estimateReadingTime|content_length|duration_str" internal/intelligence/learning.go
33:     EstimatedMinutes int                `json:"estimated_minutes"`
45:     TotalMinutes     int                `json:"total_minutes"`
46:     RemainingMinutes int                `json:"remaining_minutes"`
71:                             COALESCE(LENGTH(a.raw_content), 0) AS content_length,
72:                             COALESCE(a.metadata->>'duration', '') AS duration_str
81:                    content_length, duration_str
151:            estMinutes := estimateReadingTime(contentType, contentLength, durationStr)
159:                    EstimatedMinutes: estMinutes,
166:            path.TotalMinutes += estMinutes
170:                    path.RemainingMinutes += estMinutes
251:func estimateReadingTime(contentType string, contentLength int, durationStr string) int {
254:            if durationStr != "" {
257:                    if _, err := fmt.Sscanf(durationStr, "%d", &seconds); err == nil && seconds > 0 {
```

### Audit Evidence

Repo-CLI hygiene + targeted regression captured 2026-04-24T07:30:21Z → 07:30:29Z:

```text
$ ./smackerel.sh check
Config is in sync with SST
env_file drift guard: OK
$ go test -count=1 -run "Learning" ./internal/intelligence/...
ok      github.com/smackerel/smackerel/internal/intelligence    0.019s
```

SST sync clean; Learning-scoped regression replay PASS confirms no neighbouring intelligence test regressed.

