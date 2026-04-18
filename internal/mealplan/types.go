package mealplan

import "time"

// PlanStatus represents the lifecycle state of a meal plan.
type PlanStatus string

const (
	StatusDraft     PlanStatus = "draft"
	StatusActive    PlanStatus = "active"
	StatusCompleted PlanStatus = "completed"
	StatusArchived  PlanStatus = "archived"
)

// Plan represents a meal plan with a date range and lifecycle status.
type Plan struct {
	ID        string     `json:"id"`
	Title     string     `json:"title"`
	StartDate time.Time  `json:"start_date"`
	EndDate   time.Time  `json:"end_date"`
	Status    PlanStatus `json:"status"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// Slot represents a single recipe assignment to a date+meal in a plan.
type Slot struct {
	ID               string    `json:"id"`
	PlanID           string    `json:"plan_id"`
	SlotDate         time.Time `json:"slot_date"`
	MealType         string    `json:"meal_type"`
	RecipeArtifactID string    `json:"recipe_artifact_id"`
	Servings         int       `json:"servings"`
	BatchFlag        bool      `json:"batch_flag"`
	Notes            string    `json:"notes,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	// RecipeTitle is resolved at query time from artifacts; not stored in DB.
	RecipeTitle string `json:"recipe_title,omitempty"`
}

// PlanWithSlots combines a plan with its slot assignments.
type PlanWithSlots struct {
	Plan  Plan   `json:"plan"`
	Slots []Slot `json:"slots"`
}

// CopyResult describes the outcome of a plan copy operation.
type CopyResult struct {
	Plan         PlanWithSlots `json:"plan"`
	SlotsCopied  int           `json:"slots_copied"`
	SlotsSkipped []SkippedSlot `json:"slots_skipped,omitempty"`
}

// SkippedSlot records a slot that was omitted during plan copy.
type SkippedSlot struct {
	OriginalSlotID   string `json:"original_slot_id"`
	RecipeArtifactID string `json:"recipe_artifact_id"`
	Reason           string `json:"reason"`
}

// OverlapInfo describes an overlap between two plans.
type OverlapInfo struct {
	ConflictingPlanID    string `json:"conflicting_plan_id"`
	ConflictingPlanTitle string `json:"conflicting_plan_title"`
	OverlappingDays      int    `json:"overlapping_days"`
}

// ShoppingResult describes the outcome of shopping list generation from a plan.
type ShoppingResult struct {
	ListID         string                `json:"list_id"`
	Title          string                `json:"title"`
	ItemCount      int                   `json:"item_count"`
	ScalingSummary []ScalingSummaryEntry `json:"scaling_summary"`
	Skipped        []string              `json:"skipped,omitempty"`
}

// ScalingSummaryEntry describes how a recipe was scaled in shopping list generation.
type ScalingSummaryEntry struct {
	RecipeTitle   string `json:"recipe_title"`
	ArtifactID    string `json:"artifact_id"`
	Servings      int    `json:"servings"`
	Occurrences   int    `json:"occurrences"`
	TotalServings int    `json:"total_servings"`
}

// CalendarSyncResult describes the outcome of a CalDAV sync operation.
type CalendarSyncResult struct {
	EventsCreated int `json:"events_created"`
	EventsUpdated int `json:"events_updated"`
	EventsDeleted int `json:"events_deleted"`
	EventsFailed  int `json:"events_failed"`
}

// AllowedTransition returns true if transitioning from → to is valid.
func AllowedTransition(from, to PlanStatus) bool {
	switch from {
	case StatusDraft:
		return to == StatusActive
	case StatusActive:
		return to == StatusCompleted || to == StatusArchived
	case StatusCompleted:
		return to == StatusArchived
	case StatusArchived:
		return false
	}
	return false
}
