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

	body := `{"url": "https://example.com/article"}`
	req := httptest.NewRequest(http.MethodPost, "/api/capture", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	// No Authorization header
	rec := httptest.NewRecorder()

	deps.CaptureHandler(rec, req)

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

func TestCheckAuth_NoTokenConfigured(t *testing.T) {
	deps := &Dependencies{AuthToken: ""}
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	if !deps.checkAuth(req) {
		t.Error("checkAuth should pass when no token configured")
	}
}

func TestCheckAuth_ValidToken(t *testing.T) {
	deps := &Dependencies{AuthToken: "my-secret"}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer my-secret")

	if !deps.checkAuth(req) {
		t.Error("checkAuth should pass with valid token")
	}
}

func TestCheckAuth_InvalidToken(t *testing.T) {
	deps := &Dependencies{AuthToken: "my-secret"}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")

	if deps.checkAuth(req) {
		t.Error("checkAuth should fail with invalid token")
	}
}

func TestCheckAuth_MissingHeader(t *testing.T) {
	deps := &Dependencies{AuthToken: "my-secret"}
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	if deps.checkAuth(req) {
		t.Error("checkAuth should fail with missing header")
	}
}
