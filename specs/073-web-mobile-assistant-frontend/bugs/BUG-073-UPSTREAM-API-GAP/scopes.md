# Scopes — BUG-073-UPSTREAM-API-GAP (Tracking Bug)

**Status:** N/A — tracking bug; no own scope of work.

## Scope Inventory

| Scope ID | Name | Status | Owner |
|----------|------|--------|-------|
| (none) | (no own scope — tracking bug) | N/A | upstream specs 080 + 027 |

## Why no own scope

This bug folder exists only to track a known backend dependency for
the spec 073 wiki/graph-browse surface (Scope 5 blocker). The
dependency was resolved by two unrelated upstream specs that own
their own scopes:

- [specs/080-knowledge-graph-public-api/scopes.md](../../../080-knowledge-graph-public-api/scopes.md) — 8 JSON endpoints (covers AC-1..AC-8 / SCN-073-B01..B05)
- [specs/027-user-annotations/scopes.md](../../../027-user-annotations/scopes.md) — Scope 9 Annotation Editing API (covers SCN-073-B06)

### Definition of Done

- [x] Upstream resolution shipped (verified in [report.md](report.md#validation-report--bug-close-out-2026-06-04))

  Evidence:
  ```text
  $ git log --oneline 98c16290 e6ccdb2a | head -2
  98c16290 spec(080): promote in_progress -> done (final certification)
  e6ccdb2a spec(027): user annotations Scope 9 — annotation editing API
  $ python3 -c "import json; [print(s, json.load(open(f'specs/{s}/state.json'))['status']) for s in ['080-knowledge-graph-public-api','027-user-annotations']]"
  080-knowledge-graph-public-api done
  027-user-annotations done
  ```

- [x] bug.md `Status:` header synced to `Resolved` with one-line rationale (bubbles.bug 2026-06-04T21:24:20Z)

  Evidence:
  ```text
  $ grep -n '^\*\*Status:\*\*' specs/073-web-mobile-assistant-frontend/bugs/BUG-073-UPSTREAM-API-GAP/bug.md
  <line>:**Status:** Resolved — Upstream resolution shipped: spec 080 (commit 98c16290) + spec 027 Scope 9 (commit e6ccdb2a). Both upstream specs certified done.
  ```
  Per state.json execution.executionHistory[] entry timestamp 2026-06-04T21:24:20Z (agent: bubbles.bug, outcome: bug_artifact_synced_with_resolution_claim).

- [x] state.json `workflowMode` reassigned `bugfix-fastlane` → `validate-to-doc` (correct shape for tracking-bug close-out; no fabricated implement/test/audit phases)

  Evidence:
  ```text
  $ python3 -c "import json; s = json.load(open('specs/073-web-mobile-assistant-frontend/bugs/BUG-073-UPSTREAM-API-GAP/state.json')); print('workflowMode:', s['workflowMode']); print('changedAt:', s.get('workflowModeChangedAt'))"
  workflowMode: validate-to-doc
  changedAt: 2026-06-04T22:15:00Z
  ```
  Reassignment recorded in state.json `workflowModeChangedReason` field and in certification.observations[] (severity:low, category:framework-mode-mismatch, RESOLVED 2026-06-04T22:15:00Z).
