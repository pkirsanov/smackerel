package scan

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/drive"
	drivehealth "github.com/smackerel/smackerel/internal/drive/health"
	driveobs "github.com/smackerel/smackerel/internal/drive/observability"
)

// Connection is the provider-neutral connection snapshot a scan/monitor run
// needs from durable storage.
type Connection struct {
	ID         string
	ProviderID string
	Scope      drive.Scope
}

// Result summarizes a scan or monitor run.
type Result struct {
	SeenCount       int64
	IndexedCount    int64
	SkippedCount    int64
	UpsertedCount   int64
	MovedCount      int64
	TombstonedCount int64
}

// FileRecord is the durable provider-file read model.
type FileRecord struct {
	ArtifactID         string
	ProviderFileID     string
	ProviderRevisionID string
	Title              string
	MimeType           string
	SizeBytes          int64
	FolderPath         []string
	OwnerLabel         string
	ProviderURL        string
	VersionChain       []string
	SharingState       map[string]any
	Tombstoned         bool
	PermissionLost     bool
	ProviderModifiedAt time.Time
}

// Store is the persistence boundary used by scan and monitor services.
type Store interface {
	LoadConnection(ctx context.Context, connectionID string) (Connection, error)
	StartJob(ctx context.Context, connectionID string, phase string) (string, error)
	UpdateJob(ctx context.Context, jobID string, result Result) error
	CompleteJob(ctx context.Context, jobID string, result Result) error
	FailJob(ctx context.Context, jobID string, err error) error
	UpsertFile(ctx context.Context, conn Connection, item drive.FolderItem) (FileRecord, error)
	MarkRemoved(ctx context.Context, connectionID string, providerFileID string, kind drive.ChangeKind) error
	LoadCursor(ctx context.Context, connectionID string) (string, error)
	UpsertCursor(ctx context.Context, connectionID string, cursor string) error
	MarkRescanStarted(ctx context.Context, connectionID string) error
	MarkRescanCompleted(ctx context.Context, connectionID string) error
	RecordProviderError(ctx context.Context, connectionID string, workType string, err error) error
	RecordProviderSuccess(ctx context.Context, connectionID string) error
}

// Service walks provider pages and persists drive file metadata without
// triggering extraction/classification.
type Service struct {
	provider drive.Provider
	store    Store
}

// NewService returns a scan service.
func NewService(provider drive.Provider, store Store) *Service {
	return &Service{provider: provider, store: store}
}

// InitialScan walks every in-scope folder, creates/updates one artifact and
// one drive_files row per provider file, and records progress in
// drive_scan_jobs.
func (service *Service) InitialScan(ctx context.Context, connectionID string) (Result, error) {
	if service.provider == nil {
		return Result{}, fmt.Errorf("drive scan: provider is nil")
	}
	if service.store == nil {
		return Result{}, fmt.Errorf("drive scan: store is nil")
	}
	conn, err := service.store.LoadConnection(ctx, connectionID)
	if err != nil {
		return Result{}, err
	}
	jobID, err := service.store.StartJob(ctx, connectionID, "scan")
	if err != nil {
		return Result{}, err
	}

	result := Result{}
	folderIDs := conn.Scope.FolderIDs
	if len(folderIDs) == 0 {
		folderIDs = []string{"root"}
	}
	for _, folderID := range folderIDs {
		pageToken := ""
		for {
			items, nextPageToken, listErr := service.provider.ListFolder(ctx, connectionID, folderID, pageToken)
			if listErr != nil {
				driveobs.DriveProviderErrors.WithLabelValues(conn.ProviderID, "scan").Inc()
				slog.Error("drive scan: list folder failed",
					"provider", conn.ProviderID,
					"connection_id", connectionID,
					"folder_id", folderID,
					"error", listErr,
				)
				_ = service.store.RecordProviderError(ctx, connectionID, "scan", listErr)
				_ = service.store.FailJob(ctx, jobID, listErr)
				return result, listErr
			}
			for _, item := range items {
				result.SeenCount = result.SeenCount + 1
				if item.IsFolder {
					continue
				}
				if _, upsertErr := service.store.UpsertFile(ctx, conn, item); upsertErr != nil {
					driveobs.DriveScanFiles.WithLabelValues(conn.ProviderID, string(driveobs.OutcomeError)).Inc()
					slog.Error("drive scan: upsert file failed",
						"provider", conn.ProviderID,
						"connection_id", connectionID,
						"provider_file_id", item.ProviderFileID,
						"error", upsertErr,
					)
					_ = service.store.FailJob(ctx, jobID, upsertErr)
					return result, upsertErr
				}
				result.IndexedCount = result.IndexedCount + 1
				result.UpsertedCount = result.UpsertedCount + 1
				driveobs.DriveScanFiles.WithLabelValues(conn.ProviderID, string(driveobs.OutcomeOK)).Inc()
			}
			if updateErr := service.store.UpdateJob(ctx, jobID, result); updateErr != nil {
				return result, updateErr
			}
			if nextPageToken == "" {
				break
			}
			pageToken = nextPageToken
		}
	}
	if err := service.store.RecordProviderSuccess(ctx, connectionID); err != nil {
		return result, err
	}
	if err := service.store.CompleteJob(ctx, jobID, result); err != nil {
		return result, err
	}
	slog.Info("drive scan: completed",
		"provider", conn.ProviderID,
		"connection_id", connectionID,
		"seen", result.SeenCount,
		"indexed", result.IndexedCount,
		"skipped", result.SkippedCount,
	)
	return result, nil
}

