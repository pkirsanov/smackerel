package graphapi

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"
)

// TestClampLimit_RejectsAboveMax — SCN-080-15 regression: a caller
// asking for ?limit=10000 against a ListMax of 200 gets
// ErrLimitExceeded, not a silent clamp.
func TestClampLimit_RejectsAboveMax(t *testing.T) {
	l := Limits{ListDefault: 50, ListMax: 200, EdgesDefault: 100, EdgesMax: 500}
	_, err := l.ClampLimit(10000)
	if !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("ClampLimit(10000) err = %v; want ErrLimitExceeded", err)
	}
}

// TestClampLimit_DefaultsWhenAbsent — caller-omitted limit yields the
// configured default, not 0.
func TestClampLimit_DefaultsWhenAbsent(t *testing.T) {
	l := Limits{ListDefault: 50, ListMax: 200, EdgesDefault: 100, EdgesMax: 500}
	got, err := l.ClampLimit(0)
	if err != nil {
		t.Fatalf("ClampLimit(0) err = %v", err)
	}
	if got != 50 {
		t.Errorf("ClampLimit(0) = %d; want 50 (ListDefault)", got)
	}
}

// TestClampLimit_PassesThroughInRange — values in (0, ListMax] are
// returned unchanged.
func TestClampLimit_PassesThroughInRange(t *testing.T) {
	l := Limits{ListDefault: 50, ListMax: 200, EdgesDefault: 100, EdgesMax: 500}
	for _, req := range []int{1, 50, 199, 200} {
		got, err := l.ClampLimit(req)
		if err != nil {
			t.Fatalf("ClampLimit(%d) err = %v", req, err)
		}
		if got != req {
			t.Errorf("ClampLimit(%d) = %d; want %d", req, got, req)
		}
	}
}

// TestClampLimit_ZeroValueLimitsFailsLoud — a Limits zero value
// signals a wiring bug (SST loader skipped); ClampLimit must reject
// rather than return an invented default.
func TestClampLimit_ZeroValueLimitsFailsLoud(t *testing.T) {
	var l Limits
	if _, err := l.ClampLimit(10); !errors.Is(err, ErrLimitExceeded) {
		t.Errorf("zero-value Limits.ClampLimit err = %v; want ErrLimitExceeded", err)
	}
}

// TestClampEdgesLimit_RejectsAboveMax — same contract for the edges
// endpoint.
func TestClampEdgesLimit_RejectsAboveMax(t *testing.T) {
	l := Limits{EdgesDefault: 100, EdgesMax: 500, ListDefault: 1, ListMax: 1}
	if _, err := l.ClampEdgesLimit(10000); !errors.Is(err, ErrLimitExceeded) {
		t.Errorf("ClampEdgesLimit(10000) err = %v; want ErrLimitExceeded", err)
	}
}

// TestWriteAPIError_LimitExceededEnvelope — adversarial SCN-080-15:
// the HTTP body emitted for a too-large limit must be the uniform
// envelope with code=limit_exceeded and field=limit so the PWA can
// surface a precise error.
func TestWriteAPIError_LimitExceededEnvelope(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteAPIError(rec, ErrLimitExceeded)
	if rec.Code != 400 {
		t.Errorf("status = %d; want 400", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Errorf("content-type = %q", got)
	}
	var env ErrorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v (body=%q)", err, rec.Body.String())
	}
	if env.Error.Code != CodeLimitExceeded {
		t.Errorf("code = %q; want %q", env.Error.Code, CodeLimitExceeded)
	}
	if env.Error.Field != "limit" {
		t.Errorf("field = %q; want %q", env.Error.Field, "limit")
	}
}
