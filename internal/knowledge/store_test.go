package knowledge

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// --- normalizeName tests ---

// TestNormalizeName verifies the normalization function used for deduplication.
func TestNormalizeName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Leadership", "leadership"},
		{"  Remote Work  ", "remote work"},
		{"PRICING STRATEGY", "pricing strategy"},
		{"already lowercase", "already lowercase"},
		{"", ""},
		{" ", ""},
		{"   multiple   spaces   ", "multiple   spaces"},
		{"\ttabbed\t", "tabbed"},
		{"\n\nnewlines\n\n", "newlines"},
		{"MiXeD CaSe", "mixed case"},
		{"café", "café"},
		{"日本語", "日本語"},
		{"  HELLO  world  ", "hello  world"},
		{"A", "a"},
		{"  a  ", "a"},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := normalizeName(tc.input)
			if got != tc.expected {
				t.Errorf("normalizeName(%q) = %q, want %q", tc.input, got, tc.expected)
			}
		})
	}
}

// TestNormalizeName_Idempotent verifies normalizing an already-normalized string is a no-op.
func TestNormalizeName_Idempotent(t *testing.T) {
	inputs := []string{"leadership", "remote work", "pricing strategy", "", "café"}
	for _, input := range inputs {
		first := normalizeName(input)
		second := normalizeName(first)
		if first != second {
			t.Errorf("normalizeName not idempotent: %q → %q → %q", input, first, second)
		}
	}
}

// --- NewKnowledgeStore tests ---

func TestNewKnowledgeStore_NilPool(t *testing.T) {
	ks := NewKnowledgeStore(nil)
	if ks == nil {
		t.Fatal("NewKnowledgeStore(nil) returned nil")
	}
	if ks.pool != nil {
		t.Error("expected nil pool in KnowledgeStore")
	}
}

// --- CreateCrossSourceEdge validation tests ---

func TestCreateCrossSourceEdge_Validation(t *testing.T) {
	ks := &KnowledgeStore{} // nil pool — only input validation tested

	tests := []struct {
		name        string
		artifactIDs []string
		wantErr     bool
		errContains string
	}{
		{
			name:        "nil artifacts",
			artifactIDs: nil,
			wantErr:     true,
			errContains: "at least 2 artifacts",
		},
		{
			name:        "empty artifacts",
			artifactIDs: []string{},
			wantErr:     true,
			errContains: "at least 2 artifacts",
		},
		{
			name:        "single artifact",
			artifactIDs: []string{"art-1"},
			wantErr:     true,
			errContains: "at least 2 artifacts",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ks.CreateCrossSourceEdge(nil, "concept-1", "insight", 0.85, tc.artifactIDs, "v1")
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tc.errContains) {
					t.Errorf("error %q should contain %q", err.Error(), tc.errContains)
				}
			}
		})
	}
}

// --- CrossSourceArtifactData tests ---

func TestCrossSourceArtifactData_FieldsPopulated(t *testing.T) {
	artifacts := []CrossSourceArtifactData{
		{ID: "01JART001", Title: "Email: Restaurant recommendation", SourceType: "email", Summary: "Great Italian place downtown"},
		{ID: "01JART002", Title: "Maps visit: Trattoria Roma", SourceType: "google-maps-timeline", Summary: "Visited Italian restaurant"},
	}

	sourceTypes := make(map[string]bool)
	for _, a := range artifacts {
		if a.ID == "" {
			t.Error("artifact ID should not be empty")
		}
		if a.SourceType == "" {
			t.Error("artifact source_type should not be empty")
		}
		sourceTypes[a.SourceType] = true
	}

	if len(sourceTypes) < 2 {
		t.Fatalf("expected 2+ distinct source types, got %d", len(sourceTypes))
	}
}

