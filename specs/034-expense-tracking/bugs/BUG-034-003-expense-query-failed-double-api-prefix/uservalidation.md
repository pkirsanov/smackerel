# User Validation: BUG-034-003

## Checklist

### [Bug Fix] BUG-034-003 `/expense` Telegram command returns "Failed to query expenses"
- [ ] **What:** `/expense` Telegram command returns an empty-state reply (or the formatted list) against the live home-lab bot, never "Failed to query expenses".
  - **Steps:**
    1. Open the home-lab Telegram bot in any Telegram client.
    2. Send `/expense`.
  - **Expected:** Reply is an empty-state message (e.g. "No expenses yet") or a formatted expense list. NOT "Failed to query expenses".
  - **Verify:** Manual Telegram send + cross-check `docker logs smackerel-home-lab-smackerel-core-1` shows `GET /api/expenses status=200` (or `status=401` if bot bearer mint regressed — out of scope).
  - **Evidence:** report.md (post-fix section, to be filled by `bubbles.implement`)
  - **Notes:** Unchecked until fix is deployed and verified live. Will flip to `[x]` after `bubbles.validate` confirms the live Telegram round trip.

### [Bug Fix] BUG-034-003 — same defect class on `/api/meal-plans`
- [ ] **What:** `GET /api/meal-plans` (with valid bearer) returns 200 from the live home-lab.
  - **Steps:**
    1. SSH to the home-lab host (`<home-lab-host>`).
    2. `docker exec smackerel-home-lab-smackerel-core-1 wget -O- http://127.0.0.1:8080/api/meal-plans` with a valid bearer.
  - **Expected:** HTTP 200 with JSON body.
  - **Verify:** Status line in wget output.
  - **Evidence:** report.md (post-fix section)
  - **Notes:** Same root cause; the fix must cover both handlers.
