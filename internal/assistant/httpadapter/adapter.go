package httpadapter

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5/middleware"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
	"github.com/smackerel/smackerel/internal/auth"
)

// CaptureFn is the local capture path the adapter MUST invoke
// exactly once when AssistantResponse.CaptureRoute is true. The
// signature mirrors the Telegram adapter so the same shared response
// model drives both transports.
type CaptureFn func(ctx context.Context, userID, transportMessageID, text string)

// Options is the constructor input for NewHTTPAdapter. Every field
// is required (the constructor returns an error when any required
// dependency is nil or zero). No defaults are synthesized — SST
// per smackerel-no-defaults.
type Options struct {
	Facade  contracts.Assistant
	Capture CaptureFn
	Clock   func() time.Time
	Config  HTTPTransportConfig
}

// HTTPAdapter is the spec 069 concrete contracts.TransportAdapter
// implementation. It exposes an http.Handler for the
// POST /api/assistant/turn route; Translate/Render satisfy the
// interface for parity tests that walk both transports through the
// same code path.
type HTTPAdapter struct {
	facade  contracts.Assistant
	capture CaptureFn
	now     func() time.Time
	cfg     HTTPTransportConfig
	dedup   *turnResponseCache
}

// NewHTTPAdapter constructs the adapter and validates dependencies
// fail-loud. The capture path is REQUIRED — silent drops of
// CaptureRoute responses are a BS-001 regression.
func NewHTTPAdapter(opts Options) (*HTTPAdapter, error) {
	if opts.Facade == nil {
		return nil, errors.New("httpadapter: Facade is required")
	}
	if opts.Capture == nil {
		return nil, errors.New("httpadapter: Capture is required")
	}
	if opts.Clock == nil {
		return nil, errors.New("httpadapter: Clock is required")
	}
	if opts.Config.SchemaVersion == "" {
		return nil, errors.New("httpadapter: Config.SchemaVersion is required")
	}
	if opts.Config.SchemaVersion != SchemaVersionV1 {
		return nil, fmt.Errorf("httpadapter: Config.SchemaVersion %q does not match wire constant %q", opts.Config.SchemaVersion, SchemaVersionV1)
	}
	if len(opts.Config.TransportHintAllowlist) == 0 {
		return nil, errors.New("httpadapter: Config.TransportHintAllowlist must be non-empty")
	}
	if opts.Config.ConversationTTL <= 0 {
		return nil, errors.New("httpadapter: Config.ConversationTTL must be positive")
	}
	dedup, err := newTurnResponseCache(HTTPTurnDedupCapacity, opts.Config.ConversationTTL, opts.Clock)
	if err != nil {
		return nil, err
	}
	return &HTTPAdapter{
		facade:  opts.Facade,
		capture: opts.Capture,
		now:     opts.Clock,
		cfg:     opts.Config,
		dedup:   dedup,
	}, nil
}

// Name implements contracts.TransportAdapter.
func (a *HTTPAdapter) Name() string { return TransportName }

// Translate converts a *TurnRequest payload into a canonical
// AssistantMessage. The user id MUST be resolved by the route
// handler (from bearer-auth context) and passed via the payload
// wrapper; this method does not consult HTTP-side state.
type translatePayload struct {
	UserID  string
	Request *TurnRequest
}

// Translate implements contracts.TransportAdapter. Accepts a
// *translatePayload constructed by the route handler.
func (a *HTTPAdapter) Translate(_ context.Context, payload contracts.TransportPayload) (contracts.AssistantMessage, error) {
	p, ok := payload.(*translatePayload)
	if !ok || p == nil || p.Request == nil {
		return contracts.AssistantMessage{}, errors.New("httpadapter.Translate: payload must be *translatePayload with a non-nil Request")
	}
	req := p.Request
	if err := req.Validate(a.cfg); err != nil {
		return contracts.AssistantMessage{}, err
	}
	msg := contracts.AssistantMessage{
		UserID:               p.UserID,
		Transport:            TransportName,
		TransportMessageID:   req.TransportMessageID,
		Text:                 req.Text,
		Kind:                 contracts.MessageKind(req.Kind),
		ConfirmRef:           req.ConfirmRef,
		DisambiguationRef:    req.DisambiguationRef,
		DisambiguationChoice: req.DisambiguationChoice,
		ReceivedAt:           a.now(),
	}
	if req.ConfirmChoice != "" {
		msg.ConfirmChoice = contracts.ConfirmChoice(req.ConfirmChoice)
	}
	if req.TransportHint != "" {
		msg.TransportMetadata = map[string]string{"transport_hint": req.TransportHint}
	}
	return msg, nil
}

