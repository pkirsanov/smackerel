package mealplan

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// --- mockPlanStore implements PlanDataStore for unit testing ---

type mockPlanStore struct {
	plans     map[string]*Plan
	slots     map[string]*Slot  // keyed by slot ID
	planSlots map[string][]Slot // keyed by plan ID

	createPlanErr          error
	addSlotErr             error
	deleteSlotErr          error
	updateSlotErr          error
	recipeArtifactExistsOK bool
	recipeExistsErr        error
	overlappingPlans       []Plan
	findOverlapErr         error
	autoCompletedCount     int
	autoCompleteErr        error
}

func newMockPlanStore() *mockPlanStore {
	return &mockPlanStore{
		plans:                  make(map[string]*Plan),
		slots:                  make(map[string]*Slot),
		planSlots:              make(map[string][]Slot),
		recipeArtifactExistsOK: true,
	}
}

func (m *mockPlanStore) CreatePlan(_ context.Context, plan *Plan) error {
	if m.createPlanErr != nil {
		return m.createPlanErr
	}
	m.plans[plan.ID] = plan
	return nil
}

func (m *mockPlanStore) GetPlan(_ context.Context, planID string) (*Plan, error) {
	p, ok := m.plans[planID]
	if !ok {
		return nil, nil
	}
	return p, nil
}

func (m *mockPlanStore) GetPlanWithSlots(_ context.Context, planID string) (*PlanWithSlots, error) {
	p, ok := m.plans[planID]
	if !ok {
		return nil, nil
	}
	return &PlanWithSlots{Plan: *p, Slots: m.planSlots[planID]}, nil
}

func (m *mockPlanStore) ListPlans(_ context.Context, statusFilter string, _, _ *time.Time) ([]Plan, error) {
	var result []Plan
	for _, p := range m.plans {
		if statusFilter == "" || p.Status == PlanStatus(statusFilter) {
			result = append(result, *p)
		}
	}
	return result, nil
}

func (m *mockPlanStore) UpdatePlanStatus(_ context.Context, planID string, status PlanStatus) error {
	p, ok := m.plans[planID]
	if !ok {
		return fmt.Errorf("plan not found: %s", planID)
	}
	p.Status = status
	return nil
}

func (m *mockPlanStore) UpdatePlanTitle(_ context.Context, planID, title string) error {
	p, ok := m.plans[planID]
	if !ok {
		return fmt.Errorf("plan not found: %s", planID)
	}
	p.Title = title
	return nil
}

func (m *mockPlanStore) DeletePlan(_ context.Context, planID string) error {
	if _, ok := m.plans[planID]; !ok {
		return fmt.Errorf("plan not found: %s", planID)
	}
	delete(m.plans, planID)
	// Cascade: remove slots belonging to this plan
	for slotID, sl := range m.slots {
		if sl.PlanID == planID {
			delete(m.slots, slotID)
		}
	}
	delete(m.planSlots, planID)
	return nil
}

func (m *mockPlanStore) AddSlot(_ context.Context, slot *Slot) error {
	if m.addSlotErr != nil {
		return m.addSlotErr
	}
	m.slots[slot.ID] = slot
	m.planSlots[slot.PlanID] = append(m.planSlots[slot.PlanID], *slot)
	return nil
}

func (m *mockPlanStore) GetSlot(_ context.Context, planID, slotID string) (*Slot, error) {
	sl, ok := m.slots[slotID]
	if !ok || sl.PlanID != planID {
		return nil, nil
	}
	return sl, nil
}

func (m *mockPlanStore) UpdateSlot(_ context.Context, slot *Slot) error {
	if m.updateSlotErr != nil {
		return m.updateSlotErr
	}
	m.slots[slot.ID] = slot
	return nil
}

func (m *mockPlanStore) DeleteSlot(_ context.Context, planID, slotID string) error {
	if m.deleteSlotErr != nil {
		return m.deleteSlotErr
	}
	sl, ok := m.slots[slotID]
	if !ok || sl.PlanID != planID {
		return fmt.Errorf("slot not found: %s in plan %s", slotID, planID)
	}
	delete(m.slots, slotID)
	return nil
}

func (m *mockPlanStore) GetSlotsByDate(_ context.Context, date time.Time, mealType string) ([]Slot, *Plan, error) {
	for _, p := range m.plans {
		if p.Status == StatusActive && !date.Before(p.StartDate) && !date.After(p.EndDate) {
			var matched []Slot
			for _, sl := range m.planSlots[p.ID] {
				slotDay := sl.SlotDate.Truncate(24 * time.Hour)
				dateDay := date.Truncate(24 * time.Hour)
				if slotDay.Equal(dateDay) && (mealType == "" || sl.MealType == mealType) {
					matched = append(matched, sl)
				}
			}
			return matched, p, nil
		}
	}
	return nil, nil, nil
}

func (m *mockPlanStore) FindOverlappingPlans(_ context.Context, _, _ time.Time, _ string) ([]Plan, error) {
	if m.findOverlapErr != nil {
		return nil, m.findOverlapErr
	}
	return m.overlappingPlans, nil
}

