// Spec 043 Scope 1 — T1-06 SST grep guard.
//
// SCN-OLLAMA-006 forbids hardcoded Ollama runtime values in production
// source. Every Ollama runtime value MUST originate from
// `infrastructure.ollama.*` keys in `config/smackerel.yaml` and flow through
// the generator into `config/generated/<env>.env`. This test asserts no
// production source file contains the literal patterns `11434`, `qwen2.5`,
// or `ollama/ollama:` outside of `config/`.
//
// Allowlist (legitimate locations for the literals):
//   - `config/smackerel.yaml` — SST source of truth.
//   - `config/generated/*.env` — generator output (write-only artifact).
//   - `*_test.go`, `*_test.py`, `ml/tests/` — test fixtures with explicit
//     intent to use deterministic test values.
//   - `docs/`, `specs/`, `.github/`, `bubbles/` — narrative content.
//   - `ml/.venv/`, `node_modules/`, `.git/` — vendored / generated trees.
//
// The adversarial sub-test feeds the assertion a fixture that contains a
// forbidden literal in a production-shaped path; the assertion MUST flag it.
//
// References:
//   - specs/043-ollama-test-infrastructure/spec.md (FR-OLLAMA-006)
//   - specs/043-ollama-test-infrastructure/scopes.md (Scope 1, T1-06)
//   - specs/043-ollama-test-infrastructure/design.md §3 (configuration plan)
package config

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// forbiddenOllamaLiterals is the closed set the SST guard rejects. The
// design.md §3 "12 SST keys" hoist these out of source into config/.
var forbiddenOllamaLiterals = []string{
	"11434",
	"qwen2.5",
	"ollama/ollama:",
}

// sstGuardScanRoots are the directories the guard walks. These match the
// production source tree per scopes.md SCN-OLLAMA-006 ("internal/, cmd/,
// ml/, scripts/, Dockerfile, docker-compose*.yml").
var sstGuardScanRoots = []string{
	"internal",
	"cmd",
	"ml/app",
	"scripts",
}

// sstGuardScanFiles are individual files at repo root the guard scans.
var sstGuardScanFiles = []string{
	"docker-compose.yml",
	"docker-compose.prod.yml",
	"Dockerfile",
}

// sstGuardScannableExt limits the walker to files where Ollama literals
// would be runtime-meaningful (matches the spec's `--include` glob list).
var sstGuardScannableExt = map[string]struct{}{
	".go":   {},
	".py":   {},
	".sh":   {},
	".yml":  {},
	".yaml": {},
}

// sstGuardSkipFile returns true for files that are legitimate locations
// for the forbidden literals (test fixtures, generated artifacts).
func sstGuardSkipFile(path string) bool {
	// Test files have legitimate need to use deterministic test values.
	if strings.HasSuffix(path, "_test.go") {
		return true
	}
	if strings.HasSuffix(path, "_test.py") {
		return true
	}
	// Test directories.
	if strings.Contains(path, string(filepath.Separator)+"tests"+string(filepath.Separator)) {
		return true
	}
	// Vendored / generated trees.
	if strings.Contains(path, string(filepath.Separator)+".venv"+string(filepath.Separator)) {
		return true
	}
	if strings.Contains(path, string(filepath.Separator)+"node_modules"+string(filepath.Separator)) {
		return true
	}
	// Dockerfile in ml/ — ml/Dockerfile is build context, no ollama runtime
	// literals expected. The guard scans it; this branch is just defensive.
	return false
}

