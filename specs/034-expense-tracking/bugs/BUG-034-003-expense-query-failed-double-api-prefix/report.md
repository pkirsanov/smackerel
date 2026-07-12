# Report: BUG-034-003

## Summary

Triage filed by `bubbles.bug` on 2026-05-28. Root cause identified, fix routed to `bubbles.design` → `bubbles.plan` → `bubbles.implement`. No code changes performed in this triage session.

## Completion Statement

Pending implementation. Bug status: in_progress.

## Test Evidence

### Bug Reproduction — Before Fix (live self-hosted, 2026-05-28)

**Claim Source:** executed

Live self-hosted telegram-core slog WARN line captured immediately after the user-reported failure at 11:44 AM local (18:44 UTC):

```
{"time":"2026-05-28T18:44:24.26713234Z","level":"INFO","msg":"request","method":"GET","path":"/api/expenses","status":404,"duration_ms":0,"request_id":"9623ad891b1f/pbBNO1CDyv-003054"}
{"time":"2026-05-28T18:44:24.267231364Z","level":"WARN","msg":"expense query non-200","status":404}
```

Path-status probe inside the live container (`docker exec smackerel-self-hosted-smackerel-core-1 wget -S ...`):

```
/api/health             ->   HTTP/1.1 200 OK
/api/digest             ->   HTTP/1.1 401 Unauthorized
/api/expenses           ->   HTTP/1.1 404 Not Found      ← BUG
/api/expenses/          ->   HTTP/1.1 404 Not Found
/api/api/expenses       ->   HTTP/1.1 404 Not Found
/api/api/expenses/      ->   HTTP/1.1 404 Not Found
/api/meal-plans         ->   HTTP/1.1 404 Not Found      ← SAME BUG, spec 036
/api/api/meal-plans     ->   HTTP/1.1 404 Not Found
/api/knowledge/concepts ->   HTTP/1.1 401 Unauthorized   (sibling works)
/api/lists              ->   HTTP/1.1 401 Unauthorized   (sibling works)
```

Wiring confirmed non-nil at startup:

```
{"time":"2026-05-28T10:13:49.90964153Z","level":"INFO","msg":"expense tracking enabled","default_currency":"USD","export_max_rows":10000}
{"time":"2026-05-28T10:13:49.909664632Z","level":"INFO","msg":"meal planning enabled","meal_types":["breakfast","lunch","dinner","snack"],"default_servings":2,"calendar_sync":false}
```

Env confirmed:

```
EXPENSES_ENABLED=true
EXPENSES_DEFAULT_CURRENCY=USD
... (full expense env block present)
```

### Sibling-handler comparison (rules out auth / bearer / PASETO scope causes)

| Bot command | API path | Auth status (no bearer) | Behavior with bearer |
|---|---|---|---|
| `/concept` | `/api/knowledge/concepts` | 401 | 200 empty-state ✓ |
| `/person` | `/api/knowledge/entities` | 401 | 200 empty-state ✓ |
| `/expense` | `/api/expenses` | **404** | 404 (route does not exist) |

The bearer mint path works for siblings, so spec 044 PASETO scope claims are NOT the cause. The defect is in route registration.

### Root Cause Localized

`internal/api/expenses.go:38` and `internal/api/mealplan.go:28` use absolute prefix `r.Route("/api/expenses", ...)` / `r.Route("/api/meal-plans", ...)` while being registered INSIDE the outer `r.Route("/api", ...)` group in `internal/api/router.go:59`. Every other working handler uses relative prefixes (`r.Route("/knowledge", ...)`, `r.Route("/lists", ...)`) or relative leaf routes.

CI passed because `internal/api/expenses_test.go` mounts the handler against a bare `chi.NewRouter()`, never exercising the production composition. Coverage gap is integration-test-shaped.

### Bug Reproduction — After Fix (in-process integration regression)

**Phase:** implement  
**Claim Source:** executed (this session, 2026-05-28)

Pre-fix RED proof (captured by `git stash`-ing the fix, running the new regression test against the original buggy code, then `git stash pop`):

