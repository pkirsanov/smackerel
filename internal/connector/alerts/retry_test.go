// BUG-022-003 — per-source 429/Retry-After regression tests for the gov-alerts
// connector. Each test stands up an httptest server that returns 429 on hit 1
// (with Retry-After: 0 to keep the suite fast) and a valid source-specific
// payload on hit 2; the test asserts exactly two server hits and a successful
// parse on hit 2. Pre-fix (with the inline `if resp.StatusCode != 200`
// pattern) every one of these would fail with hit count == 1.
package alerts

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// quakePayload is a minimal valid USGS GeoJSON event collection.
const quakePayload = `{"features":[{"id":"us123","properties":{"mag":4.2,"place":"Test","time":1700000000000},"geometry":{"coordinates":[10.0,20.0,5.0]}}]}`

// nwsPayload is a minimal valid NWS active-alerts payload.
const nwsPayload = `{"features":[{"properties":{"id":"nws1","event":"Severe Thunderstorm Warning","severity":"severe","areaDesc":"Test County"}}]}`

const tsunamiAtomPayload = `<?xml version="1.0"?><feed xmlns="http://www.w3.org/2005/Atom"><entry><id>t1</id><title>Tsunami Warning</title><summary>x</summary></entry></feed>`

const volcanoPayload = `[{"id":"v1","volcanoName":"Test","alertLevel":"WATCH","colorCode":"YELLOW"}]`

const wildfirePayload = `<?xml version="1.0"?><rss><channel><item><title>Fire</title><description>d</description><link>https://x</link><guid>fire1</guid></item></channel></rss>`

const airnowPayload = `[{"DateObserved":"2026-01-01 ","HourObserved":12,"AQI":42,"ParameterName":"OZONE","ReportingArea":"Test","Category":{"Name":"Good"}}]`

const gdacsPayload = `<?xml version="1.0"?><rss><channel><item><title>Quake</title><description>d</description><link>https://x</link><guid>g1</guid><alertlevel>orange</alertlevel></item></channel></rss>`

// flipTo serves 429 (with Retry-After: 0) on the first hit and the given
// body on every subsequent hit. The returned counter is incremented per hit.
func flipTo(body string, hits *int32) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(hits, 1)
		if n == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}
}

func newRetryAlertsConnector(t *testing.T, serverURL string) *Connector {
	t.Helper()
	c := New("gov-alerts-retry-test")
	c.baseURL = serverURL
	c.nwsBaseURL = serverURL
	c.tsunamiBaseURL = serverURL
	c.volcanoBaseURL = serverURL
	c.wildfireBaseURL = serverURL
	c.airnowBaseURL = serverURL
	c.gdacsBaseURL = serverURL
	c.retryOpts.MaxAttempts = 3
	c.retryOpts.BaseDelay = 5 * time.Millisecond
	c.retryOpts.MaxDelay = 50 * time.Millisecond
	return c
}

// SCN-422-003-F: each of the 7 alerts sources honors 429 via DoWithRetry.
func TestAlertsSources_HonorRetryAfter(t *testing.T) {
	cases := []struct {
		name    string
		payload string
		call    func(c *Connector, ctx context.Context) error
	}{
		{"USGS", quakePayload, func(c *Connector, ctx context.Context) error {
			_, err := c.fetchUSGSEarthquakes(ctx, 2.0)
			return err
		}},
		{"NWS", nwsPayload, func(c *Connector, ctx context.Context) error {
			_, err := c.fetchNWSAlerts(ctx, 40.0, -120.0)
			return err
		}},
		{"Tsunami", tsunamiAtomPayload, func(c *Connector, ctx context.Context) error {
			_, err := c.fetchTsunamiAlerts(ctx)
			return err
		}},
		{"Volcano", volcanoPayload, func(c *Connector, ctx context.Context) error {
			_, err := c.fetchVolcanoAlerts(ctx)
			return err
		}},
		{"Wildfire", wildfirePayload, func(c *Connector, ctx context.Context) error {
			_, err := c.fetchWildfireAlerts(ctx)
			return err
		}},
		{"AirNow", airnowPayload, func(c *Connector, ctx context.Context) error {
			_, err := c.fetchAirNowAQI(ctx, 40.0, -120.0, "test-key")
			return err
		}},
		{"GDACS", gdacsPayload, func(c *Connector, ctx context.Context) error {
			_, err := c.fetchGDACSAlerts(ctx)
			return err
		}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			var hits int32
			srv := httptest.NewServer(flipTo(tc.payload, &hits))
			defer srv.Close()
			c := newRetryAlertsConnector(t, srv.URL)
			if err := tc.call(c, context.Background()); err != nil {
				t.Fatalf("%s: expected recovered fetch, got error: %v", tc.name, err)
			}
			if got := atomic.LoadInt32(&hits); got != 2 {
				t.Fatalf("%s: hits = %d, want 2 (proves retry happened)", tc.name, got)
			}
		})
	}
}
