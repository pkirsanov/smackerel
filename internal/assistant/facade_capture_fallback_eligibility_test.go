// Spec 074 SCOPE-04A — eligibility-gate unit test for the
// capture-as-fallback facade hook.
//
// Verifies that captureFallbackEligible reports false when the
// conversation carries either a PendingConfirm or a PendingDisambig
// — i.e. confirm-state and in-flight clarify-state turns MUST NOT be
// captured by the SCOPE-04A hook (their replies belong to the
// pending state machine, not to the unrouted/no-ground capture path).

package assistant

import (
	"testing"

	assistantctx "github.com/smackerel/smackerel/internal/assistant/context"
)

func TestCaptureFallbackEligible(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		conv assistantctx.Conversation
		want bool
	}{
		{
			name: "empty conversation is eligible",
			conv: assistantctx.Conversation{},
			want: true,
		},
		{
			name: "pending confirm blocks eligibility",
			conv: assistantctx.Conversation{
				PendingConfirm: &assistantctx.PendingConfirm{ConfirmRef: "cf-1"},
			},
			want: false,
		},
		{
			name: "pending disambig blocks eligibility",
			conv: assistantctx.Conversation{
				PendingDisambig: &assistantctx.PendingDisambig{DisambiguationRef: "d-1"},
			},
			want: false,
		},
		{
			name: "both pending states block eligibility",
			conv: assistantctx.Conversation{
				PendingConfirm:  &assistantctx.PendingConfirm{ConfirmRef: "cf-2"},
				PendingDisambig: &assistantctx.PendingDisambig{DisambiguationRef: "d-2"},
			},
			want: false,
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := captureFallbackEligible(c.conv); got != c.want {
				t.Errorf("captureFallbackEligible = %v, want %v", got, c.want)
			}
		})
	}
}
