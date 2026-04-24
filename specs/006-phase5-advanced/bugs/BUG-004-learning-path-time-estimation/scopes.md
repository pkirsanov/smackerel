# Scopes: BUG-004 — Learning Path Missing Time Estimation

## Scope 01: Add Time Estimation to Learning Paths

**Status:** Not Started
**Priority:** P2

### Definition of Done
- [ ] `LearningResource` has `EstimatedMinutes` field
- [ ] `LearningPath` has `TotalMinutes` and `RemainingMinutes` fields
- [ ] Time estimated from content length for articles/URLs
- [ ] Time estimated from duration metadata for videos
- [ ] Default estimate applied for unknown content types
- [ ] Unit tests verify time estimation logic
- [ ] `./smackerel.sh test unit` passes

DoD items un-checked because the fix has not been verified in this artifact pass (status: in_progress).
