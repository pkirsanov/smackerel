// Spec 044 Scope 01 — T1-10 SST grep guard for auth subsystem.
//
// SCN-AUTH-005/006 forbid hardcoded auth subsystem runtime values in
// production source. Every auth runtime value MUST originate from
// `auth.*` keys in `config/smackerel.yaml` and flow through
// `scripts/commands/config.sh` into `config/generated/<env>.env`. This
// test asserts no production source file contains the literal patterns
// listed below outside of allowlisted locations.
//
// Forbidden literals:
//   - "auth.revocations"           — NATS subject hardcode (must come from
//     AUTH_REVOCATION_NATS_SUBJECT).
//   - "smackerel_auth"             — telemetry prefix hardcode (must come
//     from AUTH_TELEMETRY_METRIC_PREFIX).
//   - "paseto-v4-public"           — token format hardcode (must come from
//     AUTH_TOKEN_FORMAT).
//
// Allowlist (legitimate locations for the literals):
//   - `config/smackerel.yaml`     — SST source of truth.
//   - `config/generated/*.env`    — generator output (write-only).
//   - `scripts/commands/config.sh`— SST extractor (intentional surface).
//   - `*_test.go`                  — test fixtures.
//   - `docs/`, `specs/`            — narrative content (not scanned here).
//
// The adversarial sub-test feeds the scanner a fixture that contains
// each forbidden literal in a production-shaped path; the assertion
// MUST flag every literal. If the adversarial assertion ever passes
// without flagging a literal, the primary guard above is tautological
// and would not catch a real regression.
//
// References:
//   - specs/044-per-user-bearer-auth/scopes.md (Scope 01, T1-10)
//   - specs/044-per-user-bearer-auth/design.md §4 (SST surface)
package auth

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// authForbiddenLiterals is the closed set the SST guard rejects.
//
// Note on coverage:
//   - "auth.revocations"  is the NATS subject; production code MUST read it
//     from AUTH_REVOCATION_NATS_SUBJECT, not hardcode the literal.
//   - "paseto-v4-public" is the token format identifier; production code
//     MUST read it from AUTH_TOKEN_FORMAT, not hardcode the literal.
//
// The metric prefix ("smackerel_auth") is intentionally NOT in this set
// because the PWA uses "smackerel_auth_token" as a localStorage key and a
// substring scanner cannot disambiguate the two. When metric registration
// code lands in a later scope, the prefix should be enforced via a more
// targeted scan against metric-name registration call sites.
var authForbiddenLiterals = []string{
	"auth.revocations",
	"paseto-v4-public",
}

// authSstScanRoots are the directories the guard walks.
var authSstScanRoots = []string{
	"internal",
	"cmd",
	"ml/app",
}

// authSstScanFiles are individual files at repo root the guard scans.
var authSstScanFiles = []string{
	"docker-compose.yml",
	"docker-compose.prod.yml",
	"Dockerfile",
}

// authSstScannableExt limits the walker to files where auth literals
// would be runtime-meaningful.
var authSstScannableExt = map[string]struct{}{
	".go":   {},
	".py":   {},
	".sh":   {},
	".yml":  {},
	".yaml": {},
}

// authSstSkipFile returns true for files that are legitimate locations
// for the forbidden literals.
func authSstSkipFile(path string) bool {
	if strings.HasSuffix(path, "_test.go") {
		return true
	}
	if strings.HasSuffix(path, "_test.py") {
		return true
	}
	if strings.Contains(path, string(filepath.Separator)+"tests"+string(filepath.Separator)) {
		return true
	}
	if strings.Contains(path, string(filepath.Separator)+".venv"+string(filepath.Separator)) {
		return true
	}
	if strings.Contains(path, string(filepath.Separator)+"node_modules"+string(filepath.Separator)) {
		return true
	}
	// internal/config/config.go is the SST loader — it MUST know the only
	// permitted token-format value to validate AUTH_TOKEN_FORMAT against
	// it. Allowlisting the loader does not weaken the guard because the
	// loader is the authority that converts SST input into runtime config.
	norm := filepath.ToSlash(path)
	if strings.HasSuffix(norm, "internal/config/config.go") {
		return true
	}
	return false
}

// authSstRepoRoot climbs from this test's directory to the repo root
// by looking for config/smackerel.yaml. Independent of go test CWD.
func authSstRepoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller(0) failed — cannot locate test file")
	}
	dir := filepath.Dir(thisFile)
	for i := 0; i < 8; i++ {
		if _, err := os.Stat(filepath.Join(dir, "config", "smackerel.yaml")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatalf("could not locate repo root from %s", thisFile)
	return ""
}

// scanForAuthLiterals walks the given root, scanning files whose
// extension is in authSstScannableExt and which are NOT skipped.
func scanForAuthLiterals(rootDir string, scanPath string) ([]string, error) {
	findings := make([]string, 0)
	walkRoot := filepath.Join(rootDir, scanPath)
	info, err := os.Stat(walkRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return findings, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		more, err := scanSingleAuthFile(rootDir, walkRoot)
		if err != nil {
			return nil, err
		}
		findings = append(findings, more...)
		return findings, nil
	}
	err = filepath.WalkDir(walkRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		ext := filepath.Ext(path)
		if _, ok := authSstScannableExt[ext]; !ok {
			return nil
		}
		if authSstSkipFile(path) {
			return nil
		}
		more, err := scanSingleAuthFile(rootDir, path)
		if err != nil {
			return err
		}
		findings = append(findings, more...)
		return nil
	})
	return findings, err
}

