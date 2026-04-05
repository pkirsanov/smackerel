# Managed Docs Contract

Use this file as the single source of truth for how Bubbles-managed docs work.

## Purpose

Bubbles-managed docs are the published documentation surfaces that Bubbles is expected to keep clean, current, deduplicated, and structurally coherent as part of workflow closeout.

Execution artifacts under `specs/` are still required while work is active, but they are not the long-term published truth.

## Published Truth Model

- Execution truth lives in feature, bug, and ops work packets while work is active.
- Published truth lives in the managed docs declared in the effective managed-doc registry.
- Historical truth remains in closed work packets for auditability and evidence.

Do NOT collapse all truth into one giant document. Use one authoritative location per concern.

## Managed Docs Registry

The effective managed-doc registry defines the Bubbles-managed doc set for the current project. Framework defaults live in `bubbles/docs-registry.yaml`, and project-owned overrides may live in `.github/bubbles-project.yaml`.

The registry declares:

- which docs are managed by Bubbles
- which paths they live at
- which work classes publish into them
- which minimal sections they must contain
- whether unmanaged docs require explicit opt-in targeting

## Ownership Boundary

- Bubbles owns cleanliness and truthfulness of managed docs.
- Projects may have additional docs outside the managed set.
- Unmanaged docs are project-owned unless the user explicitly targets them or the project adds them to the registry.

## Publication Rule

When feature or ops work changes durable behavior, configuration, operations, interfaces, or architecture:

1. Keep the execution packet current while work is in flight.
2. Publish durable facts into the impacted managed docs before closeout.
3. Remove obsolete or duplicate material from managed docs instead of appending around it.

## Minimal Structure Rule

Managed docs may remain open-form, but each managed doc type MUST satisfy the minimal structure declared in the registry.

This keeps docs readable and project-shaped without forcing one rigid template across all repositories.

## Validation Rule

Validation and audit must treat managed-doc freshness as a closeout requirement, not an optional cleanup pass.

If managed docs that should have been updated are stale, completion is blocked until `bubbles.docs` publishes the current truth.

## Reserved Execution Namespaces

To preserve discoverability without polluting feature numbering, reserve these namespaces under `specs/`:

- `specs/_ops/` for cross-cutting ops and infrastructure execution packets
- `specs/_bugs/` for cross-cutting bugs not owned by a single feature

Feature-bound bugs may still live under `specs/<feature>/bugs/`.