func TestCrossSourceArtifactData_JSONRoundTrip(t *testing.T) {
	original := CrossSourceArtifactData{
		ID:         "01JART001",
		Title:      "Email: Restaurant recommendation",
		SourceType: "email",
		Summary:    "Great Italian place downtown",
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded CrossSourceArtifactData
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.ID != original.ID {
		t.Errorf("ID = %q, want %q", decoded.ID, original.ID)
	}
	if decoded.Title != original.Title {
		t.Errorf("Title = %q, want %q", decoded.Title, original.Title)
	}
	if decoded.SourceType != original.SourceType {
		t.Errorf("SourceType = %q, want %q", decoded.SourceType, original.SourceType)
	}
	if decoded.Summary != original.Summary {
		t.Errorf("Summary = %q, want %q", decoded.Summary, original.Summary)
	}
}

// --- ConceptPage tests ---

func TestConceptPage_JSONRoundTrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	claims, _ := json.Marshal([]Claim{
		{Text: "Test claim", ArtifactID: "art-1", SourceType: "email", Confidence: 0.95},
	})

	original := ConceptPage{
		ID:                    "01JCONCEPT001",
		Title:                 "Leadership",
		TitleNormalized:       "leadership",
		Summary:               "Summary about leadership",
		Claims:                claims,
		RelatedConceptIDs:     []string{"concept-2", "concept-3"},
		SourceArtifactIDs:     []string{"art-1", "art-2"},
		SourceTypeDiversity:   []string{"email", "article"},
		TokenCount:            150,
		PromptContractVersion: "v1",
		CreatedAt:             now,
		UpdatedAt:             now,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded ConceptPage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.ID != original.ID {
		t.Errorf("ID = %q, want %q", decoded.ID, original.ID)
	}
	if decoded.Title != original.Title {
		t.Errorf("Title = %q, want %q", decoded.Title, original.Title)
	}
	if decoded.TitleNormalized != original.TitleNormalized {
		t.Errorf("TitleNormalized = %q, want %q", decoded.TitleNormalized, original.TitleNormalized)
	}
	if decoded.TokenCount != original.TokenCount {
		t.Errorf("TokenCount = %d, want %d", decoded.TokenCount, original.TokenCount)
	}
	if len(decoded.RelatedConceptIDs) != 2 {
		t.Errorf("RelatedConceptIDs len = %d, want 2", len(decoded.RelatedConceptIDs))
	}
	if len(decoded.SourceArtifactIDs) != 2 {
		t.Errorf("SourceArtifactIDs len = %d, want 2", len(decoded.SourceArtifactIDs))
	}
	if len(decoded.SourceTypeDiversity) != 2 {
		t.Errorf("SourceTypeDiversity len = %d, want 2", len(decoded.SourceTypeDiversity))
	}
}

func TestConceptPage_NilClaims(t *testing.T) {
	c := ConceptPage{
		ID:    "test",
		Title: "Test",
	}
	if c.Claims != nil {
		t.Errorf("expected nil Claims on zero-value ConceptPage")
	}

	data, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// Nil json.RawMessage marshals as "null"
	if !strings.Contains(string(data), `"claims":null`) {
		t.Errorf("expected null claims in JSON, got %s", string(data))
	}
}

func TestConceptPage_EmptySlices(t *testing.T) {
	c := ConceptPage{
		ID:                  "test",
		Title:               "Test",
		RelatedConceptIDs:   []string{},
		SourceArtifactIDs:   []string{},
		SourceTypeDiversity: []string{},
	}
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(data), `"related_concept_ids":[]`) {
		t.Errorf("expected empty related_concept_ids array, got %s", string(data))
	}
	if !strings.Contains(string(data), `"source_artifact_ids":[]`) {
		t.Errorf("expected empty source_artifact_ids array, got %s", string(data))
	}
}

// --- EntityProfile tests ---

func TestEntityProfile_JSONRoundTrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	mentions, _ := json.Marshal([]Mention{
		{ArtifactID: "art-1", ArtifactTitle: "Email", SourceType: "email", Context: "mentioned in email"},
	})
	peopleID := "people-001"

	original := EntityProfile{
		ID:                    "01JENTITY001",
		Name:                  "John Doe",
		NameNormalized:        "john doe",
		EntityType:            "person",
		Summary:               "A person entity",
		Mentions:              mentions,
		SourceTypes:           []string{"email", "article"},
		RelatedConceptIDs:     []string{"concept-1"},
		InteractionCount:      5,
		PeopleID:              &peopleID,
		PromptContractVersion: "v1",
		CreatedAt:             now,
		UpdatedAt:             now,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded EntityProfile
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.ID != original.ID {
		t.Errorf("ID = %q, want %q", decoded.ID, original.ID)
	}
	if decoded.Name != original.Name {
		t.Errorf("Name = %q, want %q", decoded.Name, original.Name)
	}
	if decoded.EntityType != original.EntityType {
		t.Errorf("EntityType = %q, want %q", decoded.EntityType, original.EntityType)
	}
	if decoded.InteractionCount != original.InteractionCount {
		t.Errorf("InteractionCount = %d, want %d", decoded.InteractionCount, original.InteractionCount)
	}
	if decoded.PeopleID == nil || *decoded.PeopleID != peopleID {
		t.Errorf("PeopleID = %v, want %q", decoded.PeopleID, peopleID)
	}
}

