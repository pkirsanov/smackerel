package assistant_adapter

import (
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
)

// Spec 064 SCOPE-13 — Telegram render functions for the
// open_knowledge scenario. Three response shapes:
//
//   - RenderSourcedAnswer: body + inline [N] citations capped at
//     OpenKnowledgeMaxInlineCitations (phone-screen-fit, P7).
//   - RenderRefusal: the honest, cause-specific refusal body for a
//     typed RefusalCause. It carries NO user-visible capture marker
//     (BUG-061-009): a high-band refusal reads as an honest "couldn't
//     answer", never a "saved as an idea" capture. Refusal output is
//     distinguished from a sourced answer STRUCTURALLY (the response
//     carries StatusUnavailable + a typed cause and no "[N]" citation
//     markers), not by a user-facing suffix.
//   - RenderHybridAnswer: same as RenderSourcedAnswer but groups
//     citations by Kind (graph → web → computation) with a small
//     group marker per design.md §UX / refusal taxonomy notes.
//
// Citations have a deterministic order independent of input order:
// sort by Kind (artifact, web, external_provider, computation) and
// then by Title+identifier so adversarial shuffles produce identical
// output (G021).
//
// No-defaults (G028): an unknown SourceKind is a typed error, never
// a silent passthrough. An empty Sources slice on a sourced-answer
// path is a typed error — the provenance gate is responsible for
// refusing earlier, and reaching the renderer with zero sources is
// a contract bug worth surfacing.
//
// Honest refusal (BUG-061-009): a refusal render carries no "saved as
// idea" suffix. A matched, executed request that cannot be grounded
// reads as an honest "couldn't answer"; the band-low capture
// acknowledgement is the only place "saved as an idea" belongs. Any
// silent capture the facade performs is never surfaced as the
// user-visible headline.

// OpenKnowledgeMaxInlineCitations is the phone-screen-fit cap on
// the number of inline [N] citations rendered. Additional sources
// are summarised as "... +N more sources".
const OpenKnowledgeMaxInlineCitations = 3

// ErrOpenKnowledgeRendererNoSources is returned when
// RenderSourcedAnswer or RenderHybridAnswer is called with zero
// sources. The provenance gate must refuse earlier; reaching the
// renderer with zero sources is a contract bug.
var ErrOpenKnowledgeRendererNoSources = errors.New(
	"assistant_adapter: open_knowledge renderer called with zero sources (provenance gate should have refused)",
)

// ErrOpenKnowledgeRendererEmptyBody is returned when a sourced
// renderer receives an empty body string.
var ErrOpenKnowledgeRendererEmptyBody = errors.New(
	"assistant_adapter: open_knowledge renderer called with empty body",
)

// ErrOpenKnowledgeRendererUnknownKind is returned when the renderer
// encounters a SourceKind it does not know how to format. No-defaults
// (G028): we do not silently coerce to a generic line; the caller
// must add explicit support.
var ErrOpenKnowledgeRendererUnknownKind = errors.New(
	"assistant_adapter: open_knowledge renderer received unknown SourceKind",
)

// RenderSourcedAnswer formats a successful open_knowledge answer as
// "<body>\n\n[1] ...\n[2] ...\n[3] ...\n... +N more sources" with
// deterministic citation ordering (see sortSourcesForOpenKnowledge).
// Returns a typed error on contract violations.
func RenderSourcedAnswer(body string, sources []contracts.Source) (string, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return "", ErrOpenKnowledgeRendererEmptyBody
	}
	if len(sources) == 0 {
		return "", ErrOpenKnowledgeRendererNoSources
	}
	ordered := sortSourcesForOpenKnowledge(sources)
	inline, overflow := capCitations(ordered, OpenKnowledgeMaxInlineCitations)
	lines, err := renderInlineCitationLines(inline)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString(body)
	b.WriteString("\n\n")
	for i, line := range lines {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(line)
	}
	if overflow > 0 {
		b.WriteString(fmt.Sprintf("\n... +%d more sources", overflow))
	}
	return b.String(), nil
}

// RenderHybridAnswer groups citations by Kind with a small marker
// per group: "from your graph:", "from the web:", "from computation:".
// Within each group the inline [N] numbering is contiguous across
// groups so the body's [N] back-references stay stable.
func RenderHybridAnswer(body string, sources []contracts.Source) (string, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return "", ErrOpenKnowledgeRendererEmptyBody
	}
	if len(sources) == 0 {
		return "", ErrOpenKnowledgeRendererNoSources
	}
	ordered := sortSourcesForOpenKnowledge(sources)
	inline, overflow := capCitations(ordered, OpenKnowledgeMaxInlineCitations)

	var b strings.Builder
	b.WriteString(body)
	b.WriteString("\n")

	n := 0
	var currentGroup string
	for _, src := range inline {
		group := openKnowledgeGroupLabelFor(src.Kind)
		if group == "" {
			return "", fmt.Errorf("%w: kind=%q", ErrOpenKnowledgeRendererUnknownKind, src.Kind)
		}
		if group != currentGroup {
			b.WriteString("\n")
			b.WriteString(group)
			currentGroup = group
		}
		n++
		line, err := renderInlineCitationLine(n, src)
		if err != nil {
			return "", err
		}
		b.WriteString("\n")
		b.WriteString(line)
	}
	if overflow > 0 {
		b.WriteString(fmt.Sprintf("\n... +%d more sources", overflow))
	}
	return b.String(), nil
}

