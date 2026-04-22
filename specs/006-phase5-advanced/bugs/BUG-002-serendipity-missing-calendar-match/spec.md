# Bug: BUG-002 — Serendipity Engine Missing Calendar Matching

> **Parent Spec:** [specs/006-phase5-advanced](../../spec.md)
> **Severity:** Moderate
> **Found By:** bubbles.gaps
> **Date:** April 22, 2026

## Problem

`internal/intelligence/resurface.go::SerendipityPick` implements topic matching (+2 score boost) but NOT calendar matching (+3 score boost). The design (R-505 data flow) specifies:
- "Query calendar events in next 7 days; if artifact topic/title matches event title/attendees: +3"

The `SerendipityCandidate` struct has a `CalendarMatch bool` field but it is never set to `true`.

## Impact

Serendipity resurfaces lack the highest-value context signal. The design's flagship example — "A serendipity resurface from October matches next week's team offsite perfectly" — cannot occur.

## Expected Behavior

`SerendipityPick` should query upcoming calendar events (from CalDAV connector data) and boost candidates that match event topics or attendee names.

## Root Cause

The calendar query was never implemented in the serendipity scoring loop. Only topic matching (via `edges`/`topics` query) and pinned bonus are scored.
