# Bug: BUG-001 — ML Sidecar Missing Phase 5 NATS Handlers

> **Parent Spec:** [specs/006-phase5-advanced](../../spec.md)
> **Severity:** Critical
> **Found By:** bubbles.gaps
> **Date:** April 22, 2026

## Problem

The ML sidecar (`ml/app/nats_client.py`) subscribes to 5 Phase 5 NATS subjects but has NO processing handlers. Messages on these subjects fall into the `else: logger.warning("Unknown subject")` branch, are silently ACK'd, and discarded:

- `learning.classify` — should classify resource difficulty for learning paths
- `content.analyze` — should generate writing angles from topic artifacts
- `monthly.generate` — should produce LLM-enhanced monthly report text
- `quickref.generate` — should compile quick references from source artifacts
- `seasonal.analyze` — should detect seasonal patterns with LLM commentary

## Impact

All Phase 5 features that require LLM delegation silently fail. The Go runtime falls back to local heuristics or template text, producing significantly lower-quality outputs than designed.

## Expected Behavior

Each NATS subject should have a dedicated handler in the ML sidecar's `_consume_loop` that:
1. Parses the incoming payload
2. Calls the LLM provider for intelligent analysis
3. Returns a structured response on the paired response subject

## Reproduction

1. Send a message on any Phase 5 NATS subject
2. Observe ML sidecar logs: `"Unknown subject: learning.classify"` etc.
3. Message is ACK'd but no processing occurs

## Root Cause

The `_consume_loop` in `ml/app/nats_client.py` has explicit `elif` branches for `artifacts.process`, `search.embed`, `search.rerank`, `digest.generate`, `keep.sync.request`, `keep.ocr.request`, `synthesis.extract`, `synthesis.crosssource`, and `domain.extract` — but no branches for the 5 Phase 5 subjects.
