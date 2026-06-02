// Spec 076 SCOPE-2c — TP-076-02-05 (SCN-064-A06).
//
// Cite-back verifier MUST flip a fabricated-source citation to a
// refusal under enforce mode, and MUST log-without-refusal under
// shadow mode. The test exercises the pure Decide() seam the agent
// loop wires into its terminal-turn handler.

package citeback

import (
	"errors"
	"testing"

	ok "github.com/smackerel/smackerel/internal/assistant/openknowledge"
)

func TestCiteback_FabricatedSourceFlipsToRefusal(t *testing.T) {
	// Recorded trace contains a real web source; the model's
	// "citation" points at a different URL that was never returned by
	// any tool — the canonical fabricated-source pattern.
	ws := webTrace("https://example.com/real", "T", "S")
	trace := ToolTrace{{ToolName: "web_search", RecordedSources: []ok.Source{ws}}}
	fabricated := []Citation{{
		Kind:        ok.SourceWeb,
		URL:         "https://example.com/fabricated",
		ContentHash: "deadbeef",
	}}

	verdict := Verify(fabricated, trace)
	if verdict.OK {
		t.Fatalf("setup invariant: fabricated citation must fail Verify, got OK=true")
	}
	if len(verdict.Rejected) != 1 || !errors.Is(verdict.Rejected[0].Reason, ReasonNotInTrace) {
		t.Fatalf("expected single ReasonNotInTrace rejection, got %+v", verdict.Rejected)
	}

	t.Run("enforce flips to refusal", func(t *testing.T) {
		d := Decide(verdict, EnforcementEnforce)
		if !d.Refuse {
			t.Fatalf("enforce mode: Decide.Refuse=false, want true for fabricated source")
		}
		if !d.Mismatch {
			t.Fatalf("enforce mode: Decide.Mismatch=false, want true")
		}
		if d.Mode != EnforcementEnforce {
			t.Fatalf("enforce mode: Decide.Mode=%q, want %q", d.Mode, EnforcementEnforce)
		}
		if len(d.Verdict.Rejected) != 1 {
			t.Fatalf("enforce mode: lost rejected citations, got %+v", d.Verdict.Rejected)
		}
	})

	t.Run("shadow logs but does not refuse", func(t *testing.T) {
		d := Decide(verdict, EnforcementShadow)
		if d.Refuse {
			t.Fatalf("shadow mode: Decide.Refuse=true, want false (shadow MUST NOT alter the response)")
		}
		if !d.Mismatch {
			t.Fatalf("shadow mode: Decide.Mismatch=false, want true so the agent can log the violation")
		}
		if d.Mode != EnforcementShadow {
			t.Fatalf("shadow mode: Decide.Mode=%q, want %q", d.Mode, EnforcementShadow)
		}
	})

	t.Run("happy path never refuses regardless of mode", func(t *testing.T) {
		clean := Verify(nil, trace)
		if !clean.OK {
			t.Fatalf("setup invariant: empty citations must verify clean, got %+v", clean)
		}
		for _, m := range []EnforcementMode{EnforcementShadow, EnforcementEnforce} {
			d := Decide(clean, m)
			if d.Refuse || d.Mismatch {
				t.Fatalf("mode=%q clean verdict: Refuse=%v Mismatch=%v, want both false", m, d.Refuse, d.Mismatch)
			}
		}
	})
}

func TestCitebackEnforcementMode_ParseFailLoud(t *testing.T) {
	for _, ok := range []string{"shadow", "enforce", "  enforce  "} {
		if _, err := ParseEnforcementMode(ok); err != nil {
			t.Fatalf("ParseEnforcementMode(%q) err=%v want nil", ok, err)
		}
	}
	for _, bad := range []string{"", "   ", "audit", "off", "ENFORCE"} {
		if _, err := ParseEnforcementMode(bad); err == nil {
			t.Fatalf("ParseEnforcementMode(%q) err=nil want fail-loud", bad)
		}
	}
}
