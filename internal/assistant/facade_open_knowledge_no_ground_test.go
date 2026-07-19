// Spec 074 SCOPE-04B — unit test for the open_knowledge no-ground
// trigger predicate consumed by the capture-as-fallback hook.
//
// openKnowledgeNoGround returns true iff the open_knowledge
// InvocationResult final envelope decodes to status="refused".
// The capture-as-fallback hook at internal/assistant/facade.go uses
// this predicate to map open_knowledge refusals onto
// CauseOpenKnowledgeNoGround. If this predicate misclassifies, the
// no-ground capture path either silently drops captures (status
// !="refused" mistakenly returning true is impossible; false
// negatives are the regression risk) or the hook fires on grounded
// answers (status="ok" mistakenly returning true). Both regressions
// would break SCOPE-074-04B's canonical-ack contract.

package assistant

import (
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/assistant/contracts"
)

func TestOpenKnowledgeNoGround(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		result *agent.InvocationResult
		want   bool
	}{
		{
			name:   "nil_result_is_not_no_ground",
			result: nil,
			want:   false,
		},
		{
			name:   "empty_final_is_not_no_ground",
			result: &agent.InvocationResult{Final: nil},
			want:   false,
		},
		{
			name:   "refused_status_is_no_ground",
			result: &agent.InvocationResult{Final: []byte(`{"status":"refused"}`)},
			want:   true,
		},
		{
			name:   "ok_status_is_grounded",
			result: &agent.InvocationResult{Final: []byte(`{"status":"ok"}`)},
			want:   false,
		},
		{
			name:   "non_json_final_is_not_no_ground",
			result: &agent.InvocationResult{Final: []byte(`not json`)},
			want:   false,
		},
		{
			name:   "missing_status_is_not_no_ground",
			result: &agent.InvocationResult{Final: []byte(`{"body":"hi"}`)},
			want:   false,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := openKnowledgeNoGround(tc.result); got != tc.want {
				t.Errorf("openKnowledgeNoGround() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCanonicalizeSuccessfulCaptureResponse_ClearsUpstreamFailureShape(t *testing.T) {
	emittedAt := time.Date(2026, time.July, 19, 20, 30, 0, 0, time.UTC)
	routing := &agent.RoutingDecision{}
	invocation := &agent.InvocationResult{TraceID: "trace-preserved"}
	got := canonicalizeSuccessfulCaptureResponse(contracts.AssistantResponse{
		Invocation:             invocation,
		Routing:                routing,
		Status:                 contracts.StatusSavedAsIdea,
		Sources:                []contracts.Source{{ID: "stale-source"}},
		SourcesOverflowCount:   3,
		ConfirmCard:            &contracts.ConfirmCard{},
		DisambiguationPrompt:   &contracts.DisambiguationPrompt{},
		ErrorCause:             contracts.ErrProviderUnavailable,
		CaptureRoute:           true,
		Body:                   "I don't have a sourced answer for that.",
		LegacyRetirementNotice: &contracts.NoticePayload{Command: "/weather"},
	}, emittedAt)

	if got.Status != contracts.StatusSavedAsIdea || !got.CaptureRoute {
		t.Fatalf("status=%q capture_route=%v, want saved_as_idea true", got.Status, got.CaptureRoute)
	}
	if got.ErrorCause != "" || got.Body != captureFallbackAcknowledgement {
		t.Fatalf("error_cause=%q body=%q, want empty and canonical acknowledgement", got.ErrorCause, got.Body)
	}
	if len(got.Sources) != 0 || got.SourcesOverflowCount != 0 || got.ConfirmCard != nil || got.DisambiguationPrompt != nil {
		t.Fatalf("stale response controls survived canonicalization: %+v", got)
	}
	if got.Invocation != invocation || got.Routing != routing || got.LegacyRetirementNotice == nil || !got.EmittedAt.Equal(emittedAt) {
		t.Fatalf("correlation or additive notice metadata was not preserved: %+v", got)
	}
}

func TestCanonicalizeSuccessfulCaptureResponse_LeavesExplicitFailureUnchanged(t *testing.T) {
	emittedAt := time.Date(2026, time.July, 19, 20, 35, 0, 0, time.UTC)
	want := contracts.AssistantResponse{
		Status:       contracts.StatusUnavailable,
		ErrorCause:   contracts.ErrInternalError,
		CaptureRoute: false,
		Body:         "capture failed: database unavailable",
		EmittedAt:    emittedAt,
	}
	got := canonicalizeSuccessfulCaptureResponse(want, emittedAt.Add(time.Minute))
	if got.Status != want.Status || got.ErrorCause != want.ErrorCause || got.CaptureRoute != want.CaptureRoute || got.Body != want.Body || !got.EmittedAt.Equal(want.EmittedAt) {
		t.Fatalf("explicit capture failure changed: got=%+v want=%+v", got, want)
	}
}