func (m *mockPlanStore) AutoCompletePastPlans(_ context.Context) (int, error) {
	return m.autoCompletedCount, m.autoCompleteErr
}

func (m *mockPlanStore) RecipeArtifactExists(_ context.Context, _ string) (bool, error) {
	return m.recipeArtifactExistsOK, m.recipeExistsErr
}

func (m *mockPlanStore) GetSlotByDateMeal(_ context.Context, planID string, date time.Time, mealType string) (*Slot, error) {
	for _, sl := range m.planSlots[planID] {
		slotDay := sl.SlotDate.Truncate(24 * time.Hour)
		dateDay := date.Truncate(24 * time.Hour)
		if slotDay.Equal(dateDay) && sl.MealType == mealType {
			return &sl, nil
		}
	}
	return nil, nil
}

func (m *mockPlanStore) MarkPlanUpdated(_ context.Context, _ string) error {
	return nil
}

// --- helper to build a test service ---

func newTestService(store PlanDataStore) *Service {
	return &Service{
		Store:            store,
		MealTypes:        []string{"breakfast", "lunch", "dinner", "snack"},
		DefaultServings:  4,
		CalendarSync:     false,
		AutoComplete:     false,
		AutoCompleteCron: "",
	}
}

// --- T-02-01: SCN-036-006 — Create plan with date range ---

func TestCreatePlan_ValidDateRange(t *testing.T) {
	ms := newMockPlanStore()
	svc := newTestService(ms)
	ctx := context.Background()

	start := time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 4, 26, 0, 0, 0, 0, time.UTC)

	result, err := svc.CreatePlan(ctx, "Week of Apr 20", start, end)
	if err != nil {
		t.Fatalf("CreatePlan: unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("CreatePlan: expected non-nil result")
	}
	if result.Plan.Title != "Week of Apr 20" {
		t.Errorf("title = %q, want %q", result.Plan.Title, "Week of Apr 20")
	}
	if result.Plan.Status != StatusDraft {
		t.Errorf("status = %q, want %q", result.Plan.Status, StatusDraft)
	}
	if result.Plan.StartDate.Truncate(24*time.Hour) != start {
		t.Errorf("start_date = %v, want %v", result.Plan.StartDate, start)
	}
	if result.Plan.EndDate.Truncate(24*time.Hour) != end {
		t.Errorf("end_date = %v, want %v", result.Plan.EndDate, end)
	}
	if len(result.Slots) != 0 {
		t.Errorf("slots = %d, want 0", len(result.Slots))
	}
	// Verify plan was persisted in mock store
	if len(ms.plans) != 1 {
		t.Errorf("store has %d plans, want 1", len(ms.plans))
	}
}

func TestCreatePlan_EmptyTitle(t *testing.T) {
	svc := newTestService(newMockPlanStore())
	ctx := context.Background()

	start := time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 4, 26, 0, 0, 0, 0, time.UTC)

	_, err := svc.CreatePlan(ctx, "", start, end)
	if err == nil {
		t.Fatal("expected error for empty title")
	}
	if err.Error() != "title is required" {
		t.Errorf("error = %q, want %q", err.Error(), "title is required")
	}
}

func TestCreatePlan_TitleTooLong(t *testing.T) {
	svc := newTestService(newMockPlanStore())
	ctx := context.Background()

	start := time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 4, 26, 0, 0, 0, 0, time.UTC)
	longTitle := string(make([]byte, 201))
	for i := range longTitle {
		_ = i
	}

	_, err := svc.CreatePlan(ctx, longTitle, start, end)
	if err == nil {
		t.Fatal("expected error for title > 200 chars")
	}
}

func TestCreatePlan_StoreError(t *testing.T) {
	ms := newMockPlanStore()
	ms.createPlanErr = fmt.Errorf("db connection refused")
	svc := newTestService(ms)
	ctx := context.Background()

	start := time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 4, 26, 0, 0, 0, 0, time.UTC)

	_, err := svc.CreatePlan(ctx, "Test Plan", start, end)
	if err == nil {
		t.Fatal("expected error when store fails")
	}
}

// --- T-02-02: SCN-036-007 — Date validation rejects end < start ---

func TestCreatePlan_EndBeforeStart(t *testing.T) {
	svc := newTestService(newMockPlanStore())
	ctx := context.Background()

	start := time.Date(2026, 4, 26, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC)

	_, err := svc.CreatePlan(ctx, "Invalid Dates", start, end)
	if err == nil {
		t.Fatal("expected error when end_date < start_date")
	}
	if err.Error() != "end_date must be on or after start_date" {
		t.Errorf("error = %q, want %q", err.Error(), "end_date must be on or after start_date")
	}
}

func TestCreatePlan_SameStartEnd(t *testing.T) {
	ms := newMockPlanStore()
	svc := newTestService(ms)
	ctx := context.Background()

	date := time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC)

	result, err := svc.CreatePlan(ctx, "Single Day Plan", date, date)
	if err != nil {
		t.Fatalf("CreatePlan same-day: unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for same-day plan")
	}
}

