package mealplan

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store handles PostgreSQL operations for meal plans and slots.
type Store struct {
	Pool *pgxpool.Pool
}

// NewStore creates a new meal plan store.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{Pool: pool}
}

// scanSlot scans a row into a Slot. The SQL must select columns in this order:
// s.id, s.plan_id, s.slot_date, s.meal_type, s.recipe_artifact_id,
// s.servings, s.batch_flag, s.notes, s.created_at, recipe_title.
type slotScanner interface {
	Scan(dest ...any) error
}

func scanSlot(row slotScanner) (Slot, error) {
	var sl Slot
	err := row.Scan(&sl.ID, &sl.PlanID, &sl.SlotDate, &sl.MealType,
		&sl.RecipeArtifactID, &sl.Servings, &sl.BatchFlag, &sl.Notes,
		&sl.CreatedAt, &sl.RecipeTitle)
	return sl, err
}

// CreatePlan inserts a new meal plan.
func (s *Store) CreatePlan(ctx context.Context, plan *Plan) error {
	_, err := s.Pool.Exec(ctx,
		`INSERT INTO meal_plans (id, title, start_date, end_date, status, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		plan.ID, plan.Title, plan.StartDate, plan.EndDate, plan.Status, plan.CreatedAt, plan.UpdatedAt,
	)
	return err
}

// GetPlan retrieves a plan by ID.
func (s *Store) GetPlan(ctx context.Context, planID string) (*Plan, error) {
	var p Plan
	err := s.Pool.QueryRow(ctx,
		`SELECT id, title, start_date, end_date, status, created_at, updated_at
		 FROM meal_plans WHERE id = $1`, planID,
	).Scan(&p.ID, &p.Title, &p.StartDate, &p.EndDate, &p.Status, &p.CreatedAt, &p.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return &p, err
}

// GetPlanWithSlots retrieves a plan and all its slots, with recipe titles resolved.
func (s *Store) GetPlanWithSlots(ctx context.Context, planID string) (*PlanWithSlots, error) {
	plan, err := s.GetPlan(ctx, planID)
	if err != nil || plan == nil {
		return nil, err
	}

	rows, err := s.Pool.Query(ctx,
		`SELECT s.id, s.plan_id, s.slot_date, s.meal_type, s.recipe_artifact_id,
		        s.servings, s.batch_flag, s.notes, s.created_at,
		        COALESCE(a.title, '(recipe unavailable)') AS recipe_title
		 FROM meal_plan_slots s
		 LEFT JOIN artifacts a ON a.id = s.recipe_artifact_id
		 WHERE s.plan_id = $1
		 ORDER BY s.slot_date, s.meal_type`, planID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var slots []Slot
	for rows.Next() {
		sl, err := scanSlot(rows)
		if err != nil {
			return nil, err
		}
		slots = append(slots, sl)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return &PlanWithSlots{Plan: *plan, Slots: slots}, nil
}

// ListPlans returns plans matching the given filters.
func (s *Store) ListPlans(ctx context.Context, statusFilter string, fromDate, toDate *time.Time) ([]Plan, error) {
	query := `SELECT id, title, start_date, end_date, status, created_at, updated_at FROM meal_plans WHERE 1=1`
	args := []any{}
	argIdx := 1

	if statusFilter != "" {
		query += fmt.Sprintf(" AND status = $%d", argIdx)
		args = append(args, statusFilter)
		argIdx++
	}
	if fromDate != nil {
		query += fmt.Sprintf(" AND end_date >= $%d", argIdx)
		args = append(args, *fromDate)
		argIdx++
	}
	if toDate != nil {
		query += fmt.Sprintf(" AND start_date <= $%d", argIdx)
		args = append(args, *toDate)
		argIdx++
	}
	query += " ORDER BY start_date DESC"

	rows, err := s.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var plans []Plan
	for rows.Next() {
		var p Plan
		if err := rows.Scan(&p.ID, &p.Title, &p.StartDate, &p.EndDate, &p.Status, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		plans = append(plans, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return plans, nil
}

// UpdatePlanStatus transitions a plan's status and refreshes updated_at.
func (s *Store) UpdatePlanStatus(ctx context.Context, planID string, status PlanStatus) error {
	tag, err := s.Pool.Exec(ctx,
		`UPDATE meal_plans SET status = $1, updated_at = NOW() WHERE id = $2`,
		status, planID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("plan not found: %s", planID)
	}
	return nil
}

// UpdatePlanTitle updates a plan's title.
func (s *Store) UpdatePlanTitle(ctx context.Context, planID, title string) error {
	tag, err := s.Pool.Exec(ctx,
		`UPDATE meal_plans SET title = $1, updated_at = NOW() WHERE id = $2`,
		title, planID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("plan not found: %s", planID)
	}
	return nil
}

// DeletePlan removes a plan (slots cascade via FK).
func (s *Store) DeletePlan(ctx context.Context, planID string) error {
	tag, err := s.Pool.Exec(ctx, `DELETE FROM meal_plans WHERE id = $1`, planID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("plan not found: %s", planID)
	}
	return nil
}

// AddSlot inserts a meal plan slot.
func (s *Store) AddSlot(ctx context.Context, slot *Slot) error {
	_, err := s.Pool.Exec(ctx,
		`INSERT INTO meal_plan_slots (id, plan_id, slot_date, meal_type, recipe_artifact_id, servings, batch_flag, notes, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		slot.ID, slot.PlanID, slot.SlotDate, slot.MealType, slot.RecipeArtifactID,
		slot.Servings, slot.BatchFlag, slot.Notes, slot.CreatedAt,
	)
	return err
}

// GetSlot retrieves a single slot by plan and slot ID.
func (s *Store) GetSlot(ctx context.Context, planID, slotID string) (*Slot, error) {
	sl, err := scanSlot(s.Pool.QueryRow(ctx,
		`SELECT s.id, s.plan_id, s.slot_date, s.meal_type, s.recipe_artifact_id,
		        s.servings, s.batch_flag, s.notes, s.created_at,
		        COALESCE(a.title, '(recipe unavailable)') AS recipe_title
		 FROM meal_plan_slots s
		 LEFT JOIN artifacts a ON a.id = s.recipe_artifact_id
		 WHERE s.plan_id = $1 AND s.id = $2`, planID, slotID,
	))
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return &sl, err
}

// UpdateSlot updates mutable fields of a slot.
func (s *Store) UpdateSlot(ctx context.Context, slot *Slot) error {
	tag, err := s.Pool.Exec(ctx,
		`UPDATE meal_plan_slots
		 SET recipe_artifact_id = $1, servings = $2, batch_flag = $3, notes = $4
		 WHERE id = $5 AND plan_id = $6`,
		slot.RecipeArtifactID, slot.Servings, slot.BatchFlag, slot.Notes, slot.ID, slot.PlanID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("slot not found: %s in plan %s", slot.ID, slot.PlanID)
	}
	return nil
}

// DeleteSlot removes a single slot.
func (s *Store) DeleteSlot(ctx context.Context, planID, slotID string) error {
	tag, err := s.Pool.Exec(ctx,
		`DELETE FROM meal_plan_slots WHERE id = $1 AND plan_id = $2`,
		slotID, planID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("slot not found: %s in plan %s", slotID, planID)
	}
	return nil
}

// GetSlotsByDate returns slots matching a date and optional meal type across active plans.
func (s *Store) GetSlotsByDate(ctx context.Context, date time.Time, mealType string) ([]Slot, *Plan, error) {
	// Find the active plan covering this date
	var plan Plan
	err := s.Pool.QueryRow(ctx,
		`SELECT id, title, start_date, end_date, status, created_at, updated_at
		 FROM meal_plans
		 WHERE status = 'active' AND start_date <= $1 AND end_date >= $1
		 ORDER BY created_at DESC LIMIT 1`, date,
	).Scan(&plan.ID, &plan.Title, &plan.StartDate, &plan.EndDate, &plan.Status, &plan.CreatedAt, &plan.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}

	query := `SELECT s.id, s.plan_id, s.slot_date, s.meal_type, s.recipe_artifact_id,
	                 s.servings, s.batch_flag, s.notes, s.created_at,
	                 COALESCE(a.title, '(recipe unavailable)') AS recipe_title
	          FROM meal_plan_slots s
	          LEFT JOIN artifacts a ON a.id = s.recipe_artifact_id
	          WHERE s.plan_id = $1 AND s.slot_date = $2`
	args := []any{plan.ID, date}
	if mealType != "" {
		query += " AND s.meal_type = $3"
		args = append(args, mealType)
	}
	query += " ORDER BY s.meal_type"

	rows, err := s.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	var slots []Slot
	for rows.Next() {
		sl, err := scanSlot(rows)
		if err != nil {
			return nil, nil, err
		}
		slots = append(slots, sl)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return slots, &plan, nil
}

// FindOverlappingPlans returns active plans whose date ranges overlap the given range.
func (s *Store) FindOverlappingPlans(ctx context.Context, startDate, endDate time.Time, excludePlanID string) ([]Plan, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT id, title, start_date, end_date, status, created_at, updated_at
		 FROM meal_plans
		 WHERE status = 'active'
		   AND start_date <= $1
		   AND end_date >= $2
		   AND id != $3`,
		endDate, startDate, excludePlanID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var plans []Plan
	for rows.Next() {
		var p Plan
		if err := rows.Scan(&p.ID, &p.Title, &p.StartDate, &p.EndDate, &p.Status, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		plans = append(plans, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return plans, nil
}

// AutoCompletePastPlans transitions active plans with end_date < today to completed.
// Returns the number of plans transitioned.
func (s *Store) AutoCompletePastPlans(ctx context.Context) (int, error) {
	tag, err := s.Pool.Exec(ctx,
		`UPDATE meal_plans SET status = 'completed', updated_at = NOW()
		 WHERE status = 'active' AND end_date < CURRENT_DATE`,
	)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

// RecipeArtifactExists checks if a recipe artifact exists by ID.
func (s *Store) RecipeArtifactExists(ctx context.Context, artifactID string) (bool, error) {
	var exists bool
	err := s.Pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM artifacts WHERE id = $1)`, artifactID,
	).Scan(&exists)
	return exists, err
}

// GetSlotByDateMeal returns a slot for a specific plan+date+meal combination.
func (s *Store) GetSlotByDateMeal(ctx context.Context, planID string, date time.Time, mealType string) (*Slot, error) {
	sl, err := scanSlot(s.Pool.QueryRow(ctx,
		`SELECT s.id, s.plan_id, s.slot_date, s.meal_type, s.recipe_artifact_id,
		        s.servings, s.batch_flag, s.notes, s.created_at,
		        COALESCE(a.title, '(recipe unavailable)') AS recipe_title
		 FROM meal_plan_slots s
		 LEFT JOIN artifacts a ON a.id = s.recipe_artifact_id
		 WHERE s.plan_id = $1 AND s.slot_date = $2 AND s.meal_type = $3`,
		planID, date, mealType,
	))
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return &sl, err
}

// MarkPlanUpdated touches the updated_at timestamp.
func (s *Store) MarkPlanUpdated(ctx context.Context, planID string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE meal_plans SET updated_at = NOW() WHERE id = $1`, planID)
	if err != nil {
		slog.Warn("failed to mark plan updated", "plan_id", planID, "error", err)
	}
	return err
}
