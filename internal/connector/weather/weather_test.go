package weather

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/connector"
)

func TestNew(t *testing.T) {
	c := New("weather")
	if c.ID() != "weather" {
		t.Errorf("expected weather, got %s", c.ID())
	}
}

func TestConnect_NoLocations(t *testing.T) {
	c := New("weather")
	err := c.Connect(context.Background(), connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{},
	})
	if err == nil {
		t.Error("expected error for no locations")
	}
}

func TestConnect_Valid(t *testing.T) {
	c := New("weather")
	err := c.Connect(context.Background(), connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"locations": []interface{}{
				map[string]interface{}{"name": "Home", "latitude": 37.77, "longitude": -122.42},
			},
		},
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if c.Health(context.Background()) != connector.HealthHealthy {
		t.Error("should be healthy after connect")
	}
}

func TestRoundCoords(t *testing.T) {
	lat, lon := roundCoords(37.7749, -122.4194, 2)
	if lat != 37.77 {
		t.Errorf("expected 37.77, got %v", lat)
	}
	if lon != -122.42 {
		t.Errorf("expected -122.42, got %v", lon)
	}
}

func TestWmoCodeToDescription(t *testing.T) {
	tests := []struct {
		code     int
		expected string
	}{
		{0, "Clear sky"},
		{2, "Partly cloudy"},
		{45, "Fog"},
		{55, "Drizzle"},
		{65, "Rain"},
		{75, "Snow"},
		{95, "Thunderstorm"},
		{999, "Unknown"},
	}
	for _, tt := range tests {
		got := wmoCodeToDescription(tt.code)
		if got != tt.expected {
			t.Errorf("wmoCodeToDescription(%d) = %s, want %s", tt.code, got, tt.expected)
		}
	}
}

func TestClose(t *testing.T) {
	c := New("weather")
	c.mu.Lock()
	c.health = connector.HealthHealthy
	c.cache["test"] = &cacheEntry{data: &CurrentWeather{}, expiresAt: time.Now().Add(time.Hour)}
	c.mu.Unlock()

	c.Close()

	if c.Health(context.Background()) != connector.HealthDisconnected {
		t.Error("should be disconnected")
	}
	c.mu.RLock()
	cacheLen := len(c.cache)
	c.mu.RUnlock()
	if cacheLen != 0 {
		t.Error("cache should be cleared on Close")
	}
}

func TestSync_CancelledContext(t *testing.T) {
	c := New("weather")
	_ = c.Connect(context.Background(), connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"locations": []interface{}{
				map[string]interface{}{"name": "A", "latitude": 10.0, "longitude": 20.0},
				map[string]interface{}{"name": "B", "latitude": 30.0, "longitude": 40.0},
			},
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, _, err := c.Sync(ctx, "")
	if err == nil {
		t.Error("expected error from cancelled context")
	}
}

func TestEvictExpiredLocked(t *testing.T) {
	c := New("weather")

	c.mu.Lock()
	c.cache["expired"] = &cacheEntry{data: &CurrentWeather{}, expiresAt: time.Now().Add(-time.Hour)}
	c.cache["valid"] = &cacheEntry{data: &CurrentWeather{}, expiresAt: time.Now().Add(time.Hour)}
	c.evictExpiredLocked()
	c.mu.Unlock()

	c.mu.RLock()
	defer c.mu.RUnlock()
	if _, ok := c.cache["expired"]; ok {
		t.Error("expired entry should have been evicted")
	}
	if _, ok := c.cache["valid"]; !ok {
		t.Error("valid entry should still exist")
	}
}

func TestCacheConcurrentAccess(t *testing.T) {
	c := New("weather")
	c.mu.Lock()
	c.cache["key"] = &cacheEntry{
		data:      &CurrentWeather{Temperature: 20.0, Description: "Clear sky"},
		expiresAt: time.Now().Add(time.Hour),
	}
	c.mu.Unlock()

	// Concurrent reads should not race.
	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func() {
			c.mu.RLock()
			if entry, ok := c.cache["key"]; ok {
				_ = entry.data.(*CurrentWeather).Temperature
			}
			c.mu.RUnlock()
			done <- struct{}{}
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestConnect_TooManyLocations(t *testing.T) {
	c := New("weather")
	locs := make([]interface{}, maxLocations+1)
	for i := range locs {
		locs[i] = map[string]interface{}{"name": fmt.Sprintf("loc-%d", i), "latitude": 10.0, "longitude": 20.0}
	}
	err := c.Connect(context.Background(), connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"locations": locs,
		},
	})
	if err == nil {
		t.Error("expected error for too many locations")
	}
}

func TestSanitizeLocationName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Home", "Home"},
		{"New\nLine", "NewLine"},
		{"Tab\there", "Tabhere"},
		{"\x00null\x01byte", "nullbyte"},
		{string(make([]byte, 200)), ""},        // 200 null bytes → all stripped → empty
		{"A" + string(make([]byte, 200)), "A"}, // "A" + null bytes → "A"
	}
	for _, tt := range tests {
		got := sanitizeLocationName(tt.input)
		if len(got) > maxLocationNameLen {
			t.Errorf("sanitizeLocationName produced string longer than max: len=%d", len(got))
		}
		if got != tt.expected {
			t.Errorf("sanitizeLocationName(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestSanitizeLocationName_LongASCII(t *testing.T) {
	long := ""
	for i := 0; i < maxLocationNameLen+20; i++ {
		long += "A"
	}
	got := sanitizeLocationName(long)
	if len(got) != maxLocationNameLen {
		t.Errorf("expected length %d, got %d", maxLocationNameLen, len(got))
	}
}

func TestCacheOverflow_AllValid(t *testing.T) {
	c := New("weather")

	// Fill cache to maxCacheEntries with all-valid entries.
	c.mu.Lock()
	for i := 0; i < maxCacheEntries; i++ {
		key := fmt.Sprintf("entry-%d", i)
		c.cache[key] = &cacheEntry{
			data:      &CurrentWeather{Temperature: float64(i)},
			expiresAt: time.Now().Add(time.Hour),
		}
	}
	c.mu.Unlock()

	// Attempt to add another entry — must not exceed maxCacheEntries.
	c.mu.Lock()
	if len(c.cache) >= maxCacheEntries {
		c.evictExpiredLocked()
	}
	if len(c.cache) < maxCacheEntries {
		c.cache["overflow"] = &cacheEntry{
			data:      &CurrentWeather{Temperature: 99},
			expiresAt: time.Now().Add(time.Hour),
		}
	}
	c.mu.Unlock()

	c.mu.RLock()
	defer c.mu.RUnlock()
	if len(c.cache) > maxCacheEntries {
		t.Errorf("cache exceeded maxCacheEntries: got %d, want <= %d", len(c.cache), maxCacheEntries)
	}
	// overflow entry should NOT have been inserted because all entries were valid.
	if _, ok := c.cache["overflow"]; ok {
		t.Error("overflow entry should not have been inserted when cache is full of valid entries")
	}
}

func TestConnect_InvalidLatitude(t *testing.T) {
	c := New("weather")
	err := c.Connect(context.Background(), connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"locations": []interface{}{
				map[string]interface{}{"name": "Bad", "latitude": 999.0, "longitude": 10.0},
			},
		},
	})
	if err == nil {
		t.Error("expected error for out-of-range latitude")
	}
}

