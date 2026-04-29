// Package fixtures provides an owned HTTP fixture server that stands in
// for the Google OAuth and Google Drive REST APIs during Spec 038 Scope 1
// integration tests. The fixture exposes only the endpoints that the
// real GoogleDriveProvider calls during the OAuth authorization +
// connect-and-health round trip:
//
//   - GET  /oauth2/auth     — mints a code bound to the provided state
//     and returns it in a minimal JSON payload.
//   - POST /oauth2/token    — exchanges a code for an access+refresh
//     token with a 1-hour expires_in.
//   - GET  /drive/v3/about  — returns the bound user identity, gated by
//     a Bearer access token from /oauth2/token.
//   - GET  /drive/v3/files  — empty-drive listing returning {"files":[]}.
//
// The server is deterministic: state is in-memory, scoped to the Server
// instance, and reset by constructing a new Server. Tests SHOULD treat
// every Server as disposable per-test to avoid cross-test bleed.
//
// Programmatic helper IssueAuthCode lets tests skip the user-agent
// browser step and obtain a code bound to a server-issued state token
// directly. The /oauth2/auth endpoint also issues codes (via the same
// helper) so that interactive PWA tests can drive the redirect leg
// without extra fixture wiring.
package fixtures

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
)

// Server is an httptest.Server pre-loaded with the OAuth + Drive
// endpoint handlers. The embedded *httptest.Server exposes URL/Close.
type Server struct {
	*httptest.Server

	mu sync.Mutex
	// codes maps a one-shot authorization code to the state token
	// it was bound to at issuance. Consumed by /oauth2/token.
	codes map[string]string
	// tokens maps an access token to the user email returned by
	// /drive/v3/about. Tokens are minted by /oauth2/token.
	tokens map[string]string
}

// NewServer constructs and starts a fixture server. Callers MUST defer
// Close() to release the underlying httptest listener.
func NewServer() *Server {
	s := &Server{
		codes:  make(map[string]string),
		tokens: make(map[string]string),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth2/auth", s.handleAuth)
	mux.HandleFunc("/oauth2/token", s.handleToken)
	mux.HandleFunc("/drive/v3/about", s.handleAbout)
	mux.HandleFunc("/drive/v3/files", s.handleFiles)
	s.Server = httptest.NewServer(mux)
	return s
}

func randHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand should not fail on supported platforms; tests
		// would surface this as a fixture-internal error.
		panic("fixtures: rand.Read: " + err.Error())
	}
	return hex.EncodeToString(b)
}

// IssueAuthCode mints a one-shot authorization code bound to the given
// state token. Tests use this helper to drive FinalizeConnect without
// performing a real browser redirect through /oauth2/auth.
func (s *Server) IssueAuthCode(state string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	code := "code-" + randHex(8)
	s.codes[code] = state
	return code
}

func (s *Server) handleAuth(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	if state == "" {
		http.Error(w, `{"error":"missing_state"}`, http.StatusBadRequest)
		return
	}
	code := s.IssueAuthCode(state)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"code":  code,
		"state": state,
	})
}

func (s *Server) handleToken(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, `{"error":"bad_form"}`, http.StatusBadRequest)
		return
	}
	code := r.Form.Get("code")
	if code == "" {
		http.Error(w, `{"error":"missing_code"}`, http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	if _, ok := s.codes[code]; !ok {
		s.mu.Unlock()
		http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
		return
	}
	delete(s.codes, code)
	access := "tok-" + randHex(16)
	s.tokens[access] = "fixture-user@example.com"
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"access_token":  access,
		"refresh_token": "refresh-" + randHex(8),
		"expires_in":    3600,
		"token_type":    "Bearer",
	})
}

func (s *Server) handleAbout(w http.ResponseWriter, r *http.Request) {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	token := strings.TrimPrefix(auth, "Bearer ")
	s.mu.Lock()
	email, ok := s.tokens[token]
	s.mu.Unlock()
	if !ok {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"user": map[string]any{
			"emailAddress": email,
			"displayName":  "Fixture User",
		},
	})
}

func (s *Server) handleFiles(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"files": []any{},
	})
}
