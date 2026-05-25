# Bug: BUG-026-005 Stale code/test path references in spec 026 scopes.md

## Summary

Sweep round 1 of `sweep-2026-05-24-r10` (`mode: stochastic-quality-sweep`, mapped child mode `gaps-to-doc`) ran a `bubbles.gaps` trigger probe on `specs/026-domain-extraction/` and found **4 distinct stale code/test path references** in `specs/026-domain-extraction/scopes.md` that no longer match the canonical implementation surface as of HEAD `773100f1`. The drift is exclusively in spec-artifact narrative; runtime code, tests, scenario manifest, all state-transition-guard / artifact-lint / traceability-guard verdicts on parent spec 026, and `tests/e2e/domain_e2e_test.go::TestE2E_DomainExtraction` itself are all unchanged and remain GREEN. The drift was NOT covered by BUG-026-004's `reconcile-to-doc` scope (which deliberately focused on guard pass/fail, regression-E2E planning rows, G068/G053/G040/G022, and Check 17, not on path-correctness of narrative file references).

The 4 stale references are:

1. **Scopes.md line 58 (and 134):** `internal/db/migrations/015_domain_extraction.sql` — this migration was consolidated into `internal/db/migrations/001_initial_schema.sql` and the original file is preserved at `internal/db/migrations/archive/015_domain_extraction.sql`. The narrative still cites the historical pre-consolidation path.
2. **Scopes.md line 151 (T1-06):** `internal/db/migrations_test.go` — no such file exists. The actual migration column test is `tests/integration/db_migration_test.go::TestMigrations_ArtifactsColumns` (and the sibling `TestMigrations_DomainColumnsExist` plus `TestMigrations_IndexesExist`).
3. **Scopes.md line 776 (T7-05):** `tests/integration/domain_extraction_test.go` — no such file. The canonical end-to-end coverage is `tests/e2e/domain_e2e_test.go::TestE2E_DomainExtraction` (already correctly cited by the existing T7-12 Regression E2E row).
4. **Scopes.md line 777 (T7-06):** same as #3 (the negative-path "article artifact → no domain extraction" case is covered by the same canonical E2E suite).

Two additional path references in scopes.md (`internal/nats/domain_subjects.go` at line 363 and `internal/api/domain_search.go` at line 886) already carry their own explicit "or add to existing client.go" / "(optional, if search.go is too large)" hedging parentheticals — they accurately reflect the original design alternatives where the implementation took the "use existing file" branch. They are NOT in scope of this bug.

## Severity

- [ ] Critical - System unusable, data loss
- [ ] High - Major feature broken
- [ ] Medium - Major functionality impaired
- [x] Low - Spec-artifact narrative drift misleads future readers/agents who grep scopes.md for "where does the migration live?" or "what test covers T1-06?". Runtime, tests, state-transition-guard, artifact-lint, and traceability-guard are all unaffected. `bubbles.gaps` surfaces it because the spec→test→file→evidence chain is the substrate that lets future audits, retros, and onboarding paths trust the spec.

## Status

- [x] Reported
- [x] Confirmed by sweep-2026-05-24-r10 round 1 bubbles.gaps probe
- [ ] In Progress
- [ ] Fixed
- [ ] Verified
- [ ] Closed

## Reproduction Steps

1. From clean HEAD `773100f1`, run the path-drift probe inline:

   ```bash
   python3 -c "
   import re, os
   patt = re.compile(r'\`([a-z]+(?:/[^\`\\s,()<>]+)+\.(?:go|py|sql|yaml|md|ts|tsx|js))\`')
   missing = {}
   with open('specs/026-domain-extraction/scopes.md') as fh:
       for m in patt.finditer(fh.read()):
           p = m.group(1)
           if any(p.startswith(x + '/') for x in ['internal', 'ml', 'tests', 'config', 'cmd', 'docs', 'web', 'deploy', 'scripts']) and not os.path.exists(p):
               missing.setdefault(p, 0); missing[p] += 1
   print(missing)
   "
   ```

2. Observe 4 distinct stale paths (2 of them have intentional "or use existing file" hedging text and are out of scope).
3. `bash .github/bubbles/scripts/state-transition-guard.sh specs/026-domain-extraction` continues to PASS (this drift is not covered by any state-transition check today).
4. `bash .github/bubbles/scripts/artifact-lint.sh specs/026-domain-extraction` continues to PASS.
5. `bash .github/bubbles/scripts/traceability-guard.sh specs/026-domain-extraction` continues to PASS.

## Expected Behavior

