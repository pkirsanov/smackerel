package mealplan

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// Service implements meal plan business logic.
type Service struct {
	Store            PlanDataStore
	MealTypes        []string
	DefaultServings  int
	CalendarSync     bool
	AutoComplete     bool
	AutoCompleteCron string
}

// truncateToDate strips the time component from a time.Time value,
// keeping only the date at midnight UTC.
func truncateToDate(t time.Time) time.Time {
	return t.Truncate(24 * time.Hour)
}

// NewService creates a new meal plan service.
func NewService(store PlanDataStore, mealTypes []string, defaultServings int, calendarSync, autoComplete bool, autoCompleteCron string) *Service {
	return &Service{
		Store:            store,
		MealTypes:        mealTypes,
		DefaultServings:  defaultServings,
		CalendarSync:     calendarSync,
		AutoComplete:     autoComplete,
		AutoCompleteCron: autoCompleteCron,
	}
}

// generateID creates a unique identifier with a prefix.
// Uses timestamp + random suffix to avoid collisions under concurrent access.
func generateID(prefix string) string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%s-%d-%x", prefix, time.Now().UnixNano(), b)
}

// CreatePlan creates a new draft meal plan.
func (s *Service) CreatePlan(ctx context.Context, title string, startDate, endDate time.Time) (*PlanWithSlots, error) {
	if title == "" {
		return nil, fmt.Errorf("title is required")
	}
	if len(title) > 200 {
		return nil, fmt.Errorf("title must be at most 200 characters")
	}

	// Normalize to date-only (strip time)
	startDate = truncateToDate(startDate)
	endDate = truncateToDate(endDate)

	if endDate.Before(startDate) {
		return nil, fmt.Errorf("end_date must be on or after start_date")
	}

	now := time.Now()
	plan := &Plan{
		ID:        generateID("mp"),
		Title:     title,
		StartDate: startDate,
		EndDate:   endDate,
		Status:    StatusDraft,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := s.Store.CreatePlan(ctx, plan); err != nil {
		return nil, fmt.Errorf("create plan: %w", err)
	}

	return &PlanWithSlots{Plan: *plan, Slots: []Slot{}}, nil
}

// GetPlan retrieves a plan with all its slots.
func (s *Service) GetPlan(ctx context.Context, planID string) (*PlanWithSlots, error) {
	return s.Store.GetPlanWithSlots(ctx, planID)
}

// ListPlans returns plans matching the given filters.
func (s *Service) ListPlans(ctx context.Context, statusFilter string, fromDate, toDate *time.Time) ([]Plan, error) {
	return s.Store.ListPlans(ctx, statusFilter, fromDate, toDate)
}

// DeletePlan removes a plan and all its slots.
func (s *Service) DeletePlan(ctx context.Context, planID string) error {
	return s.Store.DeletePlan(ctx, planID)
}

const (
	maxSlotNotesLen = 500
	maxSlotServings = 1000
)

// AddSlot assigns a recipe to a date+meal slot in a plan.
func (s *Service) AddSlot(ctx context.Context, planID string, slotDate time.Time, mealType, recipeArtifactID string, servings int, batchFlag bool, notes string) (*Slot, error) {
	if len(notes) > maxSlotNotesLen {
		return nil, &ServiceError{
			Code:    "MEAL_PLAN_VALIDATION",
			Message: fmt.Sprintf("notes must be at most %d characters", maxSlotNotesLen),
			Status:  422,
		}
	}
	if servings > maxSlotServings {
		return nil, &ServiceError{
			Code:    "MEAL_PLAN_VALIDATION",
			Message: fmt.Sprintf("servings must be at most %d", maxSlotServings),
			Status:  422,
		}
	}

	plan, err := s.Store.GetPlan(ctx, planID)
	if err != nil {
		return nil, fmt.Errorf("get plan: %w", err)
	}
	if plan == nil {
		return nil, &ServiceError{Code: "MEAL_PLAN_NOT_FOUND", Message: "plan not found", Status: 404}
	}

	// Validate meal type
	if !s.isValidMealType(mealType) {
		return nil, &ServiceError{
			Code:    "MEAL_PLAN_INVALID_MEAL_TYPE",
			Message: fmt.Sprintf("%q is not a configured meal type; available: %s", mealType, strings.Join(s.MealTypes, ", ")),
			Status:  422,
		}
	}

	// Normalize date
	slotDate = truncateToDate(slotDate)

	// Validate slot date within plan range
	planStart := truncateToDate(plan.StartDate)
	planEnd := truncateToDate(plan.EndDate)
	if slotDate.Before(planStart) || slotDate.After(planEnd) {
		return nil, &ServiceError{
			Code:    "MEAL_PLAN_SLOT_OUT_OF_RANGE",
			Message: fmt.Sprintf("slot date %s is outside plan range %s to %s", slotDate.Format("2006-01-02"), planStart.Format("2006-01-02"), planEnd.Format("2006-01-02")),
			Status:  422,
		}
	}

	// Validate recipe exists
	exists, err := s.Store.RecipeArtifactExists(ctx, recipeArtifactID)
	if err != nil {
		return nil, fmt.Errorf("check recipe: %w", err)
	}
	if !exists {
		return nil, &ServiceError{Code: "MEAL_PLAN_RECIPE_NOT_FOUND", Message: "recipe artifact not found", Status: 422}
	}

	// Default servings
	if servings <= 0 {
		servings = s.DefaultServings
	}

	// Check for existing slot conflict
	existing, err := s.Store.GetSlotByDateMeal(ctx, planID, slotDate, mealType)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, &ServiceError{
			Code:    "MEAL_PLAN_SLOT_CONFLICT",
			Message: fmt.Sprintf("%s %s already has %s (%d servings)", slotDate.Format("2006-01-02"), mealType, existing.RecipeTitle, existing.Servings),
			Status:  409,
			Details: existing,
		}
	}

	slot := &Slot{
		ID:               generateID("mps"),
		PlanID:           planID,
		SlotDate:         slotDate,
		MealType:         mealType,
		RecipeArtifactID: recipeArtifactID,
		Servings:         servings,
		BatchFlag:        batchFlag,
		Notes:            notes,
		CreatedAt:        time.Now(),
	}

	if err := s.Store.AddSlot(ctx, slot); err != nil {
		return nil, fmt.Errorf("add slot: %w", err)
	}

	_ = s.Store.MarkPlanUpdated(ctx, planID)
	return slot, nil
}

