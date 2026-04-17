package list

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Store manages list CRUD operations in PostgreSQL.
type Store struct {
	Pool *pgxpool.Pool
}

// NewStore creates a new list store.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{Pool: pool}
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
			continue
		}
		items = append(items, item)
	}

	return &ListWithItems{List: list, Items: items}, nil
}

// ListLists returns lists filtered by status and type.
func (s *Store) ListLists(ctx context.Context, statusFilter, typeFilter string, limit, offset int) ([]List, error) {
	if limit <= 0 {
		limit = 20
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
			continue
		}
		lists = append(lists, l)
	}
	return lists, nil
}

// UpdateItemStatus changes an item's status and updates counters.
func (s *Store) UpdateItemStatus(ctx context.Context, listID, itemID string, status ItemStatus, substitution string) error {
	now := time.Now()

	var checkedAt *time.Time
	if status == ItemDone || status == ItemSubstituted {
		checkedAt = &now
	}

	_, err := s.Pool.Exec(ctx, `
		UPDATE list_items SET status = $3, substitution = $4, checked_at = $5, updated_at = $6
		WHERE id = $2 AND list_id = $1
	`, listID, itemID, status, substitution, checkedAt, now)
	if err != nil {
		return fmt.Errorf("update item status: %w", err)
	}

	// Recalculate checked count
	_, err = s.Pool.Exec(ctx, `
		UPDATE lists SET
			checked_items = (SELECT COUNT(*) FROM list_items WHERE list_id = $1 AND status IN ('done', 'substituted')),
			updated_at = $2
		WHERE id = $1
	`, listID, now)
	if err != nil {
		slog.Warn("failed to update checked count", "list_id", listID, "error", err)
	}

	return nil
}

// AddManualItem adds a user-created item to a list.
func (s *Store) AddManualItem(ctx context.Context, listID, content, category string) (*ListItem, error) {
	now := time.Now()
	item := ListItem{
		ID:                fmt.Sprintf("itm-%s-%d", listID[:min(8, len(listID))], now.UnixNano()),
		ListID:            listID,
		Content:           content,
		Category:          category,
		Status:            ItemPending,
		SourceArtifactIDs: []string{},
		IsManual:          true,
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	_, err := s.Pool.Exec(ctx, `
		INSERT INTO list_items (id, list_id, content, category, status, source_artifact_ids, is_manual, sort_order, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, (SELECT COALESCE(MAX(sort_order), 0) + 1 FROM list_items WHERE list_id = $2), $8, $9)
	`, item.ID, listID, content, category, ItemPending, item.SourceArtifactIDs, true, now, now)
	if err != nil {
		return nil, fmt.Errorf("insert manual item: %w", err)
	}

	// Update total_items counter
	_, _ = s.Pool.Exec(ctx, `UPDATE lists SET total_items = total_items + 1, updated_at = $2 WHERE id = $1`, listID, now)

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