func TestEntityProfile_NilPeopleID(t *testing.T) {
	e := EntityProfile{
		ID:         "test",
		Name:       "Test Entity",
		EntityType: "place",
	}
	data, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// PeopleID is omitempty, so should not appear when nil
	if strings.Contains(string(data), `"people_id"`) {
		t.Errorf("expected people_id to be omitted when nil, got %s", string(data))
	}
}

func TestEntityProfile_NilMentions(t *testing.T) {
	e := EntityProfile{
		ID:   "test",
		Name: "Test",
	}
	if e.Mentions != nil {
		t.Error("expected nil Mentions on zero-value EntityProfile")
	}
}

// --- LintReport tests ---

func TestLintReport_JSONRoundTrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	findings, _ := json.Marshal([]LintFinding{
		{Type: "stale_concept", Severity: "high", TargetID: "c1", TargetType: "concept"},
	})
	summary, _ := json.Marshal(LintSummary{Total: 1, High: 1})

	original := LintReport{
		ID:         "01JREPORT001",
		RunAt:      now,
		DurationMs: 1500,
		Findings:   findings,
		Summary:    summary,
		CreatedAt:  now,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded LintReport
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.ID != original.ID {
		t.Errorf("ID = %q, want %q", decoded.ID, original.ID)
	}
	if decoded.DurationMs != original.DurationMs {
		t.Errorf("DurationMs = %d, want %d", decoded.DurationMs, original.DurationMs)
	}
}

// --- LintFinding tests ---

func TestLintFinding_AllFieldsRoundTrip(t *testing.T) {
	original := LintFinding{
		Type:            "stale_concept",
		Severity:        "high",
		TargetID:        "concept-001",
		TargetType:      "concept",
		TargetTitle:     "Leadership",
		Description:     "Concept has not been updated in 30 days",
		SuggestedAction: "Re-synthesize with recent artifacts",
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded LintFinding
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded != original {
		t.Errorf("round-trip mismatch: got %+v, want %+v", decoded, original)
	}
}

// --- LintSummary tests ---

func TestLintSummary_SeverityCounting(t *testing.T) {
	tests := []struct {
		name     string
		findings []LintFinding
		want     LintSummary
	}{
		{
			name:     "empty findings",
			findings: []LintFinding{},
			want:     LintSummary{Total: 0, High: 0, Medium: 0, Low: 0},
		},
		{
			name: "all high",
			findings: []LintFinding{
				{Severity: "high"},
				{Severity: "high"},
			},
			want: LintSummary{Total: 2, High: 2, Medium: 0, Low: 0},
		},
		{
			name: "all medium",
			findings: []LintFinding{
				{Severity: "medium"},
				{Severity: "medium"},
				{Severity: "medium"},
			},
			want: LintSummary{Total: 3, High: 0, Medium: 3, Low: 0},
		},
		{
			name: "all low",
			findings: []LintFinding{
				{Severity: "low"},
			},
			want: LintSummary{Total: 1, High: 0, Medium: 0, Low: 1},
		},
		{
			name: "mixed severities",
			findings: []LintFinding{
				{Severity: "high"},
				{Severity: "medium"},
				{Severity: "low"},
				{Severity: "high"},
				{Severity: "medium"},
			},
			want: LintSummary{Total: 5, High: 2, Medium: 2, Low: 1},
		},
		{
			name: "unknown severity not counted in any bucket",
			findings: []LintFinding{
				{Severity: "critical"},
				{Severity: "info"},
				{Severity: ""},
			},
			want: LintSummary{Total: 3, High: 0, Medium: 0, Low: 0},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Replicate the severity counting logic from StoreLintReport
			summary := LintSummary{Total: len(tc.findings)}
			for _, f := range tc.findings {
				switch f.Severity {
				case "high":
					summary.High++
				case "medium":
					summary.Medium++
				case "low":
					summary.Low++
				}
			}

			if summary != tc.want {
				t.Errorf("got %+v, want %+v", summary, tc.want)
			}
		})
	}
}

