//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	photolib "github.com/smackerel/smackerel/internal/connector/photos"
)

// TestPhotosDocumentScan_MultiPageOCRAndCleanArtifact covers
// SCN-040-011: a multi-page mobile document scan creates one document
// group and N photo rows ordered by page index. The store helper
// ListPhotosByDocumentGroup is the authoritative query used by the
// PWA + agent tools to render the cohesive document artifact.
func TestPhotosDocumentScan_MultiPageOCRAndCleanArtifact(t *testing.T) {
	pool := testPool(t)
	store := photolib.NewStore(pool)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	groupRef := "docscan-" + testID(t)
	const pages = 3
	var groupID uuid.UUID
	var firstArtifact string
	for page := 1; page <= pages; page++ {
		event := photolib.SyntheticPhotoEvent()
		event.ProviderRef = "web:doc:" + groupRef + ":p" + string(rune('0'+page))
		event.ContentHash = "sha256:doc:" + groupRef + ":p" + string(rune('0'+page))
		event.MediaRole = photolib.MediaRoleDocumentScan
		event.SourceChannel = photolib.SourceChannelWeb
		event.SourceRef = groupRef + ":session"
		event.DocumentGroupRef = groupRef
		event.DocumentPageIndex = page
		record, err := store.PublishPhotoEvent(ctx, "test-docscan", "web", event)
		if err != nil {
			t.Fatalf("publish page %d: %v", page, err)
		}
		cleanupPhoto(t, record.ArtifactID)
		if record.DocumentGroupID == nil {
			t.Fatalf("page %d missing document_group_id", page)
		}
		if record.DocumentPageIndex == nil || *record.DocumentPageIndex != page {
			t.Fatalf("page %d index mismatch: %v", page, record.DocumentPageIndex)
		}
		if page == 1 {
			groupID = *record.DocumentGroupID
			firstArtifact = record.ArtifactID
		} else if *record.DocumentGroupID != groupID {
			t.Fatalf("page %d group id drift: got %s want %s", page, *record.DocumentGroupID, groupID)
		}
	}

	rows, err := store.ListPhotosByDocumentGroup(ctx, groupID)
	if err != nil {
		t.Fatalf("list document group: %v", err)
	}
	if len(rows) != pages {
		t.Fatalf("expected %d rows for group %s, got %d (first=%s)", pages, groupID, len(rows), firstArtifact)
	}
	for i, row := range rows {
		if row.DocumentPageIndex == nil || *row.DocumentPageIndex != i+1 {
			t.Fatalf("row %d page index = %v, want %d", i, row.DocumentPageIndex, i+1)
		}
		if row.MediaRole != photolib.MediaRoleDocumentScan {
			t.Fatalf("row %d media_role = %q, want %q", i, row.MediaRole, photolib.MediaRoleDocumentScan)
		}
	}

	group, err := store.GetDocumentGroup(ctx, groupID)
	if err != nil {
		t.Fatalf("get document group: %v", err)
	}
	if group.PageCount != pages {
		t.Fatalf("group page_count = %d, want %d", group.PageCount, pages)
	}
}