// PostgresStore persists scan state to the live Smackerel database.
type PostgresStore struct {
	pool           *pgxpool.Pool
	healthRecorder *drivehealth.PostgresRecorder
}

// NewPostgresStore returns a durable scan store.
func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool, healthRecorder: drivehealth.NewPostgresRecorder(pool, drivehealth.DefaultPolicy)}
}

func (store *PostgresStore) LoadConnection(ctx context.Context, connectionID string) (Connection, error) {
	var providerID string
	var scopeJSON []byte
	if err := store.pool.QueryRow(ctx, `SELECT provider_id, scope FROM drive_connections WHERE id=$1`, connectionID).Scan(&providerID, &scopeJSON); err != nil {
		return Connection{}, fmt.Errorf("drive scan: load connection: %w", err)
	}
	var scopePayload struct {
		FolderIDs     []string `json:"folder_ids"`
		IncludeShared bool     `json:"include_shared"`
	}
	if len(scopeJSON) > 0 {
		if err := json.Unmarshal(scopeJSON, &scopePayload); err != nil {
			return Connection{}, fmt.Errorf("drive scan: decode scope: %w", err)
		}
	}
	return Connection{ID: connectionID, ProviderID: providerID, Scope: drive.Scope{FolderIDs: scopePayload.FolderIDs, IncludeShared: scopePayload.IncludeShared}}, nil
}

func (store *PostgresStore) StartJob(ctx context.Context, connectionID string, phase string) (string, error) {
	jobID := uuid.NewString()
	if _, err := store.pool.Exec(ctx,
		`INSERT INTO drive_scan_jobs (id, connection_id, phase, status) VALUES ($1, $2, $3, 'running')`,
		jobID, connectionID, phase,
	); err != nil {
		return "", fmt.Errorf("drive scan: start job: %w", err)
	}
	return jobID, nil
}

func (store *PostgresStore) UpdateJob(ctx context.Context, jobID string, result Result) error {
	_, err := store.pool.Exec(ctx,
		`UPDATE drive_scan_jobs
		 SET total_seen=$2, indexed_count=$3, skipped_count=$4, upserted_count=$5,
		     moved_count=$6, tombstoned_count=$7, updated_at=now()
		 WHERE id=$1`,
		jobID, result.SeenCount, result.IndexedCount, result.SkippedCount, result.UpsertedCount, result.MovedCount, result.TombstonedCount,
	)
	if err != nil {
		return fmt.Errorf("drive scan: update job: %w", err)
	}
	return nil
}

func (store *PostgresStore) CompleteJob(ctx context.Context, jobID string, result Result) error {
	_, err := store.pool.Exec(ctx,
		`UPDATE drive_scan_jobs
		 SET status='complete', total_seen=$2, indexed_count=$3, skipped_count=$4,
		     upserted_count=$5, moved_count=$6, tombstoned_count=$7,
		     completed_at=now(), updated_at=now()
		 WHERE id=$1`,
		jobID, result.SeenCount, result.IndexedCount, result.SkippedCount, result.UpsertedCount, result.MovedCount, result.TombstonedCount,
	)
	if err != nil {
		return fmt.Errorf("drive scan: complete job: %w", err)
	}
	return nil
}

