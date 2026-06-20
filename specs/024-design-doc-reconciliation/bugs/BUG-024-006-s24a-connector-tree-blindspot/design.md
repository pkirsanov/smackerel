# Design: BUG-024-006 §24-A architecture-tree connector-count drift (16 vs live 17) + TestConnectorCountContract 4th-surface blind spot

## Current Truth (Codebase at HEAD `7844283b`, main)

Before designing the fix, captured the relevant codebase facts directly from the working tree (every row is a real command result, redacted to `~`):

| Source | Fact | Evidence |
|--------|------|----------|
| Runtime registry | 17 connectors instantiated + registered | `cmd/core/connectors.go` line 67 `cardRewardsConn := cardrewardsConnector.New()`; slice literal lines 68-72 with `qfDecisionsConn` (L71) + `cardRewardsConn` (L72) appended |
| §22.7 inventory header | `### 22.7 Committed Connector Inventory (17 connectors)` | `docs/smackerel.md` line 2783 |
| §22.7 row 17 | Card Rewards present | `docs/smackerel.md` line 2805 `| 17 | Card Rewards | \`cardrewards/\` | Finance | … spec 083 … |` |
| §24-A architecture-tree header | `│   ├── Connector plugins (16 committed)` — **stale at 16** | `docs/smackerel.md` line 2892 |
| §24-A leaf list | 16 leaves (Gov Alerts → QF Decisions), closes at QF Decisions, then `Planned connectors:` | `docs/smackerel.md` lines 2893-2909; line 2908 `└── QF Decisions (qfdecisions/ — spec 041 read-only companion)` |
| §24-A Card Rewards | **absent** | `awk 'NR>=2892 && NR<=2910' docs/smackerel.md | grep -ic 'card.*reward\|cardrewards'` → 0 |
| `docs/Development.md` line 31 | `17 passive connectors (… Card Rewards rotating-category source via spec 083 read-only fetch)` | `docs/Development.md` line 31 |
| Contract test pinned surfaces | 3 regexes — slice literal, §22.7 header, Development.md bullet | `internal/deploy/docs_connector_count_contract_test.go` line 81 `connectorsGoSliceRe`, line 89 `smackerelMdHeaderRe = \`### 22\.7 Committed Connector Inventory \((\d+) connectors\)\``, line 95 `developmentMdBulletRe` |
| Contract test §24-A coverage | **none** — only a historical comment | `grep -in 'committed\|Connector plugins\|24-A' internal/deploy/docs_connector_count_contract_test.go` → line 12 comment only; no regex matches `Connector plugins (N committed)` |
| Live-file test result | GREEN at 17 despite §24-A=16 | `TestConnectorCountContract_LiveFile` line 236 logs `contract OK: … all agree on 17 connectors`; `./smackerel.sh test unit --go --go-run 'TestConnectorCountContract' --verbose` → `ok internal/deploy 0.029s`, exit 0 |

The fix is **localized** to two files (one production doc, one test file) plus parent governance backfill. No runtime/schema/NATS/compose/web/prompt change.

## Architecture (Two-Layer Fix, sequenced test → docs)

```
┌─────────────────────────────────────────────────────────────────────┐
│ LAYER 1 (FIRST — bubbles.test): Forward-Detection Hardening (F3)       │
│   internal/deploy/docs_connector_count_contract_test.go               │
│     + smackerelMdTreeRe  = `Connector plugins \((\d+) committed\)`     │
│     + parseSmackerelMdTreeCount([]byte) (int, error)                   │
│     ~ assertConnectorCountContract: add §24-A as a 4th surface         │
│       (parsed from the SAME docs/smackerel.md bytes already passed)    │
│     + TestConnectorCountContract_AdversarialSmackerelMdTreeLow         │
│   RED PROOF: with §24-A still at 16 and the other three at 17,         │
│     TestConnectorCountContract_LiveFile MUST fail (16 ≠ 17).           │
└─────────────────────────────────────────────────────────────────────┘
┌─────────────────────────────────────────────────────────────────────┐
│ LAYER 2 (SECOND — bubbles.docs): Real Drift Reconciliation (F2)        │
│   docs/smackerel.md §24-A (lines 2892-2908)                            │
│     line 2892: "Connector plugins (16 committed)"                      │
│       → "Connector plugins (17 committed)"                             │
│     line 2908: "└── QF Decisions (qfdecisions/ — …)"                   │
│       → "├── QF Decisions (qfdecisions/ — …)"                          │
│     + insert "└── Card Rewards (cardrewards/ — spec 083 read-only      │
│              rotating-category fetch)" as the new closing leaf          │
│   GREEN PROOF: TestConnectorCountContract_LiveFile passes (all four    │
│     surfaces = 17).                                                    │
└─────────────────────────────────────────────────────────────────────┘
┌─────────────────────────────────────────────────────────────────────┐
│ LAYER 3 (bubbles.docs): Packet + Governance + Commit                   │
│   BUG-024-006 8 artifacts (this packet)                               │
│   Parent state.json: append BUG-024-006 resolvedBugs[] + executionHist │
│   Parent report.md: append BUG-024-006 resolution section              │
│   git add path-limited; commit bubbles(024/bug-024-006); push          │
└─────────────────────────────────────────────────────────────────────┘
```

