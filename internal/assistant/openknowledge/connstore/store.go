// Package connstore — Spec 096 SCOPE-06: the RUNTIME (DB) plane of the
// operator-global multi-provider model connections.
//
// Store owns the `model_provider_connections` runtime rows (migration 061) for
// the db-mode connection slots the SCOPE-01 SST registry declares. It is the
// SINGLE seam SCOPE-03 dispatch and SCOPE-04 discovery consult through one
// predicate — EffectiveEnabled — so a connection is dispatchable / discoverable
// iff it is registry-declared AND DB-enabled AND its last test outcome is `ok`
// AND a credential is present (design §5.1).
//
// # CredentialSource (SCOPE-03)
//
// Store implements the SCOPE-03 `llm.CredentialSource`
// (`Credential(connID) (connvault.VaultRecord, bool)`) structurally: Credential
// returns the at-rest record ONLY for an effective-enabled connection. A DB
// error, an absent row, a non-effective-enabled connection, or a missing
// credential all map to `found=false` — fail-closed, so the resolver rejects
// the target and NEVER falls back to the local Ollama connection (FR-X1 /
// SCN-096-G01).
//
// # Write-only secret (design §6.1, §11.5)
//
// The only plaintext entry point is the admin handler's `PUT …/credential`,
// which hands Store an already-encrypted `connvault.VaultRecord` via
// UpsertCredential. No Store method returns, logs, or echoes the plaintext
// credential; the credential is recoverable only via the in-core vault Decrypt
// the dispatch resolver runs. The redaction (last-4) and the typed last-test
// state are the only secret-adjacent values any read surface exposes.
package connstore

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/assistant/openknowledge/connvault"
	"github.com/smackerel/smackerel/internal/config"
)

// Closed last_test_outcome vocabulary (migration 061 CHECK constraint).
const (
	TestOutcomeOK     = "ok"
	TestOutcomeFailed = "failed"
)

// credentialReadTimeout bounds the synchronous DB read the CredentialSource
// performs on the dispatch hot path. It is an internal safety bound for the
// context-free SCOPE-03 `Credential` interface, NOT an operator-tunable SST
// value (mirrors the in-code tuning constants the open-knowledge wiring uses);
// a fail-closed `found=false` is returned if it elapses.
const credentialReadTimeout = 5 * time.Second