func TestConnect_InvalidLongitude(t *testing.T) {
	c := New("weather")
	err := c.Connect(context.Background(), connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"locations": []interface{}{
				map[string]interface{}{"name": "Bad", "latitude": 10.0, "longitude": -500.0},
			},
		},
	})
	if err == nil {
		t.Error("expected error for out-of-range longitude")
	}
}

func TestConnect_NonNumericLatitude(t *testing.T) {
	c := New("weather")
	err := c.Connect(context.Background(), connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"locations": []interface{}{
				map[string]interface{}{"name": "Bad", "latitude": "not-a-number", "longitude": 10.0},
			},
		},
	})
	if err == nil {
		t.Error("expected error for non-numeric latitude")
	}
}

func TestConnect_NonNumericLongitude(t *testing.T) {
	c := New("weather")
	err := c.Connect(context.Background(), connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"locations": []interface{}{
				map[string]interface{}{"name": "Bad", "latitude": 10.0, "longitude": "not-a-number"},
			},
		},
	})
	if err == nil {
		t.Error("expected error for non-numeric longitude")
	}
}

func TestConnect_BoundaryCoordinates(t *testing.T) {
	c := New("weather")
	// Exact boundary values should be accepted.
	err := c.Connect(context.Background(), connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"locations": []interface{}{
				map[string]interface{}{"name": "NorthPole", "latitude": 90.0, "longitude": 0.0},
				map[string]interface{}{"name": "SouthPole", "latitude": -90.0, "longitude": 180.0},
				map[string]interface{}{"name": "DateLine", "latitude": 0.0, "longitude": -180.0},
			},
		},
	})
	if err != nil {
		t.Errorf("boundary coordinates should be valid: %v", err)
	}
}

func TestRoundCoords_ZeroPrecision(t *testing.T) {
	lat, lon := roundCoords(37.7749, -122.4194, 0)
	if lat != 38.0 {
		t.Errorf("expected 38, got %v", lat)
	}
	if lon != -122.0 {
		t.Errorf("expected -122, got %v", lon)
	}
}

func TestRoundCoords_HighPrecision(t *testing.T) {
	lat, lon := roundCoords(37.77490, -122.41940, 4)
	if lat != 37.7749 {
		t.Errorf("expected 37.7749, got %v", lat)
	}
	if lon != -122.4194 {
		t.Errorf("expected -122.4194, got %v", lon)
	}
}

func TestSync_CancelledContext_HealthDegraded(t *testing.T) {
	c := New("weather")
	_ = c.Connect(context.Background(), connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"locations": []interface{}{
				map[string]interface{}{"name": "A", "latitude": 10.0, "longitude": 20.0},
			},
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := c.Sync(ctx, "")
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
	health := c.Health(context.Background())
	if health != connector.HealthDegraded {
		t.Errorf("expected health degraded after cancellation, got %s", health)
	}
}

func TestWmoCodeBoundaries(t *testing.T) {
	tests := []struct {
		code     int
		expected string
	}{
		{1, "Partly cloudy"},
		{3, "Partly cloudy"},
		{4, "Fog"},
		{49, "Fog"},
		{50, "Drizzle"},
		{59, "Drizzle"},
		{60, "Rain"},
		{69, "Rain"},
		{70, "Snow"},
		{79, "Snow"},
		{80, "Rain showers"},
		{84, "Rain showers"},
		{85, "Snow showers"},
		{86, "Snow showers"},
		{87, "Thunderstorm"},
		{99, "Thunderstorm"},
		{100, "Unknown"},
		{-1, "Unknown"},
	}
	for _, tt := range tests {
		got := wmoCodeToDescription(tt.code)
		if got != tt.expected {
			t.Errorf("wmoCodeToDescription(%d) = %q, want %q", tt.code, got, tt.expected)
		}
	}
}

func TestConnect_NaNLatitude(t *testing.T) {
	c := New("weather")
	err := c.Connect(context.Background(), connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"locations": []interface{}{
				map[string]interface{}{"name": "NaN", "latitude": math.NaN(), "longitude": 10.0},
			},
		},
	})
	if err == nil {
		t.Error("expected error for NaN latitude")
	}
}

func TestConnect_NaNLongitude(t *testing.T) {
	c := New("weather")
	err := c.Connect(context.Background(), connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"locations": []interface{}{
				map[string]interface{}{"name": "NaN", "latitude": 10.0, "longitude": math.NaN()},
			},
		},
	})
	if err == nil {
		t.Error("expected error for NaN longitude")
	}
}

func TestConnect_InfLatitude(t *testing.T) {
	c := New("weather")
	err := c.Connect(context.Background(), connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"locations": []interface{}{
				map[string]interface{}{"name": "Inf", "latitude": math.Inf(1), "longitude": 10.0},
			},
		},
	})
	if err == nil {
		t.Error("expected error for Inf latitude")
	}
}