## Components

### Component 1: `internal/deploy/docs_connector_count_contract_test.go` (EXTEND) — Layer 1, bubbles.test

**Responsibility**: Pin §24-A's `Connector plugins (N committed)` line as a 4th surface in the connector-count contract, with adversarial proof.

**New regex** (mirrors the existing `smackerelMdHeaderRe` style at line 89):
```go
// smackerelMdTreeRe matches the §24-A architecture-tree connector-plugins
// header. The connector count is captured as group 1. §24-A carries the
// same count as §22.7 by intent but is a distinct surface that drifted
// undetected at the 16→17 transition (BUG-024-006).
var smackerelMdTreeRe = regexp.MustCompile(`Connector plugins \((\d+) committed\)`)
```

**New parser** (mirrors `parseSmackerelMdCount` at lines ~136-150):
```go
func parseSmackerelMdTreeCount(b []byte) (int, error) {
    m := smackerelMdTreeRe.FindSubmatch(b)
    if m == nil {
        return 0, fmt.Errorf("docs/smackerel.md does not contain a `Connector plugins (N committed)` §24-A architecture-tree header — the §24-A surface moved; restore the header or update this contract")
    }
    return parseIntStrict(string(m[1]))
}
```

**Extend `assertConnectorCountContract`**: parse the §24-A count from the `smackerelMd` bytes it already receives (no new function argument required — both §22.7 and §24-A live in `docs/smackerel.md`), and fold it into the equality assertion so the diagnostic names all four surfaces:
```go
tree, err := parseSmackerelMdTreeCount(smackerelMd)
if err != nil {
    return fmt.Errorf("contract violation (smackerel.md §24-A parse): %w", err)
}
if runtime != smackerel || runtime != development || runtime != tree {
    return fmt.Errorf(
        "contract violation: connector count disagreement — cmd/core/connectors.go=%d, docs/smackerel.md §22.7=%d, docs/smackerel.md §24-A=%d, docs/Development.md=%d — all four MUST equal the runtime count; reconcile per spec 024 R-006 + BS-004",
        runtime, smackerel, tree, development)
}
```

**New adversarial sub-test** (mirrors `TestConnectorCountContract_AdversarialSmackerelMdHigh` at lines ~290-316):
```go
// TestConnectorCountContract_AdversarialSmackerelMdTreeLow proves the
// contract catches the exact BUG-024-006 regression: §24-A stale at 16
// while §22.7 + runtime + Development.md are all at the live count.
func TestConnectorCountContract_AdversarialSmackerelMdTreeLow(t *testing.T) {
    // synthetic docs/smackerel.md whose §22.7 header = runtime count but
    // whose §24-A header = "Connector plugins (<runtime-1> committed)";
    // assertConnectorCountContract MUST return a non-nil error naming §24-A.
}
```

**Live-file update**: `TestConnectorCountContract_LiveFile`'s success log (line 236) is updated to reference four-surface agreement.

**Dependencies**: existing `repoRoot(t)`, `parseIntStrict`, `parseConnectorsGoCount`, `parseSmackerelMdCount`, `parseDevelopmentMdCount` helpers in the same file. No new imports.

### Component 2: `docs/smackerel.md` §24-A Reconciliation (EDIT) — Layer 2, bubbles.docs

