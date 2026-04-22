package pipeline

import (
	"encoding/json"
	"strings"
	"testing"
)

// --- C026-CHAOS-03: Domain data response size validation ---

// TestChaos_DomainExtractResponse_OversizedDomainData verifies that
// ValidateDomainExtractResponse rejects domain_data exceeding maxDomainDataBytes.
func TestChaos_DomainExtractResponse_OversizedDomainData(t *testing.T) {
	// Build a valid JSON payload that exceeds 512KB
	oversized := `{"domain":"recipe","data":"` + strings.Repeat("x", 600*1024) + `"}`
	resp := &DomainExtractResponse{
		ArtifactID:      "art-chaos-01",
		Success:         true,
		DomainData:      json.RawMessage(oversized),
		ContractVersion: "recipe-extraction-v1",
	}

	err := ValidateDomainExtractResponse(resp)
	if err == nil {
		t.Fatal("expected validation error for oversized domain_data")
	}
	if !strings.Contains(err.Error(), "too large") {
		t.Errorf("expected 'too large' in error, got: %v", err)
	}
}

// TestChaos_DomainExtractResponse_AtMaxSize verifies that domain_data exactly
// at the limit passes validation.
func TestChaos_DomainExtractResponse_AtMaxSize(t *testing.T) {
	// Build payload just under the limit
	data := `{"d":"` + strings.Repeat("x", maxDomainDataBytes-10) + `"}`
	if len(data) > maxDomainDataBytes {
		// Trim to fit
		data = data[:maxDomainDataBytes]
	}

	resp := &DomainExtractResponse{
		ArtifactID:      "art-chaos-02",
		Success:         true,
		DomainData:      json.RawMessage(data),
		ContractVersion: "recipe-extraction-v1",
	}

	err := ValidateDomainExtractResponse(resp)
	if err != nil {
		t.Errorf("domain_data at/under limit should pass: %v", err)
	}
}

// TestChaos_DomainExtractResponse_EmptyDomainData_Failure verifies that
// empty domain_data on a failure response is still valid.
func TestChaos_DomainExtractResponse_EmptyDomainData_Failure(t *testing.T) {
	resp := &DomainExtractResponse{
		ArtifactID:      "art-chaos-03",
		Success:         false,
		Error:           "LLM returned garbage",
		ContractVersion: "product-extraction-v1",
	}

	err := ValidateDomainExtractResponse(resp)
	if err != nil {
		t.Errorf("failure with empty domain_data should pass: %v", err)
	}
}

// TestChaos_DomainExtractResponse_NullJSONDomainData verifies that
// null/nil domain_data is handled correctly.
func TestChaos_DomainExtractResponse_NullJSONDomainData(t *testing.T) {
	resp := &DomainExtractResponse{
		ArtifactID:      "art-chaos-04",
		Success:         true,
		DomainData:      nil,
		ContractVersion: "recipe-extraction-v1",
	}

	err := ValidateDomainExtractResponse(resp)
	if err == nil {
		t.Error("success=true with nil domain_data should fail validation")
	}
}

// TestChaos_MaxDomainDataBytes_Constant verifies the constant value is sensible.
func TestChaos_MaxDomainDataBytes_Constant(t *testing.T) {
	if maxDomainDataBytes != 512*1024 {
		t.Errorf("expected maxDomainDataBytes=524288, got %d", maxDomainDataBytes)
	}
	if maxDomainDataBytes > MaxNATSMessageSize {
		t.Errorf("maxDomainDataBytes (%d) should not exceed MaxNATSMessageSize (%d)",
			maxDomainDataBytes, MaxNATSMessageSize)
	}
}
