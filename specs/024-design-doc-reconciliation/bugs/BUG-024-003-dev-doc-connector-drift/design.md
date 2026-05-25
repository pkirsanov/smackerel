# Design: BUG-024-003 docs/Development.md L31 connector-count drift + spec.md R-006 inconsistency + missing automated forward-detection contract test

## Current Truth (Codebase at HEAD `2c8e3242`)

Before designing the fix, captured the relevant codebase facts directly from the working tree:

| Source | Fact | Evidence |
|--------|------|----------|
| Connector registry | 16 connectors imported, instantiated, and registered | `cmd/core/connectors.go` L11-26 imports `alertsConnector`, `bookmarksConnector`, `browserHistConnector`, `caldavConnector`, `discordConnector`, `guesthostConnector`, `hospitableConnector`, `imapConnector`, `keepConnector`, `mapsConnector`, `marketsConnector`, `qfDecisionsConnector`, `rssConnector`, `telegramConnector`, `twitterConnector`, `weatherConnector`, `youtubeConnector` (17 imports for 16 ingestion connectors + 1 telegram which is registered separately, OR all 17 if telegram is counted; verify against L50 slice); L30-47 instantiate; L49-53 registration slice |
| Registration slice | Single line literal slice with 16 entries | `cmd/core/connectors.go` L50 (the multi-line slice literal: `imapConn, caldavConn, ytConn, rssConn, keepConn, bmConn, browserHistConn, mapsConn, hospitableConn, guesthostConn, discordConn, twitterConn, weatherConn, alertsConn, marketsConn, qfDecisionsConn`) |
| docs/smackerel.md §22.7 header | Says `(16 connectors)` after BUG-024-002 close-out | `docs/smackerel.md` L2370 |
| docs/smackerel.md §24-A tree | Says `(16 committed)` after BUG-024-002 close-out | `docs/smackerel.md` L2477 |
| docs/Development.md L31 | Says `15 passive connectors` with 15-item parenthetical missing QF Decisions | `docs/Development.md` L31 |
| docs/Development.md cross-search | Zero hits for `qfdecisions` or `QF Decisions` | `grep -cE "qfdecisions\|QF Decisions" docs/Development.md` → 0 |
| Spec 024 spec.md BS-004 | Lists 16 connectors including `qfdecisions` (updated by BUG-024-002) | `specs/024-design-doc-reconciliation/spec.md` L119-121 |
| Spec 024 spec.md R-006 | Says `15 implemented connectors` with 15-item list missing `qfdecisions` | `specs/024-design-doc-reconciliation/spec.md` L217-235 |
| No framework guard scans connector count | `grep -rEn 'connector_count\|connectors_total\|All 16 connectors are implemented' .github/bubbles/scripts/` → 0 hits | None of the four Bubbles guards parses the §22.7 header for runtime agreement |
| No Go contract test scans connector count | `ls internal/deploy/*connector*` → no match (only existing tests pin compose, monitoring docs, state-concerns schema) | A new `_test.go` file is required |
| Existing Go contract-test pattern | Pure function `assertXContract([]byte, []byte) error` + `TestXContract_LiveFile` (reads real files) + `TestXContract_AdversarialXxx` (fixtures prove non-tautological) | `internal/deploy/monitoring_docs_contract_test.go`, `internal/deploy/compose_contract_test.go`, `internal/deploy/state_concerns_contract_test.go` |
| `repoRoot(t *testing.T)` helper | Available for reuse | `internal/deploy/compose_contract_test.go` L103 |
| Baseline state-transition-guard | Parent spec 024: `TRANSITION PERMITTED with 2 warning(s)` (0 BLOCKs) | Round-9 baseline run |

The fix is **localized** to three files (one production doc, one spec doc, one new test file).

## Architecture

The fix is a three-layer chaos-hardening operation:

