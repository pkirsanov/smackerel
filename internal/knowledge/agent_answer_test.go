package knowledge

import (
	"testing"
)

func TestAgentAnswerLifecycleConstants(t *testing.T) {
	// Lock in the wire/string forms used in the migration's CHECK
	// constraint. A regression that renames the constant would also
	// silently break the DB INSERT path; this test makes that
	// breakage loud at unit-test time.
	cases := map[AgentAnswerLifecycle]string{
		AgentAnswerDerived:    "derived",
		AgentAnswerPromoted:   "promoted",
		AgentAnswerSuperseded: "superseded",
	}
	for got, want := range cases {
		if string(got) != want {
			t.Errorf("AgentAnswerLifecycle %q != expected %q", got, want)
		}
	}
}

func TestAgentAnswerSourceKindConstants(t *testing.T) {
	cases := map[AgentAnswerSourceKind]string{
		AgentAnswerSourceWeb:       "web",
		AgentAnswerSourceArtifact:  "artifact",
		AgentAnswerSourceToolTrace: "tool_computation",
	}
	for got, want := range cases {
		if string(got) != want {
			t.Errorf("AgentAnswerSourceKind %q != expected %q", got, want)
		}
	}
}

func TestNullableText(t *testing.T) {
	if got := nullableText(""); got != nil {
		t.Errorf("nullableText(\"\") = %v, want nil", got)
	}
	if got := nullableText("x"); got != "x" {
		t.Errorf("nullableText(\"x\") = %v, want \"x\"", got)
	}
}