// UpdateSlot updates a slot's mutable fields.
func (s *Service) UpdateSlot(ctx context.Context, planID, slotID string, recipeArtifactID string, servings int, batchFlag bool, notes string) (*Slot, error) {
	if len(notes) > maxSlotNotesLen {
		return nil, &ServiceError{
			Code:    "MEAL_PLAN_VALIDATION",
			Message: fmt.Sprintf("notes must be at most %d characters", maxSlotNotesLen),
			Status:  422,
		}
	}
	if servings > maxSlotServings {
		return nil, &ServiceError{
			Code:    "MEAL_PLAN_VALIDATION",
			Message: fmt.Sprintf("servings must be at most %d", maxSlotServings),
			Status:  422,
		}
	}

	existing, err := s.Store.GetSlot(ctx, planID, slotID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, &ServiceError{Code: "MEAL_PLAN_SLOT_NOT_FOUND", Message: "slot not found", Status: 404}
	}

	if recipeArtifactID != "" {
		existing.RecipeArtifactID = recipeArtifactID
	}
	if servings > 0 {
		existing.Servings = servings
	}
	existing.BatchFlag = batchFlag
	existing.Notes = notes

	if err := s.Store.UpdateSlot(ctx, existing); err != nil {
		return nil, err
	}
	_ = s.Store.MarkPlanUpdated(ctx, planID)
	return existing, nil
}

// DeleteSlot removes a slot from a plan.
func (s *Service) DeleteSlot(ctx context.Context, planID, slotID string) error {
	if err := s.Store.DeleteSlot(ctx, planID, slotID); err != nil {
		// Store.DeleteSlot returns a formatted "slot not found" message for
		// missing rows. Wrap only that case as a 404; propagate other errors.
		if strings.Contains(err.Error(), "slot not found") {
			return &ServiceError{Code: "MEAL_PLAN_SLOT_NOT_FOUND", Message: err.Error(), Status: 404}
		}
		return fmt.Errorf("delete slot: %w", err)
	}
	_ = s.Store.MarkPlanUpdated(ctx, planID)
	return nil
}