// --- T-02-03: SCN-036-008 — Assign recipe to slot ---

func TestAddSlot_ValidAssignment(t *testing.T) {
	ms := newMockPlanStore()
	svc := newTestService(ms)
	ctx := context.Background()

	// Seed a draft plan in the mock store
	plan := &Plan{
		ID:        "mp-test-1",
		Title:     "Test Plan",
		StartDate: time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2026, 4, 26, 0, 0, 0, 0, time.UTC),
		Status:    StatusDraft,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	ms.plans[plan.ID] = plan

	slotDate := time.Date(2026, 4, 22, 0, 0, 0, 0, time.UTC)
	slot, err := svc.AddSlot(ctx, plan.ID, slotDate, "dinner", "recipe-pasta-1", 4, false, "")
	if err != nil {
		t.Fatalf("AddSlot: unexpected error: %v", err)
	}
	if slot == nil {
		t.Fatal("AddSlot: expected non-nil slot")
	}
	if slot.MealType != "dinner" {
		t.Errorf("meal_type = %q, want %q", slot.MealType, "dinner")
	}
	if slot.RecipeArtifactID != "recipe-pasta-1" {
		t.Errorf("recipe_artifact_id = %q, want %q", slot.RecipeArtifactID, "recipe-pasta-1")
	}
	if slot.Servings != 4 {
		t.Errorf("servings = %d, want 4", slot.Servings)
	}
	if slot.PlanID != plan.ID {
		t.Errorf("plan_id = %q, want %q", slot.PlanID, plan.ID)
	}
	// Verify slot was persisted in mock store
	if len(ms.slots) != 1 {
		t.Errorf("store has %d slots, want 1", len(ms.slots))
	}
}

func TestAddSlot_DefaultServings(t *testing.T) {
	ms := newMockPlanStore()
	svc := newTestService(ms)
	ctx := context.Background()

	plan := &Plan{
		ID:        "mp-test-1",
		Title:     "Test Plan",
		StartDate: time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2026, 4, 26, 0, 0, 0, 0, time.UTC),
		Status:    StatusDraft,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	ms.plans[plan.ID] = plan

	slotDate := time.Date(2026, 4, 22, 0, 0, 0, 0, time.UTC)
	slot, err := svc.AddSlot(ctx, plan.ID, slotDate, "lunch", "recipe-salad-1", 0, false, "")
	if err != nil {
		t.Fatalf("AddSlot: unexpected error: %v", err)
	}
	if slot.Servings != 4 {
		t.Errorf("default servings = %d, want 4", slot.Servings)
	}
}

func TestAddSlot_InvalidMealType(t *testing.T) {
	ms := newMockPlanStore()
	svc := newTestService(ms)
	ctx := context.Background()

	plan := &Plan{
		ID:        "mp-test-1",
		Title:     "Test Plan",
		StartDate: time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2026, 4, 26, 0, 0, 0, 0, time.UTC),
		Status:    StatusDraft,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	ms.plans[plan.ID] = plan

	slotDate := time.Date(2026, 4, 22, 0, 0, 0, 0, time.UTC)
	_, err := svc.AddSlot(ctx, plan.ID, slotDate, "brunch", "recipe-1", 2, false, "")
	if err == nil {
		t.Fatal("expected error for invalid meal type")
	}
	svcErr, ok := err.(*ServiceError)
	if !ok {
		t.Fatalf("expected ServiceError, got %T", err)
	}
	if svcErr.Code != "MEAL_PLAN_INVALID_MEAL_TYPE" {
		t.Errorf("error code = %q, want %q", svcErr.Code, "MEAL_PLAN_INVALID_MEAL_TYPE")
	}
}

func TestAddSlot_DateOutOfRange(t *testing.T) {
	ms := newMockPlanStore()
	svc := newTestService(ms)
	ctx := context.Background()

	plan := &Plan{
		ID:        "mp-test-1",
		Title:     "Test Plan",
		StartDate: time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2026, 4, 26, 0, 0, 0, 0, time.UTC),
		Status:    StatusDraft,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	ms.plans[plan.ID] = plan

	// Slot date before plan start
	beforeStart := time.Date(2026, 4, 19, 0, 0, 0, 0, time.UTC)
	_, err := svc.AddSlot(ctx, plan.ID, beforeStart, "dinner", "recipe-1", 4, false, "")
	if err == nil {
		t.Fatal("expected error for slot date before plan start")
	}
	svcErr, ok := err.(*ServiceError)
	if !ok {
		t.Fatalf("expected ServiceError, got %T", err)
	}
	if svcErr.Code != "MEAL_PLAN_SLOT_OUT_OF_RANGE" {
		t.Errorf("error code = %q, want %q", svcErr.Code, "MEAL_PLAN_SLOT_OUT_OF_RANGE")
	}

	// Slot date after plan end
	afterEnd := time.Date(2026, 4, 27, 0, 0, 0, 0, time.UTC)
	_, err = svc.AddSlot(ctx, plan.ID, afterEnd, "dinner", "recipe-1", 4, false, "")
	if err == nil {
		t.Fatal("expected error for slot date after plan end")
	}
}

