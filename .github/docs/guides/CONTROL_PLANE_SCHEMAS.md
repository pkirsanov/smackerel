# Bubbles Control Plane Schemas

This document defines the schema surfaces for the control-plane redesign and active version 3 runtime contract.

Related documents:
- [Control Plane Design](CONTROL_PLANE_DESIGN.md)
- [Control Plane Rollout](CONTROL_PLANE_ROLLOUT.md)
- [Existing Repo Adoption](CONTROL_PLANE_ADOPTION.md)

The version 3 state model, `policySnapshot`, `certification.*`, and `scenario-manifest.json` are active runtime expectations in current Bubbles and are enforced by guard/lint surfaces. Sections that still say "Proposed" should be read as rollout-history or extension notes, not as permission to omit the active control-plane fields.

## Schema Set

The control plane needs fifteen concrete schema surfaces:

1. Agent capability registry
2. Execution policy registry
3. Scenario contract manifest
4. `state.json` version 3
5. Specialist result envelope
6. Transition request packet
7. Rework packet
8. Lockdown approval record
9. Invalidation ledger entry
10. Runtime lease registry
11. Workflow run-state record
12. Framework event log entry
13. Action risk classification registry
14. Project test impact map
15. Project trace contract registry

The newer surfaces above are active runtime or framework surfaces:

- `runtime lease registry` is active
- `workflow run-state` is active at `.specify/runtime/workflow-runs.json`
- `framework event log` is active at `.specify/runtime/framework-events.jsonl`
- `action risk classification registry` is active at `bubbles/action-risk-registry.yaml`
- `project test impact map` is optional project-owned config under `.github/bubbles-project.yaml` or `bubbles-project.yaml` and is consumed by `bubbles/scripts/test-impact-plan.sh`
- `project trace contract registry` is optional project-owned config under `.github/bubbles-project.yaml` or `bubbles-project.yaml` and is consumed by `bubbles/scripts/trace-contract-guard.sh`
- `framework-validate` and `release-check` are operational command surfaces that sit on top of these schemas rather than replacing them

## Extension Surface Notes

### Workflow Run-State Record

Runtime file: `.specify/runtime/workflow-runs.json`

Purpose: describe the active workflow run, pending continuation target, runtime attachment, and retry/resume posture without overloading completion certification fields.

CLI surface:

```text
bubbles run-state
bubbles run-state --active
bubbles run-state --recent
bubbles run-state --all
```

### Framework Event Log Entry

Runtime file: `.specify/runtime/framework-events.jsonl`

Purpose: represent gate outcomes, packet routing, runtime lease transitions, and policy provenance changes as typed events.

CLI surface:

```text
bubbles framework-events
bubbles framework-events --tail 50
bubbles framework-events --type runtime_lease_acquired
```

### Action Risk Classification Registry

Registry file: `bubbles/action-risk-registry.yaml`

Purpose: give framework commands and packets a stable risk vocabulary such as `read_only`, `owned_mutation`, `destructive_mutation`, `external_side_effect`, and `runtime_teardown`.

### Project Test Impact Map

Project file: `.github/bubbles-project.yaml` or `bubbles-project.yaml`

Purpose: let a project declare which changed paths imply which canonical test categories, always-run checks, and full-suite triggers. The map powers G079 impact-aware validation planning.

Minimal schema:

```yaml
testImpact:
  alwaysRun:
    - artifact-lint
  fullSuiteTriggers:
    - "proto/**"
  components:
    api:
      paths:
        - "backend/api/**"
      testCategories:
        - unit
        - integration
        - e2e-api
      alwaysRun:
        - contract-check
```

Invariants:

- optional by default; missing config is a clean no-op unless a caller uses `--require-config`
- narrows or prioritizes the first validation pass only; never removes final workflow gates, mandatory E2E, stress obligations, or user-requested broad validation
- patterns are repo-relative Bash globs
- framework upgrades must not overwrite this project-owned file

### Project Trace Contract Registry

Project file: `.github/bubbles-project.yaml` or `bubbles-project.yaml`

Purpose: let a project declare expected trace/log evidence for important workflows so validation can check runtime observability claims. The registry powers G080 trace-contract evidence checks.