func (store *PostgresStore) FailJob(ctx context.Context, jobID string, jobErr error) error {
	_, err := store.pool.Exec(ctx,
		`UPDATE drive_scan_jobs SET status='failed', last_error=$2, completed_at=now(), updated_at=now() WHERE id=$1`,
		jobID, errString(jobErr),
	)
	if err != nil {
		return fmt.Errorf("drive scan: fail job: %w", err)
	}
	return nil
}

func (store *PostgresStore) UpsertFile(ctx context.Context, conn Connection, item drive.FolderItem) (FileRecord, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt = attempt + 1 {
		record, err := store.upsertFileOnce(ctx, conn, item)
		if err == nil {
			return record, nil
		}
		lastErr = err
		if !isRetryablePostgresConflict(err) {
			return FileRecord{}, err
		}
		select {
		case <-ctx.Done():
			return FileRecord{}, ctx.Err()
		case <-time.After(time.Duration(attempt+1) * 25 * time.Millisecond):
		}
	}
	return FileRecord{}, lastErr
}

func (store *PostgresStore) upsertFileOnce(ctx context.Context, conn Connection, item drive.FolderItem) (FileRecord, error) {
	if item.ProviderFileID == "" {
		return FileRecord{}, fmt.Errorf("drive scan: provider file id is empty")
	}
	artifactID := artifactID(conn.ProviderID, conn.ID, item.ProviderFileID)
	sharing := sharingState(item)
	metadata := map[string]any{
		"provider_id":          conn.ProviderID,
		"provider_file_id":     item.ProviderFileID,
		"provider_revision_id": item.ProviderRevisionID,
		"folder_path":          item.FolderPath,
		"owner_label":          item.OwnerLabel,
		"provider_url":         item.ProviderURL,
		"mime_type":            item.MimeType,
		"size_bytes":           item.SizeBytes,
		"sharing_state":        sharing,
	}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return FileRecord{}, fmt.Errorf("drive scan: marshal metadata: %w", err)
	}
	contentHash := contentHashFor(conn.ID, item.ProviderFileID, item.ProviderRevisionID)
	versionChain, err := store.versionChain(ctx, conn.ID, item.ProviderFileID, item.ProviderRevisionID)
	if err != nil {
		return FileRecord{}, err
	}
	versionJSON, err := json.Marshal(versionChain)
	if err != nil {
		return FileRecord{}, fmt.Errorf("drive scan: marshal version chain: %w", err)
	}
	sharingJSON, err := json.Marshal(sharing)
	if err != nil {
		return FileRecord{}, fmt.Errorf("drive scan: marshal sharing state: %w", err)
	}
	providerLabelsJSON := []byte(`{}`)

	rowID := uuid.NewString()
	_, err = store.pool.Exec(ctx,
		`WITH artifact_upsert AS (
		 INSERT INTO artifacts
		  (id, artifact_type, title, summary, content_raw, content_hash, source_id,
		   source_ref, source_url, source_quality, metadata, processing_status)
		 VALUES ($1, 'drive_file', $2, '', '', $3, $4, $5, $6, 'primary', $7, 'pending')
		 ON CONFLICT (id) DO UPDATE SET
		   title=EXCLUDED.title,
		   content_hash=EXCLUDED.content_hash,
		   source_url=EXCLUDED.source_url,
		   metadata=EXCLUDED.metadata,
		   updated_at=now()
		 RETURNING id
		)
		 INSERT INTO drive_files
		  (id, artifact_id, connection_id, provider_file_id, provider_revision_id,
		   provider_url, title, mime_type, size_bytes, folder_path, provider_labels,
		   owner_label, last_modified_by, sharing_state, sensitivity, extraction_state,
		   version_chain)
		 SELECT $8, artifact_upsert.id, $9, $10, $11, $12, $13, $14, $15, $16,
		        $17, $18, $19, $20, 'none', 'pending', $21
		   FROM artifact_upsert
		 ON CONFLICT (connection_id, provider_file_id) DO UPDATE SET
		  artifact_id=EXCLUDED.artifact_id,
		  provider_revision_id=EXCLUDED.provider_revision_id,
		  provider_url=EXCLUDED.provider_url,
		  title=EXCLUDED.title,
		  mime_type=EXCLUDED.mime_type,
		  size_bytes=EXCLUDED.size_bytes,
		  folder_path=EXCLUDED.folder_path,
		  provider_labels=EXCLUDED.provider_labels,
		  owner_label=EXCLUDED.owner_label,
		  last_modified_by=EXCLUDED.last_modified_by,
		  sharing_state=EXCLUDED.sharing_state,
		  version_chain=EXCLUDED.version_chain,
		  tombstoned_at=NULL,
		  permission_lost_at=NULL,
		  updated_at=now()`,
		artifactID, item.Title, contentHash, "drive:"+conn.ProviderID, item.ProviderFileID, item.ProviderURL, metadataJSON,
		rowID, conn.ID, item.ProviderFileID, item.ProviderRevisionID,
		item.ProviderURL, item.Title, item.MimeType, item.SizeBytes, item.FolderPath,
		providerLabelsJSON, item.OwnerLabel, item.LastModifiedBy, sharingJSON, versionJSON,
	)
	if err != nil {
		return FileRecord{}, fmt.Errorf("drive scan: upsert drive_files: %w", err)
	}
	return FileRecord{ArtifactID: artifactID, ProviderFileID: item.ProviderFileID, ProviderRevisionID: item.ProviderRevisionID, Title: item.Title, MimeType: item.MimeType, SizeBytes: item.SizeBytes, FolderPath: item.FolderPath, OwnerLabel: item.OwnerLabel, ProviderURL: item.ProviderURL, VersionChain: versionChain, SharingState: sharing, ProviderModifiedAt: item.ModifiedAt}, nil
}