```
=== RUN   TestBug034003_ExpenseAndMealPlanRoutesMountedBehindAuth/expense_list_mounted_returns_401_without_bearer
2026/05/28 18:58:51 INFO request method=GET path=/api/expenses status=404 duration_ms=0
    router_mount_bug_034_003_test.go:87: GET /api/expenses status = 404, want 401 (auth gate proves route mounted; pre-fix was 404); body=404 page not found
=== RUN   TestBug034003_ExpenseAndMealPlanRoutesMountedBehindAuth/meal_plan_list_mounted_returns_401_without_bearer
2026/05/28 18:58:51 INFO request method=GET path=/api/meal-plans status=404 duration_ms=0
    router_mount_bug_034_003_test.go:87: GET /api/meal-plans status = 404, want 401 (auth gate proves route mounted; pre-fix was 404); body=404 page not found
=== RUN   TestBug034003_ExpenseAndMealPlanRoutesMountedBehindAuth/adversarial_double_prefix_expense_stays_404
2026/05/28 18:58:51 WARN bearer auth failure path=/api/api/expenses reason=missing_token
    router_mount_bug_034_003_test.go:87: GET /api/api/expenses status = 401, want 404 (old buggy shape MUST stay unreachable); body={"error":{"code":"UNAUTHORIZED","message":"Valid authentication required"}}
=== RUN   TestBug034003_ExpenseAndMealPlanRoutesMountedBehindAuth/adversarial_double_prefix_meal_plan_stays_404
2026/05/28 18:58:51 WARN bearer auth failure path=/api/api/meal-plans reason=missing_token
    router_mount_bug_034_003_test.go:87: GET /api/api/meal-plans status = 401, want 404 (old buggy shape MUST stay unreachable); body={"error":{"code":"UNAUTHORIZED","message":"Valid authentication required"}}
--- FAIL: TestBug034003_ExpenseAndMealPlanRoutesMountedBehindAuth (0.00s)
FAIL    github.com/smackerel/smackerel/internal/api     0.028s
```

Note: the pre-fix run revealed `/api/api/expenses` returns **401** in-process (not 404 as the live self-hosted probe suggested). Chi nests the inner `r.Route("/api/expenses", ...)` under the outer `/api` mount AND under the bearer-auth group, so the double-prefix path is reachable for auth-checking purposes but the handler tree is structurally broken. The adversarial sub-test (`/api/api/expenses → 404`) thus serves as a bidirectional gate: it fails on pre-fix code (got 401) and passes only when the relative-prefix fix is in place.

Post-fix GREEN proof (after `git stash pop` restoring the fix):

```
=== RUN   TestBug034003_ExpenseAndMealPlanRoutesMountedBehindAuth
=== RUN   TestBug034003_ExpenseAndMealPlanRoutesMountedBehindAuth/expense_list_mounted_returns_401_without_bearer
2026/05/28 18:58:33 WARN bearer auth failure path=/api/expenses remote_addr=192.0.2.1:1234 reason=missing_token
2026/05/28 18:58:33 INFO request method=GET path=/api/expenses status=401 duration_ms=0
=== RUN   TestBug034003_ExpenseAndMealPlanRoutesMountedBehindAuth/meal_plan_list_mounted_returns_401_without_bearer
2026/05/28 18:58:33 WARN bearer auth failure path=/api/meal-plans remote_addr=192.0.2.1:1234 reason=missing_token
2026/05/28 18:58:33 INFO request method=GET path=/api/meal-plans status=401 duration_ms=0
=== RUN   TestBug034003_ExpenseAndMealPlanRoutesMountedBehindAuth/adversarial_double_prefix_expense_stays_404
2026/05/28 18:58:33 INFO request method=GET path=/api/api/expenses status=404 duration_ms=0
=== RUN   TestBug034003_ExpenseAndMealPlanRoutesMountedBehindAuth/adversarial_double_prefix_meal_plan_stays_404
2026/05/28 18:58:33 INFO request method=GET path=/api/api/meal-plans status=404 duration_ms=0
--- PASS: TestBug034003_ExpenseAndMealPlanRoutesMountedBehindAuth (0.00s)
    --- PASS: TestBug034003_ExpenseAndMealPlanRoutesMountedBehindAuth/expense_list_mounted_returns_401_without_bearer (0.00s)
    --- PASS: TestBug034003_ExpenseAndMealPlanRoutesMountedBehindAuth/meal_plan_list_mounted_returns_401_without_bearer (0.00s)
    --- PASS: TestBug034003_ExpenseAndMealPlanRoutesMountedBehindAuth/adversarial_double_prefix_expense_stays_404 (0.00s)
    --- PASS: TestBug034003_ExpenseAndMealPlanRoutesMountedBehindAuth/adversarial_double_prefix_meal_plan_stays_404 (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/api     0.030s
```

### Existing api-package unit suite — no collateral regression

**Phase:** implement  
**Claim Source:** executed

Command: `go test ./internal/api/ -count=1`

```
ok      github.com/smackerel/smackerel/internal/api     9.482s
```

