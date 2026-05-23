package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/smackerel/smackerel/internal/auth"
	"github.com/smackerel/smackerel/internal/connector/qfdecisions"
	"github.com/smackerel/smackerel/internal/knowledge"
	"github.com/smackerel/smackerel/internal/metrics"
)

// Spec 041 Scope 7 — personal-context read API host
// (GET /api/private/qf/v1/personal-context).
//
// Bound by:
//   - SCN-SM-041-025: consent_token gates the read; the response MUST
//     include the EXACT non-influence warning string declared below.
//   - SCN-SM-041-026: sensitivity tier ceiling is the lesser of the
//     consent ceiling and the per-user privacy ceiling; items above
//     the user ceiling are redacted and counted in redaction_count;
//     the route returns 403 on consent failures and 503 on capability
//     disabled.
//   - SCN-SM-041-027: per-token rate cap of 5 reads; the 6th attempt
//     responds 429 with Retry-After and increments
//     smackerel_qf_personal_context_reads_total with outcome
//     "rate_limited".
//
// The handler is non-mutating: it never writes to user state, never
// influences QF mandate/watch/trade decisions, and emits a
// Cross-Product Audit Envelope v1 on EVERY attempt (success, redaction,
// rejection, rate-limit, capability disabled).

// PersonalContextNonInfluenceWarning is the EXACT string the route
// MUST return as response.non_influence_warning on every successful and
// degraded read. It is not configurable, not optional, and not runtime
// disabled. Tests assert the byte-for-byte text.
const PersonalContextNonInfluenceWarning = "Personal context returned for QF calibration only. Smackerel does not, and MUST NOT, influence QF mandate, watch list, trade approval, or execution decisions."

// personalContextRateLimitRetryAfterSeconds is the documented
// Retry-After window (seconds) returned with 429 responses
// (SCN-SM-041-027). It matches PersonalContextConsentMaxTTL.
const personalContextRateLimitRetryAfterSeconds = 900

// PersonalContextCapabilityStore is the persisted-capability reader.
// The Scope 7 handler reuses the same persisted capability surface as
// the Scope 4 evidence-export handler so a single capability handshake
// gates both routes.
type PersonalContextCapabilityStore interface {
	GetCapability(ctx context.Context, sourceID string) (responseJSON string, fetchedAt time.Time, status string, err error)
}

// PersonalContextConsentValidator is the read-time atomic
// validate-and-increment surface. The handler depends on the interface
// (not the concrete *qfdecisions.PersonalContextConsentTokenStore) so
// tests can inject deterministic fakes.
type PersonalContextConsentValidator interface {
	AtomicConsumeRead(ctx context.Context, req qfdecisions.PersonalContextConsentValidateRequest) (qfdecisions.PersonalContextConsentToken, error)
}

// PersonalContextSensitivityQuerier returns the sensitivity-filtered
// items + redaction count. The handler depends on the interface (not
// the concrete *knowledge.PersonalContextSensitivityQuerier) so tests
// can inject deterministic fakes.
type PersonalContextSensitivityQuerier interface {
	QueryByEntityRef(ctx context.Context, req knowledge.PersonalContextQueryRequest) (knowledge.PersonalContextQueryResult, error)
}

// PersonalContextUserCeilingProvider returns the per-user privacy
// ceiling for the given user id. Returning an empty string or a
// ceiling outside the documented vocabulary is treated as the most
// restrictive tier ("low") so a misconfigured provider can never grant
// access above what the system actually permits.
type PersonalContextUserCeilingProvider interface {
	UserPrivacyCeiling(ctx context.Context, userID string) (string, error)
}

// PersonalContextHandlers implements the Scope 7 read route.
type PersonalContextHandlers struct {
	CapabilityStore PersonalContextCapabilityStore
	Consent         PersonalContextConsentValidator
	Items           PersonalContextSensitivityQuerier
	UserCeiling     PersonalContextUserCeilingProvider
	Now             func() time.Time
}

