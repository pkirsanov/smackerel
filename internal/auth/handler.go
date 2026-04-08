package auth

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// OAuthHandler manages OAuth2 web flows for connector authorization.
type OAuthHandler struct {
	providers    map[string]OAuth2Provider
	store        *TokenStore
	states       map[string]string    // state → provider name (CSRF protection)
	stateCreated map[string]time.Time // state → creation time (for TTL eviction)
	mu           sync.Mutex
}

// NewOAuthHandler creates a new OAuth handler.
func NewOAuthHandler(store *TokenStore) *OAuthHandler {
	return &OAuthHandler{
		providers:    make(map[string]OAuth2Provider),
		store:        store,
		states:       make(map[string]string),
		stateCreated: make(map[string]time.Time),
	}
}

// RegisterProvider adds an OAuth2 provider.
func (h *OAuthHandler) RegisterProvider(provider OAuth2Provider) {
	h.providers[provider.ProviderName()] = provider
}

// StartHandler initiates the OAuth2 flow — GET /auth/{provider}/start
func (h *OAuthHandler) StartHandler(w http.ResponseWriter, r *http.Request) {
	providerName := r.PathValue("provider")
	if providerName == "" {
		http.Error(w, "provider required", http.StatusBadRequest)
		return
	}

	provider, ok := h.providers[providerName]
	if !ok {
		http.Error(w, "unknown provider: "+providerName, http.StatusNotFound)
		return
	}

	// Generate CSRF state token
	state := generateState()
	h.mu.Lock()
	// Evict entries older than 10 minutes
	cutoff := time.Now().Add(-10 * time.Minute)
	for s, created := range h.stateCreated {
		if created.Before(cutoff) {
			delete(h.states, s)
			delete(h.stateCreated, s)
		}
	}
	// Cap at 100 entries to prevent abuse
	if len(h.states) >= 100 {
		h.mu.Unlock()
		http.Error(w, "too many pending authorization requests", http.StatusTooManyRequests)
		return
	}
	h.states[state] = providerName
	h.stateCreated[state] = time.Now()
	h.mu.Unlock()

	// Determine scopes
	var scopes []string
	switch providerName {
	case "google":
		scopes = GoogleOAuth2Scopes()
	default:
		scopes = []string{"openid", "profile", "email"}
	}

	authURL := provider.AuthURL(scopes, state)
	slog.Info("OAuth flow started", "provider", providerName)

	http.Redirect(w, r, authURL, http.StatusFound)
}

// CallbackHandler handles the OAuth2 redirect — GET /auth/{provider}/callback
func (h *OAuthHandler) CallbackHandler(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	errParam := r.URL.Query().Get("error")

	if errParam != "" {
		slog.Error("OAuth callback error", "error", errParam, "description", r.URL.Query().Get("error_description"))
		http.Error(w, "OAuth error: "+errParam, http.StatusBadRequest)
		return
	}

	if code == "" {
		http.Error(w, "authorization code missing", http.StatusBadRequest)
		return
	}

	// Validate CSRF state
	h.mu.Lock()
	providerName, ok := h.states[state]
	if ok {
		delete(h.states, state)
		delete(h.stateCreated, state)
	}
	h.mu.Unlock()

	if !ok {
		http.Error(w, "invalid state parameter", http.StatusBadRequest)
		return
	}

	provider, ok := h.providers[providerName]
	if !ok {
		http.Error(w, "provider not found", http.StatusInternalServerError)
		return
	}

	// Exchange code for token
	token, err := provider.ExchangeCode(r.Context(), code)
	if err != nil {
		slog.Error("token exchange failed", "provider", providerName, "error", err)
		http.Error(w, "token exchange failed", http.StatusInternalServerError)
		return
	}

	// Store the token
	if err := h.store.Save(r.Context(), providerName, token); err != nil {
		slog.Error("token storage failed", "provider", providerName, "error", err)
		http.Error(w, "token storage failed", http.StatusInternalServerError)
		return
	}

	slog.Info("OAuth authorization complete",
		"provider", providerName,
		"scopes", token.Scopes,
		"expires_at", token.ExpiresAt,
	)

	// Return success page
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(`<!DOCTYPE html><html><body>
		<h2>Authorization successful</h2>
		<p>` + providerName + ` connected. You can close this window.</p>
		<script>setTimeout(function(){window.close()}, 3000)</script>
	</body></html>`))
}

// StatusHandler returns the connection status of all providers — GET /auth/status
func (h *OAuthHandler) StatusHandler(w http.ResponseWriter, r *http.Request) {
	status := make(map[string]interface{})
	for name := range h.providers {
		connected := h.store.HasToken(r.Context(), name)
		status[name] = map[string]interface{}{
			"connected": connected,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// generateState creates a random hex string for CSRF protection.
func generateState() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}
