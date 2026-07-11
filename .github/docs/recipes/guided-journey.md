# <img src="../../icons/cathy-trail.svg" width="28"> Guided Journey

> *"Come on, I'll walk you through it — and tell you straight what's broken."*

Use this when the feature is built and running, and you want to walk the LIVE product toward a real goal WITH the user — capturing friction at each step and routing the fixes to the right owner.

## Problem

The code is done and the tests are green, but nobody has actually walked the finished product toward a concrete user goal — and a green UI can still hide a sick backend. You want to drive the real UI step by step, learn what each step does, and at each step check that it *internally* works — not just that the screen looks right, but that the API returned the right thing, the trace is clean, and the data actually landed. Where it is unclear, inconvenient, missing, broken, or quietly wrong under the hood, you want that turned into planned refinements or a filed defect — without hand-editing specs, designs, or scopes.

## Solution

Start the cooperative-guided walkthrough with an explicit goal and a success signal:

```text
/bubbles.journey  Goal: rebalance my portfolio  (specs/NNN-portfolio-rebalance)
```

`bubbles.journey` (Cathy Curtis) drives the live **dev/validate** stack via the project's configured browser-automation tools + direct API. A journey is *almost a tutorial* — at each step she tells you plainly what the step does and why, and answers your questions before moving on. And at each step she checks **four layers**, because a green screen is not proof the thing worked:

1. **UI** — the visible outcome (browser accessibility snapshot).
2. **API** — the request that step actually fired (`browser_network_requests`) — right status, right payload.
3. **Telemetry** — the step's spans/metrics on the validate-plane Jaeger/Prometheus (resolved via `observability-endpoint-resolve.sh`) — expected spans present, **no error spans**, within SLO.
4. **Data** — a **read-only** check that the state actually landed (DB row / cache key / queue message).

She records each step's outcome as a user-facing friction verdict `works | unclear | inconvenient | missing | broken` **and** an internal verdict `backend-correct | api-mismatch | telemetry-error/missing | data-not-persisted`. "Internally works" means all four layers agree.

**She drives only where it's safe to drive.** Mutations happen ONLY on the dev/validate plane (`env=test`, G115-safe). The operate/prod plane is **read-only** — health, telemetry, and data reads only, never a mutation (INV-12). (Prod-drive is allowed only for a self-owned, single-tenant home-lab or a static local tool, with explicit consent.)

When you want the walkthrough to feed straight into planning, run it as a workflow mode:

```text
/bubbles.workflow  specs/NNN-portfolio-rebalance mode: journey-refinement
```

The mode structures `uservalidation.md` (acceptance stays human-owned — G057; she may also leave a durable, replayable walkthrough there so it doubles as onboarding), appends discovered issues to `report.md`, and routes each refinement — or hidden defect — to its owner.

## Examples

- **Portfolio analytics product** — `Goal: rebalance my portfolio`. Cathy drives the dashboard from the holdings view through the rebalance proposal to the confirmation, and flags that the "apply rebalance" affordance is unclear (friction: `unclear`) → routes a scenario refinement to `bubbles.analyst`.
- **Smackerel** — `Goal: see this month's expenses in QuickBooks`. Cathy walks the connector-linked expense view, notices the month selector defaults to last month (friction: `inconvenient`) and the export-to-QuickBooks button is missing (friction: `missing`) → files the missing-feature refinement to `bubbles.plan` and the defect to `bubbles.bug`.
- **Hidden defect (UI works, backend didn't)** — `Goal: save my rebalance as a draft`. Cathy clicks **Save draft** and the UI flashes "Saved" (friction: `works`). But `browser_network_requests` shows the `POST /drafts` came back `200` with an empty `id`, the validate-plane trace has an **error span** on `draft.persist`, and the read-back query finds **no row**. UI-`works` but internally `data-not-persisted` — a **hidden defect** → routed to `bubbles.bug` with the captured request/response, the trace, and the empty data read as the reproduction.

## When It Helps Most

- The feature is built but never walked toward a real goal
- You want usability friction captured with evidence, not opinions
- You suspect a green UI might be hiding a sick backend — and want the API, trace, and data checked too
- You need refinements routed to the right owner instead of patched inline
- You want the human to accept the experience, not automation

## Good Follow-Ups

- `/bubbles.analyst <feature>` when a friction finding is really a missing requirement
- `/bubbles.plan <feature>` when a refinement needs new scopes, tests, and DoD
- `/bubbles.bug <feature>` when a step exposed an actual defect with a reproduction
- `/bubbles.workflow <feature> mode: readiness-review` when you want a ship/no-ship synthesis after refinements land
