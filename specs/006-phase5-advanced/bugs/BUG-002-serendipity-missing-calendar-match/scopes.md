# Scopes: BUG-002 — Serendipity Engine Missing Calendar Matching

## Scope 01: Add Calendar Matching to SerendipityPick

**Status:** Done
**Priority:** P1

### Definition of Done
- [x] `SerendipityPick` queries upcoming calendar events from CalDAV artifacts
  **Evidence:** `internal/intelligence/resurface.go:245-250` adds the comment `// 3. Batch-fetch calendar matches: upcoming events in next 7 days from CalDAV` followed by `WHERE source_id = 'caldav'` query in the SerendipityPick body (line 164 onwards).
- [x] Calendar-matching candidates receive +3 score boost
  **Evidence:** `internal/intelligence/resurface.go:313` — `score += 3.0` inside the calendar-match branch (vs `+= 2.0` topic-match at line 306 and `+= 1.0` pinned bonus at line 320).
- [x] `CalendarMatch` field is set to `true` on matching candidates
  **Evidence:** `internal/intelligence/resurface.go:156` defines `CalendarMatch bool`; the calendar-match branch sets `sc.CalendarMatch = true` and applies the +3 boost. Test asserts `if !withCalendar.CalendarMatch` at `resurface_test.go:228`.
- [x] `ContextReason` includes calendar match explanation when applicable
  **Evidence:** `internal/intelligence/resurface.go:315` — `sc.ContextReason = "Matches an upcoming calendar event"`. Final reason composition at line 339 uses `Sprintf("Remember this? %s — %s", best.Title, best.ContextReason)`.
- [x] Unit tests verify calendar match scoring (+3 boost)
  **Evidence:** `internal/intelligence/resurface_test.go:209` — `TestSerendipityCandidate_CalendarMatchBoost`; assertion at line 225 `"calendar-matched candidate should score higher"`.
- [x] Unit tests verify no match when no calendar events exist
  **Evidence:** `internal/intelligence/resurface_test.go:122-132` covers the `CalendarMatch: false` branch and asserts `t.Error("CalendarMatch should be false")`.
- [x] `./smackerel.sh test unit` passes
  **Evidence:** Captured 2026-04-24 — focused `go test -count=1 -v -run "TestSerendipityCandidate_CalendarMatchBoost" ./internal/intelligence/...` returns PASS in 0.022s, plus `./smackerel.sh test unit` shows `330 passed, 2 warnings in 11.48s`. See report.md Test Evidence.
