# User Validation: BUG-026-005 Stale code/test path references in spec 026 scopes.md

Links: [bug.md](bug.md) | [spec.md](spec.md) | [design.md](design.md) | [scopes.md](scopes.md) | [report.md](report.md)

## User Acceptance

This bug is a documentation-only spec-artifact reconciliation surfaced by `sweep-2026-05-24-r10` round 1 `bubbles.gaps` trigger probe under the `gaps-to-doc` finding-owned closure rule. There is no user-facing UX or runtime behavior change.

### Acceptance Criteria

- [x] AC-01: `grep -n 'internal/db/migrations/015_domain_extraction.sql\b' specs/026-domain-extraction/scopes.md` returns 0 matches (only the `archive/015_domain_extraction.sql` form remains).
- [x] AC-02: `grep -n 'internal/db/migrations_test.go' specs/026-domain-extraction/scopes.md` returns 0 matches.
- [x] AC-03: `grep -n 'tests/integration/domain_extraction_test.go' specs/026-domain-extraction/scopes.md` returns 0 matches.
- [x] AC-04: `bash .github/bubbles/scripts/state-transition-guard.sh specs/026-domain-extraction` exits 0.
- [x] AC-05: `bash .github/bubbles/scripts/artifact-lint.sh specs/026-domain-extraction` exits 0.
- [x] AC-06: `bash .github/bubbles/scripts/traceability-guard.sh specs/026-domain-extraction` exits 0.
- [x] AC-07: `bash .github/bubbles/scripts/state-transition-guard.sh specs/026-domain-extraction/bugs/BUG-026-005-stale-spec-path-refs` exits 0.
- [x] AC-08: `bash .github/bubbles/scripts/artifact-lint.sh specs/026-domain-extraction/bugs/BUG-026-005-stale-spec-path-refs` exits 0.
- [x] AC-09: `git diff --name-only HEAD` lists only allowed paths (BUG-026-005 packet, parent 026 `scopes.md`/`state.json`).
- [x] AC-10: Single commit with prefix `bubbles(026/bug-026-005): ...`.

### Acceptance Decision

Accepted by sweep-2026-05-24-r10 round 1 finding-owned closure chain. Parent spec 026 stays `status: done` end-to-end; zero runtime regression; all three framework guards continue to PASS on both parent 026 and the BUG-026-005 packet.
