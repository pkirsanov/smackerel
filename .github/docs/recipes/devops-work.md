# Recipe: DevOps Work

> *"Get the rack humming and keep the park online."*

Use this when the work is operational delivery, not product behavior: CI/CD, build pipelines, deploy automation, monitoring, observability, release plumbing, or health-check wiring.

If the work is cross-cutting and not owned by a single feature, track it under `specs/_ops/OPS-*` instead of forcing it into a feature packet.

If the work needs a dedicated operational execution lane, use `bubbles.devops` or the `devops-to-doc` workflow mode.

## Focused DevOps Execution

```
/bubbles.workflow  devops-to-doc for 042-catalog-assistant

/bubbles.workflow  specs/_ops/OPS-001-ci-hardening mode: devops-to-doc
```

Use when:
- CI is failing but product behavior is otherwise understood
- deployment manifests or release automation need fixes
- monitoring, alerting, or observability wiring needs implementation
- build/release reproducibility needs operational changes

## Direct Specialist Use

```
/bubbles.devops  specs/042-catalog-assistant focus: deploy
/bubbles.devops  specs/042-catalog-assistant focus: monitoring
/bubbles.devops  specs/042-catalog-assistant focus: ci-cd
/bubbles.devops  specs/_ops/OPS-001-ci-hardening focus: ci-cd
```

## Ops Packet Shape

For cross-cutting ops work, keep the packet under `specs/_ops/OPS-*` with:

- `objective.md`
- `design.md`
- `scopes.md`
- `runbook.md`
- `report.md`
- `state.json`

## Stability First, Then DevOps

If you know there are operational symptoms but not the root cause, start with the stability diagnostic lane:

```
/bubbles.workflow  stabilize-to-doc for 042-catalog-assistant
```

That path now diagnoses with `bubbles.stabilize`, then routes operational execution through `bubbles.devops` before the rest of the quality chain runs.

## Not The Same As Framework Ops

Use [Framework Ops](framework-ops.md) when the problem is about Bubbles itself: hooks, gates, upgrades, framework health, or control-plane behavior.

Use this recipe when the problem is inside the target project's operational delivery surfaces.