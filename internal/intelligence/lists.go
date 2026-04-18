package intelligence

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"

	smacknats "github.com/smackerel/smackerel/internal/nats"
)

// ListCompletedEvent is published to lists.completed when a list is completed.
type ListCompletedEvent struct {
	ListID            string   `json:"list_id"`
	ListType          string   `json:"list_type"`
	Title             string   `json:"title"`
	SourceArtifactIDs []string `json:"source_artifact_ids"`
	TotalItems        int      `json:"total_items"`
	CheckedItems      int      `json:"checked_items"`
	CompletedAt       string   `json:"completed_at"`
}

// PurchaseFrequency tracks how often an item is purchased.
type PurchaseFrequency struct {
	NormalizedName string    `json:"normalized_name"`
	Count          int       `json:"count"`
	LastSeen       time.Time `json:"last_seen"`
}

// HandleListCompleted processes a lists.completed NATS event.
// It boosts relevance scores for source artifacts and tracks item purchase frequency.
func (e *Engine) HandleListCompleted(ctx context.Context, data []byte) error {
	var event ListCompletedEvent
	if err := json.Unmarshal(data, &event); err != nil {
		return fmt.Errorf("unmarshal list completed event: %w", err)
	}

	slog.Info("processing completed list for intelligence",
		"list_id", event.ListID,
		"list_type", event.ListType,
		"source_artifacts", len(event.SourceArtifactIDs),
	)

	// 1. Boost relevance scores for source artifacts
	if err := e.boostArtifactRelevance(ctx, event.SourceArtifactIDs); err != nil {
		slog.Warn("failed to boost artifact relevance", "error", err)
	}

	// 2. Track item purchase frequency for shopping lists
	if event.ListType == "shopping" {
		if err := e.trackPurchaseFrequency(ctx, event.ListID); err != nil {
			slog.Warn("failed to track purchase frequency", "error", err)
		}
	}

	return nil
}

// boostArtifactRelevance increases relevance_score for artifacts that led to actual user action.
func (e *Engine) boostArtifactRelevance(ctx context.Context, artifactIDs []string) error {
	if e.Pool == nil || len(artifactIDs) == 0 {
		return nil
	}

	// Boost by 0.1 (10%) for each completed list action, capped at 1.0
	_, err := e.Pool.Exec(ctx, `
		UPDATE artifacts
		SET relevance_score = LEAST(relevance_score + 0.1, 1.0),
		    updated_at = NOW()
		WHERE id = ANY($1)
	`, artifactIDs)
	if err != nil {
		return fmt.Errorf("boost relevance: %w", err)
	}

	slog.Info("boosted artifact relevance from list completion",
		"artifact_count", len(artifactIDs),
		"boost", 0.1,
	)
	return nil
}

// trackPurchaseFrequency records item purchase frequency from completed shopping lists.
// This data enables future pantry awareness and list pre-population.
func (e *Engine) trackPurchaseFrequency(ctx context.Context, listID string) error {
	if e.Pool == nil {
		return nil
	}

	// Query completed items from the list
	rows, err := e.Pool.Query(ctx, `
		SELECT COALESCE(normalized_name, lower(content)) as item_name
		FROM list_items
		WHERE list_id = $1 AND status IN ('done', 'substituted')
	`, listID)
	if err != nil {
		return fmt.Errorf("query list items: %w", err)
	}
	defer rows.Close()

	var itemNames []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			continue
		}
		if name != "" {
			itemNames = append(itemNames, name)
		}
	}

	if len(itemNames) == 0 {
		return nil
	}

	// Upsert into purchase_frequency tracking table
	for _, name := range itemNames {
		_, err := e.Pool.Exec(ctx, `
			INSERT INTO purchase_frequency (normalized_name, count, last_seen, updated_at)
			VALUES ($1, 1, NOW(), NOW())
			ON CONFLICT (normalized_name) DO UPDATE SET
				count = purchase_frequency.count + 1,
				last_seen = NOW(),
				updated_at = NOW()
		`, name)
		if err != nil {
			slog.Warn("failed to track purchase frequency", "item", name, "error", err)
		}
	}

	slog.Info("tracked purchase frequency from list completion",
		"list_id", listID,
		"items_tracked", len(itemNames),
	)
	return nil
}

// SubscribeListsCompleted sets up a NATS consumer for lists.completed events.
func (e *Engine) SubscribeListsCompleted(ctx context.Context) error {
	if e.NATS == nil {
		return fmt.Errorf("NATS client required for list subscription")
	}

	go func() {
		sub, err := e.NATS.Conn.Subscribe(smacknats.SubjectListsCompleted, func(msg *nats.Msg) {
			if err := e.HandleListCompleted(ctx, msg.Data); err != nil {
				slog.Error("failed to handle list completed event", "error", err)
			}
		})
		if err != nil {
			slog.Error("failed to subscribe to lists.completed", "error", err)
			return
		}
		defer sub.Unsubscribe()
		<-ctx.Done()
	}()

	slog.Info("subscribed to lists.completed events")
	return nil
}
