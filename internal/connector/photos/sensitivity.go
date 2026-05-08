package photos

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// RetrievalReason explains why EvaluateRetrieval blocked or allowed a
// photo response. Callers expose the reason as a stable code so the
// PWA, Telegram bot, and agent tools render consistent UX.
type RetrievalReason string

const (
	RetrievalAllowed             RetrievalReason = "allowed"
	RetrievalBlockedSensitive    RetrievalReason = "sensitivity_requires_reveal"
	RetrievalBlockedHidden       RetrievalReason = "hidden_no_auto_send"
	RetrievalBlockedNoReveal     RetrievalReason = "reveal_token_required"
	RetrievalBlockedRevealExpire RetrievalReason = "reveal_token_expired"
)

// RetrievalDecision is the result of EvaluateRetrieval. The contract:
// auto-send is allowed only when a non-sensitive photo is requested or
// when the caller presents a fresh reveal token belonging to that photo
// + actor pair.
type RetrievalDecision struct {
	Allowed        bool             `json:"allowed"`
	RequiresReveal bool             `json:"requires_reveal"`
	Reason         RetrievalReason  `json:"reason"`
	Sensitivity    SensitivityLevel `json:"sensitivity"`
	Labels         []string         `json:"sensitivity_labels"`
}

// EvaluateRetrieval encapsulates the server-side gate documented in
// design.md §11. It MUST be invoked by every retrieval surface (preview
// bytes, search preview URL, Telegram delivery, agent tools, digest
// inclusion) before any photo bytes are returned.
func EvaluateRetrieval(record PhotoRecord, hasValidReveal bool) RetrievalDecision {
	decision := RetrievalDecision{
		Sensitivity: record.Sensitivity,
		Labels:      append([]string(nil), record.SensitivityLabels...),
	}
	if record.Sensitivity == SensitivityHidden {
		decision.RequiresReveal = true
		if !hasValidReveal {
			decision.Reason = RetrievalBlockedHidden
			return decision
		}
		decision.Allowed = true
		decision.Reason = RetrievalAllowed
		return decision
	}
	if record.Sensitivity == SensitivitySensitive {
		decision.RequiresReveal = true
		if !hasValidReveal {
			decision.Reason = RetrievalBlockedSensitive
			return decision
		}
		decision.Allowed = true
		decision.Reason = RetrievalAllowed
		return decision
	}
	decision.Allowed = true
	decision.Reason = RetrievalAllowed
	return decision
}

// RevealToken authorises one retrieval of a sensitive photo by a
// specific actor. Tokens are short-lived (`photos.policy.sensitivity_reveal_ttl_seconds`),
// scoped to a single photo + actor, and consumed on first use. The
// plaintext token is returned only at mint time; the database stores
// the SHA-256 digest so a leaked database row cannot reveal photos.
type RevealToken struct {
	ID         uuid.UUID
	PhotoID    uuid.UUID
	ActorID    string
	Plaintext  string
	ExpiresAt  time.Time
	CreatedAt  time.Time
	ConsumedAt *time.Time
}

// MintRevealTokenInput captures the inputs for MintRevealToken.
type MintRevealTokenInput struct {
	PhotoID uuid.UUID
	ActorID string
	TTL     time.Duration
}

// Errors returned by the reveal-token store. Callers compare against
// the sentinel via errors.Is so the API layer maps them to stable
// response codes.
var (
	ErrRevealTokenNotFound       = errors.New("photos: reveal token not found")
	ErrRevealTokenExpired        = errors.New("photos: reveal token expired")
	ErrRevealTokenConsumed       = errors.New("photos: reveal token already consumed")
	ErrRevealTokenActorMismatch  = errors.New("photos: reveal token belongs to a different actor")
	ErrRevealTokenPhotoMismatch  = errors.New("photos: reveal token photo mismatch")
	ErrRevealTokenInvalidPayload = errors.New("photos: reveal token payload is invalid")
)

