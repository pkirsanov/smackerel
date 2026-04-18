-- 018_meal_plans.sql
-- Meal planning tables for spec 036: meal plan calendar with date+meal slots.

CREATE TABLE meal_plans (
    id          TEXT PRIMARY KEY,
    title       TEXT NOT NULL,
    start_date  DATE NOT NULL,
    end_date    DATE NOT NULL,
    status      TEXT NOT NULL DEFAULT 'draft',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT meal_plans_dates_check CHECK (end_date >= start_date),
    CONSTRAINT meal_plans_status_check CHECK (status IN ('draft', 'active', 'completed', 'archived'))
);

CREATE INDEX idx_meal_plans_status ON meal_plans (status);
CREATE INDEX idx_meal_plans_dates ON meal_plans (start_date, end_date);

CREATE TABLE meal_plan_slots (
    id                  TEXT PRIMARY KEY,
    plan_id             TEXT NOT NULL REFERENCES meal_plans(id) ON DELETE CASCADE,
    slot_date           DATE NOT NULL,
    meal_type           TEXT NOT NULL,
    recipe_artifact_id  TEXT NOT NULL REFERENCES artifacts(id),
    servings            INT NOT NULL DEFAULT 2,
    batch_flag          BOOLEAN NOT NULL DEFAULT false,
    notes               TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT meal_plan_slots_servings_check CHECK (servings > 0),
    CONSTRAINT meal_plan_slots_unique UNIQUE (plan_id, slot_date, meal_type)
);

CREATE INDEX idx_meal_plan_slots_plan ON meal_plan_slots (plan_id);
CREATE INDEX idx_meal_plan_slots_date ON meal_plan_slots (slot_date);
CREATE INDEX idx_meal_plan_slots_recipe ON meal_plan_slots (recipe_artifact_id);