All pre-existing `TestExpenseList_*`, `TestExpenseCorrect_*`, `TestMealPlanHandler_*` tests still pass. `mealplan_test.go` was updated to mount the handler inside an `r.Route("/api", ...)` wrapper (`mountMealPlanAPI` helper) so the test request paths (`/api/meal-plans/...`) continue to match production composition. `expenses_test.go` invokes handler methods directly (e.g. `h.List(w, req)`) and required no changes.

### `./smackerel.sh check` — config + scenario lint

**Phase:** implement  
**Claim Source:** executed

```
config-validate: ~/smackerel/config/generated/dev.env.tmp.1098085 OK
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 8, rejected: 0
scenario-lint: OK
```

### `./smackerel.sh lint` — repo lint

**Phase:** implement  
**Claim Source:** executed

```
=== Validating JSON manifests ===
  OK: web/pwa/manifest.json
  OK: PWA manifest has required fields
  OK: web/extension/manifest.json
  OK: Chrome extension manifest has required fields (MV3)
  OK: web/extension/manifest.firefox.json
  OK: Firefox extension manifest has required fields (MV2 + gecko)
=== Validating JS syntax ===
  OK: web/pwa/app.js
  OK: web/pwa/sw.js
  OK: web/pwa/lib/queue.js
  OK: web/extension/background.js
  OK: web/extension/popup/popup.js
  OK: web/extension/lib/queue.js
  OK: web/extension/lib/browser-polyfill.js
=== Checking extension version consistency ===
  OK: Extension versions match (1.0.0)
Web validation passed
```

`go vet ./internal/api/...` → exit 0.

### `./smackerel.sh test unit` — full unit suite

**Phase:** implement  
**Claim Source:** executed

Go unit tests + Python pytest (`ml/tests`) both green. Tail:

```
+ pytest ml/tests -q
........................................................................ [ 15%]
........................................................................ [ 31%]
........................................................................ [ 46%]
........................................................................ [ 62%]
........................................................................ [ 77%]
........................................................................ [ 93%]
..............................                                           [100%]
462 passed, 1 warning in 13.08s
+ echo '[py-unit] pytest ml/tests finished OK'
[py-unit] pytest ml/tests finished OK
```

The 1 warning is a third-party StarletteDeprecationWarning about httpx (not introduced by this change; pre-existing in pinned dep).

### `./smackerel.sh test integration` — full integration suite

**Phase:** implement  
**Claim Source:** executed

Ephemeral compose project `smackerel-test` (postgres, nats, smackerel-ml, smackerel-core, ollama) spun up, all 5 services healthy. Full `go test -p 1 -tags integration -v -count=1 -timeout 300s ./tests/integration/... ./internal/notification/... ./internal/assistant/...` ran and finished green. Tail:

```
=== RUN   TestEnforce_PassthroughWhenNotRequired
--- PASS: TestEnforce_PassthroughWhenNotRequired (0.00s)
=== RUN   TestEnforce_EmptyBodyEmptySourcesIsNotAViolation
--- PASS: TestEnforce_EmptyBodyEmptySourcesIsNotAViolation (0.00s)
=== RUN   TestEnforce_UnknownScenarioLabelIsBounded
--- PASS: TestEnforce_UnknownScenarioLabelIsBounded (0.00s)
=== RUN   TestEnforce_AdversarialBypass
--- PASS: TestEnforce_AdversarialBypass (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/assistant/provenance    0.013s
PASS: go-integration
Running project-scoped integration test stack teardown (exit cleanup, timeout 180s)...
```

Test stack containers + volumes + network removed cleanly on exit (test-environment-isolation policy honored). Zero failures, zero warnings, zero pre-existing failures.

### Files modified this session

| File | Change |
|------|--------|
| `internal/api/expenses.go` | `r.Route("/api/expenses", ...)` → `r.Route("/expenses", ...)`; added comment block citing BUG-034-003 |
| `internal/api/mealplan.go` | `r.Route("/api/meal-plans", ...)` → `r.Route("/meal-plans", ...)`; same comment block |
| `internal/api/mealplan_test.go` | Added `mountMealPlanAPI(r, handler)` helper that wraps `RegisterRoutes` inside `r.Route("/api", ...)` to mirror production; replaced 6 bare `handler.RegisterRoutes(r)` call sites |
| `internal/api/router_mount_bug_034_003_test.go` | NEW — regression test with 2 positive (401 mounted) + 2 adversarial (404 double-prefix) sub-tests against full `NewRouter(deps)` composition |
| `specs/034-expense-tracking/bugs/BUG-034-003-.../scopes.md` | DoD checkboxes flipped for in-process items (live self-hosted items remain unchecked pending deploy) |
| `specs/034-expense-tracking/bugs/BUG-034-003-.../report.md` | This evidence section |
| `specs/034-expense-tracking/bugs/BUG-034-003-.../state.json` | Implementation execution claim recorded; status remains `in_progress` until self-hosted redeploy lands |

