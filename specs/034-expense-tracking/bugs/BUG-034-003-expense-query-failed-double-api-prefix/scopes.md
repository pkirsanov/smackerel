# Scopes: BUG-034-003 Expense + meal-plan API routes unreachable (double-`/api` prefix)

> Ownership note: scope structure is initial triage skeleton from `bubbles.bug`. Per Bubbles G042 the planning structure must be ratified by `bubbles.plan` before implementation begins.

## Scope 1: Fix double-`/api` prefix in expense + meal-plan handlers

**Status:** [x] Done (in-process); live-host DoD items pending home-lab redeploy
**Depends On:** none

### Gherkin Scenarios (Regression Tests)

```gherkin
Feature: BUG-034-003 — Expense and meal-plan API routes are reachable behind bearer auth

  Scenario: Expense list route is mounted behind auth
    Given the full api.NewRouter(deps) is constructed with non-nil ExpenseHandler
    When GET /api/expenses is requested with no Authorization header
    Then the response status is 401 (auth gate), not 404 (route missing)

  Scenario: Meal-plan list route is mounted behind auth
    Given the full api.NewRouter(deps) is constructed with non-nil MealPlanHandler
    When GET /api/meal-plans is requested with no Authorization header
    Then the response status is 401, not 404

  Scenario: Adversarial — double-prefix path remains 404
    Given the full api.NewRouter(deps) is constructed
    When GET /api/api/expenses is requested
    Then the response status is 404 (we did not regress to double-prefix)

  Scenario: Telegram /expense command succeeds against empty DB
    Given the bot has a valid bearer-mint path
    And the expenses table is empty
    When the user sends /expense to the bot
    Then the reply is an empty-state message (e.g. "No expenses yet"), not "Failed to query expenses"
```

### Implementation Plan

1. Change `internal/api/expenses.go:38` from `r.Route("/api/expenses", ...)` to `r.Route("/expenses", ...)`.
2. Change `internal/api/mealplan.go:28` from `r.Route("/api/meal-plans", ...)` to `r.Route("/meal-plans", ...)`.
3. Update `internal/api/expenses_test.go` helper to mount the handler inside an `r.Route("/api", ...)` wrapper that mirrors production composition; keep existing `/api/expenses` request paths in the tests.
4. If `internal/api/mealplan_test.go` exists, apply the same wrapper update.
5. Add adversarial regression test (new file or appended) covering Scenarios 1–3 against full `NewRouter(deps)`.
6. Confirm the empty-state Telegram reply on home-lab after deploy (Scenario 4).

### Test Plan

| Test Type | Category | File/Location | Description | Command | Live System |
|-----------|----------|---------------|-------------|---------|-------------|
| Unit | `unit` | `internal/api/expenses_test.go` | Existing handler tests still pass with new mount wrapper | `./smackerel.sh test unit` | No |
| Unit | `unit` | `internal/api/mealplan_test.go` (if present) | Existing handler tests still pass | `./smackerel.sh test unit` | No |
| Integration | `integration` | `internal/api/router_mount_test.go` (new) | `NewRouter(deps)` mounts `/api/expenses` + `/api/meal-plans`; adversarial `/api/api/expenses` stays 404 | `./smackerel.sh test integration` | Yes |
| E2E API | `e2e-api` | Live home-lab probe captured post-deploy | `GET /api/expenses` → 401 without bearer, 200 with bearer; `/api/meal-plans` → 401 | `ssh <home-lab-host> docker exec ... wget` | Yes |
| E2E UI | `e2e-ui` | Telegram `/expense` command live | Bot reply is empty-state, not "Failed to query expenses" | Manual Telegram send + log scrape | Yes |

### Definition of Done — 3-Part Validation

- [x] `internal/api/expenses.go` prefix fixed and the change compiles — Evidence: [report.md#files-modified-this-session], `go build ./internal/api/...` exit 0
- [x] `internal/api/mealplan.go` prefix fixed and the change compiles — Evidence: [report.md#files-modified-this-session], `go build ./internal/api/...` exit 0
- [x] Existing unit tests pass (with mount-wrapper update) — Evidence: [report.md#existing-api-package-unit-suite-no-collateral-regression] (`ok github.com/smackerel/smackerel/internal/api 9.482s`)
- [x] New router-mount regression test FAILS on pre-fix code (proves it detects the bug) — Evidence: [report.md#bug-reproduction-after-fix-in-process-integration-regression] (RED block: all 4 sub-tests FAIL with status mismatch)
- [x] New router-mount regression test PASSES on post-fix code — Evidence: [report.md#bug-reproduction-after-fix-in-process-integration-regression] (GREEN block: all 4 sub-tests PASS)
- [x] Adversarial sub-test (`/api/api/expenses` → 404) is present and passing — Evidence: [report.md#bug-reproduction-after-fix-in-process-integration-regression] (`adversarial_double_prefix_expense_stays_404` PASS; `adversarial_double_prefix_meal_plan_stays_404` PASS)
- [ ] Live home-lab `GET /api/expenses` returns 401 without bearer (was 404) — **Blocked on home-lab redeploy.** Evidence: pending `bubbles.validate` post-deploy. See [report.md#live-deployment-required-not-yet-performed].
- [ ] Live home-lab `GET /api/meal-plans` returns 401 without bearer (was 404) — **Blocked on home-lab redeploy.** Evidence: pending `bubbles.validate` post-deploy.
- [ ] Live Telegram `/expense` returns empty-state, not "Failed to query expenses" — **Blocked on home-lab redeploy.** Evidence: pending `bubbles.validate` post-deploy.
- [ ] Full `./smackerel.sh test pre-push` passes — Not run in this session (no smackerel `test pre-push` command exists; equivalent surface is `check + lint + test unit + test integration`, all of which are green per the four evidence blocks above). Recommend running before merge; flagged for follow-up if a `pre-push` orchestrator is added to `smackerel.sh`.
- [x] bug.md status updated to "Fixed" then "Verified" — partial: code-fix portion marked Fixed via state.json; "Verified" pending live-host evidence post-deploy.

⚠️ E2E (live home-lab + Telegram) is MANDATORY — this is a production-deploy bug class.

### Honest Gap Summary

In-process implementation work is complete with full RED+GREEN evidence for the routing fix and adversarial coverage proving the buggy shape cannot return. Live-host verification + Telegram round-trip cannot be checked from this development session because they require a fresh image build + `apply.sh` invocation on the home-lab target, which is explicitly owner-gated per `knb` deploy adapter policy. Three live-host DoD items therefore remain `[ ]` and the bug status remains `in_progress` with `routing.nextOwner: bubbles.validate` (pending operator-run deploy).
