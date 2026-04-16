package knowledge

import (
	"encoding/json"
	"time"
)

// ConceptPage represents a pre-synthesized knowledge concept stored in the DB.
type ConceptPage struct {
	ID                    string          `json:"id"`
	Title                 string          `json:"title"`
	TitleNormalized       string          `json:"title_normalized"`
	Summary               string          `json:"summary"`
	Claims                json.RawMessage `json:"claims"`
	RelatedConceptIDs     []string        `json:"related_concept_ids"`
	SourceArtifactIDs     []string        `json:"source_artifact_ids"`
	SourceTypeDiversity   []string        `json:"source_type_diversity"`
	TokenCount            int             `json:"token_count"`
	PromptContractVersion string          `json:"prompt_contract_version"`
	CreatedAt             time.Time       `json:"created_at"`
	UpdatedAt             time.Time       `json:"updated_at"`
}

// Claim is a factual assertion extracted from an artifact.
type Claim struct {
	Text          string  `json:"text"`
	ArtifactID    string  `json:"artifact_id"`
	ArtifactTitle string  `json:"artifact_title"`
	SourceType    string  `json:"source_type"`
	ExtractedAt   string  `json:"extracted_at"`
	Confidence    float64 `json:"confidence,omitempty"`
}

// EntityProfile represents an enriched entity profile in the knowledge layer.
type EntityProfile struct {
	ID                    string          `json:"id"`
	Name                  string          `json:"name"`
	NameNormalized        string          `json:"name_normalized"`
	EntityType            string          `json:"entity_type"`
	Summary               string          `json:"summary"`
	Mentions              json.RawMessage `json:"mentions"`
	SourceTypes           []string        `json:"source_types"`
	RelatedConceptIDs     []string        `json:"related_concept_ids"`
	InteractionCount      int             `json:"interaction_count"`
	PeopleID              *string         `json:"people_id,omitempty"`
	PromptContractVersion string          `json:"prompt_contract_version"`
	CreatedAt             time.Time       `json:"created_at"`
	UpdatedAt             time.Time       `json:"updated_at"`
}

// Mention records a reference to an entity within an artifact.
type Mention struct {
	ArtifactID    string `json:"artifact_id"`
	ArtifactTitle string `json:"artifact_title"`
	SourceType    string `json:"source_type"`
	Context       string `json:"context"`
	MentionedAt   string `json:"mentioned_at"`
}

// LintReport stores the results of a knowledge quality audit.
type LintReport struct {
	ID         string          `json:"id"`
	RunAt      time.Time       `json:"run_at"`
	DurationMs int             `json:"duration_ms"`
	Findings   json.RawMessage `json:"findings"`
	Summary    json.RawMessage `json:"summary"`
	CreatedAt  time.Time       `json:"created_at"`
}

// LintFinding describes a single quality issue found during a lint run.
type LintFinding struct {
	Type            string `json:"type"`
	Severity        string `json:"severity"`
	TargetID        string `json:"target_id"`
	TargetType      string `json:"target_type"`
	TargetTitle     string `json:"target_title"`
	Description     string `json:"description"`
	SuggestedAction string `json:"suggested_action"`
}

// LintSummary provides aggregate counts from a lint run.
type LintSummary struct {
	Total  int `json:"total"`
	High   int `json:"high"`
	Medium int `json:"medium"`
	Low    int `json:"low"`
}

// ConceptMatch represents a knowledge layer match from a search query.
type ConceptMatch struct {
	ConceptID     string    `json:"concept_id"`
	Title         string    `json:"title"`
	Summary       string    `json:"summary"`
	CitationCount int       `json:"citation_count"`
	SourceTypes   []string  `json:"source_types"`
	UpdatedAt     time.Time `json:"updated_at"`
	MatchScore    float64   `json:"match_score"`
}

// KnowledgeHealthStats holds the subset of knowledge stats needed by the health endpoint.
type KnowledgeHealthStats struct {
	ConceptCount     int        `json:"concept_count"`
	EntityCount      int        `json:"entity_count"`
	SynthesisPending int        `json:"synthesis_pending"`
	LastSynthesisAt  *time.Time `json:"last_synthesis_at,omitempty"`
}

// CrossSourceArtifactData holds artifact fields needed for cross-source assessment.
type CrossSourceArtifactData struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	SourceType string `json:"source_type"`
	Summary    string `json:"summary"`
}