// MintRevealToken persists a new reveal token and returns the
// plaintext value (caller delivers it to the user; the DB stores the
// digest only).
func (store *Store) MintRevealToken(ctx context.Context, input MintRevealTokenInput, now time.Time) (*RevealToken, error) {
	if store == nil || store.pool == nil {
		return nil, fmt.Errorf("photos: store pool is nil")
	}
	actor := strings.TrimSpace(input.ActorID)
	if actor == "" {
		return nil, fmt.Errorf("photos: reveal token requires an actor")
	}
	if input.PhotoID == uuid.Nil {
		return nil, fmt.Errorf("photos: reveal token requires a photo id")
	}
	ttl := input.TTL
	if ttl <= 0 {
		ttl = 60 * time.Second
	}
	plaintext, err := newRevealPlaintext()
	if err != nil {
		return nil, err
	}
	// MIT-040-S-001: hash the random secret and persist the digest so
	// a leaked DB row cannot replay reveals; the wire blob still carries
	// the plaintext secret to the caller exactly once.
	secretHash := hashRevealSecret(plaintext)
	id := uuid.New()
	expiresAt := now.UTC().Add(ttl)
	createdAt := now.UTC()
	if _, err := store.pool.Exec(ctx, `
		INSERT INTO photo_reveal_tokens (id, photo_id, actor_id, expires_at, created_at, secret_hash)
		VALUES ($1, $2, $3, $4, $5, $6)`, id, input.PhotoID, actor, expiresAt, createdAt, secretHash); err != nil {
		return nil, fmt.Errorf("persist reveal token: %w", err)
	}
	return &RevealToken{
		ID:        id,
		PhotoID:   input.PhotoID,
		ActorID:   actor,
		Plaintext: encodeRevealToken(id, plaintext),
		ExpiresAt: expiresAt,
		CreatedAt: createdAt,
	}, nil
}

// ConsumeRevealToken validates a presented token. On success the row is
// marked consumed and the function returns the persisted record so the
// caller can audit the reveal.
//
// MIT-040-S-007 (TOCTOU): the SELECT uses FOR UPDATE so concurrent
// consumers serialize on the row lock, and the UPDATE carries an
// explicit `WHERE consumed_at IS NULL` predicate plus a RowsAffected
// check so a race-loser cannot succeed even if the FOR UPDATE is later
// removed. MIT-040-S-001: the presented secret half of the token is
// SHA-256'd and constant-time compared against `secret_hash`; a
// mismatch collapses to ErrRevealTokenNotFound so the caller cannot
// distinguish "wrong secret" from "token does not exist".
func (store *Store) ConsumeRevealToken(ctx context.Context, photoID uuid.UUID, actorID string, raw string, now time.Time) (*RevealToken, error) {
	if store == nil || store.pool == nil {
		return nil, fmt.Errorf("photos: store pool is nil")
	}
	id, presentedSecret, err := decodeRevealToken(raw)
	if err != nil {
		return nil, err
	}
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin reveal consume tx: %w", err)
	}
	defer tx.Rollback(ctx)
	var token RevealToken
	var storedHash []byte
	if err := tx.QueryRow(ctx, `
		SELECT id, photo_id, actor_id, expires_at, consumed_at, created_at, secret_hash
		  FROM photo_reveal_tokens
		 WHERE id=$1
		 FOR UPDATE`, id).Scan(&token.ID, &token.PhotoID, &token.ActorID, &token.ExpiresAt, &token.ConsumedAt, &token.CreatedAt, &storedHash); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRevealTokenNotFound
		}
		return nil, fmt.Errorf("load reveal token: %w", err)
	}
	if token.ConsumedAt != nil {
		return nil, ErrRevealTokenConsumed
	}
	if !token.ExpiresAt.After(now.UTC()) {
		return nil, ErrRevealTokenExpired
	}
	if token.PhotoID != photoID {
		return nil, ErrRevealTokenPhotoMismatch
	}
	if !strings.EqualFold(strings.TrimSpace(actorID), strings.TrimSpace(token.ActorID)) {
		return nil, ErrRevealTokenActorMismatch
	}
	presentedHash := hashRevealSecret(presentedSecret)
	if subtle.ConstantTimeCompare(presentedHash, storedHash) != 1 {
		// MIT-040-S-001: collapse to generic not-found so a wrong
		// secret cannot be distinguished from a missing token id.
		return nil, ErrRevealTokenNotFound
	}
	res, err := tx.Exec(ctx, `
		UPDATE photo_reveal_tokens
		   SET consumed_at=$2
		 WHERE id=$1 AND consumed_at IS NULL`, token.ID, now.UTC())
	if err != nil {
		return nil, fmt.Errorf("consume reveal token: %w", err)
	}
	if res.RowsAffected() != 1 {
		// MIT-040-S-007: a concurrent consumer raced past the FOR UPDATE
		// boundary (or FOR UPDATE was removed in a future regression)
		// and consumed the token between our SELECT and UPDATE. Collapse
		// to generic not-found so the loser cannot distinguish race-loss
		// from never-existed.
		return nil, ErrRevealTokenNotFound
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit reveal consume: %w", err)
	}
	consumed := now.UTC()
	token.ConsumedAt = &consumed
	return &token, nil
}

