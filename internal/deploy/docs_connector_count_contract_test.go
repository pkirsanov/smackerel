// Copyright (c) 2026 Philip Kirsanov
// SPDX-License-Identifier: MIT

// Package deploy — BUG-024-003 (chaos-sweep round 9, 2026-05-25) connector
// count agreement contract for docs-runtime drift detection.
//
// # Context
//
// Spec 024 (design-doc-reconciliation) requires that every documented
// connector count remains in lockstep with the live runtime registry. Prior
// closures (BUG-024-002, reconcile-sweep round 29 of sweep-2026-05-24-r1)
// reconciled docs/smackerel.md §22.7 + §24-A to the live count of 16 but
// did NOT propagate the change to docs/Development.md L31 and did NOT add
// any forward-detection guard. Chaos-sweep round 9 of sweep-2026-05-24-r10
// found three findings:
//
//   - F1 (HIGH): docs/Development.md L31 still claimed "15 passive
//     connectors" with a 15-item parenthetical missing QF Decisions.
//   - F2 (MEDIUM): No framework or runtime guard pinned agreement between
//     cmd/core/connectors.go, docs/smackerel.md, and docs/Development.md.
//   - F3 (LOW): specs/024-design-doc-reconciliation/spec.md R-006 said "the
//     15 implemented connectors" with a 15-item list, internally
//     contradicting BS-004 (16-item list) updated by BUG-024-002.
//
// This file closes F2 by pinning a four-surface contract. BUG-024-006
// (stochastic-quality-sweep round R29, 2026-06-16) extended the original
// three-surface contract with the §24-A architecture-tree surface after it
// silently drifted to 16 while the other three advanced to 17.
//
// The contract
//
//   - cmd/core/connectors.go contains exactly one slice literal of the form
//     "[]connector.Connector{ ... }" inside registerConnectors; the number
//     of comma-separated identifiers in that literal is the runtime
//     connector count (currently 17).
//   - docs/smackerel.md §22.7 declares the connector count via the header
//     "### 22.7 Committed Connector Inventory (N connectors)" — N MUST
//     equal the runtime count.
//   - docs/smackerel.md §24-A declares the connector count via the
//     architecture-tree header "Connector plugins (N committed)" — N MUST
//     equal the runtime count. This 4th surface was added by BUG-024-006.
//   - docs/Development.md "Current Capabilities" list declares the count
//     via the bullet "- N passive connectors (...)" — N MUST equal the
//     runtime count.
//
// All four must agree. If any one drifts, the live-file test fails with a
// precise diagnostic naming the disagreeing surfaces and the observed
// counts.
//
// # Adversarial sub-tests
//
// Four adversarial sub-tests prove the assertion is non-tautological:
//   - AdversarialConnectorsGoLow: synthetic connectors.go with 15 entries +
//     real docs → contract MUST reject.
//   - AdversarialSmackerelMdHigh: synthetic smackerel.md with §22.7 header
//     inflated to runtime+1 + real other surfaces → contract MUST reject.
//   - AdversarialSmackerelMdTreeLow: synthetic smackerel.md with §24-A tree
//     header stale at runtime-1 + real other surfaces → contract MUST
//     reject (the exact BUG-024-006 regression).
//   - AdversarialDevelopmentMdLow: synthetic Development.md with bullet
//     "- 15 passive connectors" + real other surfaces → contract MUST
//     reject.
//
// References:
//   - specs/024-design-doc-reconciliation/spec.md (BS-004, R-006, AC-5)
//   - specs/024-design-doc-reconciliation/bugs/BUG-024-003-dev-doc-connector-drift/
//   - .specify/memory/sweep-2026-05-24-r10.json (round 9 entry)
//   - cmd/core/connectors.go L49-53 (slice literal source of truth)
package deploy

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// connectorsGoSliceRe matches the slice literal "[]connector.Connector{ ... }"
// in cmd/core/connectors.go. The body is captured as group 1 and is
// processed by countSliceIdentifiers to count comma-separated identifiers
// while ignoring trailing commas, blank tokens, and line comments.
//
// `[^}]*` is intentionally greedy across newlines (Go's default regex is
// not in dotall mode, but `[^}]` matches \n because \n is not `}`). The
// live slice literal at cmd/core/connectors.go L49-53 spans three lines and
// is bounded by `{...}` with no nested braces; this regex matches it
// cleanly.
var connectorsGoSliceRe = regexp.MustCompile(`\[\]connector\.Connector\{([^}]*)\}`)

