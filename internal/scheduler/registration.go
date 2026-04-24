package scheduler

import (
	"context"

	"github.com/smackerel/smackerel/internal/knowledge"
)

// MealPlanAutoCompleter is the interface for auto-completing meal plans.
type MealPlanAutoCompleter interface {
	AutoCompletePastPlans(ctx context.Context) (int, error)
}

// SetKnowledgeLinter configures the knowledge linter and its cron expression.
// Must be called before Start().
func (s *Scheduler) SetKnowledgeLinter(linter *knowledge.Linter, cronExpr string) {
	s.knowledgeLinter = linter
	s.knowledgeLintCron = cronExpr
}

// SetMealPlanAutoComplete configures the meal plan auto-complete job.
// Must be called before Start().
func (s *Scheduler) SetMealPlanAutoComplete(svc MealPlanAutoCompleter, cronExpr string) {
	s.mealPlanSvc = svc
	s.mealPlanCron = cronExpr
}