```
┌─────────────────────────────────────────────────────────────────────┐
│ LAYER 1: Real Drift Reconciliation                                    │
│   docs/Development.md L31                                              │
│     - 15 passive connectors (...15 items, no QF Decisions...)          │
│     → - 16 passive connectors (...16 items, +QF Decisions companion)   │
│                                                                         │
│   specs/024-design-doc-reconciliation/spec.md R-006                    │
│     "the 15 implemented connectors" + 15-item list                     │
│     → "the 16 implemented connectors" + 16-item list (BS-004 parity)   │
└─────────────────────────────────────────────────────────────────────┘
┌─────────────────────────────────────────────────────────────────────┐
│ LAYER 2: Forward-Detection Hardening                                  │
│   internal/deploy/docs_connector_count_contract_test.go (NEW)         │
│     assertConnectorCountContract(connectorsGoBytes,                    │
│                                  smackerelMdBytes,                    │
│                                  developmentMdBytes []byte) error      │
│     TestConnectorCountContract_LiveFile                                │
│     TestConnectorCountContract_AdversarialConnectorsGoLow              │
│     TestConnectorCountContract_AdversarialSmackerelMdHigh              │
│     TestConnectorCountContract_AdversarialDevelopmentMdLow             │
└─────────────────────────────────────────────────────────────────────┘
┌─────────────────────────────────────────────────────────────────────┐
│ LAYER 3: Packet + Governance + Verification + Commit + Ledger         │
│   BUG-024-003 8 artifacts                                              │
│   Parent state.json: append BUG-024-003 chaos-round executionHistory   │
│                       + resolvedBugs[] entry + bump lastUpdatedAt      │
│   Parent report.md: append BUG-024-003 Chaos-Sweep Resolution section  │
│   Run guards on parent + bug; ./smackerel.sh test unit --go            │
│   git add path-limited; commit bubbles(024/bug-024-003); push          │
│   Update .specify/memory/sweep-2026-05-24-r10.json R9 locally          │
└─────────────────────────────────────────────────────────────────────┘
```

## Components

### Component 1: `docs/Development.md` L31 Reconciliation

**Responsibility**: Update the "Current Capabilities" connector bullet from 15 to 16 with QF Decisions appended.
**Interfaces**: Markdown edit; no schema impact.
**Dependencies**: Must match the canonical registration order in `cmd/core/connectors.go` L50 slice literal so the new Go contract test in Component 3 can be a sane consistency check (count-only, not order-strict — the test pins counts, not enumeration order).

### Component 2: `specs/024-design-doc-reconciliation/spec.md` R-006 Reconciliation

**Responsibility**: Update R-006 intro line + bulleted list to match BS-004 verbatim.
**Interfaces**: Markdown edit; no schema impact.
**Dependencies**: BS-004 (L119-121) was updated by BUG-024-002 — R-006 simply mirrors that change. Same connector order.

### Component 3: `internal/deploy/docs_connector_count_contract_test.go` (NEW)

**Responsibility**: Pure-function contract assertion + 1 live-file test + 3 adversarial sub-tests.

**Pure function `assertConnectorCountContract`:**
- Parses `cmd/core/connectors.go`:
  - Finds the slice literal passed to `svc.registry.Register` calls or builds.
  - Strategy: locate the `for _, c := range []connector.Connector{ ... }` block in `registerConnectors` (or equivalent), tokenize the contents inside `{ }`, count comma-separated identifier tokens, ignore trailing comma.
  - Alternative robust strategy: count distinct `*Conn` identifier names referenced inside that single slice literal block.
- Parses `docs/smackerel.md`:
  - Regex extract `N` from `### 22.7 Committed Connector Inventory \((\d+) connectors\)`.
- Parses `docs/Development.md`:
  - Regex extract `N` from `^- (\d+) passive connectors \(` (first occurrence — the "Current Capabilities" bullet).
- Asserts all three N values equal. On mismatch, return `fmt.Errorf("connector count contract violation: connectors.go registers %d, smackerel.md §22.7 claims %d, Development.md claims %d", goN, smackerelN, devN)`.

**Live-file test `TestConnectorCountContract_LiveFile`:**
- Uses existing `repoRoot(t)` helper.
- Reads the three real files via `os.ReadFile`.
- Calls the pure function. Fails the test on non-nil error.

