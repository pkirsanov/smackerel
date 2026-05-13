package mealplan

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/smackerel/smackerel/internal/drive/rules"
	"github.com/smackerel/smackerel/internal/drive/save"
)

// DriveSaveBack is the spec 038 Scope 5 meal-plan write-back helper. It
// renders a meal plan to a portable text payload (Markdown by default —
// PDF rendering is intentionally out of scope until rendering deps land
// in spec 036 Scope 7) and pushes it to the user's Drive provider via
// the shared save.Service. The resulting provider URL is recorded back
// onto the plan via UpdatePlanProviderURL so downstream digest generation
// can include a "Open meal plan in Drive" link.
type DriveSaveBack struct {
	repo   *rules.Repository
	engine *rules.Engine
	svc    *save.Service
	store  PlanProviderURLStore
}

// PlanProviderURLStore is the store surface DriveSaveBack needs to load
// a plan with its slots and to persist the provider URL onto a plan row.
// It is satisfied by mealplan.Store.
type PlanProviderURLStore interface {
	GetPlanWithSlots(ctx context.Context, planID string) (*PlanWithSlots, error)
	UpdatePlanProviderURL(ctx context.Context, planID, providerURL string) error
}

// NewDriveSaveBack constructs the helper. All parameters are required;
// nil arguments produce an explicit panic at startup so the runtime fails
// loud instead of silently swallowing meal-plan saves.
func NewDriveSaveBack(repo *rules.Repository, engine *rules.Engine, svc *save.Service, store PlanProviderURLStore) *DriveSaveBack {
	if repo == nil || engine == nil || svc == nil || store == nil {
		panic("mealplan: NewDriveSaveBack requires repo, engine, save service, and store")
	}
	return &DriveSaveBack{repo: repo, engine: engine, svc: svc, store: store}
}

// MealPlanSaveOutcome describes the save-back result for the digest layer.
type MealPlanSaveOutcome struct {
	Saved       bool
	Folder      string
	ProviderURL string
	RuleID      string
	Reason      string
	LastError   string
}

