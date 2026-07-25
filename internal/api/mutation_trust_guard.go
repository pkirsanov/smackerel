// Package api — MutationTrustGuard: product-wide CSRF + Origin protection for
// every cookie-authenticated state-changing request (BUG-070-001 SCOPE-05,
// AUTH-011). It runs for POST/PUT/PATCH/DELETE, AFTER the request authenticator
// and BEFORE the route handler, and returns 403 BEFORE any state change on the
// first failing check. SameSite/Origin/CORS/POST-only are treated as
// necessary-not-sufficient; the additional acceptance evidence is a
// server-validated, session-bound, signed double-submit proof.
//
// The proof is a distinct, single-purpose, NON-authenticating value (design
// §The Proof Is Not Session Material): a valid proof with no valid session is
// unauthenticated (401), and a valid session with no matching proof is a
// forgery (403). It is stateless — no per-request database read and no schema.
//
//	csrf_proof = base64url(nonce) "." base64url(HMAC_SHA256(csrf_signing_key, aud "|" sub "|" jti "|" nonce))
//
// The csrf_signing_key is sourced fail-loud from the auth SST and is DISTINCT
// from the PASETO signing key. Here it is supplied through the injectable
// CSRFKeyProvider so the guard logic is fully unit-testable without the
// config-SST edit landing; the config-backed provider is the deferred
// coordination item (internal/config/validate_test.go is concurrently owned).
package api

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/smackerel/smackerel/internal/auth"
)

// MutationOutcome is the closed-set result of the guard. Exactly these five
// values are emitted; nothing else (design §Presentation And Telemetry).
type MutationOutcome string

const (
	// OutcomeAccepted — trusted origin + valid session-bound proof; proceed.
	OutcomeAccepted MutationOutcome = "accepted"
	// OutcomeOriginRejected — no trusted same-origin context.
	OutcomeOriginRejected MutationOutcome = "origin_rejected"
	// OutcomeCSRFMissing — no proof, or a proof with no matching companion cookie.
	OutcomeCSRFMissing MutationOutcome = "csrf_missing"
	// OutcomeCSRFStale — proof bound to a prior (rotated/expired) session jti.
	OutcomeCSRFStale MutationOutcome = "csrf_stale"
	// OutcomeCSRFMismatch — bad signature or proof bound to a different session.
	OutcomeCSRFMismatch MutationOutcome = "csrf_mismatch"
)

// MutationFamily labels the cookie-authenticated mutation surface for bounded
// telemetry only; it never changes the acceptance decision.
type MutationFamily string

const (
	FamilyForm  MutationFamily = "form"
	FamilyHTMX  MutationFamily = "htmx"
	FamilyPWA   MutationFamily = "pwa"
	FamilyJSON  MutationFamily = "json"
	FamilyCards MutationFamily = "cards"
	FamilyAdmin MutationFamily = "admin"
)

const (
	// csrfCompanionCookie is the non-authenticating, deliberately script-readable
	// double-submit vehicle (HttpOnly=false), distinct from the HttpOnly session
	// cookie.
	csrfCompanionCookie = "smackerel_csrf"
	// csrfFormField carries the proof from server-rendered forms and HTMX.
	csrfFormField = "_csrf"
	// csrfHeader carries the proof from PWA fetch / JSON / Cards / admin.
	csrfHeader = "X-CSRF-Token"
	// preSessionAudience binds a login/registration proof that predates any
	// session (no sub/jti).
	preSessionAudience = "smackerel-anon"
)

// browserSessionAudience is the aud value the proof binds to for an
// authenticated browser session. It mirrors auth.AudienceBrowserSession so the
// two subsystems agree on the binding namespace.
var browserSessionAudience = string(auth.AudienceBrowserSession)

// CSRFKeyProvider sources the dedicated csrf_signing_key fail-loud. It is
// DISTINCT from the PASETO signing key so a CSRF-key compromise cannot mint
// sessions and a PASETO-key compromise cannot forge proofs. Injectable so the
// guard logic is fully unit-testable without the config-SST edit landing.
type CSRFKeyProvider func() ([]byte, error)

// StaticCSRFKeyProvider returns a CSRFKeyProvider over an in-memory key. Used
// by wiring that already holds the resolved SST key and by tests. It fails loud
// on an empty key.
func StaticCSRFKeyProvider(key []byte) CSRFKeyProvider {
	return func() ([]byte, error) {
		if len(key) == 0 {
			return nil, errors.New("api: csrf signing key is empty (fail-loud SST)")
		}
		return append([]byte(nil), key...), nil
	}
}

