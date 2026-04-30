package monitor

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/smackerel/smackerel/internal/drive"
	"github.com/smackerel/smackerel/internal/drive/scan"
)

type monitorTestProvider struct {
	changes     []drive.Change
	nextCursor  string
	folderItems []drive.FolderItem
	listCalls   int
}

func (provider *monitorTestProvider) ID() string { return "google" }

func (provider *monitorTestProvider) DisplayName() string { return "Google Drive" }

func (provider *monitorTestProvider) Capabilities() drive.Capabilities {
	return drive.Capabilities{SupportsVersions: true, SupportsSharing: true, SupportsChangeHistory: true}
}

func (provider *monitorTestProvider) BeginConnect(context.Context, drive.AccessMode, drive.Scope) (string, string, error) {
	return "", "", drive.ErrNotImplemented
}

func (provider *monitorTestProvider) FinalizeConnect(context.Context, string, string) (string, error) {
	return "", drive.ErrNotImplemented
}

func (provider *monitorTestProvider) Disconnect(context.Context, string) error {
	return drive.ErrNotImplemented
}

func (provider *monitorTestProvider) Scope(context.Context, string) (drive.Scope, error) {
	return drive.Scope{}, drive.ErrNotImplemented
}

func (provider *monitorTestProvider) SetScope(context.Context, string, drive.Scope) error {
	return drive.ErrNotImplemented
}

func (provider *monitorTestProvider) ListFolder(context.Context, string, string, string) ([]drive.FolderItem, string, error) {
	provider.listCalls = provider.listCalls + 1
	return provider.folderItems, "", nil
}

func (provider *monitorTestProvider) GetFile(context.Context, string, string) (drive.FileBytes, error) {
	return drive.FileBytes{}, drive.ErrNotImplemented
}

func (provider *monitorTestProvider) PutFile(context.Context, string, string, string, drive.FileBytes) (string, error) {
	return "", drive.ErrNotImplemented
}

func (provider *monitorTestProvider) Changes(context.Context, string, string) ([]drive.Change, string, error) {
	return provider.changes, provider.nextCursor, nil
}

func (provider *monitorTestProvider) Health(context.Context, string) (drive.Health, error) {
	return drive.Health{Status: drive.HealthHealthy, ObservedAt: time.Now()}, nil
}

type monitorTestStore struct {
	conn             scan.Connection
	cursor           string
	files            map[string]scan.FileRecord
	artifactIDs      map[string]bool
	rescanStarted    bool
	rescanCompleted  bool
	providerSuccess  int
	providerFailures int
	jobs             map[string]scan.Result
}

func newMonitorTestStore(conn scan.Connection) *monitorTestStore {
	return &monitorTestStore{conn: conn, files: map[string]scan.FileRecord{}, artifactIDs: map[string]bool{}, jobs: map[string]scan.Result{}}
}

func (store *monitorTestStore) LoadConnection(context.Context, string) (scan.Connection, error) {
	return store.conn, nil
}

func (store *monitorTestStore) StartJob(context.Context, string, string) (string, error) {
	jobID := uuid.NewString()
	store.jobs[jobID] = scan.Result{}
	return jobID, nil
}

func (store *monitorTestStore) UpdateJob(_ context.Context, jobID string, result scan.Result) error {
	store.jobs[jobID] = result
	return nil
}

func (store *monitorTestStore) CompleteJob(ctx context.Context, jobID string, result scan.Result) error {
	return store.UpdateJob(ctx, jobID, result)
}

func (store *monitorTestStore) FailJob(context.Context, string, error) error { return nil }

func (store *monitorTestStore) UpsertFile(_ context.Context, conn scan.Connection, item drive.FolderItem) (scan.FileRecord, error) {
	artifactID := fmt.Sprintf("drive:%s:%s:%s", conn.ProviderID, conn.ID, item.ProviderFileID)
	store.artifactIDs[artifactID] = true
	versionChain := []string{}
	if existing, ok := store.files[item.ProviderFileID]; ok {
		versionChain = append(versionChain, existing.VersionChain...)
	}
	if item.ProviderRevisionID != "" && !containsMonitorTestString(versionChain, item.ProviderRevisionID) {
		versionChain = append(versionChain, item.ProviderRevisionID)
	}
	record := scan.FileRecord{
		ArtifactID:         artifactID,
		ProviderFileID:     item.ProviderFileID,
		ProviderRevisionID: item.ProviderRevisionID,
		Title:              item.Title,
		MimeType:           item.MimeType,
		FolderPath:         item.FolderPath,
		ProviderURL:        item.ProviderURL,
		VersionChain:       versionChain,
	}
	store.files[item.ProviderFileID] = record
	return record, nil
}

func (store *monitorTestStore) MarkRemoved(_ context.Context, _ string, providerFileID string, kind drive.ChangeKind) error {
	record := store.files[providerFileID]
	if kind == drive.ChangePermLost {
		record.PermissionLost = true
	} else {
		record.Tombstoned = true
	}
	store.files[providerFileID] = record
	return nil
}

func (store *monitorTestStore) LoadCursor(context.Context, string) (string, error) {
	return store.cursor, nil
}

