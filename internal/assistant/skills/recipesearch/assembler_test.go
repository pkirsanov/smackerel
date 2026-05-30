// BUG-061-003 — recipe_search assembler unit tests covering:
//
//	S01: clean "find best recipe" populates Body+Sources (non-zero hits)
//	S03: empty-graph Final → Override{StatusUnavailable, CaptureRoute:false, actionable body}
//
// S02 (misspelling-tolerant routing) is covered by the router-level
// test in internal/agent/normalize_test.go; nothing in the assembler
// changes between the clean and misspelled paths because the router
// normalizes BEFORE the executor is invoked, so by the time the
// assembler runs, the Final shape is identical.
package recipesearch

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/assistant/contracts"
)

func okResult(final string) *agent.InvocationResult {
	return &agent.InvocationResult{
		ScenarioID: ScenarioID,
		Outcome:    agent.OutcomeOK,
		Final:      []byte(final),
	}
}

func fixedLookup(t time.Time) func(context.Context, string) (string, time.Time, bool, error) {
	return func(_ context.Context, id string) (string, time.Time, bool, error) {
		switch id {
		case "rid-1":
			return "Pasta Carbonara", t, true, nil
		case "rid-2":
			return "Margherita Pizza", t, true, nil
		}
		return "", time.Time{}, false, nil
	}
}

// S01 — non-empty Final with citations populates Body + Sources.
func TestRecipeAssembler_S01_PopulatesSources(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	asm := NewFacadeAssembler(fixedLookup(at), 5)
	out := asm(context.Background(), okResult(`{"answer":"You have Pasta Carbonara and Margherita Pizza.","cited_artifact_ids":["rid-1","rid-2"]}`))
	if out.Override != nil {
		t.Fatalf("non-empty Final must NOT set Override; got %+v", out.Override)
	}
	if out.Body == "" || !strings.Contains(out.Body, "Carbonara") {
		t.Errorf("Body = %q; want non-empty answer text", out.Body)
	}
	if len(out.Sources) != 2 {
		t.Fatalf("Sources len = %d; want 2", len(out.Sources))
	}
	if out.Sources[0].ID != "rid-1" || out.Sources[1].ID != "rid-2" {
		t.Errorf("Sources IDs = %v; want [rid-1 rid-2]", []string{out.Sources[0].ID, out.Sources[1].ID})
	}
}

// S03 — adversarial empty-graph contract.
//
// Empty answer + empty citations MUST produce a ResponseOverride with
// Status=Unavailable, CaptureRoute=false, and an actionable body. The
// body MUST NOT equal the BandLow "saved as idea" canonical string —
// without that guard a future maintainer could "fix" the empty-graph
// case by falling back to capture and silently re-introduce BUG-061-003.
func TestRecipeAssembler_S03_EmptyGraph_OverrideUnavailable_Adversarial(t *testing.T) {
	t.Parallel()
	asm := NewFacadeAssembler(fixedLookup(time.Now()), 5)
	out := asm(context.Background(), okResult(`{"answer":"","cited_artifact_ids":[]}`))

	if out.Override == nil {
		t.Fatalf("empty Final MUST emit Override; got %+v", out)
	}
	if out.Override.Status != contracts.StatusUnavailable {
		t.Errorf("Status = %q; want %q", out.Override.Status, contracts.StatusUnavailable)
	}
	if out.Override.CaptureRoute {
		t.Errorf("CaptureRoute = true; MUST be false per BUG-061-003 D5 (no fall-through to capture)")
	}
	if out.Override.ErrorCause != contracts.ErrNoMatch {
		t.Errorf("ErrorCause = %q; want %q", out.Override.ErrorCause, contracts.ErrNoMatch)
	}
	if out.Override.Body == "" {
		t.Fatal("Override.Body MUST be non-empty (Principle 8 actionable message)")
	}
	// Adversarial: body MUST name a concrete next action.
	bl := strings.ToLower(out.Override.Body)
	if !(strings.Contains(bl, "capture") || strings.Contains(bl, "connector") || strings.Contains(bl, "import")) {
		t.Errorf("Body %q does NOT name a next action (expect one of: capture/connector/import)", out.Override.Body)
	}
	// Adversarial: must NOT equal the BandLow canonical capture string.
	if strings.Contains(bl, "saved as an idea") {
		t.Errorf("Body %q regressed to BandLow capture-style copy", out.Override.Body)
	}
}

// Adversarial: malformed JSON Final (Outcome=OK but body cannot be
// parsed into recipeFinalPayload) MUST route through the provenance
// gate refusal path (zero-value SourceAssembly), NOT silently emit a
// recipe override. If the assembler ever started returning an
// Override for unparseable payloads, the facade would skip the
// provenance gate AND the empty-graph guard, producing an unsourced
// recipe response — re-introducing the BUG-061-003 trust breach in a
// different shape.
func TestRecipeAssembler_OKOutcome_MalformedJSON_NoOverride_Adversarial(t *testing.T) {
	t.Parallel()
	asm := NewFacadeAssembler(fixedLookup(time.Now()), 5)

	cases := []struct {
		name string
		raw  string
	}{
		{"not_json", `this is not json`},
		{"truncated", `{"answer":"x","cited_artifact_ids":[`},
		{"wrong_types", `{"answer":42,"cited_artifact_ids":"rid-1"}`},
		{"binary_garbage", "\x00\x01\x02not-json"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out := asm(context.Background(), okResult(tc.raw))
			if out.Override != nil {
				t.Fatalf("malformed Final MUST NOT emit Override (would skip provenance gate); got %+v", out.Override)
			}
			if out.Body != "" || len(out.Sources) != 0 || out.OverflowCount != 0 || out.Cause != "" {
				t.Fatalf("malformed Final MUST return zero-value SourceAssembly; got %+v", out)
			}
		})
	}
}

// Non-OK outcomes (timeout / schema fail) MUST NOT emit Override —
// the facade's default body rendering and provenance gate stay in
// charge so requires_provenance failures continue to refuse.
func TestRecipeAssembler_NonOKOutcome_NoOverride(t *testing.T) {
	t.Parallel()
	asm := NewFacadeAssembler(fixedLookup(time.Now()), 5)
	out := asm(context.Background(), &agent.InvocationResult{
		Outcome: agent.OutcomeProviderError,
		Final:   []byte(`{"answer":"","cited_artifact_ids":[]}`),
	})
	if out.Override != nil {
		t.Fatalf("non-OK outcome MUST NOT set Override; got %+v", out.Override)
	}
	if out.Body != "" || len(out.Sources) != 0 {
		t.Fatalf("non-OK outcome MUST return zero-value; got body=%q sources=%v", out.Body, out.Sources)
	}
}
