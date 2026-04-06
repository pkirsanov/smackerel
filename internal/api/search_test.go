package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSearchHandler_EmptyQuery(t *testing.T) {
	deps := &Dependencies{
		DB:        &mockDB{healthy: true},
		NATS:      &mockNATS{healthy: true},
		StartTime: time.Now(),
	}

	body := `{"query": ""}`
	req := httptest.NewRequest(http.MethodPost, "/api/search", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	deps.SearchHandler(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}

	var resp ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Error.Code != "EMPTY_QUERY" {
		t.Errorf("expected EMPTY_QUERY, got %q", resp.Error.Code)
	}
}

func TestSearchHandler_InvalidJSON(t *testing.T) {
	deps := &Dependencies{
		DB:        &mockDB{healthy: true},
		NATS:      &mockNATS{healthy: true},
		StartTime: time.Now(),
	}

	req := httptest.NewRequest(http.MethodPost, "/api/search", bytes.NewBufferString("not json"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	deps.SearchHandler(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestSearchHandler_NoAuth(t *testing.T) {
	deps := &Dependencies{
		DB:        &mockDB{healthy: true},
		NATS:      &mockNATS{healthy: true},
		StartTime: time.Now(),
		AuthToken: "secret",
	}

	body := `{"query": "test"}`
	req := httptest.NewRequest(http.MethodPost, "/api/search", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	deps.SearchHandler(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestSearchHandler_NoEngine(t *testing.T) {
	deps := &Dependencies{
		DB:           &mockDB{healthy: true},
		NATS:         &mockNATS{healthy: true},
		StartTime:    time.Now(),
		SearchEngine: nil,
	}

	body := `{"query": "test"}`
	req := httptest.NewRequest(http.MethodPost, "/api/search", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	deps.SearchHandler(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

func TestDigestHandler_NoAuth(t *testing.T) {
	deps := &Dependencies{
		DB:        &mockDB{healthy: true},
		NATS:      &mockNATS{healthy: true},
		StartTime: time.Now(),
		AuthToken: "secret",
	}

	req := httptest.NewRequest(http.MethodGet, "/api/digest", nil)
	rec := httptest.NewRecorder()

	deps.DigestHandler(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestDigestHandler_NoGenerator(t *testing.T) {
	deps := &Dependencies{
		DB:        &mockDB{healthy: true},
		NATS:      &mockNATS{healthy: true},
		StartTime: time.Now(),
		DigestGen: nil,
	}

	req := httptest.NewRequest(http.MethodGet, "/api/digest", nil)
	rec := httptest.NewRecorder()

	deps.DigestHandler(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

func TestSearchRequest_JSON(t *testing.T) {
	body := `{"query": "pricing strategy", "limit": 5, "filters": {"type": "video", "person": "Sarah"}}`
	var req SearchRequest
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if req.Query != "pricing strategy" {
		t.Errorf("expected 'pricing strategy', got %q", req.Query)
	}
	if req.Limit != 5 {
		t.Errorf("expected limit 5, got %d", req.Limit)
	}
	if req.Filters.Type != "video" {
		t.Errorf("expected type 'video', got %q", req.Filters.Type)
	}
	if req.Filters.Person != "Sarah" {
		t.Errorf("expected person 'Sarah', got %q", req.Filters.Person)
	}
}

func TestSearchResponse_EmptyResults(t *testing.T) {
	resp := SearchResponse{
		Results:         []SearchResult{},
		TotalCandidates: 0,
		SearchTimeMs:    50,
		Message:         "I don't have anything about that yet",
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded SearchResponse
	json.Unmarshal(data, &decoded)

	if len(decoded.Results) != 0 {
		t.Errorf("expected 0 results, got %d", len(decoded.Results))
	}
	if decoded.Message == "" {
		t.Error("expected non-empty message for empty results")
	}
}

func TestSearchResult_JSON(t *testing.T) {
	result := SearchResult{
		ArtifactID:   "art-1",
		Title:        "SaaS Pricing Video",
		ArtifactType: "video",
		Summary:      "A video about pricing",
		SourceURL:    "https://youtube.com/watch?v=test",
		Relevance:    "high",
		Explanation:  "Matches 'pricing video'",
		CreatedAt:    "2026-04-01T10:00:00Z",
		Topics:       []string{"pricing", "saas"},
		Connections:  3,
	}

	data, _ := json.Marshal(result)
	var decoded SearchResult
	json.Unmarshal(data, &decoded)

	if decoded.Relevance != "high" {
		t.Errorf("expected high relevance, got %q", decoded.Relevance)
	}
	if len(decoded.Topics) != 2 {
		t.Errorf("expected 2 topics, got %d", len(decoded.Topics))
	}
}

func TestErrorResponse_JSON(t *testing.T) {
	resp := ErrorResponse{
		Error: ErrorDetail{
			Code:               "DUPLICATE_DETECTED",
			Message:            "Already saved",
			ExistingArtifactID: "art-123",
			Title:              "Existing Article",
		},
	}

	data, _ := json.Marshal(resp)
	var decoded ErrorResponse
	json.Unmarshal(data, &decoded)

	if decoded.Error.Code != "DUPLICATE_DETECTED" {
		t.Errorf("expected DUPLICATE_DETECTED, got %q", decoded.Error.Code)
	}
	if decoded.Error.ExistingArtifactID != "art-123" {
		t.Errorf("expected art-123, got %q", decoded.Error.ExistingArtifactID)
	}
}

func TestWriteError_Status(t *testing.T) {
	rec := httptest.NewRecorder()
	writeError(rec, http.StatusBadRequest, "TEST_ERROR", "test message")

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
	if rec.Header().Get("Content-Type") != "application/json" {
		t.Error("expected JSON content type")
	}
}

func TestWriteJSON_ContentType(t *testing.T) {
	rec := httptest.NewRecorder()
	writeJSON(rec, http.StatusOK, map[string]string{"hello": "world"})

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if rec.Header().Get("Content-Type") != "application/json" {
		t.Error("expected JSON content type")
	}
}

func TestCaptureRequest_JSON(t *testing.T) {
	tests := []struct {
		name string
		body string
		url  string
		text string
	}{
		{"url only", `{"url": "https://example.com"}`, "https://example.com", ""},
		{"text only", `{"text": "an idea"}`, "", "an idea"},
		{"with context", `{"text": "note", "context": "from Sarah"}`, "", "note"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req CaptureRequest
			if err := json.Unmarshal([]byte(tt.body), &req); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if req.URL != tt.url {
				t.Errorf("expected url %q, got %q", tt.url, req.URL)
			}
			if req.Text != tt.text {
				t.Errorf("expected text %q, got %q", tt.text, req.Text)
			}
		})
	}
}

func TestCaptureResponse_JSON(t *testing.T) {
	resp := CaptureResponse{
		ArtifactID:   "cap-1",
		Title:        "Test",
		ArtifactType: "article",
		Summary:      "summary",
		Connections:  2,
		Topics:       []string{"tech"},
		ProcessingMs: 100,
	}

	data, _ := json.Marshal(resp)
	var decoded CaptureResponse
	json.Unmarshal(data, &decoded)

	if decoded.ArtifactID != "cap-1" {
		t.Errorf("unexpected artifact_id: %q", decoded.ArtifactID)
	}
	if decoded.ProcessingMs != 100 {
		t.Errorf("expected 100ms, got %d", decoded.ProcessingMs)
	}
}
