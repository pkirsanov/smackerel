// Package save implements the Spec 038 Scope 5 Save Service.
//
// The Save Service consumes Save Rule decisions, resolves the target folder
// inside a transactional `drive_folder_resolutions` table, and calls the
// provider's PutFile to write the artifact bytes. It is responsible for:
//
//   - Idempotency (drive_save_requests.idempotency_key)
//   - Concurrent-safe folder resolution (BS-016)
//   - On-existing-file policy enforcement
//   - Failure tracking (attempts + last_error)
//   - Source artifact graph linking (edges table, edge_type='drive_save')
//
// The package is provider-neutral — the only contract it depends on is
// drive.Provider (for PutFile + ListFolder). Tests substitute a fixture
// provider directly without modifying anything in this package.
package save

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/drive"
	"github.com/smackerel/smackerel/internal/drive/rules"
)

// ErrReadOnlyConnection is returned when the resolved drive_connection has
// access_mode=read_only. The Save Service refuses to proceed because the
// provider PutFile would also refuse.
var ErrReadOnlyConnection = errors.New("save: connection is read-only")

// ErrConnectionMissing is returned when no drive_connections row exists for
// the resolved provider.
var ErrConnectionMissing = errors.New("save: no drive connection for provider")

// ErrInvalidTokenInTemplate wraps rules.ErrInvalidToken with a save-service
// label so callers can distinguish render failures from provider failures.
var ErrInvalidTokenInTemplate = errors.New("save: rule template invalid token")

// ProviderResolver returns the drive.Provider registered under providerID.
type ProviderResolver interface {
	Get(id string) (drive.Provider, bool)
}

// FolderEnsurer is the provider behavior the Save Service needs in order to
// resolve a folder path to a provider folder id. The Spec 038 design uses
// the existing drive.Provider.ListFolder + a provider-side conditional
// "create folder" call. Because drive.Provider does not yet expose a
// CreateFolder method, the save service relies on the FolderEnsurer
// auxiliary interface, optionally implemented by the provider.
type FolderEnsurer interface {
	EnsureFolder(ctx context.Context, connectionID string, folderPath string) (providerFolderID string, err error)
}

// Bytes is the payload the save service writes through PutFile. Title is the
// final filename inside the resolved folder; MimeType, Body, and Size mirror
// drive.FileBytes.
type Bytes struct {
	Title    string
	MimeType string
	Body     []byte
}

// Request is the input to Service.Save.
type Request struct {
	Rule             rules.Rule
	SourceArtifactID string
	Bytes            Bytes
	// RenderedPath is the resolved target path produced by the rule
	// engine. The save service treats it as the folder path; Title is
	// the file name inside that folder.
	RenderedPath string
	ConnectionID string
	// ConfirmRequired marks the request as awaiting user confirmation.
	// The save service records the request as pending+awaiting but does
	// not call PutFile.
	ConfirmRequired bool
}

// Result describes the outcome of Service.Save.
type Result struct {
	RequestID      string
	Status         Status
	IdempotencyKey string
	TargetPath     string
	TargetFolderID string
	ProviderFileID string
	ProviderURL    string
	Attempts       int
	LastError      string
}

// Status mirrors drive_save_requests.status.
type Status string

const (
	StatusPending              Status = "pending"
	StatusWritten              Status = "written"
	StatusSkipped              Status = "skipped"
	StatusFailed               Status = "failed"
	StatusAwaitingConfirmation Status = "awaiting_confirmation"
)

// Service orchestrates Save Rule execution.
type Service struct {
	pool      *pgxpool.Pool
	registry  ProviderResolver
	urlPrefix string
	resolver  *FolderResolver
}

// NewService constructs a Save Service.
//
//   - pool      — Postgres pool for drive_save_requests, drive_folder_resolutions, edges, and drive_files lookups.
//   - registry  — drive.ProviderResolver (typically drive.DefaultRegistry).
//   - urlPrefix — provider-neutral URL prefix appended to "/{folderID}/{title}" when
//     the provider does not return a webViewLink (used by the fixture).
func NewService(pool *pgxpool.Pool, registry ProviderResolver, urlPrefix string) *Service {
	return &Service{pool: pool, registry: registry, urlPrefix: strings.TrimRight(urlPrefix, "/")}
}

// IdempotencyKey returns the deterministic save-request key for the supplied
// inputs (per design.md §5.1). Hash inputs MUST stay stable across runs so
// two identical save requests collapse to one row.
func IdempotencyKey(ruleID, sourceArtifactID, targetPath string) string {
	h := sha256.New()
	h.Write([]byte(ruleID))
	h.Write([]byte("\x00"))
	h.Write([]byte(sourceArtifactID))
	h.Write([]byte("\x00"))
	h.Write([]byte(targetPath))
	return hex.EncodeToString(h.Sum(nil))
}

