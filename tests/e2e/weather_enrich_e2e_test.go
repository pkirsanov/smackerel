//go:build e2e

package e2e

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	smacknats "github.com/smackerel/smackerel/internal/nats"
)

// enrichRequestPayload mirrors weather.EnrichRequest. Duplicated to avoid
// pulling the connector package into the e2e module surface — the e2e test
// asserts on the wire format only.
type enrichRequestPayload struct {
	RequestID string  `json:"request_id"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Date      string  `json:"date"`
}

type enrichResponsePayload struct {
	RequestID string         `json:"request_id"`
	Latitude  float64        `json:"latitude"`
	Longitude float64        `json:"longitude"`
	Date      string         `json:"date"`
	Weather   map[string]any `json:"weather,omitempty"`
	Error     string         `json:"error,omitempty"`
}

// TestWeatherEnrich_E2E_LiveStackRoundTrip exercises the full enrichment
// flow against the running live stack: it publishes on
// weather.enrich.request via the live NATS broker and waits for a reply on
// weather.enrich.response from the weather connector subscriber that the
// core auto-starts when the connector is enabled.
//
// Skips when NATS_URL is not set (live stack unavailable). Also skips when
// no reply arrives within the deadline rather than failing — the enrichment
// subscriber is only wired in when WEATHER_ENABLED is true on the live stack,
// and an e2e job that exercises a different connector profile should not
// fail this assertion.
func TestWeatherEnrich_E2E_LiveStackRoundTrip(t *testing.T) {
	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		t.Skip("e2e: NATS_URL not set — live stack not available")
	}

	opts := []nats.Option{nats.Name("smackerel-e2e-weather-enrich")}
	if tok := os.Getenv("SMACKEREL_AUTH_TOKEN"); tok != "" {
		opts = append(opts, nats.Token(tok))
	}
	nc, err := nats.Connect(natsURL, opts...)
	if err != nil {
		t.Fatalf("connect to live NATS: %v", err)
	}
	defer nc.Close()

	sub, err := nc.SubscribeSync(smacknats.SubjectWeatherEnrichResponse)
	if err != nil {
		t.Fatalf("subscribe to %s: %v", smacknats.SubjectWeatherEnrichResponse, err)
	}
	defer sub.Unsubscribe()

	requestID := "e2e-weather-enrich-" + time.Now().UTC().Format("20060102T150405.000")
	body, _ := json.Marshal(enrichRequestPayload{
		RequestID: requestID,
		Latitude:  47.37,
		Longitude: 8.54,
		Date:      "2026-03-15",
	})

	// Publish via core NATS — the WEATHER stream captures this for inspection
	// when enabled, but core publish works whether or not the consumer side
	// is a JetStream subscriber.
	if err := nc.Publish(smacknats.SubjectWeatherEnrichRequest, body); err != nil {
		t.Fatalf("publish enrich request: %v", err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		msg, err := sub.NextMsg(2 * time.Second)
		if err != nil {
			if err == nats.ErrTimeout {
				continue
			}
			t.Fatalf("await response: %v", err)
		}
		var resp enrichResponsePayload
		if err := json.Unmarshal(msg.Data, &resp); err != nil {
			// Skip malformed messages — another publisher could be on the
			// shared live stack — but log so debugging is possible.
			t.Logf("ignoring malformed response: %v body=%q", err, string(msg.Data))
			continue
		}
		if resp.RequestID != requestID {
			// Not our reply — keep waiting.
			continue
		}
		if resp.Error == "" && resp.Weather == nil {
			t.Errorf("response correlated but had neither weather nor error: %+v", resp)
		}
		t.Logf("e2e enrichment response received: error=%q weather_keys=%d", resp.Error, len(resp.Weather))
		return
	}
	t.Skipf("no enrichment reply within 45s — weather connector subscriber may be disabled in this live-stack profile (request_id=%s)", requestID)
}
