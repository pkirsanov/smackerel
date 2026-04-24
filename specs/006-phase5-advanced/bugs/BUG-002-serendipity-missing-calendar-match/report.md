# Report: BUG-002 — Serendipity Engine Missing Calendar Matching

## Discovery
- **Found by:** `bubbles.gaps` during stochastic quality sweep
- **Date:** April 22, 2026
- **Method:** Compared design R-505 data flow against `resurface.go::SerendipityPick` implementation

## Evidence
- Design spec R-505 data flow step 2: "calendar_match: query calendar events in next 7 days"
- `internal/intelligence/resurface.go` `SerendipityCandidate.CalendarMatch` field exists but is never set
- Scoring loop only has topic match (+2) and pinned bonus (+1), no calendar match (+3)

## Summary

`SerendipityPick` in `internal/intelligence/resurface.go` does not query upcoming calendar events and never sets the `CalendarMatch` field, so the documented +3 calendar-match scoring boost does not exist. This bug remains in_progress; no implementation has been verified in this artifact pass.

## Completion Statement

Status: in_progress. The fix is not yet verified in code; closure deferred until the calendar query is wired into `SerendipityPick`, the +3 boost is applied, and unit tests are captured passing in this report.

## Test Evidence

No new test execution was performed during this artifact-cleanup pass. Captured `go test ./internal/intelligence/...` output showing the new calendar-match scenarios passing is required before any DoD item is re-checked and before this bug is promoted out of `in_progress`.