Minimal schema:

```yaml
traceContracts:
  workflows:
    booking.create:
      requiredSpans:
        - name: http.request
          attributes:
            - trace_id
            - booking.id
      requiredAttributes:
        - tenant.id
      requiredInvariants:
        - booking emitted exactly one confirmation event
      redFlags:
        error:
          - Missing trace_id
        warning:
          - slow span
```

Invariants:

- optional by default; missing config is a clean no-op unless a caller uses `--require-config`
- validates actual trace/log output; file inspection or predicted trace output is not evidence
- complements outcome contracts and tests; it does not replace implementation, E2E, or validation proof
- analyst-owned Success Signals remain tech-agnostic, while design/test/validate translate them into trace spans, attributes, and invariants when trace proof is useful

### Observability Posture + Telemetry Endpoints

`traceContracts` is extended with an `observability` child sub-block (NOT a new top-level key — the existing `^traceContracts:` parser is preserved). It declares the repo's observability *posture*, two-plane telemetry *endpoints*, and env-agnostic *SLO* targets. The posture/SLO guards parse this block directly and are the schema authority.

> **Clean cutover (R2-B):** this `observability.endpoints` model REPLACES the v5 `traceContracts.liveTelemetryEndpoints` flat map. The legacy key had no agent or script consumer (an orphan foundation), so it is deleted outright in the same change — there is no deprecation cycle. Every legacy signal maps to an explicit `endpoints.operate.<signal>` entry.

Schema:

```yaml
traceContracts:
  observability:
    schemaVersion: 1        # guards assert this before semantics; unknown version fails loud
    posture: wired          # wired | opted-out — REQUIRED (absent = undeclared = nag)
    policy:
      undeclaredPosture: warn # warn | block
    decision:               # REQUIRED for wired + opted-out
      decidedAt: 2026-06-11
      decidedBy: operator
      decisionSource: "bubbles.setup focus: observability"
      lastReviewedAt: 2026-06-11
    optOut:                 # REQUIRED iff posture: opted-out
      reasonCode: no-runtime  # no-runtime | pre-monitoring | external-monitoring-only
      reason: "framework source repo; nothing to monitor"
      declaredAt: 2026-06-11
      revisitAfter: 2027-06-11
      approvedBy: operator
    endpoints:              # signal axis (4 verbs); adapter NAMES only, never URLs/tokens
      validate:             # resolves to the EPHEMERAL per-run test stack (profile: test)
        alerts: { adapter: none }
        sloBurn: { adapter: prometheus, profile: test }
        errorRate: { adapter: prometheus, profile: test }
        deployImpact: { adapter: none }
      operate:              # resolves to PROD (real URLs live in the deploy overlay env)
        alerts: { adapter: prometheus, profile: prod }
        sloBurn: { adapter: prometheus, profile: prod }
        errorRate: { adapter: prometheus, profile: prod }
        deployImpact: { adapter: prometheus, profile: prod }
    slos:                   # env-agnostic CONTRACT targets
      gateway.request:
        latencyP99Ms: 50
        errorRatePct: 0.1
        availabilityPct: 99.9
  workflows:
    booking.create:
      requiredSpans:
        - name: http.request
          attributes: [trace_id, booking.id]
      slo: gateway.request  # optional link — ignored by the existing G080 guard, read by the SLO guard
```

**Two planes, one provider adapter, a profile per plane.** A signal is selected per plane:

- `endpoints.validate.<signal>` resolves to the ephemeral per-run **test stack** (`profile: test`). Feature scopes use the validate plane only.
- `endpoints.operate.<signal>` resolves to **prod** (`profile: prod`). Prod telemetry queries are read-only and limited to the wired operate-plane consumers: `bubbles.stabilize` (incident diagnosis), `bubbles.upkeep` (the weekly `slo-review` task), and `bubbles.train` (deploy-impact / SLO burn consulted before promote and as a rollback signal). `bubbles.devops` owns the wiring execution; these three agents only fetch through it.

The 4 signals are `alerts`, `sloBurn`, `errorRate`, `deployImpact`. Each value is `{ adapter, profile }`, where `adapter` is a provider **name** matching `bubbles/adapters/observability/<name>.sh`. Adapters ship under that path:

