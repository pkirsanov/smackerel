# Bug: BUG-024-006 docs/smackerel.md §24-A architecture tree understates connector count (16 vs live 17, missing Card Rewards) + TestConnectorCountContract has a 4th-surface blind spot that lets §24-A drift undetected

## Summary

Round R29 of the stochastic-quality-sweep (`mode: stochastic-quality-sweep`, parent-expanded child mode `gaps-to-doc`) ran the gaps probe on `specs/024-design-doc-reconciliation` and surfaced a genuine committed-tree defect on the exact accuracy dimension spec 024's **R-006 connector-inventory contract** governs for `docs/smackerel.md`. Two paired findings (the IDs `F2`/`F3` mirror the finding *classes* the sibling packets closed — `F2` is the same doc-inventory-drift class BUG-024-002 fixed, `F3` is the same forward-detection-test-gap class BUG-024-003 fixed):

1. **F2 (MEDIUM, PRIMARY — real docs↔runtime drift in `docs/smackerel.md` §24-A).** The §24-A architecture tree header at line 2892 reads `│   ├── Connector plugins (16 committed)` and its 16-leaf list (lines 2893-2908) ends at `└── QF Decisions (qfdecisions/ — spec 041 read-only companion)`. It **omits Card Rewards** (`cardrewards/`, spec 083), which is the 16→17 connector. The same document's §22.7 inventory header (line 2783) says `(17 connectors)` with row 17 = Card Rewards; `docs/Development.md` line 31 says `17 passive connectors` including Card Rewards; and `cmd/core/connectors.go` lines 68-72 register **17** connectors (`cardRewardsConn` at line 72). §24-A is a single committed surface stale at 16 while the other three agree on 17 — exactly the doc↔runtime drift class spec 024 R-006 exists to prevent, and the exact same class BUG-024-002 fixed for §22.7 + §24-A at the 15→16 transition.

2. **F3 (MEDIUM, ROOT CAUSE — `TestConnectorCountContract` has a 4th-surface blind spot).** The forward-detection contract test `internal/deploy/docs_connector_count_contract_test.go` (added by BUG-024-003) pins exactly **three** surfaces via three regexes: `connectorsGoSliceRe` (line 81, the `[]connector.Connector{…}` slice), `smackerelMdHeaderRe` (line 89, the §22.7 `### 22.7 Committed Connector Inventory (N connectors)` header), and `developmentMdBulletRe` (line 95, the `- N passive connectors (` bullet). It does **not** pin §24-A's `Connector plugins (N committed)` line. Because §24-A carries the same connector count but is an **uncovered 4th surface**, it silently drifted to 16 while the three pinned surfaces moved to 17 — and the live-file test still passes GREEN, logging `contract OK: … all agree on 17 connectors` (test line 236). This is the structural reason BUG-024-002's §24-A reconciliation regressed at the 16→17 transition: nothing pins §24-A, so nothing caught it.

> **Finding-ID note:** This packet uses `F2`/`F3` (no `F1`) deliberately, to mark continuity with the sibling finding classes — `F2` ≙ the §22.7/§24-A doc-inventory drift class (BUG-024-002), `F3` ≙ the forward-detection-test-gap class (BUG-024-003). Both findings in this packet are independent and each has its own owner and one-to-one closure.

## Severity

- [ ] Critical — System unusable, data loss
- [ ] High
- [x] Medium — F2 is a real committed docs↔runtime drift on spec 024's owned R-006 accuracy dimension (a reader of §24-A undercounts the shipped connectors by one and never sees Card Rewards); F3 is the forward-detection blind spot that allowed F2 to land undetected and will allow the next §24-A drift to recur. Neither breaks runtime behavior; no production code path, schema, or contract is wrong — only the design-doc surface and its guard.
- [ ] Low

## Status

- [x] Reported
- [x] Confirmed by sweep round R29 gaps-to-doc probe (real commands, captured below)
- [x] In Progress
- [x] Fixed
- [ ] Verified
- [ ] Closed