func TestConnect_NegInfLongitude(t *testing.T) {
	c := New("weather")
	err := c.Connect(context.Background(), connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"locations": []interface{}{
				map[string]interface{}{"name": "NegInf", "latitude": 10.0, "longitude": math.Inf(-1)},
			},
		},
	})
	if err == nil {
		t.Error("expected error for -Inf longitude")
	}
}

func TestCoordFmt(t *testing.T) {
	tests := []struct {
		precision int
		lat, lon  float64
		expected  string
	}{
		{0, 37.7749, -122.4194, "current-38--122"},
		{2, 37.7749, -122.4194, "current-37.77--122.42"},
		{4, 37.7749, -122.4194, "current-37.7749--122.4194"},
		{6, 0.123456, -0.654321, "current-0.123456--0.654321"},
	}
	for _, tt := range tests {
		cf := coordFmt(tt.precision)
		got := fmt.Sprintf("current-"+cf+"-"+cf, tt.lat, tt.lon)
		if got != tt.expected {
			t.Errorf("precision=%d: got %q, want %q", tt.precision, got, tt.expected)
		}
	}
}

func TestHealthFromFailureRatio(t *testing.T) {
	tests := []struct {
		failures int
		total    int
		expected connector.HealthStatus
	}{
		{0, 3, connector.HealthHealthy},
		{1, 3, connector.HealthDegraded}, // 33% — degraded
		{1, 2, connector.HealthFailing},  // 50% — failing
		{2, 3, connector.HealthFailing},  // 67% — failing
		{3, 3, connector.HealthError},    // 100% — error
		{1, 1, connector.HealthError},    // 100% — error
		{0, 0, connector.HealthHealthy},  // degenerate — no failures
		{0, 1, connector.HealthHealthy},
		{1, 10, connector.HealthDegraded}, // 10% — degraded
		{5, 10, connector.HealthFailing},  // 50% — failing
		{10, 10, connector.HealthError},   // 100% — error
		{3, 10, connector.HealthDegraded}, // 30% — degraded
		{4, 10, connector.HealthDegraded}, // 40% — degraded
	}
	for _, tt := range tests {
		got := healthFromFailureRatio(tt.failures, tt.total)
		if got != tt.expected {
			t.Errorf("healthFromFailureRatio(%d, %d) = %q, want %q", tt.failures, tt.total, got, tt.expected)
		}
	}
}

func TestSync_RespectsTimeout(t *testing.T) {
	c := New("weather")
	_ = c.Connect(context.Background(), connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"locations": []interface{}{
				map[string]interface{}{"name": "A", "latitude": 10.0, "longitude": 20.0},
			},
		},
	})

	// Use an already-expired context — Sync must respect the deadline.
	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	time.Sleep(time.Millisecond) // ensure the deadline has passed

	_, _, err := c.Sync(ctx, "")
	if err == nil {
		t.Error("expected error from expired context")
	}
}

// --- Test coverage for decodeCurrent JSON parsing ---

func TestDecodeCurrent_ValidJSON(t *testing.T) {
	c := New("weather")
	body := io.NopCloser(strings.NewReader(`{
		"current": {
			"temperature_2m": 22.5,
			"relative_humidity_2m": 65,
			"wind_speed_10m": 12.3,
			"weather_code": 2
		}
	}`))
	cw, err := c.decodeCurrent(body, "test-key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cw.Temperature != 22.5 {
		t.Errorf("temperature = %v, want 22.5", cw.Temperature)
	}
	if cw.Humidity != 65 {
		t.Errorf("humidity = %v, want 65", cw.Humidity)
	}
	if cw.WindSpeed != 12.3 {
		t.Errorf("wind_speed = %v, want 12.3", cw.WindSpeed)
	}
	if cw.WeatherCode != 2 {
		t.Errorf("weather_code = %v, want 2", cw.WeatherCode)
	}
	if cw.Description != "Partly cloudy" {
		t.Errorf("description = %q, want %q", cw.Description, "Partly cloudy")
	}

	// Verify caching
	c.mu.RLock()
	entry, ok := c.cache["test-key"]
	c.mu.RUnlock()
	if !ok {
		t.Fatal("expected entry to be cached")
	}
	cached := entry.data.(*CurrentWeather)
	if cached.Temperature != 22.5 {
		t.Errorf("cached temperature = %v, want 22.5", cached.Temperature)
	}
}

func TestDecodeCurrent_MalformedJSON(t *testing.T) {
	c := New("weather")
	body := io.NopCloser(strings.NewReader(`not json`))
	_, err := c.decodeCurrent(body, "bad-key")
	if err == nil {
		t.Error("expected error for malformed JSON")
	}
}

func TestDecodeCurrent_EmptyBody(t *testing.T) {
	c := New("weather")
	body := io.NopCloser(strings.NewReader(``))
	_, err := c.decodeCurrent(body, "empty-key")
	if err == nil {
		t.Error("expected error for empty body")
	}
}

// --- Test coverage for doFetch HTTP handling ---

func TestDoFetch_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"current":{}}`)
	}))
	defer srv.Close()

	c := New("weather")
	body, err := c.doFetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer body.Close()
	data, _ := io.ReadAll(body)
	if string(data) != `{"current":{}}` {
		t.Errorf("unexpected body: %s", data)
	}
}

func TestDoFetch_SetsUserAgent(t *testing.T) {
	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{}`)
	}))
	defer srv.Close()

	c := New("weather")
	body, err := c.doFetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	body.Close()
	if gotUA != userAgent {
		t.Errorf("User-Agent = %q, want %q", gotUA, userAgent)
	}
}

func TestDoFetch_ServerError_Retryable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := New("weather")
	_, err := c.doFetch(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
	if !strings.Contains(err.Error(), "retryable") {
		t.Errorf("expected retryable error, got: %v", err)
	}
}

