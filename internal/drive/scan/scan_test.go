package scan

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/drive"
)

type scanTestProvider struct {
	pages map[string]folderPage
	calls []listCall
}

type folderPage struct {
	items         []drive.FolderItem
	nextPageToken string
}

type listCall struct {
	folderID  string
	pageToken string
}

func (provider *scanTestProvider) ID() string { return "google" }

func (provider *scanTestProvider) DisplayName() string { return "Google Drive" }

func (provider *scanTestProvider) Capabilities() drive.Capabilities {
	return drive.Capabilities{SupportsVersions: true, SupportsSharing: true, SupportsChangeHistory: true, MaxFileSizeBytes: 104857600}
}

func (provider *scanTestProvider) BeginConnect(context.Context, drive.AccessMode, drive.Scope) (string, string, error) {
	return "", "", drive.ErrNotImplemented
}

func (provider *scanTestProvider) FinalizeConnect(context.Context, string, string) (string, error) {
	return "", drive.ErrNotImplemented
}

func (provider *scanTestProvider) Disconnect(context.Context, string) error {
	return drive.ErrNotImplemented
}

func (provider *scanTestProvider) Scope(context.Context, string) (drive.Scope, error) {
	return drive.Scope{}, drive.ErrNotImplemented
}

func (provider *scanTestProvider) SetScope(context.Context, string, drive.Scope) error {
	return drive.ErrNotImplemented
}

func (provider *scanTestProvider) ListFolder(_ context.Context, _ string, folderID string, pageToken string) ([]drive.FolderItem, string, error) {
	provider.calls = append(provider.calls, listCall{folderID: folderID, pageToken: pageToken})
	page, ok := provider.pages[folderID+"|"+pageToken]
	if !ok {
		return nil, "", errors.New("unexpected page request")
	}
	return page.items, page.nextPageToken, nil
}

func (provider *scanTestProvider) GetFile(context.Context, string, string) (drive.FileBytes, error) {
	return drive.FileBytes{}, drive.ErrNotImplemented
}

func (provider *scanTestProvider) PutFile(context.Context, string, string, string, drive.FileBytes) (string, error) {
	return "", drive.ErrNotImplemented
}

func (provider *scanTestProvider) Changes(context.Context, string, string) ([]drive.Change, string, error) {
	return nil, "", drive.ErrNotImplemented
}

func (provider *scanTestProvider) Health(context.Context, string) (drive.Health, error) {
	return drive.Health{Status: drive.HealthHealthy, ObservedAt: time.Now()}, nil
}

func TestBulkScanPersistsDriveFilesWithArtifactLinks(t *testing.T) {
	provider := &scanTestProvider{pages: map[string]folderPage{
		"root|": {
			items: []drive.FolderItem{
				{
					ProviderFileID:     "file-001",
					ProviderRevisionID: "rev-001",
					Title:              "Air fryer manual.pdf",
					MimeType:           "application/pdf",
					SizeBytes:          12000,
					FolderPath:         []string{"Cooking", "Manuals"},
					OwnerLabel:         "owner@example.com",
					ProviderURL:        "https://drive.example/file-001",
					ModifiedAt:         time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC),
				},
			},
			nextPageToken: "page-2",
		},
		"root|page-2": {
			items: []drive.FolderItem{
				{
					ProviderFileID:     "file-002",
					ProviderRevisionID: "rev-002",
					Title:              "Receipt.jpg",
					MimeType:           "image/jpeg",
					SizeBytes:          8000,
					FolderPath:         []string{"Receipts", "2026"},
					OwnerLabel:         "owner@example.com",
					ProviderURL:        "https://drive.example/file-002",
					ModifiedAt:         time.Date(2026, 4, 2, 10, 0, 0, 0, time.UTC),
				},
				{
					ProviderFileID:     "folder-skip",
					ProviderRevisionID: "folder-rev",
					Title:              "Folder marker",
					MimeType:           "application/vnd.google-apps.folder",
					FolderPath:         []string{"Receipts"},
					IsFolder:           true,
					OwnerLabel:         "owner@example.com",
					ProviderURL:        "https://drive.example/folder-skip",
				},
			},
		},
	}}
	store := newMemoryStore(Connection{
		ID:         "conn-scan-unit",
		ProviderID: "google",
		Scope:      drive.Scope{FolderIDs: []string{"root"}},
	})
	service := NewService(provider, store)

	result, err := service.InitialScan(context.Background(), "conn-scan-unit")
	if err != nil {
		t.Fatalf("InitialScan: %v", err)
	}
	if result.IndexedCount != 2 {
		t.Fatalf("IndexedCount = %d, want 2 non-folder files", result.IndexedCount)
	}
	if result.SeenCount != 3 {
		t.Fatalf("SeenCount = %d, want 3 provider entries including folder marker", result.SeenCount)
	}
	if len(provider.calls) != 2 {
		t.Fatalf("ListFolder calls = %d, want 2 pages", len(provider.calls))
	}
	if provider.calls[0] != (listCall{folderID: "root", pageToken: ""}) || provider.calls[1] != (listCall{folderID: "root", pageToken: "page-2"}) {
		t.Fatalf("ListFolder calls = %+v, want root first page then page-2", provider.calls)
	}

	snapshot := store.snapshot("conn-scan-unit")
	if len(snapshot.Files) != 2 {
		t.Fatalf("stored files = %d, want 2", len(snapshot.Files))
	}
	manual := snapshot.Files["file-001"]
	if manual.ArtifactID == "" {
		t.Fatalf("file-001 ArtifactID is empty")
	}
	if manual.Title != "Air fryer manual.pdf" || manual.MimeType != "application/pdf" {
		t.Fatalf("file-001 metadata = %+v", manual)
	}
	if len(manual.FolderPath) != 2 || manual.FolderPath[0] != "Cooking" || manual.FolderPath[1] != "Manuals" {
		t.Fatalf("file-001 folder path = %v, want [Cooking Manuals]", manual.FolderPath)
	}
	if manual.ProviderURL == "" || manual.OwnerLabel == "" || manual.VersionChain[0] != "rev-001" {
		t.Fatalf("file-001 provider metadata incomplete: %+v", manual)
	}

	secondResult, err := service.InitialScan(context.Background(), "conn-scan-unit")
	if err != nil {
		t.Fatalf("second InitialScan: %v", err)
	}
	if secondResult.IndexedCount != 2 {
		t.Fatalf("second IndexedCount = %d, want 2", secondResult.IndexedCount)
	}
	secondSnapshot := store.snapshot("conn-scan-unit")
	if len(secondSnapshot.Files) != 2 {
		t.Fatalf("second scan stored files = %d, want 2 without duplicates", len(secondSnapshot.Files))
	}
	if secondSnapshot.ArtifactInsertCount != 2 {
		t.Fatalf("artifact inserts = %d, want exactly 2 after duplicate scan", secondSnapshot.ArtifactInsertCount)
	}
}