**Adversarial sub-test `TestConnectorCountContract_AdversarialConnectorsGoLow`:**
- Synthesizes a `connectors.go` byte buffer with 15 entries in the slice (e.g., omit `qfDecisionsConn`).
- Uses real `docs/smackerel.md` + `docs/Development.md` bytes (count 16).
- Expects pure-function to return a non-nil error matching the diagnostic regex. Test fails if the contract returns nil (proves the contract is non-tautological).

**Adversarial sub-test `TestConnectorCountContract_AdversarialSmackerelMdHigh`:**
- Synthesizes a `smackerel.md` byte buffer with header `### 22.7 Committed Connector Inventory (17 connectors)`.
- Uses real `connectors.go` + `Development.md` bytes (count 16).
- Expects pure-function to return a non-nil error.

**Adversarial sub-test `TestConnectorCountContract_AdversarialDevelopmentMdLow`:**
- Synthesizes a `Development.md` byte buffer with `- 15 passive connectors (...)`.
- Uses real `connectors.go` + `smackerel.md` bytes (count 16).
- Expects pure-function to return a non-nil error.

**Interfaces**: Standard Go test (`*testing.T`); no imports beyond `regexp`, `fmt`, `os`, `path/filepath`, `runtime`, `strings`, `testing`, and `bytes`.
**Dependencies**: `repoRoot(t)` helper from `compose_contract_test.go`.

### Component 4: Parent Spec 024 Governance Backfill

**Responsibility**: Minimal append-only updates to `state.json` and `report.md`:
- `state.json`: append BUG-024-003 chaos / implement / test / validate / audit / docs / finalize executionHistory entries + `resolvedBugs[]` entry + bump `lastUpdatedAt`.
- `report.md`: append `## BUG-024-003 Chaos-Sweep Resolution (2026-05-25)` section with Code Diff Evidence + Git-Backed Proof block.

**Interfaces**: Read by `state-transition-guard.sh`, `traceability-guard.sh`, `artifact-freshness-guard.sh`, `artifact-lint.sh`.
**Dependencies**: Existing arrays preserved; additive only.

### Component 5: Verification + Commit + Ledger

**Responsibility**: Run guards + `./smackerel.sh test unit --go` (new test). Path-limited `git add`; verify clean index; single atomic commit with subject prefix `bubbles(024/bug-024-003):`; `git push origin main`; local-only sweep ledger update.

## Data Flow

1. **Pre-edit baseline**: Verify `grep -nE '15 passive connectors' docs/Development.md` → 1 hit at L31; verify `grep -nE 'the 15 implemented connectors' specs/024-design-doc-reconciliation/spec.md` → 1 hit at L217; verify `find internal/connector -maxdepth 1 -mindepth 1 -type d -not -name photos -not -name adapters | wc -l` → 16; verify baseline `./smackerel.sh test unit --go ./internal/deploy/...` green.
2. **Layer 1 edits**: Update `docs/Development.md` L31; update `specs/024-design-doc-reconciliation/spec.md` R-006.
3. **Layer 2 file creation**: Create `internal/deploy/docs_connector_count_contract_test.go`. Run `./smackerel.sh test unit --go ./internal/deploy/...` and verify all four `TestConnectorCountContract_*` tests pass.
4. **Layer 3 packet authoring**: Write all 8 BUG-024-003 artifacts referencing the Layer 1+2 evidence.
5. **Layer 3 parent backfill**: Append BUG-024-003 entries to parent `state.json` + parent `report.md`.
6. **Layer 3 verification**: Run all four Bubbles guards on both directories. Confirm zero new BLOCKS.
7. **Layer 3 commit**: `git add` path-limited; `git diff --cached --name-status` review; `git commit -m "bubbles(024/bug-024-003): reconcile docs/Development.md connector count (15→16, +QF Decisions) + spec.md R-006 parity + add forward-detection contract test"`. Then `git push origin main`.
8. **Layer 3 ledger**: Update `.specify/memory/sweep-2026-05-24-r10.json` round 9 entry to `status: completed_owned`. Do NOT commit.

## Implementation Plan

### Iteration 1: Layer 1 — `docs/Development.md` L31 reconciliation (1 edit)

Edit L31:

```
- 15 passive connectors (IMAP email, CalDAV calendar, YouTube API, RSS/Atom, Bookmarks, Browser, Google Keep/Takeout, Google Maps, Hospitable STR, GuestHost STR, Discord, Twitter/X archive, Weather via Open-Meteo, Government Alerts via USGS, Financial Markets via Finnhub/CoinGecko)
```

