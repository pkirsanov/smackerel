// Spec 061 SCOPE-06 — unit tests for the retrieval facade-side
// source-assembler adapter. Proves parse correctness, lookup
// integration, and zero-value-return semantics.

package retrieval

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/assistant/contracts"
)

// fakeLookup is a deterministic in-memory ArtifactLookupFn keyed by ID.
type fakeLookupEntry struct {
	title      string
	capturedAt time.Time
	found      bool
	err        error
}

func fakeLookup(entries map[string]fakeLookupEntry) ArtifactLookupFn {
	return func(_ context.Context, id string) (string, time.Time, bool, error) {
		e, ok := entries[id]
		if !ok {
			return "", time.Time{}, false, nil
		}
		return e.title, e.capturedAt, e.found, e.err
	}
}

// TestNewFacadeAssembler_HappyPath_AnswerAndSources proves the
// assembler parses retrieval-qa-v1 Final shape, calls the lookup,
// and returns a SourceAssembly with Body=answer + populated Sources.
func TestNewFacadeAssembler_HappyPath_AnswerAndSources(t *testing.T) {
	t.Parallel()

	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	entries := map[string]fakeLookupEntry{
		"art-1": {title: "Note A", capturedAt: now.Add(-24 * time.Hour), found: true},
		"art-2": {title: "Note B", capturedAt: now.Add(-48 * time.Hour), found: true},
	}
	asm := NewFacadeAssembler(fakeLookup(entries), 5)

	result := &agent.InvocationResult{
		Outcome: agent.OutcomeOK,
		Final:   []byte(`{"answer":"Three notes mention Tailscale.","cited_artifact_ids":["art-1","art-2"]}`),
	}
	assembly := asm(context.Background(), result)

	if assembly.Body != "Three notes mention Tailscale." {
		t.Errorf("Body = %q; want %q", assembly.Body, "Three notes mention Tailscale.")
	}
	if len(assembly.Sources) != 2 {
		t.Fatalf("Sources length = %d; want 2", len(assembly.Sources))
	}
	if assembly.Sources[0].ID != "art-1" || assembly.Sources[1].ID != "art-2" {
		t.Errorf("Sources order = [%q, %q]; want [art-1, art-2]", assembly.Sources[0].ID, assembly.Sources[1].ID)
	}
	if assembly.Sources[0].Title != "Note A" {
		t.Errorf("Sources[0].Title = %q; want Note A", assembly.Sources[0].Title)
	}
	if assembly.Sources[0].Kind != contracts.SourceArtifact {
		t.Errorf("Sources[0].Kind = %q; want %q", assembly.Sources[0].Kind, contracts.SourceArtifact)
	}
	if assembly.OverflowCount != 0 {
		t.Errorf("OverflowCount = %d; want 0", assembly.OverflowCount)
	}
}

// TestNewFacadeAssembler_GraphDrift_DropsMissingArtifacts proves
// that when every cited ID resolves to "not found", the assembler
// returns an empty Sources slice + the body it parsed. The Facade
// then forwards this to provenance.Enforce which fires the canonical
// refusal because Sources is empty and Body is non-empty.
func TestNewFacadeAssembler_GraphDrift_DropsMissingArtifacts(t *testing.T) {
	t.Parallel()

	// lookup returns found=false for everything.
	asm := NewFacadeAssembler(fakeLookup(map[string]fakeLookupEntry{}), 5)

	result := &agent.InvocationResult{
		Outcome: agent.OutcomeOK,
		Final:   []byte(`{"answer":"Two notes mention Tailscale.","cited_artifact_ids":["art-deleted-1","art-deleted-2"]}`),
	}
	assembly := asm(context.Background(), result)

	if assembly.Body != "Two notes mention Tailscale." {
		t.Errorf("Body = %q; want body parsed from answer field", assembly.Body)
	}
	if len(assembly.Sources) != 0 {
		t.Errorf("Sources length = %d; want 0 (graph drift)", len(assembly.Sources))
	}
}

// TestNewFacadeAssembler_LookupError_DropsAndContinues proves
// transient lookup errors are silently dropped per the contract
// (counter increment is the observable). The assembler MUST NOT
// surface the error and MUST NOT panic.
func TestNewFacadeAssembler_LookupError_DropsAndContinues(t *testing.T) {
	t.Parallel()

	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	entries := map[string]fakeLookupEntry{
		"art-1": {title: "OK", capturedAt: now, found: true},
		"art-2": {err: errors.New("transient: connection refused")},
	}
	asm := NewFacadeAssembler(fakeLookup(entries), 5)

	result := &agent.InvocationResult{
		Outcome: agent.OutcomeOK,
		Final:   []byte(`{"answer":"...","cited_artifact_ids":["art-1","art-2"]}`),
	}
	assembly := asm(context.Background(), result)

	if len(assembly.Sources) != 1 {
		t.Fatalf("Sources length = %d; want 1 (art-1 survives, art-2 lookup error drops it)", len(assembly.Sources))
	}
	if assembly.Sources[0].ID != "art-1" {
		t.Errorf("Surviving Source.ID = %q; want art-1", assembly.Sources[0].ID)
	}
}

