package api

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/drive/rules"
	"github.com/smackerel/smackerel/internal/drive/save"
)

// DriveSaveHandlers exposes Spec 038 Scope 5 save endpoints.
type DriveSaveHandlers struct {
	pool        *pgxpool.Pool
	repo        *rules.Repository
	engine      *rules.Engine
	saveService *save.Service
}

// NewDriveSaveHandlers wires the save handlers. saveService and registry
// are required; pool drives artifact lookup for inline save requests.
func NewDriveSaveHandlers(pool *pgxpool.Pool, saveService *save.Service) *DriveSaveHandlers {
	if pool == nil || saveService == nil {
		return nil
	}
	return &DriveSaveHandlers{
		pool:        pool,
		repo:        rules.NewRepository(pool),
		engine:      rules.NewEngine(time.Now),
		saveService: saveService,
	}
}

// DriveSaveRequestBody is the JSON body for POST /v1/drive/save.
type DriveSaveRequestBody struct {
	SourceArtifactID string            `json:"source_artifact_id"`
	SourceKind       string            `json:"source_kind"`
	Classification   string            `json:"classification"`
	Sensitivity      string            `json:"sensitivity"`
	Confidence       float64           `json:"confidence"`
	Tokens           map[string]string `json:"tokens"`
	CapturedAt       string            `json:"captured_at"`
	// Title is the destination filename. Required because the same
	// artifact may save to different filenames per rule (e.g. a meal-plan
	// becomes "meal-plan.pdf" while a Telegram receipt becomes
	// "{receipt_id}.jpg").
	Title    string `json:"title"`
	MimeType string `json:"mime_type"`
	// DataB64 is the artifact bytes wrapped in standard base64 so JSON
	// transport stays usable. Optional — when missing, the handler reads
	// content_raw from the artifact and uses that.
	DataB64 string `json:"data_b64"`
	// RuleID optionally pins the save to a specific rule (skips engine
	// evaluation). When empty the handler picks the first stable match.
	RuleID string `json:"rule_id"`
	// ConnectionID optionally pins the save to a specific drive
	// connection. When empty the save service picks the most recent
	// healthy connection for the rule's provider.
	ConnectionID string `json:"connection_id"`
}

// DriveSaveResponse is the JSON response for POST /v1/drive/save.
type DriveSaveResponse struct {
	RequestID      string `json:"request_id"`
	Status         string `json:"status"`
	IdempotencyKey string `json:"idempotency_key"`
	TargetPath     string `json:"target_path"`
	TargetFolderID string `json:"target_folder_id"`
	ProviderFileID string `json:"provider_file_id"`
	ProviderURL    string `json:"provider_url"`
	Attempts       int    `json:"attempts"`
	LastError      string `json:"last_error"`
	RuleID         string `json:"rule_id"`
	Reason         string `json:"reason"`
}

