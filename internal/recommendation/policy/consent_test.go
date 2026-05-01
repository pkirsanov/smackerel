package policy

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestEvaluateConsent_FirstTimeRequiresFullConfirmation(t *testing.T) {
	prior := ConsentRecord{}
	draft := ConsentNamedValues{
		Scope:           map[string]any{"category": "place", "radius_meters": 1000.0},
		Sources:         []string{"google_places"},
		DeliveryChannel: "telegram",
		MaxAlerts:       1,
		WindowSeconds:   86400,
		Precision:       "neighborhood",
		HardConstraints: []string{"quiet"},
	}
	decision := EvaluateConsent(prior, draft, false, true)
	if decision.Reason != ConsentReasonCreate {
		t.Fatalf("reason = %q, want %q", decision.Reason, ConsentReasonCreate)
	}
	for _, want := range []string{"scope_named", "sources_named", "rate_limit_named", "precision_named", "delivery_named", "constraints_named"} {
		if !contains(decision.Required, want) {
			t.Fatalf("required flags missing %q: %+v", want, decision.Required)
		}
	}
}

func TestCheckConfirmation_RejectsBroadeningWithoutFlags(t *testing.T) {
	prior := ConsentRecord{
		Current: ConsentNamedValues{
			Scope:           map[string]any{"category": "product", "price_max": 800.0, "keywords": []string{"espresso"}},
			Sources:         []string{"store_a"},
			DeliveryChannel: "telegram",
			MaxAlerts:       1,
			WindowSeconds:   86400,
			Precision:       "neighborhood",
			HardConstraints: []string{"under_800"},
		},
		Revisions: []ConsentRevision{{At: time.Now().UTC(), Reason: ConsentReasonCreate}},
	}
	draft := prior.Current
	draft.Sources = []string{"store_a", "store_b"} // broadens sources
	decision := EvaluateConsent(prior, draft, true, true)
	if decision.Reason != ConsentReasonBroaden {
		t.Fatalf("reason = %q, want %q", decision.Reason, ConsentReasonBroaden)
	}
	if !contains(decision.BroadenedFields, "sources") {
		t.Fatalf("broadened fields missing sources: %+v", decision.BroadenedFields)
	}
	err := CheckConfirmation(decision, ConsentConfirmation{ScopeNamed: true, RateLimitNamed: true})
	if err == nil {
		t.Fatal("expected ErrConsentRequired, got nil")
	}
	var typed *ErrConsentRequired
	if !errors.As(err, &typed) {
		t.Fatalf("error type = %T, want *ErrConsentRequired", err)
	}
	if typed.Code != "CONSENT_REQUIRED" {
		t.Fatalf("error code = %q, want CONSENT_REQUIRED", typed.Code)
	}
	if !contains(typed.MissingFlags, "sources_named") {
		t.Fatalf("missing flags should include sources_named: %+v", typed.MissingFlags)
	}
}

func TestCheckConfirmation_AcceptsBroadeningWithMatchingFlags(t *testing.T) {
	prior := ConsentRecord{
		Current: ConsentNamedValues{
			Scope:           map[string]any{"category": "product"},
			Sources:         []string{"store_a"},
			DeliveryChannel: "telegram",
			MaxAlerts:       1,
			WindowSeconds:   86400,
			Precision:       "neighborhood",
			HardConstraints: []string{},
		},
	}
	draft := prior.Current
	draft.Sources = []string{"store_a", "store_b"}
	decision := EvaluateConsent(prior, draft, true, true)
	confirmation := ConsentConfirmation{SourcesNamed: true}
	if err := CheckConfirmation(decision, confirmation); err != nil {
		t.Fatalf("CheckConfirmation returned err: %v", err)
	}
}

func TestEvaluateConsent_PrecisionWideningTriggersBroaden(t *testing.T) {
	prior := ConsentRecord{
		Current: ConsentNamedValues{
			Scope:           map[string]any{"category": "place"},
			Sources:         []string{"google_places"},
			DeliveryChannel: "telegram",
			MaxAlerts:       1,
			WindowSeconds:   86400,
			Precision:       "city",
			HardConstraints: []string{},
		},
	}
	draft := prior.Current
	draft.Precision = "exact" // exact > city = broadens precision
	decision := EvaluateConsent(prior, draft, true, true)
	if decision.Reason != ConsentReasonBroaden {
		t.Fatalf("reason = %q, want broaden", decision.Reason)
	}
	if !contains(decision.BroadenedFields, "precision") {
		t.Fatalf("broadened fields missing precision: %+v", decision.BroadenedFields)
	}
	if !contains(decision.Required, "precision_named") {
		t.Fatalf("required flags missing precision_named: %+v", decision.Required)
	}
}