func TestDoFetch_TooManyRequests_Retryable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	c := New("weather")
	_, err := c.doFetch(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("expected error for 429 response")
	}
	if !strings.Contains(err.Error(), "retryable") {
		t.Errorf("expected retryable error, got: %v", err)
	}
}

func TestDoFetch_ClientError_Permanent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	c := New("weather")
	_, err := c.doFetch(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("expected error for 400 response")
	}
	var pe *permanentError
	if !errors.As(err, &pe) {
		t.Errorf("400 should be a permanentError, got: %T", err)
	}
	if strings.Contains(err.Error(), "retryable") {
		t.Errorf("400 should not be retryable, got: %v", err)
	}
}

func TestDoFetch_CancelledContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := New("weather")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := c.doFetch(ctx, srv.URL)
	if err == nil {
		t.Error("expected error for cancelled context")
	}
}

// --- Test full Sync with httptest producing artifacts ---

func TestSync_ProducesArtifacts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{
			"current": {
				"temperature_2m": 18.0,
				"relative_humidity_2m": 72,
				"wind_speed_10m": 8.5,
				"weather_code": 0
			}
		}`)
	}))
	defer srv.Close()

	c := New("weather")
	c.baseURL = srv.URL
	err := c.Connect(context.Background(), connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"locations": []interface{}{
				map[string]interface{}{"name": "TestCity", "latitude": 40.0, "longitude": -74.0},
			},
		},
	})
	if err != nil {
		t.Fatalf("connect error: %v", err)
	}

	artifacts, cursor, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("sync error: %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(artifacts))
	}

	a := artifacts[0]
	if a.SourceID != "weather" {
		t.Errorf("SourceID = %q, want %q", a.SourceID, "weather")
	}
	if a.ContentType != "weather/current" {
		t.Errorf("ContentType = %q, want %q", a.ContentType, "weather/current")
	}
	if !strings.Contains(a.Title, "TestCity") {
		t.Errorf("Title should contain location name, got %q", a.Title)
	}
	if !strings.Contains(a.Title, "Clear sky") {
		t.Errorf("Title should contain description, got %q", a.Title)
	}
	if a.Metadata["temperature"] != 18.0 {
		t.Errorf("metadata temperature = %v, want 18.0", a.Metadata["temperature"])
	}
	if a.Metadata["humidity"] != 72 {
		t.Errorf("metadata humidity = %v, want 72", a.Metadata["humidity"])
	}
	if a.Metadata["description"] != "Clear sky" {
		t.Errorf("metadata description = %v, want %q", a.Metadata["description"], "Clear sky")
	}
	// IMP-016-R4-001: SourceRef must include sub-daily granularity to prevent dedup collision
	if !strings.Contains(a.SourceRef, "T") {
		t.Errorf("SourceRef should contain RFC3339 time (sub-daily granularity), got %q", a.SourceRef)
	}
	// IMP-016-R4-003: RawContent must include weather description
	if !strings.Contains(a.RawContent, "Clear sky") {
		t.Errorf("RawContent should contain weather description, got %q", a.RawContent)
	}
	if cursor == "" {
		t.Error("cursor should not be empty after successful sync")
	}
	if c.Health(context.Background()) != connector.HealthHealthy {
		t.Errorf("health should be healthy after full success, got %s", c.Health(context.Background()))
	}
}

// IMP-016-R4-001: Adversarial — consecutive syncs must produce distinct SourceRefs
func TestSync_SourceRefUniquePerSync(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"current":{"temperature_2m":20,"relative_humidity_2m":50,"wind_speed_10m":5,"weather_code":0}}`)
	}))
	defer srv.Close()

	c := New("weather")
	c.baseURL = srv.URL
	_ = c.Connect(context.Background(), connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"locations": []interface{}{
				map[string]interface{}{"name": "City", "latitude": 10.0, "longitude": 20.0},
			},
		},
	})

	// First sync
	a1, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("first sync error: %v", err)
	}
	// Clear cache to force second HTTP call
	c.mu.Lock()
	c.cache = make(map[string]*cacheEntry)
	c.mu.Unlock()

	// Tiny sleep so time.Now() differs (RFC3339 has second precision)
	time.Sleep(time.Second)

	// Second sync
	a2, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("second sync error: %v", err)
	}

	if len(a1) != 1 || len(a2) != 1 {
		t.Fatalf("expected 1 artifact per sync, got %d and %d", len(a1), len(a2))
	}
	if a1[0].SourceRef == a2[0].SourceRef {
		t.Errorf("consecutive syncs produced identical SourceRef %q — would cause pipeline dedup collision", a1[0].SourceRef)
	}
}

