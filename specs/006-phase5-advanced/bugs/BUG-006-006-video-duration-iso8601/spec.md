# Bug: BUG-006-006 — Learning Path Video Duration Never Parsed (ISO 8601 vs seconds)

> **Parent Spec:** [specs/006-phase5-advanced](../../spec.md)
> **Severity:** Minor
> **Found By:** bubbles.improve (stochastic-quality-sweep round 38, `improve` trigger)
> **Date:** June 17, 2026
> **Workflow Mode:** `bugfix-fastlane`

## Problem

[BUG-004](../BUG-004-learning-path-time-estimation/spec.md) added per-resource
time estimation to learning paths (Scope 02 / R-502). Its `estimateReadingTime`
helper parses a video's duration with:

```go
var seconds int
if _, err := fmt.Sscanf(durationStr, "%d", &seconds); err == nil && seconds > 0 {
    return (seconds + 59) / 60
}
```

This assumes `metadata.duration` is a bare integer number of seconds. But the
**only producer** of that field — the YouTube connector — stores the duration in
**ISO 8601** form (`PT45M`, `PT10M`, `PT5M`, `PT1H30M`), copied verbatim from the
source:

- `internal/connector/youtube/youtube.go:136` — `"duration": vid.Duration`
- `internal/connector/youtube/youtube.go:415-416` — `vid.Duration = dur` (raw source string)
- Connector fixtures: `internal/connector/youtube/youtube_test.go:43,310,377` — `"PT45M"`, `"PT10M"`, `"PT5M"`

`fmt.Sscanf("PT45M", "%d", …)` fails on the leading `PT`, so the code silently
falls through to the hardcoded **15-minute default** for **every real video**.

The existing test `TestEstimateReadingTime_YouTube` masked the defect by feeding
`"600"` (plain seconds) — a format the connector never emits — so the green test
suite did not surface the gap.

## Impact

R-502 promises "Total resources and estimated time" including "video duration".
For any learning path containing YouTube resources:

- `LearningResource.EstimatedMinutes` is wrong for every video (always 15).
- `LearningPath.TotalMinutes` and `RemainingMinutes` are wrong by the cumulative
  per-video error (a 45-minute talk and a 5-minute clip both count as 15).

The time-commitment signal the feature exists to provide is unreliable whenever
videos are present. Observational only — no crash, no data loss.

## Expected Behavior

```gherkin
Scenario: SCN-006-006c Video duration parsed from ISO 8601 metadata
  Given a YouTube learning resource whose metadata.duration is "PT45M"
  When the learning path estimates its reading time
  Then the resource's estimated time is 45 minutes
  And it is NOT silently defaulted to 15 minutes

Scenario: SCN-006-006d Bare-seconds duration remains supported
  Given a video resource whose duration metadata is the integer string "600"
  When the learning path estimates its reading time
  Then the resource's estimated time is 10 minutes

Scenario: SCN-006-006e Unparseable duration falls back to default
  Given a video resource whose duration metadata is missing or malformed
  When the learning path estimates its reading time
  Then the resource's estimated time falls back to the 15-minute default
  And no panic or zero-length estimate is produced
```

## Scope Boundary

- **In scope:** `internal/intelligence/learning.go` (spec 006, Scope 02) duration
  parsing, plus its tests in `internal/intelligence/learning_test.go`.
- **Out of scope:** the YouTube connector's storage format (it correctly carries
  the source's ISO 8601 value); no connector, schema, config, or other spec is
  changed.
