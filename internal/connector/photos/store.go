package photos

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
}

type PhotoRecord struct {
	ID                       uuid.UUID        `json:"photo_id"`
	ArtifactID               string           `json:"artifact_id"`
	ConnectorID              string           `json:"connector_id"`
	Provider                 string           `json:"provider"`
	ProviderRef              string           `json:"provider_ref"`
	ProviderMediaKind        string           `json:"provider_media_kind"`
	MediaRole                MediaRole        `json:"media_role"`
	MIMEType                 string           `json:"mime_type"`
	Bytes                    *int64           `json:"bytes,omitempty"`
	BytesEstimated           bool             `json:"bytes_estimated"`
	Filename                 string           `json:"filename"`
	CapturedAt               *time.Time       `json:"captured_at,omitempty"`
	UploadedAt               *time.Time       `json:"uploaded_at,omitempty"`
	GeoLat                   *float64         `json:"geo_lat,omitempty"`
	GeoLon                   *float64         `json:"geo_lon,omitempty"`
	ContentHash              string           `json:"content_hash"`
	EXIF                     json.RawMessage  `json:"exif"`
	Albums                   []string         `json:"albums"`
	Tags                     []string         `json:"tags"`
	Sensitivity              SensitivityLevel `json:"sensitivity"`
	SensitivityLabels        []string         `json:"sensitivity_labels"`
	SensitivitySource        string           `json:"sensitivity_src"`
	LifecycleState           string           `json:"lifecycle_state"`
	Classification           json.RawMessage  `json:"classification"`
	ClassificationConfidence *float64         `json:"classification_confidence,omitempty"`
	RawProvider              json.RawMessage  `json:"raw_provider"`
	SourceChannel            SourceChannel    `json:"source_channel"`
	SourceRef                string           `json:"source_ref"`
	DocumentGroupID          *uuid.UUID       `json:"document_group_id,omitempty"`
	DocumentPageIndex        *int             `json:"document_page_index,omitempty"`
}

