// Spec 061 design §18.3 — provider constructor injection seam unit
// tests. Every external HTTP provider exposes ALL upstream URLs as
// constructor inputs. Empty values panic at construction so a
// misconfigured env file is caught at startup, not at first request.
//
// Companion architecture invariant: provider constructors MUST NOT
// instantiate *URL fields from `http://` / `https://` string literals.
// The architecture-test fixture in
// internal/assistant/contracts/architecture_test.go enforces that
// invariant repo-wide.

package weather

import (
	"net/http"
	"strings"
	"testing"
)

func TestNewOpenMeteoProvider_PanicsOnEmptyGeocodeURL(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("expected panic on empty geocodeURL, got none")
		}
		msg, ok := r.(string)
		if !ok {
			t.Fatalf("expected string panic, got %T: %v", r, r)
		}
		if !strings.Contains(msg, "geocodeURL must not be empty") {
			t.Fatalf("panic message must mention geocodeURL must not be empty (spec 061 §18.3); got: %s", msg)
		}
	}()
	_ = NewOpenMeteoProvider(&http.Client{}, "", "http://forecast.example/v1")
}

func TestNewOpenMeteoProvider_PanicsOnEmptyForecastURL(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("expected panic on empty forecastURL, got none")
		}
		msg, ok := r.(string)
		if !ok {
			t.Fatalf("expected string panic, got %T: %v", r, r)
		}
		if !strings.Contains(msg, "forecastURL must not be empty") {
			t.Fatalf("panic message must mention forecastURL must not be empty (spec 061 §18.3); got: %s", msg)
		}
	}()
	_ = NewOpenMeteoProvider(&http.Client{}, "http://geocode.example/v1", "")
}

func TestNewOpenMeteoProvider_AcceptsValidURLs(t *testing.T) {
	p := NewOpenMeteoProvider(
		&http.Client{},
		"http://stub-providers:8080/v1/search",
		"http://stub-providers:8080/v1/forecast",
	)
	if p == nil {
		t.Fatalf("constructor returned nil")
	}
	if p.geocodeURL != "http://stub-providers:8080/v1/search" {
		t.Fatalf("geocodeURL not stored verbatim; got %q", p.geocodeURL)
	}
	if p.forecastURL != "http://stub-providers:8080/v1/forecast" {
		t.Fatalf("forecastURL not stored verbatim; got %q", p.forecastURL)
	}
}