// MutationTrustGuardConfig configures the guard. KeyProvider and AllowedOrigins
// are required; the remaining fields default to production-safe implementations
// (crypto/rand nonce, slog observer, header/path family inference).
type MutationTrustGuardConfig struct {
	KeyProvider      CSRFKeyProvider
	AllowedOrigins   []string
	NonceSource      func() (string, error)
	Observer         func(MutationOutcome, MutationFamily)
	FamilyClassifier func(*http.Request) MutationFamily
}

// MutationTrustGuard enforces trusted-Origin + session-bound signed
// double-submit proof for every cookie-authenticated mutation.
type MutationTrustGuard struct {
	key            []byte
	allowedOrigins map[string]struct{}
	newNonce       func() (string, error)
	observe        func(MutationOutcome, MutationFamily)
	classifyFamily func(*http.Request) MutationFamily
}

// NewMutationTrustGuard constructs the guard, resolving the CSRF signing key
// fail-loud at construction (a missing/empty key is a hard error, never a
// silent default) and requiring at least one trusted origin.
func NewMutationTrustGuard(cfg MutationTrustGuardConfig) (*MutationTrustGuard, error) {
	if cfg.KeyProvider == nil {
		return nil, errors.New("api: MutationTrustGuard requires a CSRFKeyProvider")
	}
	key, err := cfg.KeyProvider()
	if err != nil {
		return nil, fmt.Errorf("api: csrf signing key unavailable: %w", err)
	}
	if len(key) == 0 {
		return nil, errors.New("api: csrf signing key is empty (fail-loud SST)")
	}
	if len(cfg.AllowedOrigins) == 0 {
		return nil, errors.New("api: MutationTrustGuard requires at least one trusted origin")
	}
	allowed := make(map[string]struct{}, len(cfg.AllowedOrigins))
	for _, o := range cfg.AllowedOrigins {
		if o = strings.TrimRight(strings.TrimSpace(o), "/"); o != "" {
			allowed[o] = struct{}{}
		}
	}
	if len(allowed) == 0 {
		return nil, errors.New("api: MutationTrustGuard trusted origins were all empty")
	}
	g := &MutationTrustGuard{
		key:            append([]byte(nil), key...),
		allowedOrigins: allowed,
		newNonce:       cfg.NonceSource,
		observe:        cfg.Observer,
		classifyFamily: cfg.FamilyClassifier,
	}
	if g.newNonce == nil {
		g.newNonce = csrfRandomNonce
	}
	if g.observe == nil {
		g.observe = csrfDefaultObserve
	}
	if g.classifyFamily == nil {
		g.classifyFamily = csrfClassifyFamily
	}
	return g, nil
}

// MutationTrustResult is the guard's decision for one request.
type MutationTrustResult struct {
	Outcome MutationOutcome
	Family  MutationFamily
}

// proofClaims is the session binding for a proof (aud|sub|jti).
type proofClaims struct {
	aud string
	sub string
	jti string
}

// sessionProofClaims derives the binding from an authenticated browser session.
func sessionProofClaims(sess auth.Session) proofClaims {
	return proofClaims{aud: browserSessionAudience, sub: sess.UserID, jti: sess.TokenID}
}

// sign computes base64url(HMAC_SHA256(key, aud|sub|jti|nonce)).
func (g *MutationTrustGuard) sign(c proofClaims, nonce string) string {
	mac := hmac.New(sha256.New, g.key)
	mac.Write([]byte(c.aud + "|" + c.sub + "|" + c.jti + "|" + nonce))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// csrfEncodeProof assembles base64url(nonce) "." signature.
func csrfEncodeProof(nonce, sig string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(nonce)) + "." + sig
}

// csrfDecodeProof splits a proof into its nonce and signature.
func csrfDecodeProof(proof string) (nonce, sig string, ok bool) {
	dot := strings.IndexByte(proof, '.')
	if dot <= 0 || dot == len(proof)-1 {
		return "", "", false
	}
	nb, err := base64.RawURLEncoding.DecodeString(proof[:dot])
	if err != nil {
		return "", "", false
	}
	return string(nb), proof[dot+1:], true
}

