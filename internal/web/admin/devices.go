// Spec 058 BUG-058-EXTERNAL-INFRA-MISSING (BLOCKER-3) — extension devices admin
// page.
//
// Renders GET /admin/extension/devices as an HTML table on the shared admin
// scaffold. It reuses the EXISTING, certified extensiondevices.Store
// aggregation (the same seam the JSON handler at /v1/admin/extension/devices
// uses) and the SAME AuthGate — so there is no duplicated query logic and no
// second auth primitive. The page is the rendered surface spec 058 design §3.2
// requires; the JSON endpoint remains for programmatic callers.
package admin

import (
	"html/template"
	"net/http"
	"sort"

	"github.com/smackerel/smackerel/internal/api/admin/extensiondevices"
)

const devicesActiveHref = "/admin/extension/devices"

// devicesContent is the inner page template (wrapped by the shared base
// layout). html/template auto-escapes every field, so the user-influenced
// SourceDeviceID / OwnerUserID values are XSS-safe by construction.
const devicesContent = `
<p class="muted">Browser-extension devices that have ingested artifacts, aggregated from <code>raw_ingest_dedup</code>. Times are UTC; the visit count is a rolling 30-day window.</p>
{{if .Devices}}
<table>
  <thead>
    <tr><th>Owner</th><th>Source device</th><th>First seen</th><th>Last seen</th><th>Visits (30d)</th></tr>
  </thead>
  <tbody>
    {{range .Devices}}
    <tr>
      <td>{{.OwnerUserID}}</td>
      <td><code>{{.SourceDeviceID}}</code></td>
      <td>{{.FirstSeenAt.UTC.Format "2006-01-02 15:04 MST"}}</td>
      <td>{{.LastSeenAt.UTC.Format "2006-01-02 15:04 MST"}}</td>
      <td>{{.VisitCount30d}}</td>
    </tr>
    {{end}}
  </tbody>
</table>
{{else}}
<div class="empty">No extension devices have ingested artifacts yet.</div>
{{end}}
`

// devicesView is the data passed to the inner template.
type devicesView struct {
	Devices []extensiondevices.Device
}

// DevicesHandler serves the HTML extension-devices admin page.
type DevicesHandler struct {
	store   extensiondevices.Store
	gate    AuthGate
	base    *template.Template
	content *template.Template
}

// NewDevicesHandler constructs the page handler. Both the store and the auth
// gate are required; a nil argument is a programming error (fail loud, matching
// extensiondevices.NewHandler).
func NewDevicesHandler(store extensiondevices.Store, gate AuthGate) *DevicesHandler {
	if store == nil {
		panic("web/admin: NewDevicesHandler requires a non-nil Store")
	}
	if gate == nil {
		panic("web/admin: NewDevicesHandler requires a non-nil AuthGate")
	}
	return &DevicesHandler{
		store:   store,
		gate:    gate,
		base:    newBaseTemplate(),
		content: template.Must(template.New("admin_devices").Parse(devicesContent)),
	}
}

// ServeHTTP implements http.Handler.
func (h *DevicesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	ownerUserID, admin, ok := h.gate(r)
	if !ok {
		http.Error(w, "bearer auth required", http.StatusUnauthorized)
		return
	}

	// Non-admin callers see ONLY their own devices (same scoping rule the JSON
	// handler enforces). An authenticated non-admin with no owner id is a
	// misconfiguration, not a viewer of every owner's devices.
	filter := ""
	if !admin {
		if ownerUserID == "" {
			http.Error(w, "non-admin session missing owner user id", http.StatusForbidden)
			return
		}
		filter = ownerUserID
	}

	devices, err := h.store.AggregateDevices(r.Context(), filter)
	if err != nil {
		http.Error(w, "devices aggregation failed", http.StatusInternalServerError)
		return
	}
	// Deterministic order: (owner_user_id, source_device_id) — matches the JSON
	// handler so the two surfaces present identically.
	sort.Slice(devices, func(i, j int) bool {
		if devices[i].OwnerUserID != devices[j].OwnerUserID {
			return devices[i].OwnerUserID < devices[j].OwnerUserID
		}
		return devices[i].SourceDeviceID < devices[j].SourceDeviceID
	})

	var inner template.HTML
	if rendered, rerr := renderContent(h.content, devicesView{Devices: devices}); rerr == nil {
		inner = rendered
	} else {
		http.Error(w, "render failed", http.StatusInternalServerError)
		return
	}
	renderPage(w, h.base, "Extension Devices", devicesActiveHref, inner)
}
