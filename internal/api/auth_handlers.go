package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/smackerel/smackerel/internal/auth"
	"github.com/smackerel/smackerel/internal/auth/revocation"
	"github.com/smackerel/smackerel/internal/config"
)

// AuthAdminHandlers groups the spec 044 Scope 01 admin HTTP endpoints
// for per-user bearer auth management. Construct via NewAuthAdminHandlers.
//
// Routes (gate behind admin scope; wiring lands in Scope 02):
//
//	POST /v1/auth/users                     enroll a user
//	POST /v1/auth/users/{user_id}/rotate    rotate a user's active token
//	POST /v1/auth/tokens/{token_id}/revoke  revoke a specific token
//	GET  /v1/auth/users                     list enrolled users
//
// The handlers DO NOT enforce admin scope themselves — the caller MUST
// wire them behind a middleware that verifies SessionFromContext.IsAdmin
// (or that the request's per-user session matches an SST-resolved
// allowlist). Scope 01 ships the handlers; Scope 02 wires the gate.
type AuthAdminHandlers struct {
	store       *auth.BearerStore
	cfg         *config.Config
	broadcaster *revocation.Broadcaster
}

// NewAuthAdminHandlers constructs the handlers. broadcaster may be nil
// when telemetry is disabled or NATS is unavailable; in that case
// revoke calls still update the canonical store, and peer instances
// pick up the revocation on their next periodic refresh.
func NewAuthAdminHandlers(store *auth.BearerStore, cfg *config.Config, broadcaster *revocation.Broadcaster) (*AuthAdminHandlers, error) {
	if store == nil {
		return nil, fmt.Errorf("api: NewAuthAdminHandlers requires non-nil BearerStore")
	}
	if cfg == nil {
		return nil, fmt.Errorf("api: NewAuthAdminHandlers requires non-nil *config.Config")
	}
	return &AuthAdminHandlers{
		store:       store,
		cfg:         cfg,
		broadcaster: broadcaster,
	}, nil
}

// EnrollRequest is the request body for POST /v1/auth/users.
type EnrollRequest struct {
	UserID string `json:"user_id"`
	Notes  string `json:"notes,omitempty"`
}

