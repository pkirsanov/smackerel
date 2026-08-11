// Spec 061 SCOPE-10 — harness behaviour tests.
//
// These tests prove the harness primitives (Classify, Run,
// FormatReport) are correct on small fixture corpora. They do NOT
// gate on the SST acceptance thresholds — that is the role of
// acceptance_test.go (build tag: integration).
//
// Determinism: Classify is a pure function of its input string; Run
// is a pure function of the corpus; FormatReport is a pure function
// of HarnessResult. Tests assert behaviour against known inputs.

package assistanteval

import (
	"strconv"
	"strings"
	"testing"
)

func TestClassify_WeatherSignal(t *testing.T) {
	cases := []string{
		"What's the weather like today?",
		"Will it rain tomorrow?",
		"Forecast for Berlin this weekend?",
		"Snow forecast for Aspen this weekend?",
	}
	for _, tc := range cases {
		t.Run(tc, func(t *testing.T) {
			got := Classify(tc)
			if got.Intent != LabelWeather {
				t.Errorf("Classify(%q).Intent = %q, want %q", tc, got.Intent, LabelWeather)
			}
			if got.CaptureFallback {
				t.Errorf("Classify(%q).CaptureFallback = true, want false", tc)
			}
		})
	}
}

func TestClassify_NotificationSignal(t *testing.T) {
	cases := []string{
		"Remind me to call the plumber tomorrow.",
		"Set a reminder for the standup on Monday morning.",
		"Notify me in 2 hours.",
		"Ping me at 7am.",
	}
	for _, tc := range cases {
		t.Run(tc, func(t *testing.T) {
			got := Classify(tc)
			if got.Intent != LabelNotifications {
				t.Errorf("Classify(%q).Intent = %q, want %q", tc, got.Intent, LabelNotifications)
			}
		})
	}
}

func TestClassify_RetrievalSignal(t *testing.T) {
	cases := []string{
		"What did I save about Tailscale?",
		"Show me my notes on Postgres.",
		"Find that article I bookmarked.",
		"Search my notes for embeddings.",
	}
	for _, tc := range cases {
		t.Run(tc, func(t *testing.T) {
			got := Classify(tc)
			if got.Intent != LabelRetrieval {
				t.Errorf("Classify(%q).Intent = %q, want %q", tc, got.Intent, LabelRetrieval)
			}
		})
	}
}

func TestClassify_CaptureSignal(t *testing.T) {
	cases := []string{
		"Idea: ship the metrics module first.",
		"Note: cosign needs an OIDC token.",
		"Today learned: pgvector supports HNSW.",
		"The bakery on 18th opens at 6am.",
	}
	for _, tc := range cases {
		t.Run(tc, func(t *testing.T) {
			got := Classify(tc)
			if got.Intent != LabelCapture {
				t.Errorf("Classify(%q).Intent = %q, want %q", tc, got.Intent, LabelCapture)
			}
			if !got.CaptureFallback {
				t.Errorf("Classify(%q).CaptureFallback = false, want true", tc)
			}
		})
	}
}

func TestClassify_AmbiguousFallback(t *testing.T) {
	// Fragments that don't trigger any rule fall to ambiguous-borderline
	// and route to capture (default-to-capture per design §3.2).
	cases := []string{
		"Yes.",
		"Tomorrow.",
		"Pull it up.",
	}
	for _, tc := range cases {
		t.Run(tc, func(t *testing.T) {
			got := Classify(tc)
			if got.Intent != LabelAmbiguous {
				t.Errorf("Classify(%q).Intent = %q, want %q", tc, got.Intent, LabelAmbiguous)
			}
			if !got.CaptureFallback {
				t.Errorf("Classify(%q).CaptureFallback = false, want true (default-to-capture)", tc)
			}
		})
	}
}

