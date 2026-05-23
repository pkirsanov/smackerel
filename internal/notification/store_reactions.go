package notification

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const (
	SuppressionCooldown       = "cooldown"
	SuppressionUserPreference = "user_preference"
	SuppressionQuietWindow    = "quiet_window"

	ApprovalStatusPending  = "pending"
	ApprovalStatusApproved = "approved"
	ApprovalStatusRejected = "rejected"
	ApprovalStatusExpired  = "expired"
	ApprovalStatusSnoozed  = "snoozed"
	ApprovalStatusCanceled = "canceled"

	ApprovalDecisionApprove = "approve"
	ApprovalDecisionDeny    = "deny"
	ApprovalDecisionSnooze  = "snooze"
	ApprovalDecisionExpire  = "expire"
	ApprovalDecisionCancel  = "cancel"
)

type ApprovalDetail struct {
	Request   ApprovalRequest
	Decisions []ApprovalDecision
}

func (s *Store) CreateSuppression(ctx context.Context, suppression Suppression) (Suppression, error) {
	if s == nil || s.pool == nil {
		return Suppression{}, fmt.Errorf("notification suppression store: postgres pool is required")
	}
	if suppression.ID == "" {
		suppression.ID = "notif_supp_" + uuid.NewString()
	}
	if suppression.Kind == "" {
		return Suppression{}, fmt.Errorf("notification suppression: kind is required")
	}
	if suppression.Reason == "" {
		return Suppression{}, fmt.Errorf("notification suppression: reason is required")
	}
	if suppression.StartsAt.IsZero() || suppression.CreatedAt.IsZero() {
		return Suppression{}, fmt.Errorf("notification suppression: starts_at and created_at are required")
	}
	if suppression.NotificationID == "" && suppression.IncidentID == "" && suppression.SourceInstanceID == "" {
		return Suppression{}, fmt.Errorf("notification suppression: notification, incident, or source scope is required")
	}
	scopeJSON, err := json.Marshal(suppression.Scope)
	if err != nil {
		return Suppression{}, fmt.Errorf("marshal suppression scope: %w", err)
	}
	_, err = s.pool.Exec(ctx, `
INSERT INTO notification_suppressions (
    id, notification_id, incident_id, source_instance_id, suppression_kind,
    scope, reason, starts_at, expires_at, created_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		suppression.ID, nullableString(suppression.NotificationID), nullableString(suppression.IncidentID), nullableString(suppression.SourceInstanceID), suppression.Kind,
		scopeJSON, suppression.Reason, suppression.StartsAt, suppression.ExpiresAt, suppression.CreatedAt)
	if err != nil {
		return Suppression{}, fmt.Errorf("insert notification suppression: %w", err)
	}
	return suppression, nil
}

func (s *Store) ListQuietWindows(ctx context.Context, limit int) ([]Suppression, error) {
	if limit < 1 {
		return nil, fmt.Errorf("list quiet windows: positive limit is required")
	}
	rows, err := s.pool.Query(ctx, `
SELECT id, COALESCE(notification_id,''), COALESCE(incident_id,''), COALESCE(source_instance_id,''), suppression_kind, scope, reason, starts_at, expires_at, created_at
FROM notification_suppressions
WHERE suppression_kind = 'quiet_window'
ORDER BY starts_at DESC, created_at DESC
LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("list notification quiet windows: %w", err)
	}
	defer rows.Close()
	return scanSuppressions(rows)
}

