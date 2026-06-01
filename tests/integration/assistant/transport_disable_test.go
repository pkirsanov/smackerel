//go:build integration

// Spec 072 SCOPE-4 — TP-072-14 / SCN-072-A07.
//
// Integration row that proves disabling the WhatsApp transport via
// the SST flag leaves Telegram and HTTP wiring untouched and that
// no WhatsApp routes are registered on the shared chi router. The
// row drives the same MountWebhookRoutes helper that cmd/core
// invokes, alongside placeholder Telegram and HTTP webhook routes
// that mirror the cmd/core mount pattern, so a regression in the
// SCOPE-4 gating would be observable here BEFORE it reaches the
// production wiring file.
//
// Adversarial guard: the test does NOT pre-register the WhatsApp
// path; if MountWebhookRoutes silently mounted routes even with
// Enabled=false, the GET/POST assertions for the WhatsApp path
// would flip to 200 and fail loudly.

package assistant_integration

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	wa "github.com/smackerel/smackerel/internal/whatsapp/assistant_adapter"
)

// telegramPath and httpPath mirror the cmd/core wiring shape: each
// transport mounts its own webhook handler on the SST-supplied
// path. Disabling one MUST NOT affect the others.
const (
	tp072_14_TelegramPath = "/v1/assistant/transports/telegram/webhook"
	tp072_14_HTTPPath     = "/api/assistant/turn"
	tp072_14_WhatsappPath = "/v1/assistant/transports/whatsapp/webhook"
)

func tp072_14_okHandler(name string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(name + ":ok"))
	})
}

// TestWhatsAppTransportDisable_TP_072_14 — SCN-072-A07.
// With assistant.transports.whatsapp.enabled=false and the other
// two transports wired, MountWebhookRoutes registers no routes,
// the WhatsApp webhook path is unreachable, and Telegram + HTTP
// transports continue to respond 200.
func TestWhatsAppTransportDisable_TP_072_14(t *testing.T) {
	mux := chi.NewRouter()
	// Mount Telegram and HTTP placeholders BEFORE the conditional
	// WhatsApp mount, mirroring cmd/core ordering.
	mux.Method(http.MethodPost, tp072_14_TelegramPath, tp072_14_okHandler("telegram"))
	mux.Method(http.MethodPost, tp072_14_HTTPPath, tp072_14_okHandler("http"))

	mounted, err := wa.MountWebhookRoutes(mux, wa.MountOptions{
		Enabled: false,
		Adapter: nil,
		Path:    tp072_14_WhatsappPath,
	})
	if err != nil {
		t.Fatalf("MountWebhookRoutes(enabled=false): unexpected error %v", err)
	}
	if mounted {
		t.Fatalf("MountWebhookRoutes(enabled=false): mounted=true, want false")
	}

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	cli := srv.Client()

	// (a) Telegram still healthy.
	if resp, err := cli.Post(srv.URL+tp072_14_TelegramPath, "application/json", strings.NewReader("{}")); err != nil {
		t.Fatalf("telegram POST: %v", err)
	} else {
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("telegram status: want 200, got %d", resp.StatusCode)
		}
	}

	// (b) HTTP transport still healthy.
	if resp, err := cli.Post(srv.URL+tp072_14_HTTPPath, "application/json", strings.NewReader("{}")); err != nil {
		t.Fatalf("http POST: %v", err)
	} else {
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("http status: want 200, got %d", resp.StatusCode)
		}
	}

	// (c) WhatsApp ingress removed — chi returns 404 for both GET
	// and POST because no method handlers are registered.
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		req, _ := http.NewRequest(method, srv.URL+tp072_14_WhatsappPath, strings.NewReader(""))
		resp, err := cli.Do(req)
		if err != nil {
			t.Fatalf("whatsapp %s: %v", method, err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("whatsapp %s status: want 404 (route unregistered), got %d", method, resp.StatusCode)
		}
	}
}
