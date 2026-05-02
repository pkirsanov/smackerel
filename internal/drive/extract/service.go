// Package extract routes scanned drive files through extraction,
// folder-context summarization, and provider-neutral classification.
package extract

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log/slog"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/drive"
	driveobs "github.com/smackerel/smackerel/internal/drive/observability"
)

const defaultMaxFileSizeBytes int64 = 25 * 1024 * 1024

// FileRecord is the provider-neutral metadata needed by the extraction layer.
type FileRecord struct {
	ArtifactID         string
	ConnectionID       string
	ProviderFileID     string
	ProviderRevisionID string
	Title              string
	MimeType           string
	SizeBytes          int64
	FolderPath         []string
	ProviderURL        string
	OwnerLabel         string
	SharingState       map[string]any
	ExtractionState    string
	SkipReason         string
	ExistingContentRaw string
	ExistingMetadata   map[string]any
	ExistingDomainData map[string]any
}

// ExtractionResult is the text extraction output for one drive file.
type ExtractionResult struct {
	State             string
	SkipReason        string
	RecommendedAction string
	Text              string
	Route             string
	CharacterCount    int
}

// FolderSummary captures reusable folder context for classification.
type FolderSummary struct {
	FolderPath     string   `json:"folder_path"`
	Topic          string   `json:"topic"`
	Audience       string   `json:"audience"`
	Classification string   `json:"classification"`
	Evidence       []string `json:"evidence"`
}

// ClassificationResult is persisted into artifacts.metadata and domain_data.
type ClassificationResult struct {
	Classification   string   `json:"classification"`
	Topic            string   `json:"topic"`
	Audience         string   `json:"audience"`
	Sensitivity      string   `json:"sensitivity"`
	DriveSensitivity string   `json:"drive_sensitivity"`
	Confidence       float64  `json:"confidence"`
	Evidence         []string `json:"evidence"`
	DomainRoutes     []string `json:"domain_routes"`
	ActionItems      []string `json:"action_items"`
	Summary          string   `json:"summary"`
	FolderPath       string   `json:"folder_path"`
}

// ProcessResult summarizes a processing pass.
type ProcessResult struct {
	ProcessedCount int
	SkippedCount   int
	BlockedCount   int
}

// SkippedBlockedItem is the API-visible skipped/blocked review row.
type SkippedBlockedItem struct {
	ArtifactID        string `json:"artifact_id"`
	ProviderFileID    string `json:"provider_file_id"`
	Title             string `json:"title"`
	MimeType          string `json:"mime_type"`
	FolderPath        string `json:"folder_path"`
	ProviderURL       string `json:"provider_url"`
	ExtractionState   string `json:"extraction_state"`
	SkipReason        string `json:"skip_reason"`
	RecommendedAction string `json:"recommended_action"`
	UpdatedAt         string `json:"updated_at"`
}

// Store persists extraction and classification outcomes.
type Store interface {
	LoadPendingFiles(ctx context.Context, connectionID string) ([]FileRecord, error)
	LoadFile(ctx context.Context, connectionID string, providerFileID string) (FileRecord, error)
	PersistExtracted(ctx context.Context, file FileRecord, extraction ExtractionResult, folder FolderSummary, classification ClassificationResult) error
	PersistSkippedBlocked(ctx context.Context, file FileRecord, state string, reason string, action string) error
	ListSkippedBlocked(ctx context.Context, connectionID string) ([]SkippedBlockedItem, error)
}

// Worker owns the content extraction and classification contract.
type Worker interface {
	Extract(ctx context.Context, file FileRecord, content []byte) (ExtractionResult, error)
	SummarizeFolder(ctx context.Context, file FileRecord, text string) (FolderSummary, error)
	Classify(ctx context.Context, file FileRecord, text string, folder FolderSummary) (ClassificationResult, error)
}

// Service coordinates provider bytes, extraction workers, and persistence.
type Service struct {
	provider         drive.Provider
	store            Store
	worker           Worker
	maxFileSizeBytes int64
}