### Live deployment — REQUIRED, NOT YET PERFORMED

**Phase:** implement  
**Claim Source:** not-run (deploy NOT executed in this session; owner's call)

The live self-hosted image (`smackerel-self-hosted-smackerel-core-1`) still runs the pre-fix binary. The user-visible `/expense` Telegram failure WILL persist until the redeploy lands. To deploy:

```bash
# 1. Build new images (CI-equivalent locally)
./smackerel.sh build

# 2. Promote to self-hosted via build manifest
bash scripts/deploy/promote.sh --target self-hosted --build-manifest <path-to-build-manifest>
#    OR direct adapter invocation (knb deploy overlay):
bash <deployment-owner>/<product>/<target>/apply.sh \
    --image-core=sha256:<new-digest> --image-ml=sha256:<unchanged-digest> \
    --bundle=<env>-<sha> --source-sha=<sha>

# 3. Verify
bash <deployment-owner>/<product>/<target>/verify.sh
```

After deploy, the post-fix live verification steps (live `/api/expenses` returns 401 without bearer, Telegram `/expense` returns empty-state, `/api/meal-plans` returns 401) MUST be captured to flip the remaining live-host DoD items and the `uservalidation.md` checkboxes. Those will be filled by `bubbles.validate` against the live tailnet endpoint AFTER the operator runs the apply step. Recommendation: bubbles.implement signals `route_required → bubbles.validate` with the deploy as a prerequisite owner action.

## Invocation Audit

This triage session did not invoke any subagents. Discovery, log retrieval, code inspection, and bug-artifact creation were all performed directly by `bubbles.bug`. Routing to specialist agents is pending the parent workflow's next dispatch (see "Routing Recommendation" in the result envelope).

This implementation session (2026-05-28T18:55-19:05Z) was performed directly by `bubbles.implement`. No subagents were invoked. Files modified: 2 source (`expenses.go`, `mealplan.go`) + 1 test edit (`mealplan_test.go`) + 1 new test (`router_mount_bug_034_003_test.go`) + 3 bug artifacts (`report.md`, `scopes.md`, `state.json`).

## Audit 2026-06-01

`bubbles.audit` re-ran both governance guards after `bubbles.plan` closed the prior 6 traceability + 2 artifact-lint findings.

**artifact-lint** (`bash .github/bubbles/scripts/artifact-lint.sh specs/034-expense-tracking/bugs/BUG-034-003-expense-query-failed-double-api-prefix`):

```
✅ Required artifact exists: spec.md / design.md / uservalidation.md / state.json / scopes.md / report.md
✅ No forbidden sidecar artifacts present
✅ scopes.md DoD contains checkbox items; all use checkbox syntax
✅ uservalidation checklist contains checkbox entries; has checked-by-default entries
✅ state.json v3 required fields present (status / execution / certification / policySnapshot)
✅ Top-level status matches certification.status
⚠️  state.json v3 missing recommended field: executionHistory (non-blocking)
⚠️  state.json completion phase block uses legacy object format — supported for compatibility
EXIT=0
```

**traceability-guard** (`timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/034-expense-tracking/bugs/BUG-034-003-expense-query-failed-double-api-prefix`):

```
--- Scenario Manifest Cross-Check (G057/G059) ---
✅ scenario-manifest.json covers 4 scenario contract(s)
✅ All linked tests from scenario-manifest.json exist
   (7× internal/api/router_mount_bug_034_003_test.go, 2× uservalidation.md)
✅ scenario-manifest.json records evidenceRefs
✅ Scope 1 scenarios mapped to Test Plan rows and concrete test files:
   - Expense list route is mounted behind auth → router_mount_bug_034_003_test.go
   - Meal-plan list route is mounted behind auth → router_mount_bug_034_003_test.go
   - Adversarial — double-prefix path remains 404 → router_mount_bug_034_003_test.go
RESULT: PASSED (0 warnings)
EXIT=0
```

**Verdict:** ⚠️ SHIP_WITH_NOTES — both governance gates clean (exit 0). The 3 live-host DoD items remain blocked on operator-run self-hosted redeploy; routing to `bubbles.validate` for final certification.
