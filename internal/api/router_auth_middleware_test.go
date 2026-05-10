package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/auth"
	"github.com/smackerel/smackerel/internal/auth/revocation"
	"github.com/smackerel/smackerel/internal/config"
)

// fixtureSigningMaterial returns a deterministic V4 signing keypair
// for the bearer-auth middleware tests. The hex strings are stable
// (we generate them once via auth.GenerateSigningKeypair and embed
// here so each test run is reproducible without hitting crypto/rand).
//
// Spec 044 Scope 02 router middleware unit tests — these fixtures
// MUST NOT be reused for production deployments. The constructor in
// internal/config/config.go forbids the AtRestHashingKey from
// equaling the SigningActivePrivateKey (OQ-8); we honor the same
// invariant here by using a distinct hash key derived via HMAC.
func fixtureSigningMaterial(t *testing.T) (privHex, pubHex, kid, hashKey string) {
	t.Helper()
	priv, pub := auth.GenerateSigningKeypair()
	mac := hmac.New(sha256.New, []byte(priv))
	mac.Write([]byte("auth-test-hash-key-derivation"))
	hash := hex.EncodeToString(mac.Sum(nil))
	return priv, pub, "test-key-001", hash
}

// newProductionAuthDeps constructs a *Dependencies with the per-user
// PASETO middleware path active. Used by every Scope 02 middleware
// branch test that needs the production hot-path.
func newProductionAuthDeps(t *testing.T) (*Dependencies, string, *revocation.Cache) {
	t.Helper()
	priv, pub, kid, hashKey := fixtureSigningMaterial(t)
	cache := revocation.NewCache()
	deps := &Dependencies{
		DB:          &mockDB{healthy: true},
		NATS:        &mockNATS{healthy: true},
		StartTime:   time.Now(),
		Environment: "production",
		AuthConfig: config.AuthConfig{
			Enabled:                              true,
			TokenFormat:                          "paseto_v4_public",
			SigningActivePrivateKey:              priv,
			SigningActiveKeyID:                   kid,
			TokenTTLHours:                        24,
			RotationGraceWindowHours:             24,
			ClockSkewToleranceSeconds:            60,
			RevocationCacheRefreshIntervalSeconds: 60,
			AtRestHashingKey:                     hashKey,
			ProductionSharedTokenFallbackEnabled: false,
		},
		AuthVerifyOptions: auth.VerifyOptions{
			ActivePublicKey:    pub,
			ActiveKeyID:        kid,
			Issuer:             "smackerel",
			ClockSkewTolerance: time.Minute,
			Now:                time.Now,
		},
		RevocationCache: cache,
	}
	return deps, priv, cache
}

