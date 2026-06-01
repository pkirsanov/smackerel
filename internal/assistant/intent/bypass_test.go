// Spec 068 SCN-068-A07 — operational commands bypass compiler explicitly.

package intent_test

import (
	"testing"

	"github.com/smackerel/smackerel/internal/assistant/intent"
)

// TestOperationalCommandBypassRecordsTraceLabel — SCN-068-A07.
//
// Every command in the carve-out set produces a trace stamped with
// label="operational_command_bypass" and outcome=OutcomeBypass; nothing
// else does.
func TestOperationalCommandBypassRecordsTraceLabel(t *testing.T) {
	cases := []struct {
		name    string
		text    string
		wantHit bool
		wantCmd string
	}{
		{name: "slash_help", text: "/help", wantHit: true, wantCmd: "/help"},
		{name: "slash_status", text: "/status", wantHit: true, wantCmd: "/status"},
		{name: "slash_reset", text: "/reset", wantHit: true, wantCmd: "/reset"},
		{name: "slash_digest", text: "/digest", wantHit: true, wantCmd: "/digest"},
		{name: "slash_recent", text: "/recent", wantHit: true, wantCmd: "/recent"},
		{name: "slash_done", text: "/done", wantHit: true, wantCmd: "/done"},
		{name: "leading_whitespace_status", text: "   /status", wantHit: true, wantCmd: "/status"},
		{name: "trailing_args_status", text: "/status now please", wantHit: true, wantCmd: "/status"},
		{name: "non_operational_ask", text: "/ask what time is it", wantHit: false},
		{name: "non_operational_weather", text: "/weather palm springs", wantHit: false},
		{name: "non_operational_remind", text: "/remind tomorrow", wantHit: false},
		{name: "natural_text", text: "make a shopping list", wantHit: false},
		{name: "empty", text: "", wantHit: false},
		{name: "whitespace_only", text: "   \t\n", wantHit: false},
		{name: "case_sensitive_uppercase", text: "/STATUS", wantHit: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd, ok := intent.IsOperationalCommand(tc.text)
			if ok != tc.wantHit {
				t.Fatalf("IsOperationalCommand(%q) ok=%v, want %v", tc.text, ok, tc.wantHit)
			}
			if ok && cmd != tc.wantCmd {
				t.Fatalf("IsOperationalCommand(%q) cmd=%q, want %q", tc.text, cmd, tc.wantCmd)
			}
			if ok {
				trace := intent.BypassTrace(tc.text, cmd)
				if trace.Outcome != intent.OutcomeBypass {
					t.Fatalf("BypassTrace outcome=%q, want %q", trace.Outcome, intent.OutcomeBypass)
				}
				if trace.Bypass == nil {
					t.Fatalf("BypassTrace produced nil Bypass record")
				}
				if trace.Bypass.Label != intent.BypassTraceLabel {
					t.Fatalf("BypassTrace label=%q, want %q", trace.Bypass.Label, intent.BypassTraceLabel)
				}
				if trace.Bypass.Command != tc.wantCmd {
					t.Fatalf("BypassTrace command=%q, want %q", trace.Bypass.Command, tc.wantCmd)
				}
				if trace.RawText != tc.text {
					t.Fatalf("BypassTrace raw_text=%q, want %q", trace.RawText, tc.text)
				}
			}
		})
	}
}

// Closed-vocabulary sentinel: the carve-out set must remain exactly
// the six commands enumerated in spec.md Hard Constraint 1. Adding or
// removing a member without owner approval is a spec violation.
func TestOperationalCommandsCarveOutIsTinyAndExplicit(t *testing.T) {
	want := []string{"/help", "/status", "/reset", "/digest", "/recent", "/done"}
	if got := len(intent.OperationalCommands); got != len(want) {
		t.Fatalf("OperationalCommands has %d entries, want %d (carve-out must stay tiny)", got, len(want))
	}
	for _, cmd := range want {
		if _, ok := intent.OperationalCommands[cmd]; !ok {
			t.Errorf("OperationalCommands missing required command %q", cmd)
		}
	}
}
