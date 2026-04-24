package weather

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestEnrich_ValidateRequest_RejectsInvalidJSON ensures a payload that is
// not valid JSON yields enrichErrInvalidPayload (and never panics).
func TestEnrich_ValidateRequest_RejectsInvalidJSON(t *testing.T) {
	_, errResp := validateEnrichRequest([]byte("{not-json"))
	if errResp == nil {
		t.Fatal("expected error response, got nil")
	}
	if errResp.Error != enrichErrInvalidPayload {
		t.Errorf("error = %q, want %q", errResp.Error, enrichErrInvalidPayload)
	}
}

// TestEnrich_ValidateRequest_RejectsMissingDate ensures empty date is
// rejected before any upstream call.
func TestEnrich_ValidateRequest_RejectsMissingDate(t *testing.T) {
	body, _ := json.Marshal(EnrichRequest{
		RequestID: "r1",
		Latitude:  37.77,
		Longitude: -122.42,
		Date:      "",
	})
	_, errResp := validateEnrichRequest(body)
	if errResp == nil || errResp.Error != enrichErrInvalidDate {
		t.Fatalf("expected %q error, got %+v", enrichErrInvalidDate, errResp)
	}
	if errResp.RequestID != "r1" {
		t.Errorf("request_id not echoed: got %q", errResp.RequestID)
	}
}

// TestEnrich_ValidateRequest_RejectsMalformedDate ensures only YYYY-MM-DD is
// accepted; other formats fail before any upstream call.
func TestEnrich_ValidateRequest_RejectsMalformedDate(t *testing.T) {
	body, _ := json.Marshal(EnrichRequest{
		RequestID: "r2",
		Latitude:  37.77,
		Longitude: -122.42,
		Date:      "03/15/2026",
	})
	_, errResp := validateEnrichRequest(body)
	if errResp == nil || errResp.Error != enrichErrInvalidDate {
		t.Fatalf("expected %q error, got %+v", enrichErrInvalidDate, errResp)
	}
}

// TestEnrich_ValidateRequest_RejectsLatitudeOutOfRange covers both the upper
// and lower latitude bounds in one table-driven test.
func TestEnrich_ValidateRequest_RejectsLatitudeOutOfRange(t *testing.T) {
	for _, lat := range []float64{-90.5, 91.0} {
		body, _ := json.Marshal(EnrichRequest{
			RequestID: "r",
			Latitude:  lat,
			Longitude: 0,
			Date:      "2026-03-15",
		})
		_, errResp := validateEnrichRequest(body)
		if errResp == nil || errResp.Error != enrichErrInvalidLatitude {
			t.Errorf("lat=%v: expected %q error, got %+v", lat, enrichErrInvalidLatitude, errResp)
		}
	}
}

// TestEnrich_ValidateRequest_RejectsLongitudeOutOfRange covers both bounds.
func TestEnrich_ValidateRequest_RejectsLongitudeOutOfRange(t *testing.T) {
	for _, lon := range []float64{-181.0, 180.5} {
		body, _ := json.Marshal(EnrichRequest{
			RequestID: "r",
			Latitude:  0,
			Longitude: lon,
			Date:      "2026-03-15",
		})
		_, errResp := validateEnrichRequest(body)
		if errResp == nil || errResp.Error != enrichErrInvalidLongitude {
			t.Errorf("lon=%v: expected %q error, got %+v", lon, enrichErrInvalidLongitude, errResp)
		}
	}
}

// TestEnrich_ValidateRequest_AcceptsValid ensures a well-formed payload
// passes validation and round-trips its fields.
func TestEnrich_ValidateRequest_AcceptsValid(t *testing.T) {
	body, _ := json.Marshal(EnrichRequest{
		RequestID: "ok",
		Latitude:  47.37,
		Longitude: 8.54,
		Date:      "2026-03-15",
	})
	req, errResp := validateEnrichRequest(body)
	if errResp != nil {
		t.Fatalf("unexpected error response: %+v", errResp)
	}
	if req.RequestID != "ok" || req.Date != "2026-03-15" {
		t.Errorf("fields not preserved: %+v", req)
	}
}