func TestRun_Determinism(t *testing.T) {
	c := &Corpus{Rows: []CorpusRow{
		{ID: "a", Text: "weather in Tokyo today?", GroundTruthIntent: LabelWeather, GroundTruthCaptureExpected: false},
		{ID: "b", Text: "remind me to email.", GroundTruthIntent: LabelNotifications, GroundTruthCaptureExpected: false},
		{ID: "c", Text: "Idea: ship it.", GroundTruthIntent: LabelCapture, GroundTruthCaptureExpected: true},
	}}
	r1 := Run(c)
	r2 := Run(c)
	if r1.RoutingAccuracy != r2.RoutingAccuracy {
		t.Errorf("non-deterministic: %.4f vs %.4f", r1.RoutingAccuracy, r2.RoutingAccuracy)
	}
}

func TestRun_AdversarialFailureSurfaces(t *testing.T) {
	// Adversarial — ground truth deliberately mismatches the classifier
	// for every row. Run MUST report 0% accuracy and the failures slice
	// MUST be populated. Proves the harness CAN fail when classifier
	// and corpus disagree (anti-tautology guard).
	c := &Corpus{Rows: []CorpusRow{
		{ID: "x1", Text: "weather in Tokyo?", GroundTruthIntent: LabelCapture, GroundTruthCaptureExpected: false},
		{ID: "x2", Text: "Idea: ship it.", GroundTruthIntent: LabelWeather, GroundTruthCaptureExpected: false},
	}}
	r := Run(c)
	if r.IntentCorrect != 0 {
		t.Errorf("expected 0 intent_correct on adversarial corpus, got %d", r.IntentCorrect)
	}
	if r.RoutingAccuracy != 0.0 {
		t.Errorf("expected 0.0 routing accuracy, got %.4f", r.RoutingAccuracy)
	}
	if len(r.Failures) != 2 {
		t.Errorf("expected 2 failures, got %d", len(r.Failures))
	}
}

func TestRun_AgainstShippedCorpus(t *testing.T) {
	// Sanity — running the harness against the real corpus produces a
	// non-degenerate result. The acceptance threshold check lives in
	// acceptance_test.go (build tag: integration) so a CI run without
	// the tag still validates that the harness CAN run end-to-end.
	c, err := LoadCorpus(corpusPath(t))
	if err != nil {
		t.Fatalf("LoadCorpus: %v", err)
	}
	r := Run(c)
	if r.Total < MinCorpusSize {
		t.Errorf("harness ran against %d rows, expected >= %d", r.Total, MinCorpusSize)
	}
	if r.RoutingAccuracy < 0 || r.RoutingAccuracy > 1 {
		t.Errorf("routing accuracy %.4f out of range [0,1]", r.RoutingAccuracy)
	}
	if r.CaptureFallbackRate < 0 || r.CaptureFallbackRate > 1 {
		t.Errorf("capture-fallback rate %.4f out of range [0,1]", r.CaptureFallbackRate)
	}
	// Log report unconditionally so CI captures the metric trace even
	// when this test passes. Useful for spec 061 SCOPE-10 evidence
	// blocks.
	t.Logf("\n%s", FormatReport(r))
}

func TestFormatReport_IncludesAllLabels(t *testing.T) {
	c := &Corpus{Rows: []CorpusRow{
		{ID: "a", Text: "weather today?", GroundTruthIntent: LabelWeather},
		{ID: "b", Text: "remind me", GroundTruthIntent: LabelNotifications},
		{ID: "c", Text: "Idea: x.", GroundTruthIntent: LabelCapture, GroundTruthCaptureExpected: true},
	}}
	r := Run(c)
	rep := FormatReport(r)
	for _, l := range AllLabels {
		if !strings.Contains(rep, l) {
			t.Errorf("report missing label %q", l)
		}
	}
}

// BUG-061-011 — the integration lane asserts that the gate reported a
// non-zero executed-assertion count. These three tests are untagged so the
// quantity the lane asserts on is proven in the default unit lane, not only
// by the lane whose wiring is in question.

