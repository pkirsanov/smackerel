package microtools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/auth"
)

// entityResolveCtx carries the authenticated caller the way production does
// (BUG-061-012): on the context, put there by the surface. These tests used to
// pass "user_id" as a tool argument, which is exactly the defect — an argument
// the model writes cannot be identity.
func entityResolveCtx(userID string) context.Context {
	return auth.WithSession(context.Background(), auth.Session{
		UserID: userID,
		Source: auth.SessionSourcePerUserToken,
		Scopes: []string{},
	})
}

type fakeEntityResolver struct {
	candidates []EntityCandidate
	err        error
	lastUser   string
	lastInput  string
	lastScope  string
	lastLimit  int
}

func (f *fakeEntityResolver) Resolve(_ context.Context, userID, input, scope string, maxCandidates int) ([]EntityCandidate, error) {
	f.lastUser = userID
	f.lastInput = input
	f.lastScope = scope
	f.lastLimit = maxCandidates
	if f.err != nil {
		return nil, f.err
	}
	return f.candidates, nil
}

func withEntityResolveServices(t *testing.T, svc *EntityResolveServices) {
	t.Helper()
	SetEntityResolveServices(svc)
	t.Cleanup(ResetEntityResolveServicesForTest)
}

func mustResolveEnvelopeRE(t *testing.T, raw json.RawMessage, err error) Envelope {
	t.Helper()
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	var env Envelope
	if uerr := json.Unmarshal(raw, &env); uerr != nil {
		t.Fatalf("unmarshal envelope: %v\nraw=%s", uerr, string(raw))
	}
	if verr := ValidateEnvelope(env); verr != nil {
		t.Fatalf("envelope failed validation: %v\nraw=%s", verr, string(raw))
	}
	return env
}

func TestEntityResolveRanksExactRecentRelationThenVectorCandidates(t *testing.T) {
	// SCN-065-A06 — resolved branch: top score clears the floor.
	resolver := &fakeEntityResolver{
		candidates: []EntityCandidate{
			{ArtifactID: "art-1", Label: "Apartment lease 2024", Score: 0.92, ArtifactType: "document", Snippet: "signed 2024-06"},
			{ArtifactID: "art-2", Label: "Car lease 2023", Score: 0.55, ArtifactType: "document"},
		},
	}
	withEntityResolveServices(t, &EntityResolveServices{
		Resolver:        resolver,
		ConfidenceFloor: 0.7,
		MaxCandidates:   5,
		Timeout:         500 * time.Millisecond,
	})

	args := json.RawMessage(`{"input":"the lease","scope":"documents","top_k":5}`)
	raw, err := handleEntityResolve(entityResolveCtx("u1"), args)
	env := mustResolveEnvelopeRE(t, raw, err)

	if env.Status != StatusResolved {
		t.Fatalf("status = %q, want resolved", env.Status)
	}
	if got := env.Value["artifact_id"]; got != "art-1" {
		t.Fatalf("artifact_id = %v, want art-1", got)
	}
	if env.Confidence != 0.92 {
		t.Fatalf("confidence = %v, want 0.92", env.Confidence)
	}
	// lastUser proves the resolver was scoped to the SESSION user. Nothing in
	// args could have set it.
	if resolver.lastUser != "u1" || resolver.lastInput != "the lease" || resolver.lastScope != "documents" || resolver.lastLimit != 5 {
		t.Fatalf("resolver call args wrong: user=%q input=%q scope=%q limit=%d",
			resolver.lastUser, resolver.lastInput, resolver.lastScope, resolver.lastLimit)
	}
}

func TestEntityResolveIgnoresModelSuppliedUserID(t *testing.T) {
	// BUG-061-012 SCN-04 adversarial: a model that still writes "user_id" must
	// not be able to steer the scope. The session user wins, and the smuggled
	// value reaches nothing.
	resolver := &fakeEntityResolver{
		candidates: []EntityCandidate{{ArtifactID: "art-1", Label: "Lease", Score: 0.95}},
	}
	withEntityResolveServices(t, &EntityResolveServices{
		Resolver:        resolver,
		ConfidenceFloor: 0.7,
		MaxCandidates:   5,
		Timeout:         500 * time.Millisecond,
	})

	args := json.RawMessage(`{"input":"the lease","user_id":"victim"}`)
	if _, err := handleEntityResolve(entityResolveCtx("caller"), args); err != nil {
		t.Fatalf("handler err: %v", err)
	}
	if resolver.lastUser != "caller" {
		t.Fatalf("resolver scoped to %q, want the session user \"caller\" — a model argument steered identity", resolver.lastUser)
	}
}

