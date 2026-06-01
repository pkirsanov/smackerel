package knowledge

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/oklog/ulid/v2"
)

// WebSnippetLifecycle enumerates the P3 (Knowledge Breathes) states a
// grounded web snippet moves through. Transitions are driven by app
// code; see TransitionWebSnippetLifecycle for the deterministic time
// rules. The future scheduler that invokes the transition is tracked
// as a routed finding in spec 064 SCOPE-11.
type WebSnippetLifecycle string

const (
	WebSnippetActive   WebSnippetLifecycle = "active"
	WebSnippetCooling  WebSnippetLifecycle = "cooling"
	WebSnippetDormant  WebSnippetLifecycle = "dormant"
	WebSnippetArchived WebSnippetLifecycle = "archived"
)

// Time thresholds (in days of idleness) for WebSnippet lifecycle
// transitions per design §"Artifact Persistence + Lifecycle (P3)".
const (
	webSnippetCoolingAfterDays  = 90
	webSnippetDormantAfterDays  = 180
	webSnippetArchivedAfterDays = 365
)

// WebSnippet is the persisted form of a grounded web search result.
// It matches one row in the `web_snippets` table.
type WebSnippet struct {
	ID               string
	URL              string
	Title            string
	Snippet          string
	ContentHash      string
	Provider         string
	FetchedAt        time.Time
	CapturedAt       time.Time
	LastReferencedAt time.Time
	LifecycleState   WebSnippetLifecycle
	GraphWeight      float64
}

// InsertWebSnippet persists a WebSnippet. If a row with the same
// ContentHash already exists, the existing row's ID is returned and
// LastReferencedAt is bumped — same-hash repeats are treated as a
// reference event, not a duplicate insert (G021 idempotence).
func (ks *KnowledgeStore) InsertWebSnippet(ctx context.Context, snip *WebSnippet) error {
	if snip.ContentHash == "" {
		return errors.New("insert web snippet: empty content_hash")
	}
	if snip.URL == "" {
		return errors.New("insert web snippet: empty url")
	}
	if snip.Provider == "" {
		return errors.New("insert web snippet: empty provider")
	}
	if snip.ID == "" {
		snip.ID = ulid.Make().String()
	}
	if snip.LifecycleState == "" {
		snip.LifecycleState = WebSnippetActive
	}
	now := time.Now().UTC()
	if snip.CapturedAt.IsZero() {
		snip.CapturedAt = now
	}
	if snip.LastReferencedAt.IsZero() {
		snip.LastReferencedAt = now
	}
	if snip.GraphWeight == 0 {
		snip.GraphWeight = 1.0
	}

	// ON CONFLICT (content_hash) → reuse existing row's ID and bump
	// last_referenced_at. The RETURNING clause yields the *stored* id
	// which differs from snip.ID when a prior row already existed.
	var storedID string
	var storedLastRef time.Time
	err := ks.pool.QueryRow(ctx, `
		INSERT INTO web_snippets
			(id, url, title, snippet, content_hash, provider,
			 fetched_at, captured_at, last_referenced_at,
			 lifecycle_state, graph_weight)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		ON CONFLICT (content_hash) DO UPDATE
			SET last_referenced_at = EXCLUDED.last_referenced_at
		RETURNING id, last_referenced_at`,
		snip.ID, snip.URL, snip.Title, snip.Snippet, snip.ContentHash,
		snip.Provider, snip.FetchedAt, snip.CapturedAt,
		snip.LastReferencedAt, string(snip.LifecycleState), snip.GraphWeight,
	).Scan(&storedID, &storedLastRef)
	if err != nil {
		return fmt.Errorf("insert web snippet: %w", err)
	}
	snip.ID = storedID
	snip.LastReferencedAt = storedLastRef
	return nil
}

// GetWebSnippetByContentHash returns the WebSnippet for the given
// content hash, or (nil, nil) if no row exists.
func (ks *KnowledgeStore) GetWebSnippetByContentHash(ctx context.Context, hash string) (*WebSnippet, error) {
	row := ks.pool.QueryRow(ctx, `
		SELECT id, url, title, snippet, content_hash, provider,
		       fetched_at, captured_at, last_referenced_at,
		       lifecycle_state, graph_weight
		FROM web_snippets WHERE content_hash = $1`, hash)
	return scanWebSnippet(row)
}

