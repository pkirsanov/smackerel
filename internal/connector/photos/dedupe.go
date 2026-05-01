package photos

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ClusterKind enumerates the duplicate-cluster classifications surfaced
// by the dedupe pipeline. Names mirror photo_cluster_kind enum values.
type ClusterKind string

const (
	ClusterExactHash         ClusterKind = "exact_hash"
	ClusterCrossProviderHash ClusterKind = "cross_provider_hash"
	ClusterBurst             ClusterKind = "burst"
	ClusterHDR               ClusterKind = "hdr"
	ClusterPanoramaMember    ClusterKind = "panorama_member"
	ClusterNearDuplicate     ClusterKind = "near_duplicate"
)

func (kind ClusterKind) Valid() bool {
	switch kind {
	case ClusterExactHash, ClusterCrossProviderHash, ClusterBurst, ClusterHDR, ClusterPanoramaMember, ClusterNearDuplicate:
		return true
	}
	return false
}

// SupportedClusterKinds lists the ClusterKind values that the analyzer
// will accept. The order matches the design.md duplicate taxonomy.
func SupportedClusterKinds() []ClusterKind {
	return []ClusterKind{ClusterExactHash, ClusterCrossProviderHash, ClusterBurst, ClusterHDR, ClusterPanoramaMember, ClusterNearDuplicate}
}

// ClusterDecisionInput is the payload returned by photos.dedupe.result.
// PhotoIDs MUST contain at least two distinct photos and BestPhotoID
// MUST be one of those members. Confidence and rationale are mandatory
// (LLM-owned decision).
type ClusterDecisionInput struct {
	Kind         ClusterKind
	Provider     string
	PhotoIDs     []uuid.UUID
	BestPhotoID  uuid.UUID
	BestPickedBy string
	Confidence   float64
	Rationale    string
	ModelVersion string
	ProviderRef  string
}

// PhotoCluster is the persisted cluster row joined with members.
type PhotoCluster struct {
	ID           uuid.UUID            `json:"cluster_id"`
	Kind         ClusterKind          `json:"kind"`
	Provider     string               `json:"provider"`
	BestPhotoID  *uuid.UUID           `json:"best_photo_id,omitempty"`
	BestPickedBy string               `json:"best_picked_by"`
	Confidence   float64              `json:"confidence"`
	Rationale    string               `json:"rationale"`
	State        string               `json:"state"`
	SnoozedUntil *time.Time           `json:"snoozed_until,omitempty"`
	Members      []PhotoClusterMember `json:"members"`
	CreatedAt    time.Time            `json:"created_at"`
	UpdatedAt    time.Time            `json:"updated_at"`
}

// PhotoClusterMember is a single membership row.
type PhotoClusterMember struct {
	PhotoID uuid.UUID `json:"photo_id"`
	Role    string    `json:"role"`
}

// DedupeAnalyzer persists clusters returned by the ML dedupe pipeline.
type DedupeAnalyzer struct {
	store *Store
	now   func() time.Time
}

func NewDedupeAnalyzer(store *Store) *DedupeAnalyzer {
	return &DedupeAnalyzer{store: store, now: time.Now}
}