- `scopes.md` cites only file paths that exist in the working tree at HEAD (or explicitly marks alternatives with hedging parentheticals).
- `bash .github/bubbles/scripts/state-transition-guard.sh specs/026-domain-extraction` continues to exit 0.
- `bash .github/bubbles/scripts/artifact-lint.sh specs/026-domain-extraction` continues to exit 0.
- `bash .github/bubbles/scripts/traceability-guard.sh specs/026-domain-extraction` continues to exit 0.

## Actual Behavior

- 4 narrative path references in scopes.md point to non-existent files; future grep/audit/onboarding workflows are misled even though the spec→manifest→test chain is intact via `scenario-manifest.json::linkedTests[]`.

## Environment

- Branch: `main`, HEAD `773100f1`
- Sweep: `sweep-2026-05-24-r10` round 1, trigger `gaps`, mapped child mode `gaps-to-doc`, executionModel `parent-expanded-child-mode`
- Parent feature: `specs/026-domain-extraction` (`status: done`)
- Guards baseline: state-transition-guard 🟡 TRANSITION PERMITTED (2 advisory warnings, both pre-existing); artifact-lint PASS; traceability-guard PASS

## Error Output

```text
$ python3 -c "import re,os; patt=re.compile(r'\`([a-z]+(?:/[^\`\\s,()<>]+)+\.(?:go|py|sql|yaml|md|ts|tsx|js))\`'); missing={}; [missing.setdefault(p,0) or missing.update({p:missing[p]+1}) for p in [m.group(1) for m in patt.finditer(open('specs/026-domain-extraction/scopes.md').read())] if any(p.startswith(x+'/') for x in ['internal','ml','tests','config','cmd','docs','web','deploy','scripts']) and not os.path.exists(p)]; print(missing)"
{'internal/db/migrations/015_domain_extraction.sql': 2,
 'internal/db/migrations_test.go': 1,
 'internal/nats/domain_subjects.go': 1,
 'tests/integration/domain_extraction_test.go': 2,
 'internal/api/domain_search.go': 1}
```

(Of these, the two with explicit "or use existing file" hedging parentheticals — `internal/nats/domain_subjects.go` line 363 and `internal/api/domain_search.go` line 886 — are intentional design alternatives and out of scope of this bug.)

## Workaround

None required — runtime is unaffected. The drift is documentation-only.

## Root Cause Analysis (Five Whys)

- **Why are 4 paths stale?** Because the implementation chose alternative file locations / consolidated migration / different test naming after the original spec was authored.
- **Why didn't BUG-026-004 catch it?** BUG-026-004 (`reconcile-to-doc`) focused on guard pass/fail (G022/G040/G053/G068/Check 8A/Check 17) and did not run a path-existence probe on narrative file references.
- **Why didn't sweep rounds 10/19 catch it?** Those rounds ran `regression-to-doc`/`harden-to-doc`/`test-to-doc` triggers that focused on runtime gaps, not on narrative-path correctness.
- **Why isn't state-transition-guard catching it today?** No current check enforces that every backtick-quoted code/test path in scopes.md resolves on disk; this is a `bubbles.gaps` semantic finding, not a mechanical guard failure.
- **Why does the scenario manifest already cite the correct paths?** Because `scenario-manifest.json::linkedTests[]` was authored/refreshed in later sweep rounds (BUG-026-001 fidelity-gap close-out) against actual test functions, while the parent scopes.md narrative was not re-synced.

## Related

- Parent: `specs/026-domain-extraction/` (status: done)
- Sibling bugs: BUG-026-001 (fidelity-gap, closed), BUG-026-002 (E2E timeout, closed), BUG-026-003 (handle-domain-extracted coverage, closed), BUG-026-004 (reconcile-artifact-drift, resolved)
- Sweep ledger entry: `.specify/memory/sweep-2026-05-24-r10.json` round 1 (owned by parent stochastic-quality-sweep workflow)
- Related runtime files (canonical paths the fix points to): `internal/db/migrations/001_initial_schema.sql`, `internal/db/migrations/archive/015_domain_extraction.sql`, `tests/integration/db_migration_test.go`, `tests/e2e/domain_e2e_test.go`

## Product Principle Alignment

This bug enforces **Principle 8 — Trust Through Transparency** (`docs/Product-Principles.md` and the constitution Model Compensations table): the spec → file-path → test chain is the surface that lets future operators, audits, and agents trust that every spec citation is grounded in real, locatable code. When scopes.md cites file paths that no longer exist, the transparency contract degrades even if the underlying runtime is correct.