type PhotoSearchRecord struct {
	PhotoRecord
	Classification  ClassificationDecision `json:"classification"`
	MatchConfidence float64                `json:"match_confidence"`
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (store *Store) PublishPhotoEvent(ctx context.Context, connectorID string, provider string, event PhotoEvent) (*PhotoRecord, error) {
	if store == nil || store.pool == nil {
		return nil, fmt.Errorf("photos: store pool is nil")
	}
	if strings.TrimSpace(connectorID) == "" {
		return nil, fmt.Errorf("photos: connector_id is required")
	}
	if strings.TrimSpace(provider) == "" {
		provider = event.ProviderName()
	}
	if strings.TrimSpace(provider) == "" {
		return nil, fmt.Errorf("photos: provider is required")
	}
	if err := event.Validate(); err != nil {
		return nil, err
	}

	artifactID := ArtifactID(provider, event.ProviderRef)
	exifBytes, err := json.Marshal(event.EXIF)
	if err != nil {
		return nil, fmt.Errorf("marshal exif: %w", err)
	}
	rawProviderBytes, err := json.Marshal(event.RawProvider)
	if err != nil {
		return nil, fmt.Errorf("marshal raw_provider: %w", err)
	}
	classificationBytes := []byte(`{}`)
	contentHash := event.ContentHash
	if contentHash == "" {
		contentHash = fallbackContentHash(provider, event.ProviderRef, event.Filename)
	}
	capturedAt := nullableTime(event.CapturedAt)
	uploadedAt := nullableTime(event.UploadedAt)

	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin photo publish transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Spec 040 Scope 5: a single content_hash may be ingested by more
	// than one provider. The artifacts table enforces a unique
	// content_hash, so the second provider must REUSE the existing
	// artifact row instead of creating a duplicate. This preserves
	// the canonical artifact-level dedupe AND lets the cross-provider
	// rerank link both `photos` rows to the same artifact.
	var existingArtifactID string
	if err := tx.QueryRow(ctx, `SELECT id FROM artifacts WHERE content_hash=$1 LIMIT 1`, contentHash).Scan(&existingArtifactID); err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("lookup artifact by content_hash: %w", err)
		}
	}
	if existingArtifactID != "" {
		artifactID = existingArtifactID
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO artifacts (
			id, artifact_type, title, summary, content_raw, content_hash,
			source_id, source_ref, source_url, source_quality, metadata, processing_status
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (id) DO UPDATE SET
			title = EXCLUDED.title,
			content_raw = EXCLUDED.content_raw,
			content_hash = EXCLUDED.content_hash,
			metadata = EXCLUDED.metadata,
			updated_at = now()
	`, artifactID, "photo", event.Filename, "", event.Filename, contentHash,
		"photos:"+provider, event.ProviderRef, "", "medium", rawProviderBytes, "processed")
	if err != nil {
		return nil, fmt.Errorf("upsert artifact for photo: %w", err)
	}

	photoID := uuid.New()
	sourceChannel := event.SourceChannel
	if sourceChannel == "" {
		sourceChannel = SourceChannelProvider
	}
	if !sourceChannel.Valid() {
		return nil, fmt.Errorf("photos: invalid source_channel %q", sourceChannel)
	}
	var documentGroupID *uuid.UUID
	var documentPageIndex *int
	if strings.TrimSpace(event.DocumentGroupRef) != "" {
		groupID, err := store.upsertDocumentGroupTx(ctx, tx, event.DocumentGroupRef)
		if err != nil {
			return nil, err
		}
		documentGroupID = &groupID
		if event.DocumentPageIndex > 0 {
			page := event.DocumentPageIndex
			documentPageIndex = &page
		}
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO photos (
			id, artifact_id, connector_id, provider, provider_ref, provider_media_kind,
			media_role, mime_type, bytes, bytes_estimated, filename, captured_at,
			uploaded_at, geo_lat, geo_lon, content_hash, exif, albums, tags,
			sensitivity, sensitivity_labels, sensitivity_src, lifecycle_state,
			classification, classification_confidence, raw_provider,
			source_channel, source_ref, document_group_id, document_page_index
		) VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8, $9, $10, $11, $12,
			$13, $14, $15, $16, $17, $18, $19,
			$20, $21, $22, $23,
			$24, $25, $26,
			$27, $28, $29, $30
		)
		ON CONFLICT (provider, provider_ref) DO UPDATE SET
			artifact_id = EXCLUDED.artifact_id,
			connector_id = EXCLUDED.connector_id,
			provider_media_kind = EXCLUDED.provider_media_kind,
			media_role = EXCLUDED.media_role,
			mime_type = EXCLUDED.mime_type,
			bytes = EXCLUDED.bytes,
			bytes_estimated = EXCLUDED.bytes_estimated,
			filename = EXCLUDED.filename,
			captured_at = EXCLUDED.captured_at,
			uploaded_at = EXCLUDED.uploaded_at,
			geo_lat = EXCLUDED.geo_lat,
			geo_lon = EXCLUDED.geo_lon,
			content_hash = EXCLUDED.content_hash,
			exif = EXCLUDED.exif,
			albums = EXCLUDED.albums,
			tags = EXCLUDED.tags,
			sensitivity = EXCLUDED.sensitivity,
			sensitivity_labels = EXCLUDED.sensitivity_labels,
			sensitivity_src = EXCLUDED.sensitivity_src,
			raw_provider = EXCLUDED.raw_provider,
			source_channel = EXCLUDED.source_channel,
			source_ref = EXCLUDED.source_ref,
			document_group_id = EXCLUDED.document_group_id,
			document_page_index = EXCLUDED.document_page_index,
			updated_at = now()
	`, photoID, artifactID, connectorID, provider, event.ProviderRef, event.ProviderMediaKind,
		string(event.MediaRole), event.MIMEType, event.Bytes, event.BytesEstimated, event.Filename, capturedAt,
		uploadedAt, event.GeoLat, event.GeoLon, contentHash, exifBytes, nonNilStrings(event.Albums), nonNilStrings(event.Tags),
		string(event.Sensitivity.Level), nonNilStrings(event.Sensitivity.Labels), event.Sensitivity.Source, "unknown",
		classificationBytes, nil, rawProviderBytes,
		string(sourceChannel), event.SourceRef, documentGroupID, documentPageIndex)
	if err != nil {
		return nil, fmt.Errorf("upsert photo: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit photo publish transaction: %w", err)
	}
	return store.GetByArtifactID(ctx, artifactID)
}