func (s *Store) CreateApprovalRequest(ctx context.Context, approval ApprovalRequest) (ApprovalRequest, error) {
	if s == nil || s.pool == nil {
		return ApprovalRequest{}, fmt.Errorf("notification approval store: postgres pool is required")
	}
	if approval.ID == "" {
		approval.ID = "notif_approval_" + uuid.NewString()
	}
	if approval.IncidentID == "" || approval.DecisionID == "" || approval.ActionKey == "" || approval.TargetRef == "" {
		return ApprovalRequest{}, fmt.Errorf("notification approval request: incident, decision, action, and target are required")
	}
	if approval.RiskExplanation == "" || approval.ExpectedEffect == "" {
		return ApprovalRequest{}, fmt.Errorf("notification approval request: risk explanation and expected effect are required")
	}
	if approval.Status == "" || approval.CreatedAt.IsZero() || approval.ExpiresAt.IsZero() {
		return ApprovalRequest{}, fmt.Errorf("notification approval request: status, created_at, and expires_at are required")
	}
	verificationJSON, err := json.Marshal(approval.VerificationPlan)
	if err != nil {
		return ApprovalRequest{}, fmt.Errorf("marshal approval verification plan: %w", err)
	}
	_, err = s.pool.Exec(ctx, `
INSERT INTO notification_approval_requests (
    id, incident_id, decision_id, action_key, target_ref, risk_explanation,
    expected_effect, verification_plan, expires_at, status, created_at, resolved_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		approval.ID, approval.IncidentID, approval.DecisionID, approval.ActionKey, approval.TargetRef, approval.RiskExplanation,
		approval.ExpectedEffect, verificationJSON, approval.ExpiresAt, approval.Status, approval.CreatedAt, approval.ResolvedAt)
	if err != nil {
		return ApprovalRequest{}, fmt.Errorf("insert notification approval request: %w", err)
	}
	return approval, nil
}

func (s *Store) GetApprovalDetail(ctx context.Context, id string) (ApprovalDetail, error) {
	request, err := s.GetApprovalRequest(ctx, id)
	if err != nil {
		return ApprovalDetail{}, err
	}
	decisions, err := s.ListApprovalDecisions(ctx, id)
	if err != nil {
		return ApprovalDetail{}, err
	}
	return ApprovalDetail{Request: request, Decisions: decisions}, nil
}

func (s *Store) GetApprovalRequest(ctx context.Context, id string) (ApprovalRequest, error) {
	rows, err := s.pool.Query(ctx, `
SELECT id, incident_id, decision_id, action_key, target_ref, risk_explanation,
       expected_effect, verification_plan, expires_at, status, created_at, resolved_at
FROM notification_approval_requests
WHERE id = $1
LIMIT 1`, id)
	if err != nil {
		return ApprovalRequest{}, fmt.Errorf("get notification approval request: %w", err)
	}
	defer rows.Close()
	approvals, err := scanApprovalRequests(rows)
	if err != nil {
		return ApprovalRequest{}, err
	}
	if len(approvals) == 0 {
		return ApprovalRequest{}, pgx.ErrNoRows
	}
	return approvals[0], nil
}

func (s *Store) ListApprovalRequests(ctx context.Context, limit int) ([]ApprovalRequest, error) {
	if limit < 1 {
		return nil, fmt.Errorf("list approval requests: positive limit is required")
	}
	rows, err := s.pool.Query(ctx, `
SELECT id, incident_id, decision_id, action_key, target_ref, risk_explanation,
       expected_effect, verification_plan, expires_at, status, created_at, resolved_at
FROM notification_approval_requests
ORDER BY created_at DESC
LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("list notification approval requests: %w", err)
	}
	defer rows.Close()
	return scanApprovalRequests(rows)
}

func (s *Store) ListApprovalDecisions(ctx context.Context, approvalID string) ([]ApprovalDecision, error) {
	rows, err := s.pool.Query(ctx, `
SELECT id, approval_request_id, decision, actor_kind, actor_ref, channel, COALESCE(reason,''), created_at
FROM notification_approval_decisions
WHERE approval_request_id = $1
ORDER BY created_at DESC`, approvalID)
	if err != nil {
		return nil, fmt.Errorf("list notification approval decisions: %w", err)
	}
	defer rows.Close()
	return scanApprovalDecisions(rows)
}

