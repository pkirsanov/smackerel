// Spec 096 SCOPE-06 — the operator-gated web admin connection surface
// (/v1/admin/model-connections*), the RUNTIME-plane surface that lets the
// operator wire, test, enable, and disable each SST-declared db-mode connection
// slot (design §6.1, §5.1, §11.4).
//
// THREE binding contracts this handler enforces:
//
//   - WRITE-ONLY SECRET (design §6.1, §11.5). `PUT …/credential` is write-only:
//     it encrypts the entered secret through the SCOPE-02 vault and persists
//     the at-rest record, returning ONLY a redacted view (last-4). NO endpoint
//     ever returns, echoes, or logs the plaintext credential; reads expose only
//     secret_present + secret_redaction + the typed last-test state.
//
//   - TRUTHFUL TEST, NEVER A FALSE SUCCESS (design §6.1; SCN-096-W04).
//     `POST …/test` runs a live per-kind reachability+credential probe and
//     records the TRUTHFUL typed outcome (ok | failed) + typed detail
//     (auth_failed | unreachable | timeout). A failed probe persists `failed`
//     and is NEVER reported `ok`, and the handler NEVER substitutes Ollama.
//
//   - CLOSED-SET SLOT + 409 ENABLE-GUARD (design §5.1). An id not in the SST
//     registry is 404 (a brand-new kind is an SST topology edit, not a UI
//     invention). `enable` is refused 409 unless a credential is present AND the
//     last test = ok — no enabling an unverified connection into the catalog.
//
// The operator-only boundary (R1) is enforced by the OperatorGate middleware
// (model_connections_operator_gate.go) mounted in front of this handler.
package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel/attribute"

	"github.com/smackerel/smackerel/internal/assistant/openknowledge/connstore"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/connvault"
	okmetrics "github.com/smackerel/smackerel/internal/assistant/openknowledge/metrics"
	"github.com/smackerel/smackerel/internal/assistant/tracing"
	"github.com/smackerel/smackerel/internal/config"
)

// modelConnStore is the subset of connstore.Store the admin handler consumes.
// Defined here (consumer-side) so the handler is unit-testable with an
// in-memory fake and carries no DB import beyond the connstore types.
type modelConnStore interface {
	Connection(id string) (config.ModelConnection, bool)
	ListDeclared() []config.ModelConnection
	Get(ctx context.Context, connID string) (connstore.Record, bool, error)
	List(ctx context.Context) ([]connstore.Record, error)
	UpsertCredential(ctx context.Context, connID, kind string, rec connvault.VaultRecord) error
	RecordTest(ctx context.Context, connID, kind, outcome, detail string, at time.Time) error
	SetEnabled(ctx context.Context, connID string, enabled bool) error
}

// ProbeResult is the TRUTHFUL outcome of a live per-kind connection probe.
// Outcome is connstore.TestOutcomeOK | connstore.TestOutcomeFailed; Detail is
// the typed reason for a failure ("" for ok, else auth_failed | unreachable |
// timeout). A probe NEVER reports ok for a failed reachability/credential check.
type ProbeResult struct {
	Outcome string
	Detail  string
}

// Typed failure details (design §6.1; SCN-096-W04). A failed probe carries
// exactly one; ok carries none.
const (
	ProbeDetailAuthFailed  = "auth_failed"
	ProbeDetailUnreachable = "unreachable"
	ProbeDetailTimeout     = "timeout"
)

// ConnectionProbe runs a live reachability+credential probe for one connection
// using the DECRYPTED secret bundle. The production implementation
// (HTTPConnectionProbe) probes the provider endpoint; unit tests inject a fake
// to drive the handler's truthful-reporting logic deterministically.
type ConnectionProbe interface {
	Probe(ctx context.Context, conn config.ModelConnection, secret map[string]string) ProbeResult
}

// ModelConnectionsAdminHandler serves /v1/admin/model-connections*. It holds
// the runtime store, the in-core vault (for encrypt-on-write + decrypt-for-probe),
// and the per-kind probe seam. It is mounted behind the OperatorGate.
type ModelConnectionsAdminHandler struct {
	store modelConnStore
	vault *connvault.SecretVault
	probe ConnectionProbe
	now   func() time.Time
	// Spec 096 §13 — connection-test observability seam. recorder is NEVER nil
	// (the constructor defaults it to okmetrics.Nop{}); tracer is nil until
	// WithObservability wires the boot tracer, and a nil tracer makes the
	// model.connection.test span a no-op. The probe decrypts a credential, but
	// nothing emitted through this seam carries a secret (only connection_id /
	// kind / the closed ok|failed outcome).
	recorder okmetrics.Recorder
	tracer   *tracing.Tracer
}

