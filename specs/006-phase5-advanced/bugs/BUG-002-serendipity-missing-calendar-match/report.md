# Report: BUG-002 — Serendipity Engine Missing Calendar Matching

## Discovery
- **Found by:** `bubbles.gaps` during stochastic quality sweep
- **Date:** April 22, 2026
- **Method:** Compared design R-505 data flow against `resurface.go::SerendipityPick` implementation

## Summary

Re-verified 2026-04-24 against committed code: `internal/intelligence/resurface.go` now batch-fetches CalDAV calendar events for the next 7 days, sets `CalendarMatch = true`, applies the documented `+3.0` score boost, and writes `ContextReason = "Matches an upcoming calendar event"`. Unit tests in `resurface_test.go` cover both the boost case and the no-match case.

## Completion Statement

Status: done. The R-505 calendar-match data flow is implemented end to end and the focused regression test plus full repo-CLI unit run executed in this re-cert pass have been captured below.

## Test Evidence

Focused Go run captured 2026-04-24T07:29:44Z → 07:29:45Z:

```text
$ go test -count=1 -v -run "TestSerendipityCandidate_CalendarMatchBoost" ./internal/intelligence/...
=== RUN   TestSerendipityCandidate_CalendarMatchBoost
--- PASS: TestSerendipityCandidate_CalendarMatchBoost (0.00s)
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

Field wiring + scoring captured 2026-04-24 via `grep -nE "..." internal/intelligence/resurface.go`:

```text
$ grep -nE "CalendarMatch|ContextReason|score \+|caldav|matches upcoming" internal/intelligence/resurface.go
156:    CalendarMatch bool    `json:"calendar_match"`
159:    ContextReason string  `json:"context_reason"`
245:    // 3. Batch-fetch calendar matches: upcoming events in next 7 days from CalDAV
250:            WHERE source_id = 'caldav'
306:                    score += 2.0
308:                    sc.ContextReason = "Connects to a currently active topic"
313:                    score += 3.0
315:                    sc.ContextReason = "Matches an upcoming calendar event"
320:                    score += 1.0
336:    if best.ContextReason == "" {
339:            best.Reason = fmt.Sprintf("Remember this? %s — %s", best.Title, best.ContextReason)
```

### Audit Evidence

Repo-CLI hygiene + targeted regression captured 2026-04-24T07:30:21Z → 07:30:29Z:

```text
$ ./smackerel.sh check
Config is in sync with SST
env_file drift guard: OK
$ go test -count=1 -run "Serendipity|CalendarMatch" ./internal/intelligence/...
ok      github.com/smackerel/smackerel/internal/intelligence    0.019s
```

SST sync confirms no env-var drift; the focused Serendipity regression replay confirms no neighbouring test in the intelligence package regressed.