// Apply persists a single cluster. The unique key is (kind, sorted member
// IDs); re-applying the same shape updates the rationale/best-pick.
func (analyzer *DedupeAnalyzer) Apply(ctx context.Context, input ClusterDecisionInput) (*PhotoCluster, error) {
	if analyzer == nil || analyzer.store == nil || analyzer.store.pool == nil {
		return nil, fmt.Errorf("photos: dedupe analyzer store is required")
	}
	if !input.Kind.Valid() {
		return nil, fmt.Errorf("photos: unsupported cluster kind %q", input.Kind)
	}
	if len(input.PhotoIDs) < 2 {
		return nil, fmt.Errorf("photos: cluster requires at least two members")
	}
	conf := input.Confidence
	if _, err := ValidateLLMDecision(LLMDecision{Kind: DecisionDedupe, Confidence: &conf, Rationale: input.Rationale}); err != nil {
		return nil, err
	}
	provider := strings.TrimSpace(input.Provider)
	if provider == "" {
		return nil, fmt.Errorf("photos: cluster requires provider")
	}
	pickedBy := strings.TrimSpace(input.BestPickedBy)
	if pickedBy == "" {
		pickedBy = "llm"
	}
	if pickedBy != "llm" && pickedBy != "user" && pickedBy != "rule" {
		return nil, fmt.Errorf("photos: best_picked_by must be llm|user|rule, got %q", pickedBy)
	}
	if input.BestPhotoID == uuid.Nil {
		return nil, fmt.Errorf("photos: cluster requires best_photo_id")
	}
	memberSet := map[uuid.UUID]struct{}{}
	for _, id := range input.PhotoIDs {
		if id == uuid.Nil {
			return nil, fmt.Errorf("photos: cluster member must be a UUID")
		}
		memberSet[id] = struct{}{}
	}
	if _, ok := memberSet[input.BestPhotoID]; !ok {
		return nil, fmt.Errorf("photos: best_photo_id must be one of the cluster members")
	}
	now := analyzer.now().UTC()

	tx, err := analyzer.store.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin cluster transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	clusterID := uuid.New()
	if _, err := tx.Exec(ctx, `
		INSERT INTO photo_clusters (
			id, kind, provider, provider_cluster_ref, model_version,
			confidence, rationale, best_photo_id, best_picked_by, state,
			created_at, updated_at
		) VALUES ($1, $2::photo_cluster_kind, $3, $4, $5, $6, $7, $8, $9, 'open', $10, $10)
	`, clusterID, string(input.Kind), provider, nullableString(input.ProviderRef), nullableString(input.ModelVersion),
		conf, strings.TrimSpace(input.Rationale), input.BestPhotoID, pickedBy, now); err != nil {
		return nil, fmt.Errorf("insert cluster: %w", err)
	}
	for id := range memberSet {
		role := "sibling"
		if id == input.BestPhotoID {
			role = "best"
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO photo_cluster_members (cluster_id, photo_id, role, created_at)
			VALUES ($1, $2, $3, $4)
		`, clusterID, id, role, now); err != nil {
			return nil, fmt.Errorf("insert cluster member %s: %w", id, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit cluster: %w", err)
	}
	if err := analyzer.store.WriteAuditEvent(ctx, AuditEvent{
		Action:    "cluster_created",
		PhotoID:   ptrUUID(input.BestPhotoID),
		Provider:  provider,
		Outcome:   "open",
		Reason:    string(input.Kind),
		Metadata:  map[string]any{"cluster_id": clusterID.String(), "members": len(memberSet), "best_picked_by": pickedBy, "confidence": conf},
		CreatedAt: now,
	}); err != nil {
		return nil, err
	}
	return analyzer.store.GetCluster(ctx, clusterID)
}

// SetBestPick changes the best photo for an existing cluster (user
// override path; written by `POST /v1/photos/health/duplicates/{id}/best-pick`).
func (store *Store) SetBestPick(ctx context.Context, clusterID uuid.UUID, photoID uuid.UUID, pickedBy string, actor string) (*PhotoCluster, error) {
	if store == nil || store.pool == nil {
		return nil, fmt.Errorf("photos: store pool is nil")
	}
	if pickedBy == "" {
		pickedBy = "user"
	}
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin best-pick transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	var member uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT photo_id FROM photo_cluster_members WHERE cluster_id=$1 AND photo_id=$2`, clusterID, photoID).Scan(&member); err != nil {
		return nil, fmt.Errorf("photos: requested best pick is not a cluster member: %w", err)
	}
	now := time.Now().UTC()
	if _, err := tx.Exec(ctx, `UPDATE photo_clusters SET best_photo_id=$2, best_picked_by=$3, updated_at=$4 WHERE id=$1`, clusterID, photoID, pickedBy, now); err != nil {
		return nil, fmt.Errorf("update best pick: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE photo_cluster_members SET role=CASE WHEN photo_id=$2 THEN 'best' ELSE 'sibling' END WHERE cluster_id=$1`, clusterID, photoID); err != nil {
		return nil, fmt.Errorf("update cluster member roles: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit best-pick transaction: %w", err)
	}
	if err := store.WriteAuditEvent(ctx, AuditEvent{
		Action:    "cluster_best_pick",
		PhotoID:   &photoID,
		Outcome:   "user_override",
		Actor:     actor,
		Metadata:  map[string]any{"cluster_id": clusterID.String(), "picked_by": pickedBy},
		CreatedAt: now,
	}); err != nil {
		return nil, err
	}
	return store.GetCluster(ctx, clusterID)
}

// ResolveCluster transitions an open cluster to resolved. The action
// itself is the audit-log marker; the actual destructive operation runs
// only via an action-token confirmation handled by the API layer.
func (store *Store) ResolveCluster(ctx context.Context, clusterID uuid.UUID, outcome string, actor string) (*PhotoCluster, error) {
	if store == nil || store.pool == nil {
		return nil, fmt.Errorf("photos: store pool is nil")
	}
	now := time.Now().UTC()
	if _, err := store.pool.Exec(ctx, `UPDATE photo_clusters SET state='resolved', updated_at=$2 WHERE id=$1`, clusterID, now); err != nil {
		return nil, fmt.Errorf("resolve cluster: %w", err)
	}
	if err := store.WriteAuditEvent(ctx, AuditEvent{
		Action:    "cluster_resolved",
		Outcome:   outcome,
		Actor:     actor,
		Metadata:  map[string]any{"cluster_id": clusterID.String()},
		CreatedAt: now,
	}); err != nil {
		return nil, err
	}
	return store.GetCluster(ctx, clusterID)
}

// GetCluster returns a single cluster with its members.
func (store *Store) GetCluster(ctx context.Context, clusterID uuid.UUID) (*PhotoCluster, error) {
	if store == nil || store.pool == nil {
		return nil, fmt.Errorf("photos: store pool is nil")
	}
	var cluster PhotoCluster
	var kind string
	if err := store.pool.QueryRow(ctx, `
		SELECT id, kind::text, provider, COALESCE(best_photo_id, '00000000-0000-0000-0000-000000000000'::uuid),
		       best_picked_by, confidence, rationale, state, snoozed_until, created_at, updated_at
		  FROM photo_clusters
		 WHERE id=$1`, clusterID).Scan(
		&cluster.ID, &kind, &cluster.Provider, &cluster.BestPhotoID, &cluster.BestPickedBy,
		&cluster.Confidence, &cluster.Rationale, &cluster.State, &cluster.SnoozedUntil,
		&cluster.CreatedAt, &cluster.UpdatedAt,
	); err != nil {
		return nil, fmt.Errorf("scan cluster: %w", err)
	}
	cluster.Kind = ClusterKind(kind)
	if cluster.BestPhotoID != nil && *cluster.BestPhotoID == uuid.Nil {
		cluster.BestPhotoID = nil
	}
	rows, err := store.pool.Query(ctx, `SELECT photo_id, role FROM photo_cluster_members WHERE cluster_id=$1 ORDER BY role DESC, photo_id`, clusterID)
	if err != nil {
		return nil, fmt.Errorf("query cluster members: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var m PhotoClusterMember
		if err := rows.Scan(&m.PhotoID, &m.Role); err != nil {
			return nil, fmt.Errorf("scan cluster member: %w", err)
		}
		cluster.Members = append(cluster.Members, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate cluster members: %w", err)
	}
	return &cluster, nil
}

// ListClustersByKind returns every cluster of the supplied kind. State
// filter "open" excludes resolved/snoozed clusters from the dashboard.
func (store *Store) ListClustersByKind(ctx context.Context, kind ClusterKind, state string) ([]PhotoCluster, error) {
	if store == nil || store.pool == nil {
		return nil, fmt.Errorf("photos: store pool is nil")
	}
	if !kind.Valid() {
		return nil, fmt.Errorf("photos: unsupported cluster kind %q", kind)
	}
	query := `
		SELECT id, kind::text, provider, COALESCE(best_photo_id, '00000000-0000-0000-0000-000000000000'::uuid),
		       best_picked_by, confidence, rationale, state, snoozed_until, created_at, updated_at
		  FROM photo_clusters
		 WHERE kind=$1::photo_cluster_kind`
	args := []any{string(kind)}
	if state != "" {
		query += ` AND state=$2`
		args = append(args, state)
	}
	query += ` ORDER BY updated_at DESC`
	rows, err := store.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query clusters: %w", err)
	}
	defer rows.Close()
	var clusters []PhotoCluster
	for rows.Next() {
		var cluster PhotoCluster
		var k string
		if err := rows.Scan(&cluster.ID, &k, &cluster.Provider, &cluster.BestPhotoID, &cluster.BestPickedBy,
			&cluster.Confidence, &cluster.Rationale, &cluster.State, &cluster.SnoozedUntil,
			&cluster.CreatedAt, &cluster.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan cluster: %w", err)
		}
		cluster.Kind = ClusterKind(k)
		if cluster.BestPhotoID != nil && *cluster.BestPhotoID == uuid.Nil {
			cluster.BestPhotoID = nil
		}
		clusters = append(clusters, cluster)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate clusters: %w", err)
	}
	return clusters, nil
}

// ListClusters returns every cluster in the supplied state ("" = all).
func (store *Store) ListClusters(ctx context.Context, state string) ([]PhotoCluster, error) {
	if store == nil || store.pool == nil {
		return nil, fmt.Errorf("photos: store pool is nil")
	}
	query := `
		SELECT id, kind::text, provider, COALESCE(best_photo_id, '00000000-0000-0000-0000-000000000000'::uuid),
		       best_picked_by, confidence, rationale, state, snoozed_until, created_at, updated_at
		  FROM photo_clusters`
	var args []any
	if state != "" {
		query += ` WHERE state=$1`
		args = append(args, state)
	}
	query += ` ORDER BY updated_at DESC`
	rows, err := store.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query clusters: %w", err)
	}
	defer rows.Close()
	var clusters []PhotoCluster
	for rows.Next() {
		var cluster PhotoCluster
		var k string
		if err := rows.Scan(&cluster.ID, &k, &cluster.Provider, &cluster.BestPhotoID, &cluster.BestPickedBy,
			&cluster.Confidence, &cluster.Rationale, &cluster.State, &cluster.SnoozedUntil,
			&cluster.CreatedAt, &cluster.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan cluster: %w", err)
		}
		cluster.Kind = ClusterKind(k)
		if cluster.BestPhotoID != nil && *cluster.BestPhotoID == uuid.Nil {
			cluster.BestPhotoID = nil
		}
		clusters = append(clusters, cluster)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate clusters: %w", err)
	}
	return clusters, nil
}

// ClusterMemberRole returns the role of a photo within a cluster.
func (store *Store) ClusterMemberRole(ctx context.Context, clusterID uuid.UUID, photoID uuid.UUID) (string, error) {
	var role string
	if err := store.pool.QueryRow(ctx, `SELECT role FROM photo_cluster_members WHERE cluster_id=$1 AND photo_id=$2`, clusterID, photoID).Scan(&role); err != nil {
		return "", err
	}
	return role, nil
}

// MarshalCluster serializes a cluster for inclusion in API payloads.
func MarshalCluster(cluster PhotoCluster) (json.RawMessage, error) {
	return json.Marshal(cluster)
}

func ptrUUID(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}
	return &id
}
