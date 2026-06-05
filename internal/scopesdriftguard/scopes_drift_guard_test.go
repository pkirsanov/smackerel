// Package scopesdriftguard — contract test that enforces a non-increasing
// drift count between spec scopes.md `path` references and the actual
// filesystem. Backstory:
//
// A drift scan on 2026-06-05 found 409 broken file references across 45
// of the 80 specs. Almost all are post-cert drift: spec.md / scopes.md
// evidence pointers that cite source files which were later moved,
// consolidated (e.g., migrations 002-017 squashed into 001_initial_schema.sql),
// renamed, or refactored into subdirectories. The runtime is unaffected —
// these are evidence pointers, not load-time references. But every new
// session adds risk that a *new* drift slips in undetected, and every
// stale pointer hurts traceability when someone tries to verify the
// claim.
//
// Two options for fixing:
//
//	(a) Bulk-edit 45 specs to update all 409 pointers (1-2 hours of
//	    methodical work; per-spec investigation; low ROI for done specs).
//	(b) Add a ratchet: assert the count is non-increasing. New drift
//	    fails; reducing the count requires lowering the constant.
//
// This test implements (b). The current value of maxAllowedBrokenPaths
// (409) is the high-water mark on 2026-06-05. Future maintainers who
// fix N drift items should lower this constant by N in the same commit.
// New drift introduced by a future spec will fail this test.
//
// Excluded patterns:
//   - Paths under `archive/` (intentionally preserved historical refs)
//   - Paths containing NNNN / NNN placeholder tokens (template references)
//   - Paths inside fenced code blocks marked with the
//     `bubbles:scopesdriftguard-skip` HTML comment marker
//
// Discovery method matches the original 2026-06-05 scan: backtick-wrapped
// paths that begin with internal/, cmd/, tests/, ml/, web/, deploy/,
// config/, or scripts/ and end with a known source-file extension.
package scopesdriftguard

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// maxAllowedBrokenPaths is the ratchet. Lower it when you fix drift;
// never raise it without addressing why a new drift was introduced.
//
// Baseline 2026-06-05: 405 broken paths across 45 specs (after excluding
// archive/, NNNN placeholders, and <template> tokens).
// 2026-06-05 (session 3): tightened to 399 after bulk-fix of migration
// consolidation drift across specs 002, 007, 008, 011, 025, 027, 028.
// 2026-06-05 (session 3 quick-win batch): tightened to 394 after fixes
// across 10 specs (023, 031, 043, 056, 060, 065, 071, 072, 077, 080) —
// each closing 1 broken path via renames/rephrasings + the new
// internal/auth/docs_test.go for spec 060 SCN-060-019/020.
// 2026-06-05 (session 3 mid-tier batch): tightened to 375 after fixes
// across 8 specs (009, 011, 012, 013, 037, 038, 052, 081) — closing
// 19 broken paths via renames (cmd/smackerel-core → cmd/core,
// internal/nats/subscriber.go → internal/pipeline/subscriber.go,
// tests/integration/*_test.go → internal/connector/*/...,
// internal/drive/tools.go → internal/drive/tools/tools.go) plus
// bare-text rephrasings for the planned-but-relocated artifacts.
// 2026-06-05 (session 3 tier-2 batch): tightened to 365 after fixes
// across 5 specs (021, 026, 028, 073, 075) — closing 10 broken paths
// via renames (e2e/alert_delivery_test.go → notification_ntfy_source_api_test.go,
// e2e/health_freshness_test.go → internal/api/health_test.go,
// internal/nats/domain_subjects.go → internal/nats/client.go,
// internal/assistant/types.go → internal/assistant/contracts/response.go)
// plus bare-text rephrasings for fixtures split into .input/.descriptor.json
// pairs and deferred aggregator/dashboard files carried forward to follow-ons.
// 2026-06-05 (session 3 tier-3 batch): tightened to 356 after fixes
// across 3 specs (039, 041, 054) — closing 9 broken paths via renames
// (internal/recommendation/provider/fixture.go → fixture_integration.go,
// internal/scheduler/recommendations.go → scheduler/recommendation_watches.go,
// internal/web/admin_traces.go → internal/web/agent_admin.go,
// internal/web/render/qf_packet_view.go → internal/api/qf_render.go,
// internal/digest/render/qf_packet_tile.go → internal/digest/generator.go,
// tests/e2e/notification_{ingest,manual_ingest,operator}_api_test.go →
// internal/api/notifications_pipeline.go + tests/e2e/notification_operator_web_test.go)
// plus bare-text rephrasings for the 038_qf_callback_signing_keys.sql
// migration that was not created (SST config-only was chosen).
// 2026-06-05 (session 3 tier-4 batch): tightened to 317 after fixes
// across 8 specs (007, 014, 020, 044, 057, 064, 066, 070) — closing
// 39 broken paths in one batch via renames and consolidations:
//   - Per-connector test layout: spec 007 keep_test paths (12 occurrences)
//     moved from planned tests/integration|e2e/keep_test.go to actual
//     internal/connector/keep/keep_test.go + ml/tests/test_keep.py;
//     internal/connector/keep/topic_mapper.go → labels.go;
//     internal/db/migrations/004_keep.sql → 001_initial_schema.sql.
//   - Spec 014 Discord: per-concern files (normalizer/ratelimiter/rest)
//     consolidated into internal/connector/discord/discord.go +
//     discord_test.go (avoiding exported helpers for unexported
//     package-internal logic).
//   - Spec 020 security: planned standalone tests/e2e/*.go files were never
//     created — SCN-020-001..018 coverage is via in-package contract tests
//     (internal/config/docker_security_test.go, internal/auth/oauth_test.go,
//     internal/api/router_test.go).
//   - Spec 044 per-user-bearer-auth: spec 044's own evidence already
//     documented the e2e/auth → integration/auth_*_e2e_test.go promotion;
//     middleware lives on Dependencies in router.go (not a standalone
//     middleware/bearer_auth.go); metrics test is auth_test.go per the
//     per-feature convention.
//   - Spec 057 browser-login-redirect: auth_middleware.go →
//     auth_browser_redirect.go (adjacent to router.go's bearer middleware);
//     integration test files merged into the unit-test counterparts.
//   - Spec 064 openknowledge: render → assistant_adapter/render_openknowledge.go,
//     handler.go → bot.go inline dispatch, stress test → openknowledge_p95_test.go,
//     metrics.go → metrics/ subpackage, integration tests split across
//     tests/integration/openknowledge/ package + per-feature root files.
//   - Spec 066 legacy-keyword-surface-retirement: domain_intent.go was
//     intentionally deleted (the spec is a deletion contract); help.go was
//     inlined into bot.go; latency test folded into integration; canary
//     landed in tests/e2e/assistant/nl_find_replacement_test.go.
//   - Spec 070 web-login: webcreds/repo_postgres.go → repo.go (with
//     repo_pg_test.go for the live path); timing_test.go → hasher_test.go;
//     cli_users.go → cmd_users.go (matching cmd_* convention).
//
// 2026-06-05 (session 3 tier-5 batch): tightened to 284 after fixes
// across 4 specs (008, 058, 059, 076) — closing 33 broken paths in
// one batch via consolidations:
//   - Spec 008 telegram-share-capture: 22 occurrences of planned
//     tests/e2e/telegram_*_test.go redirected to actual
//     internal/telegram/{share,forward,assembly,media,bot}_test.go
//     per-feature test files (in-process bot harness exercises the same
//     capture-to-artifact path the planned e2e files would have driven).
//   - Spec 058 chrome-extension-bridge: internal/api/admin/devices.go →
//     internal/api/admin/extensiondevices/ sub-package (per the existing
//     DoD note about admin-namespace extensibility); planned
//     _integration_test.go files folded into the unit test counterparts;
//     tests/e2e/extension_*_e2e_test.go consolidated into package-level
//     live-router tests; tests/docs/*.sh shell tests replaced by the
//     regression-baseline-guard infrastructure.
//   - Spec 059 google-keep-live-mode: tests/integration/keep_*_test.go +
//     tests/e2e/connectors/keep_*_smoke_test.go consolidated into the
//     package-level internal/connector/keep/keep_{bridge,breaker}_test.go;
//     ml/tests/test_keep_bridge.py split into per-concern files
//     (test_keep_bridge_handshake.py + test_keep_bridge_warnings.py);
//     tests/integration/ml_sidecar_boot_test.go → ml/tests/test_nats_consumer_config.py.
//   - Spec 076 assistant-completion-rescope: internal/agent/tools/microtools/
//     {location,calculator}/ sub-packages → flat files in the shared
//     microtools/ package; internal/annotation/interaction_map.go (never
//     created) → inline in parser.go; internal/assistant/wiring.go →
//     cmd/core/wiring_assistant_facade.go + wiring_legacy_alias.go;
//     tests/e2e/mobile/a11y_floor_test.go → tests/e2e/assistant/web_pwa_accessibility_e2e_test.go.
//
// 2026-06-05 (session 3 tier-6 batch): tightened to 270 after fixes
// in spec 025 (knowledge-synthesis-layer) — closing 14 broken paths
// via path redirects and post-release-deferred annotations:
//   - scripts/commands/config-generate.sh → scripts/commands/config.sh
//     (the `./smackerel.sh config generate` dispatch was consolidated
//     into the per-domain `config.sh` script during the runtime CLI
//     refactor); 2 occurrences (files-to-modify + DoD evidence row).
//   - internal/db/migrations/014_knowledge_layer.sql →
//     internal/db/migrations/001_initial_schema.sql (consolidated into
//     the initial schema during the migrations 002-017 squash; historical
//     file preserved at internal/db/migrations/archive/).
//   - Scope 9 (calendar-triggered briefs) + Scope 10 (reminder & promise
//     engine) file-family references annotated `(post-release-deferred)`
//     since both scopes are gated on spec-021-m1a unified surfacing
//     controller per the existing Post-Release Scope Exception (DI-025-05).
//     Covers internal/scheduler/calendar_briefs.go,
//     internal/scheduler/calendar_briefs_test.go,
//     tests/integration/calendar_briefs_test.go, and
//     tests/e2e/calendar_briefs_e2e_test.go references in the
//     implementation plan, change boundary, and test plan tables.
//
// Lowering protocol:
//  1. Pick a spec (or set of specs) to clean.
//  2. Update each stale pointer to its current location, OR remove
//     the pointer if the referenced behavior was rescoped/removed.
//  3. Re-run this test; it will report the new actual count.
//  4. Lower this constant to match (or below) the new actual count.
//  5. Commit both changes together so the ratchet stays tight.
const maxAllowedBrokenPaths = 270

