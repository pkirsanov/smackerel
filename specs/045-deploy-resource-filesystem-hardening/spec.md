# Feature: Deploy Resource and Filesystem Hardening

## Status

In Progress - planning packet created

## Review Findings

- STB-001: Compose/deploy runtime lacks explicit CPU limits for deployment resource classes.
- STB-002: ML model memory planning is not aligned to operator-provided model and resource-class choices.
- SEC-DEP-006: Runtime containers do not yet have a documented read-only-root-filesystem posture with explicit writable mounts.

## Outcome Contract

**Intent:** Make Smackerel deployment resource envelopes and writable filesystem surfaces explicit, enforceable, and testable before deployment.

**Success Signal:** Deploy/runtime configuration declares CPU and memory expectations for core, ML, and supporting services; ML model selection is bounded by a documented deployment resource class; and containers run with read-only root filesystems except for named writable paths that have contract tests.

**Hard Constraints:**

- All configuration values must originate from `config/smackerel.yaml` or generated deployment config.
- No generated config file is edited by hand.
- No target-host adapter behavior is implemented inside Smackerel product source.
- PostgreSQL and NATS data volumes remain writable where required.

**Failure Condition:** A deployment can start with unbounded CPU consumption, an ML model that exceeds the declared resource class, or writable container roots beyond explicit writable paths.

## Requirements

- **FR-045-001:** Product configuration MUST expose service-level resource limits or deploy-contract fields for CPU and memory.
- **FR-045-002:** ML-sidecar model configuration MUST validate the configured model against a documented memory envelope.
- **FR-045-003:** Runtime compose/deploy surfaces MUST express read-only root filesystem intent for services that do not require root writes.
- **FR-045-004:** Every writable path required by core, ML, PostgreSQL, NATS, or Ollama MUST be named explicitly.
- **FR-045-005:** Tests MUST fail if read-only-root or resource limit invariants regress.

## User Scenarios (Gherkin)

```gherkin
Scenario: SCN-045-A01 Operator sees bounded service resources
  Given Smackerel is prepared for deployment
  When the deploy contract or generated runtime config is inspected
  Then core and ML services have explicit CPU and memory envelopes
  And those values originate from the SST configuration pipeline

Scenario: SCN-045-A02 ML model selection fits the memory envelope
  Given the operator configures a model and deployment resource class
  When config validation runs
  Then the selected model is accepted only if the documented memory envelope can support it
  And incompatible model choices fail loudly before runtime start

Scenario: SCN-045-A03 Container roots are read-only except explicit mounts
  Given the runtime stack is generated
  When the service containers start
  Then writable root filesystem access is denied where read-only root is declared
  And required writable directories are backed by explicit tmpfs or named volumes
```

## Product Principle Alignment

This spec supports Principle 6, Invisible By Default, Felt Not Heard, by making runtime limits predictable instead of noisy. It supports Principle 8, Trust Through Transparency, by requiring explicit evidence for resource and filesystem invariants.

## Non-Goals

- Implementing target-host adapter scripts.
- Changing application business logic.
- Selecting a universal production model for every operator environment.