// Render implements contracts.TransportAdapter. The HTTP adapter
// renders responses synchronously through ServeHTTP; this method is
// a no-op so the interface stays satisfied for parity tests that
// invoke every adapter through the same loop.
func (a *HTTPAdapter) Render(_ context.Context, _ contracts.TransportIdentity, _ contracts.AssistantResponse) error {
	return nil
}

// Identity implements contracts.TransportAdapter.
func (a *HTTPAdapter) Identity(_ context.Context, payload contracts.TransportPayload) (contracts.TransportIdentity, error) {
	p, ok := payload.(*translatePayload)
	if !ok || p == nil {
		return contracts.TransportIdentity{}, errors.New("httpadapter.Identity: payload must be *translatePayload")
	}
	return contracts.TransportIdentity{UserID: p.UserID, Transport: TransportName}, nil
}

// Start implements contracts.TransportAdapter. The HTTP adapter is
// driven by the chi router, not a background loop; Start records
// the bound facade and returns.
func (a *HTTPAdapter) Start(_ context.Context, _ contracts.Assistant) error { return nil }

// Stop implements contracts.TransportAdapter.
func (a *HTTPAdapter) Stop(_ context.Context) error { return nil }

// Validate enforces the wire schema v1 contract. Errors carry a
// stable, redaction-safe message so HTTP handlers can map them to
// 400 responses without leaking input bytes.
func (r *TurnRequest) Validate(cfg HTTPTransportConfig) error {
	if r.SchemaVersion != SchemaVersionV1 {
		return fmt.Errorf("invalid_assistant_turn: schema_version must be %q, got %q", SchemaVersionV1, r.SchemaVersion)
	}
	if strings.TrimSpace(r.TransportMessageID) == "" {
		return errors.New("invalid_assistant_turn: transport_message_id is required")
	}
	kindOK := false
	for _, k := range allowedKinds {
		if string(k) == r.Kind {
			kindOK = true
			break
		}
	}
	if !kindOK {
		return fmt.Errorf("invalid_assistant_turn: kind must be one of text|confirm|disambiguation|reset, got %q", r.Kind)
	}
	switch contracts.MessageKind(r.Kind) {
	case contracts.KindText:
		if strings.TrimSpace(r.Text) == "" {
			return errors.New("invalid_assistant_turn: text is required when kind=text")
		}
	case contracts.KindConfirm:
		if strings.TrimSpace(r.ConfirmRef) == "" {
			return errors.New("invalid_assistant_turn: confirm_ref is required when kind=confirm")
		}
		if r.ConfirmChoice != string(contracts.ConfirmPositive) && r.ConfirmChoice != string(contracts.ConfirmNegative) {
			return fmt.Errorf("invalid_assistant_turn: confirm_choice must be %q or %q when kind=confirm, got %q",
				contracts.ConfirmPositive, contracts.ConfirmNegative, r.ConfirmChoice)
		}
	case contracts.KindDisambiguation:
		if strings.TrimSpace(r.DisambiguationRef) == "" {
			return errors.New("invalid_assistant_turn: disambiguation_ref is required when kind=disambiguation")
		}
		if r.DisambiguationChoice < 1 {
			return fmt.Errorf("invalid_assistant_turn: disambiguation_choice must be >= 1 when kind=disambiguation, got %d", r.DisambiguationChoice)
		}
	case contracts.KindReset:
		// No additional fields required.
	}
	if r.TransportHint != "" {
		ok := false
		for _, allowed := range cfg.TransportHintAllowlist {
			if allowed == r.TransportHint {
				ok = true
				break
			}
		}
		if !ok {
			return fmt.Errorf("invalid_assistant_turn: transport_hint %q is not in the allowlist", r.TransportHint)
		}
	}
	return nil
}

