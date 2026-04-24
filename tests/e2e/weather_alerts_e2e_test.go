//go:build e2e

package e2e

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

	"github.com/smackerel/smackerel/internal/connector"
	"github.com/smackerel/smackerel/internal/connector/weather"
	smacknats "github.com/smackerel/smackerel/internal/nats"
)

// TestWeatherAlerts_E2E_FullStack exercises the complete NWS alert path
// against the live NATS broker: an httptest server stands in for the NWS
// upstream so this test does not depend on api.weather.gov, but everything
// past the HTTP boundary is real — the real weather connector, real
// JetStream publish, real subscriber on alerts.notify.
//
// Skips when NATS_URL is not set (live stack unavailable).
func TestWeatherAlerts_E2E_FullStack(t *testing.T) {
	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		t.Skip("e2e: NATS_URL not set — live stack not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	authToken := os.Getenv("SMACKEREL_AUTH_TOKEN")
	c, err := smacknats.Connect(ctx, natsURL, authToken)
	if err != nil {
		t.Fatalf("connect to live NATS: %v", err)
	}
	defer c.Close()

	if _, err := c.JetStream.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:      "ALERTS",
		Subjects:  []string{"alerts.>"},
		Retention: jetstream.LimitsPolicy,
		MaxAge:    7 * 24 * time.Hour,
		Storage:   jetstream.FileStorage,
	}); err != nil {
		t.Fatalf("ensure ALERTS stream: %v", err)
	}

	// Unique alert ID so the assertion is robust against any leftover
	// notifications from prior runs on the same broker.
	alertID := fmt.Sprintf("nws.alert.e2e.%d", time.Now().UnixNano())

	nwsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{
  "features": [
    {
      "properties": {
        "id": %q,
        "event": "Severe Thunderstorm Warning",
        "severity": "Severe",
        "headline": "Severe storm imminent",
        "description": "E2E description",
        "instruction": "Take cover",
        "areaDesc": "E2E County",
        "effective": "2026-04-24T12:00:00-05:00",
        "expires": "2026-04-24T13:00:00-05:00"
      }
    }
  ]
}`, alertID)
	}))
	defer nwsSrv.Close()

	openMeteoSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("daily") != "" {
			fmt.Fprint(w, `{"daily":{"time":["2026-04-24"],"temperature_2m_max":[20],"temperature_2m_min":[10],"weather_code":[0],"precipitation_sum":[0]}}`)
			return
		}
		fmt.Fprint(w, `{"current":{"temperature_2m":18,"apparent_temperature":17,"relative_humidity_2m":50,"wind_speed_10m":5,"weather_code":0,"is_day":1}}`)
	}))
	defer openMeteoSrv.Close()

	conn := weather.New("weather-alerts-e2e")
	conn.SetBaseURL(openMeteoSrv.URL)
	conn.SetNWSURL(nwsSrv.URL)
	conn.SetAlertPublisher(c.Publish, smacknats.SubjectAlertsNotify)

	cfg := connector.ConnectorConfig{
		AuthType: "none",
		Enabled:  true,
		SourceConfig: map[string]interface{}{
			"locations": []interface{}{
				map[string]interface{}{
					"name":      "E2ECity",
					"latitude":  30.27,
					"longitude": -97.74,
				},
			},
			"enable_alerts": true,
			"forecast_days": float64(1),
			"precision":     float64(2),
		},
	}
	if err := conn.Connect(ctx, cfg); err != nil {
		t.Fatalf("weather connect: %v", err)
	}

	// Subscribe BEFORE Sync so we don't miss the publish.
	sub, err := c.Conn.SubscribeSync(smacknats.SubjectAlertsNotify)
	if err != nil {
		t.Fatalf("subscribe alerts.notify: %v", err)
	}
	defer sub.Unsubscribe()

	syncCtx, syncCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer syncCancel()
	artifacts, _, err := conn.Sync(syncCtx, "")
	if err != nil {
		t.Fatalf("sync: %v", err)
	}

	// Verify the weather/alert artifact landed in the Sync result.
	var alertArtifact bool
	for _, a := range artifacts {
		if a.SourceRef == alertID && a.ContentType == "weather/alert" {
			alertArtifact = true
			tier, _ := a.Metadata["processing_tier"].(string)
			if tier != "full" {
				t.Errorf("Severe alert tier = %q, want full", tier)
			}
		}
	}
	if !alertArtifact {
		t.Fatalf("expected weather/alert artifact with id %q in sync output; got %d artifacts", alertID, len(artifacts))
	}

	// Verify the JetStream publish landed on alerts.notify.
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		msg, err := sub.NextMsg(2 * time.Second)
		if err != nil {
			if err == nats.ErrTimeout {
				continue
			}
			t.Fatalf("await alerts.notify: %v", err)
		}
		var notif map[string]interface{}
		if err := json.Unmarshal(msg.Data, &notif); err != nil {
			t.Logf("ignoring malformed notification: %v body=%q", err, string(msg.Data))
			continue
		}
		if notif["alert_id"] != alertID {
			// Foreign notification on shared broker — keep waiting.
			continue
		}
		if notif["severity"] != "Severe" {
			t.Errorf("severity = %v, want Severe", notif["severity"])
		}
		if notif["source"] != "nws" {
			t.Errorf("source = %v, want nws", notif["source"])
		}
		return
	}
	t.Fatalf("no alerts.notify message correlated to alert_id=%s within deadline", alertID)
}
