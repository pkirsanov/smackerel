# OPS-002 Scopes

## Scope 1 — Classify the G088 backlog and execute the clean-backfill slice

**Status:** Done

### Gherkin Scenarios

See [spec.md](spec.md): SCN-OPS-002-1, SCN-OPS-002-2, SCN-OPS-002-3.

### Implementation Plan

- Author `analysis.py` (read-only) that classifies every `done` spec into
  CLEAN-BACKFILL / NEEDS-RECERT / NO-SOURCE using the faithful-timestamp rule.
- Surface the historical cert timestamp to a top-level `certifiedAt` for the
  single CLEAN-BACKFILL spec (018), with a backfill note documenting the source
  and the no-fresh-review-claimed boundary.
- Record the full classified inventory in [report.md](report.md).
- Record the NEEDS-RECERT / NO-SOURCE / STALE sets as the tracked, review-gated
  remediation queue (no timestamp bump without per-spec review).

### Test Plan

| ID | Test | Expectation |
|----|------|-------------|
| T-OPS-002-1 | `python3 analysis.py` | Reproduces CLEAN-BACKFILL=1, NEEDS-RECERT=38, NO-SOURCE=6 |
| T-OPS-002-2 | `post-cert-spec-edit-guard.sh specs/018-financial-markets-connector` | PASS (exit 0) after backfill |
| T-OPS-002-3 | review-discipline audit | Zero `certifiedAt` writes for NEEDS-RECERT/NO-SOURCE specs in this packet |

### Definition of Done

- [x] `analysis.py` authored and classifies the portfolio deterministically
  - Evidence: [report.md](report.md) "Classified Inventory" section with tool output.
- [x] Spec 018 top-level `certifiedAt` backfilled faithfully (historical source, no review claimed)
  - Evidence: [report.md](report.md) "Clean-Backfill Execution (spec 018)" — `post-cert-spec-edit-guard` PASS.
- [x] NEEDS-RECERT (38) / NO-SOURCE (6) / STALE (21) recorded as the review-gated remediation queue
  - Evidence: [report.md](report.md) "Tracked Remediation Queue" + "Anti-Fabrication Boundary".
- [x] No bulk/blanket timestamp mutation performed (anti-fabrication preserved)
  - Evidence: only `specs/018-.../state.json` changed; verified by git diff --name-only in [report.md](report.md).
- [x] Change Boundary is respected and zero excluded file families were changed
  - Evidence: [report.md](report.md) "Change Boundary Verification".
