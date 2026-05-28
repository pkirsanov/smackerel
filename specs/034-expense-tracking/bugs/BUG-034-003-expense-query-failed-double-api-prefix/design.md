# Design: Root cause analysis and fix design

> Ownership note: this design.md is initial triage output from `bubbles.bug`. Per Bubbles G042 the routing-fix design must be ratified by `bubbles.design` before `bubbles.implement` is invoked.

## Root cause

The expense (spec 034) and meal-plan (spec 036) HTTP handlers register their sub-routes with an ABSOLUTE prefix `/api/...` while being mounted INSIDE an outer `r.Route("/api", ...)` group in `internal/api/router.go`. Every other handler in the file uses RELATIVE prefixes (e.g. `r.Route("/knowledge", ...)`, `r.Route("/lists", ...)`) or relative leaf routes (`r.Get("/digest", ...)`), so they compose correctly under the outer `/api` mount.

### Code locations

`internal/api/router.go:59` — outer mount:

```go
r.Route("/api", func(r chi.Router) {
    r.Use(middleware.Throttle(100))
    r.Get("/health", deps.HealthHandler)                         // → /api/health  ✓
    r.Group(func(r chi.Router) {
        r.Use(deps.bearerAuthMiddleware)
        r.Post("/capture", deps.CaptureHandler)                  // → /api/capture ✓
        r.Route("/knowledge", func(r chi.Router) { ... })        // → /api/knowledge/* ✓
        r.Route("/lists", func(r chi.Router) { ... })            // → /api/lists/* ✓
        if deps.ExpenseHandler != nil {
            deps.ExpenseHandler.RegisterRoutes(r)                // BROKEN — see below
        }
        if deps.MealPlanHandler != nil {
            deps.MealPlanHandler.RegisterRoutes(r)               // BROKEN — same class
        }
    })
})
```

`internal/api/expenses.go:37-46` — broken registration:

```go
func (h *ExpenseHandler) RegisterRoutes(r chi.Router) {
    r.Route("/api/expenses", func(r chi.Router) {   // ← absolute prefix, double /api
        r.Get("/", h.List)
        r.Get("/export", h.Export)
        r.Get("/{id}", h.Get)
        r.Patch("/{id}", h.Correct)
        r.Post("/{id}/classify", h.ClassifyEndpoint)
        r.Post("/suggestions/{id}/accept", h.AcceptSuggestion)
        r.Post("/suggestions/{id}/dismiss", h.DismissSuggestion)
    })
}
```

`internal/api/mealplan.go:28` — same defect:

```go
r.Route("/api/meal-plans", func(r chi.Router) { ... })
```

### Why even `/api/api/expenses` returns 404

When chi composes a `Route("/api/expenses")` registration inside a parent that has already mounted at `/api` AND wrapped a `Group` with middleware, the resulting nested path is not reachable at `/api/api/expenses` either (live probe confirmed both `/api/expenses` and `/api/api/expenses` return 404, not 401). The exact chi internals are not relevant to the fix: the policy is "use relative prefixes inside group routes", which every working handler in the file already follows.

### Why CI didn't catch it

`internal/api/expenses_test.go` mounts the handler against a fresh bare `chi.NewRouter()` (no outer `/api` group), so the absolute prefix resolves to `/api/expenses` and tests pass. The integration coverage gap is documented in spec.md negative criteria.

## Proposed fix

Two-character change per handler (drop the `/api` prefix), plus an integration regression test that exercises `api.NewRouter(deps)` end-to-end.

```go
// internal/api/expenses.go
func (h *ExpenseHandler) RegisterRoutes(r chi.Router) {
    r.Route("/expenses", func(r chi.Router) {       // was: "/api/expenses"
        r.Get("/", h.List)
        ...
    })
}

// internal/api/mealplan.go
r.Route("/meal-plans", func(r chi.Router) { ... })  // was: "/api/meal-plans"
```

### Test updates

`internal/api/expenses_test.go` currently uses `httptest.NewRequest("GET", "/api/expenses?...")`. With the fix, when mounted against a bare router via `RegisterRoutes`, the path becomes `/expenses` not `/api/expenses`. Two options:

1. Update the test helper to mount the handler inside an `r.Route("/api", ...)` wrapper that mirrors the production composition, keeping the existing `/api/expenses` request paths. (Preferred — also exercises the production composition.)
2. Change all request paths in the test to `/expenses`. (Rejected — loses production-shape coverage.)

### New regression test (adversarial)

Add `TestRouter_ExpenseAndMealPlanRoutesMounted` in `internal/api/expenses_test.go` (or a new `internal/api/router_mount_test.go`) that:

1. Builds full `NewRouter(deps)` with non-nil `ExpenseHandler` and `MealPlanHandler`.
2. Asserts `GET /api/expenses` (no bearer) returns **401** — proving the route is mounted behind the auth gate.
3. Asserts `GET /api/meal-plans` (no bearer) returns **401**.
4. Adversarial: asserts `GET /api/api/expenses` returns **404** — proving we did NOT regress to the double-prefix shape.

The adversarial step ensures a future agent who "fixes" the bug by leaving `/api/expenses` in the handler and removing the outer mount, or vice versa, cannot pass the test.

## Affected files (fix scope)

- `internal/api/expenses.go` — prefix change (1 line)
- `internal/api/mealplan.go` — prefix change (1 line)
- `internal/api/expenses_test.go` — adjust mount helper to mirror production
- `internal/api/mealplan_test.go` (if exists) — same
- `internal/api/router_mount_test.go` (new) OR appended to existing test file — adversarial regression

## Deploy

Standard `./smackerel.sh build` + adapter `apply.sh` cycle. No DB migration, no config change, no secret rotation.

## Risk

Minimal. The change moves routes from "not reachable" to "reachable at the documented path". No behavior depends on `/api/expenses` returning 404. The Telegram bot is the only known caller; its URL constants already point at `/api/expenses` (correct post-fix path).
