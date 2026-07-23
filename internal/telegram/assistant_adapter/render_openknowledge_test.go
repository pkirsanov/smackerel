package assistant_adapter

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
)

// Helper constructors so tests stay readable.

func webSrc(title, urlStr, provider, hash string) contracts.Source {
	return contracts.Source{
		ID:    urlStr,
		Title: title,
		Kind:  contracts.SourceWeb,
		Ref: contracts.WebSourceRef{
			URL:         urlStr,
			Provider:    provider,
			FetchedAt:   time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC),
			ContentHash: hash,
			Snippet:     "snippet for " + title,
		},
	}
}

func artifactSrc(title, id string) contracts.Source {
	return contracts.Source{
		ID:    id,
		Title: title,
		Kind:  contracts.SourceArtifact,
		Ref: contracts.ArtifactRef{
			ArtifactID: id,
			CapturedAt: time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC),
		},
	}
}

func compSrc(tool, inputHash, outputHash string) contracts.Source {
	return contracts.Source{
		ID:    tool,
		Title: tool,
		Kind:  contracts.SourceToolComputation,
		Ref: contracts.ComputationSourceRef{
			Tool:       tool,
			InputHash:  inputHash,
			OutputHash: outputHash,
		},
	}
}

// TestRenderSourcedAnswer_SingleWeb covers a single web source with
// the expected "[1] Title — domain (web)" shape.
func TestRenderSourcedAnswer_SingleWeb(t *testing.T) {
	t.Parallel()
	body := "Pad Thai uses tamarind, fish sauce, palm sugar."
	got, err := RenderSourcedAnswer(body, []contracts.Source{
		webSrc("Pad Thai 101", "https://www.example.com/recipes/pad-thai", "searxng", "h1"),
	})
	if err != nil {
		t.Fatalf("RenderSourcedAnswer: %v", err)
	}
	want := body + "\n\n[1] Pad Thai 101 — example.com (web)"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// TestRenderSourcedAnswer_MixedThree exercises one source of each
// renderable kind (artifact, web, computation), asserts kind-grouped
// deterministic ordering, and asserts each marker is distinct.
func TestRenderSourcedAnswer_MixedThree(t *testing.T) {
	t.Parallel()
	body := "Mixed answer."
	got, err := RenderSourcedAnswer(body, []contracts.Source{
		compSrc("calculator", "ih", "oh"),
		webSrc("Pad Thai 101", "https://example.com/x", "searxng", "h1"),
		artifactSrc("My note", "aaaaaaaabbbb"),
	})
	if err != nil {
		t.Fatalf("RenderSourcedAnswer: %v", err)
	}
	want := body + "\n\n" +
		"[1] My note (your graph)\n" +
		"[2] Pad Thai 101 — example.com (web)\n" +
		"[3] computed via calculator (computed)"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// TestRenderSourcedAnswer_OverflowSummary covers >3 sources: only 3
// inline, "... +N more sources" tail.
func TestRenderSourcedAnswer_OverflowSummary(t *testing.T) {
	t.Parallel()
	body := "Many."
	got, err := RenderSourcedAnswer(body, []contracts.Source{
		webSrc("A", "https://a.example.com/x", "searxng", "h1"),
		webSrc("B", "https://b.example.com/x", "searxng", "h2"),
		webSrc("C", "https://c.example.com/x", "searxng", "h3"),
		webSrc("D", "https://d.example.com/x", "searxng", "h4"),
		webSrc("E", "https://e.example.com/x", "searxng", "h5"),
	})
	if err != nil {
		t.Fatalf("RenderSourcedAnswer: %v", err)
	}
	if !strings.Contains(got, "... +2 more sources") {
		t.Errorf("expected overflow summary in output, got:\n%s", got)
	}
	if strings.Contains(got, "[4]") || strings.Contains(got, "[5]") {
		t.Errorf("expected inline citations capped at 3, got:\n%s", got)
	}
}

// TestRenderSourcedAnswer_StableOrderUnderShuffle is the adversarial
// determinism test: two different input orderings of the same
// source set MUST produce identical output.
func TestRenderSourcedAnswer_StableOrderUnderShuffle(t *testing.T) {
	t.Parallel()
	body := "Order test."
	orderA := []contracts.Source{
		artifactSrc("alpha", "id-a"),
		webSrc("beta", "https://example.com/b", "p", "h"),
		compSrc("calculator", "ih", "oh"),
	}
	orderB := []contracts.Source{
		compSrc("calculator", "ih", "oh"),
		webSrc("beta", "https://example.com/b", "p", "h"),
		artifactSrc("alpha", "id-a"),
	}
	a, errA := RenderSourcedAnswer(body, orderA)
	b, errB := RenderSourcedAnswer(body, orderB)
	if errA != nil || errB != nil {
		t.Fatalf("RenderSourcedAnswer errs: %v / %v", errA, errB)
	}
	if a != b {
		t.Errorf("shuffled inputs produced different output:\nA:\n%s\nB:\n%s", a, b)
	}
}

// TestRenderSourcedAnswer_AdversarialEmptySources asserts G021: the
// renderer refuses to fabricate a sourced answer with zero sources.
func TestRenderSourcedAnswer_AdversarialEmptySources(t *testing.T) {
	t.Parallel()
	got, err := RenderSourcedAnswer("any body", nil)
	if !errors.Is(err, ErrOpenKnowledgeRendererNoSources) {
		t.Errorf("expected ErrOpenKnowledgeRendererNoSources, got err=%v output=%q", err, got)
	}
	if got != "" {
		t.Errorf("expected empty output on error, got %q", got)
	}
}

func TestRenderSourcedAnswer_EmptyBody(t *testing.T) {
	t.Parallel()
	_, err := RenderSourcedAnswer("   ", []contracts.Source{artifactSrc("t", "id")})
	if !errors.Is(err, ErrOpenKnowledgeRendererEmptyBody) {
		t.Errorf("expected ErrOpenKnowledgeRendererEmptyBody, got %v", err)
	}
}

// TestRenderSourcedAnswer_UnknownKind exercises the no-defaults
// (G028) contract: an unknown SourceKind is a typed error, never a
// silent passthrough.
func TestRenderSourcedAnswer_UnknownKind(t *testing.T) {
	t.Parallel()
	unknown := contracts.Source{
		ID: "x", Title: "y", Kind: contracts.SourceKind("bogus"),
	}
	_, err := RenderSourcedAnswer("body", []contracts.Source{unknown})
	if !errors.Is(err, ErrOpenKnowledgeRendererUnknownKind) {
		t.Errorf("expected ErrOpenKnowledgeRendererUnknownKind, got %v", err)
	}
}

// TestRenderRefusal_AllCauses table-drives every cause in
// AllRefusalCauses and asserts the honest canonical body is returned
// verbatim with NO user-visible capture marker (BUG-061-009).
func TestRenderRefusal_AllCauses(t *testing.T) {
	t.Parallel()
	for _, cause := range contracts.AllRefusalCauses {
		cause := cause
		t.Run(string(cause), func(t *testing.T) {
			t.Parallel()
			got := RenderRefusal(cause)
			canonical := contracts.CanonicalRefusalBodyFor(cause)
			if got != canonical {
				t.Errorf("RenderRefusal(%s) = %q; want the canonical body verbatim %q (no suffix)", cause, got, canonical)
			}
			if strings.Contains(got, "(saved as idea)") {
				t.Errorf("RenderRefusal(%s) = %q; must NOT contain a capture marker", cause, got)
			}
		})
	}
}

// TestRenderRefusal_Default asserts the default cause produces the
// honest default canonical body with no capture marker.
func TestRenderRefusal_Default(t *testing.T) {
	t.Parallel()
	got := RenderRefusal(contracts.RefusalDefault)
	want := "I don't have a sourced answer for that."
	if got != want {
		t.Errorf("got: %q\nwant: %q", got, want)
	}
}

// TestOpenKnowledgeAdversarialG021_RefusalDistinguishableFromSourced
// is the headline adversarial G021 test: a refusal is STRUCTURALLY
// distinguishable from a sourced answer (BUG-061-009) — a sourced
// answer carries "[1]" citation markers; a refusal is the honest
// canonical body with NO "[N]" markers and NO "(saved as idea)"
// capture marker.
func TestOpenKnowledgeAdversarialG021_RefusalDistinguishableFromSourced(t *testing.T) {
	t.Parallel()
	sourced, err := RenderSourcedAnswer("answer body", []contracts.Source{
		webSrc("ref", "https://example.com/", "searxng", "h"),
	})
	if err != nil {
		t.Fatalf("RenderSourcedAnswer: %v", err)
	}
	for _, cause := range contracts.AllRefusalCauses {
		refusal := RenderRefusal(cause)
		if strings.Contains(refusal, "(saved as idea)") {
			t.Errorf("refusal[%s] must not contain a capture marker: %q", cause, refusal)
		}
		if strings.Contains(refusal, "[1]") {
			t.Errorf("refusal[%s] must not contain [1]: %q", cause, refusal)
		}
		if refusal != contracts.CanonicalRefusalBodyFor(cause) {
			t.Errorf("refusal[%s] = %q; want the honest canonical body", cause, refusal)
		}
	}
	if !strings.Contains(sourced, "[1]") {
		t.Errorf("sourced answer missing [1] marker: %q", sourced)
	}
	if strings.Contains(sourced, "(saved as idea)") {
		t.Errorf("sourced answer must not contain a capture marker: %q", sourced)
	}
}

// TestRenderHybridAnswer_GroupedMixedKinds asserts the grouping
// labels appear in the deterministic order (graph, web, computation)
// and that citation numbering is contiguous across groups.
func TestRenderHybridAnswer_GroupedMixedKinds(t *testing.T) {
	t.Parallel()
	got, err := RenderHybridAnswer("Hybrid body.", []contracts.Source{
		compSrc("calculator", "ih", "oh"),
		webSrc("Web title", "https://example.com/x", "searxng", "h"),
		artifactSrc("Note title", "aaaaaaaabbbb"),
	})
	if err != nil {
		t.Fatalf("RenderHybridAnswer: %v", err)
	}
	want := "Hybrid body.\n" +
		"\nfrom your graph:\n[1] Note title (your graph)" +
		"\nfrom the web:\n[2] Web title — example.com (web)" +
		"\nfrom computation:\n[3] computed via calculator (computed)"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// TestRenderHybridAnswer_StableUnderShuffle: same as the sourced
// variant — input order has no effect on output.
func TestRenderHybridAnswer_StableUnderShuffle(t *testing.T) {
	t.Parallel()
	a, errA := RenderHybridAnswer("b", []contracts.Source{
		artifactSrc("alpha", "id-a"),
		webSrc("beta", "https://example.com/b", "p", "h"),
		compSrc("calculator", "ih", "oh"),
	})
	b, errB := RenderHybridAnswer("b", []contracts.Source{
		compSrc("calculator", "ih", "oh"),
		webSrc("beta", "https://example.com/b", "p", "h"),
		artifactSrc("alpha", "id-a"),
	})
	if errA != nil || errB != nil {
		t.Fatalf("RenderHybridAnswer errs: %v / %v", errA, errB)
	}
	if a != b {
		t.Errorf("shuffled inputs produced different output:\nA:\n%s\nB:\n%s", a, b)
	}
}

// TestRenderHybridAnswer_RejectsEmpty mirrors the sourced contract.
func TestRenderHybridAnswer_RejectsEmpty(t *testing.T) {
	t.Parallel()
	_, err := RenderHybridAnswer("body", nil)
	if !errors.Is(err, ErrOpenKnowledgeRendererNoSources) {
		t.Errorf("expected ErrOpenKnowledgeRendererNoSources, got %v", err)
	}
	_, err = RenderHybridAnswer("", []contracts.Source{artifactSrc("t", "id")})
	if !errors.Is(err, ErrOpenKnowledgeRendererEmptyBody) {
		t.Errorf("expected ErrOpenKnowledgeRendererEmptyBody, got %v", err)
	}
}
