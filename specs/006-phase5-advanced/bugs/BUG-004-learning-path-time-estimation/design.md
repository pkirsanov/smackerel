# Design: BUG-004 — Learning Path Missing Time Estimation

## Fix Design

1. Add `EstimatedMinutes int` to `LearningResource` struct
2. Add `TotalMinutes int` and `RemainingMinutes int` to `LearningPath` struct
3. In `GetLearningPaths`, estimate time from artifact metadata:
   - For articles/URLs: query `raw_content` length, divide by 200 wpm (~1000 chars/min)
   - For YouTube: query `metadata->>'duration'` if available
   - Default: 10 minutes for unknown types
4. Sum total and remaining (uncompleted) time on each path

### Files Changed
- `internal/intelligence/learning.go` — add time fields and estimation logic
- `internal/intelligence/learning_test.go` — add tests for time estimation