// Option configures Service.
type Option func(*Service)

// WithMaxFileSizeBytes overrides the default extraction cap.
func WithMaxFileSizeBytes(maxBytes int64) Option {
	return func(service *Service) {
		service.maxFileSizeBytes = maxBytes
	}
}

// NewService returns a drive extraction service.
func NewService(provider drive.Provider, store Store, worker Worker, opts ...Option) *Service {
	service := &Service{
		provider:         provider,
		store:            store,
		worker:           worker,
		maxFileSizeBytes: defaultMaxFileSizeBytes,
	}
	for _, opt := range opts {
		opt(service)
	}
	return service
}

// ProcessPending processes every pending file for a connection.
func (service *Service) ProcessPending(ctx context.Context, connectionID string) (ProcessResult, error) {
	if err := service.validate(); err != nil {
		return ProcessResult{}, err
	}
	files, err := service.store.LoadPendingFiles(ctx, connectionID)
	if err != nil {
		return ProcessResult{}, err
	}
	result := ProcessResult{}
	for _, file := range files {
		state, err := service.processFile(ctx, file)
		if err != nil {
			return result, err
		}
		switch state {
		case "complete":
			result.ProcessedCount++
		case "skipped":
			result.SkippedCount++
		case "blocked":
			result.BlockedCount++
		}
	}
	return result, nil
}

// RefreshMovedFileContext refreshes folder summary and classification after a
// metadata-only move. It intentionally does not call Provider.GetFile.
func (service *Service) RefreshMovedFileContext(ctx context.Context, connectionID string, providerFileID string) error {
	if err := service.validate(); err != nil {
		return err
	}
	file, err := service.store.LoadFile(ctx, connectionID, providerFileID)
	if err != nil {
		return err
	}
	if file.ExistingContentRaw == "" || file.ExtractionState != "complete" {
		return nil
	}
	folder, err := service.worker.SummarizeFolder(ctx, file, file.ExistingContentRaw)
	if err != nil {
		return err
	}
	classification, err := service.worker.Classify(ctx, file, file.ExistingContentRaw, folder)
	if err != nil {
		return err
	}
	extraction := ExtractionResult{State: "complete", Text: file.ExistingContentRaw, Route: "metadata_refresh", CharacterCount: len(file.ExistingContentRaw)}
	return service.store.PersistExtracted(ctx, file, extraction, folder, classification)
}

func (service *Service) validate() error {
	if service.provider == nil {
		return fmt.Errorf("drive extract: provider is nil")
	}
	if service.store == nil {
		return fmt.Errorf("drive extract: store is nil")
	}
	if service.worker == nil {
		return fmt.Errorf("drive extract: worker is nil")
	}
	if service.maxFileSizeBytes <= 0 {
		return fmt.Errorf("drive extract: max file size must be positive")
	}
	return nil
}