// resolveConnection returns the connection_id and access_mode for the rule's
// provider. When req.ConnectionID is non-empty, the service prefers it; the
// row MUST exist and MUST belong to the rule's provider. Otherwise the
// service picks the most recent healthy connection for the provider.
func (s *Service) resolveConnection(ctx context.Context, req Request) (string, drive.AccessMode, error) {
	if req.ConnectionID != "" {
		var (
			providerID string
			accessMode string
		)
		err := s.pool.QueryRow(ctx,
			`SELECT provider_id, access_mode FROM drive_connections WHERE id=$1`, req.ConnectionID,
		).Scan(&providerID, &accessMode)
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", ErrConnectionMissing
		}
		if err != nil {
			return "", "", fmt.Errorf("save: lookup connection: %w", err)
		}
		if providerID != req.Rule.ProviderID {
			return "", "", fmt.Errorf("save: connection %s belongs to provider %q, rule wants %q", req.ConnectionID, providerID, req.Rule.ProviderID)
		}
		return req.ConnectionID, drive.AccessMode(accessMode), nil
	}
	var (
		connectionID string
		accessMode   string
	)
	err := s.pool.QueryRow(ctx,
		`SELECT id, access_mode FROM drive_connections WHERE provider_id=$1 AND status IN ('healthy','degraded') ORDER BY updated_at DESC LIMIT 1`,
		req.Rule.ProviderID,
	).Scan(&connectionID, &accessMode)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", ErrConnectionMissing
	}
	if err != nil {
		return "", "", fmt.Errorf("save: lookup connection by provider: %w", err)
	}
	return connectionID, drive.AccessMode(accessMode), nil
}

// Save executes one Save Rule decision.
//
// The flow follows design.md §5.3:
//  1. Compute idempotency key.
//  2. Look up drive_save_requests by key. If status='written', return existing.
//  3. Insert a new pending row (or pick up an existing pending row).
//  4. Resolve connection (must be read_save).
//  5. EnsureFolder transactionally (drive_folder_resolutions unique by (connection_id,folder_path)).
//  6. PutFile (or no-op if ConfirmRequired).
//  7. Update drive_save_requests with provider_file_id + provider_url + status.
//  8. Insert edges row (artifact -> drive_file) for graph traversal.
func (s *Service) Save(ctx context.Context, req Request) (Result, error) {
	if s == nil || s.pool == nil || s.registry == nil {
		return Result{}, errors.New("save: service not wired (pool/registry)")
	}
	if req.SourceArtifactID == "" {
		return Result{}, errors.New("save: source_artifact_id required")
	}
	if req.Rule.ID == "" {
		return Result{}, errors.New("save: rule.ID required")
	}
	if req.RenderedPath == "" {
		return Result{}, errors.New("save: rendered_path required")
	}
	if req.Bytes.Title == "" {
		return Result{}, errors.New("save: bytes.title required")
	}

	provider, ok := s.registry.Get(req.Rule.ProviderID)
	if !ok {
		return Result{}, fmt.Errorf("save: provider %q not registered", req.Rule.ProviderID)
	}

	targetPath := req.RenderedPath + "/" + req.Bytes.Title
	idempotencyKey := IdempotencyKey(req.Rule.ID, req.SourceArtifactID, targetPath)

	if existing, ok, err := s.loadByIdempotencyKey(ctx, idempotencyKey); err != nil {
		return Result{}, err
	} else if ok && existing.Status == StatusWritten {
		return existing, nil
	}

	connectionID, accessMode, err := s.resolveConnection(ctx, req)
	if err != nil {
		return Result{}, err
	}

	requestID, attempts, lastError, _, err := s.upsertSaveRequest(ctx, req, connectionID, targetPath, idempotencyKey)
	if err != nil {
		return Result{}, err
	}

	if req.ConfirmRequired {
		_ = s.markAwaitingConfirmation(ctx, requestID)
		return Result{
			RequestID:      requestID,
			Status:         StatusAwaitingConfirmation,
			IdempotencyKey: idempotencyKey,
			TargetPath:     targetPath,
			Attempts:       attempts,
			LastError:      lastError,
		}, nil
	}

	if accessMode != drive.AccessReadSave {
		failErr := s.markFailed(ctx, requestID, ErrReadOnlyConnection.Error())
		if failErr != nil {
			return Result{}, failErr
		}
		return Result{
			RequestID:      requestID,
			Status:         StatusFailed,
			IdempotencyKey: idempotencyKey,
			TargetPath:     targetPath,
			LastError:      ErrReadOnlyConnection.Error(),
			Attempts:       attempts + 1,
		}, ErrReadOnlyConnection
	}

	folderID, err := s.resolveFolder(ctx, provider, connectionID, req.Rule.ProviderID, req.RenderedPath, req.Rule.OnMissingFolder, requestID)
	if err != nil {
		_ = s.markFailed(ctx, requestID, err.Error())
		return Result{
			RequestID:      requestID,
			Status:         StatusFailed,
			IdempotencyKey: idempotencyKey,
			TargetPath:     targetPath,
			LastError:      err.Error(),
			Attempts:       attempts + 1,
		}, err
	}

	providerFileID, err := provider.PutFile(ctx, connectionID, folderID, req.Bytes.Title, drive.FileBytes{
		MimeType: req.Bytes.MimeType,
		Reader:   newBytesReadCloser(req.Bytes.Body),
		Size:     int64(len(req.Bytes.Body)),
	})
	if err != nil {
		_ = s.markFailed(ctx, requestID, err.Error())
		return Result{
			RequestID:      requestID,
			Status:         StatusFailed,
			IdempotencyKey: idempotencyKey,
			TargetPath:     targetPath,
			TargetFolderID: folderID,
			LastError:      err.Error(),
			Attempts:       attempts + 1,
		}, err
	}

	providerURL := s.composeProviderURL(req.Rule.ProviderID, folderID, providerFileID, req.Bytes.Title)
	if err := s.markWritten(ctx, requestID, connectionID, req.Rule.ProviderID, providerFileID, providerURL, folderID); err != nil {
		return Result{}, err
	}
	if err := s.linkArtifactGraph(ctx, req.SourceArtifactID, requestID, providerFileID); err != nil {
		// Linking failure does not roll back the save — the provider file
		// already exists. The error is surfaced so callers/log lines see it.
		return Result{
			RequestID:      requestID,
			Status:         StatusWritten,
			IdempotencyKey: idempotencyKey,
			TargetPath:     targetPath,
			TargetFolderID: folderID,
			ProviderFileID: providerFileID,
			ProviderURL:    providerURL,
			Attempts:       attempts + 1,
			LastError:      "graph_link_failed: " + err.Error(),
		}, nil
	}
	return Result{
		RequestID:      requestID,
		Status:         StatusWritten,
		IdempotencyKey: idempotencyKey,
		TargetPath:     targetPath,
		TargetFolderID: folderID,
		ProviderFileID: providerFileID,
		ProviderURL:    providerURL,
		Attempts:       attempts + 1,
	}, nil
}