// Save handles POST /v1/drive/save. It evaluates the rule engine against
// the supplied artifact metadata and (when a rule matches) calls the Save
// Service.
func (h *DriveSaveHandlers) Save(w http.ResponseWriter, r *http.Request) {
	var req DriveSaveRequestBody
	if !decodeJSONBody(w, r, &req, "INVALID_REQUEST", "invalid JSON body") {
		return
	}
	if strings.TrimSpace(req.SourceArtifactID) == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "source_artifact_id required")
		return
	}
	if strings.TrimSpace(req.Title) == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "title required")
		return
	}
	bytes, err := decodeSaveBody(req.DataB64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "data_b64: "+err.Error())
		return
	}
	if len(bytes) == 0 {
		// Fall back to artifact content_raw.
		bytes, err = h.loadArtifactBytes(r.Context(), req.SourceArtifactID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
			return
		}
	}

	rule, decision, err := h.resolveRule(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusBadRequest, "RULE_RESOLUTION_FAILED", err.Error())
		return
	}
	if decision.Selected == nil {
		_ = h.repo.AppendAudit(r.Context(), "", req.SourceArtifactID, rules.OutcomeSkipped, "no_rule_matched")
		writeJSON(w, http.StatusOK, DriveSaveResponse{
			Status: "no_match",
			Reason: "no_rule_matched",
		})
		return
	}
	if decision.Selected.RenderError != nil {
		_ = h.repo.AppendAudit(r.Context(), rule.ID, req.SourceArtifactID, rules.OutcomeFailed, decision.Selected.RenderError.Error())
		writeJSON(w, http.StatusOK, DriveSaveResponse{
			RuleID:    rule.ID,
			Status:    "failed",
			LastError: decision.Selected.RenderError.Error(),
			Reason:    "render_error",
		})
		return
	}

	// Audit conflict before save.
	if len(decision.Conflicts) > 1 {
		for _, conflict := range decision.Conflicts {
			_ = h.repo.AppendAudit(r.Context(), conflict.RuleID, req.SourceArtifactID, rules.OutcomeConflict, "stable_winner="+rule.ID)
		}
	}

	saveReq := save.Request{
		Rule:             rule,
		SourceArtifactID: req.SourceArtifactID,
		ConnectionID:     req.ConnectionID,
		ConfirmRequired:  decision.Selected.ConfirmRequired,
		RenderedPath:     decision.Selected.RenderedPath,
		Bytes: save.Bytes{
			Title:    req.Title,
			MimeType: req.MimeType,
			Body:     bytes,
		},
	}
	result, err := h.saveService.Save(r.Context(), saveReq)
	if err != nil {
		_ = h.repo.AppendAudit(r.Context(), rule.ID, req.SourceArtifactID, rules.OutcomeFailed, err.Error())
		writeJSON(w, http.StatusBadGateway, DriveSaveResponse{
			RequestID:      result.RequestID,
			Status:         string(result.Status),
			IdempotencyKey: result.IdempotencyKey,
			TargetPath:     result.TargetPath,
			TargetFolderID: result.TargetFolderID,
			ProviderFileID: result.ProviderFileID,
			ProviderURL:    result.ProviderURL,
			Attempts:       result.Attempts,
			LastError:      err.Error(),
			RuleID:         rule.ID,
			Reason:         "save_failed",
		})
		return
	}
	auditOutcome := rules.OutcomeMatched
	if result.Status == save.StatusAwaitingConfirmation {
		auditOutcome = rules.OutcomeAwaitingConfirmation
	}
	_ = h.repo.AppendAudit(r.Context(), rule.ID, req.SourceArtifactID, auditOutcome, "rendered_path="+result.TargetPath)

	writeJSON(w, http.StatusOK, DriveSaveResponse{
		RequestID:      result.RequestID,
		Status:         string(result.Status),
		IdempotencyKey: result.IdempotencyKey,
		TargetPath:     result.TargetPath,
		TargetFolderID: result.TargetFolderID,
		ProviderFileID: result.ProviderFileID,
		ProviderURL:    result.ProviderURL,
		Attempts:       result.Attempts,
		LastError:      result.LastError,
		RuleID:         rule.ID,
		Reason:         decision.Selected.Reason,
	})
}

// DriveSaveRequestView is one row in the GET /v1/drive/save/requests response.
type DriveSaveRequestView struct {
	ID               string `json:"id"`
	RuleID           string `json:"rule_id"`
	SourceArtifactID string `json:"source_artifact_id"`
	TargetPath       string `json:"target_path"`
	Status           string `json:"status"`
	Attempts         int    `json:"attempts"`
	LastError        string `json:"last_error"`
	ProviderFileID   string `json:"provider_file_id"`
	ProviderURL      string `json:"provider_url"`
	CreatedAt        string `json:"created_at"`
	CompletedAt      string `json:"completed_at"`
}