func TestExecutedAssertions_CountsRoutingPlusCaptureRows(t *testing.T) {
	// 4 rows, 2 of them capture-expected → one routing assertion per row
	// plus one capture assertion per capture-expected row = 6.
	c := &Corpus{Rows: []CorpusRow{
		{ID: "a", Text: "weather in Tokyo today?", GroundTruthIntent: LabelWeather, GroundTruthCaptureExpected: false},
		{ID: "b", Text: "remind me to email tomorrow.", GroundTruthIntent: LabelNotifications, GroundTruthCaptureExpected: false},
		{ID: "c", Text: "Idea: ship it.", GroundTruthIntent: LabelCapture, GroundTruthCaptureExpected: true},
		{ID: "d", Text: "Note: cosign needs an OIDC token.", GroundTruthIntent: LabelCapture, GroundTruthCaptureExpected: true},
	}}
	r := Run(c)

	if r.Total != 4 {
		t.Fatalf("fixture precondition: Total = %d, want 4", r.Total)
	}
	if r.CaptureExpected != 2 {
		t.Fatalf("fixture precondition: CaptureExpected = %d, want 2", r.CaptureExpected)
	}
	if got := ExecutedAssertions(r); got != 6 {
		t.Errorf("ExecutedAssertions = %d, want 6 (Total %d + CaptureExpected %d)", got, r.Total, r.CaptureExpected)
	}
}

func TestExecutedAssertions_ZeroOnEmptyCorpus(t *testing.T) {
	// Anti-tautology proof for the lane's `>= 1` check. If the count were
	// positive by construction, that check would be decorative: a gate that
	// evaluated nothing would still satisfy it. An empty corpus MUST yield
	// exactly 0, so the lane's comparison is a real discrimination.
	r := Run(&Corpus{Rows: nil})

	if got := ExecutedAssertions(r); got != 0 {
		t.Errorf("ExecutedAssertions on an empty corpus = %d, want exactly 0; the integration lane's non-zero check would be vacuous", got)
	}
	if !strings.Contains(FormatGateMarker(r), "executed_assertions=0") {
		t.Errorf("marker for an empty corpus must report executed_assertions=0, got %q", FormatGateMarker(r))
	}
}

func TestFormatGateMarker_SingleLineParseableWithPrefix(t *testing.T) {
	c := &Corpus{Rows: []CorpusRow{
		{ID: "a", Text: "weather in Tokyo today?", GroundTruthIntent: LabelWeather, GroundTruthCaptureExpected: false},
		{ID: "b", Text: "Idea: ship it.", GroundTruthIntent: LabelCapture, GroundTruthCaptureExpected: true},
	}}
	r := Run(c)
	marker := FormatGateMarker(r)

	if strings.Contains(marker, "\n") {
		t.Fatalf("marker must be exactly one line so the lane can anchor its grep to ^, got %q", marker)
	}
	if !strings.HasPrefix(marker, GateMarkerPrefix+" ") {
		t.Fatalf("marker must start with %q at column zero, got %q", GateMarkerPrefix+" ", marker)
	}

	// The count must round-trip through the same textual form the lane
	// parses: everything after "executed_assertions=" up to the next space.
	const key = "executed_assertions="
	idx := strings.Index(marker, key)
	if idx < 0 {
		t.Fatalf("marker missing %q, got %q", key, marker)
	}
	field := marker[idx+len(key):]
	if space := strings.Index(field, " "); space >= 0 {
		field = field[:space]
	}
	parsed, err := strconv.Atoi(field)
	if err != nil {
		t.Fatalf("executed_assertions field %q is not an integer: %v (marker %q)", field, err, marker)
	}
	if parsed != ExecutedAssertions(r) {
		t.Errorf("parsed executed_assertions = %d, want %d (marker %q)", parsed, ExecutedAssertions(r), marker)
	}
	if parsed < 1 {
		t.Errorf("fixture precondition: a non-empty corpus must yield a positive count, got %d", parsed)
	}
}
