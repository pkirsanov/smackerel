package selfknowledge

// docsource_test.go — spec 104 SCOPE-05 unit tests.

import (
	"strings"
	"testing"
)

func TestDocCorpus_Entries_FromEmbeddedOverview(t *testing.T) {
	t.Parallel()
	entries, err := NewDocCorpus().Entries()
	if err != nil {
		t.Fatalf("Entries: %v", err)
	}
	if len(entries) != len(curatedDocSections) {
		t.Fatalf("got %d entries, want %d (one per declared section)", len(entries), len(curatedDocSections))
	}
	byID := make(map[string]CapabilityEntry, len(entries))
	for _, e := range entries {
		if strings.TrimSpace(e.Body) == "" {
			t.Errorf("entry %q has empty body", e.ID)
		}
		if e.Kind != KindFeature && e.Kind != KindUsecase {
			t.Errorf("entry %q kind=%q, want feature|usecase", e.ID, e.Kind)
		}
		if e.SourceRef != docSourceRef {
			t.Errorf("entry %q SourceRef=%q, want %q", e.ID, e.SourceRef, docSourceRef)
		}
		byID[e.ID] = e
	}
	// The overview entry exists, is a feature, and carries real product text.
	overview, ok := byID["feature:overview"]
	if !ok {
		t.Fatalf("missing feature:overview entry (ids=%v)", keysOf(byID))
	}
	if !strings.Contains(strings.ToLower(overview.Body), "second brain") {
		t.Errorf("feature:overview body does not mention the product framing; got %q", overview.Body)
	}
	// The use-case facet is classified as a usecase.
	if uc, ok := byID["usecase:usecases"]; !ok || uc.Kind != KindUsecase {
		t.Errorf("missing/mis-kinded usecase:usecases entry: %+v (ok=%v)", uc, ok)
	}
}

func TestExtractDocSection_MissingAnchorFailsLoud(t *testing.T) {
	t.Parallel()
	md := "## Present\nbody text\n"
	if _, err := extractDocSection(md, "Absent"); err == nil {
		t.Fatal("want fail-loud error for a missing anchor, got nil")
	}
}

func TestExtractDocSection_EmptyBodyFailsLoud(t *testing.T) {
	t.Parallel()
	md := "## Empty\n\n## Next\nbody\n"
	if _, err := extractDocSection(md, "Empty"); err == nil {
		t.Fatal("want fail-loud error for an empty-body section, got nil")
	}
}

func TestExtractDocSection_StopsAtNextHeading(t *testing.T) {
	t.Parallel()
	md := "## A\nalpha line\n\n## B\nbeta line\n"
	body, err := extractDocSection(md, "A")
	if err != nil {
		t.Fatalf("extractDocSection: %v", err)
	}
	if !strings.Contains(body, "alpha line") || strings.Contains(body, "beta line") {
		t.Errorf("section A body leaked into B or missing content: %q", body)
	}
}

func TestDocCorpus_DeclaredAnchorMissingFromMarkdownFailsLoud(t *testing.T) {
	t.Parallel()
	// A DocCorpus whose declared section is absent from its markdown must fail
	// loud (proves the lockstep contract between curatedDocSections and the
	// embedded file, not just the happy embedded path).
	dc := &DocCorpus{
		markdown: "## Only This\nbody\n",
		sections: []docSection{{anchor: "Not There", id: "x", kind: KindFeature}},
	}
	if _, err := dc.Entries(); err == nil {
		t.Fatal("want fail-loud error when a declared anchor is missing, got nil")
	}
}

func keysOf(m map[string]CapabilityEntry) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
