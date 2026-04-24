//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/smackerel/smackerel/internal/connector/weather"
	smacknats "github.com/smackerel/smackerel/internal/nats"
)

// weatherEnrichTestClient connects to live NATS, ensures the WEATHER stream,
// and returns a *smacknats.Client. Skips the test if NATS_URL is unset.
func weatherEnrichTestClient(t *testing.T) *smacknats.Client {
	t.Helper()
	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		t.Skip("integration: NATS_URL not set — live stack not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	authToken := os.Getenv("SMACKEREL_AUTH_TOKEN")
	c, err := smacknats.Connect(ctx, natsURL, authToken)
	if err != nil {
		t.Fatalf("connect to test NATS: %v", err)
	}
	t.Cleanup(c.Close)

	// Ensure the WEATHER stream exists with weather.> pattern. Use
	// CreateOrUpdateStream so this test is idempotent if the stream was
	// already provisioned by EnsureStreams in a previous run.
	if _, err := c.JetStream.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:      "WEATHER",
		Subjects:  []string{"weather.>"},
		Retention: jetstream.WorkQueuePolicy,
		MaxAge:    7 * 24 * time.Hour,
		Storage:   jetstream.FileStorage,
	}); err != nil {
		t.Fatalf("ensure WEATHER stream: %v", err)
	}
	return c
}

// subscribeEnrichResponses creates a synchronous core-NATS subscription on
// weather.enrich.response. Tests use core-NATS subscribe (not a JetStream
// consumer) because the subscriber under test publishes to JetStream, which
// fans out to all core-NATS subscribers attached to the subject.
func subscribeEnrichResponses(t *testing.T, c *smacknats.Client) *nats.Subscription {
	t.Helper()
	sub, err := c.Conn.SubscribeSync(smacknats.SubjectWeatherEnrichResponse)
	if err != nil {
		t.Fatalf("subscribe to %s: %v", smacknats.SubjectWeatherEnrichResponse, err)
	}
	t.Cleanup(func() { _ = sub.Unsubscribe() })
	return sub
}