// smackerelMdHeaderRe matches the §22.7 Committed Connector Inventory
// header. The connector count is captured as group 1. The header MUST
// remain a `### 22.7` heading per BUG-024-002 closure; if the heading is
// reformatted, this regex fails and the live-file test reports a missing
// header diagnostic (which is still actionable — the contract surface
// moved and needs to be re-pinned here).
var smackerelMdHeaderRe = regexp.MustCompile(`### 22\.7 Committed Connector Inventory \((\d+) connectors\)`)

// smackerelMdTreeRe matches the §24-A architecture-tree connector-plugins
// header. The connector count is captured as group 1. §24-A carries the
// same count as §22.7 by intent but is a distinct surface that drifted
// undetected at the 16→17 transition (BUG-024-006) — §22.7, the runtime
// slice, and docs/Development.md all advanced to 17 while §24-A stayed at
// 16. There is exactly one `Connector plugins (N committed)` line in
// docs/smackerel.md, so this regex pins it unambiguously.
var smackerelMdTreeRe = regexp.MustCompile(`Connector plugins \((\d+) committed\)`)

// developmentMdBulletRe matches the "Current Capabilities" connector
// bullet in docs/Development.md. The connector count is captured as group
// 1. The (?m) flag makes ^ match line starts so it does not accidentally
// catch the count from a different "- N" bullet earlier in the file.
var developmentMdBulletRe = regexp.MustCompile(`(?m)^- (\d+) passive connectors \(`)

// countSliceIdentifiers parses the body of a Go slice literal and returns
// the count of comma-separated identifier tokens, ignoring blank tokens,
// trailing commas, and `//` line comments. It is deliberately strict: it
// does NOT handle block comments or nested literals because the live
// connectors.go slice has none of those. If the slice grows complex
// enough to need that, the test will fail loud and the parser can be
// extended.
func countSliceIdentifiers(body string) int {
	// Strip `//` line comments.
	var stripped strings.Builder
	for _, line := range strings.Split(body, "\n") {
		if idx := strings.Index(line, "//"); idx >= 0 {
			line = line[:idx]
		}
		stripped.WriteString(line)
		stripped.WriteString("\n")
	}

	count := 0
	for _, tok := range strings.Split(stripped.String(), ",") {
		if strings.TrimSpace(tok) != "" {
			count++
		}
	}
	return count
}

// parseConnectorsGoCount extracts the runtime connector count from
// cmd/core/connectors.go. Returns an error if the slice literal cannot be
// located (which would indicate the registration shape changed and the
// contract surface needs to be re-pinned).
func parseConnectorsGoCount(b []byte) (int, error) {
	m := connectorsGoSliceRe.FindSubmatch(b)
	if m == nil {
		return 0, fmt.Errorf("cmd/core/connectors.go does not contain a `[]connector.Connector{...}` slice literal — the registration shape changed; re-pin the contract surface in docs_connector_count_contract_test.go")
	}
	return countSliceIdentifiers(string(m[1])), nil
}

// parseSmackerelMdCount extracts the documented connector count from the
// §22.7 header in docs/smackerel.md.
func parseSmackerelMdCount(b []byte) (int, error) {
	m := smackerelMdHeaderRe.FindSubmatch(b)
	if m == nil {
		return 0, fmt.Errorf("docs/smackerel.md does not contain a `### 22.7 Committed Connector Inventory (N connectors)` header — BUG-024-002 closure surface is missing; restore the header or update this contract")
	}
	n, err := parseIntStrict(string(m[1]))
	if err != nil {
		return 0, fmt.Errorf("docs/smackerel.md §22.7 header connector count is not a positive integer: %w", err)
	}
	return n, nil
}

// parseSmackerelMdTreeCount extracts the documented connector count from
// the §24-A architecture-tree "Connector plugins (N committed)" header in
// docs/smackerel.md. This is the 4th contract surface, added by
// BUG-024-006 after §24-A silently drifted to 16 while §22.7, the runtime
// slice, and docs/Development.md all advanced to 17.
func parseSmackerelMdTreeCount(b []byte) (int, error) {
	m := smackerelMdTreeRe.FindSubmatch(b)
	if m == nil {
		return 0, fmt.Errorf("docs/smackerel.md does not contain a `Connector plugins (N committed)` §24-A architecture-tree header — the §24-A surface moved; restore the header or update this contract")
	}
	n, err := parseIntStrict(string(m[1]))
	if err != nil {
		return 0, fmt.Errorf("docs/smackerel.md §24-A `Connector plugins (N committed)` count is not a positive integer: %w", err)
	}
	return n, nil
}