// TestEnrich_HandleRequest_SuccessAndShape exercises the happy path against
// an httptest archive backend and verifies the response shape, including
// request_id correlation and embedded weather data.
func TestEnrich_HandleRequest_SuccessAndShape(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"daily":{"time":["2026-03-15"],"temperature_2m_max":[14],"temperature_2m_min":[6],"weather_code":[65],"precipitation_sum":[8]}}`)
	}))
	defer srv.Close()

	c := New("weather")
	c.SetArchiveURL(srv.URL)
	c.config.Precision = 2

	body, _ := json.Marshal(EnrichRequest{
		RequestID: "req-success",
		Latitude:  47.37,
		Longitude: 8.54,
		Date:      "2026-03-15",
	})

	respBytes := handleEnrichmentRequest(context.Background(), c, body)
	var resp EnrichResponse
	if err := json.Unmarshal(respBytes, &resp); err != nil {
		t.Fatalf("response not valid JSON: %v", err)
	}
	if resp.RequestID != "req-success" {
		t.Errorf("request_id = %q, want %q", resp.RequestID, "req-success")
	}
	if resp.Error != "" {
		t.Errorf("unexpected error: %q", resp.Error)
	}
	if resp.Weather == nil {
		t.Fatal("weather field missing on success")
	}
	if resp.Weather.Temperature != 10.0 {
		t.Errorf("temperature = %v, want 10.0 (avg of 14/6)", resp.Weather.Temperature)
	}
	if resp.Weather.WeatherCode != 65 {
		t.Errorf("weather_code = %d, want 65", resp.Weather.WeatherCode)
	}
}

// TestEnrich_HandleRequest_CacheReuse verifies a second identical request
// is served from the in-memory cache and does not hit the upstream server
// again. This is the contract that "historical data is cached permanently".
func TestEnrich_HandleRequest_CacheReuse(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"daily":{"time":["2026-03-15"],"temperature_2m_max":[10],"temperature_2m_min":[2],"weather_code":[3],"precipitation_sum":[0]}}`)
	}))
	defer srv.Close()

	c := New("weather")
	c.SetArchiveURL(srv.URL)
	c.config.Precision = 2

	body, _ := json.Marshal(EnrichRequest{
		RequestID: "cache-1",
		Latitude:  47.37,
		Longitude: 8.54,
		Date:      "2026-03-15",
	})

	first := handleEnrichmentRequest(context.Background(), c, body)
	if hits != 1 {
		t.Fatalf("expected 1 upstream hit after first call, got %d", hits)
	}

	// Second call: same coords and date, different request_id — must hit cache.
	body2, _ := json.Marshal(EnrichRequest{
		RequestID: "cache-2",
		Latitude:  47.37,
		Longitude: 8.54,
		Date:      "2026-03-15",
	})
	second := handleEnrichmentRequest(context.Background(), c, body2)
	if hits != 1 {
		t.Errorf("expected upstream hit count to remain 1 after cache hit, got %d", hits)
	}

	var r1, r2 EnrichResponse
	if err := json.Unmarshal(first, &r1); err != nil {
		t.Fatalf("first response: %v", err)
	}
	if err := json.Unmarshal(second, &r2); err != nil {
		t.Fatalf("second response: %v", err)
	}
	if r1.Weather == nil || r2.Weather == nil {
		t.Fatal("weather missing on cached path")
	}
	if r1.Weather.Temperature != r2.Weather.Temperature {
		t.Errorf("cached payload diverged: %v vs %v", r1.Weather.Temperature, r2.Weather.Temperature)
	}
	if r2.RequestID != "cache-2" {
		t.Errorf("second request_id not echoed: got %q", r2.RequestID)
	}
}

// TestEnrich_HandleRequest_FetchErrorReturnsErrorResponse ensures a 4xx from
// the archive API surfaces as an enrichErrFetchFailed response rather than a
// dropped message. Covers the "publish error response" DoD requirement on
// the upstream-failure path.
func TestEnrich_HandleRequest_FetchErrorReturnsErrorResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad request", http.StatusBadRequest)
	}))
	defer srv.Close()

	c := New("weather")
	c.SetArchiveURL(srv.URL)
	c.config.Precision = 2

	body, _ := json.Marshal(EnrichRequest{
		RequestID: "err-1",
		Latitude:  47.37,
		Longitude: 8.54,
		Date:      "2026-03-15",
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	respBytes := handleEnrichmentRequest(ctx, c, body)
	var resp EnrichResponse
	if err := json.Unmarshal(respBytes, &resp); err != nil {
		t.Fatalf("response not valid JSON: %v", err)
	}
	if resp.Error != enrichErrFetchFailed {
		t.Errorf("error = %q, want %q", resp.Error, enrichErrFetchFailed)
	}
	if resp.Weather != nil {
		t.Errorf("weather should be nil on error, got %+v", resp.Weather)
	}
	if resp.RequestID != "err-1" {
		t.Errorf("request_id not echoed on error: %q", resp.RequestID)
	}
}

// TestEnrich_HandleRequest_InvalidPayloadReturnsErrorResponse is the
// adversarial regression for the DoD claim that "invalid request payloads
// produce an error response". Without the validation step the subscriber
// would attempt to fetchHistorical with a zero date and the test would
// observe a fetch_failed error from the archive call instead of the
// invalid_date error code, failing this assertion.
func TestEnrich_HandleRequest_InvalidPayloadReturnsErrorResponse(t *testing.T) {
	c := New("weather")
	// SetArchiveURL deliberately points at an unreachable host: if validation
	// failed open and a fetch were attempted, this test would either hang
	// on dial (no enrichFetchTimeout in validation path) or return
	// fetch_failed instead of invalid_date.
	c.SetArchiveURL("http://127.0.0.1:1")
	c.config.Precision = 2

	respBytes := handleEnrichmentRequest(context.Background(), c, []byte(`{"request_id":"bad","latitude":0,"longitude":0,"date":"yesterday"}`))
	var resp EnrichResponse
	if err := json.Unmarshal(respBytes, &resp); err != nil {
		t.Fatalf("response not valid JSON: %v", err)
	}
	if resp.Error != enrichErrInvalidDate {
		t.Errorf("error = %q, want %q (validation must run before fetch)", resp.Error, enrichErrInvalidDate)
	}
	if resp.RequestID != "bad" {
		t.Errorf("request_id not echoed: %q", resp.RequestID)
	}
}

// TestEnrich_StartSubscriber_RejectsNilClient guards against a wiring bug
// where the subscriber would silently no-op if NATS were not initialized.
func TestEnrich_StartSubscriber_RejectsNilClient(t *testing.T) {
	c := New("weather")
	if _, err := StartEnrichmentSubscriber(context.Background(), nil, c); err == nil {
		t.Error("expected error for nil NATS client")
	}
}
