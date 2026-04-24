package weather

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// nwsActiveAlertsBody returns a minimal but realistic NWS GeoJSON
// FeatureCollection payload for use in handler responses.
const nwsTwoAlertBody = `{
  "features": [
    {
      "properties": {
        "id": "urn:oid:2.49.0.1.840.0.alpha-extreme",
        "event": "Tornado Warning",
        "severity": "Extreme",
        "headline": "Tornado Warning issued",
        "description": "Take shelter immediately",
        "instruction": "Move to interior room",
        "areaDesc": "Travis County, TX",
        "effective": "2026-04-24T12:00:00-05:00",
        "expires": "2026-04-24T13:00:00-05:00"
      }
    },
    {
      "properties": {
        "id": "urn:oid:2.49.0.1.840.0.beta-moderate",
        "event": "Flood Advisory",
        "severity": "Moderate",
        "headline": "Flood Advisory issued",
        "description": "Minor flooding expected",
        "instruction": "",
        "areaDesc": "Travis County, TX",
        "effective": "2026-04-24T12:00:00-05:00",
        "expires": "2026-04-24T18:00:00-05:00"
      }
    }
  ]
}`

const nwsEmptyBody = `{"features":[]}`

func TestNWSClient_FetchActiveAlerts_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.RawQuery, "point=30.2672,-97.7431") {
			t.Errorf("unexpected query: %q", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/geo+json")
		fmt.Fprint(w, nwsTwoAlertBody)
	}))
	defer srv.Close()

	c := NewNWSClient(srv.URL, "test-agent")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	alerts, err := c.FetchActiveAlerts(ctx, 30.2672, -97.7431)
	if err != nil {
		t.Fatalf("FetchActiveAlerts: %v", err)
	}
	if len(alerts) != 2 {
		t.Fatalf("got %d alerts, want 2", len(alerts))
	}
	if alerts[0].Severity != "Extreme" || alerts[0].Event != "Tornado Warning" {
		t.Errorf("alert[0] = %+v", alerts[0])
	}
	if alerts[0].Effective.IsZero() || alerts[0].Expires.IsZero() {
		t.Errorf("alert[0] effective/expires not parsed: %+v", alerts[0])
	}
	if alerts[1].Severity != "Moderate" {
		t.Errorf("alert[1] severity = %q, want Moderate", alerts[1].Severity)
	}
}

func TestNWSClient_FetchActiveAlerts_EmptyResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, nwsEmptyBody)
	}))
	defer srv.Close()

	c := NewNWSClient(srv.URL, "test-agent")
	alerts, err := c.FetchActiveAlerts(context.Background(), 30.0, -97.0)
	if err != nil {
		t.Fatalf("FetchActiveAlerts: %v", err)
	}
	if len(alerts) != 0 {
		t.Errorf("got %d alerts, want 0", len(alerts))
	}
}

func TestNWSClient_FetchActiveAlerts_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewNWSClient(srv.URL, "test-agent")
	_, err := c.FetchActiveAlerts(context.Background(), 30.0, -97.0)
	if err == nil {
		t.Fatal("expected error on 500 response, got nil")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error %q does not mention status code", err)
	}
}

func TestNWSClient_FetchActiveAlerts_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"features": [`) // truncated
	}))
	defer srv.Close()

	c := NewNWSClient(srv.URL, "test-agent")
	_, err := c.FetchActiveAlerts(context.Background(), 30.0, -97.0)
	if err == nil {
		t.Fatal("expected decode error, got nil")
	}
	if !strings.Contains(err.Error(), "decode") {
		t.Errorf("error %q should mention decode", err)
	}
}

func TestNWSClient_FetchActiveAlerts_RejectsInvalidLatitude(t *testing.T) {
	c := NewNWSClient("http://127.0.0.1:1", "test-agent")
	for _, bad := range []float64{-91, 91, 1000} {
		_, err := c.FetchActiveAlerts(context.Background(), bad, 0)
		if err == nil {
			t.Errorf("latitude %v: expected error, got nil", bad)
			continue
		}
		if !strings.Contains(err.Error(), "latitude") {
			t.Errorf("latitude %v: error %q should mention latitude", bad, err)
		}
	}
}

func TestNWSClient_FetchActiveAlerts_RejectsInvalidLongitude(t *testing.T) {
	c := NewNWSClient("http://127.0.0.1:1", "test-agent")
	for _, bad := range []float64{-181, 181, 9999} {
		_, err := c.FetchActiveAlerts(context.Background(), 0, bad)
		if err == nil {
			t.Errorf("longitude %v: expected error, got nil", bad)
			continue
		}
		if !strings.Contains(err.Error(), "longitude") {
			t.Errorf("longitude %v: error %q should mention longitude", bad, err)
		}
	}
}

func TestNWSClient_FetchActiveAlerts_UserAgentSet(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("User-Agent")
		fmt.Fprint(w, nwsEmptyBody)
	}))
	defer srv.Close()

	const wantUA = "smackerel-test/1.0 (test@example.invalid)"
	c := NewNWSClient(srv.URL, wantUA)
	if _, err := c.FetchActiveAlerts(context.Background(), 30.0, -97.0); err != nil {
		t.Fatalf("FetchActiveAlerts: %v", err)
	}
	if got != wantUA {
		t.Errorf("User-Agent = %q, want %q", got, wantUA)
	}

	// Default UA path uses package-level userAgent constant.
	c2 := NewNWSClient(srv.URL, "")
	if _, err := c2.FetchActiveAlerts(context.Background(), 30.0, -97.0); err != nil {
		t.Fatalf("FetchActiveAlerts (default UA): %v", err)
	}
	if got != userAgent {
		t.Errorf("default User-Agent = %q, want %q", got, userAgent)
	}
}

func TestMapCAPSeverityToTier_AllLevels(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"Extreme", "full"},
		{"Severe", "full"},
		{"Moderate", "standard"},
		{"Minor", "light"},
		{"Unknown", "light"},
		{"", "light"},
		{"Bogus", "light"},
	}
	for _, tc := range cases {
		if got := mapCAPSeverityToTier(tc.in); got != tc.want {
			t.Errorf("mapCAPSeverityToTier(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestNWSClient_FetchActiveAlerts_CacheReuse(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		fmt.Fprint(w, nwsTwoAlertBody)
	}))
	defer srv.Close()

	c := NewNWSClient(srv.URL, "test-agent")
	for i := 0; i < 3; i++ {
		alerts, err := c.FetchActiveAlerts(context.Background(), 30.2672, -97.7431)
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		if len(alerts) != 2 {
			t.Errorf("call %d: got %d alerts, want 2", i, len(alerts))
		}
	}
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Errorf("upstream hits = %d, want 1 (cache should serve calls 2 and 3)", got)
	}
}

func TestIsHighSeverity(t *testing.T) {
	for _, tc := range []struct {
		sev  string
		want bool
	}{
		{"Extreme", true},
		{"Severe", true},
		{"Moderate", false},
		{"Minor", false},
		{"Unknown", false},
		{"", false},
	} {
		if got := isHighSeverity(tc.sev); got != tc.want {
			t.Errorf("isHighSeverity(%q) = %v, want %v", tc.sev, got, tc.want)
		}
	}
}