// parseDevelopmentMdCount extracts the documented connector count from
// the "Current Capabilities" bullet in docs/Development.md.
func parseDevelopmentMdCount(b []byte) (int, error) {
	m := developmentMdBulletRe.FindSubmatch(b)
	if m == nil {
		return 0, fmt.Errorf("docs/Development.md does not contain a `- N passive connectors (...)` bullet in the Current Capabilities list — the doc surface moved; restore the bullet or update this contract")
	}
	n, err := parseIntStrict(string(m[1]))
	if err != nil {
		return 0, fmt.Errorf("docs/Development.md `- N passive connectors` count is not a positive integer: %w", err)
	}
	return n, nil
}

// parseIntStrict is a tiny helper that accepts only digit-only positive
// integers; rejects empty strings, leading zeros (other than "0" itself),
// and any non-digit content.
func parseIntStrict(s string) (int, error) {
	if s == "" {
		return 0, fmt.Errorf("empty count string")
	}
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("non-digit character %q in count %q", r, s)
		}
		n = n*10 + int(r-'0')
	}
	return n, nil
}

// assertConnectorCountContract returns nil iff cmd/core/connectors.go,
// docs/smackerel.md §22.7, docs/smackerel.md §24-A, and docs/Development.md
// "Current Capabilities" all agree on the same positive connector count.
// Both §22.7 and §24-A are parsed from the same smackerelMd bytes.
func assertConnectorCountContract(connectorsGo, smackerelMd, developmentMd []byte) error {
	runtime, err := parseConnectorsGoCount(connectorsGo)
	if err != nil {
		return fmt.Errorf("contract violation (runtime parse): %w", err)
	}
	if runtime <= 0 {
		return fmt.Errorf("contract violation: cmd/core/connectors.go slice literal contains 0 identifiers — runtime registry is empty")
	}

	smackerel, err := parseSmackerelMdCount(smackerelMd)
	if err != nil {
		return fmt.Errorf("contract violation (smackerel.md parse): %w", err)
	}

	development, err := parseDevelopmentMdCount(developmentMd)
	if err != nil {
		return fmt.Errorf("contract violation (Development.md parse): %w", err)
	}

	tree, err := parseSmackerelMdTreeCount(smackerelMd)
	if err != nil {
		return fmt.Errorf("contract violation (smackerel.md §24-A parse): %w", err)
	}

	if runtime != smackerel || runtime != development || runtime != tree {
		return fmt.Errorf(
			"contract violation: connector count disagreement — cmd/core/connectors.go=%d, docs/smackerel.md §22.7=%d, docs/smackerel.md §24-A=%d, docs/Development.md=%d — all four MUST equal the runtime count; reconcile per spec 024 R-006 + BS-004",
			runtime, smackerel, tree, development,
		)
	}

	return nil
}

// TestConnectorCountContract_LiveFile asserts the contract holds against
// the live runtime registry and the live docs surfaces.
func TestConnectorCountContract_LiveFile(t *testing.T) {
	root := repoRoot(t)

	connectorsGo, err := os.ReadFile(filepath.Join(root, "cmd", "core", "connectors.go"))
	if err != nil {
		t.Fatalf("failed to read cmd/core/connectors.go: %v", err)
	}
	smackerelMd, err := os.ReadFile(filepath.Join(root, "docs", "smackerel.md"))
	if err != nil {
		t.Fatalf("failed to read docs/smackerel.md: %v", err)
	}
	developmentMd, err := os.ReadFile(filepath.Join(root, "docs", "Development.md"))
	if err != nil {
		t.Fatalf("failed to read docs/Development.md: %v", err)
	}

	if err := assertConnectorCountContract(connectorsGo, smackerelMd, developmentMd); err != nil {
		t.Fatalf("live connector-count contract violated (spec 024 R-006 + BS-004 + AC-5): %v", err)
	}

	runtime, _ := parseConnectorsGoCount(connectorsGo)
	t.Logf("contract OK: cmd/core/connectors.go + docs/smackerel.md §22.7 + docs/smackerel.md §24-A + docs/Development.md all agree on %d connectors (spec 024 R-006 + BS-004 + AC-5 in sync)", runtime)
}