> **Fix-complete (2026-06-16), pending consolidation.** F3 RED-proved by `bubbles.test` (contract test extended to pin §24-A as a 4th surface) and F2 GREEN-closed by `bubbles.docs` (`docs/smackerel.md` §24-A header `16 committed` → `17 committed` + Card Rewards leaf inserted after QF Decisions). `./smackerel.sh test unit --go --go-run 'TestConnectorCountContract' --verbose` exits 0 with four-surface agreement on 17; `./smackerel.sh check` exits 0. **Verified** and **Closed** remain unchecked because the consolidated path-limited commit and the parent spec 024 governance recertification are deferred to the end-of-sweep `bubbles.devops` pass (stochastic-quality-sweep R29). See report.md → "bubbles.docs — F2 §24-A Reconciliation GREEN Proof".

## Reproduction Steps

1. From clean HEAD `7844283b` on `main`, run `grep -n 'Committed Connector Inventory (\|Connector plugins (' docs/smackerel.md`.
2. Observe two surfaces in the SAME document disagree:
   - `2783:### 22.7 Committed Connector Inventory (17 connectors)`
   - `2892:│   ├── Connector plugins (16 committed)`
3. Print the §24-A leaf block: `awk 'NR>=2892 && NR<=2912' docs/smackerel.md`. Observe 16 leaves (Gov Alerts → QF Decisions) ending at line 2908 `└── QF Decisions (qfdecisions/ — spec 041 read-only companion)`, immediately followed by line 2909 `Planned connectors:`. There is no Card Rewards leaf.
4. Confirm Card Rewards is present in every OTHER surface: `grep -n 'cardrewards\|Card Rewards' docs/smackerel.md cmd/core/connectors.go docs/Development.md` → §22.7 row 17 (`docs/smackerel.md:2805`), `cmd/core/connectors.go:67` (`cardRewardsConn := …`), and `docs/Development.md:31`. The §24-A block (lines 2892-2910) returns zero `cardrewards`/`Card Rewards` matches.
5. Confirm the runtime registers 17: `grep -n 'connector.Connector{\|qfDecisionsConn,\|cardRewardsConn,' cmd/core/connectors.go` → slice literal opens at line 68, `qfDecisionsConn` at line 71, `cardRewardsConn` at line 72.
6. Read `internal/deploy/docs_connector_count_contract_test.go` and confirm its pinned-surface regexes (lines 81, 89, 95) cover connectors.go + §22.7 + Development.md but **not** the §24-A `Connector plugins (N committed)` line. The only mention of `§24-A` in the file is a historical comment at line 12, not an assertion.
7. Run `./smackerel.sh test unit --go --go-run 'TestConnectorCountContract' --verbose`. Observe `TestConnectorCountContract_LiveFile` PASSES with the log `contract OK: cmd/core/connectors.go + docs/smackerel.md §22.7 + docs/Development.md all agree on 17 connectors` — green at 17 **despite §24-A being stale at 16**, because the test never inspects §24-A. Exit code 0.

## Expected Behavior

- `docs/smackerel.md` §24-A header reads `Connector plugins (17 committed)` and the leaf list enumerates all 17 connectors — Card Rewards (`cardrewards/`) appended after QF Decisions to match the §22.7 canonical ordering.
- `awk 'NR>=2892 && NR<=2912' docs/smackerel.md | grep -ic 'card.*reward'` ≥ 1 (Card Rewards present in the §24-A tree).
- All four documented/runtime surfaces agree on 17: §22.7 header, §24-A header, `docs/Development.md` line 31, and `cmd/core/connectors.go` slice literal.
- `internal/deploy/docs_connector_count_contract_test.go` pins §24-A's `Connector plugins (N committed)` line as a **4th** surface in `assertConnectorCountContract`, with an adversarial sub-test that drives the contract RED when §24-A claims a count different from the runtime (e.g. reverts to 16 while the others are 17).
- `./smackerel.sh test unit --go --go-run 'TestConnectorCountContract' --verbose` exits 0 with the live-file test asserting four-surface agreement, and the new adversarial sub-test proving the §24-A surface is now non-tautologically pinned.
- Spec 024 `status` remains `done`. `cmd/core/connectors.go`, `internal/connector/cardrewards/`, schema, NATS topology, web template, prompt contract, Telegram command, deploy script, compose file, and `smackerel.yaml` are unchanged.

## Actual Behavior

