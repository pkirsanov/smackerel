# BUG-024-006 User Validation

Links: [bug.md](bug.md) | [spec.md](spec.md) | [design.md](design.md) | [scopes.md](scopes.md) | [report.md](report.md)

## Checklist

Discovery-phase items (verified by `bubbles.bug` with real commands at HEAD `7844283b`) are checked `[x]`. Fix-acceptance items are unchecked `[ ]` until the fix owners (`bubbles.test` → `bubbles.docs`) complete them.

### Discovery / Documentation / Root-Cause (this packet — complete)

- [x] DV-01: The defect is real and committed — `grep -n 'Committed Connector Inventory (\|Connector plugins (' docs/smackerel.md` shows §22.7 (line 2783) at `(17 connectors)` and §24-A (line 2892) at `(16 committed)` in the same document.
- [x] DV-02: §24-A omits Card Rewards — the §24-A leaf list (lines 2893-2908) has 16 leaves ending at `└── QF Decisions` (line 2908); `awk 'NR>=2892 && NR<=2910' docs/smackerel.md | grep -ic 'card.*reward\|cardrewards'` returns 0.
- [x] DV-03: The other three surfaces agree on 17 incl. Card Rewards — §22.7 row 17 (`docs/smackerel.md:2805`), `cmd/core/connectors.go:67` (`cardRewardsConn`), and `docs/Development.md:31` all name Card Rewards; the slice literal (lines 68-72) registers 17.
- [x] DV-04: The contract test has a 4th-surface blind spot — its three regexes (lines 81/89/95) pin connectors.go + §22.7 + Development.md; none matches `Connector plugins (N committed)`; the only §24-A mention (line 12) is a historical comment.
- [x] DV-05: The blind spot is proven — `./smackerel.sh test unit --go --go-run 'TestConnectorCountContract' --verbose` exits 0 with `TestConnectorCountContract_LiveFile` logging "all agree on 17" (test line 236) despite §24-A=16.
- [x] DV-06: The 8-artifact packet exists with substantive content (bug.md, spec.md, design.md, scopes.md, report.md, scenario-manifest.json, uservalidation.md, state.json) under `specs/024-design-doc-reconciliation/bugs/BUG-024-006-s24a-connector-tree-blindspot/`.
- [x] DV-07: Root cause documented — Five-Whys in bug.md traces F2 (§24-A not advanced 16→17 when Card Rewards landed) and F3 (§24-A is an unpinned 4th surface, so nothing caught the drift); fix routing recorded test → docs.

### Fix Acceptance (owned downstream — pending)

- [ ] AC-01: `grep -n 'Connector plugins (' docs/smackerel.md` returns `(17 committed)` (owner: bubbles.docs).
- [ ] AC-02: `awk 'NR>=2892 && NR<=2913' docs/smackerel.md | grep -ic 'card.*reward'` ≥ 1; §24-A enumerates 17 connectors with Card Rewards after QF Decisions (owner: bubbles.docs).
- [ ] AC-03: All four surfaces agree on 17 — §22.7, §24-A, `docs/Development.md` L31, `cmd/core/connectors.go` slice (owner: bubbles.docs).
- [ ] AC-04: `internal/deploy/docs_connector_count_contract_test.go` pins §24-A's `Connector plugins (N committed)` line as a 4th surface, with a new adversarial sub-test (owner: bubbles.test).
- [ ] AC-05: Pre-fix RED proof captured — the §24-A pin makes `TestConnectorCountContract_LiveFile` fail at §24-A=16 vs runtime=17 BEFORE the doc edit (owner: bubbles.test).
- [ ] AC-06: Post-fix GREEN proof captured — `./smackerel.sh test unit --go --go-run 'TestConnectorCountContract' --verbose` exits 0 with all sub-tests passing AFTER the doc edit (owner: bubbles.docs).
- [ ] AC-07: All four framework guards green on parent spec 024 and on the BUG-024-006 packet folder; spec 024 stays `status: done` (owner: bubbles.docs).
- [ ] AC-08: Parent governance backfilled — `resolvedBugs[]` extended with BUG-024-006, `lastUpdatedAt` bumped, parent `report.md` carries a `## BUG-024-006 …` section (owner: bubbles.docs).
- [ ] AC-09: Single atomic commit `bubbles(024/bug-024-006):` with path-limited `git add`; `git push origin main` without `--no-verify` (owner: bubbles.docs).
- [ ] AC-10: `cmd/core/connectors.go`, `internal/connector/cardrewards/`, schema, NATS topology, web template, prompt contract, Telegram command, deploy script, compose file, and `smackerel.yaml` unchanged (owner: bubbles.docs).

## One-To-One Finding Closure Accounting

- **F2 (MEDIUM, §24-A docs↔runtime drift):** documented in this packet; to be closed by FR-01 (bubbles.docs).
- **F3 (MEDIUM, contract-test 4th-surface blind spot):** documented in this packet; to be closed by FR-02 (bubbles.test).

Both findings are accounted for in a single packet with explicit owners; no cherry-picking. This packet is a discovery + documentation deliverable: `bubbles.bug` verified the defect with real commands and routed the fix without performing it.

— bubbles.bug (autonomous), `bugfix-fastlane` mode, stochastic-quality-sweep round R29 (gaps-to-doc), 2026-06-16
