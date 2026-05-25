# Spec: BUG-026-005 Stale code/test path references in spec 026 scopes.md

Links: [bug.md](bug.md) | [design.md](design.md) | [scopes.md](scopes.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

This file restates the bug's specification surface in spec form so `bash .github/bubbles/scripts/artifact-lint.sh` and `bash .github/bubbles/scripts/state-transition-guard.sh` can read a `spec.md` next to the rest of the 6-artifact set.

## Business Context

`specs/026-domain-extraction/` is currently `status: done` and all framework guards pass at HEAD `773100f1`. Sweep round 1 of `sweep-2026-05-24-r10` ran the `bubbles.gaps` trigger probe per the mapped `gaps-to-doc` child workflow and surfaced 4 distinct stale file-path references in `specs/026-domain-extraction/scopes.md` that no longer match the canonical implementation. This drift was not in scope of BUG-026-004 (which focused on guard pass/fail) and does not break any framework check, but it makes the spec→file→test chain less trustworthy for future audits and onboarding. Per the `gaps-to-doc` mode contract, each finding must be tracked through a bug packet with a full 6-artifact set and the finding-owned planning + delivery closure chain.

## Use Cases

- **UC-01:** An engineer greps `specs/026-domain-extraction/scopes.md` for `internal/db/migrations/` to locate the domain migration; the cited path must resolve to a file that exists in the working tree (or carry explicit hedging text).
- **UC-02:** An engineer reads Test Plan rows T1-06, T7-05, T7-06 and follows the cited test path; the path must resolve to an actual test file with the documented behavior.
- **UC-03:** A future `bubbles.gaps` probe on spec 026 must not re-surface the same 4 path-drift findings — they should be permanently fixed.
- **UC-04:** All three framework guards (`state-transition-guard.sh`, `artifact-lint.sh`, `traceability-guard.sh`) on `specs/026-domain-extraction/` must continue to PASS after the fix.

## Functional Requirements

- **FR-01:** `specs/026-domain-extraction/scopes.md` line 58 path reference MUST be updated to `internal/db/migrations/archive/015_domain_extraction.sql` (with a parenthetical noting consolidation into `internal/db/migrations/001_initial_schema.sql`).
- **FR-02:** `specs/026-domain-extraction/scopes.md` line 134 narrative reference MUST be updated to the same canonical form.
- **FR-03:** `specs/026-domain-extraction/scopes.md` line 151 (Test Plan row T1-06) MUST be updated to cite `tests/integration/db_migration_test.go::TestMigrations_ArtifactsColumns`.
- **FR-04:** `specs/026-domain-extraction/scopes.md` lines 776, 777 (Test Plan rows T7-05, T7-06) MUST be updated to cite `tests/e2e/domain_e2e_test.go::TestE2E_DomainExtraction`.
- **FR-05:** No runtime code, migration, schema, NATS contract, prompt contract, ML sidecar, web template, Telegram command, or config value may be changed by this packet.
- **FR-06:** No file outside `specs/026-domain-extraction/` and `specs/026-domain-extraction/bugs/BUG-026-005-stale-spec-path-refs/` may be changed by this packet.
- **FR-07:** Parent spec 026 stays `status: done` end-to-end; its `resolvedBugs[]` and `lastUpdatedAt` get the BUG-026-005 close-out entry.

## Gherkin Scenarios

See `scopes.md` for the full Gherkin set. Scenarios cover:

- SCN-B0265-01: scopes.md migration-file references (lines 58, 134) updated to canonical `archive/015_domain_extraction.sql` form with consolidation note.
- SCN-B0265-02: scopes.md migration-test reference (T1-06) updated to `tests/integration/db_migration_test.go::TestMigrations_ArtifactsColumns`.
- SCN-B0265-03: scopes.md domain-extraction integration-test references (T7-05, T7-06) updated to `tests/e2e/domain_e2e_test.go::TestE2E_DomainExtraction`.
- SCN-B0265-04: All three guards (`state-transition-guard.sh`, `artifact-lint.sh`, `traceability-guard.sh`) on `specs/026-domain-extraction/` continue to PASS.
- SCN-B0265-05: A re-run of the path-drift probe on scopes.md returns 0 in-scope stale paths (the 2 design-alternative paths with hedging parentheticals are explicitly out of scope and remain).

## Acceptance Criteria

- **AC-01:** `grep -n 'internal/db/migrations/015_domain_extraction.sql\b' specs/026-domain-extraction/scopes.md` returns 0 matches (only the `archive/015_domain_extraction.sql` form remains).
- **AC-02:** `grep -n 'internal/db/migrations_test.go' specs/026-domain-extraction/scopes.md` returns 0 matches.
- **AC-03:** `grep -n 'tests/integration/domain_extraction_test.go' specs/026-domain-extraction/scopes.md` returns 0 matches.
- **AC-04:** `bash .github/bubbles/scripts/state-transition-guard.sh specs/026-domain-extraction` exits 0.
- **AC-05:** `bash .github/bubbles/scripts/artifact-lint.sh specs/026-domain-extraction` exits 0.
- **AC-06:** `bash .github/bubbles/scripts/traceability-guard.sh specs/026-domain-extraction` exits 0.
- **AC-07:** `bash .github/bubbles/scripts/state-transition-guard.sh specs/026-domain-extraction/bugs/BUG-026-005-stale-spec-path-refs` exits 0.
- **AC-08:** `bash .github/bubbles/scripts/artifact-lint.sh specs/026-domain-extraction/bugs/BUG-026-005-stale-spec-path-refs` exits 0.
- **AC-09:** `git diff --name-only HEAD` lists only allowed paths (this bug's packet, parent 026 `scopes.md`/`state.json`).
- **AC-10:** Single commit with prefix `bubbles(026/bug-026-005): ...` (satisfies Check 17 structured commit gate for spec 026 and registers the bug close-out).

## Product Principle Alignment

This bug enforces **Principle 8 — Trust Through Transparency** (per `docs/Product-Principles.md` and the constitution Model Compensations table): the spec → file → test chain is the surface that lets future operators trust that every "Done" claim is grounded in locatable, real evidence. When scopes.md cites file paths that no longer exist, the transparency contract degrades even if the underlying runtime is correct.

## Non-Goals

- Modifying any runtime code, migration, schema, NATS contract, prompt contract, ML sidecar, web template, Telegram command, or config value.
- Re-opening parent spec 026. Parent stays `status: done` end-to-end.
- Modifying the 2 out-of-scope design-alternative references (`internal/nats/domain_subjects.go` line 363, `internal/api/domain_search.go` line 886) which already carry explicit "or use existing file" hedging parentheticals — they accurately reflect the original planned design alternatives.
- Updating `internal/db/migrations/001_initial_schema.sql`, `tests/integration/db_migration_test.go`, or `tests/e2e/domain_e2e_test.go` themselves. All three are correct and remain GREEN.
- Touching `specs/054-notification-intelligence-handler/`, `specs/055-notification-source-ntfy-adapter/`, or any other in-flight WIP across the repo.
- Updating `.specify/memory/sweep-2026-05-24-r10.json` (the parent stochastic-quality-sweep workflow owns the ledger).
