# OPS-002 Design — Faithful G088 `certifiedAt` remediation

## Current Truth

- Gate G088 (`bubbles/scripts/post-cert-spec-edit-guard.sh`) reads the
  **top-level** `state.json.certifiedAt`. If absent on a `done` spec it fails
  immediately ("G088 requires top-level certifiedAt"). If present, it checks for
  planning-truth commits (`spec.md`/`design.md`/`scopes.md`) dated after it.
- 45 done specs lack the top-level field (older schema); 21 have it but with a
  later planning edit.
- The remediation precedent is the individually-closed bugs from the sweep:
  BUG-029-007, BUG-024-004, BUG-049-002, BUG-054-001 — each added a faithful
  top-level `certifiedAt` (and, where a planning edit followed, a
  `bubbles.spec-review` CURRENT entry).

## Faithful timestamp rule (`analysis.py`)

Priority order for the historical cert timestamp:

1. latest `executionHistory[]` entry with `statusAfter == "done"` (`runEndedAt`)
2. `certification.certifiedAt`
3. `certification.completedAt`
4. top-level `completedAt`
5. `lastUpdatedAt`

Classification:

- **CLEAN-BACKFILL** — faithful timestamp ≥ latest planning commit ⇒ surface the
  timestamp to top-level `certifiedAt`. No review claimed (it is a pure
  schema-shape migration of an already-recorded certification).
- **NEEDS-RECERT** — a planning commit is newer than the faithful timestamp ⇒ a
  faithful recert needs a real per-spec review of those specific edits before
  writing a fresh `certifiedAt` + `bubbles.spec-review` CURRENT entry.
- **NO-SOURCE** — no derivable timestamp ⇒ per-spec reconstruction from git
  history of the cert commit.

## Anti-fabrication boundary (the core design constraint)

The post-cert planning commits are a **mix** of benign housekeeping and
substantive work (verified via `git log --since` of planning files). Because a
recert's `bubbles.spec-review CURRENT` entry asserts that the post-cert edits
were reviewed and the artifacts are trustworthy, that assertion may only be
written after a **real per-spec review**. A bulk timestamp bump is therefore
explicitly forbidden by this design.

## Execution in this packet

1. Run `analysis.py` → produce the classified inventory (recorded in report.md).
2. Execute the single CLEAN-BACKFILL faithfully (spec 018).
3. Record the NEEDS-RECERT (38), NO-SOURCE (6), and STALE (21) sets as the
   tracked, review-gated remediation queue.

## Change Boundary

- **Allowed:** this OPS-002 folder; `specs/018-financial-markets-connector/state.json`
  (top-level `certifiedAt` backfill only).
- **Excluded:** any `spec.md`/`design.md`/`scopes.md` of any target spec; any
  runtime/source/config; any bulk timestamp mutation.

## Test Strategy

| Test | Type | Asserts |
|------|------|---------|
| T-OPS-002-1 | tool | `analysis.py` reproduces the committed classification counts |
| T-OPS-002-2 | guard | `post-cert-spec-edit-guard.sh specs/018-...` PASSES after backfill |
| T-OPS-002-3 | review-discipline | No `certifiedAt` written for NEEDS-RECERT/NO-SOURCE specs in this packet |
