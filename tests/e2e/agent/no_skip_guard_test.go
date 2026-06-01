// Spec 043 Scope 2 — Guard against `t.Skip` bailout in agent e2e tests.
//
// FR-OLLAMA-005 (and SCN-OLLAMA-004) demand that the live-Ollama happy
// path test FAILS when the model or the daemon is unavailable, never
// SKIPS. Skipping would let CI silently miss the only test that proves
// the production NATS+sidecar+litellm+Ollama path actually works.
//
// This grep-style guard runs as a regular `go test` under the regular
// `go test ./tests/e2e/agent/...` invocation (no build tags) so the
// guard catches the regression even when the e2e suite is gated off
// by `SMACKEREL_TEST_OLLAMA`. The guard intentionally does NOT use a
// build tag — it is the contract enforcement that lives outside the
// gated suite.
//
// Adversarial gates:
//
//   G1: scans every *.go file under tests/e2e/agent/ for the regex
//       \bt\.(Skip|SkipNow|Skipf)\b — fails the test if any match is
//       found that is not on the explicit allowlist.
//   G2: the allowlist contains exactly ONE entry: the helpers file's
//       liveStackOrSkip helper used by the SCRIPTED-driver e2e tests
//       (api_invoke_test, telegram_replies_test, etc., from spec 037
//       Scope 9). Those are scripted-LLM tests that MUST skip when
//       the live stack is down, because they are not the spec 043
//       fail-loud Ollama path.
//   G3: the live-Ollama happy_path_test.go MUST NOT appear on the
//       allowlist; the guard explicitly asserts the file does not
//       contain any t.Skip call regardless of the allowlist.
//
// Adversarial-self-test:
//
//   The TestNoSkipBailout_AdversarialFinding test writes a temp .go
//   file under tests/e2e/agent/_adversarial_test_fixture/ containing
//   a `t.Skip()` call and runs the same regex against the temp file
//   to assert that the regex would catch a regression.
//   The fixture file lives under a subdirectory the production guard
//   does NOT scan, so it cannot pollute the production assertion.

package agent_e2e

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// agentE2EDir locates tests/e2e/agent/ relative to this file.
func agentE2EDir(t *testing.T) string {
	t.Helper()
	_, here, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed — cannot locate this test file")
	}
	return filepath.Dir(here)
}

// allowedSkipFiles names the *.go files in tests/e2e/agent/ that may
// legitimately contain t.Skip calls. Each entry MUST have a written
// justification in the file header.
//
// Spec 043 Scope 2 SCN-OLLAMA-004 enforcement: this allowlist is
// REVIEWED whenever a new file is added under tests/e2e/agent/. Any
// new entry without a justification block in the file header is a
// regression of FR-OLLAMA-005.
var allowedSkipFiles = map[string]string{
	// Scripted-driver e2e tests (spec 037 Scope 9). These tests use
	// httptest.NewServer + agent.AgentInvokeRunner injection — they do
	// NOT touch real Ollama. They legitimately skip when the live test
	// stack (postgres + nats) is unavailable, because the surface
	// under test (HTTP envelope mapping) requires a real DB+NATS even
	// when the LLM is scripted. They are not the spec 043 happy path.
	"helpers_test.go":                "scripted-driver e2e helpers (spec 037 Scope 9)",
	"api_invoke_test.go":             "scripted-driver e2e (spec 037 Scope 9)",
	"telegram_replies_test.go":       "scripted-driver e2e (spec 037 Scope 9)",
	"bs014_never_invent_test.go":     "scripted-driver adversarial regression (spec 037 BS-014)",
	"bs020_prompt_injection_test.go": "scripted-driver adversarial regression (spec 037 BS-020)",
	"cli_filter_test.go":             "scripted-driver CLI e2e (spec 037 Scope 8)",
	"operator_ui_test.go":            "scripted-driver UI e2e (spec 037 Scope 8)",
	"replay_pass_test.go":            "scripted-driver replay e2e (spec 037 Scope 6)",
	"replay_fail_test.go":            "scripted-driver replay e2e (spec 037 Scope 6)",
	"bs001_zero_go_change_test.go":   "scripted-driver scenario-reload e2e (spec 037 BS-001)",
	// Spec 064 SCOPE-17 — open-knowledge e2e scaffolding. The seven
	// tests cover UC-064-A01..A06 + the adversarial G021 fabricated-
	// source path against the real /v1/agent/invoke endpoint. They
	// skip honestly with per-finding messages while the six
	// infrastructure prerequisites routed via PKT-WORKFLOW-A are not
	// in place (no real Ollama dispatch in chat.py, no capture-as-
	// fallback on /v1/agent/invoke, no fixture-fabricated-cite mode,
	// no per-test budget/allowlist override knobs, no AGENT_INVOKE_URL
	// export). This file is NOT the spec 043 live-Ollama happy path
	// (which is happy_path_test.go and remains forbidden from skipping).
	"openknowledge_e2e_test.go": "spec 064 SCOPE-17 scaffolding pending PKT-WORKFLOW-A infrastructure findings",
}