func (service *Service) processFile(ctx context.Context, file FileRecord) (string, error) {
	prov := service.provider.ID()
	if file.SizeBytes > service.maxFileSizeBytes {
		driveobs.DriveExtractFiles.WithLabelValues(prov, string(driveobs.OutcomeSkipped)).Inc()
		slog.Info("drive extract: file skipped",
			"provider", prov, "connection_id", file.ConnectionID,
			"provider_file_id", file.ProviderFileID, "reason", "file_too_large",
			"size_bytes", file.SizeBytes,
		)
		return "skipped", service.store.PersistSkippedBlocked(ctx, file, "skipped", "file_too_large", recommendedAction("file_too_large"))
	}
	if isBlockedBinary(file.MimeType) {
		driveobs.DriveExtractFiles.WithLabelValues(prov, string(driveobs.OutcomeBlocked)).Inc()
		slog.Info("drive extract: file blocked",
			"provider", prov, "connection_id", file.ConnectionID,
			"provider_file_id", file.ProviderFileID, "reason", "unsupported_binary",
			"mime_type", file.MimeType,
		)
		return "blocked", service.store.PersistSkippedBlocked(ctx, file, "blocked", "unsupported_binary", recommendedAction("unsupported_binary"))
	}

	body, err := service.provider.GetFile(ctx, file.ConnectionID, file.ProviderFileID)
	if err != nil {
		driveobs.DriveProviderErrors.WithLabelValues(prov, "extract").Inc()
		driveobs.DriveExtractFiles.WithLabelValues(prov, string(driveobs.OutcomeError)).Inc()
		slog.Warn("drive extract: provider GetFile failed",
			"provider", prov, "connection_id", file.ConnectionID,
			"provider_file_id", file.ProviderFileID, "error", err,
		)
		return "blocked", service.store.PersistSkippedBlocked(ctx, file, "blocked", "provider_read_failed", recommendedAction("provider_read_failed"))
	}
	defer body.Reader.Close()
	content, err := io.ReadAll(io.LimitReader(body.Reader, service.maxFileSizeBytes+1))
	if err != nil {
		driveobs.DriveProviderErrors.WithLabelValues(prov, "extract").Inc()
		driveobs.DriveExtractFiles.WithLabelValues(prov, string(driveobs.OutcomeError)).Inc()
		return "blocked", service.store.PersistSkippedBlocked(ctx, file, "blocked", "provider_read_failed", recommendedAction("provider_read_failed"))
	}
	if int64(len(content)) > service.maxFileSizeBytes {
		driveobs.DriveExtractFiles.WithLabelValues(prov, string(driveobs.OutcomeSkipped)).Inc()
		return "skipped", service.store.PersistSkippedBlocked(ctx, file, "skipped", "file_too_large", recommendedAction("file_too_large"))
	}
	extraction, err := service.worker.Extract(ctx, file, content)
	if err != nil {
		driveobs.DriveExtractFiles.WithLabelValues(prov, string(driveobs.OutcomeError)).Inc()
		return "blocked", err
	}
	if extraction.State != "complete" {
		driveobs.DriveExtractFiles.WithLabelValues(prov, string(driveobs.OutcomeSkipped)).Inc()
		return extraction.State, service.store.PersistSkippedBlocked(ctx, file, extraction.State, extraction.SkipReason, extraction.RecommendedAction)
	}
	folder, err := service.worker.SummarizeFolder(ctx, file, extraction.Text)
	if err != nil {
		driveobs.DriveExtractFiles.WithLabelValues(prov, string(driveobs.OutcomeError)).Inc()
		return "blocked", err
	}
	classification, err := service.worker.Classify(ctx, file, extraction.Text, folder)
	if err != nil {
		driveobs.DriveExtractFiles.WithLabelValues(prov, string(driveobs.OutcomeError)).Inc()
		return "blocked", err
	}
	if err := service.store.PersistExtracted(ctx, file, extraction, folder, classification); err != nil {
		driveobs.DriveExtractFiles.WithLabelValues(prov, string(driveobs.OutcomeError)).Inc()
		return "blocked", err
	}
	driveobs.DriveExtractFiles.WithLabelValues(prov, string(driveobs.OutcomeOK)).Inc()
	return "complete", nil
}

// PostgresStore persists drive extraction state in the canonical DB.
type PostgresStore struct {
	pool *pgxpool.Pool
}

// NewPostgresStore returns a Postgres-backed extraction store.
func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

// LoadPendingFiles returns pending drive files in stable order.
func (store *PostgresStore) LoadPendingFiles(ctx context.Context, connectionID string) ([]FileRecord, error) {
	rows, err := store.pool.Query(ctx, fileRecordQuery(`f.connection_id=$1 AND f.extraction_state='pending' AND f.tombstoned_at IS NULL AND f.permission_lost_at IS NULL ORDER BY f.created_at, f.provider_file_id`), connectionID)
	if err != nil {
		return nil, fmt.Errorf("drive extract: load pending files: %w", err)
	}
	defer rows.Close()
	return scanFileRecords(rows)
}