→

```
- 16 passive connectors (IMAP email, CalDAV calendar, YouTube API, RSS/Atom, Bookmarks, Browser, Google Keep/Takeout, Google Maps, Hospitable STR, GuestHost STR, Discord, Twitter/X archive, Weather via Open-Meteo, Government Alerts via USGS, Financial Markets via Finnhub/CoinGecko, QF Decisions companion via spec 041 read-only packet flow)
```

### Iteration 2: Layer 1 — `specs/024-design-doc-reconciliation/spec.md` R-006 reconciliation

Update R-006 intro: `the 15 implemented connectors` → `the 16 implemented connectors`. Append `- QF Decisions companion (\`qfdecisions/\`) — spec 041 read-only DecisionPacket ingestion, no financial advice generation` as the 16th bullet after the existing 15 bullets to match BS-004's order verbatim.

### Iteration 3: Layer 2 — Create `internal/deploy/docs_connector_count_contract_test.go`

Authored as a single Go test file following the precedent established by `monitoring_docs_contract_test.go` and `state_concerns_contract_test.go`. Structure:

```go
package deploy

import (
    "bytes"
    "fmt"
    "os"
    "path/filepath"
    "regexp"
    "strings"
    "testing"
)

// assertConnectorCountContract — pure function, returns nil iff all three
// surfaces agree on connector count.
func assertConnectorCountContract(connectorsGo, smackerelMd, developmentMd []byte) error {
    goN, err := parseConnectorsGoCount(connectorsGo)
    if err != nil { return fmt.Errorf("parse connectors.go: %w", err) }

    smackerelN, err := parseSmackerelMdCount(smackerelMd)
    if err != nil { return fmt.Errorf("parse smackerel.md §22.7: %w", err) }

    developmentN, err := parseDevelopmentMdCount(developmentMd)
    if err != nil { return fmt.Errorf("parse Development.md L31: %w", err) }

    if goN != smackerelN || goN != developmentN {
        return fmt.Errorf(
            "connector count contract violation: cmd/core/connectors.go=%d, docs/smackerel.md §22.7=%d, docs/Development.md=%d",
            goN, smackerelN, developmentN)
    }
    return nil
}

// parseConnectorsGoCount — find the slice literal of connector.Connector
// values inside registerConnectors and count comma-separated identifiers.
func parseConnectorsGoCount(b []byte) (int, error) {
    // Locate the slice literal: `[]connector.Connector{ ... }`
    re := regexp.MustCompile(`\[\]connector\.Connector\{([^}]*)\}`)
    m := re.FindSubmatch(b)
    if m == nil {
        return 0, fmt.Errorf("no []connector.Connector{...} slice literal found")
    }
    inner := strings.TrimSpace(string(m[1]))
    // Strip trailing comma if present
    inner = strings.TrimSuffix(inner, ",")
    // Count comma-separated tokens, ignoring whitespace and comments
    var count int
    for _, tok := range strings.Split(inner, ",") {
        tok = strings.TrimSpace(tok)
        // Strip line comments
        if idx := strings.Index(tok, "//"); idx >= 0 {
            tok = strings.TrimSpace(tok[:idx])
        }
        if tok == "" { continue }
        count++
    }
    return count, nil
}

// parseSmackerelMdCount — find §22.7 header and extract N.
func parseSmackerelMdCount(b []byte) (int, error) {
    re := regexp.MustCompile(`### 22\.7 Committed Connector Inventory \((\d+) connectors\)`)
    m := re.FindSubmatch(b)
    if m == nil {
        return 0, fmt.Errorf("no §22.7 header with `(N connectors)` found")
    }
    var n int
    if _, err := fmt.Sscanf(string(m[1]), "%d", &n); err != nil {
        return 0, err
    }
    return n, nil
}

