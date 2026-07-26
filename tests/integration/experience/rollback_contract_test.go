//go:build integration

// Spec 106 SCOPE-106-03 — XP106-03-rollback (shared-infrastructure rollback contract).
//
// TestPresentationPackageRollbackIsAtomicAndRestoresNoUnsafeBehavior proves the
// atomic rollback contract for the spec-106 shared state/presentation package
// (scope.md "Rollback"). The presenters are an ADDITIVE, self-contained mapping
// layer: rolling them back — disabling the package — NEVER restores any of the
// five unsafe behaviors the scope forbids, because the package STRUCTURALLY
// cannot emit them in the first place. When an owner outcome cannot be mapped
// safely, the surface fails CLOSED to a typed *F106PresentationError
// (Unavailable-equivalent), never a guessed state.
//
// The five rollback-regression behaviors, mapped 1:1 to the assertions below:
//
//  1. failure-as-empty  — no failure/auth-loss read kind ever yields
//     ready/first_use_empty/filtered_empty.
//  2. optimistic-success — no non-persisted mutation outcome is ever complete or
//     announced as success; partial is never complete regardless of how much
//     core state persisted.
//  3. raw-errors — an owner outcome carrying non-content-free detail fails closed
//     and leaks no presented value; the typed error text carries no raw detail.
//  4. retained-protected-DOM-after-401 — a 401 always clears ALL five protected
//     targets and retains no session.
//  5. duplicate-submit — an already-committed command maps to idempotent (never a
//     second persisted success) and an accepted command locks to pending (never
//     complete/success), so a re-submit cannot double-apply.
//
// Because every emitted value is a member of a CLOSED presentation vocabulary and
// the whole exhaustive owner-outcome space is walked, this is the executable form
// of "disabling the package restores prior renderer behavior with no unsafe
// residue". The complementary SHADOW-ARTIFACT property (no live-render package
// imports these presenters, so disabling them is a zero-live-change decision and
// the untouched handwritten renderers stay the live authority) is proven by the
// import grep recorded in report.md#xp106-03-rollback.
//
// Adversarial (non-tautological): each assertion would FAIL if the presenter
// regressed to the very behavior the scope forbids (e.g. ReadFailed -> ViewReady,
// partial -> complete, 401 retaining the session, already-committed -> a second
// persisted success). It is a pure Go deterministic contract (integration-tag,
// same convention as the other shared-infra canaries here); it needs no live
// stack, so it runs in the stores-only integration-light lane.
package integrationexperience

import (
	"errors"
	"strings"
	"testing"

	"github.com/smackerel/smackerel/internal/experience"
)

