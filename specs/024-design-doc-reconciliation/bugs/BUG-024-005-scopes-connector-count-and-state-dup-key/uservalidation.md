# BUG-024-005 User Validation

Links: [bug.md](bug.md) | [spec.md](spec.md) | [design.md](design.md) | [scopes.md](scopes.md) | [scenario-manifest.json](scenario-manifest.json) | [report.md](report.md)

## Checklist

- [x] AC-01: `state.json` has exactly one top-level `lastUpdatedAt`; duplicate-key detector reports none. Verified: `raw.count('"lastUpdatedAt"') == 1`; `object_pairs_hook` detector returns `[]` (pre-fix was 2 + flagged `['lastUpdatedAt']`).
- [x] AC-02: `state.json` `certifiedAt` and `lastUpdatedAt` == `2026-06-06T18:00:00Z`; a `bubbles.spec-review` `reviewStatus: CURRENT` `executionHistory` entry is present; `requiresRevalidation` is absent. Verified via `python3` probe.
- [x] AC-03: The 8 substantive Scope 2 connector-count claims read "16"; `SCN-024-06` body lists `qfdecisions` (between `markets` and `rss`); the 7 historical/rollback/grep-contract "15" references (rollback `git revert` contract ×2, stale-ref grep searches, BUG-024-002 red-phase evidence, consumer-impact "zero stale 15-refs" assertions ×3) are preserved verbatim.
- [x] AC-04: `traceability-guard.sh specs/024-design-doc-reconciliation` exits 0 `RESULT: PASSED (0 warnings)`; STG Check 22 (G068) reports `All 6 Gherkin scenarios have faithful DoD items`. The `SCN-024-06` Gherkin title and DoD bullet were changed in lockstep.
- [x] AC-05: `artifact-lint.sh specs/024-design-doc-reconciliation` exits 0 `Artifact lint PASSED.`
- [x] AC-06: `artifact-freshness-guard.sh specs/024-design-doc-reconciliation` exits 0 `RESULT: PASS (0 failures, 0 warnings)`.
- [x] AC-07: `go test -run TestConnectorCountContract ./internal/deploy/...` exits 0 with all 4 sub-tests PASS (LiveFile + AdversarialConnectorsGoLow + AdversarialSmackerelMdHigh + AdversarialDevelopmentMdLow). BUG-024-003 forward-detection contract preserved verbatim — zero edits to `internal/deploy/docs_connector_count_contract_test.go`.
- [x] AC-08: Spec 024 `status` remains `done`; `certification.status`, `completedScopes` `["1","2"]`, `certifiedCompletedPhases`, and both per-scope `certifiedAt` entries (2026-04-10T14:00:00Z + 2026-04-10T14:30:00Z) preserved verbatim.
- [x] AC-09: `resolvedBugs[]` contains a BUG-024-005 entry with `finalStatus: "resolved"`; `report.md` has exactly 1 `## BUG-024-005 Harden-Sweep Resolution` section with Code Diff Evidence + Git-Backed Proof; zero `/home/<user>/` paths (verified `grep -cE '/home/<user>/' ... report.md` == 0).
- [x] AC-10: BUG-024-005 packet's own gate passes: `artifact-lint.sh specs/024-design-doc-reconciliation/bugs/BUG-024-005-scopes-connector-count-and-state-dup-key` exits 0 `Artifact lint PASSED.`
- [x] AC-11: G088 / STG Check 30 honestly report the uncommitted `scopes.md` worktree edit (`postCertEdits=1`, `certifiedAt=2026-06-06T18:00:00Z`) — the designed pre-commit handoff state that clears when the parent commits before 18:00:00Z. This is NOT faked green; the report.md captures the real `🔴 BLOCK` output and explains the clear-on-commit mechanism.
- [x] AC-12: Nothing committed or pushed; `git status --short specs/024-design-doc-reconciliation/` shows 3 modified parent files + the new BUG-024-005 packet (untracked); HEAD unchanged.

## One-To-One Finding Closure Accounting

- **F1 (LOW, state.json duplicate top-level `lastUpdatedAt`) closed:** removed the legacy `2026-05-25T00:00:00Z` key after `failures[]`; recertified header `certifiedAt`/`lastUpdatedAt` → `2026-06-06T18:00:00Z`. Post-fix `lastUpdatedAt` literal count 2 → 1; duplicate-key detector reports NONE.
- **F2 (MEDIUM, scopes.md Scope-2 15→16 connector drift) closed:** reconciled 8 substantive sites to 16; inserted `qfdecisions` alphabetically in `SCN-024-06` body; preserved 7 historical "15" references. G068 fidelity kept green by lockstep `SCN-024-06` title + DoD edit; `SCN-024-06` now agrees with `spec.md`, `scenario-manifest.json`, `docs/smackerel.md` §22.7, and the live registry.

2 findings, 2 closed, 0 unresolved.

The packet honors the harden-to-doc NON-NEGOTIABLE governance:
- Full closure chain executed (bug → analyze → design → plan → harden → implement → test → regression → simplify → stabilize → security → chaos → validate → audit → docs → finalize + spec-review CURRENT).
- Parent spec 024 `state.json` + `report.md` extended with harden-round closure evidence.
- BUG-024-003 forward-detection contract test preserved verbatim (`TestConnectorCountContract` 4/4 PASS).
- Per the round hard rule, NO COMMIT and NO PUSH — all changes left in the working tree for the parent stochastic-quality-sweep / operator. Committing before `certifiedAt=2026-06-06T18:00:00Z` clears Gate G088.

I accept the closure and authorize sweep round 7 to be marked `completed_owned` with this packet as the recorded evidence.

— bubbles.workflow (autonomous), parent-expanded `harden-to-doc` mode, sweep-2026-06-06-r20b round 7, 2026-06-06