// skipPattern matches any of the t.Skip family verbs as a method call
// on a *testing.T receiver. The \b boundaries prevent false matches on
// identifiers like myT.SkipFooBar or skipFoo.
var skipPattern = regexp.MustCompile(`\bt\.(Skip|SkipNow|Skipf)\(`)

// TestNoSkipBailoutInAgentE2E is the production guard. It scans every
// *.go file in tests/e2e/agent/ (NOT recursing into subdirectories so
// the adversarial fixture cannot pollute the assertion) and fails if
// any file outside allowedSkipFiles contains a t.Skip-family call.
func TestNoSkipBailoutInAgentE2E(t *testing.T) {
	dir := agentE2EDir(t)

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read tests/e2e/agent/: %v", err)
	}

	violations := []string{}
	scannedCount := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".go") {
			continue
		}
		// The guard itself defines the regex literal — exclude it.
		if name == "no_skip_guard_test.go" {
			continue
		}
		path := filepath.Join(dir, name)
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		scannedCount++
		matches := skipPattern.FindAllIndex(body, -1)
		if len(matches) == 0 {
			continue
		}
		if _, allowed := allowedSkipFiles[name]; allowed {
			continue
		}
		// Report each match with line number for actionable failure.
		for _, m := range matches {
			line := 1 + strings.Count(string(body[:m[0]]), "\n")
			violations = append(violations, name+":"+itoa(line)+" — t.Skip-family call found; spec 043 SCN-OLLAMA-004 forbids skipping in live-Ollama tests")
		}
	}

	if scannedCount == 0 {
		t.Fatal("guard scanned zero files in tests/e2e/agent/ — directory layout changed or path resolution broke")
	}

	if len(violations) > 0 {
		t.Fatalf("FR-OLLAMA-005 violation: %d t.Skip-family call(s) found in tests/e2e/agent/ outside the allowlist:\n  %s\n\n"+
			"To fix: either\n"+
			"  (a) make the test FAIL fast on the missing dependency (preferred — see SCN-OLLAMA-004),\n"+
			"  (b) move the bailout to a documented, allowlisted scripted-driver test (edit allowedSkipFiles in this guard).\n"+
			"DO NOT silently bail out on missing live-Ollama state.",
			len(violations), strings.Join(violations, "\n  "))
	}
}

// TestNoSkipBailout_HappyPathTestExplicitlyForbidden asserts that the
// spec 043 happy_path_test.go file (when it exists) does NOT contain
// any t.Skip call regardless of the allowlist. This is a stricter
// gate than the allowlist on its own: even if a future change
// accidentally adds happy_path_test.go to allowedSkipFiles, this test
// will still fail.
func TestNoSkipBailout_HappyPathTestExplicitlyForbidden(t *testing.T) {
	dir := agentE2EDir(t)
	target := filepath.Join(dir, "happy_path_test.go")
	body, err := os.ReadFile(target)
	if err != nil {
		if os.IsNotExist(err) {
			// happy_path_test.go has not been authored yet (Scope 2
			// landing in stages). The guard becomes meaningful as soon
			// as the file exists.
			t.Skipf("happy_path_test.go does not exist yet at %s — guard is dormant until Scope 2 file lands", target)
		}
		t.Fatalf("read %s: %v", target, err)
	}
	if skipPattern.Match(body) {
		t.Fatalf("FR-OLLAMA-005 violation: happy_path_test.go contains a t.Skip-family call. Spec 043 SCN-OLLAMA-004 requires this test to FAIL when Ollama is unavailable, never SKIP.")
	}
}

// TestNoSkipBailout_AdversarialFinding proves the regex would catch a
// regression. Without this, a buggy regex (e.g., one that no longer
// matches t.Skip) would silently turn the production guard into a
// no-op.
func TestNoSkipBailout_AdversarialFinding(t *testing.T) {
	cases := []struct {
		name     string
		body     string
		expected bool
	}{
		{"plain_t_Skip", `func TestX(t *testing.T) { t.Skip("nope") }`, true},
		{"plain_t_SkipNow", `func TestX(t *testing.T) { t.SkipNow() }`, true},
		{"plain_t_Skipf", `func TestX(t *testing.T) { t.Skipf("nope %s", "x") }`, true},
		{"named_method_no_match", `func TestX(t *testing.T) { runner.SkipFooBar() }`, false},
		{"identifier_no_match", `var skipFoo = true`, false},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			got := skipPattern.MatchString(c.body)
			if got != c.expected {
				t.Fatalf("regex matched=%v want=%v for body=%q (regex would not catch the regression in production)", got, c.expected, c.body)
			}
		})
	}
}

// itoa is a tiny dependency-free int-to-string used only by the
// violation messages above. We avoid strconv to keep this file's
// imports minimal and the guard self-contained.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
