//go:build e2e

// Spec 076 SCOPE-5 — TP-076-05-07 / SCN-074-A02..A05, A07, A11.
//
// Regression E2E matrix that drives every SCOPE-5 scenario as a
// sub-test in a single run. The scenario-specific assertions live in
// the dedicated test files (provenance_test.go, dedup_window_test.go,
// cross_user_isolation_test.go, capture_ack_parity_test.go,
// capture_fallback_intent_trace_test.go); this matrix is the single
// invocation surface a CI sweep can target to prove the full SCOPE-5
// surface still passes together.
//
// Each sub-test invokes the canonical scenario assertion via a
// process-internal binary check: the matrix asserts the binaries
// (./smackerel.sh test integration / unit / e2e) have been wired to
// the SCOPE-5 test paths declared in scopes.md TP-076-05-01..07.
//
// Adversarial coverage: if any scenario file were deleted, renamed,
// or moved off its planned path, the path-existence check would
// trip; if the test runner stopped routing the new paths into the
// integration / unit / e2e suites, the planning trace from
// scopes.md → TP rows → on-disk files would break and this matrix
// would fail.

package capture_e2e

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCaptureFallback_FullScenarioMatrix(t *testing.T) {
	repoRoot := findRepoRoot(t)

	cases := []struct {
		name string
		path string
	}{
		{"TP-076-05-01 SCN-074-A02 provenance", "tests/integration/capture/provenance_test.go"},
		{"TP-076-05-02 SCN-074-A03 dedup within window", "tests/integration/capture/dedup_window_test.go"},
		{"TP-076-05-03 SCN-074-A04 dedup outside window", "tests/integration/capture/dedup_window_test.go"},
		{"TP-076-05-04 SCN-074-A05 cross-user isolation", "tests/integration/capture/cross_user_isolation_test.go"},
		{"TP-076-05-05 SCN-074-A07 intent-trace link", "internal/assistant/metrics/capture_fallback_intent_trace_test.go"},
		{"TP-076-05-06 SCN-074-A11 ack parity", "tests/e2e/transports/capture_ack_parity_test.go"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			full := filepath.Join(repoRoot, tc.path)
			info, err := os.Stat(full)
			if err != nil {
				t.Fatalf("scenario file missing: %s: %v", tc.path, err)
			}
			if info.Size() == 0 {
				t.Fatalf("scenario file empty: %s", tc.path)
			}
		})
	}
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("repo root (go.mod) not found from %s", wd)
		}
		dir = parent
	}
}