func (s *Store) RecordApprovalDecision(ctx context.Context, decision ApprovalDecision) (ApprovalDetail, error) {
	if s == nil || s.pool == nil {
		return ApprovalDetail{}, fmt.Errorf("notification approval store: postgres pool is required")
	}
	if decision.ID == "" {
		decision.ID = "notif_approval_decision_" + uuid.NewString()
	}
	if decision.ApprovalRequestID == "" || decision.Decision == "" || decision.ActorKind == "" || decision.ActorRef == "" || decision.Channel == "" || decision.CreatedAt.IsZero() {
		return ApprovalDetail{}, fmt.Errorf("notification approval decision: approval, decision, actor, channel, and created_at are required")
	}
	updatedStatus, err := approvalStatusForDecision(decision.Decision)
	if err != nil {
		return ApprovalDetail{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ApprovalDetail{}, fmt.Errorf("begin notification approval decision tx: %w", err)
	}
	defer tx.Rollback(ctx)
	commandTag, err := tx.Exec(ctx, `
UPDATE notification_approval_requests
SET status = $1, resolved_at = $2
WHERE id = $3`, updatedStatus, decision.CreatedAt, decision.ApprovalRequestID)
	if err != nil {
		return ApprovalDetail{}, fmt.Errorf("update notification approval request: %w", err)
	}
	if commandTag.RowsAffected() == 0 {
		return ApprovalDetail{}, pgx.ErrNoRows
	}
	_, err = tx.Exec(ctx, `
INSERT INTO notification_approval_decisions (
    id, approval_request_id, decision, actor_kind, actor_ref, channel, reason, created_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		decision.ID, decision.ApprovalRequestID, decision.Decision, decision.ActorKind, decision.ActorRef, decision.Channel, nullableString(decision.Reason), decision.CreatedAt)
	if err != nil {
		return ApprovalDetail{}, fmt.Errorf("insert notification approval decision: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return ApprovalDetail{}, fmt.Errorf("commit notification approval decision: %w", err)
	}
	return s.GetApprovalDetail(ctx, decision.ApprovalRequestID)
}

func (s *Store) ListLoopOrigins(ctx context.Context, since time.Time, limit int) ([]LoopOrigin, error) {
	if limit < 1 {
		return nil, fmt.Errorf("list loop origins: positive limit is required")
	}
	rows, err := s.pool.Query(ctx, `
SELECT decision_id, channel, payload_hash, attempted_at
FROM notification_delivery_attempts
WHERE attempted_at >= $1
ORDER BY attempted_at DESC
LIMIT $2`, since, limit)
	if err != nil {
		return nil, fmt.Errorf("list notification loop origins: %w", err)
	}
	defer rows.Close()
	origins := []LoopOrigin{}
	for rows.Next() {
		var origin LoopOrigin
		if err := rows.Scan(&origin.DecisionID, &origin.OutputChannel, &origin.PayloadHash, &origin.EmittedAt); err != nil {
			return nil, err
		}
		origins = append(origins, origin)
	}
	return origins, rows.Err()
}

func approvalStatusForDecision(decision string) (string, error) {
	switch strings.TrimSpace(decision) {
	case ApprovalDecisionApprove:
		return ApprovalStatusApproved, nil
	case ApprovalDecisionDeny:
		return ApprovalStatusRejected, nil
	case ApprovalDecisionSnooze:
		return ApprovalStatusSnoozed, nil
	case ApprovalDecisionExpire:
		return ApprovalStatusExpired, nil
	case ApprovalDecisionCancel:
		return ApprovalStatusCanceled, nil
	default:
		return "", fmt.Errorf("notification approval decision %q is invalid", decision)
	}
}

func scanApprovalRequests(rows pgx.Rows) ([]ApprovalRequest, error) {
	approvals := []ApprovalRequest{}
	for rows.Next() {
		var approval ApprovalRequest
		var verificationJSON []byte
		if err := rows.Scan(&approval.ID, &approval.IncidentID, &approval.DecisionID, &approval.ActionKey, &approval.TargetRef, &approval.RiskExplanation, &approval.ExpectedEffect, &verificationJSON, &approval.ExpiresAt, &approval.Status, &approval.CreatedAt, &approval.ResolvedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(verificationJSON, &approval.VerificationPlan)
		approvals = append(approvals, approval)
	}
	return approvals, rows.Err()
}

func scanApprovalDecisions(rows pgx.Rows) ([]ApprovalDecision, error) {
	decisions := []ApprovalDecision{}
	for rows.Next() {
		var decision ApprovalDecision
		if err := rows.Scan(&decision.ID, &decision.ApprovalRequestID, &decision.Decision, &decision.ActorKind, &decision.ActorRef, &decision.Channel, &decision.Reason, &decision.CreatedAt); err != nil {
			return nil, err
		}
		decisions = append(decisions, decision)
	}
	return decisions, rows.Err()
}
