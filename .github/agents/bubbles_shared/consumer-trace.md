# Consumer Trace

Purpose: canonical source for rename/removal dependency-chain rules.

## Rules
- Renamed or removed routes, paths, contracts, identifiers, symbols, and UI targets require a Consumer Impact Sweep.
- Completion is blocked while stale first-party references remain.
- Required surfaces include navigation, breadcrumbs, redirects, API clients, generated clients, docs, config, deep links, and tests.
- Planning must enumerate the affected consumer surfaces.
- Testing and audit must verify stale-reference scans and consumer-facing regressions.
