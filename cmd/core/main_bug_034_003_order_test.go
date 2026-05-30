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
// home-lab.

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