// Record is the runtime (DB) row for one connection slot. Secret is non-nil
// only when a credential has been stored; it carries the at-rest VaultRecord
// (ciphertext + nonce + key_version + last-4 redaction) — NEVER plaintext.
type Record struct {
	ConnectionID    string
	ProviderKind    string
	Enabled         bool
	Secret          *connvault.VaultRecord
	LastTestedAt    *time.Time
	LastTestOutcome string
	LastTestDetail  string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// HasCredential reports whether an at-rest credential is stored (the
// ciphertext + nonce are present). The redaction/last-test columns are not
// part of this predicate — only the recoverable cipher material is.
func (r Record) HasCredential() bool {
	return r.Secret != nil && len(r.Secret.Ciphertext) > 0 && len(r.Secret.Nonce) > 0
}

// EffectiveEnabled is THE single gate SCOPE-03 dispatch and SCOPE-04 discovery
// consult for a db-mode connection (design §5.1): registry-declared AND DB
// `enabled` AND `last_test_outcome = 'ok'` AND a credential present. A
// connection that is not effective-enabled is simply ABSENT from the catalog
// (graceful degradation) and is NEVER dispatched (never a silent Ollama
// fallback) — the SAME predicate on both planes.
func EffectiveEnabled(rec Record, registryDeclared bool) bool {
	return registryDeclared &&
		rec.Enabled &&
		rec.LastTestOutcome == TestOutcomeOK &&
		rec.HasCredential()
}

// Store is the pgx-backed runtime-plane store for db-mode connection slots.
type Store struct {
	pool     *pgxpool.Pool
	registry map[string]config.ModelConnection // every SST-declared slot, keyed by id
	now      func() time.Time
}

// NewStore builds the runtime store from the live pool and the SST-declared
// connections (the closed-set slot registry — the admin surface refuses any id
// absent from it). The registry includes ALL declared kinds; only db-mode
// slots ever acquire a DB row.
func NewStore(pool *pgxpool.Pool, conns []config.ModelConnection) *Store {
	reg := make(map[string]config.ModelConnection, len(conns))
	for _, c := range conns {
		reg[c.ID] = c
	}
	return &Store{pool: pool, registry: reg, now: time.Now}
}

// WithNow overrides the clock (tests assert app-written timestamps). Returns
// the receiver for chaining.
func (s *Store) WithNow(now func() time.Time) *Store {
	s.now = now
	return s
}

// Connection returns the SST-declared registry connection for id. found=false
// means the id is not a declared slot (the admin surface answers 404 — a
// brand-new kind is an SST topology edit, not a UI invention).
func (s *Store) Connection(id string) (config.ModelConnection, bool) {
	c, ok := s.registry[id]
	return c, ok
}

// ListDeclared returns every SST-declared connection slot (the closed registry
// set). The admin list surface overlays these with their runtime rows so a
// declared-but-unconfigured slot is still shown.
func (s *Store) ListDeclared() []config.ModelConnection {
	out := make([]config.ModelConnection, 0, len(s.registry))
	for _, c := range s.registry {
		out = append(out, c)
	}
	return out
}

const selectColumns = `
	connection_id, provider_kind, enabled,
	secret_ciphertext, secret_nonce, secret_key_version, secret_redaction,
	last_tested_at, last_test_outcome, last_test_detail,
	created_at, updated_at
`

// Get reads the runtime row for connID. found=false means no row exists yet —
// a declared-but-unconfigured slot (no credential stored).
func (s *Store) Get(ctx context.Context, connID string) (Record, bool, error) {
	if s == nil || s.pool == nil {
		return Record{}, false, errors.New("connstore: Store requires a non-nil Pool")
	}
	row := s.pool.QueryRow(ctx, `SELECT `+selectColumns+` FROM model_provider_connections WHERE connection_id = $1`, connID)
	rec, err := scanRecord(row)
	if errors.Is(err, errNoRow) {
		return Record{}, false, nil
	}
	if err != nil {
		return Record{}, false, fmt.Errorf("connstore: read connection %q: %w", connID, err)
	}
	return rec, true, nil
}

// List reads every runtime row (the configured slots). It does NOT synthesize
// rows for declared-but-unconfigured slots; the admin handler overlays the SST
// registry to present every declared slot, configured or not.
func (s *Store) List(ctx context.Context) ([]Record, error) {
	if s == nil || s.pool == nil {
		return nil, errors.New("connstore: Store requires a non-nil Pool")
	}
	rows, err := s.pool.Query(ctx, `SELECT `+selectColumns+` FROM model_provider_connections ORDER BY connection_id`)
	if err != nil {
		return nil, fmt.Errorf("connstore: list connections: %w", err)
	}
	defer rows.Close()
	var out []Record
	for rows.Next() {
		rec, err := scanRecordRows(rows)
		if err != nil {
			return nil, fmt.Errorf("connstore: scan connection row: %w", err)
		}
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("connstore: iterate connection rows: %w", err)
	}
	return out, nil
}

const upsertCredentialSQL = `
INSERT INTO model_provider_connections
	(connection_id, provider_kind, enabled,
	 secret_ciphertext, secret_nonce, secret_key_version, secret_redaction,
	 last_tested_at, last_test_outcome, last_test_detail,
	 created_at, updated_at)
VALUES ($1, $2, false, $3, $4, $5, $6, NULL, NULL, NULL, $7, $7)
ON CONFLICT (connection_id) DO UPDATE SET
	secret_ciphertext  = EXCLUDED.secret_ciphertext,
	secret_nonce       = EXCLUDED.secret_nonce,
	secret_key_version = EXCLUDED.secret_key_version,
	secret_redaction   = EXCLUDED.secret_redaction,
	last_tested_at     = NULL,
	last_test_outcome  = NULL,
	last_test_detail   = NULL,
	updated_at         = EXCLUDED.updated_at
`

// UpsertCredential write-only-stores the encrypted credential for a db-mode
// slot, creating the row if absent (enabled=false on create — the operator
// MUST test then enable). Storing a NEW credential RESETS the last-test state
// (a rotated key invalidates the prior probe), so the slot leaves the catalog
// until it is re-tested `ok`. It persists ONLY the at-rest cipher material +
// last-4 redaction — never the plaintext.
func (s *Store) UpsertCredential(ctx context.Context, connID, kind string, rec connvault.VaultRecord) error {
	if s == nil || s.pool == nil {
		return errors.New("connstore: Store requires a non-nil Pool")
	}
	if len(rec.Ciphertext) == 0 || len(rec.Nonce) == 0 || rec.KeyVersion <= 0 {
		return fmt.Errorf("connstore: refusing to store an incomplete vault record for connection %q", connID)
	}
	now := s.now().UTC()
	if _, err := s.pool.Exec(ctx, upsertCredentialSQL,
		connID, kind, rec.Ciphertext, rec.Nonce, rec.KeyVersion, rec.Redaction, now); err != nil {
		return fmt.Errorf("connstore: store credential for connection %q: %w", connID, err)
	}
	return nil
}

const recordTestSQL = `
INSERT INTO model_provider_connections
	(connection_id, provider_kind, enabled,
	 last_tested_at, last_test_outcome, last_test_detail,
	 created_at, updated_at)
VALUES ($1, $2, false, $3, $4, $5, $6, $6)
ON CONFLICT (connection_id) DO UPDATE SET
	last_tested_at    = EXCLUDED.last_tested_at,
	last_test_outcome = EXCLUDED.last_test_outcome,
	last_test_detail  = EXCLUDED.last_test_detail,
	updated_at        = EXCLUDED.updated_at
`

// RecordTest persists the TRUTHFUL typed test outcome (`ok` | `failed`) + its
// typed detail + tested-at instant. It NEVER persists the secret — detail is a
// typed reason only (`auth_failed` | `unreachable` | `timeout` | ""). A failed
// probe persists `failed`, so the 409 enable-guard refuses the slot.
func (s *Store) RecordTest(ctx context.Context, connID, kind, outcome, detail string, at time.Time) error {
	if s == nil || s.pool == nil {
		return errors.New("connstore: Store requires a non-nil Pool")
	}
	if outcome != TestOutcomeOK && outcome != TestOutcomeFailed {
		return fmt.Errorf("connstore: refusing to persist unknown test outcome %q for connection %q", outcome, connID)
	}
	if _, err := s.pool.Exec(ctx, recordTestSQL, connID, kind, at.UTC(), outcome, detail, s.now().UTC()); err != nil {
		return fmt.Errorf("connstore: record test outcome for connection %q: %w", connID, err)
	}
	return nil
}

const setEnabledSQL = `
UPDATE model_provider_connections SET enabled = $2, updated_at = $3 WHERE connection_id = $1
`

// SetEnabled flips the runtime enable toggle. It returns ErrNoRow when no row
// exists for connID (a slot can only be enabled/disabled after a credential
// has been stored). The 409 enable-guard lives in the admin handler; this is
// the persistence primitive.
func (s *Store) SetEnabled(ctx context.Context, connID string, enabled bool) error {
	if s == nil || s.pool == nil {
		return errors.New("connstore: Store requires a non-nil Pool")
	}
	tag, err := s.pool.Exec(ctx, setEnabledSQL, connID, enabled, s.now().UTC())
	if err != nil {
		return fmt.Errorf("connstore: set enabled=%v for connection %q: %w", enabled, connID, err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNoRow
	}
	return nil
}

// Credential implements the SCOPE-03 llm.CredentialSource. It returns the
// at-rest VaultRecord for connID ONLY when the connection is effective-enabled.
// A DB error, an absent row, a non-effective-enabled connection, or a missing
// credential all map to found=false — fail-closed, so the dispatch resolver
// rejects the target and NEVER substitutes Ollama. It is context-free per the
// SCOPE-03 interface; the bounded read uses an internal timeout.
func (s *Store) Credential(connID string) (connvault.VaultRecord, bool) {
	if s == nil || s.pool == nil {
		return connvault.VaultRecord{}, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), credentialReadTimeout)
	defer cancel()
	rec, found, err := s.Get(ctx, connID)
	if err != nil || !found {
		return connvault.VaultRecord{}, false
	}
	_, declared := s.registry[connID]
	if !EffectiveEnabled(rec, declared) {
		return connvault.VaultRecord{}, false
	}
	return *rec.Secret, true
}

// DiscoveryConnections returns the registry connections SCOPE-04 discovery
// should build adapters for — the SINGLE effective-enabled gate the aggregator
// consults, identical to the one dispatch uses:
//
//   - a no-secret (Ollama / local) slot that is registry-`enabled` (its live
//     reachability is then probed by the aggregator's `/api/tags` adapter);
//   - a db-mode slot that is EffectiveEnabled (DB enabled + last_test=ok +
//     credential present).
//
// A db-mode slot that is not effective-enabled is omitted (absent from the
// catalog) — never silently dropped at dispatch, never an Ollama fallback.
func (s *Store) DiscoveryConnections(ctx context.Context) ([]config.ModelConnection, error) {
	if s == nil || s.pool == nil {
		return nil, errors.New("connstore: Store requires a non-nil Pool")
	}
	rows, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]Record, len(rows))
	for _, r := range rows {
		byID[r.ConnectionID] = r
	}
	var out []config.ModelConnection
	for id, conn := range s.registry {
		switch conn.SecretRef.Mode {
		case config.ModelConnectionSecretModeNone:
			// Ollama / local: registry-enabled slots are handed to discovery;
			// the aggregator's live probe decides reachability.
			if conn.Enabled {
				out = append(out, conn)
			}
		case config.ModelConnectionSecretModeDB:
			if rec, ok := byID[id]; ok && EffectiveEnabled(rec, true) {
				out = append(out, conn)
			}
		default:
			// env-mode and any future mode are not part of the SCOPE-06
			// runtime-plane gate; they are handled by their own registry path.
		}
	}
	return out, nil
}