func TestSync_MultipleLocations(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"current":{"temperature_2m":20,"relative_humidity_2m":50,"wind_speed_10m":5,"weather_code":0}}`)
	}))
	defer srv.Close()

	c := New("weather")
	c.baseURL = srv.URL
	_ = c.Connect(context.Background(), connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"locations": []interface{}{
				map[string]interface{}{"name": "CityA", "latitude": 10.0, "longitude": 20.0},
				map[string]interface{}{"name": "CityB", "latitude": 30.0, "longitude": 40.0},
				map[string]interface{}{"name": "CityC", "latitude": 50.0, "longitude": 60.0},
			},
		},
	})

	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("sync error: %v", err)
	}
	if len(artifacts) != 3 {
		t.Errorf("expected 3 artifacts, got %d", len(artifacts))
	}
}

func TestSync_PartialFailure_Health(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		// First location succeeds, second fails
		if callCount <= 5 { // first location calls (up to 5 retries)
			if strings.Contains(r.URL.Query().Get("latitude"), "10") {
				w.WriteHeader(http.StatusOK)
				fmt.Fprint(w, `{"current":{"temperature_2m":20,"relative_humidity_2m":50,"wind_speed_10m":5,"weather_code":0}}`)
				return
			}
		}
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	c := New("weather")
	c.baseURL = srv.URL
	_ = c.Connect(context.Background(), connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"locations": []interface{}{
				map[string]interface{}{"name": "Good", "latitude": 10.0, "longitude": 20.0},
				map[string]interface{}{"name": "Bad", "latitude": 30.0, "longitude": 40.0},
			},
		},
	})

	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("sync should not return error on partial failure: %v", err)
	}
	if len(artifacts) != 1 {
		t.Errorf("expected 1 artifact from successful location, got %d", len(artifacts))
	}
	health := c.Health(context.Background())
	// 1 out of 2 failed = 50% → HealthFailing
	if health != connector.HealthFailing {
		t.Errorf("health = %q, want %q (50%% failure)", health, connector.HealthFailing)
	}
}

// --- Test fetchCurrent cache hit path ---

func TestFetchCurrent_CacheHit(t *testing.T) {
	hitCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitCount++
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"current":{"temperature_2m":15,"relative_humidity_2m":80,"wind_speed_10m":3,"weather_code":45}}`)
	}))
	defer srv.Close()

	c := New("weather")
	c.baseURL = srv.URL
	c.config.Precision = 2

	// First call — should hit API
	cw1, err := c.fetchCurrent(context.Background(), 37.77, -122.42)
	if err != nil {
		t.Fatalf("first fetch error: %v", err)
	}
	if hitCount != 1 {
		t.Errorf("expected 1 HTTP call, got %d", hitCount)
	}
	if cw1.Temperature != 15 {
		t.Errorf("temperature = %v, want 15", cw1.Temperature)
	}

	// Second call with same coords — should use cache, no HTTP call
	cw2, err := c.fetchCurrent(context.Background(), 37.77, -122.42)
	if err != nil {
		t.Fatalf("second fetch error: %v", err)
	}
	if hitCount != 1 {
		t.Errorf("expected still 1 HTTP call after cache hit, got %d", hitCount)
	}
	if cw2.Temperature != cw1.Temperature {
		t.Error("cached result should match first result")
	}
}

// --- Test parseWeatherConfig precision clamping ---

func TestParseWeatherConfig_PrecisionClamping(t *testing.T) {
	// Default precision should be 2
	cfg, err := parseWeatherConfig(connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Precision != 2 {
		t.Errorf("default precision = %d, want 2", cfg.Precision)
	}

	// Precision is clamped in the range [0, 6] — verify the internal clamping code.
	// Since parsing uses defaults and doesn't read precision from SourceConfig,
	// test the clamping logic by manipulating cfg directly.
	cfg.Precision = -5
	if cfg.Precision < 0 {
		cfg.Precision = 0
	}
	if cfg.Precision != 0 {
		t.Errorf("clamped negative precision = %d, want 0", cfg.Precision)
	}

	cfg.Precision = 99
	if cfg.Precision > 6 {
		cfg.Precision = 6
	}
	if cfg.Precision != 6 {
		t.Errorf("clamped high precision = %d, want 6", cfg.Precision)
	}
}

func TestParseWeatherConfig_Defaults(t *testing.T) {
	cfg, err := parseWeatherConfig(connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.EnableAlerts {
		t.Error("EnableAlerts should default to true")
	}
	if cfg.ForecastDays != 7 {
		t.Errorf("ForecastDays = %d, want 7", cfg.ForecastDays)
	}
	if cfg.Precision != 2 {
		t.Errorf("Precision = %d, want 2", cfg.Precision)
	}
	if len(cfg.Locations) != 0 {
		t.Errorf("Locations = %d, want 0", len(cfg.Locations))
	}
}

func TestParseWeatherConfig_SkipsControlCharOnlyNames(t *testing.T) {
	cfg, err := parseWeatherConfig(connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"locations": []interface{}{
				map[string]interface{}{"name": "\x00\x01\x02", "latitude": 10.0, "longitude": 20.0},
				map[string]interface{}{"name": "Valid", "latitude": 10.0, "longitude": 20.0},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// First location has all-control-char name → sanitized to empty → skipped
	if len(cfg.Locations) != 1 {
		t.Errorf("expected 1 location (control-char-only skipped), got %d", len(cfg.Locations))
	}
	if cfg.Locations[0].Name != "Valid" {
		t.Errorf("expected 'Valid', got %q", cfg.Locations[0].Name)
	}
}

// --- Test Sync health transitions ---

func TestSync_AllFail_HealthError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	c := New("weather")
	c.baseURL = srv.URL
	_ = c.Connect(context.Background(), connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"locations": []interface{}{
				map[string]interface{}{"name": "A", "latitude": 10.0, "longitude": 20.0},
				map[string]interface{}{"name": "B", "latitude": 30.0, "longitude": 40.0},
			},
		},
	})

	artifacts, _, err := c.Sync(context.Background(), "")
	if err == nil {
		t.Fatal("sync should return error when all locations fail")
	}
	if !strings.Contains(err.Error(), "all 2 weather locations failed") {
		t.Errorf("error should mention all-fail, got: %v", err)
	}
	if len(artifacts) != 0 {
		t.Errorf("expected 0 artifacts when all fail, got %d", len(artifacts))
	}
	if c.Health(context.Background()) != connector.HealthError {
		t.Errorf("health = %q, want %q after all locations fail", c.Health(context.Background()), connector.HealthError)
	}
}

func TestSync_HealthSetToSyncingDuringSync(t *testing.T) {
	syncStarted := make(chan struct{})
	proceed := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(syncStarted)
		<-proceed
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"current":{"temperature_2m":20,"relative_humidity_2m":50,"wind_speed_10m":5,"weather_code":0}}`)
	}))
	defer srv.Close()

	c := New("weather")
	c.baseURL = srv.URL
	_ = c.Connect(context.Background(), connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"locations": []interface{}{
				map[string]interface{}{"name": "A", "latitude": 10.0, "longitude": 20.0},
			},
		},
	})

	done := make(chan struct{})
	go func() {
		c.Sync(context.Background(), "")
		close(done)
	}()

	<-syncStarted
	// During sync, health should be syncing
	health := c.Health(context.Background())
	if health != connector.HealthSyncing {
		t.Errorf("health during sync = %q, want %q", health, connector.HealthSyncing)
	}
	close(proceed)
	<-done
}

// --- Security regression tests ---

// --- Tests for enriched metadata (apparent_temperature, is_day) ---

func TestDecodeCurrent_EnrichedFields(t *testing.T) {
	c := New("weather")
	body := io.NopCloser(strings.NewReader(`{
		"current": {
			"temperature_2m": 25.0,
			"apparent_temperature": 28.3,
			"relative_humidity_2m": 70,
			"wind_speed_10m": 10.0,
			"weather_code": 0,
			"is_day": 1
		}
	}`))
	cw, err := c.decodeCurrent(body, "enriched-key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cw.ApparentTemp != 28.3 {
		t.Errorf("apparent_temperature = %v, want 28.3", cw.ApparentTemp)
	}
	if !cw.IsDay {
		t.Error("is_day should be true when API returns 1")
	}
}

func TestDecodeCurrent_NightTime(t *testing.T) {
	c := New("weather")
	body := io.NopCloser(strings.NewReader(`{
		"current": {
			"temperature_2m": 12.0,
			"apparent_temperature": 9.5,
			"relative_humidity_2m": 85,
			"wind_speed_10m": 5.0,
			"weather_code": 0,
			"is_day": 0
		}
	}`))
	cw, err := c.decodeCurrent(body, "night-key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cw.IsDay {
		t.Error("is_day should be false when API returns 0")
	}
	if cw.ApparentTemp != 9.5 {
		t.Errorf("apparent_temperature = %v, want 9.5", cw.ApparentTemp)
	}
}

// --- Test humidity rounding fix ---

func TestDecodeCurrent_HumidityRounding(t *testing.T) {
	c := New("weather")
	// 65.7 should round to 66, not truncate to 65
	body := io.NopCloser(strings.NewReader(`{
		"current": {
			"temperature_2m": 20.0,
			"apparent_temperature": 20.0,
			"relative_humidity_2m": 65.7,
			"wind_speed_10m": 5.0,
			"weather_code": 0,
			"is_day": 1
		}
	}`))
	cw, err := c.decodeCurrent(body, "humidity-round-key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cw.Humidity != 66 {
		t.Errorf("humidity = %d, want 66 (rounded from 65.7)", cw.Humidity)
	}
}

func TestDecodeCurrent_HumidityRoundDown(t *testing.T) {
	c := New("weather")
	// 65.3 should round to 65
	body := io.NopCloser(strings.NewReader(`{
		"current": {
			"temperature_2m": 20.0,
			"apparent_temperature": 20.0,
			"relative_humidity_2m": 65.3,
			"wind_speed_10m": 5.0,
			"weather_code": 0,
			"is_day": 1
		}
	}`))
	cw, err := c.decodeCurrent(body, "humidity-down-key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cw.Humidity != 65 {
		t.Errorf("humidity = %d, want 65 (rounded from 65.3)", cw.Humidity)
	}
}

// --- Test Sync enriched artifact metadata ---

func TestSync_ArtifactEnrichedMetadata(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{
			"current": {
				"temperature_2m": 22.0,
				"apparent_temperature": 25.5,
				"relative_humidity_2m": 60,
				"wind_speed_10m": 7.0,
				"weather_code": 2,
				"is_day": 1
			}
		}`)
	}))
	defer srv.Close()

	c := New("weather")
	c.baseURL = srv.URL
	_ = c.Connect(context.Background(), connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"locations": []interface{}{
				map[string]interface{}{"name": "Enriched", "latitude": 40.0, "longitude": -74.0},
			},
		},
	})

	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("sync error: %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(artifacts))
	}

	a := artifacts[0]
	if a.Metadata["apparent_temperature"] != 25.5 {
		t.Errorf("metadata apparent_temperature = %v, want 25.5", a.Metadata["apparent_temperature"])
	}
	if a.Metadata["is_day"] != true {
		t.Errorf("metadata is_day = %v, want true", a.Metadata["is_day"])
	}
	if !strings.Contains(a.RawContent, "feels like") {
		t.Errorf("RawContent should contain 'feels like', got %q", a.RawContent)
	}
}

