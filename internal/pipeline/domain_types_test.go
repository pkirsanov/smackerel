package pipeline

import (
	"encoding/json"
	"testing"
)

func TestValidateDomainExtractRequest_RequiresArtifactID(t *testing.T) {
	r := &DomainExtractRequest{ContractVersion: "v1", Title: "test"}
	if err := ValidateDomainExtractRequest(r); err == nil {
		t.Fatal("expected error for empty artifact_id")
	}
}

func TestValidateDomainExtractRequest_RequiresContractVersion(t *testing.T) {
	r := &DomainExtractRequest{ArtifactID: "a1", Title: "test"}
	if err := ValidateDomainExtractRequest(r); err == nil {
		t.Fatal("expected error for empty contract_version")
	}
}

func TestValidateDomainExtractRequest_RequiresContent(t *testing.T) {
	r := &DomainExtractRequest{ArtifactID: "a1", ContractVersion: "v1"}
	if err := ValidateDomainExtractRequest(r); err == nil {
		t.Fatal("expected error when no content fields set")
	}
}

func TestValidateDomainExtractRequest_AcceptsValidInput(t *testing.T) {
	cases := []struct {
		name string
		req  DomainExtractRequest
	}{
		{"with title", DomainExtractRequest{ArtifactID: "a1", ContractVersion: "v1", Title: "test"}},
		{"with summary", DomainExtractRequest{ArtifactID: "a1", ContractVersion: "v1", Summary: "test"}},
		{"with content_raw", DomainExtractRequest{ArtifactID: "a1", ContractVersion: "v1", ContentRaw: "test"}},
		{"with all", DomainExtractRequest{ArtifactID: "a1", ContractVersion: "v1", Title: "t", Summary: "s", ContentRaw: "c"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateDomainExtractRequest(&tc.req); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidateDomainExtractResponse_RequiresArtifactID(t *testing.T) {
	r := &DomainExtractResponse{Success: false}
	if err := ValidateDomainExtractResponse(r); err == nil {
		t.Fatal("expected error for empty artifact_id")
	}
}

func TestValidateDomainExtractResponse_SuccessRequiresDomainData(t *testing.T) {
	r := &DomainExtractResponse{ArtifactID: "a1", Success: true}
	if err := ValidateDomainExtractResponse(r); err == nil {
		t.Fatal("expected error for success=true without domain_data")
	}
}

func TestValidateDomainExtractResponse_SuccessWithDomainData(t *testing.T) {
	r := &DomainExtractResponse{
		ArtifactID: "a1",
		Success:    true,
		DomainData: json.RawMessage(`{"domain":"recipe"}`),
	}
	if err := ValidateDomainExtractResponse(r); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateDomainExtractResponse_FailureAllowsEmptyDomainData(t *testing.T) {
	r := &DomainExtractResponse{
		ArtifactID: "a1",
		Success:    false,
		Error:      "LLM timeout",
	}
	if err := ValidateDomainExtractResponse(r); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDomainExtractRequest_JSONRoundTrip(t *testing.T) {
	req := DomainExtractRequest{
		ArtifactID:      "art-001",
		ContentType:     "recipe",
		Title:           "Pasta Carbonara",
		Summary:         "Classic Italian pasta dish",
		ContentRaw:      "Ingredients: eggs, guanciale, pecorino...",
		SourceURL:       "https://example.com/carbonara",
		ContractVersion: "recipe-extraction-v1",
		RetryCount:      0,
		TraceID:         "trace-123",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded DomainExtractRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.ArtifactID != req.ArtifactID {
		t.Errorf("artifact_id mismatch: %s vs %s", decoded.ArtifactID, req.ArtifactID)
	}
	if decoded.ContractVersion != req.ContractVersion {
		t.Errorf("contract_version mismatch: %s vs %s", decoded.ContractVersion, req.ContractVersion)
	}
	if decoded.SourceURL != req.SourceURL {
		t.Errorf("source_url mismatch: %s vs %s", decoded.SourceURL, req.SourceURL)
	}
}

func TestDomainExtractResponse_JSONRoundTrip(t *testing.T) {
	resp := DomainExtractResponse{
		ArtifactID:       "art-001",
		Success:          true,
		DomainData:       json.RawMessage(`{"domain":"recipe","ingredients":[{"name":"eggs","quantity":"4","unit":""}]}`),
		ContractVersion:  "recipe-extraction-v1",
		ProcessingTimeMs: 2500,
		ModelUsed:        "claude-sonnet-4-20250514",
		TokensUsed:       1234,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded DomainExtractResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.ArtifactID != resp.ArtifactID {
		t.Errorf("artifact_id mismatch")
	}
	if !decoded.Success {
		t.Error("expected success=true")
	}
	if decoded.ProcessingTimeMs != 2500 {
		t.Errorf("processing_time_ms mismatch: %d", decoded.ProcessingTimeMs)
	}
	if string(decoded.DomainData) != string(resp.DomainData) {
		t.Errorf("domain_data mismatch")
	}
}