func isRetryablePostgresConflict(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "40P01" || pgErr.Code == "40001"
	}
	return false
}

func (store *PostgresStore) versionChain(ctx context.Context, connectionID string, providerFileID string, revisionID string) ([]string, error) {
	var raw []byte
	err := store.pool.QueryRow(ctx, `SELECT version_chain FROM drive_files WHERE connection_id=$1 AND provider_file_id=$2`, connectionID, providerFileID).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		if revisionID == "" {
			return []string{}, nil
		}
		return []string{revisionID}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("drive scan: read version chain: %w", err)
	}
	var chain []string
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &chain); err != nil {
			return nil, fmt.Errorf("drive scan: decode version chain: %w", err)
		}
	}
	if revisionID != "" && !containsString(chain, revisionID) {
		chain = append(chain, revisionID)
	}
	return chain, nil
}

func (store *PostgresStore) MarkRemoved(ctx context.Context, connectionID string, providerFileID string, kind drive.ChangeKind) error {
	column := "tombstoned_at"
	if kind == drive.ChangePermLost {
		column = "permission_lost_at"
	}
	_, err := store.pool.Exec(ctx,
		fmt.Sprintf(`UPDATE drive_files SET %s=now(), updated_at=now() WHERE connection_id=$1 AND provider_file_id=$2`, column),
		connectionID, providerFileID,
	)
	if err != nil {
		return fmt.Errorf("drive scan: mark removed: %w", err)
	}
	return nil
}

func (store *PostgresStore) LoadCursor(ctx context.Context, connectionID string) (string, error) {
	var cursor string
	err := store.pool.QueryRow(ctx, `SELECT cursor FROM drive_cursors WHERE connection_id=$1`, connectionID).Scan(&cursor)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("drive scan: load cursor: %w", err)
	}
	return cursor, nil
}

func (store *PostgresStore) UpsertCursor(ctx context.Context, connectionID string, cursor string) error {
	_, err := store.pool.Exec(ctx,
		`INSERT INTO drive_cursors (connection_id, cursor, updated_at)
		 VALUES ($1, $2, now())
		 ON CONFLICT (connection_id) DO UPDATE SET cursor=EXCLUDED.cursor, updated_at=now()`,
		connectionID, cursor,
	)
	if err != nil {
		return fmt.Errorf("drive scan: upsert cursor: %w", err)
	}
	return nil
}

func (store *PostgresStore) MarkRescanStarted(ctx context.Context, connectionID string) error {
	_, err := store.pool.Exec(ctx,
		`INSERT INTO drive_cursors (connection_id, cursor, last_rescan_started_at, updated_at)
		 VALUES ($1, '', now(), now())
		 ON CONFLICT (connection_id) DO UPDATE SET last_rescan_started_at=now(), updated_at=now()`, connectionID,
	)
	return err
}

func (store *PostgresStore) MarkRescanCompleted(ctx context.Context, connectionID string) error {
	_, err := store.pool.Exec(ctx, `UPDATE drive_cursors SET last_rescan_completed_at=now(), updated_at=now() WHERE connection_id=$1`, connectionID)
	return err
}

func (store *PostgresStore) RecordProviderError(ctx context.Context, connectionID string, workType string, err error) error {
	_, recordErr := store.healthRecorder.RecordProviderError(ctx, connectionID, workType, err)
	return recordErr
}

