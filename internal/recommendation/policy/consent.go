package policy

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// ConsentNamedValues captures the user-reviewed knobs that determine whether a
// watch may be enabled or broadened. Every key is required; missing keys MUST
// be treated as "not confirmed" so consent always has a positive expression.
type ConsentNamedValues struct {
	Scope            map[string]any
	Sources          []string
	DeliveryChannel  string
	MaxAlerts        int
	WindowSeconds    int
	Precision        string
	HardConstraints  []string
	SponsoredAllowed bool
}

// ConsentRevision is a single immutable entry in
// `recommendation_watches.consent.revisions[]`.
type ConsentRevision struct {
	At          time.Time          `json:"at"`
	NamedValues ConsentNamedValues `json:"named_values"`
	Reason      string             `json:"reason"`
}

// ConsentRecord is the persisted JSONB shape behind the watch consent column.
// It MUST always carry a populated `current` snapshot; an empty `current` means
// the watch has never been consented and cannot be enabled.
type ConsentRecord struct {
	Current   ConsentNamedValues `json:"current"`
	Revisions []ConsentRevision  `json:"revisions"`
}

// ConsentConfirmation is the API client-supplied confirmation payload sent on
// create/enable/broaden requests.
type ConsentConfirmation struct {
	ScopeNamed       bool `json:"scope_named"`
	SourcesNamed     bool `json:"sources_named"`
	RateLimitNamed   bool `json:"rate_limit_named"`
	PrecisionNamed   bool `json:"precision_named"`
	DeliveryNamed    bool `json:"delivery_named"`
	ConstraintsNamed bool `json:"constraints_named"`
	SponsoredNamed   bool `json:"sponsored_named"`
}

// ConsentReason enumerates the ledger-visible reasons we record a revision.
const (
	ConsentReasonCreate  = "create"
	ConsentReasonEnable  = "enable"
	ConsentReasonBroaden = "broaden"
	ConsentReasonReplace = "replace"
)

// ErrConsentRequired indicates that a create/enable/broaden request lacks the
// consent confirmation flags required to proceed.
type ErrConsentRequired struct {
	Code            string   // always "CONSENT_REQUIRED"
	Reason          string   // one of: create | enable | broaden
	MissingFlags    []string // unconfirmed flags from ConsentConfirmation
	BroadenedFields []string // fields that broadened relative to the prior revision (broaden only)
}

func (e *ErrConsentRequired) Error() string {
	parts := []string{"consent required: " + e.Reason}
	if len(e.MissingFlags) > 0 {
		parts = append(parts, "missing="+strings.Join(e.MissingFlags, ","))
	}
	if len(e.BroadenedFields) > 0 {
		parts = append(parts, "broadened="+strings.Join(e.BroadenedFields, ","))
	}
	return strings.Join(parts, " ")
}

// IsConsentRequired returns true when the supplied error is an
// *ErrConsentRequired.
func IsConsentRequired(err error) bool {
	var target *ErrConsentRequired
	return errors.As(err, &target)
}

// ConsentDecision is the return value of EvaluateConsent.
type ConsentDecision struct {
	Reason          string   // create | enable | broaden | unchanged
	BroadenedFields []string // populated when Reason == "broaden"
	Required        []string // every consent flag the request needs
}

// EvaluateConsent classifies a draft watch update against the prior consent
// record. The reason is one of:
//
//	create     — no prior current snapshot (first-time consent)
//	enable     — prior snapshot existed but draft enables a previously disabled watch
//	broaden    — draft expands scope/sources/delivery/rate/precision/constraints
//	unchanged  — no consent revision required
func EvaluateConsent(prior ConsentRecord, draft ConsentNamedValues, prevEnabled, draftEnabled bool) ConsentDecision {
	if isEmptyConsent(prior.Current) {
		return ConsentDecision{
			Reason:   ConsentReasonCreate,
			Required: requiredFlagsForDraft(draft),
		}
	}
	broadened := broadenedFields(prior.Current, draft)
	if len(broadened) > 0 {
		return ConsentDecision{
			Reason:          ConsentReasonBroaden,
			BroadenedFields: broadened,
			Required:        requiredFlagsForBroadening(broadened),
		}
	}
	if !prevEnabled && draftEnabled {
		return ConsentDecision{
			Reason:   ConsentReasonEnable,
			Required: []string{"scope_named", "sources_named", "rate_limit_named", "precision_named"},
		}
	}
	return ConsentDecision{Reason: "unchanged"}
}