// NewModelConnectionsAdminHandler builds the handler. vault may be nil only for
// an Ollama-only deployment (no db-mode slot) — a db-mode credential write then
// fails loud rather than persisting an unencrypted secret.
func NewModelConnectionsAdminHandler(store modelConnStore, vault *connvault.SecretVault, probe ConnectionProbe) *ModelConnectionsAdminHandler {
	return &ModelConnectionsAdminHandler{store: store, vault: vault, probe: probe, now: time.Now, recorder: okmetrics.Nop{}}
}

// WithNow overrides the clock (tests assert tested_at). Returns the receiver.
func (h *ModelConnectionsAdminHandler) WithNow(now func() time.Time) *ModelConnectionsAdminHandler {
	h.now = now
	return h
}

// connectionTestSpanName is the spec 096 §13 operator connection-test span. Its
// attrs are connection_id/kind/outcome — all secret-free (connection_id is the
// operator-visible slug; kind + outcome are the closed vocab). The probe's
// free-form Detail is DELIBERATELY excluded so no endpoint specifics can leak.
const connectionTestSpanName = "model.connection.test"

// WithObservability wires the spec 096 §13 connection-test observability seam:
// the openknowledge metrics Recorder (the operator connection-test counter) and
// the assistant Tracer (the model.connection.test span). A nil recorder falls
// back to the no-op okmetrics.Nop{}; a nil tracer disables span emission while
// the metric path still runs. Additive + non-blocking — an Ollama-only /
// metrics-disabled deployment keeps the Nop recorder + nil tracer and emits
// nothing. Returns the receiver for chaining (mirrors the catalog aggregator).
//
// SECRET-SAFETY: the Test handler decrypts a credential to probe, but this seam
// only ever receives the connection_id (operator-visible slug), the closed kind,
// and the closed ok|failed outcome — NEVER the api_key, the decrypted bundle, or
// the probe's free-form Detail.
func (h *ModelConnectionsAdminHandler) WithObservability(recorder okmetrics.Recorder, tracer *tracing.Tracer) *ModelConnectionsAdminHandler {
	if recorder == nil {
		recorder = okmetrics.Nop{}
	}
	h.recorder = recorder
	h.tracer = tracer
	return h
}

// modelConnectionView is the read/redacted-write envelope. It carries NO
// plaintext credential field by construction — only the presence flag, the
// last-4 redaction, and the typed last-test state.
type modelConnectionView struct {
	ConnectionID    string         `json:"connection_id"`
	Kind            string         `json:"kind"`
	Enabled         bool           `json:"enabled"`
	Params          map[string]any `json:"params,omitempty"`
	SecretPresent   bool           `json:"secret_present"`
	SecretRedaction string         `json:"secret_redaction,omitempty"`
	LastTestedAt    *time.Time     `json:"last_tested_at,omitempty"`
	LastTestOutcome string         `json:"last_test_outcome,omitempty"`
	ModelCount      int            `json:"model_count"`
}

// putCredentialRequest is the write-only credential body. It carries ONLY the
// kind's secret fields; no connection id and no enabled flag (those are path /
// separate endpoints — the actor and slot are never taken from the body).
type putCredentialRequest struct {
	SecretFields map[string]string `json:"secret_fields"`
}

// testResultView is the POST …/test response.
type testResultView struct {
	Outcome  string    `json:"outcome"`
	Detail   string    `json:"detail,omitempty"`
	TestedAt time.Time `json:"tested_at"`
}

