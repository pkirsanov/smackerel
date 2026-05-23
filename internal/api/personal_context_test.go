package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/smackerel/smackerel/internal/connector/qfdecisions"
	"github.com/smackerel/smackerel/internal/knowledge"
	"github.com/smackerel/smackerel/internal/metrics"
)

// Spec 041 Scope 7 — handler unit tests (SCN-SM-041-025/026/027).
//
// These tests exercise the route without a live database by injecting
// fakes for the capability store, consent validator, sensitivity
// querier, and per-user ceiling provider. They prove the documented
// failure matrix, the EXACT non-influence warning string, the metric
// label vocabulary, and the rate-limit Retry-After contract.

type fakePersonalContextCapabilityStore struct {
	responseJSON string
	status       string
	err          error
}

func (s fakePersonalContextCapabilityStore) GetCapability(_ context.Context, _ string) (string, time.Time, string, error) {
	return s.responseJSON, time.Now().UTC(), s.status, s.err
}

type fakePersonalContextConsent struct {
	calls []qfdecisions.PersonalContextConsentValidateRequest
	token qfdecisions.PersonalContextConsentToken
	err   error
}

func (f *fakePersonalContextConsent) AtomicConsumeRead(_ context.Context, req qfdecisions.PersonalContextConsentValidateRequest) (qfdecisions.PersonalContextConsentToken, error) {
	f.calls = append(f.calls, req)
	return f.token, f.err
}

type fakePersonalContextItems struct {
	calls  []knowledge.PersonalContextQueryRequest
	result knowledge.PersonalContextQueryResult
	err    error
}

func (f *fakePersonalContextItems) QueryByEntityRef(_ context.Context, req knowledge.PersonalContextQueryRequest) (knowledge.PersonalContextQueryResult, error) {
	f.calls = append(f.calls, req)
	return f.result, f.err
}

type fakePersonalContextUserCeiling struct {
	tier string
	err  error
}

func (f fakePersonalContextUserCeiling) UserPrivacyCeiling(_ context.Context, _ string) (string, error) {
	return f.tier, f.err
}

func capabilityJSONWithPersonalContext(t *testing.T, enabled bool) string {
	t.Helper()
	cap := qfdecisions.QFBridgeCapability{
		PersonalContextPullSupported: enabled,
	}
	buf, err := json.Marshal(cap)
	if err != nil {
		t.Fatalf("marshal capability: %v", err)
	}
	return string(buf)
}

func TestPersonalContextRead_HappyPath_NonInfluenceWarningExactString(t *testing.T) {
	// SCN-SM-041-025 — successful read returns items, includes the EXACT
	// mandatory non-influence warning string, advertises the consent
	// + user ceilings, and emits outcome=ok.
	metrics.QFPersonalContextReadsTotal.Reset()

	cap := fakePersonalContextCapabilityStore{
		responseJSON: capabilityJSONWithPersonalContext(t, true),
		status:       qfdecisions.CapabilityStatusCompatible,
	}
	consent := &fakePersonalContextConsent{
		token: qfdecisions.PersonalContextConsentToken{
			TokenID:            "pct_test",
			EntityRef:          "user:42",
			MaxSensitivityTier: qfdecisions.PersonalContextTierHigh,
			ReadsUsed:          1,
		},
	}
	items := &fakePersonalContextItems{
		result: knowledge.PersonalContextQueryResult{
			Items: []knowledge.PersonalContextItem{
				{
					ArtifactID:      "art-1",
					Kind:            "note",
					SensitivityTier: qfdecisions.PersonalContextTierLow,
					Summary:         "low-tier note",
					SourceRef:       "https://example.invalid/n",
					CapturedAt:      time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC),
				},
			},
			EffectiveTier: qfdecisions.PersonalContextTierLow,
		},
	}
	user := fakePersonalContextUserCeiling{tier: qfdecisions.PersonalContextTierHigh}

	h := NewPersonalContextHandlers(cap, consent, items, user, func() time.Time { return time.Now().UTC() })
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/private/qf/v1/personal-context?entity_ref=user:42&max_sensitivity=high&consent_token=pct_test&requester_id=qf-test", nil)
	h.Read(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d body %s, want 200", rec.Code, rec.Body.String())
	}
	var resp PersonalContextResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v body=%s", err, rec.Body.String())
	}
	// EXACT non-influence warning. This adversarial check would fail if
	// the constant is ever softened or made configurable.
	if resp.NonInfluenceWarning != PersonalContextNonInfluenceWarning {
		t.Fatalf("non_influence_warning mismatch:\n got %q\nwant %q", resp.NonInfluenceWarning, PersonalContextNonInfluenceWarning)
	}
	if got, want := resp.NonInfluenceWarning, "Personal context returned for QF calibration only. Smackerel does not, and MUST NOT, influence QF mandate, watch list, trade approval, or execution decisions."; got != want {
		t.Fatalf("non_influence_warning is not the documented byte sequence:\n got %q\nwant %q", got, want)
	}
	if len(resp.Items) != 1 {
		t.Fatalf("got %d items, want 1", len(resp.Items))
	}
	if resp.RedactionCount != 0 {
		t.Fatalf("redaction_count=%d, want 0", resp.RedactionCount)
	}
	if resp.ConsentCeiling != qfdecisions.PersonalContextTierHigh {
		t.Fatalf("consent_ceiling=%q, want high", resp.ConsentCeiling)
	}
	if resp.UserCeiling != qfdecisions.PersonalContextTierHigh {
		t.Fatalf("user_ceiling=%q, want high", resp.UserCeiling)
	}
	if resp.EffectiveTier != qfdecisions.PersonalContextTierHigh {
		t.Fatalf("effective_tier=%q, want high", resp.EffectiveTier)
	}
	if resp.TokenReadsUsed != 1 {
		t.Fatalf("token_reads_used=%d, want 1", resp.TokenReadsUsed)
	}
	if resp.TokenReadsRemaining != qfdecisions.PersonalContextConsentMaxReads-1 {
		t.Fatalf("token_reads_remaining=%d, want %d", resp.TokenReadsRemaining, qfdecisions.PersonalContextConsentMaxReads-1)
	}
	got := testutilCounterValue(t, metrics.QFPersonalContextReadsTotal, "ok", qfdecisions.PersonalContextTierHigh)
	if got != 1 {
		t.Fatalf("ok counter increment = %v, want 1", got)
	}
}