func TestEvaluateConsent_RateRelaxationTriggersBroaden(t *testing.T) {
	prior := ConsentRecord{
		Current: ConsentNamedValues{
			Scope:           map[string]any{"category": "place"},
			Sources:         []string{"google_places"},
			DeliveryChannel: "telegram",
			MaxAlerts:       1,
			WindowSeconds:   86400,
			Precision:       "neighborhood",
			HardConstraints: []string{},
		},
	}
	draft := prior.Current
	draft.MaxAlerts = 5 // 5 per day = much higher than 1 per day
	decision := EvaluateConsent(prior, draft, true, true)
	if decision.Reason != ConsentReasonBroaden {
		t.Fatalf("reason = %q, want broaden", decision.Reason)
	}
	if !contains(decision.BroadenedFields, "rate_limit") {
		t.Fatalf("broadened fields missing rate_limit: %+v", decision.BroadenedFields)
	}
}

func TestEvaluateConsent_HardConstraintRemovalIsBroadening(t *testing.T) {
	prior := ConsentRecord{
		Current: ConsentNamedValues{
			Scope:           map[string]any{"category": "place"},
			Sources:         []string{"google_places"},
			DeliveryChannel: "telegram",
			MaxAlerts:       1,
			WindowSeconds:   86400,
			Precision:       "neighborhood",
			HardConstraints: []string{"vegetarian", "quiet"},
		},
	}
	draft := prior.Current
	draft.HardConstraints = []string{"vegetarian"} // removed quiet → relaxed
	decision := EvaluateConsent(prior, draft, true, true)
	if decision.Reason != ConsentReasonBroaden {
		t.Fatalf("reason = %q, want broaden", decision.Reason)
	}
	if !contains(decision.BroadenedFields, "hard_constraints") {
		t.Fatalf("broadened fields missing hard_constraints: %+v", decision.BroadenedFields)
	}
}

func TestApplyRevision_AppendOnlyAndPreservesPriorRevisions(t *testing.T) {
	at1 := time.Date(2026, 4, 30, 10, 0, 0, 0, time.UTC)
	at2 := time.Date(2026, 4, 30, 11, 0, 0, 0, time.UTC)
	prior := ConsentRecord{
		Current:   ConsentNamedValues{Sources: []string{"store_a"}, Precision: "city", DeliveryChannel: "telegram", MaxAlerts: 1, WindowSeconds: 86400},
		Revisions: []ConsentRevision{{At: at1, Reason: ConsentReasonCreate, NamedValues: ConsentNamedValues{Sources: []string{"store_a"}, Precision: "city"}}},
	}
	draft := ConsentNamedValues{Sources: []string{"store_a", "store_b"}, Precision: "city", DeliveryChannel: "telegram", MaxAlerts: 1, WindowSeconds: 86400}
	updated := ApplyRevision(prior, draft, ConsentReasonBroaden, at2)
	if len(updated.Revisions) != 2 {
		t.Fatalf("revisions = %d, want 2 (append-only)", len(updated.Revisions))
	}
	if updated.Revisions[0].Reason != ConsentReasonCreate {
		t.Fatalf("prior revision reason = %q, want create (must be preserved)", updated.Revisions[0].Reason)
	}
	if updated.Revisions[1].Reason != ConsentReasonBroaden {
		t.Fatalf("appended revision reason = %q, want broaden", updated.Revisions[1].Reason)
	}
	if len(updated.Current.Sources) != 2 {
		t.Fatalf("current sources = %v, want 2 entries", updated.Current.Sources)
	}
	if !updated.Revisions[1].At.Equal(at2.UTC()) {
		t.Fatalf("revision timestamp = %v, want %v", updated.Revisions[1].At, at2)
	}
	// Adversarial: prior revision must not be mutated by later edits to draft.
	draft.Sources = []string{"store_x"}
	if updated.Revisions[1].NamedValues.Sources[0] == "store_x" || updated.Revisions[1].NamedValues.Sources[1] == "store_x" {
		t.Fatalf("revision named values aliased the caller draft slice: %+v", updated.Revisions[1].NamedValues.Sources)
	}
}