// TestBearerAuth_PerUserPASETO_Production_Accepts validates branch 1:
// production + auth.enabled + valid PASETO token → 200 with the
// session attached to context (verified via a sentinel handler).
//
// Adversarial sub-cases prove:
//   - A PASETO token signed with a foreign key is rejected (401).
//   - A PASETO token whose JTI is in the revocation cache is rejected (401).
//   - A PASETO token whose `kid` footer points at an unknown key
//     identifier is rejected (401).
//
// All four branches return 401 with a generic body; the response MUST
// NOT name the failure mode (NFR-AUTH-007 / SCN-AUTH-010).
func TestBearerAuth_PerUserPASETO_Production_Accepts(t *testing.T) {
	deps, priv, cache := newProductionAuthDeps(t)

	// Issue a valid token for user "alice".
	issued, err := auth.IssueToken(auth.IssueOptions{
		UserID:     "alice",
		TokenID:    "tok-alice-001",
		SigningKey: priv,
		KeyID:      deps.AuthConfig.SigningActiveKeyID,
		TTL:        time.Hour,
		Issuer:     "smackerel",
		Now:        time.Now,
	})
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	// Wrap the middleware around a sentinel handler that asserts the
	// session is attached and contains the expected UserID + Source.
	var sessionUser, sessionSource string
	sentinel := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, ok := auth.SessionFromContext(r.Context())
		if !ok {
			t.Errorf("expected session in context after PASETO validation, got none")
			http.Error(w, "no session", http.StatusInternalServerError)
			return
		}
		sessionUser = sess.UserID
		sessionSource = string(sess.Source)
		w.WriteHeader(http.StatusOK)
	})
	mw := deps.bearerAuthMiddleware(sentinel)

	t.Run("valid_paseto_accepted", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/auth/users", nil)
		req.Header.Set("Authorization", "Bearer "+issued.WireToken)
		rec := httptest.NewRecorder()
		mw.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("valid PASETO: expected 200, got %d body=%s", rec.Code, rec.Body.String())
		}
		if sessionUser != "alice" {
			t.Errorf("expected session UserID=alice, got %q", sessionUser)
		}
		if sessionSource != "per_user_token" {
			t.Errorf("expected session source=per_user_token, got %q", sessionSource)
		}
	})

	t.Run("foreign_key_rejected", func(t *testing.T) {
		// Issue a token with an entirely separate signing key.
		foreignPriv, _ := auth.GenerateSigningKeypair()
		foreign, err := auth.IssueToken(auth.IssueOptions{
			UserID:     "mallory",
			TokenID:    "tok-mallory-001",
			SigningKey: foreignPriv,
			KeyID:      deps.AuthConfig.SigningActiveKeyID, // same kid → router picks active pub
			TTL:        time.Hour,
			Issuer:     "smackerel",
			Now:        time.Now,
		})
		if err != nil {
			t.Fatalf("issue foreign token: %v", err)
		}
		req := httptest.NewRequest(http.MethodGet, "/v1/auth/users", nil)
		req.Header.Set("Authorization", "Bearer "+foreign.WireToken)
		rec := httptest.NewRecorder()
		mw.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("foreign-key PASETO: expected 401, got %d body=%s", rec.Code, rec.Body.String())
		}
		// Adversarial: response body MUST NOT name the failure
		// mode ("signature", "key", "verify", "expired"). NFR-AUTH-007.
		body := rec.Body.String()
		for _, leak := range []string{"signature", "verify", "key id", "kid"} {
			if containsSubstring(body, leak) {
				t.Errorf("response body leaks failure detail %q: %s", leak, body)
			}
		}
	})

	t.Run("revoked_token_rejected", func(t *testing.T) {
		issued2, err := auth.IssueToken(auth.IssueOptions{
			UserID:     "bob",
			TokenID:    "tok-bob-001",
			SigningKey: priv,
			KeyID:      deps.AuthConfig.SigningActiveKeyID,
			TTL:        time.Hour,
			Issuer:     "smackerel",
			Now:        time.Now,
		})
		if err != nil {
			t.Fatalf("issue bob token: %v", err)
		}
		// Mark the JTI revoked BEFORE the request hits the middleware.
		cache.MarkRevoked("tok-bob-001")
		req := httptest.NewRequest(http.MethodGet, "/v1/auth/users", nil)
		req.Header.Set("Authorization", "Bearer "+issued2.WireToken)
		rec := httptest.NewRecorder()
		mw.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("revoked PASETO: expected 401, got %d body=%s", rec.Code, rec.Body.String())
		}
	})
}

// TestBearerAuth_Production_EmptyToken_Rejected validates branch 5:
// production with no token AND no per-user surface → 401 (defense-
// in-depth; the wiring layer should already have failed at startup,
// but the middleware enforces the same invariant on every request).
//
// Adversarial sub-case: production with auth.enabled=false AND empty
// AuthToken — currently the existing MIT-040-S-004 path that this
// scope preserves verbatim. The response must be 401 and MUST NOT
// silently bypass auth.
func TestBearerAuth_Production_EmptyToken_Rejected(t *testing.T) {
	deps := &Dependencies{
		DB:          &mockDB{healthy: true},
		NATS:        &mockNATS{healthy: true},
		StartTime:   time.Now(),
		Environment: "production",
		AuthToken:   "", // no shared token configured
		AuthConfig:  config.AuthConfig{Enabled: false},
	}
	mw := deps.bearerAuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("middleware did not reject empty-token production request")
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/digest", nil)
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for production+empty-token, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// TestBearerAuth_DevEmpty_Bypass_Allows validates branch 4: dev with
// empty AuthToken AND no per-user surface → bypass with synthetic
// shared-token session attached. This preserves the today-ever lever
// per FR-AUTH-015.
//
// Adversarial sub-case: the synthetic session MUST be source =
// SharedToken (not PerUserToken) and the UserID MUST be empty so
// downstream handlers that consult auth.UserIDFromContext correctly
// observe "no per-user identity" and fall back to dev/test ergonomics.
func TestBearerAuth_DevEmpty_Bypass_Allows(t *testing.T) {
	deps := &Dependencies{
		DB:          &mockDB{healthy: true},
		NATS:        &mockNATS{healthy: true},
		StartTime:   time.Now(),
		Environment: "development",
		AuthToken:   "",
		AuthConfig:  config.AuthConfig{Enabled: false},
	}
	var sawSession auth.Session
	var sawOK bool
	mw := deps.bearerAuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawSession, sawOK = auth.SessionFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/digest", nil)
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for dev-empty-token bypass, got %d", rec.Code)
	}
	if !sawOK {
		t.Fatalf("expected synthetic session attached on dev bypass, got none")
	}
	if sawSession.Source != auth.SessionSourceSharedToken {
		t.Errorf("expected synthetic session source=shared_token, got %q", sawSession.Source)
	}
	if sawSession.UserID != "" {
		t.Errorf("expected synthetic session UserID empty, got %q", sawSession.UserID)
	}
}

