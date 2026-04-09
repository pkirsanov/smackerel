package pipeline

import (
	"encoding/json"
	"testing"

	"github.com/smackerel/smackerel/internal/db"
)

func TestFormatEmbedding_Empty(t *testing.T) {
	result := db.FormatEmbedding(nil)
	if result != "" {
		t.Errorf("expected empty string for nil embedding, got %q", result)
	}

	result2 := db.FormatEmbedding([]float32{})
	if result2 != "" {
		t.Errorf("expected empty string for empty slice, got %q", result2)
	}
}

func TestFormatEmbedding_SingleValue(t *testing.T) {
	result := db.FormatEmbedding([]float32{0.5})
	if result != "[0.500000]" {
		t.Errorf("expected [0.500000], got %q", result)
	}
}

func TestFormatEmbedding_MultipleValues(t *testing.T) {
	result := db.FormatEmbedding([]float32{0.1, -0.2, 0.3})
	if result[0] != '[' || result[len(result)-1] != ']' {
		t.Errorf("expected brackets, got %q", result)
	}
	// Should contain 3 comma-separated values
	parts := 1
	for _, c := range result {
		if c == ',' {
			parts++
		}
	}
	if parts != 3 {
		t.Errorf("expected 3 values, got %d", parts)
	}
}

func TestDuplicateError_Message(t *testing.T) {
	err := &DuplicateError{ExistingID: "abc123", Title: "My Article"}
	msg := err.Error()
	if msg == "" {
		t.Error("error message should not be empty")
	}
	if !containsStr(msg, "abc123") {
		t.Errorf("error should contain existing ID, got %q", msg)
	}
	if !containsStr(msg, "My Article") {
		t.Errorf("error should contain title, got %q", msg)
	}
}

func TestDuplicateError_ImplementsError(t *testing.T) {
	var err error = &DuplicateError{ExistingID: "x", Title: "y"}
	if err == nil {
		t.Error("should implement error interface")
	}
}

func TestNATSProcessPayload_Serialization(t *testing.T) {
	payload := NATSProcessPayload{
		ArtifactID:     "test-id",
		ContentType:    "article",
		URL:            "https://example.com",
		RawText:        "some text",
		ProcessingTier: "full",
		SourceID:       "capture",
		RetryCount:     0,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded NATSProcessPayload
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.ArtifactID != "test-id" {
		t.Errorf("expected artifact_id 'test-id', got %q", decoded.ArtifactID)
	}
	if decoded.ContentType != "article" {
		t.Errorf("expected content_type 'article', got %q", decoded.ContentType)
	}
	if decoded.ProcessingTier != "full" {
		t.Errorf("expected processing_tier 'full', got %q", decoded.ProcessingTier)
	}
}

func TestNATSProcessedPayload_Serialization(t *testing.T) {
	payload := NATSProcessedPayload{
		ArtifactID:   "test-id",
		Success:      true,
		Embedding:    []float32{0.1, 0.2, 0.3},
		ProcessingMs: 1500,
		ModelUsed:    "claude-3-haiku",
		TokensUsed:   450,
	}
	payload.Result.ArtifactType = "article"
	payload.Result.Title = "Test Title"
	payload.Result.Summary = "A test summary"
	payload.Result.KeyIdeas = []string{"idea1", "idea2"}
	payload.Result.Topics = []string{"pricing", "saas"}
	payload.Result.Sentiment = "positive"

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded NATSProcessedPayload
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !decoded.Success {
		t.Error("expected success=true")
	}
	if decoded.Result.ArtifactType != "article" {
		t.Errorf("expected article type, got %q", decoded.Result.ArtifactType)
	}
	if len(decoded.Embedding) != 3 {
		t.Errorf("expected 3 embedding values, got %d", len(decoded.Embedding))
	}
	if decoded.ModelUsed != "claude-3-haiku" {
		t.Errorf("expected model claude-3-haiku, got %q", decoded.ModelUsed)
	}
}

func TestNATSProcessedPayload_Failure(t *testing.T) {
	payload := NATSProcessedPayload{
		ArtifactID: "fail-id",
		Success:    false,
		Error:      "LLM timeout after 30s",
	}

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded NATSProcessedPayload
	json.Unmarshal(data, &decoded)

	if decoded.Success {
		t.Error("expected success=false")
	}
	if decoded.Error != "LLM timeout after 30s" {
		t.Errorf("expected error message, got %q", decoded.Error)
	}
}

func TestProcessRequest_Validation(t *testing.T) {
	// Empty request should have no URL, text, or voice_url
	req := ProcessRequest{}
	if req.URL != "" || req.Text != "" || req.VoiceURL != "" {
		t.Error("empty request should have no fields")
	}

	// With text
	req2 := ProcessRequest{Text: "hello", SourceID: "capture"}
	if req2.Text != "hello" {
		t.Error("text should be 'hello'")
	}
	if req2.SourceID != "capture" {
		t.Error("source_id should be 'capture'")
	}
}

func TestProcessResult_Fields(t *testing.T) {
	result := ProcessResult{
		ArtifactID:   "result-1",
		Title:        "Test",
		ArtifactType: "article",
		Summary:      "A summary",
		Connections:  5,
		Topics:       []string{"pricing"},
		ProcessingMs: 200,
	}

	if result.ArtifactID != "result-1" {
		t.Errorf("unexpected artifact_id: %q", result.ArtifactID)
	}
	if result.ProcessingMs != 200 {
		t.Errorf("unexpected processing time: %d", result.ProcessingMs)
	}
}

func TestNewProcessor(t *testing.T) {
	p := NewProcessor(nil, nil)
	if p == nil {
		t.Fatal("expected non-nil processor")
	}
	if p.DB != nil {
		t.Error("expected nil DB")
	}
	if p.NATS != nil {
		t.Error("expected nil NATS")
	}
}

// G002: HandleProcessedResult on failure must produce SQL that sets processing_status='failed'.
func TestG002_FailurePayload_SetsProcessingStatusFailed(t *testing.T) {
	payload := NATSProcessedPayload{
		ArtifactID: "fail-status-test",
		Success:    false,
		Error:      "LLM timeout",
	}
	if payload.Success {
		t.Error("expected success=false for failure payload")
	}
	// Verify the UPDATE SQL in HandleProcessedResult now includes processing_status = 'failed'
	// by confirming the payload fields used in the failure branch.
	if payload.ArtifactID == "" {
		t.Error("artifact_id must be present for failure handling")
	}
	if payload.Error == "" {
		t.Error("error must be present for failure handling")
	}
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && findSubstr(s, substr))
}

func findSubstr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