// NewPersonalContextHandlers returns a Scope 7 handler.
func NewPersonalContextHandlers(
	capabilityStore PersonalContextCapabilityStore,
	consent PersonalContextConsentValidator,
	items PersonalContextSensitivityQuerier,
	userCeiling PersonalContextUserCeilingProvider,
	now func() time.Time,
) *PersonalContextHandlers {
	if now == nil {
		now = time.Now
	}
	return &PersonalContextHandlers{
		CapabilityStore: capabilityStore,
		Consent:         consent,
		Items:           items,
		UserCeiling:     userCeiling,
		Now:             now,
	}
}

// PersonalContextResponse is the JSON contract returned on success and
// degraded reads (SCN-SM-041-025/026).
type PersonalContextResponse struct {
	Items               []knowledge.PersonalContextItem `json:"items"`
	RedactionCount      int                             `json:"redaction_count"`
	EffectiveTier       string                          `json:"effective_tier"`
	ConsentCeiling      string                          `json:"consent_ceiling"`
	UserCeiling         string                          `json:"user_ceiling"`
	NonInfluenceWarning string                          `json:"non_influence_warning"`
	TokenReadsUsed      int                             `json:"token_reads_used"`
	TokenReadsRemaining int                             `json:"token_reads_remaining"`
}