**Responsibility**: Bring the §24-A architecture tree to 17 with Card Rewards.
**Exact edits**:
- Line 2892 `│   ├── Connector plugins (16 committed)` → `│   ├── Connector plugins (17 committed)`.
- Line 2908 `│   │   └── QF Decisions (qfdecisions/ — spec 041 read-only companion)` → `│   │   ├── QF Decisions (qfdecisions/ — spec 041 read-only companion)` (change closing `└──` to mid `├──`).
- Insert after line 2908: `│   │   └── Card Rewards (cardrewards/ — spec 083 read-only rotating-category fetch)`.
**Interfaces**: Markdown edit; no schema impact. The Card Rewards leaf text keeps the read-only framing consistent with §22.7 row 17 (Principle 10 boundary).
**Dependencies**: must keep the §24-A glyph alignment (`│   │   ├──` / `│   │   └──`) consistent with the surrounding tree.

### Component 3: Parent Spec 024 Governance Backfill (EDIT) — Layer 3, bubbles.docs

**Responsibility**: append-only updates to parent `state.json` (`resolvedBugs[]` entry + executionHistory + `lastUpdatedAt` bump) and parent `report.md` (a `## BUG-024-006 …` resolution section). Existing arrays preserved; additive only. Spec 024 stays `status: done`.

## Data Flow

1. **Pre-fix baseline** (this discovery packet, DONE): verify §24-A=16 (line 2892); §22.7=17 (line 2783); Development.md=17 (line 31); slice=17 (lines 68-72); contract test pins 3 surfaces; `TestConnectorCountContract_LiveFile` GREEN at 17 with §24-A uninspected.
2. **Layer 1 (bubbles.test)**: add the §24-A regex + parser + 4th-surface assertion + adversarial sub-test. Run `./smackerel.sh test unit --go --go-run 'TestConnectorCountContract' --verbose` and capture the **RED** live-file failure (`§24-A=16` vs runtime=17). Run the new adversarial sub-test in isolation and confirm it rejects synthetic §24-A drift.
3. **Layer 2 (bubbles.docs)**: apply the three §24-A edits. Re-run the contract test and capture the **GREEN** pass (exit 0, four-surface agreement).
4. **Layer 3 (bubbles.docs)**: author/refresh the BUG-024-006 packet evidence, backfill parent governance, run all framework guards on parent + bug folder, path-limited `git add`, single atomic commit `bubbles(024/bug-024-006): …`, push.

## Implementation Plan

### Iteration 1 (bubbles.test): extend the contract test (Layer 1)
Add `smackerelMdTreeRe`, `parseSmackerelMdTreeCount`, the 4th-surface branch in `assertConnectorCountContract`, the new `TestConnectorCountContract_AdversarialSmackerelMdTreeLow` sub-test, and update the live-file success log. Capture the RED live-file run against the still-stale §24-A.

### Iteration 2 (bubbles.docs): reconcile §24-A (Layer 2)
Apply the line-2892 header edit + the line-2908 glyph edit + the inserted Card Rewards leaf. Capture the GREEN contract-test run.

### Iteration 3 (bubbles.docs): packet + governance + commit (Layer 3)
Mark BUG-024-006 DoD items `[x]` with inline evidence; backfill parent `state.json` + `report.md`; run guards; path-limited commit + push.

## Verification Checklist

- [ ] §24-A header line 2892 = `(17 committed)` (FR-01)
- [ ] §24-A leaf list enumerates Card Rewards after QF Decisions; 17 leaves total (FR-01)
- [ ] `smackerelMdTreeRe` + `parseSmackerelMdTreeCount` + 4th-surface assertion added (FR-02)
- [ ] `TestConnectorCountContract_AdversarialSmackerelMdTreeLow` rejects synthetic §24-A drift (FR-02)
- [ ] RED evidence: live-file test fails at §24-A=16 vs runtime=17 BEFORE the doc edit (FR-03/AC-05)
- [ ] GREEN evidence: `./smackerel.sh test unit --go --go-run 'TestConnectorCountContract' --verbose` exit 0 AFTER the doc edit (FR-03/AC-06)
- [ ] All four framework guards green on parent + bug folder (AC-07/AC-08/AC-10)
- [ ] Single atomic commit `bubbles(024/bug-024-006):`; spec 024 stays `done` (FR-05/AC-09/AC-12)
