package save

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/drive/rules"
)

// FolderStore abstracts the persistence behavior the FolderResolver needs.
// Production code uses pgFolderStore (drive_folder_resolutions table); the
// unit test for SCN-038-015 uses an in-memory store so concurrent-resolution
// behavior can be verified without a DB. The integration canary test
// (TestDriveSaveCanary_IdempotentFolderResolutionAndGraphLinks) validates
// the same contract end-to-end against PostgreSQL.
type FolderStore interface {
	// Lookup returns the existing provider_folder_id for a (connectionID,
	// folderPath) tuple. An empty string with a nil error MUST mean "no
	// mapping yet" and lets the caller fall through to TryInsert.
	Lookup(ctx context.Context, connectionID, folderPath string) (string, error)
	// TryInsert atomically inserts the (connectionID, folderPath,
	// providerFolderID) tuple or returns the winning provider_folder_id
	// when a concurrent insert wrote first. Implementations MUST NOT
	// return an error on conflict; that is the success path of the
	// "exactly one mapping" contract.
	TryInsert(ctx context.Context, connectionID, providerID, folderPath, providerFolderID, requestID string) (winningProviderFolderID string, err error)
}

// FolderResolver coordinates folder lookup, provider EnsureFolder, and the
// transactional unique insert into drive_folder_resolutions. It is the
// only path the Save Service uses to map a logical folder path to a
// provider folder id.
type FolderResolver struct {
	store   FolderStore
	ensurer FolderEnsurer
	mu      sync.Mutex // guards in-process call coalescing per (conn,path)
	calls   map[string]*folderCall
}

type folderCall struct {
	wg     sync.WaitGroup
	result string
	err    error
}

// NewFolderResolver constructs a FolderResolver.
func NewFolderResolver(store FolderStore, ensurer FolderEnsurer) *FolderResolver {
	return &FolderResolver{store: store, ensurer: ensurer, calls: make(map[string]*folderCall)}
}

// Resolve returns the provider folder id for the requested path. Concurrent
// callers asking for the same (connectionID, folderPath) coalesce inside the
// process, then fall through to FolderStore.TryInsert which is responsible
// for the durable "exactly one mapping" contract via a unique constraint.
func (r *FolderResolver) Resolve(ctx context.Context, connectionID, providerID, folderPath, requestID string, onMissing rules.OnMissingFolder) (string, error) {
	cleaned := strings.Trim(folderPath, "/")
	if cleaned == "" {
		return "", errors.New("save: empty folder path")
	}
	if existing, err := r.store.Lookup(ctx, connectionID, cleaned); err != nil {
		return "", err
	} else if existing != "" {
		return existing, nil
	}

	key := connectionID + "\x00" + cleaned
	r.mu.Lock()
	if call, ok := r.calls[key]; ok {
		r.mu.Unlock()
		call.wg.Wait()
		return call.result, call.err
	}
	call := &folderCall{}
	call.wg.Add(1)
	r.calls[key] = call
	r.mu.Unlock()

	defer func() {
		call.wg.Done()
		r.mu.Lock()
		delete(r.calls, key)
		r.mu.Unlock()
	}()

	if r.ensurer == nil {
		if onMissing == rules.OnMissingFail {
			call.err = fmt.Errorf("save: folder %q missing and on_missing_folder=fail", cleaned)
			return "", call.err
		}
		call.err = fmt.Errorf("save: provider does not implement FolderEnsurer (path=%q)", cleaned)
		return "", call.err
	}
	providerFolderID, err := r.ensurer.EnsureFolder(ctx, connectionID, cleaned)
	if err != nil {
		call.err = fmt.Errorf("save: ensure provider folder: %w", err)
		return "", call.err
	}
	winning, err := r.store.TryInsert(ctx, connectionID, providerID, cleaned, providerFolderID, requestID)
	if err != nil {
		call.err = err
		return "", err
	}
	call.result = winning
	return winning, nil
}

// pgFolderStore is the production FolderStore backed by drive_folder_resolutions.
type pgFolderStore struct {
	pool *pgxpool.Pool
}

// NewPostgresFolderStore returns a FolderStore that talks to the
// drive_folder_resolutions table.
func NewPostgresFolderStore(pool *pgxpool.Pool) FolderStore {
	return &pgFolderStore{pool: pool}
}

// Lookup implements FolderStore.
func (s *pgFolderStore) Lookup(ctx context.Context, connectionID, folderPath string) (string, error) {
	var providerFolderID string
	err := s.pool.QueryRow(ctx,
		`SELECT provider_folder_id FROM drive_folder_resolutions
		  WHERE connection_id=$1 AND folder_path=$2`, connectionID, folderPath,
	).Scan(&providerFolderID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("save: lookup folder resolution: %w", err)
	}
	return providerFolderID, nil
}

// TryInsert implements FolderStore. Concurrent inserts of the same
// (connection_id, folder_path) tuple all see exactly one winning row thanks
// to the migration-021 UNIQUE constraint; losers re-read and return the
// winning provider_folder_id.
func (s *pgFolderStore) TryInsert(ctx context.Context, connectionID, providerID, folderPath, providerFolderID, requestID string) (string, error) {
	tag, err := s.pool.Exec(ctx,
		`INSERT INTO drive_folder_resolutions
		     (id, connection_id, provider_id, folder_path, provider_folder_id, created_by_request_id)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (connection_id, folder_path) DO NOTHING`,
		uuid.NewString(), connectionID, providerID, folderPath, providerFolderID, nullableUUID(requestID),
	)
	if err != nil {
		return "", fmt.Errorf("save: insert folder resolution: %w", err)
	}
	if tag.RowsAffected() == 1 {
		return providerFolderID, nil
	}
	return s.Lookup(ctx, connectionID, folderPath)
}