// CheckConfirmation asserts that the supplied confirmation flags satisfy the
// decision's required flag set. When unsatisfied, it returns an
// *ErrConsentRequired with the missing flags so the API layer can return
// 422 CONSENT_REQUIRED.
func CheckConfirmation(decision ConsentDecision, confirmation ConsentConfirmation) error {
	if decision.Reason == "unchanged" {
		return nil
	}
	missing := []string{}
	for _, flag := range decision.Required {
		if !flagSet(confirmation, flag) {
			missing = append(missing, flag)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return &ErrConsentRequired{
		Code:            "CONSENT_REQUIRED",
		Reason:          decision.Reason,
		MissingFlags:    missing,
		BroadenedFields: append([]string(nil), decision.BroadenedFields...),
	}
}

// ApplyRevision returns a new ConsentRecord with the draft snapshot recorded
// under `current` and a new immutable revision appended to `revisions`. The
// prior revisions are preserved verbatim — append-only.
func ApplyRevision(prior ConsentRecord, draft ConsentNamedValues, reason string, at time.Time) ConsentRecord {
	revision := ConsentRevision{
		At:          at.UTC(),
		NamedValues: cloneConsentValues(draft),
		Reason:      reason,
	}
	revisions := append([]ConsentRevision(nil), prior.Revisions...)
	revisions = append(revisions, revision)
	return ConsentRecord{
		Current:   cloneConsentValues(draft),
		Revisions: revisions,
	}
}

// MarshalJSON wraps the persisted JSONB encoding for storage callers.
func MarshalConsentRecord(rec ConsentRecord) ([]byte, error) {
	return json.Marshal(rec)
}

// UnmarshalConsentRecord parses the persisted JSONB column back into a record,
// tolerating empty snapshots (treated as "no consent yet").
func UnmarshalConsentRecord(payload []byte) (ConsentRecord, error) {
	rec := ConsentRecord{Revisions: []ConsentRevision{}}
	if len(payload) == 0 || string(payload) == "null" {
		return rec, nil
	}
	if err := json.Unmarshal(payload, &rec); err != nil {
		return rec, fmt.Errorf("decode consent record: %w", err)
	}
	if rec.Revisions == nil {
		rec.Revisions = []ConsentRevision{}
	}
	return rec, nil
}

func cloneConsentValues(values ConsentNamedValues) ConsentNamedValues {
	out := ConsentNamedValues{
		DeliveryChannel:  values.DeliveryChannel,
		MaxAlerts:        values.MaxAlerts,
		WindowSeconds:    values.WindowSeconds,
		Precision:        values.Precision,
		SponsoredAllowed: values.SponsoredAllowed,
	}
	if values.Scope != nil {
		scope := make(map[string]any, len(values.Scope))
		for key, value := range values.Scope {
			scope[key] = value
		}
		out.Scope = scope
	}
	if values.Sources != nil {
		sources := append([]string(nil), values.Sources...)
		sort.Strings(sources)
		out.Sources = sources
	}
	if values.HardConstraints != nil {
		constraints := append([]string(nil), values.HardConstraints...)
		sort.Strings(constraints)
		out.HardConstraints = constraints
	}
	return out
}

func isEmptyConsent(values ConsentNamedValues) bool {
	if values.DeliveryChannel != "" || values.MaxAlerts != 0 || values.WindowSeconds != 0 ||
		values.Precision != "" || values.SponsoredAllowed {
		return false
	}
	if len(values.Scope) > 0 || len(values.Sources) > 0 || len(values.HardConstraints) > 0 {
		return false
	}
	return true
}

func broadenedFields(prior, draft ConsentNamedValues) []string {
	out := []string{}
	if !sourcesContained(prior.Sources, draft.Sources) {
		out = append(out, "sources")
	}
	if scopeBroadened(prior.Scope, draft.Scope) {
		out = append(out, "scope")
	}
	if precisionBroadened(prior.Precision, draft.Precision) {
		out = append(out, "precision")
	}
	if rateBroadened(prior.MaxAlerts, prior.WindowSeconds, draft.MaxAlerts, draft.WindowSeconds) {
		out = append(out, "rate_limit")
	}
	if prior.DeliveryChannel != draft.DeliveryChannel && draft.DeliveryChannel != "" {
		out = append(out, "delivery_channel")
	}
	if hardConstraintsRelaxed(prior.HardConstraints, draft.HardConstraints) {
		out = append(out, "hard_constraints")
	}
	if !prior.SponsoredAllowed && draft.SponsoredAllowed {
		out = append(out, "sponsored")
	}
	sort.Strings(out)
	return out
}

func sourcesContained(prior, draft []string) bool {
	priorSet := map[string]struct{}{}
	for _, value := range prior {
		priorSet[strings.ToLower(strings.TrimSpace(value))] = struct{}{}
	}
	for _, value := range draft {
		key := strings.ToLower(strings.TrimSpace(value))
		if key == "" {
			continue
		}
		if _, ok := priorSet[key]; !ok {
			return false
		}
	}
	return true
}

func scopeBroadened(prior, draft map[string]any) bool {
	if len(draft) == 0 {
		return false
	}
	if len(prior) == 0 {
		return true
	}
	for _, key := range []string{"category", "type", "anchor"} {
		if priorValue, hadPrior := prior[key]; hadPrior {
			if draftValue, hadDraft := draft[key]; hadDraft && !valuesEqual(priorValue, draftValue) {
				return true
			}
		}
	}
	if priorRadius, ok := numericValue(prior["radius_meters"]); ok {
		if draftRadius, ok := numericValue(draft["radius_meters"]); ok && draftRadius > priorRadius {
			return true
		}
	}
	if priorLimit, ok := numericValue(prior["price_max"]); ok {
		if draftLimit, ok := numericValue(draft["price_max"]); ok && draftLimit > priorLimit {
			return true
		}
	}
	if priorThreshold, ok := numericValue(prior["price_drop_threshold_pct"]); ok {
		if draftThreshold, ok := numericValue(draft["price_drop_threshold_pct"]); ok && draftThreshold < priorThreshold {
			return true
		}
	}
	if priorKeywords, ok := stringSliceValue(prior["keywords"]); ok {
		draftKeywords, _ := stringSliceValue(draft["keywords"])
		if !sourcesContained(priorKeywords, draftKeywords) {
			return true
		}
	}
	return false
}

func precisionBroadened(prior, draft string) bool {
	if draft == "" || prior == draft {
		return false
	}
	rank := map[string]int{"city": 0, "neighborhood": 1, "exact": 2}
	priorRank, priorOk := rank[prior]
	draftRank, draftOk := rank[draft]
	if !priorOk || !draftOk {
		return false
	}
	return draftRank > priorRank
}

func rateBroadened(priorMax, priorWindow, draftMax, draftWindow int) bool {
	priorRate := rate(priorMax, priorWindow)
	draftRate := rate(draftMax, draftWindow)
	return draftRate > priorRate
}

func rate(maxAlerts, windowSeconds int) float64 {
	if maxAlerts <= 0 || windowSeconds <= 0 {
		return 0
	}
	return float64(maxAlerts) / float64(windowSeconds)
}

func hardConstraintsRelaxed(prior, draft []string) bool {
	priorSet := map[string]struct{}{}
	for _, value := range prior {
		priorSet[strings.ToLower(strings.TrimSpace(value))] = struct{}{}
	}
	draftSet := map[string]struct{}{}
	for _, value := range draft {
		draftSet[strings.ToLower(strings.TrimSpace(value))] = struct{}{}
	}
	for key := range priorSet {
		if _, ok := draftSet[key]; !ok {
			return true
		}
	}
	return false
}

func requiredFlagsForDraft(_ ConsentNamedValues) []string {
	return []string{
		"scope_named",
		"sources_named",
		"rate_limit_named",
		"precision_named",
		"delivery_named",
		"constraints_named",
	}
}

func requiredFlagsForBroadening(fields []string) []string {
	mapping := map[string]string{
		"scope":            "scope_named",
		"sources":          "sources_named",
		"rate_limit":       "rate_limit_named",
		"precision":        "precision_named",
		"delivery_channel": "delivery_named",
		"hard_constraints": "constraints_named",
		"sponsored":        "sponsored_named",
	}
	out := map[string]struct{}{}
	for _, field := range fields {
		if flag, ok := mapping[field]; ok {
			out[flag] = struct{}{}
		}
	}
	flags := make([]string, 0, len(out))
	for flag := range out {
		flags = append(flags, flag)
	}
	sort.Strings(flags)
	return flags
}

func flagSet(confirmation ConsentConfirmation, flag string) bool {
	switch flag {
	case "scope_named":
		return confirmation.ScopeNamed
	case "sources_named":
		return confirmation.SourcesNamed
	case "rate_limit_named":
		return confirmation.RateLimitNamed
	case "precision_named":
		return confirmation.PrecisionNamed
	case "delivery_named":
		return confirmation.DeliveryNamed
	case "constraints_named":
		return confirmation.ConstraintsNamed
	case "sponsored_named":
		return confirmation.SponsoredNamed
	}
	return false
}

func valuesEqual(a, b any) bool {
	return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
}

func numericValue(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case json.Number:
		f, err := typed.Float64()
		if err != nil {
			return 0, false
		}
		return f, true
	}
	return 0, false
}

func stringSliceValue(value any) ([]string, bool) {
	switch typed := value.(type) {
	case []string:
		return typed, true
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out, true
	}
	return nil, false
}
