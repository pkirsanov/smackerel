//go:build integration

// Spec 065 SCOPE-4 — entity_resolve integration tests.
//
// These cases exercise the spec 037 agent registry path end-to-end
// for the entity_resolve micro-tool:
//
//   - SCN-065-A06 user-scope isolation: the wired Resolver receives
//     the userID supplied at the tool boundary, and a Resolver that
//     refuses to surface artifacts outside that user produces only
//     in-scope candidates in the envelope.
//   - SCN-065-A06 ambiguity floor: when the top candidate's score is
//     below ConfidenceFloor, the envelope status is "ambiguous"
//     with the ranked candidate list; when it clears the floor, the
//     envelope status is "resolved" with the top artifact_id.
//
// The tests wire a fake Resolver via SetEntityResolveServices, drive
// the registered handler through agent.ByName("entity_resolve"), and
// assert the envelope schema validates and the user-scope contract
// is honored. Integration-tagged because the assertion exercises the
// production registry (init()-registered handlers, schema compile,
// envelope validation), not just the unit-level fake.

package assistant_integration

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/agent/tools/microtools"
)

// scopedFakeResolver enforces user-scope isolation: it returns
// candidates only for the userID it was constructed with, mimicking
// a graph adapter that filters by user_id at the storage layer.
type scopedFakeResolver struct {
	ownerUserID string
	candidates  []microtools.EntityCandidate
	seenUserIDs []string
}

func (r *scopedFakeResolver) Resolve(_ context.Context, userID, _, _ string, _ int) ([]microtools.EntityCandidate, error) {
	r.seenUserIDs = append(r.seenUserIDs, userID)
	if userID != r.ownerUserID {
		// Cross-user request: an honest graph adapter returns zero
		// candidates because the user owns no artifacts.
		return nil, nil
	}
	return r.candidates, nil
}

func TestEntityResolveIntegration_UserScopedGraphCandidatesOnly(t *testing.T) {
	// SCN-065-A06 user-scope isolation: userA owns two lease docs.
	// A call for userA must return them; a call for userB (who owns
	// nothing) must return failed/no_candidates without leaking.
	const userA = "user-A"
	const userB = "user-B"
	resolver := &scopedFakeResolver{
		ownerUserID: userA,
		candidates: []microtools.EntityCandidate{
			{ArtifactID: "art-A1", Label: "Apartment lease 2024", Score: 0.91, ArtifactType: "document"},
			{ArtifactID: "art-A2", Label: "Car lease 2023", Score: 0.74, ArtifactType: "document"},
		},
	}
	microtools.SetEntityResolveServices(&microtools.EntityResolveServices{
		Resolver:        resolver,
		ConfidenceFloor: 0.7,
		MaxCandidates:   5,
		Timeout:         500 * time.Millisecond,
	})
	t.Cleanup(microtools.ResetEntityResolveServicesForTest)

	tool, ok := agent.ByName(microtools.EntityResolveToolName)
	if !ok {
		t.Fatalf("agent.ByName(%q) returned !ok; init() registration regressed", microtools.EntityResolveToolName)
	}

	t.Run("owner_sees_own_artifacts", func(t *testing.T) {
		raw, err := tool.Handler(context.Background(), json.RawMessage(fmt.Sprintf(`{"input":"the lease","user_id":%q,"scope":"documents"}`, userA)))
		if err != nil {
			t.Fatalf("handler error: %v", err)
		}
		env := unmarshalEnvelope(t, raw)
		if env.Status != microtools.StatusResolved {
			t.Fatalf("status = %q, want %q", env.Status, microtools.StatusResolved)
		}
		if got := env.Value["artifact_id"]; got != "art-A1" {
			t.Fatalf("artifact_id = %v, want art-A1", got)
		}
	})

	t.Run("other_user_gets_no_candidates_without_leak", func(t *testing.T) {
		raw, err := tool.Handler(context.Background(), json.RawMessage(fmt.Sprintf(`{"input":"the lease","user_id":%q,"scope":"documents"}`, userB)))
		if err != nil {
			t.Fatalf("handler error: %v", err)
		}
		env := unmarshalEnvelope(t, raw)
		if env.Status != microtools.StatusFailed {
			t.Fatalf("status = %q, want %q (no candidates for non-owner)", env.Status, microtools.StatusFailed)
		}
		// Defensive: nothing from userA's set may have leaked.
		if env.Value != nil {
			if got, ok := env.Value["artifact_id"]; ok {
				t.Errorf("artifact_id %v leaked into envelope for non-owner; cross-user isolation regressed", got)
			}
		}
		if len(env.Candidates) > 0 {
			t.Errorf("candidates non-empty (%d) for non-owner; cross-user isolation regressed", len(env.Candidates))
		}
	})

	t.Run("resolver_observed_both_user_ids", func(t *testing.T) {
		if len(resolver.seenUserIDs) < 2 {
			t.Fatalf("resolver observed user_ids = %v, want both owner and non-owner calls", resolver.seenUserIDs)
		}
		if resolver.seenUserIDs[0] != userA || resolver.seenUserIDs[1] != userB {
			t.Fatalf("resolver user_id sequence = %v, want [%q %q] — userID must round-trip from tool input", resolver.seenUserIDs, userA, userB)
		}
	})
}