// AddBatchSlots creates one slot per day across a date range for the same recipe+meal.
func (s *Service) AddBatchSlots(ctx context.Context, planID string, startDate, endDate time.Time, mealType, recipeArtifactID string, servings int) ([]Slot, error) {
	startDate = truncateToDate(startDate)
	endDate = truncateToDate(endDate)

	var slots []Slot
	for d := startDate; !d.After(endDate); d = d.AddDate(0, 0, 1) {
		slot, err := s.AddSlot(ctx, planID, d, mealType, recipeArtifactID, servings, true, "")
		if err != nil {
			return slots, fmt.Errorf("batch slot for %s: %w", d.Format("2006-01-02"), err)
		}
		slots = append(slots, *slot)
	}
	return slots, nil
}

// ActivatePlan transitions a plan from draft to active, with overlap detection.
func (s *Service) ActivatePlan(ctx context.Context, planID string, force bool) (*OverlapInfo, error) {
	plan, err := s.Store.GetPlan(ctx, planID)
	if err != nil {
		return nil, err
	}
	if plan == nil {
		return nil, &ServiceError{Code: "MEAL_PLAN_NOT_FOUND", Message: "plan not found", Status: 404}
	}

	if !AllowedTransition(plan.Status, StatusActive) {
		return nil, &ServiceError{
			Code:    "MEAL_PLAN_INVALID_TRANSITION",
			Message: fmt.Sprintf("cannot transition from %s to active", plan.Status),
			Status:  422,
		}
	}

	// Check for overlapping active plans
	if !force {
		overlapping, err := s.Store.FindOverlappingPlans(ctx, plan.StartDate, plan.EndDate, planID)
		if err != nil {
			return nil, err
		}
		if len(overlapping) > 0 {
			conflict := overlapping[0]
			overlapStart := plan.StartDate
			if conflict.StartDate.After(overlapStart) {
				overlapStart = conflict.StartDate
			}
			overlapEnd := plan.EndDate
			if conflict.EndDate.Before(overlapEnd) {
				overlapEnd = conflict.EndDate
			}
			days := int(overlapEnd.Sub(overlapStart).Hours()/24) + 1

			info := &OverlapInfo{
				ConflictingPlanID:    conflict.ID,
				ConflictingPlanTitle: conflict.Title,
				OverlappingDays:      days,
			}
			return info, &ServiceError{
				Code:    "MEAL_PLAN_OVERLAP",
				Message: fmt.Sprintf("%d days overlap with %q", days, conflict.Title),
				Status:  409,
				Details: info,
			}
		}
	}

	if err := s.Store.UpdatePlanStatus(ctx, planID, StatusActive); err != nil {
		return nil, err
	}
	return nil, nil
}

// TransitionPlan changes a plan's status.
func (s *Service) TransitionPlan(ctx context.Context, planID string, newStatus PlanStatus) error {
	plan, err := s.Store.GetPlan(ctx, planID)
	if err != nil {
		return err
	}
	if plan == nil {
		return &ServiceError{Code: "MEAL_PLAN_NOT_FOUND", Message: "plan not found", Status: 404}
	}

	if !AllowedTransition(plan.Status, newStatus) {
		return &ServiceError{
			Code:    "MEAL_PLAN_INVALID_TRANSITION",
			Message: fmt.Sprintf("cannot transition from %s to %s", plan.Status, newStatus),
			Status:  422,
		}
	}

	return s.Store.UpdatePlanStatus(ctx, planID, newStatus)
}

// UpdatePlanTitle updates a plan's title.
func (s *Service) UpdatePlanTitle(ctx context.Context, planID, title string) error {
	if title == "" {
		return fmt.Errorf("title is required")
	}
	if len(title) > 200 {
		return fmt.Errorf("title must be at most 200 characters")
	}
	return s.Store.UpdatePlanTitle(ctx, planID, title)
}

// QueryByDate looks up planned meals for a date and optional meal type.
func (s *Service) QueryByDate(ctx context.Context, date time.Time, mealType string) ([]Slot, *Plan, error) {
	date = truncateToDate(date)
	return s.Store.GetSlotsByDate(ctx, date, mealType)
}

