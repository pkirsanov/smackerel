# Docker Lifecycle Governance

Purpose: define the portable Bubbles-level Docker governance contract for build freshness, cleanup safety, persistent-state protection, validation isolation, and stack grouping.

This file is project-agnostic. Project-specific commands, ports, services, and storage resources belong in repo-local instructions and docs.

## Governing Principles

1. Build fast when inputs are unchanged.
2. Rebuild when source or dependency identity changes.
3. Preserve persistent state by default.
4. Route tests and validation to disposable storage.
5. Group stacks predictably across development, validation, and CI.

## Lifecycle Classes

Every Docker-managed resource should be classified into one lifecycle class.

| Class | Meaning | Default cleanup policy |
|---|---|---|
| `persistent` | Data must survive rebuilds, restarts, and normal cleanup | Never remove automatically |
| `ephemeral` | Disposable runtime state for tests, validation, or short-lived stacks | Remove automatically when no longer needed |
| `cache` | Rebuild acceleration artifacts such as BuildKit or package caches | Prune only under pressure or explicit cleanup |
| `tooling` | Operator tools and debug UIs | Safe to recreate; never treated as stateful data |
| `monitoring` | Observability services and their storage | Preserve by default unless the repo explicitly marks them disposable |

## Labels

Labels are required for automation. Name-based matching may exist as a fallback, but labels are the primary contract.

Projects should use reverse-DNS label keys within their own namespace.

Required logical labels:

- `*.lifecycle.class`
- `*.lifecycle.owner`
- `*.prune.protect`
- `*.data.tier`
- `*.environment`
- `*.stack.group`

Recommended value conventions:

- `lifecycle.class`: `persistent`, `ephemeral`, `cache`, `tooling`, `monitoring`
- `prune.protect`: `true`, `false`
- `data.tier`: `irreplaceable`, `persistent`, `disposable`
- `stack.group`: `infra`, `services`, `frontend`, `admin`, `monitoring`, `tooling`, `validation`

## Freshness Contract

Build skip is allowed only when the system can prove the intended image matches the current workspace identity.

Minimum image identity labels:

- `org.opencontainers.image.revision`
- `*.freshness.source-hash`
- `*.freshness.deps-hash`
- `*.build.time`

Rules:

- `latest` tags and timestamps are not freshness evidence.
- A source change must invalidate a build path that produces new runtime artifacts.
- Deploy/start flows should verify image identity before and after startup.
- `--no-cache` and `--pull` are explicit freshness controls, not routine defaults.

## Cleanup Contract

Default cleanup behavior must be conservative.

Required rules:

- Project-scoped cleanup first.
- System-wide cleanup only by explicit opt-in.
- Volume pruning requires a second explicit opt-in.
- Resources labeled as protected persistent state must be excluded from automated cleanup.
- Cache cleanup should be pressure-driven where possible.

Preferred cleanup order:

1. stopped containers
2. dangling images
3. unused build cache
4. unused networks
5. disposable volumes only if explicitly requested

## Persistent Store Protection

Persistent stores must use named volumes. For stores with lifecycle managed outside the application stack, use external volumes.

Examples of resources that normally qualify:

- primary databases
- secret-management databases and backing stores
- uploaded user content
- curated development datasets

The protected object is the volume, not the container.

## Validation And Test Isolation

Tests and validation must not write into the main persistent development store.

Preferred strategies:

- `tmpfs` for disposable databases or high-churn validation state on Linux
- isolated Compose project names for validation stacks
- dedicated test volumes explicitly marked `ephemeral`

Validation cleanup should remove only validation resources, never the main developer state.

## Grouping And Isolation

Use Compose project names, profiles, and labels as the primary grouping mechanism.

Rules:

- Compose project name isolates concurrent stacks.
- Profiles separate optional groups such as monitoring, tooling, validation, and full-stack services.
- `container_name` may improve operator clarity, but should not be the primary grouping contract.

## Project Responsibilities

Each repo should provide:

- a repo-local instruction describing actual persistent resources
- a repo-local cleanup entrypoint
- a repo-local build entrypoint with freshness verification
- a repo-local description of validation/test storage isolation
- a repo-local mapping of stack groups and Compose profiles

## Non-Goals

- This file does not define repo-specific commands.
- This file does not assign port ranges.
- This file does not prescribe a single implementation language or shell layout.

## Related References

- `agents/bubbles_shared/agent-common.md`
- `agents/bubbles_shared/project-config-contract.md`
- `bubbles/workflows.yaml`