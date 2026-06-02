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

	"github.com/smackerel/smackerel/internal/agent"
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