// CopyPlan duplicates a plan with date-shifted slots.
func (s *Service) CopyPlan(ctx context.Context, sourcePlanID string, newStartDate time.Time, newTitle string, servingOverrides map[string]int) (*CopyResult, error) {
	source, err := s.Store.GetPlanWithSlots(ctx, sourcePlanID)
	if err != nil {
		return nil, err
	}
	if source == nil {
		return nil, &ServiceError{Code: "MEAL_PLAN_NOT_FOUND", Message: "source plan not found", Status: 404}
	}

	newStartDate = truncateToDate(newStartDate)
	sourceStart := truncateToDate(source.Plan.StartDate)
	dayOffset := newStartDate.Sub(sourceStart)
	newEndDate := source.Plan.EndDate.Add(dayOffset)

	if newTitle == "" {
		newTitle = source.Plan.Title
	}

	now := time.Now()
	newPlan := &Plan{
		ID:        generateID("mp"),
		Title:     newTitle,
		StartDate: newStartDate,
		EndDate:   newEndDate,
		Status:    StatusDraft,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := s.Store.CreatePlan(ctx, newPlan); err != nil {
		return nil, fmt.Errorf("create copied plan: %w", err)
	}

	result := &CopyResult{
		Plan: PlanWithSlots{Plan: *newPlan, Slots: []Slot{}},
	}

	for _, srcSlot := range source.Slots {
		// Check recipe still exists
		exists, err := s.Store.RecipeArtifactExists(ctx, srcSlot.RecipeArtifactID)
		if err != nil {
			slog.Warn("copy plan: failed to check recipe", "artifact_id", srcSlot.RecipeArtifactID, "error", err)
			result.SlotsSkipped = append(result.SlotsSkipped, SkippedSlot{
				OriginalSlotID:   srcSlot.ID,
				RecipeArtifactID: srcSlot.RecipeArtifactID,
				Reason:           "failed to verify recipe artifact",
			})
			continue
		}
		if !exists {
			result.SlotsSkipped = append(result.SlotsSkipped, SkippedSlot{
				OriginalSlotID:   srcSlot.ID,
				RecipeArtifactID: srcSlot.RecipeArtifactID,
				Reason:           "recipe artifact not found",
			})
			continue
		}

		newSlotDate := srcSlot.SlotDate.Add(dayOffset)
		servings := srcSlot.Servings
		if override, ok := servingOverrides[srcSlot.MealType]; ok && override > 0 {
			servings = override
		}

		newSlot := &Slot{
			ID:               generateID("mps"),
			PlanID:           newPlan.ID,
			SlotDate:         newSlotDate,
			MealType:         srcSlot.MealType,
			RecipeArtifactID: srcSlot.RecipeArtifactID,
			Servings:         servings,
			BatchFlag:        srcSlot.BatchFlag,
			Notes:            srcSlot.Notes,
			CreatedAt:        now,
		}

		if err := s.Store.AddSlot(ctx, newSlot); err != nil {
			slog.Warn("copy plan: failed to add slot", "slot_date", newSlotDate, "error", err)
			result.SlotsSkipped = append(result.SlotsSkipped, SkippedSlot{
				OriginalSlotID:   srcSlot.ID,
				RecipeArtifactID: srcSlot.RecipeArtifactID,
				Reason:           fmt.Sprintf("failed to create slot: %v", err),
			})
			continue
		}

		result.Plan.Slots = append(result.Plan.Slots, *newSlot)
		result.SlotsCopied++
	}

	return result, nil
}

// AutoCompletePastPlans transitions expired active plans to completed.
func (s *Service) AutoCompletePastPlans(ctx context.Context) (int, error) {
	return s.Store.AutoCompletePastPlans(ctx)
}

func (s *Service) isValidMealType(mealType string) bool {
	for _, mt := range s.MealTypes {
		if mt == mealType {
			return true
		}
	}
	return false
}

// ServiceError is a typed error with HTTP status and error code.
type ServiceError struct {
	Code    string
	Message string
	Status  int
	Details any
}

func (e *ServiceError) Error() string {
	return e.Message
}