// LoadFile returns one drive file by provider file ID.
func (store *PostgresStore) LoadFile(ctx context.Context, connectionID string, providerFileID string) (FileRecord, error) {
	row := store.pool.QueryRow(ctx, fileRecordQuery(`f.connection_id=$1 AND f.provider_file_id=$2`), connectionID, providerFileID)
	file, err := scanFileRecord(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return FileRecord{}, fmt.Errorf("drive extract: file %s not found", providerFileID)
	}
	if err != nil {
		return FileRecord{}, err
	}
	return file, nil
}

// PersistExtracted stores searchable content, classification metadata, and folder summary.
func (store *PostgresStore) PersistExtracted(ctx context.Context, file FileRecord, extraction ExtractionResult, folder FolderSummary, classification ClassificationResult) error {
	metadataJSON, domainJSON, topicsJSON, actionsJSON, err := buildArtifactPayloads(file, extraction, folder, classification)
	if err != nil {
		return err
	}
	folderJSON, err := json.Marshal(folder)
	if err != nil {
		return fmt.Errorf("drive extract: marshal folder summary: %w", err)
	}
	contentHash := contentHash(file.ArtifactID, extraction.Text)
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("drive extract: begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `
		UPDATE artifacts
		   SET content_raw=$2,
		       content_hash=$3,
		       summary=$4,
		       topics=$5,
		       action_items=$6,
		       metadata=$7,
		       domain_data=$8,
		       domain_extraction_status='complete',
		       domain_schema_version='drive-classification-v1',
		       domain_extracted_at=now(),
		       processing_status='completed',
		       updated_at=now()
		 WHERE id=$1`, file.ArtifactID, extraction.Text, contentHash, classification.Summary, topicsJSON, actionsJSON, metadataJSON, domainJSON)
	if err != nil {
		return fmt.Errorf("drive extract: update artifact: %w", err)
	}
	_, err = tx.Exec(ctx, `
		UPDATE drive_files
		   SET extraction_state='complete',
		       skip_reason=NULL,
		       sensitivity=$3,
		       updated_at=now()
		 WHERE connection_id=$1 AND provider_file_id=$2`, file.ConnectionID, file.ProviderFileID, classification.DriveSensitivity)
	if err != nil {
		return fmt.Errorf("drive extract: update drive file: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO drive_folders (id, connection_id, provider_folder_id, folder_path, folder_summary, summarized_at)
		VALUES ($1, $2, $3, $4, $5, now())
		ON CONFLICT (connection_id, provider_folder_id) DO UPDATE SET
			folder_path=EXCLUDED.folder_path,
			folder_summary=EXCLUDED.folder_summary,
			summarized_at=now()`, uuid.NewString(), file.ConnectionID, providerFolderID(file.FolderPath), file.FolderPath, folderJSON)
	if err != nil {
		return fmt.Errorf("drive extract: upsert folder summary: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("drive extract: commit: %w", err)
	}
	return nil
}

// PersistSkippedBlocked marks a file as visible skipped/blocked work.
func (store *PostgresStore) PersistSkippedBlocked(ctx context.Context, file FileRecord, state string, reason string, action string) error {
	metadata := cloneMap(file.ExistingMetadata)
	driveMetadata := ensureMap(metadata, "drive")
	driveMetadata["extraction"] = map[string]any{
		"state":              state,
		"skip_reason":        reason,
		"recommended_action": action,
		"folder_path":        strings.Join(file.FolderPath, "/"),
	}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("drive extract: marshal skipped metadata: %w", err)
	}
	_, err = store.pool.Exec(ctx, `
		WITH artifact_update AS (
			UPDATE artifacts
			   SET metadata=$4,
			       processing_status=$5,
			       updated_at=now()
			 WHERE id=$1
		)
		UPDATE drive_files
		   SET extraction_state=$2,
		       skip_reason=$3,
		       updated_at=now()
		 WHERE artifact_id=$1`, file.ArtifactID, state, reason, metadataJSON, state)
	if err != nil {
		return fmt.Errorf("drive extract: persist skipped/blocked: %w", err)
	}
	return nil
}