// RenderRefusal renders the honest, cause-specific refusal body for
// cause (BUG-061-009). It carries NO user-visible capture marker: a
// high-band refusal must read as an honest "couldn't answer", never a
// "saved as an idea" capture. Refusal output stays distinguishable from
// a sourced answer STRUCTURALLY (the response is StatusUnavailable with
// a typed cause and no "[N]" citation markers), not by a user-facing
// suffix.
func RenderRefusal(cause contracts.RefusalCause) string {
	return strings.TrimSpace(contracts.CanonicalRefusalBodyFor(cause))
}

// openKnowledgeKindOrder is the deterministic ordering applied
// before citation numbering. The order is fixed in code (NOT derived
// from AllSourceKinds) so changes to the closed-vocabulary list do
// not silently re-order user-visible output.
var openKnowledgeKindOrder = map[contracts.SourceKind]int{
	contracts.SourceArtifact:         0,
	contracts.SourceWeb:              1,
	contracts.SourceExternalProvider: 2,
	contracts.SourceToolComputation:  3,
}

// sortSourcesForOpenKnowledge returns a copy of src sorted by Kind
// (per openKnowledgeKindOrder) then by Title then by ID so input
// order has no effect on rendered output.
func sortSourcesForOpenKnowledge(src []contracts.Source) []contracts.Source {
	out := make([]contracts.Source, len(src))
	copy(out, src)
	sort.SliceStable(out, func(i, j int) bool {
		ki, kj := openKnowledgeKindOrder[out[i].Kind], openKnowledgeKindOrder[out[j].Kind]
		if ki != kj {
			return ki < kj
		}
		if out[i].Title != out[j].Title {
			return out[i].Title < out[j].Title
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// capCitations returns the inline subset and the overflow count.
func capCitations(src []contracts.Source, max int) ([]contracts.Source, int) {
	if len(src) <= max {
		return src, 0
	}
	return src[:max], len(src) - max
}

// renderInlineCitationLines numbers each entry from 1.
func renderInlineCitationLines(src []contracts.Source) ([]string, error) {
	out := make([]string, 0, len(src))
	for i, s := range src {
		line, err := renderInlineCitationLine(i+1, s)
		if err != nil {
			return nil, err
		}
		out = append(out, line)
	}
	return out, nil
}

// renderInlineCitationLine formats one "[N] ... (<kind-marker>)" row.
func renderInlineCitationLine(n int, s contracts.Source) (string, error) {
	switch s.Kind {
	case contracts.SourceArtifact:
		title := strings.TrimSpace(s.Title)
		if title == "" {
			title = "(untitled)"
		}
		return fmt.Sprintf("[%d] %s (your graph)", n, title), nil
	case contracts.SourceWeb:
		ref, _ := s.Ref.(contracts.WebSourceRef)
		title := strings.TrimSpace(s.Title)
		if title == "" {
			title = strings.TrimSpace(ref.URL)
		}
		if title == "" {
			title = "(untitled)"
		}
		domain := extractDomain(ref.URL)
		if domain == "" {
			return fmt.Sprintf("[%d] %s (web)", n, title), nil
		}
		return fmt.Sprintf("[%d] %s — %s (web)", n, title, domain), nil
	case contracts.SourceExternalProvider:
		ref, _ := s.Ref.(contracts.ExternalProviderRef)
		title := strings.TrimSpace(s.Title)
		provider := strings.TrimSpace(ref.ProviderName)
		if title == "" {
			title = provider
		}
		if title == "" {
			title = "(untitled)"
		}
		if provider == "" {
			return fmt.Sprintf("[%d] %s (external)", n, title), nil
		}
		return fmt.Sprintf("[%d] %s — %s (external)", n, title, provider), nil
	case contracts.SourceToolComputation:
		ref, _ := s.Ref.(contracts.ComputationSourceRef)
		tool := strings.TrimSpace(ref.Tool)
		if tool == "" {
			tool = strings.TrimSpace(s.Title)
		}
		if tool == "" {
			tool = "(unknown tool)"
		}
		return fmt.Sprintf("[%d] computed via %s (computed)", n, tool), nil
	default:
		return "", fmt.Errorf("%w: kind=%q", ErrOpenKnowledgeRendererUnknownKind, s.Kind)
	}
}

// openKnowledgeGroupLabelFor returns the group header for a Kind, or
// "" for unsupported kinds (caller must treat as a typed error).
func openKnowledgeGroupLabelFor(k contracts.SourceKind) string {
	switch k {
	case contracts.SourceArtifact:
		return "from your graph:"
	case contracts.SourceWeb:
		return "from the web:"
	case contracts.SourceExternalProvider:
		return "from external sources:"
	case contracts.SourceToolComputation:
		return "from computation:"
	default:
		return ""
	}
}

// extractDomain returns the host of a URL with a leading "www."
// stripped. Returns "" on parse failure or missing host.
func extractDomain(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	host := u.Hostname()
	if host == "" {
		return ""
	}
	return strings.TrimPrefix(host, "www.")
}