func TestAddSlot_NotesTooLong(t *testing.T) {
	svc := newTestService(newMockPlanStore())
	ctx := context.Background()

	longNotes := string(make([]byte, 501))
	_, err := svc.AddSlot(ctx, "mp-1", time.Now(), "dinner", "recipe-1", 4, false, longNotes)
	if err == nil {
		t.Fatal("expected error for notes exceeding max length")
	}
	svcErr, ok := err.(*ServiceError)
	if !ok {
		t.Fatalf("expected ServiceError, got %T", err)
	}
	if svcErr.Code != "MEAL_PLAN_VALIDATION" {
		t.Errorf("error code = %q, want %q", svcErr.Code, "MEAL_PLAN_VALIDATION")
	}
}

func TestAddSlot_ServingsOverMax(t *testing.T) {
	svc := newTestService(newMockPlanStore())
	ctx := context.Background()

	_, err := svc.AddSlot(ctx, "mp-1", time.Now(), "dinner", "recipe-1", 1001, false, "")
	if err == nil {
		t.Fatal("expected error for servings > 1000")
	}
	svcErr, ok := err.(*ServiceError)
	if !ok {
		t.Fatalf("expected ServiceError, got %T", err)
	}
	if svcErr.Code != "MEAL_PLAN_VALIDATION" {
		t.Errorf("error code = %q, want %q", svcErr.Code, "MEAL_PLAN_VALIDATION")
	}
}

func TestAddSlot_PlanNotFound(t *testing.T) {
	ms := newMockPlanStore()
	svc := newTestService(ms)
	ctx := context.Background()

	_, err := svc.AddSlot(ctx, "nonexistent", time.Now(), "dinner", "recipe-1", 4, false, "")
	if err == nil {
		t.Fatal("expected error for nonexistent plan")
	}
	svcErr, ok := err.(*ServiceError)
	if !ok {
		t.Fatalf("expected ServiceError, got %T", err)
	}
	if svcErr.Code != "MEAL_PLAN_NOT_FOUND" {
		t.Errorf("error code = %q, want %q", svcErr.Code, "MEAL_PLAN_NOT_FOUND")
	}
}

func TestAddSlot_RecipeNotFound(t *testing.T) {
	ms := newMockPlanStore()
	ms.recipeArtifactExistsOK = false
	svc := newTestService(ms)
	ctx := context.Background()

	plan := &Plan{
		ID:        "mp-test-1",
		Title:     "Test Plan",
		StartDate: time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2026, 4, 26, 0, 0, 0, 0, time.UTC),
		Status:    StatusDraft,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	ms.plans[plan.ID] = plan

	slotDate := time.Date(2026, 4, 22, 0, 0, 0, 0, time.UTC)
	_, err := svc.AddSlot(ctx, plan.ID, slotDate, "dinner", "missing-recipe", 4, false, "")
	if err == nil {
		t.Fatal("expected error for missing recipe artifact")
	}
	svcErr, ok := err.(*ServiceError)
	if !ok {
		t.Fatalf("expected ServiceError, got %T", err)
	}
	if svcErr.Code != "MEAL_PLAN_RECIPE_NOT_FOUND" {
		t.Errorf("error code = %q, want %q", svcErr.Code, "MEAL_PLAN_RECIPE_NOT_FOUND")
	}
}

// --- T-02-04: SCN-036-009 — Unique slot constraint returns 409 ---

func TestAddSlot_ConflictReturns409(t *testing.T) {
	ms := newMockPlanStore()
	svc := newTestService(ms)
	ctx := context.Background()

	plan := &Plan{
		ID:        "mp-test-1",
		Title:     "Test Plan",
		StartDate: time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2026, 4, 26, 0, 0, 0, 0, time.UTC),
		Status:    StatusDraft,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	ms.plans[plan.ID] = plan

	slotDate := time.Date(2026, 4, 22, 0, 0, 0, 0, time.UTC)

	// First slot succeeds
	_, err := svc.AddSlot(ctx, plan.ID, slotDate, "dinner", "recipe-1", 4, false, "")
	if err != nil {
		t.Fatalf("first AddSlot: unexpected error: %v", err)
	}

	// Second slot for same date+meal_type should conflict
	_, err = svc.AddSlot(ctx, plan.ID, slotDate, "dinner", "recipe-2", 2, false, "")
	if err == nil {
		t.Fatal("expected 409 conflict error for duplicate date+meal_type")
	}
	svcErr, ok := err.(*ServiceError)
	if !ok {
		t.Fatalf("expected ServiceError, got %T", err)
	}
	if svcErr.Code != "MEAL_PLAN_SLOT_CONFLICT" {
		t.Errorf("error code = %q, want %q", svcErr.Code, "MEAL_PLAN_SLOT_CONFLICT")
	}
	if svcErr.Status != 409 {
		t.Errorf("status = %d, want 409", svcErr.Status)
	}
}

