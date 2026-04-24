//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/smackerel/smackerel/internal/connector"
	"github.com/smackerel/smackerel/internal/connector/weather"
	smacknats "github.com/smackerel/smackerel/internal/nats"
)

// weatherAlertsTestClient connects to live NATS, ensures the ALERTS stream,
// and returns a *smacknats.Client. Skips the test if NATS_URL is unset.
func weatherAlertsTestClient(t *testing.T) *smacknats.Client {
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

	if _, err := c.JetStream.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:      "ALERTS",
		Subjects:  []string{"alerts.>"},
		Retention: jetstream.LimitsPolicy,
		MaxAge:    7 * 24 * time.Hour,
		Storage:   jetstream.FileStorage,
	}); err != nil {
		t.Fatalf("ensure ALERTS stream: %v", err)
	}
	return c
}

// nwsAlertsBody returns a stub NWS GeoJSON payload with a single alert at
// the given severity and id so each test can craft the upstream response it
// expects.
func nwsAlertsBody(id, severity, event string) string {
	return fmt.Sprintf(`{
  "features": [
    {
      "properties": {
        "id": %q,
        "event": %q,
        "severity": %q,
        "headline": %q,
        "description": "stub description",
        "instruction": "stub instruction",
        "areaDesc": "Test County",
        "effective": "2026-04-24T12:00:00-05:00",
        "expires": "2026-04-24T13:00:00-05:00"
      }
    }
  ]
}`, id, event, severity, event+" issued")
}

func connectWeather(t *testing.T, conn *weather.Connector, locName string) {
	t.Helper()
	cfg := connector.ConnectorConfig{
		AuthType: "none",
		Enabled:  true,
		SourceConfig: map[string]interface{}{
			"locations": []interface{}{
				map[string]interface{}{
					"name":      locName,
					"latitude":  30.27,
					"longitude": -97.74,
				},
			},
			"enable_alerts": true,
			"forecast_days": float64(1),
			"precision":     float64(2),
		},
	}
	if err := conn.Connect(context.Background(), cfg); err != nil {
		t.Fatalf("weather connect: %v", err)
	}
}

// TestWeatherAlerts_PublishedToAlertsNotify verifies that a high-severity
// (Extreme) NWS alert flowing through the connector during Sync is published
// on the alerts.notify subject via real NATS.
func TestWeatherAlerts_PublishedToAlertsNotify(t *testing.T) {
	c := weatherAlertsTestClient(t)

	// NWS upstream stub returns one Extreme alert.
	nwsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, nwsAlertsBody("nws.alert.extreme.int1", "Extreme", "Tornado Warning"))
	}))
	defer nwsSrv.Close()

	// Open-Meteo upstream stub returns minimally valid current+forecast so
	// Sync does not error out on the non-alert path.
	openMeteoSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("daily") != "" {
			fmt.Fprint(w, `{"daily":{"time":["2026-04-24"],"temperature_2m_max":[20],"temperature_2m_min":[10],"weather_code":[0],"precipitation_sum":[0]}}`)
			return
		}
		fmt.Fprint(w, `{"current":{"temperature_2m":18,"apparent_temperature":17,"relative_humidity_2m":50,"wind_speed_10m":5,"weather_code":0,"is_day":1}}`)
	}))
	defer openMeteoSrv.Close()

	conn := weather.New("weather-alerts-int-published")
	conn.SetBaseURL(openMeteoSrv.URL)
	conn.SetNWSURL(nwsSrv.URL)
	conn.SetAlertPublisher(c.Publish, smacknats.SubjectAlertsNotify)
	connectWeather(t, conn, "AustinTX")

	// Subscribe to alerts.notify before triggering Sync.
	sub, err := c.Conn.SubscribeSync(smacknats.SubjectAlertsNotify)
	if err != nil {
		t.Fatalf("subscribe alerts.notify: %v", err)
	}
	defer sub.Unsubscribe()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, _, err := conn.Sync(ctx, ""); err != nil {
		t.Fatalf("sync: %v", err)
	}

	msg, err := sub.NextMsg(10 * time.Second)
	if err != nil {
		t.Fatalf("await alerts.notify: %v", err)
	}
	var notif map[string]interface{}
	if err := json.Unmarshal(msg.Data, &notif); err != nil {
		t.Fatalf("decode notification: %v", err)
	}
	if notif["alert_id"] != "nws.alert.extreme.int1" {
		t.Errorf("alert_id = %v, want nws.alert.extreme.int1", notif["alert_id"])
	}
	if notif["severity"] != "Extreme" {
		t.Errorf("severity = %v, want Extreme", notif["severity"])
	}
	if notif["source"] != "nws" {
		t.Errorf("source = %v, want nws", notif["source"])
	}
}