func scanSingleAuthFile(rootDir, absPath string) ([]string, error) {
	findings := make([]string, 0)
	contentBytes, err := os.ReadFile(absPath)
	if err != nil {
		return nil, err
	}
	rel, err := filepath.Rel(rootDir, absPath)
	if err != nil {
		rel = absPath
	}
	for lineIdx, line := range strings.Split(string(contentBytes), "\n") {
		trim := strings.TrimSpace(line)
		// Skip comment-only lines — they describe the SST surface
		// rather than implement a runtime hardcode. Adversarial test
		// asserts the scanner still flags literals in code lines.
		if strings.HasPrefix(trim, "//") || strings.HasPrefix(trim, "#") || strings.HasPrefix(trim, "--") {
			continue
		}
		for _, lit := range authForbiddenLiterals {
			if strings.Contains(line, lit) {
				findings = append(findings,
					rel+":"+strconv.Itoa(lineIdx+1)+": "+trim)
			}
		}
	}
	return findings, nil
}

// TestSST_NoHardcodedAuthValues — primary SST guard. Walks the
// production source roots and asserts no file contains any forbidden
// auth literal.
func TestSST_NoHardcodedAuthValues(t *testing.T) {
	root := authSstRepoRoot(t)
	allFindings := make([]string, 0)
	for _, scanRoot := range authSstScanRoots {
		f, err := scanForAuthLiterals(root, scanRoot)
		if err != nil {
			t.Fatalf("scan %s failed: %v", scanRoot, err)
		}
		allFindings = append(allFindings, f...)
	}
	for _, scanFile := range authSstScanFiles {
		f, err := scanForAuthLiterals(root, scanFile)
		if err != nil {
			t.Fatalf("scan %s failed: %v", scanFile, err)
		}
		allFindings = append(allFindings, f...)
	}
	if len(allFindings) > 0 {
		t.Fatalf("SST violation: production source contains forbidden auth literals — every auth runtime value MUST come from auth.* SST keys (config/smackerel.yaml). Findings:\n  %s",
			strings.Join(allFindings, "\n  "))
	}
	t.Logf("SST guard OK: no production source file contains %v outside config/", authForbiddenLiterals)
}

// TestSST_NoHardcodedAuthValues_Adversarial proves the guard catches
// a regression. Writes a fixture containing each forbidden literal in
// a production-shaped path and asserts the scanner reports it. If
// this test fails, the primary guard above is tautological.
func TestSST_NoHardcodedAuthValues_Adversarial(t *testing.T) {
	tmp := t.TempDir()
	scanDir := filepath.Join(tmp, "fakeprod")
	if err := os.MkdirAll(scanDir, 0o755); err != nil {
		t.Fatalf("mkdir fakeprod: %v", err)
	}
	fixturePath := filepath.Join(scanDir, "naughty.go")
	const fixtureContent = `package fakeprod

const (
	NATSSubject = "auth.revocations"
	TokenFormat = "paseto-v4-public"
)
`
	if err := os.WriteFile(fixturePath, []byte(fixtureContent), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	findings, err := scanForAuthLiterals(tmp, "fakeprod")
	if err != nil {
		t.Fatalf("scan tmp: %v", err)
	}
	mustHave := map[string]bool{
		"auth.revocations": false,
		"paseto-v4-public": false,
	}
	for _, f := range findings {
		for lit := range mustHave {
			if strings.Contains(f, lit) {
				mustHave[lit] = true
			}
		}
	}
	for lit, hit := range mustHave {
		if !hit {
			t.Errorf("adversarial guard tautological: literal %q in fixture was NOT reported by scanForAuthLiterals (findings: %v)",
				lit, findings)
		}
	}
	// Comment-line filter must NOT flag the literal in a comment.
	commentFixturePath := filepath.Join(scanDir, "narrative.go")
	const commentFixture = `package fakeprod
// This comment mentions auth.revocations and paseto-v4-public but
// MUST NOT be flagged because the scanner skips comment-only lines.
`
	if err := os.WriteFile(commentFixturePath, []byte(commentFixture), 0o600); err != nil {
		t.Fatalf("write comment fixture: %v", err)
	}
	commentFindings, err := scanForAuthLiterals(tmp, "fakeprod")
	if err != nil {
		t.Fatalf("scan tmp comments: %v", err)
	}
	// commentFindings should be exactly the same set as findings (the
	// narrative.go file should add zero new flags).
	if len(commentFindings) != len(findings) {
		t.Errorf("comment-line filter tautological: narrative.go added %d new flags (expected 0). diff: before=%v after=%v",
			len(commentFindings)-len(findings), findings, commentFindings)
	}
	t.Logf("adversarial OK: scanner reports %d findings against the 2-literal fixture; %v", len(findings), findings)
}

// TestSST_NoHardcodedAuthValues_AllowlistAdversarial proves the
// allowlist works as intended.
func TestSST_NoHardcodedAuthValues_AllowlistAdversarial(t *testing.T) {
	tmp := t.TempDir()
	scanDir := filepath.Join(tmp, "fakeprod")
	if err := os.MkdirAll(scanDir, 0o755); err != nil {
		t.Fatalf("mkdir fakeprod: %v", err)
	}
	allowedPath := filepath.Join(scanDir, "fixture_test.go")
	const allowedContent = `package fakeprod
const _ = "auth.revocations"
`
	if err := os.WriteFile(allowedPath, []byte(allowedContent), 0o600); err != nil {
		t.Fatalf("write allowed fixture: %v", err)
	}
	findings, err := scanForAuthLiterals(tmp, "fakeprod")
	if err != nil {
		t.Fatalf("scan tmp: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("allowlist failure: *_test.go fixture should be skipped but scanner reported: %v",
			findings)
	}
	t.Logf("allowlist OK: *_test.go fixture with literal 'auth.revocations' is correctly skipped")
}