// MintProof mints the session-bound proof for a browser session. Called
// wherever the session cookie is issued/rotated (password login, API-token
// exchange, rotation) so the companion cookie and echo channels can carry it.
func (g *MutationTrustGuard) MintProof(sess auth.Session) (string, error) {
	nonce, err := g.newNonce()
	if err != nil {
		return "", err
	}
	return csrfEncodeProof(nonce, g.sign(sessionProofClaims(sess), nonce)), nil
}

// MintPreSessionProof mints an anonymous proof for login/registration POSTs
// that run before any session exists (binds a random nonce, no sub/jti).
func (g *MutationTrustGuard) MintPreSessionProof() (string, error) {
	nonce, err := g.newNonce()
	if err != nil {
		return "", err
	}
	return csrfEncodeProof(nonce, g.sign(proofClaims{aud: preSessionAudience}, nonce)), nil
}

// Evaluate runs the ordered checks against an authenticated session and returns
// the first failing outcome (or accepted). priorTokenIDs are same-subject jtis
// still inside a rotation grace window (empty until the SCOPE-02 rotation
// history lands); a proof matching a prior jti is classified csrf_stale rather
// than csrf_mismatch. The guard reads no database.
func (g *MutationTrustGuard) Evaluate(r *http.Request, sess auth.Session, family MutationFamily, priorTokenIDs ...string) MutationTrustResult {
	// 1. Trusted same-origin context.
	if !g.trustedOrigin(r) {
		return MutationTrustResult{Outcome: OutcomeOriginRejected, Family: family}
	}
	// 2. Proof present.
	presented := csrfExtractPresentedProof(r)
	if presented == "" {
		return MutationTrustResult{Outcome: OutcomeCSRFMissing, Family: family}
	}
	// 3. Double-submit cross-check vs the companion cookie (constant-time). A
	//    header/field with no matching cookie is rejected.
	cookie, err := r.Cookie(csrfCompanionCookie)
	if err != nil || cookie.Value == "" {
		return MutationTrustResult{Outcome: OutcomeCSRFMissing, Family: family}
	}
	if !csrfConstantTimeEqual(presented, cookie.Value) {
		return MutationTrustResult{Outcome: OutcomeCSRFMismatch, Family: family}
	}
	// 4. Signature + binding to the authenticated session's aud|sub|jti.
	nonce, sig, ok := csrfDecodeProof(presented)
	if !ok {
		return MutationTrustResult{Outcome: OutcomeCSRFMismatch, Family: family}
	}
	current := sessionProofClaims(sess)
	if csrfConstantTimeEqual(sig, g.sign(current, nonce)) {
		return MutationTrustResult{Outcome: OutcomeAccepted, Family: family}
	}
	// A signature that instead binds a prior jti of the SAME subject is stale
	// (rotated/expired session), not a mismatch.
	for _, prior := range priorTokenIDs {
		if csrfConstantTimeEqual(sig, g.sign(proofClaims{aud: current.aud, sub: current.sub, jti: prior}, nonce)) {
			return MutationTrustResult{Outcome: OutcomeCSRFStale, Family: family}
		}
	}
	return MutationTrustResult{Outcome: OutcomeCSRFMismatch, Family: family}
}

// EvaluatePreSession runs the same trusted-Origin + signed double-submit check
// for a login/registration POST that predates any session (design §Pre-Session
// Mutations). The proof binds the anonymous audience with no sub/jti.
func (g *MutationTrustGuard) EvaluatePreSession(r *http.Request, family MutationFamily) MutationTrustResult {
	if !g.trustedOrigin(r) {
		return MutationTrustResult{Outcome: OutcomeOriginRejected, Family: family}
	}
	presented := csrfExtractPresentedProof(r)
	if presented == "" {
		return MutationTrustResult{Outcome: OutcomeCSRFMissing, Family: family}
	}
	cookie, err := r.Cookie(csrfCompanionCookie)
	if err != nil || cookie.Value == "" {
		return MutationTrustResult{Outcome: OutcomeCSRFMissing, Family: family}
	}
	if !csrfConstantTimeEqual(presented, cookie.Value) {
		return MutationTrustResult{Outcome: OutcomeCSRFMismatch, Family: family}
	}
	nonce, sig, ok := csrfDecodeProof(presented)
	if !ok {
		return MutationTrustResult{Outcome: OutcomeCSRFMismatch, Family: family}
	}
	if csrfConstantTimeEqual(sig, g.sign(proofClaims{aud: preSessionAudience}, nonce)) {
		return MutationTrustResult{Outcome: OutcomeAccepted, Family: family}
	}
	return MutationTrustResult{Outcome: OutcomeCSRFMismatch, Family: family}
}

