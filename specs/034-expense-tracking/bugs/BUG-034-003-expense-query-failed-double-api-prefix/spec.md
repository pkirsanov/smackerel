# Spec: Expected behavior for `/api/expenses` HTTP surface

## Acceptance criteria

1. `GET /api/expenses` (with valid bearer) returns HTTP 200 with JSON `{ok: true, data: {expenses: [...], total: "..."}}`.
2. `GET /api/expenses` without auth returns HTTP 401 (same as siblings `/api/digest`, `/api/lists`, `/api/knowledge/concepts`).
3. All sub-routes registered by `ExpenseHandler.RegisterRoutes` are reachable at exactly the paths their handler comments document (e.g. `GET /api/expenses/export`, `GET /api/expenses/{id}`, `PATCH /api/expenses/{id}`, `POST /api/expenses/{id}/classify`, `POST /api/expenses/suggestions/{id}/accept|dismiss`).
4. The same correctness applies to `MealPlanHandler.RegisterRoutes` (`/api/meal-plans/...`).
5. `/expense` Telegram command returns either an empty-state reply (e.g. `No expenses yet`) when the DB is empty, or a formatted list when non-empty — never `Failed to query expenses` for the no-data case.

## Negative / regression criteria

1. A router-level integration test MUST construct the full `api.NewRouter(deps)` and assert that `GET /api/expenses` returns 401 (not 404) with no bearer header. The existing unit tests in `internal/api/expenses_test.go` mount the handler against a bare `chi.NewRouter()` and therefore did NOT detect the production-routing defect.
2. The same router-level integration test MUST cover `/api/meal-plans` to catch the identical defect class.
3. Both tests MUST fail before the fix and pass after — adversarial regression requirement.

## Out of scope

- DB schema changes (the bug is purely routing).
- Telegram handler refactors (the bot code path is correct; it correctly reports the upstream 404).
- Auth / PASETO scope work (siblings prove bearer mint works).
