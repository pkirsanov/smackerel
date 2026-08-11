//go:build integration

// Spec 061 SCOPE-10 — acceptance gate.
//
// Runs the harness end-to-end and asserts the spec 061 §17 contract:
//   - routing accuracy >= ASSISTANT_EVAL_ROUTING_ACCURACY_MIN
//   - capture-fallback rate >= ASSISTANT_EVAL_CAPTURE_FALLBACK_MIN
//
// Honest failure: when the classifier or the corpus drifts and the
// gate is missed, this test fails LOUDLY with the full report in the
// failure message. Operators tune the corpus, tune the classifier,
// OR wire in a real LLM router until the gate passes again.
//
// Build tag `integration` keeps it out of the default `go test ./...`
// pass so corpus development doesn't fight the gate. The tag alone does
// NOT make the gate run anywhere: scripts/runtime/go-integration.sh
// selects packages by an explicit allow-list, not `./...`, so the gate
// runs only because `./tests/eval/...` is in that list. Both halves are
// required, and internal/deploy/eval_lane_contract_test.go asserts the
// pair — see BUG-061-011, where the tag was present, the package was
// not, and the gate executed in no lane for months without a surface
// turning red.

package assistanteval

import (
	"fmt"
	"os"
	"strconv"
	"testing"
)

func mustFloatEnv(t *testing.T, key string) float64 {
	t.Helper()
	v := os.Getenv(key)
	if v == "" {
		t.Fatalf("SST contract violation: %s is empty; should be set by config/generated/<env>.env", key)
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		t.Fatalf("SST contract violation: %s = %q is not a float: %v", key, v, err)
	}
	return f
}

func TestAcceptanceGate_RoutingAccuracyAndCaptureFallback(t *testing.T) {
	routingMin := mustFloatEnv(t, "ASSISTANT_EVAL_ROUTING_ACCURACY_MIN")
	captureMin := mustFloatEnv(t, "ASSISTANT_EVAL_CAPTURE_FALLBACK_MIN")

	c, err := LoadCorpus(corpusPath(t))
	if err != nil {
		t.Fatalf("LoadCorpus: %v", err)
	}
	if err := ValidateCorpus(c); err != nil {
		t.Fatalf("ValidateCorpus: %v", err)
	}

	r := Run(c)

	// BUG-061-011 — emit the machine-readable marker UNCONDITIONALLY and
	// BEFORE the threshold comparisons below, so the integration lane can
	// prove the gate ran whether it passed or failed. A marker printed
	// only on success cannot distinguish "failed" from "never ran", which
	// is precisely the discrimination the lane needs.
	fmt.Println(FormatGateMarker(r))

	report := FormatReport(r)

	if r.RoutingAccuracy < routingMin {
		t.Errorf("acceptance-gate FAIL: routing accuracy %.4f < required %.4f\n\n%s",
			r.RoutingAccuracy, routingMin, report)
	}
	if r.CaptureFallbackRate < captureMin {
		t.Errorf("acceptance-gate FAIL: capture-fallback rate %.4f < required %.4f\n\n%s",
			r.CaptureFallbackRate, captureMin, report)
	}

	// Always log the report when the test passes — useful for spec 061
	// SCOPE-10 evidence capture.
	if !t.Failed() {
		fmt.Println(report)
	}
}