// Middleware wraps a cookie-authenticated mutation handler. Safe methods pass
// through. For POST/PUT/PATCH/DELETE it reads the authenticated session (the
// guard runs AFTER the request authenticator), evaluates the trust checks, and
// on any non-accepted outcome returns 403 with one non-enumerating body BEFORE
// calling the handler — so no state change can occur on a forged request. It
// emits only bounded telemetry (outcome + family), never token/cookie/nonce or
// identity.
func (g *MutationTrustGuard) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !csrfIsMutatingMethod(r.Method) {
			next.ServeHTTP(w, r)
			return
		}
		sess, ok := auth.SessionFromContext(r.Context())
		if !ok {
			// No session ⇒ 401 territory, not a forgery (the proof authenticates
			// nothing). The request authenticator should have rejected first;
			// this is defense-in-depth.
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Valid authentication required")
			return
		}
		res := g.Evaluate(r, sess, g.classifyFamily(r))
		g.observe(res.Outcome, res.Family)
		if res.Outcome != OutcomeAccepted {
			// 403 BEFORE any state change; one non-enumerating body shared by
			// missing/stale/mismatch/origin. The operator role is subject to the
			// identical guard and never bypasses it.
			writeError(w, http.StatusForbidden, "FORBIDDEN", "request blocked")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// trustedOrigin reports whether the request carries a trusted same-origin
// context: the Origin header (or the Referer fallback when Origin is absent)
// resolves to an allowlisted origin. Absent/foreign origin ⇒ not trusted.
func (g *MutationTrustGuard) trustedOrigin(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		if ref := strings.TrimSpace(r.Header.Get("Referer")); ref != "" {
			origin = csrfOriginFromURL(ref)
		}
	}
	if origin == "" {
		return false
	}
	_, ok := g.allowedOrigins[strings.TrimRight(origin, "/")]
	return ok
}

// csrfExtractPresentedProof reads the proof from the X-CSRF-Token header
// (fetch/JSON/Cards/admin) or the _csrf form field (server forms/HTMX). The
// header takes precedence; the form field is read only for form-encoded bodies
// so a JSON body is never consumed.
func csrfExtractPresentedProof(r *http.Request) string {
	if h := strings.TrimSpace(r.Header.Get(csrfHeader)); h != "" {
		return h
	}
	if csrfIsFormEncoded(r) {
		if v := strings.TrimSpace(r.PostFormValue(csrfFormField)); v != "" {
			return v
		}
	}
	return ""
}

// csrfClassifyFamily infers the mutation family for telemetry. The live wiring
// may override this per mount (e.g. to label Cards).
func csrfClassifyFamily(r *http.Request) MutationFamily {
	if r.Header.Get("HX-Request") != "" {
		return FamilyHTMX
	}
	if strings.HasPrefix(r.URL.Path, "/admin") {
		return FamilyAdmin
	}
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		return FamilyJSON
	}
	if csrfIsFormEncoded(r) {
		return FamilyForm
	}
	return FamilyPWA
}

// csrfDefaultObserve emits bounded telemetry: outcome + family only, with no
// token, cookie, nonce, or identity value (design §Presentation And Telemetry).
func csrfDefaultObserve(outcome MutationOutcome, family MutationFamily) {
	slog.Info("mutation_trust_guard", "outcome", string(outcome), "family", string(family))
}

// csrfRandomNonce returns a fresh base64url nonce.
func csrfRandomNonce() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// csrfConstantTimeEqual compares two strings in constant time.
func csrfConstantTimeEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// csrfIsMutatingMethod reports whether the method changes state.
func csrfIsMutatingMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

// csrfIsFormEncoded reports whether the request body is form-encoded.
func csrfIsFormEncoded(r *http.Request) bool {
	ct := r.Header.Get("Content-Type")
	return strings.HasPrefix(ct, "application/x-www-form-urlencoded") ||
		strings.HasPrefix(ct, "multipart/form-data")
}

// csrfOriginFromURL returns scheme://host[:port] for a Referer URL, or "".
func csrfOriginFromURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}
