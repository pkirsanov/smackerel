// Spec 072 SCOPE-4 — WhatsApp ingress mount helper. Centralizes
// the conditional route registration so cmd/core wiring and the
// disable/status regression tests exercise the same gating path:
// when assistant.transports.whatsapp.enabled is false, NO chi
// routes are registered on the supplied mux and the operator-status
// gauges report 0; when true, the GET+POST webhook routes mount
// and the gauges report 1. The helper is the single SST-respecting
// boundary between cmd/core and the adapter so an integration row
// can prove disabling WhatsApp leaves Telegram and HTTP wiring
// untouched.

package assistant_adapter

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// MountOptions is the input for MountWebhookRoutes. Enabled is the
// SST-resolved assistant.transports.whatsapp.enabled value; when
// false the helper updates the status gauges only and returns
// without touching the mux. When true the Adapter and Path MUST
// both be supplied; passing nil/empty is a wiring error.
type MountOptions struct {
	Enabled bool
	Adapter *Adapter
	Path    string
}

// MountWebhookRoutes registers (or omits) the WhatsApp webhook
// routes on the supplied chi.Mux and updates the operator-status
// gauges. It is the single SST-respecting boundary between cmd/core
// and the adapter; disabling WhatsApp in config MUST leave Telegram
// and HTTP wiring untouched, which is mechanically enforced by
// this helper returning early without touching the mux when
// Enabled=false.
//
// Returns (mounted, err): mounted=true exactly when the GET+POST
// routes were registered.
func MountWebhookRoutes(mux *chi.Mux, opts MountOptions) (bool, error) {
	if !opts.Enabled {
		SetTransportStatus(false, false)
		return false, nil
	}
	if mux == nil {
		SetTransportStatus(true, false)
		return false, errors.New("whatsapp_adapter: MountWebhookRoutes requires chi.Mux when enabled=true")
	}
	if opts.Adapter == nil {
		SetTransportStatus(true, false)
		return false, errors.New("whatsapp_adapter: MountWebhookRoutes requires non-nil Adapter when enabled=true")
	}
	if opts.Path == "" {
		SetTransportStatus(true, false)
		return false, errors.New("whatsapp_adapter: MountWebhookRoutes requires non-empty Path when enabled=true")
	}
	handler := NewWebhookHandler(WebhookHandlerOptions{Adapter: opts.Adapter})
	mux.Method(http.MethodGet, opts.Path, handler)
	mux.Method(http.MethodPost, opts.Path, handler)
	SetTransportStatus(true, true)
	return true, nil
}
