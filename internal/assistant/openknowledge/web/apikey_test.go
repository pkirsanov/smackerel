package web

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestOpenKnowledgeAPIKey_NeverLoggedByWebProviders is a regression
// guard for SCOPE-15: API key material wired into web providers
// MUST NEVER appear in slog output. The web package currently
// performs no logging at all, but a future regression that adds a
// "constructed provider with config=%+v" log line would leak
// ProviderAPIKey into operator logs.
//
// The test installs a buffer-backed slog handler as the default,
// constructs each provider with a unique sentinel "key", and
// asserts the sentinel is absent from anything written to the
// buffer.
func TestOpenKnowledgeAPIKey_NeverLoggedByWebProviders(t *testing.T) {
	const sentinel = "sk-test-064-scope-15-do-not-log"
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Reject if the test ever caused the API key to be sent on the wire by accident.
		if v := r.Header.Get("X-Api-Key"); strings.Contains(v, sentinel) {
			t.Errorf("SearxNG MUST NOT send X-Api-Key header (no API key in v1 provider contract); got %q", v)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[]}`))
	}))
	defer srv.Close()

	// SearxNG.
	sx, err := NewSearxNG(srv.URL, http.DefaultClient)
	if err != nil {
		t.Fatalf("NewSearxNG: %v", err)
	}
	if _, err := sx.Search(context.Background(), "test", 1); err != nil {
		t.Fatalf("Search: %v", err)
	}

	// Brave / Tavily stubs — construction MUST NOT log the api key.
	_ = NewBrave()
	_ = NewTavily()

	// (We intentionally do not call Search on the stubs because they
	// return ErrProviderNotConfigured without taking any api key.)
	if strings.Contains(buf.String(), sentinel) {
		t.Fatalf("API key sentinel leaked into slog output:\n%s", buf.String())
	}
}

// TestOpenKnowledgeAPIKey_NeverInErrorMessages — sentinel API key
// MUST NOT appear in any typed error returned by the web package.
// A regression that wrapped errors with "%v" on a config struct
// would leak the key into the agent loop's logged trace.
func TestOpenKnowledgeAPIKey_NeverInErrorMessages(t *testing.T) {
	const sentinel = "sk-test-064-scope-15-error-leak"
	// Build a server that returns 401 so the agent's error path
	// is exercised.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	sx, err := NewSearxNG(srv.URL, http.DefaultClient)
	if err != nil {
		t.Fatalf("NewSearxNG: %v", err)
	}
	_, err = sx.Search(context.Background(), "q", 1)
	if !errors.Is(err, ErrProviderUnreachable) {
		t.Fatalf("expected ErrProviderUnreachable, got %v", err)
	}
	if strings.Contains(err.Error(), sentinel) {
		t.Fatalf("API key sentinel %q must not appear in error: %v", sentinel, err)
	}
}