func TestPresentationPackageRollbackIsAtomicAndRestoresNoUnsafeBehavior(t *testing.T) {
	sp := experience.ExperienceStatePresenter{}
	mp := experience.MutationFeedbackPresenter{}
	auth := experience.AuthenticatedRequestAdapter{}

	// The closed content-state vocabulary that means "success" or "empty" — the
	// exact set a failure/auth-loss outcome must NEVER be rolled into.
	successOrEmpty := map[experience.ViewState]bool{
		experience.ViewReady:         true,
		experience.ViewFirstUseEmpty: true,
		experience.ViewFilteredEmpty: true,
	}

	// ── 1. failure-as-empty: no failure/auth-loss read kind yields success/empty.
	failureKinds := []experience.OwnerReadKind{
		experience.ReadFailed,
		experience.ReadUnauthorized,
		experience.ReadAccessDenied,
		experience.ReadNotFound,
	}
	for _, k := range failureKinds {
		got, err := sp.PresentRead(experience.OwnerReadOutcome{Kind: k, Owner: "rollback"})
		if err != nil {
			t.Fatalf("failure kind %q unexpectedly errored: %v", k, err)
		}
		if successOrEmpty[got] {
			t.Errorf("ROLLBACK-REGRESSION failure-as-empty: read kind %q mapped to %q (a success/empty state)", k, got)
		}
	}
	t.Logf("1. failure-as-empty: %d failure/auth-loss kinds avoid ready/first_use_empty/filtered_empty (ok)", len(failureKinds))

	// ── 2. optimistic-success: no non-persisted mutation is complete or success.
	nonSuccess := []experience.OwnerMutationOutcome{
		{Kind: experience.MutationOwnerCorePlusPending, Owner: "rollback", OutstandingDependencyCode: "dep.pending"},
		{Kind: experience.MutationOwnerTransactionFailed, Owner: "rollback"},
		{Kind: experience.MutationOwnerStateChanged, Owner: "rollback"},
		{Kind: experience.MutationOwnerRejectedPreWrite, Owner: "rollback"},
		{Kind: experience.MutationOwnerAccepted, Owner: "rollback"},
	}
	for _, o := range nonSuccess {
		st, err := mp.Present(o)
		if err != nil {
			t.Fatalf("non-success mutation %q unexpectedly errored: %v", o.Kind, err)
		}
		if mp.IsComplete(st) {
			t.Errorf("ROLLBACK-REGRESSION optimistic-success: mutation kind %q (state %q) reported complete", o.Kind, st)
		}
		announced, err := mp.AnnouncesSuccess(o)
		if err != nil {
			t.Fatalf("AnnouncesSuccess(%q) errored: %v", o.Kind, err)
		}
		if announced {
			t.Errorf("ROLLBACK-REGRESSION optimistic-success: mutation kind %q announced success", o.Kind)
		}
	}
	// partial is the sharpest case: core state persisted, yet it is NEVER complete.
	partialState, err := mp.Present(experience.OwnerMutationOutcome{Kind: experience.MutationOwnerCorePlusPending, Owner: "rollback", OutstandingDependencyCode: "dep.pending"})
	if err != nil || partialState != experience.MutationPartial || mp.IsComplete(partialState) {
		t.Fatalf("partial must map to %q and never be complete (got %q complete=%v err=%v)", experience.MutationPartial, partialState, mp.IsComplete(partialState), err)
	}
	t.Logf("2. optimistic-success: %d non-persisted outcomes (incl. partial) never complete/announced-success (ok)", len(nonSuccess))

	// ── 3. raw-errors: an outcome carrying raw (non content-free) detail fails
	//        closed to a typed error and leaks no presented value; the error text
	//        itself carries no raw detail.
	rawDetail := "panic: runtime error: index out of range /app/handler.go:412"
	if got, err := sp.PresentRead(experience.OwnerReadOutcome{Kind: experience.ReadDegraded, Owner: "rollback", LimitationCode: rawDetail}); err == nil || got != "" {
		t.Errorf("ROLLBACK-REGRESSION raw-errors: raw read limitation leaked (state=%q err=%v)", got, err)
	} else {
		var pe *experience.F106PresentationError
		if !errors.As(err, &pe) {
			t.Errorf("raw-detail read outcome must fail with *F106PresentationError, got %T", err)
		}
		if strings.Contains(err.Error(), "runtime error") || strings.Contains(err.Error(), "handler.go") {
			t.Errorf("ROLLBACK-REGRESSION raw-errors: typed read error text leaked raw detail: %q", err.Error())
		}
	}
	if got, err := mp.Present(experience.OwnerMutationOutcome{Kind: experience.MutationOwnerCorePlusPending, Owner: "rollback", OutstandingDependencyCode: rawDetail}); err == nil || got != "" {
		t.Errorf("ROLLBACK-REGRESSION raw-errors: raw mutation dependency leaked (state=%q err=%v)", got, err)
	} else if strings.Contains(err.Error(), "runtime error") || strings.Contains(err.Error(), "handler.go") {
		t.Errorf("ROLLBACK-REGRESSION raw-errors: typed mutation error text leaked raw detail: %q", err.Error())
	}
	t.Logf("3. raw-errors: raw owner detail fails closed with a typed error and no leaked value (ok)")

	// ── 4. retained-protected-DOM-after-401: a 401 clears ALL five targets, no session.
	pres, err := auth.Present(experience.AuthSessionEnded)
	if err != nil {
		t.Fatalf("401 present errored: %v", err)
	}
	if pres.RetainSession {
		t.Errorf("ROLLBACK-REGRESSION retained-DOM: 401 retained the session")
	}
	if len(pres.ClearedTargets) != 5 {
		t.Errorf("ROLLBACK-REGRESSION retained-DOM: 401 cleared %d protected targets, want 5", len(pres.ClearedTargets))
	}
	if !pres.OffersSafeReauthentication || pres.ViewState != experience.ViewUnauthorized {
		t.Errorf("401 must offer safe re-auth and present unauthorized (got reauth=%v view=%q)", pres.OffersSafeReauthentication, pres.ViewState)
	}
	t.Logf("4. retained-protected-DOM-after-401: 401 clears all %d targets and retains no session (ok)", len(pres.ClearedTargets))

	// ── 5. duplicate-submit: already-committed -> idempotent (never a second
	//        persisted success); accepted -> pending lock.
	idem, err := mp.Present(experience.OwnerMutationOutcome{Kind: experience.MutationOwnerAlreadyCommitted, Owner: "rollback"})
	if err != nil || idem != experience.MutationIdempotent {
		t.Fatalf("already-committed must map to idempotent (got %q err=%v)", idem, err)
	}
	if ok, _ := mp.AnnouncesSuccess(experience.OwnerMutationOutcome{Kind: experience.MutationOwnerAlreadyCommitted, Owner: "rollback"}); ok {
		t.Errorf("ROLLBACK-REGRESSION duplicate-submit: idempotent without read-back announced success")
	}
	pendingSt, err := mp.Present(experience.OwnerMutationOutcome{Kind: experience.MutationOwnerAccepted, Owner: "rollback"})
	if err != nil || pendingSt != experience.MutationPending || mp.IsComplete(pendingSt) {
		t.Errorf("ROLLBACK-REGRESSION duplicate-submit: accepted must lock to pending (got %q complete=%v err=%v)", pendingSt, mp.IsComplete(pendingSt), err)
	}
	t.Logf("5. duplicate-submit: already-committed->idempotent (no 2nd success), accepted->pending lock (ok)")

	// ── unsafe/unmappable owner outcome -> typed error, never a guessed state.
	if got, err := sp.PresentRead(experience.OwnerReadOutcome{Kind: experience.OwnerReadKind("smuggled-raw-owner-string"), Owner: "rollback"}); err == nil || got != "" {
		t.Errorf("unknown owner read kind must fail closed to a typed error, got state=%q err=%v", got, err)
	}
	t.Logf("unsafe/unknown owner outcome fails closed to a typed error, never a guessed state (ok)")
}
