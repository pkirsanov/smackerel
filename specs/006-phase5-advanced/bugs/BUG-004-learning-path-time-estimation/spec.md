# Bug: BUG-004 — Learning Path Missing Time Estimation

> **Parent Spec:** [specs/006-phase5-advanced](../../spec.md)
> **Severity:** Minor
> **Found By:** bubbles.gaps
> **Date:** April 22, 2026

## Problem

Design R-502 specifies learning paths should include time estimates:
- Articles: `word_count / 200 wpm`
- Videos: `duration from metadata`
- Books: `page_count * 2 min/page`

The `LearningResource` struct has no time estimate field and `GetLearningPaths` does not compute estimated completion times. The `LearningPath` struct has `TotalCount`/`CompletedCount` but no `EstimatedTimeMinutes` or per-resource time.

## Impact

Users cannot assess the time commitment of a learning path — the design's promise of "Total resources and estimated time" is missing.

## Expected Behavior

Each `LearningResource` should have an `EstimatedMinutes` field, and `LearningPath` should sum remaining time.
