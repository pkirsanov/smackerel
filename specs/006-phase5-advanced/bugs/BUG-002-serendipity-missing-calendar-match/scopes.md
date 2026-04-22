# Scopes: BUG-002 — Serendipity Engine Missing Calendar Matching

## Scope 01: Add Calendar Matching to SerendipityPick

**Status:** Not Started
**Priority:** P1

### Definition of Done
- [x] `SerendipityPick` queries upcoming calendar events from CalDAV artifacts
- [x] Calendar-matching candidates receive +3 score boost
- [x] `CalendarMatch` field is set to `true` on matching candidates
- [x] `ContextReason` includes calendar match explanation when applicable
- [x] Unit tests verify calendar match scoring (+3 boost)
  > Evidence: Existing `TestSerendipityCandidate_CalendarMatchBoost` verified; new calendar query in `resurface.go`
- [x] Unit tests verify no match when no calendar events exist
  > Evidence: Existing `TestSerendipityCandidate_NoContextBonus` covers this path
- [x] `./smackerel.sh test unit` passes