// RenderJSON converts a contracts.AssistantResponse plus the
// request-echo metadata into the v1 wire response shape. Pure
// function — safe for golden contract tests.
func RenderJSON(resp contracts.AssistantResponse, transportMessageID, requestID string, facadeInvoked bool) TurnResponse {
	out := TurnResponse{
		SchemaVersion:        SchemaVersionV1,
		Transport:            TransportName,
		TransportMessageID:   transportMessageID,
		Status:               string(resp.Status),
		Body:                 resp.Body,
		Sources:              renderSources(resp.Sources),
		SourcesOverflowCount: resp.SourcesOverflowCount,
		ConfirmCard:          renderConfirmCard(resp.ConfirmCard),
		DisambiguationPrompt: renderDisambiguation(resp.DisambiguationPrompt),
		ErrorCause:           string(resp.ErrorCause),
		CaptureRoute:         resp.CaptureRoute,
		FacadeInvoked:        facadeInvoked,
		EmittedAt:            resp.EmittedAt.UTC().Format(time.RFC3339Nano),
		Trace: TraceJSON{
			RequestID: requestID,
		},
	}
	if resp.Invocation != nil {
		out.Trace.AgentTraceID = resp.Invocation.TraceID
		// AssistantTurnID is populated by the audit substrate (SCOPE-08+);
		// surface the agent trace id as the stable correlator until then.
		out.Trace.AssistantTurnID = resp.Invocation.TraceID
	}
	// Spec 075 SCOPE-075-06.2b — copy structured legacy-retirement
	// notice into the optional wire field. Nil-safe: when absent,
	// `omitempty` drops the key from the JSON body entirely so
	// schema_version stays "v1" for non-retired turns.
	if resp.LegacyRetirementNotice != nil {
		out.Notice = &NoticeJSON{
			Command:            resp.LegacyRetirementNotice.Command,
			ReplacementExample: resp.LegacyRetirementNotice.ReplacementExample,
			CopyKey:            resp.LegacyRetirementNotice.CopyKey,
			WindowID:           resp.LegacyRetirementNotice.WindowID,
		}
	}
	return out
}

func renderSources(in []contracts.Source) []SourceJSON {
	out := make([]SourceJSON, 0, len(in))
	for _, s := range in {
		entry := SourceJSON{ID: s.ID, Title: s.Title, Kind: string(s.Kind)}
		switch ref := s.Ref.(type) {
		case contracts.ArtifactRef:
			entry.ArtifactID = ref.ArtifactID
			if !ref.CapturedAt.IsZero() {
				entry.ArtifactCapturedAt = ref.CapturedAt.UTC().Format(time.RFC3339Nano)
			}
		case contracts.ExternalProviderRef:
			entry.ProviderName = ref.ProviderName
			if !ref.RetrievedAt.IsZero() {
				entry.ProviderRetrievedAt = ref.RetrievedAt.UTC().Format(time.RFC3339Nano)
			}
		case contracts.WebSourceRef:
			entry.URL = ref.URL
			entry.WebProvider = ref.Provider
			entry.WebContentHash = ref.ContentHash
			entry.WebSnippet = ref.Snippet
			if !ref.FetchedAt.IsZero() {
				entry.WebFetchedAt = ref.FetchedAt.UTC().Format(time.RFC3339Nano)
			}
		case contracts.ComputationSourceRef:
			entry.ComputationTool = ref.Tool
			entry.ComputationInputHash = ref.InputHash
			entry.ComputationOutputHash = ref.OutputHash
		}
		out = append(out, entry)
	}
	return out
}

func renderConfirmCard(c *contracts.ConfirmCard) *ConfirmCardJSON {
	if c == nil {
		return nil
	}
	return &ConfirmCardJSON{
		ProposedAction: c.ProposedAction,
		ConfirmRef:     c.ConfirmRef,
		PositiveLabel:  c.PositiveLabel,
		NegativeLabel:  c.NegativeLabel,
		TimeoutSeconds: int(c.Timeout / time.Second),
	}
}

func renderDisambiguation(d *contracts.DisambiguationPrompt) *DisambiguationJSON {
	if d == nil {
		return nil
	}
	out := &DisambiguationJSON{
		DisambiguationRef: d.DisambiguationRef,
		TimeoutSeconds:    int(d.Timeout / time.Second),
		Choices:           make([]DisambiguationChoiceJSON, 0, len(d.Choices)),
	}
	for _, c := range d.Choices {
		out.Choices = append(out.Choices, DisambiguationChoiceJSON{
			Number: c.Number, ID: c.ID, Label: c.Label, Shortcut: c.Shortcut,
		})
	}
	return out
}

