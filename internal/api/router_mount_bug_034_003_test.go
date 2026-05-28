package api

// Regression coverage for BUG-034-003: expense + meal-plan API routes were
// registered with an absolute "/api/..." prefix inside the production router's
// outer r.Route("/api", ...) group, producing unreachable double-prefixed
// paths. Pre-existing unit tests in expenses_test.go and mealplan_test.go
// mount each handler against a bare chi.NewRouter() (or a hand-rolled router)
// and therefore never exercised production router composition. This file
// closes that gap by asserting the routes are reachable under the production
// router AND that the buggy double-prefix path stays 404 (adversarial).

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// newBug034003TestDeps returns a Dependencies bundle that mirrors the
// production router shape enough to exercise the /api group's bearer auth
// gate. Handler internals are intentionally zero-value: the regression test
// only needs the routes to be MOUNTED — the bearer-auth middleware fires
// before the handler body, so a nil DB pool never executes.
func newBug034003TestDeps() *Dependencies {
	return &Dependencies{
		DB:              &mockDB{healthy: true},
		NATS:            &mockNATS{healthy: true},
		StartTime:       time.Now(),
		AuthToken:       "bug-034-003-test-secret",
		ExpenseHandler:  &ExpenseHandler{},
		MealPlanHandler: &MealPlanHandler{},
	}
}

// TestBug034003_ExpenseAndMealPlanRoutesMountedBehindAuth proves the fix.
//
// Pre-fix, every assertion below failed with 404 (route never mounted because
// of the double-/api prefix). Post-fix, /api/expenses and /api/meal-plans hit
// the bearer-auth gate and return 401; the adversarial /api/api/expenses and
// /api/api/meal-plans stay 404 (proving we did not regress to the old shape).
func TestBug034003_ExpenseAndMealPlanRoutesMountedBehindAuth(t *testing.T) {
	router := NewRouter(newBug034003TestDeps())

	cases := []struct {
		name     string
		method   string
		path     string
		wantCode int
		why      string
	}{
		{
			name:     "expense_list_mounted_returns_401_without_bearer",
			method:   http.MethodGet,
			path:     "/api/expenses",
			wantCode: http.StatusUnauthorized,
			why:      "auth gate proves route mounted; pre-fix was 404",
		},
		{
			name:     "meal_plan_list_mounted_returns_401_without_bearer",
			method:   http.MethodGet,
			path:     "/api/meal-plans",
			wantCode: http.StatusUnauthorized,
			why:      "auth gate proves route mounted; pre-fix was 404",
		},
		{
			name:     "adversarial_double_prefix_expense_stays_404",
			method:   http.MethodGet,
			path:     "/api/api/expenses",
			wantCode: http.StatusNotFound,
			why:      "old buggy shape MUST stay unreachable",
		},
		{
			name:     "adversarial_double_prefix_meal_plan_stays_404",
			method:   http.MethodGet,
			path:     "/api/api/meal-plans",
			wantCode: http.StatusNotFound,
			why:      "old buggy shape MUST stay unreachable",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != tc.wantCode {
				t.Fatalf("%s %s status = %d, want %d (%s); body=%s",
					tc.method, tc.path, rec.Code, tc.wantCode, tc.why, rec.Body.String())
			}
		})
	}
}