| Adapter | Purpose |
|---------|---------|
| `none`       | Default. No telemetry source wired (returns the neutral empty value per verb). |
| `prometheus` | Reference adapter. Queries a Prometheus HTTP API; the operator/overlay sets the env. |

**Normalized adapter payloads.** Adapters normalize provider-specific responses to these per-verb canonical shapes before returning data to Bubbles (the lint/contract tests validate these shapes, NOT raw provider envelopes). The `none` adapter returns the neutral empty value for each verb (`[]` for alerts, `{}` for the maps):

| Verb | Canonical shape | Neutral (`none`) value |
|------|-----------------|------------------------|
| `fetch-alerts`        | JSON **array** of alert objects `{ id, service, severity, startedAt, summary }` | `[]` |
| `fetch-slo-burn`      | JSON **map** `service.name → burn-rate (float)` | `{}` |
| `fetch-error-rate`    | JSON **map** `service.name → error-pct (float)` | `{}` |
| `fetch-deploy-impact` | JSON **map** `sourceSha → { service, regressionDelta }` | `{}` |

**Profile-to-env binding.** Profile names (`test`, `prod`) are NOT adapter filenames. The plane resolver invokes the provider adapter with profile-specific env materialized into the adapter's native variables. The caller/overlay owns the plane-scoped input env; the resolver maps the selected profile into the adapter-native env. Example for the `prometheus` provider:

| Plane | Profile | Input env owned by caller/overlay | Adapter subprocess env |
|-------|---------|-----------------------------------|------------------------|
| validate | `test` | `BUBBLES_OBS_VALIDATE_PROMETHEUS_BASE_URL`, `BUBBLES_OBS_VALIDATE_PROMETHEUS_QUERY_SLO_BURN`, … | `PROMETHEUS_BASE_URL`, `PROMETHEUS_QUERY_SLO_BURN`, … |
| operate | `prod` | `BUBBLES_OBS_OPERATE_PROMETHEUS_BASE_URL`, `BUBBLES_OBS_OPERATE_PROMETHEUS_QUERY_SLO_BURN`, … | `PROMETHEUS_BASE_URL`, `PROMETHEUS_QUERY_SLO_BURN`, … |

Resolvers fail loud when the selected profile lacks required env, and a `--plane validate` resolution must not read operate-plane env even when prod env exists.

The reference resolver is `bubbles/scripts/observability-endpoint-resolve.sh`: `--plane <validate|operate> --signal <alerts|sloBurn|errorRate|deployImpact>` reads `endpoints.<plane>.<signal>` and materializes the plane-scoped `BUBBLES_OBS_<PLANE>_PROMETHEUS_*` env into adapter-native `PROMETHEUS_*` env on stdout. A `--plane validate` resolution structurally reads ONLY `endpoints.validate.*` and `BUBBLES_OBS_VALIDATE_*` env (prod-block); missing required profile env fails loud (exit 1); a missing `yq` WARN-and-skips to the neutral `adapter=none`. There is no bypass flag.

**Adapter contract (every adapter implements all 4 verbs):** each verb prints its canonical shape on stdout (exit 0); a `none` exit / parse failure is exit 1 (adapter unavailable — NOT a framework failure). Validated by `bubbles/scripts/observability-adapter-lint.sh`.

**Consumer status.** The adapter layer ships the mechanism, the lint, the selftest, and the docs and is **available for adapter authors today**; the runtime consumer that fetches operate-plane telemetry during ops workflows is wired in a later scope. Do not assume a live `bubbles.retro` / `bubbles.stabilize` consumer exists yet.

**Captured-evidence convention.** Test runs deposit telemetry to `.specify/runtime/observability/<workflow>.<signal>.{txt,json}` (trace/log evidence may stay line-oriented text; SLO evidence is normalized JSON — see `project-config-contract.md`). The MCP `record_evidence` path writes the provenance row to `.specify/runtime/tool-calls.jsonl`; the parsed metric artifact under `.specify/runtime/observability/` is the SLO guard's numeric input. `.specify/runtime/` is gitignored.

## 1. Agent Capability Registry