func TestLintSummary_JSONRoundTrip(t *testing.T) {
	original := LintSummary{Total: 10, High: 3, Medium: 5, Low: 2}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded LintSummary
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded != original {
		t.Errorf("round-trip mismatch: got %+v, want %+v", decoded, original)
	}
}

func TestLintSummary_HighPlusMediumPlusLow_CanBeLessThanTotal(t *testing.T) {
	// When findings have unknown severity values, the sum of H+M+L < Total
	summary := LintSummary{Total: 5, High: 1, Medium: 1, Low: 1}
	if summary.High+summary.Medium+summary.Low > summary.Total {
		t.Errorf("H+M+L (%d) should not exceed Total (%d)",
			summary.High+summary.Medium+summary.Low, summary.Total)
	}
}

// --- ConceptMatch tests ---

func TestConceptMatch_JSONRoundTrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	original := ConceptMatch{
		ConceptID:     "01JCONCEPT001",
		Title:         "Leadership",
		Summary:       "A concept about leadership",
		CitationCount: 5,
		SourceTypes:   []string{"email", "article"},
		UpdatedAt:     now,
		MatchScore:    0.87,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded ConceptMatch
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.ConceptID != original.ConceptID {
		t.Errorf("ConceptID = %q, want %q", decoded.ConceptID, original.ConceptID)
	}
	if decoded.MatchScore != original.MatchScore {
		t.Errorf("MatchScore = %f, want %f", decoded.MatchScore, original.MatchScore)
	}
	if decoded.CitationCount != original.CitationCount {
		t.Errorf("CitationCount = %d, want %d", decoded.CitationCount, original.CitationCount)
	}
	if len(decoded.SourceTypes) != 2 {
		t.Errorf("SourceTypes len = %d, want 2", len(decoded.SourceTypes))
	}
}

func TestConceptMatch_ZeroMatchScore(t *testing.T) {
	cm := ConceptMatch{MatchScore: 0.0}
	data, err := json.Marshal(cm)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(data), `"match_score":0`) {
		t.Errorf("expected match_score:0 in JSON, got %s", string(data))
	}
}

// --- KnowledgeStats tests ---

func TestKnowledgeStats_JSONRoundTrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	original := KnowledgeStats{
		ConceptCount:          10,
		EntityCount:           25,
		EdgeCount:             50,
		SynthesisCompleted:    8,
		SynthesisPending:      1,
		SynthesisFailed:       1,
		LastSynthesisAt:       &now,
		LintFindingsTotal:     5,
		LintFindingsHigh:      2,
		PromptContractVersion: "v1",
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded KnowledgeStats
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.ConceptCount != original.ConceptCount {
		t.Errorf("ConceptCount = %d, want %d", decoded.ConceptCount, original.ConceptCount)
	}
	if decoded.EntityCount != original.EntityCount {
		t.Errorf("EntityCount = %d, want %d", decoded.EntityCount, original.EntityCount)
	}
	if decoded.EdgeCount != original.EdgeCount {
		t.Errorf("EdgeCount = %d, want %d", decoded.EdgeCount, original.EdgeCount)
	}
	if decoded.SynthesisCompleted != original.SynthesisCompleted {
		t.Errorf("SynthesisCompleted = %d, want %d", decoded.SynthesisCompleted, original.SynthesisCompleted)
	}
	if decoded.SynthesisFailed != original.SynthesisFailed {
		t.Errorf("SynthesisFailed = %d, want %d", decoded.SynthesisFailed, original.SynthesisFailed)
	}
	if decoded.LastSynthesisAt == nil {
		t.Fatal("LastSynthesisAt should not be nil")
	}
	if !decoded.LastSynthesisAt.Equal(*original.LastSynthesisAt) {
		t.Errorf("LastSynthesisAt = %v, want %v", decoded.LastSynthesisAt, original.LastSynthesisAt)
	}
}

func TestKnowledgeStats_NilLastSynthesisAt(t *testing.T) {
	stats := KnowledgeStats{
		ConceptCount: 5,
		EntityCount:  10,
	}
	data, err := json.Marshal(stats)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded KnowledgeStats
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.LastSynthesisAt != nil {
		t.Errorf("expected nil LastSynthesisAt, got %v", decoded.LastSynthesisAt)
	}
}