func TestEvaluateConsent_UnchangedNeedsNoConfirmation(t *testing.T) {
	prior := ConsentRecord{
		Current: ConsentNamedValues{
			Scope:           map[string]any{"category": "place"},
			Sources:         []string{"google_places"},
			DeliveryChannel: "telegram",
			MaxAlerts:       1,
			WindowSeconds:   86400,
			Precision:       "neighborhood",
			HardConstraints: []string{"quiet"},
		},
	}
	decision := EvaluateConsent(prior, prior.Current, true, true)
	if decision.Reason != "unchanged" {
		t.Fatalf("reason = %q, want unchanged", decision.Reason)
	}
	if err := CheckConfirmation(decision, ConsentConfirmation{}); err != nil {
		t.Fatalf("CheckConfirmation should pass with no confirmation when unchanged: %v", err)
	}
}

func TestEvaluateConsent_EnableTransitionRequiresAllFlags(t *testing.T) {
	prior := ConsentRecord{
		Current: ConsentNamedValues{
			Scope:           map[string]any{"category": "place"},
			Sources:         []string{"google_places"},
			DeliveryChannel: "telegram",
			MaxAlerts:       1,
			WindowSeconds:   86400,
			Precision:       "neighborhood",
		},
	}
	decision := EvaluateConsent(prior, prior.Current, false, true)
	if decision.Reason != ConsentReasonEnable {
		t.Fatalf("reason = %q, want enable", decision.Reason)
	}
	err := CheckConfirmation(decision, ConsentConfirmation{ScopeNamed: true, SourcesNamed: true, RateLimitNamed: true})
	if err == nil {
		t.Fatal("expected CONSENT_REQUIRED when enabling without precision_named")
	}
	var typed *ErrConsentRequired
	if !errors.As(err, &typed) || !contains(typed.MissingFlags, "precision_named") {
		t.Fatalf("missing flags should include precision_named: %v", err)
	}
}

func TestEvaluateConsent_ScopeKeywordWideningIsBroadening(t *testing.T) {
	prior := ConsentRecord{
		Current: ConsentNamedValues{
			Scope:           map[string]any{"category": "product", "keywords": []string{"espresso"}},
			Sources:         []string{"store_a"},
			DeliveryChannel: "telegram",
			MaxAlerts:       1,
			WindowSeconds:   86400,
			Precision:       "neighborhood",
		},
	}
	draft := prior.Current
	draft.Scope = map[string]any{"category": "product", "keywords": []string{"espresso", "appliance"}}
	decision := EvaluateConsent(prior, draft, true, true)
	if decision.Reason != ConsentReasonBroaden {
		t.Fatalf("reason = %q, want broaden", decision.Reason)
	}
	if !contains(decision.BroadenedFields, "scope") {
		t.Fatalf("broadened fields missing scope: %+v", decision.BroadenedFields)
	}
}

func TestUnmarshalConsentRecord_TolerantOfEmptyAndNullPayloads(t *testing.T) {
	for _, payload := range [][]byte{nil, []byte(""), []byte("null"), []byte(`{}`)} {
		rec, err := UnmarshalConsentRecord(payload)
		if err != nil {
			t.Fatalf("payload=%q err=%v", string(payload), err)
		}
		if rec.Revisions == nil {
			t.Fatalf("revisions should be initialized to non-nil for payload=%q", string(payload))
		}
	}
}

func contains(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}

// Adversarial guard: error message must include a 'consent required' phrase that
// callers can rely on for log/test assertions.
func TestErrConsentRequired_MessageContainsCanonicalPrefix(t *testing.T) {
	err := &ErrConsentRequired{Code: "CONSENT_REQUIRED", Reason: "broaden", MissingFlags: []string{"sources_named"}}
	if !strings.HasPrefix(err.Error(), "consent required:") {
		t.Fatalf("error message = %q, want prefix 'consent required:'", err.Error())
	}
	if !IsConsentRequired(err) {
		t.Fatal("IsConsentRequired should detect *ErrConsentRequired")
	}
}