- `docs/smackerel.md` §24-A header (line 2892) says `Connector plugins (16 committed)` with a 16-leaf list missing Card Rewards; §22.7 (line 2783), `docs/Development.md` (line 31), and `cmd/core/connectors.go` (lines 68-72) all say 17.
- `TestConnectorCountContract_LiveFile` passes GREEN claiming "all agree on 17" because it pins only 3 surfaces and never reads §24-A. A revert of §24-A to any wrong count (or, as today, its failure to advance from 16 to 17) is undetectable by the existing guard.

## Environment

- Branch: `main`, HEAD `7844283b`
- Sweep: stochastic-quality-sweep round R29, mode `stochastic-quality-sweep`, parent-expanded child mode `gaps-to-doc`, 2026-06-16
- Parent feature: `specs/024-design-doc-reconciliation` (`status: done`; owns the R-006 connector-inventory accuracy contract for `docs/smackerel.md`)
- Source-of-truth: `cmd/core/connectors.go` lines 68-72 (slice literal — 17 connectors wired: …, `qfDecisionsConn` L71, `cardRewardsConn` L72)
- `internal/connector/cardrewards/` is the spec 083 Card Rewards connector (registered at `cmd/core/connectors.go:67`); `internal/connector/photos/` is a photo-library package intentionally not a registered connector
- Drifted surface: `docs/smackerel.md` lines 2892-2908 (§24-A architecture tree); blind-spot surface: `internal/deploy/docs_connector_count_contract_test.go`

## Error Output

```text
$ grep -n 'Committed Connector Inventory (\|Connector plugins (' docs/smackerel.md
2783:### 22.7 Committed Connector Inventory (17 connectors)
2892:│   ├── Connector plugins (16 committed)

$ awk 'NR>=2892 && NR<=2909 {printf "%d: %s\n", NR, $0}' docs/smackerel.md
2892: │   ├── Connector plugins (16 committed)
2893: │   │   ├── Gov Alerts (alerts/)
...  (Bookmarks, Browser History, CalDAV, Discord, GuestHost, Hospitable, IMAP,
      Google Keep, Google Maps, Financial Markets, RSS/Podcasts, Twitter/X, Weather, YouTube) ...
2908: │   │   └── QF Decisions (qfdecisions/ — spec 041 read-only companion)
2909: │   │   Planned connectors:
# 16 leaves; Card Rewards absent; tree closes at QF Decisions then "Planned connectors:"

$ awk 'NR>=2892 && NR<=2910' docs/smackerel.md | grep -ic 'card.*reward\|cardrewards'
0

$ grep -n 'cardrewards\|Card Rewards' docs/smackerel.md cmd/core/connectors.go docs/Development.md
docs/smackerel.md:2805:| 17 | Card Rewards | `cardrewards/` | Finance | ... spec 083 ... |
cmd/core/connectors.go:67:    cardRewardsConn := cardrewardsConnector.New()
docs/Development.md:31:- 17 passive connectors (... Card Rewards rotating-category source via spec 083 read-only fetch)
# Card Rewards present in §22.7 + connectors.go + Development.md, absent from §24-A

$ grep -n 'connector.Connector{\|qfDecisionsConn,\|cardRewardsConn,' cmd/core/connectors.go
68:    for _, c := range []connector.Connector{
71:            discordConn, twitterConn, weatherConn, alertsConn, marketsConn, qfDecisionsConn,
72:            cardRewardsConn,

$ ./smackerel.sh test unit --go --go-run 'TestConnectorCountContract' --verbose
=== RUN   TestConnectorCountContract_LiveFile
    docs_connector_count_contract_test.go:236: contract OK: cmd/core/connectors.go + docs/smackerel.md §22.7 + docs/Development.md all agree on 17 connectors (spec 024 R-006 + BS-004 + AC-5 in sync)
--- PASS: TestConnectorCountContract_LiveFile (0.00s)
--- PASS: TestConnectorCountContract_AdversarialConnectorsGoLow (0.00s)
--- PASS: TestConnectorCountContract_AdversarialSmackerelMdHigh (0.00s)
--- PASS: TestConnectorCountContract_AdversarialDevelopmentMdLow (0.00s)
ok      github.com/smackerel/smackerel/internal/deploy  0.029s
EXIT_CODE=0
# GREEN at 17 while §24-A is stale at 16 — the live test never inspects §24-A (the blind spot)
```

## Workaround

