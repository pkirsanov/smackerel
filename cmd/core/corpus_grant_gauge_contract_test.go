package main

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// Spec 108 SCOPE-03 — the resolved corpus-grant enforcement stage MUST be
// published to the gauge, not merely logged.
//
// This guards a defect that actually shipped: `resolveCorpusGrantEnforcement`
// returned ENFORCE and main.go logged `"stage":"ENFORCE"`, but
// `metrics.SetCorpusGrantEnforcementMode` was never called ANYWHERE in
// production code — its only occurrence was its own definition. So
// `smackerel_auth_corpus_grant_enforcement_mode` sat at 0 forever while the
// process was genuinely enforcing.
//
// That combination is worse than a missing metric. An operator confirming a
// rollback out of ENFORCE (SCN-108-C04) reads the CURRENT stage from /metrics
// rather than scrolling for a boot log line, and a gauge that is permanently 0
// answers "already rolled back" no matter what the process is doing.
//
// A source-level assertion is deliberate. The behavioural proof lives in the
// corpus-enforce e2e lane, which asserts the gauge reads 1 on a stack booted in
// ENFORCE — but that lane needs a dedicated stack, so it cannot run on every
// `test unit`. This test costs nothing and fails the moment the publish call is
// deleted, which is the regression that already happened once.
func TestCorpusGrantEnforcementStageIsPublishedToTheGauge(t *testing.T) {
	source := readMainSource(t)

	if !strings.Contains(source, "metrics.SetCorpusGrantEnforcementMode(corpusGrantEnforce)") {
		t.Fatal("cmd/core/main.go does not publish the resolved corpus-grant stage to the gauge. " +
			"Logging the stage is not enough: smackerel_auth_corpus_grant_enforcement_mode is how an operator " +
			"confirms the live stage and verifies a rollback, and an unpublished gauge reports OBSERVE forever.")
	}

	// The publish must be fed by the SAME resolved variable the log line uses.
	// A publish wired to a re-read of the environment, or to a literal, could
	// drift from the value the router actually gated on — which is precisely
	// the gauge-vs-reality disagreement this test exists to prevent.
	resolved := regexp.MustCompile(`corpusGrantEnforce,\s*err\s*:=\s*resolveCorpusGrantEnforcement\(`)
	if !resolved.MatchString(source) {
		t.Error("the resolved stage is no longer bound to `corpusGrantEnforce` via resolveCorpusGrantEnforcement; " +
			"the gauge assertion above can no longer prove the published value is the one the gate uses")
	}

	logIdx := strings.Index(source, `"corpus grant enforcement stage resolved"`)
	gaugeIdx := strings.Index(source, "metrics.SetCorpusGrantEnforcementMode(")
	if logIdx >= 0 && gaugeIdx >= 0 && gaugeIdx < logIdx {
		t.Error("the gauge is published before the stage is resolved and logged; " +
			"publish must follow resolution so the two cannot report different stages")
	}
}

// TestCorpusGrantGaugeGuardRejectsAnUnpublishedStage is the adversarial control.
//
// Without it the guard above could be satisfied by any file that merely
// mentions the call, and a reviewer would have no evidence the check can fail.
// This feeds the guard's own predicate a source that resolves and logs the
// stage but never publishes it — the exact shape of the shipped defect — and
// requires the predicate to reject it.
func TestCorpusGrantGaugeGuardRejectsAnUnpublishedStage(t *testing.T) {
	defective := `
	corpusGrantEnforce, err := resolveCorpusGrantEnforcement(corpusGrantEnforcementEnv())
	if err != nil {
		return err
	}
	slog.Info("corpus grant enforcement stage resolved", "enforce", corpusGrantEnforce)
`
	if strings.Contains(defective, "metrics.SetCorpusGrantEnforcementMode(corpusGrantEnforce)") {
		t.Fatal("the adversarial fixture accidentally contains the publish call, so it cannot demonstrate the guard fails")
	}

	// The real guard's predicate, applied to the defective source, must reject.
	if publishesStageGauge(defective) {
		t.Error("the guard predicate accepted a source that resolves and logs the stage but never publishes the gauge; " +
			"it would not have caught the defect that shipped")
	}
	if !publishesStageGauge(readMainSource(t)) {
		t.Error("the guard predicate rejected the real main.go, so it is not measuring what the guard claims")
	}
}

func publishesStageGauge(source string) bool {
	return strings.Contains(source, "metrics.SetCorpusGrantEnforcementMode(corpusGrantEnforce)")
}

func readMainSource(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read cmd/core/main.go: %v", err)
	}
	return string(raw)
}