// parseDevelopmentMdCount — find the first "- N passive connectors (" bullet.
func parseDevelopmentMdCount(b []byte) (int, error) {
    re := regexp.MustCompile(`(?m)^- (\d+) passive connectors \(`)
    m := re.FindSubmatch(b)
    if m == nil {
        return 0, fmt.Errorf("no `- N passive connectors (` bullet found")
    }
    var n int
    if _, err := fmt.Sscanf(string(m[1]), "%d", &n); err != nil {
        return 0, err
    }
    return n, nil
}

func TestConnectorCountContract_LiveFile(t *testing.T) {
    root := repoRoot(t)
    connectorsGo, err := os.ReadFile(filepath.Join(root, "cmd/core/connectors.go"))
    if err != nil { t.Fatalf("read cmd/core/connectors.go: %v", err) }
    smackerelMd, err := os.ReadFile(filepath.Join(root, "docs/smackerel.md"))
    if err != nil { t.Fatalf("read docs/smackerel.md: %v", err) }
    developmentMd, err := os.ReadFile(filepath.Join(root, "docs/Development.md"))
    if err != nil { t.Fatalf("read docs/Development.md: %v", err) }
    if err := assertConnectorCountContract(connectorsGo, smackerelMd, developmentMd); err != nil {
        t.Fatalf("connector count contract violation on live files: %v", err)
    }
}

func TestConnectorCountContract_AdversarialConnectorsGoLow(t *testing.T) {
    // Synthesize a connectors.go with 15 entries (real has 16)
    fake := []byte(`
package main
func registerConnectors(svc *Service) {
    _ = []connector.Connector{
        c1, c2, c3, c4, c5, c6, c7, c8,
        c9, c10, c11, c12, c13, c14, c15,
    }
}`)
    realSmackerelMd := []byte(`### 22.7 Committed Connector Inventory (16 connectors)`)
    realDevelopmentMd := []byte(`- 16 passive connectors (one, two, ...)`)
    if err := assertConnectorCountContract(fake, realSmackerelMd, realDevelopmentMd); err == nil {
        t.Fatalf("contract MUST fail when connectors.go has 15 entries but docs claim 16 (adversarial regression check)")
    }
}

func TestConnectorCountContract_AdversarialSmackerelMdHigh(t *testing.T) {
    fakeConnectorsGo := []byte(`_ = []connector.Connector{a, b, c, d, e, f, g, h, i, j, k, l, m, n, o, p}`)
    fakeSmackerelMd := []byte(`### 22.7 Committed Connector Inventory (17 connectors)`)
    realDevelopmentMd := []byte(`- 16 passive connectors (one, ...)`)
    if err := assertConnectorCountContract(fakeConnectorsGo, fakeSmackerelMd, realDevelopmentMd); err == nil {
        t.Fatalf("contract MUST fail when smackerel.md §22.7 claims 17 but runtime + Development.md say 16")
    }
}

func TestConnectorCountContract_AdversarialDevelopmentMdLow(t *testing.T) {
    fakeConnectorsGo := []byte(`_ = []connector.Connector{a, b, c, d, e, f, g, h, i, j, k, l, m, n, o, p}`)
    fakeSmackerelMd := []byte(`### 22.7 Committed Connector Inventory (16 connectors)`)
    fakeDevelopmentMd := []byte(`- 15 passive connectors (one, two, ...)`)
    if err := assertConnectorCountContract(fakeConnectorsGo, fakeSmackerelMd, fakeDevelopmentMd); err == nil {
        t.Fatalf("contract MUST fail when Development.md says 15 but runtime + smackerel.md say 16")
    }
}

// silence unused-import linter while keeping bytes available for future extensions
var _ = bytes.NewBuffer
```

The `bytes` placeholder line is removed if the parser ultimately doesn't need it — the final implementation will be lint-clean with no unused imports.

### Iteration 4: Layer 3 — Parent spec 024 state.json + report.md backfill

- `state.json`: append 7 executionHistory entries (`bubbles.chaos:chaos`, `bubbles.implement:implement`, `bubbles.test:test`, `bubbles.validate:validate`, `bubbles.audit:audit`, `bubbles.docs:docs`, `bubbles.workflow:finalize`) for the BUG-024-003 close-out. Append `resolvedBugs[]` entry for BUG-024-003. Bump `lastUpdatedAt`.
- `report.md`: append `## BUG-024-003 Chaos-Sweep Resolution (2026-05-25)` section with Code Diff Evidence table + Git-Backed Proof block (post-commit guard outputs + `./smackerel.sh test unit --go` output).