// sstRepoRoot climbs from this test's directory to the repo root by
// looking for config/smackerel.yaml. Independent of `go test` CWD.
func sstRepoRoot(t *testing.T) string {
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

// scanForOllamaLiterals walks the given root, scanning files whose
// extension is in sstGuardScannableExt and which are NOT skipped by
// sstGuardSkipFile. For each forbidden literal found, it appends a finding
// of the form "<rel-path>:<line-no>: <line-content>". The returned slice
// is empty if and only if no production file contains a forbidden literal.
func scanForOllamaLiterals(rootDir string, scanPath string) ([]string, error) {
	findings := make([]string, 0)
	walkRoot := filepath.Join(rootDir, scanPath)
	info, err := os.Stat(walkRoot)
	if err != nil {
		// Missing scan root is not a failure — repo layout may evolve.
		if os.IsNotExist(err) {
			return findings, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		// Single-file scan.
		more, err := scanSingleFile(rootDir, walkRoot)
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
		if _, ok := sstGuardScannableExt[ext]; !ok {
			return nil
		}
		if sstGuardSkipFile(path) {
			return nil
		}
		more, err := scanSingleFile(rootDir, path)
		if err != nil {
			return err
		}
		findings = append(findings, more...)
		return nil
	})
	return findings, err
}

func scanSingleFile(rootDir, absPath string) ([]string, error) {
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
		for _, lit := range forbiddenOllamaLiterals {
			if strings.Contains(line, lit) {
				findings = append(findings,
					rel+":"+itoa(lineIdx+1)+": "+strings.TrimSpace(line))
			}
		}
	}
	return findings, nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

// TestSST_NoHardcodedOllamaValues is the primary SST guard. It walks the
// production source roots and asserts no file contains any forbidden Ollama
// literal. Test files are allowlisted (see sstGuardSkipFile).
func TestSST_NoHardcodedOllamaValues(t *testing.T) {
	root := sstRepoRoot(t)
	allFindings := make([]string, 0)
	for _, scanRoot := range sstGuardScanRoots {
		f, err := scanForOllamaLiterals(root, scanRoot)
		if err != nil {
			t.Fatalf("scan %s failed: %v", scanRoot, err)
		}
		allFindings = append(allFindings, f...)
	}
	for _, scanFile := range sstGuardScanFiles {
		f, err := scanForOllamaLiterals(root, scanFile)
		if err != nil {
			t.Fatalf("scan %s failed: %v", scanFile, err)
		}
		allFindings = append(allFindings, f...)
	}
	if len(allFindings) > 0 {
		t.Fatalf("SST violation: production source contains forbidden Ollama literals — every Ollama runtime value MUST come from infrastructure.ollama.* SST keys (config/smackerel.yaml). Findings:\n  %s",
			strings.Join(allFindings, "\n  "))
	}
	t.Logf("SST guard OK: no production source file contains %v outside config/", forbiddenOllamaLiterals)
}

// TestSST_NoHardcodedOllamaValues_Adversarial proves the guard catches a
// regression. It writes a fixture file under a temp scan root that contains
// each forbidden literal and asserts the scanner reports it. If this test
// fails, the SST guard above is tautological — it would NOT catch a real
// regression that re-introduces a hardcoded Ollama value.
func TestSST_NoHardcodedOllamaValues_Adversarial(t *testing.T) {
	tmp := t.TempDir()
	scanDir := filepath.Join(tmp, "fakeprod")
	if err := os.MkdirAll(scanDir, 0o755); err != nil {
		t.Fatalf("mkdir fakeprod: %v", err)
	}
	fixturePath := filepath.Join(scanDir, "naughty.go")
	const fixtureContent = `package fakeprod

const (
	OllamaURL   = "http://localhost:11434"
	OllamaImage = "ollama/ollama:0.6"
	OllamaModel = "qwen2.5:0.5b-instruct"
)
`
	if err := os.WriteFile(fixturePath, []byte(fixtureContent), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	findings, err := scanForOllamaLiterals(tmp, "fakeprod")
	if err != nil {
		t.Fatalf("scan tmp: %v", err)
	}
	// Each forbidden literal must surface at least once.
	mustHave := map[string]bool{
		"11434":          false,
		"qwen2.5":        false,
		"ollama/ollama:": false,
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
			t.Errorf("adversarial guard tautological: literal %q in fixture was NOT reported by scanForOllamaLiterals (findings: %v)",
				lit, findings)
		}
	}
	t.Logf("adversarial OK: scanner reports %d findings against the 3-literal fixture; %v", len(findings), findings)
}

// TestSST_NoHardcodedOllamaValues_AllowlistAdversarial proves the
// allowlist works as intended. A fixture file named *_test.go MUST NOT be
// flagged. If the allowlist were absent, this test's setup would itself
// trigger the production guard above when run from inside internal/config/.
func TestSST_NoHardcodedOllamaValues_AllowlistAdversarial(t *testing.T) {
	tmp := t.TempDir()
	scanDir := filepath.Join(tmp, "fakeprod")
	if err := os.MkdirAll(scanDir, 0o755); err != nil {
		t.Fatalf("mkdir fakeprod: %v", err)
	}
	// File name ending in _test.go MUST be skipped by sstGuardSkipFile.
	allowedPath := filepath.Join(scanDir, "fixture_test.go")
	const allowedContent = `package fakeprod
const _ = "11434"
`
	if err := os.WriteFile(allowedPath, []byte(allowedContent), 0o600); err != nil {
		t.Fatalf("write allowed fixture: %v", err)
	}
	findings, err := scanForOllamaLiterals(tmp, "fakeprod")
	if err != nil {
		t.Fatalf("scan tmp: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("allowlist failure: *_test.go fixture should be skipped but scanner reported: %v",
			findings)
	}
	t.Logf("allowlist OK: *_test.go fixture with literal '11434' is correctly skipped")
}