// ListSkippedBlocked returns reviewable skipped/blocked files.
func (store *PostgresStore) ListSkippedBlocked(ctx context.Context, connectionID string) ([]SkippedBlockedItem, error) {
	rows, err := store.pool.Query(ctx, `
		SELECT artifact_id, provider_file_id, title, mime_type,
		       array_to_string(folder_path, '/'), provider_url, extraction_state,
		       COALESCE(skip_reason, ''), updated_at::text
		  FROM drive_files
		 WHERE connection_id=$1 AND extraction_state IN ('skipped', 'blocked')
		 ORDER BY extraction_state, skip_reason, title`, connectionID)
	if err != nil {
		return nil, fmt.Errorf("drive extract: list skipped blocked: %w", err)
	}
	defer rows.Close()
	items := []SkippedBlockedItem{}
	for rows.Next() {
		var item SkippedBlockedItem
		if err := rows.Scan(&item.ArtifactID, &item.ProviderFileID, &item.Title, &item.MimeType, &item.FolderPath, &item.ProviderURL, &item.ExtractionState, &item.SkipReason, &item.UpdatedAt); err != nil {
			return nil, fmt.Errorf("drive extract: scan skipped blocked: %w", err)
		}
		item.RecommendedAction = recommendedAction(item.SkipReason)
		items = append(items, item)
	}
	return items, rows.Err()
}

func fileRecordQuery(whereClause string) string {
	return `
		SELECT f.artifact_id, f.connection_id::text, f.provider_file_id,
		       COALESCE(f.provider_revision_id, ''), f.title, f.mime_type, f.size_bytes,
		       f.folder_path, f.provider_url, f.owner_label, f.sharing_state,
		       f.extraction_state, COALESCE(f.skip_reason, ''), COALESCE(a.content_raw, ''),
		       COALESCE(a.metadata, '{}'::jsonb), COALESCE(a.domain_data, '{}'::jsonb)
		  FROM drive_files f
		  JOIN artifacts a ON a.id=f.artifact_id
		 WHERE ` + whereClause
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanFileRecords(rows pgx.Rows) ([]FileRecord, error) {
	files := []FileRecord{}
	for rows.Next() {
		file, err := scanFileRecord(rows)
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}
	return files, rows.Err()
}

func scanFileRecord(scanner rowScanner) (FileRecord, error) {
	var file FileRecord
	var sharingJSON []byte
	var metadataJSON []byte
	var domainJSON []byte
	if err := scanner.Scan(
		&file.ArtifactID,
		&file.ConnectionID,
		&file.ProviderFileID,
		&file.ProviderRevisionID,
		&file.Title,
		&file.MimeType,
		&file.SizeBytes,
		&file.FolderPath,
		&file.ProviderURL,
		&file.OwnerLabel,
		&sharingJSON,
		&file.ExtractionState,
		&file.SkipReason,
		&file.ExistingContentRaw,
		&metadataJSON,
		&domainJSON,
	); err != nil {
		return FileRecord{}, err
	}
	file.SharingState = decodeMap(sharingJSON)
	file.ExistingMetadata = decodeMap(metadataJSON)
	file.ExistingDomainData = decodeMap(domainJSON)
	return file, nil
}

func buildArtifactPayloads(file FileRecord, extraction ExtractionResult, folder FolderSummary, classification ClassificationResult) ([]byte, []byte, []byte, []byte, error) {
	metadata := cloneMap(file.ExistingMetadata)
	driveMetadata := ensureMap(metadata, "drive")
	driveMetadata["provider_file_id"] = file.ProviderFileID
	driveMetadata["provider_revision_id"] = file.ProviderRevisionID
	driveMetadata["folder_path"] = file.FolderPath
	driveMetadata["mime_type"] = file.MimeType
	driveMetadata["extraction"] = map[string]any{
		"state":           extraction.State,
		"route":           extraction.Route,
		"character_count": extraction.CharacterCount,
	}
	driveMetadata["folder_summary"] = folder
	driveMetadata["classification"] = classification

	domainData := map[string]any{
		"source_kind":    "drive_file",
		"classification": classification.Classification,
		"topic":          classification.Topic,
		"audience":       classification.Audience,
		"sensitivity":    classification.Sensitivity,
		"confidence":     classification.Confidence,
		"evidence":       classification.Evidence,
		"domain_routes":  classification.DomainRoutes,
		"action_items":   classification.ActionItems,
		"folder_context": map[string]any{
			"folder_path": folder.FolderPath,
			"topic":       folder.Topic,
			"audience":    folder.Audience,
		},
	}
	topics := uniqueStrings([]string{classification.Topic, classification.Classification})
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("drive extract: marshal metadata: %w", err)
	}
	domainJSON, err := json.Marshal(domainData)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("drive extract: marshal domain data: %w", err)
	}
	topicsJSON, err := json.Marshal(topics)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("drive extract: marshal topics: %w", err)
	}
	actionsJSON, err := json.Marshal(classification.ActionItems)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("drive extract: marshal actions: %w", err)
	}
	return metadataJSON, domainJSON, topicsJSON, actionsJSON, nil
}

