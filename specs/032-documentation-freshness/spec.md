# Feature: 032 — Documentation Freshness & Operational Guides

## Problem Statement

Smackerel's documentation has drifted from the implemented reality. `docs/Development.md` doesn't mention the 3 newest packages (`internal/domain/`, `internal/annotation/`, `internal/list/`) or migrations 015-017. There are no documented system requirements (the stack needs ~11.3GB RAM + ~9GB disk). There is no operational runbook for common tasks (restart stuck connector, re-run failed migration, force re-process artifact, set up TLS). This scored 7/10 in the system review.

## Outcome Contract

**Intent:** All managed docs reflect the current state of the codebase. New operators can deploy and troubleshoot without reading source code. System requirements are explicit.

**Success Signal:** A new operator reads README.md and knows they need 16GB RAM. They follow the ops runbook to restart a stuck connector. Development.md lists all 40+ packages with their purpose.

**Hard Constraints:**
- Documentation must be verified against real codebase state (no copy-paste from memory)
- System requirements must be measured, not estimated
- Ops runbook procedures must be tested against the real stack
- No documentation of planned-but-unimplemented features

**Failure Condition:** If docs describe features that don't exist or omit features that do, they actively mislead. If system requirements are wrong, users deploy on underpowered hardware and blame the system.

## Goals

1. Update README.md with system requirements (RAM, disk, Docker version)
2. Update docs/Development.md with all current packages, migrations, and prompt contracts
3. Create docs/Operations.md with common operational procedures
4. Document TLS setup for network-exposed deployments
5. Document connector troubleshooting (restart, re-sync, disable)

## Non-Goals

- API reference documentation (auto-generated from code in future)
- User tutorial or onboarding guide (product-level, not ops)
- Architecture decision records (ADRs)
- Changelog generation

## User Scenarios (Gherkin)

```gherkin
Scenario: New operator checks system requirements
  Given a person is evaluating Smackerel for self-hosted deployment
  When they read the README.md
  Then they find minimum RAM, disk space, Docker version, and OS requirements

Scenario: Developer finds package documentation
  Given a developer wants to understand the annotation system
  When they read docs/Development.md
  Then they find internal/annotation/ listed with its purpose and key types

Scenario: Operator restarts a stuck connector
  Given a connector has stopped syncing
  When the operator reads docs/Operations.md
  Then they find step-by-step instructions to restart the connector via API

Scenario: Operator sets up TLS
  Given an operator wants to expose Smackerel to their home network
  When they read the TLS section of docs/Operations.md
  Then they find instructions for reverse proxy setup with Caddy or nginx
```

## Acceptance Criteria

- [ ] README.md contains system requirements section (minimum/recommended RAM, disk, Docker version)
- [ ] docs/Development.md lists all Go packages under internal/ with one-line descriptions
- [ ] docs/Development.md lists all migrations (001-017) with purpose
- [ ] docs/Development.md lists all prompt contracts with purpose
- [ ] docs/Operations.md exists with sections: deployment, connector management, troubleshooting, TLS setup, backup/restore
- [ ] All documented commands are verified against the real stack
- [ ] No documentation references unimplemented features
