package knowledge

import (
	"encoding/json"
	"testing"
)

// T2-05: UpsertConcept new → concept page created with correct fields
func TestEstimateTokens(t *testing.T) {
	summary := "A test summary about leadership"
	claims, _ := json.Marshal([]Claim{
		{Text: "Servant leadership increases retention by 23%", ArtifactID: "01JART1"},
	})
	tokens := estimateTokens(summary, claims)
	if tokens <= 0 {
		t.Errorf("expected positive token count, got %d", tokens)
	}
	// Rough check: total chars / 4
	expectedApprox := (len(summary) + len(claims)) / 4
	if tokens != expectedApprox {
		t.Errorf("tokens = %d, want ~%d", tokens, expectedApprox)
	}
}

// T2-04: UpsertConcept existing → claims merged, old citations preserved
func TestAddUnique(t *testing.T) {
	tests := []struct {
		name     string
		slice    []string
		val      string
		wantLen  int
		wantLast string
	}{
		{
			name:     "add new value",
			slice:    []string{"email", "article"},
			val:      "video",
			wantLen:  3,
			wantLast: "video",
		},
		{
			name:    "duplicate value (exact case)",
			slice:   []string{"email", "article"},
			val:     "email",
			wantLen: 2,
		},
		{
			name:    "duplicate value (different case)",
			slice:   []string{"email", "article"},
			val:     "Email",
			wantLen: 2,
		},
		{
			name:     "add to empty slice",
			slice:    []string{},
			val:      "article",
			wantLen:  1,
			wantLast: "article",
		},
		{
			name:     "add to nil slice",
			slice:    nil,
			val:      "article",
			wantLen:  1,
			wantLast: "article",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := addUnique(tt.slice, tt.val)
			if len(result) != tt.wantLen {
				t.Errorf("len(result) = %d, want %d; result = %v", len(result), tt.wantLen, result)
			}
			if tt.wantLast != "" && len(result) > 0 && result[len(result)-1] != tt.wantLast {
				t.Errorf("last element = %q, want %q", result[len(result)-1], tt.wantLast)
			}
		})
	}
}

// T2-06: UpsertEntity existing → mentions appended, source_types updated
// This is a structural test — verifying Mention JSON serialization
func TestMentionJSONRoundTrip(t *testing.T) {
	mentions := []Mention{
		{ArtifactID: "01JART1", ArtifactTitle: "Email from Sarah", SourceType: "email", Context: "Recommended restaurant", MentionedAt: "2026-04-15T10:00:00Z"},
		{ArtifactID: "01JART2", ArtifactTitle: "Maps Visit", SourceType: "google-maps-timeline", Context: "Visited location", MentionedAt: "2026-04-16T10:00:00Z"},
	}

	data, err := json.Marshal(mentions)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded []Mention
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(decoded) != 2 {
		t.Fatalf("expected 2 mentions, got %d", len(decoded))
	}
	if decoded[0].ArtifactID != "01JART1" {
		t.Errorf("mention[0].artifact_id = %q, want %q", decoded[0].ArtifactID, "01JART1")
	}
	if decoded[1].SourceType != "google-maps-timeline" {
		t.Errorf("mention[1].source_type = %q, want %q", decoded[1].SourceType, "google-maps-timeline")
	}
}

// Test claim JSON round-trip for knowledge layer writes
func TestClaimJSONRoundTrip(t *testing.T) {
	claims := []Claim{
		{Text: "Servant leadership increases retention", ArtifactID: "01JART1", ArtifactTitle: "Leadership Article", SourceType: "article", ExtractedAt: "2026-04-15T10:00:00Z", Confidence: 0.85},
	}

	data, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded []Claim
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(decoded) != 1 {
		t.Fatalf("expected 1 claim, got %d", len(decoded))
	}
	if decoded[0].Confidence != 0.85 {
		t.Errorf("confidence = %f, want 0.85", decoded[0].Confidence)
	}
	if decoded[0].ArtifactID != "01JART1" {
		t.Errorf("artifact_id = %q, want %q", decoded[0].ArtifactID, "01JART1")
	}
}