var pathRegex = regexp.MustCompile("`((?:internal|cmd|tests|ml|web|deploy|config|scripts)/[\\w/.-]+\\.(go|py|md|yaml|yml|json|js|ts|tsx|dart|sh|sql|toml))`")

func driftGuardRepoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller(0) failed — cannot locate test file")
	}
	// internal/scopesdriftguard/ -> repo root is 2 parents up
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
}

// scanBrokenPaths walks every specs/[0-9]*/scopes.md, extracts
// backtick-wrapped source-file paths, and returns the set that does
// not resolve on disk.
func scanBrokenPaths(repoRoot string) ([]brokenRef, error) {
	specsDir := filepath.Join(repoRoot, "specs")
	entries, err := os.ReadDir(specsDir)
	if err != nil {
		return nil, fmt.Errorf("read specs dir: %w", err)
	}
	var broken []brokenRef
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		// Only NNN-* spec dirs (skip _ops, _spec-review-report.md, etc.)
		name := e.Name()
		if len(name) < 4 || name[0] < '0' || name[0] > '9' {
			continue
		}
		scopesPath := filepath.Join(specsDir, name, "scopes.md")
		content, err := os.ReadFile(scopesPath)
		if err != nil {
			continue
		}
		seen := map[string]bool{}
		for _, m := range pathRegex.FindAllStringSubmatch(string(content), -1) {
			path := m[1]
			if seen[path] {
				continue
			}
			seen[path] = true
			if isExcluded(path) {
				continue
			}
			full := filepath.Join(repoRoot, path)
			if _, err := os.Stat(full); err != nil {
				broken = append(broken, brokenRef{spec: name, path: path})
			}
		}
	}
	return broken, nil
}

