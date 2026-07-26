package experience

import (
	"errors"
	"testing"
)

// TestExperienceStateAvailabilityAndMutationAxesRemainClosedIndependentAndFailClosed
// is the SCOPE-106-03 slice-1 fast unit lane (XP106-03-U). It proves the three
// renderer-neutral presentation axes — content ViewState, capability
// Availability, and command MutationState — plus the presentation-only auth
// adapter, are:
//
//   - CLOSED: every enum rejects an unknown value and every abstract owner
//     outcome maps totally onto its own closed vocabulary;
//   - INDEPENDENT: availability cannot be created from a route/flag/HTTP-200/
//     mounted-handler/empty-array/health signal (SCN-106-005), and a valid auth
//     outcome imposes no content override;
//   - FAIL-CLOSED: an unknown, contradictory, or unsafe owner outcome returns a
//     typed *F106PresentationError, never a downgrade to empty/ready/available/
//     success;
//
// and that failure never becomes empty or success, and a partial mutation is
// never complete or announced as success (SCN-106-010). SCN-106-004 (optional
// capability honest) is covered by the needs_setup/disabled/unavailable cases.
func TestExperienceStateAvailabilityAndMutationAxesRemainClosedIndependentAndFailClosed(t *testing.T) {
	sp := ExperienceStatePresenter{}
	mp := MutationFeedbackPresenter{}
	auth := AuthenticatedRequestAdapter{}

	// validRead returns a structurally-valid outcome for a kind (supplying a
	// limitation only where the kind requires one).
	validRead := func(k OwnerReadKind) OwnerReadOutcome {
		o := OwnerReadOutcome{Kind: k, Owner: "search"}
		if k == ReadDegraded || k == ReadStale {
			o.LimitationCode = "provider_subset"
		}
		return o
	}
	// validMutation returns a structurally-valid outcome for a kind.
	validMutation := func(k OwnerMutationKind) OwnerMutationOutcome {
		o := OwnerMutationOutcome{Kind: k, Owner: "lists"}
		if k == MutationOwnerCommittedReadBack {
			o.ReadBackConfirmed = true
		}
		if k == MutationOwnerCorePlusPending {
			o.OutstandingDependencyCode = "outbox_dispatch"
		}
		return o
	}

	// ── Axis CLOSED + TOTAL: read kinds ──────────────────────────────────────
	t.Run("content_axis_closed_and_total", func(t *testing.T) {
		for k := range readKindToView {
			got, err := sp.PresentRead(validRead(k))
			if err != nil {
				t.Fatalf("PresentRead(%q) unexpected error: %v", k, err)
			}
			if !got.valid() {
				t.Fatalf("PresentRead(%q) produced invalid ViewState %q", k, got)
			}
		}
		if OwnerReadKind("teleport").valid() {
			t.Fatal("OwnerReadKind.valid accepted an unknown kind (axis not closed)")
		}
		if ViewState("teleport").valid() {
			t.Fatal("ViewState.valid accepted an unknown state (axis not closed)")
		}
	})

	// ── SCN-106-004: optional capability is honest ───────────────────────────
	t.Run("scn_106_004_optional_capability_honest", func(t *testing.T) {
		cases := map[OwnerReadKind]ViewState{
			ReadNeedsSetup: ViewNeedsSetup,
			ReadDisabled:   ViewDisabled,
		}
		for k, want := range cases {
			got, err := sp.PresentRead(validRead(k))
			if err != nil || got != want {
				t.Fatalf("PresentRead(%q) = (%q,%v); want (%q,nil) — optional state must be exact, not an outage or ready journey", k, got, err, want)
			}
		}
	})

	// ── SCN-106-005 / independence: only resolved readiness creates availability
	t.Run("scn_106_005_availability_independent_of_structural_facts", func(t *testing.T) {
		if got, err := sp.PresentAvailability(SignalReadinessResolved, AvailabilityAvailable); err != nil || got != AvailabilityAvailable {
			t.Fatalf("PresentAvailability(readiness, available) = (%q,%v); want (available,nil)", got, err)
		}
		// A route/flag/200/handler/empty/health signal must NOT create Available.
		for _, bad := range []AvailabilitySignal{
			SignalRouteRegistered, SignalFlagEnabled, SignalHTTP200,
			SignalHandlerMounted, SignalEmptyArray, SignalHealthOK,
		} {
			got, err := sp.PresentAvailability(bad, AvailabilityAvailable)
			if err == nil {
				t.Fatalf("PresentAvailability(%q, available) returned no error — structural fact fabricated availability", bad)
			}
			if got != "" {
				t.Fatalf("PresentAvailability(%q, available) leaked a value %q on failure", bad, got)
			}
			var fpe *F106PresentationError
			if !errors.As(err, &fpe) {
				t.Fatalf("PresentAvailability(%q,...) error is not *F106PresentationError: %v", bad, err)
			}
		}
	})

	// ── Failure never becomes empty or success ───────────────────────────────
	t.Run("failure_never_empty_or_success", func(t *testing.T) {
		successOrEmpty := map[ViewState]bool{ViewReady: true, ViewFirstUseEmpty: true, ViewFilteredEmpty: true}
		successKinds := map[OwnerReadKind]bool{ReadPopulated: true, ReadEmptyNoRecord: true, ReadEmptyFiltered: true}
		for k, vs := range readKindToView {
			if successOrEmpty[vs] && !successKinds[k] {
				t.Fatalf("non-success kind %q maps to success/empty state %q", k, vs)
			}
		}
		if got, _ := sp.PresentRead(validRead(ReadFailed)); got != ViewError {
			t.Fatalf("PresentRead(failed) = %q; want error (failure must never become empty/success)", got)
		}
		if got, _ := sp.PresentRead(validRead(ReadUnauthorized)); got != ViewUnauthorized {
			t.Fatalf("PresentRead(unauthorized) = %q; want unauthorized", got)
		}
	})

	// ── SCN-106-010: partial is never complete or announced as success ───────
	t.Run("scn_106_010_partial_never_complete_success_needs_readback", func(t *testing.T) {
		partial := validMutation(MutationOwnerCorePlusPending)
		if got, err := mp.Present(partial); err != nil || got != MutationPartial {
			t.Fatalf("Present(core_plus_pending) = (%q,%v); want (partial,nil)", got, err)
		}
		if mp.IsComplete(MutationPartial) {
			t.Fatal("IsComplete(partial) = true; partial is NEVER complete")
		}
		if ok, err := mp.AnnouncesSuccess(partial); err != nil || ok {
			t.Fatalf("AnnouncesSuccess(partial) = (%v,%v); partial must NEVER be announced as success", ok, err)
		}
		// persisted announces success only WITH authoritative read-back.
		if ok, err := mp.AnnouncesSuccess(validMutation(MutationOwnerCommittedReadBack)); err != nil || !ok {
			t.Fatalf("AnnouncesSuccess(persisted+readback) = (%v,%v); want (true,nil)", ok, err)
		}
		// persisted WITHOUT read-back fails closed (never a silent success).
		noReadBack := OwnerMutationOutcome{Kind: MutationOwnerCommittedReadBack, Owner: "lists"}
		if _, err := mp.Present(noReadBack); err == nil {
			t.Fatal("Present(persisted without read-back) returned no error — success without authoritative read-back")
		}
		if OwnerMutationKind("levitate").valid() || MutationState("levitate").valid() {
			t.Fatal("mutation axis accepted an unknown value (axis not closed)")
		}
	})

	// ── Auth axis: 401 clears everything, 403 retains, authorized no override ─
	t.Run("auth_401_clears_403_retains_authorized_no_override", func(t *testing.T) {
		p401, err := auth.Present(AuthSessionEnded)
		if err != nil {
			t.Fatalf("Present(session_ended) error: %v", err)
		}
		if p401.ViewState != ViewUnauthorized || p401.RetainSession || !p401.OffersSafeReauthentication {
			t.Fatalf("401 presentation wrong: %+v", p401)
		}
		if len(p401.ClearedTargets) != len(allProtectedTargets) {
			t.Fatalf("401 cleared %d targets; want all %d protected surfaces", len(p401.ClearedTargets), len(allProtectedTargets))
		}
		p403, err := auth.Present(AuthAccessDenied)
		if err != nil {
			t.Fatalf("Present(access_denied) error: %v", err)
		}
		if p403.ViewState != ViewAccessDenied || !p403.RetainSession || len(p403.ClearedTargets) != 0 || p403.OffersSafeReauthentication {
			t.Fatalf("403 presentation wrong (must retain session, clear nothing, no login loop): %+v", p403)
		}
		pOK, err := auth.Present(AuthAuthorized)
		if err != nil || pOK.ViewState != "" || !pOK.RetainSession {
			t.Fatalf("authorized presentation wrong (auth must not override content axis): %+v err=%v", pOK, err)
		}
	})

	// ── FAIL-CLOSED: unknown, contradictory, and unsafe outcomes ─────────────
	t.Run("fail_closed_unknown_contradictory_and_unsafe", func(t *testing.T) {
		assertStateErr := func(name string, o OwnerReadOutcome) {
			t.Helper()
			got, err := sp.PresentRead(o)
			if err == nil {
				t.Fatalf("%s: PresentRead returned no error (must fail closed)", name)
			}
			if got != "" {
				t.Fatalf("%s: PresentRead leaked ViewState %q on failure", name, got)
			}
			var fpe *F106PresentationError
			if !errors.As(err, &fpe) || fpe.Surface != "state" {
				t.Fatalf("%s: error is not a state *F106PresentationError: %v", name, err)
			}
		}
		assertStateErr("unknown-read-kind", OwnerReadOutcome{Kind: "invented", Owner: "search"})
		assertStateErr("degraded-without-limitation", OwnerReadOutcome{Kind: ReadDegraded, Owner: "search"})
		assertStateErr("stale-without-limitation", OwnerReadOutcome{Kind: ReadStale, Owner: "search"})
		assertStateErr("limitation-on-success", OwnerReadOutcome{Kind: ReadPopulated, Owner: "search", LimitationCode: "x"})
		// Redaction: a raw error string in a code field must be rejected.
		assertStateErr("unsafe-limitation-code", OwnerReadOutcome{Kind: ReadDegraded, Owner: "search", LimitationCode: "panic: runtime error: index out of range /app/x.go:42"})
		assertStateErr("unsafe-action-code", OwnerReadOutcome{Kind: ReadFailed, Owner: "search", ActionCode: "https://host/reset?token=abc"})

		if _, err := mp.Present(OwnerMutationOutcome{Kind: "invented"}); err == nil {
			t.Fatal("Present(unknown mutation kind) returned no error")
		}
		if _, err := mp.Present(OwnerMutationOutcome{Kind: MutationOwnerCorePlusPending, Owner: "lists"}); err == nil {
			t.Fatal("Present(partial without outstanding dependency) returned no error")
		}
		if _, err := mp.Present(OwnerMutationOutcome{Kind: MutationOwnerAccepted, Owner: "lists", OutstandingDependencyCode: "x"}); err == nil {
			t.Fatal("Present(non-partial with outstanding dependency) returned no error")
		}
		if _, err := mp.Present(OwnerMutationOutcome{Kind: MutationOwnerCorePlusPending, Owner: "lists", OutstandingDependencyCode: "raw error: boom"}); err == nil {
			t.Fatal("Present(partial with unsafe dependency code) returned no error")
		}
		if _, err := sp.PresentAvailability(SignalReadinessResolved, Availability("invented")); err == nil {
			t.Fatal("PresentAvailability(readiness, unknown) returned no error")
		}
		if _, err := auth.Present(AuthOutcome("invented")); err == nil {
			t.Fatal("Present(unknown auth outcome) returned no error")
		}
	})
}
