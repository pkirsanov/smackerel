package photos

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ActionKind enumerates the photo mutations that funnel through the
// shared confirmation contract (FR-020). Read-only operations do not need
// an action token.
type ActionKind string

const (
	ActionArchive       ActionKind = "archive"
	ActionDelete        ActionKind = "delete"
	ActionAlbumRemove   ActionKind = "album_remove"
	ActionTag           ActionKind = "tag"
	ActionMarkSensitive ActionKind = "mark_sensitive"
	ActionFavorite      ActionKind = "favorite"
)

func (kind ActionKind) RequiresTextConfirmation() bool {
	return kind == ActionDelete
}

func (kind ActionKind) Valid() bool {
	switch kind {
	case ActionArchive, ActionDelete, ActionAlbumRemove, ActionTag, ActionMarkSensitive, ActionFavorite:
		return true
	}
	return false
}

// ActionScope is the immutable selector that an action token authorises.
// Callers pin a specific photo set or a (cluster_id, member_role) tuple
// at plan time; the confirm-time scope must match exactly or the token
// is rejected for drift.
type ActionScope struct {
	PhotoIDs   []string `json:"photo_ids"`
	ClusterID  string   `json:"cluster_id,omitempty"`
	AlbumID    string   `json:"album_id,omitempty"`
	Provider   string   `json:"provider,omitempty"`
	Reason     string   `json:"reason,omitempty"`
	RemovalIDs []string `json:"removal_ids,omitempty"`
}

func (scope ActionScope) Normalize() ActionScope {
	out := ActionScope{
		ClusterID: strings.TrimSpace(scope.ClusterID),
		AlbumID:   strings.TrimSpace(scope.AlbumID),
		Provider:  strings.TrimSpace(scope.Provider),
		Reason:    strings.TrimSpace(scope.Reason),
	}
	out.PhotoIDs = uniqueSortedNonEmpty(scope.PhotoIDs)
	out.RemovalIDs = uniqueSortedNonEmpty(scope.RemovalIDs)
	return out
}

func (scope ActionScope) Hash() (string, error) {
	normalized := scope.Normalize()
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return "", fmt.Errorf("hash action scope: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// ActionToken is the mint-time/confirm-time record persisted to
// photo_action_tokens. The struct never carries provider credentials —
// callers resolve the writer from the connector at confirm time.
type ActionToken struct {
	ID            uuid.UUID
	ActorID       string
	Action        ActionKind
	Scope         ActionScope
	ScopeHash     string
	PhotoCount    int
	BytesEstimate int64
	ConfidenceMin *float64
	ConfidenceMax *float64
	RequiresText  bool
	ExpiresAt     time.Time
	ConsumedAt    *time.Time
	CreatedAt     time.Time
}

// MintPhotoActionTokenInput captures the planning-side inputs.
type MintPhotoActionTokenInput struct {
	ActorID       string
	Action        ActionKind
	Scope         ActionScope
	BytesEstimate int64
	ConfidenceMin *float64
	ConfidenceMax *float64
	TTL           time.Duration
	RequiresText  bool
}

// ConfirmPhotoActionTokenInput captures the confirm-side inputs.
type ConfirmPhotoActionTokenInput struct {
	TokenID          uuid.UUID
	ActorID          string
	Scope            ActionScope
	TextConfirmation string
}

// Errors returned by the action-token store. Each error is matched
// directly by callers via errors.Is so drift, expiry, and double-spend
// surface as distinct codes in the API layer.
var (
	ErrActionTokenNotFound        = errors.New("photos: action token not found")
	ErrActionTokenExpired         = errors.New("photos: action token expired")
	ErrActionTokenAlreadyConsumed = errors.New("photos: action token already consumed")
	ErrActionTokenScopeDrift      = errors.New("photos: action token scope drift")
	ErrActionTokenActorMismatch   = errors.New("photos: action token actor mismatch")
	ErrActionTokenTextMissing     = errors.New("photos: action token requires text confirmation")
	ErrActionTokenInvalidAction   = errors.New("photos: action token has invalid action kind")
	ErrActionTokenScopeEmpty      = errors.New("photos: action token scope is empty")
)

// MintActionToken persists a new action token. The store is expected to
// own the database; callers in tests use NewActionTokenStore(nil) to
// exercise validation logic against an in-memory shim.
func (store *Store) MintActionToken(ctx context.Context, input MintPhotoActionTokenInput, now time.Time) (*ActionToken, error) {
	if store == nil || store.pool == nil {
		return nil, fmt.Errorf("photos: store pool is nil")
	}
	token, err := buildMintedActionToken(input, now)
	if err != nil {
		return nil, err
	}
	scopeBytes, err := json.Marshal(token.Scope)
	if err != nil {
		return nil, fmt.Errorf("encode action scope: %w", err)
	}
	if _, err := store.pool.Exec(ctx, `
		INSERT INTO photo_action_tokens (
			id, photo_id, action_kind, token_hash, expires_at, created_at,
			actor_id, scope_payload, scope_hash, photo_count, bytes_estimate,
			confidence_min, confidence_max, requires_text
		) VALUES ($1, NULL, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`, token.ID, string(token.Action), token.ID.String(), token.ExpiresAt, token.CreatedAt,
		token.ActorID, scopeBytes, token.ScopeHash, token.PhotoCount, token.BytesEstimate,
		token.ConfidenceMin, token.ConfidenceMax, token.RequiresText); err != nil {
		return nil, fmt.Errorf("persist photo action token: %w", err)
	}
	return token, nil
}

// ConfirmActionToken validates the supplied scope against the persisted
// token and marks it consumed. The caller is expected to perform the
// underlying provider mutation only after this returns nil.
func (store *Store) ConfirmActionToken(ctx context.Context, input ConfirmPhotoActionTokenInput, now time.Time) (*ActionToken, error) {
	if store == nil || store.pool == nil {
		return nil, fmt.Errorf("photos: store pool is nil")
	}
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin action confirm transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var token ActionToken
	var scopeBytes []byte
	var consumed *time.Time
	var actor string
	var actionKind string
	if err := tx.QueryRow(ctx, `
		SELECT id, action_kind, expires_at, consumed_at, actor_id,
		       scope_payload, scope_hash, photo_count, bytes_estimate,
		       confidence_min, confidence_max, requires_text, created_at
		  FROM photo_action_tokens
		 WHERE id=$1
	`, input.TokenID).Scan(
		&token.ID, &actionKind, &token.ExpiresAt, &consumed, &actor,
		&scopeBytes, &token.ScopeHash, &token.PhotoCount, &token.BytesEstimate,
		&token.ConfidenceMin, &token.ConfidenceMax, &token.RequiresText, &token.CreatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrActionTokenNotFound
		}
		return nil, fmt.Errorf("load photo action token: %w", err)
	}
	token.Action = ActionKind(actionKind)
	token.ActorID = actor
	token.ConsumedAt = consumed
	if err := json.Unmarshal(scopeBytes, &token.Scope); err != nil {
		return nil, fmt.Errorf("decode action scope: %w", err)
	}
	if err := validateActionConfirm(token, input, now); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `UPDATE photo_action_tokens SET consumed_at=$2 WHERE id=$1`, token.ID, now.UTC()); err != nil {
		return nil, fmt.Errorf("mark action token consumed: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit action confirm transaction: %w", err)
	}
	consumedAt := now.UTC()
	token.ConsumedAt = &consumedAt
	return &token, nil
}

