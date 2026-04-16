package pipeline

import (
	"encoding/json"
	"testing"
)

func TestValidateSynthesisExtractRequest_Valid(t *testing.T) {
	req := &SynthesisExtractRequest{
		ArtifactID:            "01JART001",
		ContentType:           "article",
		Title:                 "Test Article",
		PromptContractVersion: "ingest-synthesis-v1",
	}
	if err := ValidateSynthesisExtractRequest(req); err != nil {
		t.Fatalf("expected valid request, got error: %v", err)
	}
}

func TestValidateSynthesisExtractRequest_MissingArtifactID(t *testing.T) {
	req := &SynthesisExtractRequest{
		ContentType:           "article",
		Title:                 "Test",
		PromptContractVersion: "v1",
	}
	if err := ValidateSynthesisExtractRequest(req); err == nil {
		t.Fatal("expected error for missing artifact_id")
	}
}

func TestValidateSynthesisExtractRequest_MissingContentType(t *testing.T) {
	req := &SynthesisExtractRequest{
		ArtifactID:            "01JART001",
		Title:                 "Test",
		PromptContractVersion: "v1",
	}
	if err := ValidateSynthesisExtractRequest(req); err == nil {
		t.Fatal("expected error for missing content_type")
	}
}

func TestValidateSynthesisExtractRequest_MissingContent(t *testing.T) {
	req := &SynthesisExtractRequest{
		ArtifactID:            "01JART001",
		ContentType:           "article",
		PromptContractVersion: "v1",
	}
	if err := ValidateSynthesisExtractRequest(req); err == nil {
		t.Fatal("expected error for missing content (no title, summary, or content_raw)")
	}
}

func TestValidateSynthesisExtractRequest_MissingContractVersion(t *testing.T) {
	req := &SynthesisExtractRequest{
		ArtifactID:  "01JART001",
		ContentType: "article",
		Title:       "Test",
	}
	if err := ValidateSynthesisExtractRequest(req); err == nil {
		t.Fatal("expected error for missing prompt_contract_version")
	}
}

func TestValidateSynthesisExtractResponse_Valid(t *testing.T) {
	resp := &SynthesisExtractResponse{ArtifactID: "01JART001", Success: true}
	if err := ValidateSynthesisExtractResponse(resp); err != nil {
		t.Fatalf("expected valid response, got error: %v", err)
	}
}

func TestValidateSynthesisExtractResponse_MissingArtifactID(t *testing.T) {
	resp := &SynthesisExtractResponse{Success: true}
	if err := ValidateSynthesisExtractResponse(resp); err == nil {
		t.Fatal("expected error for missing artifact_id")
	}
}

func TestSynthesisExtractResponse_JSONRoundTrip(t *testing.T) {
	resp := SynthesisExtractResponse{
		ArtifactID:            "01JART001",
		Success:               true,
		PromptContractVersion: "ingest-synthesis-v1",
		ProcessingTimeMs:      1500,
		ModelUsed:             "ollama/llama3.2",
		TokensUsed:            800,
		Result: &SynthesisResult{
			Concepts: []ExtractedConcept{
				{
					Name:        "Leadership",
					Description: "Organizational management approach",
					Claims: []ExtractedClaim{
						{Text: "Servant leadership increases retention", Confidence: 0.85},
					},
					IsNew: true,
				},
			},
			Entities: []ExtractedEntity{
				{Name: "Sarah Chen", Type: "person", Context: "Leadership consultant"},
			},
			Relationships: []ExtractedRelationship{
				{Source: "Leadership", Target: "Remote Work", Type: "CONCEPT_RELATES_TO", Description: "Both address team management"},
			},
			Contradictions: []ExtractedContradiction{
				{Concept: "Outreach", ExistingClaim: "2% rate", NewClaim: "15% rate", ExistingArtifactID: "01JARTA"},
			},
		},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded SynthesisExtractResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.ArtifactID != resp.ArtifactID {
		t.Errorf("artifact_id = %q, want %q", decoded.ArtifactID, resp.ArtifactID)
	}
	if decoded.Result == nil {
		t.Fatal("result is nil after round-trip")
	}
	if len(decoded.Result.Concepts) != 1 {
		t.Fatalf("concepts count = %d, want 1", len(decoded.Result.Concepts))
	}
	if decoded.Result.Concepts[0].Name != "Leadership" {
		t.Errorf("concept name = %q, want %q", decoded.Result.Concepts[0].Name, "Leadership")
	}
	if len(decoded.Result.Concepts[0].Claims) != 1 {
		t.Fatalf("claims count = %d, want 1", len(decoded.Result.Concepts[0].Claims))
	}
	if decoded.Result.Concepts[0].Claims[0].Confidence != 0.85 {
		t.Errorf("claim confidence = %f, want 0.85", decoded.Result.Concepts[0].Claims[0].Confidence)
	}
}

func TestCrossSourceRequest_JSONRoundTrip(t *testing.T) {
	req := CrossSourceRequest{
		ConceptID:             "01JCPT001",
		ConceptTitle:          "Italian Restaurants",
		PromptContractVersion: "cross-source-connection-v1",
		Artifacts: []CrossSourceArtifactSummary{
			{ID: "01JART1", Title: "Email from Sarah", SourceType: "email", Summary: "Restaurant rec"},
			{ID: "01JART2", Title: "Google Maps visit", SourceType: "google-maps-timeline", Summary: "Visited Trattoria"},
		},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded CrossSourceRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.ConceptID != req.ConceptID {
		t.Errorf("concept_id = %q, want %q", decoded.ConceptID, req.ConceptID)
	}
	if len(decoded.Artifacts) != 2 {
		t.Fatalf("artifacts count = %d, want 2", len(decoded.Artifacts))
	}
}

func TestCrossSourceResponse_JSONRoundTrip(t *testing.T) {
	resp := CrossSourceResponse{
		ConceptID:             "01JCPT001",
		HasGenuineConnection:  true,
		InsightText:           "Sarah recommended the restaurant and the user visited 5 days later.",
		Confidence:            0.92,
		ArtifactIDs:           []string{"01JART1", "01JART2"},
		PromptContractVersion: "cross-source-connection-v1",
		ProcessingTimeMs:      2100,
		ModelUsed:             "ollama/llama3.2",
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded CrossSourceResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !decoded.HasGenuineConnection {
		t.Error("has_genuine_connection should be true")
	}
	if decoded.Confidence != 0.92 {
		t.Errorf("confidence = %f, want 0.92", decoded.Confidence)
	}
}
