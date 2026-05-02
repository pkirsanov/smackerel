// Spec 038 Scope 7 — Postgres-backed Searcher and provider-agnostic
// BytesFetcher implementations of the retrieve.Service contract.
//
// These types live alongside Service so the production wiring layer
// (cmd/core/wiring.go) can construct them without forcing this package
// to import internal/drive (which would create an import cycle because
// internal/drive registers agent tools that import this package).
// BytesFetcher therefore takes a pure provider-lookup function rather
// than the drive.Registry concrete type.
package retrieve

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresSearcher queries drive_files JOIN artifacts for candidates
// matching the request query. Provider-neutral identifiers are derived
// from drive_connections.provider_id.
type PostgresSearcher struct {
	pool *pgxpool.Pool
}

// NewPostgresSearcher constructs a searcher backed by the supplied pool.
// pool MUST NOT be nil.
func NewPostgresSearcher(pool *pgxpool.Pool) *PostgresSearcher {
	if pool == nil {
		panic("retrieve.NewPostgresSearcher: pool is required")
	}
	return &PostgresSearcher{pool: pool}
}

// SearchDrive implements Searcher.
func (p *PostgresSearcher) SearchDrive(ctx context.Context, req RetrieveRequest) ([]RetrieveCandidate, error) {
	if p == nil || p.pool == nil {
		return nil, errors.New("retrieve: postgres searcher not configured")
	}
	query := strings.TrimSpace(req.Query)
	if query == "" {
		return nil, errors.New("retrieve: empty search query")
	}
	likePattern := "%" + query + "%"

	limit := req.Limit
	if limit <= 0 {
		limit = DefaultLimit
	}

	rows, err := p.pool.Query(ctx, `
		SELECT a.id,
		       a.title,
		       COALESCE(array_to_string(f.folder_path, '/'), '') AS folder,
		       f.sensitivity,
		       f.size_bytes,
		       c.provider_id,
		       f.provider_url
		  FROM artifacts a
		  JOIN drive_files f ON f.artifact_id = a.id
		  JOIN drive_connections c ON c.id = f.connection_id
		 WHERE a.artifact_type = 'drive_file'
		   AND f.tombstoned_at IS NULL
		   AND f.permission_lost_at IS NULL
		   AND (
		         a.title ILIKE $1
		      OR COALESCE(a.content_raw, '') ILIKE $1
		      OR EXISTS (
		           SELECT 1 FROM unnest(f.folder_path) AS p WHERE p ILIKE $1
		         )
		   )
		 ORDER BY a.updated_at DESC
		 LIMIT $2
	`, likePattern, limit)
	if err != nil {
		return nil, fmt.Errorf("retrieve: search query: %w", err)
	}
	defer rows.Close()

	out := make([]RetrieveCandidate, 0, limit)
	for rows.Next() {
		var c RetrieveCandidate
		if err := rows.Scan(
			&c.ArtifactID,
			&c.Title,
			&c.Folder,
			&c.Sensitivity,
			&c.SizeBytes,
			&c.Provider,
			&c.ProviderURL,
		); err != nil {
			return nil, fmt.Errorf("retrieve: scan candidate: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("retrieve: iterate candidates: %w", err)
	}
	return out, nil
}

// ProviderGetFileFunc is the abstract provider-bytes lookup the
// ProviderBytesFetcher delegates to. Production wires this to a closure
// that resolves the provider through drive.DefaultRegistry; tests
// substitute fakes directly.
//
// The function MUST stream and MUST return the same MimeType the
// drive.Provider would. Caller is responsible for closing the returned
// io.ReadCloser.
type ProviderGetFileFunc func(ctx context.Context, providerID, connectionID, providerFileID string) (io.ReadCloser, string, error)

// ProviderBytesFetcher resolves an artifact id to its provider connection
// and provider-specific file id via Postgres, then fetches the bytes
// through the supplied ProviderGetFileFunc. The fetcher is provider-
// neutral by construction: it never branches on the provider type.
type ProviderBytesFetcher struct {
	pool    *pgxpool.Pool
	getFile ProviderGetFileFunc
}

// NewProviderBytesFetcher constructs a BytesFetcher. Both arguments are
// required; passing nil for either panics so the runtime fails loud.
func NewProviderBytesFetcher(pool *pgxpool.Pool, getFile ProviderGetFileFunc) *ProviderBytesFetcher {
	if pool == nil {
		panic("retrieve.NewProviderBytesFetcher: pool is required")
	}
	if getFile == nil {
		panic("retrieve.NewProviderBytesFetcher: getFile is required")
	}
	return &ProviderBytesFetcher{pool: pool, getFile: getFile}
}

// GetArtifactBytes implements BytesFetcher.
func (f *ProviderBytesFetcher) GetArtifactBytes(ctx context.Context, artifactID string) ([]byte, string, error) {
	if f == nil || f.pool == nil || f.getFile == nil {
		return nil, "", errors.New("retrieve: bytes fetcher not configured")
	}
	var (
		connectionID   string
		providerFileID string
		providerID     string
	)
	err := f.pool.QueryRow(ctx, `
		SELECT f.connection_id, f.provider_file_id, c.provider_id
		  FROM drive_files f
		  JOIN drive_connections c ON c.id = f.connection_id
		 WHERE f.artifact_id = $1
	`, artifactID).Scan(&connectionID, &providerFileID, &providerID)
	if err != nil {
		return nil, "", fmt.Errorf("retrieve: lookup drive_file: %w", err)
	}
	reader, mime, err := f.getFile(ctx, providerID, connectionID, providerFileID)
	if err != nil {
		return nil, "", fmt.Errorf("retrieve: provider GetFile: %w", err)
	}
	if reader == nil {
		return nil, "", errors.New("retrieve: provider returned nil reader")
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, "", fmt.Errorf("retrieve: read provider bytes: %w", err)
	}
	return data, mime, nil
}
