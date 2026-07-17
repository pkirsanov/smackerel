# Live Deployment Convergence

## Situation

A product is deployed, or nearly deployed, but "the service is running" is not
enough. The target must deliver every intended scenario with real connectors,
required seed data, and browser-visible proof. Product code and target-specific
deployment configuration may live in different repositories.

Use `bubbles.goal` for this mission. It is one outcome with dependent diagnostic,
delivery, action, and verification nodes. Use `bubbles.sprint` only when you have
multiple independent outcomes and a time budget.

## Reusable Prompt

Replace every angle-bracket value. Omit a section only when it genuinely does
not apply.

```text
/bubbles.goal  Converge <product> on <target> to verified live operation.

Repositories:
- Product repository: <product-repo>
- Deployment-owner repository: <deployment-repo>
- Supporting repositories: <repo list or none>

Current state:
- Deployment entry point: <repository-standard deploy command or contract>
- Live base URL: <target URL resolved from deployment config>
- Known failures or unknowns: <facts, not assumptions>

Required live capabilities:
- Required connectors:
  - Keyless: <connector list>
  - Credentialed: <connector list plus logical secret references>
- Required seed entities: <entity type and values>
- Required scenarios: every scenario exposed by the deployed capability set,
  using scenario-manifest.json and current reachable product surfaces as the
  inventory sources.

Success signal:
- The approved immutable artifact is deployed through the owning adapter.
- Every required keyless connector returns real, current data.
- Every required credentialed connector returns real, current data using an
  approved secret reference, with no literal secret in source, logs, chat, or
  runtime evidence.
- Required seed entities exist and are visible through the product.
- Playwright executes the complete deployed journey matrix in autonomous batch
  mode and verifies UI plus the requests each step fires.
- API responses, operate-plane telemetry, and read-only persisted-state checks
  agree with the visible result for every journey.
- Every discovered defect is fixed, redeployed, and replayed until the full
  matrix is clean.

Hard constraints:
- Use each repository's documented command surface and ownership boundaries.
- Keep target-specific hosts, ports, identities, manifests, and secrets only in
  the deployment-owner repository.
- Treat any secret found in a public deployment, browser bundle, git history,
  terminal output, or chat as exposed: do not copy it; rotate it and provision
  the replacement through the approved secret store.
- A value may be reused from a public source only when repository policy
  explicitly classifies it as non-secret.
- Do not substitute mocks, canned responses, screenshots, or health-only checks
  for live connector and journey execution.
- Do not silently skip an unavailable connector or scenario. File and route the
  blocker, then keep the root outcome non-terminal.
- Require pre-mutation approval for deploy, target configuration, and seed-data
  action nodes. Preserve a tested rollback path.
- Production/operate telemetry and data access are read-only. User-requested
  writes are allowed only on a self-owned single-tenant target after approval.

Execution preferences:
- Compile this as a cross-repo goal scenario.
- Preview and lint the scenario DAG before mutation.
- Use parallelScopes: dag-dry first, then parallelScopes: dag with
  maxParallelScopes: 2 only for independent implementation scopes with disjoint
  write surfaces.
- Run bubbles.journey in autonomous batch mode for the final journey matrix.
- Finish only when the root success signal is demonstrated with current
  execution evidence.
```

## Expected Scenario Shape

The goal runner compiles the prompt into existing modes and specialists. It does
not invent a deployment-specific workflow mode.

| Node | Type | Owner | Depends on | Purpose |
|------|------|-------|------------|---------|
| live-baseline | diagnostic | `bubbles.system-review` | - | Inventory the running target, deployed scenarios, connectors, and failures |
| product-remediation | delivery | `full-delivery` in the product repo | live-baseline | Repair product behavior and scenario coverage |
| deployment-remediation | delivery | `devops-to-doc` in the deployment-owner repo | live-baseline | Repair target config, connector wiring, artifact consumption, and rollback |
| predeploy-proof | verification | `validate-only` in each owning repo | both remediation nodes | Prove each repository is ready without cross-repo certification |
| deploy | action | approved deployment surface | predeploy-proof | Deploy the verified immutable artifact |
| seed-data | action | approved product/operator surface | deploy | Create only the requested target data |
| live-journeys | verification | `bubbles.journey` in batch mode | seed-data | Replay every deployed scenario through Playwright and four-layer checks |
| final-proof | verification | `bubbles.validate` | live-journeys | Verify the root outcome rather than only node completion |

Action nodes remain sequential and approval-gated. Read-only/idempotent phases
may fan out under the workflow phase DAG. Independent implementation scopes may
run in parallel worktrees under `parallelScopes: dag`; two writers to the same
scope, shared state, target config, or host singleton never run concurrently.

## Connector Contract

Build a connector matrix before changing configuration:

| Connector | Class | Enablement source | Credential source | Live probe | Result |
|-----------|-------|-------------------|-------------------|------------|--------|
| `<name>` | `keyless` | project config | none | real request + freshness assertion | pass/fail |
| `<name>` | `credentialed` | project config | logical secret reference | real request + freshness assertion | pass/fail |

For a credentialed connector, the agent may verify that a secret reference is
present and use the repository's approved injection path. It must never retrieve
or print the secret value. If the only known copy is public, rotate first.

For a keyless connector, "enabled" means a live request returns valid current
data. A configured flag or healthy process is not sufficient evidence.

## Journey Coverage Contract

1. Inventory stable scenario IDs from `scenario-manifest.json` where present.
2. Reconcile them with the capabilities and routes actually deployed.
3. Build a matrix with scenario ID, precondition, action, expected UI, expected
   API call, telemetry signal, persisted-state assertion, and cleanup rule.
4. Drive UI scenarios with Playwright. Capture the request each action fires.
5. Verify the API response, read-only operate-plane telemetry, and persisted
   state agree with the UI. Explicitly mark genuinely absent layers.
6. Route every issue through its owning bug/spec/ops path. After remediation,
   redeploy and replay the affected scenario plus the full regression matrix.

`bubbles.chaos` may add stochastic journeys after deterministic coverage is
green. It does not replace the declared journey matrix.

## When Not To Use

- Several unrelated backlog items under a deadline: use `bubbles.sprint`.
- One already-planned spec with independent scopes: use `bubbles.workflow` with
  `parallelScopes: dag-dry`, then `parallelScopes: dag`.
- A guided walkthrough that should stop after each user step: use
  `bubbles.journey` without batch mode.
- A deployment target that does not yet have an adapter contract: use
  [Add A Deployment Target](add-deployment-target.md) first.

## Related Recipes

- [Cross-Repo Goal Scenario](cross-repo-scenario.md)
- [Autonomous Goal](autonomous-goal.md)
- [Parallel Scope Execution](parallel-scopes.md)
- [Build-Once Deploy-Many](build-once-deploy-many.md)
- [Guided Journey](guided-journey.md)
- [Coordinate Runtime Leases](runtime-coordination.md)