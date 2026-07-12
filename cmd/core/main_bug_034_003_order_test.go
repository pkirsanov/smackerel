package main

// BUG-034-003 follow-up adversarial regression: the d1d57f30 fix
// corrected the double-prefix issue in internal/api/expenses.go and
// internal/api/router.go, but a second latent bug existed in
// cmd/core/main.go: api.NewRouter(deps) was invoked BEFORE
// wireExpenseTracking(...) populated deps.ExpenseHandler, so the
// router-init guard `if deps.ExpenseHandler != nil` was false and the
// expense routes silently never registered. /api/expenses returned 404
// in production even though "expense tracking enabled" appeared in
// startup logs.
//
// This source-scan test pins the ordering: wireExpenseTracking MUST
// appear before api.NewRouter in main.go. If a future refactor reverts
// the order, this test fails loudly long before a release reaches
// self-hosted.

import (
	"os"
	"strings"
	"testing"
)

func TestBug034003_WireExpenseTrackingPrecedesNewRouter(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	body := string(src)

	wireIdx := strings.Index(body, "wireExpenseTracking(ctx, cfg, svc, deps)")
	if wireIdx < 0 {
		t.Fatal("wireExpenseTracking call not found in main.go — regression-test target moved; update the test")
	}
	routerIdx := strings.Index(body, "router := api.NewRouter(deps)")
	if routerIdx < 0 {
		t.Fatal("`router := api.NewRouter(deps)` call not found in main.go — regression-test target moved; update the test")
	}

	if wireIdx >= routerIdx {
		t.Fatalf("BUG-034-003 follow-up regression: wireExpenseTracking (offset %d) MUST precede api.NewRouter (offset %d). When NewRouter runs first, deps.ExpenseHandler is still nil and the /api/expenses routes are never registered — /api/expenses returns 404 in production even though startup logs claim 'expense tracking enabled'.", wireIdx, routerIdx)
	}

	// Adversarial sub-assertion: ensure no SECOND wireExpenseTracking
	// call still lingers in the post-router block (which would make
	// the test pass on the first occurrence while a regression sits
	// further down the file).
	if strings.Count(body, "wireExpenseTracking(ctx, cfg, svc, deps)") != 1 {
		t.Fatal("wireExpenseTracking should be called exactly once; multiple calls suggest an incomplete fix")
	}
}

// TestBug034004_WireMealPlanningHandlerPrecedesNewRouter pins the
// equivalent ordering for meal-plans (BUG-034-004 follow-up). Same
// class of bug as BUG-034-003: wireMealPlanning used to run AFTER
// api.NewRouter, so deps.MealPlanHandler was nil at router-init time
// and /api/meal-plans returned 404. The fix splits wireMealPlanning
// into wireMealPlanningHandler (early — no scheduler/tgBot dep) and
// wireMealPlanningSchedulerAndBot (late — needs sched + tgBot).
func TestBug034004_WireMealPlanningHandlerPrecedesNewRouter(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	body := string(src)

	handlerIdx := strings.Index(body, "wireMealPlanningHandler(cfg, svc, deps, listResolver, listStore)")
	if handlerIdx < 0 {
		t.Fatal("wireMealPlanningHandler call not found in main.go — regression-test target moved; update the test")
	}
	routerIdx := strings.Index(body, "router := api.NewRouter(deps)")
	if routerIdx < 0 {
		t.Fatal("`router := api.NewRouter(deps)` call not found in main.go — regression-test target moved; update the test")
	}

	if handlerIdx >= routerIdx {
		t.Fatalf("BUG-034-004 follow-up regression: wireMealPlanningHandler (offset %d) MUST precede api.NewRouter (offset %d). When NewRouter runs first, deps.MealPlanHandler is still nil and /api/meal-plans returns 404 in production.", handlerIdx, routerIdx)
	}

	// Adversarial: the unified wireMealPlanning(...) must NOT come back.
	if strings.Contains(body, "wireMealPlanning(cfg, svc, deps, sched, listResolver, listStore, tgBot)") {
		t.Fatal("BUG-034-004 regression: the old unified wireMealPlanning(...) signature is back; the fix has been reverted. The function MUST stay split into wireMealPlanningHandler (early) and wireMealPlanningSchedulerAndBot (late).")
	}

	// Adversarial: the late wiring must come AFTER NewRouter (it needs
	// sched + tgBot which are constructed later).
	lateIdx := strings.Index(body, "wireMealPlanningSchedulerAndBot(cfg, svc, sched, tgBot)")
	if lateIdx < 0 {
		t.Fatal("wireMealPlanningSchedulerAndBot call not found in main.go — the late-wiring half of the BUG-034-004 fix is missing")
	}
	if lateIdx <= routerIdx {
		t.Fatalf("wireMealPlanningSchedulerAndBot (offset %d) MUST come after api.NewRouter (offset %d) — it depends on sched + tgBot which are constructed later", lateIdx, routerIdx)
	}
}