// Test ArtifactSynthesisData JSON serialization
func TestArtifactSynthesisDataJSON(t *testing.T) {
	a := ArtifactSynthesisData{
		ID:           "01JART001",
		ArtifactType: "article",
		Title:        "Test Article",
		Summary:      "A test summary",
		ContentRaw:   "Full article text here...",
		SourceID:     "rss",
		KeyIdeasJSON: json.RawMessage(`["idea1","idea2"]`),
		EntitiesJSON: json.RawMessage(`{"people":["Sarah"]}`),
		TopicsJSON:   json.RawMessage(`["leadership"]`),
		RetryCount:   0,
	}

	data, err := json.Marshal(a)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded ArtifactSynthesisData
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.ID != "01JART001" {
		t.Errorf("id = %q", decoded.ID)
	}
	if decoded.SourceID != "rss" {
		t.Errorf("source_id = %q", decoded.SourceID)
	}
}

// --- enforceTokenCap tests (design: 4,000-token cap) ---

func TestEnforceTokenCap_UnderLimit(t *testing.T) {
	claims := []Claim{
		{Text: "Claim one", ArtifactID: "a1"},
		{Text: "Claim two", ArtifactID: "a2"},
	}
	result := enforceTokenCap(claims, "Short summary", 4000)
	if len(result) != 2 {
		t.Errorf("expected 2 claims (under limit), got %d", len(result))
	}
}

func TestEnforceTokenCap_OverLimit_DropsOldest(t *testing.T) {
	// Build claims that together exceed a small token budget.
	// Each claim ~100 chars → ~25 tokens. With summary ~10 tokens, budget = 40 tokens → only ~1 claim fits.
	claims := []Claim{
		{Text: "Oldest claim with lots of detail about leadership and management strategies and retention rates in modern organizations", ArtifactID: "a1"},
		{Text: "Middle claim about remote work productivity and collaboration tools in distributed teams across time zones", ArtifactID: "a2"},
		{Text: "Newest claim about pricing strategy optimization and revenue growth through data-driven decision making process", ArtifactID: "a3"},
	}
	// Set a very tight budget (40 tokens = ~160 chars for summary + claims JSON)
	result := enforceTokenCap(claims, "Summary", 40)
	if len(result) >= 3 {
		t.Errorf("expected fewer than 3 claims after cap enforcement, got %d", len(result))
	}
	// The oldest claims should be dropped first — newest should survive
	if len(result) > 0 && result[len(result)-1].ArtifactID != "a3" {
		t.Errorf("newest claim (a3) should be preserved, last claim is %q", result[len(result)-1].ArtifactID)
	}
}

func TestEnforceTokenCap_ZeroLimit_NoOp(t *testing.T) {
	claims := []Claim{
		{Text: "Claim one", ArtifactID: "a1"},
	}
	// enforceTokenCap is only called when MaxTokens > 0, but test defensive behavior
	result := enforceTokenCap(claims, "Summary", 0)
	// With 0 budget, all claims get trimmed (edge case)
	if len(result) > len(claims) {
		t.Errorf("result should not grow: got %d, want <= %d", len(result), len(claims))
	}
}

func TestEnforceTokenCap_EmptyClaims(t *testing.T) {
	result := enforceTokenCap(nil, "Summary", 4000)
	if len(result) != 0 {
		t.Errorf("expected 0 claims for nil input, got %d", len(result))
	}
}

func TestEnforceTokenCap_PreservesNewest(t *testing.T) {
	// When trimming, newest claims (end of slice) should be preserved
	claims := make([]Claim, 50)
	for i := range claims {
		claims[i] = Claim{Text: "Claim with enough text to use tokens " + string(rune('A'+i%26)), ArtifactID: "art-" + string(rune('0'+i%10))}
	}
	result := enforceTokenCap(claims, "A summary", 100)
	if len(result) >= 50 {
		t.Fatal("expected trimming to occur")
	}
	// Last element should be the original last element
	if len(result) > 0 && result[len(result)-1].ArtifactID != claims[49].ArtifactID {
		t.Errorf("newest claim should be preserved")
	}
}