// ListRequests handles GET /v1/drive/save/requests for Screen 7 (recent
// save activity / failure surface). Supports optional ?status=...
// (comma-separated) and ?rule_id=... and ?limit=N.
func (h *DriveSaveHandlers) ListRequests(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := parseLimit(raw); err == nil {
			limit = n
		}
	}
	statusFilter := r.URL.Query().Get("status")
	ruleID := r.URL.Query().Get("rule_id")
	args := []any{limit}
	clauses := []string{}
	if statusFilter != "" {
		statuses := strings.Split(statusFilter, ",")
		args = append(args, statuses)
		clauses = append(clauses, fmt.Sprintf("status = ANY($%d)", len(args)))
	}
	if ruleID != "" {
		args = append(args, ruleID)
		clauses = append(clauses, fmt.Sprintf("rule_id = $%d", len(args)))
	}
	where := ""
	if len(clauses) > 0 {
		where = "WHERE " + strings.Join(clauses, " AND ")
	}
	query := fmt.Sprintf(`SELECT id, COALESCE(rule_id::text,''), source_artifact_id, target_path, status,
		        attempts, COALESCE(last_error,''), COALESCE(provider_file_id,''), COALESCE(provider_url,''),
		        created_at, completed_at
		   FROM drive_save_requests %s
		  ORDER BY created_at DESC LIMIT $1`, where)
	rows, err := h.pool.Query(r.Context(), query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}
	defer rows.Close()
	out := []DriveSaveRequestView{}
	for rows.Next() {
		var (
			view        DriveSaveRequestView
			createdAt   time.Time
			completedAt sql.NullTime
		)
		if err := rows.Scan(&view.ID, &view.RuleID, &view.SourceArtifactID, &view.TargetPath, &view.Status,
			&view.Attempts, &view.LastError, &view.ProviderFileID, &view.ProviderURL,
			&createdAt, &completedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
			return
		}
		view.CreatedAt = createdAt.UTC().Format(time.RFC3339Nano)
		if completedAt.Valid {
			view.CompletedAt = completedAt.Time.UTC().Format(time.RFC3339Nano)
		}
		out = append(out, view)
	}
	writeJSON(w, http.StatusOK, struct {
		Requests []DriveSaveRequestView `json:"requests"`
	}{Requests: out})
}

func (h *DriveSaveHandlers) resolveRule(ctx context.Context, req DriveSaveRequestBody) (rules.Rule, rules.Decision, error) {
	if req.RuleID != "" {
		rule, err := h.repo.Get(ctx, req.RuleID)
		if err != nil {
			return rules.Rule{}, rules.Decision{}, err
		}
		artifact := buildArtifactForEval(req)
		decision := h.engine.Evaluate(ctx, artifact, []rules.Rule{rule})
		return rule, decision, nil
	}
	all, err := h.repo.List(ctx)
	if err != nil {
		return rules.Rule{}, rules.Decision{}, err
	}
	artifact := buildArtifactForEval(req)
	decision := h.engine.Evaluate(ctx, artifact, all)
	if decision.Selected == nil {
		return rules.Rule{}, decision, nil
	}
	for _, rule := range all {
		if rule.ID == decision.Selected.RuleID {
			return rule, decision, nil
		}
	}
	return rules.Rule{}, decision, errors.New("save: matched rule id missing from repository result")
}

func buildArtifactForEval(req DriveSaveRequestBody) rules.Artifact {
	artifact := rules.Artifact{
		ID:             req.SourceArtifactID,
		SourceKind:     req.SourceKind,
		Classification: req.Classification,
		Sensitivity:    req.Sensitivity,
		Confidence:     req.Confidence,
		Tokens:         req.Tokens,
	}
	if req.CapturedAt != "" {
		if t, err := time.Parse(time.RFC3339, req.CapturedAt); err == nil {
			artifact.CapturedAt = t
		}
	}
	return artifact
}

func (h *DriveSaveHandlers) loadArtifactBytes(ctx context.Context, artifactID string) ([]byte, error) {
	var contentRaw string
	err := h.pool.QueryRow(ctx,
		`SELECT COALESCE(content_raw, '') FROM artifacts WHERE id=$1`, artifactID,
	).Scan(&contentRaw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("artifact %s not found", artifactID)
	}
	if err != nil {
		return nil, fmt.Errorf("load artifact: %w", err)
	}
	if contentRaw == "" {
		return nil, fmt.Errorf("artifact %s has no content_raw and no inline data_b64", artifactID)
	}
	return []byte(contentRaw), nil
}

func decodeSaveBody(raw string) ([]byte, error) {
	if raw == "" {
		return nil, nil
	}
	return base64.StdEncoding.DecodeString(raw)
}