func decodeMap(raw []byte) map[string]any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	decoded := map[string]any{}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return map[string]any{}
	}
	return decoded
}

func cloneMap(input map[string]any) map[string]any {
	out := map[string]any{}
	for key, value := range input {
		out[key] = value
	}
	return out
}

func ensureMap(parent map[string]any, key string) map[string]any {
	if existing, ok := parent[key].(map[string]any); ok {
		return existing
	}
	created := map[string]any{}
	parent[key] = created
	return created
}

func contentHash(artifactID string, text string) string {
	sum := sha256.Sum256([]byte(artifactID + "\x00" + text))
	return "drive-content:" + hex.EncodeToString(sum[:])
}

func providerFolderID(folderPath []string) string {
	if len(folderPath) == 0 {
		return "path:/"
	}
	return "path:" + strings.Join(folderPath, "/")
}

func recommendedAction(reason string) string {
	switch reason {
	case "file_too_large":
		return "Open the file in Drive or raise the configured drive extraction size limit."
	case "unsupported_binary":
		return "Open the file in Drive or add a supported export/transcript format."
	case "unsupported_mime_type":
		return "Add a supported text, PDF, image, office, or audio export for this file."
	case "provider_read_failed":
		return "Reconnect the provider or restore file permission, then retry extraction."
	case "no_extractable_text":
		return "Open the file in Drive or provide OCR/transcript text Smackerel can read."
	default:
		return "Review the file in Drive and retry extraction when it becomes readable."
	}
}

func isBlockedBinary(mimeType string) bool {
	switch mimeType {
	case "application/zip", "application/x-zip-compressed":
		return true
	default:
		return false
	}
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" || seen[trimmed] {
			continue
		}
		seen[trimmed] = true
		out = append(out, trimmed)
	}
	sort.Strings(out)
	return out
}

// RuleBasedWorker is a deterministic worker used by tests and local bootstrap
// runs where the ML sidecar is unavailable. The Python sidecar owns the LLM
// prompt contract; this worker mirrors the output schema for live-stack tests.
type RuleBasedWorker struct{}

// NewRuleBasedWorker returns a deterministic worker.
func NewRuleBasedWorker() RuleBasedWorker {
	return RuleBasedWorker{}
}