func (s *Service) loadByIdempotencyKey(ctx context.Context, key string) (Result, bool, error) {
	var (
		requestID      string
		statusValue    string
		targetPath     string
		providerFileID string
		providerURL    string
		folderID       string
		attempts       int
		lastError      string
	)
	err := s.pool.QueryRow(ctx,
		`SELECT id, status, target_path, COALESCE(provider_file_id,''), COALESCE(provider_url,''),
		        COALESCE(target_folder_id,''), attempts, COALESCE(last_error,'')
		   FROM drive_save_requests WHERE idempotency_key=$1`, key,
	).Scan(&requestID, &statusValue, &targetPath, &providerFileID, &providerURL, &folderID, &attempts, &lastError)
	if errors.Is(err, pgx.ErrNoRows) {
		return Result{}, false, nil
	}
	if err != nil {
		return Result{}, false, fmt.Errorf("save: lookup idempotency: %w", err)
	}
	return Result{
		RequestID:      requestID,
		Status:         Status(statusValue),
		IdempotencyKey: key,
		TargetPath:     targetPath,
		TargetFolderID: folderID,
		ProviderFileID: providerFileID,
		ProviderURL:    providerURL,
		Attempts:       attempts,
		LastError:      lastError,
	}, true, nil
}

func (s *Service) upsertSaveRequest(ctx context.Context, req Request, connectionID, targetPath, idempotencyKey string) (string, int, string, Status, error) {
	id := uuid.NewString()
	return s.upsertSaveRequestRobust(ctx, req, connectionID, targetPath, idempotencyKey, id)
}

