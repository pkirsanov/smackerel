package graph

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oklog/ulid/v2"

	"github.com/smackerel/smackerel/internal/db"
)

// Counter op_kind constants for hospitality_counter_applications dedup ledger.
// BUG-013-001 (sweep R5 / stabilize-to-doc): the artifacts.processed NATS
// consumer is configured with AckPolicy=AckExplicitPolicy, AckWait=30s, and
// MaxDeliver>1, so HandleProcessedResult — and therefore LinkArtifact — can be
// invoked more than once for the same artifact_id on AckWait expiry, process
// crash before ack, or transient handler-error Naks. Each counter mutation
// claims its (artifact_id, op_kind) pair via ON CONFLICT DO NOTHING before
// applying the underlying repo increment, so redelivery is a no-op for
// counters. Edge writes remain unchanged because they are already idempotent
// via ON CONFLICT DO UPDATE on the edges unique constraint.
const (
	hospitalityOpKindGuestStayIncrement      = "guest_stay_increment"
	hospitalityOpKindPropertyBookingIncrement = "property_booking_increment"
	hospitalityOpKindPropertyIssueDelta       = "property_issue_delta"
)

// HospitalityLinker extends the standard graph linker with hospitality-specific
// node creation and edge strategies for GuestHost connector artifacts.
type HospitalityLinker struct {
	guestRepo    *db.GuestRepository
	propertyRepo *db.PropertyRepository
	pool         *pgxpool.Pool
	linker       *Linker
}

// NewHospitalityLinker creates a new HospitalityLinker.
func NewHospitalityLinker(
	guestRepo *db.GuestRepository,
	propertyRepo *db.PropertyRepository,
	pool *pgxpool.Pool,
	linker *Linker,
) *HospitalityLinker {
	return &HospitalityLinker{
		guestRepo:    guestRepo,
		propertyRepo: propertyRepo,
		pool:         pool,
		linker:       linker,
	}
}

// hospitalityMeta holds the hospitality-specific fields parsed from an artifact's content_raw.
type hospitalityMeta struct {
	PropertyID   string  `json:"propertyId"`
	PropertyName string  `json:"propertyName"`
	GuestEmail   string  `json:"guestEmail"`
	GuestName    string  `json:"guestName"`
	BookingID    string  `json:"bookingId"`
	CheckIn      string  `json:"checkinDate"`
	CheckOut     string  `json:"checkoutDate"`
	Revenue      float64 `json:"totalAmount"`
	Category     string  `json:"category"`
	Amount       float64 `json:"amount"`
	Rating       string  `json:"rating"`
	Status       string  `json:"status"`
}

