package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

// TestAuth_GeneratePKCEPairS256 pins the PKCE S256 derivation against the
// canonical RFC 7636 Appendix B test vector and asserts the generated
// verifier's charset/length contract (SCN-BUG-056-002-001).
func TestAuth_GeneratePKCEPairS256(t *testing.T) {
	t.Parallel()

	// RFC 7636 Appendix B — the normative worked example. This is the
	// authoritative proof that PKCEChallengeS256 computes
	// base64url-nopad(SHA-256(ASCII(code_verifier))) correctly.
	const (
		rfcVerifier  = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
		rfcChallenge = "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
	)
	if got := PKCEChallengeS256(rfcVerifier); got != rfcChallenge {
		t.Fatalf("RFC 7636 Appendix B vector mismatch:\n  verifier  = %s\n  want      = %s\n  got       = %s",
			rfcVerifier, rfcChallenge, got)
	}

	// Generated pair: verifier charset + length, and challenge consistency.
	unreserved := regexp.MustCompile(`^[A-Za-z0-9\-._~]+$`)
	for i := 0; i < 64; i++ {
		verifier, challenge, err := GeneratePKCEPair()
		if err != nil {
			t.Fatalf("GeneratePKCEPair: %v", err)
		}
		if l := len(verifier); l < 43 || l > 128 {
			t.Fatalf("verifier length %d out of RFC 7636 [43,128] bound: %q", l, verifier)
		}
		if !unreserved.MatchString(verifier) {
			t.Fatalf("verifier contains characters outside the unreserved set: %q", verifier)
		}
		if challenge != PKCEChallengeS256(verifier) {
			t.Fatalf("GeneratePKCEPair challenge is not S256(verifier): verifier=%q challenge=%q", verifier, challenge)
		}
		// A challenge must never equal its verifier (it is a hash).
		if challenge == verifier {
			t.Fatalf("challenge equals verifier (no hashing applied): %q", verifier)
		}
	}

	// Two consecutive verifiers must differ (crypto-random, not constant).
	v1, _, _ := GeneratePKCEPair()
	v2, _, _ := GeneratePKCEPair()
	if v1 == v2 {
		t.Fatalf("two GeneratePKCEPair verifiers collided: %q", v1)
	}
}

// TestAuth_OAuth2PKCEBasicAuthStyle asserts the additive PKCE + confidential-
// client Basic-auth behavior of GenericOAuth2 (SCN-BUG-056-002-002):
//   - AuthURLWithPKCE carries code_challenge + code_challenge_method=S256.
//   - With TokenEndpointAuthStyle="basic", ExchangeCodeWithVerifier sends
//     Authorization: Basic base64(id:secret), includes code_verifier in the
//     body, and OMITS client_secret from the body.
//   - With the default style, client_secret stays in the body and no Basic
//     header is sent (proving zero ripple to existing callers).
func TestAuth_OAuth2PKCEBasicAuthStyle(t *testing.T) {
	t.Parallel()

	const (
		clientID     = "twitter-client-id"
		clientSecret = "twitter-client-secret"
		code         = "auth-code-123"
		verifier     = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	)

	// --- AuthURLWithPKCE shape ---
	provider := NewGenericOAuth2("twitter", OAuth2Config{
		ClientID:               clientID,
		ClientSecret:           clientSecret,
		RedirectURL:            "http://127.0.0.1/callback",
		AuthEndpoint:           "https://twitter.com/i/oauth2/authorize",
		TokenEndpointAuthStyle: "basic",
		HTTPTimeoutSeconds:     5,
	})
	authURL := provider.AuthURLWithPKCE(
		[]string{"offline.access", "tweet.read", "bookmark.read"},
		"state-token-xyz", PKCEChallengeS256(verifier),
	)
	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("parse authorize URL: %v", err)
	}
	q := parsed.Query()
	if q.Get("code_challenge") != PKCEChallengeS256(verifier) {
		t.Fatalf("authorize URL missing/incorrect code_challenge: %q", authURL)
	}
	if q.Get("code_challenge_method") != "S256" {
		t.Fatalf("authorize URL missing code_challenge_method=S256: %q", authURL)
	}
	if q.Get("state") != "state-token-xyz" {
		t.Fatalf("authorize URL missing state: %q", authURL)
	}
	if !strings.Contains(q.Get("scope"), "bookmark.read") {
		t.Fatalf("authorize URL missing scopes: %q", authURL)
	}

	// --- ExchangeCodeWithVerifier (basic style) wire capture ---
	var (
		gotAuthHeader string
		gotBody       string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuthHeader = r.Header.Get("Authorization")
		raw, _ := io.ReadAll(io.LimitReader(r.Body, 1<<16))
		gotBody = string(raw)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"token_type":    "bearer",
			"expires_in":    7200,
			"access_token":  "at-value",
			"refresh_token": "rt-value",
			"scope":         "offline.access tweet.read bookmark.read",
		})
	}))
	defer srv.Close()
	provider.Config.TokenEndpoint = srv.URL

	tok, err := provider.ExchangeCodeWithVerifier(context.Background(), code, verifier)
	if err != nil {
		t.Fatalf("ExchangeCodeWithVerifier: %v", err)
	}
	if tok.AccessToken != "at-value" || tok.RefreshToken != "rt-value" {
		t.Fatalf("unexpected token decoded: %+v", tok)
	}

	wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte(clientID+":"+clientSecret))
	if gotAuthHeader != wantAuth {
		t.Fatalf("Authorization header mismatch:\n  want %q\n  got  %q", wantAuth, gotAuthHeader)
	}
	bodyVals, err := url.ParseQuery(gotBody)
	if err != nil {
		t.Fatalf("parse captured body: %v", err)
	}
	if bodyVals.Get("code_verifier") != verifier {
		t.Fatalf("token request body missing code_verifier: %q", gotBody)
	}
	if bodyVals.Get("grant_type") != "authorization_code" {
		t.Fatalf("token request body missing grant_type=authorization_code: %q", gotBody)
	}
	if bodyVals.Has("client_secret") || strings.Contains(gotBody, "client_secret") {
		t.Fatalf("basic-auth style MUST omit client_secret from the body, got: %q", gotBody)
	}

	// --- Default style: secret stays in body, no Basic header (no ripple) ---
	var (
		defaultAuthHeader string
		defaultBody       string
	)
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defaultAuthHeader = r.Header.Get("Authorization")
		raw, _ := io.ReadAll(io.LimitReader(r.Body, 1<<16))
		defaultBody = string(raw)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"token_type":   "bearer",
			"expires_in":   3600,
			"access_token": "at2",
		})
	}))
	defer srv2.Close()
	bodyStyle := NewGenericOAuth2("legacy", OAuth2Config{
		ClientID:           clientID,
		ClientSecret:       clientSecret,
		RedirectURL:        "http://127.0.0.1/callback",
		TokenEndpoint:      srv2.URL,
		HTTPTimeoutSeconds: 5,
		// TokenEndpointAuthStyle left empty == "body" (existing behavior).
	})
	if _, err := bodyStyle.ExchangeCode(context.Background(), code); err != nil {
		t.Fatalf("default-style ExchangeCode: %v", err)
	}
	if defaultAuthHeader != "" {
		t.Fatalf("default style must NOT send an Authorization header, got: %q", defaultAuthHeader)
	}
	if dv, _ := url.ParseQuery(defaultBody); dv.Get("client_secret") != clientSecret {
		t.Fatalf("default style must keep client_secret in the body, got: %q", defaultBody)
	}
}
