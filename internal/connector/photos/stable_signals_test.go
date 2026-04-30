package photos

import "testing"

func TestStableSignals_DoNotMakeLLMOwnedDecisions(t *testing.T) {
	signals := StableSignals{
		Filename:    "IMG_1234-LIGHTROOM-EDIT.jpg",
		MIMEType:    "image/jpeg",
		ContentHash: "sha256:fixture",
		PHash:       "abcd1234",
		EXIF: map[string]any{
			"Software":         "Adobe Lightroom Classic",
			"DateTimeOriginal": "2026:03:14 12:30:00",
		},
		Albums: []string{"Receipts", "Travel"},
	}

	if got := signals.SeedFacts(); len(got) == 0 {
		t.Fatal("SeedFacts returned no stable facts")
	}
	if classification := signals.HeuristicClassification(); classification != nil {
		t.Fatalf("stable signals produced classification %#v; want nil because LLM owns final classification", classification)
	}
	if lifecycle := signals.HeuristicLifecycleDecision(); lifecycle != nil {
		t.Fatalf("stable signals produced lifecycle decision %#v; want nil because LLM owns final lifecycle", lifecycle)
	}
	if dedupe := signals.HeuristicDuplicateBestPick(); dedupe != nil {
		t.Fatalf("stable signals produced duplicate best-pick %#v; want nil because LLM owns final dedupe", dedupe)
	}
}

func TestStableSignalsRejectLLMDecisionMissingConfidenceOrRationale(t *testing.T) {
	cases := []struct {
		name     string
		decision LLMDecision
	}{
		{name: "missing confidence", decision: LLMDecision{Kind: DecisionClassification, Rationale: "scene includes a receipt"}},
		{name: "missing rationale", decision: LLMDecision{Kind: DecisionLifecycle, Confidence: ptrFloat(0.82)}},
		{name: "blank rationale", decision: LLMDecision{Kind: DecisionSensitivity, Confidence: ptrFloat(0.91), Rationale: "  "}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			issue, err := ValidateLLMDecision(tc.decision)
			if err == nil {
				t.Fatal("expected missing confidence/rationale to fail")
			}
			if issue == nil {
				t.Fatal("expected visible classification issue")
			}
			if !issue.Visible {
				t.Fatalf("issue.Visible = false, want true: %+v", issue)
			}
			if issue.Code == "" || issue.Message == "" {
				t.Fatalf("issue should include code and message: %+v", issue)
			}
		})
	}
}

func TestStableSignalsAcceptCompleteLLMDecision(t *testing.T) {
	issue, err := ValidateLLMDecision(LLMDecision{
		Kind:       DecisionRemoval,
		Confidence: ptrFloat(0.77),
		Rationale:  "The RAW has a matching processed export and no unique annotations.",
	})
	if err != nil {
		t.Fatalf("ValidateLLMDecision: %v", err)
	}
	if issue != nil {
		t.Fatalf("issue = %+v, want nil", issue)
	}
}

func ptrFloat(v float64) *float64 { return &v }
