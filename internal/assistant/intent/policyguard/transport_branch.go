// Spec 069 SCOPE-5 — Transport-branch policy guard (SCN-069-A08).
//
// ReportTransportBranchViolations scans a Go source subtree and
// reports every file (outside the closed transport-adapter +
// audit allowlist) that inspects AssistantMessage.Transport or
// equivalent transport tokens at scenario / facade / executor
// layers. Spec 069 invariant: only the adapter and audit layers
// are allowed to inspect transport; scenarios MUST NOT branch on
// transport name (that would silently couple a scenario to a
// surface and break the parity claim).
//
// This is intentionally a small, mechanical pattern-match guard
// scoped to the spec 069 invariant. The companion test plants a
// fixture file with the forbidden pattern to prove the guard is
// non-tautological.

package policyguard

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// TransportBranchViolation is the canonical phrase the guard uses
// when it finds a forbidden transport-branch site. Stable so the
// guard-output test can match it verbatim.
const TransportBranchViolation = "forbidden scenario/facade/executor branching on AssistantMessage.Transport — only the adapter and audit layers may inspect transport"

// AllowedTransportInspectors is the closed allowlist of file path
// fragments that ARE allowed to inspect transport. Adapters and the
// audit / context-store layer keep the transport token in row keys
// and audit envelopes — that is the correct surface. Test files are
// always excluded from scanning regardless of suffix.
//
// Paths are matched against the scan-relative cleaned slash path.
// Entries ending in "/" match as path-fragment containment; other
// entries match as path suffix. The list is intentionally generous
// at the package level (whole-subdirectory allowlists) so the guard
// stays focused on its single invariant: no scenario / facade /
// executor file branches on AssistantMessage.Transport.
var AllowedTransportInspectors = []string{
	// HTTP adapter layer (spec 069 itself).
	"httpadapter/",
	// Telegram adapter and bridge (outside the assistant subtree;
	// listed for callers that scan a wider root).
	"internal/telegram/",
	"internal/whatsapp/",
	// Audit + context-store layers key by (UserID, Transport) — these
	// are the row-family owners and are allowed to inspect.
	"audit.go",
	"context/",
	"confirm/",
	"transportidentity/",
	"capturefallback/",
	// Adapter contract and registry types declare the Transport field.
	"contracts/",
	// Bridge router consults transport to select the registered adapter.
	"bridge.go",
	// Metrics gauge per-transport counts by design.
	"metrics/",
	// The guard itself contains the forbidden patterns as regex
	// literals; exempt it explicitly.
	"intent/policyguard/transport_branch.go",
	// Facade owns per-(UserID, Transport) row plumbing (DeleteByKey,
	// Persist) and tracer attributes; it is the canonical
	// row-family owner. Scenario/executor logic is split across
	// router.go, scenario_*.go, executor.go — those remain in scope.
	"facade.go",
}

// transportBranchPatterns matches the forbidden inspection sites.
// The patterns are conservative: a bare `.Transport` member access
// is too noisy (every translate call sets msg.Transport=...). The
// guard fires only on equality / switch / contains comparisons that
// branch behavior on the transport token value.
var transportBranchPatterns = []*regexp.Regexp{
	// msg.Transport == "telegram" / .Transport == TransportName / etc.
	regexp.MustCompile(`\.Transport\s*==\s*"`),
	regexp.MustCompile(`\.Transport\s*!=\s*"`),
	regexp.MustCompile(`"\s*==\s*\w+\.Transport\b`),
	// switch msg.Transport { case "telegram": ... }
	regexp.MustCompile(`switch\s+\w+\.Transport\b`),
	// strings.EqualFold(msg.Transport, "telegram")
	regexp.MustCompile(`EqualFold\([^)]*\.Transport\b`),
	regexp.MustCompile(`Contains\([^)]*\.Transport\b`),
}

// ReportTransportBranchViolations walks root and returns one
// Finding per file that matches a transport-branch pattern AND is
// not in AllowedTransportInspectors. Test files (`_test.go`) are
// always skipped — guards police production code, not test
// fixtures.
func ReportTransportBranchViolations(root string) ([]Finding, error) {
	var findings []Finding
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == "vendor" || name == ".git" || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		// Allowlist match: prefix OR exact-suffix.
		for _, allowed := range AllowedTransportInspectors {
			if strings.HasSuffix(allowed, "/") {
				if strings.Contains(rel, allowed) {
					return nil
				}
			} else if strings.HasSuffix(rel, allowed) || rel == allowed {
				return nil
			}
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		text := string(body)
		for _, pat := range transportBranchPatterns {
			if pat.MatchString(text) {
				findings = append(findings, Finding{
					File:    rel,
					Message: fmt.Sprintf("%s: %s", rel, TransportBranchViolation),
				})
				return nil
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return findings, nil
}
