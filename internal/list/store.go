package list

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/metrics"
	smacknats "github.com/smackerel/smackerel/internal/nats"
)

// Store manages list CRUD operations in PostgreSQL.
type Store struct {
	Pool *pgxpool.Pool
	NATS *smacknats.Client
}

// NewStore creates a new list store.
func NewStore(pool *pgxpool.Pool, nc *smacknats.Client) *Store {
	return &Store{Pool: pool, NATS: nc}
}

// CreateList inserts a list and its items in a single transaction.
func (s *Store) CreateList(ctx context.Context, list *List, items []ListItem) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `
		INSERT INTO lists (id, list_type, title, status, source_artifact_ids, source_query, domain, total_items, checked_items, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`, list.ID, list.ListType, list.Title, list.Status, list.SourceArtifactIDs,
		list.SourceQuery, list.Domain, len(items), 0, list.CreatedAt, list.UpdatedAt)
	if err != nil {
		return fmt.Errorf("insert list: %w", err)
	}

	for _, item := range items {
		_, err = tx.Exec(ctx, `
			INSERT INTO list_items (id, list_id, content, category, status, source_artifact_ids, is_manual, quantity, unit, normalized_name, sort_order, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		`, item.ID, list.ID, item.Content, item.Category, item.Status,
			item.SourceArtifactIDs, item.IsManual, item.Quantity, item.Unit,
			item.NormalizedName, item.SortOrder, item.CreatedAt, item.UpdatedAt)
		if err != nil {
			return fmt.Errorf("insert item %s: %w", item.ID, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	list.TotalItems = len(items)
	slog.Info("list created", "list_id", list.ID, "type", list.ListType, "items", len(items))

	// Publish NATS event for intelligence consumption
	if s.NATS != nil {
		event := map[string]any{
			"list_id":        list.ID,
			"list_type":      list.ListType,
			"domain":         list.Domain,
			"artifact_count": len(list.SourceArtifactIDs),
			"item_count":     len(items),
		}
		if data, err := json.Marshal(event); err == nil {
			if pubErr := s.NATS.Publish(ctx, smacknats.SubjectListsCreated, data); pubErr != nil {
				metrics.ListEventsPublishFailed.WithLabelValues(smacknats.SubjectListsCreated).Inc()
				slog.Warn("failed to publish lists.created event", "list_id", list.ID, "error", pubErr)
			}
		}
	}

	return nil
}

// GetList retrieves a list with all its items.
func (s *Store) GetList(ctx context.Context, listID string) (*ListWithItems, error) {
	var list List
	err := s.Pool.QueryRow(ctx, `
		SELECT id, list_type, title, status, source_artifact_ids, COALESCE(source_query, ''),
		       COALESCE(domain, ''), total_items, checked_items, created_at, updated_at, completed_at
		FROM lists WHERE id = $1
	`, listID).Scan(&list.ID, &list.ListType, &list.Title, &list.Status,
		&list.SourceArtifactIDs, &list.SourceQuery, &list.Domain,
		&list.TotalItems, &list.CheckedItems, &list.CreatedAt, &list.UpdatedAt, &list.CompletedAt)
	if err != nil {
		return nil, fmt.Errorf("get list: %w", err)
	}

	rows, err := s.Pool.Query(ctx, `
		SELECT id, list_id, content, COALESCE(category, ''), status, COALESCE(substitution, ''),
		       source_artifact_ids, is_manual, quantity, COALESCE(unit, ''), COALESCE(normalized_name, ''),
		       sort_order, checked_at, COALESCE(notes, ''), created_at, updated_at
		FROM list_items WHERE list_id = $1 ORDER BY sort_order
	`, listID)
	if err != nil {
		return nil, fmt.Errorf("get list items: %w", err)
	}
	defer rows.Close()

	var items []ListItem
	for rows.Next() {
		var item ListItem
		if err := rows.Scan(&item.ID, &item.ListID, &item.Content, &item.Category,
			&item.Status, &item.Substitution, &item.SourceArtifactIDs, &item.IsManual,
			&item.Quantity, &item.Unit, &item.NormalizedName, &item.SortOrder,
			&item.CheckedAt, &item.Notes, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan list item for list %s: %w", listID, err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate list items for list %s: %w", listID, err)
	}

	return &ListWithItems{List: list, Items: items}, nil
}

// ListLists returns lists filtered by status and type.
// Callers MUST supply limit > 0 and offset >= 0; the store rejects invalid pagination
// inputs explicitly (no silent default-fallback masking — see Gate G028 / requireNoDefaultsNoFallbacks).
func (s *Store) ListLists(ctx context.Context, statusFilter, typeFilter string, limit, offset int) ([]List, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("list lists: limit must be > 0, got %d", limit)
	}
	if offset < 0 {
		return nil, fmt.Errorf("list lists: offset must be >= 0, got %d", offset)
	}

	query := `SELECT id, list_type, title, status, source_artifact_ids, COALESCE(source_query, ''),
	                  COALESCE(domain, ''), total_items, checked_items, created_at, updated_at, completed_at
	           FROM lists WHERE 1=1`
	args := []any{}
	argN := 1

	if statusFilter != "" {
		query += fmt.Sprintf(" AND status = $%d", argN)
		args = append(args, statusFilter)
		argN++
	}
	if typeFilter != "" {
		query += fmt.Sprintf(" AND list_type = $%d", argN)
		args = append(args, typeFilter)
		argN++
	}

	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d OFFSET $%d", argN, argN+1)
	args = append(args, limit, offset)

	rows, err := s.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list lists: %w", err)
	}
	defer rows.Close()

	var lists []List
	for rows.Next() {
		var l List
		if err := rows.Scan(&l.ID, &l.ListType, &l.Title, &l.Status, &l.SourceArtifactIDs,
			&l.SourceQuery, &l.Domain, &l.TotalItems, &l.CheckedItems,
			&l.CreatedAt, &l.UpdatedAt, &l.CompletedAt); err != nil {
			return nil, fmt.Errorf("scan list row: %w", err)
		}
		lists = append(lists, l)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate lists: %w", err)
	}
	return lists, nil
}