// LinkArtifact creates hospitality-specific graph nodes and edges for a
// GuestHost-sourced artifact. It reads the artifact from the DB, parses
// hospitality metadata from content_raw, and creates guest/property nodes
// plus typed edges based on the artifact's content type.
func (l *HospitalityLinker) LinkArtifact(ctx context.Context, artifactID string) error {
	// Read artifact source_id and content type to decide if this is hospitality data
	var sourceID, artifactType, contentRaw string
	err := l.pool.QueryRow(ctx, `
		SELECT source_id, artifact_type, COALESCE(content_raw, '')
		FROM artifacts WHERE id = $1
	`, artifactID).Scan(&sourceID, &artifactType, &contentRaw)
	if err != nil {
		return fmt.Errorf("read artifact for hospitality linking: %w", err)
	}

	// Only process GuestHost-sourced artifacts
	if sourceID != "guesthost" {
		return nil
	}

	if contentRaw == "" {
		return nil
	}

	var meta hospitalityMeta
	if err := json.Unmarshal([]byte(contentRaw), &meta); err != nil {
		slog.Debug("hospitality linker: could not parse content_raw",
			"artifact_id", artifactID, "error", err)
		return nil
	}

	var edgesCreated int

	// Upsert guest node if we have guest email
	var guestNode *db.GuestNode
	if meta.GuestEmail != "" {
		g, err := l.guestRepo.UpsertByEmail(ctx, meta.GuestEmail, meta.GuestName, "guesthost")
		if err != nil {
			slog.Warn("hospitality linker: guest upsert failed",
				"artifact_id", artifactID, "email", meta.GuestEmail, "error", err)
		} else {
			guestNode = g
		}
	}

	// Upsert property node if we have property ID
	var propertyNode *db.PropertyNode
	if meta.PropertyID != "" {
		p, err := l.propertyRepo.UpsertByExternalID(ctx, meta.PropertyID, "guesthost", meta.PropertyName)
		if err != nil {
			slog.Warn("hospitality linker: property upsert failed",
				"artifact_id", artifactID, "property_id", meta.PropertyID, "error", err)
		} else {
			propertyNode = p
		}
	}

	// Create type-specific edges
	switch artifactType {
	case "booking":
		edgesCreated += l.linkBooking(ctx, artifactID, guestNode, propertyNode, meta)

	case "review":
		edgesCreated += l.linkReview(ctx, artifactID, guestNode, propertyNode)

	case "task":
		edgesCreated += l.linkTask(ctx, artifactID, propertyNode, meta)

	case "financial":
		edgesCreated += l.linkExpense(ctx, artifactID, propertyNode, meta)

	case "guest_message":
		edgesCreated += l.linkMessage(ctx, artifactID, guestNode, propertyNode, meta)

	case "property":
		// Property update — no special edges beyond the node upsert
	}

	if edgesCreated > 0 {
		slog.Info("hospitality graph linked",
			"artifact_id", artifactID,
			"artifact_type", artifactType,
			"edges_created", edgesCreated,
		)
	}

	return nil
}

// linkBooking creates STAYED_AT (guest→property) edges and updates aggregate stats.
func (l *HospitalityLinker) linkBooking(ctx context.Context, artifactID string, guest *db.GuestNode, property *db.PropertyNode, meta hospitalityMeta) int {
	count := 0

	if guest != nil && property != nil {
		if err := l.createEdge(ctx, "guest", guest.ID, "property", property.ID, "STAYED_AT", 1.0); err == nil {
			count++
		}

		// Update guest stay stats — BUG-013-001: gated behind
		// hospitality_counter_applications so NATS redelivery does not drift
		// guests.total_stays / total_spend.
		claimed, claimErr := l.tryClaimCounterApplication(ctx, artifactID, hospitalityOpKindGuestStayIncrement)
		if claimErr != nil {
			slog.Warn("hospitality linker: claim guest stay idempotency failed", "artifact_id", artifactID, "guest_id", guest.ID, "error", claimErr)
		} else if claimed {
			if err := l.guestRepo.IncrementStay(ctx, guest.ID, meta.Revenue); err != nil {
				slog.Warn("hospitality linker: increment guest stay failed", "guest_id", guest.ID, "error", err)
			}
		} else {
			slog.Debug("hospitality linker: guest stay counter already applied (NATS redelivery dedup)", "artifact_id", artifactID, "guest_id", guest.ID)
		}

		// Update property booking stats — BUG-013-001: gated behind
		// hospitality_counter_applications so NATS redelivery does not drift
		// properties.total_bookings / total_revenue.
		claimed, claimErr = l.tryClaimCounterApplication(ctx, artifactID, hospitalityOpKindPropertyBookingIncrement)
		if claimErr != nil {
			slog.Warn("hospitality linker: claim property booking idempotency failed", "artifact_id", artifactID, "property_id", property.ID, "error", claimErr)
		} else if claimed {
			if err := l.propertyRepo.IncrementBookings(ctx, property.ID, meta.Revenue); err != nil {
				slog.Warn("hospitality linker: increment property bookings failed", "property_id", property.ID, "error", err)
			}
		} else {
			slog.Debug("hospitality linker: property booking counter already applied (NATS redelivery dedup)", "artifact_id", artifactID, "property_id", property.ID)
		}
	}

	// Link artifact to booking period
	if property != nil {
		metaMap := map[string]string{"checkin": meta.CheckIn, "checkout": meta.CheckOut}
		metadataBytes, marshalErr := json.Marshal(metaMap)
		if marshalErr != nil {
			slog.Warn("hospitality linker: marshal booking metadata failed", "error", marshalErr)
			metadataBytes = []byte("{}")
		}
		if err := l.createEdgeWithMetadata(ctx, "artifact", artifactID, "property", property.ID, "DURING_STAY", 1.0, string(metadataBytes)); err == nil {
			count++
		}
	}

	return count
}

