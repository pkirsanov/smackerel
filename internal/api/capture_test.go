package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/smackerel/smackerel/internal/db"
)

// mockArtifactStore implements ArtifactQuerier for testing.
type mockArtifactStore struct {
	recentItems []db.RecentArtifact
	recentErr   error
	artifact    *db.ArtifactDetail
	artifactErr error
	exportRes   *db.ExportResult
	exportErr   error
}

func (m *mockArtifactStore) RecentArtifacts(_ context.Context, limit int) ([]db.RecentArtifact, error) {
	if m.recentErr != nil {
		return nil, m.recentErr
	}
	return m.recentItems, nil
}

func (m *mockArtifactStore) GetArtifact(_ context.Context, id string) (*db.ArtifactDetail, error) {
	if m.artifactErr != nil {
		return nil, m.artifactErr
	}
	return m.artifact, nil
}

func (m *mockArtifactStore) ExportArtifacts(_ context.Context, cursor time.Time, limit int) (*db.ExportResult, error) {
	if m.exportErr != nil {
		return nil, m.exportErr
	}
	return m.exportRes, nil
}

func TestCaptureHandler_EmptyBody(t *testing.T) {
	deps := &Dependencies{
		DB:        &mockDB{healthy: true},
		NATS:      &mockNATS{healthy: true},
		StartTime: time.Now(),
	}

	body := `{}`
	req := httptest.NewRequest(http.MethodPost, "/api/capture", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	deps.CaptureHandler(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}

	var resp ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}

	if resp.Error.Code != "INVALID_INPUT" {
		t.Errorf("expected error code INVALID_INPUT, got %q", resp.Error.Code)
	}
}