func (s *Service) upsertSaveRequestRobust(ctx context.Context, req Request, connectionID, targetPath, idempotencyKey, id string) (string, int, string, Status, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", 0, "", "", fmt.Errorf("save: begin upsert tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var (
		existingID       string
		existingStat     string
		existingAttempts int
		existingErr      string
	)
	err = tx.QueryRow(ctx,
		`SELECT id, status, attempts, COALESCE(last_error,'') FROM drive_save_requests WHERE idempotency_key=$1 FOR UPDATE`,
		idempotencyKey,
	).Scan(&existingID, &existingStat, &existingAttempts, &existingErr)
	if err == nil {
		// Existing row — bump attempts and reset status to pending so we retry.
		_, err = tx.Exec(ctx,
			`UPDATE drive_save_requests SET attempts=attempts+1, status='pending', last_error='' WHERE id=$1`,
			existingID,
		)
		if err != nil {
			return "", 0, "", "", fmt.Errorf("save: bump attempts: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return "", 0, "", "", err
		}
		return existingID, existingAttempts, existingErr, StatusPending, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", 0, "", "", fmt.Errorf("save: lookup existing: %w", err)
	}
	_, err = tx.Exec(ctx,
		`INSERT INTO drive_save_requests
		     (id, rule_id, source_artifact_id, target_path, idempotency_key, status,
		      attempts, connection_id, provider_id)
		 VALUES ($1, $2, $3, $4, $5, 'pending', 0, $6, $7)`,
		id, req.Rule.ID, req.SourceArtifactID, targetPath, idempotencyKey, connectionID, req.Rule.ProviderID,
	)
	if err != nil {
		if isUniqueViolation(err) {
			// Concurrent insert won the race. Roll back and read the
			// winner's row in a fresh transaction so caller observes
			// the canonical drive_save_requests entry instead of
			// duplicating it.
			_ = tx.Rollback(ctx)
			var (
				winnerID       string
				winnerStat     string
				winnerAttempts int
				winnerErr      string
			)
			rerr := s.pool.QueryRow(ctx,
				`SELECT id, status, attempts, COALESCE(last_error,'') FROM drive_save_requests WHERE idempotency_key=$1`,
				idempotencyKey,
			).Scan(&winnerID, &winnerStat, &winnerAttempts, &winnerErr)
			if rerr != nil {
				return "", 0, "", "", fmt.Errorf("save: read winner after race: %w", rerr)
			}
			return winnerID, winnerAttempts, winnerErr, Status(winnerStat), nil
		}
		return "", 0, "", "", fmt.Errorf("save: insert pending: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return "", 0, "", "", err
	}
	return id, 0, "", StatusPending, nil
}

// isUniqueViolation reports whether err is a Postgres unique-constraint
// violation (SQLSTATE 23505).
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}

func (s *Service) markFailed(ctx context.Context, requestID, errorMsg string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE drive_save_requests SET status='failed', last_error=$2, completed_at=$3 WHERE id=$1`,
		requestID, errorMsg, time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("save: mark failed: %w", err)
	}
	return nil
}

func (s *Service) markWritten(ctx context.Context, requestID, connectionID, providerID, providerFileID, providerURL, folderID string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE drive_save_requests
		    SET status='written', last_error='', provider_file_id=$2, provider_url=$3,
		        target_folder_id=$4, connection_id=$5, provider_id=$6, completed_at=$7
		  WHERE id=$1`,
		requestID, providerFileID, providerURL, folderID, connectionID, providerID, time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("save: mark written: %w", err)
	}
	return nil
}

func (s *Service) markAwaitingConfirmation(ctx context.Context, requestID string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE drive_save_requests SET status='awaiting_confirmation' WHERE id=$1`,
		requestID,
	)
	if err != nil {
		return fmt.Errorf("save: mark awaiting confirmation: %w", err)
	}
	return nil
}

func (s *Service) linkArtifactGraph(ctx context.Context, sourceArtifactID, requestID, providerFileID string) error {
	id := "edge-drive-save-" + requestID
	_, err := s.pool.Exec(ctx,
		`INSERT INTO edges (id, src_type, src_id, dst_type, dst_id, edge_type, weight, metadata)
		 VALUES ($1, 'artifact', $2, 'drive_file', $3, 'drive_save', 1.0, jsonb_build_object('save_request_id', $4::text))
		 ON CONFLICT (src_type, src_id, dst_type, dst_id, edge_type) DO NOTHING`,
		id, sourceArtifactID, providerFileID, requestID,
	)
	if err != nil {
		return fmt.Errorf("save: link artifact graph: %w", err)
	}
	return nil
}

func (s *Service) composeProviderURL(providerID, folderID, fileID, title string) string {
	prefix := s.urlPrefix
	if prefix == "" {
		prefix = "drive://" + providerID
	}
	return prefix + "/file/" + folderID + "/" + fileID
}

// resolveFolder ensures the folder exists in drive_folder_resolutions and on
// the provider, returning the provider folder id. Concurrent calls for the
// same path collapse to one mapping (BS-016) — the per-process FolderResolver
// guarantees in-process coalescing, and the unique (connection_id,
// folder_path) constraint guarantees cross-process exactly-one mapping.
func (s *Service) resolveFolder(ctx context.Context, provider drive.Provider, connectionID, providerID, folderPath string, onMissing rules.OnMissingFolder, requestID string) (string, error) {
	if s.resolver == nil {
		ensurer, _ := provider.(FolderEnsurer)
		s.resolver = NewFolderResolver(NewPostgresFolderStore(s.pool), ensurer)
	}
	return s.resolver.Resolve(ctx, connectionID, providerID, folderPath, requestID, onMissing)
}
