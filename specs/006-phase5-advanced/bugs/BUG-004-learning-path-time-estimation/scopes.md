# Scopes: BUG-004 — Learning Path Missing Time Estimation

## Scope 01: Add Time Estimation to Learning Paths

**Status:** Not Started
**Priority:** P2

### Definition of Done
- [x] `LearningResource` has `EstimatedMinutes` field
- [x] `LearningPath` has `TotalMinutes` and `RemainingMinutes` fields
- [x] Time estimated from content length for articles/URLs
- [x] Time estimated from duration metadata for videos
- [x] Default estimate applied for unknown content types
- [x] Unit tests verify time estimation logic
  > Evidence: 8 new tests in `learning_test.go` (TestEstimateReadingTime_*)
- [x] `./smackerel.sh test unit` passes
