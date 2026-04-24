# Scopes: BUG-004 — Learning Path Missing Time Estimation

## Scope 01: Add Time Estimation to Learning Paths

**Status:** Done
**Priority:** P2

### Definition of Done
- [x] `LearningResource` has `EstimatedMinutes` field
  **Evidence:** `internal/intelligence/learning.go:33` — `EstimatedMinutes int                `json:"estimated_minutes"`` inside the `LearningResource` struct.
- [x] `LearningPath` has `TotalMinutes` and `RemainingMinutes` fields
  **Evidence:** `internal/intelligence/learning.go:45-46` — `TotalMinutes int` and `RemainingMinutes int` on the `LearningPath` struct.
- [x] Time estimated from content length for articles/URLs
  **Evidence:** `internal/intelligence/learning.go:71` SQL projects `COALESCE(LENGTH(a.raw_content), 0) AS content_length`; `learning.go:151` calls `estimateReadingTime(contentType, contentLength, durationStr)` per resource.
- [x] Time estimated from duration metadata for videos
  **Evidence:** `internal/intelligence/learning.go:72` SQL projects `COALESCE(a.metadata->>'duration', '') AS duration_str`; `learning.go:249-257` parses video duration in `estimateReadingTime` and converts seconds to minutes.
- [x] Default estimate applied for unknown content types
  **Evidence:** `internal/intelligence/learning.go::estimateReadingTime` (lines 251 onwards) returns a default minute count when content type is unrecognised — covered explicitly by `TestEstimateReadingTime_UnknownType` (PASS).
- [x] Unit tests verify time estimation logic
  **Evidence:** `internal/intelligence/learning_test.go` includes `TestEstimateReadingTime_Article`, `TestEstimateReadingTime_ShortArticle`, `TestEstimateReadingTime_ZeroLength`, `TestEstimateReadingTime_YouTube`, `TestEstimateReadingTime_YouTubeNoDuration`, `TestEstimateReadingTime_PDF`, `TestEstimateReadingTime_UnknownType`, `TestEstimateReadingTime_YouTubeRoundsUp` — all PASS in this re-cert run (see report.md Test Evidence).
- [x] `./smackerel.sh test unit` passes
  **Evidence:** Captured 2026-04-24 — `./smackerel.sh test unit` returns `330 passed, 2 warnings in 11.48s` and the focused intelligence run PASSes in 0.022s. See report.md Test Evidence.