// TestConnectorCountContract_AdversarialConnectorsGoLow proves the
// contract catches a regression where the runtime registry is silently
// shrunk to 15 while docs continue to claim 16.
func TestConnectorCountContract_AdversarialConnectorsGoLow(t *testing.T) {
	root := repoRoot(t)
	smackerelMd, err := os.ReadFile(filepath.Join(root, "docs", "smackerel.md"))
	if err != nil {
		t.Fatalf("failed to read docs/smackerel.md: %v", err)
	}
	developmentMd, err := os.ReadFile(filepath.Join(root, "docs", "Development.md"))
	if err != nil {
		t.Fatalf("failed to read docs/Development.md: %v", err)
	}

	// Synthetic connectors.go with only 15 identifiers in the slice
	// literal (qfDecisionsConn dropped) — drift from real docs (count 16).
	const fixture = `package main

func registerConnectors() {
	for _, c := range []connector.Connector{
		imapConn, caldavConn, ytConn, rssConn, keepConn,
		bmConn, browserHistConn, mapsConn, hospitableConn, guesthostConn,
		discordConn, twitterConn, weatherConn, alertsConn, marketsConn,
	} {
		_ = c
	}
}
`
	err = assertConnectorCountContract([]byte(fixture), smackerelMd, developmentMd)
	if err == nil {
		t.Fatal("adversarial contract test failed: connectors.go with 15 identifiers was accepted against real docs (count 16) — contract is tautological; it would NOT catch a regression where a connector is silently un-registered")
	}
	if !strings.Contains(err.Error(), "cmd/core/connectors.go=15") {
		t.Fatalf("adversarial contract test failed: error did not name the disagreeing runtime count: %v", err)
	}
	t.Logf("adversarial OK: connectors.go=15 vs docs=16 is rejected with: %v", err)
}

// TestConnectorCountContract_AdversarialSmackerelMdHigh proves the
// contract catches a regression where docs/smackerel.md §22.7 header is
// inflated above the real runtime count while runtime + Development.md
// stay correct. The synthetic header is set to runtime+1 so the fixture
// always drifts regardless of how many connectors exist (self-adjusts as
// the registry grows — currently 17).
func TestConnectorCountContract_AdversarialSmackerelMdHigh(t *testing.T) {
	root := repoRoot(t)
	connectorsGo, err := os.ReadFile(filepath.Join(root, "cmd", "core", "connectors.go"))
	if err != nil {
		t.Fatalf("failed to read cmd/core/connectors.go: %v", err)
	}
	developmentMd, err := os.ReadFile(filepath.Join(root, "docs", "Development.md"))
	if err != nil {
		t.Fatalf("failed to read docs/Development.md: %v", err)
	}

	runtime, err := parseConnectorsGoCount(connectorsGo)
	if err != nil {
		t.Fatalf("failed to parse runtime connector count: %v", err)
	}

	// Synthetic smackerel.md with the §22.7 header inflated to runtime+1 —
	// drift from the real runtime + real Development.md (both equal to
	// runtime). The §24-A tree header is held AT the runtime count so the
	// only disagreeing surface is §22.7 (this test pins the §22.7 inflation
	// mode specifically; §24-A drift is covered by AdversarialSmackerelMdTreeLow).
	inflated := runtime + 1
	fixture := fmt.Sprintf(`# Smackerel

### 22.7 Committed Connector Inventory (%d connectors)

(table body intentionally omitted — only the header line is parsed)

│   ├── Connector plugins (%d committed)
`, inflated, runtime)
	err = assertConnectorCountContract(connectorsGo, []byte(fixture), developmentMd)
	if err == nil {
		t.Fatalf("adversarial contract test failed: docs/smackerel.md header claiming %d was accepted against runtime (%d) — contract is tautological; it would NOT catch a regression where the §22.7 header is hand-inflated", inflated, runtime)
	}
	if !strings.Contains(err.Error(), fmt.Sprintf("docs/smackerel.md §22.7=%d", inflated)) {
		t.Fatalf("adversarial contract test failed: error did not name the inflated smackerel.md count: %v", err)
	}
	t.Logf("adversarial OK: smackerel.md=%d vs runtime+Development.md=%d is rejected with: %v", inflated, runtime, err)
}