// GetWebSnippetByID returns the WebSnippet for the given id, or
// (nil, nil) if no row exists.
func (ks *KnowledgeStore) GetWebSnippetByID(ctx context.Context, id string) (*WebSnippet, error) {
	row := ks.pool.QueryRow(ctx, `
		SELECT id, url, title, snippet, content_hash, provider,
		       fetched_at, captured_at, last_referenced_at,
		       lifecycle_state, graph_weight
		FROM web_snippets WHERE id = $1`, id)
	return scanWebSnippet(row)
}

// ListActiveWebSnippets returns up to `limit` active web snippets
// ordered by most-recently-referenced first.
func (ks *KnowledgeStore) ListActiveWebSnippets(ctx context.Context, limit int) ([]*WebSnippet, error) {
	if limit <= 0 {
		return nil, errors.New("list active web snippets: limit must be > 0")
	}
	rows, err := ks.pool.Query(ctx, `
		SELECT id, url, title, snippet, content_hash, provider,
		       fetched_at, captured_at, last_referenced_at,
		       lifecycle_state, graph_weight
		FROM web_snippets
		WHERE lifecycle_state = 'active'
		ORDER BY last_referenced_at DESC
		LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("list active web snippets: %w", err)
	}
	defer rows.Close()
	var out []*WebSnippet
	for rows.Next() {
		s, err := scanWebSnippet(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// NextWebSnippetLifecycle is a pure-function decision: given a
// snippet's current state and how long it has been idle as of `now`,
// return the next lifecycle state. Same state in → same state out
// when no threshold is crossed. Called by the future scheduler
// (out of scope for SCOPE-11; tracked as a routed finding).
//
// Rules (design §"Artifact Persistence + Lifecycle"):
//
//	active   → cooling   after 90 days idle
//	cooling  → dormant   after 180 days idle
//	dormant  → archived  after 365 days idle
//	archived → archived  (terminal)
func NextWebSnippetLifecycle(current WebSnippetLifecycle, lastReferenced, now time.Time) WebSnippetLifecycle {
	idleDays := int(now.Sub(lastReferenced).Hours() / 24)
	switch current {
	case WebSnippetActive:
		if idleDays >= webSnippetCoolingAfterDays {
			return WebSnippetCooling
		}
	case WebSnippetCooling:
		if idleDays >= webSnippetDormantAfterDays {
			return WebSnippetDormant
		}
	case WebSnippetDormant:
		if idleDays >= webSnippetArchivedAfterDays {
			return WebSnippetArchived
		}
	}
	return current
}

// TransitionWebSnippetLifecycle updates the snippet's lifecycle_state
// in the DB if NextWebSnippetLifecycle says it should change. Returns
// the new state (which may equal the old state — no-op transitions
// are not an error).
func (ks *KnowledgeStore) TransitionWebSnippetLifecycle(ctx context.Context, id string, now time.Time) (WebSnippetLifecycle, error) {
	snip, err := ks.GetWebSnippetByID(ctx, id)
	if err != nil {
		return "", err
	}
	if snip == nil {
		return "", fmt.Errorf("transition web snippet lifecycle: not found: %s", id)
	}
	next := NextWebSnippetLifecycle(snip.LifecycleState, snip.LastReferencedAt, now)
	if next == snip.LifecycleState {
		return next, nil
	}
	if _, err := ks.pool.Exec(ctx,
		`UPDATE web_snippets SET lifecycle_state = $1 WHERE id = $2`,
		string(next), id,
	); err != nil {
		return "", fmt.Errorf("transition web snippet lifecycle: %w", err)
	}
	return next, nil
}

// rowScanner is the narrow contract shared by pgx.Row and pgx.Rows
// for single-row scans.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanWebSnippet(row rowScanner) (*WebSnippet, error) {
	s := &WebSnippet{}
	var state string
	err := row.Scan(
		&s.ID, &s.URL, &s.Title, &s.Snippet, &s.ContentHash, &s.Provider,
		&s.FetchedAt, &s.CapturedAt, &s.LastReferencedAt,
		&state, &s.GraphWeight,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("scan web snippet: %w", err)
	}
	s.LifecycleState = WebSnippetLifecycle(state)
	return s, nil
}