// UpdateItemStatus changes an item's status and updates counters atomically.
// The item-status update and the checked_items recalculation run in a single
// transaction so a transient DB failure on the recalc cannot leave checked_items
// permanently drifted relative to list_items.status.
func (s *Store) UpdateItemStatus(ctx context.Context, listID, itemID string, status ItemStatus, substitution string) error {
	now := time.Now()

	var checkedAt *time.Time
	if status == ItemDone || status == ItemSubstituted {
		checkedAt = &now
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin update item status transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	tag, err := tx.Exec(ctx, `
		UPDATE list_items SET status = $3, substitution = $4, checked_at = $5, updated_at = $6
		WHERE id = $2 AND list_id = $1
	`, listID, itemID, status, substitution, checkedAt, now)
	if err != nil {
		return fmt.Errorf("update item status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("update item status: item %s not found in list %s", itemID, listID)
	}

	// Recalculate checked count in the same transaction so item-status and
	// list.checked_items either both commit or both roll back.
	if _, err := tx.Exec(ctx, `
		UPDATE lists SET
			checked_items = (SELECT COUNT(*) FROM list_items WHERE list_id = $1 AND status IN ('done', 'substituted')),
			updated_at = $2
		WHERE id = $1
	`, listID, now); err != nil {
		return fmt.Errorf("recalculate checked_items for list %s: %w", listID, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit update item status transaction: %w", err)
	}

	metrics.ListItemStatusChanges.WithLabelValues(string(status)).Inc()
	return nil
}

// AddManualItem adds a user-created item to a list. Insert + counter recalc run
// inside a single transaction with a FOR UPDATE row lock on the parent list so
// concurrent AddManualItem calls against the same list serialize and cannot race
// on MAX(sort_order). The counter is recomputed from COUNT(*) so it is
// self-healing rather than drifting on a missed increment.
func (s *Store) AddManualItem(ctx context.Context, listID, content, category string) (*ListItem, error) {
	now := time.Now()
	rand := uuid.NewString()
	item := ListItem{
		ID:                fmt.Sprintf("itm-%s-%s", listID[:min(8, len(listID))], rand[:8]),
		ListID:            listID,
		Content:           content,
		Category:          category,
		Status:            ItemPending,
		SourceArtifactIDs: []string{},
		IsManual:          true,
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin add manual item transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Serialize concurrent AddManualItem against the same list so the
	// MAX(sort_order) read and the INSERT cannot interleave with another
	// inserter and produce duplicate sort_order values.
	var lockedID string
	if err := tx.QueryRow(ctx, `SELECT id FROM lists WHERE id = $1 FOR UPDATE`, listID).Scan(&lockedID); err != nil {
		return nil, fmt.Errorf("lock list %s for manual item insert: %w", listID, err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO list_items (id, list_id, content, category, status, source_artifact_ids, is_manual, sort_order, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, (SELECT COALESCE(MAX(sort_order), 0) + 1 FROM list_items WHERE list_id = $2), $8, $9)
	`, item.ID, listID, content, category, ItemPending, item.SourceArtifactIDs, true, now, now); err != nil {
		return nil, fmt.Errorf("insert manual item: %w", err)
	}

	// Self-healing counter: recalculate from COUNT(*) inside the same tx so an
	// insert that commits cannot leave total_items drifted.
	if _, err := tx.Exec(ctx, `
		UPDATE lists SET
			total_items = (SELECT COUNT(*) FROM list_items WHERE list_id = $1),
			updated_at = $2
		WHERE id = $1
	`, listID, now); err != nil {
		return nil, fmt.Errorf("recalculate total_items for list %s: %w", listID, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit add manual item transaction: %w", err)
	}

	return &item, nil
}

// CompleteList marks a list as completed.
func (s *Store) CompleteList(ctx context.Context, listID string) error {
	now := time.Now()
	_, err := s.Pool.Exec(ctx, `
		UPDATE lists SET status = 'completed', completed_at = $2, updated_at = $2 WHERE id = $1
	`, listID, now)
	if err != nil {
		return fmt.Errorf("complete list: %w", err)
	}
	slog.Info("list completed", "list_id", listID)

	// Publish NATS event for intelligence consumption
	if s.NATS != nil {
		// Consolidate the 4 separate QueryRow round-trips into a single SELECT with
		// FILTER aggregates. Reduces DB load and removes inconsistency windows
		// between the individual count reads.
		var listType, domain string
		var itemsDone, itemsSkipped, itemsSubstituted int
		if err := s.Pool.QueryRow(ctx, `
			SELECT l.list_type, COALESCE(l.domain, ''),
			       COUNT(*) FILTER (WHERE li.status = 'done'),
			       COUNT(*) FILTER (WHERE li.status = 'skipped'),
			       COUNT(*) FILTER (WHERE li.status = 'substituted')
			FROM lists l LEFT JOIN list_items li ON li.list_id = l.id
			WHERE l.id = $1
			GROUP BY l.id, l.list_type, l.domain
		`, listID).Scan(&listType, &domain, &itemsDone, &itemsSkipped, &itemsSubstituted); err != nil {
			slog.Warn("failed to fetch list stats for completion event", "list_id", listID, "error", err)
		}

		metrics.ListsCompleted.WithLabelValues(listType).Inc()

		event := map[string]any{
			"list_id":           listID,
			"list_type":         listType,
			"domain":            domain,
			"items_done":        itemsDone,
			"items_skipped":     itemsSkipped,
			"items_substituted": itemsSubstituted,
		}
		if data, err := json.Marshal(event); err == nil {
			if pubErr := s.NATS.Publish(ctx, smacknats.SubjectListsCompleted, data); pubErr != nil {
				metrics.ListEventsPublishFailed.WithLabelValues(smacknats.SubjectListsCompleted).Inc()
				slog.Warn("failed to publish lists.completed event", "list_id", listID, "error", pubErr)
			}
		}
	}

	return nil
}

// RemoveItem deletes a specific item from a list and recalculates counters in a
// single transaction so total_items / checked_items cannot drift relative to
// list_items.
func (s *Store) RemoveItem(ctx context.Context, listID, itemID string) error {
	now := time.Now()

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin remove item transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	tag, err := tx.Exec(ctx, `DELETE FROM list_items WHERE id = $1 AND list_id = $2`, itemID, listID)
	if err != nil {
		return fmt.Errorf("delete item: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("item %s not found in list %s", itemID, listID)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE lists SET
			total_items = (SELECT COUNT(*) FROM list_items WHERE list_id = $1),
			checked_items = (SELECT COUNT(*) FROM list_items WHERE list_id = $1 AND status IN ('done', 'substituted')),
			updated_at = $2
		WHERE id = $1
	`, listID, now); err != nil {
		return fmt.Errorf("recalculate counters after item delete for list %s: %w", listID, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit remove item transaction: %w", err)
	}

	return nil
}

// ArchiveList marks a list as archived.
func (s *Store) ArchiveList(ctx context.Context, listID string) error {
	now := time.Now()
	_, err := s.Pool.Exec(ctx, `
		UPDATE lists SET status = 'archived', updated_at = $2 WHERE id = $1
	`, listID, now)
	if err != nil {
		return fmt.Errorf("archive list: %w", err)
	}
	return nil
}