// List handles GET /v1/admin/model-connections — every SST-declared slot with
// its runtime status (enabled, secret presence + last-4, last-test, model
// count). NO secrets. Declared-but-unconfigured slots appear with
// enabled=false / secret_present=false.
func (h *ModelConnectionsAdminHandler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.store.List(r.Context())
	if err != nil {
		slog.Warn("model-connections admin: list failed", "error", err)
		writeError(w, http.StatusInternalServerError, "LIST_FAILED", "could not list model connections")
		return
	}
	byID := make(map[string]connstore.Record, len(rows))
	for _, row := range rows {
		byID[row.ConnectionID] = row
	}
	// Present every SST-declared db-mode slot (the closed registry set),
	// overlaying its runtime row when one exists. A declared-but-unconfigured
	// slot appears with enabled=false / secret_present=false.
	views := make([]modelConnectionView, 0)
	for _, conn := range h.store.ListDeclared() {
		if conn.SecretRef.Mode != config.ModelConnectionSecretModeDB {
			continue
		}
		row, hasRow := byID[conn.ID]
		views = append(views, h.viewFor(conn, row, hasRow))
	}
	writeJSON(w, http.StatusOK, map[string]any{"connections": views})
}

// GetOne handles GET /v1/admin/model-connections/{id} — one slot. 404 when the
// id is not a declared slot (closed-set fail-loud).
func (h *ModelConnectionsAdminHandler) GetOne(w http.ResponseWriter, r *http.Request) {
	conn, ok := h.resolveSlot(w, r)
	if !ok {
		return
	}
	row, found, err := h.store.Get(r.Context(), conn.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "READ_FAILED", "could not read model connection")
		return
	}
	writeJSON(w, http.StatusOK, h.viewFor(conn, row, found))
}

// PutCredential handles PUT /v1/admin/model-connections/{id}/credential — the
// write-only secret store. It encrypts the entered fields and returns the
// REDACTED view; the cleartext is NEVER echoed. 404 unknown/non-db-mode slot;
// 422 missing required secret field for the kind.
func (h *ModelConnectionsAdminHandler) PutCredential(w http.ResponseWriter, r *http.Request) {
	conn, ok := h.resolveDBModeSlot(w, r)
	if !ok {
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 64*1024))
	if err != nil {
		writeError(w, http.StatusBadRequest, "BODY_READ_ERROR", "could not read request body")
		return
	}
	var req putCredentialRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, "BODY_INVALID_JSON", "request body is not valid JSON")
		return
	}
	bundle := nonEmptySecretFields(req.SecretFields)
	if missing := validateSecretFields(conn.Kind, bundle); len(missing) > 0 {
		writeError(w, http.StatusUnprocessableEntity, "MISSING_SECRET_FIELD",
			"missing required secret field(s) for kind "+conn.Kind+": "+strings.Join(missing, ", "))
		return
	}
	if h.vault == nil {
		writeError(w, http.StatusInternalServerError, "VAULT_NOT_CONFIGURED", "credential vault is not configured")
		return
	}
	rec, err := h.vault.Encrypt(conn.ID, conn.Kind, bundle)
	if err != nil {
		// The vault error never carries plaintext; do not echo the bundle.
		slog.Warn("model-connections admin: encrypt failed", "connection", conn.ID, "error", err)
		writeError(w, http.StatusInternalServerError, "ENCRYPT_FAILED", "could not encrypt credential")
		return
	}
	if err := h.store.UpsertCredential(r.Context(), conn.ID, conn.Kind, rec); err != nil {
		slog.Warn("model-connections admin: store credential failed", "connection", conn.ID, "error", err)
		writeError(w, http.StatusInternalServerError, "STORE_FAILED", "could not store credential")
		return
	}
	// Re-read so the redacted view reflects the persisted row (last-test reset).
	row, found, _ := h.store.Get(r.Context(), conn.ID)
	if !found {
		// Construct directly from the record we just wrote (no row read-back).
		row = connstore.Record{ConnectionID: conn.ID, ProviderKind: conn.Kind, Secret: &rec}
	}
	writeJSON(w, http.StatusOK, h.viewFor(conn, row, true))
}

