package graphapi

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

// TestWriteError_EmitsUniformEnvelope — design.md §8: every spec 080
// error response carries {"error":{"code","message","field"}}.
func TestWriteError_EmitsUniformEnvelope(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteError(rec, 400, CodeInvalidCursor, "cursor", "cursor is malformed or signature does not verify")
	if rec.Code != 400 {
		t.Fatalf("status = %d; want 400", rec.Code)
	}
	var env ErrorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v (body=%q)", err, rec.Body.String())
	}
	if env.Error.Code != CodeInvalidCursor {
		t.Errorf("code = %q; want %q", env.Error.Code, CodeInvalidCursor)
	}
	if env.Error.Field != "cursor" {
		t.Errorf("field = %q; want %q", env.Error.Field, "cursor")
	}
}

// TestWriteAPIError_Unauthenticated — SCN-080-09 adversarial: the 401
// envelope must carry code=unauthenticated and a non-empty message
// (so the caller learns why) but no graph data.
func TestWriteAPIError_Unauthenticated(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteAPIError(rec, ErrUnauthenticated)
	if rec.Code != 401 {
		t.Fatalf("status = %d; want 401", rec.Code)
	}
	var env ErrorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if env.Error.Code != CodeUnauthenticated {
		t.Errorf("code = %q; want %q", env.Error.Code, CodeUnauthenticated)
	}
	if env.Error.Message == "" {
		t.Error("message is empty; SCN-080-09 requires a non-empty reason string")
	}
	// Adversarial: response body must NOT leak topic/people/places data.
	body := rec.Body.String()
	for _, leak := range []string{"topic", "people", "places", "graph"} {
		if containsCaseInsensitive(body, leak) {
			t.Errorf("401 body leaked %q-like content: %q", leak, body)
		}
	}
}

// TestWriteAPIError_MissingScope — SCN-080-10 adversarial: bearer
// present but missing knowledge-graph:read scope yields 403/forbidden.
func TestWriteAPIError_MissingScope(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteAPIError(rec, ErrMissingScope)
	if rec.Code != 403 {
		t.Fatalf("status = %d; want 403", rec.Code)
	}
	var env ErrorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if env.Error.Code != CodeForbidden {
		t.Errorf("code = %q; want %q", env.Error.Code, CodeForbidden)
	}
}

// TestWriteAPIError_MalformedCursor — SCN-080-11 adversarial: the
// field locator must point at "cursor" so the PWA can highlight the
// offending query parameter.
func TestWriteAPIError_MalformedCursor(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteAPIError(rec, ErrMalformedCursor)
	if rec.Code != 400 {
		t.Fatalf("status = %d; want 400", rec.Code)
	}
	var env ErrorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if env.Error.Code != CodeInvalidCursor {
		t.Errorf("code = %q; want %q", env.Error.Code, CodeInvalidCursor)
	}
	if env.Error.Field != "cursor" {
		t.Errorf("field = %q; want %q", env.Error.Field, "cursor")
	}
}

// TestAPIError_WithFieldDoesNotMutateSingleton — sanity: cloning via
// WithField must not corrupt the package-level singletons that other
// goroutines might use concurrently.
func TestAPIError_WithFieldDoesNotMutateSingleton(t *testing.T) {
	originalField := ErrMissingParam.Field
	clone := ErrMissingParam.WithField("topicId")
	if ErrMissingParam.Field != originalField {
		t.Errorf("singleton Field mutated: was %q, now %q", originalField, ErrMissingParam.Field)
	}
	if clone.Field != "topicId" {
		t.Errorf("clone Field = %q; want %q", clone.Field, "topicId")
	}
}

func containsCaseInsensitive(haystack, needle string) bool {
	hl := []byte(haystack)
	for i := range hl {
		if hl[i] >= 'A' && hl[i] <= 'Z' {
			hl[i] += 'a' - 'A'
		}
	}
	nl := []byte(needle)
	for i := range nl {
		if nl[i] >= 'A' && nl[i] <= 'Z' {
			nl[i] += 'a' - 'A'
		}
	}
	return indexOf(string(hl), string(nl)) >= 0
}

func indexOf(s, sub string) int {
	if sub == "" {
		return 0
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