func TestAddSlot_DifferentMealTypeSameDate(t *testing.T) {
	ms := newMockPlanStore()
	svc := newTestService(ms)
	ctx := context.Background()

	plan := &Plan{
		ID:        "mp-test-1",
		Title:     "Test Plan",
		StartDate: time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2026, 4, 26, 0, 0, 0, 0, time.UTC),
		Status:    StatusDraft,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	ms.plans[plan.ID] = plan

	slotDate := time.Date(2026, 4, 22, 0, 0, 0, 0, time.UTC)

	// Dinner slot
	_, err := svc.AddSlot(ctx, plan.ID, slotDate, "dinner", "recipe-1", 4, false, "")
	if err != nil {
		t.Fatalf("dinner AddSlot: unexpected error: %v", err)
	}

	// Lunch slot on same date should succeed
	_, err = svc.AddSlot(ctx, plan.ID, slotDate, "lunch", "recipe-2", 2, false, "")
	if err != nil {
		t.Fatalf("lunch AddSlot: unexpected error: %v", err)
	}

	if len(ms.slots) != 2 {
		t.Errorf("store has %d slots, want 2", len(ms.slots))
	}
}

// --- Status transition tests (complements service_test.go) ---

func TestActivatePlan_ValidTransition(t *testing.T) {
	ms := newMockPlanStore()
	svc := newTestService(ms)
	ctx := context.Background()

	plan := &Plan{
		ID:        "mp-test-1",
		Title:     "Test Plan",
		StartDate: time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2026, 4, 26, 0, 0, 0, 0, time.UTC),
		Status:    StatusDraft,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	ms.plans[plan.ID] = plan

	overlap, err := svc.ActivatePlan(ctx, plan.ID, false)
	if err != nil {
		t.Fatalf("ActivatePlan: unexpected error: %v", err)
	}
	if overlap != nil {
		t.Errorf("expected no overlap, got %+v", overlap)
	}
	if ms.plans[plan.ID].Status != StatusActive {
		t.Errorf("status = %q, want %q", ms.plans[plan.ID].Status, StatusActive)
	}
}

func TestActivatePlan_ForbiddenFromCompleted(t *testing.T) {
	ms := newMockPlanStore()
	svc := newTestService(ms)
	ctx := context.Background()

	plan := &Plan{
		ID:        "mp-test-1",
		Title:     "Test Plan",
		StartDate: time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2026, 4, 26, 0, 0, 0, 0, time.UTC),
		Status:    StatusCompleted,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	ms.plans[plan.ID] = plan

	_, err := svc.ActivatePlan(ctx, plan.ID, false)
	if err == nil {
		t.Fatal("expected error activating completed plan")
	}
	svcErr, ok := err.(*ServiceError)
	if !ok {
		t.Fatalf("expected ServiceError, got %T", err)
	}
	if svcErr.Code != "MEAL_PLAN_INVALID_TRANSITION" {
		t.Errorf("error code = %q, want %q", svcErr.Code, "MEAL_PLAN_INVALID_TRANSITION")
	}
}

func TestTransitionPlan_AllValidPaths(t *testing.T) {
	tests := []struct {
		name   string
		from   PlanStatus
		to     PlanStatus
		wantOK bool
	}{
		{"draft→active", StatusDraft, StatusActive, true},
		{"active→completed", StatusActive, StatusCompleted, true},
		{"active→archived", StatusActive, StatusArchived, true},
		{"completed→archived", StatusCompleted, StatusArchived, true},
		{"draft→completed forbidden", StatusDraft, StatusCompleted, false},
		{"completed→active forbidden", StatusCompleted, StatusActive, false},
		{"archived→anything forbidden", StatusArchived, StatusActive, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ms := newMockPlanStore()
			svc := newTestService(ms)
			ctx := context.Background()

			plan := &Plan{
				ID:        "mp-test-1",
				Title:     "Test Plan",
				StartDate: time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC),
				EndDate:   time.Date(2026, 4, 26, 0, 0, 0, 0, time.UTC),
				Status:    tt.from,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}
			ms.plans[plan.ID] = plan

			err := svc.TransitionPlan(ctx, plan.ID, tt.to)
			if tt.wantOK && err != nil {
				t.Fatalf("expected success, got error: %v", err)
			}
			if !tt.wantOK && err == nil {
				t.Fatal("expected error for forbidden transition")
			}
			if !tt.wantOK {
				svcErr, ok := err.(*ServiceError)
				if !ok {
					t.Fatalf("expected ServiceError, got %T", err)
				}
				if svcErr.Code != "MEAL_PLAN_INVALID_TRANSITION" {
					t.Errorf("error code = %q, want %q", svcErr.Code, "MEAL_PLAN_INVALID_TRANSITION")
				}
			}
		})
	}
}

// --- Overlap detection ---