Runtime file: `bubbles/agent-capabilities.yaml`

```yaml
version: 1
generatedFrom:
  - agents/bubbles.*.agent.md
  - bubbles/agent-ownership.yaml
  - bubbles/workflows.yaml
workflowModeGrants:
  defaultAllowed: false
  executionModel: direct-authorized-runner
  topLevelRuntimeRequired: true
  nestedWorkflowRunnerDispatch: forbidden
  agents:
    bubbles.workflow:
      modes: ["*"]
      excludedModes: [autonomous-goal, autonomous-sprint, iterate]
      maxRootModesPerRun: 1
    bubbles.goal:
      modes: ["*"]
      excludedModes: [autonomous-sprint, iterate]
    bubbles.bug:
      modes: [bugfix-fastlane]
    bubbles.releases:
      modes: [release-planning-to-doc]
agents:
  bubbles.workflow:
    class: orchestrator
    canExecuteWorkflowModes: true
    ownsPhases:
      - finalize
    delegatesPhases:
      select: bubbles.iterate
      bootstrap:
        - bubbles.analyst
        - bubbles.ux
        - bubbles.design
        - bubbles.plan
      implement: bubbles.implement
      test: bubbles.test
      regression: bubbles.regression
      docs: bubbles.docs
      validate: bubbles.validate
      audit: bubbles.audit
      chaos: bubbles.chaos
    canAskUserDirectly: false
    mayWriteState:
      execution:
        - activeAgent
        - currentPhase
        - runStartedAt
      certification: []
  bubbles.validate:
    class: certification
    ownsPhases:
      - validate
      - certify-state
    canAskUserDirectly: false
    mayWriteState:
      execution: []
      certification:
        - status
        - completedScopes
        - certifiedCompletedPhases
        - scopeProgress
        - lockdownState
        - invalidationLedger
    mustDelegate:
      planning: bubbles.plan
      design: bubbles.design
      businessRequirements: bubbles.analyst
      ux: bubbles.ux
      implementation: bubbles.implement
      testCoverage: bubbles.test
  bubbles.grill:
    class: interactive-gate
    ownsPhases:
      - interrogate
    canAskUserDirectly: true
    mayWriteState:
      execution:
        - approvalPrompts
      certification: []
```

### Invariants

- generated file only; do not hand-edit
- every specialist phase must resolve to exactly one owning agent or explicit owner chain; `analyze`, `discover`, `bootstrap`, and `finalize` resolve through `activeWorkflowRunner`
- agents must resolve into one primary class; hybrids are not allowed
- certification fields may only be owned by `bubbles.validate`
- `canAskUserDirectly` must be explicit for every agent
- workflow mode execution is default-deny and allowed only by `workflowModeGrants`
- workflow-running orchestrators execute modes only from the top-level runtime and never invoke another workflow runner as a subagent

## 2. Execution Policy Registry

Runtime file: `.specify/memory/bubbles.config.json`

```json
{
  "version": 2,
  "defaults": {
    "grill": {
      "mode": "off",
      "source": "repo-default"
    },
    "tdd": {
      "mode": "scenario-first",
      "defaultForModes": ["bugfix-fastlane", "chaos-hardening"],
      "source": "repo-default"
    },
    "autoCommit": {
      "mode": "off",
      "source": "repo-default"
    },
    "lockdown": {
      "default": false,
      "requireGrillForInvalidation": true,
      "source": "repo-default"
    },
    "regression": {
      "immutability": "protected-scenarios",
      "source": "repo-default"
    },
    "validation": {
      "certificationRequired": true,
      "source": "repo-default"
    },
    "runtime": {
      "leaseTtlMinutes": 20,
      "staleAfterMinutes": 60,
      "reusePolicy": "fingerprint-match-only",
      "source": "repo-default"
    }
  },
  "modeOverrides": {
    "bugfix-fastlane": {
      "tdd": {
        "mode": "scenario-first",
        "source": "workflow-forced"
      }
    },
    "chaos-hardening": {
      "tdd": {
        "mode": "scenario-first",
        "source": "workflow-forced"
      }
    }
  }
}
```

### Proposed CLI Surface