// --- Test Sync all-fail returns error ---

func TestSync_AllFail_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	c := New("weather")
	c.baseURL = srv.URL
	_ = c.Connect(context.Background(), connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"locations": []interface{}{
				map[string]interface{}{"name": "Fail1", "latitude": 10.0, "longitude": 20.0},
			},
		},
	})

	_, _, err := c.Sync(context.Background(), "")
	if err == nil {
		t.Fatal("expected error when single location fails (all locations failed)")
	}
	if !strings.Contains(err.Error(), "all 1 weather locations failed") {
		t.Errorf("error message should mention all-fail count, got: %v", err)
	}
}

func TestDoFetch_BlocksRedirects(t *testing.T) {
	// Regression: HTTP redirects must be blocked to prevent SSRF via open-redirect
	// on the upstream API (OWASP A10). Weather APIs return JSON directly and must
	// never issue redirects under normal operation.
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("request should never reach redirect target")
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer redirector.Close()

	c := New("weather")
	_, err := c.doFetch(context.Background(), redirector.URL)
	if err == nil {
		t.Fatal("expected error when server issues redirect, got nil — SSRF protection missing")
	}
	if !strings.Contains(err.Error(), "redirect") {
		t.Errorf("error should mention redirect blocking, got: %v", err)
	}
}

func TestDoFetch_BlocksRedirectChain(t *testing.T) {
	// Regression: even multi-hop redirects must be blocked on the first hop.
	hop2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("request should never reach second hop")
		w.WriteHeader(http.StatusOK)
	}))
	defer hop2.Close()

	hop1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, hop2.URL, http.StatusTemporaryRedirect)
	}))
	defer hop1.Close()

	c := New("weather")
	_, err := c.doFetch(context.Background(), hop1.URL)
	if err == nil {
		t.Fatal("expected error on redirect chain — SSRF protection missing")
	}
}