func (store *PostgresStore) RecordProviderSuccess(ctx context.Context, connectionID string) error {
	_, recordErr := store.healthRecorder.RecordProviderSuccess(ctx, connectionID)
	return recordErr
}

type memoryStore struct {
	mu          sync.Mutex
	connection  Connection
	files       map[string]FileRecord
	artifactIDs map[string]bool
	cursor      string
	jobs        map[string]Result
}

type memorySnapshot struct {
	Files               map[string]FileRecord
	ArtifactInsertCount int
}

func newMemoryStore(conn Connection) *memoryStore {
	return &memoryStore{connection: conn, files: map[string]FileRecord{}, artifactIDs: map[string]bool{}, jobs: map[string]Result{}}
}

func (store *memoryStore) LoadConnection(context.Context, string) (Connection, error) {
	return store.connection, nil
}

func (store *memoryStore) StartJob(context.Context, string, string) (string, error) {
	return uuid.NewString(), nil
}

func (store *memoryStore) UpdateJob(_ context.Context, jobID string, result Result) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.jobs[jobID] = result
	return nil
}

func (store *memoryStore) CompleteJob(ctx context.Context, jobID string, result Result) error {
	return store.UpdateJob(ctx, jobID, result)
}

func (store *memoryStore) FailJob(context.Context, string, error) error { return nil }

func (store *memoryStore) UpsertFile(_ context.Context, conn Connection, item drive.FolderItem) (FileRecord, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	artifact := artifactID(conn.ProviderID, conn.ID, item.ProviderFileID)
	chain := []string{}
	if existing, ok := store.files[item.ProviderFileID]; ok {
		chain = append(chain, existing.VersionChain...)
	}
	if item.ProviderRevisionID != "" && !containsString(chain, item.ProviderRevisionID) {
		chain = append(chain, item.ProviderRevisionID)
	}
	if !store.artifactIDs[artifact] {
		store.artifactIDs[artifact] = true
	}
	record := FileRecord{ArtifactID: artifact, ProviderFileID: item.ProviderFileID, ProviderRevisionID: item.ProviderRevisionID, Title: item.Title, MimeType: item.MimeType, SizeBytes: item.SizeBytes, FolderPath: item.FolderPath, OwnerLabel: item.OwnerLabel, ProviderURL: item.ProviderURL, VersionChain: chain, SharingState: sharingState(item), ProviderModifiedAt: item.ModifiedAt}
	store.files[item.ProviderFileID] = record
	return record, nil
}

func (store *memoryStore) MarkRemoved(_ context.Context, _ string, providerFileID string, kind drive.ChangeKind) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	record := store.files[providerFileID]
	if kind == drive.ChangePermLost {
		record.PermissionLost = true
	} else {
		record.Tombstoned = true
	}
	store.files[providerFileID] = record
	return nil
}

func (store *memoryStore) LoadCursor(context.Context, string) (string, error) {
	return store.cursor, nil
}

func (store *memoryStore) UpsertCursor(_ context.Context, _ string, cursor string) error {
	store.cursor = cursor
	return nil
}

func (store *memoryStore) MarkRescanStarted(context.Context, string) error { return nil }

func (store *memoryStore) MarkRescanCompleted(context.Context, string) error { return nil }

func (store *memoryStore) RecordProviderError(context.Context, string, string, error) error {
	return nil
}

func (store *memoryStore) RecordProviderSuccess(context.Context, string) error { return nil }

func (store *memoryStore) snapshot(string) memorySnapshot {
	store.mu.Lock()
	defer store.mu.Unlock()
	files := make(map[string]FileRecord, len(store.files))
	for key, value := range store.files {
		files[key] = value
	}
	return memorySnapshot{Files: files, ArtifactInsertCount: len(store.artifactIDs)}
}

func artifactID(providerID string, connectionID string, providerFileID string) string {
	return "drive:" + providerID + ":" + connectionID + ":" + providerFileID
}

func contentHashFor(connectionID string, providerFileID string, revisionID string) string {
	digest := sha256.Sum256([]byte(connectionID + "|" + providerFileID + "|" + revisionID))
	return hex.EncodeToString(digest[:])
}

func sharingState(item drive.FolderItem) map[string]any {
	if item.SharingState != nil {
		return item.SharingState
	}
	return map[string]any{"shared": false}
}

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return strings.TrimSpace(err.Error())
}