```text
bubbles policy status
bubbles policy get tdd.mode
bubbles policy set tdd.mode scenario-first
bubbles policy set grill.mode required-on-ambiguity
bubbles policy set lockdown.default true
bubbles policy reset grill.mode
```

### Invariants

- repo-local mutable defaults live here, not in agent files
- every effective policy value must preserve provenance
- workflow may override defaults, but the override must be recorded

### Adoption Example: First Control-Plane Run In An Existing Repo

When a repo already has Bubbles framework files but no repo-local policy registry yet, the bootstrap step should add the file without rewriting existing constitutions, command registries, or feature specs.

```json
{
  "version": 2,
  "defaults": {
    "grill": {
      "mode": "off",
      "source": "repo-default"
    },
    "tdd": {
      "mode": "scenario-first",
      "defaultForModes": ["bugfix-fastlane", "chaos-hardening"],
      "source": "repo-default"
    },
    "autoCommit": {
      "mode": "off",
      "source": "repo-default"
    },
    "lockdown": {
      "default": false,
      "requireGrillForInvalidation": true,
      "source": "repo-default"
    },
    "regression": {
      "immutability": "protected-scenarios",
      "source": "repo-default"
    },
    "validation": {
      "certificationRequired": true,
      "source": "repo-default"
    },
    "runtime": {
      "leaseTtlMinutes": 20,
      "staleAfterMinutes": 60,
      "reusePolicy": "fingerprint-match-only",
      "source": "repo-default"
    }
  },
  "modeOverrides": {},
  "metrics": {
    "enabled": false
  }
}
```

Adoption rule:

- adding this file is safe and additive
- repo defaults belong here even when prompts describe the same modes conceptually
- the first control-plane-aware workflow run records the effective values into `state.json.policySnapshot`

## 10. Runtime Lease Registry

Runtime file: `.specify/runtime/resource-leases.json`

```json
{
  "version": 1,
  "leases": [
    {
      "leaseId": "rls_20260401120000_1234",
      "repo": "example-app",
      "sessionId": "workflow-session-a",
      "agent": "bubbles.validate",
      "worktree": "/workspace/example-app",
      "branch": "feature/parallel-runtime",
      "purpose": "validation",
      "environment": "dev",
      "composeProject": "example-app-dev-validation-cmpabc12345",
      "stackGroup": "validation",
      "shareMode": "shared-compatible",
      "compatibilityFingerprint": "sha256:abc123...",
      "resources": "container:example-app-backend|volume:example-app-postgres-data",
      "attachedSessions": "workflow-session-a,workflow-session-b",
      "startedAt": "2026-04-01T12:00:00Z",
      "lastHeartbeatAt": "2026-04-01T12:05:00Z",
      "expiresAt": "2026-04-01T12:25:00Z",
      "weight": 8,
      "status": "active"
    }
  ]
}
```

### CLI Surface

```text
bubbles runtime leases
bubbles runtime summary
bubbles runtime doctor
bubbles runtime acquire --purpose validation --share-mode shared-compatible --fingerprint-file docker-compose.yml
bubbles runtime acquire --purpose build --weight heavy --wait 600
bubbles runtime attach <lease-id>
bubbles runtime heartbeat <lease-id>
bubbles runtime release <lease-id>
bubbles runtime reclaim-stale
```

### Invariants

- `.specify/runtime/` is runtime-generated and stays untracked except for `.gitignore`
- `shared-compatible` reuse requires an exact compatibility fingerprint match
- `exclusive` leases block concurrent acquisition for the same repo/purpose/environment tuple
- weighted admission is opt-in via `runtime.capacityWeight` (`bubbles.config.json` → `runtime`, default `0` = disabled); when `> 0`, `acquire` sums the per-lease `weight` over effectively-active leases and refuses (or `--wait`s) when `active_sum + new_weight` would exceed the budget, so two heavy builds cannot OOM one host. `weightClasses` (default `{ light: 1, medium: 4, heavy: 8 }`) maps `--weight` names to units; `--weight-units N` overrides. `summary`/`doctor` print `Runtime capacity: <active>/<capacityWeight> weight units`
- `doctor` must surface stale leases and active compose/fingerprint conflicts
- status and doctor surfaces may summarize the registry, but the registry itself is the runtime source of truth