func TestCaptureHandler_InvalidJSON(t *testing.T) {
	deps := &Dependencies{
		DB:        &mockDB{healthy: true},
		NATS:      &mockNATS{healthy: true},
		StartTime: time.Now(),
	}

	req := httptest.NewRequest(http.MethodPost, "/api/capture", bytes.NewBufferString("not json"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	deps.CaptureHandler(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestCaptureHandler_NoPipeline(t *testing.T) {
	deps := &Dependencies{
		DB:        &mockDB{healthy: true},
		NATS:      &mockNATS{healthy: true},
		StartTime: time.Now(),
		Pipeline:  nil, // No pipeline configured
	}

	body := `{"url": "https://example.com/article"}`
	req := httptest.NewRequest(http.MethodPost, "/api/capture", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	deps.CaptureHandler(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}

	var resp ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}

	if resp.Error.Code != "ML_UNAVAILABLE" {
		t.Errorf("expected error code ML_UNAVAILABLE, got %q", resp.Error.Code)
	}
}

func TestCaptureHandler_AuthRequired(t *testing.T) {
	deps := &Dependencies{
		DB:        &mockDB{healthy: true},
		NATS:      &mockNATS{healthy: true},
		StartTime: time.Now(),
		AuthToken: "test-secret-token",
	}

	router := NewRouter(deps)
	body := `{"url": "https://example.com/article"}`
	req := httptest.NewRequest(http.MethodPost, "/api/capture", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	// No Authorization header
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestCaptureHandler_AuthCorrectToken(t *testing.T) {
	deps := &Dependencies{
		DB:        &mockDB{healthy: true},
		NATS:      &mockNATS{healthy: true},
		StartTime: time.Now(),
		AuthToken: "test-secret-token",
		Pipeline:  nil, // Will hit 503 for ML_UNAVAILABLE
	}

	body := `{"url": "https://example.com/article"}`
	req := httptest.NewRequest(http.MethodPost, "/api/capture", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-secret-token")
	rec := httptest.NewRecorder()

	deps.CaptureHandler(rec, req)

	// Should pass auth and get 503 (no pipeline) rather than 401
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 (past auth, no pipeline), got %d", rec.Code)
	}
}

func TestCaptureHandler_DBUnavailable_Returns503(t *testing.T) {
	deps := &Dependencies{
		DB:        &mockDB{healthy: false},
		NATS:      &mockNATS{healthy: true},
		StartTime: time.Now(),
		Pipeline:  nil,
	}

	body := `{"url": "https://example.com/article"}`
	req := httptest.NewRequest(http.MethodPost, "/api/capture", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	deps.CaptureHandler(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for DB unavailable, got %d", rec.Code)
	}

	var resp ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if resp.Error.Code != "DB_UNAVAILABLE" {
		t.Errorf("expected error code DB_UNAVAILABLE, got %q", resp.Error.Code)
	}
}

func TestCaptureHandler_DBHealthy_ContinuesProcessing(t *testing.T) {
	deps := &Dependencies{
		DB:        &mockDB{healthy: true},
		NATS:      &mockNATS{healthy: true},
		StartTime: time.Now(),
		Pipeline:  nil, // Will hit ML_UNAVAILABLE after passing DB check
	}

	body := `{"url": "https://example.com/article"}`
	req := httptest.NewRequest(http.MethodPost, "/api/capture", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	deps.CaptureHandler(rec, req)

	// DB is healthy, so it passes DB check and hits ML_UNAVAILABLE (no pipeline)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 ML_UNAVAILABLE (past DB check, no pipeline), got %d", rec.Code)
	}
	var resp ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if resp.Error.Code != "ML_UNAVAILABLE" {
		t.Errorf("expected ML_UNAVAILABLE (not DB_UNAVAILABLE), got %q", resp.Error.Code)
	}
}

func TestCaptureHandler_OversizedBody(t *testing.T) {
	deps := &Dependencies{
		DB:        &mockDB{healthy: true},
		NATS:      &mockNATS{healthy: true},
		StartTime: time.Now(),
		Pipeline:  nil,
	}

	// Create body larger than 1MB limit
	bigBody := bytes.Repeat([]byte("x"), 2<<20)
	req := httptest.NewRequest(http.MethodPost, "/api/capture", bytes.NewReader(bigBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	deps.CaptureHandler(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for oversized body, got %d", rec.Code)
	}
}

func TestCaptureHandler_NilDB_Returns503(t *testing.T) {
	deps := &Dependencies{
		DB:        nil, // DB not configured — must not bypass health gate
		NATS:      &mockNATS{healthy: true},
		StartTime: time.Now(),
		Pipeline:  nil,
	}

	body := `{"url": "https://example.com/article"}`
	req := httptest.NewRequest(http.MethodPost, "/api/capture", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	deps.CaptureHandler(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for nil DB, got %d", rec.Code)
	}

	var resp ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if resp.Error.Code != "DB_UNAVAILABLE" {
		t.Errorf("expected DB_UNAVAILABLE, got %q", resp.Error.Code)
	}
}

func TestCaptureHandler_TextOnly(t *testing.T) {
	deps := &Dependencies{
		DB:        &mockDB{healthy: true},
		NATS:      &mockNATS{healthy: true},
		StartTime: time.Now(),
		Pipeline:  nil,
	}

	body := `{"text": "my quick note about pricing"}`
	req := httptest.NewRequest(http.MethodPost, "/api/capture", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	deps.CaptureHandler(rec, req)

	// No pipeline → 503, but should pass input validation
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 (past validation, no pipeline), got %d", rec.Code)
	}
}

func TestCaptureHandler_VoiceURLOnly(t *testing.T) {
	deps := &Dependencies{
		DB:        &mockDB{healthy: true},
		NATS:      &mockNATS{healthy: true},
		StartTime: time.Now(),
		Pipeline:  nil,
	}

	body := `{"voice_url": "https://example.com/audio.ogg"}`
	req := httptest.NewRequest(http.MethodPost, "/api/capture", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	deps.CaptureHandler(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 (past validation, no pipeline), got %d", rec.Code)
	}
}

// === SCN-023-02: RecentHandler uses typed ArtifactQuerier (no type assertions) ===

func TestRecentHandler_NilArtifactStore_Returns503(t *testing.T) {
	deps := &Dependencies{
		DB:            &mockDB{healthy: true},
		NATS:          &mockNATS{healthy: true},
		StartTime:     time.Now(),
		ArtifactStore: nil,
	}

	req := httptest.NewRequest(http.MethodGet, "/api/recent", nil)
	rec := httptest.NewRecorder()

	deps.RecentHandler(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for nil ArtifactStore, got %d", rec.Code)
	}

	var resp ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if resp.Error.Code != "DB_UNAVAILABLE" {
		t.Errorf("expected DB_UNAVAILABLE, got %q", resp.Error.Code)
	}
}

func TestRecentHandler_Success(t *testing.T) {
	now := time.Now()
	store := &mockArtifactStore{
		recentItems: []db.RecentArtifact{
			{ID: "a1", Title: "First", ArtifactType: "article", Summary: "Summary 1", CreatedAt: now},
			{ID: "a2", Title: "Second", ArtifactType: "note", Summary: "Summary 2", CreatedAt: now.Add(-time.Hour)},
		},
	}
	deps := &Dependencies{
		DB:            &mockDB{healthy: true},
		NATS:          &mockNATS{healthy: true},
		StartTime:     time.Now(),
		ArtifactStore: store,
	}

	req := httptest.NewRequest(http.MethodGet, "/api/recent", nil)
	rec := httptest.NewRecorder()

	deps.RecentHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", ct)
	}

	var body map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if _, ok := body["results"]; !ok {
		t.Error("expected 'results' key in response")
	}
}

func TestRecentHandler_QueryError(t *testing.T) {
	store := &mockArtifactStore{
		recentErr: fmt.Errorf("connection refused"),
	}
	deps := &Dependencies{
		DB:            &mockDB{healthy: true},
		NATS:          &mockNATS{healthy: true},
		StartTime:     time.Now(),
		ArtifactStore: store,
	}

	req := httptest.NewRequest(http.MethodGet, "/api/recent", nil)
	rec := httptest.NewRecorder()

	deps.RecentHandler(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

func TestRecentHandler_LimitCapped(t *testing.T) {
	store := &mockArtifactStore{recentItems: []db.RecentArtifact{}}
	deps := &Dependencies{
		DB:            &mockDB{healthy: true},
		NATS:          &mockNATS{healthy: true},
		StartTime:     time.Now(),
		ArtifactStore: store,
	}

	// Limit > 50 should be capped to 50 — handler still returns 200
	req := httptest.NewRequest(http.MethodGet, "/api/recent?limit=999", nil)
	rec := httptest.NewRecorder()

	deps.RecentHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

// === SCN-023-02: ArtifactDetailHandler uses typed ArtifactQuerier ===

func TestArtifactDetailHandler_NilArtifactStore_Returns503(t *testing.T) {
	deps := &Dependencies{
		DB:            &mockDB{healthy: true},
		NATS:          &mockNATS{healthy: true},
		StartTime:     time.Now(),
		ArtifactStore: nil,
	}

	// Need Chi router context for URL params
	r := chi.NewRouter()
	r.Get("/api/artifact/{id}", deps.ArtifactDetailHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/artifact/test-id", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for nil ArtifactStore, got %d", rec.Code)
	}
}

func TestArtifactDetailHandler_Success(t *testing.T) {
	now := time.Now()
	store := &mockArtifactStore{
		artifact: &db.ArtifactDetail{
			ID:             "art-123",
			Title:          "Test Article",
			ArtifactType:   "article",
			Summary:        "A test summary",
			SourceURL:      "https://example.com",
			Sentiment:      "neutral",
			SourceQuality:  "high",
			ProcessingTier: "full",
			CreatedAt:      now,
			UpdatedAt:      now,
		},
	}
	deps := &Dependencies{
		DB:            &mockDB{healthy: true},
		NATS:          &mockNATS{healthy: true},
		StartTime:     time.Now(),
		ArtifactStore: store,
	}

	r := chi.NewRouter()
	r.Get("/api/artifact/{id}", deps.ArtifactDetailHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/artifact/art-123", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if body["artifact_id"] != "art-123" {
		t.Errorf("expected artifact_id=art-123, got %v", body["artifact_id"])
	}
	if body["title"] != "Test Article" {
		t.Errorf("expected title=Test Article, got %v", body["title"])
	}
}

func TestArtifactDetailHandler_NotFound(t *testing.T) {
	store := &mockArtifactStore{
		artifactErr: fmt.Errorf("get artifact: no rows in result set"),
	}
	deps := &Dependencies{
		DB:            &mockDB{healthy: true},
		NATS:          &mockNATS{healthy: true},
		StartTime:     time.Now(),
		ArtifactStore: store,
	}

	r := chi.NewRouter()
	r.Get("/api/artifact/{id}", deps.ArtifactDetailHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/artifact/nonexistent", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}

	var resp ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if resp.Error.Code != "NOT_FOUND" {
		t.Errorf("expected NOT_FOUND, got %q", resp.Error.Code)
	}
}

// === SCN-023-02: ExportHandler uses typed ArtifactQuerier ===

func TestExportHandler_NilArtifactStore_Returns503(t *testing.T) {
	deps := &Dependencies{
		DB:            &mockDB{healthy: true},
		NATS:          &mockNATS{healthy: true},
		StartTime:     time.Now(),
		ArtifactStore: nil,
	}

	req := httptest.NewRequest(http.MethodGet, "/api/export", nil)
	rec := httptest.NewRecorder()

	deps.ExportHandler(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for nil ArtifactStore, got %d", rec.Code)
	}
}

func TestExportHandler_Success(t *testing.T) {
	store := &mockArtifactStore{
		exportRes: &db.ExportResult{
			Artifacts: []db.ExportedArtifact{
				{ArtifactID: "e1", Title: "Exported", ArtifactType: "article"},
			},
			NextCursor: time.Now(),
		},
	}
	deps := &Dependencies{
		DB:            &mockDB{healthy: true},
		NATS:          &mockNATS{healthy: true},
		StartTime:     time.Now(),
		ArtifactStore: store,
	}

	req := httptest.NewRequest(http.MethodGet, "/api/export", nil)
	rec := httptest.NewRecorder()

	deps.ExportHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	ct := rec.Header().Get("Content-Type")
	if ct != "application/x-ndjson" {
		t.Errorf("expected Content-Type application/x-ndjson, got %s", ct)
	}

	cursor := rec.Header().Get("X-Next-Cursor")
	if cursor == "" {
		t.Error("expected X-Next-Cursor header when results exist")
	}
}

func TestExportHandler_InvalidCursor(t *testing.T) {
	store := &mockArtifactStore{exportRes: &db.ExportResult{}}
	deps := &Dependencies{
		DB:            &mockDB{healthy: true},
		NATS:          &mockNATS{healthy: true},
		StartTime:     time.Now(),
		ArtifactStore: store,
	}

	req := httptest.NewRequest(http.MethodGet, "/api/export?cursor=not-a-timestamp", nil)
	rec := httptest.NewRecorder()

	deps.ExportHandler(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid cursor, got %d", rec.Code)
	}
}

func TestExportHandler_QueryError(t *testing.T) {
	store := &mockArtifactStore{
		exportErr: fmt.Errorf("export query failed"),
	}
	deps := &Dependencies{
		DB:            &mockDB{healthy: true},
		NATS:          &mockNATS{healthy: true},
		StartTime:     time.Now(),
		ArtifactStore: store,
	}

	req := httptest.NewRequest(http.MethodGet, "/api/export", nil)
	rec := httptest.NewRecorder()

	deps.ExportHandler(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}