func TestActivatePlan_OverlapDetected(t *testing.T) {
	ms := newMockPlanStore()
	ms.overlappingPlans = []Plan{
		{
			ID:        "mp-conflict",
			Title:     "Conflicting Plan",
			StartDate: time.Date(2026, 4, 22, 0, 0, 0, 0, time.UTC),
			EndDate:   time.Date(2026, 4, 28, 0, 0, 0, 0, time.UTC),
			Status:    StatusActive,
		},
	}
	svc := newTestService(ms)
	ctx := context.Background()

	plan := &Plan{
		ID:        "mp-test-1",
		Title:     "Test Plan",
		StartDate: time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2026, 4, 26, 0, 0, 0, 0, time.UTC),
		Status:    StatusDraft,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	ms.plans[plan.ID] = plan

	overlap, err := svc.ActivatePlan(ctx, plan.ID, false)
	if err == nil {
		t.Fatal("expected overlap error")
	}
	svcErr, ok := err.(*ServiceError)
	if !ok {
		t.Fatalf("expected ServiceError, got %T", err)
	}
	if svcErr.Code != "MEAL_PLAN_OVERLAP" {
		t.Errorf("error code = %q, want %q", svcErr.Code, "MEAL_PLAN_OVERLAP")
	}
	if svcErr.Status != 409 {
		t.Errorf("status = %d, want 409", svcErr.Status)
	}
	if overlap == nil {
		t.Fatal("expected overlap info")
	}
	if overlap.ConflictingPlanID != "mp-conflict" {
		t.Errorf("conflicting_plan_id = %q, want %q", overlap.ConflictingPlanID, "mp-conflict")
	}
	if overlap.OverlappingDays <= 0 {
		t.Errorf("overlapping_days = %d, want > 0", overlap.OverlappingDays)
	}
}

func TestActivatePlan_ForceIgnoresOverlap(t *testing.T) {
	ms := newMockPlanStore()
	ms.overlappingPlans = []Plan{
		{
			ID:        "mp-conflict",
			Title:     "Conflicting Plan",
			StartDate: time.Date(2026, 4, 22, 0, 0, 0, 0, time.UTC),
			EndDate:   time.Date(2026, 4, 28, 0, 0, 0, 0, time.UTC),
			Status:    StatusActive,
		},
	}
	svc := newTestService(ms)
	ctx := context.Background()

	plan := &Plan{
		ID:        "mp-test-1",
		Title:     "Test Plan",
		StartDate: time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2026, 4, 26, 0, 0, 0, 0, time.UTC),
		Status:    StatusDraft,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	ms.plans[plan.ID] = plan

	overlap, err := svc.ActivatePlan(ctx, plan.ID, true)
	if err != nil {
		t.Fatalf("ActivatePlan with force: unexpected error: %v", err)
	}
	if overlap != nil {
		t.Errorf("expected no overlap info with force=true, got %+v", overlap)
	}
	if ms.plans[plan.ID].Status != StatusActive {
		t.Errorf("status = %q, want %q", ms.plans[plan.ID].Status, StatusActive)
	}
}

// --- DeletePlan cascade behavior ---

func TestDeletePlan_RemovesPlanAndSlots(t *testing.T) {
	ms := newMockPlanStore()
	svc := newTestService(ms)
	ctx := context.Background()

	plan := &Plan{
		ID:        "mp-test-1",
		Title:     "Test Plan",
		StartDate: time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2026, 4, 26, 0, 0, 0, 0, time.UTC),
		Status:    StatusDraft,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	ms.plans[plan.ID] = plan

	// Add some slots
	for i := 0; i < 5; i++ {
		slot := &Slot{
			ID:               fmt.Sprintf("mps-test-%d", i),
			PlanID:           plan.ID,
			SlotDate:         plan.StartDate.AddDate(0, 0, i),
			MealType:         "dinner",
			RecipeArtifactID: "recipe-1",
			Servings:         4,
			CreatedAt:        time.Now(),
		}
		ms.slots[slot.ID] = slot
		ms.planSlots[plan.ID] = append(ms.planSlots[plan.ID], *slot)
	}

	if len(ms.slots) != 5 {
		t.Fatalf("pre-delete: store has %d slots, want 5", len(ms.slots))
	}

	err := svc.DeletePlan(ctx, plan.ID)
	if err != nil {
		t.Fatalf("DeletePlan: unexpected error: %v", err)
	}

	// Plan should be gone
	if len(ms.plans) != 0 {
		t.Errorf("plans remaining = %d, want 0", len(ms.plans))
	}
	// Slots should be cascade-deleted
	if len(ms.slots) != 0 {
		t.Errorf("slots remaining = %d, want 0", len(ms.slots))
	}
}

func TestDeletePlan_NotFound(t *testing.T) {
	ms := newMockPlanStore()
	svc := newTestService(ms)
	ctx := context.Background()

	err := svc.DeletePlan(ctx, "nonexistent")
	if err == nil {
		t.Fatal("expected error for deleting nonexistent plan")
	}
}

