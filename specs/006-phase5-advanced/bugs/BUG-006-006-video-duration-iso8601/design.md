# Design: BUG-006-006 — Learning Path Video Duration Never Parsed

## Root Cause

`estimateReadingTime` (in `internal/intelligence/learning.go`) used
`fmt.Sscanf(durationStr, "%d", &seconds)` to read a video duration, which only
succeeds for a string that begins with decimal digits. The YouTube connector —
the sole writer of `metadata.duration` — stores ISO 8601 durations (`PT45M`,
`PT1H30M`). `Sscanf` returns an error on the leading `PT`, the `err == nil`
guard is false, and control falls through to `return 15`. Net effect: the real
duration is discarded and every video is reported as 15 minutes.

The defect was latent because the only duration test fed `"600"` (plain
seconds), which the connector never produces, so CI stayed green.

## Fix Design

Introduce a dedicated parser that understands the format actually stored, while
preserving the legacy plain-seconds path so nothing regresses:

1. Add `parseVideoDurationSeconds(durationStr string) (int, bool)`:
   - Parse ISO 8601 time-only durations via a compiled regexp
     `^PT(?:(\d+)H)?(?:(\d+)M)?(?:(\d+)S)?$` (hours/minutes/seconds, each
     optional, at least one required). This is exactly the YouTube form.
   - Fall back to bare integer seconds (`strconv.Atoi`) for backward
     compatibility with any caller/connector that stores plain seconds.
   - Return `(0, false)` for empty/`"PT"`/junk input so the caller keeps its
     existing 15-minute default — no panic, no zero-length estimate.
2. Replace the `fmt.Sscanf` branch in `estimateReadingTime`'s YouTube case with
   `if seconds, ok := parseVideoDurationSeconds(durationStr); ok { … }`.

Minute conversion is unchanged: `(seconds + 59) / 60` (round up).

### Why a regexp (not `time.ParseDuration`)

Go's `time.ParseDuration` accepts `"45m"`/`"1h30m"` but **not** ISO 8601
`"PT45M"`, so it cannot consume the stored value. A small anchored regexp is the
minimal correct parser and rejects malformed input cleanly.

### Files Changed

- `internal/intelligence/learning.go` — add `regexp`/`strconv` imports,
  `videoDurationISO8601RE`, `parseVideoDurationSeconds`; rewrite the YouTube
  branch of `estimateReadingTime`.
- `internal/intelligence/learning_test.go` — add adversarial regression
  `TestEstimateReadingTime_YouTubeISO8601` (would fail if the bug returned), plus
  `TestEstimateReadingTime_YouTubePlainSecondsStillWorks` (backward-compat pin)
  and `TestEstimateReadingTime_YouTubeUnparseableDuration` (fallback safety).

### Blast Radius

`estimateReadingTime` is a pure helper called only from
`GetLearningPaths`. No SQL, schema, API, connector, or config surface changes.
Article/PDF/note estimation paths are untouched.