func (store *Store) GetByID(ctx context.Context, id uuid.UUID) (*PhotoRecord, error) {
	return store.get(ctx, "p.id=$1", id)
}

func (store *Store) GetByArtifactID(ctx context.Context, artifactID string) (*PhotoRecord, error) {
	return store.get(ctx, "p.artifact_id=$1", artifactID)
}

func (store *Store) GetByProviderRef(ctx context.Context, provider string, providerRef string) (*PhotoRecord, error) {
	return store.get(ctx, "p.provider=$1 AND p.provider_ref=$2", provider, providerRef)
}

func (store *Store) get(ctx context.Context, where string, args ...any) (*PhotoRecord, error) {
	if store == nil || store.pool == nil {
		return nil, fmt.Errorf("photos: store pool is nil")
	}
	query := `
		SELECT p.id, p.artifact_id, p.connector_id, p.provider, p.provider_ref,
		       p.provider_media_kind, p.media_role::text, p.mime_type, p.bytes,
		       p.bytes_estimated, p.filename, p.captured_at, p.uploaded_at,
		       p.geo_lat, p.geo_lon, COALESCE(p.content_hash, ''), p.exif,
		       p.albums, p.tags, p.sensitivity::text, p.sensitivity_labels,
		       p.sensitivity_src, p.lifecycle_state::text, p.classification,
		       p.classification_confidence, p.raw_provider,
		       p.source_channel, p.source_ref, p.document_group_id, p.document_page_index
		  FROM photos p
		 WHERE ` + where
	var rec PhotoRecord
	var role string
	var sensitivity string
	var sourceChannel string
	if err := store.pool.QueryRow(ctx, query, args...).Scan(
		&rec.ID, &rec.ArtifactID, &rec.ConnectorID, &rec.Provider, &rec.ProviderRef,
		&rec.ProviderMediaKind, &role, &rec.MIMEType, &rec.Bytes,
		&rec.BytesEstimated, &rec.Filename, &rec.CapturedAt, &rec.UploadedAt,
		&rec.GeoLat, &rec.GeoLon, &rec.ContentHash, &rec.EXIF,
		&rec.Albums, &rec.Tags, &sensitivity, &rec.SensitivityLabels,
		&rec.SensitivitySource, &rec.LifecycleState, &rec.Classification,
		&rec.ClassificationConfidence, &rec.RawProvider,
		&sourceChannel, &rec.SourceRef, &rec.DocumentGroupID, &rec.DocumentPageIndex,
	); err != nil {
		return nil, err
	}
	rec.MediaRole = MediaRole(role)
	rec.Sensitivity = SensitivityLevel(sensitivity)
	rec.SourceChannel = SourceChannel(sourceChannel)
	return &rec, nil
}