// Test handles POST /v1/admin/model-connections/{id}/test — the live per-kind
// reachability+credential probe. It decrypts the stored credential, probes, and
// persists the TRUTHFUL typed outcome. 404 unknown/non-db-mode slot; 409 when
// no credential is stored (nothing to probe with).
func (h *ModelConnectionsAdminHandler) Test(w http.ResponseWriter, r *http.Request) {
	conn, ok := h.resolveDBModeSlot(w, r)
	if !ok {
		return
	}
	row, found, err := h.store.Get(r.Context(), conn.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "READ_FAILED", "could not read model connection")
		return
	}
	if !found || !row.HasCredential() {
		writeError(w, http.StatusConflict, "CREDENTIAL_REQUIRED", "store a credential before testing the connection")
		return
	}
	if h.vault == nil {
		writeError(w, http.StatusInternalServerError, "VAULT_NOT_CONFIGURED", "credential vault is not configured")
		return
	}
	secret, err := h.vault.Decrypt(*row.Secret)
	if err != nil {
		slog.Warn("model-connections admin: decrypt for test failed", "connection", conn.ID, "error", err)
		writeError(w, http.StatusInternalServerError, "DECRYPT_FAILED", "could not decrypt stored credential")
		return
	}
	result := h.probe.Probe(r.Context(), conn, secret)
	// Normalize defensively: anything that is not an explicit ok is a failure.
	// A probe MUST NEVER yield a false ok (SCN-096-W04) — and a malformed/empty
	// outcome is treated as a failure, never silently passed.
	outcome := connstore.TestOutcomeFailed
	detail := result.Detail
	if result.Outcome == connstore.TestOutcomeOK {
		outcome = connstore.TestOutcomeOK
		detail = ""
	} else if detail == "" {
		detail = ProbeDetailUnreachable
	}
	at := h.now().UTC()
	if err := h.store.RecordTest(r.Context(), conn.ID, conn.Kind, outcome, detail, at); err != nil {
		slog.Warn("model-connections admin: record test failed", "connection", conn.ID, "error", err)
		writeError(w, http.StatusInternalServerError, "RECORD_TEST_FAILED", "could not record test outcome")
		return
	}
	// Spec 096 §13 — emit the connection-test observability AFTER the truthful
	// outcome is recorded. SECRET-SAFETY: only the connection_id (operator slug),
	// the closed kind, and the closed ok|failed outcome are emitted — NEVER the
	// decrypted secret or the probe's free-form Detail (which can carry endpoint
	// specifics). A Nop recorder + nil tracer make this a no-op (additive,
	// non-blocking — the truthful test/record behaviour above is unchanged).
	h.recorder.IncConnectionTest(conn.Kind, outcome)
	if h.tracer != nil {
		_, span := h.tracer.StartSpan(r.Context(), connectionTestSpanName, "", "", "", "", "",
			attribute.String("connection_id", conn.ID),
			attribute.String("kind", conn.Kind),
			attribute.String("outcome", outcome),
		)
		spanStatus, spanErrorCause := "ok", ""
		if outcome != connstore.TestOutcomeOK {
			spanStatus, spanErrorCause = "error", outcome
		}
		tracing.EndSpan(span, spanStatus, spanErrorCause)
	}
	writeJSON(w, http.StatusOK, testResultView{Outcome: outcome, Detail: detail, TestedAt: at})
}

// Enable handles POST /v1/admin/model-connections/{id}/enable — the 409-guarded
// enable. It is refused unless a credential is present AND the last test = ok
// (no enabling an unverified connection into the catalog). 404 unknown/non-db-mode.
func (h *ModelConnectionsAdminHandler) Enable(w http.ResponseWriter, r *http.Request) {
	conn, ok := h.resolveDBModeSlot(w, r)
	if !ok {
		return
	}
	row, found, err := h.store.Get(r.Context(), conn.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "READ_FAILED", "could not read model connection")
		return
	}
	if !found || !row.HasCredential() || row.LastTestOutcome != connstore.TestOutcomeOK {
		writeError(w, http.StatusConflict, "ENABLE_PRECONDITION",
			"cannot enable: the connection needs a stored credential and a passing test first")
		return
	}
	if err := h.store.SetEnabled(r.Context(), conn.ID, true); err != nil {
		writeError(w, http.StatusInternalServerError, "ENABLE_FAILED", "could not enable connection")
		return
	}
	row.Enabled = true
	writeJSON(w, http.StatusOK, h.viewFor(conn, row, true))
}