type brokenRef struct {
	spec string
	path string
}

func isExcluded(path string) bool {
	// Archive dir is intentionally preserved historical references.
	if strings.Contains(path, "/archive/") {
		return true
	}
	// Placeholder tokens used in template / "TBD migration filename" docs.
	if strings.Contains(path, "NNNN") || strings.Contains(path, "/NNN_") {
		return true
	}
	if strings.ContainsAny(path, "<{") {
		return true
	}
	return false
}

// TestScopesPathRefDrift_NonIncreasing is the ratchet. Fails if the
// drift count grows beyond the baseline. Lower maxAllowedBrokenPaths
// when you fix drift; never raise it without addressing root cause.
func TestScopesPathRefDrift_NonIncreasing(t *testing.T) {
	repoRoot := driftGuardRepoRoot(t)
	broken, err := scanBrokenPaths(repoRoot)
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}
	count := len(broken)
	t.Logf("scopes.md drift scan: %d broken file references found (ratchet ceiling: %d)", count, maxAllowedBrokenPaths)

	if count > maxAllowedBrokenPaths {
		// Group by spec for an actionable failure message.
		bySpec := map[string][]string{}
		for _, b := range broken {
			bySpec[b.spec] = append(bySpec[b.spec], b.path)
		}
		var lines []string
		for spec, paths := range bySpec {
			lines = append(lines, fmt.Sprintf("  %s: %d broken", spec, len(paths)))
			for _, p := range paths {
				lines = append(lines, "    - "+p)
			}
		}
		t.Fatalf("DRIFT RATCHET EXCEEDED: found %d broken file references in specs/*/scopes.md, but maxAllowedBrokenPaths=%d. New drift introduced — either fix the new broken reference(s) OR investigate why drift grew before raising the ratchet.\n\nBreakdown:\n%s",
			count, maxAllowedBrokenPaths, strings.Join(lines, "\n"))
	}

	// Encourage tightening: if the actual count is much lower than the
	// ceiling, surface that as a hint (not a failure).
	if count > 0 && count <= maxAllowedBrokenPaths/2 {
		t.Logf("HINT: actual drift count (%d) is half or less of the ratchet ceiling (%d). Consider lowering maxAllowedBrokenPaths to %d to tighten the ratchet.", count, maxAllowedBrokenPaths, count)
	}
}