func (store *Store) UpdateClassification(ctx context.Context, photoID uuid.UUID, decision ClassificationDecision) error {
	if store == nil || store.pool == nil {
		return fmt.Errorf("photos: store pool is nil")
	}
	if _, err := decision.Validate(); err != nil {
		return err
	}
	classificationBytes, err := json.Marshal(decision)
	if err != nil {
		return fmt.Errorf("marshal classification: %w", err)
	}
	searchText := strings.TrimSpace(decision.SearchText())
	if searchText == "" {
		searchText = decision.Caption
	}
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin classification transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var artifactID string
	if err := tx.QueryRow(ctx, `
		UPDATE photos
		   SET classification=$2,
		       classification_confidence=$3,
		       classification_rationale=$4,
		       lifecycle_state=CASE WHEN lifecycle_state='unknown' THEN 'active'::photo_lifecycle_state ELSE lifecycle_state END,
		       updated_at=now()
		 WHERE id=$1
		 RETURNING artifact_id
	`, photoID, classificationBytes, decision.Confidence, decision.Rationale).Scan(&artifactID); err != nil {
		return fmt.Errorf("update photo classification: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE artifacts
		   SET summary=$2,
		       content_raw=$3,
		       processing_status='processed',
		       updated_at=now()
		 WHERE id=$1
	`, artifactID, decision.Caption, searchText); err != nil {
		return fmt.Errorf("update photo artifact classification: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit classification transaction: %w", err)
	}
	return nil
}

func (store *Store) MarkDeleted(ctx context.Context, connectorID string, provider string, providerRef string) error {
	if store == nil || store.pool == nil {
		return fmt.Errorf("photos: store pool is nil")
	}
	if strings.TrimSpace(connectorID) == "" {
		return fmt.Errorf("photos: connector_id is required")
	}
	if strings.TrimSpace(provider) == "" || strings.TrimSpace(providerRef) == "" {
		return fmt.Errorf("photos: provider and provider_ref are required")
	}
	artifactID := ArtifactID(provider, providerRef)
	contentHash := "deleted:" + provider + ":" + providerRef
	rawProviderBytes, err := json.Marshal(map[string]any{"provider": provider, "asset_id": providerRef, "deleted": true})
	if err != nil {
		return err
	}
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin photo tombstone transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		INSERT INTO artifacts (
			id, artifact_type, title, summary, content_raw, content_hash,
			source_id, source_ref, source_url, source_quality, metadata, processing_status
		) VALUES ($1, 'photo', $2, '', '', $3, $4, $5, '', 'low', $6, 'tombstoned')
		ON CONFLICT (id) DO UPDATE SET
			processing_status='tombstoned',
			metadata=EXCLUDED.metadata,
			updated_at=now()
	`, artifactID, providerRef, contentHash, "photos:"+provider, providerRef, rawProviderBytes); err != nil {
		return fmt.Errorf("upsert tombstone artifact: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO photos (
			id, artifact_id, connector_id, provider, provider_ref, provider_media_kind,
			media_role, mime_type, bytes_estimated, filename, content_hash, exif,
			albums, tags, sensitivity, sensitivity_labels, sensitivity_src,
			lifecycle_state, classification, raw_provider
		) VALUES (
			$1, $2, $3, $4, $5, 'deleted',
			'unknown', 'application/octet-stream', false, $5, $6, '{}',
			'{}', '{}', 'none', '{}', 'provider',
			'deleted', '{}', $7
		)
		ON CONFLICT (provider, provider_ref) DO UPDATE SET
			connector_id=EXCLUDED.connector_id,
			lifecycle_state='deleted',
			raw_provider=EXCLUDED.raw_provider,
			updated_at=now()
	`, uuid.New(), artifactID, connectorID, provider, providerRef, contentHash, rawProviderBytes); err != nil {
		return fmt.Errorf("upsert photo tombstone: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit photo tombstone transaction: %w", err)
	}
	return nil
}

func (store *Store) Search(ctx context.Context, query string, limit int) ([]PhotoSearchRecord, error) {
	if store == nil || store.pool == nil {
		return nil, fmt.Errorf("photos: store pool is nil")
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return []PhotoSearchRecord{}, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := store.pool.Query(ctx, `
		WITH searchable_photos AS (
			SELECT p.*,
			       LOWER(
					COALESCE(p.filename, '') || ' ' ||
					COALESCE(array_to_string(p.albums, ' '), '') || ' ' ||
					COALESCE(array_to_string(p.tags, ' '), '') || ' ' ||
					COALESCE(p.classification::text, '') || ' ' ||
					COALESCE(p.exif::text, '') || ' ' ||
					COALESCE(a.content_raw, '') || ' ' ||
					COALESCE(a.summary, '')
			       ) AS search_text
			  FROM photos p
			  JOIN artifacts a ON a.id = p.artifact_id
		)
		SELECT p.id, p.artifact_id, p.connector_id, p.provider, p.provider_ref,
		       p.provider_media_kind, p.media_role::text, p.mime_type, p.bytes,
		       p.bytes_estimated, p.filename, p.captured_at, p.uploaded_at,
		       p.geo_lat, p.geo_lon, COALESCE(p.content_hash, ''), p.exif,
		       p.albums, p.tags, p.sensitivity::text, p.sensitivity_labels,
		       p.sensitivity_src, p.lifecycle_state::text, p.classification,
		       p.classification_confidence, p.raw_provider,
		       p.source_channel, p.source_ref, p.document_group_id, p.document_page_index,
		       COALESCE(p.classification_confidence, 0.25) AS match_confidence
		  FROM searchable_photos p
		 WHERE p.lifecycle_state::text <> 'deleted'
		   AND NOT EXISTS (
			SELECT 1
			  FROM unnest(regexp_split_to_array(LOWER($1), '\s+')) AS term
			 WHERE term <> '' AND p.search_text NOT LIKE '%' || term || '%'
		   )
		 ORDER BY match_confidence DESC, p.captured_at DESC NULLS LAST, p.updated_at DESC
		 LIMIT $2
	`, query, limit)
	if err != nil {
		return nil, fmt.Errorf("photo search query: %w", err)
	}
	defer rows.Close()

	var results []PhotoSearchRecord
	for rows.Next() {
		var rec PhotoRecord
		var role string
		var sensitivity string
		var sourceChannel string
		var matchConfidence float64
		if err := rows.Scan(
			&rec.ID, &rec.ArtifactID, &rec.ConnectorID, &rec.Provider, &rec.ProviderRef,
			&rec.ProviderMediaKind, &role, &rec.MIMEType, &rec.Bytes,
			&rec.BytesEstimated, &rec.Filename, &rec.CapturedAt, &rec.UploadedAt,
			&rec.GeoLat, &rec.GeoLon, &rec.ContentHash, &rec.EXIF,
			&rec.Albums, &rec.Tags, &sensitivity, &rec.SensitivityLabels,
			&rec.SensitivitySource, &rec.LifecycleState, &rec.Classification,
			&rec.ClassificationConfidence, &rec.RawProvider,
			&sourceChannel, &rec.SourceRef, &rec.DocumentGroupID, &rec.DocumentPageIndex,
			&matchConfidence,
		); err != nil {
			return nil, fmt.Errorf("scan photo search row: %w", err)
		}
		rec.MediaRole = MediaRole(role)
		rec.Sensitivity = SensitivityLevel(sensitivity)
		rec.SourceChannel = SourceChannel(sourceChannel)
		var classification ClassificationDecision
		if len(rec.Classification) > 0 {
			_ = json.Unmarshal(rec.Classification, &classification)
		}
		results = append(results, PhotoSearchRecord{PhotoRecord: rec, Classification: classification, MatchConfidence: matchConfidence})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate photo search rows: %w", err)
	}
	return results, nil
}

func (store *Store) UpsertConnectorState(ctx context.Context, state ConnectorState) error {
	if store == nil || store.pool == nil {
		return fmt.Errorf("photos: store pool is nil")
	}
	progressBytes, err := json.Marshal(state.Progress)
	if err != nil {
		return fmt.Errorf("marshal scan progress: %w", err)
	}
	skipsBytes, err := json.Marshal(state.Skips)
	if err != nil {
		return fmt.Errorf("marshal skip ledger: %w", err)
	}
	scopeBytes, err := json.Marshal(state.Scope)
	if err != nil {
		return fmt.Errorf("marshal connector scope: %w", err)
	}
	if state.Status == "" {
		state.Status = "healthy"
	}
	if state.UpdatedAt.IsZero() {
		state.UpdatedAt = time.Now().UTC()
	}
	_, err = store.pool.Exec(ctx, `
		INSERT INTO photo_sync_state (
			connector_id, provider, cursor, progress, skipped, scope,
			status, last_sync_at, monitoring_lag_seconds, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (connector_id) DO UPDATE SET
			provider=EXCLUDED.provider,
			cursor=EXCLUDED.cursor,
			progress=EXCLUDED.progress,
			skipped=EXCLUDED.skipped,
			scope=EXCLUDED.scope,
			status=EXCLUDED.status,
			last_sync_at=EXCLUDED.last_sync_at,
			monitoring_lag_seconds=EXCLUDED.monitoring_lag_seconds,
			updated_at=EXCLUDED.updated_at
	`, state.ConnectorID, state.Provider, state.Cursor, progressBytes, skipsBytes, scopeBytes,
		state.Status, state.LastSyncAt, state.MonitoringLagSeconds, state.UpdatedAt)
	if err != nil {
		return fmt.Errorf("upsert photo connector state: %w", err)
	}
	return nil
}

func (store *Store) UpsertCapabilities(ctx context.Context, connectorID string, report CapabilityReport) error {
	if store == nil || store.pool == nil {
		return fmt.Errorf("photos: store pool is nil")
	}
	capabilityBytes, err := json.Marshal(report.Capabilities)
	if err != nil {
		return fmt.Errorf("marshal photo capabilities: %w", err)
	}
	_, err = store.pool.Exec(ctx, `
		INSERT INTO photo_capabilities (connector_id, provider, provider_version, capabilities, detected_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, now())
		ON CONFLICT (connector_id) DO UPDATE SET
			provider=EXCLUDED.provider,
			provider_version=EXCLUDED.provider_version,
			capabilities=EXCLUDED.capabilities,
			detected_at=EXCLUDED.detected_at,
			updated_at=now()
	`, connectorID, report.Provider, report.ProviderVersion, capabilityBytes, report.DetectedAt)
	if err != nil {
		return fmt.Errorf("upsert photo capabilities: %w", err)
	}
	return nil
}

func (store *Store) GetConnectorState(ctx context.Context, connectorID string) (*ConnectorState, error) {
	states, err := store.queryConnectorStates(ctx, "WHERE s.connector_id=$1", connectorID)
	if err != nil {
		return nil, err
	}
	if len(states) == 0 {
		return nil, pgx.ErrNoRows
	}
	return &states[0], nil
}

func (store *Store) ListConnectorStates(ctx context.Context) ([]ConnectorState, error) {
	return store.queryConnectorStates(ctx, "", nil)
}

func (store *Store) queryConnectorStates(ctx context.Context, where string, arg any) ([]ConnectorState, error) {
	if store == nil || store.pool == nil {
		return nil, fmt.Errorf("photos: store pool is nil")
	}
	query := `
		SELECT s.connector_id, s.provider, COALESCE(s.cursor, ''), s.progress, s.skipped,
		       s.scope, s.status, s.last_sync_at, s.monitoring_lag_seconds, s.updated_at,
		       COALESCE(c.provider_version, ''), COALESCE(c.capabilities, '{}'::jsonb), c.detected_at
		  FROM photo_sync_state s
		  LEFT JOIN photo_capabilities c ON c.connector_id = s.connector_id
		 ` + where + `
		 ORDER BY s.updated_at DESC`
	var rows pgx.Rows
	var err error
	if arg == nil {
		rows, err = store.pool.Query(ctx, query)
	} else {
		rows, err = store.pool.Query(ctx, query, arg)
	}
	if err != nil {
		return nil, fmt.Errorf("query photo connector states: %w", err)
	}
	defer rows.Close()

	var states []ConnectorState
	for rows.Next() {
		var state ConnectorState
		var progressBytes []byte
		var skipsBytes []byte
		var scopeBytes []byte
		var capabilitiesBytes []byte
		var detectedAt *time.Time
		if err := rows.Scan(&state.ConnectorID, &state.Provider, &state.Cursor, &progressBytes, &skipsBytes,
			&scopeBytes, &state.Status, &state.LastSyncAt, &state.MonitoringLagSeconds, &state.UpdatedAt,
			&state.Capabilities.ProviderVersion, &capabilitiesBytes, &detectedAt); err != nil {
			return nil, fmt.Errorf("scan photo connector state: %w", err)
		}
		state.Capabilities.Provider = state.Provider
		if detectedAt != nil {
			state.Capabilities.DetectedAt = *detectedAt
		}
		if err := json.Unmarshal(progressBytes, &state.Progress); err != nil {
			return nil, fmt.Errorf("decode scan progress: %w", err)
		}
		if err := json.Unmarshal(skipsBytes, &state.Skips); err != nil {
			return nil, fmt.Errorf("decode skip ledger: %w", err)
		}
		if err := json.Unmarshal(scopeBytes, &state.Scope); err != nil {
			return nil, fmt.Errorf("decode connector scope: %w", err)
		}
		if len(capabilitiesBytes) > 0 {
			var capabilityMap map[Capability]CapabilityEntry
			if err := json.Unmarshal(capabilitiesBytes, &capabilityMap); err == nil {
				state.Capabilities.Capabilities = capabilityMap
			}
		}
		states = append(states, state)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate photo connector states: %w", err)
	}
	return states, nil
}

func ArtifactID(provider string, providerRef string) string {
	return "photo:" + provider + ":" + providerRef
}

// QualityHistogramBucket is one row returned by QualityHistogram.
type QualityHistogramBucket struct {
	Bucket string `json:"bucket"`
	Count  int    `json:"count"`
}

// QualityHistogram returns confidence buckets for every photo with a
// classification recorded. Used by /v1/photos/health/quality so the
// dashboard has live data without depending on the dedicated aesthetic
// model pipeline (Scope 5).
func (store *Store) QualityHistogram(ctx context.Context) ([]QualityHistogramBucket, error) {
	if store == nil || store.pool == nil {
		return nil, fmt.Errorf("photos: store pool is nil")
	}
	rows, err := store.pool.Query(ctx, `
		SELECT bucket, COUNT(*)::int AS total FROM (
			SELECT CASE
				WHEN classification_confidence IS NULL THEN 'unclassified'
				WHEN classification_confidence >= 0.9 THEN 'excellent'
				WHEN classification_confidence >= 0.75 THEN 'good'
				WHEN classification_confidence >= 0.5 THEN 'fair'
				ELSE 'poor'
			END AS bucket
			FROM photos
			WHERE lifecycle_state::text <> 'deleted'
		) bucketed
		GROUP BY bucket
		ORDER BY bucket
	`)
	if err != nil {
		return nil, fmt.Errorf("query quality histogram: %w", err)
	}
	defer rows.Close()
	var buckets []QualityHistogramBucket
	for rows.Next() {
		var bucket QualityHistogramBucket
		if err := rows.Scan(&bucket.Bucket, &bucket.Count); err != nil {
			return nil, fmt.Errorf("scan quality bucket: %w", err)
		}
		buckets = append(buckets, bucket)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate quality buckets: %w", err)
	}
	return buckets, nil
}

func fallbackContentHash(parts ...string) string {
	h := sha256.Sum256([]byte(strings.Join(parts, ":")))
	return "sha256:" + hex.EncodeToString(h[:])
}

func nullableTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
