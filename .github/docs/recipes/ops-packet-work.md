# Recipe: Ops Packet Work

> *"Get the rack humming and keep the park online."*

---

## The Situation

You have infrastructure, CI/CD, deployment, monitoring, or platform work that is real delivery work, but it does not belong inside one feature packet.

Track it under `specs/_ops/OPS-*` so it stays discoverable without pretending it is a product feature.

## The Commands

```
/bubbles.workflow  specs/_ops/OPS-001-ci-hardening mode: devops-to-doc
```

Or direct execution:

```
/bubbles.devops  specs/_ops/OPS-001-ci-hardening focus: ci-cd
```

## Packet Shape

Each cross-cutting ops packet should include:

- `objective.md`
- `design.md`
- `scopes.md`
- `runbook.md`
- `report.md`
- `state.json`

## Use This When

- CI pipelines need repair or hardening
- build or release automation needs work across the whole repo
- deployment, rollback, or health-check flows need changes
- monitoring, dashboards, alerts, or observability wiring are the main deliverable
- the work affects multiple features or the whole platform

## Do Not Use This When

- the change is really a product feature with operational side effects
- the work is only a docs publication pass
- the issue belongs to Bubbles framework internals instead of the target project

For Bubbles framework internals, use [Framework Ops](framework-ops.md).
For feature-bound operational work, [DevOps Work](devops-work.md) is enough.