package mealplan

import (
	"context"
	"time"
)

// PlanDataStore defines the data-access contract for meal plan operations.
// *Store satisfies this interface. Tests can substitute a mock implementation.
type PlanDataStore interface {
	CreatePlan(ctx context.Context, plan *Plan) error
	GetPlan(ctx context.Context, planID string) (*Plan, error)
	GetPlanWithSlots(ctx context.Context, planID string) (*PlanWithSlots, error)
	ListPlans(ctx context.Context, statusFilter string, fromDate, toDate *time.Time) ([]Plan, error)
	UpdatePlanStatus(ctx context.Context, planID string, status PlanStatus) error
	UpdatePlanTitle(ctx context.Context, planID, title string) error
	DeletePlan(ctx context.Context, planID string) error
	AddSlot(ctx context.Context, slot *Slot) error
	GetSlot(ctx context.Context, planID, slotID string) (*Slot, error)
	UpdateSlot(ctx context.Context, slot *Slot) error
	DeleteSlot(ctx context.Context, planID, slotID string) error
	GetSlotsByDate(ctx context.Context, date time.Time, mealType string) ([]Slot, *Plan, error)
	FindOverlappingPlans(ctx context.Context, startDate, endDate time.Time, excludePlanID string) ([]Plan, error)
	AutoCompletePastPlans(ctx context.Context) (int, error)
	RecipeArtifactExists(ctx context.Context, artifactID string) (bool, error)
	GetSlotByDateMeal(ctx context.Context, planID string, date time.Time, mealType string) (*Slot, error)
	MarkPlanUpdated(ctx context.Context, planID string) error
}
