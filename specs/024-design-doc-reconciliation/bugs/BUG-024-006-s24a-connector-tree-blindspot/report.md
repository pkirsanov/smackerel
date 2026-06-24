# Report: BUG-024-006 — §24-A architecture-tree connector-count drift (16 vs live 17) + TestConnectorCountContract 4th-surface blind spot

## Summary

**Sweep round:** stochastic-quality-sweep round R29 (mode: `stochastic-quality-sweep`, parent-expanded child mode: `gaps-to-doc`), 2026-06-16.
**Parent spec:** `specs/024-design-doc-reconciliation` (status stays `done`; owns the R-006 connector-inventory accuracy contract for `docs/smackerel.md`).
**Bug status:** `in_progress` — discovery + documentation + root-cause complete; fix routed downstream (test → docs).
**Workflow mode:** `bugfix-fastlane`.
**Findings:** F2 (MEDIUM, primary) + F3 (MEDIUM, root cause), to be closed one-to-one by their owners.

The gaps probe on spec 024 discovered a genuine committed-tree defect on the exact accuracy dimension R-006 governs:

- **F2 (MEDIUM, docs↔runtime drift — owner `bubbles.docs`):** `docs/smackerel.md` §24-A architecture-tree header (line 2892) reads `Connector plugins (16 committed)` and its 16-leaf list (lines 2893-2908) ends at QF Decisions, omitting Card Rewards (`cardrewards/`, spec 083). The SAME document's §22.7 header (line 2783) says `(17 connectors)`, `docs/Development.md` line 31 says `17 passive connectors`, and `cmd/core/connectors.go` lines 68-72 register 17. §24-A is the single surface stale at 16.
- **F3 (MEDIUM, missing forward-detection — owner `bubbles.test`):** `internal/deploy/docs_connector_count_contract_test.go` pins only 3 surfaces (slice literal regex line 81, §22.7 header regex line 89, `docs/Development.md` bullet regex line 95). §24-A's `Connector plugins (N committed)` line is an uncovered 4th surface, so it silently drifted to 16 while the 3 pinned surfaces moved to 17 — and `TestConnectorCountContract_LiveFile` still passes GREEN, logging "all agree on 17" because it never inspects §24-A.

The defect classes mirror the two sibling closures: F2 is the docs↔runtime drift class BUG-024-002 fixed (for §22.7 + §24-A at 15→16); F3 is the forward-detection-test-gap class BUG-024-003 fixed (by creating the test this packet now extends).

## Completion Statement

The discovery, documentation, and root-cause analysis for BUG-024-006 are **complete and verified by real commands** (captured under Test Evidence below at HEAD `7844283b`, `main`). This packet (the `bubbles.bug` discovery phase) performs **no** code or documentation edits and does **not** commit — per the `bugfix-fastlane` mandate, the fix is owned downstream and sequenced test → docs:

1. **`bubbles.test` (FR-02, root cause F3):** extend `internal/deploy/docs_connector_count_contract_test.go` to pin §24-A's `Connector plugins (N committed)` line as a 4th surface in `assertConnectorCountContract` (`smackerelMdTreeRe` + `parseSmackerelMdTreeCount` + a new `TestConnectorCountContract_AdversarialSmackerelMdTreeLow` sub-test), then capture the RED live-file run that the still-stale §24-A=16 vs runtime=17 produces.
2. **`bubbles.docs` (FR-01, primary F2):** reconcile `docs/smackerel.md` §24-A (header line 2892 `16 committed` → `17 committed`; line 2908 glyph; insert the Card Rewards leaf after QF Decisions), then capture the GREEN run, backfill parent governance, and land a single atomic `bubbles(024/bug-024-006):` commit.

Bug `state.json::status` remains `in_progress` (certification.status `in_progress`); `nextRequiredOwner` is `bubbles.test`. No DoD item is marked complete and no completion is claimed for the fix, because the fix has not been performed by this agent. Spec 024 stays `status: done` throughout.

## Test Evidence

### Discovery Evidence — defect verification (captured by `bubbles.bug`, pre-fix, HEAD `7844283b`)

