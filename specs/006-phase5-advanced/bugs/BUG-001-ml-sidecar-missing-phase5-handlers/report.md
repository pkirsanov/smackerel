# Report: BUG-001 — ML Sidecar Missing Phase 5 NATS Handlers

## Discovery
- **Found by:** `bubbles.gaps` during stochastic quality sweep
- **Date:** April 22, 2026
- **Method:** Searched for NATS subject usage in Go and Python code; found subjects defined in contract and subscribed in ML sidecar but no handler branches

## Evidence
- `ml/app/nats_client.py` lines 32-36: subjects listed in `SUBSCRIBE_SUBJECTS`
- `ml/app/nats_client.py` lines 68-72: response mapping in `SUBJECT_RESPONSE_MAP`
- `ml/app/nats_client.py` ~line 290: `else: logger.warning("Unknown subject: %s", subject)` catches all Phase 5 messages
- Go side publishes to `smk.monthly.generate` (monthly.go:250) and `smk.content.analyze` (monthly.go:389) but responses are never consumed

## Summary

Phase 5 NATS subjects (`learning.classify`, `content.analyze`, `monthly.generate`, `quickref.generate`, `seasonal.analyze`) are subscribed by the ML sidecar but no handler branches exist. Messages fall through to the `Unknown subject` warning. This bug remains in_progress; no implementation has been verified in this artifact pass.

## Completion Statement

Status: in_progress. The fix is not yet verified in code; closure deferred until each Phase 5 handler is implemented and `./smackerel.sh test unit` is captured passing in this report.

## Test Evidence

No new test execution was performed during this artifact-cleanup pass. Implementation evidence (handler branches in `ml/app/nats_client.py` and corresponding unit tests under `ml/tests/`) is required before any DoD item is re-checked and before this bug is promoted out of `in_progress`.
