# Report: BUG-004 — Learning Path Missing Time Estimation

## Discovery
- **Found by:** `bubbles.gaps` during stochastic quality sweep
- **Date:** April 22, 2026
- **Method:** Compared design R-502 data flow step 5 against `learning.go` implementation

## Evidence
- Design R-502 step 5: "Estimate total time: articles: word_count / 200 wpm, videos: duration from metadata"
- `internal/intelligence/learning.go` `LearningResource` struct: no time field
- `internal/intelligence/learning.go` `LearningPath` struct: only `TotalCount`/`CompletedCount`, no time

## Summary

`LearningResource` and `LearningPath` in `internal/intelligence/learning.go` lack the time-estimation fields required by design R-502 step 5. This bug remains in_progress; no implementation has been verified in this artifact pass.

## Completion Statement

Status: in_progress. The fix is not yet verified in code; closure deferred until `EstimatedMinutes`, `TotalMinutes`, and `RemainingMinutes` are added with the documented estimation rules and unit tests are captured passing in this report.

## Test Evidence

No new test execution was performed during this artifact-cleanup pass. Captured `go test ./internal/intelligence/...` output showing the time-estimation tests passing is required before any DoD item is re-checked and before this bug is promoted out of `in_progress`.