// linkReview creates REVIEWED (guest→property) edges.
func (l *HospitalityLinker) linkReview(ctx context.Context, artifactID string, guest *db.GuestNode, property *db.PropertyNode) int {
	count := 0

	if guest != nil && property != nil {
		if err := l.createEdge(ctx, "guest", guest.ID, "property", property.ID, "REVIEWED", 1.0); err == nil {
			count++
		}
	}

	// Link artifact to property
	if property != nil {
		if err := l.createEdge(ctx, "artifact", artifactID, "property", property.ID, "REVIEWED", 0.8); err == nil {
			count++
		}
	}

	return count
}

// linkTask creates ISSUE_AT (artifact→property) edges and adjusts issue count.
// IMP-013-IMP-002: completed tasks decrement instead of increment.
func (l *HospitalityLinker) linkTask(ctx context.Context, artifactID string, property *db.PropertyNode, meta hospitalityMeta) int {
	count := 0

	if property != nil {
		if err := l.createEdge(ctx, "artifact", artifactID, "property", property.ID, "ISSUE_AT", 1.0); err == nil {
			count++
		}

		delta := 1
		if meta.Status == "completed" {
			delta = -1
		}
		// BUG-013-001: gate UpdateIssueCount behind
		// hospitality_counter_applications so NATS redelivery does not drift
		// properties.issue_count. The same artifact_id always reduces to the
		// same delta because meta.Status is parsed from the immutable
		// content_raw payload on each call.
		claimed, claimErr := l.tryClaimCounterApplication(ctx, artifactID, hospitalityOpKindPropertyIssueDelta)
		if claimErr != nil {
			slog.Warn("hospitality linker: claim property issue delta idempotency failed", "artifact_id", artifactID, "property_id", property.ID, "error", claimErr)
		} else if claimed {
			if err := l.propertyRepo.UpdateIssueCount(ctx, property.ID, delta); err != nil {
				slog.Warn("hospitality linker: update issue count failed", "property_id", property.ID, "error", err)
			}
		} else {
			slog.Debug("hospitality linker: property issue delta already applied (NATS redelivery dedup)", "artifact_id", artifactID, "property_id", property.ID)
		}
	}

	return count
}

// linkExpense creates EXPENSE_AT (artifact→property) edges for expenses.
// IMP-013-IMP-003: Use EXPENSE_AT instead of ISSUE_AT for semantic correctness.
func (l *HospitalityLinker) linkExpense(ctx context.Context, artifactID string, property *db.PropertyNode, meta hospitalityMeta) int {
	count := 0

	if property != nil {
		metaMap := map[string]interface{}{"category": meta.Category, "amount": meta.Amount}
		metadataBytes, marshalErr := json.Marshal(metaMap)
		if marshalErr != nil {
			slog.Warn("hospitality linker: marshal expense metadata failed", "error", marshalErr)
			metadataBytes = []byte("{}")
		}
		if err := l.createEdgeWithMetadata(ctx, "artifact", artifactID, "property", property.ID, "EXPENSE_AT", 0.7, string(metadataBytes)); err == nil {
			count++
		}
	}

	return count
}

