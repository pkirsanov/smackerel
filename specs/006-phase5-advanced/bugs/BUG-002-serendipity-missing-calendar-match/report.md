# Report: BUG-002 — Serendipity Engine Missing Calendar Matching

## Discovery
- **Found by:** `bubbles.gaps` during stochastic quality sweep
- **Date:** April 22, 2026
- **Method:** Compared design R-505 data flow against `resurface.go::SerendipityPick` implementation

## Evidence
- Design spec R-505 data flow step 2: "calendar_match: query calendar events in next 7 days"
- `internal/intelligence/resurface.go` `SerendipityCandidate.CalendarMatch` field exists but is never set
- Scoring loop only has topic match (+2) and pinned bonus (+1), no calendar match (+3)