## 3. Scenario Contract Manifest

Runtime file: `specs/<feature>/scenario-manifest.json`

```json
{
  "version": 1,
  "featureDir": "specs/042-catalog-assistant",
  "generatedAt": "2026-03-26T12:00:00Z",
  "scenarios": [
    {
      "scenarioId": "SCN-042-001",
      "scope": "02-search-flow",
      "title": "Guest can open the catalog search screen",
      "gherkin": {
        "given": "a guest is on the landing page",
        "when": "the guest opens search",
        "then": "the catalog search screen appears"
      },
      "gherkinHash": "sha256:...",
      "behaviorClass": "ui",
      "changeType": "new",
      "requiredTestType": "e2e-ui",
      "regressionRequired": true,
      "lockdown": false,
      "linkedTests": [
        {
          "file": "dashboard/e2e/tests/catalog-search.spec.ts",
          "testId": "guest-open-search"
        }
      ],
      "evidenceRefs": [
        "report.md#scenario-scn-042-001"
      ],
      "replacedBy": null,
      "invalidatedBy": null
    }
  ]
}
```

### Invariants

- scenario IDs are stable across implementation churn until the behavior contract is explicitly invalidated
- every changed user-visible or external behavior must appear here
- every scenario must point to live-system tests when its behavior class requires it

### Adoption Example: Selective Scenario Lift For An Active Existing Scope

Existing features do not need an all-or-nothing manifest migration. If only one scope is actively changing, only the changed behavior in that scope needs to be lifted into `scenario-manifest.json` immediately.

```json
{
  "version": 1,
  "featureDir": "specs/019-visual-page-builder",
  "generatedAt": "2026-03-27T10:30:00Z",
  "scenarios": [
    {
      "scenarioId": "SCN-019-014",
      "scope": "03-layout-persistence",
      "title": "Host sees the updated section order after reload",
      "gherkin": {
        "given": "a host has reordered page sections",
        "when": "the host reloads the page builder",
        "then": "the saved section order remains visible"
      },
      "gherkinHash": "sha256:...",
      "behaviorClass": "ui",
      "changeType": "changed",
      "requiredTestType": "e2e-ui",
      "regressionRequired": true,
      "lockdown": false,
      "linkedTests": [
        {
          "file": "dashboard/e2e/tests/page-builder.spec.ts",
          "testId": "host-reload-persists-section-order"
        }
      ],
      "evidenceRefs": [
        "report.md#scenario-scn-019-014"
      ],
      "replacedBy": null,
      "invalidatedBy": null
    }
  ]
}
```

Adoption rule:

- do not bulk-invent scenario IDs for untouched historical behavior just to satisfy the new schema
- do lift every active changed user-visible or externally observable behavior into the manifest before certification
- untouched prose scenarios may remain in markdown until that behavior is reopened by a later workflow

## 4. `state.json` Version 3

Runtime file: `specs/<feature>/state.json`

```json
{
  "version": 3,
  "featureDir": "specs/042-catalog-assistant",
  "featureName": "Catalog Assistant",
  "workflowMode": "full-delivery",
  "execution": {
    "activeAgent": "bubbles.workflow",
    "currentPhase": "implement",
    "currentScope": "02-search-flow",
    "runStartedAt": "2026-03-26T12:00:00Z",
    "completedPhaseClaims": ["select", "bootstrap", "implement"],
    "pendingTransitionRequests": ["TR-042-001"]
  },
  "certification": {
    "status": "in_progress",
    "completedScopes": ["01-schema"],
    "certifiedCompletedPhases": ["select", "bootstrap"],
    "scopeProgress": [
      {
        "scope": "01-schema",
        "status": "done",
        "certifiedAt": "2026-03-26T12:15:00Z"
      },
      {
        "scope": "02-search-flow",
        "status": "in_progress",
        "certifiedAt": null
      }
    ],
    "lockdownState": {
      "active": false,
      "lockedScenarioIds": []
    }
  },
  "policySnapshot": {
    "grill": {
      "mode": "required-on-ambiguity",
      "source": "repo-default"
    },
    "tdd": {
      "mode": "scenario-first",
      "source": "workflow-forced"
    }
  },
  "transitionRequests": [
    "TR-042-001"
  ],
  "reworkQueue": [],
  "executionHistory": []
}
```