None for F2 — a reader of §24-A undercounts the shipped connectors and never sees Card Rewards; the doc must be reconciled. For F3, contributors adding the next connector must remember to hand-edit §24-A in addition to §22.7 + `docs/Development.md` + the slice literal; that human reminder already failed once here (Card Rewards advanced three surfaces to 17 but left §24-A at 16) and will fail again until §24-A is pinned by the contract test.

## Root Cause Analysis (Five Whys)

- **Why did F2 land?** Because spec 083 added `internal/connector/cardrewards/` and advanced the connector count 16→17. The author updated §22.7, `docs/Development.md`, and the runtime slice (and the contract test's expected count followed to 17), but did **not** update the §24-A architecture tree — it stayed at `(16 committed)` with a 16-leaf list.
- **Why was §24-A missed when the three other surfaces were updated?** Because nothing forces §24-A to move in lockstep. The §22.7 header, `docs/Development.md` bullet, and the slice literal are the three surfaces the contract test pins; a contributor reconciling "what the test checks" naturally updates those three and overlooks §24-A, which the test is silent about.
- **Why is the contract test silent about §24-A?** Because BUG-024-003 (the packet that created the test) pinned exactly the three surfaces that were drifting at the time (`connectors.go`, §22.7, `docs/Development.md`). §24-A had just been reconciled to 16 by BUG-024-002 and was correct on that day, so it was never added to the test's pinned-surface set. The test's own header comment (line 12) even records that BUG-024-002 "reconciled docs/smackerel.md §22.7 + §24-A to the live count of 16" — but only §22.7 got a regex.
- **Why did the GREEN test mask the drift?** Because `TestConnectorCountContract_LiveFile` asserts agreement among only the three pinned surfaces. When all three moved to 17, the test passed and logged "all agree on 17" — a true statement about three surfaces that reads as a four-surface guarantee but is not.
- **Why will this recur without the F3 fix?** Because §24-A and §22.7 will keep carrying the same number by intent, but only §22.7 is mechanically pinned. Every future connector addition is one more opportunity for §24-A to lag. Pinning §24-A as a 4th surface (F3) is the minimum forward-detection that makes the next §24-A drift fail at `./smackerel.sh test unit --go` time instead of waiting for a stochastic sweep round.

## Fix Routing (this packet does NOT fix — discovery + documentation only)

Per `bugfix-fastlane`, the fix is sequenced **test → docs** so the new guard proves RED before the doc edit turns it GREEN (adversarial-first, anti-tautology):

1. **`bubbles.test` (FIRST — extend the contract, RED-prove):** add a 4th pinned surface for §24-A's `Connector plugins (N committed)` line to `assertConnectorCountContract`, plus an adversarial sub-test that drives the contract RED when §24-A disagrees. With the §24-A surface pinned and the doc still at 16, `TestConnectorCountContract_LiveFile` MUST go RED (16 ≠ 17). That RED run is the regression proof.
2. **`bubbles.docs` (SECOND — reconcile §24-A, GREEN):** edit `docs/smackerel.md` §24-A header `(16 committed)` → `(17 committed)` and insert the Card Rewards leaf after QF Decisions (line 2908). The live-file test then goes GREEN (all four surfaces agree on 17).

## Related

- Parent: `specs/024-design-doc-reconciliation/`
- Prior sibling bugs:
  - `specs/024-design-doc-reconciliation/bugs/BUG-024-002-reconcile-artifact-drift/` (reconciled §22.7 + §24-A to 16 at the 15→16 transition; the §24-A surface this packet re-reconciles at 16→17 is the same surface BUG-024-002 last touched)
  - `specs/024-design-doc-reconciliation/bugs/BUG-024-003-dev-doc-connector-drift/` (created `internal/deploy/docs_connector_count_contract_test.go` pinning the 3 surfaces; this packet extends that test with the missing §24-A 4th surface)
  - `specs/024-design-doc-reconciliation/bugs/BUG-024-005-scopes-connector-count-and-state-dup-key/` (residual 15→16 reconciliation drift)
- Source-of-truth spec for the 16→17 connector: `specs/083-*` (Card Rewards Companion; introduced `internal/connector/cardrewards/`)
- Reference pattern (Go contract test + adversarial sub-tests): `internal/deploy/docs_connector_count_contract_test.go` (the file F3 extends), `internal/deploy/monitoring_docs_contract_test.go`, `internal/deploy/compose_contract_test.go` (`repoRoot` helper)