// Extract extracts text from supported drive file bytes.
func (RuleBasedWorker) Extract(_ context.Context, file FileRecord, content []byte) (ExtractionResult, error) {
	var text string
	var route string
	switch {
	case isTextMime(file.MimeType):
		text = decodeText(content)
		route = "text"
	case file.MimeType == "application/pdf":
		text, route = extractPDFText(content)
	case file.MimeType == "image/svg+xml":
		text = stripTags(decodeText(content))
		route = "image_ocr"
	case file.MimeType == "application/vnd.openxmlformats-officedocument.wordprocessingml.document":
		text = extractDocxText(content)
		route = "office_document"
	case strings.HasPrefix(file.MimeType, "audio/"):
		text = extractTranscript(content)
		route = "audio_transcript"
	default:
		return ExtractionResult{State: "skipped", SkipReason: "unsupported_mime_type", RecommendedAction: recommendedAction("unsupported_mime_type")}, nil
	}
	text = normalizeText(text)
	if text == "" {
		return ExtractionResult{State: "blocked", SkipReason: "no_extractable_text", RecommendedAction: recommendedAction("no_extractable_text")}, nil
	}
	return ExtractionResult{State: "complete", Text: text, Route: route, CharacterCount: len(text)}, nil
}

// SummarizeFolder returns deterministic folder context.
func (RuleBasedWorker) SummarizeFolder(_ context.Context, file FileRecord, text string) (FolderSummary, error) {
	folderPath := strings.Join(file.FolderPath, "/")
	lowerContext := strings.ToLower(folderPath + " " + file.Title + " " + text)
	classification := inferClassification(lowerContext)
	return FolderSummary{
		FolderPath:     folderPath,
		Topic:          inferTopic(file.FolderPath, classification),
		Audience:       inferAudience(lowerContext),
		Classification: classification,
		Evidence:       []string{"folder context " + folderPath},
	}, nil
}

// Classify returns provider-neutral domain routing metadata.
func (RuleBasedWorker) Classify(_ context.Context, file FileRecord, text string, folder FolderSummary) (ClassificationResult, error) {
	lowerContext := strings.ToLower(strings.Join(file.FolderPath, " ") + " " + file.Title + " " + text)
	classification := inferClassification(lowerContext)
	if folder.Classification == "recipe" && classification == "expense" {
		classification = "recipe"
	}
	routes := routesForClassification(classification, lowerContext)
	evidence := []string{evidencePhrase(text), "folder context " + folder.FolderPath}
	return ClassificationResult{
		Classification:   classification,
		Topic:            inferTopic(file.FolderPath, classification),
		Audience:         inferAudience(lowerContext),
		Sensitivity:      inferLLMSensitivity(classification, lowerContext),
		DriveSensitivity: inferDriveSensitivity(classification, lowerContext),
		Confidence:       0.87,
		Evidence:         evidence,
		DomainRoutes:     routes,
		ActionItems:      actionItems(text),
		Summary:          summary(text),
		FolderPath:       folder.FolderPath,
	}, nil
}

func isTextMime(mimeType string) bool {
	switch mimeType {
	case "text/plain", "text/markdown", "text/csv", "application/json":
		return true
	default:
		return false
	}
}

func decodeText(content []byte) string {
	return string(bytes.TrimPrefix(content, []byte{0xef, 0xbb, 0xbf}))
}

func extractPDFText(content []byte) (string, string) {
	source := decodeText(content)
	if match := regexp.MustCompile(`SMACKEREL_OCR_TEXT\(([^)]*)\)`).FindStringSubmatch(source); len(match) == 2 {
		return match[1], "pdf_ocr"
	}
	matches := regexp.MustCompile(`\(([^()]{3,})\)`).FindAllStringSubmatch(source, -1)
	parts := []string{}
	for _, match := range matches {
		parts = append(parts, match[1])
	}
	return strings.Join(parts, "\n"), "pdf_text"
}

func extractDocxText(content []byte) string {
	reader, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		return ""
	}
	for _, file := range reader.File {
		if file.Name != "word/document.xml" {
			continue
		}
		body, err := file.Open()
		if err != nil {
			return ""
		}
		defer body.Close()
		data, err := io.ReadAll(body)
		if err != nil {
			return ""
		}
		return stripTags(string(data))
	}
	return ""
}

