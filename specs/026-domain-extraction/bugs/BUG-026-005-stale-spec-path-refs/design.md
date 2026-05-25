# Design: BUG-026-005 Stale code/test path references in spec 026 scopes.md

Links: [bug.md](bug.md) | [spec.md](spec.md) | [scopes.md](scopes.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

## Current Truth

Solution-blind probe at HEAD `773100f1` (parent spec 026 status: done):

- `specs/026-domain-extraction/scopes.md` cites 4 file paths that don't exist:
  - Line 58: `**SQL (`internal/db/migrations/015_domain_extraction.sql`):**`
  - Line 134: `- `internal/db/migrations/015_domain_extraction.sql` — migration adding domain columns and indexes`
  - Line 151: T1-06 cites `internal/db/migrations_test.go`
  - Lines 776, 777: T7-05 and T7-06 cite `tests/integration/domain_extraction_test.go`
- Actual canonical paths on disk:
  - `internal/db/migrations/archive/015_domain_extraction.sql` (preserved historical file)
  - `internal/db/migrations/001_initial_schema.sql` (consolidates spec 026 migration)
  - `tests/integration/db_migration_test.go::TestMigrations_ArtifactsColumns` (migration column verification test that runs on live PostgreSQL)
  - `tests/integration/db_migration_test.go::TestMigrations_DomainColumnsExist` (the dedicated domain-columns test)
  - `tests/integration/db_migration_test.go::TestMigrations_IndexesExist` (the GIN-index verification test)
  - `tests/e2e/domain_e2e_test.go::TestE2E_DomainExtraction` (the canonical end-to-end recipe-vs-article integration test)
- `scenario-manifest.json` already cites the canonical paths via `linkedTests[]` — the drift is exclusively in scopes.md narrative.
- All three framework guards (state-transition-guard.sh, artifact-lint.sh, traceability-guard.sh) on `specs/026-domain-extraction/` continue to PASS at HEAD `773100f1`.
- BUG-026-004 (the most recent reconciliation bug) explicitly took FR-04 "No runtime code path, schema, API contract, NATS topology, config value, web template, prompt contract, or Telegram command may be changed by this packet" — and likewise did not touch narrative path references in scopes.md.

## Architecture Decision

**Decision:** Update the 4 stale narrative path references in-place. Do not regenerate, re-author, or refactor scopes.md.

**Rationale:**
- The drift is purely textual: scopes.md narrative cites old paths from when the original spec was authored, but the canonical implementation moved to alternative locations during normal repo evolution (migration consolidation, integration vs E2E test placement).
- The scenario manifest (the machine-readable source of truth for scenario → test linkage) already cites the correct paths.
- Surgical edits preserve all other scopes.md content (Gherkin scenarios, DoD bullets, Test Plan rows for other scopes, Code Surfaces lists, etc.) which BUG-026-004 verified are gate-compliant.
- Zero runtime impact: no code, migration, schema, NATS contract, prompt contract, ML sidecar, web template, Telegram command, or config value is touched.

**Alternatives considered:**
- **Reopen BUG-026-004** — Rejected. BUG-026-004 is resolved and its scope was deliberately bounded to G022/G040/G053/G068/Check 8A/Check 17. Re-opening it would dilute its closure record.
- **Promote to a new spec** — Rejected. The change is a documentation-only narrative fix entirely contained within one spec; a full new spec would be over-governance.
- **Skip the fix because guards pass** — Rejected. The `bubbles.gaps` probe surfaces it precisely because guards don't cover narrative path correctness; under the `gaps-to-doc` finding-owned closure rule, every finding gets a bug packet and a closure chain.
- **Also fix the 2 design-alternative references** (`internal/nats/domain_subjects.go` line 363, `internal/api/domain_search.go` line 886) — Rejected. Both already carry explicit "or use existing file" hedging parentheticals and accurately reflect the original planned design alternatives. They are intentional, not stale.

## Approach

**Implementation Strategy:**
1. Use `multi_replace_string_in_file` to apply 4 targeted in-place edits to `specs/026-domain-extraction/scopes.md`:
   - Edit 1: Update line 58 SQL header block to reference `archive/015_domain_extraction.sql` with consolidation note.
   - Edit 2: Update line 134 narrative bullet to the same canonical form.
   - Edit 3: Update line 151 T1-06 to cite `tests/integration/db_migration_test.go::TestMigrations_ArtifactsColumns` (and change "unit" to "integration" since the migration column test runs on a live PostgreSQL).
   - Edit 4: Update lines 776, 777 T7-05/T7-06 to cite `tests/e2e/domain_e2e_test.go::TestE2E_DomainExtraction` (and change "integration" to "e2e" to match the canonical test location).
2. Update parent spec 026 `state.json::resolvedBugs[]` to register the BUG-026-005 close-out entry and advance `lastUpdatedAt`.
3. Run all three guards on both parent 026 and the BUG-026-005 packet; verify each exits 0.
4. Path-limited git stage of only the BUG-026-005 packet (6 files) + parent 026 scopes.md + parent 026 state.json.
5. Commit with prefix `bubbles(026/bug-026-005): ...`.
6. Push (pre-push hook runs full validation; no `--no-verify`).

**Why this approach is safe:**
- Zero touch of `internal/`, `cmd/`, `ml/`, `web/`, `tests/`, `deploy/`, `config/`, `scripts/`, or any other runtime surface.
- Zero touch of any other spec folder.
- Zero touch of `.specify/memory/sweep-2026-05-24-r10.json` (parent stochastic-quality-sweep workflow owns it).
- All four edits are mechanical text replacements verified against the actual on-disk implementation paths.

## Risk Analysis

- **Risk: guard regressions.** Mitigation: Run all three guards on parent 026 and the BUG-026-005 folder before commit; rollback if any new BLOCK appears.
- **Risk: scope creep into the 2 design-alternative references.** Mitigation: Explicitly out-of-scope per spec.md FR-08 and Non-Goals.
- **Risk: stale paths re-introduced by future spec authors.** Mitigation: Out of scope of this bug; could be addressed by a future framework gate that probes path-existence in narrative file references (would be a separate Bubbles framework change).
- **Risk: PII leakage in evidence blocks.** Mitigation: Every captured terminal evidence block redacts `/home/<user>/` to `~/` per repo PII policy before staging.

## Test Strategy

- **Verification probe:** Re-run the inline path-drift probe (`python3` one-liner from bug.md Reproduction Steps step 1) and confirm the 4 in-scope paths are gone, leaving only the 2 documented design-alternative paths.
- **Guards:** Run `state-transition-guard.sh`, `artifact-lint.sh`, `traceability-guard.sh` on both parent 026 and the BUG-026-005 folder. All must exit 0.
- **Regression test for runtime:** None — zero runtime change. `tests/e2e/domain_e2e_test.go::TestE2E_DomainExtraction` remains GREEN by construction (last verified GREEN in sweep-2026-05-23-r30 rounds 10/19 and validated via runtime probe in BUG-026-002/003 close-outs).
- **Anti-regression scope:** After applying edits, confirm no other content in scopes.md was inadvertently changed; `git diff specs/026-domain-extraction/scopes.md` must show only the 4 targeted hunks.

## Implementation Notes

- The migration consolidation reflects normal repo evolution: when migrations 002-015 were merged into a single `001_initial_schema.sql` for clarity, the original files were preserved under `archive/` rather than deleted, so the historical content remains discoverable. This is a deliberate repo pattern, not a deletion.
- T1-06 in the existing scopes.md was labeled "unit" — this was incorrect even before consolidation, because migration application requires a real database connection. Updating the row to "integration" plus the actual integration test path fixes both the path and the label.
- T7-05 and T7-06 were labeled "integration" but the canonical end-to-end domain extraction test was placed under `tests/e2e/` because it requires the full live stack (Postgres + NATS + ML sidecar + Ollama). Updating both rows to "e2e" plus the canonical E2E test path fixes both the path and the label.
- The Test Plan T1-07 row already correctly cites `tests/integration/db_migration_test.go` with the right tests; T1-06's drift is genuinely just a stale row that should have been merged into T1-07 at consolidation time but was left behind.

## Code Diff Evidence

This bug packet edits only narrative file references in `specs/026-domain-extraction/scopes.md` and `specs/026-domain-extraction/state.json::resolvedBugs[]` + `lastUpdatedAt`. There is no runtime code, schema, migration, NATS contract, prompt contract, ML sidecar, web template, Telegram command, or config diff to capture. The verbatim 4 scopes.md hunks are documented in `scopes.md` Definition of Done evidence blocks and in `report.md`.
