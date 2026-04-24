# Scopes: BUG-002 — Serendipity Engine Missing Calendar Matching

## Scope 01: Add Calendar Matching to SerendipityPick

**Status:** Not Started
**Priority:** P1

### Definition of Done
- [ ] `SerendipityPick` queries upcoming calendar events from CalDAV artifacts
- [ ] Calendar-matching candidates receive +3 score boost
- [ ] `CalendarMatch` field is set to `true` on matching candidates
- [ ] `ContextReason` includes calendar match explanation when applicable
- [ ] Unit tests verify calendar match scoring (+3 boost)
- [ ] Unit tests verify no match when no calendar events exist
- [ ] `./smackerel.sh test unit` passes

DoD items un-checked because the fix has not been verified in this artifact pass (status: in_progress).
