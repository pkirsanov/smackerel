//go:build integration

// Spec 072 SCOPE-4 — TP-072-15 / SCN-072-A07.
//
// Integration row that proves the WhatsApp transport status
// metrics distinguish disabled, credential-ready, and rejection
// counts.

package monitoring_integration

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	wa "github.com/smackerel/smackerel/internal/whatsapp/assistant_adapter"
)

const (
	tp072_15_Path = "/v1/assistant/transports/whatsapp/webhook"
)

type tp072_15_NoOpRegistry struct{}

func (tp072_15_NoOpRegistry) Resolve(_ context.Context, _, _ string) (string, error) {
	return "user-072-15", nil
}

func scrapeMetric(t *testing.T, name string, labels map[string]string) (float64, bool) {
	t.Helper()
	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	for _, fam := range families {
		if fam.GetName() != name {
			continue
		}
		for _, m := range fam.GetMetric() {
			if !labelsMatch(m.GetLabel(), labels) {
				continue
			}
			if m.Counter != nil {
				return m.Counter.GetValue(), true
			}
			if m.Gauge != nil {
				return m.Gauge.GetValue(), true
			}
		}
	}
	return 0, false
}

func labelsMatch(have []*dto.LabelPair, want map[string]string) bool {
	got := map[string]string{}
	for _, lp := range have {
		got[lp.GetName()] = lp.GetValue()
	}
	for k, v := range want {
		if got[k] != v {
			return false
		}
	}
	return true
}

// TestWhatsAppTransportStatus_TP_072_15 — SCN-072-A07.
// Disabled boot → enabled gauge == 0 AND credentials_ready == 0.
// Enabled boot → enabled gauge == 1 AND credentials_ready == 1.
// One bad-signature POST → webhook_auth_failures{reason=mismatch}
// increments by 1.
func TestWhatsAppTransportStatus_TP_072_15(t *testing.T) {
	// (a) Disabled snapshot.
	if _, err := wa.MountWebhookRoutes(chi.NewRouter(), wa.MountOptions{Enabled: false}); err != nil {
		t.Fatalf("MountWebhookRoutes(false): %v", err)
	}
	enabled, ok := scrapeMetric(t, "assistant_whatsapp_enabled", nil)
	if !ok {
		t.Fatalf("assistant_whatsapp_enabled gauge not registered")
	}
	if enabled != 0 {
		t.Errorf("disabled boot: assistant_whatsapp_enabled = %v, want 0", enabled)
	}
	creds, ok := scrapeMetric(t, "assistant_whatsapp_credentials_ready", nil)
	if !ok {
		t.Fatalf("assistant_whatsapp_credentials_ready gauge not registered")
	}
	if creds != 0 {
		t.Errorf("disabled boot: assistant_whatsapp_credentials_ready = %v, want 0", creds)
	}

	// (b) Enabled snapshot — construct a real adapter and remount.
	const appSecret = "tp-072-15-secret"
	adapter, err := wa.NewAdapter(wa.Options{
		Verify:                    wa.HMACVerifier{AppSecret: appSecret, VerifyToken: "vt"},
		IdentityRegistry:          tp072_15_NoOpRegistry{},
		IdentityHashKey:           "hk",
		MaxTextChars:              4096,
		RateLimitPerUserPerMinute: 30,
	})
	if err != nil {
		t.Fatalf("NewAdapter: %v", err)
	}
	mux := chi.NewRouter()
	mounted, err := wa.MountWebhookRoutes(mux, wa.MountOptions{
		Enabled: true,
		Adapter: adapter,
		Path:    tp072_15_Path,
	})
	if err != nil || !mounted {
		t.Fatalf("MountWebhookRoutes(true): mounted=%v err=%v", mounted, err)
	}
	if v, _ := scrapeMetric(t, "assistant_whatsapp_enabled", nil); v != 1 {
		t.Errorf("enabled boot: assistant_whatsapp_enabled = %v, want 1", v)
	}
	if v, _ := scrapeMetric(t, "assistant_whatsapp_credentials_ready", nil); v != 1 {
		t.Errorf("enabled boot: assistant_whatsapp_credentials_ready = %v, want 1", v)
	}

	// (c) Rejection counter — capture baseline, fire one bad
	// signature, assert exactly +1.
	rejectLabels := map[string]string{"reason": "mismatch"}
	before, _ := scrapeMetric(t, "assistant_whatsapp_webhook_auth_failures_total", rejectLabels)

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	body := []byte(`{"object":"whatsapp_business_account","entry":[]}`)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+tp072_15_Path, bytes.NewReader(body))
	req.Header.Set(wa.SignatureHeader, "sha256=deadbeef") // deliberately wrong
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("bad-signature status: want 401, got %d", resp.StatusCode)
	}
	after, ok := scrapeMetric(t, "assistant_whatsapp_webhook_auth_failures_total", rejectLabels)
	if !ok {
		t.Fatalf("assistant_whatsapp_webhook_auth_failures_total{reason=mismatch} not present after rejection")
	}
	if delta := after - before; delta != 1 {
		t.Errorf("rejection delta: want +1, got %v (before=%v after=%v)", delta, before, after)
	}

	// (d) Adversarial: distinct disabled/credential-error states
	// MUST be visible — credentials_ready=0 while enabled=1 is the
	// configured-but-unconstructed marker. Reset to that state via
	// SetTransportStatus directly.
	wa.SetTransportStatus(true, false)
	en, _ := scrapeMetric(t, "assistant_whatsapp_enabled", nil)
	cr, _ := scrapeMetric(t, "assistant_whatsapp_credentials_ready", nil)
	if en != 1 || cr != 0 {
		t.Errorf("credential-error marker: want enabled=1 creds=0, got enabled=%v creds=%v", en, cr)
	}

	// Restore enabled+ready for subsequent rows.
	wa.SetTransportStatus(true, true)
}
