package pipeline

import "fmt"

// SynthesisExtractRequest is what core publishes to synthesis.extract for ML processing.
type SynthesisExtractRequest struct {
	ArtifactID            string                    `json:"artifact_id"`
	ContentType           string                    `json:"content_type"`
	Title                 string                    `json:"title"`
	Summary               string                    `json:"summary"`
	ContentRaw            string                    `json:"content_raw"`
	KeyIdeas              []string                  `json:"key_ideas"`
	Entities              map[string][]string       `json:"entities"`
	Topics                []string                  `json:"topics"`
	SourceID              string                    `json:"source_id"`
	SourceType            string                    `json:"source_type"`
	ExistingConcepts      []SynthesisConceptSummary `json:"existing_concepts"`
	ExistingEntities      []SynthesisEntitySummary  `json:"existing_entities"`
	PromptContractVersion string                    `json:"prompt_contract_version"`
	RetryCount            int                       `json:"retry_count"`
	TraceID               string                    `json:"trace_id,omitempty"`
}

// SynthesisConceptSummary is a lightweight concept summary included in extraction requests.
type SynthesisConceptSummary struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Summary string `json:"summary"`
}

// SynthesisEntitySummary is a lightweight entity summary included in extraction requests.
type SynthesisEntitySummary struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

// SynthesisExtractResponse is what ML sidecar publishes to synthesis.extracted.
type SynthesisExtractResponse struct {
	ArtifactID            string           `json:"artifact_id"`
	Success               bool             `json:"success"`
	Error                 string           `json:"error,omitempty"`
	Result                *SynthesisResult `json:"result,omitempty"`
	PromptContractVersion string           `json:"prompt_contract_version"`
	ProcessingTimeMs      int64            `json:"processing_time_ms"`
	ModelUsed             string           `json:"model_used"`
	TokensUsed            int              `json:"tokens_used"`
}

// SynthesisResult holds the extracted knowledge from an artifact.
type SynthesisResult struct {
	Concepts       []ExtractedConcept       `json:"concepts"`
	Entities       []ExtractedEntity        `json:"entities"`
	Relationships  []ExtractedRelationship  `json:"relationships"`
	Contradictions []ExtractedContradiction `json:"contradictions"`
}

// ExtractedConcept is a concept extracted by the ML sidecar.
type ExtractedConcept struct {
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Claims      []ExtractedClaim `json:"claims"`
	IsNew       bool             `json:"is_new"`
}

// ExtractedClaim is a factual assertion extracted from an artifact.
type ExtractedClaim struct {
	Text       string  `json:"text"`
	Confidence float64 `json:"confidence"`
}

// ExtractedEntity is an entity extracted by the ML sidecar.
type ExtractedEntity struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Context string `json:"context"`
}

// ExtractedRelationship is a relationship between concepts or entities.
type ExtractedRelationship struct {
	Source      string `json:"source"`
	Target      string `json:"target"`
	Type        string `json:"type"`
	Description string `json:"description"`
}

// ExtractedContradiction flags conflicting claims between new and existing knowledge.
type ExtractedContradiction struct {
	Concept            string `json:"concept"`
	ExistingClaim      string `json:"existing_claim"`
	NewClaim           string `json:"new_claim"`
	ExistingArtifactID string `json:"existing_artifact_id"`
}

// CrossSourceRequest is what core publishes to synthesis.crosssource.
type CrossSourceRequest struct {
	ConceptID             string                       `json:"concept_id"`
	ConceptTitle          string                       `json:"concept_title"`
	Artifacts             []CrossSourceArtifactSummary `json:"artifacts"`
	PromptContractVersion string                       `json:"prompt_contract_version"`
	TraceID               string                       `json:"trace_id,omitempty"`
}

// CrossSourceArtifactSummary is a lightweight artifact summary for cross-source assessment.
type CrossSourceArtifactSummary struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	SourceType string `json:"source_type"`
	Summary    string `json:"summary"`
}

// CrossSourceResponse is what ML sidecar publishes to synthesis.crosssource.result.
type CrossSourceResponse struct {
	ConceptID             string   `json:"concept_id"`
	HasGenuineConnection  bool     `json:"has_genuine_connection"`
	InsightText           string   `json:"insight_text"`
	Confidence            float64  `json:"confidence"`
	ArtifactIDs           []string `json:"artifact_ids"`
	PromptContractVersion string   `json:"prompt_contract_version"`
	ProcessingTimeMs      int64    `json:"processing_time_ms"`
	ModelUsed             string   `json:"model_used"`
}

// ValidateSynthesisExtractRequest validates a synthesis extraction request.
func ValidateSynthesisExtractRequest(r *SynthesisExtractRequest) error {
	if r.ArtifactID == "" {
		return fmt.Errorf("SynthesisExtractRequest: artifact_id is required")
	}
	if r.ContentType == "" {
		return fmt.Errorf("SynthesisExtractRequest: content_type is required")
	}
	if r.ContentRaw == "" && r.Summary == "" && r.Title == "" {
		return fmt.Errorf("SynthesisExtractRequest: at least one of content_raw, summary, or title is required")
	}
	if r.PromptContractVersion == "" {
		return fmt.Errorf("SynthesisExtractRequest: prompt_contract_version is required")
	}
	return nil
}

// ValidateSynthesisExtractResponse validates a synthesis extraction response.
func ValidateSynthesisExtractResponse(r *SynthesisExtractResponse) error {
	if r.ArtifactID == "" {
		return fmt.Errorf("SynthesisExtractResponse: artifact_id is required")
	}
	return nil
}
