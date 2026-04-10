package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

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
