package selfknowledge

// docsource.go — spec 104 SCOPE-05: the curated product-doc self-knowledge
// facet. This is the ONLY partly-curated facet; every other facet is derived
// fresh-by-construction from an executable SST (scenarios.yaml, shortcuts).
//
// The runtime core image ships ONLY the compiled binary (see Dockerfile —
// `COPY --from=builder /bin/smackerel-core`), NOT the repo docs/ tree, so the
// overview text is embedded into the binary via //go:embed. corpus/product_overview.md
// is the single, bounded, reviewed source; its "## <anchor>" H2 sections are
// declared in curatedDocSections. A declared anchor missing from the markdown
// fails LOUD at ingest time (G028 — no silent empty/partial corpus).

import (
	_ "embed"
	"fmt"
	"strings"
)

//go:embed corpus/product_overview.md
var productOverviewMarkdown string

// docSourceRef anchors the curated facet's provenance.
const docSourceRef = "internal/assistant/selfknowledge/corpus/product_overview.md"

// docSection declares one curated section to ingest. anchor is the EXACT H2
// heading text ("## <anchor>") in the embedded markdown; id is the stable
// CapabilityEntry ID suffix; kind classifies the entry (feature|usecase).
type docSection struct {
	anchor string
	id     string
	kind   string
}

// curatedDocSections is the bounded, reviewed allow-list of product-overview
// sections ingested as self-knowledge. Adding a section = add an entry here
// AND the matching "## <anchor>" block in corpus/product_overview.md; the two
// are kept in lockstep by the fail-loud check in Entries().
var curatedDocSections = []docSection{
	{anchor: "What Smackerel Is", id: "overview", kind: KindFeature},
	{anchor: "What You Can Do With It", id: "usecases", kind: KindUsecase},
	{anchor: "How It Works", id: "how-it-works", kind: KindFeature},
}

// DocCorpus implements the ingestor's DocSource over the embedded curated
// product overview. It is deterministic (declared-section order) so
// re-ingestion is idempotent.
type DocCorpus struct {
	markdown string
	sections []docSection
}

// NewDocCorpus returns the production DocCorpus over the embedded overview.
func NewDocCorpus() *DocCorpus {
	return &DocCorpus{markdown: productOverviewMarkdown, sections: curatedDocSections}
}

// Entries parses each declared section out of the embedded markdown into a
// CapabilityEntry. A declared anchor whose "## <anchor>" heading is absent or
// whose body is empty is a fail-loud error (no silent drop, G028).
func (d *DocCorpus) Entries() ([]CapabilityEntry, error) {
	out := make([]CapabilityEntry, 0, len(d.sections))
	for _, s := range d.sections {
		body, err := extractDocSection(d.markdown, s.anchor)
		if err != nil {
			return nil, fmt.Errorf("selfknowledge docsource: %w", err)
		}
		out = append(out, CapabilityEntry{
			Kind:      s.kind,
			ID:        s.kind + ":" + s.id,
			Title:     s.anchor,
			Body:      body,
			SourceRef: docSourceRef,
		})
	}
	return out, nil
}

// extractDocSection returns the trimmed text under the "## <anchor>" H2 up to
// the next "## " heading (or end of document). It errors if the heading is not
// present or the section body is empty.
func extractDocSection(md, anchor string) (string, error) {
	heading := "## " + anchor
	lines := strings.Split(md, "\n")
	start := -1
	for i, ln := range lines {
		if strings.TrimSpace(ln) == heading {
			start = i + 1
			break
		}
	}
	if start == -1 {
		return "", fmt.Errorf("declared section anchor %q not found in curated product overview", anchor)
	}
	var body []string
	for i := start; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "## ") {
			break
		}
		body = append(body, lines[i])
	}
	text := strings.TrimSpace(strings.Join(body, "\n"))
	if text == "" {
		return "", fmt.Errorf("declared section anchor %q has an empty body", anchor)
	}
	return text, nil
}
