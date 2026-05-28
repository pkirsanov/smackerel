# BUG-034-003: `/expense` Telegram command returns "Failed to query expenses" — `/api/expenses` route never mounted (double-`/api` prefix in RegisterRoutes)

**Status:** Confirmed
**Severity:** High (P0 — entire expense + meal-plan HTTP API surfaces unreachable)
**Reported:** 2026-05-28 11:44 AM (home-lab)
**Reporter:** User (Telegram bot interaction)
**Parent spec:** `specs/034-expense-tracking/`
**Co-affected spec:** `specs/036-meal-planning/` (identical defect in `internal/api/mealplan.go:28`)

## Failure transcript

| Telegram command | Bot reply | API path called | Live HTTP status |
|---|---|---|---|
| `/expense` | `Failed to query expenses` | `GET /api/expenses` | **404 Not Found** |
| `/concept` | `No concept pages yet` ✓ | `GET /api/knowledge/concepts` | 401 (auth gate) → 200 with bearer |
| `/person` | `No entity profiles yet` ✓ | `GET /api/knowledge/entities` | 401 (auth gate) → 200 with bearer |

Bot reply path (one of four "Failed to query expenses" emit points in `internal/telegram/bot.go`) is line **1216** — the non-200 branch:

```go
if resp.StatusCode != http.StatusOK {
    slog.Warn("expense query non-200", "status", resp.StatusCode)
    b.reply(msg.Chat.ID, "Failed to query expenses")
    return
}
```

## Live evidence (home-lab, 2026-05-28)

```
{"time":"2026-05-28T18:44:24.26713234Z","level":"INFO","msg":"request","method":"GET","path":"/api/expenses","status":404,"duration_ms":0,"request_id":"9623ad891b1f/pbBNO1CDyv-003054"}
{"time":"2026-05-28T18:44:24.267231364Z","level":"WARN","msg":"expense query non-200","status":404}
```

Path-status probe inside the live container:

```
/api/health             ->   HTTP/1.1 200 OK
/api/digest             ->   HTTP/1.1 401 Unauthorized   (route exists, auth gate)
/api/knowledge/concepts ->   HTTP/1.1 401 Unauthorized   (route exists, auth gate)
/api/lists              ->   HTTP/1.1 401 Unauthorized   (route exists, auth gate)
/api/expenses           ->   HTTP/1.1 404 Not Found      ← BUG
/api/expenses/          ->   HTTP/1.1 404 Not Found
/api/api/expenses       ->   HTTP/1.1 404 Not Found
/api/api/expenses/      ->   HTTP/1.1 404 Not Found
/api/meal-plans         ->   HTTP/1.1 404 Not Found      ← SAME BUG, spec 036
/api/api/meal-plans     ->   HTTP/1.1 404 Not Found
```

Startup wiring confirmed non-nil:

```
{"level":"INFO","msg":"expense tracking enabled","default_currency":"USD","export_max_rows":10000}
{"level":"INFO","msg":"meal planning enabled",...}
```

`EXPENSES_ENABLED=true` is present in `app.env` and in the running container env. `deps.ExpenseHandler != nil` is satisfied. The defect is in the route-registration path itself.

## Reproduction steps

1. Send `/expense` to the home-lab Telegram bot.
2. Observe reply: `Failed to query expenses`.
3. SSH to home-lab: `docker exec smackerel-home-lab-smackerel-core-1 wget -S -O /dev/null http://127.0.0.1:8080/api/expenses` → 404.
4. Compare to `/api/digest` → 401 (route exists, auth required).

## Expected behavior

`GET /api/expenses` (with valid bearer) should return 200 with the expense list (empty array when there are no expenses, mirroring the empty-state response of `/concept` and `/person`).

## Severity rationale

- ENTIRE expense HTTP surface (`/api/expenses`, `/api/expenses/export`, `/api/expenses/{id}`, `/api/expenses/{id}/classify`, `/api/expenses/suggestions/{id}/accept|dismiss`) is unreachable from ANY client.
- ENTIRE meal-plan HTTP surface (`/api/meal-plans/...`) is unreachable from ANY client (same defect class).
- Spec 034 was certified `done` and spec 036 was certified `done`. CI passed. The defect has been live since the routes were wired — likely the entire production lifetime of both features.
- Affects Telegram bot, any future web/admin UI, any external API consumer.

## Affected handlers (same defect class)

- `internal/api/expenses.go:38` — `r.Route("/api/expenses", ...)`
- `internal/api/mealplan.go:28` — `r.Route("/api/meal-plans", ...)`