func buildMintedActionToken(input MintPhotoActionTokenInput, now time.Time) (*ActionToken, error) {
	if !input.Action.Valid() {
		return nil, ErrActionTokenInvalidAction
	}
	scope := input.Scope.Normalize()
	if len(scope.PhotoIDs) == 0 && len(scope.RemovalIDs) == 0 && scope.ClusterID == "" {
		return nil, ErrActionTokenScopeEmpty
	}
	hash, err := scope.Hash()
	if err != nil {
		return nil, err
	}
	requiresText := input.RequiresText || input.Action.RequiresTextConfirmation()
	ttl := input.TTL
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	count := len(scope.PhotoIDs)
	if count == 0 {
		count = len(scope.RemovalIDs)
	}
	if count == 0 && scope.ClusterID != "" {
		count = 1
	}
	actor := strings.TrimSpace(input.ActorID)
	if actor == "" {
		actor = "system"
	}
	return &ActionToken{
		ID:            uuid.New(),
		ActorID:       actor,
		Action:        input.Action,
		Scope:         scope,
		ScopeHash:     hash,
		PhotoCount:    count,
		BytesEstimate: input.BytesEstimate,
		ConfidenceMin: input.ConfidenceMin,
		ConfidenceMax: input.ConfidenceMax,
		RequiresText:  requiresText,
		ExpiresAt:     now.Add(ttl).UTC(),
		CreatedAt:     now.UTC(),
	}, nil
}

func validateActionConfirm(token ActionToken, input ConfirmPhotoActionTokenInput, now time.Time) error {
	if token.ConsumedAt != nil {
		return ErrActionTokenAlreadyConsumed
	}
	if !token.ExpiresAt.IsZero() && now.After(token.ExpiresAt) {
		return ErrActionTokenExpired
	}
	if strings.TrimSpace(input.ActorID) != "" && strings.TrimSpace(input.ActorID) != token.ActorID {
		return ErrActionTokenActorMismatch
	}
	if token.RequiresText && strings.TrimSpace(input.TextConfirmation) != strings.ToUpper(string(token.Action)) {
		return ErrActionTokenTextMissing
	}
	expectedHash, err := input.Scope.Hash()
	if err != nil {
		return err
	}
	if expectedHash != token.ScopeHash {
		return ErrActionTokenScopeDrift
	}
	return nil
}

// MintActionTokenInMemory exposes the validation portion of the mint flow
// for unit tests that exercise the contract without a database. The
// returned token is identical to MintActionToken minus persistence.
func MintActionTokenInMemory(input MintPhotoActionTokenInput, now time.Time) (*ActionToken, error) {
	return buildMintedActionToken(input, now)
}

// ConfirmActionTokenInMemory exposes the confirm-side validation in the
// same way for unit tests.
func ConfirmActionTokenInMemory(token ActionToken, input ConfirmPhotoActionTokenInput, now time.Time) error {
	return validateActionConfirm(token, input, now)
}

func uniqueSortedNonEmpty(in []string) []string {
	if len(in) == 0 {
		return []string{}
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, raw := range in {
		v := strings.TrimSpace(raw)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}