// TestWeatherEnrich_Integration_RoundTrip publishes a request on
// weather.enrich.request and asserts the corresponding response arrives on
// weather.enrich.response with the same request_id and a populated weather
// payload. The archive backend is an httptest server so this test does not
// depend on the public Open-Meteo API.
func TestWeatherEnrich_Integration_RoundTrip(t *testing.T) {
	c := weatherEnrichTestClient(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"daily":{"time":["2026-03-15"],"temperature_2m_max":[14],"temperature_2m_min":[6],"weather_code":[65],"precipitation_sum":[8]}}`)
	}))
	defer srv.Close()

	conn := weather.New("weather-enrich-int-roundtrip")
	conn.SetArchiveURL(srv.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	sub, err := weather.StartEnrichmentSubscriber(ctx, c, conn)
	if err != nil {
		t.Fatalf("start enrichment subscriber: %v", err)
	}
	defer sub.Unsubscribe()

	respSub := subscribeEnrichResponses(t, c)

	reqBody, _ := json.Marshal(weather.EnrichRequest{
		RequestID: "int-roundtrip",
		Latitude:  47.37,
		Longitude: 8.54,
		Date:      "2026-03-15",
	})
	if err := c.Publish(ctx, smacknats.SubjectWeatherEnrichRequest, reqBody); err != nil {
		t.Fatalf("publish enrich request: %v", err)
	}

	msg, err := respSub.NextMsg(10 * time.Second)
	if err != nil {
		t.Fatalf("await enrich response: %v", err)
	}
	var resp weather.EnrichResponse
	if err := json.Unmarshal(msg.Data, &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.RequestID != "int-roundtrip" {
		t.Errorf("request_id = %q, want %q", resp.RequestID, "int-roundtrip")
	}
	if resp.Error != "" {
		t.Errorf("unexpected error: %q", resp.Error)
	}
	if resp.Weather == nil || resp.Weather.WeatherCode != 65 {
		t.Errorf("weather payload missing or wrong code: %+v", resp.Weather)
	}
}

// TestWeatherEnrich_Integration_CacheReuse verifies that two requests for the
// same date+location produce identical responses while only one upstream
// archive call is made.
func TestWeatherEnrich_Integration_CacheReuse(t *testing.T) {
	c := weatherEnrichTestClient(t)

	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"daily":{"time":["2026-03-15"],"temperature_2m_max":[10],"temperature_2m_min":[2],"weather_code":[3],"precipitation_sum":[0]}}`)
	}))
	defer srv.Close()

	conn := weather.New("weather-enrich-int-cache")
	conn.SetArchiveURL(srv.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	sub, err := weather.StartEnrichmentSubscriber(ctx, c, conn)
	if err != nil {
		t.Fatalf("start enrichment subscriber: %v", err)
	}
	defer sub.Unsubscribe()

	respSub := subscribeEnrichResponses(t, c)

	for i, id := range []string{"cache-a", "cache-b"} {
		body, _ := json.Marshal(weather.EnrichRequest{
			RequestID: id,
			Latitude:  47.37,
			Longitude: 8.54,
			Date:      "2026-03-15",
		})
		if err := c.Publish(ctx, smacknats.SubjectWeatherEnrichRequest, body); err != nil {
			t.Fatalf("publish request %d: %v", i, err)
		}
		msg, err := respSub.NextMsg(10 * time.Second)
		if err != nil {
			t.Fatalf("await response %d: %v", i, err)
		}
		var resp weather.EnrichResponse
		if err := json.Unmarshal(msg.Data, &resp); err != nil {
			t.Fatalf("decode response %d: %v", i, err)
		}
		if resp.RequestID != id {
			t.Errorf("response %d: request_id = %q, want %q", i, resp.RequestID, id)
		}
		if resp.Weather == nil || resp.Weather.WeatherCode != 3 {
			t.Errorf("response %d: unexpected weather %+v", i, resp.Weather)
		}
	}

	if hits != 1 {
		t.Errorf("expected exactly 1 upstream archive call across 2 requests (cache reuse), got %d", hits)
	}
}

// TestWeatherEnrich_Integration_InvalidRequestErrorPath publishes a bad
// payload and asserts the subscriber publishes a structured error response
// rather than dropping the message. This is the live-stack regression for
// "invalid request payloads produce error response" — without the validation
// step in handleEnrichmentRequest the subscriber would either time out
// against a real fetch attempt or emit fetch_failed instead.
func TestWeatherEnrich_Integration_InvalidRequestErrorPath(t *testing.T) {
	c := weatherEnrichTestClient(t)

	conn := weather.New("weather-enrich-int-invalid")
	// Point at a closed port so any accidental fetch fails fast and
	// distinguishably from the validation error code.
	conn.SetArchiveURL("http://127.0.0.1:1")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	sub, err := weather.StartEnrichmentSubscriber(ctx, c, conn)
	if err != nil {
		t.Fatalf("start enrichment subscriber: %v", err)
	}
	defer sub.Unsubscribe()

	respSub := subscribeEnrichResponses(t, c)

	if err := c.Publish(ctx, smacknats.SubjectWeatherEnrichRequest, []byte(`{"request_id":"bad-int","latitude":0,"longitude":0,"date":"yesterday"}`)); err != nil {
		t.Fatalf("publish bad request: %v", err)
	}

	msg, err := respSub.NextMsg(10 * time.Second)
	if err != nil {
		t.Fatalf("await error response: %v", err)
	}
	var resp weather.EnrichResponse
	if err := json.Unmarshal(msg.Data, &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.RequestID != "bad-int" {
		t.Errorf("request_id = %q, want %q", resp.RequestID, "bad-int")
	}
	if resp.Error == "" {
		t.Error("expected non-empty error code on invalid request")
	}
	if resp.Weather != nil {
		t.Errorf("weather should be nil on error, got %+v", resp.Weather)
	}
}
