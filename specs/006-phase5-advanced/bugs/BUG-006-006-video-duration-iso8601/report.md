# Report: BUG-006-006 — Learning Path Video Duration Never Parsed

## Discovery
- **Found by:** `bubbles.improve` (stochastic-quality-sweep round 38, `improve` trigger)
- **Date:** June 17, 2026
- **Method:** Traced the spec-006 learning-path estimator (`internal/intelligence/learning.go`) against its only data producer. `estimateReadingTime` parses `metadata.duration` as bare integer seconds (`fmt.Sscanf(durationStr, "%d", …)`), but the YouTube connector — the sole writer of that field — stores ISO 8601 (`PT45M`). Mismatch → every video silently defaults to 15 minutes.

## Summary

`estimateReadingTime`'s YouTube branch now parses the ISO 8601 duration form the
connector actually emits, via a dedicated `parseVideoDurationSeconds` helper that
also retains the legacy bare-seconds path and the 15-minute fallback. The defect
was latent because the prior `TestEstimateReadingTime_YouTube` fed `"600"` (plain
seconds) — a format the connector never produces — so CI stayed green.

## Completion Statement

Status: done. The spec-006 learning-path estimator now honors the ISO 8601
video-duration form the YouTube connector actually emits. The fix ships with an
adversarial regression captured RED before the fix and GREEN after, the full
`internal/intelligence` package and repo Go unit suite stay green, the bug packet
passes `artifact-lint`, and the parent spec 006 `traceability-guard` remains
PASSED. All evidence is captured below in the same session.

## Root-Cause Evidence

Producer stores ISO 8601 verbatim; consumer expected seconds:

```text
$ grep -nE '"duration"|vid.Duration' internal/connector/youtube/youtube.go
31:     Duration       string    `json:"duration"`
136:                    "duration":        vid.Duration,
415:            if dur := getStr(vm, "duration"); dur != "" {
416:                            vid.Duration = dur

$ grep -n '"duration"' internal/connector/youtube/youtube_test.go
43:                                     "duration":    "PT45M",
310:                                    "duration":   "PT10M",
377:                                    "duration":    "PT5M",
```

## Before Fix (Bug Reproduced — RED)

The adversarial regression was added first and run against the **unfixed** code.

```text
$ ./smackerel.sh test unit --go --go-run 'TestEstimateReadingTime_YouTubeISO8601|TestEstimateReadingTime_YouTubePlainSecondsStillWorks|TestEstimateReadingTime_YouTubeUnparseableDuration' --verbose
=== RUN   TestEstimateReadingTime_YouTubeISO8601
    learning_test.go:341: estimateReadingTime(youtube, 0, "PT45M") = 15, want 45 (ISO 8601 duration must be honored, not defaulted to 15)
    learning_test.go:341: estimateReadingTime(youtube, 0, "PT10M") = 15, want 10 (ISO 8601 duration must be honored, not defaulted to 15)
    learning_test.go:341: estimateReadingTime(youtube, 0, "PT5M") = 15, want 5 (ISO 8601 duration must be honored, not defaulted to 15)
    learning_test.go:341: estimateReadingTime(youtube, 0, "PT1H") = 15, want 60 (ISO 8601 duration must be honored, not defaulted to 15)
    learning_test.go:341: estimateReadingTime(youtube, 0, "PT1H30M") = 15, want 90 (ISO 8601 duration must be honored, not defaulted to 15)
    learning_test.go:341: estimateReadingTime(youtube, 0, "PT90S") = 15, want 2 (ISO 8601 duration must be honored, not defaulted to 15)
    learning_test.go:341: estimateReadingTime(youtube, 0, "PT2H5M30S") = 15, want 126 (ISO 8601 duration must be honored, not defaulted to 15)
--- FAIL: TestEstimateReadingTime_YouTubeISO8601 (0.00s)
=== RUN   TestEstimateReadingTime_YouTubePlainSecondsStillWorks
--- PASS: TestEstimateReadingTime_YouTubePlainSecondsStillWorks (0.00s)
=== RUN   TestEstimateReadingTime_YouTubeUnparseableDuration
--- PASS: TestEstimateReadingTime_YouTubeUnparseableDuration (0.00s)
FAIL
FAIL    github.com/smackerel/smackerel/internal/intelligence    0.099s
```

The backward-compat and fallback cases already passed (they exercise paths that
were not broken), proving the regression isolates exactly the ISO 8601 gap — it
is adversarial, not tautological.

## The Fix

`internal/intelligence/learning.go` — YouTube branch of `estimateReadingTime`
(~line 260) now calls `parseVideoDurationSeconds`, and the new helper
(`videoDurationISO8601RE` + `parseVideoDurationSeconds`, ~line 291) parses ISO
8601 (`PT#H#M#S`) with a bare-integer-seconds fallback and a safe `(0,false)`
return for junk input.

```text
$ grep -nE 'parseVideoDurationSeconds|videoDurationISO8601RE' internal/intelligence/learning.go
266:            if seconds, ok := parseVideoDurationSeconds(durationStr); ok {
291:    var videoDurationISO8601RE = regexp.MustCompile(`^PT(?:(\d+)H)?(?:(\d+)M)?(?:(\d+)S)?$`)
298:    func parseVideoDurationSeconds(durationStr string) (int, bool) {
```

