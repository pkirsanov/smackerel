# Report: BUG-004 — Learning Path Missing Time Estimation

## Discovery
- **Found by:** `bubbles.gaps` during stochastic quality sweep
- **Date:** April 22, 2026
- **Method:** Compared design R-502 data flow step 5 against `learning.go` implementation

## Evidence
- Design R-502 step 5: "Estimate total time: articles: word_count / 200 wpm, videos: duration from metadata"
- `internal/intelligence/learning.go` `LearningResource` struct: no time field
- `internal/intelligence/learning.go` `LearningPath` struct: only `TotalCount`/`CompletedCount`, no time