// floorFakeResolver returns a fixed candidate list whose scores are
// configurable per test case. Used to exercise the resolved/
// ambiguous boundary.
type floorFakeResolver struct {
	candidates []microtools.EntityCandidate
}

func (r *floorFakeResolver) Resolve(_ context.Context, _, _, _ string, _ int) ([]microtools.EntityCandidate, error) {
	return r.candidates, nil
}

func TestEntityResolveIntegration_LowConfidenceReturnsAmbiguous(t *testing.T) {
	const floor = 0.7
	cases := []struct {
		name       string
		topScore   float64
		wantStatus microtools.Status
	}{
		{name: "top_above_floor_resolves", topScore: 0.92, wantStatus: microtools.StatusResolved},
		{name: "top_equal_floor_resolves", topScore: floor, wantStatus: microtools.StatusResolved},
		{name: "top_below_floor_is_ambiguous", topScore: 0.5, wantStatus: microtools.StatusAmbiguous},
		{name: "tiny_score_is_ambiguous_not_failed", topScore: 0.05, wantStatus: microtools.StatusAmbiguous},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			resolver := &floorFakeResolver{
				candidates: []microtools.EntityCandidate{
					{ArtifactID: "art-1", Label: "Apartment lease 2024", Score: tc.topScore, ArtifactType: "document"},
					{ArtifactID: "art-2", Label: "Car lease 2023", Score: tc.topScore * 0.5, ArtifactType: "document"},
				},
			}
			microtools.SetEntityResolveServices(&microtools.EntityResolveServices{
				Resolver:        resolver,
				ConfidenceFloor: floor,
				MaxCandidates:   5,
				Timeout:         500 * time.Millisecond,
			})
			t.Cleanup(microtools.ResetEntityResolveServicesForTest)

			tool, ok := agent.ByName(microtools.EntityResolveToolName)
			if !ok {
				t.Fatalf("agent.ByName(%q) returned !ok", microtools.EntityResolveToolName)
			}
			raw, err := tool.Handler(context.Background(), json.RawMessage(`{"input":"the lease","user_id":"u1","scope":"documents"}`))
			if err != nil {
				t.Fatalf("handler error: %v", err)
			}
			env := unmarshalEnvelope(t, raw)
			if env.Status != tc.wantStatus {
				t.Fatalf("status = %q, want %q (top score=%g, floor=%g)", env.Status, tc.wantStatus, tc.topScore, floor)
			}
			if tc.wantStatus == microtools.StatusAmbiguous {
				if len(env.Candidates) == 0 {
					t.Errorf("ambiguous envelope has zero candidates; spec 061 disambiguation cannot run")
				}
			}
		})
	}
}

func unmarshalEnvelope(t *testing.T, raw json.RawMessage) microtools.Envelope {
	t.Helper()
	var env microtools.Envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v\nraw=%s", err, string(raw))
	}
	if err := microtools.ValidateEnvelope(env); err != nil {
		t.Fatalf("envelope failed validation: %v\nraw=%s", err, string(raw))
	}
	return env
}
