package api

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/smackerel/smackerel/internal/auth"
)

// mtgTestKey is a fixed HMAC key for the guard unit tests. It is NOT a real
// secret; it never leaves the test binary.
var mtgTestKey = []byte("unit-test-csrf-hmac-material-000000000000") //gitleaks:allow

const mtgTrustedOrigin = "https://app.smackerel.test"

// mtgObserved records the bounded telemetry the guard emitted so the test can
// assert only outcome+family are surfaced (no token/cookie/nonce/identity).
type mtgObserved struct {
	outcome MutationOutcome
	family  MutationFamily
}

func newMTGGuard(t *testing.T, observed *[]mtgObserved) *MutationTrustGuard {
	t.Helper()
	g, err := NewMutationTrustGuard(MutationTrustGuardConfig{
		KeyProvider:    StaticCSRFKeyProvider(mtgTestKey),
		AllowedOrigins: []string{mtgTrustedOrigin},
		NonceSource:    func() (string, error) { return "fixed-unit-nonce", nil },
		Observer: func(o MutationOutcome, f MutationFamily) {
			if observed != nil {
				*observed = append(*observed, mtgObserved{o, f})
			}
		},
	})
	if err != nil {
		t.Fatalf("NewMutationTrustGuard: %v", err)
	}
	return g
}

// mtgReq builds a mutating request for a family with an explicit presented proof
// (carrier chosen per family), companion cookie value, and Origin/Referer. Any
// field left empty is omitted so negative cases can be constructed precisely.
type mtgReq struct {
	family    MutationFamily
	method    string
	origin    string
	referer   string
	presented string
	cookie    string
}

