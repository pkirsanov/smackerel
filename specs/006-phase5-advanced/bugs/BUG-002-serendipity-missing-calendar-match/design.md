# Design: BUG-002 — Serendipity Engine Missing Calendar Matching

## Fix Design

Add calendar event matching to `SerendipityPick` in `internal/intelligence/resurface.go`.

### Approach
1. After the batch topic match query, add a batch calendar match query:
   - Query upcoming calendar events (next 7 days) from artifacts where `source_id = 'caldav'`
   - Extract event titles and attendee names
   - Match candidate artifact titles/topics against event keywords
2. If a match is found, set `CalendarMatch = true` and add +3 to the context score
3. Update `ContextReason` to reflect the calendar match

### Files Changed
- `internal/intelligence/resurface.go` — add calendar event query and scoring
- `internal/intelligence/resurface_test.go` — add calendar match scoring tests