// Disable handles POST /v1/admin/model-connections/{id}/disable — removes the
// connection's models from the combined catalog. Idempotent: a never-configured
// slot is already disabled. 404 unknown/non-db-mode.
func (h *ModelConnectionsAdminHandler) Disable(w http.ResponseWriter, r *http.Request) {
	conn, ok := h.resolveDBModeSlot(w, r)
	if !ok {
		return
	}
	row, found, err := h.store.Get(r.Context(), conn.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "READ_FAILED", "could not read model connection")
		return
	}
	if !found {
		writeJSON(w, http.StatusOK, h.viewFor(conn, connstore.Record{}, false))
		return
	}
	if err := h.store.SetEnabled(r.Context(), conn.ID, false); err != nil {
		writeError(w, http.StatusInternalServerError, "DISABLE_FAILED", "could not disable connection")
		return
	}
	row.Enabled = false
	writeJSON(w, http.StatusOK, h.viewFor(conn, row, true))
}

// resolveSlot resolves {id} against the closed SST registry. 404 when the id is
// not a declared slot (a UI-invented slot is never accepted).
func (h *ModelConnectionsAdminHandler) resolveSlot(w http.ResponseWriter, r *http.Request) (config.ModelConnection, bool) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	conn, ok := h.store.Connection(id)
	if !ok {
		writeError(w, http.StatusNotFound, "UNKNOWN_SLOT", "no model-connection slot declared for that id")
		return config.ModelConnection{}, false
	}
	return conn, true
}

// resolveDBModeSlot resolves {id} AND requires it be a db-mode slot. A non-db-mode
// slot (ollama / env) does not accept a web-entered credential and is 404.
func (h *ModelConnectionsAdminHandler) resolveDBModeSlot(w http.ResponseWriter, r *http.Request) (config.ModelConnection, bool) {
	conn, ok := h.resolveSlot(w, r)
	if !ok {
		return config.ModelConnection{}, false
	}
	if conn.SecretRef.Mode != config.ModelConnectionSecretModeDB {
		writeError(w, http.StatusNotFound, "NOT_DB_MODE_SLOT", "this connection slot is not credential-managed via the web admin surface")
		return config.ModelConnection{}, false
	}
	return conn, true
}

// viewFor builds the redacted view for a declared slot, overlaying the runtime
// row (when present). It NEVER includes a plaintext credential — only presence,
// the last-4 redaction, and the typed last-test state.
func (h *ModelConnectionsAdminHandler) viewFor(conn config.ModelConnection, row connstore.Record, hasRow bool) modelConnectionView {
	v := modelConnectionView{
		ConnectionID: conn.ID,
		Kind:         conn.Kind,
		Params:       conn.Params,
		ModelCount:   len(conn.Models.List),
	}
	if hasRow {
		v.Enabled = row.Enabled
		v.SecretPresent = row.HasCredential()
		if row.Secret != nil {
			v.SecretRedaction = row.Secret.Redaction
		}
		v.LastTestedAt = row.LastTestedAt
		v.LastTestOutcome = row.LastTestOutcome
	}
	return v
}

// nonEmptySecretFields trims and drops empty values from the entered secret
// fields. The result is the bundle encrypted as one blob; empty values never
// reach the vault.
func nonEmptySecretFields(fields map[string]string) map[string]string {
	out := make(map[string]string, len(fields))
	for k, v := range fields {
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if k != "" && v != "" {
			out[k] = v
		}
	}
	return out
}

// validateSecretFields returns the required secret field name(s) MISSING for a
// kind (design §4 per-kind secret fields). An empty result means the bundle
// satisfies the kind's contract. google accepts EITHER a Gemini api_key OR a
// Vertex service_account JSON (one-of).
func validateSecretFields(kind string, fields map[string]string) []string {
	switch kind {
	case config.ModelConnectionKindAnthropic, config.ModelConnectionKindOpenAI, config.ModelConnectionKindAzureFoundry:
		return missingFields(fields, "api_key")
	case config.ModelConnectionKindBedrock:
		return missingFields(fields, "aws_access_key_id", "aws_secret_access_key")
	case config.ModelConnectionKindGoogle:
		if fields["api_key"] != "" || fields["service_account"] != "" {
			return nil
		}
		return []string{"api_key|service_account"}
	default:
		// ollama / unknown — no web-entered secret fields (the caller already
		// 404s a non-db-mode slot; this is defense in depth).
		return []string{"<kind accepts no web-entered secret>"}
	}
}

// missingFields returns the subset of required keys absent (or empty) from fields.
func missingFields(fields map[string]string, required ...string) []string {
	var missing []string
	for _, k := range required {
		if fields[k] == "" {
			missing = append(missing, k)
		}
	}
	return missing
}