// TestNewFacadeAssembler_MalformedFinal_ReturnsZeroValue proves
// that a Final payload that does not match the retrieval-qa-v1
// shape (e.g. a plain string literal) yields a zero-value
// SourceAssembly. The Facade interprets this as "no override" and
// the default-render path stands.
func TestNewFacadeAssembler_MalformedFinal_ReturnsZeroValue(t *testing.T) {
	t.Parallel()

	asm := NewFacadeAssembler(fakeLookup(nil), 5)

	cases := []struct {
		name  string
		final []byte
	}{
		{"empty", nil},
		{"empty-string", []byte("")},
		{"plain-string-literal", []byte(`"just an answer with no structure"`)},
		{"not-json", []byte(`not even json`)},
		{"wrong-shape", []byte(`{"foo":"bar"}`)},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			result := &agent.InvocationResult{
				Outcome: agent.OutcomeOK,
				Final:   c.final,
			}
			assembly := asm(context.Background(), result)
			// "wrong-shape" successfully unmarshals to the local
			// payload (Answer and CitedArtifactIDs are both zero
			// values), so we accept either (a) empty body + nil
			// Sources (the canonical zero-value path) or (b) the
			// parsed-but-empty payload. The Facade behavior is
			// identical for both because Body is empty and Sources
			// is empty.
			if len(assembly.Sources) != 0 {
				t.Errorf("Sources length = %d; want 0", len(assembly.Sources))
			}
			if assembly.OverflowCount != 0 {
				t.Errorf("OverflowCount = %d; want 0", assembly.OverflowCount)
			}
		})
	}
}

// TestNewFacadeAssembler_NonOKOutcome_ReturnsZeroValue proves that
// non-OK outcomes (timeout, provider error, schema failure) skip
// the assembler entirely — the Facade's default body-translation
// path handles those.
func TestNewFacadeAssembler_NonOKOutcome_ReturnsZeroValue(t *testing.T) {
	t.Parallel()

	asm := NewFacadeAssembler(fakeLookup(nil), 5)

	cases := []agent.Outcome{
		agent.OutcomeTimeout,
		agent.OutcomeProviderError,
		agent.OutcomeSchemaFailure,
		agent.OutcomeLoopLimit,
	}
	for _, oc := range cases {
		oc := oc
		t.Run(string(oc), func(t *testing.T) {
			t.Parallel()
			result := &agent.InvocationResult{
				Outcome: oc,
				Final:   []byte(`{"answer":"x","cited_artifact_ids":["art-1"]}`),
			}
			assembly := asm(context.Background(), result)
			if assembly.Body != "" {
				t.Errorf("Body = %q; want empty (non-OK outcome bypasses assembler)", assembly.Body)
			}
			if len(assembly.Sources) != 0 {
				t.Errorf("Sources length = %d; want 0", len(assembly.Sources))
			}
		})
	}
}

// TestNewFacadeAssembler_NilResult_ReturnsZeroValue proves defensive
// nil-handling so the assembler is safe to invoke from any future
// caller (e.g. a unit test that passes nil).
func TestNewFacadeAssembler_NilResult_ReturnsZeroValue(t *testing.T) {
	t.Parallel()

	asm := NewFacadeAssembler(fakeLookup(nil), 5)
	assembly := asm(context.Background(), nil)
	if assembly.Body != "" {
		t.Errorf("Body = %q; want empty", assembly.Body)
	}
	if len(assembly.Sources) != 0 {
		t.Errorf("Sources length = %d; want 0", len(assembly.Sources))
	}
}

// TestNewFacadeAssembler_PanicsOnNilLookupOrZeroMax proves the
// constructor catches wiring-time misconfiguration immediately
// rather than silently producing empty Sources on every request.
func TestNewFacadeAssembler_PanicsOnNilLookup(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("NewFacadeAssembler MUST panic on nil ArtifactLookupFn")
		}
	}()
	_ = NewFacadeAssembler(nil, 5)
}

func TestNewFacadeAssembler_PanicsOnZeroSourcesMax(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("NewFacadeAssembler MUST panic on sourcesMax <= 0")
		}
	}()
	_ = NewFacadeAssembler(fakeLookup(nil), 0)
}

// TestNewFacadeAssembler_RespectsSourcesMax proves the assembler
// forwards the sourcesMax cap into AssembleSources so Overflow is
// reported correctly.
func TestNewFacadeAssembler_RespectsSourcesMax(t *testing.T) {
	t.Parallel()

	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	entries := map[string]fakeLookupEntry{
		"art-1": {title: "A", capturedAt: now, found: true},
		"art-2": {title: "B", capturedAt: now, found: true},
		"art-3": {title: "C", capturedAt: now, found: true},
		"art-4": {title: "D", capturedAt: now, found: true},
	}
	asm := NewFacadeAssembler(fakeLookup(entries), 2) // cap = 2

	result := &agent.InvocationResult{
		Outcome: agent.OutcomeOK,
		Final:   []byte(`{"answer":"four cited","cited_artifact_ids":["art-1","art-2","art-3","art-4"]}`),
	}
	assembly := asm(context.Background(), result)

	if len(assembly.Sources) != 2 {
		t.Fatalf("Sources length = %d; want 2 (capped)", len(assembly.Sources))
	}
	if assembly.OverflowCount != 2 {
		t.Errorf("OverflowCount = %d; want 2 (4 cited, 2 cap → 2 overflow)", assembly.OverflowCount)
	}
}
