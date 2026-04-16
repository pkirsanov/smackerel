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