// EnrollResponse is the response body for POST /v1/auth/users.
type EnrollResponse struct {
	UserID    string    `json:"user_id"`
	TokenID   string    `json:"token_id"`
	WireToken string    `json:"token"`
	IssuedAt  time.Time `json:"issued_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// HandleEnroll implements POST /v1/auth/users. Returns 401 when the
// caller is not an admin; 400 when the body is malformed; 409 when the
// user_id already exists. The minted token is returned in the response
// body exactly once — callers MUST capture it.
func (h *AuthAdminHandlers) HandleEnroll(w http.ResponseWriter, r *http.Request) {
	sess, ok := auth.SessionFromContext(r.Context())
	if !ok || !h.callerIsAdmin(sess) {
		writeError(w, http.StatusUnauthorized, "FORBIDDEN", "admin scope required")
		return
	}

	var req EnrollRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "request body must be JSON: "+err.Error())
		return
	}
	if req.UserID == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "user_id is required")
		return
	}

	enrolledBy := sess.UserID
	if enrolledBy == "" {
		enrolledBy = string(sess.Source)
	}

	if err := h.store.Enroll(r.Context(), auth.EnrollUserParams{
		UserID:     req.UserID,
		EnrolledBy: enrolledBy,
		Notes:      req.Notes,
	}); err != nil {
		slog.Warn("auth admin enroll failed", "user_id", req.UserID, "error", err)
		// pgx returns a unique-violation wrapper; surface as 409.
		writeError(w, http.StatusConflict, "USER_EXISTS", err.Error())
		return
	}

	wire, tokenID, issuedAt, expiresAt, err := h.issueAndPersist(r, req.UserID, enrolledBy, "admin_api", "")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ISSUE_FAILED", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, EnrollResponse{
		UserID:    req.UserID,
		TokenID:   tokenID,
		WireToken: wire,
		IssuedAt:  issuedAt,
		ExpiresAt: expiresAt,
	})
}

// RotateRequest is the request body for POST /v1/auth/users/{user_id}/rotate.
type RotateRequest struct {
	PriorTokenID string `json:"prior_token_id"`
}

// HandleRotate implements POST /v1/auth/users/{user_id}/rotate.
func (h *AuthAdminHandlers) HandleRotate(w http.ResponseWriter, r *http.Request) {
	sess, ok := auth.SessionFromContext(r.Context())
	if !ok || !h.callerIsAdmin(sess) {
		writeError(w, http.StatusUnauthorized, "FORBIDDEN", "admin scope required")
		return
	}

	userID := chi.URLParam(r, "user_id")
	if userID == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "user_id path param required")
		return
	}

	var req RotateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "request body must be JSON: "+err.Error())
		return
	}
	if req.PriorTokenID == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "prior_token_id required")
		return
	}

	rotatedBy := sess.UserID
	if rotatedBy == "" {
		rotatedBy = string(sess.Source)
	}

	wire, tokenID, issuedAt, expiresAt, err := h.issueAndPersist(r, userID, rotatedBy, "admin_api", req.PriorTokenID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ISSUE_FAILED", err.Error())
		return
	}

	if err := h.store.MarkTokenRotated(r.Context(), req.PriorTokenID); err != nil {
		writeError(w, http.StatusInternalServerError, "ROTATE_PERSIST_FAILED", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, EnrollResponse{
		UserID:    userID,
		TokenID:   tokenID,
		WireToken: wire,
		IssuedAt:  issuedAt,
		ExpiresAt: expiresAt,
	})
}

// RevokeRequest is the request body for POST /v1/auth/tokens/{token_id}/revoke.
type RevokeRequest struct {
	Reason string `json:"reason,omitempty"`
}

// HandleRevoke implements POST /v1/auth/tokens/{token_id}/revoke.
func (h *AuthAdminHandlers) HandleRevoke(w http.ResponseWriter, r *http.Request) {
	sess, ok := auth.SessionFromContext(r.Context())
	if !ok || !h.callerIsAdmin(sess) {
		writeError(w, http.StatusUnauthorized, "FORBIDDEN", "admin scope required")
		return
	}

	tokenID := chi.URLParam(r, "token_id")
	if tokenID == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "token_id path param required")
		return
	}

	var req RevokeRequest
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "request body must be JSON: "+err.Error())
			return
		}
	}

	revokedBy := sess.UserID
	if revokedBy == "" {
		revokedBy = string(sess.Source)
	}

	if err := h.store.RevokeToken(r.Context(), tokenID, revokedBy, req.Reason); err != nil {
		writeError(w, http.StatusInternalServerError, "REVOKE_FAILED", err.Error())
		return
	}

	if h.broadcaster != nil {
		if err := h.broadcaster.Publish(tokenID, req.Reason); err != nil {
			// Soft failure — DB is canonical; peer instances pick up
			// via periodic refresh ≤ NFR-AUTH-006 worst case.
			slog.Warn("revocation broadcast failed", "token_id", tokenID, "error", err)
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleListUsers implements GET /v1/auth/users.
func (h *AuthAdminHandlers) HandleListUsers(w http.ResponseWriter, r *http.Request) {
	sess, ok := auth.SessionFromContext(r.Context())
	if !ok || !h.callerIsAdmin(sess) {
		writeError(w, http.StatusUnauthorized, "FORBIDDEN", "admin scope required")
		return
	}

	users, err := h.store.ListUsers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "LIST_FAILED", err.Error())
		return
	}

	type listEntry struct {
		UserID     string    `json:"user_id"`
		EnrolledAt time.Time `json:"enrolled_at"`
		EnrolledBy string    `json:"enrolled_by"`
		Status     string    `json:"status"`
		Notes      string    `json:"notes,omitempty"`
	}
	out := make([]listEntry, 0, len(users))
	for _, u := range users {
		out = append(out, listEntry{
			UserID:     u.UserID,
			EnrolledAt: u.EnrolledAt,
			EnrolledBy: u.EnrolledBy,
			Status:     u.Status,
			Notes:      u.Notes,
		})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"users": out,
		"count": len(out),
	})
}

// callerIsAdmin enforces the spec 044 admin scope decision. The
// bootstrap session is always admin; the shared-token session is admin
// only when production_shared_token_fallback_enabled is true (otherwise
// the legacy single-tenant ergonomic is locked out). Per-user sessions
// are admin only when the user_id matches the SST-resolved allowlist;
// Scope 01 ships an empty allowlist (no per-user admin), so per-user
// sessions are rejected here and the operator MUST use the bootstrap
// flow until the allowlist surface lands in a later scope.
func (h *AuthAdminHandlers) callerIsAdmin(sess auth.Session) bool {
	switch sess.Source {
	case auth.SessionSourceBootstrap:
		return true
	case auth.SessionSourceSharedToken:
		if h.cfg.Environment != "production" {
			return true
		}
		return h.cfg.Auth.ProductionSharedTokenFallbackEnabled
	case auth.SessionSourcePerUserToken:
		// Future scope: SST allowlist of per-user admin user_ids.
		return false
	default:
		return false
	}
}

// issueAndPersist mirrors cmd_auth.go's helper but lives in the api
// package to avoid cross-package importing of an unexported helper.
// Returns the wire token (one-shot), token id, iat, exp.
func (h *AuthAdminHandlers) issueAndPersist(r *http.Request, userID, issuedBy, issuedSource, rotatedFrom string) (
	wire, tokenID string, issuedAt, expiresAt time.Time, err error) {

	if h.cfg.Auth.SigningActivePrivateKey == "" || h.cfg.Auth.SigningActiveKeyID == "" {
		return "", "", time.Time{}, time.Time{},
			fmt.Errorf("auth.signing.active_private_key and active_key_id MUST be set to issue tokens")
	}
	if h.cfg.Auth.AtRestHashingKey == "" {
		return "", "", time.Time{}, time.Time{},
			fmt.Errorf("auth.at_rest_hashing_key MUST be set to persist tokens at rest")
	}

	tokenID, err = generateRandomTokenID()
	if err != nil {
		return "", "", time.Time{}, time.Time{}, err
	}

	issued, err := auth.IssueToken(auth.IssueOptions{
		UserID:     userID,
		TokenID:    tokenID,
		SigningKey: h.cfg.Auth.SigningActivePrivateKey,
		KeyID:      h.cfg.Auth.SigningActiveKeyID,
		TTL:        time.Duration(h.cfg.Auth.TokenTTLHours) * time.Hour,
		Issuer:     "smackerel",
		Now:        time.Now,
	})
	if err != nil {
		return "", "", time.Time{}, time.Time{}, fmt.Errorf("issue token: %w", err)
	}

	hashed, err := auth.HashToken(issued.WireToken, h.cfg.Auth.AtRestHashingKey)
	if err != nil {
		return "", "", time.Time{}, time.Time{}, fmt.Errorf("hash token: %w", err)
	}

	if err := h.store.PersistToken(r.Context(), auth.PersistTokenParams{
		TokenID:            tokenID,
		UserID:             userID,
		KeyID:              h.cfg.Auth.SigningActiveKeyID,
		IssuedAt:           issued.IssuedAt,
		ExpiresAt:          issued.ExpiresAt,
		HashedToken:        hashed,
		IssuedBy:           issuedBy,
		IssuedSource:       issuedSource,
		RotatedFromTokenID: rotatedFrom,
	}); err != nil {
		return "", "", time.Time{}, time.Time{}, fmt.Errorf("persist token: %w", err)
	}

	return issued.WireToken, tokenID, issued.IssuedAt, issued.ExpiresAt, nil
}

// generateRandomTokenID produces a 128-bit random token id, hex-encoded.
func generateRandomTokenID() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", fmt.Errorf("rand: %w", err)
	}
	return hex.EncodeToString(buf[:]), nil
}