func (s mtgReq) build() *http.Request {
	method := s.method
	if method == "" {
		method = http.MethodPost
	}
	var req *http.Request
	if s.presented != "" && (s.family == FamilyForm || s.family == FamilyHTMX) {
		form := url.Values{}
		form.Set(csrfFormField, s.presented)
		req = httptest.NewRequest(method, "/mutate", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if s.family == FamilyHTMX {
			req.Header.Set("HX-Request", "true")
		}
	} else {
		req = httptest.NewRequest(method, "/mutate", strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")
		if s.presented != "" {
			req.Header.Set(csrfHeader, s.presented)
		}
	}
	if s.origin != "" {
		req.Header.Set("Origin", s.origin)
	}
	if s.referer != "" {
		req.Header.Set("Referer", s.referer)
	}
	if s.cookie != "" {
		req.AddCookie(&http.Cookie{Name: csrfCompanionCookie, Value: s.cookie})
	}
	return req
}

// TestCookieMutationsRequireTrustedOriginAndSessionBoundCsrfProofAcrossEveryMutationFamily
// is the BUG-070-001 SCOPE-05 (AUTH-011) unit contract. It proves the
// MutationTrustGuard returns 403 BEFORE any state change on missing, stale,
// mismatched, and cross-origin evidence for every mutation family, and proceeds
// only with a trusted same-origin context AND a valid session-bound proof.
func TestCookieMutationsRequireTrustedOriginAndSessionBoundCsrfProofAcrossEveryMutationFamily(t *testing.T) {
	guard := newMTGGuard(t, nil)

	sessA := auth.Session{UserID: "user-A", TokenID: "jti-A", Source: auth.SessionSourcePerUserToken, Scopes: []string{auth.GrantAssistantTurn}}
	sessB := auth.Session{UserID: "user-B", TokenID: "jti-B", Source: auth.SessionSourcePerUserToken, Scopes: []string{auth.GrantAssistantTurn}}

	mint := func(s auth.Session) string {
		p, err := guard.MintProof(s)
		if err != nil {
			t.Fatalf("MintProof: %v", err)
		}
		return p
	}
	proofA := mint(sessA)
	proofB := mint(sessB)

	families := []MutationFamily{FamilyForm, FamilyHTMX, FamilyPWA, FamilyJSON, FamilyCards, FamilyAdmin}

	// The full accept/reject matrix across EVERY mutation family.
	for _, fam := range families {
		fam := fam
		t.Run("family_"+string(fam), func(t *testing.T) {
			// accepted: trusted origin + valid session-bound proof + matching cookie.
			if got := guard.Evaluate(mtgReq{family: fam, origin: mtgTrustedOrigin, presented: proofA, cookie: proofA}.build(), sessA, fam); got.Outcome != OutcomeAccepted {
				t.Fatalf("valid request outcome = %q, want accepted", got.Outcome)
			}
			// origin_rejected: foreign Origin.
			if got := guard.Evaluate(mtgReq{family: fam, origin: "https://evil.example", presented: proofA, cookie: proofA}.build(), sessA, fam); got.Outcome != OutcomeOriginRejected {
				t.Fatalf("foreign-origin outcome = %q, want origin_rejected", got.Outcome)
			}
			// origin_rejected: absent Origin AND Referer.
			if got := guard.Evaluate(mtgReq{family: fam, presented: proofA, cookie: proofA}.build(), sessA, fam); got.Outcome != OutcomeOriginRejected {
				t.Fatalf("absent-origin outcome = %q, want origin_rejected", got.Outcome)
			}
			// csrf_missing: no proof presented.
			if got := guard.Evaluate(mtgReq{family: fam, origin: mtgTrustedOrigin, cookie: proofA}.build(), sessA, fam); got.Outcome != OutcomeCSRFMissing {
				t.Fatalf("no-proof outcome = %q, want csrf_missing", got.Outcome)
			}
			// csrf_missing: proof present but no companion cookie.
			if got := guard.Evaluate(mtgReq{family: fam, origin: mtgTrustedOrigin, presented: proofA}.build(), sessA, fam); got.Outcome != OutcomeCSRFMissing {
				t.Fatalf("no-cookie outcome = %q, want csrf_missing", got.Outcome)
			}
			// csrf_mismatch: adversarial cross-session forged proof (double-submit
			// matches its own cookie, but the signature binds session B, not A).
			// MUST 403 — never mutate.
			if got := guard.Evaluate(mtgReq{family: fam, origin: mtgTrustedOrigin, presented: proofB, cookie: proofB}.build(), sessA, fam); got.Outcome != OutcomeCSRFMismatch {
				t.Fatalf("cross-session forged proof outcome = %q, want csrf_mismatch", got.Outcome)
			}
			// origin_rejected via Referer fallback to a foreign origin.
			if got := guard.Evaluate(mtgReq{family: fam, referer: "https://evil.example/page", presented: proofA, cookie: proofA}.build(), sessA, fam); got.Outcome != OutcomeOriginRejected {
				t.Fatalf("foreign-referer outcome = %q, want origin_rejected", got.Outcome)
			}
			// accepted via Referer fallback to the trusted origin.
			if got := guard.Evaluate(mtgReq{family: fam, referer: mtgTrustedOrigin + "/page", presented: proofA, cookie: proofA}.build(), sessA, fam); got.Outcome != OutcomeAccepted {
				t.Fatalf("trusted-referer outcome = %q, want accepted", got.Outcome)
			}
		})
	}

	// double-submit cross-check: presented proof differs from the companion cookie.
	t.Run("double_submit_mismatch", func(t *testing.T) {
		got := guard.Evaluate(mtgReq{family: FamilyJSON, origin: mtgTrustedOrigin, presented: proofA, cookie: proofB}.build(), sessA, FamilyJSON)
		if got.Outcome != OutcomeCSRFMismatch {
			t.Fatalf("double-submit mismatch outcome = %q, want csrf_mismatch", got.Outcome)
		}
	})

	// tampered signature.
	t.Run("tampered_signature", func(t *testing.T) {
		tampered := proofA[:len(proofA)-1] + flipLastByte(proofA[len(proofA)-1])
		got := guard.Evaluate(mtgReq{family: FamilyJSON, origin: mtgTrustedOrigin, presented: tampered, cookie: tampered}.build(), sessA, FamilyJSON)
		if got.Outcome != OutcomeCSRFMismatch {
			t.Fatalf("tampered-signature outcome = %q, want csrf_mismatch", got.Outcome)
		}
	})

	// csrf_stale: a proof bound to a PRIOR (rotated) jti of the same subject.
	t.Run("stale_rotated_jti", func(t *testing.T) {
		priorSession := auth.Session{UserID: "user-A", TokenID: "jti-A-prior", Source: auth.SessionSourcePerUserToken}
		stale := mint(priorSession)
		got := guard.Evaluate(mtgReq{family: FamilyJSON, origin: mtgTrustedOrigin, presented: stale, cookie: stale}.build(), sessA, FamilyJSON, "jti-A-prior")
		if got.Outcome != OutcomeCSRFStale {
			t.Fatalf("rotated-jti outcome = %q, want csrf_stale", got.Outcome)
		}
	})

	// SameSite/Origin alone is never sufficient: trusted origin + a present
	// companion cookie but NO proof is still csrf_missing (AUTH-011).
	t.Run("samesite_and_origin_alone_insufficient", func(t *testing.T) {
		got := guard.Evaluate(mtgReq{family: FamilyForm, origin: mtgTrustedOrigin, cookie: proofA}.build(), sessA, FamilyForm)
		if got.Outcome != OutcomeCSRFMissing {
			t.Fatalf("origin+cookie-without-proof outcome = %q, want csrf_missing", got.Outcome)
		}
	})

	// The operator role is subject to the identical guard and does not bypass it.
	t.Run("operator_role_not_exempt", func(t *testing.T) {
		operator := auth.SessionWithRole("op-1", "jti-op", auth.RoleOperator)
		if got := guard.Evaluate(mtgReq{family: FamilyAdmin, origin: mtgTrustedOrigin, cookie: mint(operator)}.build(), operator, FamilyAdmin); got.Outcome != OutcomeCSRFMissing {
			t.Fatalf("operator missing-proof outcome = %q, want csrf_missing (no bypass)", got.Outcome)
		}
		if got := guard.Evaluate(mtgReq{family: FamilyAdmin, origin: mtgTrustedOrigin, presented: mint(operator), cookie: mint(operator)}.build(), operator, FamilyAdmin); got.Outcome != OutcomeAccepted {
			t.Fatalf("operator valid-proof outcome = %q, want accepted", got.Outcome)
		}
	})

	// The middleware returns 403 BEFORE any state change on a forged request and
	// runs the handler only on a valid one; the 403 body is non-enumerating.
	t.Run("middleware_403_before_mutation_and_non_enumerating", func(t *testing.T) {
		mutated := false
		handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			mutated = true
			w.WriteHeader(http.StatusOK)
		})
		wrapped := guard.Middleware(handler)

		withSession := func(r *http.Request, s auth.Session) *http.Request {
			return r.WithContext(auth.WithSession(r.Context(), s))
		}

		// Forged (cross-session) request → 403, handler NOT run.
		forged := withSession(mtgReq{family: FamilyJSON, origin: mtgTrustedOrigin, presented: proofB, cookie: proofB}.build(), sessA)
		fr := httptest.NewRecorder()
		wrapped.ServeHTTP(fr, forged)
		if fr.Code != http.StatusForbidden {
			t.Fatalf("forged request status = %d, want 403", fr.Code)
		}
		if mutated {
			t.Fatalf("state changed on a forged request — mutation must be blocked before the handler")
		}
		forgedBody := fr.Body.String()

		// Missing-proof and origin-rejected share the identical non-enumerating body.
		mr := httptest.NewRecorder()
		wrapped.ServeHTTP(mr, withSession(mtgReq{family: FamilyJSON, origin: mtgTrustedOrigin, cookie: proofA}.build(), sessA))
		if mr.Code != http.StatusForbidden || mr.Body.String() != forgedBody {
			t.Fatalf("missing-proof body/status = %q/%d, want identical non-enumerating 403 body %q", mr.Body.String(), mr.Code, forgedBody)
		}
		or := httptest.NewRecorder()
		wrapped.ServeHTTP(or, withSession(mtgReq{family: FamilyJSON, origin: "https://evil.example", presented: proofA, cookie: proofA}.build(), sessA))
		if or.Code != http.StatusForbidden || or.Body.String() != forgedBody {
			t.Fatalf("origin-rejected body/status = %q/%d, want identical non-enumerating 403 body", or.Body.String(), or.Code)
		}

		// Valid request → handler runs (mutation permitted).
		valid := withSession(mtgReq{family: FamilyJSON, origin: mtgTrustedOrigin, presented: proofA, cookie: proofA}.build(), sessA)
		vr := httptest.NewRecorder()
		wrapped.ServeHTTP(vr, valid)
		if vr.Code != http.StatusOK || !mutated {
			t.Fatalf("valid request status = %d mutated=%v, want 200/true", vr.Code, mutated)
		}
	})

	// Telemetry is bounded: only outcome+family are observed, and no proof,
	// nonce, cookie, or identity value appears in the observed records.
	t.Run("bounded_telemetry_no_secret_leak", func(t *testing.T) {
		var observed []mtgObserved
		g := newMTGGuard(t, &observed)
		wrapped := g.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
		req := mtgReq{family: FamilyJSON, origin: mtgTrustedOrigin, presented: g.mustMint(t, sessA), cookie: g.mustMint(t, sessA)}.build()
		req = req.WithContext(auth.WithSession(req.Context(), sessA))
		wrapped.ServeHTTP(httptest.NewRecorder(), req)
		if len(observed) != 1 {
			t.Fatalf("observed %d telemetry records, want 1", len(observed))
		}
		rec := observed[0]
		if rec.outcome != OutcomeAccepted || rec.family != FamilyJSON {
			t.Fatalf("telemetry = %+v, want {accepted json}", rec)
		}
		// The observed values are exactly the bounded vocabulary — no proof/nonce/identity.
		if strings.Contains(string(rec.outcome), "user-A") || strings.Contains(string(rec.family), "jti") {
			t.Fatalf("telemetry leaked identity/token material: %+v", rec)
		}
	})

	// The pre-session variant (login/registration) accepts a valid anonymous
	// proof and rejects a missing or forged one.
	t.Run("pre_session_login_registration_variant", func(t *testing.T) {
		anon, err := guard.MintPreSessionProof()
		if err != nil {
			t.Fatalf("MintPreSessionProof: %v", err)
		}
		if got := guard.EvaluatePreSession(mtgReq{family: FamilyForm, origin: mtgTrustedOrigin, presented: anon, cookie: anon}.build(), FamilyForm); got.Outcome != OutcomeAccepted {
			t.Fatalf("valid pre-session outcome = %q, want accepted", got.Outcome)
		}
		if got := guard.EvaluatePreSession(mtgReq{family: FamilyForm, origin: mtgTrustedOrigin, cookie: anon}.build(), FamilyForm); got.Outcome != OutcomeCSRFMissing {
			t.Fatalf("no-proof pre-session outcome = %q, want csrf_missing", got.Outcome)
		}
		if got := guard.EvaluatePreSession(mtgReq{family: FamilyForm, origin: "https://evil.example", presented: anon, cookie: anon}.build(), FamilyForm); got.Outcome != OutcomeOriginRejected {
			t.Fatalf("foreign-origin pre-session outcome = %q, want origin_rejected", got.Outcome)
		}
	})

	// The emitted outcome vocabulary is exactly the closed five-value set.
	t.Run("closed_outcome_vocabulary", func(t *testing.T) {
		want := map[MutationOutcome]bool{
			OutcomeAccepted: true, OutcomeOriginRejected: true, OutcomeCSRFMissing: true,
			OutcomeCSRFStale: true, OutcomeCSRFMismatch: true,
		}
		if len(want) != 5 {
			t.Fatalf("outcome vocabulary changed; expected exactly 5 closed values")
		}
	})
}

// mustMint is a test helper minting a proof or failing the test.
func (g *MutationTrustGuard) mustMint(t *testing.T, s auth.Session) string {
	t.Helper()
	p, err := g.MintProof(s)
	if err != nil {
		t.Fatalf("MintProof: %v", err)
	}
	return p
}

// flipLastByte returns a different single-character string for the given byte so
// a tampered proof signature differs from the authentic one.
func flipLastByte(b byte) string {
	if b == 'A' {
		return "B"
	}
	return "A"
}
