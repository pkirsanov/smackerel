package telegram

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/drive/rules"
	"github.com/smackerel/smackerel/internal/drive/save"
)

// DriveSaveBridge wraps the Spec 038 Scope 5 Save Service so the Telegram
// bot can persist receipt-shaped artifacts straight to the user's
// configured Drive provider after capture and reply with the destination
// folder + a correction action callback.
//
// The bridge is a thin orchestration helper: it owns no state, holds no
// timers, and delegates every behavioural decision (rule selection,
// idempotency, folder coalescing, retries) to rules.Engine and
// save.Service. Construction is dependency-injected so unit tests can pass
// in fakes; production wiring is in cmd/core/wiring.go.
type DriveSaveBridge struct {
	pool   *pgxpool.Pool
	repo   *rules.Repository
	engine *rules.Engine
	svc    *save.Service
}

// NewDriveSaveBridge constructs the bridge. All four dependencies are
// required; nil arguments produce an explicit panic at startup so the
// runtime fails loud instead of silently swallowing receipt save events.
func NewDriveSaveBridge(pool *pgxpool.Pool, repo *rules.Repository, engine *rules.Engine, svc *save.Service) *DriveSaveBridge {
	if pool == nil || repo == nil || engine == nil || svc == nil {
		panic("telegram: NewDriveSaveBridge requires pool, repo, engine, and save service")
	}
	return &DriveSaveBridge{pool: pool, repo: repo, engine: engine, svc: svc}
}

// ReceiptSaveInput carries the Telegram-side context the bridge needs to
// score a captured artifact against the Save Rules and persist it.
type ReceiptSaveInput struct {
	ArtifactID     string
	Classification string
	Sensitivity    string
	Confidence     float64
	Tokens         map[string]string
	Title          string
	MimeType       string
	Body           []byte
}

// ReceiptSaveOutcome describes the bridge result for the Telegram reply.
type ReceiptSaveOutcome struct {
	Saved        bool
	Skipped      bool
	Folder       string
	ProviderURL  string
	RuleID       string
	Reason       string
	LastError    string
	RequiresConf bool
}

