package weather

import (
	"context"
	"fmt"
	"math"
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
