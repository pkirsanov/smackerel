//go:build integration

// Spec 106 SCOPE-106-03 — XP106-03-I (integration, live stack).
//
// TestRealOwnerOutcomesProjectWithoutFalseEmptyReadyOrSuccess feeds REAL,
// owner-classified typed outcomes from ACTUAL smackerel domain owners through
// the renderer-neutral spec-106 presenters and proves the three scope
// invariants hold against real production determinations — NOT hand-built
// fixtures of the presenter's own seam:
//
//   - readiness owner (internal/recommendation/availability.Determine): the REAL
//     provider-backed readiness determination. An enabled capability with no
//     usable provider (SCN-106-005) resolves to a real CapabilityUnavailable /
//     Ready()==false, and the presenter projects Availability Unavailable while
//     the content axis never fabricates a ready/empty/success state. A real
//     degraded / available / disabled determination projects its exact truthful
//     state.
//   - persistence + post-commit read-back owner
//     (internal/intelligence.DeriveSynthesisHealth): the REAL durable-write
//     health truth. A committed-but-partial output (SCN-106-010) is NEVER
//     complete or announced as success, a commit whose mandatory read-back gate
//     did not verify is NEVER success, and the presenter's IsComplete /
//     AnnouncesSuccess EXACTLY track the owner's Persisted / Healthy truth for
//     every real outcome.
//   - digest read owner (internal/digest.Generator): a REAL live-PostgreSQL
//     round-trip. A real populated row read back through the real reader never
//     projects empty; a real zero-row read projects an honest first-use-empty,
//     never a fabricated ready.
//
// This is a REAL integration test: it imports and exercises the actual owner
// production code with NO mock, NO stub, and NO interception. The readiness and
// synthesis determinations are pure production functions (their own live-DB
// derivations are the owners' deferred slices, not this scope's); the digest
// lane drives the real disposable Postgres brought up by
// `./smackerel.sh test integration` and SKIPs (repo live-lane convention) when
// DATABASE_URL is unset so it is a no-op outside the live integration lane.
//
// Adversarial (non-tautological): the SCN-106-005 case additionally proves a
// registered route CANNOT fabricate availability for the very same real
// zero-provider capability — otherwise the "unavailable" projection could not
// tell a resolved-readiness fact from a structural one.
package integrationexperience

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/db"
	"github.com/smackerel/smackerel/internal/digest"
	"github.com/smackerel/smackerel/internal/experience"
	"github.com/smackerel/smackerel/internal/intelligence"
	"github.com/smackerel/smackerel/internal/recommendation"
	"github.com/smackerel/smackerel/internal/recommendation/availability"
)