// --- T-02-12: SCN-036-017 — Batch slot creation ---

func TestAddBatchSlots_CreatesOnePerDay(t *testing.T) {
	ms := newMockPlanStore()
	svc := newTestService(ms)
	ctx := context.Background()

	plan := &Plan{
		ID:        "mp-test-1",
		Title:     "Test Plan",
		StartDate: time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2026, 4, 26, 0, 0, 0, 0, time.UTC),
		Status:    StatusDraft,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	ms.plans[plan.ID] = plan

	batchStart := time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC)
	batchEnd := time.Date(2026, 4, 23, 0, 0, 0, 0, time.UTC)

	slots, err := svc.AddBatchSlots(ctx, plan.ID, batchStart, batchEnd, "breakfast", "recipe-oats-1", 2)
	if err != nil {
		t.Fatalf("AddBatchSlots: unexpected error: %v", err)
	}

	// Apr 20, 21, 22, 23 = 4 days
	if len(slots) != 4 {
		t.Fatalf("slots created = %d, want 4", len(slots))
	}

	for i, sl := range slots {
		expectedDate := batchStart.AddDate(0, 0, i).Truncate(24 * time.Hour)
		slotDate := sl.SlotDate.Truncate(24 * time.Hour)
		if !slotDate.Equal(expectedDate) {
			t.Errorf("slot[%d] date = %v, want %v", i, slotDate, expectedDate)
		}
		if sl.MealType != "breakfast" {
			t.Errorf("slot[%d] meal_type = %q, want %q", i, sl.MealType, "breakfast")
		}
		if sl.Servings != 2 {
			t.Errorf("slot[%d] servings = %d, want 2", i, sl.Servings)
		}
		if !sl.BatchFlag {
			t.Errorf("slot[%d] batch_flag = false, want true", i)
		}
	}
}

func TestAddBatchSlots_SingleDay(t *testing.T) {
	ms := newMockPlanStore()
	svc := newTestService(ms)
	ctx := context.Background()

	plan := &Plan{
		ID:        "mp-test-1",
		Title:     "Test Plan",
		StartDate: time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2026, 4, 26, 0, 0, 0, 0, time.UTC),
		Status:    StatusDraft,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	ms.plans[plan.ID] = plan

	date := time.Date(2026, 4, 22, 0, 0, 0, 0, time.UTC)
	slots, err := svc.AddBatchSlots(ctx, plan.ID, date, date, "lunch", "recipe-1", 3)
	if err != nil {
		t.Fatalf("AddBatchSlots single day: unexpected error: %v", err)
	}
	if len(slots) != 1 {
		t.Errorf("slots created = %d, want 1", len(slots))
	}
}

func TestAddBatchSlots_ConflictStopsOnError(t *testing.T) {
	ms := newMockPlanStore()
	svc := newTestService(ms)
	ctx := context.Background()

	plan := &Plan{
		ID:        "mp-test-1",
		Title:     "Test Plan",
		StartDate: time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2026, 4, 26, 0, 0, 0, 0, time.UTC),
		Status:    StatusDraft,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	ms.plans[plan.ID] = plan

	// Pre-add a slot for Apr 21 dinner to cause a conflict
	existingSlot := &Slot{
		ID:               "mps-existing",
		PlanID:           plan.ID,
		SlotDate:         time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC),
		MealType:         "dinner",
		RecipeArtifactID: "recipe-existing",
		Servings:         4,
		CreatedAt:        time.Now(),
	}
	ms.slots[existingSlot.ID] = existingSlot
	ms.planSlots[plan.ID] = append(ms.planSlots[plan.ID], *existingSlot)

	batchStart := time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC)
	batchEnd := time.Date(2026, 4, 23, 0, 0, 0, 0, time.UTC)

	// Batch should succeed for Apr 20, then fail on Apr 21 due to conflict
	slots, err := svc.AddBatchSlots(ctx, plan.ID, batchStart, batchEnd, "dinner", "recipe-batch", 2)
	if err == nil {
		t.Fatal("expected error when batch encounters conflict")
	}
	// Should have created 1 slot (Apr 20) before hitting the conflict
	if len(slots) != 1 {
		t.Errorf("slots before conflict = %d, want 1", len(slots))
	}
}

// --- UpdateSlot validation ---

func TestUpdateSlot_NotesTooLongViaService(t *testing.T) {
	ms := newMockPlanStore()
	svc := newTestService(ms)
	ctx := context.Background()

	longNotes := string(make([]byte, 501))
	_, err := svc.UpdateSlot(ctx, "mp-1", "slot-1", "recipe-1", 4, false, longNotes)
	if err == nil {
		t.Fatal("expected error for notes exceeding max length")
	}
	svcErr, ok := err.(*ServiceError)
	if !ok {
		t.Fatalf("expected ServiceError, got %T", err)
	}
	if svcErr.Status != 422 {
		t.Errorf("status = %d, want 422", svcErr.Status)
	}
}