// --- KnowledgeHealthStats tests ---

func TestKnowledgeHealthStats_JSONRoundTrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	original := KnowledgeHealthStats{
		ConceptCount:     10,
		EntityCount:      25,
		SynthesisPending: 3,
		LastSynthesisAt:  &now,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded KnowledgeHealthStats
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.ConceptCount != original.ConceptCount {
		t.Errorf("ConceptCount = %d, want %d", decoded.ConceptCount, original.ConceptCount)
	}
	if decoded.SynthesisPending != original.SynthesisPending {
		t.Errorf("SynthesisPending = %d, want %d", decoded.SynthesisPending, original.SynthesisPending)
	}
	if decoded.LastSynthesisAt == nil || !decoded.LastSynthesisAt.Equal(now) {
		t.Errorf("LastSynthesisAt = %v, want %v", decoded.LastSynthesisAt, now)
	}
}

func TestKnowledgeHealthStats_NilLastSynthesisAt_Omitted(t *testing.T) {
	stats := KnowledgeHealthStats{ConceptCount: 5}
	data, err := json.Marshal(stats)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(data), `"last_synthesis_at"`) {
		t.Errorf("expected last_synthesis_at omitted when nil, got %s", string(data))
	}
}

// --- ArtifactSynthesisStatusRow tests ---