// TestConnectorCountContract_AdversarialDevelopmentMdLow proves the
// contract catches the exact F1 regression that BUG-024-003 closed:
// docs/Development.md "- N passive connectors" bullet stale at 15
// while runtime + smackerel.md are both at 16.
func TestConnectorCountContract_AdversarialDevelopmentMdLow(t *testing.T) {
	root := repoRoot(t)
	connectorsGo, err := os.ReadFile(filepath.Join(root, "cmd", "core", "connectors.go"))
	if err != nil {
		t.Fatalf("failed to read cmd/core/connectors.go: %v", err)
	}
	smackerelMd, err := os.ReadFile(filepath.Join(root, "docs", "smackerel.md"))
	if err != nil {
		t.Fatalf("failed to read docs/smackerel.md: %v", err)
	}

	// Synthetic Development.md with the exact F1 drift: "- 15 passive
	// connectors (...)" bullet missing the 16th entry, while runtime and
	// smackerel.md are at 16.
	const fixture = `# Development

## Current Capabilities

- 15 passive connectors (IMAP email, CalDAV calendar, YouTube API, RSS/Atom, Bookmarks, Browser, Google Keep/Takeout, Google Maps, Hospitable STR, GuestHost STR, Discord, Twitter/X archive, Weather via Open-Meteo, Government Alerts via USGS, Financial Markets via Finnhub/CoinGecko)
- Knowledge graph
`
	err = assertConnectorCountContract(connectorsGo, smackerelMd, []byte(fixture))
	if err == nil {
		t.Fatal("adversarial contract test failed: docs/Development.md with stale '- 15 passive connectors' bullet was accepted against runtime + smackerel.md (both 16) — this is the exact F1 regression BUG-024-003 closed; contract is tautological")
	}
	if !strings.Contains(err.Error(), "docs/Development.md") || !strings.Contains(err.Error(), "=15") {
		t.Fatalf("adversarial contract test failed: error did not name the stale Development.md count: %v", err)
	}
	t.Logf("adversarial OK: Development.md=15 vs runtime+smackerel.md=16 is rejected with: %v", err)
}

// TestConnectorCountContract_AdversarialSmackerelMdTreeLow proves the
// contract catches the EXACT BUG-024-006 regression: the §24-A
// architecture-tree header "Connector plugins (N committed)" stale at one
// below the live count while §22.7, the runtime slice, and
// docs/Development.md all advanced. Before this packet, §24-A was an
// unpinned 4th surface, so this exact drift passed GREEN (the live-file
// test logged "all agree on N" while §24-A silently lagged). The fixture
// self-adjusts to the live registry size (runtime-1) so it stays
// adversarial as connectors are added, and it asserts the diagnostic names
// the §24-A surface specifically — not merely that *some* surface drifted.
func TestConnectorCountContract_AdversarialSmackerelMdTreeLow(t *testing.T) {
	root := repoRoot(t)
	connectorsGo, err := os.ReadFile(filepath.Join(root, "cmd", "core", "connectors.go"))
	if err != nil {
		t.Fatalf("failed to read cmd/core/connectors.go: %v", err)
	}
	developmentMd, err := os.ReadFile(filepath.Join(root, "docs", "Development.md"))
	if err != nil {
		t.Fatalf("failed to read docs/Development.md: %v", err)
	}

	runtime, err := parseConnectorsGoCount(connectorsGo)
	if err != nil {
		t.Fatalf("failed to parse runtime connector count: %v", err)
	}
	if runtime < 2 {
		t.Fatalf("runtime connector count %d is too small to construct a runtime-1 §24-A drift fixture", runtime)
	}

	// Synthetic docs/smackerel.md whose §22.7 header is CORRECT at the
	// runtime count but whose §24-A architecture-tree header is stale at
	// runtime-1 — the precise BUG-024-006 shape (§24-A lagged the 16→17
	// transition). Runtime + Development.md are passed as the real files
	// (both equal to runtime), so §24-A is the sole disagreeing surface.
	stale := runtime - 1
	fixture := fmt.Sprintf(`# Smackerel

### 22.7 Committed Connector Inventory (%d connectors)

(table body intentionally omitted — only the §22.7 header line is parsed)

│   ├── Connector plugins (%d committed)
│   │   ├── Gov Alerts (alerts/)
│   │   └── QF Decisions (qfdecisions/ — spec 041 read-only companion)
│   │   Planned connectors:
`, runtime, stale)

	err = assertConnectorCountContract(connectorsGo, []byte(fixture), developmentMd)
	if err == nil {
		t.Fatalf("adversarial contract test failed: docs/smackerel.md §24-A tree claiming %d was accepted against runtime (%d) + §22.7 (%d) + Development.md (%d) — §24-A is NOT pinned; this is the exact BUG-024-006 blind spot where §24-A lags the live count and the guard stays GREEN", stale, runtime, runtime, runtime)
	}
	if !strings.Contains(err.Error(), fmt.Sprintf("docs/smackerel.md §24-A=%d", stale)) {
		t.Fatalf("adversarial contract test failed: error did not name the stale §24-A count (expected `docs/smackerel.md §24-A=%d`): %v", stale, err)
	}
	t.Logf("adversarial OK: §24-A=%d vs runtime+§22.7+Development.md=%d is rejected with: %v", stale, runtime, err)
}