func stripTags(source string) string {
	withoutTags := regexp.MustCompile(`<[^>]+>`).ReplaceAllString(source, " ")
	return html.UnescapeString(withoutTags)
}

func extractTranscript(content []byte) string {
	match := regexp.MustCompile(`(?is)TRANSCRIPT:\s*(.*)`).FindStringSubmatch(decodeText(content))
	if len(match) == 2 {
		return match[1]
	}
	return ""
}

func normalizeText(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

func inferClassification(lowerContext string) string {
	switch {
	case strings.Contains(lowerContext, "meal") || strings.Contains(lowerContext, "recipe") || strings.Contains(lowerContext, "chickpea") || strings.Contains(lowerContext, "tomato") || strings.Contains(lowerContext, "basil"):
		return "recipe"
	case strings.Contains(lowerContext, "receipt") || strings.Contains(lowerContext, "paid") || strings.Contains(lowerContext, "invoice") || strings.Contains(lowerContext, "total"):
		return "expense"
	case strings.Contains(lowerContext, "annotation") || strings.Contains(lowerContext, "note"):
		return "annotation"
	case strings.Contains(lowerContext, "action") || strings.Contains(lowerContext, "buy") || strings.Contains(lowerContext, "todo") || strings.Contains(lowerContext, "list"):
		return "list"
	default:
		return "note"
	}
}

func inferTopic(folderPath []string, classification string) string {
	if len(folderPath) > 0 {
		return folderPath[0]
	}
	return classification
}

func inferAudience(lowerContext string) string {
	if strings.Contains(lowerContext, "business") || strings.Contains(lowerContext, "invoice") {
		return "business"
	}
	if strings.Contains(lowerContext, "meal") || strings.Contains(lowerContext, "grocery") || strings.Contains(lowerContext, "household") {
		return "household"
	}
	return "personal"
}

func inferLLMSensitivity(classification string, lowerContext string) string {
	if classification == "expense" || strings.Contains(lowerContext, "card") || strings.Contains(lowerContext, "invoice") {
		return "confidential"
	}
	return "none"
}

func inferDriveSensitivity(classification string, lowerContext string) string {
	if classification == "expense" || strings.Contains(lowerContext, "card") || strings.Contains(lowerContext, "invoice") {
		return "financial"
	}
	if strings.Contains(lowerContext, "passport") || strings.Contains(lowerContext, "ssn") {
		return "identity"
	}
	return "none"
}

func routesForClassification(classification string, lowerContext string) []string {
	routes := []string{"digest"}
	switch classification {
	case "recipe":
		routes = append(routes, "recipes", "meal_plan", "lists")
	case "expense":
		routes = append(routes, "expenses")
	case "annotation":
		routes = append(routes, "annotations")
	case "list":
		routes = append(routes, "lists")
	}
	if strings.Contains(lowerContext, "action") || strings.Contains(lowerContext, "buy") || strings.Contains(lowerContext, "todo") {
		routes = append(routes, "action_items")
	}
	return uniqueStrings(routes)
}

func actionItems(text string) []string {
	items := []string{}
	for _, sentence := range regexp.MustCompile(`[.!?]`).Split(text, -1) {
		trimmed := strings.TrimSpace(sentence)
		lower := strings.ToLower(trimmed)
		if strings.Contains(lower, "action:") || strings.Contains(lower, "buy ") || strings.Contains(lower, "todo") {
			items = append(items, trimmed)
		}
	}
	return items
}

func evidencePhrase(text string) string {
	trimmed := strings.TrimSpace(text)
	if len(trimmed) <= 80 {
		return trimmed
	}
	return trimmed[:80]
}

func summary(text string) string {
	trimmed := strings.TrimSpace(text)
	if len(trimmed) <= 160 {
		return trimmed
	}
	return trimmed[:160]
}

// UpdatedAtNow returns a stable helper for tests that need review items.
func UpdatedAtNow() string {
	return time.Now().UTC().Format(time.RFC3339)
}
