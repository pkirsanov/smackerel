package assistant_adapter

import (
	"fmt"
	"strings"
	"time"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
)

// renderSourcesBlock formats AssistantResponse.Sources as the
// trailing numbered list per spec.md §14.B.1:
//
//	sources:
//	  1. <id-short> — <title> (<captured-date>)
//	  2. <provider-name> — <title> (<retrieved-rfc3339>)
//	  … +<N> more
//
// Where:
//   - <id-short> is the first 8 hex characters of an artifact UUID.
//   - <captured-date> is the artifact's CapturedAt rendered as YYYY-MM-DD UTC.
//   - <provider-name> is the external provider's canonical name.
//   - <retrieved-rfc3339> is RFC3339 UTC.
//
// Mixed-source ordering follows the capability layer's emit order
// (most-relevant first) and is never re-sorted by the renderer.
// When overflowCount > 0, the trailing "… +N more" indicator is
// rendered exactly as shown.
//
// Returns the empty string when sources is nil/empty AND
// overflowCount == 0.
func renderSourcesBlock(sources []contracts.Source, overflowCount int, mode MarkdownMode) string {
	if len(sources) == 0 && overflowCount == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(escapeForMode("sources:", mode))
	for i, src := range sources {
		line := renderSourceLine(i+1, src)
		b.WriteString("\n")
		b.WriteString(escapeForMode(line, mode))
	}
	if overflowCount > 0 {
		b.WriteString("\n")
		b.WriteString(escapeForMode(fmt.Sprintf("  … +%d more", overflowCount), mode))
	}
	return b.String()
}

// renderSourceLine formats a single source row. Indentation is two
// spaces per spec.md §14.B.1.
func renderSourceLine(num int, src contracts.Source) string {
	switch src.Kind {
	case contracts.SourceArtifact:
		artifactRef, _ := src.Ref.(contracts.ArtifactRef)
		idShort := artifactIDShort(artifactRef.ArtifactID, src.ID)
		capturedDate := artifactRef.CapturedAt.UTC().Format("2006-01-02")
		title := strings.TrimSpace(src.Title)
		if title == "" {
			title = "(untitled)"
		}
		return fmt.Sprintf("  %d. %s — %s (%s)", num, idShort, title, capturedDate)
	case contracts.SourceExternalProvider:
		providerRef, _ := src.Ref.(contracts.ExternalProviderRef)
		providerName := strings.TrimSpace(providerRef.ProviderName)
		if providerName == "" {
			providerName = strings.TrimSpace(src.ID)
		}
		if providerName == "" {
			providerName = "external"
		}
		retrieved := providerRef.RetrievedAt.UTC().Format(time.RFC3339)
		title := strings.TrimSpace(src.Title)
		if title == "" {
			title = providerName
		}
		return fmt.Sprintf("  %d. %s — %s (%s)", num, providerName, title, retrieved)
	default:
		// Unknown SourceKind — degrade to "<id> — <title>" without
		// claiming a date we cannot prove. The capability layer's
		// closed-vocabulary contract makes this branch unreachable
		// in production; the renderer still refuses to fabricate.
		title := strings.TrimSpace(src.Title)
		return fmt.Sprintf("  %d. %s — %s", num, src.ID, title)
	}
}

// artifactIDShort returns the first 8 lowercase hex characters of an
// artifact UUID per spec.md §14.B.1. When the ID is not a recognizable
// hex string, the function returns the raw ID truncated to 8 runes.
//
// Falls back to the source-level ID when artifactID is empty (some
// SourceArtifact instances populate Source.ID instead of
// ArtifactRef.ArtifactID; both are accepted).
func artifactIDShort(artifactID, fallback string) string {
	id := strings.TrimSpace(artifactID)
	if id == "" {
		id = strings.TrimSpace(fallback)
	}
	if id == "" {
		return "(no-id)"
	}
	// Strip dashes for UUID-like IDs to keep the prefix dense.
	dense := strings.ReplaceAll(id, "-", "")
	runes := []rune(dense)
	if len(runes) >= 8 {
		return strings.ToLower(string(runes[:8]))
	}
	return strings.ToLower(string(runes))
}