### Invariants

- `execution` records claims and in-flight state
- `certification` records authoritative state
- only `bubbles.validate` may mutate `certification`
- promotion to `done` is impossible without validate certification

### Adoption Example: Legacy State To Version 3 Migration

Many active specs already have a legacy state shape where a single top-level status and ad hoc completed phase lists act as both execution trace and completion authority. The migration must separate those concerns.

Legacy example:

```json
{
  "status": "done",
  "completedPhases": ["implement", "test", "docs"],
  "completedScopes": ["01-api", "02-ui"]
}
```

Migrated version 3 example:

```json
{
  "version": 3,
  "workflowMode": "full-delivery",
  "execution": {
    "activeAgent": "bubbles.workflow",
    "currentPhase": "validate",
    "currentScope": null,
    "runStartedAt": "2026-03-27T11:00:00Z",
    "completedPhaseClaims": ["implement", "test", "docs"],
    "pendingTransitionRequests": []
  },
  "certification": {
    "status": "in_progress",
    "completedScopes": ["01-api"],
    "certifiedCompletedPhases": ["implement", "test"],
    "scopeProgress": [
      {
        "scope": "01-api",
        "status": "done",
        "certifiedAt": "2026-03-27T10:55:00Z"
      },
      {
        "scope": "02-ui",
        "status": "in_progress",
        "certifiedAt": null
      }
    ],
    "lockdownState": {
      "active": false,
      "lockedScenarioIds": []
    }
  },
  "policySnapshot": {
    "grill": {
      "mode": "off",
      "source": "repo-default"
    },
    "tdd": {
      "mode": "scenario-first",
      "source": "repo-default"
    },
    "validation": {
      "certificationRequired": true,
      "source": "repo-default"
    }
  },
  "transitionRequests": [],
  "reworkQueue": [],
  "executionHistory": []
}
```

Migration rule:

- move claims of work performed into `execution.completedPhaseClaims`
- let `bubbles.validate` decide what survives into `certification.*`
- if old `done` state is not fully defensible, the migrated `certification.status` must reopen to `in_progress` or `blocked` rather than preserving a false green state

## 5. Specialist Result Envelope

Proposed payload: returned by every agent or mapped-mode phase invocation.

```json
{
  "resultId": "RES-042-001",
  "agent": "bubbles.gaps",
  "roleClass": "diagnostic",
  "outcome": "route_required",
  "featureDir": "specs/042-catalog-assistant",
  "scopeIds": ["02-search-flow"],
  "dodItems": ["DOD-02-04"],
  "scenarioIds": ["SCN-042-002"],
  "artifactsCreated": [],
  "artifactsUpdated": ["report.md"],
  "evidenceRefs": [
    "report.md#gap-finding-scn-042-002"
  ],
  "nextRequiredOwner": "bubbles.implement",
  "packetRef": "RW-042-001",
  "blockedReason": null
}
```

### Invariants

- every agent invocation must return exactly one result envelope
- valid outcomes are `completed_owned`, `completed_diagnostic`, `route_required`, and `blocked`
- only owners or execution specialists may return `completed_owned`
- diagnostic and certification agents may return `completed_diagnostic`, `route_required`, or `blocked`
- `route_required` must reference a concrete packet or embedded packet payload
- `blocked` must carry a concrete reason plus evidence references

## 6. Transition Request Packet

Runtime storage: embedded in state or stored under `specs/<feature>/transitions/`

```json
{
  "transitionRequestId": "TR-042-001",
  "requestedBy": "bubbles.implement",
  "requestedAt": "2026-03-26T12:20:00Z",
  "target": {
    "kind": "scope",
    "id": "02-search-flow",
    "requestedStatus": "done"
  },
  "basis": {
    "dodItems": ["DOD-02-03", "DOD-02-04"],
    "scenarioIds": ["SCN-042-001", "SCN-042-002"],
    "evidenceRefs": [
      "report.md#scope-02-evidence"
    ]
  },
  "status": "pending-validation"
}
```