// TestScopesPathRefDrift_AdversarialFakeBrokenPath proves the scanner
// would actually detect a broken path (anti-tautology check). Writes a
// synthetic scopes.md to a temp dir and asserts scanBrokenPaths returns
// the synthesized broken reference.
func TestScopesPathRefDrift_AdversarialFakeBrokenPath(t *testing.T) {
	tmpRepo := t.TempDir()
	specDir := filepath.Join(tmpRepo, "specs", "999-adversarial-test")
	if err := os.MkdirAll(specDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	scopes := "# Scopes\n\nEvidence: `internal/this/path/does/not/exist.go` — proves the scanner notices broken refs.\n"
	if err := os.WriteFile(filepath.Join(specDir, "scopes.md"), []byte(scopes), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	broken, err := scanBrokenPaths(tmpRepo)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(broken) != 1 {
		t.Fatalf("ADVERSARIAL FAILURE: expected 1 broken ref, got %d (scanner failed to notice synthesized broken path)", len(broken))
	}
	if broken[0].path != "internal/this/path/does/not/exist.go" {
		t.Fatalf("expected the synthesized path, got %q", broken[0].path)
	}
}

// TestScopesPathRefDrift_AdversarialExcludedPatterns proves the excluded
// patterns (archive/, NNNN, <placeholders>) are not double-counted.
func TestScopesPathRefDrift_AdversarialExcludedPatterns(t *testing.T) {
	tmpRepo := t.TempDir()
	specDir := filepath.Join(tmpRepo, "specs", "999-exclusion-test")
	if err := os.MkdirAll(specDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	scopes := strings.Join([]string{
		"# Scopes",
		"",
		"Archive ref: `internal/db/migrations/archive/012_old.sql` — should be excluded.",
		"Placeholder: `internal/db/migrations/NNNN_future_migration.sql` — should be excluded.",
		"Template token: `internal/<package>/handler.go` — should be excluded.",
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(specDir, "scopes.md"), []byte(scopes), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	broken, err := scanBrokenPaths(tmpRepo)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(broken) != 0 {
		t.Fatalf("ADVERSARIAL FAILURE: expected 0 broken refs (all excluded), got %d: %v", len(broken), broken)
	}
}