// linkMessage creates DURING_STAY edges linking messages to properties via booking context.
func (l *HospitalityLinker) linkMessage(ctx context.Context, artifactID string, guest *db.GuestNode, property *db.PropertyNode, meta hospitalityMeta) int {
	count := 0

	if property != nil {
		metaMap := map[string]string{"booking_id": meta.BookingID}
		metadataBytes, marshalErr := json.Marshal(metaMap)
		if marshalErr != nil {
			slog.Warn("hospitality linker: marshal message metadata failed", "error", marshalErr)
			metadataBytes = []byte("{}")
		}
		if err := l.createEdgeWithMetadata(ctx, "artifact", artifactID, "property", property.ID, "DURING_STAY", 0.8, string(metadataBytes)); err == nil {
			count++
		}
	}

	if guest != nil && property != nil {
		if err := l.createEdge(ctx, "guest", guest.ID, "property", property.ID, "MESSAGED", 0.6); err == nil {
			count++
		}
	}

	return count
}

// createEdge creates an edge in the graph, ignoring duplicates.
func (l *HospitalityLinker) createEdge(ctx context.Context, srcType, srcID, dstType, dstID, edgeType string, weight float32) error {
	return l.createEdgeWithMetadata(ctx, srcType, srcID, dstType, dstID, edgeType, weight, "{}")
}

// tryClaimCounterApplication attempts to claim a one-time counter mutation
// against hospitality_counter_applications. Returns (true, nil) when this is
// the first time (artifactID, opKind) has been seen and the caller MUST
// proceed with the counter mutation. Returns (false, nil) when the row
// already exists (NATS redelivery; counter MUST be skipped). Returns
// (false, err) on infrastructure errors — callers MUST NOT apply the counter
// in this case because we cannot tell whether it has already been applied.
//
// See BUG-013-001 for the redelivery scenarios this guards against.
func (l *HospitalityLinker) tryClaimCounterApplication(ctx context.Context, artifactID, opKind string) (bool, error) {
	var inserted bool
	err := l.pool.QueryRow(ctx, `
		WITH ins AS (
			INSERT INTO hospitality_counter_applications (artifact_id, op_kind)
			VALUES ($1, $2)
			ON CONFLICT (artifact_id, op_kind) DO NOTHING
			RETURNING 1
		)
		SELECT EXISTS (SELECT 1 FROM ins)
	`, artifactID, opKind).Scan(&inserted)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("claim counter application: %w", err)
	}
	return inserted, nil
}

// createEdgeWithMetadata creates an edge with JSON metadata, ignoring duplicates.
func (l *HospitalityLinker) createEdgeWithMetadata(ctx context.Context, srcType, srcID, dstType, dstID, edgeType string, weight float32, metadata string) error {
	id := ulid.Make().String()
	_, err := l.pool.Exec(ctx, `
		INSERT INTO edges (id, src_type, src_id, dst_type, dst_id, edge_type, weight, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (src_type, src_id, dst_type, dst_id, edge_type) DO UPDATE SET weight = $7, metadata = $8
	`, id, srcType, srcID, dstType, dstID, edgeType, weight, metadata)
	if err != nil {
		slog.Debug("hospitality edge creation failed",
			"src_type", srcType, "src_id", srcID,
			"dst_type", dstType, "dst_id", dstID,
			"edge_type", edgeType, "error", err)
	}
	return err
}

// SeedHospitalityTopics creates the initial hospitality topics in the knowledge graph.
// This is safe to call multiple times — topics are upserted by name.
func SeedHospitalityTopics(ctx context.Context, pool *pgxpool.Pool) error {
	topicNames := []string{
		"guest-experience",
		"property-maintenance",
		"revenue-management",
		"booking-operations",
		"guest-communication",
	}

	ids := make([]string, len(topicNames))
	for i := range topicNames {
		ids[i] = ulid.Make().String()
	}

	_, err := pool.Exec(ctx, `
		INSERT INTO topics (id, name, state)
		SELECT unnest($1::text[]), unnest($2::text[]), 'emerging'
		ON CONFLICT (name) DO NOTHING
	`, ids, topicNames)
	if err != nil {
		return fmt.Errorf("seed hospitality topics: %w", err)
	}

	slog.Info("hospitality topics seeded", "count", len(topicNames))
	return nil
}