// ServeHTTP implements http.Handler for POST /api/assistant/turn.
// SCOPE-1a behavior: validates the wire schema, resolves the user
// from auth context, calls Facade.Handle exactly once, optionally
// invokes the capture path, and renders the response. Auth/scope
// and rate/body limits are layered on top by SCOPE-2.
func (a *HTTPAdapter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	requestID := middleware.GetReqID(r.Context())

	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		a.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "", requestID, false)
		return
	}
	if !a.cfg.Enabled {
		a.writeError(w, http.StatusServiceUnavailable, "assistant_http_disabled", "", requestID, false)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		a.writeError(w, http.StatusBadRequest, "invalid_assistant_turn", "", requestID, false)
		return
	}
	defer func() { _ = r.Body.Close() }()

	var req TurnRequest
	if err := json.Unmarshal(body, &req); err != nil {
		a.writeError(w, http.StatusBadRequest, "invalid_assistant_turn", "", requestID, false)
		return
	}

	userID := auth.UserIDFromContext(r.Context())
	if userID == "" {
		// Spec 069 SCOPE-2 / F-069-USERID-BINDING: shared-token and
		// dev-bypass sessions land here with Session.UserID="". The
		// adapter substitutes the SST-configured synthetic user id
		// (assistant.transports.http.shared_user_id) so single-user
		// dev/test and the production shared-token fallback resolve
		// to a stable identifier the facade can key on. Per-user
		// PASETO sessions never reach this branch because the bearer
		// middleware populates UserID from the sub claim.
		if sess, ok := auth.SessionFromContext(r.Context()); ok && sess.Source != "" && a.cfg.SharedUserID != "" {
			userID = a.cfg.SharedUserID
		} else {
			a.writeError(w, http.StatusUnauthorized, "auth_required", req.TransportMessageID, requestID, false)
			return
		}
	}

	msg, err := a.Translate(r.Context(), &translatePayload{UserID: userID, Request: &req})
	if err != nil {
		a.writeError(w, http.StatusBadRequest, "invalid_assistant_turn", req.TransportMessageID, requestID, false)
		return
	}
	fingerprintBody, err := json.Marshal(req)
	if err != nil {
		a.writeError(w, http.StatusBadRequest, "invalid_assistant_turn", req.TransportMessageID, requestID, false)
		return
	}
	lease, err := a.dedup.begin(userID, req.TransportMessageID, sha256.Sum256(fingerprintBody))
	switch {
	case errors.Is(err, errTransportMessageIDConflict):
		a.writeError(w, http.StatusConflict, "transport_message_id_conflict", req.TransportMessageID, requestID, false)
		return
	case errors.Is(err, errTurnDedupCapacity):
		a.writeError(w, http.StatusServiceUnavailable, "assistant_turn_capacity_exceeded", req.TransportMessageID, requestID, false)
		return
	case err != nil:
		a.writeError(w, http.StatusInternalServerError, "assistant_turn_failed", req.TransportMessageID, requestID, false)
		return
	}
	if !lease.owner {
		cached, ok := lease.wait(r.Context())
		if !ok {
			return
		}
		replayed := cached.response
		replayed.Trace.RequestID = requestID
		a.writeResponse(w, cached.status, replayed)
		return
	}

	resp, err := a.facade.Handle(r.Context(), msg)
	if err != nil {
		out := a.errorResponse("assistant_turn_failed", req.TransportMessageID, requestID, true)
		lease.complete(turnDedupResult{status: http.StatusInternalServerError, response: out})
		a.writeResponse(w, http.StatusInternalServerError, out)
		return
	}

	if resp.CaptureRoute {
		// Capture-as-fallback is inviolable (Hard Constraint 5 / BS-001):
		// the user's prompt MUST persist even if the client has already
		// disconnected. net/http cancels r.Context() the instant the
		// connection drops, which would abort the downstream
		// pipeline.Process Postgres INSERT / NATS publish and silently
		// lose the prompt. Decouple the durable capture write from
		// request cancellation while preserving request-scoped values
		// (request id, trace correlation via middleware.GetReqID).
		// Spec 069 chaos Round 39 — F-069-CHAOS39-CAPTURE-CTX-CANCEL.
		a.capture(context.WithoutCancel(r.Context()), userID, req.TransportMessageID, req.Text)
	}

	out := RenderJSON(resp, req.TransportMessageID, requestID, true)
	lease.complete(turnDedupResult{status: http.StatusOK, response: out})
	a.writeResponse(w, http.StatusOK, out)
}

func (a *HTTPAdapter) writeError(w http.ResponseWriter, status int, code, transportMessageID, requestID string, facadeInvoked bool) {
	a.writeResponse(w, status, a.errorResponse(code, transportMessageID, requestID, facadeInvoked))
}

func (a *HTTPAdapter) errorResponse(code, transportMessageID, requestID string, facadeInvoked bool) TurnResponse {
	return TurnResponse{
		SchemaVersion:      SchemaVersionV1,
		Transport:          TransportName,
		TransportMessageID: transportMessageID,
		Status:             string(contracts.StatusUnavailable),
		ErrorCause:         code,
		Sources:            []SourceJSON{},
		FacadeInvoked:      facadeInvoked,
		EmittedAt:          a.now().UTC().Format(time.RFC3339Nano),
		Trace:              TraceJSON{RequestID: requestID},
	}
}

func (a *HTTPAdapter) writeResponse(w http.ResponseWriter, status int, out TurnResponse) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(out)
}