func TestPersonalContextRead_DegradedWhenUserCeilingRedacts(t *testing.T) {
	// SCN-SM-041-026 — items above the user ceiling are counted in
	// redaction_count and the outcome metric label is "degraded".
	metrics.QFPersonalContextReadsTotal.Reset()

	cap := fakePersonalContextCapabilityStore{
		responseJSON: capabilityJSONWithPersonalContext(t, true),
		status:       qfdecisions.CapabilityStatusCompatible,
	}
	consent := &fakePersonalContextConsent{
		token: qfdecisions.PersonalContextConsentToken{
			TokenID:            "pct_test",
			EntityRef:          "user:42",
			MaxSensitivityTier: qfdecisions.PersonalContextTierHigh,
			ReadsUsed:          1,
		},
	}
	items := &fakePersonalContextItems{
		result: knowledge.PersonalContextQueryResult{
			Items: []knowledge.PersonalContextItem{
				{ArtifactID: "art-low", SensitivityTier: qfdecisions.PersonalContextTierLow},
			},
			RedactionCount: 2,
			EffectiveTier:  qfdecisions.PersonalContextTierLow,
		},
	}
	user := fakePersonalContextUserCeiling{tier: qfdecisions.PersonalContextTierLow}

	h := NewPersonalContextHandlers(cap, consent, items, user, nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/private/qf/v1/personal-context?entity_ref=user:42&max_sensitivity=high&consent_token=pct_test&requester_id=qf-test", nil)
	h.Read(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d body %s, want 200", rec.Code, rec.Body.String())
	}
	var resp PersonalContextResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.RedactionCount != 2 {
		t.Fatalf("redaction_count=%d, want 2", resp.RedactionCount)
	}
	if resp.EffectiveTier != qfdecisions.PersonalContextTierLow {
		t.Fatalf("effective_tier=%q, want low", resp.EffectiveTier)
	}
	got := testutilCounterValue(t, metrics.QFPersonalContextReadsTotal, "degraded", qfdecisions.PersonalContextTierHigh)
	if got != 1 {
		t.Fatalf("degraded counter = %v, want 1", got)
	}
}

func TestPersonalContextRead_CapabilityDisabledReturns503(t *testing.T) {
	// SCN-SM-041-026 — capability gate failure returns 503 with
	// PERSONAL_CONTEXT_DISABLED_BY_CAPABILITY and metric outcome
	// capability_disabled.
	metrics.QFPersonalContextReadsTotal.Reset()

	cap := fakePersonalContextCapabilityStore{
		responseJSON: capabilityJSONWithPersonalContext(t, false),
		status:       qfdecisions.CapabilityStatusCompatible,
	}
	consent := &fakePersonalContextConsent{}
	items := &fakePersonalContextItems{}
	user := fakePersonalContextUserCeiling{tier: qfdecisions.PersonalContextTierHigh}

	h := NewPersonalContextHandlers(cap, consent, items, user, nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/private/qf/v1/personal-context?entity_ref=user:42&max_sensitivity=medium&consent_token=pct_test&requester_id=qf-test", nil)
	h.Read(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("got status %d body %s, want 503", rec.Code, rec.Body.String())
	}
	if !containsCode(t, rec.Body.Bytes(), qfdecisions.PersonalContextErrDisabledByCapability) {
		t.Fatalf("body=%s does not contain %s", rec.Body.String(), qfdecisions.PersonalContextErrDisabledByCapability)
	}
	if len(consent.calls) != 0 {
		t.Fatalf("consent validator should not be called on capability failure; got %d calls", len(consent.calls))
	}
	got := testutilCounterValue(t, metrics.QFPersonalContextReadsTotal, "capability_disabled", qfdecisions.PersonalContextTierMedium)
	if got != 1 {
		t.Fatalf("capability_disabled counter = %v, want 1", got)
	}
}

func TestPersonalContextRead_ConsentScopeViolationReturns403(t *testing.T) {
	// SCN-SM-041-026 — consent scope mismatch returns 403 with the
	// documented code and metric outcome "rejected".
	metrics.QFPersonalContextReadsTotal.Reset()
	cap := fakePersonalContextCapabilityStore{
		responseJSON: capabilityJSONWithPersonalContext(t, true),
		status:       qfdecisions.CapabilityStatusCompatible,
	}
	consent := &fakePersonalContextConsent{
		token: qfdecisions.PersonalContextConsentToken{TokenID: "pct_test", ReadsUsed: 1},
		err: &qfdecisions.PersonalContextValidationError{
			Code:    qfdecisions.PersonalContextErrConsentScopeViolation,
			Message: "entity_ref does not match consent binding",
		},
	}
	items := &fakePersonalContextItems{}
	user := fakePersonalContextUserCeiling{tier: qfdecisions.PersonalContextTierHigh}
	h := NewPersonalContextHandlers(cap, consent, items, user, nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/private/qf/v1/personal-context?entity_ref=user:42&max_sensitivity=high&consent_token=pct_test&requester_id=qf-test", nil)
	h.Read(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("got status %d body %s, want 403", rec.Code, rec.Body.String())
	}
	if !containsCode(t, rec.Body.Bytes(), qfdecisions.PersonalContextErrConsentScopeViolation) {
		t.Fatalf("body=%s does not contain %s", rec.Body.String(), qfdecisions.PersonalContextErrConsentScopeViolation)
	}
	if got := testutilCounterValue(t, metrics.QFPersonalContextReadsTotal, "rejected", qfdecisions.PersonalContextTierHigh); got != 1 {
		t.Fatalf("rejected counter = %v, want 1", got)
	}
}

func TestPersonalContextRead_ConsentExpiredReturns403(t *testing.T) {
	// SCN-SM-041-026 — expired token returns 403 PERSONAL_CONTEXT_CONSENT_EXPIRED.
	metrics.QFPersonalContextReadsTotal.Reset()
	cap := fakePersonalContextCapabilityStore{
		responseJSON: capabilityJSONWithPersonalContext(t, true),
		status:       qfdecisions.CapabilityStatusCompatible,
	}
	consent := &fakePersonalContextConsent{
		token: qfdecisions.PersonalContextConsentToken{TokenID: "pct_test", ReadsUsed: 1},
		err: &qfdecisions.PersonalContextValidationError{
			Code:    qfdecisions.PersonalContextErrConsentExpired,
			Message: "expired",
		},
	}
	h := NewPersonalContextHandlers(cap, consent, &fakePersonalContextItems{}, fakePersonalContextUserCeiling{tier: qfdecisions.PersonalContextTierHigh}, nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/private/qf/v1/personal-context?entity_ref=user:42&max_sensitivity=low&consent_token=pct_test&requester_id=qf-test", nil)
	h.Read(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s want 403", rec.Code, rec.Body.String())
	}
	if !containsCode(t, rec.Body.Bytes(), qfdecisions.PersonalContextErrConsentExpired) {
		t.Fatalf("body=%s missing %s", rec.Body.String(), qfdecisions.PersonalContextErrConsentExpired)
	}
}

func TestPersonalContextRead_RateLimitReturns429WithRetryAfter(t *testing.T) {
	// SCN-SM-041-027 — 6th attempt returns 429 + Retry-After header +
	// metric outcome rate_limited. The handler counts EVERY attempt
	// regardless of outcome so the cap is concurrency-safe.
	metrics.QFPersonalContextReadsTotal.Reset()
	cap := fakePersonalContextCapabilityStore{
		responseJSON: capabilityJSONWithPersonalContext(t, true),
		status:       qfdecisions.CapabilityStatusCompatible,
	}
	consent := &fakePersonalContextConsent{
		token: qfdecisions.PersonalContextConsentToken{
			TokenID:   "pct_test",
			ReadsUsed: qfdecisions.PersonalContextConsentMaxReads + 1,
		},
		err: &qfdecisions.PersonalContextValidationError{
			Code:    qfdecisions.PersonalContextErrRateLimitExceeded,
			Message: "5-read cap exceeded",
		},
	}
	h := NewPersonalContextHandlers(cap, consent, &fakePersonalContextItems{}, fakePersonalContextUserCeiling{tier: qfdecisions.PersonalContextTierHigh}, nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/private/qf/v1/personal-context?entity_ref=user:42&max_sensitivity=medium&consent_token=pct_test&requester_id=qf-test", nil)
	h.Read(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status=%d body=%s want 429", rec.Code, rec.Body.String())
	}
	retry := rec.Header().Get("Retry-After")
	if retry == "" {
		t.Fatalf("Retry-After header missing on 429")
	}
	if _, err := strconv.Atoi(retry); err != nil {
		t.Fatalf("Retry-After header %q is not numeric: %v", retry, err)
	}
	if got := testutilCounterValue(t, metrics.QFPersonalContextReadsTotal, "rate_limited", qfdecisions.PersonalContextTierMedium); got != 1 {
		t.Fatalf("rate_limited counter = %v, want 1", got)
	}
}

func TestPersonalContextRead_InvalidMaxSensitivityReturns400(t *testing.T) {
	// Vocabulary guard — max_sensitivity outside {low,medium,high}
	// returns 400 PERSONAL_CONTEXT_CONSENT_SCOPE_VIOLATION; the consent
	// validator is not called.
	metrics.QFPersonalContextReadsTotal.Reset()
	consent := &fakePersonalContextConsent{}
	h := NewPersonalContextHandlers(
		fakePersonalContextCapabilityStore{
			responseJSON: capabilityJSONWithPersonalContext(t, true),
			status:       qfdecisions.CapabilityStatusCompatible,
		},
		consent,
		&fakePersonalContextItems{},
		fakePersonalContextUserCeiling{tier: qfdecisions.PersonalContextTierHigh},
		nil,
	)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/private/qf/v1/personal-context?entity_ref=user:42&max_sensitivity=ULTRA&consent_token=pct_test", nil)
	h.Read(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s want 400", rec.Code, rec.Body.String())
	}
	if len(consent.calls) != 0 {
		t.Fatalf("consent validator should not be called for invalid tier; got %d", len(consent.calls))
	}
}

func TestPersonalContextRead_MissingConsentTokenReturns400(t *testing.T) {
	consent := &fakePersonalContextConsent{}
	h := NewPersonalContextHandlers(
		fakePersonalContextCapabilityStore{
			responseJSON: capabilityJSONWithPersonalContext(t, true),
			status:       qfdecisions.CapabilityStatusCompatible,
		},
		consent,
		&fakePersonalContextItems{},
		fakePersonalContextUserCeiling{tier: qfdecisions.PersonalContextTierHigh},
		nil,
	)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/private/qf/v1/personal-context?entity_ref=user:42&max_sensitivity=medium", nil)
	h.Read(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s want 400", rec.Code, rec.Body.String())
	}
	if len(consent.calls) != 0 {
		t.Fatalf("consent validator should not be called when token is missing; got %d", len(consent.calls))
	}
}

// containsCode reports whether the error response body's error.code
// equals want.
func containsCode(t *testing.T, body []byte, want string) bool {
	t.Helper()
	var env ErrorResponse
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("unmarshal error response: %v body=%s", err, string(body))
	}
	return env.Error.Code == want
}

// testutilCounterValue returns the current value of the personal-context
// reads counter for the given (outcome, sensitivity_tier) label set
// using the standard prometheus testutil helper.
func testutilCounterValue(t *testing.T, vec *prometheus.CounterVec, outcome, tier string) float64 {
	t.Helper()
	c, err := vec.GetMetricWithLabelValues(outcome, tier)
	if err != nil {
		t.Fatalf("get counter with labels (%q,%q): %v", outcome, tier, err)
	}
	return testutil.ToFloat64(c)
}