The two disagreeing surfaces inside the SAME document, the omitted Card Rewards leaf, the 3-surface agreement at 17, the runtime slice at 17, and the contract test's pinned-surface set:

```text
$ grep -n 'Committed Connector Inventory (\|Connector plugins (' docs/smackerel.md
2783:### 22.7 Committed Connector Inventory (17 connectors)
2892:│   ├── Connector plugins (16 committed)

$ awk 'NR>=2892 && NR<=2909 {printf "%d: %s\n", NR, $0}' docs/smackerel.md
2892: │   ├── Connector plugins (16 committed)
2893: │   │   ├── Gov Alerts (alerts/)
2894: │   │   ├── Bookmarks (bookmarks/)
2895: │   │   ├── Browser History (browser/)
2896: │   │   ├── CalDAV (caldav/ — go-webdav)
2897: │   │   ├── Discord (discord/ — discordgo)
2898: │   │   ├── GuestHost (guesthost/)
2899: │   │   ├── Hospitable (hospitable/)
2900: │   │   ├── IMAP Email (imap/ — go-imap v2)
2901: │   │   ├── Google Keep (keep/)
2902: │   │   ├── Google Maps (maps/)
2903: │   │   ├── Financial Markets (markets/)
2904: │   │   ├── RSS/Podcasts (rss/ — gofeed)
2905: │   │   ├── Twitter/X (twitter/)
2906: │   │   ├── Weather (weather/)
2907: │   │   ├── YouTube (youtube/)
2908: │   │   └── QF Decisions (qfdecisions/ — spec 041 read-only companion)
2909: │   │   Planned connectors:
# 16 leaves (Gov Alerts → QF Decisions); tree closes at QF Decisions; Card Rewards absent

$ awk 'NR>=2892 && NR<=2910' docs/smackerel.md | grep -ic 'card.*reward\|cardrewards'
0

$ grep -n 'cardrewards\|Card Rewards' docs/smackerel.md cmd/core/connectors.go docs/Development.md
docs/smackerel.md:2805:| 17 | Card Rewards | `cardrewards/` | Finance | ... (spec 083) ... no financial advice |
cmd/core/connectors.go:67:    cardRewardsConn := cardrewardsConnector.New()
docs/Development.md:31:- 17 passive connectors (... Card Rewards rotating-category source via spec 083 read-only fetch)
# Card Rewards present in §22.7 (row 17) + connectors.go + Development.md; absent only from §24-A

$ grep -n 'connector.Connector{\|qfDecisionsConn,\|cardRewardsConn,' cmd/core/connectors.go
68:    for _, c := range []connector.Connector{
71:            discordConn, twitterConn, weatherConn, alertsConn, marketsConn, qfDecisionsConn,
72:            cardRewardsConn,
# runtime slice = 17 connectors (qfDecisionsConn L71 + cardRewardsConn L72)
```

The contract test's pinned-surface set — three regexes, none matching the §24-A `Connector plugins (N committed)` line:

```text
$ grep -in 'committed\|Connector plugins\|24-A' internal/deploy/docs_connector_count_contract_test.go
12:// reconciled docs/smackerel.md §22.7 + §24-A to the live count of 16 but   <-- historical COMMENT only
34://     "### 22.7 Committed Connector Inventory (N connectors)" — N MUST
89:var smackerelMdHeaderRe = regexp.MustCompile(`### 22\.7 Committed Connector Inventory \((\d+) connectors\)`)
# No regex matches "Connector plugins (N committed)" — §24-A is the uncovered 4th surface