### Invariants

- execution agents may request promotion
- only validate may resolve the request as approved or rejected
- a request without evidence refs is invalid

## 7. Rework Packet

Runtime storage: embedded in state or stored under `specs/<feature>/rework/`

```json
{
  "reworkId": "RW-042-001",
  "createdBy": "bubbles.validate",
  "createdAt": "2026-03-26T12:30:00Z",
  "reason": "scenario-proof-missing",
  "owner": "bubbles.test",
  "scope": "02-search-flow",
  "dodItems": ["DOD-02-04"],
  "scenarioIds": ["SCN-042-002"],
  "requiredActions": [
    "add failing targeted e2e-ui proof",
    "link the test to SCN-042-002",
    "re-run validation"
  ],
  "narrowExecutionContext": {
    "files": ["dashboard/e2e/tests/catalog-search.spec.ts"],
    "functions": [],
    "commands": ["E2E_UI_TEST_COMMAND"],
    "workflowMode": null
  },
  "status": "open"
}
```

### Invariants

- validate never reopens work without a concrete packet
- route-required outcomes must include an owner and scenario or DoD references
- workflow must not report phase success while open rework packets remain
- diagnostic agents use narrow execution context instead of fixing inline

## 8. Lockdown Approval Record

Runtime file: `specs/<feature>/lockdown-approvals.json`

```json
{
  "approvalId": "LKA-042-001",
  "scenarioId": "SCN-042-001",
  "requestedBy": "bubbles.workflow",
  "approvedVia": "bubbles.grill",
  "approvedAt": "2026-03-26T12:40:00Z",
  "approvedBy": "user",
  "reason": "Product behavior intentionally changing for new checkout flow",
  "replacementScenarioId": "SCN-042-017"
}
```

### Invariants

- only locked scenarios require this record
- approval alone is not enough; it must pair with invalidation and replacement planning

## 9. Invalidation Ledger Entry

Runtime file: `specs/<feature>/invalidation-ledger.json`

```json
{
  "invalidationId": "INV-042-001",
  "scenarioId": "SCN-042-001",
  "invalidatedAt": "2026-03-26T12:45:00Z",
  "invalidatedBy": "bubbles.validate",
  "approvedBy": "LKA-042-001",
  "reason": "Approved behavior change",
  "replacementScenarioId": "SCN-042-017",
  "affectedTests": [
    "dashboard/e2e/tests/catalog-search.spec.ts::guest-open-search"
  ]
}
```

### Invariants

- protected regression tests may only change after invalidation exists
- only validate may certify invalidation
- invalidation must point to the replacement scenario when behavior is replaced, not removed entirely

## Schema Relationships

The schemas work together in this order:

1. capability registry decides ownership, role class, and child-workflow privileges
2. policy registry resolves defaults and provenance
3. scenario manifest defines behavior contracts
4. every agent returns a specialist result envelope
5. execution agents write transition requests
6. validate certifies or rejects through state version 3
7. rejected transitions create rework packets
8. lockdown approvals and invalidation entries govern protected scenario changes

## Adoption Bundle For Existing Repos

An existing repo becomes control-plane-ready when the following schema bundle is present or intentionally introduced during the first migration pass:

1. `.specify/memory/bubbles.config.json` exists
2. active migrated specs use `state.json` version 3 with separate `execution` and `certification`
3. active changed behavior is represented in `scenario-manifest.json`
4. `policySnapshot` is recorded on each control-plane-aware run
5. transition and rework packets are used instead of narrative-only reopen instructions

This bundle is intentionally incremental. Historical untouched specs do not need immediate full conversion, but any spec being actively changed must enter this schema set before it can be certified complete.

## Minimum Mechanical Enforcement Needed

These schemas become meaningful only when paired with mechanical enforcement:

- capability registry lint
- role-class and no-hybrid guard
- policy provenance guard
- scenario manifest guard
- result-envelope completeness guard
- validate-only certification guard
- lockdown guard
- regression immutability guard
- rework packet completeness guard
- child-workflow-depth guard

Without those guards, the schemas would remain descriptive instead of authoritative.