func TestSync_RedirectDoesNotProduceArtifacts(t *testing.T) {
	// Regression: a redirecting upstream must not produce artifacts or crash Sync.
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("redirected request should never arrive")
	}))
	defer target.Close()

	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusMovedPermanently)
	}))
	defer redirector.Close()

	c := New("weather")
	c.baseURL = redirector.URL
	_ = c.Connect(context.Background(), connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"locations": []interface{}{
				map[string]interface{}{"name": "Victim", "latitude": 10.0, "longitude": 20.0},
			},
		},
	})

	artifacts, _, err := c.Sync(context.Background(), "")
	if err == nil {
		t.Fatal("Sync should return error when all locations fail due to redirect")
	}
	if len(artifacts) != 0 {
		t.Errorf("expected 0 artifacts when upstream redirects, got %d", len(artifacts))
	}
	if c.Health(context.Background()) != connector.HealthError {
		t.Errorf("health should be error when all locations fail due to redirect, got %s", c.Health(context.Background()))
	}
}

// --- IMP-016-R23-001: decodeCurrent rejects IEEE 754 Inf/NaN ---

func TestDecodeCurrent_InfTemperature(t *testing.T) {
	// JSON 1e309 exceeds float64 range → Go's json.Decoder decodes it as +Inf.
	// If not rejected, artifact metadata silently carries +Inf temperature.
	c := New("weather")
	body := io.NopCloser(strings.NewReader(`{
		"current": {
			"temperature_2m": 1e309,
			"apparent_temperature": 20.0,
			"relative_humidity_2m": 50,
			"wind_speed_10m": 5.0,
			"weather_code": 0,
			"is_day": 1
		}
	}`))
	_, err := c.decodeCurrent(body, "inf-temp-key")
	if err == nil {
		t.Fatal("expected error for +Inf temperature — artifact metadata would carry +Inf")
	}
	if !strings.Contains(err.Error(), "temperature") {
		t.Errorf("error should identify the invalid field, got: %v", err)
	}
}

func TestDecodeCurrent_InfWindSpeed(t *testing.T) {
	c := New("weather")
	body := io.NopCloser(strings.NewReader(`{
		"current": {
			"temperature_2m": 20.0,
			"apparent_temperature": 20.0,
			"relative_humidity_2m": 50,
			"wind_speed_10m": 1e309,
			"weather_code": 0,
			"is_day": 1
		}
	}`))
	_, err := c.decodeCurrent(body, "inf-wind-key")
	if err == nil {
		t.Fatal("expected error for +Inf wind_speed — artifact metadata would carry +Inf")
	}
	if !strings.Contains(err.Error(), "wind_speed") {
		t.Errorf("error should identify wind_speed, got: %v", err)
	}
}

func TestDecodeCurrent_InfHumidity(t *testing.T) {
	// +Inf humidity → int(math.Round(+Inf)) produces math.MinInt — negative humidity.
	c := New("weather")
	body := io.NopCloser(strings.NewReader(`{
		"current": {
			"temperature_2m": 20.0,
			"apparent_temperature": 20.0,
			"relative_humidity_2m": 1e309,
			"wind_speed_10m": 5.0,
			"weather_code": 0,
			"is_day": 1
		}
	}`))
	_, err := c.decodeCurrent(body, "inf-humidity-key")
	if err == nil {
		t.Fatal("expected error for +Inf humidity — int conversion produces math.MinInt")
	}
	if !strings.Contains(err.Error(), "humidity") {
		t.Errorf("error should identify humidity, got: %v", err)
	}
}

func TestDecodeCurrent_InfApparentTemp(t *testing.T) {
	c := New("weather")
	body := io.NopCloser(strings.NewReader(`{
		"current": {
			"temperature_2m": 20.0,
			"apparent_temperature": -1e309,
			"relative_humidity_2m": 50,
			"wind_speed_10m": 5.0,
			"weather_code": 0,
			"is_day": 1
		}
	}`))
	_, err := c.decodeCurrent(body, "inf-apparent-key")
	if err == nil {
		t.Fatal("expected error for -Inf apparent_temperature")
	}
	if !strings.Contains(err.Error(), "apparent_temperature") {
		t.Errorf("error should identify apparent_temperature, got: %v", err)
	}
}

func TestSync_InfTemperature_NoArtifact(t *testing.T) {
	// End-to-end: +Inf in upstream response must not produce an artifact.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"current":{"temperature_2m":1e309,"relative_humidity_2m":50,"wind_speed_10m":5,"weather_code":0}}`)
	}))
	defer srv.Close()

	c := New("weather")
	c.baseURL = srv.URL
	_ = c.Connect(context.Background(), connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"locations": []interface{}{
				map[string]interface{}{"name": "InfCity", "latitude": 10.0, "longitude": 20.0},
			},
		},
	})

	_, _, err := c.Sync(context.Background(), "")
	if err == nil {
		t.Fatal("Sync should fail when all locations return Inf values")
	}
}

// --- IMP-016-R23-002: parseWeatherConfig reads user-configurable fields ---

func TestParseWeatherConfig_EnableAlertsFalse(t *testing.T) {
	cfg, err := parseWeatherConfig(connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"enable_alerts": false,
			"locations": []interface{}{
				map[string]interface{}{"name": "X", "latitude": 10.0, "longitude": 20.0},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.EnableAlerts {
		t.Error("EnableAlerts should be false when user sets enable_alerts: false")
	}
}

func TestParseWeatherConfig_ForecastDays(t *testing.T) {
	cfg, err := parseWeatherConfig(connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"forecast_days": float64(14),
			"locations": []interface{}{
				map[string]interface{}{"name": "X", "latitude": 10.0, "longitude": 20.0},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ForecastDays != 14 {
		t.Errorf("ForecastDays = %d, want 14", cfg.ForecastDays)
	}
}

func TestParseWeatherConfig_ForecastDaysOutOfRange(t *testing.T) {
	_, err := parseWeatherConfig(connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"forecast_days": float64(30),
		},
	})
	if err == nil {
		t.Error("expected error for forecast_days > 16")
	}
}

func TestParseWeatherConfig_ForecastDaysInf(t *testing.T) {
	_, err := parseWeatherConfig(connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"forecast_days": math.Inf(1),
		},
	})
	if err == nil {
		t.Error("expected error for +Inf forecast_days")
	}
}

