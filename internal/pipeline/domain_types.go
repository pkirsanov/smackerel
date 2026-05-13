package pipeline

import (
	"encoding/json"
	"fmt"
)

// DomainExtractRequest is what core publishes to domain.extract for ML processing.
type DomainExtractRequest struct {
	ArtifactID      string `json:"artifact_id"`
	ContentType     string `json:"content_type"`
	Title           string `json:"title"`
	Summary         string `json:"summary"`
	ContentRaw      string `json:"content_raw"`
	SourceURL       string `json:"source_url,omitempty"`
	ContractVersion string `json:"contract_version"`
	RetryCount      int    `json:"retry_count"`
	TraceID         string `json:"trace_id,omitempty"`
}

// ValidateDomainExtractRequest validates a domain extraction request.
func ValidateDomainExtractRequest(r *DomainExtractRequest) error {
	if r.ArtifactID == "" {
		return fmt.Errorf("DomainExtractRequest: artifact_id is required")
	}
	if r.ContractVersion == "" {
		return fmt.Errorf("DomainExtractRequest: contract_version is required")
	}
	if r.ContentRaw == "" && r.Summary == "" && r.Title == "" {
		return fmt.Errorf("DomainExtractRequest: at least one of content_raw, summary, or title is required")
	}
	return nil
}

// DomainExtractResponse is what ML sidecar publishes to domain.extracted.
type DomainExtractResponse struct {
	ArtifactID       string          `json:"artifact_id"`
	Success          bool            `json:"success"`
	Error            string          `json:"error,omitempty"`
	DomainData       json.RawMessage `json:"domain_data,omitempty"`
	ContractVersion  string          `json:"contract_version"`
	ProcessingTimeMs int64           `json:"processing_time_ms"`
	ModelUsed        string          `json:"model_used"`
	TokensUsed       int             `json:"tokens_used"`
}

// maxDomainDataBytes is the maximum allowed size for domain_data in a response.
// Defense-in-depth: prevents oversized LLM output from bloating storage and NATS.
const maxDomainDataBytes = 512 * 1024 // 512KB

// ValidateDomainExtractResponse validates a domain extraction response.
func ValidateDomainExtractResponse(r *DomainExtractResponse) error {
	if r.ArtifactID == "" {
		return fmt.Errorf("DomainExtractResponse: artifact_id is required")
	}
	if r.Success && len(r.DomainData) == 0 {
		return fmt.Errorf("DomainExtractResponse: domain_data is required when success is true")
	}
	// C026-CHAOS-03: Reject oversized domain_data to prevent storage/transport bloat.
	if len(r.DomainData) > maxDomainDataBytes {
		return fmt.Errorf("DomainExtractResponse: domain_data too large: %d bytes exceeds max %d", len(r.DomainData), maxDomainDataBytes)
	}
	// HARDEN-026-1: ProcessingTimeMs is a wall-clock duration; negative values are
	// physically impossible and would corrupt Prometheus latency histograms if
	// observed. Reject them at the validation boundary.
	if r.ProcessingTimeMs < 0 {
		return fmt.Errorf("DomainExtractResponse: processing_time_ms must be >= 0, got %d", r.ProcessingTimeMs)
	}
	// HARDEN-026-2: TokensUsed is a counter from the LLM provider; negative values
	// are impossible and would mislead operators reading audit logs. Reject them
	// at the validation boundary.
	if r.TokensUsed < 0 {
		return fmt.Errorf("DomainExtractResponse: tokens_used must be >= 0, got %d", r.TokensUsed)
	}
	return nil
}