func TestEntityResolveLowConfidenceReturnsAmbiguous(t *testing.T) {
	// SCN-065-A06 — ambiguous branch: top score below floor.
	resolver := &fakeEntityResolver{
		candidates: []EntityCandidate{
			{ArtifactID: "art-a", Label: "Lease (apartment)", Score: 0.55, Snippet: "Apt 4B"},
			{ArtifactID: "art-b", Label: "Lease (storage unit)", Score: 0.50, Snippet: "Unit 22"},
			{ArtifactID: "art-c", Label: "Lease (car)", Score: 0.45},
		},
	}
	withEntityResolveServices(t, &EntityResolveServices{
		Resolver:        resolver,
		ConfidenceFloor: 0.7,
		MaxCandidates:   5,
		Timeout:         500 * time.Millisecond,
	})

	args := json.RawMessage(`{"input":"the lease"}`)
	raw, err := handleEntityResolve(entityResolveCtx("u1"), args)
	env := mustResolveEnvelopeRE(t, raw, err)

	if env.Status != StatusAmbiguous {
		t.Fatalf("status = %q, want ambiguous", env.Status)
	}
	if len(env.Candidates) != 3 {
		t.Fatalf("candidates len = %d, want 3", len(env.Candidates))
	}
	for i, c := range env.Candidates {
		if c.Rank != i+1 {
			t.Fatalf("candidate[%d].rank = %d, want %d", i, c.Rank, i+1)
		}
	}
}

func TestEntityResolveZeroCandidatesFailsLoud(t *testing.T) {
	resolver := &fakeEntityResolver{candidates: nil}
	withEntityResolveServices(t, &EntityResolveServices{
		Resolver:        resolver,
		ConfidenceFloor: 0.5,
		MaxCandidates:   5,
		Timeout:         500 * time.Millisecond,
	})

	args := json.RawMessage(`{"input":"nothing matches"}`)
	raw, err := handleEntityResolve(entityResolveCtx("u1"), args)
	env := mustResolveEnvelopeRE(t, raw, err)

	if env.Status != StatusFailed {
		t.Fatalf("status = %q, want failed", env.Status)
	}
	if env.Error == nil || env.Error.Code != "no_candidates" {
		t.Fatalf("error = %+v, want code=no_candidates", env.Error)
	}
}

func TestEntityResolveResolverErrorBecomesFailedEnvelope(t *testing.T) {
	resolver := &fakeEntityResolver{err: errors.New("graph timeout")}
	withEntityResolveServices(t, &EntityResolveServices{
		Resolver:        resolver,
		ConfidenceFloor: 0.5,
		MaxCandidates:   3,
		Timeout:         200 * time.Millisecond,
	})

	args := json.RawMessage(`{"input":"x"}`)
	raw, err := handleEntityResolve(entityResolveCtx("u1"), args)
	env := mustResolveEnvelopeRE(t, raw, err)

	if env.Status != StatusFailed {
		t.Fatalf("status = %q, want failed", env.Status)
	}
	if env.Error == nil || env.Error.Code != "resolver_error" || !strings.Contains(env.Error.Message, "graph timeout") {
		t.Fatalf("error = %+v, want resolver_error containing 'graph timeout'", env.Error)
	}
}

func TestEntityResolveRejectsUnidentifiedCaller(t *testing.T) {
	// BUG-061-012 SCN-02. This replaces the old "missing user_id argument"
	// case: identity is no longer an argument, so the equivalent refusal is
	// "no principal on the context". Keeping the assertion (rather than
	// deleting the test with its argument) is what preserves the only proof
	// that the tool refuses an unnamed caller instead of resolving globally.
	//
	// The two identity refusals stay distinct: a surface that injected nothing
	// and a surface that injected a principal identifying nobody are different
	// wiring bugs with different fixes.
	withEntityResolveServices(t, &EntityResolveServices{
		Resolver:        &fakeEntityResolver{},
		ConfidenceFloor: 0.5,
		MaxCandidates:   3,
		Timeout:         100 * time.Millisecond,
	})

	cases := []struct {
		name string
		ctx  context.Context
		args string
		want string
	}{
		{"no principal", context.Background(), `{"input":"x"}`, "entity_resolve_no_principal"},
		{"principal identifies nobody", entityResolveCtx(""), `{"input":"x"}`, "entity_resolve_principal_without_user"},
		{"empty input", entityResolveCtx("u"), `{"input":""}`, "entity_resolve_empty_input"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := handleEntityResolve(tc.ctx, json.RawMessage(tc.args))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestEntityResolveNotConfiguredFailsLoud(t *testing.T) {
	ResetEntityResolveServicesForTest()
	_, err := handleEntityResolve(entityResolveCtx("u"), json.RawMessage(`{"input":"x"}`))
	if err == nil || !strings.Contains(err.Error(), "entity_resolve_not_configured") {
		t.Fatalf("err = %v, want entity_resolve_not_configured", err)
	}
}

func TestEntityResolveClampsTopKToMaxCandidates(t *testing.T) {
	resolver := &fakeEntityResolver{
		candidates: []EntityCandidate{{ArtifactID: "a", Label: "A", Score: 0.99}},
	}
	withEntityResolveServices(t, &EntityResolveServices{
		Resolver:        resolver,
		ConfidenceFloor: 0.5,
		MaxCandidates:   3,
		Timeout:         100 * time.Millisecond,
	})

	args := json.RawMessage(`{"input":"x","top_k":100}`)
	_, err := handleEntityResolve(entityResolveCtx("u"), args)
	if err != nil {
		t.Fatalf("handler err: %v", err)
	}
	if resolver.lastLimit != 3 {
		t.Fatalf("limit = %d, want clamped to 3", resolver.lastLimit)
	}
}