// SavePlan renders the supplied plan + slots into a Markdown payload, runs
// the Save Rules engine with SourceKind=meal_plan, and saves to Drive.
// The resulting provider URL is recorded onto the plan row so the digest
// generator can expose it under the meal-plan section.
//
// Title defaults to the plan's title when empty. Tokens automatically
// include {plan_title}, {start_date}, {end_date}, {year}, {month}, and
// {classification} so rule templates like "Plans/{year}/{plan_title}.md"
// just work.
func (b *DriveSaveBack) SavePlan(ctx context.Context, planID, artifactID string) (MealPlanSaveOutcome, error) {
	if strings.TrimSpace(planID) == "" {
		return MealPlanSaveOutcome{}, errors.New("mealplan: SavePlan: plan_id required")
	}
	if strings.TrimSpace(artifactID) == "" {
		return MealPlanSaveOutcome{}, errors.New("mealplan: SavePlan: artifact_id required")
	}
	pws, err := b.store.GetPlanWithSlots(ctx, planID)
	if err != nil {
		return MealPlanSaveOutcome{}, fmt.Errorf("mealplan: SavePlan: load plan: %w", err)
	}
	if pws == nil {
		return MealPlanSaveOutcome{}, fmt.Errorf("mealplan: SavePlan: plan not found: %s", planID)
	}
	body := renderPlanMarkdown(pws)
	tokens := map[string]string{
		"plan_title":     sanitizeFilename(pws.Plan.Title),
		"start_date":     pws.Plan.StartDate.Format("2006-01-02"),
		"end_date":       pws.Plan.EndDate.Format("2006-01-02"),
		"classification": "meal_plan",
	}

	all, err := b.repo.List(ctx)
	if err != nil {
		return MealPlanSaveOutcome{}, fmt.Errorf("mealplan: SavePlan: list rules: %w", err)
	}
	artifact := rules.Artifact{
		ID:             artifactID,
		SourceKind:     string(rules.SourceMealPlan),
		Classification: "meal_plan",
		Sensitivity:    string(rules.SensitivityNone),
		Confidence:     1.0,
		Tokens:         tokens,
		CapturedAt:     time.Now().UTC(),
	}
	decision := b.engine.Evaluate(ctx, artifact, all)
	if decision.Selected == nil {
		_ = b.repo.AppendAudit(ctx, "", artifactID, rules.OutcomeSkipped, "no_rule_matched")
		return MealPlanSaveOutcome{Reason: "no_rule_matched"}, nil
	}
	var rule rules.Rule
	for _, r := range all {
		if r.ID == decision.Selected.RuleID {
			rule = r
			break
		}
	}
	if rule.ID == "" {
		return MealPlanSaveOutcome{}, errors.New("mealplan: SavePlan: matched rule missing from repository")
	}
	if decision.Selected.RenderError != nil {
		_ = b.repo.AppendAudit(ctx, rule.ID, artifactID, rules.OutcomeFailed, decision.Selected.RenderError.Error())
		return MealPlanSaveOutcome{
			RuleID:    rule.ID,
			LastError: decision.Selected.RenderError.Error(),
			Reason:    "render_error",
		}, nil
	}
	title := sanitizeFilename(pws.Plan.Title) + ".md"
	res, err := b.svc.Save(ctx, save.Request{
		Rule:             rule,
		SourceArtifactID: artifactID,
		ConfirmRequired:  decision.Selected.ConfirmRequired,
		RenderedPath:     decision.Selected.RenderedPath,
		Bytes: save.Bytes{
			Title:    title,
			MimeType: "text/markdown",
			Body:     []byte(body),
		},
	})
	if err != nil {
		_ = b.repo.AppendAudit(ctx, rule.ID, artifactID, rules.OutcomeFailed, err.Error())
		return MealPlanSaveOutcome{
			RuleID:    rule.ID,
			LastError: err.Error(),
			Reason:    "save_failed",
		}, err
	}

	if res.Status == save.StatusWritten && res.ProviderURL != "" {
		if err := b.store.UpdatePlanProviderURL(ctx, planID, res.ProviderURL); err != nil {
			return MealPlanSaveOutcome{}, fmt.Errorf("mealplan: SavePlan: persist provider_url: %w", err)
		}
	}
	auditOutcome := rules.OutcomeMatched
	if res.Status == save.StatusAwaitingConfirmation {
		auditOutcome = rules.OutcomeAwaitingConfirmation
	}
	_ = b.repo.AppendAudit(ctx, rule.ID, artifactID, auditOutcome, "rendered_path="+res.TargetPath)
	return MealPlanSaveOutcome{
		Saved:       res.Status == save.StatusWritten,
		Folder:      res.TargetPath,
		ProviderURL: res.ProviderURL,
		RuleID:      rule.ID,
		Reason:      decision.Selected.Reason,
	}, nil
}

func renderPlanMarkdown(pws *PlanWithSlots) string {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "# %s\n\n", pws.Plan.Title)
	fmt.Fprintf(&buf, "_Start_: %s\n", pws.Plan.StartDate.Format("2006-01-02"))
	fmt.Fprintf(&buf, "_End_: %s\n", pws.Plan.EndDate.Format("2006-01-02"))
	fmt.Fprintf(&buf, "_Status_: %s\n\n", pws.Plan.Status)
	if len(pws.Slots) == 0 {
		fmt.Fprintln(&buf, "_(no slots)_")
		return buf.String()
	}
	fmt.Fprintln(&buf, "## Slots")
	fmt.Fprintln(&buf)
	for _, s := range pws.Slots {
		label := s.RecipeTitle
		if label == "" && s.RecipeArtifactID != "" {
			label = "recipe:" + s.RecipeArtifactID
		}
		if label == "" {
			label = "(unspecified)"
		}
		fmt.Fprintf(&buf, "- **%s** %s — %s (servings: %d)\n",
			s.SlotDate.Format("2006-01-02"), s.MealType, label, s.Servings)
		if s.Notes != "" {
			fmt.Fprintf(&buf, "  > %s\n", s.Notes)
		}
	}
	return buf.String()
}

func sanitizeFilename(name string) string {
	cleaned := strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|':
			return '_'
		case '\n', '\r', '\t':
			return ' '
		default:
			return r
		}
	}, name)
	cleaned = strings.TrimSpace(cleaned)
	if cleaned == "" {
		return "meal-plan"
	}
	return cleaned
}