func TestUpdateSlot_ServingsOverMaxViaService(t *testing.T) {
	ms := newMockPlanStore()
	svc := newTestService(ms)
	ctx := context.Background()

	_, err := svc.UpdateSlot(ctx, "mp-1", "slot-1", "recipe-1", 1001, false, "")
	if err == nil {
		t.Fatal("expected error for servings > 1000")
	}
}

func TestUpdateSlot_SlotNotFound(t *testing.T) {
	ms := newMockPlanStore()
	svc := newTestService(ms)
	ctx := context.Background()

	_, err := svc.UpdateSlot(ctx, "mp-1", "nonexistent", "recipe-1", 4, false, "")
	if err == nil {
		t.Fatal("expected error for nonexistent slot")
	}
	svcErr, ok := err.(*ServiceError)
	if !ok {
		t.Fatalf("expected ServiceError, got %T", err)
	}
	if svcErr.Code != "MEAL_PLAN_SLOT_NOT_FOUND" {
		t.Errorf("error code = %q, want %q", svcErr.Code, "MEAL_PLAN_SLOT_NOT_FOUND")
	}
}

// --- NewStore construction ---

func TestNewStore_NilPool(t *testing.T) {
	store := NewStore(nil)
	if store == nil {
		t.Fatal("NewStore should return non-nil even with nil pool")
	}
	if store.Pool != nil {
		t.Fatal("expected nil pool")
	}
}

// --- UpdatePlanTitle validation ---

func TestUpdatePlanTitle_EmptyTitle(t *testing.T) {
	svc := newTestService(newMockPlanStore())
	ctx := context.Background()

	err := svc.UpdatePlanTitle(ctx, "mp-1", "")
	if err == nil {
		t.Fatal("expected error for empty title")
	}
}

func TestUpdatePlanTitle_TitleTooLong(t *testing.T) {
	svc := newTestService(newMockPlanStore())
	ctx := context.Background()

	longTitle := string(make([]byte, 201))
	err := svc.UpdatePlanTitle(ctx, "mp-1", longTitle)
	if err == nil {
		t.Fatal("expected error for title > 200 chars")
	}
}

// --- AutoCompletePastPlans ---

func TestAutoCompletePastPlans(t *testing.T) {
	ms := newMockPlanStore()
	ms.autoCompletedCount = 3
	svc := newTestService(ms)
	ctx := context.Background()

	count, err := svc.AutoCompletePastPlans(ctx)
	if err != nil {
		t.Fatalf("AutoCompletePastPlans: unexpected error: %v", err)
	}
	if count != 3 {
		t.Errorf("auto-completed = %d, want 3", count)
	}
}

// --- CopyPlan ---

func TestCopyPlan_ShiftsSlotDates(t *testing.T) {
	ms := newMockPlanStore()
	svc := newTestService(ms)
	ctx := context.Background()

	// Source plan: Apr 20-26
	sourcePlan := &Plan{
		ID:        "mp-source",
		Title:     "Source Plan",
		StartDate: time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2026, 4, 26, 0, 0, 0, 0, time.UTC),
		Status:    StatusCompleted,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	ms.plans[sourcePlan.ID] = sourcePlan

	// Add a slot to source
	sourceSlot := Slot{
		ID:               "mps-source-1",
		PlanID:           sourcePlan.ID,
		SlotDate:         time.Date(2026, 4, 22, 0, 0, 0, 0, time.UTC),
		MealType:         "dinner",
		RecipeArtifactID: "recipe-1",
		Servings:         4,
		CreatedAt:        time.Now(),
	}
	ms.slots[sourceSlot.ID] = &sourceSlot
	ms.planSlots[sourcePlan.ID] = []Slot{sourceSlot}

	// Copy to start at Apr 27 (7 days later)
	newStart := time.Date(2026, 4, 27, 0, 0, 0, 0, time.UTC)
	result, err := svc.CopyPlan(ctx, sourcePlan.ID, newStart, "Copied Plan", nil)
	if err != nil {
		t.Fatalf("CopyPlan: unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Plan.Plan.Title != "Copied Plan" {
		t.Errorf("title = %q, want %q", result.Plan.Plan.Title, "Copied Plan")
	}
	if result.Plan.Plan.Status != StatusDraft {
		t.Errorf("status = %q, want %q", result.Plan.Plan.Status, StatusDraft)
	}
	if result.SlotsCopied != 1 {
		t.Errorf("slots_copied = %d, want 1", result.SlotsCopied)
	}

	// The slot should be shifted by 7 days: Apr 22 → Apr 29
	if len(result.Plan.Slots) != 1 {
		t.Fatalf("copied slots = %d, want 1", len(result.Plan.Slots))
	}
	expectedSlotDate := time.Date(2026, 4, 29, 0, 0, 0, 0, time.UTC)
	copiedDate := result.Plan.Slots[0].SlotDate.Truncate(24 * time.Hour)
	if !copiedDate.Equal(expectedSlotDate) {
		t.Errorf("copied slot date = %v, want %v", copiedDate, expectedSlotDate)
	}
}