// Read implements GET /api/private/qf/v1/personal-context.
func (h *PersonalContextHandlers) Read(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.CapabilityStore == nil || h.Consent == nil || h.Items == nil || h.UserCeiling == nil {
		h.emitMetric("rejected", "")
		h.emitAudit("rejected", "personal_context_not_configured", "", "")
		writeError(w, http.StatusServiceUnavailable, "personal_context_unavailable", "personal-context read API is not configured")
		return
	}

	query := r.URL.Query()
	entityRef := strings.TrimSpace(query.Get("entity_ref"))
	requestedTier := strings.TrimSpace(query.Get("max_sensitivity"))
	consentToken := strings.TrimSpace(query.Get("consent_token"))
	requesterID := strings.TrimSpace(query.Get("requester_id"))
	if requesterID == "" {
		requesterID = strings.TrimSpace(auth.UserIDFromContext(r.Context()))
	}

	if entityRef == "" {
		h.emitMetric("rejected", requestedTier)
		h.emitAudit("rejected", "missing_entity_ref", requesterID, requestedTier)
		writeError(w, http.StatusBadRequest, "missing_entity_ref", "entity_ref query parameter is required")
		return
	}
	if requestedTier == "" {
		h.emitMetric("rejected", "")
		h.emitAudit("rejected", "missing_max_sensitivity", requesterID, "")
		writeError(w, http.StatusBadRequest, "missing_max_sensitivity", "max_sensitivity query parameter is required")
		return
	}
	if !isPersonalContextTier(requestedTier) {
		h.emitMetric("rejected", requestedTier)
		h.emitAudit("rejected", qfdecisions.PersonalContextErrConsentScopeViolation, requesterID, requestedTier)
		writeError(w, http.StatusBadRequest, qfdecisions.PersonalContextErrConsentScopeViolation, "max_sensitivity must be one of low|medium|high")
		return
	}
	if consentToken == "" {
		h.emitMetric("rejected", requestedTier)
		h.emitAudit("rejected", "missing_consent_token", requesterID, requestedTier)
		writeError(w, http.StatusBadRequest, "missing_consent_token", "consent_token query parameter is required")
		return
	}

	// Capability gate (SCN-SM-041-026 503 case).
	capability, err := h.loadCapability(r.Context())
	if err != nil {
		h.emitMetric("capability_disabled", requestedTier)
		h.emitAudit("capability_disabled", qfdecisions.PersonalContextErrDisabledByCapability, requesterID, requestedTier)
		writeError(w, http.StatusServiceUnavailable, qfdecisions.PersonalContextErrDisabledByCapability, err.Error())
		return
	}
	if !capability.PersonalContextPullSupported {
		h.emitMetric("capability_disabled", requestedTier)
		h.emitAudit("capability_disabled", qfdecisions.PersonalContextErrDisabledByCapability, requesterID, requestedTier)
		writeError(w, http.StatusServiceUnavailable, qfdecisions.PersonalContextErrDisabledByCapability,
			"QF capability personal_context_pull_supported is false; personal-context read is disabled")
		return
	}

	// Atomic validate + increment. The increment is recorded for EVERY
	// outcome (success, rejection, rate-limit) so the 5-read cap is
	// concurrency-safe (SCN-SM-041-027).
	token, validationErr := h.Consent.AtomicConsumeRead(r.Context(), qfdecisions.PersonalContextConsentValidateRequest{
		TokenID:                  consentToken,
		EntityRef:                entityRef,
		RequestedSensitivityTier: requestedTier,
	})
	if validationErr != nil {
		var v *qfdecisions.PersonalContextValidationError
		if errors.As(validationErr, &v) {
			outcome, status := outcomeAndStatusForConsentError(v.Code)
			h.emitMetric(outcome, requestedTier)
			h.emitAudit(outcome, v.Code, requesterID, requestedTier)
			if status == http.StatusTooManyRequests {
				w.Header().Set("Retry-After", strconv.Itoa(personalContextRateLimitRetryAfterSeconds))
			}
			writeError(w, status, v.Code, v.Message)
			return
		}
		h.emitMetric("rejected", requestedTier)
		h.emitAudit("rejected", "consent_validation_error", requesterID, requestedTier)
		writeError(w, http.StatusBadGateway, "consent_validation_error", validationErr.Error())
		return
	}

	// Per-user privacy ceiling lookup.
	userCeiling, err := h.UserCeiling.UserPrivacyCeiling(r.Context(), requesterID)
	if err != nil {
		h.emitMetric("rejected", requestedTier)
		h.emitAudit("rejected", "user_privacy_ceiling_unavailable", requesterID, requestedTier)
		writeError(w, http.StatusServiceUnavailable, "user_privacy_ceiling_unavailable",
			"per-user privacy ceiling lookup failed")
		return
	}
	userCeiling = strings.TrimSpace(userCeiling)
	if !isPersonalContextTier(userCeiling) {
		// Misconfigured / unknown user ceiling collapses to the
		// most-restrictive tier so we cannot accidentally over-share.
		userCeiling = qfdecisions.PersonalContextTierLow
	}

	// Effective ceiling = min(requestedTier, userCeiling). The
	// requestedTier is already <= consent ceiling (the consent store
	// enforces that), so this is equivalent to
	// min(consent, user, requested).
	consentCeiling := requestedTier
	effectiveTier := qfdecisions.PersonalContextTierMinimum(consentCeiling, userCeiling)

	result, err := h.Items.QueryByEntityRef(r.Context(), knowledge.PersonalContextQueryRequest{
		EntityRef:       entityRef,
		ConsentCeiling:  consentCeiling,
		UserCeiling:     userCeiling,
		TierLessOrEqual: qfdecisions.PersonalContextTierLessOrEqual,
	})
	if err != nil {
		h.emitMetric("rejected", requestedTier)
		h.emitAudit("rejected", "personal_context_query_failed", requesterID, requestedTier)
		writeError(w, http.StatusInternalServerError, "personal_context_query_failed", err.Error())
		return
	}

	outcome := "ok"
	if result.RedactionCount > 0 {
		outcome = "degraded"
	}
	h.emitMetric(outcome, requestedTier)
	h.emitAudit(outcome, "", requesterID, requestedTier)

	readsRemaining := qfdecisions.PersonalContextConsentMaxReads - token.ReadsUsed
	if readsRemaining < 0 {
		readsRemaining = 0
	}

	items := result.Items
	if items == nil {
		items = []knowledge.PersonalContextItem{}
	}

	writeJSON(w, http.StatusOK, PersonalContextResponse{
		Items:               items,
		RedactionCount:      result.RedactionCount,
		EffectiveTier:       effectiveTier,
		ConsentCeiling:      consentCeiling,
		UserCeiling:         userCeiling,
		NonInfluenceWarning: PersonalContextNonInfluenceWarning,
		TokenReadsUsed:      token.ReadsUsed,
		TokenReadsRemaining: readsRemaining,
	})
}

