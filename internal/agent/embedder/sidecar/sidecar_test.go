package sidecar

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNew_Validation(t *testing.T) {
	cases := []struct {
		name, baseURL, token string
		timeout              time.Duration
		wantErrSubstr        string
	}{
		{"empty_baseURL", "", "tok", time.Second, "baseURL"},
		{"whitespace_baseURL", "   ", "tok", time.Second, "baseURL"},
		{"empty_token", "http://x", "", time.Second, "authToken"},
		{"zero_timeout", "http://x", "tok", 0, "timeout"},
		{"negative_timeout", "http://x", "tok", -1, "timeout"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := New(tc.baseURL, tc.token, tc.timeout)
			if err == nil || !strings.Contains(err.Error(), tc.wantErrSubstr) {
				t.Fatalf("want error containing %q, got %v", tc.wantErrSubstr, err)
			}
		})
	}
}

func TestEmbed_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/embed" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method %s", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("auth header = %q", got)
		}
		var req embedRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Text != "hello" {
			t.Errorf("text = %q", req.Text)
		}
		_ = json.NewEncoder(w).Encode(embedResponse{
			Vector: []float32{0.1, 0.2, 0.3},
			Dim:    3,
			Model:  "stub",
		})
	}))
	defer srv.Close()

	e, err := New(srv.URL, "test-token", time.Second)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	vec, err := e.Embed(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(vec) != 3 || vec[0] != 0.1 {
		t.Errorf("unexpected vector %v", vec)
	}
}

func TestEmbed_NonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()
	e, _ := New(srv.URL, "tok", time.Second)
	_, err := e.Embed(context.Background(), "x")
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("want 401 error, got %v", err)
	}
}

func TestEmbed_EmptyVector(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(embedResponse{Vector: []float32{}, Dim: 0})
	}))
	defer srv.Close()
	e, _ := New(srv.URL, "tok", time.Second)
	_, err := e.Embed(context.Background(), "x")
	if err == nil || !strings.Contains(err.Error(), "empty vector") {
		t.Fatalf("want empty-vector error, got %v", err)
	}
}

func TestEmbed_DimMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(embedResponse{Vector: []float32{1, 2, 3}, Dim: 5})
	}))
	defer srv.Close()
	e, _ := New(srv.URL, "tok", time.Second)
	_, err := e.Embed(context.Background(), "x")
	if err == nil || !strings.Contains(err.Error(), "dim") {
		t.Fatalf("want dim-mismatch error, got %v", err)
	}
}
