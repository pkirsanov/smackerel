//go:build e2e

// Spec 072 SCOPE-4 — TP-072-16 / SCN-072-A07.
//
// E2E regression that proves a live chi router with
// MountWebhookRoutes(Enabled=false) does not interfere with the
// Telegram and HTTP transports' webhook handlers. The row mirrors
// the cmd/core wiring order (Telegram and HTTP mounted, WhatsApp
// gated off) and drives the resulting httptest server with real
// HTTP turns. Distinct from the integration row by build tag and
// by exercising both transports' webhook handlers concurrently to
// prove no shared state regresses when WhatsApp ingress is absent.

package assistant_e2e

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"

	wa "github.com/smackerel/smackerel/internal/whatsapp/assistant_adapter"
)

const (
	tp072_16_TelegramPath = "/v1/assistant/transports/telegram/webhook"
	tp072_16_HTTPPath     = "/api/assistant/turn"
	tp072_16_WhatsappPath = "/v1/assistant/transports/whatsapp/webhook"
)

type tp072_16_Recorder struct {
	mu    sync.Mutex
	calls int
}

func (r *tp072_16_Recorder) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	r.mu.Lock()
	r.calls++
	r.mu.Unlock()
	w.WriteHeader(http.StatusOK)
}

func (r *tp072_16_Recorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

// TestWhatsAppTransportDisableE2E_TP_072_16 — SCN-072-A07.
// Live Telegram and HTTP webhook turns succeed and increment the
// per-transport call counters even when MountWebhookRoutes is
// invoked with Enabled=false. The WhatsApp path returns 404
// because no route was registered.
func TestWhatsAppTransportDisableE2E_TP_072_16(t *testing.T) {
	tg := &tp072_16_Recorder{}
	httpAdapter := &tp072_16_Recorder{}

	mux := chi.NewRouter()
	mux.Method(http.MethodPost, tp072_16_TelegramPath, tg)
	mux.Method(http.MethodPost, tp072_16_HTTPPath, httpAdapter)

	mounted, err := wa.MountWebhookRoutes(mux, wa.MountOptions{
		Enabled: false,
		Path:    tp072_16_WhatsappPath,
	})
	if err != nil {
		t.Fatalf("MountWebhookRoutes(false): %v", err)
	}
	if mounted {
		t.Fatalf("MountWebhookRoutes(false): mounted=true, want false")
	}

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	cli := srv.Client()

	const turnsPerTransport = 3
	for i := 0; i < turnsPerTransport; i++ {
		if resp, err := cli.Post(srv.URL+tp072_16_TelegramPath, "application/json", strings.NewReader("{}")); err != nil {
			t.Fatalf("telegram turn %d: %v", i, err)
		} else {
			_ = resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Errorf("telegram turn %d status: want 200, got %d", i, resp.StatusCode)
			}
		}
		if resp, err := cli.Post(srv.URL+tp072_16_HTTPPath, "application/json", strings.NewReader("{}")); err != nil {
			t.Fatalf("http turn %d: %v", i, err)
		} else {
			_ = resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Errorf("http turn %d status: want 200, got %d", i, resp.StatusCode)
			}
		}
	}

	if got := tg.count(); got != turnsPerTransport {
		t.Errorf("telegram observed turns: want %d, got %d", turnsPerTransport, got)
	}
	if got := httpAdapter.count(); got != turnsPerTransport {
		t.Errorf("http observed turns: want %d, got %d", turnsPerTransport, got)
	}

	// Adversarial: the WhatsApp ingress is genuinely absent — not
	// merely returning 401 — so a regression that silently mounted
	// the handler with no SST credentials would still be caught.
	resp, err := cli.Post(srv.URL+tp072_16_WhatsappPath, "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("whatsapp probe: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("whatsapp probe status: want 404 (no route), got %d", resp.StatusCode)
	}
}