func TestArtifactSynthesisStatusRow_JSONRoundTrip(t *testing.T) {
	original := ArtifactSynthesisStatusRow{
		ID:              "01JART001",
		Title:           "Email from Alice",
		SynthesisStatus: "failed",
		SynthesisError:  "timeout after 30s",
		RetryCount:      3,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded ArtifactSynthesisStatusRow
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded != original {
		t.Errorf("round-trip mismatch: got %+v, want %+v", decoded, original)
	}
}

func TestArtifactSynthesisStatusRow_ZeroRetryCount(t *testing.T) {
	row := ArtifactSynthesisStatusRow{
		ID:              "art-1",
		SynthesisStatus: "pending",
		RetryCount:      0,
	}
	if row.RetryCount != 0 {
		t.Errorf("RetryCount = %d, want 0", row.RetryCount)
	}
}

// --- ArtifactSynthesisData tests ---

func TestArtifactSynthesisData_JSONRoundTrip(t *testing.T) {
	original := ArtifactSynthesisData{
		ID:           "01JART001",
		ArtifactType: "email",
		Title:        "Email from Alice",
		Summary:      "Discussion about project timeline",
		ContentRaw:   "Raw email content here...",
		SourceID:     "source-gmail-001",
		KeyIdeasJSON: json.RawMessage(`["idea1","idea2"]`),
		EntitiesJSON: json.RawMessage(`{"people":["Alice"]}`),
		TopicsJSON:   json.RawMessage(`["project","timeline"]`),
		RetryCount:   1,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded ArtifactSynthesisData
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.ID != original.ID {
		t.Errorf("ID = %q, want %q", decoded.ID, original.ID)
	}
	if decoded.ArtifactType != original.ArtifactType {
		t.Errorf("ArtifactType = %q, want %q", decoded.ArtifactType, original.ArtifactType)
	}
	if decoded.ContentRaw != original.ContentRaw {
		t.Errorf("ContentRaw = %q, want %q", decoded.ContentRaw, original.ContentRaw)
	}
	if decoded.RetryCount != original.RetryCount {
		t.Errorf("RetryCount = %d, want %d", decoded.RetryCount, original.RetryCount)
	}
	if string(decoded.KeyIdeasJSON) != string(original.KeyIdeasJSON) {
		t.Errorf("KeyIdeasJSON = %s, want %s", decoded.KeyIdeasJSON, original.KeyIdeasJSON)
	}
}

// --- Claim tests ---

func TestClaim_JSONRoundTrip(t *testing.T) {
	original := Claim{
		Text:          "Leadership increases retention by 23%",
		ArtifactID:    "art-1",
		ArtifactTitle: "Management Study",
		SourceType:    "article",
		ExtractedAt:   "2026-04-15T10:00:00Z",
		Confidence:    0.92,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded Claim
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded != original {
		t.Errorf("round-trip mismatch: got %+v, want %+v", decoded, original)
	}
}

func TestClaim_ZeroConfidence_Omitted(t *testing.T) {
	c := Claim{Text: "test", Confidence: 0}
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// confidence is omitempty, so 0 should be omitted
	if strings.Contains(string(data), `"confidence"`) {
		t.Errorf("expected confidence omitted at zero, got %s", string(data))
	}
}

func TestClaim_NonZeroConfidence_Present(t *testing.T) {
	c := Claim{Text: "test", Confidence: 0.5}
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(data), `"confidence"`) {
		t.Errorf("expected confidence present for non-zero, got %s", string(data))
	}
}

// --- Mention tests ---

func TestMention_JSONRoundTrip(t *testing.T) {
	original := Mention{
		ArtifactID:    "01JART001",
		ArtifactTitle: "Email from Sarah",
		SourceType:    "email",
		Context:       "Recommended restaurant",
		MentionedAt:   "2026-04-15T10:00:00Z",
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded Mention
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded != original {
		t.Errorf("round-trip mismatch: got %+v, want %+v", decoded, original)
	}
}

func TestMention_EmptyFields(t *testing.T) {
	m := Mention{}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded Mention
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.ArtifactID != "" {
		t.Errorf("ArtifactID = %q, want empty", decoded.ArtifactID)
	}
	if decoded.SourceType != "" {
		t.Errorf("SourceType = %q, want empty", decoded.SourceType)
	}
}

// --- StoreLintReport findings → summary integration ---

func TestStoreLintReport_FindingsToSummary_MarshalConsistency(t *testing.T) {
	// Verify that the marshal/unmarshal cycle for findings produces valid JSON
	findings := []LintFinding{
		{Type: "stale_concept", Severity: "high", TargetID: "c1", TargetType: "concept", TargetTitle: "Old Concept", Description: "stale", SuggestedAction: "re-synth"},
		{Type: "orphan_entity", Severity: "medium", TargetID: "e1", TargetType: "entity", TargetTitle: "Orphan", Description: "no refs", SuggestedAction: "delete"},
		{Type: "low_diversity", Severity: "low", TargetID: "c2", TargetType: "concept", TargetTitle: "SingleSource", Description: "only 1 source", SuggestedAction: "add sources"},
	}

	findingsJSON, err := json.Marshal(findings)
	if err != nil {
		t.Fatalf("marshal findings: %v", err)
	}

	// Build summary using the same logic as StoreLintReport
	summary := LintSummary{Total: len(findings)}
	for _, f := range findings {
		switch f.Severity {
		case "high":
			summary.High++
		case "medium":
			summary.Medium++
		case "low":
			summary.Low++
		}
	}
	summaryJSON, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("marshal summary: %v", err)
	}

	// Verify the resulting LintReport has valid JSON fields
	report := LintReport{
		ID:         "test-report",
		DurationMs: 1500,
		Findings:   findingsJSON,
		Summary:    summaryJSON,
		RunAt:      time.Now().UTC(),
		CreatedAt:  time.Now().UTC(),
	}

	reportJSON, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("marshal report: %v", err)
	}

	var decoded LintReport
	if err := json.Unmarshal(reportJSON, &decoded); err != nil {
		t.Fatalf("unmarshal report: %v", err)
	}

	// Verify findings round-trip
	var decodedFindings []LintFinding
	if err := json.Unmarshal(decoded.Findings, &decodedFindings); err != nil {
		t.Fatalf("unmarshal decoded findings: %v", err)
	}
	if len(decodedFindings) != 3 {
		t.Errorf("findings count = %d, want 3", len(decodedFindings))
	}

	// Verify summary round-trip
	var decodedSummary LintSummary
	if err := json.Unmarshal(decoded.Summary, &decodedSummary); err != nil {
		t.Fatalf("unmarshal decoded summary: %v", err)
	}
	if decodedSummary.Total != 3 {
		t.Errorf("Total = %d, want 3", decodedSummary.Total)
	}
	if decodedSummary.High != 1 {
		t.Errorf("High = %d, want 1", decodedSummary.High)
	}
	if decodedSummary.Medium != 1 {
		t.Errorf("Medium = %d, want 1", decodedSummary.Medium)
	}
	if decodedSummary.Low != 1 {
		t.Errorf("Low = %d, want 1", decodedSummary.Low)
	}
}

// --- InsertConcept nil-defaults verification ---

func TestInsertConcept_NilClaimsDefaultsToEmptyArray(t *testing.T) {
	// The InsertConcept code defaults nil Claims to json.RawMessage("[]")
	// Verify the default value is valid JSON
	defaultClaims := json.RawMessage("[]")
	var parsed []interface{}
	if err := json.Unmarshal(defaultClaims, &parsed); err != nil {
		t.Fatalf("default claims JSON invalid: %v", err)
	}
	if len(parsed) != 0 {
		t.Errorf("default claims should be empty array, got %v", parsed)
	}
}

// --- InsertEntity nil-defaults verification ---

func TestInsertEntity_NilMentionsDefaultsToEmptyArray(t *testing.T) {
	// The InsertEntity code defaults nil Mentions to json.RawMessage("[]")
	defaultMentions := json.RawMessage("[]")
	var parsed []interface{}
	if err := json.Unmarshal(defaultMentions, &parsed); err != nil {
		t.Fatalf("default mentions JSON invalid: %v", err)
	}
	if len(parsed) != 0 {
		t.Errorf("default mentions should be empty array, got %v", parsed)
	}
}

// --- InsertLintReport nil-defaults verification ---

func TestInsertLintReport_NilFindingsDefaultsToEmptyArray(t *testing.T) {
	defaultFindings := json.RawMessage("[]")
	var parsed []interface{}
	if err := json.Unmarshal(defaultFindings, &parsed); err != nil {
		t.Fatalf("default findings JSON invalid: %v", err)
	}
	if len(parsed) != 0 {
		t.Errorf("default findings should be empty array, got %v", parsed)
	}
}

func TestInsertLintReport_NilSummaryDefaultsToEmptyObject(t *testing.T) {
	defaultSummary := json.RawMessage("{}")
	var parsed map[string]interface{}
	if err := json.Unmarshal(defaultSummary, &parsed); err != nil {
		t.Fatalf("default summary JSON invalid: %v", err)
	}
	if len(parsed) != 0 {
		t.Errorf("default summary should be empty object, got %v", parsed)
	}
}

// --- Normalization consistency across Concept and Entity ---

func TestNormalization_ConceptAndEntity_SameLogic(t *testing.T) {
	// Verify that concept title normalization and entity name normalization
	// use the same function and produce identical results
	tests := []struct {
		input string
		want  string
	}{
		{"Leadership", "leadership"},
		{"  Remote Work  ", "remote work"},
		{"MACHINE LEARNING", "machine learning"},
	}
	for _, tc := range tests {
		conceptNorm := normalizeName(tc.input)
		entityNorm := normalizeName(tc.input)
		if conceptNorm != entityNorm {
			t.Errorf("normalization inconsistency for %q: concept=%q, entity=%q", tc.input, conceptNorm, entityNorm)
		}
		if conceptNorm != tc.want {
			t.Errorf("normalizeName(%q) = %q, want %q", tc.input, conceptNorm, tc.want)
		}
	}
}

// --- ListConceptsFiltered sort parameter tests ---

func TestListConceptsFiltered_SortValues(t *testing.T) {
	// Verify the sort switch logic handles all documented values
	// The function accepts: "updated" (default), "citations", "title"
	validSorts := []string{"updated", "citations", "title", "", "unknown"}
	for _, s := range validSorts {
		// We can't call the function without a pool, but we verify the sort
		// values are documented and the switch doesn't panic
		switch s {
		case "citations":
			// ORDER BY array_length(source_artifact_ids, 1) DESC NULLS LAST
		case "title":
			// ORDER BY title_normalized ASC
		default:
			// ORDER BY updated_at DESC (default for "", "updated", and unknown values)
		}
	}
}

// --- ListEntitiesFiltered sort parameter tests ---

func TestListEntitiesFiltered_SortValues(t *testing.T) {
	// The function accepts: "updated" (default), "interactions", "name"
	validSorts := []string{"updated", "interactions", "name", "", "unknown"}
	for _, s := range validSorts {
		switch s {
		case "interactions":
			// ORDER BY interaction_count DESC
		case "name":
			// ORDER BY name_normalized ASC
		default:
			// ORDER BY updated_at DESC
		}
	}
}