// TestWeatherAlerts_DedupBlocksRepeatedAlertID asserts that re-syncing with
// the same upstream alert ID does not produce a second notification.
func TestWeatherAlerts_DedupBlocksRepeatedAlertID(t *testing.T) {
	c := weatherAlertsTestClient(t)

	var hits int32
	nwsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		fmt.Fprint(w, nwsAlertsBody("nws.alert.extreme.dedup", "Extreme", "Tornado Warning"))
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

	conn := weather.New("weather-alerts-int-dedup")
	conn.SetBaseURL(openMeteoSrv.URL)
	conn.SetNWSURL(nwsSrv.URL)
	conn.SetAlertPublisher(c.Publish, smacknats.SubjectAlertsNotify)
	connectWeather(t, conn, "DallasTX")

	sub, err := c.Conn.SubscribeSync(smacknats.SubjectAlertsNotify)
	if err != nil {
		t.Fatalf("subscribe alerts.notify: %v", err)
	}
	defer sub.Unsubscribe()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for i := 0; i < 2; i++ {
		if _, _, err := conn.Sync(ctx, ""); err != nil {
			t.Fatalf("sync %d: %v", i, err)
		}
	}

	// First sync should have published exactly one notification; second sync
	// should publish nothing because the dedup map blocks the repeat ID.
	msg, err := sub.NextMsg(5 * time.Second)
	if err != nil {
		t.Fatalf("await first notify: %v", err)
	}
	var notif map[string]interface{}
	if err := json.Unmarshal(msg.Data, &notif); err != nil {
		t.Fatalf("decode notification: %v", err)
	}
	if notif["alert_id"] != "nws.alert.extreme.dedup" {
		t.Errorf("alert_id = %v, want nws.alert.extreme.dedup", notif["alert_id"])
	}

	// Drain — must NOT receive a second message for the same ID.
	if msg, err := sub.NextMsg(2 * time.Second); err == nil {
		t.Errorf("dedup violated: received second notification: %s", string(msg.Data))
	} else if err != nats.ErrTimeout {
		t.Fatalf("unexpected error draining: %v", err)
	}
}

// TestWeatherAlerts_LowSeverityNotPublishedToNotify proves that a Moderate
// alert produces an artifact (not asserted here) but is NOT published on
// alerts.notify. This is the negative regression for the
// "only Extreme/Severe go to alerts.notify" requirement — without the
// isHighSeverity gate the test would receive a Moderate-tagged notification.
func TestWeatherAlerts_LowSeverityNotPublishedToNotify(t *testing.T) {
	c := weatherAlertsTestClient(t)

	nwsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, nwsAlertsBody("nws.alert.moderate.int1", "Moderate", "Flood Advisory"))
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

	conn := weather.New("weather-alerts-int-lowsev")
	conn.SetBaseURL(openMeteoSrv.URL)
	conn.SetNWSURL(nwsSrv.URL)
	conn.SetAlertPublisher(c.Publish, smacknats.SubjectAlertsNotify)
	connectWeather(t, conn, "HoustonTX")

	sub, err := c.Conn.SubscribeSync(smacknats.SubjectAlertsNotify)
	if err != nil {
		t.Fatalf("subscribe alerts.notify: %v", err)
	}
	defer sub.Unsubscribe()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	artifacts, _, err := conn.Sync(ctx, "")
	if err != nil {
		t.Fatalf("sync: %v", err)
	}

	// Confirm the Moderate alert artifact WAS produced (so we know the
	// connector ran the alert path, not skipped it entirely).
	var sawModerateArtifact bool
	for _, a := range artifacts {
		if a.ContentType == "weather/alert" && a.SourceRef == "nws.alert.moderate.int1" {
			sawModerateArtifact = true
			tier, _ := a.Metadata["processing_tier"].(string)
			if tier != "standard" {
				t.Errorf("Moderate alert tier = %q, want standard", tier)
			}
		}
	}
	if !sawModerateArtifact {
		t.Fatalf("expected weather/alert artifact for Moderate alert; got artifacts=%+v", artifacts)
	}

	// Must NOT have received a notify message for the Moderate alert.
	if msg, err := sub.NextMsg(3 * time.Second); err == nil {
		t.Errorf("Moderate alert was published to alerts.notify (expected only Extreme/Severe): %s", string(msg.Data))
	} else if err != nats.ErrTimeout {
		t.Fatalf("unexpected error: %v", err)
	}
}