$ grep -nE 'regexp\.MustCompile' internal/deploy/docs_connector_count_contract_test.go
81:var connectorsGoSliceRe = regexp.MustCompile(`\[\]connector\.Connector\{([^}]*)\}`)
89:var smackerelMdHeaderRe = regexp.MustCompile(`### 22\.7 Committed Connector Inventory \((\d+) connectors\)`)
95:var developmentMdBulletRe = regexp.MustCompile(`(?m)^- (\d+) passive connectors \(`)
```

### Blind-spot proof — the live test passes GREEN at 17 while §24-A is stale at 16

`./smackerel.sh test unit --go --go-run 'TestConnectorCountContract' --verbose` (exit 0). The live-file test logs "all agree on 17" because it only inspects the 3 pinned surfaces; §24-A=16 is never read:

```text
=== RUN   TestConnectorCountContract_LiveFile
    docs_connector_count_contract_test.go:236: contract OK: cmd/core/connectors.go + docs/smackerel.md §22.7 + docs/Development.md all agree on 17 connectors (spec 024 R-006 + BS-004 + AC-5 in sync)
--- PASS: TestConnectorCountContract_LiveFile (0.00s)
=== RUN   TestConnectorCountContract_AdversarialConnectorsGoLow
--- PASS: TestConnectorCountContract_AdversarialConnectorsGoLow (0.00s)
=== RUN   TestConnectorCountContract_AdversarialSmackerelMdHigh
--- PASS: TestConnectorCountContract_AdversarialSmackerelMdHigh (0.00s)
=== RUN   TestConnectorCountContract_AdversarialDevelopmentMdLow
--- PASS: TestConnectorCountContract_AdversarialDevelopmentMdLow (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/deploy  0.029s
[go-unit] go test ./... finished OK
EXIT_CODE=0
```

**Interpretation (Claim Source: executed):** the GREEN result is the defect — a four-surface accuracy contract is being asserted across only three surfaces, so the §24-A drift to 16 is invisible to the guard. This is the exact blind spot F3 closes.

### Fix-phase evidence (owned downstream — recorded by the fix owners, not this agent)

The RED→GREEN proof is produced by the fix owners and will live in this section / the parent report:

- **RED (owner `bubbles.test`):** after the §24-A surface is pinned and BEFORE the doc edit, `./smackerel.sh test unit --go --go-run 'TestConnectorCountContract' --verbose` fails the live-file test with a `docs/smackerel.md §24-A=16` vs runtime=17 diagnostic.
- **GREEN (owner `bubbles.docs`):** after the §24-A header→`(17 committed)` + Card Rewards leaf edit, the same command exits 0 with four-surface agreement.

This is the anti-tautology guarantee — the new guard is demonstrated catching the live drift before the doc edit silences it. (Claim Source: not-run by `bubbles.bug` — owned by the fix phase.)

## Finding Closure Accounting (one-to-one)

- **F2 (MEDIUM, §24-A docs↔runtime drift):** to be closed by FR-01 (`bubbles.docs` — §24-A header 16→17 + Card Rewards leaf). Verification: `grep -n 'Connector plugins (' docs/smackerel.md` → `(17 committed)`; Card Rewards leaf present.
- **F3 (MEDIUM, contract-test 4th-surface blind spot):** to be closed by FR-02 (`bubbles.test` — §24-A 4th-surface pin + adversarial sub-test). Verification: the new adversarial sub-test rejects synthetic §24-A=16 vs runtime=17; the live-file test asserts four-surface agreement.

Both findings are routed in the same packet; no cherry-picking, no easy-subset remediation.

## bubbles.test — F3 Test-Extension RED Proof (Layer 1, 2026-06-16)

**Owner:** `bubbles.test`. **Finding addressed:** F3 (the forward-detection 4th-surface blind spot). **Surface touched:** `internal/deploy/docs_connector_count_contract_test.go` ONLY. `docs/smackerel.md` §24-A was deliberately left stale at `(16 committed)` so the RED stands as genuine proof — the doc reconciliation (F2) is owned by `bubbles.docs` (Layer 2, next step) and is what turns this RED → GREEN.

### What was changed (the §24-A 4th-surface pin)

1. **New regex** `smackerelMdTreeRe = regexp.MustCompile(`Connector plugins \((\d+) committed\)`)` — pins the §24-A architecture-tree header (the previously-uncovered 4th surface).
2. **New parser** `parseSmackerelMdTreeCount([]byte) (int, error)` — mirrors `parseSmackerelMdCount`; hard-errors if the §24-A header is absent.
3. **`assertConnectorCountContract` extended** — parses §24-A from the same `smackerelMd` bytes and folds it into the equality so the diagnostic names all four surfaces (`cmd/core/connectors.go`, `docs/smackerel.md §22.7`, `docs/smackerel.md §24-A`, `docs/Development.md`).
4. **New adversarial sub-test** `TestConnectorCountContract_AdversarialSmackerelMdTreeLow` — synthetic smackerel.md with §22.7 = runtime but §24-A = runtime-1 (the exact BUG-024-006 shape); asserts the contract rejects it and names `docs/smackerel.md §24-A=`.
5. **Existing `AdversarialSmackerelMdHigh` fixture** gained a §24-A line (held at runtime) so its §24-A parse succeeds and the §22.7-inflation intent is preserved — no regression in the three pre-existing adversarial sub-tests.

### Code Diff Evidence (Claim Source: executed)

```text
$ git --no-pager diff --stat internal/deploy/docs_connector_count_contract_test.go
 .../deploy/docs_connector_count_contract_test.go   | 139 ++++++++++++++++++---
 1 file changed, 121 insertions(+), 18 deletions(-)

$ grep -n 'smackerelMdTreeRe\|parseSmackerelMdTreeCount\|AdversarialSmackerelMdTreeLow' internal/deploy/docs_connector_count_contract_test.go
57://   - AdversarialSmackerelMdTreeLow: synthetic smackerel.md with §24-A tree
100:// smackerelMdTreeRe matches the §24-A architecture-tree connector-plugins
107:var smackerelMdTreeRe = regexp.MustCompile(`Connector plugins \((\d+) committed\)`)
168:// parseSmackerelMdTreeCount extracts the documented connector count from
173:func parseSmackerelMdTreeCount(b []byte) (int, error) {
174:    m := smackerelMdTreeRe.FindSubmatch(b)
239:    tree, err := parseSmackerelMdTreeCount(smackerelMd)
399:// TestConnectorCountContract_AdversarialSmackerelMdTreeLow proves the
409:func TestConnectorCountContract_AdversarialSmackerelMdTreeLow(t *testing.T) {

$ grep -n 'Connector plugins (' docs/smackerel.md          # §24-A still stale at 16 (NOT fixed — F2/bubbles.docs)
2892:│   ├── Connector plugins (16 committed)

$ awk '/func TestConnectorCountContract_AdversarialSmackerelMdTreeLow/,/^}/' internal/deploy/docs_connector_count_contract_test.go | grep -nE 'return'
NO bare return / bailout found in new sub-test    # no silent-pass early-exit in the new adversarial sub-test
```

### RED Run Evidence — `TestConnectorCountContract_LiveFile` now FAILS at §24-A=16 vs 17 (Claim Source: executed)

Command (sanctioned repo CLI): `./smackerel.sh test unit --go --go-run 'TestConnectorCountContract' --verbose`. With §24-A stale at 16 and the other three surfaces at 17, the live-file test fails — exactly the drift the new pin is designed to catch. The three pre-existing adversarial sub-tests AND the new §24-A adversarial sub-test all PASS, proving the pin is non-tautological and there is no regression:

```text
=== RUN   TestConnectorCountContract_LiveFile
    docs_connector_count_contract_test.go:273: live connector-count contract violated (spec 024 R-006 + BS-004 + AC-5): contract violation: connector count disagreement — cmd/core/connectors.go=17, docs/smackerel.md §22.7=17, docs/smackerel.md §24-A=16, docs/Development.md=17 — all four MUST equal the runtime count; reconcile per spec 024 R-006 + BS-004
--- FAIL: TestConnectorCountContract_LiveFile (0.00s)
=== RUN   TestConnectorCountContract_AdversarialConnectorsGoLow
    docs_connector_count_contract_test.go:315: adversarial OK: connectors.go=15 vs docs=16 is rejected with: contract violation: connector count disagreement — cmd/core/connectors.go=15, docs/smackerel.md §22.7=17, docs/smackerel.md §24-A=16, docs/Development.md=17 — ...
--- PASS: TestConnectorCountContract_AdversarialConnectorsGoLow (0.00s)
=== RUN   TestConnectorCountContract_AdversarialSmackerelMdHigh
    docs_connector_count_contract_test.go:361: adversarial OK: smackerel.md=18 vs runtime+Development.md=17 is rejected with: contract violation: connector count disagreement — cmd/core/connectors.go=17, docs/smackerel.md §22.7=18, docs/smackerel.md §24-A=17, docs/Development.md=17 — ...
--- PASS: TestConnectorCountContract_AdversarialSmackerelMdHigh (0.00s)
=== RUN   TestConnectorCountContract_AdversarialDevelopmentMdLow
    docs_connector_count_contract_test.go:396: adversarial OK: Development.md=15 vs runtime+smackerel.md=16 is rejected with: contract violation: ... docs/smackerel.md §24-A=16, docs/Development.md=15 — ...
--- PASS: TestConnectorCountContract_AdversarialDevelopmentMdLow (0.00s)
=== RUN   TestConnectorCountContract_AdversarialSmackerelMdTreeLow
    docs_connector_count_contract_test.go:453: adversarial OK: §24-A=16 vs runtime+§22.7+Development.md=17 is rejected with: contract violation: connector count disagreement — cmd/core/connectors.go=17, docs/smackerel.md §22.7=17, docs/smackerel.md §24-A=16, docs/Development.md=17 — all four MUST equal the runtime count; reconcile per spec 024 R-006 + BS-004
--- PASS: TestConnectorCountContract_AdversarialSmackerelMdTreeLow (0.00s)
FAIL
FAIL    github.com/smackerel/smackerel/internal/deploy  0.018s
...
FAIL
EXIT_CODE=1
```

**Interpretation (Claim Source: executed):** the RED is genuine and exit code is `1`. Before this change the identical command exited `0` (GREEN) with §24-A uninspected (see "Blind-spot proof" above); after pinning §24-A as the 4th surface, the live drift (§24-A=16 vs the other three at 17) is now detected as a hard `--- FAIL`. The new `TestConnectorCountContract_AdversarialSmackerelMdTreeLow` independently proves the pin is non-tautological (it rejects a synthetic §24-A=runtime-1 and names `docs/smackerel.md §24-A=16`), and the three pre-existing adversarial sub-tests remain PASS (no regression in the BUG-024-003 3-surface contract).

### Hand-off

This RED is the F3 closure proof. It will turn GREEN when `bubbles.docs` (Layer 2) reconciles `docs/smackerel.md` §24-A header `16 committed` → `17 committed` and inserts the Card Rewards leaf (F2). `nextRequiredOwner: bubbles.docs`. No commit performed by `bubbles.test`; spec 024 stays `status: done`.

## bubbles.docs — F2 §24-A Reconciliation GREEN Proof (Layer 2, 2026-06-16)

**Owner:** `bubbles.docs`. **Finding addressed:** F2 (the §24-A docs↔runtime drift, primary). **Surface touched:** `docs/smackerel.md` §24-A architecture tree ONLY (lines 2892 + 2907-2908). This is the truthful doc correction that turns the `bubbles.test` RED (above) → GREEN. No runtime/schema/test edit by this agent; the contract test was left exactly as `bubbles.test` extended it, so the GREEN below is produced solely by the doc reconciliation.

### What was changed (the §24-A 16→17 reconciliation)

1. **Header line 2892** `│   ├── Connector plugins (16 committed)` → `│   ├── Connector plugins (17 committed)` — brings the architecture-tree header into agreement with §22.7 (17), the runtime slice (17), and `docs/Development.md` (17).
2. **QF Decisions glyph** `│   │   └── QF Decisions (qfdecisions/ — spec 041 read-only companion)` → `│   │   ├── QF Decisions (...)` — changes the previously-terminal leaf to a mid leaf so the tree stays well-formed.
3. **Card Rewards leaf inserted** after QF Decisions as the new terminal leaf: `│   │   └── Card Rewards (cardrewards/ — spec 083 read-only rotating-category fetch)` — matches the §22.7 row-17 read-only framing (Principle 10 QF Companion / recommendation-only boundary). The §24-A tree now enumerates 17 leaves (Gov Alerts → Card Rewards).

No other connector leaf, count, glyph, or unrelated content was altered. §22.7 (already 17) and `docs/Development.md` (already 17) were NOT re-touched.

### F2 Code Diff Evidence (Claim Source: executed)

```text
$ git --no-pager diff --stat docs/smackerel.md
 docs/smackerel.md | 5 +++--
 1 file changed, 3 insertions(+), 2 deletions(-)

$ git --no-pager diff docs/smackerel.md
diff --git a/docs/smackerel.md b/docs/smackerel.md
index 9439c3ef..5552d4ad 100644
--- a/docs/smackerel.md
+++ b/docs/smackerel.md
@@ -2889,7 +2889,7 @@ docker-compose.yml
 │   │   ├── POST /api/search       # Semantic search
 │   │   ├── GET  /api/digest       # Daily/weekly digest
 │   │   └── GET  /api/health       # Health check
-│   ├── Connector plugins (16 committed)
+│   ├── Connector plugins (17 committed)
 │   │   ├── Gov Alerts (alerts/)
 │   │   ├── Bookmarks (bookmarks/)
 │   │   ├── Browser History (browser/)
@@ -2905,7 +2905,8 @@ docker-compose.yml
 │   │   ├── Twitter/X (twitter/)
 │   │   ├── Weather (weather/)
 │   │   ├── YouTube (youtube/)
-│   │   └── QF Decisions (qfdecisions/ — spec 041 read-only companion)
+│   │   ├── QF Decisions (qfdecisions/ — spec 041 read-only companion)
+│   │   └── Card Rewards (cardrewards/ — spec 083 read-only rotating-category fetch)
 │   │   Planned connectors:
 │   │   ├── Gmail SDK (google-api-go) 🔜
 │   │   ├── Google Calendar SDK (google-api-go) 🔜

$ grep -n 'Connector plugins (' docs/smackerel.md
2892:│   ├── Connector plugins (17 committed)

$ awk 'NR>=2892 && NR<=2913' docs/smackerel.md | grep -ic 'card.*reward'
1
```

### F2 GREEN Run Evidence — `TestConnectorCountContract_LiveFile` now PASSES at four-surface 17 (Claim Source: executed)

Command (sanctioned repo CLI): `./smackerel.sh test unit --go --go-run 'TestConnectorCountContract' --verbose`. After the §24-A reconciliation, the live-file test PASSES (all four surfaces agree on 17) and the new `AdversarialSmackerelMdTreeLow` sub-test plus the three pre-existing adversarial sub-tests all still PASS — proving the doc fix closed the drift without weakening the guard:

```text
=== RUN   TestConnectorCountContract_LiveFile
    docs_connector_count_contract_test.go:277: contract OK: cmd/core/connectors.go + docs/smackerel.md §22.7 + docs/smackerel.md §24-A + docs/Development.md all agree on 17 connectors (spec 024 R-006 + BS-004 + AC-5 in sync)
--- PASS: TestConnectorCountContract_LiveFile (0.00s)
=== RUN   TestConnectorCountContract_AdversarialConnectorsGoLow
    docs_connector_count_contract_test.go:315: adversarial OK: connectors.go=15 vs docs=16 is rejected with: contract violation: connector count disagreement — cmd/core/connectors.go=15, docs/smackerel.md §22.7=17, docs/smackerel.md §24-A=17, docs/Development.md=17 — all four MUST equal the runtime count; reconcile per spec 024 R-006 + BS-004
--- PASS: TestConnectorCountContract_AdversarialConnectorsGoLow (0.00s)
=== RUN   TestConnectorCountContract_AdversarialSmackerelMdHigh
    docs_connector_count_contract_test.go:361: adversarial OK: smackerel.md=18 vs runtime+Development.md=17 is rejected with: contract violation: connector count disagreement — cmd/core/connectors.go=17, docs/smackerel.md §22.7=18, docs/smackerel.md §24-A=17, docs/Development.md=17 — all four MUST equal the runtime count; reconcile per spec 024 R-006 + BS-004
--- PASS: TestConnectorCountContract_AdversarialSmackerelMdHigh (0.00s)
=== RUN   TestConnectorCountContract_AdversarialDevelopmentMdLow
    docs_connector_count_contract_test.go:396: adversarial OK: Development.md=15 vs runtime+smackerel.md=16 is rejected with: contract violation: connector count disagreement — cmd/core/connectors.go=17, docs/smackerel.md §22.7=17, docs/smackerel.md §24-A=17, docs/Development.md=15 — all four MUST equal the runtime count; reconcile per spec 024 R-006 + BS-004
--- PASS: TestConnectorCountContract_AdversarialDevelopmentMdLow (0.01s)
=== RUN   TestConnectorCountContract_AdversarialSmackerelMdTreeLow
    docs_connector_count_contract_test.go:453: adversarial OK: §24-A=16 vs runtime+§22.7+Development.md=17 is rejected with: contract violation: connector count disagreement — cmd/core/connectors.go=17, docs/smackerel.md §22.7=17, docs/smackerel.md §24-A=16, docs/Development.md=17 — all four MUST equal the runtime count; reconcile per spec 024 R-006 + BS-004
--- PASS: TestConnectorCountContract_AdversarialSmackerelMdTreeLow (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/deploy  0.079s
[go-unit] go test ./... finished OK
EXIT_CODE=0
```

### F2 Doc-Freshness / Config Gate Evidence — `./smackerel.sh check` (Claim Source: executed)

```text
$ ./smackerel.sh check
config-validate: ~/smackerel/config/generated/dev.env.tmp.<pid> OK
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 16, rejected: 0
scenario-lint: OK
EXIT_CODE=0
```

**Interpretation (Claim Source: executed):** the GREEN is genuine and exit code is `0`. Before this doc edit the identical command exited `1` (the `bubbles.test` RED above, §24-A=16 vs the other three at 17); after reconciling §24-A to 17 with the Card Rewards leaf, `TestConnectorCountContract_LiveFile` now passes with the four-surface diagnostic explicitly naming `docs/smackerel.md §24-A` in the agreement set. The new `TestConnectorCountContract_AdversarialSmackerelMdTreeLow` still rejects a synthetic §24-A=16 (proving the pin remains non-tautological — the test was NOT weakened to force green), and the three pre-existing adversarial sub-tests remain PASS (no regression in the BUG-024-003 3-surface contract). `./smackerel.sh check` (config/SST/scenario-lint freshness) is clean at exit 0. F2 is closed.

### Hand-off

F2 (§24-A docs↔runtime drift) and F3 (contract-test 4th-surface blind spot) are both closed: F3 by the `bubbles.test` RED proof, F2 by this `bubbles.docs` GREEN proof. The consolidated path-limited commit and the parent spec 024 governance recertification are **routed to the end-of-sweep `bubbles.devops` pass** (stochastic-quality-sweep R29) — this agent performs NO commit and does NOT recertify spec 024. `nextRequiredOwner: bubbles.devops` (consolidated commit + parent recert, owned downstream). Spec 024 stays `status: done` throughout.

## bubbles.docs — Parent Governance Backfill + GREEN Re-Verification (2026-06-24)

**Owner:** `bubbles.docs`. **Scope:** the parent spec 024 governance recertification half of the 2026-06-16 hand-off (the consolidated commit half remains the orchestrator's). **Surfaces touched:** `specs/024-design-doc-reconciliation/state.json` + `specs/024-design-doc-reconciliation/report.md` (parent governance) and this BUG packet's `scopes.md`/`bug.md`/`state.json` only.

### State of the fix (already committed)

The F2 doc reconciliation and F3 contract-test pin from the 2026-06-16 session are **already committed to `main`** — they are not pending working-tree edits. Verified at `HEAD` (clean working tree):

```text
$ git --no-pager show HEAD:docs/smackerel.md | grep -n 'Connector plugins (\|Card Rewards (cardrewards'
2922:│   ├── Connector plugins (17 committed)
2939:│   │   └── Card Rewards (cardrewards/ — spec 083 read-only rotating-category fetch)

$ grep -n 'smackerelMdTreeRe\|parseSmackerelMdTreeCount\|AdversarialSmackerelMdTreeLow' internal/deploy/docs_connector_count_contract_test.go | head -3
107:var smackerelMdTreeRe = regexp.MustCompile(`Connector plugins \((\d+) committed\)`)
173:func parseSmackerelMdTreeCount(b []byte) (int, error) {
409:func TestConnectorCountContract_AdversarialSmackerelMdTreeLow(t *testing.T) {
```

Because §24-A is committed at 17, a **fresh live-file RED cannot be reproduced** without reverting committed truth. The original live RED is preserved in this report's "bubbles.test — F3 Test-Extension RED Proof" (2026-06-16, `EXIT_CODE=1`); today the RED condition is permanently encoded in the **passing** `TestConnectorCountContract_AdversarialSmackerelMdTreeLow` sub-test (it rejects synthetic §24-A=16 vs runtime=17).

### GREEN re-verification on the current tree (Claim Source: executed, 2026-06-24)

`./smackerel.sh test unit --go --go-run 'TestConnectorCountContract' --verbose`:

```text
=== RUN   TestConnectorCountContract_LiveFile
    docs_connector_count_contract_test.go:277: contract OK: cmd/core/connectors.go + docs/smackerel.md §22.7 + docs/smackerel.md §24-A + docs/Development.md all agree on 17 connectors (spec 024 R-006 + BS-004 + AC-5 in sync)
--- PASS: TestConnectorCountContract_LiveFile (0.00s)
--- PASS: TestConnectorCountContract_AdversarialConnectorsGoLow (0.00s)
--- PASS: TestConnectorCountContract_AdversarialSmackerelMdHigh (0.00s)
--- PASS: TestConnectorCountContract_AdversarialDevelopmentMdLow (0.00s)
--- PASS: TestConnectorCountContract_AdversarialSmackerelMdTreeLow (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/deploy  0.048s
[go-unit] go test ./... finished OK
EXIT_CODE=0
```

### Governance backfill applied

- Parent `state.json`: appended a BUG-024-006 `resolvedBugs[]` entry + a `bubbles.docs` `[docs, finalize]` `executionHistory` entry; bumped `lastUpdatedAt` `2026-06-17`→`2026-06-24`. Validated parseable; `resolvedBugs` ids end with `BUG-024-006-s24a-connector-tree-blindspot`.
- Parent `report.md`: added the `## BUG-024-006 — §24-A …` resolution section (RED historical + GREEN today + boundary + pre-existing note).
- This packet: `scopes.md` DoD guards-differential + governance-backfill items `[x]`; `bug.md` Verified `[x]`; `state.json` governance `executionHistory` entry + `currentPhase: finalize`.

Gate G088 **PASS** (Check 30) — the backfill edits only `state.json`/`report.md` (governance), not `spec.md`/`design.md`/`scopes.md` planning truth, so `certifiedAt 2026-06-06T23:00:00Z` is unchanged.

### Remaining + disposition

The only remaining item is the **orchestrator's consolidated central commit** (the bug's terminal `done` transition is gated on it); bug `state.json` stays `in_progress` with `nextRequiredOwner: bubbles.devops`. **No commit/push** performed by this session. **Pre-existing, out-of-scope:** the parent state-transition-guard (4 blocks) + artifact-lint (5 issues) flag missing `gaps`+`harden` specialist phases (full-delivery required-phase gate drift post-dating the 2026-06-06 certification) — present identically before and after this backfill, not introduced by BUG-024-006, and not cleanly fixable within its scope (no `bubbles.gaps`/`bubbles.harden` provenance source for Check 6B). Parent spec 024 stays `status: done` throughout.