// CheckRevealToken validates a presented token without consuming it.
// Used by EvaluateRetrieval helpers that want to surface a non-binding
// preview state alongside the consumable preview endpoint. MIT-040-S-001:
// the secret half is SHA-256'd and constant-time compared against the
// stored digest, with a mismatch collapsing to ErrRevealTokenNotFound.
func (store *Store) CheckRevealToken(ctx context.Context, photoID uuid.UUID, actorID string, raw string, now time.Time) error {
	if strings.TrimSpace(raw) == "" {
		return ErrRevealTokenNotFound
	}
	if store == nil || store.pool == nil {
		return fmt.Errorf("photos: store pool is nil")
	}
	id, presentedSecret, err := decodeRevealToken(raw)
	if err != nil {
		return err
	}
	var token RevealToken
	var storedHash []byte
	if err := store.pool.QueryRow(ctx, `
		SELECT id, photo_id, actor_id, expires_at, consumed_at, created_at, secret_hash
		  FROM photo_reveal_tokens
		 WHERE id=$1`, id).Scan(&token.ID, &token.PhotoID, &token.ActorID, &token.ExpiresAt, &token.ConsumedAt, &token.CreatedAt, &storedHash); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrRevealTokenNotFound
		}
		return fmt.Errorf("load reveal token: %w", err)
	}
	if token.ConsumedAt != nil {
		return ErrRevealTokenConsumed
	}
	if !token.ExpiresAt.After(now.UTC()) {
		return ErrRevealTokenExpired
	}
	if token.PhotoID != photoID {
		return ErrRevealTokenPhotoMismatch
	}
	if !strings.EqualFold(strings.TrimSpace(actorID), strings.TrimSpace(token.ActorID)) {
		return ErrRevealTokenActorMismatch
	}
	presentedHash := hashRevealSecret(presentedSecret)
	if subtle.ConstantTimeCompare(presentedHash, storedHash) != 1 {
		return ErrRevealTokenNotFound
	}
	return nil
}

func newRevealPlaintext() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate reveal token: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

func encodeRevealToken(id uuid.UUID, secret string) string {
	return id.String() + "." + secret
}

func decodeRevealToken(raw string) (uuid.UUID, string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return uuid.Nil, "", ErrRevealTokenInvalidPayload
	}
	parts := strings.SplitN(raw, ".", 2)
	if len(parts) != 2 {
		return uuid.Nil, "", ErrRevealTokenInvalidPayload
	}
	id, err := uuid.Parse(parts[0])
	if err != nil {
		return uuid.Nil, "", ErrRevealTokenInvalidPayload
	}
	secret := strings.TrimSpace(parts[1])
	if secret == "" {
		return uuid.Nil, "", ErrRevealTokenInvalidPayload
	}
	return id, secret, nil
}

// hashRevealSecret returns the SHA-256 digest of the random secret half
// of a reveal token blob. MintRevealToken stores the digest in
// `photo_reveal_tokens.secret_hash`; ConsumeRevealToken / CheckRevealToken
// constant-time compare the presented secret's digest against it
// (MIT-040-S-001). The wire format remains `<uuid>.<secret>` — only the
// server-side validation changed.
func hashRevealSecret(secret string) []byte {
	sum := sha256.Sum256([]byte(secret))
	return sum[:]
}
