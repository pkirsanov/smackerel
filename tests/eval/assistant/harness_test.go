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