## Test Evidence

The RED reproduction is in the "Before Fix" section above; the GREEN result and
the full-suite regression sweep follow.

### After Fix (GREEN)

Re-running the full `estimateReadingTime` YouTube family after the fix — the new
adversarial test passes and every pre-existing case stays green:

```text
$ ./smackerel.sh test unit --go --go-run 'TestEstimateReadingTime_YouTubeISO8601|TestEstimateReadingTime_YouTubePlainSecondsStillWorks|TestEstimateReadingTime_YouTubeUnparseableDuration|TestEstimateReadingTime_YouTube$|TestEstimateReadingTime_YouTubeNoDuration|TestEstimateReadingTime_YouTubeRoundsUp' --verbose
=== RUN   TestEstimateReadingTime_YouTube
--- PASS: TestEstimateReadingTime_YouTube (0.00s)
=== RUN   TestEstimateReadingTime_YouTubeNoDuration
--- PASS: TestEstimateReadingTime_YouTubeNoDuration (0.00s)
=== RUN   TestEstimateReadingTime_YouTubeRoundsUp
--- PASS: TestEstimateReadingTime_YouTubeRoundsUp (0.00s)
=== RUN   TestEstimateReadingTime_YouTubeISO8601
--- PASS: TestEstimateReadingTime_YouTubeISO8601 (0.00s)
=== RUN   TestEstimateReadingTime_YouTubePlainSecondsStillWorks
--- PASS: TestEstimateReadingTime_YouTubePlainSecondsStillWorks (0.00s)
=== RUN   TestEstimateReadingTime_YouTubeUnparseableDuration
--- PASS: TestEstimateReadingTime_YouTubeUnparseableDuration (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/intelligence    0.101s
```

## Regression Evidence (full suite)

Full `internal/intelligence` package plus the whole repo Go unit suite — no
neighboring test regressed (`go test ./...` finished OK, exit 0):

```text
$ ./smackerel.sh test unit --go --go-run 'Learning|EstimateReadingTime|DetectGaps|DifficultyOrder|ClassifyDifficulty|NormalizeDifficulty'
ok      github.com/smackerel/smackerel/internal/intelligence    0.094s
ok      github.com/smackerel/smackerel/internal/intelligence/surfacing  0.011s [no tests to run]
...
[go-unit] go test ./... finished OK
WRAPPER_EXIT=0
```

### Validation Evidence

Spec-scoped guards for the BUG-006-006 delta.

Bug-packet artifact integrity — `artifact-lint` PASSES (full 6-artifact set, DoD
completion gate, bugfix-fastlane report sections, anti-fabrication evidence
checks all green):

```text
$ bash .github/bubbles/scripts/artifact-lint.sh specs/006-phase5-advanced/bugs/BUG-006-006-video-duration-iso8601
✅ Required artifact exists: spec.md
✅ Required artifact exists: design.md
✅ Required artifact exists: uservalidation.md
✅ Required artifact exists: state.json
✅ Required artifact exists: scopes.md
✅ Required artifact exists: report.md
✅ DoD completion gate passed for status 'done' (all DoD checkboxes are checked)
✅ Workflow mode 'bugfix-fastlane' permits current status 'done' (ceiling: done)
✅ All 1 scope(s) in scopes.md are marked Done
✅ All checked DoD items in scopes.md have evidence blocks
✅ All 5 evidence blocks in report.md contain legitimate terminal output
Artifact lint PASSED
```

Parent spec traceability — `traceability-guard` on `specs/006-phase5-advanced`
remains PASSED (the code fix did not disturb the parent's scenario/DoD mapping;
bug folders are DoD-only and are not themselves traceability-guard targets, the
same as the sibling done bug BUG-004):

```text
$ bash .github/bubbles/scripts/traceability-guard.sh specs/006-phase5-advanced
--- Traceability Summary ---
ℹ️  Scenarios checked: 28
ℹ️  Test rows checked: 45
ℹ️  Scenario-to-row mappings: 28
ℹ️  Concrete test file references: 28
ℹ️  Report evidence references: 28
ℹ️  DoD fidelity scenarios: 28 (mapped: 28, unmapped: 0)

RESULT: PASSED (0 warnings)
```

### Audit Evidence

**Change boundary** — only three surfaces touched, all within spec 006 Scope 02 +
this bug packet:

- `internal/intelligence/learning.go` — duration parser.
- `internal/intelligence/learning_test.go` — adversarial + backward-compat + fallback tests.
- `specs/006-phase5-advanced/bugs/BUG-006-006-video-duration-iso8601/**` — this packet.

No connector, schema, config, CI, or other-spec file was changed; the YouTube
connector's ISO 8601 storage format is correct and was left untouched. The
adversarial regression `TestEstimateReadingTime_YouTubeISO8601` is the standing
guard against reintroduction (it fails if the parser ever regresses to the
seconds-only path).