// loadCapability mirrors the QF evidence handler's persisted-capability
// reader so a single capability handshake gates both routes.
func (h *PersonalContextHandlers) loadCapability(ctx context.Context) (qfdecisions.QFBridgeCapability, error) {
	responseJSON, _, status, err := h.CapabilityStore.GetCapability(ctx, qfdecisions.DefaultConnectorID)
	if err != nil {
		return qfdecisions.QFBridgeCapability{}, errors.New("persisted QF capability is unavailable: " + err.Error())
	}
	if status != qfdecisions.CapabilityStatusCompatible {
		return qfdecisions.QFBridgeCapability{}, errors.New("persisted QF capability status is " + status + ", want " + qfdecisions.CapabilityStatusCompatible)
	}
	if strings.TrimSpace(responseJSON) == "" {
		return qfdecisions.QFBridgeCapability{}, errors.New("persisted QF capability response is empty")
	}
	var capability qfdecisions.QFBridgeCapability
	if err := json.Unmarshal([]byte(responseJSON), &capability); err != nil {
		return qfdecisions.QFBridgeCapability{}, errors.New("persisted QF capability response is unreadable: " + err.Error())
	}
	return capability, nil
}

// emitMetric increments the bounded-label personal-context read
// counter. Out-of-vocabulary outcome or sensitivity_tier values fall
// back to "rejected"/"unknown" so the registry never sees a label that
// could explode cardinality.
func (h *PersonalContextHandlers) emitMetric(outcome, sensitivityTier string) {
	if metrics.QFPersonalContextReadsTotal == nil {
		return
	}
	o := strings.TrimSpace(outcome)
	switch o {
	case "ok", "rejected", "degraded", "rate_limited", "capability_disabled":
	default:
		o = "rejected"
	}
	t := strings.TrimSpace(sensitivityTier)
	if !isPersonalContextTier(t) {
		t = "unknown"
	}
	metrics.QFPersonalContextReadsTotal.WithLabelValues(o, t).Inc()
}

// emitAudit emits the Cross-Product Audit Envelope v1 for the read
// attempt. The handler emits on EVERY attempt so the audit trail is
// 1:1 with the metric counter.
func (h *PersonalContextHandlers) emitAudit(outcome, reason, requesterID, sensitivityTier string) {
	auditOutcome := outcome
	switch outcome {
	case "ok":
		auditOutcome = qfdecisions.AuditOutcomeOK
	case "rejected":
		auditOutcome = qfdecisions.AuditOutcomeRejected
	case "degraded":
		auditOutcome = qfdecisions.AuditOutcomeDegraded
	case "rate_limited":
		auditOutcome = qfdecisions.AuditOutcomeRateLimited
	case "capability_disabled":
		auditOutcome = qfdecisions.AuditOutcomeCapabilityDisabled
	}
	now := time.Now().UTC()
	if h.Now != nil {
		now = h.Now().UTC()
	}
	actorRef := strings.TrimSpace(requesterID)
	envelope := qfdecisions.BuildCrossProductAuditEnvelopeV1(qfdecisions.AuditEnvelopeInput{
		Action:          qfdecisions.AuditActionPersonalContextRead,
		Outcome:         auditOutcome,
		Reason:          reason,
		ActorRef:        actorRef,
		SensitivityTier: sensitivityTier,
		ObservedAt:      now,
	})
	qfdecisions.EmitConnectorAuditEnvelope(envelope)
	slog.Info("qf-personal-context: read_attempt",
		"outcome", auditOutcome,
		"reason", reason,
		"sensitivity_tier", sensitivityTier,
	)
}

func isPersonalContextTier(tier string) bool {
	switch tier {
	case qfdecisions.PersonalContextTierLow,
		qfdecisions.PersonalContextTierMedium,
		qfdecisions.PersonalContextTierHigh:
		return true
	}
	return false
}

func outcomeAndStatusForConsentError(code string) (string, int) {
	switch code {
	case qfdecisions.PersonalContextErrRateLimitExceeded:
		return "rate_limited", http.StatusTooManyRequests
	case qfdecisions.PersonalContextErrConsentScopeViolation,
		qfdecisions.PersonalContextErrConsentExpired,
		qfdecisions.PersonalContextErrConsentCeilingRaised,
		qfdecisions.PersonalContextErrTokenRevoked:
		return "rejected", http.StatusForbidden
	case qfdecisions.PersonalContextErrTokenNotFound:
		return "rejected", http.StatusForbidden
	}
	return "rejected", http.StatusForbidden
}