func (store *monitorTestStore) UpsertCursor(_ context.Context, _ string, cursor string) error {
	store.cursor = cursor
	return nil
}

func (store *monitorTestStore) MarkRescanStarted(context.Context, string) error {
	store.rescanStarted = true
	return nil
}

func (store *monitorTestStore) MarkRescanCompleted(context.Context, string) error {
	store.rescanCompleted = true
	return nil
}

func (store *monitorTestStore) RecordProviderError(context.Context, string, string, error) error {
	store.providerFailures = store.providerFailures + 1
	return nil
}

func (store *monitorTestStore) RecordProviderSuccess(context.Context, string) error {
	store.providerSuccess = store.providerSuccess + 1
	return nil
}

func TestMonitorAppliesProviderDeltasWithoutDuplicateArtifacts(t *testing.T) {
	conn := scan.Connection{ID: "conn-monitor-unit", ProviderID: "google", Scope: drive.Scope{FolderIDs: []string{"root"}}}
	store := newMonitorTestStore(conn)
	_, err := store.UpsertFile(context.Background(), conn, drive.FolderItem{ProviderFileID: "modified-file", ProviderRevisionID: "rev-1", Title: "Before.txt", MimeType: "text/plain"})
	if err != nil {
		t.Fatalf("seed modified-file: %v", err)
	}
	for _, providerFileID := range []string{"trashed-file", "deleted-file", "permission-file"} {
		_, err := store.UpsertFile(context.Background(), conn, drive.FolderItem{ProviderFileID: providerFileID, ProviderRevisionID: "rev-1", Title: providerFileID, MimeType: "text/plain"})
		if err != nil {
			t.Fatalf("seed %s: %v", providerFileID, err)
		}
	}

	provider := &monitorTestProvider{
		nextCursor: "cursor-next",
		changes: []drive.Change{
			{Kind: drive.ChangeUpsert, ProviderFileID: "modified-file", Item: drive.FolderItem{ProviderFileID: "modified-file", ProviderRevisionID: "rev-2", Title: "After.txt", MimeType: "text/plain", FolderPath: []string{"Updated"}}},
			{Kind: drive.ChangeMove, ProviderFileID: "moved-file", Item: drive.FolderItem{ProviderFileID: "moved-file", ProviderRevisionID: "move-rev-1", Title: "Moved.txt", MimeType: "text/plain", FolderPath: []string{"Moved", "Archive"}}},
			{Kind: drive.ChangeTrash, ProviderFileID: "trashed-file"},
			{Kind: drive.ChangeDelete, ProviderFileID: "deleted-file"},
			{Kind: drive.ChangePermLost, ProviderFileID: "permission-file"},
			{Kind: drive.ChangeCursorInv},
		},
		folderItems: []drive.FolderItem{{ProviderFileID: "rescan-file", ProviderRevisionID: "rescan-rev-1", Title: "Rescan.txt", MimeType: "text/plain", FolderPath: []string{"Rescan"}}},
	}

	result, err := NewService(provider, store).RunOnce(context.Background(), conn.ID)
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if result.SeenCount != 6 || result.UpsertedCount != 2 || result.MovedCount != 1 || result.TombstonedCount != 3 {
		t.Fatalf("result = %+v, want seen=6 upserted=2 moved=1 tombstoned=3", result)
	}
	if store.cursor != "cursor-next" {
		t.Fatalf("cursor = %q, want cursor-next", store.cursor)
	}
	modified := store.files["modified-file"]
	if modified.ArtifactID != "drive:google:conn-monitor-unit:modified-file" || len(modified.VersionChain) != 2 || modified.VersionChain[0] != "rev-1" || modified.VersionChain[1] != "rev-2" {
		t.Fatalf("modified file record = %+v, want same artifact with rev-1/rev-2 chain", modified)
	}
	if len(store.artifactIDs) != 6 {
		t.Fatalf("artifact identity count = %d, want 6 unique provider identities without duplicate modified artifact", len(store.artifactIDs))
	}
	if got := store.files["moved-file"].FolderPath; len(got) != 2 || got[0] != "Moved" || got[1] != "Archive" {
		t.Fatalf("moved folder path = %v, want [Moved Archive]", got)
	}
	if !store.files["trashed-file"].Tombstoned || !store.files["deleted-file"].Tombstoned {
		t.Fatalf("trash/delete changes were not tombstoned: trashed=%+v deleted=%+v", store.files["trashed-file"], store.files["deleted-file"])
	}
	if !store.files["permission-file"].PermissionLost {
		t.Fatalf("permission-lost change did not mark PermissionLost: %+v", store.files["permission-file"])
	}
	if !store.rescanStarted || !store.rescanCompleted || provider.listCalls != 1 || store.files["rescan-file"].ProviderRevisionID != "rescan-rev-1" {
		t.Fatalf("cursor invalidation did not run bounded rescan: started=%t completed=%t listCalls=%d rescan=%+v", store.rescanStarted, store.rescanCompleted, provider.listCalls, store.files["rescan-file"])
	}
	if store.providerSuccess != 2 || store.providerFailures != 0 {
		t.Fatalf("provider health counters success=%d failures=%d, want success=2 failures=0", store.providerSuccess, store.providerFailures)
	}
}

func containsMonitorTestString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}