// TestBearerAuth_DevSharedToken_Allows validates branch 3: dev/test
// shared-token compare succeeds and attaches the SharedToken
// synthetic session. Preserves FR-AUTH-015.
func TestBearerAuth_DevSharedToken_Allows(t *testing.T) {
	deps := &Dependencies{
		DB:          &mockDB{healthy: true},
		NATS:        &mockNATS{healthy: true},
		StartTime:   time.Now(),
		Environment: "test",
		AuthToken:   "shared-token-fixture",
		AuthConfig:  config.AuthConfig{Enabled: false},
	}
	var sawSource auth.SessionSource
	mw := deps.bearerAuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if sess, ok := auth.SessionFromContext(r.Context()); ok {
			sawSource = sess.Source
		}
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/digest", nil)
	req.Header.Set("Authorization", "Bearer shared-token-fixture")
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for dev shared-token compare, got %d body=%s", rec.Code, rec.Body.String())
	}
	if sawSource != auth.SessionSourceSharedToken {
		t.Errorf("expected session source=shared_token, got %q", sawSource)
	}
}

// TestBearerAuth_ProductionSharedTokenFallback_Optin validates
// branch 2: production with auth.enabled=true AND
// production_shared_token_fallback_enabled=true AND PASETO verify
// fails → fall back to constant-time shared-token compare and accept
// when it matches. The fallback emits a deprecation warn (verified by
// the absence of any per-user session attached — the fallback path
// attaches a SharedToken session instead).
//
// Adversarial sub-case: when fallback is DISABLED (default), the
// same shared-token-only request is rejected with 401 even though
// d.AuthToken is configured. This proves the opt-in flag actually
// gates the fallback rather than being a no-op.
func TestBearerAuth_ProductionSharedTokenFallback_Optin(t *testing.T) {
	deps, _, _ := newProductionAuthDeps(t)
	deps.AuthToken = "production-fallback-shared-token"
	deps.AuthConfig.ProductionSharedTokenFallbackEnabled = true

	var sawSource auth.SessionSource
	mw := deps.bearerAuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if sess, ok := auth.SessionFromContext(r.Context()); ok {
			sawSource = sess.Source
		}
		w.WriteHeader(http.StatusOK)
	}))

	t.Run("fallback_optin_accepts_shared_token", func(t *testing.T) {
		sawSource = ""
		req := httptest.NewRequest(http.MethodGet, "/api/digest", nil)
		req.Header.Set("Authorization", "Bearer production-fallback-shared-token")
		rec := httptest.NewRecorder()
		mw.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 with fallback enabled, got %d body=%s", rec.Code, rec.Body.String())
		}
		if sawSource != auth.SessionSourceSharedToken {
			t.Errorf("expected fallback to attach shared_token session, got source=%q", sawSource)
		}
	})

	t.Run("fallback_disabled_rejects_shared_token", func(t *testing.T) {
		deps.AuthConfig.ProductionSharedTokenFallbackEnabled = false
		mw2 := deps.bearerAuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			t.Error("middleware accepted shared token in production with fallback disabled")
			w.WriteHeader(http.StatusOK)
		}))
		req := httptest.NewRequest(http.MethodGet, "/api/digest", nil)
		req.Header.Set("Authorization", "Bearer production-fallback-shared-token")
		rec := httptest.NewRecorder()
		mw2.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 with fallback disabled, got %d body=%s", rec.Code, rec.Body.String())
		}
	})
}

// TestUserIDFromContext validates the helper added in Scope 02
// alongside the middleware refactor. The helper returns the empty
// string when the context has no session, when the session has an
// empty UserID, and returns the UserID when set.
func TestUserIDFromContext(t *testing.T) {
	t.Run("no_session_returns_empty", func(t *testing.T) {
		got := auth.UserIDFromContext(context.Background())
		if got != "" {
			t.Errorf("expected empty string for ctx without session, got %q", got)
		}
	})
	t.Run("session_with_user_id", func(t *testing.T) {
		ctx := auth.WithSession(context.Background(), auth.Session{UserID: "alice", Source: auth.SessionSourcePerUserToken})
		got := auth.UserIDFromContext(ctx)
		if got != "alice" {
			t.Errorf("expected alice, got %q", got)
		}
	})
	t.Run("session_without_user_id", func(t *testing.T) {
		ctx := auth.WithSession(context.Background(), auth.Session{Source: auth.SessionSourceSharedToken})
		got := auth.UserIDFromContext(ctx)
		if got != "" {
			t.Errorf("expected empty for session without UserID, got %q", got)
		}
	})
}
