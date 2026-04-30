package photos

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
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
	_, err = tx.Exec(ctx, `
		INSERT INTO photos (
			id, artifact_id, connector_id, provider, provider_ref, provider_media_kind,
			media_role, mime_type, bytes, bytes_estimated, filename, captured_at,
			uploaded_at, geo_lat, geo_lon, content_hash, exif, albums, tags,
			sensitivity, sensitivity_labels, sensitivity_src, lifecycle_state,
			classification, classification_confidence, raw_provider
		) VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8, $9, $10, $11, $12,
			$13, $14, $15, $16, $17, $18, $19,
			$20, $21, $22, $23,
			$24, $25, $26
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
			updated_at = now()
	`, photoID, artifactID, connectorID, provider, event.ProviderRef, event.ProviderMediaKind,
		string(event.MediaRole), event.MIMEType, event.Bytes, event.BytesEstimated, event.Filename, capturedAt,
		uploadedAt, event.GeoLat, event.GeoLon, contentHash, exifBytes, nonNilStrings(event.Albums), nonNilStrings(event.Tags),
		string(event.Sensitivity.Level), nonNilStrings(event.Sensitivity.Labels), event.Sensitivity.Source, "unknown",
		classificationBytes, nil, rawProviderBytes)
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

func (store *Store) get(ctx context.Context, where string, arg any) (*PhotoRecord, error) {
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
		       p.classification_confidence, p.raw_provider
		  FROM photos p
		 WHERE ` + where
	var rec PhotoRecord
	var role string
	var sensitivity string
	if err := store.pool.QueryRow(ctx, query, arg).Scan(
		&rec.ID, &rec.ArtifactID, &rec.ConnectorID, &rec.Provider, &rec.ProviderRef,
		&rec.ProviderMediaKind, &role, &rec.MIMEType, &rec.Bytes,
		&rec.BytesEstimated, &rec.Filename, &rec.CapturedAt, &rec.UploadedAt,
		&rec.GeoLat, &rec.GeoLon, &rec.ContentHash, &rec.EXIF,
		&rec.Albums, &rec.Tags, &sensitivity, &rec.SensitivityLabels,
		&rec.SensitivitySource, &rec.LifecycleState, &rec.Classification,
		&rec.ClassificationConfidence, &rec.RawProvider,
	); err != nil {
		return nil, err
	}
	rec.MediaRole = MediaRole(role)
	rec.Sensitivity = SensitivityLevel(sensitivity)
	return &rec, nil
}

func ArtifactID(provider string, providerRef string) string {
	return "photo:" + provider + ":" + providerRef
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
