# <img src="../../icons/donna-whistle.svg" width="28"> Recipe: Autonomous Sprint

> *"We're on a schedule, people. Next!"* — Donna

## Situation

You have multiple goals and a fixed time window. Those goals can mix features, bugs, ops cleanup, docs cleanup, or stabilization work. You want the agent to prioritize them, execute as many as possible to full completion, manage the clock, and stop gracefully when time expires.

## Command

```
/bubbles.sprint  minutes: <time_budget>
1. <goal 1>
2. <goal 2>
3. <goal 3>
```

## Examples

### Mixed Feature + Bug Sprint
```
/bubbles.sprint  minutes: 240
1. Fix the calendar sync bug
2. Add the deposit hold/release feature
3. Improve browser E2E coverage for the page builder
```

### Start-of-Day Bug & Cleanup Sprint
```
/bubbles.sprint  minutes: 120
- Fix broken E2E tests for theming
- Stabilize the deploy pipeline and close config drift
- Update API documentation
- Add missing unit tests for booking service
```

### Half-Day Feature Sprint
```
/bubbles.sprint  minutes: 180
1. Implement property search with location filtering
2. Add search results pagination
3. Add search result sorting by price and rating
```

### DevOps & Ops Cleanup Sprint
```
/bubbles.sprint  minutes: 150
1. Fix all Docker build warnings across services
2. Add health check endpoints to services missing them
3. Set up Prometheus metrics collection for the gateway
4. Update deployment documentation
```

### Code Quality Sprint
```
/bubbles.sprint  minutes: 120
1. Eliminate all lint warnings in the booking service
2. Add input validation to all public API endpoints
3. Remove dead code and unused imports across backend
```

### Pre-Release Sprint
```
/bubbles.sprint  minutes: 300
1. Fix all failing E2E tests
2. Close all open bug tickets for the current milestone
3. Update user-facing documentation
4. Run security audit and fix findings
5. Update changelog and release notes
```

### End-of-Week Cleanup Sprint
```
/bubbles.sprint  minutes: 90
- Fix that flaky test in the auth module
- Update the README with the new API endpoints
- Remove the deprecated v1 booking endpoints
- Add missing error handling in the payment flow
```

## What Happens

1. Donna parses all goals, estimates effort (small/medium/large), sorts by priority
2. For each goal (in priority order):
   - Checks the clock — enough time?
   - If yes: executes via `bubbles.goal` convergence loop
   - If no: tries to fit a smaller goal, or stops
3. At wrap-up: produces sprint report (completed / in-progress / not-started)
4. Saves state for resume

## Time Management

- Clock checked before each goal AND each scope
- Current scope always finished completely (never broken state)
- If a large goal won't fit but a small one will, Donna reorders
- Last 15 minutes reserved for wrap-up and docs

## Resume After Interruption

```
/bubbles.sprint  mode: autonomous-sprint resume: true
```

## When To Use

- Start of day: "here are today's priorities, work until lunch"
- Sprint planning: "handle this backlog in the next 4 hours"
- Multiple independent tasks that need attention
- Mixed bug, ops, hardening, and cleanup backlogs that should be time-boxed instead of run forever

## When NOT To Use

- Single focused goal → use `bubbles.goal` instead
- No time pressure → use `bubbles.iterate` instead
- Goals have complex inter-dependencies → use `bubbles.workflow` with explicit ordering

---

*"That one's done. What's next on the board?"*