func TestRealOwnerOutcomesProjectWithoutFalseEmptyReadyOrSuccess(t *testing.T) {
	sp := experience.ExperienceStatePresenter{}
	mp := experience.MutationFeedbackPresenter{}

	// ── Readiness owner → availability + content axes (SCN-106-004 / 005) ─────
	//
	// availFromSnapshot / readFromSnapshot are pure seam ADAPTERS: they consume
	// the already-resolved availability.AvailabilitySnapshot (the readiness
	// owner's typed outcome) and translate it onto the presenter seam. They
	// re-derive NOTHING — the owner's Determine did all the readiness logic.
	availFromSnapshot := func(s availability.AvailabilitySnapshot) experience.Availability {
		switch s.State {
		case availability.CapabilityAvailable:
			return experience.AvailabilityAvailable
		case availability.CapabilityDegraded:
			return experience.AvailabilityDegraded
		default: // CapabilityDisabled, CapabilityUnavailable
			return experience.AvailabilityUnavailable
		}
	}
	readFromSnapshot := func(s availability.AvailabilitySnapshot) experience.OwnerReadOutcome {
		switch s.State {
		case availability.CapabilityAvailable:
			return experience.OwnerReadOutcome{Kind: experience.ReadPopulated, Owner: "recommendation"}
		case availability.CapabilityDegraded:
			return experience.OwnerReadOutcome{Kind: experience.ReadDegraded, Owner: "recommendation", LimitationCode: string(s.Cause)}
		case availability.CapabilityDisabled:
			return experience.OwnerReadOutcome{Kind: experience.ReadDisabled, Owner: "recommendation"}
		default: // CapabilityUnavailable — enabled but no usable provider.
			if s.Cause == availability.CauseAllProvidersUnavailable {
				return experience.OwnerReadOutcome{Kind: experience.ReadFailed, Owner: "recommendation"}
			}
			// zero_configured / adapter_missing / zero_category: a configuration
			// gap — honest "needs setup", never a fabricated ready/empty.
			return experience.OwnerReadOutcome{Kind: experience.ReadNeedsSetup, Owner: "recommendation"}
		}
	}
	prodProvider := func(id string, health availability.HealthStatus) availability.ProviderState {
		return availability.ProviderState{
			ID: id, DisplayName: id,
			Class:            availability.ProviderClassProduction,
			OperatorSelected: true, Enabled: true, Configured: true, Registered: true,
			Categories: []recommendation.Category{recommendation.CategoryPlace},
			Health:     health,
		}
	}
	determine := func(enabled bool, providers ...availability.ProviderState) availability.AvailabilitySnapshot {
		return availability.Determine(availability.Input{
			Enabled:     enabled,
			Category:    recommendation.CategoryPlace,
			Operation:   availability.OperationRequest,
			Providers:   providers,
			EvaluatedAt: time.Now().UTC(),
			ValidFor:    time.Minute,
		})
	}

	t.Run("readiness_owner_availability_and_content_are_truthful", func(t *testing.T) {
		notReadyOrEmpty := map[experience.ViewState]bool{
			experience.ViewReady: true, experience.ViewFirstUseEmpty: true, experience.ViewFilteredEmpty: true,
		}
		cases := []struct {
			name string
			snap availability.AvailabilitySnapshot
		}{
			{"enabled_zero_providers", determine(true)},
			{"all_providers_unhealthy", determine(true, prodProvider("p1", availability.HealthUnhealthy))},
			{"partial_coverage", determine(true, prodProvider("p1", availability.HealthHealthy), prodProvider("p2", availability.HealthUnhealthy))},
			{"full_coverage", determine(true, prodProvider("p1", availability.HealthHealthy))},
			{"disabled", determine(false)},
		}
		for _, c := range cases {
			gotAvail, err := sp.PresentAvailability(experience.SignalReadinessResolved, availFromSnapshot(c.snap))
			if err != nil {
				t.Fatalf("%s: PresentAvailability(readiness,...) errored: %v", c.name, err)
			}
			gotView, err := sp.PresentRead(readFromSnapshot(c.snap))
			if err != nil {
				t.Fatalf("%s: PresentRead(real owner outcome) errored: %v", c.name, err)
			}
			if c.snap.Ready() {
				// A real ready capability may be available or degraded, but MUST
				// NOT be projected Unavailable (a false-negative outage).
				if gotAvail == experience.AvailabilityUnavailable {
					t.Fatalf("%s: real Ready() owner outcome projected Unavailable (%s/%s)", c.name, c.snap.State, c.snap.Cause)
				}
			} else {
				// A real not-ready capability MUST project Unavailable and MUST
				// NOT fabricate a ready/empty/success content state.
				if gotAvail == experience.AvailabilityAvailable {
					t.Fatalf("%s: real not-ready owner outcome projected Available (%s/%s)", c.name, c.snap.State, c.snap.Cause)
				}
				if notReadyOrEmpty[gotView] {
					t.Fatalf("%s: real not-ready owner outcome fabricated a ready/empty content state %q (%s/%s)", c.name, gotView, c.snap.State, c.snap.Cause)
				}
			}
		}

		// ── SCN-106-005 explicit: enabled + zero providers ───────────────────
		zero := determine(true)
		if zero.State != availability.CapabilityUnavailable || zero.Cause != availability.CauseZeroConfiguredProviders || zero.Ready() {
			t.Fatalf("real readiness owner did not classify enabled+zero-providers as unavailable/not-ready: state=%s cause=%s ready=%v", zero.State, zero.Cause, zero.Ready())
		}
		gotAvail, err := sp.PresentAvailability(experience.SignalReadinessResolved, availFromSnapshot(zero))
		if err != nil || gotAvail != experience.AvailabilityUnavailable {
			t.Fatalf("SCN-106-005: availability = (%q,%v); want (unavailable,nil)", gotAvail, err)
		}
		gotView, _ := sp.PresentRead(readFromSnapshot(zero))
		if notReadyOrEmpty[gotView] {
			t.Fatalf("SCN-106-005: content projected a fabricated ready/empty state %q for a real zero-provider capability", gotView)
		}
		// Adversarial: a registered route CANNOT fabricate availability for the
		// very same real zero-provider capability.
		if _, err := sp.PresentAvailability(experience.SignalRouteRegistered, experience.AvailabilityAvailable); err == nil {
			t.Fatal("SCN-106-005: a registered route fabricated Available for a real zero-provider capability")
		}
	})

	// ── Persistence + read-back owner → mutation axis (SCN-106-010) ───────────
	//
	// mutationFromSynth is a pure seam ADAPTER over the already-derived
	// intelligence.SynthesisHealth verdict. The owner's DeriveSynthesisHealth
	// did all the persistence/read-back logic; the adapter only translates.
	mutationFromSynth := func(h intelligence.SynthesisHealth) experience.OwnerMutationOutcome {
		switch h.State {
		case intelligence.SynthesisReadyCurrent, intelligence.SynthesisReadyQuiet:
			return experience.OwnerMutationOutcome{Kind: experience.MutationOwnerCommittedReadBack, Owner: "synthesis", ReadBackConfirmed: true}
		case intelligence.SynthesisDegradedPartial:
			return experience.OwnerMutationOutcome{Kind: experience.MutationOwnerCorePlusPending, Owner: "synthesis", OutstandingDependencyCode: "partial_output"}
		case intelligence.SynthesisRunning:
			return experience.OwnerMutationOutcome{Kind: experience.MutationOwnerAccepted, Owner: "synthesis"}
		default: // read-degraded (commit without verified read-back), write-failed, etc.
			return experience.OwnerMutationOutcome{Kind: experience.MutationOwnerTransactionFailed, Owner: "synthesis"}
		}
	}

	t.Run("synthesis_owner_partial_never_complete_commit_alone_never_success", func(t *testing.T) {
		cases := []struct {
			name string
			out  intelligence.SynthesisPersistenceOutcome
		}{
			{"committed_full_readback_ok", intelligence.SynthesisPersistenceOutcome{Phase: intelligence.PhaseCommitted, ReadBack: intelligence.ReadBackOK, Output: intelligence.OutputKindFull}},
			{"committed_quiet_readback_ok", intelligence.SynthesisPersistenceOutcome{Phase: intelligence.PhaseCommitted, ReadBack: intelligence.ReadBackOK, Output: intelligence.OutputKindQuiet}},
			{"committed_partial_readback_ok", intelligence.SynthesisPersistenceOutcome{Phase: intelligence.PhaseCommitted, ReadBack: intelligence.ReadBackOK, Output: intelligence.OutputKindPartial}},
			{"committed_readback_mismatch", intelligence.SynthesisPersistenceOutcome{Phase: intelligence.PhaseCommitted, ReadBack: intelligence.ReadBackMismatch}},
			{"write_failed", intelligence.SynthesisPersistenceOutcome{Phase: intelligence.PhaseWriteFailed}},
			{"running", intelligence.SynthesisPersistenceOutcome{Phase: intelligence.PhaseRunning}},
		}
		for _, c := range cases {
			h := intelligence.DeriveSynthesisHealth(c.out) // REAL owner verdict
			state, err := mp.Present(mutationFromSynth(h))
			if err != nil {
				t.Fatalf("%s: Present(real synthesis outcome) errored: %v", c.name, err)
			}
			announces, err := mp.AnnouncesSuccess(mutationFromSynth(h))
			if err != nil {
				t.Fatalf("%s: AnnouncesSuccess errored: %v", c.name, err)
			}
			// The presenter's completeness MUST EXACTLY track the owner's
			// Persisted truth for every real outcome.
			if mp.IsComplete(state) != h.Persisted {
				t.Fatalf("%s: IsComplete(%s)=%v but owner Persisted=%v — completeness must track persistence truth", c.name, state, mp.IsComplete(state), h.Persisted)
			}
			// Success is NEVER announced for an outcome the owner did not persist.
			if !h.Persisted && announces {
				t.Fatalf("%s: announced success for a non-persisted owner outcome (state=%s)", c.name, h.State)
			}
			// A genuinely healthy owner outcome IS announced as success.
			if h.Healthy && !announces {
				t.Fatalf("%s: healthy owner outcome (state=%s) was not announced as success", c.name, h.State)
			}
		}

		// ── SCN-106-010 explicit: a real committed-but-partial output ────────
		partial := intelligence.DeriveSynthesisHealth(intelligence.SynthesisPersistenceOutcome{
			Phase: intelligence.PhaseCommitted, ReadBack: intelligence.ReadBackOK, Output: intelligence.OutputKindPartial,
		})
		if partial.State != intelligence.SynthesisDegradedPartial || partial.Persisted {
			t.Fatalf("real synthesis owner did not classify committed-partial as degraded/not-persisted: state=%s persisted=%v", partial.State, partial.Persisted)
		}
		state, _ := mp.Present(mutationFromSynth(partial))
		if state != experience.MutationPartial {
			t.Fatalf("SCN-106-010: real partial owner outcome mapped to %q; want partial", state)
		}
		if mp.IsComplete(state) {
			t.Fatal("SCN-106-010: a real partial synthesis outcome was reported complete")
		}
		if ok, _ := mp.AnnouncesSuccess(mutationFromSynth(partial)); ok {
			t.Fatal("SCN-106-010: a real partial synthesis outcome was announced as success")
		}
	})

	// ── Live digest read owner → real Postgres round-trip (no false-empty) ────
	t.Run("live_digest_read_never_false_empty", func(t *testing.T) {
		dbURL := os.Getenv("DATABASE_URL")
		if dbURL == "" {
			t.Skip("live digest read requires DATABASE_URL — run via `./smackerel.sh test integration` (disposable stack)")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()

		pool, err := pgxpool.New(ctx, dbURL)
		if err != nil {
			t.Fatalf("connect DATABASE_URL: %v", err)
		}
		defer pool.Close()
		if err := pool.Ping(ctx); err != nil {
			t.Fatalf("ping DATABASE_URL: %v", err)
		}
		if err := db.Migrate(ctx, pool); err != nil {
			t.Fatalf("apply migrations: %v", err)
		}

		// classifyDigestRead is a structural seam ADAPTER over the real reader's
		// (*digest.Digest, error) return. It re-derives no digest business rule;
		// it only distinguishes "a real populated row", "a real no-row read", and
		// "a real read fault".
		classifyDigestRead := func(d *digest.Digest, err error) experience.OwnerReadOutcome {
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return experience.OwnerReadOutcome{Kind: experience.ReadEmptyNoRecord, Owner: "digest"}
				}
				return experience.OwnerReadOutcome{Kind: experience.ReadFailed, Owner: "digest"}
			}
			if d.IsQuiet {
				return experience.OwnerReadOutcome{Kind: experience.ReadDegraded, Owner: "digest", LimitationCode: "quiet_day"}
			}
			return experience.OwnerReadOutcome{Kind: experience.ReadPopulated, Owner: "digest"}
		}

		gen := digest.NewGenerator(pool, nil, nil)
		if _, err := pool.Exec(ctx, "DELETE FROM digests"); err != nil {
			t.Fatalf("clean digests: %v", err)
		}
		t.Cleanup(func() { _, _ = pool.Exec(context.Background(), "DELETE FROM digests") })

		// A real populated write via the REAL production write path, read back
		// through the REAL reader, must project ready — never false-empty.
		const date = "2028-05-20"
		const prose = "XP106-03-I live marker: a real populated digest row read back through the real reader must render ready, never false-empty."
		if err := gen.HandleDigestResult(ctx, date, prose, 42, "test-model"); err != nil {
			t.Fatalf("HandleDigestResult (real write): %v", err)
		}
		d, err := gen.GetLatest(ctx, date) // REAL live read
		if err != nil {
			t.Fatalf("GetLatest (real populated read): %v", err)
		}
		gotView, perr := sp.PresentRead(classifyDigestRead(d, err))
		if perr != nil {
			t.Fatalf("PresentRead(real populated digest) errored: %v", perr)
		}
		if gotView != experience.ViewReady {
			t.Fatalf("a real populated digest row projected %q; want ready (never false-empty)", gotView)
		}

		// A real zero-row read must project an honest first-use-empty — never a
		// fabricated ready.
		if _, err := pool.Exec(ctx, "DELETE FROM digests"); err != nil {
			t.Fatalf("clean digests (empty case): %v", err)
		}
		d2, err2 := gen.GetLatest(ctx, "") // REAL live read, no rows
		emptyView, perr := sp.PresentRead(classifyDigestRead(d2, err2))
		if perr != nil {
			t.Fatalf("PresentRead(real empty digest) errored: %v", perr)
		}
		if emptyView != experience.ViewFirstUseEmpty {
			t.Fatalf("a real zero-row digest read projected %q; want first_use_empty (honest empty, never fabricated ready)", emptyView)
		}
	})
}