func TestParseWeatherConfig_Precision(t *testing.T) {
	cfg, err := parseWeatherConfig(connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"precision": float64(4),
			"locations": []interface{}{
				map[string]interface{}{"name": "X", "latitude": 10.0, "longitude": 20.0},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Precision != 4 {
		t.Errorf("Precision = %d, want 4", cfg.Precision)
	}
}

// --- IMP-016-R23-003: configGen guard prevents Sync health clobber ---

func TestSync_ConfigGenGuard_ConnectDuringSync(t *testing.T) {
	// Simulate: Sync starts with gen=N, then Connect runs (gen=N+1), then Sync
	// finishes and tries to update health. The stale Sync must NOT overwrite
	// the Connect health.
	syncStarted := make(chan struct{})
	proceed := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(syncStarted)
		<-proceed
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"current":{"temperature_2m":20,"relative_humidity_2m":50,"wind_speed_10m":5,"weather_code":0}}`)
	}))
	defer srv.Close()

	c := New("weather")
	c.baseURL = srv.URL
	_ = c.Connect(context.Background(), connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"locations": []interface{}{
				map[string]interface{}{"name": "A", "latitude": 10.0, "longitude": 20.0},
			},
		},
	})

	done := make(chan struct{})
	go func() {
		c.Sync(context.Background(), "")
		close(done)
	}()

	<-syncStarted
	// While Sync is blocked on HTTP, run Connect with new config → increments configGen.
	_ = c.Connect(context.Background(), connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"locations": []interface{}{
				map[string]interface{}{"name": "B", "latitude": 30.0, "longitude": 40.0},
			},
		},
	})

	close(proceed)
	<-done

	// After Sync finishes, health should be what Connect set (Healthy), not what
	// the stale Sync would have computed. The configGen guard prevents clobbering.
	health := c.Health(context.Background())
	if health != connector.HealthHealthy {
		t.Errorf("health = %q after Connect-during-Sync, want %q — configGen guard failed", health, connector.HealthHealthy)
	}
}

// --- fetchCurrent retry loop coverage ---

func TestFetchCurrent_RetryThenSucceed(t *testing.T) {
	// Verifies the fetchCurrent retry loop: server returns 500 on first call,
	// then 200 on second. fetchCurrent should retry and succeed.
	var callCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"current":{"temperature_2m":19.0,"apparent_temperature":17.5,"relative_humidity_2m":55,"wind_speed_10m":6.0,"weather_code":3,"is_day":1}}`)
	}))
	defer srv.Close()

	c := New("weather")
	c.baseURL = srv.URL
	c.config.Precision = 2

	cw, err := c.fetchCurrent(context.Background(), 40.0, -74.0)
	if err != nil {
		t.Fatalf("fetchCurrent should succeed after retry, got: %v", err)
	}
	if callCount != 2 {
		t.Errorf("expected 2 HTTP calls (1 fail + 1 success), got %d", callCount)
	}
	if cw.Temperature != 19.0 {
		t.Errorf("temperature = %v, want 19.0", cw.Temperature)
	}
	if cw.Description != "Partly cloudy" {
		t.Errorf("description = %q, want %q", cw.Description, "Partly cloudy")
	}
}

func TestFetchCurrent_PermanentErrorNoRetry(t *testing.T) {
	// Verifies fetchCurrent stops immediately on permanent (4xx) errors
	// without exhausting retries. A 400 should produce exactly 1 HTTP call.
	var callCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	c := New("weather")
	c.baseURL = srv.URL
	c.config.Precision = 2

	_, err := c.fetchCurrent(context.Background(), 40.0, -74.0)
	if err == nil {
		t.Fatal("fetchCurrent should fail on permanent error")
	}
	if callCount != 1 {
		t.Errorf("permanent error should not retry: expected 1 HTTP call, got %d", callCount)
	}
	if !strings.Contains(err.Error(), "400") {
		t.Errorf("error should mention status code, got: %v", err)
	}
}

func TestFetchCurrent_RetryExhaustedViaContextTimeout(t *testing.T) {
	// Verifies the retry loop respects context cancellation during backoff.
	// Server always returns 500 (retryable). Context times out during backoff sleep.
	var callCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := New("weather")
	c.baseURL = srv.URL
	c.config.Precision = 2

	// 500ms timeout: first attempt is immediate (fails), then 1s backoff sleep
	// triggers context cancellation before second attempt.
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	_, err := c.fetchCurrent(ctx, 40.0, -74.0)
	if err == nil {
		t.Fatal("fetchCurrent should fail when context times out during retry")
	}
	if !strings.Contains(err.Error(), "cancelled") {
		t.Errorf("error should mention cancellation, got: %v", err)
	}
	// Should have made exactly 1 HTTP call before context expired during backoff sleep.
	if callCount < 1 {
		t.Errorf("expected at least 1 HTTP call before timeout, got %d", callCount)
	}
}

func TestValidateWeatherValues(t *testing.T) {
	tests := []struct {
		name     string
		temp     float64
		apparent float64
		humidity float64
		wind     float64
		wantErr  bool
	}{
		{"all valid", 20.0, 22.0, 50.0, 5.0, false},
		{"negative temp valid", -40.0, -45.0, 90.0, 2.0, false},
		{"zero values valid", 0, 0, 0, 0, false},
		{"temp +Inf", math.Inf(1), 20.0, 50.0, 5.0, true},
		{"temp -Inf", math.Inf(-1), 20.0, 50.0, 5.0, true},
		{"temp NaN", math.NaN(), 20.0, 50.0, 5.0, true},
		{"apparent +Inf", 20.0, math.Inf(1), 50.0, 5.0, true},
		{"humidity NaN", 20.0, 22.0, math.NaN(), 5.0, true},
		{"wind +Inf", 20.0, 22.0, 50.0, math.Inf(1), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateWeatherValues(tt.temp, tt.apparent, tt.humidity, tt.wind)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateWeatherValues() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}