// SaveReceipt evaluates the configured Save Rules against the supplied
// artifact metadata and, when a rule matches, calls the Save Service. It
// returns a structured outcome the Telegram bot can render as a reply
// message and pair with a "Wrong folder?" correction action.
func (b *DriveSaveBridge) SaveReceipt(ctx context.Context, in ReceiptSaveInput) (ReceiptSaveOutcome, error) {
	if strings.TrimSpace(in.ArtifactID) == "" {
		return ReceiptSaveOutcome{}, errors.New("telegram: SaveReceipt: artifact_id required")
	}
	if strings.TrimSpace(in.Title) == "" {
		return ReceiptSaveOutcome{}, errors.New("telegram: SaveReceipt: title required")
	}
	if len(in.Body) == 0 {
		// Allow callers to pass a zero-byte body ONLY when content_raw
		// already exists in artifacts; the bridge falls back to that.
		body, err := b.loadArtifactBytes(ctx, in.ArtifactID)
		if err != nil {
			return ReceiptSaveOutcome{}, err
		}
		in.Body = body
	}

	all, err := b.repo.List(ctx)
	if err != nil {
		return ReceiptSaveOutcome{}, fmt.Errorf("telegram: SaveReceipt: list rules: %w", err)
	}
	artifact := rules.Artifact{
		ID:             in.ArtifactID,
		SourceKind:     string(rules.SourceTelegram),
		Classification: in.Classification,
		Sensitivity:    in.Sensitivity,
		Confidence:     in.Confidence,
		Tokens:         in.Tokens,
	}
	decision := b.engine.Evaluate(ctx, artifact, all)
	if decision.Selected == nil {
		_ = b.repo.AppendAudit(ctx, "", in.ArtifactID, rules.OutcomeSkipped, "no_rule_matched")
		return ReceiptSaveOutcome{Skipped: true, Reason: "no_rule_matched"}, nil
	}
	var rule rules.Rule
	for _, r := range all {
		if r.ID == decision.Selected.RuleID {
			rule = r
			break
		}
	}
	if rule.ID == "" {
		return ReceiptSaveOutcome{}, errors.New("telegram: SaveReceipt: matched rule missing from repository")
	}
	if decision.Selected.RenderError != nil {
		_ = b.repo.AppendAudit(ctx, rule.ID, in.ArtifactID, rules.OutcomeFailed, decision.Selected.RenderError.Error())
		return ReceiptSaveOutcome{
			RuleID:    rule.ID,
			LastError: decision.Selected.RenderError.Error(),
			Reason:    "render_error",
		}, nil
	}
	req := save.Request{
		Rule:             rule,
		SourceArtifactID: in.ArtifactID,
		ConfirmRequired:  decision.Selected.ConfirmRequired,
		RenderedPath:     decision.Selected.RenderedPath,
		Bytes: save.Bytes{
			Title:    in.Title,
			MimeType: in.MimeType,
			Body:     in.Body,
		},
	}
	res, err := b.svc.Save(ctx, req)
	if err != nil {
		_ = b.repo.AppendAudit(ctx, rule.ID, in.ArtifactID, rules.OutcomeFailed, err.Error())
		return ReceiptSaveOutcome{
			RuleID:    rule.ID,
			LastError: err.Error(),
			Reason:    "save_failed",
		}, err
	}
	outcome := ReceiptSaveOutcome{
		RuleID:       rule.ID,
		Folder:       res.TargetPath,
		ProviderURL:  res.ProviderURL,
		Reason:       decision.Selected.Reason,
		RequiresConf: res.Status == save.StatusAwaitingConfirmation,
		Saved:        res.Status == save.StatusWritten,
	}
	auditOutcome := rules.OutcomeMatched
	if outcome.RequiresConf {
		auditOutcome = rules.OutcomeAwaitingConfirmation
	}
	_ = b.repo.AppendAudit(ctx, rule.ID, in.ArtifactID, auditOutcome, "rendered_path="+res.TargetPath)
	return outcome, nil
}

// FormatReceiptReply renders the bot reply message for an outcome. Public
// so unit tests can pin the wording without instantiating a real bot.
func FormatReceiptReply(outcome ReceiptSaveOutcome) string {
	switch {
	case outcome.Saved && outcome.ProviderURL != "":
		return fmt.Sprintf("📁 Saved to Drive: %s\n%s", outcome.Folder, outcome.ProviderURL)
	case outcome.Saved:
		return fmt.Sprintf("📁 Saved to Drive: %s", outcome.Folder)
	case outcome.RequiresConf:
		return fmt.Sprintf("⚠️ Receipt matched rule %s but confidence is below threshold — confirm to save to %s.", outcome.RuleID, outcome.Folder)
	case outcome.Skipped:
		return "Receipt captured. No save rule matched, so the file stays in Smackerel only."
	case outcome.LastError != "":
		return fmt.Sprintf("Receipt captured but Drive save failed: %s", outcome.LastError)
	default:
		return "Receipt captured."
	}
}

func (b *DriveSaveBridge) loadArtifactBytes(ctx context.Context, artifactID string) ([]byte, error) {
	var contentRaw string
	err := b.pool.QueryRow(ctx,
		`SELECT COALESCE(content_raw, '') FROM artifacts WHERE id=$1`, artifactID,
	).Scan(&contentRaw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("telegram: SaveReceipt: artifact %s not found", artifactID)
	}
	if err != nil {
		return nil, fmt.Errorf("telegram: SaveReceipt: load artifact: %w", err)
	}
	if contentRaw == "" {
		return nil, fmt.Errorf("telegram: SaveReceipt: artifact %s has no content_raw and no inline body", artifactID)
	}
	return []byte(contentRaw), nil
}
