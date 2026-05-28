package assistant_adapter

import (
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
)

// TestRenderSourcesBlock_Artifact asserts the artifact source line
// shape per spec.md §14.B.1:
//
//  1. <8-hex> — <title> (<YYYY-MM-DD>)
func TestRenderSourcesBlock_Artifact(t *testing.T) {
	t.Parallel()
	captured := time.Date(2025, 3, 14, 9, 26, 53, 0, time.UTC)
	sources := []contracts.Source{
		{
			ID:    "ignored-source-id",
			Title: "Pad Thai recipe notes",
			Kind:  contracts.SourceArtifact,
			Ref: contracts.ArtifactRef{
				ArtifactID: "abcdef0123456789-fffeeeddd",
				CapturedAt: captured,
			},
		},
	}
	got := renderSourcesBlock(sources, 0, PlainText)
	want := "sources:\n  1. abcdef01 — Pad Thai recipe notes (2025-03-14)"
	if got != want {
		t.Errorf("renderSourcesBlock(artifact)\n got: %q\nwant: %q", got, want)
	}
}

// TestRenderSourcesBlock_ExternalProvider asserts the external-provider
// line shape with RFC3339 retrieved-at timestamps.
func TestRenderSourcesBlock_ExternalProvider(t *testing.T) {
	t.Parallel()
	retrieved := time.Date(2025, 3, 14, 9, 26, 53, 0, time.UTC)
	sources := []contracts.Source{
		{
			ID:    "open-meteo-london-20250314",
			Title: "London 7-day forecast",
			Kind:  contracts.SourceExternalProvider,
			Ref: contracts.ExternalProviderRef{
				ProviderName: "open-meteo",
				RetrievedAt:  retrieved,
			},
		},
	}
	got := renderSourcesBlock(sources, 0, PlainText)
	want := "sources:\n  1. open-meteo — London 7-day forecast (2025-03-14T09:26:53Z)"
	if got != want {
		t.Errorf("renderSourcesBlock(external)\n got: %q\nwant: %q", got, want)
	}
}

// TestRenderSourcesBlock_Mixed asserts artifact + external sources
// render in their slice order (capability-layer authoritative
// ordering) without re-sorting.
func TestRenderSourcesBlock_Mixed(t *testing.T) {
	t.Parallel()
	sources := []contracts.Source{
		{
			ID:    "a",
			Title: "weather forecast",
			Kind:  contracts.SourceExternalProvider,
			Ref: contracts.ExternalProviderRef{
				ProviderName: "open-meteo",
				RetrievedAt:  time.Date(2025, 3, 14, 9, 26, 53, 0, time.UTC),
			},
		},
		{
			ID:    "b",
			Title: "previously-saved London tips",
			Kind:  contracts.SourceArtifact,
			Ref: contracts.ArtifactRef{
				ArtifactID: "11223344-5566-7788-99aa-bbccddeeff00",
				CapturedAt: time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC),
			},
		},
	}
	got := renderSourcesBlock(sources, 0, PlainText)
	lines := strings.Split(got, "\n")
	if len(lines) != 3 {
		t.Fatalf("got %d lines; want 3 (header + 2 sources)", len(lines))
	}
	if !strings.HasPrefix(lines[1], "  1. open-meteo — ") {
		t.Errorf("line 1 = %q; want external-first ordering", lines[1])
	}
	if !strings.HasPrefix(lines[2], "  2. 11223344 — ") {
		t.Errorf("line 2 = %q; want 8-hex artifact ID dense (no dashes)", lines[2])
	}
}

// TestRenderSourcesBlock_OverflowIndicator asserts the trailing
// "… +N more" indicator format.
func TestRenderSourcesBlock_OverflowIndicator(t *testing.T) {
	t.Parallel()
	sources := []contracts.Source{
		{
			ID:    "a",
			Title: "first",
			Kind:  contracts.SourceArtifact,
			Ref: contracts.ArtifactRef{
				ArtifactID: "aaaaaaaa00000000",
				CapturedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			},
		},
	}
	got := renderSourcesBlock(sources, 4, PlainText)
	if !strings.HasSuffix(got, "  … +4 more") {
		t.Errorf("got = %q; want trailing overflow indicator", got)
	}
}

// TestRenderSourcesBlock_Empty asserts the function returns "" when
// there are no sources AND no overflow (so the renderer can skip
// the separator).
func TestRenderSourcesBlock_Empty(t *testing.T) {
	t.Parallel()
	got := renderSourcesBlock(nil, 0, PlainText)
	if got != "" {
		t.Errorf("got = %q; want empty string", got)
	}
}

// TestArtifactIDShortFallback asserts the artifact ID falls back to
// Source.ID when ArtifactRef.ArtifactID is empty (some capability
// callers populate Source.ID instead).
func TestArtifactIDShortFallback(t *testing.T) {
	t.Parallel()
	got := artifactIDShort("", "deadbeefcafef00d")
	if got != "deadbeef" {
		t.Errorf("got = %q; want deadbeef", got)
	}
}