### Iteration 5: Layer 3 — Verify, commit, push, ledger

- Run `./smackerel.sh test unit --go ./internal/deploy/...` and confirm `TestConnectorCountContract_*` pass.
- Run all four Bubbles guards on parent + bug; confirm zero new BLOCKS.
- `git add docs/Development.md specs/024-design-doc-reconciliation/spec.md specs/024-design-doc-reconciliation/state.json specs/024-design-doc-reconciliation/report.md specs/024-design-doc-reconciliation/bugs/BUG-024-003-dev-doc-connector-drift/ internal/deploy/docs_connector_count_contract_test.go`.
- `git diff --cached --name-status` review for stray staging. Abort if anything else appears.
- `git commit -m "bubbles(024/bug-024-003): reconcile docs/Development.md connector count (15→16, +QF Decisions) + spec.md R-006 parity + add forward-detection contract test"`.
- `git push origin main` (pre-push hook validates; no `--no-verify`).
- Update `.specify/memory/sweep-2026-05-24-r10.json` R9 locally. Do NOT commit.

## Verification Checklist

```bash
# Layer 1 — real drift closed
grep -nE '15 passive connectors' docs/Development.md  # → 0 hits
grep -cE 'qfdecisions|QF Decisions' docs/Development.md  # → ≥ 1
grep -nE 'the 16 implemented connectors' specs/024-design-doc-reconciliation/spec.md  # → 1 hit (R-006)

# Layer 2 — forward-detection test passes
./smackerel.sh test unit --go ./internal/deploy/...  # → all TestConnectorCountContract_* pass

# Layer 3 — parent + bug gates clean
bash .github/bubbles/scripts/state-transition-guard.sh specs/024-design-doc-reconciliation  # → 0 new BLOCKs
bash .github/bubbles/scripts/artifact-lint.sh specs/024-design-doc-reconciliation  # → exit 0
bash .github/bubbles/scripts/traceability-guard.sh specs/024-design-doc-reconciliation  # → exit 0
bash .github/bubbles/scripts/artifact-lint.sh specs/024-design-doc-reconciliation/bugs/BUG-024-003-dev-doc-connector-drift  # → exit 0
```

## Security & Compliance

No security implications — docs + spec + new test-only Go file. The new test file does not compile into any binary.

## Observability

Not applicable — docs + test edits only.

## Testing Strategy

- **AC-04 / AC-05 enforcement** — the new contract test itself proves non-tautological via three adversarial sub-tests.
- **Regression** — re-run `./smackerel.sh test unit --go ./internal/deploy/...` after the contract test is added; confirm full deploy-tests suite remains green.
- **Adversarial-fidelity self-check** — manually run each `TestConnectorCountContract_Adversarial*` and confirm it would have caught the original drift (15 in Development.md vs 16 in connectors.go).

## Risks & Open Questions

No open questions. Key risks:

1. **Slice-literal regex fragility.** The pure function relies on a single `\[\]connector\.Connector\{...\}` slice literal in `cmd/core/connectors.go`. If a future refactor splits registration across multiple slice literals or moves them out of `registerConnectors`, the parser would need updating. Mitigation: the parser fails loudly with `no []connector.Connector{...} slice literal found`, making the regression obvious; the adversarial sub-tests prove the failure path.
2. **Counting cleanup.** Comma-separated identifier tokenization must ignore inline comments and trailing commas — handled in `parseConnectorsGoCount`.
3. **`docs/Development.md` line drift.** The regex `^- (\d+) passive connectors \(` matches the first occurrence; if a future doc reorganization puts the bullet later in the file with intervening `- N passive connectors (` text, the parser would catch the first hit. Acceptable as a contract: the connector count claim should appear exactly once in the doc.
4. **Path-limited `git add` discipline.** Same risk profile as BUG-024-002: stray staging must be caught by `git diff --cached --name-status` pre-commit review.
5. **No `--no-verify` push.** Hard constraint from user critical-rules. The pre-push hook runs full validation; commits must pass it cleanly.
